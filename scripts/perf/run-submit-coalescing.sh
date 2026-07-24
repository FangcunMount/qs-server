#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K6_BIN="${K6_BIN:-k6}"
EXPECTED_COLLECTION_REPLICAS="${EXPECTED_COLLECTION_REPLICAS:-2}"
COLLECTION_COMPOSE_PROJECT="${COLLECTION_COMPOSE_PROJECT:-qs-collection}"
COLLECTION_COMPOSE_SERVICE="${COLLECTION_COMPOSE_SERVICE:-qs-collection-server}"
COLLECTION_NETWORK="${COLLECTION_NETWORK:-qs-network}"
APISERVER_METRICS_URL="${APISERVER_METRICS_URL:-http://127.0.0.1:8081/metrics}"
COALESCING_SCENARIO="${COALESCING_SCENARIO:-healthy}"
PERF_ISOLATED_ENV="${PERF_ISOLATED_ENV:-}"

docker_cmd=(docker)
if [[ "${DOCKER_SUDO:-0}" == "1" ]]; then
  docker_cmd=(sudo docker)
fi

if ! command -v "$K6_BIN" >/dev/null 2>&1; then
  echo "k6 is required: set K6_BIN or install k6" >&2
  exit 1
fi
if ! [[ "$EXPECTED_COLLECTION_REPLICAS" =~ ^[1-9][0-9]*$ ]]; then
  echo "EXPECTED_COLLECTION_REPLICAS must be a positive integer" >&2
  exit 1
fi
if [[ "$EXPECTED_COLLECTION_REPLICAS" -lt 2 ]]; then
  echo "SubmitCoalescer acceptance requires at least two collection replicas" >&2
  exit 1
fi
if [[ "$PERF_ISOLATED_ENV" != "true" ]]; then
  echo "PERF_ISOLATED_ENV=true is required: exact Prometheus deltas are unsafe under unrelated submit traffic" >&2
  exit 1
fi
if [[ -z "${COLLECTION_TOKEN:-${TOKEN:-}}" ]]; then
  echo "COLLECTION_TOKEN or TOKEN is required" >&2
  exit 1
fi
if [[ -z "${SUBMIT_PAYLOAD_JSON:-}" ]]; then
  echo "SUBMIT_PAYLOAD_JSON is required" >&2
  exit 1
fi
if [[ "$COALESCING_SCENARIO" == "conflict" && -z "${CONFLICT_PAYLOAD_JSON:-}" ]]; then
  echo "CONFLICT_PAYLOAD_JSON is required for COALESCING_SCENARIO=conflict" >&2
  exit 1
fi

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
  echo "found ${#container_ids[@]} running collection replicas, expected ${EXPECTED_COLLECTION_REPLICAS}" >&2
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
  if ! curl -fsS --max-time 5 "${base_url}/readyz" >/dev/null; then
    echo "collection replica ${container_id} is not ready at ${base_url}/readyz" >&2
    exit 1
  fi
  if ! curl -fsS --max-time 5 "${base_url}/metrics" >/dev/null; then
    echo "collection replica ${container_id} metrics are unavailable at ${base_url}/metrics" >&2
    exit 1
  fi
  collection_urls+=("$base_url")
  collection_metrics_urls+=("${base_url}/metrics")
done

if ! curl -fsS --max-time 5 "$APISERVER_METRICS_URL" >/dev/null; then
  echo "apiserver metrics are unavailable at ${APISERVER_METRICS_URL}" >&2
  exit 1
fi

join_by_comma() {
  local IFS=,
  echo "$*"
}

export COLLECTION_BASE_URLS
COLLECTION_BASE_URLS="$(join_by_comma "${collection_urls[@]}")"
export COLLECTION_METRICS_URLS
COLLECTION_METRICS_URLS="$(join_by_comma "${collection_metrics_urls[@]}")"
export APISERVER_METRICS_URL
export COALESCING_SCENARIO
export PERF_ISOLATED_ENV

echo "SubmitCoalescer acceptance: scenario=${COALESCING_SCENARIO} replicas=${#container_ids[@]} network=${COLLECTION_NETWORK}"
exec "$K6_BIN" run "${SCRIPT_DIR}/k6-submit-coalescing.js"
