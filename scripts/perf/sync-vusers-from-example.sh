#!/usr/bin/env bash
# 用 example 中的 vusers（及 description）覆盖本地 qpsProfiles 同名档，便于 4C/8G VU 收紧后同步到 tmp/perf/qs-perf.config.json。
set -euo pipefail

LOCAL="${1:-tmp/perf/qs-perf.config.json}"
EXAMPLE="${2:-scripts/perf/qs-perf.config.example.json}"

if [[ ! -f "$LOCAL" ]]; then
  echo "config not found: $LOCAL (run: make perf-init)" >&2
  exit 1
fi
if [[ ! -f "$EXAMPLE" ]]; then
  echo "example not found: $EXAMPLE" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

next="$(jq -c --slurpfile ex "$EXAMPLE" '
  .vuserSizing = ($ex[0].vuserSizing // .vuserSizing)
  | reduce ($ex[0].qpsProfiles | keys[]) as $k (.;
      if (.qpsProfiles[$k] // null) and ($ex[0].qpsProfiles[$k].vusers // null) then
        .qpsProfiles[$k].vusers = $ex[0].qpsProfiles[$k].vusers
        | if ($ex[0].qpsProfiles[$k].description // null) then
            .qpsProfiles[$k].description = $ex[0].qpsProfiles[$k].description
          else . end
      else . end
    )
' "$LOCAL")"

jq . <<<"$next" > "${LOCAL}.tmp"
mv "${LOCAL}.tmp" "$LOCAL"
echo "overlaid vusers from example -> $LOCAL"
