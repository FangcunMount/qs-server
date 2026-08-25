#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
for script in \
  mongodb-source-backup.sh \
  mongodb-source-backup-transfer.sh \
  mongodb-source-backup-finalize.sh \
  mongodb-source-backup-validate.sh \
  mongodb-source-backup-space-budget.sh \
  mongodb-source-backup-servera-cleanup.sh \
  mongodb-source-backup-cleanup.sh; do
  bash -n "$SCRIPT_DIR/$script"
  if [ "$script" = mongodb-source-backup-space-budget.sh ]; then
    output="$(MONGO_BACKUP_REQUIRED_FREE_BYTES='unsafe' bash "$SCRIPT_DIR/$script" 2>&1 || true)"
    if [[ "$output" != *"invalid MongoDB backup free-space requirement"* ]]; then
      echo "$script did not reject an unsafe free-space requirement before side effects" >&2
      exit 1
    fi
    continue
  fi
  output="$(MONGO_BACKUP_FILE='../unsafe.archive.gz' MONGO_BACKUP_RUN_TOKEN='unsafe' bash "$SCRIPT_DIR/$script" 2>&1 || true)"
  if [[ "$output" != *"invalid MongoDB backup file name"* ]]; then
    echo "$script did not reject an unsafe backup file name before side effects" >&2
    exit 1
  fi
done
echo "MongoDB source-backup script validation OK"
