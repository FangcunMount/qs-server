#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K6_BIN="${K6_BIN:-k6}"
DEGRADED_SUBMIT_MODE="${DEGRADED_SUBMIT_MODE:-low}"
EXPECTED_COLLECTION_REPLICAS="${EXPECTED_COLLECTION_REPLICAS:-2}"
COLLECTION_COMPOSE_PROJECT="${COLLECTION_COMPOSE_PROJECT:-qs-collection}"
COLLECTION_COMPOSE_SERVICE="${COLLECTION_COMPOSE_SERVICE:-server}"
COLLECTION_NETWORK="${COLLECTION_NETWORK:-qs-network}"
PERF_ISOLATED_ENV="${PERF_ISOLATED_ENV:-}"
REDIS_FAILURE_CONFIRMED="${REDIS_FAILURE_CONFIRMED:-}"
ARTIFACT_DIR="${ARTIFACT_DIR:-artifacts/perf/submit-redis-degraded-${DEGRADED_SUBMIT_MODE}}"

docker_cmd=(docker)
if [[ "${DOCKER_SUDO:-0}" == "1" ]]; then
  docker_cmd=(sudo docker)
fi

for command_name in "$K6_BIN" curl awk python3; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "${command_name} is required" >&2
    exit 1
  fi
done
if [[ "$PERF_ISOLATED_ENV" != "true" ]]; then
  echo "PERF_ISOLATED_ENV=true is required because acceptance compares exact per-instance metric deltas" >&2
  exit 1
fi
if [[ "$REDIS_FAILURE_CONFIRMED" != "true" ]]; then
  echo "Isolate the RateLimit Redis path after collection completed its first control sync, then set REDIS_FAILURE_CONFIRMED=true" >&2
  exit 1
fi
if ! [[ "$EXPECTED_COLLECTION_REPLICAS" =~ ^[1-9][0-9]*$ ]] ||
  [[ "$EXPECTED_COLLECTION_REPLICAS" -lt 2 ]]; then
  echo "EXPECTED_COLLECTION_REPLICAS must be an integer of at least 2" >&2
  exit 1
fi
case "$DEGRADED_SUBMIT_MODE" in
  low | global_overload | user_overload) ;;
  *)
    echo "DEGRADED_SUBMIT_MODE must be low, global_overload, or user_overload" >&2
    exit 1
    ;;
esac
if [[ -z "${SUBMIT_CASES_JSON:-}" ]]; then
  echo "SUBMIT_CASES_JSON is required; use [{\"token\":\"...\",\"payload\":{...}}]" >&2
  exit 1
fi

case_count="$(
  python3 -c 'import json,os; value=json.loads(os.environ["SUBMIT_CASES_JSON"]); print(len(value) if isinstance(value,list) else 0)'
)"
case "$DEGRADED_SUBMIT_MODE" in
  low)
    minimum_cases=2
    ;;
  global_overload)
    minimum_cases=6
    ;;
  user_overload)
    minimum_cases=1
    if [[ "$case_count" -ne 1 ]]; then
      echo "user_overload requires exactly one submit case" >&2
      exit 1
    fi
    ;;
esac
if [[ "$case_count" -lt "$minimum_cases" ]]; then
  echo "${DEGRADED_SUBMIT_MODE} requires at least ${minimum_cases} submit cases, found ${case_count}" >&2
  exit 1
fi

mkdir -p "$ARTIFACT_DIR"

container_ids=()
while IFS= read -r container_id; do
  [[ -n "$container_id" ]] && container_ids+=("$container_id")
done < <(
  "${docker_cmd[@]}" ps \
    --filter "label=com.docker.compose.project=${COLLECTION_COMPOSE_PROJECT}" \
    --filter "label=com.docker.compose.service=${COLLECTION_COMPOSE_SERVICE}" \
    --filter "status=running" \
    --format '{{.ID}}' |
    sort
)
if [[ "${#container_ids[@]}" -ne "$EXPECTED_COLLECTION_REPLICAS" ]]; then
  echo "found ${#container_ids[@]} collection replicas, expected ${EXPECTED_COLLECTION_REPLICAS}" >&2
  exit 1
fi

collection_urls=()
collection_metrics_urls=()
for container_id in "${container_ids[@]}"; do
  container_ip="$(
    "${docker_cmd[@]}" inspect "$container_id" \
      --format "{{with index .NetworkSettings.Networks \"${COLLECTION_NETWORK}\"}}{{.IPAddress}}{{end}}"
  )"
  if [[ -z "$container_ip" ]]; then
    echo "container ${container_id} has no IP on ${COLLECTION_NETWORK}" >&2
    exit 1
  fi
  base_url="http://${container_ip}:8080"
  serve_code="$(curl -sS --max-time 5 -o "${ARTIFACT_DIR}/${container_id}-serve-readyz.json" -w '%{http_code}' "${base_url}/serve-readyz" || true)"
  ready_code="$(curl -sS --max-time 5 -o "${ARTIFACT_DIR}/${container_id}-readyz.json" -w '%{http_code}' "${base_url}/readyz" || true)"
  if [[ "$serve_code" != "200" || "$ready_code" != "503" ]]; then
    echo "container ${container_id} readiness mismatch: /serve-readyz=${serve_code}, /readyz=${ready_code}; want 200/503" >&2
    exit 1
  fi
  curl -fsS --max-time 5 "${base_url}/governance/resilience" \
    >"${ARTIFACT_DIR}/${container_id}-resilience.json"
  curl -fsS --max-time 5 "${base_url}/metrics" \
    >"${ARTIFACT_DIR}/${container_id}-metrics-before.prom"
  collection_urls+=("$base_url")
  collection_metrics_urls+=("${base_url}/metrics")
done

metric_total() {
  local metric_file="$1"
  local outcome="$2"
  awk -v outcome="$outcome" '
    /^qs_resilience_decision_total\{/ &&
    index($0, "component=\"collection-server\"") &&
    index($0, "kind=\"rate_limit\"") &&
    index($0, "scope=\"submit\"") &&
    index($0, "strategy=\"local_fallback\"") &&
    index($0, "outcome=\"" outcome "\"") {
      total += $NF
    }
    END { printf "%.0f", total + 0 }
  ' "$metric_file"
}

join_by_comma() {
  local IFS=,
  echo "$*"
}

export COLLECTION_BASE_URLS
COLLECTION_BASE_URLS="$(join_by_comma "${collection_urls[@]}")"
export DEGRADED_SUBMIT_MODE
export PERF_ISOLATED_ENV

echo "Redis-degraded submit acceptance: mode=${DEGRADED_SUBMIT_MODE} replicas=${#container_ids[@]} cases=${case_count}"
"$K6_BIN" run \
  --summary-export "${ARTIFACT_DIR}/k6-summary.json" \
  "${SCRIPT_DIR}/k6-submit-redis-degraded.js" |
  tee "${ARTIFACT_DIR}/k6.log"

degraded_delta=0
limited_delta=0
for index in "${!container_ids[@]}"; do
  container_id="${container_ids[$index]}"
  metrics_after="${ARTIFACT_DIR}/${container_id}-metrics-after.prom"
  curl -fsS --max-time 5 "${collection_metrics_urls[$index]}" >"$metrics_after"
  before_degraded="$(metric_total "${ARTIFACT_DIR}/${container_id}-metrics-before.prom" degraded_open)"
  after_degraded="$(metric_total "$metrics_after" degraded_open)"
  before_limited="$(metric_total "${ARTIFACT_DIR}/${container_id}-metrics-before.prom" rate_limited)"
  after_limited="$(metric_total "$metrics_after" rate_limited)"
  instance_degraded_delta=$((after_degraded - before_degraded))
  instance_limited_delta=$((after_limited - before_limited))
  degraded_delta=$((degraded_delta + instance_degraded_delta))
  limited_delta=$((limited_delta + instance_limited_delta))
  echo "instance=${container_id} local_fallback degraded_open_delta=${instance_degraded_delta} rate_limited_delta=${instance_limited_delta}"
done

if [[ "$degraded_delta" -le 0 ]]; then
  echo "local_fallback degraded_open metric did not increase" >&2
  exit 1
fi
if [[ "$DEGRADED_SUBMIT_MODE" == "low" && "$limited_delta" -ne 0 ]]; then
  echo "low mode unexpectedly rate limited ${limited_delta} decisions" >&2
  exit 1
fi
if [[ "$DEGRADED_SUBMIT_MODE" != "low" && "$limited_delta" -le 0 ]]; then
  echo "overload mode did not produce local_fallback rate_limited decisions" >&2
  exit 1
fi

echo "PASS: degraded_open_delta=${degraded_delta} rate_limited_delta=${limited_delta} artifacts=${ARTIFACT_DIR}"
