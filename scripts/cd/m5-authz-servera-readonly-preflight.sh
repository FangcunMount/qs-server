#!/usr/bin/env bash
# Read-only M5-06 gate. This script never installs a timer or changes authorization.
set -Eeuo pipefail

for tool in sudo docker systemctl systemd-run timeout sha256sum jq; do
  command -v "$tool" >/dev/null || { echo "missing recovery prerequisite: $tool" >&2; exit 1; }
done
: "${EXPECTED_API_SHA:?expected API commit is required}"
[[ "$EXPECTED_API_SHA" =~ ^[0-9a-f]{40}$ ]] || { echo "expected API commit must be a full SHA" >&2; exit 1; }

container=qs-apiserver
image=$(sudo -n docker inspect "$container" --format '{{.Config.Image}}')
state=$(sudo -n docker inspect "$container" --format '{{.State.Status}}')
health=$(sudo -n docker inspect "$container" --format '{{.State.Health.Status}}')
hostname=$(sudo -n docker inspect "$container" --format '{{.Config.Hostname}}')
[[ "$image" == *":${EXPECTED_API_SHA}" && "$state" == running && "$health" == healthy ]] || {
  echo "API image or health differs from this workflow commit" >&2
  exit 1
}
[[ "$hostname" =~ ^[0-9a-f]{12}$ ]] || { echo "unexpected API container hostname" >&2; exit 1; }

config=/app/configs/apiserver.prod.yaml
guard=$(sudo -n docker exec "$container" awk '
  /^  authz-version-guard:/ { inside=1; next }
  inside && /^  [^ ]/ { exit }
  inside && /^    (enabled|max-age|poll-interval|read-timeout):/ { print }
' "$config")
sync=$(sudo -n docker exec "$container" awk '
  /^  authz-sync:/ { inside=1; next }
  inside && /^  [^ ]/ { exit }
  inside && /^    (enabled|provider|topic|ephemeral-nsq):/ { print }
' "$config")
for expected in 'enabled: true' 'max-age: 10s' 'poll-interval: 5s' 'read-timeout: 2s'; do
  printf '%s\n' "$guard" | grep -Eq "^[[:space:]]*${expected}([[:space:]#]|$)" || {
    echo "missing expected guard setting: $expected" >&2
    exit 1
  }
done
printf '%s\n' "$sync" | grep -Eq '^[[:space:]]*enabled:[[:space:]]*true([[:space:]#]|$)'
printf '%s\n' "$sync" | grep -Eq '^[[:space:]]*provider:[[:space:]]*"?nsq"?([[:space:]#]|$)'
printf '%s\n' "$sync" | grep -Eq '^[[:space:]]*topic:[[:space:]]*"?iam\.authz\.version\.v2"?([[:space:]#]|$)'
printf '%s\n' "$sync" | grep -Eq '^[[:space:]]*ephemeral-nsq:[[:space:]]*false([[:space:]#]|$)'

timers=$(systemctl list-timers --all --no-legend --no-pager) || {
  echo "cannot inspect existing M5 recovery timers" >&2
  exit 1
}
active=$(printf '%s\n' "$timers" | grep -E 'rm-m5-.*(role-restore|channel-unpause)' || true)
[[ -z "$active" ]] || { echo "a prior M5 recovery timer already exists" >&2; exit 1; }

printf 'API image=%s state=%s health=%s hostname=%s\n' "$image" "$state" "$health" "$hostname"
printf 'guard=enabled/10s/5s/2s authz_sync=NSQ/persistent recovery_tools=present prior_timers=none\n'
