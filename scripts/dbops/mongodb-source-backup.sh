#!/usr/bin/env bash
set -Eeuo pipefail

BACKUP_FILE="${MONGO_BACKUP_FILE:-}"
RUN_TOKEN="${MONGO_BACKUP_RUN_TOKEN:-}"
MONGO_CONTAINER="${MONGO_BACKUP_LOCAL_CONTAINER:-mongodb}"
SOURCE_DIR="${MONGO_BACKUP_SOURCE_DIR:-/opt/backups/qs-server/mongodb-staging}"
REQUIRED_FREE_BYTES="${MONGO_BACKUP_REQUIRED_FREE_BYTES:-1073741824}"

if ! [[ "$BACKUP_FILE" =~ ^qs_mongodb_backup_[0-9]{8}_[0-9]{6}\.archive\.gz$ ]]; then
  echo "invalid MongoDB backup file name" >&2
  exit 1
fi
if ! [[ "$RUN_TOKEN" =~ ^[0-9]+-[0-9]+$ ]]; then
  echo "invalid MongoDB backup run token" >&2
  exit 1
fi
if ! [[ "$MONGO_CONTAINER" =~ ^[A-Za-z0-9_.-]+$ ]]; then
  echo "invalid local MongoDB container name" >&2
  exit 1
fi
if ! [[ "$REQUIRED_FREE_BYTES" =~ ^[0-9]+$ ]] || (( REQUIRED_FREE_BYTES < 1 )); then
  echo "invalid MongoDB backup free-space requirement" >&2
  exit 1
fi
: "${MONGODB_HOST:?MONGODB_HOST is required}"
: "${MONGODB_DBNAME:?MONGODB_DBNAME is required}"
: "${MONGODB_USERNAME:?MONGODB_USERNAME is required}"
: "${MONGODB_PASSWORD:?MONGODB_PASSWORD is required}"

if sudo -n true 2>/dev/null; then
  run_privileged() { sudo "$@"; }
else
  : "${SUDO_PASSWORD:?SUDO_PASSWORD is required when passwordless sudo is unavailable}"
  run_privileged() { printf '%s\n' "$SUDO_PASSWORD" | sudo -S "$@"; }
fi

if ! run_privileged docker network inspect infra-network >/dev/null 2>&1; then
  echo "infra-network is unavailable on the MongoDB source host" >&2
  exit 1
fi
if [ "$(run_privileged docker inspect -f '{{.State.Running}}' "$MONGO_CONTAINER" 2>/dev/null || true)" != true ]; then
  echo "the configured local MongoDB container is not running on this host" >&2
  exit 1
fi
if ! run_privileged docker inspect -f '{{range $name, $_ := .NetworkSettings.Networks}}{{println $name}}{{end}}' "$MONGO_CONTAINER" |
  grep -Fqx infra-network; then
  echo "the configured MongoDB container is not attached to infra-network" >&2
  exit 1
fi
local_endpoint=false
if [ "$MONGODB_HOST" = "$MONGO_CONTAINER" ]; then
  local_endpoint=true
fi
if run_privileged docker inspect -f '{{range (index .NetworkSettings.Networks "infra-network").Aliases}}{{println .}}{{end}}' "$MONGO_CONTAINER" |
  grep -Fqx -- "$MONGODB_HOST"; then
  local_endpoint=true
fi
container_ip="$(run_privileged docker inspect -f '{{(index .NetworkSettings.Networks "infra-network").IPAddress}}' "$MONGO_CONTAINER")"
if [ -n "$container_ip" ] && [ "$MONGODB_HOST" = "$container_ip" ]; then
  local_endpoint=true
fi
if [ "$local_endpoint" != true ]; then
  echo "MONGODB_HOST does not identify the configured local MongoDB container" >&2
  exit 1
fi

run_privileged install -d -m 0750 "$SOURCE_DIR"
run_privileged chown "$(id -u):$(id -g)" "$SOURCE_DIR"
available_bytes="$(df -PB1 "$SOURCE_DIR" | awk 'NR == 2 { print $4 }')"
if ! [[ "$available_bytes" =~ ^[0-9]+$ ]] || (( available_bytes < REQUIRED_FREE_BYTES )); then
  echo "insufficient source-host space: available=${available_bytes:-unknown} required=${REQUIRED_FREE_BYTES}" >&2
  exit 1
fi

SOURCE_FILE="$SOURCE_DIR/$BACKUP_FILE"
PARTIAL_FILE="$SOURCE_FILE.partial"
CHECKSUM_FILE="$SOURCE_FILE.sha256"
BACKUP_CONTAINER="qs-mongodb-source-backup-$RUN_TOKEN"
if [ -e "$SOURCE_FILE" ] || [ -e "$PARTIAL_FILE" ] || [ -e "$CHECKSUM_FILE" ]; then
  echo "source backup staging files already exist for this run" >&2
  exit 1
fi

cleanup_container() {
  run_privileged docker rm -f "$BACKUP_CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup_container EXIT

MONGODB_PORT="${MONGODB_PORT:-27017}"
MONGODB_URI="mongodb://${MONGODB_HOST}:${MONGODB_PORT}/?connectTimeoutMS=30000&socketTimeoutMS=0"
run_privileged docker run --rm --network infra-network \
  --name "$BACKUP_CONTAINER" \
  --label "com.fangcunmount.qs-server.operation=mongodb-source-backup" \
  --label "com.fangcunmount.qs-server.github-run-token=$RUN_TOKEN" \
  -v "$SOURCE_DIR:/backup" \
  -e MONGODB_URI="$MONGODB_URI" \
  -e MONGODB_USERNAME="$MONGODB_USERNAME" \
  -e MONGODB_PASSWORD="$MONGODB_PASSWORD" \
  -e MONGODB_DBNAME="$MONGODB_DBNAME" \
  -e BACKUP_FILE="$BACKUP_FILE" \
  --entrypoint /usr/bin/timeout \
  mongo:7.0 345m /bin/bash -Eeuo pipefail -c '
    partial="/backup/$BACKUP_FILE.partial"
    final="/backup/$BACKUP_FILE"
    cleanup_partial() { rm -f -- "$partial"; }
    trap cleanup_partial EXIT
    mongodump \
      --uri="$MONGODB_URI" \
      --username="$MONGODB_USERNAME" \
      --password="$MONGODB_PASSWORD" \
      --authenticationDatabase=admin \
      --db="$MONGODB_DBNAME" \
      --numParallelCollections=4 \
      --gzip \
      --archive="$partial"
    test -s "$partial"
    mv -- "$partial" "$final"
    chmod 0640 "$final"
    trap - EXIT
  '

test -s "$SOURCE_FILE"
source_checksum="$(run_privileged sha256sum "$SOURCE_FILE" | awk '{ print $1 }')"
if ! [[ "$source_checksum" =~ ^[0-9a-f]{64}$ ]]; then
  echo "failed to calculate source archive checksum" >&2
  exit 1
fi
printf '%s\n' "$source_checksum" >"$CHECKSUM_FILE"
run_privileged chown "$(id -u):$(id -g)" "$SOURCE_FILE" "$CHECKSUM_FILE"
chmod 0640 "$SOURCE_FILE" "$CHECKSUM_FILE"
trap - EXIT
echo "MongoDB source backup ready: file=$BACKUP_FILE bytes=$(stat -c %s "$SOURCE_FILE") checksum=$source_checksum"
