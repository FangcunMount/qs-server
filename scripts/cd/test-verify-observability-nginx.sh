#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

FAKE_BIN="$TEST_ROOT/bin"
FAKE_RUNNER="$FAKE_BIN/fake-privileged"
FAKE_CURL="$FAKE_BIN/curl"
WORKER_READY_ATTEMPT_FILE="$TEST_ROOT/worker-ready-attempt"
mkdir -p "$FAKE_BIN"

cat >"$FAKE_RUNNER" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

[ "${1:-}" = docker ] || exit 64
shift

case "${1:-}" in
  inspect)
    printf '%s\n' true
    ;;
  exec)
    shift
    [ "${1:-}" = nginx ] || exit 65
    shift
    case "${1:-}" in
      nginx)
        if [ "${2:-}" = -T ]; then
          cat <<'CONFIG'
server_name worker.fangcunmount.cn;
server_name nsqd.fangcunmount.cn;
proxy_pass http://prometheus:9090;
proxy_pass http://nsqd:4151;
proxy_pass http://nsqlookupd:4161;
location = /stats {}
location = /nodes {}
CONFIG
        fi
        ;;
      getent)
        printf '%s\n' '172.20.0.10 STREAM upstream'
        ;;
      *)
        exit 66
        ;;
    esac
    ;;
  *)
    exit 67
    ;;
esac
EOF

cat >"$FAKE_CURL" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

url="${@: -1}"
case "$url" in
  */metrics)
    printf '%s\n' 'qs_runtime_component_ready 1'
    ;;
  *collect.fangcunmount.cn*/readyz)
    printf '%s\n' '{"data":{"result":[{"value":[1,"2"]}]}}'
    ;;
  *worker.fangcunmount.cn*/readyz)
    attempt=0
    if [ -r "$WORKER_READY_ATTEMPT_FILE" ]; then
      attempt=$(cat "$WORKER_READY_ATTEMPT_FILE")
    fi
    attempt=$((attempt + 1))
    printf '%s\n' "$attempt" >"$WORKER_READY_ATTEMPT_FILE"
    ready=2
    if [ "${FAKE_ALWAYS_STALE:-0}" != 1 ] && [ "$attempt" -ge 3 ]; then
      ready=3
    fi
    printf '{"data":{"result":[{"value":[1,"%s"]}]}}\n' "$ready"
    ;;
  *nsqd.fangcunmount.cn*/stats)
    printf '%s\n' '{"topics":[]}'
    ;;
  *)
    exit 68
    ;;
esac
EOF
chmod 0755 "$FAKE_RUNNER" "$FAKE_CURL"

run_verifier() {
  env \
    PATH="$FAKE_BIN:$PATH" \
    PRIVILEGE_RUNNER="$FAKE_RUNNER" \
    WORKER_READY_ATTEMPT_FILE="$WORKER_READY_ATTEMPT_FILE" \
    FAKE_ALWAYS_STALE="${FAKE_ALWAYS_STALE:-0}" \
    EXPECTED_COLLECTION_REPLICAS=2 \
    EXPECTED_WORKER_REPLICAS=3 \
    READY_PROBE_ATTEMPTS="${READY_PROBE_ATTEMPTS:-3}" \
    READY_PROBE_INTERVAL_SECONDS=0 \
    "$SCRIPT_DIR/verify-observability-nginx.sh" verify-only
}

success_output="$(run_verifier 2>&1)"
grep -Fq 'reported ready replicas=2, expected=3' <<<"$success_output"
grep -Fq 'converged after 3/3 attempts' <<<"$success_output"
grep -Fq 'Public K6 observability route probes passed' <<<"$success_output"

rm -f "$WORKER_READY_ATTEMPT_FILE"
if FAKE_ALWAYS_STALE=1 READY_PROBE_ATTEMPTS=2 run_verifier >"$TEST_ROOT/failure.log" 2>&1; then
  echo "observability convergence gate unexpectedly accepted a stale replica count" >&2
  exit 1
fi
grep -Fq 'did not converge after 2 attempts' "$TEST_ROOT/failure.log"

echo '[OK] public observability gate retries bounded replica convergence and fails closed'
