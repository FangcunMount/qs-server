#!/usr/bin/env bash
# Exact-ID read-only Mongo answer and Outbox facts; never outputs answers.
set -Eeuo pipefail
: "${ANSWER_SHEET_ID:?}"
: "${MONGODB_HOST:?}"
: "${MONGODB_USERNAME:?}"
: "${MONGODB_PASSWORD:?}"
: "${MONGODB_DBNAME:?}"
[[ "$ANSWER_SHEET_ID" =~ ^[1-9][0-9]{0,19}$ ]] || { echo 'invalid answer sheet ID' >&2; exit 1; }

dir=$(mktemp -d /tmp/rm-m5-answer-audit.XXXXXX)
trap 'rm -f "$dir/audit.js"; rmdir "$dir"' EXIT
cat >"$dir/audit.js" <<'MONGOJS'
const targetDB = db.getSiblingDB(process.env.MONGODB_DBNAME)
const answerID = Long.fromString(process.env.ANSWER_SHEET_ID)
const sheet = targetDB.answersheets.findOne(
  {domain_id: answerID, deleted_at: null},
  {domain_id: 1, org_id: 1, filled_at: 1, durable_acceptance: 1,
   "admission.model_kind": 1, "admission.model_code": 1,
   "admission.model_version": 1})
if (!sheet) throw new Error("exact answer sheet not found")
const acceptance = sheet.durable_acceptance || null
print("ANSWER_SHEET")
printjson({domain_id: String(sheet.domain_id), org_id: String(sheet.org_id),
  filled_at: sheet.filled_at, durable_acceptance: acceptance,
  admission: sheet.admission || null})
if (!acceptance || Number(acceptance.schema_version) !== 1 || !acceptance.event_id)
  throw new Error("answer sheet lacks standard durable acceptance")
const rows = targetDB.rm_outbox.find(
  {message_id: acceptance.event_id},
  {message_id: 1, event_type: 1, state: 1, attempt_count: 1,
   failure_count: 1, last_error_code: 1, destination: 1,
   created_at: 1, updated_at: 1, transport_confirmed_at: 1}
).limit(10).toArray()
print("STANDARD_MONGO_OUTBOX")
printjson(rows)
print("LEGACY_MONGO_OUTBOX_MATCHES")
print(targetDB.domain_event_outbox.countDocuments({event_id: acceptance.event_id}))
MONGOJS
chmod 0700 "$dir"
chmod 0600 "$dir/audit.js"
sudo -n docker run --rm --network infra-network --user "$(id -u):$(id -g)" \
  -v "$dir:/audit:ro" -e HOME=/tmp \
  -e MONGODB_DBNAME="$MONGODB_DBNAME" -e ANSWER_SHEET_ID="$ANSWER_SHEET_ID" \
  mongo:7.0 mongosh --host "$MONGODB_HOST" --port "${MONGODB_PORT:-27017}" \
  --username "$MONGODB_USERNAME" --password="$MONGODB_PASSWORD" \
  --authenticationDatabase admin --quiet --file /audit/audit.js
