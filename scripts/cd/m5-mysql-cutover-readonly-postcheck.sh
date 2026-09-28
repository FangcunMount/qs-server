#!/usr/bin/env bash
# One-off read-only confirmation of the deployed API and MySQL Outbox states.
set -Eeuo pipefail
: "${EXPECTED_API_SHA:?}"
[[ "$EXPECTED_API_SHA" =~ ^[0-9a-f]{40}$ ]] || exit 1
: "${MYSQL_HOST:?}"
: "${MYSQL_USERNAME:?}"
: "${MYSQL_PASSWORD:?}"
: "${MYSQL_DATABASE:?}"

image=$(sudo -n docker inspect qs-apiserver --format '{{.Config.Image}}')
state=$(sudo -n docker inspect qs-apiserver --format '{{.State.Status}}')
health=$(sudo -n docker inspect qs-apiserver --format '{{.State.Health.Status}}')
[[ "$image" == *":${EXPECTED_API_SHA}" && "$state" == running && "$health" == healthy ]] || {
  echo 'API image or health differs from target' >&2
  exit 1
}
settings=$(sudo -n docker exec qs-apiserver awk '
  /^  standard_outbox:/ {section=1; next}
  section && /^  [^ ]/ {section=0}
  section && /^    (mongo|assessment):/ {sub(/[[:space:]]*#.*/, ""); sub(/^[[:space:]]+/, ""); print}
' /app/configs/apiserver.prod.yaml)
[[ $(grep -Fxc 'mongo: true' <<<"$settings") == 1 &&
   $(grep -Fxc 'assessment: true' <<<"$settings") == 1 ]] || {
  echo 'deployed standard Outbox profile differs from target' >&2
  exit 1
}
printf 'API=%s health=healthy standard_mongo=true standard_assessment=true\n' "$EXPECTED_API_SHA"
sudo -n docker run --rm --network infra-network -e MYSQL_PWD="$MYSQL_PASSWORD" mysql:8.0 \
  mysql --protocol=tcp --host="$MYSQL_HOST" --port="${MYSQL_PORT:-3306}" \
  --user="$MYSQL_USERNAME" --database="$MYSQL_DATABASE" --batch --raw --skip-column-names \
  --execute="SELECT 'standard',event_type,state,COUNT(*) FROM rm_outbox GROUP BY event_type,state UNION ALL SELECT 'legacy',event_type,status,COUNT(*) FROM domain_event_outbox WHERE status <> 'published' AND event_type IN ('evaluation.requested','evaluation.retry.requested','evaluation.outcome.committed','evaluation.failed','task.opened.reminder.requested') GROUP BY event_type,status"
