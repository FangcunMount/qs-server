#!/usr/bin/env bash

# Select one sudo mode for the entire deployment. A successful `sudo -n true`
# can be backed by a cached credential or a command-specific sudoers rule; it
# does not prove a later retention command can run without a password.
select_deploy_sudo() {
  if [ -n "${SUDO_PASSWORD:-}" ]; then
    sudo_pw() { sudo -S "$@" <<<"$SUDO_PASSWORD"; }
    export -f sudo_pw
    SUDO="sudo_pw"
    $SUDO -v || return 1
    echo "Using sudo with password."
  elif sudo -n true 2>/dev/null; then
    SUDO="sudo"
    echo "Using passwordless sudo."
  else
    echo "sudo needs password. Provide SUDO_PASSWORD or configure NOPASSWD." >&2
    return 1
  fi
}
