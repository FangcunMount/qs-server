#!/usr/bin/env bash
# Shared host lock: acquire BEFORE loading/pulling or replacing any image/container.
# SUDO is the existing deploy wrapper (sudo or sudo_pw).
# Publish a prepared inode with existing sudo permissions. The package directory
# and retention directory must share a filesystem; failed initialization stops deploy.
# A subshell keeps cleanup separate from the caller's deployment EXIT trap.
initialize_image_deploy_lock() (
  local directory="$1" lock temporary
  lock="$directory/deploy.lock"
  $SUDO mkdir -p "$directory" || exit 1
  $SUDO chmod 0755 "$directory" || exit 1
  if [ -e "$lock" ] || [ -L "$lock" ]; then
    [ -f "$lock" ] && [ ! -L "$lock" ]
    exit $?
  fi
  temporary="$(mktemp -d "$SCRIPT_DIR/.retention-lock.XXXXXX")" || exit 1
  trap 'rm -rf -- "$temporary"' EXIT
  : > "$temporary/lock" || exit 1
  $SUDO chown root:root "$temporary/lock" || exit 1
  $SUDO chmod 0666 "$temporary/lock" || exit 1
  # No force: never replace an inode another process may already have locked.
  if ! $SUDO ln -- "$temporary/lock" "$lock"; then
    [ -f "$lock" ] && [ ! -L "$lock" ] || exit 1
  fi
)

acquire_image_deploy_lock() {
  command -v python3 >/dev/null
  command -v flock >/dev/null
  initialize_image_deploy_lock /var/lib/fangcun-image-retention || return 1
  exec 9<>/var/lib/fangcun-image-retention/deploy.lock
  flock -w 1800 9
  RETENTION_PREVIOUS_IDS=()
  local container_id image_id container_ids
  container_ids="$($SUDO docker ps -aq)"
  while IFS= read -r container_id; do
    [ -z "$container_id" ] && continue
    image_id="$($SUDO docker inspect --format '{{.Image}}' "$container_id")"
    RETENTION_PREVIOUS_IDS+=(--protect-image-id "$image_id")
  done <<< "$container_ids"
}

retain_successful_image() {
  local image_ref="$1"
  # The deploy package is writable by deploy. Never execute its Python file as
  # root; the host administrator installs an identical, root-owned copy.
  local trusted_helper="/usr/local/libexec/fangcun/image-retention.py"
  if [ -L "$trusted_helper" ] || [ ! -f "$trusted_helper" ] || \
      [ "$(stat -c '%u:%a' "$trusted_helper" 2>/dev/null)" != "0:755" ] || \
      [ "$(stat -c '%u:%a' "$(dirname "$trusted_helper")" 2>/dev/null)" != "0:755" ] || \
      ! cmp -s "$SCRIPT_DIR/image-retention.py" "$trusted_helper"; then
    echo "::warning::Trusted image retention helper missing, untrusted or differs; skip cleanup." >&2
    return 0
  fi
  local ancestor
  ancestor=$(dirname "$trusted_helper")
  while :; do
    if [ -L "$ancestor" ] || [ "$(stat -c '%u' "$ancestor")" != 0 ] || \
        [ -n "$(find "$ancestor" -maxdepth 0 -perm /022 -print)" ]; then
      echo '::warning::Untrusted image retention parent; skip cleanup.' >&2
      return 0
    fi
    [ "$ancestor" = / ] && break
    ancestor=$(dirname "$ancestor")
  done
  if ! $SUDO /usr/bin/python3 -I "$trusted_helper" \
      --service "${IMAGE_NAME##*/}" --image-ref "$image_ref" --apply --deployment-locked "${RETENTION_PREVIOUS_IDS[@]}"; then
    echo "::warning::Deployment succeeded, but image retention failed; inspect the server retention audit." >&2
  fi
}
