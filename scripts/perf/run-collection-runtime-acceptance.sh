#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMAND="${1:-help}"
EXPECTED_COLLECTION_REPLICAS="${EXPECTED_COLLECTION_REPLICAS:-2}"
COLLECTION_COMPOSE_PROJECT="${COLLECTION_COMPOSE_PROJECT:-qs-collection}"
COLLECTION_COMPOSE_SERVICE="${COLLECTION_COMPOSE_SERVICE:-server}"
COLLECTION_NETWORK="${COLLECTION_NETWORK:-qs-network}"
ARTIFACT_ROOT="${ARTIFACT_ROOT:-artifacts/perf/collection-runtime-acceptance}"

docker_cmd=(docker)
if [[ "${DOCKER_SUDO:-0}" == "1" ]]; then
  docker_cmd=(sudo docker)
fi

usage() {
  cat <<'USAGE'
Usage: scripts/perf/run-collection-runtime-acceptance.sh <command>

Commands:
  status             Print /readyz and /serve-readyz for every collection replica.
  healthy-smoke      Online HTTP contract smoke; skips exact global metric deltas.
  healthy            Formal healthy-Redis baseline with exact metric deltas.
  degraded-low       Isolated Redis failure: 20 QPS multi-writer acceptance.
  degraded-global    Isolated Redis failure: 120 QPS global fallback ceiling.
  degraded-user      Isolated Redis failure: 30 QPS single-writer fallback ceiling.
  recovery           Formal post-recovery baseline and limiter-strategy verification.

Required inputs:
  healthy-smoke/healthy/recovery:
    COLLECTION_TOKEN or TOKEN, SUBMIT_PAYLOAD_JSON
  degraded-*:
    SUBMIT_CASES_JSON

Safety gates:
  healthy:       PERF_ISOLATED_ENV=true
  degraded-*:    PERF_ISOLATED_ENV=true REDIS_FAILURE_CONFIRMED=true
  recovery:      PERF_ISOLATED_ENV=true REDIS_RECOVERY_CONFIRMED=true

The script never injects or restores a Redis failure. Operators must establish and
verify the isolated environment before setting the confirmation variables.
USAGE
}

require_true() {
  local name="$1"
  if [[ "${!name:-}" != "true" ]]; then
    echo "${name}=true is required for ${COMMAND}" >&2
    exit 1
  fi
}

validate_replica_count() {
  if ! [[ "$EXPECTED_COLLECTION_REPLICAS" =~ ^[1-9][0-9]*$ ]] ||
    [[ "$EXPECTED_COLLECTION_REPLICAS" -lt 2 ]]; then
    echo "EXPECTED_COLLECTION_REPLICAS must be an integer of at least 2" >&2
    exit 1
  fi
}

container_ids=()
collection_urls=()

discover_collection_replicas() {
  validate_replica_count
  container_ids=()
  collection_urls=()
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

  for container_id in "${container_ids[@]}"; do
    local container_ip
    container_ip="$(
      "${docker_cmd[@]}" inspect "$container_id" \
        --format "{{with index .NetworkSettings.Networks \"${COLLECTION_NETWORK}\"}}{{.IPAddress}}{{end}}"
    )"
    if [[ -z "$container_ip" ]]; then
      echo "container ${container_id} has no IP on ${COLLECTION_NETWORK}" >&2
      exit 1
    fi
    collection_urls+=("http://${container_ip}:8080")
  done
}

readiness_code() {
  local output_file="$1"
  local url="$2"
  curl -sS --max-time 5 -o "$output_file" -w '%{http_code}' "$url" || true
}

capture_status() {
  local artifact_dir="$1"
  local expected_ready="${2:-}"
  local expected_serve="${3:-}"
  mkdir -p "$artifact_dir"

  for index in "${!container_ids[@]}"; do
    local container_id="${container_ids[$index]}"
    local base_url="${collection_urls[$index]}"
    local ready_code
    local serve_code
    ready_code="$(readiness_code "${artifact_dir}/${container_id}-readyz.json" "${base_url}/readyz")"
    serve_code="$(readiness_code "${artifact_dir}/${container_id}-serve-readyz.json" "${base_url}/serve-readyz")"
    echo "instance=${container_id} base_url=${base_url} readyz=${ready_code} serve_readyz=${serve_code}"
    if [[ -n "$expected_ready" && "$ready_code" != "$expected_ready" ]]; then
      echo "container ${container_id} /readyz=${ready_code}, want ${expected_ready}" >&2
      exit 1
    fi
    if [[ -n "$expected_serve" && "$serve_code" != "$expected_serve" ]]; then
      echo "container ${container_id} /serve-readyz=${serve_code}, want ${expected_serve}" >&2
      exit 1
    fi
  done
}

capture_metrics() {
  local artifact_dir="$1"
  local suffix="$2"
  mkdir -p "$artifact_dir"
  for index in "${!container_ids[@]}"; do
    curl -fsS --max-time 5 "${collection_urls[$index]}/metrics" \
      >"${artifact_dir}/${container_ids[$index]}-metrics-${suffix}.prom"
  done
}

rate_decision_total() {
  local metric_file="$1"
  local strategy="$2"
  local outcome="$3"
  awk -v strategy="$strategy" -v outcome="$outcome" '
    /^qs_resilience_decision_total\{/ &&
    index($0, "component=\"collection-server\"") &&
    index($0, "kind=\"rate_limit\"") &&
    index($0, "scope=\"submit\"") &&
    index($0, "strategy=\"" strategy "\"") &&
    index($0, "outcome=\"" outcome "\"") {
      total += $NF
    }
    END { printf "%.0f", total + 0 }
  ' "$metric_file"
}

metric_delta_across_replicas() {
  local artifact_dir="$1"
  local strategy="$2"
  local outcome="$3"
  local total=0
  for container_id in "${container_ids[@]}"; do
    local before
    local after
    before="$(rate_decision_total "${artifact_dir}/${container_id}-metrics-before.prom" "$strategy" "$outcome")"
    after="$(rate_decision_total "${artifact_dir}/${container_id}-metrics-after.prom" "$strategy" "$outcome")"
    total=$((total + after - before))
  done
  echo "$total"
}

run_recovery_acceptance() {
  require_true PERF_ISOLATED_ENV
  require_true REDIS_RECOVERY_CONFIRMED
  discover_collection_replicas

  local artifact_dir="${ARTIFACT_DIR:-${ARTIFACT_ROOT}/recovery}"
  capture_status "$artifact_dir" 200 200
  capture_metrics "$artifact_dir" before

  PERF_ISOLATED_ENV=true VERIFY_METRICS=true COALESCING_SCENARIO=healthy \
    "${SCRIPT_DIR}/run-submit-coalescing.sh"

  capture_metrics "$artifact_dir" after
  # Recovery must stop strategy="local_fallback" and resume strategy="redis".
  local fallback_delta
  local redis_allowed_delta
  fallback_delta="$(metric_delta_across_replicas "$artifact_dir" local_fallback degraded_open)"
  redis_allowed_delta="$(metric_delta_across_replicas "$artifact_dir" redis allowed)"
  if [[ "$fallback_delta" -ne 0 ]]; then
    echo "local_fallback/degraded_open increased by ${fallback_delta} after Redis recovery" >&2
    exit 1
  fi
  if [[ "$redis_allowed_delta" -le 0 ]]; then
    echo "strategy=redis outcome=allowed did not increase after Redis recovery" >&2
    exit 1
  fi
  echo "PASS: recovery local_fallback_delta=${fallback_delta} redis_allowed_delta=${redis_allowed_delta} artifacts=${artifact_dir}"
}

case "$COMMAND" in
  help | -h | --help)
    usage
    ;;
  status)
    discover_collection_replicas
    capture_status "${ARTIFACT_DIR:-${ARTIFACT_ROOT}/status}"
    ;;
  healthy-smoke)
    echo "NOTICE: healthy-smoke verifies HTTP behavior and creates one durable AnswerSheet; it is not formal exact-metric acceptance."
    VERIFY_METRICS=false COALESCING_REQUESTS="${COALESCING_REQUESTS:-20}" COALESCING_SCENARIO=healthy \
      "${SCRIPT_DIR}/run-submit-coalescing.sh"
    ;;
  healthy)
    require_true PERF_ISOLATED_ENV
    PERF_ISOLATED_ENV=true VERIFY_METRICS=true COALESCING_SCENARIO=healthy \
      "${SCRIPT_DIR}/run-submit-coalescing.sh"
    ;;
  degraded-low)
    require_true PERF_ISOLATED_ENV
    require_true REDIS_FAILURE_CONFIRMED
    PERF_ISOLATED_ENV=true REDIS_FAILURE_CONFIRMED=true DEGRADED_SUBMIT_MODE=low \
      "${SCRIPT_DIR}/run-submit-redis-degraded.sh"
    ;;
  degraded-global)
    require_true PERF_ISOLATED_ENV
    require_true REDIS_FAILURE_CONFIRMED
    PERF_ISOLATED_ENV=true REDIS_FAILURE_CONFIRMED=true DEGRADED_SUBMIT_MODE=global_overload \
      "${SCRIPT_DIR}/run-submit-redis-degraded.sh"
    ;;
  degraded-user)
    require_true PERF_ISOLATED_ENV
    require_true REDIS_FAILURE_CONFIRMED
    PERF_ISOLATED_ENV=true REDIS_FAILURE_CONFIRMED=true DEGRADED_SUBMIT_MODE=user_overload \
      "${SCRIPT_DIR}/run-submit-redis-degraded.sh"
    ;;
  recovery)
    run_recovery_acceptance
    ;;
  *)
    echo "unknown command: ${COMMAND}" >&2
    usage >&2
    exit 1
    ;;
esac
