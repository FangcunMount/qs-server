#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

FAKE_BIN="$TEST_ROOT/bin"
TEST_HOME="$TEST_ROOT/home"
SSH_ARGS_FILE="$TEST_ROOT/ssh-args"
GITHUB_ENV_FILE="$TEST_ROOT/github-env"
mkdir -p "$FAKE_BIN" "$TEST_HOME"

printf '%s\n' \
  '#!/usr/bin/env bash' \
  'exit 0' >"$FAKE_BIN/nc"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'printf "%s\n" "$*" >"$SSH_ARGS_FILE"' \
  'printf "%s\n" "GitHub SSH test stub"' >"$FAKE_BIN/ssh"
chmod +x "$FAKE_BIN/nc" "$FAKE_BIN/ssh"

export HOME="$TEST_HOME"
export PATH="$FAKE_BIN:$PATH"
export SSH_ARGS_FILE
export GITHUB_ENV="$GITHUB_ENV_FILE"
export HTTP_PROXY=http://127.0.0.1:7890
export HTTPS_PROXY="$HTTP_PROXY"
export ALL_PROXY=socks5://127.0.0.1:7891
export NO_PROXY=127.0.0.1,localhost
export RUNNER_SSH_PROXY_HOST=127.0.0.1
export RUNNER_SSH_PROXY_PORT=7890
export RUNNER_NETWORK_SSH_CONNECT_TIMEOUT_SECONDS=7

"$SCRIPT_DIR/setup-runner-network.sh" >/dev/null
"$SCRIPT_DIR/setup-runner-network.sh" >/dev/null

test "$(grep -c '^Host github.com$' "$HOME/.ssh/config")" -eq 1
grep -Fq 'ProxyCommand nc -X connect -x 127.0.0.1:7890 %h %p' "$HOME/.ssh/config"
grep -Fq -- '-o BatchMode=yes' "$SSH_ARGS_FILE"
grep -Fq -- '-o ConnectTimeout=7' "$SSH_ARGS_FILE"
grep -Fq -- '-o ConnectionAttempts=1' "$SSH_ARGS_FILE"
grep -Fq -- '-T git@github.com' "$SSH_ARGS_FILE"
grep -Fxq 'HTTP_PROXY=http://127.0.0.1:7890' "$GITHUB_ENV_FILE"

if RUNNER_NETWORK_SSH_CONNECT_TIMEOUT_SECONDS=invalid \
  "$SCRIPT_DIR/setup-runner-network.sh" >/dev/null 2>&1; then
  echo "invalid SSH connect timeout unexpectedly succeeded" >&2
  exit 1
fi
