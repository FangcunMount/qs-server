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

# Always refresh Host block so stale HostName on the runner cannot pin the wrong server.
if [ -f "$CONFIG" ]; then
  awk -v host="$RUNNER_SSH_ALIAS" '
    $0 ~ "^Host " host "$" { skip=1; next }
    skip && /^Host / { skip=0 }
    !skip { print }
  ' "$CONFIG" >"${CONFIG}.tmp"
  mv "${CONFIG}.tmp" "$CONFIG"
fi

cat >>"$CONFIG" <<EOF

Host ${RUNNER_SSH_ALIAS}
  HostName ${RUNNER_SSH_HOST}
  User ${RUNNER_SSH_USER}
  Port ${RUNNER_SSH_PORT}
  IdentityFile ${KEY_FILE}
  StrictHostKeyChecking accept-new
EOF

runner_ip="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
echo "=========================================="
echo "CD SSH deploy connectivity"
echo "Deploy runner local: hostname=$(hostname) primary_ip=${runner_ip:-unknown}"
echo "Deploy target configured: ${RUNNER_SSH_USER}@${RUNNER_SSH_HOST}:${RUNNER_SSH_PORT} (alias=${RUNNER_SSH_ALIAS})"
if command -v ssh >/dev/null 2>&1; then
  ssh -G "${RUNNER_SSH_ALIAS}" 2>/dev/null | awk '/^(hostname|user|port) /{print "ssh -G resolved: "$0}' || true
fi
echo "Probing deploy target over SSH..."
ssh "${RUNNER_SSH_ALIAS}" 'echo "Deploy target remote: hostname=$(hostname) primary_ip=$(hostname -I 2>/dev/null | awk "{print \$1}") user=$(whoami) pwd=$(pwd)"'
echo "=========================================="
