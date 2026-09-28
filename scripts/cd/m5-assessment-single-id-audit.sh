#!/usr/bin/env bash
# Exact-ID read-only assessment lookup. Prints no answer content or user details.
set -Eeuo pipefail
: "${ASSESSMENT_ID:?}"
: "${MYSQL_HOST:?}"
: "${MYSQL_USERNAME:?}"
: "${MYSQL_PASSWORD:?}"
: "${MYSQL_DATABASE:?}"
[[ "$ASSESSMENT_ID" =~ ^[1-9][0-9]{0,19}$ ]] || { echo 'invalid assessment ID' >&2; exit 1; }

query() {
  sudo -n docker run --rm --network infra-network -e MYSQL_PWD="$MYSQL_PASSWORD" mysql:8.0 \
    mysql --protocol=tcp --host="$MYSQL_HOST" --port="${MYSQL_PORT:-3306}" \
    --user="$MYSQL_USERNAME" --database="$MYSQL_DATABASE" --batch --raw --execute="$1"
}

echo 'ASSESSMENT'
query "SELECT id,org_id,answer_sheet_id,status,evaluation_model_kind,evaluation_model_code,evaluation_model_version,submitted_at,evaluated_at,failed_at,created_at FROM assessment WHERE id=${ASSESSMENT_ID} AND deleted_at IS NULL LIMIT 1"
echo 'EVALUATION_RUNS'
query "SELECT resource_id,attempt_no,status,error_code,retryable,started_at,finished_at FROM runtime_checkpoint WHERE scope='evaluation_run' AND assessment_id=${ASSESSMENT_ID} AND deleted_at IS NULL ORDER BY attempt_no LIMIT 20"
echo 'EVALUATION_OUTCOME'
query "SELECT id,evaluation_run_id,model_kind,model_code,model_version,evaluated_at FROM evaluation_outcome WHERE assessment_id=${ASSESSMENT_ID} LIMIT 1"
echo 'LEGACY_MYSQL_OUTBOX'
query "SELECT event_id,event_type,status,attempt_count,created_at,published_at FROM domain_event_outbox WHERE aggregate_id='${ASSESSMENT_ID}' ORDER BY id DESC LIMIT 30"
