#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source=/dev/null
. "$SCRIPT_DIR/image-metadata.sh"

: "${DOCKER_REGISTRY:?DOCKER_REGISTRY is required}"
: "${DOCKER_REPOSITORY:?DOCKER_REPOSITORY is required}"
: "${GHCR_USERNAME:?GHCR_USERNAME is required}"
: "${WWW_UID:?WWW_UID is required}"
: "${WWW_GID:?WWW_GID is required}"

APP_UID="${WWW_UID}"
APP_GID="${WWW_GID}"
IMAGE_TAG="${IMAGE_TAG:-latest}"

case "$IMAGE_TAG" in
  ""|*[!A-Za-z0-9_.-]*)
    echo "IMAGE_TAG must contain only letters, digits, underscores, periods, and dashes; got: ${IMAGE_TAG}" >&2
    exit 1
    ;;
esac

if sudo -n true 2>/dev/null; then
  SUDO="sudo"
  echo "Using passwordless sudo."
else
  if [ -z "${SUDO_PASSWORD:-}" ]; then
    echo "sudo needs password. Provide SUDO_PASSWORD or configure NOPASSWD." >&2
    exit 1
  fi
  sudo_pw() { sudo -S "$@" <<<"$SUDO_PASSWORD"; }
  export -f sudo_pw
  SUDO="sudo_pw"
  $SUDO -v || true
  echo "Using sudo with password."
fi

if [ "$SUDO" = "sudo_pw" ]; then
  SUDO_ASKPASS_SCRIPT="$(mktemp)"
  printf '%s\n' '#!/bin/sh' 'printf '\''%s\n'\'' "$SUDO_PASSWORD"' >"$SUDO_ASKPASS_SCRIPT"
  chmod 700 "$SUDO_ASKPASS_SCRIPT"
fi

cleanup_auth_helpers() {
  if [ -n "${SUDO_ASKPASS_SCRIPT:-}" ]; then
    rm -f "$SUDO_ASKPASS_SCRIPT"
  fi
}
trap cleanup_auth_helpers EXIT

docker_login_with_token() {
  local username="$1"
  local token="$2"
  local registry="${3:-}"
  local token_file rc

  token_file="$(mktemp)"
  chmod 600 "$token_file"
  printf '%s' "$token" >"$token_file"
  rc=1

  if [ "$SUDO" = "sudo_pw" ]; then
    if [ -n "$registry" ]; then
      if SUDO_ASKPASS="$SUDO_ASKPASS_SCRIPT" sudo -A docker login "$registry" -u "$username" --password-stdin <"$token_file" >/dev/null; then
        rc=0
      else
        rc=$?
      fi
    else
      if SUDO_ASKPASS="$SUDO_ASKPASS_SCRIPT" sudo -A docker login -u "$username" --password-stdin <"$token_file" >/dev/null; then
        rc=0
      else
        rc=$?
      fi
    fi
  else
    if [ -n "$registry" ]; then
      if $SUDO docker login "$registry" -u "$username" --password-stdin <"$token_file" >/dev/null; then
        rc=0
      else
        rc=$?
      fi
    else
      if $SUDO docker login -u "$username" --password-stdin <"$token_file" >/dev/null; then
        rc=0
      else
        rc=$?
      fi
    fi
  fi

  rm -f "$token_file"
  return "$rc"
}

prepare_dirs_and_backup() {
  $SUDO mkdir -p "/opt/qs-server/${CONTAINER_NAME}/configs/env"
  $SUDO mkdir -p "/data/logs/qs-server/${CONTAINER_NAME}"
  $SUDO mkdir -p "/opt/backups/qs-server/${CONTAINER_NAME}"

  BACKUP_DIR="/opt/backups/qs-server/${CONTAINER_NAME}"
  $SUDO chown "$(id -u):$(id -g)" "$BACKUP_DIR"
  $SUDO chmod 0750 "$BACKUP_DIR"

  local timestamp
  timestamp=$(date +%Y%m%d_%H%M%S)
  if [ -d "/opt/qs-server/${CONTAINER_NAME}/configs" ] && [ "$(ls -A "/opt/qs-server/${CONTAINER_NAME}/configs" 2>/dev/null)" != "" ]; then
    $SUDO tar -czf "$BACKUP_DIR/backup_${timestamp}.tar.gz" \
      "/opt/qs-server/${CONTAINER_NAME}/configs" \
      2>/dev/null || echo "No previous backup"
  fi
}

extract_package() {
  if [ ! -f "$PKG_PATH" ]; then
    echo "${PKG_PATH} not found" >&2
    ls -al /tmp/deploy-package*.tar.gz 2>/dev/null || true
    exit 1
  fi

  DEPLOY_TMP="${DEPLOY_TMP:-/tmp/qs-deploy-${PACKAGE_SUFFIX}-$$}"
  mkdir -p "$DEPLOY_TMP"
  tar -xzf "$PKG_PATH" -C "$DEPLOY_TMP"
}

sync_configs() {
  $SUDO rsync -a "$DEPLOY_TMP/configs/" "/opt/qs-server/${CONTAINER_NAME}/configs/"
  $SUDO chown -R "$APP_UID:$APP_GID" "/opt/qs-server/${CONTAINER_NAME}/configs"
  $SUDO chown -R "$APP_UID:$APP_GID" "/data/logs/qs-server/${CONTAINER_NAME}"
}

ensure_networks() {
  if ! $SUDO docker network ls --format '{{.Name}}' | grep -w qs-network >/dev/null 2>&1; then
    echo "Creating Docker network qs-network..."
    $SUDO docker network create qs-network
  fi
  if ! $SUDO docker network ls --format '{{.Name}}' | grep -w infra-network >/dev/null 2>&1; then
    echo "infra-network not found. Please ensure infrastructure is deployed first." >&2
    exit 1
  fi
}

setup_grpc_certs() {
  local cert_name="$1"
  local grpc_cert_dir="/data/infra/ssl/grpc"

  if ! $SUDO test -d "$grpc_cert_dir"; then
    echo "gRPC certificate directory not found: $grpc_cert_dir" >&2
    exit 1
  fi

  local grpc_ca="$grpc_cert_dir/ca/ca-chain.crt"
  local grpc_crt="$grpc_cert_dir/server/${cert_name}.crt"
  local grpc_key="$grpc_cert_dir/server/${cert_name}.key"
  local f

  for f in "$grpc_ca" "$grpc_crt" "$grpc_key"; do
    if ! $SUDO test -r "$f"; then
      echo "Missing or unreadable gRPC mTLS file: $f" >&2
      exit 1
    fi
  done

  $SUDO chown "$APP_UID:$APP_GID" "$grpc_ca" "$grpc_crt" "$grpc_key"
  $SUDO chmod 0644 "$grpc_ca" "$grpc_crt"
  $SUDO chmod 0640 "$grpc_key"
}

setup_apiserver_paths() {
  $SUDO mkdir -p /data/image/qrcode
  $SUDO chmod 0777 /data/image
  $SUDO chown -R "$APP_UID:$APP_GID" /data/image/qrcode
  $SUDO chmod 0755 /data/image/qrcode
}

setup_apiserver_web_tls() {
  local cert_host_path="/data/ssl/certs/fangcunmount.cn.crt"
  local key_host_path="/data/ssl/private/fangcunmount.cn.key"

  if ! $SUDO test -r "$cert_host_path"; then
    echo "Web CERT not readable: $cert_host_path" >&2
    $SUDO ls -l "$cert_host_path" || true
    exit 1
  fi
  if ! $SUDO test -r "$key_host_path"; then
    echo "Web KEY not readable: $key_host_path" >&2
    $SUDO namei -l "$key_host_path" || true
    exit 1
  fi

  local tls_cert_dir tls_key_dir
  tls_cert_dir="$(dirname "$cert_host_path")"
  tls_key_dir="$(dirname "$key_host_path")"
  $SUDO chown "$APP_UID:$APP_GID" "$cert_host_path" "$key_host_path" "$tls_cert_dir" "$tls_key_dir"
  $SUDO chmod 0755 "$tls_cert_dir"
  $SUDO chmod 0750 "$tls_key_dir"
  $SUDO chmod 0644 "$cert_host_path"
  $SUDO chmod 0640 "$key_host_path"
}

select_image() {
  local image="${DOCKER_REGISTRY}/${DOCKER_REPOSITORY}/${IMAGE_NAME}:${IMAGE_TAG}"
  local ghcr_login_ok=0

  echo "Checking registry login for ${DOCKER_REPOSITORY}/${IMAGE_NAME}"
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    docker_login_with_token "$GHCR_USERNAME" "$GITHUB_TOKEN" "$DOCKER_REGISTRY" && ghcr_login_ok=1 || ghcr_login_ok=0
  fi

  if [ "$ghcr_login_ok" -ne 1 ]; then
    if [ -n "${DOCKERHUB_USERNAME:-}" ] && [ -n "${DOCKERHUB_TOKEN:-}" ]; then
      echo "GHCR login failed; falling back to Docker Hub..."
      local dockerhub_login_ok=0
      docker_login_with_token "$DOCKERHUB_USERNAME" "$DOCKERHUB_TOKEN" && dockerhub_login_ok=1 || dockerhub_login_ok=0
      if [ "$dockerhub_login_ok" -ne 1 ]; then
        echo "Docker Hub login failed; verify DOCKERHUB_USERNAME/DOCKERHUB_TOKEN." >&2
        exit 1
      fi
      image="${DOCKERHUB_USERNAME}/${IMAGE_NAME}:${IMAGE_TAG}"
    else
      echo "GHCR login failed and Docker Hub credentials missing."
    fi
  fi

  printf -v "$IMAGE_ENV_VAR" '%s' "$image"
  export "$IMAGE_ENV_VAR"
}

cleanup_old_backups() {
  local old_backups backup_file
  old_backups="$($SUDO ls -t "$BACKUP_DIR"/backup_*.tar.gz 2>/dev/null || true)"
  old_backups="$(printf '%s\n' "$old_backups" | tail -n +6 || true)"
  if [ -n "$old_backups" ]; then
    printf '%s\n' "$old_backups" | while IFS= read -r backup_file; do
      [ -z "$backup_file" ] && continue
      $SUDO rm -f "$backup_file" || true
    done
  fi
}

stop_single_container() {
  if $SUDO docker ps -a --format '{{.Names}}' | grep -w "$CONTAINER_NAME" >/dev/null 2>&1; then
    echo "Stopping existing container..."
    $SUDO docker stop "$CONTAINER_NAME" || true
    $SUDO docker rm "$CONTAINER_NAME" || true
  fi
}

docker_compose_pull_supports_quiet() {
  $SUDO docker compose pull --help 2>/dev/null | grep -q -- '--quiet'
}

docker_compose() {
  local image_value="${!IMAGE_ENV_VAR:-}"
  if [ -z "$image_value" ]; then
    echo "${IMAGE_ENV_VAR} is not set before docker compose execution" >&2
    exit 1
  fi

  $SUDO env "$IMAGE_ENV_VAR=$image_value" docker compose "$@"
}

docker_compose_pull() {
  local -a compose_args=("$@")
  local image_value="${!IMAGE_ENV_VAR:-}"
  if [ -z "$image_value" ]; then
    echo "${IMAGE_ENV_VAR} is not set before docker compose pull" >&2
    exit 1
  fi

  if docker_compose_pull_supports_quiet; then
    $SUDO env "$IMAGE_ENV_VAR=$image_value" docker compose "${compose_args[@]}" pull --quiet "$COMPOSE_SERVICE"
  else
    $SUDO env "$IMAGE_ENV_VAR=$image_value" COMPOSE_PROGRESS=plain docker compose "${compose_args[@]}" pull "$COMPOSE_SERVICE"
  fi
}

deploy_http_service() {
  stop_single_container

  cd "/opt/qs-server/${CONTAINER_NAME}"
  docker_compose_pull -f "$DEPLOY_TMP/docker-compose.prod.yml"
  docker_compose -f "$DEPLOY_TMP/docker-compose.prod.yml" up -d "$COMPOSE_SERVICE"

  echo "Waiting for service to be ready (in-container health check)..."
  local attempts=0
  local max_attempts=60
  while [ "$attempts" -lt "$max_attempts" ]; do
    if $SUDO docker exec "$CONTAINER_NAME" wget -qO- "http://127.0.0.1:${INTERNAL_HTTP_PORT}${HEALTH_PATH}" >/dev/null 2>&1; then
      echo "Health check passed (attempt $attempts)"
      $SUDO docker ps --filter "name=${CONTAINER_NAME}" --format "table {{.Names}}\t{{.Status}}"
      return 0
    fi
    attempts=$((attempts + 1))
    if [ "$attempts" -lt "$max_attempts" ]; then
      echo "Health check attempt $attempts/$max_attempts, retrying..."
      sleep 5
    fi
  done

  echo "Service failed to start after $max_attempts attempts" >&2
  $SUDO docker logs --tail 100 "$CONTAINER_NAME" || true
  exit 1
}

deploy_worker() {
  : "${WORKER_REPLICAS:?WORKER_REPLICAS is required}"
  if ! [[ "$WORKER_REPLICAS" =~ ^[0-9]+$ ]] || [ "$WORKER_REPLICAS" -lt 1 ]; then
    echo "WORKER_REPLICAS must be a positive integer, got: $WORKER_REPLICAS" >&2
    exit 1
  fi

  echo "Cleaning up legacy worker containers..."
  local legacy_workers
  legacy_workers=$($SUDO docker ps -a --format '{{.ID}} {{.Names}}' | awk '$2 == "qs-worker" || $2 ~ /^qs-deploy-worker-[0-9]+-qs-worker-[0-9]+$/ {print $1}')
  if [ -n "$legacy_workers" ]; then
    local legacy_worker
    printf '%s\n' "$legacy_workers" | while IFS= read -r legacy_worker; do
      [ -z "$legacy_worker" ] && continue
      $SUDO docker rm -f "$legacy_worker" || true
    done
  fi

  cd "/opt/qs-server/${CONTAINER_NAME}"
  docker_compose_pull -p qs-worker -f "$DEPLOY_TMP/docker-compose.prod.yml"
  docker_compose -p qs-worker -f "$DEPLOY_TMP/docker-compose.prod.yml" up -d --scale "${COMPOSE_SERVICE}=${WORKER_REPLICAS}" "$COMPOSE_SERVICE"

  echo "Waiting for container to start..."
  sleep 10

  local running_count worker_containers first_worker ready
  running_count=$(docker_compose -p qs-worker -f "$DEPLOY_TMP/docker-compose.prod.yml" ps --status running -q "$COMPOSE_SERVICE" | wc -l | tr -d ' ')

  if [ "$running_count" -lt "$WORKER_REPLICAS" ]; then
    echo "Worker replicas failed to reach expected count (${running_count}/${WORKER_REPLICAS})" >&2
    docker_compose -p qs-worker -f "$DEPLOY_TMP/docker-compose.prod.yml" ps "$COMPOSE_SERVICE" || true
    docker_compose -p qs-worker -f "$DEPLOY_TMP/docker-compose.prod.yml" logs --tail 100 "$COMPOSE_SERVICE" || true
    exit 1
  fi

  echo "Worker replicas are running (${running_count}/${WORKER_REPLICAS})"
  docker_compose -p qs-worker -f "$DEPLOY_TMP/docker-compose.prod.yml" ps "$COMPOSE_SERVICE"
  worker_containers="$(docker_compose -p qs-worker -f "$DEPLOY_TMP/docker-compose.prod.yml" ps -q "$COMPOSE_SERVICE")"
  first_worker="$(printf '%s\n' "$worker_containers" | sed -n '1p')"
  if [ -z "$first_worker" ]; then
    echo "No running worker container found for connectivity check" >&2
    exit 1
  fi

  echo "Verifying worker can resolve and reach qs-apiserver:9090 ..."
  ready=0
  for _ in $(seq 1 20); do
    if $SUDO docker exec "$first_worker" sh -lc 'getent hosts qs-apiserver >/dev/null 2>&1 && nc -z qs-apiserver 9090 >/dev/null 2>&1'; then
      ready=1
      break
    fi
    sleep 3
  done
  if [ "$ready" -ne 1 ]; then
    echo "Worker cannot resolve or reach qs-apiserver:9090 from inside container" >&2
    $SUDO docker exec "$first_worker" sh -lc 'echo "--- /etc/resolv.conf ---"; cat /etc/resolv.conf; echo "--- getent ---"; getent hosts qs-apiserver || true; echo "--- nc ---"; nc -vz qs-apiserver 9090 || true'
    docker_compose -p qs-worker -f "$DEPLOY_TMP/docker-compose.prod.yml" logs --tail 100 "$COMPOSE_SERVICE" || true
    exit 1
  fi

  echo "Worker can resolve and reach qs-apiserver:9090"
  echo "Recent logs (all worker replicas):"
  docker_compose -p qs-worker -f "$DEPLOY_TMP/docker-compose.prod.yml" logs --tail 20 "$COMPOSE_SERVICE"
}

echo "=========================================="
echo "Deploying ${CONTAINER_NAME}"
echo "Image tag: ${IMAGE_TAG}"
echo "=========================================="

prepare_dirs_and_backup
extract_package
sync_configs
ensure_networks

case "$SERVICE" in
  apiserver)
    setup_apiserver_paths
    setup_apiserver_web_tls
    setup_grpc_certs qs-apiserver
    select_image
    deploy_http_service
    ;;
  collection)
    setup_grpc_certs qs-collection-server
    select_image
    deploy_http_service
    ;;
  worker)
    setup_grpc_certs qs-worker
    select_image
    deploy_worker
    ;;
esac

cleanup_old_backups
rm -rf "$DEPLOY_TMP"
rm -f "$PKG_PATH"

echo "=========================================="
echo "${CONTAINER_NAME} deployment completed"
echo "=========================================="
