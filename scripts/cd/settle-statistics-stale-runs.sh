#!/usr/bin/env bash
set -Eeuo pipefail

CONFIRM_SETTLEMENT="${CONFIRM_SETTLEMENT:-}"
PRIVILEGE_RUNNER="${PRIVILEGE_RUNNER-sudo}"
DOCKER_BIN="${DOCKER_BIN:-docker}"
MYSQL_PORT="${MYSQL_PORT:-3306}"
REDIS_PORT="${REDIS_PORT:-6379}"
REDIS_AUDIT_CONTAINER="qs-server-statistics-settlement-redis"

if [ "$CONFIRM_SETTLEMENT" != "SETTLE-6-STATISTICS-RUNS" ]; then
  echo "Statistics settlement confirmation does not match the exact maintenance action" >&2
  exit 1
fi

for required_name in MYSQL_HOST MYSQL_USERNAME MYSQL_PASSWORD MYSQL_DATABASE REDIS_HOST; do
  if [ -z "${!required_name:-}" ]; then
    echo "${required_name} is required" >&2
    exit 1
  fi
done

run_privileged() {
  if [ -n "$PRIVILEGE_RUNNER" ]; then
    "$PRIVILEGE_RUNNER" "$@"
  else
    "$@"
  fi
}

cleanup() {
  run_privileged "$DOCKER_BIN" rm -f "$REDIS_AUDIT_CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if ! command -v "$DOCKER_BIN" >/dev/null 2>&1; then
  echo "Docker command is unavailable: $DOCKER_BIN" >&2
  exit 1
fi
if [ -n "$PRIVILEGE_RUNNER" ] && ! command -v "$PRIVILEGE_RUNNER" >/dev/null 2>&1; then
  echo "Privilege runner is unavailable: $PRIVILEGE_RUNNER" >&2
  exit 1
fi
if ! run_privileged "$DOCKER_BIN" network inspect infra-network >/dev/null 2>&1; then
  echo "infra-network is unavailable" >&2
  exit 1
fi

host_rebuild_processes="$(
  ps -eo comm= | awk '$1 ~ /^rebuild_statist/ { count++ } END { print count + 0 }'
)"
matching_rebuild_containers="$(
  run_privileged "$DOCKER_BIN" ps --format '{{.Names}}\t{{.Image}}' |
    awk 'tolower($0) ~ /rebuild[-_]?statistics|statistics[-_]?rebuild/ { count++ } END { print count + 0 }'
)"
if [ "$host_rebuild_processes" -ne 0 ] || [ "$matching_rebuild_containers" -ne 0 ]; then
  echo "Statistics rebuild activity detected: host=${host_rebuild_processes} containers=${matching_rebuild_containers}" >&2
  exit 1
fi

redis_cli_args=(-h "$REDIS_HOST" -p "$REDIS_PORT" --no-auth-warning --raw)
if [ -n "${REDIS_USERNAME:-}" ]; then
  redis_cli_args+=(--user "$REDIS_USERNAME")
fi
if [ "${REDIS_USE_SSL:-false}" = "true" ]; then
  redis_cli_args+=(--tls)
  if [ "${REDIS_SSL_INSECURE_SKIP_VERIFY:-false}" = "true" ]; then
    redis_cli_args+=(--insecure)
  fi
fi

statistics_lock_ttl="$(
  run_privileged "$DOCKER_BIN" run --rm --network infra-network \
    --name "$REDIS_AUDIT_CONTAINER" \
    --label "com.fangcunmount.qs-server.operation=statistics-stale-run-settlement-preflight" \
    -e REDISCLI_AUTH="${REDIS_PASSWORD:-}" \
    redis:7-alpine redis-cli "${redis_cli_args[@]}" -n 6 TTL 'cache:lock:statistics:1' |
    tr -d '\r'
)"
if [ "$statistics_lock_ttl" != "-2" ]; then
  echo "Statistics task lock is not absent: database=6 ttl_seconds=${statistics_lock_ttl}" >&2
  exit 1
fi

echo "Statistics stale-run preflight passed: task_lock_absent=true host_rebuild_processes=0 matching_rebuild_containers=0"

settlement_output="$(
  run_privileged "$DOCKER_BIN" run --rm --network infra-network \
    --label "com.fangcunmount.qs-server.operation=statistics-stale-run-settlement" \
    -e MYSQL_PWD="$MYSQL_PASSWORD" \
    mysql:8.0 mysql --protocol=tcp \
      --host="$MYSQL_HOST" \
      --port="$MYSQL_PORT" \
      --user="$MYSQL_USERNAME" \
      --database="$MYSQL_DATABASE" \
      --batch --raw --skip-column-names <<'MYSQLSQL'
SET @settled_at = CURRENT_TIMESTAMP(3);
START TRANSACTION;

SELECT id
FROM statistics_sync_run
WHERE id IN (
  631012088902332974,
  631034496552022574,
  631349010061341230,
  631362645156442670,
  631363406154183214,
  631444782798877230
)
ORDER BY id
FOR UPDATE;

SET @eligible_count = (
  SELECT COUNT(*)
  FROM statistics_sync_run
  WHERE id IN (
    631012088902332974,
    631034496552022574,
    631349010061341230,
    631362645156442670,
    631363406154183214,
    631444782798877230
  )
    AND org_id = 1
    AND trigger_type = 'manual'
    AND run_mode IN ('repair', 'validate')
    AND status = 'running'
    AND stage IN ('collecting_assessment', 'collecting_plan')
    AND data_committed_at IS NULL
    AND finished_at IS NULL
    AND cache_generation = 0
    AND cache_published_at IS NULL
    AND cache_resume_count = 0
    AND error_code = ''
    AND error_message = ''
    AND updated_at <= @settled_at - INTERVAL 30 MINUTE
);

UPDATE statistics_sync_run
SET status = 'failed',
    finished_at = @settled_at,
    error_code = 'stale_run_reconciled',
    error_message = 'Manually settled as stale after confirming no active process or lease, no committed data, and no resume requirement.'
WHERE id IN (
  631012088902332974,
  631034496552022574,
  631349010061341230,
  631362645156442670,
  631363406154183214,
  631444782798877230
)
  AND @eligible_count = 6
  AND org_id = 1
  AND trigger_type = 'manual'
  AND run_mode IN ('repair', 'validate')
  AND status = 'running'
  AND stage IN ('collecting_assessment', 'collecting_plan')
  AND data_committed_at IS NULL
  AND finished_at IS NULL
  AND cache_generation = 0
  AND cache_published_at IS NULL
  AND cache_resume_count = 0
  AND error_code = ''
  AND error_message = ''
  AND updated_at <= @settled_at - INTERVAL 30 MINUTE;

SET @affected_rows = ROW_COUNT();
SET @verified_rows = (
  SELECT COUNT(*)
  FROM statistics_sync_run
  WHERE id IN (
    631012088902332974,
    631034496552022574,
    631349010061341230,
    631362645156442670,
    631363406154183214,
    631444782798877230
  )
    AND status = 'failed'
    AND finished_at = @settled_at
    AND error_code = 'stale_run_reconciled'
    AND data_committed_at IS NULL
    AND cache_generation = 0
    AND cache_published_at IS NULL
);

SELECT CONCAT('SETTLEMENT_RESULT|', @eligible_count, '|', @affected_rows, '|', @verified_rows, '|', DATE_FORMAT(@settled_at, '%Y-%m-%dT%H:%i:%s.%f'));
COMMIT;
MYSQLSQL
)"

printf '%s\n' "$settlement_output"
settlement_result="$(printf '%s\n' "$settlement_output" | awk -F '|' '$1 == "SETTLEMENT_RESULT" {print $0}' | tail -1)"
expected_result_prefix='SETTLEMENT_RESULT|6|6|6|'
case "$settlement_result" in
  "$expected_result_prefix"*) ;;
  *)
    echo "Statistics settlement did not produce the exact committed result" >&2
    exit 1
    ;;
esac

postcheck_output="$(
  run_privileged "$DOCKER_BIN" run --rm --network infra-network \
    --label "com.fangcunmount.qs-server.operation=statistics-stale-run-settlement-postcheck" \
    -e MYSQL_PWD="$MYSQL_PASSWORD" \
    mysql:8.0 mysql --protocol=tcp \
      --host="$MYSQL_HOST" \
      --port="$MYSQL_PORT" \
      --user="$MYSQL_USERNAME" \
      --database="$MYSQL_DATABASE" \
      --batch --raw --skip-column-names \
      --execute="
        SELECT CONCAT('POSTCHECK|', COUNT(*), '|',
          SUM(status = 'failed'), '|',
          SUM(error_code = 'stale_run_reconciled'), '|',
          SUM(data_committed_at IS NULL), '|',
          SUM(cache_generation = 0 AND cache_published_at IS NULL))
        FROM statistics_sync_run
        WHERE id IN (
          631012088902332974,
          631034496552022574,
          631349010061341230,
          631362645156442670,
          631363406154183214,
          631444782798877230
        );
      "
)"
printf '%s\n' "$postcheck_output"
if [ "$postcheck_output" != "POSTCHECK|6|6|6|6|6" ]; then
  echo "Statistics post-commit verification failed" >&2
  exit 1
fi

trap - EXIT
cleanup
echo "Statistics stale-run settlement passed: exact_targets=6 failed=6 committed_data=0"
