#!/usr/bin/env bash
# Read-only QS authorization subscription check for the approved M5-06 cutover.
# Run locally on serverA or serverD after the API-only deployment. No pause,
# role mutation, channel deletion, or message replay is performed here.
set -Eeuo pipefail

fail() { printf '%s\n' "$*" >&2; exit 1; }

case "${1:-serverA}" in
  serverA)
    for tool in sudo docker jq awk grep; do
      command -v "$tool" >/dev/null || fail "missing read-only prerequisite: $tool"
    done
    : "${EXPECTED_API_SHA:?full deployed API commit required}"
    [[ "$EXPECTED_API_SHA" =~ ^[0-9a-f]{40}$ ]] || fail 'EXPECTED_API_SHA must be a full SHA'
    container=qs-apiserver
    image=$(sudo -n docker inspect "$container" --format '{{.Config.Image}}')
    state=$(sudo -n docker inspect "$container" --format '{{.State.Status}}')
    health=$(sudo -n docker inspect "$container" --format '{{.State.Health.Status}}')
    [[ "$image" == *":${EXPECTED_API_SHA}" && "$state" == running && "$health" == healthy ]] ||
      fail 'API image, running state, or health differs from approved commit'

    config=/app/configs/apiserver.prod.yaml
    settings=$(sudo -n docker exec "$container" awk '
      /^  authz-version-guard:/ { section="guard"; next }
      /^  authz-sync:/ { section="sync"; next }
      section != "" && /^  [^ ]/ { section="" }
      section == "guard" && /^    (enabled|max-age|poll-interval|read-timeout):/ {
        sub(/[[:space:]]*#.*/, ""); gsub(/"/, "");
        sub(/^[[:space:]]+/, ""); sub(/[[:space:]]+$/, ""); print "guard " $0
      }
      section == "sync" && /^    (enabled|provider|topic|ephemeral-nsq):/ {
        sub(/[[:space:]]*#.*/, ""); gsub(/"/, "");
        sub(/^[[:space:]]+/, ""); sub(/[[:space:]]+$/, ""); print "sync " $0
      }
    ' "$config")
    for expected in 'guard enabled: true' 'guard max-age: 10s' \
                    'guard poll-interval: 5s' 'guard read-timeout: 2s' \
                    'sync enabled: true' 'sync provider: nsq' \
                    'sync topic: iam.authz.version.v2' 'sync ephemeral-nsq: true'; do
      [[ $(grep -Fxc "$expected" <<<"$settings") == 1 ]] ||
        fail "missing or duplicated authorization setting: $expected"
    done
    overrides=$(sudo -n docker inspect "$container" | jq -r '
      [.[0].Config.Env[]? | split("=")[0] |
       select(test("AUTHZ_SYNC|AUTHZ_VERSION_GUARD"))] | join(",")
    ')
    [[ -z "$overrides" ]] || fail "authorization settings have environment overrides requiring review: $overrides"
    flags=$(sudo -n docker inspect "$container" | jq -r '
      [((.[0].Config.Entrypoint // []) + (.[0].Config.Cmd // []))[] |
       select(startswith("--iam.authz-sync") or startswith("--iam.authz-version-guard"))] | length
    ')
    [[ "$flags" == 0 ]] || fail 'authorization settings have command-line overrides requiring review'
    printf 'PASS serverA API=%s healthy=true ephemeral-nsq=true committed-version-guard=10s/5s/2s overrides=none\n' "$EXPECTED_API_SHA"
    ;;
  serverD)
    for tool in curl jq; do
      command -v "$tool" >/dev/null || fail "missing read-only prerequisite: $tool"
    done
    stats=$(curl -fsS --max-time 5 'http://127.0.0.1:4151/stats?format=json')
    jq -e '.health == "OK"' <<<"$stats" >/dev/null || fail 'NSQ is not healthy'
    qs=$(jq -c '[.topics[] | select(.topic_name == "iam.authz.version.v2") | .channels[] |
      select((.channel_name | startswith("qs-authz-sync-apiserver-") and endswith("#ephemeral") and length <= 64)
        and .client_count > 0)]' <<<"$stats")
    [[ $(jq 'length' <<<"$qs") == 1 ]] || fail 'expected exactly one online QS ephemeral authorization channel'
    jq -e '.[0] | .paused == false and .client_count == 1 and .depth == 0 and .in_flight_count == 0 and .deferred_count == 0' <<<"$qs" >/dev/null ||
      fail 'online QS authorization channel is paused, shared, or backlogged'
    iam=$(jq -c '[.topics[] | select(.topic_name == "iam.authz.version.v2") | .channels[] |
      select((.channel_name | startswith("iam-policy-sync.")) and .client_count > 0)]' <<<"$stats")
    [[ $(jq 'length' <<<"$iam") == 1 ]] || fail 'expected one online IAM policy channel'
    failed=$(jq -c '[.topics[] | select(.topic_name == "cb.failed.5bbc2fd050986ad10b90cc51")]' <<<"$stats")
    [[ $(jq 'length' <<<"$failed") == 1 ]] || fail 'stable failed-handoff topic missing or duplicated'
    jq -e '.[0].channels | length == 1 and .[0].channel_name == "cb-failed-handler" and
      .[0].depth == 0 and .[0].in_flight_count == 0 and .[0].deferred_count == 0' <<<"$failed" >/dev/null ||
      fail 'stable failed-handoff channel is unexpected or has unreviewed work'
    offline=$(jq '[.topics[] | select(.topic_name == "iam.authz.version.v2") | .channels[] |
      select(.client_count == 0)] | length' <<<"$stats")
    printf 'PASS serverD NSQ=OK qs_channel=%s iam_online=1 stable_failed_handoff=empty offline_history_channels=%s\n' \
      "$(jq -r '.[0].channel_name' <<<"$qs")" "$offline"
    ;;
  *) fail 'usage: m5-authz-ephemeral-postcheck.sh serverA|serverD' ;;
esac
