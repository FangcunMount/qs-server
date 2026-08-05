#!/usr/bin/env bash
set -Eeuo pipefail

APISERVER_CONTAINER="${APISERVER_CONTAINER:-qs-apiserver}"
WORKER_DNS_NAME="${WORKER_DNS_NAME:-qs-worker-governance}"
EXPECTED_WORKER_REPLICAS="${EXPECTED_WORKER_REPLICAS:-3}"
WORKER_GOVERNANCE_PORT="${WORKER_GOVERNANCE_PORT:-9092}"
WORKER_GOVERNANCE_TIMEOUT="${WORKER_GOVERNANCE_TIMEOUT:-3}"
DNS_RETRY_ATTEMPTS="${DNS_RETRY_ATTEMPTS:-20}"
DNS_RETRY_INTERVAL_SECONDS="${DNS_RETRY_INTERVAL_SECONDS:-1}"
PRIVILEGE_RUNNER="${PRIVILEGE_RUNNER:-sudo}"

require_positive_integer() {
  local name="$1"
  local value="$2"
  if ! [[ "$value" =~ ^[0-9]+$ ]] || [ "$value" -lt 1 ]; then
    echo "$name must be a positive integer, got: $value" >&2
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
    resilience_instance="$(printf '%s' "$resilience_snapshot" | json_string_field instance_id)"
    if [ -z "$resilience_instance" ] || [ "$resilience_instance" != "$redis_instance" ]; then
      echo "Worker ${ip} governance instance_id mismatch" >&2
      return 1
    fi

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
  require_positive_integer DNS_RETRY_INTERVAL_SECONDS "$DNS_RETRY_INTERVAL_SECONDS"

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
    sleep "$DNS_RETRY_INTERVAL_SECONDS"
  done

  if [ "$resolved_count" -ne "$EXPECTED_WORKER_REPLICAS" ]; then
    echo "Docker DNS ${WORKER_DNS_NAME} returned ${resolved_count} unique IPv4 addresses, want ${EXPECTED_WORKER_REPLICAS}" >&2
    printf '%s\n' "$resolved_ips" >&2
    return 1
  fi

  echo "Docker DNS ${WORKER_DNS_NAME} resolved the expected worker endpoints:"
  printf '%s\n' "$resolved_ips"
  verify_worker_snapshots "$resolved_ips"
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
