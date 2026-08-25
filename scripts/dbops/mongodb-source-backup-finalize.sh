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

REMOTE_PARTIAL="/tmp/qs-mongodb-source-$RUN_TOKEN.partial"
REMOTE_CHECKSUM="$REMOTE_PARTIAL.sha256"
TARGET_PARTIAL="$BACKUP_DIR/$BACKUP_FILE.partial"
TARGET_FILE="$BACKUP_DIR/$BACKUP_FILE"
cleanup() {
  run_privileged rm -f -- "$REMOTE_PARTIAL" "$REMOTE_CHECKSUM" "$TARGET_PARTIAL" || true
}
trap cleanup EXIT

test -s "$REMOTE_PARTIAL"
test -s "$REMOTE_CHECKSUM"
expected_checksum="$(tr -d '[:space:]' <"$REMOTE_CHECKSUM")"
actual_checksum="$(sha256sum "$REMOTE_PARTIAL" | awk '{ print $1 }')"
if ! [[ "$expected_checksum" =~ ^[0-9a-f]{64}$ ]] || [ "$actual_checksum" != "$expected_checksum" ]; then
  echo "serverA staging checksum mismatch" >&2
  exit 1
fi

run_privileged install -d -m 0750 "$BACKUP_DIR"
if run_privileged test -e "$TARGET_FILE"; then
  echo "refusing to overwrite an existing MongoDB backup" >&2
  exit 1
fi
run_privileged mv -- "$REMOTE_PARTIAL" "$TARGET_PARTIAL"
run_privileged chown root:root "$TARGET_PARTIAL"
run_privileged chmod 0640 "$TARGET_PARTIAL"
target_checksum="$(run_privileged sha256sum "$TARGET_PARTIAL" | awk '{ print $1 }')"
if [ "$target_checksum" != "$expected_checksum" ]; then
  echo "serverA target checksum mismatch" >&2
  exit 1
fi
run_privileged mv -- "$TARGET_PARTIAL" "$TARGET_FILE"
run_privileged rm -f -- "$REMOTE_CHECKSUM"
trap - EXIT
echo "MongoDB backup finalized on serverA: file=$BACKUP_FILE bytes=$(run_privileged stat -c %s "$TARGET_FILE") checksum=$expected_checksum"
