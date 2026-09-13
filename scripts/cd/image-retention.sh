#!/usr/bin/env bash
# Shared host lock: acquire BEFORE loading/pulling or replacing any image/container.
# SUDO is the existing deploy wrapper (sudo or sudo_pw).
acquire_image_deploy_lock() {
  command -v python3 >/dev/null
  command -v flock >/dev/null
  $SUDO mkdir -p /var/lib/fangcun-image-retention
  $SUDO chmod 0755 /var/lib/fangcun-image-retention
  # Do not replace/unlink the inode: all deployment users lock the same file.
  $SUDO touch /var/lib/fangcun-image-retention/deploy.lock
  $SUDO chmod 0666 /var/lib/fangcun-image-retention/deploy.lock
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
