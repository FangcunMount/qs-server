#!/usr/bin/env bash
set -euo pipefail

label="${1:-snapshot}"
out_dir="${OUT_DIR:-tmp/perf/$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$out_dir"

snapshot_url() {
  local name="$1"
  local url="$2"
  local output="$out_dir/${label}-${name}"
  if [[ -z "$url" ]]; then
    return
  fi
  if ! curl -fsS "$url" -o "$output"; then
    echo "failed to snapshot $name from $url" >"$output.err"
  fi
}

snapshot_url "collection-metrics.txt" "${COLLECTION_METRICS_URL:-http://127.0.0.1:18083/metrics}"
snapshot_url "collection-resilience.json" "${COLLECTION_RESILIENCE_URL:-http://127.0.0.1:18083/governance/resilience}"
snapshot_url "collection-redis.json" "${COLLECTION_REDIS_URL:-http://127.0.0.1:18083/governance/redis}"

snapshot_url "apiserver-metrics.txt" "${APISERVER_METRICS_URL:-http://127.0.0.1:18082/metrics}"
snapshot_url "worker-metrics.txt" "${WORKER_METRICS_URL:-http://127.0.0.1:9092/metrics}"
snapshot_url "worker-resilience.json" "${WORKER_RESILIENCE_URL:-http://127.0.0.1:9092/governance/resilience}"
snapshot_url "worker-redis.json" "${WORKER_REDIS_URL:-http://127.0.0.1:9092/governance/redis}"

snapshot_url "nsqd-stats.json" "${NSQD_STATS_URL:-http://127.0.0.1:4151/stats?format=json}"
snapshot_url "nsqlookupd-nodes.json" "${NSQLOOKUPD_NODES_URL:-http://127.0.0.1:4161/nodes}"

if [[ -n "${DOCKER_STATS_CONTAINERS:-}" ]] && command -v docker >/dev/null 2>&1; then
  # shellcheck disable=SC2086
  docker stats --no-stream --format 'table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}\t{{.BlockIO}}' $DOCKER_STATS_CONTAINERS \
    >"$out_dir/${label}-docker-stats.txt" || true
fi

if [[ -n "${REDIS_CLI_ARGS:-}" ]] && command -v redis-cli >/dev/null 2>&1; then
  # shellcheck disable=SC2086
  redis-cli $REDIS_CLI_ARGS INFO stats >"$out_dir/${label}-redis-info-stats.txt" || true
  # shellcheck disable=SC2086
  redis-cli $REDIS_CLI_ARGS INFO commandstats >"$out_dir/${label}-redis-info-commandstats.txt" || true
fi

echo "$out_dir"
