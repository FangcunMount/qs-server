#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$SCRIPT_DIR/image-metadata.sh"

: "${DOCKER_REGISTRY:?DOCKER_REGISTRY is required}"
: "${DOCKER_REPOSITORY:?DOCKER_REPOSITORY is required}"
: "${DEPLOY_REF:?DEPLOY_REF is required}"
: "${DEPLOY_SHA:?DEPLOY_SHA is required}"
: "${WWW_UID:?WWW_UID is required}"
: "${WWW_GID:?WWW_GID is required}"

case "${WWW_UID}:${WWW_GID}" in
  *[!0-9:]*|:*|*:|"")
    echo "WWW_UID/WWW_GID must be numeric" >&2
    exit 1
    ;;
esac

BUILD_TIME="${BUILD_TIME:-$(date -u '+%Y-%m-%dT%H:%M:%SZ')}"
BUILD_CACHE_REF="${BUILD_CACHE_REF:-${DOCKER_REGISTRY}/${DOCKER_REPOSITORY}/${IMAGE_NAME}:buildcache}"
CACHE_ARGS="--cache-from type=registry,ref=${BUILD_CACHE_REF} --cache-to type=registry,ref=${BUILD_CACHE_REF},mode=max,ignore-error=true"

# One-time retirement preload: cache only the approved API image; never publish
# latest or invoke deployment. R4 removes this branch after the window closes.
if [ -n "${RETIREMENT_PRELOAD_REF:-}" ]; then
  [ "$SERVICE" = apiserver ] && [ "${#DEPLOY_SHA}" -eq 40 ] || exit 1
  case "$DEPLOY_SHA" in *[!0-9a-f]*) exit 1 ;; esac
  [ "$RETIREMENT_PRELOAD_REF" = "qs-retirement/qs-apiserver:${DEPLOY_SHA}" ] || exit 1
  set -- --load --platform linux/amd64 --label "org.opencontainers.image.revision=${DEPLOY_SHA}" --tag "$RETIREMENT_PRELOAD_REF"
else
  set -- --push --tag "${DOCKER_REGISTRY}/${DOCKER_REPOSITORY}/${IMAGE_NAME}:latest" --tag "${DOCKER_REGISTRY}/${DOCKER_REPOSITORY}/${IMAGE_NAME}:${DEPLOY_SHA}"
fi
# shellcheck disable=SC2086
docker buildx build \
  --file "$DOCKERFILE" \
  "$@" \
  --build-arg "VERSION=${DEPLOY_REF}" \
  --build-arg "BUILD_TIME=${BUILD_TIME}" \
  --build-arg "GIT_COMMIT=${DEPLOY_SHA}" \
  --build-arg "GIT_BRANCH=${DEPLOY_REF}" \
  --build-arg "RUN_UID=${WWW_UID}" \
  --build-arg "RUN_GID=${WWW_GID}" \
  $CACHE_ARGS \
  .
