#!/usr/bin/env bash
# 从 GitHub 仓库/组织 Variables 解析 CD 部署配置，写入 GITHUB_OUTPUT。
# 未配置时使用与 scripts/cd/runner-dotenv.example 一致的默认值。
set -euo pipefail

: "${GITHUB_OUTPUT:?GITHUB_OUTPUT is required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"

org="${GITHUB_REPOSITORY%%/*}"

get_var() {
  local name="$1"
  gh variable get "$name" --repo "$GITHUB_REPOSITORY" 2>/dev/null \
    || gh variable get "$name" --org "$org" 2>/dev/null \
    || true
}

export_registry="$(get_var QS_DEPLOY_EXPORT_REGISTRY)"
http_proxy="$(get_var QS_DEPLOY_HTTP_PROXY)"
https_proxy="$(get_var QS_DEPLOY_HTTPS_PROXY)"
all_proxy="$(get_var QS_DEPLOY_ALL_PROXY)"
no_proxy="$(get_var QS_DEPLOY_NO_PROXY)"

{
  echo "deploy_export_registry=${export_registry:-acr}"
  echo "self_hosted_http_proxy=${http_proxy:-http://127.0.0.1:7890}"
  if [ -n "${https_proxy}" ]; then
    echo "self_hosted_https_proxy=${https_proxy}"
  else
    echo "self_hosted_https_proxy=${http_proxy:-http://127.0.0.1:7890}"
  fi
  echo "self_hosted_all_proxy=${all_proxy:-socks5://127.0.0.1:7891}"
  echo "self_hosted_no_proxy=${no_proxy:-127.0.0.1,localhost,100.64.0.0/10,.aliyuncs.com,.personal.cr.aliyuncs.com}"
} >>"$GITHUB_OUTPUT"
