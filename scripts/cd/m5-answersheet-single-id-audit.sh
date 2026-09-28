#!/usr/bin/env bash
# Exact-ID read-only Mongo answer and Outbox facts; never outputs answers.
set -Eeuo pipefail
: "${ANSWER_SHEET_ID:?}"
: "${ASSESSMENT_ID:?}"
: "${OUTCOME_ID:?}"
: "${MONGODB_HOST:?}"
: "${MONGODB_USERNAME:?}"
: "${MONGODB_PASSWORD:?}"
: "${MONGODB_DBNAME:?}"
[[ "$ANSWER_SHEET_ID" =~ ^[1-9][0-9]{0,19}$ ]] || { echo 'invalid answer sheet ID' >&2; exit 1; }
[[ "$ASSESSMENT_ID" =~ ^[1-9][0-9]{0,19}$ ]] || { echo 'invalid assessment ID' >&2; exit 1; }
[[ "$OUTCOME_ID" =~ ^[1-9][0-9]{0,19}$ ]] || { echo 'invalid outcome ID' >&2; exit 1; }

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

const assessmentID = Long.fromString(process.env.ASSESSMENT_ID)
const outcomeID = Long.fromString(process.env.OUTCOME_ID)
const generation = targetDB.report_generations.findOne(
  {outcome_id: outcomeID, deleted_at: null},
  {domain_id: 1, outcome_id: 1, status: 1, latest_run_id: 1,
   report_id: 1, transaction_schema_version: 1, created_at: 1, updated_at: 1})
print("REPORT_GENERATION")
printjson(generation || null)
const artifact = targetDB.interpret_report_artifacts.findOne(
  {assessment_id: assessmentID, outcome_id: outcomeID, deleted_at: null},
  {domain_id: 1, generation_id: 1, interpretation_run_id: 1,
   assessment_id: 1, outcome_id: 1, generated_at: 1, model: 1})
print("REPORT_ARTIFACT_METADATA")
printjson(artifact || null)
const catalog = targetDB.report_query_catalog.findOne(
  {assessment_id: assessmentID},
  {assessment_id: 1, outcome_id: 1, generation_id: 1,
   source_kind: 1, source_id: 1, updated_at: 1})
print("REPORT_CATALOG")
printjson(catalog || null)
const candidates = targetDB.rm_outbox.find(
  {event_type: "interpretation.report.generated",
   created_at: {$gte: new Date("2026-09-28T13:48:00Z")}},
  {message_id: 1, event_type: 1, state: 1, attempt_count: 1,
   failure_count: 1, last_error_code: 1, transport_confirmed_at: 1,
   created_at: 1, payload: 1, _id: 0}
).limit(101).toArray()
if (candidates.length > 100) throw new Error("report event audit budget exceeded")
const matched = candidates.filter(row => {
  const wire = JSON.parse(Buffer.from(row.payload.buffer).toString("utf8"))
  if (wire.type !== "component-base.messaging.message.v1" || !wire.payload)
    throw new Error("unexpected report event wire envelope")
  const domain = JSON.parse(Buffer.from(wire.payload, "base64").toString("utf8"))
  if (domain.eventType !== "interpretation.report.generated")
    throw new Error("unexpected report domain event type")
  return String(domain.data?.assessment_id) === process.env.ASSESSMENT_ID
}).map(({payload, ...metadata}) => metadata)
print("MATCHED_STANDARD_REPORT_EVENTS")
printjson({candidate_count: candidates.length, matched})
MONGOJS
chmod 0700 "$dir"
chmod 0600 "$dir/audit.js"
sudo -n docker run --rm --network infra-network --user "$(id -u):$(id -g)" \
  -v "$dir:/audit:ro" -e HOME=/tmp \
  -e MONGODB_DBNAME="$MONGODB_DBNAME" -e ANSWER_SHEET_ID="$ANSWER_SHEET_ID" \
  -e ASSESSMENT_ID="$ASSESSMENT_ID" -e OUTCOME_ID="$OUTCOME_ID" \
  mongo:7.0 mongosh --host "$MONGODB_HOST" --port "${MONGODB_PORT:-27017}" \
  --username "$MONGODB_USERNAME" --password="$MONGODB_PASSWORD" \
  --authenticationDatabase admin --quiet --file /audit/audit.js
