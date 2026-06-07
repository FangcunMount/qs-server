#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/image-metadata.sh"

: "${RUNNER_SSH_ALIAS:?RUNNER_SSH_ALIAS is required}"
: "${SERVICE:?SERVICE is required}"
: "${IMAGE_TAG:?IMAGE_TAG is required}"
: "${DOCKER_REGISTRY:?DOCKER_REGISTRY is required}"
: "${DOCKER_REPOSITORY:?DOCKER_REPOSITORY is required}"
: "${WWW_UID:?WWW_UID is required}"
: "${WWW_GID:?WWW_GID is required}"

PACKAGE_FILE="${DEPLOY_PACKAGE:-deploy-package-${PACKAGE_SUFFIX}.tar.gz}"
IMAGE_FILE="${DEPLOY_IMAGE_PACKAGE:-deploy-image-${PACKAGE_SUFFIX}.tar.gz}"
REMOTE_PACKAGE="/tmp/deploy-package-${PACKAGE_SUFFIX}.tar.gz"
REMOTE_IMAGE="/tmp/deploy-image-${PACKAGE_SUFFIX}.tar.gz"

for f in "$PACKAGE_FILE" "$IMAGE_FILE"; do
  if [ ! -f "$f" ]; then
    echo "Missing file: $f" >&2
    exit 1
  fi
done

echo "Uploading ${PACKAGE_FILE} and ${IMAGE_FILE} to ${RUNNER_SSH_ALIAS}..."
scp "$PACKAGE_FILE" "$IMAGE_FILE" "${RUNNER_SSH_ALIAS}:/tmp/"

echo "Running remote-deploy.sh on ${RUNNER_SSH_ALIAS}..."
ssh "${RUNNER_SSH_ALIAS}" \
  SERVICE="$SERVICE" \
  IMAGE_TAG="$IMAGE_TAG" \
  DEPLOY_IMAGE_SOURCE="${DEPLOY_IMAGE_SOURCE:-tarball}" \
  IMAGE_TARBALL="$REMOTE_IMAGE" \
  DOCKER_REGISTRY="$DOCKER_REGISTRY" \
  DOCKER_REPOSITORY="$DOCKER_REPOSITORY" \
  GHCR_USERNAME="${GHCR_USERNAME:-}" \
  GITHUB_TOKEN="${GITHUB_TOKEN:-}" \
  DOCKERHUB_USERNAME="${DOCKERHUB_USERNAME:-}" \
  DOCKERHUB_TOKEN="${DOCKERHUB_TOKEN:-}" \
  SUDO_PASSWORD="${SUDO_PASSWORD:-}" \
  WWW_UID="$WWW_UID" \
  WWW_GID="$WWW_GID" \
  WORKER_REPLICAS="${WORKER_REPLICAS:-}" \
  PKG_PATH="$REMOTE_PACKAGE" \
  bash -s <<'REMOTE'
set -Eeuo pipefail
BOOTSTRAP_TMP="/tmp/qs-deploy-bootstrap-${SERVICE}-$$"
mkdir -p "$BOOTSTRAP_TMP"
trap 'rm -rf "$BOOTSTRAP_TMP"' EXIT
tar -xzf "$PKG_PATH" -C "$BOOTSTRAP_TMP"
bash "$BOOTSTRAP_TMP/scripts/cd/remote-deploy.sh"
REMOTE
