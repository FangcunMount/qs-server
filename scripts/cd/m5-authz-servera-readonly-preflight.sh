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

# Report only the non-secret M5-05 feature flag. A default, mounted config or
# environment override can each determine the value used by Viper. An explicit
# command-line override is unexpected for this deployment and needs review.
reminder_file=$(sudo -n docker exec "$container" awk '
  /^eventing:/ { inside=1; next }
  inside && /^[^ ]/ { exit }
  inside && /^  task_opened_reminder:/ { print $2 }
' "$config")
[[ -n "$reminder_file" ]] || reminder_file=unset
[[ "$reminder_file" =~ ^(true|false|unset)$ ]] || {
  echo "invalid or duplicate task_opened_reminder config value" >&2
  exit 1
}
reminder_env=$(sudo -n docker inspect "$container" | jq -r '
  [.[0].Config.Env[]? | select(startswith("QS_APISERVER_EVENTING_TASK_OPENED_REMINDER=")) | split("=")[1]]
  | if length == 0 then "unset" elif length == 1 then .[0] else "duplicate" end
')
[[ "$reminder_env" =~ ^(true|false|unset)$ ]] || {
  echo "invalid or duplicate task_opened_reminder environment value" >&2
  exit 1
}
reminder_flags=$(sudo -n docker inspect "$container" | jq -r '
  [.[0].Config.Cmd[]? | select(startswith("--eventing.task-opened-reminder"))] | length
')
[[ "$reminder_flags" == 0 ]] || {
  echo "explicit task_opened_reminder command flag requires manual review" >&2
  exit 1
}
reminder_effective=false
[[ "$reminder_file" == unset ]] || reminder_effective=$reminder_file
[[ "$reminder_env" == unset ]] || reminder_effective=$reminder_env

timers=$(systemctl list-timers --all --no-legend --no-pager) || {
  echo "cannot inspect existing M5 recovery timers" >&2
  exit 1
}
active=$(printf '%s\n' "$timers" | grep -E 'rm-m5-.*(role-restore|channel-unpause)' || true)
[[ -z "$active" ]] || { echo "a prior M5 recovery timer already exists" >&2; exit 1; }

printf 'API image=%s state=%s health=%s hostname=%s\n' "$image" "$state" "$health" "$hostname"
printf 'guard=enabled/10s/5s/2s authz_sync=NSQ/persistent recovery_tools=present prior_timers=none\n'
printf 'task_opened_reminder file=%s env=%s effective=%s\n' "$reminder_file" "$reminder_env" "$reminder_effective"
