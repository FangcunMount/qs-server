#!/usr/bin/env sh
set -eu

: "${SERVICE:?SERVICE is required}"

case "$SERVICE" in
  apiserver|qs-apiserver)
    SERVICE=apiserver
    IMAGE_NAME=qs-apiserver
    DOCKERFILE=build/docker/Dockerfile.qs-apiserver
    COMPOSE_SERVICE=qs-apiserver
    CONTAINER_NAME=qs-apiserver
    PACKAGE_SUFFIX=apiserver
    IMAGE_ENV_VAR=DOCKER_IMAGE_QS_APISERVER
    INTERNAL_HTTP_PORT="${INTERNAL_HTTP_PORT:-8080}"
    HEALTH_PATH="${HEALTH_PATH:-/health}"
    ;;
  collection|collection-server|qs-collection-server)
    SERVICE=collection
    IMAGE_NAME=qs-collection-server
    DOCKERFILE=build/docker/Dockerfile.collection-server
    COMPOSE_SERVICE=qs-collection-server
    CONTAINER_NAME=qs-collection-server
    PACKAGE_SUFFIX=collection
    IMAGE_ENV_VAR=DOCKER_IMAGE_QS_COLLECTION_SERVER
    INTERNAL_HTTP_PORT="${INTERNAL_HTTP_PORT:-8080}"
    HEALTH_PATH="${HEALTH_PATH:-/health}"
    ;;
  worker|qs-worker)
    SERVICE=worker
    IMAGE_NAME=qs-worker
    DOCKERFILE=build/docker/Dockerfile.qs-worker
    COMPOSE_SERVICE=qs-worker
    CONTAINER_NAME=qs-worker
    PACKAGE_SUFFIX=worker
    IMAGE_ENV_VAR=DOCKER_IMAGE_QS_WORKER
    ;;
  *)
    echo "Unsupported SERVICE: $SERVICE" >&2
    exit 1
    ;;
esac

DEPLOY_PACKAGE="${DEPLOY_PACKAGE:-deploy-package-${PACKAGE_SUFFIX}.tar.gz}"
PKG_PATH="${PKG_PATH:-/tmp/${DEPLOY_PACKAGE}}"

export SERVICE IMAGE_NAME DOCKERFILE COMPOSE_SERVICE CONTAINER_NAME PACKAGE_SUFFIX IMAGE_ENV_VAR DEPLOY_PACKAGE PKG_PATH
export INTERNAL_HTTP_PORT="${INTERNAL_HTTP_PORT:-}" HEALTH_PATH="${HEALTH_PATH:-}"

if [ "${IMAGE_METADATA_PRINT:-}" = "1" ]; then
  cat <<EOF
SERVICE=${SERVICE}
IMAGE_NAME=${IMAGE_NAME}
DOCKERFILE=${DOCKERFILE}
COMPOSE_SERVICE=${COMPOSE_SERVICE}
CONTAINER_NAME=${CONTAINER_NAME}
PACKAGE_SUFFIX=${PACKAGE_SUFFIX}
IMAGE_ENV_VAR=${IMAGE_ENV_VAR}
DEPLOY_PACKAGE=${DEPLOY_PACKAGE}
PKG_PATH=${PKG_PATH}
EOF
fi
