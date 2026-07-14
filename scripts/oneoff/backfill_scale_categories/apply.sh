#!/usr/bin/env bash
set -euo pipefail

# Backfill category through the protected model-management and release APIs.
# It never writes published_assessment_models directly: basic-info forks the
# published draft, and assessment-releases/{code}/publish creates the new
# immutable snapshot through the normal release transaction.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
ASSIGNMENTS_FILE="${ASSIGNMENTS_FILE:-${ROOT_DIR}/scripts/oneoff/backfill_scale_categories/assignments.json}"
APPLY=false

usage() {
  cat <<'EOF'
Usage:
  scripts/oneoff/backfill_scale_categories/apply.sh --dry-run
  QS_APISERVER_URL=https://qs.example.com QS_OPERATOR_TOKEN=... \
    scripts/oneoff/backfill_scale_categories/apply.sh --apply

Environment:
  QS_APISERVER_URL   apiserver origin, without /api/v1 (required for --apply)
  QS_COLLECTION_URL  collection-server origin (default: https://collect.fangcunmount.cn)
  QS_OPERATOR_TOKEN  operator bearer token with Manage + Publish AssessmentModels (required for --apply)
  ASSIGNMENTS_FILE   optional path to the reviewed JSON assignment manifest
EOF
}

case "${1:---dry-run}" in
  --dry-run) ;;
  --apply) APPLY=true ;;
  -h|--help) usage; exit 0 ;;
  *) usage >&2; exit 2 ;;
esac

command -v curl >/dev/null || { echo "curl is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
[[ -f "$ASSIGNMENTS_FILE" ]] || { echo "assignment manifest not found: $ASSIGNMENTS_FILE" >&2; exit 1; }

allowed_categories='["adhd","td","asd","pressure","sii","efn","emt","slp","personality"]'
jq -e --argjson allowed "$allowed_categories" '
  type == "array" and length == 21 and
  (([.[].code] | length) == ([.[].code] | unique | length)) and
  all(.[]; (.code | type == "string" and length > 0) and (.category as $category | $allowed | index($category) != null) and ((.skip // false) | type == "boolean"))
' "$ASSIGNMENTS_FILE" >/dev/null || {
  echo "assignment manifest must contain 21 unique reviewed codes and supported categories" >&2
  exit 1
}

echo "Reviewed assignments:"
jq -r '.[] | "  \(.code)  \(.category)  \(.reason)\(if (.skip // false) then " [skip]" else "" end)"' "$ASSIGNMENTS_FILE"

if ! "$APPLY"; then
  echo "Dry run only. No model or published snapshot was changed."
  exit 0
fi

: "${QS_APISERVER_URL:?QS_APISERVER_URL is required with --apply}"
: "${QS_OPERATOR_TOKEN:?QS_OPERATOR_TOKEN is required with --apply}"
QS_COLLECTION_URL="${QS_COLLECTION_URL:-https://collect.fangcunmount.cn}"
API_BASE="${QS_APISERVER_URL%/}/api/v1"
COLLECTION_BASE="${QS_COLLECTION_URL%/}/api/v1"
PREFLIGHT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/qs-scale-categories.XXXXXX")"
trap 'rm -rf "$PREFLIGHT_DIR"' EXIT

request() {
  local method="$1" url="$2" body="${3:-}"
  local response
  if [[ -n "$body" ]]; then
    response="$(curl -sS --fail-with-body -X "$method" "$url" \
      -H "Authorization: Bearer ${QS_OPERATOR_TOKEN}" \
      -H 'Content-Type: application/json' \
      --data "$body")"
  else
    response="$(curl -sS --fail-with-body -X "$method" "$url" \
      -H "Authorization: Bearer ${QS_OPERATOR_TOKEN}")"
  fi
  jq -e '.code == 0' >/dev/null <<<"$response" || {
    echo "request failed: ${method} ${url}" >&2
    jq -c '.' <<<"$response" >&2
    return 1
  }
  printf '%s' "$response"
}

preflight_failures=()
while IFS=$'\t' read -r code category skip; do
  if [[ "$skip" == "true" ]]; then
    echo "preflight skip ${code}: explicitly archived"
    continue
  fi
  if ! model="$(request GET "${API_BASE}/assessment-models/${code}")"; then
    preflight_failures+=("${code}")
    continue
  fi
  printf '%s' "$model" > "${PREFLIGHT_DIR}/${code}.json"
done < <(jq -r '.[] | [.code, .category, (.skip // false)] | @tsv' "$ASSIGNMENTS_FILE")

if (( ${#preflight_failures[@]} > 0 )); then
  echo "Backfill was not applied. The following models cannot be read as editable drafts:" >&2
  printf '  %s\n' "${preflight_failures[@]}" >&2
  echo "Repair or explicitly archive/remove their stale published snapshots before rerunning." >&2
  exit 2
fi

while IFS=$'\t' read -r code category skip; do
  if [[ "$skip" == "true" ]]; then
    echo "skip ${code}: explicitly archived; no metadata update or republish"
    continue
  fi
  model="$(<"${PREFLIGHT_DIR}/${code}.json")"
  current_category="$(jq -r '.data.category // ""' <<<"$model")"
  if [[ "$current_category" == "$category" ]]; then
    echo "skip ${code}: category already ${category}"
    continue
  fi

  payload="$(jq -c --arg category "$category" '.data | {
    title, description, sub_kind, algorithm, product_channel,
    category: $category, stages, applicable_ages, reporters, tags
  }' <<<"$model")"
  request PUT "${API_BASE}/assessment-models/${code}/basic-info" "$payload" >/dev/null
  request POST "${API_BASE}/assessment-releases/${code}/publish" >/dev/null
  echo "published ${code}: ${current_category:-<empty>} -> ${category}"
done < <(jq -r '.[] | [.code, .category, (.skip // false)] | @tsv' "$ASSIGNMENTS_FILE")

echo "Verifying public medical catalogue categories:"
while read -r category expected; do
  response="$(curl -sS --fail-with-body "${COLLECTION_BASE}/assessment-models?kind=scale&category=${category}&page=1&page_size=100")"
  actual="$(jq -r '.data.total // -1' <<<"$response")"
  if [[ "$actual" != "$expected" ]]; then
    echo "verification failed: category=${category}, got ${actual}, want ${expected}" >&2
    exit 1
  fi
  echo "  ${category}: ${actual}"
done <<'EOF'
adhd 5
td 2
asd 2
pressure 1
sii 2
efn 1
emt 3
slp 2
EOF

echo "Scale category backfill completed."
