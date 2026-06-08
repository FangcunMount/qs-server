#!/usr/bin/env bash
set -euo pipefail

IAM_BASE_URL="${IAM_BASE_URL:-https://iam.fangcunmount.cn}"
IAM_LOGIN_URL="${IAM_LOGIN_URL:-${IAM_BASE_URL%/}/api/v2/authn/login}"
IAM_TENANT_ID="${IAM_TENANT_ID:-1}"
IAM_DEVICE_ID="${IAM_DEVICE_ID:-seeddata-k6}"
IAM_OMIT_TENANT_ID="${IAM_OMIT_TENANT_ID:-false}"

if [[ -z "${IAM_USERNAME:-}" || -z "${IAM_PASSWORD:-}" ]]; then
  echo "IAM_USERNAME and IAM_PASSWORD are required" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

tenant_id="$IAM_TENANT_ID"
case "$(printf '%s' "$IAM_OMIT_TENANT_ID" | tr '[:upper:]' '[:lower:]')" in
  1|true|yes|y|on)
    tenant_id=""
    ;;
esac

payload="$(
  jq -n \
    --arg username "$IAM_USERNAME" \
    --arg password "$IAM_PASSWORD" \
    --arg tenant_id "$tenant_id" \
    --arg device_id "$IAM_DEVICE_ID" \
    '{
      auth_method: "password",
      method_payload: {
        username: $username,
        password: $password
      },
      device_id: $device_id
    } | if $tenant_id != "" then .method_payload.tenant_id = ($tenant_id | tonumber) else . end'
)"

curl -fsS \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  -X POST "$IAM_LOGIN_URL" \
  -d "$payload" |
  jq -r '.data.access_token // .access_token // empty'
