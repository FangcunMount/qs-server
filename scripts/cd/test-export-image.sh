#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

FAKE_BIN="$TEST_ROOT/bin"
FAKE_DOCKER="$FAKE_BIN/docker"
mkdir -p "$FAKE_BIN" "$TEST_ROOT/payload"
printf '{}\n' >"$TEST_ROOT/payload/manifest.json"

cat >"$FAKE_DOCKER" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

case "${1:-}" in
  pull)
    if [ "${FAKE_DOCKER_MODE:-serialize}" = "retry" ]; then
      if mkdir "$FAKE_DOCKER_ROOT/first-pull" 2>/dev/null; then
        exit 1
      fi
      exit 0
    fi
    if ! mkdir "$FAKE_DOCKER_ROOT/active-pull" 2>/dev/null; then
      touch "$FAKE_DOCKER_ROOT/concurrent-pull"
      exit 1
    fi
    sleep 1
    rmdir "$FAKE_DOCKER_ROOT/active-pull"
    ;;
  save)
    # The exporter must stream; writing a raw archive doubles peak disk use.
    if [ "$#" != 2 ]; then
      echo "docker save must write stdout" >&2
      exit 2
    fi
    if [ "${FAKE_DOCKER_MODE:-}" = corrupt ]; then
      printf 'not a tar archive'
      exit 0
    fi
    tar -cf - -C "$FAKE_DOCKER_ROOT/payload" manifest.json
    if [ "${FAKE_DOCKER_MODE:-}" = fail_after_tar ]; then
      exit 1
    fi
    ;;
  *)
    echo "unexpected fake docker command: $*" >&2
    exit 2
    ;;
esac
EOF
chmod +x "$FAKE_DOCKER"

COMMON_ENV=(
  PATH="$FAKE_BIN:$PATH"
  DOCKER_REGISTRY=ghcr.io
  DOCKER_REPOSITORY=fangcunmount
  DEPLOY_SHA=0123456789abcdef0123456789abcdef01234567
  EXPORT_IMAGE_REGISTRY=ghcr
  CD_DOCKER_EXPORT_LOCK_DIR="$TEST_ROOT/export.lock"
  CD_DOCKER_EXPORT_LOCK_WAIT_SECONDS=10
  CD_DOCKER_EXPORT_LOCK_POLL_SECONDS=1
  DOCKER_PULL_ATTEMPTS=2
  DOCKER_PULL_RETRY_DELAY_SECONDS=0
  FAKE_DOCKER_ROOT="$TEST_ROOT"
)

env "${COMMON_ENV[@]}" FAKE_DOCKER_MODE=serialize SERVICE=apiserver \
  DEPLOY_IMAGE_PACKAGE="$TEST_ROOT/apiserver.tar.gz" "$SCRIPT_DIR/export-image.sh" >/dev/null &
first_pid=$!
env "${COMMON_ENV[@]}" FAKE_DOCKER_MODE=serialize SERVICE=worker \
  DEPLOY_IMAGE_PACKAGE="$TEST_ROOT/worker.tar.gz" "$SCRIPT_DIR/export-image.sh" >/dev/null &
second_pid=$!
wait "$first_pid"
wait "$second_pid"

if [ -e "$TEST_ROOT/concurrent-pull" ]; then
  echo "shared Docker export lock allowed concurrent pulls" >&2
  exit 1
fi
gzip -t "$TEST_ROOT/apiserver.tar.gz"
gzip -t "$TEST_ROOT/worker.tar.gz"

env "${COMMON_ENV[@]}" FAKE_DOCKER_MODE=retry SERVICE=collection \
  DEPLOY_IMAGE_PACKAGE="$TEST_ROOT/collection.tar.gz" "$SCRIPT_DIR/export-image.sh" >/dev/null
gzip -t "$TEST_ROOT/collection.tar.gz"

echo "deployment image export serialization and retry contract passed"

for mode in fail_after_tar corrupt; do
  output="$TEST_ROOT/failed-${mode}.tar.gz"
  if env "${COMMON_ENV[@]}" FAKE_DOCKER_MODE="$mode" SERVICE=apiserver \
    DEPLOY_IMAGE_PACKAGE="$output" "$SCRIPT_DIR/export-image.sh" >/dev/null 2>&1; then
    echo "invalid export accepted: $mode" >&2
    exit 1
  fi
  [ ! -e "$output" ]
  [ ! -d "$TEST_ROOT/export.lock" ]
  if compgen -G "${output}.tmp.*" >/dev/null; then
    echo "partial export leaked: $mode" >&2
    exit 1
  fi
done

echo "streaming export failure and integrity contract passed"

cat >"$FAKE_BIN/gzip" <<'GZIP'
#!/usr/bin/env bash
printf 'partial compressed bytes'
exit 7
GZIP
chmod +x "$FAKE_BIN/gzip"
output="$TEST_ROOT/compression-failed.tar.gz"
printf 'previous complete artifact' >"$output"
if env "${COMMON_ENV[@]}" FAKE_DOCKER_MODE=serialize SERVICE=apiserver \
  DEPLOY_IMAGE_PACKAGE="$output" "$SCRIPT_DIR/export-image.sh" >/dev/null 2>&1; then
  echo "compression failure accepted" >&2
  exit 1
fi
[ "$(cat "$output")" = 'previous complete artifact' ]
[ ! -d "$TEST_ROOT/export.lock" ]
if compgen -G "${output}.tmp.*" >/dev/null; then
  echo "failed compressed export leaked" >&2
  exit 1
fi
rm "$FAKE_BIN/gzip"
echo "compression failure preserves prior artifact and releases lock"
