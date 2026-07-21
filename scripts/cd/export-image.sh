#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/image-metadata.sh"

: "${DOCKER_REGISTRY:?DOCKER_REGISTRY is required}"
: "${DOCKER_REPOSITORY:?DOCKER_REPOSITORY is required}"
: "${DEPLOY_SHA:?DEPLOY_SHA is required}"

EXPORT_IMAGE_REGISTRY="${EXPORT_IMAGE_REGISTRY:-ghcr}"
case "$EXPORT_IMAGE_REGISTRY" in
  ghcr)
    IMAGE="${DOCKER_REGISTRY}/${DOCKER_REPOSITORY}/${IMAGE_NAME}:${DEPLOY_SHA}"
    ;;
  dockerhub)
    : "${DOCKERHUB_USERNAME:?DOCKERHUB_USERNAME is required for EXPORT_IMAGE_REGISTRY=dockerhub}"
    IMAGE="${DOCKERHUB_USERNAME}/${IMAGE_NAME}:${DEPLOY_SHA}"
    ;;
  acr)
    : "${ALIYUN_ACR_REGISTRY:?ALIYUN_ACR_REGISTRY is required for EXPORT_IMAGE_REGISTRY=acr}"
    : "${ALIYUN_ACR_NAMESPACE:?ALIYUN_ACR_NAMESPACE is required for EXPORT_IMAGE_REGISTRY=acr}"
    IMAGE="${ALIYUN_ACR_REGISTRY}/${ALIYUN_ACR_NAMESPACE}/${IMAGE_NAME}:${DEPLOY_SHA}"
    ;;
  *)
    echo "EXPORT_IMAGE_REGISTRY must be ghcr, dockerhub, or acr; got: ${EXPORT_IMAGE_REGISTRY}" >&2
    exit 1
    ;;
esac

OUTPUT="${DEPLOY_IMAGE_PACKAGE:-deploy-image-${PACKAGE_SUFFIX}.tar.gz}"

echo "Pulling ${IMAGE} (${EXPORT_IMAGE_REGISTRY}) for tarball export..."
pull_started=$(date +%s)
# Mac mini runner 为 ARM64，目标机为 linux/amd64，必须指定平台
docker pull --platform linux/amd64 "$IMAGE"
pull_elapsed=$(($(date +%s) - pull_started))
echo "Pulled ${IMAGE} in ${pull_elapsed}s"

echo "Exporting ${IMAGE} to ${OUTPUT}..."
export_started=$(date +%s)
# 不用 `docker save | gzip` 管道：sh 无 pipefail，docker save 中途失败时 gzip 仍会把
# 残缺内容压成合法 gzip 并 exit 0，生成"gzip 完整但内含 tar 截断"的坏包（曾导致目标机
# docker load "unexpected EOF"）。改为先 save 到文件（失败即 set -e 退出），再压缩。
RAW_TAR="${OUTPUT%.gz}"
[ "$RAW_TAR" = "$OUTPUT" ] && RAW_TAR="${OUTPUT}.raw.tar"
rm -f "$RAW_TAR"
docker save "$IMAGE" -o "$RAW_TAR"
gzip -1 -c "$RAW_TAR" >"$OUTPUT"
rm -f "$RAW_TAR"
# 端到端自检：确保 gzip 内的 tar 可完整解出，否则在 runner 端立刻失败（避免坏包流向线上）。
if ! gzip -dc "$OUTPUT" | tar -tf - >/dev/null 2>&1; then
  echo "Export integrity check failed: ${OUTPUT} contains a truncated/corrupt tar" >&2
  exit 1
fi
export_elapsed=$(($(date +%s) - export_started))
size="$(du -h "$OUTPUT" | awk '{print $1}')"
echo "Created ${OUTPUT} (${size}) in ${export_elapsed}s"
