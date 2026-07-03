#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

if ! command -v ghz >/dev/null 2>&1; then
  echo "ghz is not installed. Install it first, for example: go install github.com/bojand/ghz/cmd/ghz@latest" >&2
  exit 127
fi

case_name="${CASE:-collection-submit}"
target="${GRPC_TARGET:-127.0.0.1:9090}"
rps="${RPS:-60}"
duration="${DURATION:-300s}"
concurrency="${CONCURRENCY:-60}"
timeout="${TIMEOUT:-30s}"
format="${FORMAT:-pretty}"
output="${OUTPUT:-}"
metadata="${GHZ_METADATA_JSON:-{}}"

proto=""
call=""
data=""

questionnaire_code="${QUESTIONNAIRE_CODE:-${Q_CODE:-QCODE_DEMO}}"
questionnaire_version="${QUESTIONNAIRE_VERSION:-${Q_VER:-1.0}}"
writer_id="${WRITER_ID:-601002327771460142}"
testee_id="${TESTEE_ID:-601002327771460142}"
org_id="${ORG_ID:-1}"
answers_json="${ANSWERS_JSON:-[{\"question_code\":\"Q1\",\"question_type\":\"Radio\",\"value\":\"A\"}]}"
answersheet_id="${ANSWERSHEET_ID:-1}"
assessment_id="${ASSESSMENT_ID:-1}"

case "$case_name" in
  collection-submit)
    proto="api/grpc/proto/answersheet/answersheet.proto"
    call="answersheet.AnswerSheetService.SaveAnswerSheet"
    data='{
      "questionnaire_code": "'"$questionnaire_code"'",
      "questionnaire_version": "'"$questionnaire_version"'",
      "idempotency_key": "ghz-collection-submit-{{.RequestNumber}}-{{.UUID}}",
      "title": "ghz collection submit equivalent",
      "writer_id": '"$writer_id"',
      "testee_id": '"$testee_id"',
      "org_id": '"$org_id"',
      "answers": '"$answers_json"'
    }'
    ;;
  worker-score)
    proto="api/grpc/proto/internalapi/internal.proto"
    call="internalapi.InternalService.CalculateAnswerSheetScore"
    data='{"answersheet_id": '"$answersheet_id"'}'
    ;;
  worker-create-assessment)
    proto="api/grpc/proto/internalapi/internal.proto"
    call="internalapi.InternalService.CreateAssessmentFromAnswerSheet"
    data='{
      "answersheet_id": '"$answersheet_id"',
      "questionnaire_code": "'"$questionnaire_code"'",
      "questionnaire_version": "'"$questionnaire_version"'",
      "testee_id": '"$testee_id"',
      "org_id": '"$org_id"',
      "filler_id": '"$writer_id"',
      "filler_type": "'"${FILLER_TYPE:-self}"'",
      "origin_type": "'"${ORIGIN_TYPE:-adhoc}"'"
    }'
    ;;
  worker-evaluate)
    proto="api/grpc/proto/internalapi/internal.proto"
    call="internalapi.InternalService.EvaluateAssessment"
    data='{"assessment_id": '"$assessment_id"'}'
    ;;
  worker-attention)
    proto="api/grpc/proto/internalapi/internal.proto"
    call="internalapi.InternalService.SyncAssessmentAttention"
    data='{
      "testee_id": '"$testee_id"',
      "risk_level": "'"${RISK_LEVEL:-high}"'",
      "mark_key_focus": '"${MARK_KEY_FOCUS:-true}"'
    }'
    ;;
  *)
    echo "Unknown CASE=$case_name. Use collection-submit, worker-score, worker-create-assessment, worker-evaluate, or worker-attention." >&2
    exit 2
    ;;
esac

tls_args=()
if [[ "${GRPC_PLAINTEXT:-false}" == "true" ]]; then
  tls_args+=(--insecure)
else
  if [[ -n "${GRPC_CACERT:-}" ]]; then
    tls_args+=(--cacert "$GRPC_CACERT")
  fi
  if [[ -n "${GRPC_CERT:-}" ]]; then
    tls_args+=(--cert "$GRPC_CERT")
  fi
  if [[ -n "${GRPC_KEY:-}" ]]; then
    tls_args+=(--key "$GRPC_KEY")
  fi
  if [[ -n "${GRPC_CNAME:-}" ]]; then
    tls_args+=(--cname "$GRPC_CNAME")
  fi
  if [[ "${GRPC_SKIP_TLS_VERIFY:-false}" == "true" ]]; then
    tls_args+=(--skipTLS)
  fi
fi

output_args=()
if [[ -n "$output" ]]; then
  output_args+=(--output "$output")
fi

echo "Running ghz case=$case_name target=$target rps=$rps duration=$duration concurrency=$concurrency"
ghz \
  --proto "$proto" \
  --import-paths "api/grpc/proto" \
  --call "$call" \
  --data "$data" \
  --metadata "$metadata" \
  --rps "$rps" \
  --duration "$duration" \
  --concurrency "$concurrency" \
  --timeout "$timeout" \
  --format "$format" \
  --count-errors \
  "${tls_args[@]}" \
  "${output_args[@]}" \
  "$target"
