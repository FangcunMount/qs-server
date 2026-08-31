#!/usr/bin/env bash
set -Eeuo pipefail

# GitHub Actions services on macOS do not inherit the interactive Homebrew PATH.
export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin${PATH:+:$PATH}"

OPERATION="${MONGO_BACKUP_OPERATION:-}"
BACKUP_DIR="${MONGO_BACKUP_DIR:-$HOME/backups/qs-server/mongodb}"
BACKUP_FILE_INPUT="${MONGO_BACKUP_FILE:-}"
RETENTION_COUNT="${MONGO_BACKUP_RETENTION_COUNT:-3}"
REMOTE_MONGO_PORT="${MONGODB_PORT:-27017}"
SSH_PORT="${MONGO_BACKUP_SSH_PORT:-22}"
TUNNEL_PORT="${MONGO_BACKUP_TUNNEL_PORT:-37017}"

case "$OPERATION" in
  backup | validate | restore) ;;
  *)
    echo "invalid Mac mini MongoDB backup operation" >&2
    exit 1
    ;;
esac
if ! [[ "$BACKUP_DIR" == /*/backups/qs-server/mongodb ]] || [[ "$BACKUP_DIR" == *'..'* ]]; then
  echo "unsafe Mac mini MongoDB backup directory" >&2
  exit 1
fi
if ! [[ "$RETENTION_COUNT" =~ ^[0-9]+$ ]] || (( RETENTION_COUNT != 3 )); then
  echo "Mac mini MongoDB backup retention must be exactly 3" >&2
  exit 1
fi
for port in "$REMOTE_MONGO_PORT" "$SSH_PORT" "$TUNNEL_PORT"; do
  if ! [[ "$port" =~ ^[0-9]+$ ]] || (( port < 1 || port > 65535 )); then
    echo "invalid MongoDB backup port" >&2
    exit 1
  fi
done
if [ "$OPERATION" != backup ] &&
  ! [[ "$BACKUP_FILE_INPUT" =~ ^qs_mongodb_backup_[0-9]{8}_[0-9]{6}\.archive\.gz$ ]]; then
  echo "invalid MongoDB backup file name" >&2
  exit 1
fi

: "${MONGO_BACKUP_HOST:?MONGO_BACKUP_HOST is required}"
: "${MONGO_BACKUP_USERNAME:?MONGO_BACKUP_USERNAME is required}"
: "${MONGO_BACKUP_SSH_KEY:?MONGO_BACKUP_SSH_KEY is required}"
: "${MONGO_BACKUP_SSH_FINGERPRINT:?MONGO_BACKUP_SSH_FINGERPRINT is required}"
: "${MONGODB_DBNAME:?MONGODB_DBNAME is required}"
: "${MONGODB_USERNAME:?MONGODB_USERNAME is required}"
: "${MONGODB_PASSWORD:?MONGODB_PASSWORD is required}"

for command_name in mongodump mongorestore ssh ssh-keyscan ssh-keygen nc shasum; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "required Mac mini command is unavailable: $command_name" >&2
    exit 1
  fi
done

install -d -m 0750 "$BACKUP_DIR"
TEMP_DIR="$(mktemp -d "${RUNNER_TEMP:-/tmp}/qs-mongodb-mac-mini.XXXXXX")"
KEY_FILE="$TEMP_DIR/serverc.key"
KNOWN_HOSTS="$TEMP_DIR/serverc.known_hosts"
MONGO_CONFIG="$TEMP_DIR/mongodb-tools.yml"
TUNNEL_PID=''
PARTIAL_FILE=''
CHECKSUM_PARTIAL=''
cleanup() {
  if [[ "$TUNNEL_PID" =~ ^[0-9]+$ ]]; then
    kill "$TUNNEL_PID" >/dev/null 2>&1 || true
    wait "$TUNNEL_PID" >/dev/null 2>&1 || true
  fi
  if [ -n "$PARTIAL_FILE" ]; then
    rm -f -- "$PARTIAL_FILE"
  fi
  if [ -n "$CHECKSUM_PARTIAL" ]; then
    rm -f -- "$CHECKSUM_PARTIAL"
  fi
  rm -rf -- "$TEMP_DIR"
}
trap cleanup EXIT

umask 077
printf '%s\n' "$MONGO_BACKUP_SSH_KEY" >"$KEY_FILE"
chmod 0600 "$KEY_FILE"
ssh-keyscan -T 10 -p "$SSH_PORT" "$MONGO_BACKUP_HOST" 2>/dev/null >"$KNOWN_HOSTS"
actual_fingerprints="$(ssh-keygen -lf "$KNOWN_HOSTS" -E sha256 | awk '{ print $2 }')"
if ! grep -Fqx -- "$MONGO_BACKUP_SSH_FINGERPRINT" <<<"$actual_fingerprints"; then
  echo "serverC SSH host key fingerprint mismatch" >&2
  exit 1
fi

if nc -z 127.0.0.1 "$TUNNEL_PORT" >/dev/null 2>&1; then
  echo "MongoDB backup tunnel port is already in use: $TUNNEL_PORT" >&2
  exit 1
fi
ssh \
  -i "$KEY_FILE" \
  -p "$SSH_PORT" \
  -o BatchMode=yes \
  -o IdentitiesOnly=yes \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=30 \
  -o ServerAliveCountMax=3 \
  -o StrictHostKeyChecking=yes \
  -o "UserKnownHostsFile=$KNOWN_HOSTS" \
  -N -L "127.0.0.1:$TUNNEL_PORT:127.0.0.1:$REMOTE_MONGO_PORT" \
  "$MONGO_BACKUP_USERNAME@$MONGO_BACKUP_HOST" &
TUNNEL_PID=$!

tunnel_ready=false
for _ in {1..20}; do
  if ! kill -0 "$TUNNEL_PID" >/dev/null 2>&1; then
    wait "$TUNNEL_PID"
  fi
  if nc -z 127.0.0.1 "$TUNNEL_PORT" >/dev/null 2>&1; then
    tunnel_ready=true
    break
  fi
  sleep 1
done
if [ "$tunnel_ready" != true ]; then
  echo "MongoDB SSH tunnel did not become ready" >&2
  exit 1
fi

# MongoDB Database Tools accepts sensitive options from a permission-restricted
# YAML config file, keeping the password out of process arguments and logs.
escaped_password="${MONGODB_PASSWORD//\'/\'\'}"
{
  printf "password: '%s'\n" "$escaped_password"
  printf "uri: 'mongodb://127.0.0.1:%s/?connectTimeoutMS=30000&socketTimeoutMS=0'\n" "$TUNNEL_PORT"
} >"$MONGO_CONFIG"
chmod 0600 "$MONGO_CONFIG"
COMMON_ARGS=(
  --config="$MONGO_CONFIG"
  --username="$MONGODB_USERNAME"
  --authenticationDatabase=admin
)

validate_archive() {
  local archive_file="$1"
  local checksum_file="$archive_file.sha256"
  if [ ! -s "$archive_file" ]; then
    echo "MongoDB backup archive is missing or empty: $(basename "$archive_file")" >&2
    return 1
  fi
  if [ ! -s "$checksum_file" ]; then
    echo "MongoDB backup checksum is missing: $(basename "$checksum_file")" >&2
    return 1
  fi
  local expected_checksum actual_checksum
  expected_checksum="$(tr -d '[:space:]' <"$checksum_file")"
  actual_checksum="$(shasum -a 256 "$archive_file" | awk '{ print $1 }')"
  if ! [[ "$expected_checksum" =~ ^[0-9a-f]{64}$ ]] || [ "$actual_checksum" != "$expected_checksum" ]; then
    echo "MongoDB backup checksum mismatch: $(basename "$archive_file")" >&2
    return 1
  fi
  mongorestore \
    "${COMMON_ARGS[@]}" \
    --archive="$archive_file" \
    --gzip \
    --nsInclude="$MONGODB_DBNAME.*" \
    --dryRun \
    --verbose
}

case "$OPERATION" in
  backup)
    BACKUP_FILE="qs_mongodb_backup_$(TZ=Asia/Shanghai date +%Y%m%d_%H%M%S).archive.gz"
    FINAL_FILE="$BACKUP_DIR/$BACKUP_FILE"
    PARTIAL_FILE="$FINAL_FILE.partial"
    CHECKSUM_PARTIAL="$PARTIAL_FILE.sha256"
    if [ -e "$FINAL_FILE" ] || [ -e "$PARTIAL_FILE" ] || [ -e "$CHECKSUM_PARTIAL" ]; then
      echo "refusing to overwrite existing Mac mini MongoDB backup staging" >&2
      exit 1
    fi

    latest_archive="$(find "$BACKUP_DIR" -maxdepth 1 -type f -name 'qs_mongodb_backup_*.archive.gz' -print | LC_ALL=C sort -r | head -1)"
    required_free_bytes=1073741824
    if [ -n "$latest_archive" ]; then
      latest_size="$(stat -f %z "$latest_archive")"
      if [[ "$latest_size" =~ ^[0-9]+$ ]] && (( latest_size * 2 > required_free_bytes )); then
        required_free_bytes=$(( latest_size * 2 ))
      fi
    fi
    available_kib="$(df -Pk "$BACKUP_DIR" | awk 'NR == 2 { print $4 }')"
    available_bytes=$(( available_kib * 1024 ))
    if (( available_bytes < required_free_bytes )); then
      echo "insufficient Mac mini backup space: available=$available_bytes required=$required_free_bytes" >&2
      exit 1
    fi

    DUMP_LOG="$TEMP_DIR/mongodump.log"
    dump_attempt=1
    while true; do
      : >"$DUMP_LOG"
      if mongodump \
        "${COMMON_ARGS[@]}" \
        --db="$MONGODB_DBNAME" \
        --numParallelCollections=4 \
        --gzip \
        --archive="$PARTIAL_FILE" 2>&1 | tee "$DUMP_LOG"; then
        break
      fi
      if (( dump_attempt >= 2 )) || ! grep -Fq '(CursorNotFound)' "$DUMP_LOG"; then
        exit 1
      fi
      echo "MongoDB dump cursor was lost; removing incomplete archive and retrying once"
      rm -f -- "$PARTIAL_FILE"
      dump_attempt=$(( dump_attempt + 1 ))
    done
    test -s "$PARTIAL_FILE"
    archive_checksum="$(shasum -a 256 "$PARTIAL_FILE" | awk '{ print $1 }')"
    printf '%s\n' "$archive_checksum" >"$CHECKSUM_PARTIAL"
    chmod 0640 "$PARTIAL_FILE" "$CHECKSUM_PARTIAL"

    # Validate the partial archive before publishing it as a restorable backup.
    validate_archive "$PARTIAL_FILE"
    mv -- "$PARTIAL_FILE" "$FINAL_FILE"
    mv -- "$CHECKSUM_PARTIAL" "$FINAL_FILE.sha256"
    PARTIAL_FILE=''
    CHECKSUM_PARTIAL=''

    backup_files=()
    while IFS= read -r backup_path; do
      backup_files+=("$backup_path")
    done < <(find "$BACKUP_DIR" -maxdepth 1 -type f -name 'qs_mongodb_backup_*.archive.gz' -print | LC_ALL=C sort -r)
    if (( ${#backup_files[@]} > RETENTION_COUNT )); then
      for old_backup in "${backup_files[@]:RETENTION_COUNT}"; do
        rm -f -- "$old_backup" "$old_backup.sha256"
        echo "Removed expired Mac mini MongoDB backup: $(basename "$old_backup")"
      done
    fi
    echo "MongoDB backup completed on Mac mini: file=$BACKUP_FILE bytes=$(stat -f %z "$FINAL_FILE") checksum=$archive_checksum retained=$RETENTION_COUNT"
    if [ -n "${GITHUB_OUTPUT:-}" ]; then
      echo "backup_file=$BACKUP_FILE" >>"$GITHUB_OUTPUT"
    fi
    ;;
  validate)
    validate_archive "$BACKUP_DIR/$BACKUP_FILE_INPUT"
    echo "MongoDB backup validation succeeded on Mac mini: file=$BACKUP_FILE_INPUT"
    ;;
  restore)
    validate_archive "$BACKUP_DIR/$BACKUP_FILE_INPUT"
    echo "WARNING: restoring $BACKUP_FILE_INPUT into $MONGODB_DBNAME with --drop"
    sleep 5
    mongorestore \
      "${COMMON_ARGS[@]}" \
      --archive="$BACKUP_DIR/$BACKUP_FILE_INPUT" \
      --gzip \
      --drop \
      --nsInclude="$MONGODB_DBNAME.*"
    echo "MongoDB restore from Mac mini succeeded: file=$BACKUP_FILE_INPUT"
    ;;
esac
