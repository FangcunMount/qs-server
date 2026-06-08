#!/usr/bin/env bash
set -euo pipefail

IAM_BASE_URL="${IAM_BASE_URL:-https://iam.fangcunmount.cn}"
IAM_LOGIN_URL="${IAM_LOGIN_URL:-${IAM_BASE_URL%/}/api/v2/authn/login}"
IAM_TENANT_ID="${IAM_TENANT_ID:-1}"
IAM_DEVICE_ID_PREFIX="${IAM_DEVICE_ID_PREFIX:-seeddata-k6}"
IAM_USERS_FILE="${IAM_USERS_FILE:-}"
IAM_USERS_GROUP="${IAM_USERS_GROUP:-}"
IAM_OMIT_TENANT_ID="${IAM_OMIT_TENANT_ID:-}"
IAM_USERS_LIMIT="${IAM_USERS_LIMIT:-0}"
TOKENS_OUTPUT_FILE="${TOKENS_OUTPUT_FILE:-}"
selected_users_group="$IAM_USERS_GROUP"

if [[ -z "$selected_users_group" ]]; then
  output_lower="$(printf '%s' "$TOKENS_OUTPUT_FILE" | tr '[:upper:]' '[:lower:]')"
  if [[ "$output_lower" == *collection* ]]; then
    selected_users_group="collection_users"
  elif [[ "$output_lower" == *apiserver* || "$output_lower" == *api-server* || "$output_lower" == *api_server* ]]; then
    selected_users_group="apiserver_users"
  fi
fi

if [[ -z "$IAM_USERS_FILE" ]]; then
  echo "IAM_USERS_FILE is required" >&2
  exit 1
fi

if [[ ! -f "$IAM_USERS_FILE" ]]; then
  echo "IAM_USERS_FILE not found: $IAM_USERS_FILE" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

users_json="$(
  jq -c \
    --arg group "$selected_users_group" \
  '
    def only_non_empty_group:
      if (((.collection_users // []) | length) > 0) and (((.apiserver_users // []) | length) == 0) then .collection_users
      elif (((.apiserver_users // []) | length) > 0) and (((.collection_users // []) | length) == 0) then .apiserver_users
      else []
      end;

    if type == "array" then .
    elif $group != "" then (.[$group] // [])
    else (.users // .credentials // only_non_empty_group)
    end
  ' "$IAM_USERS_FILE"
)"

count="$(jq 'length' <<<"$users_json")"
if [[ "$count" -le 0 ]]; then
  if [[ -n "$selected_users_group" ]]; then
    echo "IAM_USERS_FILE contains no users for IAM_USERS_GROUP=$selected_users_group" >&2
  else
    echo "IAM_USERS_FILE contains no users. Set IAM_USERS_GROUP=collection_users or IAM_USERS_GROUP=apiserver_users when using grouped credentials." >&2
  fi
  exit 1
fi

if [[ "$IAM_USERS_LIMIT" =~ ^[0-9]+$ && "$IAM_USERS_LIMIT" -gt 0 && "$IAM_USERS_LIMIT" -lt "$count" ]]; then
  count="$IAM_USERS_LIMIT"
fi

tokens_json='[]'

for ((i = 0; i < count; i++)); do
  user_json="$(jq -c ".[$i]" <<<"$users_json")"
  username="$(jq -r '.username // .user // empty' <<<"$user_json")"
  password="$(jq -r '.password // empty' <<<"$user_json")"
  tenant_id="$(jq -r '.tenant_id // .tenantId // empty' <<<"$user_json")"
  device_id="$(jq -r '.device_id // .deviceId // empty' <<<"$user_json")"
  omit_tenant_id="$(jq -r '.omit_tenant_id // .omitTenantId // empty' <<<"$user_json")"

  if [[ -z "$username" || -z "$password" ]]; then
    echo "user index $i missing username/password" >&2
    exit 1
  fi
  if [[ -z "$omit_tenant_id" ]]; then
    omit_tenant_id="$IAM_OMIT_TENANT_ID"
  fi
  if [[ -z "$omit_tenant_id" && "$selected_users_group" == "collection_users" ]]; then
    omit_tenant_id="true"
  fi

  case "$(printf '%s' "$omit_tenant_id" | tr '[:upper:]' '[:lower:]')" in
    1|true|yes|y|on)
      tenant_id=""
      ;;
    *)
      if [[ -z "$tenant_id" ]]; then
        tenant_id="$IAM_TENANT_ID"
      fi
      ;;
  esac
  if [[ -z "$device_id" ]]; then
    device_id="${IAM_DEVICE_ID_PREFIX}-$((i + 1))"
  fi

  payload="$(
    jq -n \
      --arg username "$username" \
      --arg password "$password" \
      --arg tenant_id "$tenant_id" \
      --arg device_id "$device_id" \
      '{
        auth_method: "password",
        method_payload: {
          username: $username,
          password: $password
        },
        device_id: $device_id
      } | if $tenant_id != "" then .method_payload.tenant_id = ($tenant_id | tonumber) else . end'
  )"

  response="$(
    curl -fsS \
      -H 'Accept: application/json' \
      -H 'Content-Type: application/json' \
      -X POST "$IAM_LOGIN_URL" \
      -d "$payload"
  )" || {
    echo "IAM login failed for user index $i in group ${selected_users_group:-<default>}" >&2
    exit 1
  }

  token="$(jq -r '.data.access_token // .access_token // empty' <<<"$response")"

  if [[ -z "$token" ]]; then
    echo "IAM login returned empty token for user index $i" >&2
    exit 1
  fi

  tokens_json="$(jq --arg token "$token" '. + [$token]' <<<"$tokens_json")"
done

if [[ -n "$TOKENS_OUTPUT_FILE" ]]; then
  mkdir -p "$(dirname "$TOKENS_OUTPUT_FILE")"
  umask 077
  jq -c '.' <<<"$tokens_json" >"$TOKENS_OUTPUT_FILE"
  echo "$TOKENS_OUTPUT_FILE"
else
  jq -r 'join(",")' <<<"$tokens_json"
fi
