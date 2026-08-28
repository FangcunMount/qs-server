#!/usr/bin/env bash
set -Eeuo pipefail

NGINX_CONTAINER="${NGINX_CONTAINER:-nginx}"
NGINX_CONFIG_SOURCE="${NGINX_CONFIG_SOURCE:-}"
NGINX_CONFIG_DEST="${NGINX_CONFIG_DEST:-/data/apps/nginx-configs/perf-observability.conf}"
NGINX_CONFIG_BACKUP_DIR="${NGINX_CONFIG_BACKUP_DIR:-/opt/backups/qs-server/qs-collection-server}"
PUBLIC_COLLECTION_METRICS_URL="${PUBLIC_COLLECTION_METRICS_URL:-https://collect.fangcunmount.cn/perf/metrics}"
PUBLIC_COLLECTION_READY_URL="${PUBLIC_COLLECTION_READY_URL:-https://collect.fangcunmount.cn/perf/readyz}"
PUBLIC_WORKER_METRICS_URL="${PUBLIC_WORKER_METRICS_URL:-https://worker.fangcunmount.cn/metrics}"
PUBLIC_WORKER_READY_URL="${PUBLIC_WORKER_READY_URL:-https://worker.fangcunmount.cn/readyz}"
PUBLIC_NSQD_STATS_URL="${PUBLIC_NSQD_STATS_URL:-https://nsqd.fangcunmount.cn/stats}"
EXPECTED_COLLECTION_REPLICAS="${EXPECTED_COLLECTION_REPLICAS:-2}"
EXPECTED_WORKER_REPLICAS="${EXPECTED_WORKER_REPLICAS:-3}"
READY_PROBE_ATTEMPTS="${READY_PROBE_ATTEMPTS:-12}"
READY_PROBE_INTERVAL_SECONDS="${READY_PROBE_INTERVAL_SECONDS:-5}"
VERIFY_PUBLIC_ROUTES="${VERIFY_PUBLIC_ROUTES:-true}"
PRIVILEGE_RUNNER="${PRIVILEGE_RUNNER:-sudo}"

run_privileged() {
  "$PRIVILEGE_RUNNER" "$@"
}

require_privilege_runner() {
  if ! command -v "$PRIVILEGE_RUNNER" >/dev/null 2>&1; then
    echo "Privilege runner is unavailable: $PRIVILEGE_RUNNER" >&2
    return 1
  fi
}

require_positive_integer() {
  local name="$1"
  local value="$2"
  case "$value" in
    ''|*[!0-9]*|0)
      echo "$name must be a positive integer, got: $value" >&2
      return 1
      ;;
  esac
}

require_nonnegative_integer() {
  local name="$1"
  local value="$2"
  case "$value" in
    ''|*[!0-9]*)
      echo "$name must be a non-negative integer, got: $value" >&2
      return 1
      ;;
  esac
}

preflight() {
  require_privilege_runner
  if [ "$(run_privileged docker inspect "$NGINX_CONTAINER" --format '{{.State.Running}}' 2>/dev/null || true)" != "true" ]; then
    echo "Nginx container $NGINX_CONTAINER is not running" >&2
    return 1
  fi
  run_privileged docker exec "$NGINX_CONTAINER" nginx -t
}

verify_internal_dns() {
  local name
  for name in prometheus nsqd nsqlookupd; do
    if ! run_privileged docker exec "$NGINX_CONTAINER" getent ahostsv4 "$name" >/dev/null 2>&1; then
      echo "Nginx cannot resolve observability upstream ${name} on its Docker networks" >&2
      return 1
    fi
  done
  echo "Nginx observability upstream DNS passed"
}

verify_effective_config() {
  local effective
  effective="$(run_privileged docker exec "$NGINX_CONTAINER" nginx -T 2>&1)"
  local required
  for required in \
    'server_name worker.fangcunmount.cn;' \
    'server_name nsqd.fangcunmount.cn;' \
    'http://prometheus:9090' \
    'http://nsqd:4151' \
    'http://nsqlookupd:4161' \
    'location = /stats' \
    'location = /nodes'; do
    if ! grep -Fq "$required" <<<"$effective"; then
      echo "Effective Nginx observability config is missing: $required" >&2
      return 1
    fi
  done
  echo "Effective Nginx observability route contract passed"
}

probe_url() {
  local url="$1"
  local host="$2"
  local expected="$3"
  local body
  body="$(mktemp)"
  if ! curl --fail --silent --show-error \
    --connect-timeout 5 \
    --max-time 15 \
    --noproxy '*' \
    --resolve "${host}:443:127.0.0.1" \
    "$url" >"$body"; then
    rm -f "$body"
    return 1
  fi
  if ! grep -Fq "$expected" "$body"; then
    echo "Observability probe ${url} did not contain ${expected}" >&2
    rm -f "$body"
    return 1
  fi
  rm -f "$body"
}

probe_ready_replicas_once() {
  local url="$1"
  local host="$2"
  local expected="$3"
  local body ready
  body="$(mktemp)"
  if ! curl --fail --silent --show-error \
    --connect-timeout 5 \
    --max-time 15 \
    --noproxy '*' \
    --resolve "${host}:443:127.0.0.1" \
    "$url" >"$body"; then
    rm -f "$body"
    return 1
  fi
  ready="$(sed -n 's/.*"value":\[[^,]*,"\([0-9][0-9]*\)"\].*/\1/p' "$body" | head -n 1)"
  rm -f "$body"
  if [ "$ready" != "$expected" ]; then
    echo "Observability probe ${url} reported ready replicas=${ready:-invalid}, expected=${expected}" >&2
    return 1
  fi
}

probe_ready_replicas() {
  local url="$1"
  local host="$2"
  local expected="$3"
  local attempt
  for attempt in $(seq 1 "$READY_PROBE_ATTEMPTS"); do
    if probe_ready_replicas_once "$url" "$host" "$expected"; then
      if [ "$attempt" -gt 1 ]; then
        echo "Observability probe ${url} converged after ${attempt}/${READY_PROBE_ATTEMPTS} attempts"
      fi
      return 0
    fi
    if [ "$attempt" -lt "$READY_PROBE_ATTEMPTS" ]; then
      echo "Observability probe ${url} not converged, retrying (${attempt}/${READY_PROBE_ATTEMPTS})" >&2
      sleep "$READY_PROBE_INTERVAL_SECONDS"
    fi
  done
  echo "Observability probe ${url} did not converge after ${READY_PROBE_ATTEMPTS} attempts" >&2
  return 1
}

verify_public_routes() {
  case "$EXPECTED_COLLECTION_REPLICAS:$EXPECTED_WORKER_REPLICAS" in
    *[!0-9:]*|0:*|*:0|:*|*:)
      echo "Expected replica counts must be positive integers" >&2
      return 1
      ;;
  esac
  require_positive_integer READY_PROBE_ATTEMPTS "$READY_PROBE_ATTEMPTS"
  require_nonnegative_integer READY_PROBE_INTERVAL_SECONDS "$READY_PROBE_INTERVAL_SECONDS"
  probe_url "$PUBLIC_COLLECTION_METRICS_URL" collect.fangcunmount.cn 'qs_runtime_component_ready'
  probe_ready_replicas "$PUBLIC_COLLECTION_READY_URL" collect.fangcunmount.cn "$EXPECTED_COLLECTION_REPLICAS"
  probe_url "$PUBLIC_WORKER_METRICS_URL" worker.fangcunmount.cn 'qs_runtime_component_ready'
  probe_ready_replicas "$PUBLIC_WORKER_READY_URL" worker.fangcunmount.cn "$EXPECTED_WORKER_REPLICAS"
  probe_url "$PUBLIC_NSQD_STATS_URL" nsqd.fangcunmount.cn '"topics"'
  echo "Public K6 observability route probes passed"
}

verify_runtime() {
  preflight
  verify_effective_config
  verify_internal_dns
  if [ "$VERIFY_PUBLIC_ROUTES" = "true" ]; then
    verify_public_routes
  else
    echo "Public observability probes deferred until all application deployments finish"
  fi
}

PREVIOUS_CONFIG_PATH=""
BACKUP_CONFIG_PATH=""

restore_previous_config() {
  run_privileged rm -f "$NGINX_CONFIG_DEST"
  if [ -n "$BACKUP_CONFIG_PATH" ]; then
    run_privileged rsync -a "$BACKUP_CONFIG_PATH" "$NGINX_CONFIG_DEST"
    run_privileged chmod 0644 "$NGINX_CONFIG_DEST"
  fi
}

rollback_config() {
  local original_status="$1"
  trap - ERR
  echo "Observability Nginx verification failed; restoring previous config" >&2
  if ! restore_previous_config; then
    echo "CRITICAL: failed to restore the previous observability Nginx config" >&2
  elif ! run_privileged docker exec "$NGINX_CONTAINER" nginx -t; then
    echo "CRITICAL: restored Nginx configuration does not pass nginx -t" >&2
  elif ! run_privileged docker exec "$NGINX_CONTAINER" nginx -s reload; then
    echo "CRITICAL: failed to reload restored Nginx configuration" >&2
  else
    echo "Previous observability Nginx config restored"
  fi
  exit "$original_status"
}

install_and_verify() {
  preflight
  : "${NGINX_CONFIG_SOURCE:?NGINX_CONFIG_SOURCE is required for install-and-verify}"
  if [ ! -r "$NGINX_CONFIG_SOURCE" ]; then
    echo "Nginx observability config source is not readable: $NGINX_CONFIG_SOURCE" >&2
    return 1
  fi

  run_privileged mkdir -p "$(dirname "$NGINX_CONFIG_DEST")" "$NGINX_CONFIG_BACKUP_DIR"
  local timestamp
  timestamp="$(date +%Y%m%d_%H%M%S)"
  if [ -e "$NGINX_CONFIG_DEST" ]; then
    PREVIOUS_CONFIG_PATH="$NGINX_CONFIG_DEST"
    BACKUP_CONFIG_PATH="${NGINX_CONFIG_BACKUP_DIR}/perf-observability-nginx-${timestamp}.conf"
    run_privileged rsync -a "$PREVIOUS_CONFIG_PATH" "$BACKUP_CONFIG_PATH"
    run_privileged chmod 0600 "$BACKUP_CONFIG_PATH"
    echo "Backed up ${PREVIOUS_CONFIG_PATH} to ${BACKUP_CONFIG_PATH}"
  fi

  trap 'rollback_config $?' ERR
  run_privileged rsync -a "$NGINX_CONFIG_SOURCE" "$NGINX_CONFIG_DEST"
  run_privileged chmod 0644 "$NGINX_CONFIG_DEST"
  run_privileged docker exec "$NGINX_CONTAINER" nginx -t
  run_privileged docker exec "$NGINX_CONTAINER" nginx -s reload
  verify_runtime
  trap - ERR
  echo "Observability Nginx config installed and verified: $NGINX_CONFIG_DEST"
}

case "${1:-verify-only}" in
  preflight)
    preflight
    ;;
  install-and-verify)
    install_and_verify
    ;;
  verify-only)
    verify_runtime
    ;;
  *)
    echo "Usage: verify-observability-nginx.sh preflight|install-and-verify|verify-only" >&2
    exit 2
    ;;
esac
