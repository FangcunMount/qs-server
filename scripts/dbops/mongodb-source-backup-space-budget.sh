#!/usr/bin/env bash
set -Eeuo pipefail

REQUIRED_FREE_BYTES="${MONGO_BACKUP_REQUIRED_FREE_BYTES:-}"
BACKUP_DIR="${MONGO_BACKUP_TARGET_DIR:-/opt/backups/qs-server/mongodb}"
if ! [[ "$REQUIRED_FREE_BYTES" =~ ^[0-9]+$ ]] || (( REQUIRED_FREE_BYTES < 1 )); then
  echo "invalid MongoDB backup free-space requirement" >&2
  exit 1
fi

if sudo -n true 2>/dev/null; then
  run_privileged() { sudo "$@"; }
else
  : "${SUDO_PASSWORD:?SUDO_PASSWORD is required when passwordless sudo is unavailable}"
  run_privileged() { printf '%s\n' "$SUDO_PASSWORD" | sudo -S "$@"; }
fi

latest_file="$(
  run_privileged find "$BACKUP_DIR" -maxdepth 1 -type f -name 'qs_mongodb_backup_*.archive.gz' -printf '%T@ %p\n' |
    sort -nr |
    awk 'NR == 1 { $1 = ""; sub(/^ /, ""); print }'
)"
if [ -z "$latest_file" ] || ! run_privileged test -s "$latest_file"; then
  echo "no recent valid MongoDB archive is available for the source-space preflight" >&2
  exit 1
fi

recent_size="$(run_privileged stat -c %s "$latest_file")"
if ! [[ "$recent_size" =~ ^[0-9]+$ ]] || (( recent_size < 1 )); then
  echo "invalid recent MongoDB archive size" >&2
  exit 1
fi
minimum_required=$(( recent_size * 2 ))
if (( REQUIRED_FREE_BYTES < minimum_required )); then
  echo "source-space budget is below twice the latest archive: configured=$REQUIRED_FREE_BYTES required=$minimum_required" >&2
  exit 1
fi

echo "MongoDB source-space budget accepted: recent_archive_bytes=$recent_size required_free_bytes=$REQUIRED_FREE_BYTES"
