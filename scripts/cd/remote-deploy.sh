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
COLLECTION_COMPOSE_PROJECT="${COLLECTION_COMPOSE_PROJECT:-qs-collection}"
WORKER_COMPOSE_PROJECT="${WORKER_COMPOSE_PROJECT:-qs-worker}"

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
  if [ "$SERVICE" != "collection" ]; then
    $SUDO mkdir -p "/data/logs/qs-server/${CONTAINER_NAME}"
  fi
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
  local expected_cn="${2:-}"
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

  if [ -n "$expected_cn" ]; then
    if ! command -v openssl >/dev/null 2>&1; then
      echo "openssl is required to verify the gRPC certificate identity" >&2
      exit 1
    fi

    # Do not use `$SUDO openssl`: serverA deploy sudoers allows rsync/chown/chmod
    # but not /usr/bin/openssl. Copy to a user-owned temp file instead.
    local tmp_crt cert_subject cert_cn
    tmp_crt="$(mktemp)"
    if ! $SUDO rsync -a "$grpc_crt" "$tmp_crt"; then
      rm -f "$tmp_crt"
      echo "Failed to stage gRPC certificate for CN verification: $grpc_crt" >&2
      exit 1
    fi
    $SUDO chown "$(id -u):$(id -g)" "$tmp_crt"
    $SUDO chmod 0600 "$tmp_crt"
    if ! cert_subject="$(openssl x509 -in "$tmp_crt" -noout -subject -nameopt RFC2253)"; then
      rm -f "$tmp_crt"
      echo "Failed to parse gRPC certificate: $grpc_crt" >&2
      exit 1
    fi
    rm -f "$tmp_crt"
    cert_subject="${cert_subject#subject=}"
    cert_cn="$(printf '%s\n' "$cert_subject" | tr ',' '\n' | sed -n 's/^CN=//p' | head -n 1)"
    if [ "$cert_cn" != "$expected_cn" ]; then
      echo "Unexpected gRPC certificate CN for ${cert_name}: got ${cert_cn:-<missing>}, want ${expected_cn}" >&2
      exit 1
    fi
  fi
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

image_tarball_path() {
  printf '%s' "${IMAGE_TARBALL:-/tmp/deploy-image-${PACKAGE_SUFFIX}.tar.gz}"
}

write_compose_image_env() {
  local image="$1"
  printf -v "$IMAGE_ENV_VAR" '%s' "$image"
  export "$IMAGE_ENV_VAR"
  COMPOSE_ENV_FILE="$DEPLOY_TMP/compose-image.env"
  printf '%s=%s\n' "$IMAGE_ENV_VAR" "$image" >"$COMPOSE_ENV_FILE"
  chmod 0600 "$COMPOSE_ENV_FILE"
  export COMPOSE_ENV_FILE
}

load_image_from_tarball() {
  local tarball image_ref
  tarball="$(image_tarball_path)"
  if [ ! -f "$tarball" ]; then
    return 1
  fi

  image_ref="${DOCKER_REGISTRY}/${DOCKER_REPOSITORY}/${IMAGE_NAME}:${IMAGE_TAG}"
  echo "Loading ${IMAGE_NAME} from tarball ${tarball}..."
  local load_started load_elapsed load_output loaded_ref
  load_started=$(date +%s)
  # 不能用 `gzip -dc | $SUDO docker load`：带密码 sudo 是 `sudo -S ... <<<"$PASSWORD"`，
  # here-string 会把 docker load 的 stdin 改成密码串、冲掉管道里的镜像数据，导致
  # "unexpected EOF"。改用 docker load -i（自动解压 gzip），stdin 不被占用。
  load_output="$($SUDO docker load -i "$tarball")"
  printf '%s\n' "$load_output"
  load_elapsed=$(($(date +%s) - load_started))
  echo "Loaded ${IMAGE_NAME} from tarball in ${load_elapsed}s"

  # 镜像可能用其它 registry（如 ACR）导出，load 进来的 repotag 与 compose 引用的
  # ghcr ref 不一致，会导致 compose "No such image"。把实际加载的 ref retag 成期望 ref。
  loaded_ref="$(printf '%s\n' "$load_output" | sed -n 's/^Loaded image: //p' | head -n1)"
  if [ -n "$loaded_ref" ] && [ "$loaded_ref" != "$image_ref" ]; then
    echo "Retagging ${loaded_ref} -> ${image_ref}"
    $SUDO docker tag "$loaded_ref" "$image_ref"
  fi
  if ! $SUDO docker image inspect "$image_ref" >/dev/null 2>&1; then
    echo "Image ${image_ref} not present after load/retag" >&2
    return 1
  fi
  rm -f "$tarball"
  IMAGE_LOADED_FROM_TARBALL=1
  export IMAGE_LOADED_FROM_TARBALL
  write_compose_image_env "$image_ref"
  return 0
}

select_image_from_registry() {
  local image="${DOCKER_REGISTRY}/${DOCKER_REPOSITORY}/${IMAGE_NAME}:${IMAGE_TAG}"
  local pull_registry="${DEPLOY_PULL_REGISTRY:-dockerhub}"

  case "$pull_registry" in
    dockerhub|ghcr|auto) ;;
    *)
      echo "DEPLOY_PULL_REGISTRY must be dockerhub, ghcr, or auto; got: ${pull_registry}" >&2
      exit 1
      ;;
  esac

  if [ "$pull_registry" = "dockerhub" ] || [ "$pull_registry" = "auto" ]; then
    if [ -n "${DOCKERHUB_USERNAME:-}" ] && [ -n "${DOCKERHUB_TOKEN:-}" ]; then
      echo "Checking Docker Hub login for ${DOCKERHUB_USERNAME}/${IMAGE_NAME}"
      if docker_login_with_token "$DOCKERHUB_USERNAME" "$DOCKERHUB_TOKEN"; then
        image="${DOCKERHUB_USERNAME}/${IMAGE_NAME}:${IMAGE_TAG}"
        write_compose_image_env "$image"
        return 0
      fi
      echo "Docker Hub login failed." >&2
      if [ "$pull_registry" = "dockerhub" ]; then
        exit 1
      fi
    elif [ "$pull_registry" = "dockerhub" ]; then
      echo "Docker Hub credentials missing for DEPLOY_PULL_REGISTRY=dockerhub." >&2
      exit 1
    fi
  fi

  echo "Checking GHCR login for ${DOCKER_REPOSITORY}/${IMAGE_NAME}"
  if [ -z "${GITHUB_TOKEN:-}" ]; then
    echo "GHCR token missing; cannot pull ${image}." >&2
    exit 1
  fi
  if ! docker_login_with_token "$GHCR_USERNAME" "$GITHUB_TOKEN" "$DOCKER_REGISTRY"; then
    echo "GHCR login failed; verify GHCR credentials." >&2
    exit 1
  fi

  write_compose_image_env "$image"
}

select_image() {
  local source="${DEPLOY_IMAGE_SOURCE:-auto}"
  local tarball
  tarball="$(image_tarball_path)"

  case "$source" in
    tarball)
      if [ ! -f "$tarball" ]; then
        echo "DEPLOY_IMAGE_SOURCE=tarball but tarball not found: ${tarball}" >&2
        exit 1
      fi
      load_image_from_tarball
      return 0
      ;;
    registry)
      select_image_from_registry
      return 0
      ;;
    auto)
      if load_image_from_tarball; then
        return 0
      fi
      select_image_from_registry
      return 0
      ;;
    *)
      echo "DEPLOY_IMAGE_SOURCE must be auto, tarball, or registry; got: ${source}" >&2
      exit 1
      ;;
  esac
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

remove_legacy_compose_service() {
  local project="$1"
  local legacy_service="$2"
  local container_ids container_id

  if [ "$legacy_service" = "$COMPOSE_SERVICE" ]; then
    return 0
  fi

  container_ids="$(
    $SUDO docker ps -a \
      --filter "label=com.docker.compose.project=${project}" \
      --filter "label=com.docker.compose.service=${legacy_service}" \
      --format '{{.ID}}'
  )"
  if [ -z "$container_ids" ]; then
    return 0
  fi

  echo "Removing containers from legacy Compose service ${project}/${legacy_service}..."
  while IFS= read -r container_id; do
    [ -z "$container_id" ] && continue
    $SUDO docker rm -f "$container_id"
  done <<<"$container_ids"
}

LEGACY_COLLECTION_CONTAINER_IDS=""

quiesce_legacy_collection_service() {
  LEGACY_COLLECTION_CONTAINER_IDS="$(
    $SUDO docker ps \
      --filter "label=com.docker.compose.project=${COLLECTION_COMPOSE_PROJECT}" \
      --filter "label=com.docker.compose.service=qs-collection-server" \
      --format '{{.ID}}'
  )"
  if [ -z "$LEGACY_COLLECTION_CONTAINER_IDS" ]; then
    return 0
  fi

  echo "Stopping legacy collection Compose containers before DNS and routing verification..."
  local container_id
  while IFS= read -r container_id; do
    [ -z "$container_id" ] && continue
    if ! $SUDO docker stop "$container_id"; then
      restore_legacy_collection_service
      return 1
    fi
  done <<<"$LEGACY_COLLECTION_CONTAINER_IDS"
}

restore_legacy_collection_service() {
  if [ -z "$LEGACY_COLLECTION_CONTAINER_IDS" ]; then
    return 0
  fi

  echo "Restarting legacy collection Compose containers after failed cutover..." >&2
  local container_id
  while IFS= read -r container_id; do
    [ -z "$container_id" ] && continue
    $SUDO docker start "$container_id" || true
  done <<<"$LEGACY_COLLECTION_CONTAINER_IDS"
}

verify_collection_nginx() {
  local mode="$1"
  local verifier="$DEPLOY_TMP/scripts/cd/verify-collection-nginx.sh"
  if [ ! -x "$verifier" ]; then
    echo "Collection Nginx verifier is missing or not executable: $verifier" >&2
    return 1
  fi

  PRIVILEGE_RUNNER="$SUDO" \
    NGINX_CONFIG_SOURCE="$DEPLOY_TMP/configs/nginx/conf.d/collect.fangcunmount.cn.conf" \
    NGINX_CONFIG_BACKUP_DIR="$BACKUP_DIR" \
    EXPECTED_COLLECTION_REPLICAS="$COLLECTION_REPLICAS" \
    COLLECTION_COMPOSE_PROJECT="$COLLECTION_COMPOSE_PROJECT" \
    COLLECTION_COMPOSE_SERVICE="$COMPOSE_SERVICE" \
    ROUTING_PROBE_REQUESTS="${COLLECTION_ROUTING_PROBE_REQUESTS:-40}" \
    bash "$verifier" "$mode"
}

docker_compose_pull_supports_quiet() {
  $SUDO docker compose pull --help 2>/dev/null | grep -q -- '--quiet'
}

docker_compose() {
  if [ -z "${COMPOSE_ENV_FILE:-}" ] || [ ! -f "$COMPOSE_ENV_FILE" ]; then
    echo "COMPOSE_ENV_FILE is not ready before docker compose execution" >&2
    exit 1
  fi

  $SUDO docker compose --env-file "$COMPOSE_ENV_FILE" "$@"
}

# 镜像要么已通过 tarball docker load，要么已由 docker_compose_pull 提前拉好，
# compose up 不应再回源拉取（否则会因 ghcr 无凭据而 "error from registry: denied"）。
# 老版本 compose 不支持 --pull 时返回空，保持兼容。
compose_up_pull_never_flag() {
  if $SUDO docker compose up --help 2>/dev/null | grep -q -- '--pull'; then
    printf '%s' '--pull never'
  fi
}

resolve_compose_image_ref() {
  if [ -z "${COMPOSE_ENV_FILE:-}" ] || [ ! -f "$COMPOSE_ENV_FILE" ]; then
    echo "COMPOSE_ENV_FILE is not ready before resolving compose image" >&2
    exit 1
  fi

  # shellcheck disable=SC1090
  . "$COMPOSE_ENV_FILE"
  printf '%s\n' "${!IMAGE_ENV_VAR}"
}

docker_compose_pull() {
  local -a compose_args=("$@")
  local image_ref pull_started pull_elapsed
  if [ -z "${COMPOSE_ENV_FILE:-}" ] || [ ! -f "$COMPOSE_ENV_FILE" ]; then
    echo "COMPOSE_ENV_FILE is not ready before docker compose pull" >&2
    exit 1
  fi

  image_ref="$(resolve_compose_image_ref)"
  if [ "${IMAGE_LOADED_FROM_TARBALL:-0}" = "1" ]; then
    echo "Image already loaded from tarball; skipping registry pull for ${image_ref}"
    return 0
  fi

  if $SUDO docker image inspect "$image_ref" >/dev/null 2>&1; then
    echo "Image ${image_ref} already present locally; skipping registry pull"
    return 0
  fi

  echo "Pulling ${COMPOSE_SERVICE} image tag ${IMAGE_TAG} from registry..."
  pull_started=$(date +%s)
  if docker_compose_pull_supports_quiet; then
    docker_compose "${compose_args[@]}" pull --quiet "$COMPOSE_SERVICE"
  else
    docker_compose "${compose_args[@]}" pull "$COMPOSE_SERVICE"
  fi
  pull_elapsed=$(($(date +%s) - pull_started))
  echo "Pulled ${COMPOSE_SERVICE} image in ${pull_elapsed}s"
}

deploy_http_service() {
  cd "/opt/qs-server/${CONTAINER_NAME}"
  docker_compose_pull -f "$DEPLOY_TMP/docker-compose.prod.yml"

  stop_single_container
  # shellcheck disable=SC2046
  docker_compose -f "$DEPLOY_TMP/docker-compose.prod.yml" up -d $(compose_up_pull_never_flag) "$COMPOSE_SERVICE"

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

collection_container_ids() {
  docker_compose \
    -p "$COLLECTION_COMPOSE_PROJECT" \
    -f "$DEPLOY_TMP/docker-compose.prod.yml" \
    ps --status running -q "$COMPOSE_SERVICE"
}

verify_collection_images() {
  local container_ids running_count container_id running_image
  container_ids="$(collection_container_ids)"
  running_count="$(printf '%s\n' "$container_ids" | sed '/^$/d' | wc -l | tr -d '[:space:]')"
  if [ "$running_count" -ne "$COLLECTION_REPLICAS" ]; then
    echo "Collection image verification found ${running_count}/${COLLECTION_REPLICAS} running replicas" >&2
    return 1
  fi

  while IFS= read -r container_id; do
    [ -z "$container_id" ] && continue
    running_image="$($SUDO docker inspect "$container_id" --format '{{.Config.Image}}' 2>/dev/null || true)"
    echo "Collection replica ${container_id} image: ${running_image}"
    case "$running_image" in
      *:"${IMAGE_TAG}")
        ;;
      *)
        echo "Collection replica ${container_id} is not running tag ${IMAGE_TAG}" >&2
        return 1
        ;;
    esac
  done <<<"$container_ids"
}

collection_serving_ready() {
  local container_id="$1"
  local probe_output

  if probe_output="$($SUDO docker exec "$container_id" wget -S -O /dev/null \
    "http://127.0.0.1:${INTERNAL_HTTP_PORT}/serve-readyz" 2>&1)"; then
    return 0
  fi

  # One-release compatibility window: an old image does not expose the new
  # serving-readiness endpoint. A real 503 must never fall back to /readyz.
  if printf '%s\n' "$probe_output" | grep -Eq 'HTTP/[0-9.]+[[:space:]]+404'; then
    echo "Collection replica ${container_id} does not expose /serve-readyz; falling back to /readyz for this release"
    $SUDO docker exec "$container_id" wget -qO- \
      "http://127.0.0.1:${INTERNAL_HTTP_PORT}/readyz" >/dev/null 2>&1
    return
  fi

  return 1
}

deploy_collection() {
  : "${COLLECTION_REPLICAS:?COLLECTION_REPLICAS is required}"
  if ! [[ "$COLLECTION_REPLICAS" =~ ^[0-9]+$ ]] || [ "$COLLECTION_REPLICAS" -lt 1 ]; then
    echo "COLLECTION_REPLICAS must be a positive integer, got: $COLLECTION_REPLICAS" >&2
    exit 1
  fi

  cd "/opt/qs-server/${CONTAINER_NAME}"
  docker_compose_pull \
    -p "$COLLECTION_COMPOSE_PROJECT" \
    -f "$DEPLOY_TMP/docker-compose.prod.yml"

  # Remove the pre-scale container created by the legacy single-instance
  # project. Subsequent deploys are reconciled by the stable compose project.
  stop_single_container
  # shellcheck disable=SC2046
  docker_compose \
    -p "$COLLECTION_COMPOSE_PROJECT" \
    -f "$DEPLOY_TMP/docker-compose.prod.yml" \
    up -d $(compose_up_pull_never_flag) \
    --scale "${COMPOSE_SERVICE}=${COLLECTION_REPLICAS}" \
    "$COMPOSE_SERVICE"

  echo "Waiting for ${COLLECTION_REPLICAS} collection replicas to become serving-ready..."
  local attempts=0
  local max_attempts=60
  local container_ids running_count ready_count container_id
  while [ "$attempts" -lt "$max_attempts" ]; do
    container_ids="$(collection_container_ids)"
    running_count="$(printf '%s\n' "$container_ids" | sed '/^$/d' | wc -l | tr -d '[:space:]')"
    ready_count=0
    while IFS= read -r container_id; do
      [ -z "$container_id" ] && continue
      if collection_serving_ready "$container_id"; then
        ready_count=$((ready_count + 1))
      fi
    done <<<"$container_ids"

    if [ "$running_count" -eq "$COLLECTION_REPLICAS" ] &&
      [ "$ready_count" -eq "$COLLECTION_REPLICAS" ]; then
      echo "Collection replicas are serving-ready (${ready_count}/${COLLECTION_REPLICAS})"
      verify_collection_images
      quiesce_legacy_collection_service
      if ! verify_collection_nginx install-and-verify; then
        restore_legacy_collection_service
        return 1
      fi
      remove_legacy_compose_service "$COLLECTION_COMPOSE_PROJECT" "qs-collection-server"
      docker_compose \
        -p "$COLLECTION_COMPOSE_PROJECT" \
        -f "$DEPLOY_TMP/docker-compose.prod.yml" \
        ps "$COMPOSE_SERVICE"
      return 0
    fi

    attempts=$((attempts + 1))
    if [ "$attempts" -lt "$max_attempts" ]; then
      echo "Collection serving readiness ${ready_count}/${COLLECTION_REPLICAS} (running ${running_count}), attempt ${attempts}/${max_attempts}"
      sleep 5
    fi
  done

  echo "Collection replicas failed serving readiness after ${max_attempts} attempts" >&2
  docker_compose \
    -p "$COLLECTION_COMPOSE_PROJECT" \
    -f "$DEPLOY_TMP/docker-compose.prod.yml" \
    ps "$COMPOSE_SERVICE" || true
  docker_compose \
    -p "$COLLECTION_COMPOSE_PROJECT" \
    -f "$DEPLOY_TMP/docker-compose.prod.yml" \
    logs --tail 100 "$COMPOSE_SERVICE" || true
  exit 1
}

deploy_worker() {
  : "${WORKER_REPLICAS:?WORKER_REPLICAS is required}"
  if ! [[ "$WORKER_REPLICAS" =~ ^[0-9]+$ ]] || [ "$WORKER_REPLICAS" -lt 1 ]; then
    echo "WORKER_REPLICAS must be a positive integer, got: $WORKER_REPLICAS" >&2
    exit 1
  fi

  cd "/opt/qs-server/${CONTAINER_NAME}"
  docker_compose_pull -p "$WORKER_COMPOSE_PROJECT" -f "$DEPLOY_TMP/docker-compose.prod.yml"

  echo "Cleaning up legacy non-Compose worker containers..."
  local legacy_workers
  legacy_workers=$($SUDO docker ps -a --format '{{.ID}} {{.Names}}' | awk '$2 == "qs-worker" || $2 ~ /^qs-deploy-worker-[0-9]+-qs-worker-[0-9]+$/ {print $1}')
  if [ -n "$legacy_workers" ]; then
    local legacy_worker
    printf '%s\n' "$legacy_workers" | while IFS= read -r legacy_worker; do
      [ -z "$legacy_worker" ] && continue
      $SUDO docker rm -f "$legacy_worker" || true
    done
  fi

  # shellcheck disable=SC2046
  docker_compose -p "$WORKER_COMPOSE_PROJECT" -f "$DEPLOY_TMP/docker-compose.prod.yml" up -d $(compose_up_pull_never_flag) --scale "${COMPOSE_SERVICE}=${WORKER_REPLICAS}" "$COMPOSE_SERVICE"

  echo "Waiting for container to start..."
  sleep 10

  local running_count worker_containers first_worker ready
  running_count=$(docker_compose -p "$WORKER_COMPOSE_PROJECT" -f "$DEPLOY_TMP/docker-compose.prod.yml" ps --status running -q "$COMPOSE_SERVICE" | wc -l | tr -d ' ')

  if [ "$running_count" -lt "$WORKER_REPLICAS" ]; then
    echo "Worker replicas failed to reach expected count (${running_count}/${WORKER_REPLICAS})" >&2
    docker_compose -p "$WORKER_COMPOSE_PROJECT" -f "$DEPLOY_TMP/docker-compose.prod.yml" ps "$COMPOSE_SERVICE" || true
    docker_compose -p "$WORKER_COMPOSE_PROJECT" -f "$DEPLOY_TMP/docker-compose.prod.yml" logs --tail 100 "$COMPOSE_SERVICE" || true
    exit 1
  fi

  echo "Worker replicas are running (${running_count}/${WORKER_REPLICAS})"
  docker_compose -p "$WORKER_COMPOSE_PROJECT" -f "$DEPLOY_TMP/docker-compose.prod.yml" ps "$COMPOSE_SERVICE"
  worker_containers="$(docker_compose -p "$WORKER_COMPOSE_PROJECT" -f "$DEPLOY_TMP/docker-compose.prod.yml" ps -q "$COMPOSE_SERVICE")"
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
    docker_compose -p "$WORKER_COMPOSE_PROJECT" -f "$DEPLOY_TMP/docker-compose.prod.yml" logs --tail 100 "$COMPOSE_SERVICE" || true
    exit 1
  fi

  echo "Worker can resolve and reach qs-apiserver:9090"
  remove_legacy_compose_service "$WORKER_COMPOSE_PROJECT" "qs-worker"
  echo "Recent logs (all worker replicas):"
  docker_compose -p "$WORKER_COMPOSE_PROJECT" -f "$DEPLOY_TMP/docker-compose.prod.yml" logs --tail 20 "$COMPOSE_SERVICE"
}

echo "=========================================="
echo "Deploying ${CONTAINER_NAME}"
echo "Image tag: ${IMAGE_TAG}"
echo "Deploy host: hostname=$(hostname) tailscale_ip=$(tailscale ip -4 2>/dev/null || true) primary_ip=$(hostname -I 2>/dev/null | awk '{print $1}') user=$(id -un)"
echo "=========================================="

prepare_dirs_and_backup
extract_package
sync_configs
ensure_networks

case "$SERVICE" in
  apiserver)
    setup_apiserver_paths
    setup_apiserver_web_tls
    setup_grpc_certs qs-apiserver qs-apiserver.svc
    select_image
    deploy_http_service
    ;;
  collection)
    echo "Deploy target: serverA (co-located with qs-apiserver). Stop legacy qs-collection-server on serverB after cutover."
    setup_grpc_certs qs-collection-server qs-collection-server.svc
    verify_collection_nginx preflight
    select_image
    deploy_collection
    ;;
  worker)
    setup_grpc_certs qs-worker qs-worker.svc
    select_image
    deploy_worker
    ;;
esac

cleanup_old_backups
rm -rf "$DEPLOY_TMP"
rm -f "$PKG_PATH"

verify_running_image() {
  case "$SERVICE" in
    collection|worker)
      return 0
      ;;
  esac

  local running_image
  running_image="$($SUDO docker inspect "$CONTAINER_NAME" --format '{{.Config.Image}}' 2>/dev/null || true)"
  echo "Running image: ${running_image}"
  case "$running_image" in
    *:"${IMAGE_TAG}")
      return 0
      ;;
  esac
  echo "Deploy verification failed: ${CONTAINER_NAME} is not running tag ${IMAGE_TAG}" >&2
  exit 1
}

verify_running_image

echo "=========================================="
echo "${CONTAINER_NAME} deployment completed"
echo "=========================================="
