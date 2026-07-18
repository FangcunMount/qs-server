#!/usr/bin/env bash
# 将 qs-perf.config.example.json 中新增的 qpsProfiles / paths 合并进本地配置（本地已有键优先，不覆盖 token/URL）。
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

migrate_runtime_paths() {
  jq --slurpfile ex "$EXAMPLE" '
    def migrate_path:
      gsub("/api/v1/personality-models"; "/api/v1/typology-models")
      | gsub("/api/v1/personality-assessment-sessions"; "/api/v1/typology-assessment-sessions")
      | gsub("/api/v1/personality-assessments"; "/api/v1/typology-assessments");
    def retired_statistics_path:
      startswith("/api/v1/statistics/system")
      or startswith("/api/v1/statistics/questionnaires/");
    def current_statistics_paths:
      map(select(retired_statistics_path | not))
      | if length > 0 then . else ($ex[0].paths.statistics // []) end;
    walk(if type == "string" then migrate_path else . end)
    | .paths.statistics = ((.paths.statistics // []) | current_statistics_paths)
    | .qpsProfiles |= with_entries(
        if (.value.paths.statistics? | type) == "array"
        then .value.paths.statistics |= current_statistics_paths
        else .
        end
      )
  '
}

before="$(jq -c . "$LOCAL")"
before_profile_keys="$(jq -c '.qpsProfiles // {} | keys' "$LOCAL")"
before_path_keys="$(jq -c '.paths // {} | keys' "$LOCAL")"
next="$(jq -c --slurpfile ex "$EXAMPLE" '
  .qpsProfiles = (($ex[0].qpsProfiles // {}) + (.qpsProfiles // {}))
  | .paths = (($ex[0].paths // {}) + (.paths // {}))
' "$LOCAL" | migrate_runtime_paths)"

if [[ "$next" == "$before" ]]; then
  echo "qs-perf.config.json already up to date: $LOCAL"
  exit 0
fi

migrated_paths="$(jq -n --argjson before "$before" --argjson after "$next" '
  ([ $before, $after ]
    | map([.. | strings | select(test("/api/v1/personality-"))] | unique | sort)
    | if (.[0] | length) > 0 and (.[0] != .[1]) then ["typology path migration applied"] else [] end)
  + ([ $before, $after ]
    | map([.. | strings | select(test("/api/v1/statistics/(system|questionnaires/)"))] | unique | sort)
    | if (.[0] | length) > 0 and (.[0] != .[1]) then ["retired statistics paths removed"] else [] end)
  | join(", ")
')"

added_profiles="$(jq -n --argjson before "$before_profile_keys" --argjson after "$(jq -c '.qpsProfiles // {} | keys' <<<"$next")" '
  [$after[] | select(. as $k | ($before | index($k) | not))]
  | if length > 0 then join(", ") else empty end
')"
added_paths="$(jq -n --argjson before "$before_path_keys" --argjson after "$(jq -c '.paths // {} | keys' <<<"$next")" '
  [$after[] | select(. as $k | ($before | index($k) | not))]
  | if length > 0 then join(", ") else empty end
')"

jq . <<<"$next" > "${LOCAL}.tmp"
mv "${LOCAL}.tmp" "$LOCAL"

echo "merged qpsProfiles/paths from example -> $LOCAL"
if [[ -n "$migrated_paths" ]]; then
  echo "  $migrated_paths"
fi
if [[ -n "$added_profiles" ]]; then
  echo "  new profiles: $added_profiles"
fi
if [[ -n "$added_paths" ]]; then
  echo "  new paths: $added_paths"
fi
if [[ -z "$added_profiles" && -z "$added_paths" ]]; then
  echo "  (no new keys; normalized JSON / paths fill-in only)"
fi
