#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

FAKE_DOCKER="$TEST_ROOT/docker"
cat >"$FAKE_DOCKER" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

case "${1:-}" in
  inspect)
    printf 'true\n'
    ;;
  exec)
    query=""
    for argument in "$@"; do
      case "$argument" in
        --post-data=query=*) query="${argument#--post-data=query=}" ;;
      esac
    done
    if [ -z "$query" ]; then
      echo "fake docker received no Prometheus query" >&2
      exit 2
    fi
    value=1
    case "$query" in
      count*offset*) value="${FAKE_ANCHOR_SERIES:-1}" ;;
      count*) value="${FAKE_CURRENT_SERIES:-1}" ;;
      *increase*)
        value=0
        if [ "${FAKE_ACTIVE:-0}" = 1 ] && [[ "$query" == *qs_actor_deprecated_practitioner_route_total* ]]; then
          value=2
        fi
        ;;
      *)
        echo "unexpected Prometheus query: $query" >&2
        exit 2
        ;;
    esac
    printf '{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1,"%s"]}]}}\n' "$value"
    ;;
  *)
    echo "unexpected fake docker command: $*" >&2
    exit 2
    ;;
esac
EOF
chmod +x "$FAKE_DOCKER"

COMMON_ENV=(
  WINDOW_DAYS=30
  PRIVILEGE_RUNNER=
  DOCKER_BIN="$FAKE_DOCKER"
)

zero_output="$(env "${COMMON_ENV[@]}" "$SCRIPT_DIR/audit-compatibility-observation.sh")"
if [ "$(printf '%s\n' "$zero_output" | awk -F '\t' '$6 == "zero_window_candidate" {count++} END {print count + 0}')" -ne 3 ]; then
  echo "zero-window observation did not classify all public compatibility metrics" >&2
  exit 1
fi

active_output="$(env "${COMMON_ENV[@]}" FAKE_ACTIVE=1 "$SCRIPT_DIR/audit-compatibility-observation.sh")"
if ! printf '%s\n' "$active_output" | grep -Fq $'actor_practitioners\t1\t1\t2\tactive_compatibility'; then
  echo "active compatibility hit was not preserved" >&2
  exit 1
fi

incomplete_output="$(env "${COMMON_ENV[@]}" FAKE_ANCHOR_SERIES=0 "$SCRIPT_DIR/audit-compatibility-observation.sh")"
if [ "$(printf '%s\n' "$incomplete_output" | awk -F '\t' '$6 == "observation_window_incomplete" {count++} END {print count + 0}')" -ne 3 ]; then
  echo "incomplete history was not distinguished from a zero window" >&2
  exit 1
fi

if env "${COMMON_ENV[@]}" FAKE_CURRENT_SERIES=0 \
  "$SCRIPT_DIR/audit-compatibility-observation.sh" >/dev/null 2>&1; then
  echo "observation accepted a missing current metric" >&2
  exit 1
fi

if env "${COMMON_ENV[@]}" WINDOW_DAYS=0 \
  "$SCRIPT_DIR/audit-compatibility-observation.sh" >/dev/null 2>&1; then
  echo "observation accepted an invalid window" >&2
  exit 1
fi

echo "compatibility observation contract passed"
