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
docker pull "$IMAGE"
pull_elapsed=$(($(date +%s) - pull_started))
echo "Pulled ${IMAGE} in ${pull_elapsed}s"

echo "Exporting ${IMAGE} to ${OUTPUT}..."
export_started=$(date +%s)
docker save "$IMAGE" | gzip -1 >"$OUTPUT"
export_elapsed=$(($(date +%s) - export_started))
size="$(du -h "$OUTPUT" | awk '{print $1}')"
echo "Created ${OUTPUT} (${size}) in ${export_elapsed}s"
