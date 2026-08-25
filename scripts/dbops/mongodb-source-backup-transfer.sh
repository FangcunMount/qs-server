#!/usr/bin/env bash
set -Eeuo pipefail

BACKUP_FILE="${MONGO_BACKUP_FILE:-}"
RUN_TOKEN="${MONGO_BACKUP_RUN_TOKEN:-}"
SOURCE_DIR="${MONGO_BACKUP_SOURCE_DIR:-/opt/backups/qs-server/mongodb-staging}"
if ! [[ "$BACKUP_FILE" =~ ^qs_mongodb_backup_[0-9]{8}_[0-9]{6}\.archive\.gz$ ]]; then
  echo "invalid MongoDB backup file name" >&2
  exit 1
fi
if ! [[ "$RUN_TOKEN" =~ ^[0-9]+-[0-9]+$ ]]; then
  echo "invalid MongoDB backup run token" >&2
  exit 1
fi
: "${MONGO_BACKUP_HOST:?MONGO_BACKUP_HOST is required}"
: "${MONGO_BACKUP_USERNAME:?MONGO_BACKUP_USERNAME is required}"
: "${MONGO_BACKUP_SSH_KEY:?MONGO_BACKUP_SSH_KEY is required}"
: "${MONGO_BACKUP_SSH_FINGERPRINT:?MONGO_BACKUP_SSH_FINGERPRINT is required}"
: "${SVRA_HOST:?SVRA_HOST is required}"
: "${SVRA_USERNAME:?SVRA_USERNAME is required}"
: "${SVRA_SSH_KEY:?SVRA_SSH_KEY is required}"
: "${SVRA_SSH_FINGERPRINT:?SVRA_SSH_FINGERPRINT is required}"

MONGO_BACKUP_SSH_PORT="${MONGO_BACKUP_SSH_PORT:-22}"
SVRA_SSH_PORT="${SVRA_SSH_PORT:-22}"
for port in "$MONGO_BACKUP_SSH_PORT" "$SVRA_SSH_PORT"; do
  if ! [[ "$port" =~ ^[0-9]+$ ]] || (( port < 1 || port > 65535 )); then
    echo "invalid SSH port" >&2
    exit 1
  fi
done

TRANSFER_DIR="$(mktemp -d "${RUNNER_TEMP:-/tmp}/qs-mongodb-transfer.XXXXXX")"
SOURCE_KEY_FILE="$TRANSFER_DIR/source.key"
SOURCE_KNOWN_HOSTS="$TRANSFER_DIR/source.known_hosts"
SVRA_KEY_FILE="$TRANSFER_DIR/servera.key"
SVRA_KNOWN_HOSTS="$TRANSFER_DIR/servera.known_hosts"
LOCAL_ARCHIVE="$TRANSFER_DIR/$BACKUP_FILE"
LOCAL_CHECKSUM="$LOCAL_ARCHIVE.sha256"
REMOTE_PARTIAL="/tmp/qs-mongodb-source-$RUN_TOKEN.partial"
REMOTE_CHECKSUM="$REMOTE_PARTIAL.sha256"
remote_uploaded=false

cleanup() {
  if [ "$remote_uploaded" = false ] && [ -s "$SVRA_KEY_FILE" ] && [ -s "$SVRA_KNOWN_HOSTS" ]; then
    ssh -q -i "$SVRA_KEY_FILE" -p "$SVRA_SSH_PORT" \
      -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
      -o UserKnownHostsFile="$SVRA_KNOWN_HOSTS" \
      "$SVRA_USERNAME@$SVRA_HOST" "rm -f -- '$REMOTE_PARTIAL' '$REMOTE_CHECKSUM'" || true
  fi
  rm -rf -- "$TRANSFER_DIR"
}
trap cleanup EXIT

umask 077
printf '%s\n' "$MONGO_BACKUP_SSH_KEY" >"$SOURCE_KEY_FILE"
printf '%s\n' "$SVRA_SSH_KEY" >"$SVRA_KEY_FILE"
ssh-keyscan -T 10 -p "$MONGO_BACKUP_SSH_PORT" "$MONGO_BACKUP_HOST" 2>/dev/null >"$SOURCE_KNOWN_HOSTS"
ssh-keyscan -T 10 -p "$SVRA_SSH_PORT" "$SVRA_HOST" 2>/dev/null >"$SVRA_KNOWN_HOSTS"
for spec in \
  "$SOURCE_KNOWN_HOSTS|$MONGO_BACKUP_SSH_FINGERPRINT|MongoDB source" \
  "$SVRA_KNOWN_HOSTS|$SVRA_SSH_FINGERPRINT|serverA"; do
  IFS='|' read -r known_hosts expected label <<<"$spec"
  if [ ! -s "$known_hosts" ] || ! ssh-keygen -lf "$known_hosts" -E sha256 | awk '{ print $2 }' | grep -Fqx -- "$expected"; then
    echo "$label SSH fingerprint mismatch" >&2
    exit 1
  fi
done

scp -q -i "$SOURCE_KEY_FILE" -P "$MONGO_BACKUP_SSH_PORT" \
  -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile="$SOURCE_KNOWN_HOSTS" \
  "$MONGO_BACKUP_USERNAME@$MONGO_BACKUP_HOST:$SOURCE_DIR/$BACKUP_FILE" "$LOCAL_ARCHIVE"
scp -q -i "$SOURCE_KEY_FILE" -P "$MONGO_BACKUP_SSH_PORT" \
  -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile="$SOURCE_KNOWN_HOSTS" \
  "$MONGO_BACKUP_USERNAME@$MONGO_BACKUP_HOST:$SOURCE_DIR/$BACKUP_FILE.sha256" "$LOCAL_CHECKSUM"

expected_checksum="$(tr -d '[:space:]' <"$LOCAL_CHECKSUM")"
actual_checksum="$(shasum -a 256 "$LOCAL_ARCHIVE" | awk '{ print $1 }')"
if ! [[ "$expected_checksum" =~ ^[0-9a-f]{64}$ ]] || [ "$actual_checksum" != "$expected_checksum" ]; then
  echo "MongoDB source archive checksum mismatch" >&2
  exit 1
fi

scp -q -i "$SVRA_KEY_FILE" -P "$SVRA_SSH_PORT" \
  -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile="$SVRA_KNOWN_HOSTS" \
  "$LOCAL_ARCHIVE" "$SVRA_USERNAME@$SVRA_HOST:$REMOTE_PARTIAL"
scp -q -i "$SVRA_KEY_FILE" -P "$SVRA_SSH_PORT" \
  -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
  -o UserKnownHostsFile="$SVRA_KNOWN_HOSTS" \
  "$LOCAL_CHECKSUM" "$SVRA_USERNAME@$SVRA_HOST:$REMOTE_CHECKSUM"
remote_uploaded=true
echo "MongoDB compressed archive transferred to serverA staging: file=$BACKUP_FILE checksum=$actual_checksum"
