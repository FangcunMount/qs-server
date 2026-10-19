#!/usr/bin/env bash
# 加载自托管 runner 的 .env（代理、可选 OSS_SESSION_TOKEN 等）。
set -euo pipefail

_mysql_saved_names=()
_mysql_saved_values=()
while IFS= read -r _mysql_name; do
  _mysql_saved_names+=("$_mysql_name")
  _mysql_saved_values+=("${!_mysql_name}")
done < <(compgen -A variable MYSQL_ || true)

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

while IFS= read -r _mysql_name; do unset "$_mysql_name"; done < <(compgen -A variable MYSQL_ || true)
for ((_mysql_index=0; _mysql_index<${#_mysql_saved_names[@]}; _mysql_index++)); do
  printf -v "${_mysql_saved_names[$_mysql_index]}" '%s' "${_mysql_saved_values[$_mysql_index]}"
  export "${_mysql_saved_names[$_mysql_index]}"
done
unset _mysql_saved_names _mysql_saved_values _mysql_name _mysql_index
