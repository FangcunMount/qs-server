#!/usr/bin/env bash
set -Eeuo pipefail

: "${RUNNER_SSH_KEY:?RUNNER_SSH_KEY is required}"
: "${RUNNER_SSH_HOST:?RUNNER_SSH_HOST is required}"
: "${RUNNER_SSH_USER:?RUNNER_SSH_USER is required}"

RUNNER_SSH_PORT="${RUNNER_SSH_PORT:-22}"
RUNNER_SSH_ALIAS="${RUNNER_SSH_ALIAS:-deploy-target}"
KEY_FILE="${RUNNER_SSH_KEY_FILE:-$HOME/.ssh/runner_${RUNNER_SSH_ALIAS}_key}"
CONFIG="${RUNNER_SSH_CONFIG:-$HOME/.ssh/config}"

mkdir -p "$(dirname "$KEY_FILE")"
chmod 700 "$(dirname "$KEY_FILE")"
printf '%s\n' "$RUNNER_SSH_KEY" >"$KEY_FILE"
chmod 600 "$KEY_FILE"

touch "$CONFIG"
chmod 600 "$CONFIG"
if ! grep -q "^Host ${RUNNER_SSH_ALIAS}$" "$CONFIG" 2>/dev/null; then
  cat >>"$CONFIG" <<EOF

Host ${RUNNER_SSH_ALIAS}
  HostName ${RUNNER_SSH_HOST}
  User ${RUNNER_SSH_USER}
  Port ${RUNNER_SSH_PORT}
  IdentityFile ${KEY_FILE}
  StrictHostKeyChecking accept-new
EOF
fi

echo "SSH config ready for ${RUNNER_SSH_ALIAS} (${RUNNER_SSH_USER}@${RUNNER_SSH_HOST}:${RUNNER_SSH_PORT})"
