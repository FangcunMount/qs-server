#!/usr/bin/env bash
set -Eeuo pipefail
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
: "${RUNNER_SSH_CONFIG:?RUNNER_SSH_CONFIG is required}"
: "${RUNNER_SSH_ALIAS:?RUNNER_SSH_ALIAS is required}"
[[ "${MAINTENANCE_IMAGE:-}" =~ ^sha256:[0-9a-f]{64}$ ]]
[[ "${GITHUB_RUN_ID:-}" =~ ^[0-9]{1,20}$ ]]
[[ "${GITHUB_RUN_ATTEMPT:-}" =~ ^[0-9]{1,4}$ ]]
case "${MAINTENANCE_MODE:-}" in preflight|stage) ;; *) exit 1 ;; esac
umask 077
bootstrap=$(mktemp "${RUNNER_TEMP:-/tmp}/qs-m5-transport.XXXXXX")
trap 'rm -f "$bootstrap"' EXIT
{
  echo 'set -Eeuo pipefail'
  printf 'export SUDO_PASSWORD=%q\n' "${SUDO_PASSWORD:-}"
  printf 'exec python3 - --expected-image-id %q --operation-id %q' "$MAINTENANCE_IMAGE" "${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}"
  if [ "$MAINTENANCE_MODE" = stage ]; then printf ' --stage'; fi
  printf " <<'PY_M5_MAINTENANCE'\n"
  cat "$SCRIPT_DIR/m5_maintenance_container.py"
  echo 'PY_M5_MAINTENANCE'
} > "$bootstrap"
# Only fixed Docker commands use sudo inside Python. Python itself remains the
# SSH account, and credentials travel through private stdin, never SSH argv.
ssh -F "$RUNNER_SSH_CONFIG" -o ConnectTimeout=20 "$RUNNER_SSH_ALIAS" 'bash -s' < "$bootstrap"
