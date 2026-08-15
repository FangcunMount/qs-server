#!/usr/bin/env bash
set -Eeuo pipefail

EXPECTED_WORKER_REPLICAS="${EXPECTED_WORKER_REPLICAS:-3}"
WORKER_COMPOSE_PROJECT="${WORKER_COMPOSE_PROJECT:-qs-worker}"
WORKER_COMPOSE_SERVICE="${WORKER_COMPOSE_SERVICE:-runtime}"
WORKER_COMPOSE_FILE="${WORKER_COMPOSE_FILE:?WORKER_COMPOSE_FILE is required}"
WORKER_COMPOSE_ENV_FILE="${WORKER_COMPOSE_ENV_FILE:?WORKER_COMPOSE_ENV_FILE is required}"
WORKER_IMAGE_TAG="${WORKER_IMAGE_TAG:?WORKER_IMAGE_TAG is required}"
WORKER_READY_URL="${WORKER_READY_URL:-http://127.0.0.1:9092/readyz}"
WORKER_READY_ATTEMPTS="${WORKER_READY_ATTEMPTS:-60}"
WORKER_READY_INTERVAL_SECONDS="${WORKER_READY_INTERVAL_SECONDS:-3}"
WORKER_READY_TIMEOUT_SECONDS="${WORKER_READY_TIMEOUT_SECONDS:-3}"
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

run_privileged() {
  "$PRIVILEGE_RUNNER" "$@"
}

docker_compose() {
  run_privileged docker compose \
    --env-file "$WORKER_COMPOSE_ENV_FILE" \
    -p "$WORKER_COMPOSE_PROJECT" \
    -f "$WORKER_COMPOSE_FILE" \
    "$@"
}

running_worker_container_ids() {
  docker_compose ps --status running -q "$WORKER_COMPOSE_SERVICE"
}

all_worker_container_ids() {
  docker_compose ps -a -q "$WORKER_COMPOSE_SERVICE"
}

worker_image_matches() {
  local container_id="$1"
  local running_image
  running_image="$(run_privileged docker inspect "$container_id" --format '{{.Config.Image}}' 2>/dev/null || true)"
  case "$running_image" in
    *:"${WORKER_IMAGE_TAG}")
      return 0
      ;;
  esac
  return 1
}

worker_is_ready() {
  local container_id="$1"
  local response
  if ! response="$(run_privileged docker exec "$container_id" \
    wget -qO- -T "$WORKER_READY_TIMEOUT_SECONDS" "$WORKER_READY_URL" 2>/dev/null)"; then
    return 1
  fi
  grep -Fq '"status":"ready"' <<<"$response"
}

show_failure_diagnostics() {
  local container_ids container_id

  echo "Worker deployment state:" >&2
  docker_compose ps "$WORKER_COMPOSE_SERVICE" >&2 || true

  container_ids="$(all_worker_container_ids || true)"
  while IFS= read -r container_id; do
    [ -z "$container_id" ] && continue
    run_privileged docker inspect "$container_id" \
      --format 'container={{.Name}} status={{.State.Status}} exit={{.State.ExitCode}} restarts={{.RestartCount}} image={{.Config.Image}} error={{.State.Error}}' \
      >&2 || true
  done <<<"$container_ids"

  # Never dump the complete worker log here: startup configuration can contain
  # credentials. Keep only bounded lifecycle failures and redact URI userinfo.
  docker_compose logs --tail 200 "$WORKER_COMPOSE_SERVICE" 2>&1 |
    grep -Ei '(fatal|panic|failed to (prepare|initialize|connect)|connection (timed out|refused))' |
    sed -E 's#([[:alnum:]+.-]+://)[^/@[:space:]]+@#\1***@#g' |
    tail -n 40 >&2 || true
}

verify() {
  if ! command -v "$PRIVILEGE_RUNNER" >/dev/null 2>&1; then
    echo "Privilege runner is unavailable: $PRIVILEGE_RUNNER" >&2
    return 1
  fi
  require_positive_integer EXPECTED_WORKER_REPLICAS "$EXPECTED_WORKER_REPLICAS"
  require_positive_integer WORKER_READY_ATTEMPTS "$WORKER_READY_ATTEMPTS"
  require_nonnegative_integer WORKER_READY_INTERVAL_SECONDS "$WORKER_READY_INTERVAL_SECONDS"
  require_positive_integer WORKER_READY_TIMEOUT_SECONDS "$WORKER_READY_TIMEOUT_SECONDS"

  local attempt container_ids running_count ready_count image_count container_id
  for attempt in $(seq 1 "$WORKER_READY_ATTEMPTS"); do
    container_ids="$(running_worker_container_ids || true)"
    running_count="$(printf '%s\n' "$container_ids" | sed '/^$/d' | wc -l | tr -d '[:space:]')"
    ready_count=0
    image_count=0

    while IFS= read -r container_id; do
      [ -z "$container_id" ] && continue
      if worker_image_matches "$container_id"; then
        image_count=$((image_count + 1))
      fi
      if worker_is_ready "$container_id"; then
        ready_count=$((ready_count + 1))
      fi
    done <<<"$container_ids"

    if [ "$running_count" -eq "$EXPECTED_WORKER_REPLICAS" ] &&
      [ "$image_count" -eq "$EXPECTED_WORKER_REPLICAS" ] &&
      [ "$ready_count" -eq "$EXPECTED_WORKER_REPLICAS" ]; then
      echo "Worker deployment gate passed: running=${running_count}/${EXPECTED_WORKER_REPLICAS} image=${image_count}/${EXPECTED_WORKER_REPLICAS} ready=${ready_count}/${EXPECTED_WORKER_REPLICAS}"
      docker_compose ps "$WORKER_COMPOSE_SERVICE"
      return 0
    fi

    echo "Worker deployment gate attempt ${attempt}/${WORKER_READY_ATTEMPTS}: running=${running_count}/${EXPECTED_WORKER_REPLICAS} image=${image_count}/${EXPECTED_WORKER_REPLICAS} ready=${ready_count}/${EXPECTED_WORKER_REPLICAS}"
    if [ "$attempt" -lt "$WORKER_READY_ATTEMPTS" ]; then
      sleep "$WORKER_READY_INTERVAL_SECONDS"
    fi
  done

  echo "Worker deployment gate failed after ${WORKER_READY_ATTEMPTS} attempts" >&2
  show_failure_diagnostics
  return 1
}

usage() {
  echo "Usage: wait-worker-readiness.sh [verify]" >&2
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
