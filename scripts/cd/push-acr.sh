#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/image-metadata.sh"

: "${DOCKER_REGISTRY:?DOCKER_REGISTRY is required}"
: "${DOCKER_REPOSITORY:?DOCKER_REPOSITORY is required}"
: "${DEPLOY_SHA:?DEPLOY_SHA is required}"
: "${ALIYUN_ACR_REGISTRY:?ALIYUN_ACR_REGISTRY is required}"
: "${ALIYUN_ACR_NAMESPACE:?ALIYUN_ACR_NAMESPACE is required}"

SRC="${DOCKER_REGISTRY}/${DOCKER_REPOSITORY}/${IMAGE_NAME}:${DEPLOY_SHA}"
DST_BASE="${ALIYUN_ACR_REGISTRY}/${ALIYUN_ACR_NAMESPACE}/${IMAGE_NAME}"

docker pull "$SRC"
docker tag "$SRC" "${DST_BASE}:latest"
docker tag "$SRC" "${DST_BASE}:${DEPLOY_SHA}"
docker push "${DST_BASE}:latest"
docker push "${DST_BASE}:${DEPLOY_SHA}"

echo "Image pushed to Aliyun ACR:"
echo "  - ${DST_BASE}:latest"
echo "  - ${DST_BASE}:${DEPLOY_SHA}"
