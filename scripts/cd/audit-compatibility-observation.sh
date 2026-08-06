#!/usr/bin/env bash
set -Eeuo pipefail

WINDOW_DAYS="${WINDOW_DAYS:-30}"
APISERVER_CONTAINER="${APISERVER_CONTAINER:-qs-apiserver}"
PROMETHEUS_BASE_URL="${PROMETHEUS_BASE_URL:-http://prometheus:9090}"
QUERY_TIMEOUT_SECONDS="${QUERY_TIMEOUT_SECONDS:-10}"
PRIVILEGE_RUNNER="${PRIVILEGE_RUNNER-sudo}"
DOCKER_BIN="${DOCKER_BIN:-docker}"
PYTHON_BIN="${PYTHON_BIN:-python3}"

require_positive_integer() {
  local name="$1"
  local value="$2"
  if ! [[ "$value" =~ ^[0-9]+$ ]] || [ "$value" -lt 1 ]; then
    echo "$name must be a positive integer, got: $value" >&2
    return 1
  fi
}

run_privileged() {
  if [ -n "$PRIVILEGE_RUNNER" ]; then
    "$PRIVILEGE_RUNNER" "$@"
  else
    "$@"
  fi
}

parse_single_vector_value() {
  "$PYTHON_BIN" -c '
import json
import sys
from decimal import Decimal, InvalidOperation

payload = json.load(sys.stdin)
if payload.get("status") != "success":
    raise SystemExit("Prometheus query did not succeed")
result = payload.get("data", {}).get("result", [])
if not result:
    print("absent")
    raise SystemExit(0)
if len(result) != 1:
    raise SystemExit(f"Prometheus query returned {len(result)} series, want exactly one")
try:
    value = Decimal(result[0]["value"][1])
except (InvalidOperation, KeyError, IndexError, TypeError) as exc:
    raise SystemExit(f"Prometheus query returned an invalid value: {exc}")
print(format(value, "f"))
'
}

prometheus_query() {
  local query="$1"
  local response
  response="$(run_privileged "$DOCKER_BIN" exec "$APISERVER_CONTAINER" \
    wget -qO- -T "$QUERY_TIMEOUT_SECONDS" \
    --post-data="query=$query" \
    "${PROMETHEUS_BASE_URL%/}/api/v1/query")"
  printf '%s' "$response" | parse_single_vector_value
}

is_zero() {
  "$PYTHON_BIN" -c '
import sys
from decimal import Decimal
raise SystemExit(0 if Decimal(sys.argv[1]) == 0 else 1)
' "$1"
}

require_positive_integer WINDOW_DAYS "$WINDOW_DAYS"
require_positive_integer QUERY_TIMEOUT_SECONDS "$QUERY_TIMEOUT_SECONDS"
if [ "$WINDOW_DAYS" -gt 90 ]; then
  echo "WINDOW_DAYS must not exceed 90, got: $WINDOW_DAYS" >&2
  exit 1
fi
if [ -n "$PRIVILEGE_RUNNER" ] && ! command -v "$PRIVILEGE_RUNNER" >/dev/null 2>&1; then
  echo "Privilege runner is unavailable: $PRIVILEGE_RUNNER" >&2
  exit 1
fi
if ! command -v "$DOCKER_BIN" >/dev/null 2>&1; then
  echo "Docker command is unavailable: $DOCKER_BIN" >&2
  exit 1
fi
if ! command -v "$PYTHON_BIN" >/dev/null 2>&1; then
  echo "Python command is unavailable: $PYTHON_BIN" >&2
  exit 1
fi
if [ "$(run_privileged "$DOCKER_BIN" inspect "$APISERVER_CONTAINER" --format '{{.State.Running}}' 2>/dev/null || true)" != "true" ]; then
  echo "Apiserver container is not running: $APISERVER_CONTAINER" >&2
  exit 1
fi

metrics=(
  "actor_practitioners|qs_actor_deprecated_practitioner_route_total"
  "statistics_validate_only|qs_statistics_deprecated_validate_only_total"
  "interpretation_generate_from_assessment|qs_interpretation_deprecated_generate_report_from_assessment_total"
)

echo "Public compatibility observation (Prometheus read-only):"
printf 'window_days\tmetric\tcurrent_series\tanchor_series\twindow_increase\tclassification\n'
for item in "${metrics[@]}"; do
  IFS='|' read -r name metric <<<"$item"
  current_series="$(prometheus_query "count(${metric})")"
  if [ "$current_series" = "absent" ] || is_zero "$current_series"; then
    echo "Compatibility metric is absent from the current Prometheus snapshot: $metric" >&2
    exit 1
  fi

  anchor_series="$(prometheus_query "count(${metric} offset ${WINDOW_DAYS}d)")"
  increase="$(prometheus_query "sum(increase(${metric}[${WINDOW_DAYS}d]))")"
  if [ "$increase" = "absent" ]; then
    echo "Compatibility metric has no samples in the requested window: $metric" >&2
    exit 1
  fi

  classification="active_compatibility"
  if [ "$anchor_series" = "absent" ] || is_zero "$anchor_series"; then
    classification="observation_window_incomplete"
  elif is_zero "$increase"; then
    classification="zero_window_candidate"
  fi
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$WINDOW_DAYS" "$name" "$current_series" "$anchor_series" "$increase" "$classification"
done

echo "Compatibility observation completed; zero_window_candidate still requires caller confirmation before removal."
