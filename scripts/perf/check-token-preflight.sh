#!/usr/bin/env bash
set -euo pipefail

CONFIG_FILE="${PERF_CONFIG_FILE:-tmp/perf/qs-perf.config.json}"

if [[ ! -f "$CONFIG_FILE" ]]; then
  echo "config file not found: $CONFIG_FILE" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi

config_dir="$(cd "$(dirname "$CONFIG_FILE")" && pwd)"

json_value() {
  jq -r "$1 // empty" "$CONFIG_FILE"
}

resolve_path() {
  local path="$1"
  if [[ -z "$path" ]]; then
    return 0
  fi
  if [[ "$path" == /* ]]; then
    printf '%s\n' "$path"
  else
    printf '%s/%s\n' "$config_dir" "$path"
  fi
}

count_tokens() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo 0
    return
  fi
  jq 'if type == "array" then length elif type == "object" and (.tokens|type) == "array" then .tokens|length else 0 end' "$file"
}

first_token() {
  local file="$1"
  if [[ -z "$file" || ! -f "$file" ]]; then
    return 0
  fi
  jq -r 'if type == "array" then .[0] elif type == "object" and (.tokens|type) == "array" then .tokens[0] else empty end' "$file"
}

token_expiry_summary() {
  local label="$1"
  local file="$2"
  if [[ -z "$file" || ! -f "$file" ]]; then
    echo "$label: file missing"
    return
  fi
  node - "$label" "$file" <<'NODE'
const fs = require("fs");
const label = process.argv[2];
const file = process.argv[3];
const tokens = JSON.parse(fs.readFileSync(file, "utf8"));
const list = Array.isArray(tokens) ? tokens : (Array.isArray(tokens.tokens) ? tokens.tokens : []);
const now = Math.floor(Date.now() / 1000);
let expired = 0;
let invalid = 0;
let minExp = Infinity;
let maxExp = 0;
for (const token of list) {
  const part = String(token || "").split(".")[1] || "";
  if (!part) {
    invalid += 1;
    continue;
  }
  let base64 = part.replace(/-/g, "+").replace(/_/g, "/");
  while (base64.length % 4) base64 += "=";
  try {
    const claims = JSON.parse(Buffer.from(base64, "base64").toString("utf8"));
    if (!claims.exp) {
      invalid += 1;
      continue;
    }
    if (claims.exp <= now) expired += 1;
    minExp = Math.min(minExp, claims.exp);
    maxExp = Math.max(maxExp, claims.exp);
  } catch (_) {
    invalid += 1;
  }
}
const minTTL = minExp === Infinity ? "n/a" : String(minExp - now);
const maxTTL = maxExp === 0 ? "n/a" : String(maxExp - now);
console.log(`${label}: count=${list.length} expired=${expired} invalid=${invalid} min_ttl_seconds=${minTTL} max_ttl_seconds=${maxTTL}`);
NODE
}

http_status() {
  local label="$1"
  local url="$2"
  local token="$3"
  if [[ -z "$token" ]]; then
    echo "$label: no token"
    return
  fi
  local status
  status="$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $token" "$url")"
  echo "$label: $status"
}

http_json_status() {
  local label="$1"
  local url="$2"
  local token="$3"
  local body="$4"
  if [[ -z "$token" ]]; then
    echo "$label: no token"
    return
  fi
  local status
  status="$(curl -sS -o /dev/null -w '%{http_code}' \
    -X POST \
    -H "Authorization: Bearer $token" \
    -H 'Content-Type: application/json' \
    --data "$body" \
    "$url")"
  echo "$label: $status"
}

tokens_file="$(resolve_path "$(json_value '.tokensFile')")"
collection_tokens_file="$(resolve_path "$(json_value '.collectionTokensFile')")"
apiserver_tokens_file="$(resolve_path "$(json_value '.apiserverTokensFile')")"

collection_effective_file="$tokens_file"
if [[ -n "$collection_tokens_file" && -f "$collection_tokens_file" ]]; then
  collection_effective_file="$collection_tokens_file"
fi

apiserver_effective_file="$tokens_file"
if [[ -n "$apiserver_tokens_file" && -f "$apiserver_tokens_file" ]]; then
  apiserver_effective_file="$apiserver_tokens_file"
fi

collection_base_url="$(json_value '.collectionBaseUrl')"
apiserver_base_url="$(json_value '.apiserverBaseUrl')"
scale_code="$(jq -r '(.scaleCodes // ["3adyDE"])[0]' "$CONFIG_FILE")"
personality_model_code="$(jq -r '(.personalityModelCodes // ["MBTI_OEJTS"])[0]' "$CONFIG_FILE")"
questionnaire_code="$(jq -r '(.questionnaireCodes // [])[0] // empty' "$CONFIG_FILE")"
org_id="$(json_value '.orgId')"
if [[ -z "$org_id" ]]; then
  org_id="1"
fi

echo "config=$CONFIG_FILE"
echo "tokensFile=$tokens_file count=$(count_tokens "$tokens_file")"
if [[ -n "$collection_tokens_file" ]]; then
  echo "collectionTokensFile=$collection_tokens_file count=$(count_tokens "$collection_tokens_file")"
fi
if [[ -n "$apiserver_tokens_file" ]]; then
  echo "apiserverTokensFile=$apiserver_tokens_file count=$(count_tokens "$apiserver_tokens_file")"
fi

token_expiry_summary "collection_effective" "$collection_effective_file"
token_expiry_summary "apiserver_effective" "$apiserver_effective_file"

collection_token="$(first_token "$collection_effective_file")"
apiserver_token="$(first_token "$apiserver_effective_file")"

if [[ -z "$questionnaire_code" && -n "$collection_token" && -n "$scale_code" ]]; then
  questionnaire_code="$(curl -sS -H "Authorization: Bearer $collection_token" \
    "${collection_base_url%/}/api/v1/assessment-models/${scale_code}" 2>/dev/null | jq -r '.questionnaire_code // .data.questionnaire_code // empty' 2>/dev/null || true)"
fi

http_status "collection assessment model list" "${collection_base_url%/}/api/v1/assessment-models?kind=scale&page=1&page_size=20" "$collection_token"
http_status "collection assessment model options" "${collection_base_url%/}/api/v1/assessment-models/options?kind=scale" "$collection_token"
http_status "collection assessment model hot" "${collection_base_url%/}/api/v1/assessment-models/hot?kind=scale&limit=5" "$collection_token"
http_status "collection assessment model ${scale_code}" "${collection_base_url%/}/api/v1/assessment-models/${scale_code}" "$collection_token"
http_status "collection typology models" "${collection_base_url%/}/api/v1/typology-models?page=1&page_size=1" "$collection_token"
http_status "collection typology categories" "${collection_base_url%/}/api/v1/typology-models/categories" "$collection_token"
http_status "collection typology model ${personality_model_code}" "${collection_base_url%/}/api/v1/typology-models/${personality_model_code}" "$collection_token"
if [[ -n "$questionnaire_code" ]]; then
  http_status "collection questionnaire ${questionnaire_code}" "${collection_base_url%/}/api/v1/questionnaires/${questionnaire_code}" "$collection_token"
else
  echo "collection questionnaire: skipped (no questionnaire_code)"
fi
http_status "apiserver testees" "${apiserver_base_url%/}/api/v1/testees?org_id=${org_id}&page=1&page_size=1" "$apiserver_token"
http_status "apiserver statistics overview" "${apiserver_base_url%/}/api/v1/statistics/overview?preset=7d" "$apiserver_token"
http_json_status "apiserver statistics content batch" "${apiserver_base_url%/}/api/v1/statistics/contents/batch" "$apiserver_token" \
  "{\"items\":[{\"type\":\"scale\",\"code\":\"${scale_code}\"}]}"
