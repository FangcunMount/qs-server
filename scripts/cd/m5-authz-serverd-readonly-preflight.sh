#!/usr/bin/env bash
# Read-only M5-06 gate. This script never pauses or drains an NSQ channel.
set -Eeuo pipefail

for tool in curl jq systemctl systemd-run; do
  command -v "$tool" >/dev/null || { echo "missing recovery prerequisite: $tool" >&2; exit 1; }
done

stats=$(curl -fsS --max-time 5 'http://127.0.0.1:4151/stats?format=json')
printf '%s\n' "$stats" | jq -e '.health == "OK"' >/dev/null || {
  echo "NSQ is not healthy" >&2
  exit 1
}
channels=$(printf '%s\n' "$stats" | jq -c '[
  .topics[] | select(.topic_name == "iam.authz.version.v2") | .channels[] |
  select(.channel_name | test("^qs-authz-sync-apiserver-[0-9a-f]{12}-[0-9]+$")) |
  select(.client_count > 0)
]')
[[ $(printf '%s\n' "$channels" | jq 'length') -eq 1 ]] || {
  echo "expected exactly one online QS authorization channel" >&2
  exit 1
}
printf '%s\n' "$channels" | jq -e '.[0] | .paused == false and .client_count == 1 and .depth == 0 and .in_flight_count == 0 and .deferred_count == 0' >/dev/null || {
  echo "the current QS authorization channel is not ready for a bounded test" >&2
  exit 1
}
channel=$(printf '%s\n' "$channels" | jq -r '.[0].channel_name')

active=$(systemctl list-timers --all --no-legend --no-pager | grep -E 'rm-m5-.*(role-restore|channel-unpause)' || true)
[[ -z "$active" ]] || { echo "a prior M5 recovery timer already exists" >&2; exit 1; }

printf 'NSQ health=%s channel=%s paused=false clients=1 depth=0 in_flight=0 deferred=0\n' \
  "$(printf '%s\n' "$stats" | jq -r .health)" "$channel"
printf 'authz_v2_channels=%s offline=%s failed_handoff_topics=%s recovery_tools=present prior_timers=none\n' \
  "$(printf '%s\n' "$stats" | jq '[.topics[] | select(.topic_name == "iam.authz.version.v2") | .channels[]] | length')" \
  "$(printf '%s\n' "$stats" | jq '[.topics[] | select(.topic_name == "iam.authz.version.v2") | .channels[] | select(.client_count == 0)] | length')" \
  "$(printf '%s\n' "$stats" | jq '[.topics[] | select(.topic_name | startswith("cb.failed."))] | length')"
