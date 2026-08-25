#!/usr/bin/env bash
set -Eeuo pipefail

BACKUP_FILE="${MONGO_BACKUP_FILE:-}"
BACKUP_DIR="${MONGO_BACKUP_TARGET_DIR:-/opt/backups/qs-server/mongodb}"
if ! [[ "$BACKUP_FILE" =~ ^qs_mongodb_backup_[0-9]{8}_[0-9]{6}\.archive\.gz$ ]]; then
  echo "invalid MongoDB backup file name" >&2
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

if ! run_privileged test -s "$BACKUP_DIR/$BACKUP_FILE"; then
  echo "MongoDB backup archive is missing or empty on serverA" >&2
  exit 1
fi
validation_succeeded=false
cleanup_invalid_archive() {
  if [ "$validation_succeeded" = false ]; then
    run_privileged rm -f -- "$BACKUP_DIR/$BACKUP_FILE" || true
  fi
}
trap cleanup_invalid_archive EXIT
if ! run_privileged docker network inspect infra-network >/dev/null 2>&1; then
  echo "infra-network is unavailable on serverA" >&2
  exit 1
fi
run_privileged docker run --rm --network infra-network \
  -v "$BACKUP_DIR:/backup:ro" \
  --entrypoint /usr/bin/timeout \
  mongo:7.0 110m mongorestore \
    --host="$MONGODB_HOST" \
    --port="${MONGODB_PORT:-27017}" \
    --username="$MONGODB_USERNAME" \
    --password="$MONGODB_PASSWORD" \
    --authenticationDatabase=admin \
    --archive="/backup/$BACKUP_FILE" \
    --gzip \
    --nsInclude="$MONGODB_DBNAME.*" \
    --dryRun \
    --verbose

mapfile -t backups < <(
  run_privileged find "$BACKUP_DIR" -maxdepth 1 -type f -name 'qs_mongodb_backup_*.archive.gz' -printf '%T@ %p\n' |
    sort -nr |
    awk '{ print $2 }'
)
if (( ${#backups[@]} > 5 )); then
  for old_backup in "${backups[@]:5}"; do
    run_privileged rm -f -- "$old_backup"
  done
fi
validation_succeeded=true
trap - EXIT
echo "MongoDB source-side backup dry-run validation succeeded: file=$BACKUP_FILE"
