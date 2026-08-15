#!/usr/bin/env bash
set -Eeuo pipefail

WORKER_ENV_FILE="${WORKER_ENV_FILE:?WORKER_ENV_FILE is required}"
WORKER_IMAGE_REF="${WORKER_IMAGE_REF:?WORKER_IMAGE_REF is required}"
WORKER_NETWORK="${WORKER_NETWORK:-infra-network}"
WORKER_DEPENDENCY_TIMEOUT_SECONDS="${WORKER_DEPENDENCY_TIMEOUT_SECONDS:-5}"
PRIVILEGE_RUNNER="${PRIVILEGE_RUNNER:-sudo}"

require_positive_integer() {
  local name="$1"
  local value="$2"
  if ! [[ "$value" =~ ^[0-9]+$ ]] || [ "$value" -lt 1 ]; then
    echo "$name must be a positive integer, got: $value" >&2
    return 1
  fi
}

env_value() {
  local name="$1"
  run_privileged sed -n "s/^${name}=//p" "$WORKER_ENV_FILE" | tail -n 1
}

parse_endpoint() {
  local name="$1"
  local endpoint="$2"
  local host port

  case "$endpoint" in
    ""|*[[:space:]]*)
      echo "$name must be a non-empty host:port endpoint" >&2
      return 1
      ;;
  esac
  host="${endpoint%:*}"
  port="${endpoint##*:}"
  if [ -z "$host" ] || [ "$host" = "$endpoint" ]; then
    echo "$name must use host:port format, got: $endpoint" >&2
    return 1
  fi
  require_positive_integer "${name} port" "$port"
  printf '%s\n%s\n' "$host" "$port"
}

run_privileged() {
  "$PRIVILEGE_RUNNER" "$@"
}

verify_tcp_dependency() {
  local name="$1"
  local endpoint="$2"
  local parsed host port
  parsed="$(parse_endpoint "$name" "$endpoint")"
  host="$(printf '%s\n' "$parsed" | sed -n '1p')"
  port="$(printf '%s\n' "$parsed" | sed -n '2p')"

  echo "Preflighting worker dependency ${name} at ${host}:${port} from ${WORKER_NETWORK}..."
  if ! run_privileged docker run --rm \
    --network "$WORKER_NETWORK" \
    --entrypoint /bin/sh \
    "$WORKER_IMAGE_REF" \
    -c 'protocol_byte=$({ sleep "$3"; } | nc -w "$3" "$1" "$2" | od -An -tu1 -N5 | awk "{print \$5}"); test "$protocol_byte" = 10' sh \
    "$host" "$port" "$WORKER_DEPENDENCY_TIMEOUT_SECONDS"; then
    echo "Worker dependency preflight failed: ${name} ${host}:${port} did not return a MySQL greeting through ${WORKER_NETWORK}" >&2
    return 1
  fi
  echo "Worker dependency preflight passed: ${name} ${host}:${port}"
}

verify() {
  if ! command -v "$PRIVILEGE_RUNNER" >/dev/null 2>&1; then
    echo "Privilege runner is unavailable: $PRIVILEGE_RUNNER" >&2
    return 1
  fi
  if ! run_privileged test -r "$WORKER_ENV_FILE"; then
    echo "Worker environment file is unreadable: $WORKER_ENV_FILE" >&2
    return 1
  fi
  require_positive_integer WORKER_DEPENDENCY_TIMEOUT_SECONDS "$WORKER_DEPENDENCY_TIMEOUT_SECONDS"

  # MySQL became a startup-critical worker dependency with the durable runtime
  # ledger migration. Probe it before Compose replaces healthy old replicas.
  verify_tcp_dependency QS_WORKER_MYSQL_HOST "$(env_value QS_WORKER_MYSQL_HOST)"
}

usage() {
  echo "Usage: verify-worker-dependencies.sh [verify]" >&2
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
