#!/usr/bin/env bash
set -Eeuo pipefail

BACKUP_FILE="${MONGO_BACKUP_FILE:-}"
SOURCE_DIR="${MONGO_BACKUP_SOURCE_DIR:-/opt/backups/qs-server/mongodb-staging}"
if ! [[ "$BACKUP_FILE" =~ ^qs_mongodb_backup_[0-9]{8}_[0-9]{6}\.archive\.gz$ ]]; then
  echo "invalid MongoDB backup file name" >&2
  exit 1
fi

if sudo -n true 2>/dev/null; then
  run_privileged() { sudo "$@"; }
else
  : "${SUDO_PASSWORD:?SUDO_PASSWORD is required when passwordless sudo is unavailable}"
  run_privileged() { printf '%s\n' "$SUDO_PASSWORD" | sudo -S "$@"; }
fi

run_privileged rm -f -- \
  "$SOURCE_DIR/$BACKUP_FILE" \
  "$SOURCE_DIR/$BACKUP_FILE.partial" \
  "$SOURCE_DIR/$BACKUP_FILE.sha256"
echo "MongoDB source staging cleaned: file=$BACKUP_FILE"
