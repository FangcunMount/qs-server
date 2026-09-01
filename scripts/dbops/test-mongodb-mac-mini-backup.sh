#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/mongodb-mac-mini-backup.sh"
WORKFLOW="$SCRIPT_DIR/../../.github/workflows/db-ops.yml"
bash -n "$SCRIPT"

output="$(MONGO_BACKUP_OPERATION=unsafe bash "$SCRIPT" 2>&1 || true)"
if [[ "$output" != *"invalid Mac mini MongoDB backup operation"* ]]; then
  echo "Mac mini backup script did not reject an invalid operation before side effects" >&2
  exit 1
fi

output="$(MONGO_BACKUP_OPERATION=backup MONGO_BACKUP_DIR=/tmp/unsafe bash "$SCRIPT" 2>&1 || true)"
if [[ "$output" != *"unsafe Mac mini MongoDB backup directory"* ]]; then
  echo "Mac mini backup script did not reject an unsafe backup directory" >&2
  exit 1
fi

output="$(MONGO_BACKUP_OPERATION=validate MONGO_BACKUP_FILE=../unsafe.archive.gz bash "$SCRIPT" 2>&1 || true)"
if [[ "$output" != *"invalid MongoDB backup file name"* ]]; then
  echo "Mac mini backup script did not reject an unsafe backup name" >&2
  exit 1
fi

if grep -n -- '--password=' "$SCRIPT" >/dev/null; then
  echo "Mac mini backup script exposes the MongoDB password through process arguments" >&2
  exit 1
fi
if ! grep -Fq 'MONGO_BACKUP_RETENTION_COUNT:-3' "$SCRIPT"; then
  echo "Mac mini backup script does not default to three retained backups" >&2
  exit 1
fi
if ! grep -Fq 'socketTimeoutMS=0' "$SCRIPT"; then
  echo "Mac mini backup script does not preserve unlimited archive read timeout semantics" >&2
  exit 1
fi
if ! grep -Fq "grep -Fq '(CursorNotFound)'" "$SCRIPT" ||
  ! grep -Fq 'dump_attempt >= 2' "$SCRIPT" ||
  ! grep -Fq 'removing incomplete archive and retrying once' "$SCRIPT"; then
  echo "Mac mini backup script does not bound CursorNotFound recovery to one retry" >&2
  exit 1
fi
if grep -En 'mongodb-source-backup|MongoDB Source-side Backup|Keeping last 5|keeping last 5' \
  "$WORKFLOW" "$SCRIPT" >/dev/null; then
  echo "legacy serverA/source-host MongoDB backup path is still active" >&2
  exit 1
fi
if ! grep -Fq 'name: MongoDB Backup on Mac mini' "$WORKFLOW"; then
  echo "scheduled MongoDB backup is not routed to the Mac mini" >&2
  exit 1
fi

echo "MongoDB Mac mini backup script validation OK"
