#!/usr/bin/env bash
set -Eeuo pipefail

: "${HTTP_PROXY:?HTTP_PROXY is required}"
: "${HTTPS_PROXY:?HTTPS_PROXY is required}"

export HTTP_PROXY HTTPS_PROXY
export ALL_PROXY="${ALL_PROXY:-}"
export NO_PROXY="${NO_PROXY:-127.0.0.1,localhost}"

proxy_host="${RUNNER_SSH_PROXY_HOST:-127.0.0.1}"
proxy_port="${RUNNER_SSH_PROXY_PORT:-7890}"
if [ -n "${RUNNER_SSH_PROXY:-}" ]; then
  case "$RUNNER_SSH_PROXY" in
    http://*:*)
      proxy_host="${RUNNER_SSH_PROXY#http://}"
      proxy_host="${proxy_host%%:*}"
      proxy_port="${RUNNER_SSH_PROXY##*:}"
      ;;
    http://*)
      proxy_host="${RUNNER_SSH_PROXY#http://}"
      ;;
  esac
elif [ -n "${HTTP_PROXY:-}" ]; then
  case "$HTTP_PROXY" in
    http://*:*)
      proxy_host="${HTTP_PROXY#http://}"
      proxy_host="${proxy_host%%:*}"
      proxy_port="${HTTP_PROXY##*:}"
      ;;
  esac
fi

if ! command -v nc >/dev/null 2>&1; then
  echo "nc is required for GitHub SSH over HTTP CONNECT proxy" >&2
  exit 1
fi

mkdir -p ~/.ssh
chmod 700 ~/.ssh
CONFIG="${HOME}/.ssh/config"
touch "$CONFIG"
chmod 600 "$CONFIG"

if ! grep -q '^Host github.com$' "$CONFIG" 2>/dev/null; then
  cat >>"$CONFIG" <<EOF

Host github.com
  HostName ssh.github.com
  Port 443
  User git
  ProxyCommand nc -X connect -x ${proxy_host}:${proxy_port} %h %p
  StrictHostKeyChecking accept-new
EOF
fi

if [ -n "${GITHUB_ENV:-}" ]; then
  {
    echo "HTTP_PROXY=${HTTP_PROXY}"
    echo "HTTPS_PROXY=${HTTPS_PROXY}"
    echo "ALL_PROXY=${ALL_PROXY}"
    echo "NO_PROXY=${NO_PROXY}"
  } >>"$GITHUB_ENV"
fi

echo "Runner network ready:"
echo "  HTTP_PROXY=${HTTP_PROXY}"
echo "  GitHub SSH: git@github.com -> ssh.github.com:443 via ${proxy_host}:${proxy_port}"

if [ "${RUNNER_NETWORK_TEST_GITHUB_SSH:-1}" = "1" ]; then
  echo "Testing GitHub SSH..."
  ssh -T git@github.com 2>&1 | head -5 || true
fi
