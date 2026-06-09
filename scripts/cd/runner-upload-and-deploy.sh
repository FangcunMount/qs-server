#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/image-metadata.sh"
# shellcheck source=/dev/null
. "$SCRIPT_DIR/deploy-target.sh"

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

DEPLOY_HOST="$(resolve_ssh_hostname "${RUNNER_SSH_ALIAS}" || true)"
DEPLOY_HOST="${DEPLOY_HOST:-${RUNNER_SSH_HOST:-}}"

echo "=========================================="
echo "CD upload+deploy: service=${SERVICE} image_tag=${IMAGE_TAG}"
echo "SSH alias=${RUNNER_SSH_ALIAS} target_host=${DEPLOY_HOST:-unknown}"
if command -v ssh >/dev/null 2>&1 && [ -n "${RUNNER_SSH_ALIAS:-}" ]; then
  ssh -G "${RUNNER_SSH_ALIAS}" 2>/dev/null | awk '/^(hostname|user|port) /{print "ssh -G resolved: "$0}' || true
fi
echo "Deploy runner local: hostname=$(hostname) tailscale_ip=$(tailscale ip -4 2>/dev/null || true)"
echo "=========================================="

LOCAL_BOOT="$(mktemp)"
trap 'rm -f "$LOCAL_BOOT"' EXIT

cat >"$LOCAL_BOOT" <<'BOOT'
#!/usr/bin/env bash
set -Eeuo pipefail

: "${SERVICE:?SERVICE is required}"
: "${PKG_PATH:?PKG_PATH is required}"

echo "=========================================="
echo "CD bootstrap remote: service=${SERVICE} pkg=${PKG_PATH}"
echo "Deploy host: hostname=$(hostname) tailscale_ip=$(tailscale ip -4 2>/dev/null || true) primary_ip=$(hostname -I 2>/dev/null | awk '{print $1}') user=$(whoami)"
echo "=========================================="

BOOTSTRAP_TMP="/tmp/qs-deploy-bootstrap-${SERVICE}-$$"
mkdir -p "$BOOTSTRAP_TMP"
trap 'rm -rf "$BOOTSTRAP_TMP"' EXIT
tar -xzf "$PKG_PATH" -C "$BOOTSTRAP_TMP"
bash "$BOOTSTRAP_TMP/scripts/cd/remote-deploy.sh"
BOOT
chmod 700 "$LOCAL_BOOT"

run_bootstrap() {
  local boot_script="$1"
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
  bash -se "$boot_script"
}

if is_local_deploy_target "$DEPLOY_HOST"; then
  echo "Deploy target is local (${DEPLOY_HOST}); skipping SSH/SCP and running bootstrap on this host."
  cp -f "$PACKAGE_FILE" "$REMOTE_PACKAGE"
  cp -f "$IMAGE_FILE" "$REMOTE_IMAGE"
  echo "Running remote-deploy.sh locally..."
  if ! run_bootstrap "$LOCAL_BOOT"; then
    echo "remote-deploy.sh failed on local host" >&2
    exit 1
  fi
  exit 0
fi

echo "Uploading ${PACKAGE_FILE} and ${IMAGE_FILE} to ${RUNNER_SSH_ALIAS}..."
scp "$PACKAGE_FILE" "$IMAGE_FILE" "${RUNNER_SSH_ALIAS}:/tmp/"

REMOTE_BOOT="/tmp/qs-cd-bootstrap-${SERVICE}-$$.sh"
echo "Uploading bootstrap script to ${RUNNER_SSH_ALIAS}:${REMOTE_BOOT} ..."
scp "$LOCAL_BOOT" "${RUNNER_SSH_ALIAS}:${REMOTE_BOOT}"

echo "Running remote-deploy.sh on ${RUNNER_SSH_ALIAS}..."
if ! ssh "${RUNNER_SSH_ALIAS}" \
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
  bash -se "$REMOTE_BOOT"
then
  echo "remote-deploy.sh failed on ${RUNNER_SSH_ALIAS}" >&2
  exit 1
fi

ssh "${RUNNER_SSH_ALIAS}" rm -f "$REMOTE_BOOT" || true
