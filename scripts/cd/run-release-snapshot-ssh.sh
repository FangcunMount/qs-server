#!/usr/bin/env bash
set -Eeuo pipefail
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
: "${RUNNER_SSH_CONFIG:?RUNNER_SSH_CONFIG is required}"
: "${RUNNER_SSH_ALIAS:?RUNNER_SSH_ALIAS is required}"
umask 077
bootstrap=$(mktemp "${RUNNER_TEMP:-/tmp}/release-snapshot.XXXXXX")
trap 'rm -f "$bootstrap"' EXIT
{
  echo 'set -Eeuo pipefail'
  printf 'export SUDO_PASSWORD=%q\n' "${SUDO_PASSWORD:-}"
  echo "exec python3 - --sudo-docker <<'PY_RELEASE_SNAPSHOT'"
  cat "$SCRIPT_DIR/release-snapshot.py"
  echo 'PY_RELEASE_SNAPSHOT'
} > "$bootstrap"
ssh -F "$RUNNER_SSH_CONFIG" -o ConnectTimeout=20 "$RUNNER_SSH_ALIAS" 'bash -s' < "$bootstrap"
