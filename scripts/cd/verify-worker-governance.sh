#!/usr/bin/env bash
set -Eeuo pipefail

APISERVER_CONTAINER="${APISERVER_CONTAINER:-qs-apiserver}"
WORKER_DNS_NAME="${WORKER_DNS_NAME:-qs-worker-governance}"
EXPECTED_WORKER_REPLICAS="${EXPECTED_WORKER_REPLICAS:-3}"
WORKER_GOVERNANCE_PORT="${WORKER_GOVERNANCE_PORT:-9092}"
WORKER_GOVERNANCE_TIMEOUT="${WORKER_GOVERNANCE_TIMEOUT:-3}"
DNS_RETRY_ATTEMPTS="${DNS_RETRY_ATTEMPTS:-20}"
DNS_RETRY_INTERVAL_SECONDS="${DNS_RETRY_INTERVAL_SECONDS:-1}"
GOVERNANCE_RETRY_ATTEMPTS="${GOVERNANCE_RETRY_ATTEMPTS:-20}"
GOVERNANCE_RETRY_INTERVAL_SECONDS="${GOVERNANCE_RETRY_INTERVAL_SECONDS:-3}"
PRIVILEGE_RUNNER="${PRIVILEGE_RUNNER:-sudo}"

require_positive_integer() {
  local name="$1"
  local value="$2"
  if ! [[ "$value" =~ ^[0-9]+$ ]] || [ "$value" -lt 1 ]; then
    echo "$name must be a positive integer, got: $value" >&2
    return 1
  fi
}

require_nonnegative_integer() {
  local name="$1"
  local value="$2"
  if ! [[ "$value" =~ ^[0-9]+$ ]]; then
    echo "$name must be a non-negative integer, got: $value" >&2
    return 1
  fi
}

require_privilege_runner() {
  if ! command -v "$PRIVILEGE_RUNNER" >/dev/null 2>&1; then
    echo "Privilege runner is unavailable: $PRIVILEGE_RUNNER" >&2
    return 1
  fi
}

run_privileged() {
  "$PRIVILEGE_RUNNER" "$@"
}

worker_dns_ip_set() {
  run_privileged docker exec "$APISERVER_CONTAINER" getent ahostsv4 "$WORKER_DNS_NAME" 2>/dev/null |
    awk '$1 ~ /^[0-9]+([.][0-9]+){3}$/ {print $1}' |
    sort -u
}

json_string_field() {
  local field="$1"
  sed -n "s/.*\"${field}\":\"\\([^\"]*\\)\".*/\\1/p" | head -n 1
}

fetch_snapshot() {
  local ip="$1"
  local path="$2"
  run_privileged docker exec "$APISERVER_CONTAINER" \
    wget -qO- -T "$WORKER_GOVERNANCE_TIMEOUT" \
    "http://${ip}:${WORKER_GOVERNANCE_PORT}${path}"
}

assert_resilience_lock_capability() {
  local snapshot="$1"
  local name="$2"
  local kind="$3"
  local ttl_seconds="$4"
  local renew_every_seconds="$5"
  local capability
  capability="$(
    printf '%s' "$snapshot" |
      tr -d '\n' |
      grep -o "\"name\":\"${name}\"[^}]*" |
      head -n 1 || true
  )"
  if [ -z "$capability" ]; then
    echo "Worker resilience snapshot is missing lock capability ${name}" >&2
    return 1
  fi
  local expected
  for expected in \
    "\"kind\":\"${kind}\"" \
    '"strategy":"redis_lease"' \
    '"configured":true' \
    '"degraded":false' \
    "\"ttl_seconds\":${ttl_seconds}" \
    '"renewal_mode":"auto"' \
    "\"renew_every_seconds\":${renew_every_seconds}"; do
    if ! grep -Fq "$expected" <<<"$capability"; then
      echo "Worker lock capability ${name} is not healthy: missing ${expected}" >&2
      return 1
    fi
  done
}

verify_worker_snapshots() {
  local resolved_ips="$1"
  local ip redis_snapshot resilience_snapshot redis_instance resilience_instance
  local instance_ids=""

  while IFS= read -r ip; do
    [ -z "$ip" ] && continue

    if ! redis_snapshot="$(fetch_snapshot "$ip" /governance/redis)"; then
      echo "Worker ${ip} Redis governance snapshot is unreachable" >&2
      return 1
    fi
    if ! grep -Fq '"ready":true' <<<"$redis_snapshot"; then
      echo "Worker ${ip} Redis governance snapshot is not ready" >&2
      return 1
    fi
    redis_instance="$(printf '%s' "$redis_snapshot" | json_string_field instance_id)"
    if [ -z "$redis_instance" ]; then
      echo "Worker ${ip} Redis governance snapshot has no instance_id" >&2
      return 1
    fi

    if ! resilience_snapshot="$(fetch_snapshot "$ip" /governance/resilience)"; then
      echo "Worker ${ip} resilience governance snapshot is unreachable" >&2
      return 1
    fi
    if ! grep -Fq '"summary":{"ready":true' <<<"$resilience_snapshot"; then
      echo "Worker ${ip} resilience governance snapshot is not ready" >&2
      return 1
    fi
    resilience_instance="$(printf '%s' "$resilience_snapshot" | json_string_field instance_id)"
    if [ -z "$resilience_instance" ] || [ "$resilience_instance" != "$redis_instance" ]; then
      echo "Worker ${ip} governance instance_id mismatch" >&2
      return 1
    fi
    assert_resilience_lock_capability "$resilience_snapshot" answersheet_processing duplicate_suppression 300 100
    assert_resilience_lock_capability "$resilience_snapshot" attention_projection_reconcile leader 1800 600

    printf 'Worker governance endpoint %s instance=%s ready\n' "$ip" "$redis_instance"
    instance_ids="${instance_ids}${redis_instance}"$'\n'
  done <<<"$resolved_ips"

  local unique_instance_count
  unique_instance_count="$(printf '%s' "$instance_ids" | sed '/^$/d' | sort -u | wc -l | tr -d '[:space:]')"
  if [ "$unique_instance_count" -ne "$EXPECTED_WORKER_REPLICAS" ]; then
    echo "Worker governance returned ${unique_instance_count} unique instances, want ${EXPECTED_WORKER_REPLICAS}" >&2
    return 1
  fi
}

verify() {
  require_privilege_runner
  require_positive_integer EXPECTED_WORKER_REPLICAS "$EXPECTED_WORKER_REPLICAS"
  require_positive_integer WORKER_GOVERNANCE_PORT "$WORKER_GOVERNANCE_PORT"
  require_positive_integer WORKER_GOVERNANCE_TIMEOUT "$WORKER_GOVERNANCE_TIMEOUT"
  require_positive_integer DNS_RETRY_ATTEMPTS "$DNS_RETRY_ATTEMPTS"
  require_nonnegative_integer DNS_RETRY_INTERVAL_SECONDS "$DNS_RETRY_INTERVAL_SECONDS"
  require_positive_integer GOVERNANCE_RETRY_ATTEMPTS "$GOVERNANCE_RETRY_ATTEMPTS"
  require_nonnegative_integer GOVERNANCE_RETRY_INTERVAL_SECONDS "$GOVERNANCE_RETRY_INTERVAL_SECONDS"

  if [ "$(run_privileged docker inspect "$APISERVER_CONTAINER" --format '{{.State.Running}}' 2>/dev/null || true)" != "true" ]; then
    echo "API server container $APISERVER_CONTAINER is not running" >&2
    return 1
  fi

  local resolved_ips=""
  local resolved_count=0
  local attempt
  for attempt in $(seq 1 "$DNS_RETRY_ATTEMPTS"); do
    resolved_ips="$(worker_dns_ip_set || true)"
    resolved_count="$(printf '%s\n' "$resolved_ips" | sed '/^$/d' | wc -l | tr -d '[:space:]')"
    if [ "$resolved_count" -eq "$EXPECTED_WORKER_REPLICAS" ]; then
      break
    fi
    if [ "$attempt" -lt "$DNS_RETRY_ATTEMPTS" ]; then
      sleep "$DNS_RETRY_INTERVAL_SECONDS"
    fi
  done

  if [ "$resolved_count" -ne "$EXPECTED_WORKER_REPLICAS" ]; then
    echo "Docker DNS ${WORKER_DNS_NAME} returned ${resolved_count} unique IPv4 addresses, want ${EXPECTED_WORKER_REPLICAS}" >&2
    printf '%s\n' "$resolved_ips" >&2
    return 1
  fi

  local snapshot_output=""
  local snapshot_error=""
  local governance_ready=0
  for attempt in $(seq 1 "$GOVERNANCE_RETRY_ATTEMPTS"); do
    # Re-resolve on every attempt because Compose may replace worker IPs while
    # the replicas are converging.
    resolved_ips="$(worker_dns_ip_set || true)"
    resolved_count="$(printf '%s\n' "$resolved_ips" | sed '/^$/d' | wc -l | tr -d '[:space:]')"
    if [ "$resolved_count" -ne "$EXPECTED_WORKER_REPLICAS" ]; then
      snapshot_error="Docker DNS ${WORKER_DNS_NAME} returned ${resolved_count} unique IPv4 addresses during governance readiness, want ${EXPECTED_WORKER_REPLICAS}"
    elif snapshot_output="$(verify_worker_snapshots "$resolved_ips" 2>&1)"; then
      governance_ready=1
      break
    else
      snapshot_error="$snapshot_output"
    fi

    if [ "$attempt" -lt "$GOVERNANCE_RETRY_ATTEMPTS" ]; then
      sleep "$GOVERNANCE_RETRY_INTERVAL_SECONDS"
    fi
  done

  if [ "$governance_ready" -ne 1 ]; then
    echo "Worker governance endpoints did not become ready after ${GOVERNANCE_RETRY_ATTEMPTS} attempts" >&2
    printf '%s\n' "$snapshot_error" >&2
    return 1
  fi

  echo "Docker DNS ${WORKER_DNS_NAME} resolved the expected worker endpoints:"
  printf '%s\n' "$resolved_ips"
  printf '%s\n' "$snapshot_output"
  echo "Worker governance reverse verification passed (${EXPECTED_WORKER_REPLICAS} endpoints)"
}

usage() {
  echo "Usage: verify-worker-governance.sh [verify]" >&2
}

case "${1:-verify}" in
  verify)
    verify
    ;;
  *)
    usage
    exit 2
    ;;
esac
