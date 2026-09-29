#!/usr/bin/env bash
set -Eeuo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
fixture_dir=$(mktemp -d)
trap 'rm -rf "$fixture_dir"' EXIT

cat >"$fixture_dir/curl" <<'EOF'
#!/usr/bin/env bash
cat "$FAKE_NSQ_STATS_FILE"
EOF
chmod +x "$fixture_dir/curl"

cat >"$fixture_dir/stats.json" <<'EOF'
{
  "health": "OK",
  "topics": [
    {"topic_name": "iam.authz.version.v2", "channels": [
      {"channel_name": "iam-policy-sync.test#ephemeral", "client_count": 1, "paused": false, "depth": 0, "in_flight_count": 0, "deferred_count": 0},
      {"channel_name": "qs-authz-sync-apiserver-abcdef123456-1#ephemeral", "client_count": 1, "paused": false, "depth": 0, "in_flight_count": 0, "deferred_count": 0},
      {"channel_name": "qs-authz-sync-apiserver-old-1", "client_count": 0, "paused": false, "depth": 3, "in_flight_count": 0, "deferred_count": 0}
    ]},
    {"topic_name": "cb.failed.5bbc2fd050986ad10b90cc51", "channels": [
      {"channel_name": "cb-failed-handler", "client_count": 1, "paused": false, "depth": 0, "in_flight_count": 0, "deferred_count": 0}
    ]}
  ]
}
EOF

export FAKE_NSQ_STATS_FILE="$fixture_dir/stats.json"
PATH="$fixture_dir:$PATH" bash "$script_dir/m5-authz-ephemeral-postcheck.sh" serverD >"$fixture_dir/pass.log"
grep -q '^PASS serverD ' "$fixture_dir/pass.log"

jq '(.topics[] | select(.topic_name == "iam.authz.version.v2") | .channels[] |
  select(.channel_name | startswith("qs-authz-sync-apiserver-")) | .paused) = true' \
  "$fixture_dir/stats.json" >"$fixture_dir/paused.json"
export FAKE_NSQ_STATS_FILE="$fixture_dir/paused.json"
if PATH="$fixture_dir:$PATH" bash "$script_dir/m5-authz-ephemeral-postcheck.sh" serverD >"$fixture_dir/paused.log" 2>&1; then
  echo 'paused QS authorization channel unexpectedly passed' >&2
  exit 1
fi

jq '(.topics[] | select(.topic_name == "iam.authz.version.v2") | .channels[] |
  select(.channel_name == "qs-authz-sync-apiserver-old-1") | .client_count) = 1' \
  "$fixture_dir/stats.json" >"$fixture_dir/old-online.json"
export FAKE_NSQ_STATS_FILE="$fixture_dir/old-online.json"
if PATH="$fixture_dir:$PATH" bash "$script_dir/m5-authz-ephemeral-postcheck.sh" serverD >"$fixture_dir/old-online.log" 2>&1; then
  echo 'concurrent old QS authorization channel unexpectedly passed' >&2
  exit 1
fi

jq '(.topics[] | select(.topic_name == "cb.failed.5bbc2fd050986ad10b90cc51") |
  .channels[0].depth) = 1' "$fixture_dir/stats.json" >"$fixture_dir/failed-backlog.json"
export FAKE_NSQ_STATS_FILE="$fixture_dir/failed-backlog.json"
if PATH="$fixture_dir:$PATH" bash "$script_dir/m5-authz-ephemeral-postcheck.sh" serverD >"$fixture_dir/failed-backlog.log" 2>&1; then
  echo 'failed-handoff backlog unexpectedly passed' >&2
  exit 1
fi

echo 'PASS M5 AuthZ ephemeral NSQ postcheck fixtures'
