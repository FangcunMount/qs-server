#!/usr/bin/env bash

# Shared helpers for deciding whether the deploy target is the current host.

is_local_deploy_target() {
  local target="${1:-}"
  local ip

  if [ -z "$target" ]; then
    return 1
  fi

  case "$target" in
    127.0.0.1 | localhost | ::1)
      return 0
      ;;
  esac

  if command -v tailscale >/dev/null 2>&1; then
    while IFS= read -r ip; do
      [ -n "$ip" ] && [ "$target" = "$ip" ] && return 0
    done < <(tailscale ip -4 2>/dev/null || true)
  fi

  while IFS= read -r ip; do
    [ -n "$ip" ] && [ "$target" = "$ip" ] && return 0
  done < <(hostname -I 2>/dev/null || true)

  return 1
}

resolve_ssh_hostname() {
  local alias="${1:-}"
  if [ -z "$alias" ] || ! command -v ssh >/dev/null 2>&1; then
    return 1
  fi
  ssh -G "$alias" 2>/dev/null | awk '/^hostname / { print $2; exit }'
}
