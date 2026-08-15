#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

FAKE_RUNNER="$TEST_ROOT/fake-privileged"
FAIL_ONCE_MARKER="$TEST_ROOT/fail-once"

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
    [ "${1:-}" = qs-apiserver ] || exit 65
    shift
    case "${1:-}" in
      getent)
        printf '%s\n' \
          '172.20.0.128 STREAM qs-worker-governance' \
          '172.20.0.129 STREAM qs-worker-governance' \
          '172.20.0.130 STREAM qs-worker-governance'
        ;;
      wget)
        url="${@: -1}"
        if [ "${FAKE_ALWAYS_FAIL:-0}" = 1 ]; then
          exit 1
        fi
        if [[ "$url" == *172.20.0.128*/governance/redis ]] && [ ! -f "$FAKE_FAIL_ONCE_MARKER" ]; then
          touch "$FAKE_FAIL_ONCE_MARKER"
          exit 1
        fi

        ip="${url#http://}"
        ip="${ip%%:*}"
        instance_id="worker-${ip##*.}"
        case "$url" in
          */governance/redis)
            printf '{"instance_id":"%s","ready":true}\n' "$instance_id"
            ;;
          */governance/resilience)
            printf '{"instance_id":"%s","summary":{"ready":true},"capabilities":[{"name":"answersheet_processing","kind":"duplicate_suppression","strategy":"redis_lease","configured":true,"degraded":false,"ttl_seconds":300,"renewal_mode":"auto","renew_every_seconds":100},{"name":"attention_projection_reconcile","kind":"leader","strategy":"redis_lease","configured":true,"degraded":false,"ttl_seconds":1800,"renewal_mode":"auto","renew_every_seconds":600}]}\n' "$instance_id"
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
    ;;
  *)
    exit 68
    ;;
esac
EOF
chmod 0755 "$FAKE_RUNNER"

run_verifier() {
  env \
    PRIVILEGE_RUNNER="$FAKE_RUNNER" \
    FAKE_FAIL_ONCE_MARKER="$FAIL_ONCE_MARKER" \
    FAKE_ALWAYS_FAIL="${FAKE_ALWAYS_FAIL:-0}" \
    EXPECTED_WORKER_REPLICAS=3 \
    DNS_RETRY_ATTEMPTS=1 \
    DNS_RETRY_INTERVAL_SECONDS=0 \
    GOVERNANCE_RETRY_ATTEMPTS=2 \
    GOVERNANCE_RETRY_INTERVAL_SECONDS=0 \
    "$SCRIPT_DIR/verify-worker-governance.sh" verify
}

success_output="$(run_verifier)"
grep -Fq 'Worker governance reverse verification passed (3 endpoints)' <<<"$success_output"
for suffix in 128 129 130; do
  grep -Fq "172.20.0.${suffix} instance=worker-${suffix} ready" <<<"$success_output"
done

if FAKE_ALWAYS_FAIL=1 run_verifier >"$TEST_ROOT/failure.log" 2>&1; then
  echo "worker governance gate unexpectedly accepted unreachable endpoints" >&2
  exit 1
fi
grep -Fq 'did not become ready after 2 attempts' "$TEST_ROOT/failure.log"
grep -Fq 'Redis governance snapshot is unreachable' "$TEST_ROOT/failure.log"

echo '[OK] worker governance retries bounded endpoint convergence and fails closed'
