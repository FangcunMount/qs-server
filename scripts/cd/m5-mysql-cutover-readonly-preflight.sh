#!/usr/bin/env bash
# One-off read-only production waterline before enabling the MySQL profile.
set -Eeuo pipefail
: "${MYSQL_HOST:?}"
: "${MYSQL_USERNAME:?}"
: "${MYSQL_PASSWORD:?}"
: "${MYSQL_DATABASE:?}"

query() {
  sudo -n docker run --rm --network infra-network -e MYSQL_PWD="$MYSQL_PASSWORD" mysql:8.0 \
    mysql --protocol=tcp --host="$MYSQL_HOST" --port="${MYSQL_PORT:-3306}" \
    --user="$MYSQL_USERNAME" --database="$MYSQL_DATABASE" --batch --raw --skip-column-names --execute="$1"
}

echo 'MYSQL_OUTBOX_WATERLINE'
query "SELECT 'legacy',status,COUNT(*) FROM domain_event_outbox GROUP BY status UNION ALL SELECT 'standard',state,COUNT(*) FROM rm_outbox GROUP BY state"
unfinished=$(query "SELECT COUNT(*) FROM domain_event_outbox WHERE status <> 'published' AND event_type IN ('evaluation.requested','evaluation.retry.requested','evaluation.outcome.committed','evaluation.failed','task.opened.reminder.requested')")
printf 'legacy_profile_unfinished=%s\n' "$unfinished"
[[ "$unfinished" == 0 ]] || { echo 'legacy assessment profile has unfinished messages' >&2; exit 1; }
standard=$(query "SELECT COUNT(*) FROM rm_outbox")
printf 'standard_mysql_rows=%s\n' "$standard"
[[ "$standard" == 0 ]] || { echo 'standard MySQL Outbox already has messages; inspect before cutover' >&2; exit 1; }
