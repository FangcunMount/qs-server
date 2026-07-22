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

snapshot_mongo_outbox() {
  local output="$out_dir/${label}-mongo-outbox.json"
  if [[ -z "${MONGO_URI:-}" ]] || ! command -v mongosh >/dev/null 2>&1; then
    return
  fi
  mongosh "$MONGO_URI" --quiet --eval '
db.domain_event_outbox.aggregate([
  { $match: { status: { $in: ["pending", "publishing", "failed"] } } },
  {
    $group: {
      _id: { status: "$status", event_type: "$event_type" },
      count: { $sum: 1 },
      oldest: { $min: "$created_at" },
      newest: { $max: "$created_at" }
    }
  },
  { $sort: { count: -1 } }
]).toArray()
' >"$output" 2>"$output.err" || true
}

snapshot_mysql_query() {
  local name="$1"
  local sql="$2"
  local output="$out_dir/${label}-${name}"
  if [[ -z "${MYSQL_CLI_ARGS:-}" ]] || ! command -v mysql >/dev/null 2>&1; then
    return
  fi
  # shellcheck disable=SC2086
  mysql $MYSQL_CLI_ARGS -N -B -e "$sql" >"$output" 2>"$output.err" || true
}

snapshot_mongo_outbox

snapshot_mysql_query "mysql-outbox.txt" "
SELECT status, event_type, COUNT(*) AS cnt
FROM domain_event_outbox
WHERE status IN ('pending', 'publishing', 'failed')
GROUP BY status, event_type
ORDER BY cnt DESC;
"

snapshot_mysql_query "statistics-facts.txt" "
SELECT 'access' AS family, COUNT(*) AS row_count, MAX(occurred_at) AS latest_occurred_at
FROM statistics_access_fact WHERE org_id = ${PERF_ORG_ID:-1}
UNION ALL
SELECT 'assessment', COUNT(*), MAX(occurred_at)
FROM statistics_assessment_fact WHERE org_id = ${PERF_ORG_ID:-1}
UNION ALL
SELECT 'plan', COUNT(*), MAX(occurred_at)
FROM statistics_plan_fact WHERE org_id = ${PERF_ORG_ID:-1};
"

snapshot_mysql_query "statistics-results.txt" "
SELECT 'access_daily' AS result_name, COUNT(*) AS row_count, MAX(updated_at) AS latest_updated_at
FROM statistics_access_daily WHERE org_id = ${PERF_ORG_ID:-1}
UNION ALL
SELECT 'assessment_daily', COUNT(*), MAX(updated_at)
FROM statistics_assessment_daily WHERE org_id = ${PERF_ORG_ID:-1}
UNION ALL
SELECT 'plan_activity_daily', COUNT(*), MAX(updated_at)
FROM statistics_plan_activity_daily WHERE org_id = ${PERF_ORG_ID:-1}
UNION ALL
SELECT 'plan_fulfillment_daily', COUNT(*), MAX(updated_at)
FROM statistics_plan_fulfillment_daily WHERE org_id = ${PERF_ORG_ID:-1}
UNION ALL
SELECT 'org_snapshot', COUNT(*), MAX(updated_at)
FROM statistics_org_snapshot WHERE org_id = ${PERF_ORG_ID:-1};
"

snapshot_mysql_query "statistics-sync-runs.txt" "
SELECT id, mode, status, stage, as_of_date, started_at, finished_at, error_code
FROM statistics_sync_run
WHERE org_id = ${PERF_ORG_ID:-1}
ORDER BY id DESC
LIMIT 20;
"

echo "$out_dir"
