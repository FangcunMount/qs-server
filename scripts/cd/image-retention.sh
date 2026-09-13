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
  if ! $SUDO python3 "$SCRIPT_DIR/image-retention.py" \
      --service "${IMAGE_NAME##*/}" --image-ref "$image_ref" --apply --deployment-locked "${RETENTION_PREVIOUS_IDS[@]}"; then
    echo "::warning::Deployment succeeded, but image retention failed; inspect the server retention audit." >&2
  fi
}
