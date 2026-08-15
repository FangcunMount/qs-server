#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

FAKE_RUNNER="$TEST_ROOT/fake-privileged"
CALL_LOG="$TEST_ROOT/calls.log"
READY_MARKER="$TEST_ROOT/not-ready-once"
COMPOSE_FILE="$TEST_ROOT/docker-compose.yml"
ENV_FILE="$TEST_ROOT/worker.env"

touch "$COMPOSE_FILE" "$ENV_FILE" "$CALL_LOG"

cat >"$FAKE_RUNNER" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

printf '%s\n' "$*" >>"$FAKE_CALL_LOG"
[ "${1:-}" = docker ] || exit 64
shift

case "${1:-}" in
  compose)
    shift
    case " $* " in
      *" ps --status running -q runtime "*|*" ps -a -q runtime "*)
        printf '%s\n' worker-1 worker-2 worker-3
        ;;
      *" ps runtime "*)
        printf '%s\n' 'NAME STATUS' 'worker-1 Up' 'worker-2 Up' 'worker-3 Up'
        ;;
      *" logs --tail 200 runtime "*)
        printf '%s\n' 'runtime | [FATAL] Failed to prepare resources: connection timed out'
        ;;
      *)
        echo "unexpected fake docker compose call: $*" >&2
        exit 65
        ;;
    esac
    ;;
  inspect)
    container_id="${2:-}"
    if printf '%s\n' "$*" | grep -Fq '{{.Config.Image}}'; then
      printf 'ghcr.io/fangcunmount/qs-worker:%s\n' "$FAKE_IMAGE_TAG"
    else
      printf 'container=/%s status=running exit=0 restarts=0 image=ghcr.io/fangcunmount/qs-worker:%s error=\n' \
        "$container_id" "$FAKE_IMAGE_TAG"
    fi
    ;;
  exec)
    container_id="${2:-}"
    if [ "${FAKE_ALWAYS_NOT_READY:-0}" = 1 ]; then
      exit 1
    fi
    if [ "$container_id" = worker-2 ] && [ ! -f "$FAKE_READY_MARKER" ]; then
      touch "$FAKE_READY_MARKER"
      printf '%s\n' '{"status":"degraded"}'
      exit 0
    fi
    printf '%s\n' '{"status":"ready"}'
    ;;
  *)
    echo "unexpected fake docker call: $*" >&2
    exit 66
    ;;
esac
EOF
chmod 0755 "$FAKE_RUNNER"

run_gate() {
  env \
    PRIVILEGE_RUNNER="$FAKE_RUNNER" \
    FAKE_CALL_LOG="$CALL_LOG" \
    FAKE_READY_MARKER="$READY_MARKER" \
    FAKE_IMAGE_TAG="${FAKE_IMAGE_TAG:-new-sha}" \
    FAKE_ALWAYS_NOT_READY="${FAKE_ALWAYS_NOT_READY:-0}" \
    EXPECTED_WORKER_REPLICAS=3 \
    WORKER_COMPOSE_FILE="$COMPOSE_FILE" \
    WORKER_COMPOSE_ENV_FILE="$ENV_FILE" \
    WORKER_IMAGE_TAG=new-sha \
    WORKER_READY_ATTEMPTS="${WORKER_READY_ATTEMPTS:-2}" \
    WORKER_READY_INTERVAL_SECONDS=0 \
    WORKER_READY_TIMEOUT_SECONDS=1 \
    "$SCRIPT_DIR/wait-worker-readiness.sh" verify
}

success_output="$(run_gate)"
grep -Fq 'attempt 1/2: running=3/3 image=3/3 ready=2/3' <<<"$success_output"
grep -Fq 'gate passed: running=3/3 image=3/3 ready=3/3' <<<"$success_output"
for container_id in worker-1 worker-2 worker-3; do
  grep -Fq "docker exec ${container_id} wget" "$CALL_LOG"
done

rm -f "$READY_MARKER"
if FAKE_IMAGE_TAG=old-sha WORKER_READY_ATTEMPTS=1 run_gate >"$TEST_ROOT/failure.log" 2>&1; then
  echo "worker readiness gate unexpectedly accepted the wrong image tag" >&2
  exit 1
fi
grep -Fq 'running=3/3 image=0/3 ready=2/3' "$TEST_ROOT/failure.log"
grep -Fq 'Worker deployment gate failed after 1 attempts' "$TEST_ROOT/failure.log"

echo '[OK] worker deploy waits for every replica image and /readyz'
