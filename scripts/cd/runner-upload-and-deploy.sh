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

# 把部署所需的环境变量用 printf %q 安全转义后，写进 bootstrap 脚本的 export 段。
# 这样变量不会经过 ssh 命令行传递（inline `VAR=val ... bash file` 在含特殊字符
# 的 secret 上会被远端 shell 截断，导致脚本被忽略、命令静默成功），从根本上避免
# 部署"看似成功实则空跑"的问题。
emit_export() {
  # $1=变量名 $2=值；%q 保证任意字符（# ; 空格 引号等）都安全
  printf 'export %s=%q\n' "$1" "$2" >>"$LOCAL_BOOT"
}

{
  echo '#!/usr/bin/env bash'
  echo 'set -Eeuo pipefail'
  echo ''
} >"$LOCAL_BOOT"

emit_export SERVICE             "$SERVICE"
emit_export IMAGE_TAG           "$IMAGE_TAG"
emit_export DEPLOY_IMAGE_SOURCE "${DEPLOY_IMAGE_SOURCE:-tarball}"
emit_export IMAGE_TARBALL       "$REMOTE_IMAGE"
emit_export DOCKER_REGISTRY     "$DOCKER_REGISTRY"
emit_export DOCKER_REPOSITORY   "$DOCKER_REPOSITORY"
emit_export GHCR_USERNAME       "${GHCR_USERNAME:-}"
emit_export GITHUB_TOKEN        "${GITHUB_TOKEN:-}"
emit_export DOCKERHUB_USERNAME  "${DOCKERHUB_USERNAME:-}"
emit_export DOCKERHUB_TOKEN     "${DOCKERHUB_TOKEN:-}"
emit_export SUDO_PASSWORD       "${SUDO_PASSWORD:-}"
emit_export WWW_UID             "$WWW_UID"
emit_export WWW_GID             "$WWW_GID"
emit_export WORKER_REPLICAS     "${WORKER_REPLICAS:-}"
emit_export PKG_PATH            "$REMOTE_PACKAGE"

cat >>"$LOCAL_BOOT" <<'BOOT'

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

# 脚本含 secret，限制权限
chmod 600 "$LOCAL_BOOT"

if is_local_deploy_target "$DEPLOY_HOST"; then
  echo "Deploy target is local (${DEPLOY_HOST}); skipping SSH/SCP and running bootstrap on this host."
  cp -f "$PACKAGE_FILE" "$REMOTE_PACKAGE"
  cp -f "$IMAGE_FILE" "$REMOTE_IMAGE"
  echo "Running remote-deploy.sh locally..."
  if ! bash "$LOCAL_BOOT"; then
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
ssh "${RUNNER_SSH_ALIAS}" "chmod 600 ${REMOTE_BOOT}" || true

# 远端只执行脚本本身，不在命令行 inline 任何变量（变量已写进脚本的 export 段）。
echo "Running remote-deploy.sh on ${RUNNER_SSH_ALIAS}..."
rc=0
ssh "${RUNNER_SSH_ALIAS}" "bash ${REMOTE_BOOT}" || rc=$?
ssh "${RUNNER_SSH_ALIAS}" "rm -f ${REMOTE_BOOT}" || true
if [ "$rc" -ne 0 ]; then
  echo "remote-deploy.sh failed on ${RUNNER_SSH_ALIAS} (exit ${rc})" >&2
  exit "$rc"
fi
