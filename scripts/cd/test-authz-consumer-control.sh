#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

FAKE_BIN="$TEST_ROOT/bin"
RUNNING_FILE="$TEST_ROOT/running"
CALL_LOG="$TEST_ROOT/calls"
STATE_ROOT="$TEST_ROOT/state"
mkdir -p "$FAKE_BIN"
printf '%s\n' app-1 collection-1 >"$RUNNING_FILE"

cat >"$FAKE_BIN/sudo" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
if [ "${1:-}" = -n ] && [ "${2:-}" = true ]; then
  exit 0
fi
exec "$@"
EOF

cat >"$FAKE_BIN/docker" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf '%s\n' "$*" >>"$FAKE_CALL_LOG"
case "${1:-}" in
  ps)
    cat "$FAKE_RUNNING_FILE"
    ;;
  stop)
    id="$2"
    awk -v id="$id" '$0 != id' "$FAKE_RUNNING_FILE" >"$FAKE_RUNNING_FILE.next"
    mv "$FAKE_RUNNING_FILE.next" "$FAKE_RUNNING_FILE"
    ;;
  start)
    printf '%s\n' "$2" >>"$FAKE_RUNNING_FILE"
    ;;
  *) exit 64 ;;
esac
EOF
chmod 0755 "$FAKE_BIN/sudo" "$FAKE_BIN/docker"

run_control() {
  env \
    PATH="$FAKE_BIN:$PATH" \
    FAKE_CALL_LOG="$CALL_LOG" \
    FAKE_RUNNING_FILE="$RUNNING_FILE" \
    QS_AUTHZ_CONSUMER_OPERATION="$1" \
    QS_AUTHZ_CONSUMER_HOST_ROLE=app \
    QS_AUTHZ_RELEASE_SHA=0123456789abcdef0123456789abcdef01234567 \
    QS_AUTHZ_CONSUMER_STATE_ROOT="$STATE_ROOT" \
    "$SCRIPT_DIR/authz-consumer-control.sh"
}

run_control stop >/dev/null
if [ -s "$RUNNING_FILE" ]; then
  echo 'stop left authorization consumers running' >&2
  exit 1
fi
state_file="$STATE_ROOT/0123456789abcdef0123456789abcdef01234567.app-containers"
grep -Fxq app-1 "$state_file"
grep -Fxq collection-1 "$state_file"

run_control start >/dev/null
grep -Fxq app-1 "$RUNNING_FILE"
grep -Fxq collection-1 "$RUNNING_FILE"

if env \
  PATH="$FAKE_BIN:$PATH" \
  QS_AUTHZ_CONSUMER_OPERATION=stop \
  QS_AUTHZ_CONSUMER_HOST_ROLE=app \
  QS_AUTHZ_RELEASE_SHA=main \
  QS_AUTHZ_CONSUMER_STATE_ROOT="$STATE_ROOT" \
  "$SCRIPT_DIR/authz-consumer-control.sh" >/dev/null 2>&1; then
  echo 'invalid release SHA was accepted' >&2
  exit 1
fi

cd_workflow="$SCRIPT_DIR/../../.github/workflows/cd.yml"
if [ "$(grep -Fc "vars.AUTHZ_CUTOVER_AUTO_DEPLOY_PAUSED != 'true'" "$cd_workflow")" -ne 2 ]; then
  echo 'automatic production deploy pause guards are incomplete' >&2
  exit 1
fi

control_workflow="$SCRIPT_DIR/../../.github/workflows/authz-consumer-control.yml"
for token in \
  'name: AuthZ Production Consumer Control' \
  'group: qs-production-authz-consumer-control' \
  'STOP_AUTHZ_CONSUMERS' \
  'Control serverA consumers' \
  'Control serverD consumers'; do
  grep -Fq "$token" "$control_workflow"
done

echo '[OK] AuthZ maintenance stop is exact, reversible, and manually gated'
