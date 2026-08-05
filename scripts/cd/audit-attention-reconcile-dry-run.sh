#!/usr/bin/env bash
set -Eeuo pipefail

EXPECTED_WORKER_REPLICAS="${EXPECTED_WORKER_REPLICAS:-3}"
EXPECTED_DEPLOY_SHA="${EXPECTED_DEPLOY_SHA:-}"
MIN_SUCCESSFUL_ROUNDS="${MIN_SUCCESSFUL_ROUNDS:-1}"
MIN_MISSING="${MIN_MISSING:-1}"
WORKER_METRICS_PORT="${WORKER_METRICS_PORT:-9092}"
PRIVILEGE_RUNNER="${PRIVILEGE_RUNNER-sudo}"
DOCKER_BIN="${DOCKER_BIN:-docker}"

require_non_negative_integer() {
  local name="$1"
  local value="$2"
  if ! [[ "$value" =~ ^[0-9]+$ ]]; then
    echo "$name must be a non-negative integer, got: $value" >&2
    return 1
  fi
}

require_positive_integer() {
  local name="$1"
  local value="$2"
  require_non_negative_integer "$name" "$value"
  if [ "$value" -lt 1 ]; then
    echo "$name must be greater than zero, got: $value" >&2
    return 1
  fi
}

run_privileged() {
  if [ -n "$PRIVILEGE_RUNNER" ]; then
    "$PRIVILEGE_RUNNER" "$@"
  else
    "$@"
  fi
}

metric_sum() {
  local metrics="$1"
  local metric_name="$2"
  local required_label="${3:-}"
  local required_label_2="${4:-}"
  printf '%s\n' "$metrics" | awk \
    -v metric_name="$metric_name" \
    -v required_label="$required_label" \
    -v required_label_2="$required_label_2" '
      $1 ~ ("^" metric_name "({|$)") &&
        (required_label == "" || index($0, required_label) > 0) &&
        (required_label_2 == "" || index($0, required_label_2) > 0) {
          total += $NF
        }
      END { printf "%.0f", total + 0 }
    '
}

add_numbers() {
  awk -v left="$1" -v right="$2" 'BEGIN { printf "%.0f", left + right }'
}

require_positive_integer EXPECTED_WORKER_REPLICAS "$EXPECTED_WORKER_REPLICAS"
require_positive_integer MIN_SUCCESSFUL_ROUNDS "$MIN_SUCCESSFUL_ROUNDS"
require_non_negative_integer MIN_MISSING "$MIN_MISSING"
require_positive_integer WORKER_METRICS_PORT "$WORKER_METRICS_PORT"
if ! [[ "$EXPECTED_DEPLOY_SHA" =~ ^[0-9a-f]{7,40}$ ]]; then
  echo "EXPECTED_DEPLOY_SHA must be a 7-40 character lowercase Git SHA" >&2
  exit 1
fi
if [ -n "$PRIVILEGE_RUNNER" ] && ! command -v "$PRIVILEGE_RUNNER" >/dev/null 2>&1; then
  echo "Privilege runner is unavailable: $PRIVILEGE_RUNNER" >&2
  exit 1
fi
if ! command -v "$DOCKER_BIN" >/dev/null 2>&1; then
  echo "Docker command is unavailable: $DOCKER_BIN" >&2
  exit 1
fi

container_ids="$(run_privileged "$DOCKER_BIN" ps \
  --filter 'label=com.docker.compose.project=qs-worker' \
  --filter 'label=com.docker.compose.service=runtime' \
  --filter 'status=running' \
  --format '{{.ID}}')"
container_count="$(printf '%s\n' "$container_ids" | sed '/^$/d' | wc -l | tr -d '[:space:]')"
if [ "$container_count" -ne "$EXPECTED_WORKER_REPLICAS" ]; then
  echo "Running worker containers=${container_count}, want ${EXPECTED_WORKER_REPLICAS}" >&2
  exit 1
fi

total_successful_rounds=0
total_created=0
latest_missing_max=0

while IFS= read -r container_id; do
  [ -z "$container_id" ] && continue

  image_ref="$(run_privileged "$DOCKER_BIN" inspect "$container_id" --format '{{.Config.Image}}')"
  case "$image_ref" in
    *:"$EXPECTED_DEPLOY_SHA") ;;
    *)
      echo "Worker ${container_id} image ${image_ref} does not match ${EXPECTED_DEPLOY_SHA}" >&2
      exit 1
      ;;
  esac

  for expected_config in \
    'attention-projection-reconcile-enabled:[[:space:]]*true' \
    'attention-projection-reconcile-from:[[:space:]]*"2026-08-05T00:00:00Z"' \
    'attention-projection-reconcile-dry-run:[[:space:]]*true'; do
    if ! run_privileged "$DOCKER_BIN" exec "$container_id" \
      grep -Eq "^[[:space:]]*${expected_config}[[:space:]]*(#.*)?$" /app/configs/worker.prod.yaml; then
      echo "Worker ${container_id} is missing effective config: ${expected_config}" >&2
      exit 1
    fi
  done

  metrics="$(run_privileged "$DOCKER_BIN" exec "$container_id" \
    wget -qO- -T 5 "http://127.0.0.1:${WORKER_METRICS_PORT}/metrics")"
  attention_metrics="$(printf '%s\n' "$metrics" | grep '^attention_fact_reconcile_' || true)"
  if [ -z "$attention_metrics" ]; then
    echo "Worker ${container_id} exposes no Attention reconcile metrics" >&2
    exit 1
  fi
  if printf '%s\n' "$attention_metrics" | grep -Eq 'dry_run="false"'; then
    echo "Worker ${container_id} exposed a non-dry-run Attention reconcile metric" >&2
    exit 1
  fi

  successful_rounds="$(metric_sum "$attention_metrics" attention_fact_reconcile_rounds_total 'dry_run="true"' 'result="success"')"
  created="$(metric_sum "$attention_metrics" attention_fact_reconcile_total 'dry_run="true"' 'result="created"')"
  latest_missing="$(metric_sum "$attention_metrics" attention_fact_reconcile_missing 'dry_run="true"')"
  consecutive_failures="$(metric_sum "$attention_metrics" attention_fact_reconcile_consecutive_failures)"
  if [ "$consecutive_failures" -ne 0 ]; then
    echo "Worker ${container_id} has ${consecutive_failures} consecutive Attention reconcile failures" >&2
    exit 1
  fi

  total_successful_rounds="$(add_numbers "$total_successful_rounds" "$successful_rounds")"
  total_created="$(add_numbers "$total_created" "$created")"
  if [ "$latest_missing" -gt "$latest_missing_max" ]; then
    latest_missing_max="$latest_missing"
  fi

  echo "Worker ${container_id} image=${image_ref} successful_rounds=${successful_rounds} latest_missing=${latest_missing} created=${created} consecutive_failures=${consecutive_failures}"
  printf '%s\n' "$attention_metrics" | sort
done <<<"$container_ids"

if [ "$total_successful_rounds" -lt "$MIN_SUCCESSFUL_ROUNDS" ]; then
  echo "Successful dry-run rounds=${total_successful_rounds}, want at least ${MIN_SUCCESSFUL_ROUNDS}" >&2
  exit 1
fi
if [ "$total_created" -ne 0 ]; then
  echo "Dry-run reported created=${total_created}, want 0" >&2
  exit 1
fi
if [ "$latest_missing_max" -lt "$MIN_MISSING" ]; then
  echo "Latest missing maximum=${latest_missing_max}, want at least ${MIN_MISSING}" >&2
  exit 1
fi

echo "Attention reconcile dry-run audit passed: replicas=${container_count} successful_rounds=${total_successful_rounds} latest_missing_max=${latest_missing_max} created=${total_created}"
