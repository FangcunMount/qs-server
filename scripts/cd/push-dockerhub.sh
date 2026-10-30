#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$SCRIPT_DIR/image-metadata.sh"

: "${DOCKER_REGISTRY:?DOCKER_REGISTRY is required}"
: "${DOCKER_REPOSITORY:?DOCKER_REPOSITORY is required}"
: "${DOCKERHUB_USERNAME:?DOCKERHUB_USERNAME is required}"
: "${DEPLOY_SHA:?DEPLOY_SHA is required}"

SRC="${DOCKER_REGISTRY}/${DOCKER_REPOSITORY}/${IMAGE_NAME}:${DEPLOY_SHA}"
DST="${DOCKERHUB_USERNAME}/${IMAGE_NAME}"
docker pull "$SRC"
docker tag "$SRC" "${DST}:${DEPLOY_SHA}"
docker push "${DST}:${DEPLOY_SHA}"
if [ "${PUBLISH_LATEST:-true}" = true ]; then
  docker tag "$SRC" "${DST}:latest"
  docker push "${DST}:latest"
fi
