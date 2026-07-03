#!/usr/bin/env bash
# 加载自托管 runner 的 .env（代理、可选 OSS_SESSION_TOKEN 等）。
set -euo pipefail

for env_file in \
  "${HOME}/.env" \
  "/opt/actions-runner/runner1/.env" \
  "/opt/actions-runner/runner2/.env" \
  "/opt/actions-runner/runner3/.env"; do
  if [ -f "$env_file" ]; then
    set -a
    # shellcheck source=/dev/null
    . "$env_file"
    set +a
    break
  fi
done
