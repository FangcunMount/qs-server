#!/usr/bin/env bash
set -Eeuo pipefail

BACKUP_FILE="${MONGO_BACKUP_FILE:-}"
RUN_TOKEN="${MONGO_BACKUP_RUN_TOKEN:-}"
BACKUP_DIR="${MONGO_BACKUP_TARGET_DIR:-/opt/backups/qs-server/mongodb}"
if ! [[ "$BACKUP_FILE" =~ ^qs_mongodb_backup_[0-9]{8}_[0-9]{6}\.archive\.gz$ ]]; then
  echo "invalid MongoDB backup file name" >&2
  exit 1
fi
if ! [[ "$RUN_TOKEN" =~ ^[0-9]+-[0-9]+$ ]]; then
  echo "invalid MongoDB backup run token" >&2
  exit 1
fi

if sudo -n true 2>/dev/null; then
  run_privileged() { sudo "$@"; }
else
  : "${SUDO_PASSWORD:?SUDO_PASSWORD is required when passwordless sudo is unavailable}"
  run_privileged() { printf '%s\n' "$SUDO_PASSWORD" | sudo -S "$@"; }
fi

run_privileged rm -f -- \
  "/tmp/qs-mongodb-source-$RUN_TOKEN.partial" \
  "/tmp/qs-mongodb-source-$RUN_TOKEN.partial.sha256" \
  "$BACKUP_DIR/$BACKUP_FILE.partial"
echo "serverA MongoDB transfer staging cleaned: file=$BACKUP_FILE"
