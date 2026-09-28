#!/usr/bin/env bash
# One-off read-only active channel waterline before the MySQL profile cutover.
set -Eeuo pipefail
stats=$(curl -fsS --max-time 5 'http://127.0.0.1:4151/stats?format=json')
jq -e '.health == "OK"' <<<"$stats" >/dev/null
channels=$(jq -c '[.topics[] | select(.topic_name == "qs.evaluation.lifecycle") | .channels[] |
  select(.channel_name == "qs-worker" or .channel_name == "qs-apiserver-modelcatalog-hot-rank-v1") |
  {name: .channel_name, paused, clients: .client_count, depth, in_flight: .in_flight_count, deferred: .deferred_count}]' <<<"$stats")
[[ $(jq 'length' <<<"$channels") == 2 ]] || { echo 'expected worker and hot-rank channels' >&2; exit 1; }
jq -e 'all(.[]; .paused == false and .clients > 0)' <<<"$channels" >/dev/null || {
  echo 'assessment channel is paused or disconnected' >&2
  exit 1
}
printf 'nsq=OK active_assessment_channels=%s\n' "$channels"
