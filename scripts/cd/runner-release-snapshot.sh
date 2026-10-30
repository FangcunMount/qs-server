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
  cat "$SCRIPT_DIR/sudo-selection.sh"
  cat <<'BOOT'
select_deploy_sudo >&2
if [ "$SUDO" = sudo_pw ]; then
  SUDO_ASKPASS=$(mktemp)
  export SUDO_ASKPASS
  trap 'rm -f "$SUDO_ASKPASS"' EXIT
  printf '%s\n' '#!/bin/sh' 'printf '\''%s\n'\'' "$SUDO_PASSWORD"' > "$SUDO_ASKPASS"
  chmod 700 "$SUDO_ASKPASS"
  snapshot_sudo=(sudo -A)
else
  snapshot_sudo=(sudo -n)
fi
"${snapshot_sudo[@]}" python3 - <<'PY_RELEASE_SNAPSHOT'
BOOT
  cat "$SCRIPT_DIR/release-snapshot.py"
  echo 'PY_RELEASE_SNAPSHOT'
} > "$bootstrap"
ssh -F "$RUNNER_SSH_CONFIG" -o ConnectTimeout=20 "$RUNNER_SSH_ALIAS" \
  'bash -s' < "$bootstrap"
