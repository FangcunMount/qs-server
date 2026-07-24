#!/usr/bin/env bash
set -Eeuo pipefail

NGINX_CONTAINER="${NGINX_CONTAINER:-nginx}"
NGINX_MIN_VERSION="${NGINX_MIN_VERSION:-1.27.3}"
NGINX_CONFIG_DEST="${NGINX_CONFIG_DEST:-/data/apps/nginx-configs/collect.conf}"
NGINX_CONFIG_LEGACY_DEST="${NGINX_CONFIG_LEGACY_DEST:-/data/apps/nginx-configs/collect.fangcunmount.cn.conf}"
NGINX_CONFIG_BACKUP_DIR="${NGINX_CONFIG_BACKUP_DIR:-/opt/backups/qs-server/qs-collection-server}"
COLLECTION_COMPOSE_PROJECT="${COLLECTION_COMPOSE_PROJECT:-qs-collection}"
COLLECTION_COMPOSE_SERVICE="${COLLECTION_COMPOSE_SERVICE:-server}"
COLLECTION_NETWORK="${COLLECTION_NETWORK:-infra-network}"
COLLECTION_DNS_NAME="${COLLECTION_DNS_NAME:-qs-collection-server}"
EXPECTED_COLLECTION_REPLICAS="${EXPECTED_COLLECTION_REPLICAS:-2}"
PUBLIC_HEALTH_URL="${PUBLIC_HEALTH_URL:-https://collect.fangcunmount.cn/health}"
PUBLIC_HEALTH_RESOLVE="${PUBLIC_HEALTH_RESOLVE:-collect.fangcunmount.cn:443:127.0.0.1}"
ROUTING_PROBE_REQUESTS="${ROUTING_PROBE_REQUESTS:-40}"
PRIVILEGE_RUNNER="${PRIVILEGE_RUNNER:-sudo}"

require_privilege_runner() {
  if ! command -v "$PRIVILEGE_RUNNER" >/dev/null 2>&1; then
    echo "Privilege runner is unavailable: $PRIVILEGE_RUNNER" >&2
    return 1
  fi
}

run_privileged() {
  "$PRIVILEGE_RUNNER" "$@"
}

version_at_least() {
  local current="$1"
  local minimum="$2"
  local -a current_parts minimum_parts
  local i current_part minimum_part

  if ! [[ "$current" =~ ^[0-9]+([.][0-9]+){0,2}$ ]]; then
    echo "Invalid current version: $current" >&2
    return 2
  fi
  if ! [[ "$minimum" =~ ^[0-9]+([.][0-9]+){0,2}$ ]]; then
    echo "Invalid minimum version: $minimum" >&2
    return 2
  fi

  IFS=. read -r -a current_parts <<<"$current"
  IFS=. read -r -a minimum_parts <<<"$minimum"
  for i in 0 1 2; do
    current_part="${current_parts[$i]:-0}"
    minimum_part="${minimum_parts[$i]:-0}"
    if ((10#$current_part > 10#$minimum_part)); then
      return 0
    fi
    if ((10#$current_part < 10#$minimum_part)); then
      return 1
    fi
  done
  return 0
}

require_positive_integer() {
  local name="$1"
  local value="$2"
  if ! [[ "$value" =~ ^[0-9]+$ ]] || [ "$value" -lt 1 ]; then
    echo "$name must be a positive integer, got: $value" >&2
    return 1
  fi
}

nginx_version() {
  local version_output
  version_output="$(run_privileged docker exec "$NGINX_CONTAINER" nginx -v 2>&1)"
  version_output="${version_output##*nginx/}"
  printf '%s\n' "${version_output%%[[:space:]]*}"
}

preflight() {
  require_privilege_runner
  require_positive_integer EXPECTED_COLLECTION_REPLICAS "$EXPECTED_COLLECTION_REPLICAS"
  require_positive_integer ROUTING_PROBE_REQUESTS "$ROUTING_PROBE_REQUESTS"

  if [ "$(run_privileged docker inspect "$NGINX_CONTAINER" --format '{{.State.Running}}' 2>/dev/null || true)" != "true" ]; then
    echo "Nginx container $NGINX_CONTAINER is not running" >&2
    return 1
  fi

  local current_version
  current_version="$(nginx_version)"
  if ! version_at_least "$current_version" "$NGINX_MIN_VERSION"; then
    echo "Nginx ${current_version} is below required ${NGINX_MIN_VERSION}" >&2
    return 1
  fi
  echo "Nginx version gate passed (${current_version} >= ${NGINX_MIN_VERSION})"
}

collection_container_ids() {
  run_privileged docker ps \
    --filter "label=com.docker.compose.project=${COLLECTION_COMPOSE_PROJECT}" \
    --filter "label=com.docker.compose.service=${COLLECTION_COMPOSE_SERVICE}" \
    --filter status=running \
    --format '{{.ID}}'
}

collection_ip_set() {
  local container_ids="$1"
  local container_id
  while IFS= read -r container_id; do
    [ -z "$container_id" ] && continue
    run_privileged docker inspect "$container_id" \
      --format "{{with index .NetworkSettings.Networks \"${COLLECTION_NETWORK}\"}}{{.IPAddress}}{{end}}"
  done <<<"$container_ids" | sed '/^$/d' | sort -u
}

nginx_dns_ip_set() {
  run_privileged docker exec "$NGINX_CONTAINER" getent ahostsv4 "$COLLECTION_DNS_NAME" 2>/dev/null |
    awk '$1 ~ /^[0-9]+([.][0-9]+){3}$/ {print $1}' |
    sort -u
}

verify_effective_config() {
  local effective upstream_count upstream
  effective="$(run_privileged docker exec "$NGINX_CONTAINER" nginx -T 2>&1)"
  upstream_count="$(
    printf '%s\n' "$effective" |
      grep -Ec '^[[:space:]]*upstream[[:space:]]+collect-api[[:space:]]*\{' ||
      true
  )"
  if [ "$upstream_count" -ne 1 ]; then
    echo "Effective Nginx config has ${upstream_count} collect-api upstream blocks, want 1" >&2
    return 1
  fi

  upstream="$(
    printf '%s\n' "$effective" |
      awk '
        /^[[:space:]]*upstream[[:space:]]+collect-api[[:space:]]*\{/ {
          inside = 1
        }
        inside {
          print
          if ($0 ~ /^[[:space:]]*}[[:space:]]*$/) {
            exit
          }
        }
      '
  )"
  for required in \
    'zone collect_api 64k;' \
    'resolver 127.0.0.11 valid=10s ipv6=off;' \
    'resolver_timeout 5s;' \
    'server qs-collection-server:8080 resolve;'; do
    if ! grep -Fq "$required" <<<"$upstream"; then
      echo "Effective collect-api upstream is missing: $required" >&2
      return 1
    fi
  done
  if grep -Eq '(^|[[:space:]])ip_hash[[:space:]]*;|weight[[:space:]]*=|(^|[[:space:]])backup([[:space:];]|$)' <<<"$upstream"; then
    echo "Effective collect-api upstream contains a forbidden sticky, weighted, or backup policy" >&2
    return 1
  fi
  echo "Effective Nginx collect-api upstream contract passed"
}

verify_dns() {
  local container_ids container_count expected_ips resolved_ips attempt
  container_ids="$(collection_container_ids)"
  container_count="$(printf '%s\n' "$container_ids" | sed '/^$/d' | wc -l | tr -d '[:space:]')"
  if [ "$container_count" -ne "$EXPECTED_COLLECTION_REPLICAS" ]; then
    echo "Running collection replicas: ${container_count}/${EXPECTED_COLLECTION_REPLICAS}" >&2
    return 1
  fi
  expected_ips="$(collection_ip_set "$container_ids")"
  if [ "$(printf '%s\n' "$expected_ips" | sed '/^$/d' | wc -l | tr -d '[:space:]')" -ne "$EXPECTED_COLLECTION_REPLICAS" ]; then
    echo "Collection container IP set does not contain ${EXPECTED_COLLECTION_REPLICAS} unique addresses:" >&2
    printf '%s\n' "$expected_ips" >&2
    return 1
  fi

  resolved_ips=""
  for attempt in $(seq 1 20); do
    resolved_ips="$(nginx_dns_ip_set || true)"
    if [ "$resolved_ips" = "$expected_ips" ]; then
      echo "Docker DNS resolves ${COLLECTION_DNS_NAME} to both collection replicas:"
      printf '%s\n' "$resolved_ips"
      return 0
    fi
    sleep 1
  done

  echo "Docker DNS result does not match collection replica addresses" >&2
  echo "Expected:" >&2
  printf '%s\n' "$expected_ips" >&2
  echo "Resolved from Nginx:" >&2
  printf '%s\n' "$resolved_ips" >&2
  return 1
}

health_request_metric() {
  local container_id="$1"
  run_privileged docker exec "$container_id" wget -qO- http://127.0.0.1:8080/metrics |
    awk '
      /^gin_requests_total\{/ &&
      /code="200"/ &&
      /method="GET"/ &&
      /url="\/health"/ {
        total += $NF
      }
      END {
        printf "%.0f\n", total + 0
      }
    '
}

verify_request_distribution() {
  local container_ids container_id container_name before_value after_value delta total_delta request_number
  local -A before=()

  container_ids="$(collection_container_ids)"
  while IFS= read -r container_id; do
    [ -z "$container_id" ] && continue
    before["$container_id"]="$(health_request_metric "$container_id")"
  done <<<"$container_ids"

  for request_number in $(seq 1 "$ROUTING_PROBE_REQUESTS"); do
    curl --fail --silent --show-error \
      --connect-timeout 5 \
      --max-time 10 \
      --noproxy '*' \
      --resolve "$PUBLIC_HEALTH_RESOLVE" \
      "$PUBLIC_HEALTH_URL" >/dev/null
  done

  total_delta=0
  while IFS= read -r container_id; do
    [ -z "$container_id" ] && continue
    before_value="${before[$container_id]}"
    after_value="$(health_request_metric "$container_id")"
    delta=$((after_value - before_value))
    container_name="$(run_privileged docker inspect "$container_id" --format '{{.Name}}')"
    container_name="${container_name#/}"
    echo "Collection routing metric ${container_name}: before=${before_value} after=${after_value} delta=${delta}"
    if [ "$delta" -le 0 ]; then
      echo "Collection replica ${container_name} received no /health requests through Nginx" >&2
      return 1
    fi
    total_delta=$((total_delta + delta))
  done <<<"$container_ids"

  if [ "$total_delta" -lt "$ROUTING_PROBE_REQUESTS" ]; then
    echo "Observed /health delta ${total_delta}, want at least ${ROUTING_PROBE_REQUESTS}" >&2
    return 1
  fi
  echo "Nginx request distribution passed (${total_delta} observed requests across all replicas)"
}

verify_runtime() {
  preflight
  verify_effective_config
  verify_dns
  verify_request_distribution
}

PREVIOUS_CONFIG_PATH=""
BACKUP_CONFIG_PATH=""

restore_previous_config() {
  run_privileged rm -f "$NGINX_CONFIG_DEST"
  if [ -n "$BACKUP_CONFIG_PATH" ] && [ -n "$PREVIOUS_CONFIG_PATH" ]; then
    run_privileged rsync -a "$BACKUP_CONFIG_PATH" "$PREVIOUS_CONFIG_PATH"
    run_privileged chmod 0644 "$PREVIOUS_CONFIG_PATH"
  fi
}

rollback_config() {
  local original_status="$1"
  trap - ERR
  echo "Collection Nginx verification failed; restoring previous config" >&2
  if ! restore_previous_config; then
    echo "CRITICAL: failed to restore the previous Nginx configuration" >&2
  elif ! run_privileged docker exec "$NGINX_CONTAINER" nginx -t; then
    echo "CRITICAL: restored Nginx configuration does not pass nginx -t" >&2
  elif ! run_privileged docker exec "$NGINX_CONTAINER" nginx -s reload; then
    echo "CRITICAL: failed to reload restored Nginx configuration" >&2
  else
    echo "Previous Nginx configuration restored"
  fi
  exit "$original_status"
}

install_and_verify() {
  preflight
  : "${NGINX_CONFIG_SOURCE:?NGINX_CONFIG_SOURCE is required for install-and-verify}"
  if [ ! -r "$NGINX_CONFIG_SOURCE" ]; then
    echo "Nginx config source is not readable: $NGINX_CONFIG_SOURCE" >&2
    return 1
  fi
  if [ "$NGINX_CONFIG_DEST" != "$NGINX_CONFIG_LEGACY_DEST" ] &&
    [ -e "$NGINX_CONFIG_DEST" ] &&
    [ -e "$NGINX_CONFIG_LEGACY_DEST" ]; then
    echo "Both canonical and legacy collection Nginx configs exist; refusing ambiguous replacement" >&2
    return 1
  fi

  run_privileged mkdir -p "$(dirname "$NGINX_CONFIG_DEST")" "$NGINX_CONFIG_BACKUP_DIR"
  local timestamp
  timestamp="$(date +%Y%m%d_%H%M%S)"
  if [ -e "$NGINX_CONFIG_DEST" ]; then
    PREVIOUS_CONFIG_PATH="$NGINX_CONFIG_DEST"
  elif [ "$NGINX_CONFIG_DEST" != "$NGINX_CONFIG_LEGACY_DEST" ] &&
    [ -e "$NGINX_CONFIG_LEGACY_DEST" ]; then
    PREVIOUS_CONFIG_PATH="$NGINX_CONFIG_LEGACY_DEST"
  fi
  if [ -n "$PREVIOUS_CONFIG_PATH" ]; then
    BACKUP_CONFIG_PATH="${NGINX_CONFIG_BACKUP_DIR}/collect-nginx-${timestamp}.conf"
    run_privileged rsync -a "$PREVIOUS_CONFIG_PATH" "$BACKUP_CONFIG_PATH"
    run_privileged chmod 0600 "$BACKUP_CONFIG_PATH"
    echo "Backed up ${PREVIOUS_CONFIG_PATH} to ${BACKUP_CONFIG_PATH}"
  fi

  trap 'rollback_config $?' ERR
  if [ "$PREVIOUS_CONFIG_PATH" = "$NGINX_CONFIG_LEGACY_DEST" ]; then
    run_privileged rm -f "$NGINX_CONFIG_LEGACY_DEST"
  fi
  # Local rsync writes through a temporary file and renames it into place,
  # keeping the Nginx config replacement atomic without elevating this script.
  run_privileged rsync -a "$NGINX_CONFIG_SOURCE" "$NGINX_CONFIG_DEST"
  run_privileged chmod 0644 "$NGINX_CONFIG_DEST"

  run_privileged docker exec "$NGINX_CONTAINER" nginx -t
  run_privileged docker exec "$NGINX_CONTAINER" nginx -s reload
  verify_runtime
  trap - ERR
  echo "Collection Nginx config installed and verified: $NGINX_CONFIG_DEST"
}

usage() {
  cat <<'EOF'
Usage:
  verify-collection-nginx.sh preflight
  verify-collection-nginx.sh install-and-verify
  verify-collection-nginx.sh verify-only
  verify-collection-nginx.sh --version-at-least CURRENT MINIMUM
EOF
}

case "${1:-}" in
  --version-at-least)
    if [ "$#" -ne 3 ]; then
      usage >&2
      exit 2
    fi
    version_at_least "$2" "$3"
    ;;
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
    usage >&2
    exit 2
    ;;
esac
