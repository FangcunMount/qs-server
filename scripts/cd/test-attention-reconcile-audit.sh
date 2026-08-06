#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

FAKE_DOCKER="$TEST_ROOT/docker"
cat >"$FAKE_DOCKER" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

case "${1:-}" in
  ps)
    printf 'worker-1\nworker-2\nworker-3\n'
    ;;
  inspect)
    printf 'ghcr.io/fangcunmount/qs-worker:%s\n' "${EXPECTED_DEPLOY_SHA}"
    ;;
  exec)
    container_id="$2"
    command_name="$3"
    if [ "$command_name" = "grep" ]; then
      exit 0
    fi
    if [ "$command_name" = "awk" ]; then
      printf '%s\n' "${FAKE_TARGET_COUNT:-91}"
      exit 0
    fi
    if [ "$command_name" != "wget" ]; then
      echo "unexpected fake docker exec command: $command_name" >&2
      exit 2
    fi
    printf '# HELP attention_fact_reconcile_consecutive_failures Consecutive failures.\n'
    printf '# TYPE attention_fact_reconcile_consecutive_failures gauge\n'
    printf 'attention_fact_reconcile_consecutive_failures 0\n'
    if [ "$container_id" = "worker-1" ]; then
      printf 'attention_fact_reconcile_rounds_total{dry_run="%s",result="success"} 1\n' "${FAKE_DRY_RUN:-true}"
      printf 'attention_fact_reconcile_total{dry_run="%s",result="created"} %s\n' "${FAKE_DRY_RUN:-true}" "${FAKE_CREATED:-0}"
      printf 'attention_fact_reconcile_total{dry_run="%s",result="mismatched"} %s\n' "${FAKE_DRY_RUN:-true}" "${FAKE_MISMATCHED:-0}"
      printf 'attention_fact_reconcile_missing{dry_run="%s"} 33\n' "${FAKE_DRY_RUN:-true}"
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
  EXPECTED_WORKER_REPLICAS=3
  EXPECTED_DEPLOY_SHA=0123456789abcdef0123456789abcdef01234567
  MIN_SUCCESSFUL_ROUNDS=1
  MIN_MISSING=1
  EXPECTED_MISSING=33
  EXPECTED_TARGET_COUNT=91
  EXPECTED_TARGET_FINGERPRINT=75bc40d269404a337ccd3fabae57fec2768424fc660e1e9b76796bb3a3404a09
  PRIVILEGE_RUNNER=
  DOCKER_BIN="$FAKE_DOCKER"
)

env "${COMMON_ENV[@]}" "$SCRIPT_DIR/audit-attention-reconcile-dry-run.sh" >/dev/null

if env "${COMMON_ENV[@]}" FAKE_CREATED=1 \
  "$SCRIPT_DIR/audit-attention-reconcile-dry-run.sh" >/dev/null 2>&1; then
  echo "audit accepted a dry-run that reported created records" >&2
  exit 1
fi

if env "${COMMON_ENV[@]}" FAKE_TARGET_COUNT=90 \
  "$SCRIPT_DIR/audit-attention-reconcile-dry-run.sh" >/dev/null 2>&1; then
  echo "audit accepted the wrong target report count" >&2
  exit 1
fi

env "${COMMON_ENV[@]}" EXPECTED_DRY_RUN=false EXPECTED_CREATED=33 \
  FAKE_DRY_RUN=false FAKE_CREATED=33 \
  "$SCRIPT_DIR/audit-attention-reconcile-dry-run.sh" >/dev/null

if env "${COMMON_ENV[@]}" EXPECTED_DRY_RUN=false EXPECTED_CREATED=33 \
  FAKE_DRY_RUN=false FAKE_CREATED=33 FAKE_MISMATCHED=1 \
  "$SCRIPT_DIR/audit-attention-reconcile-dry-run.sh" >/dev/null 2>&1; then
  echo "audit accepted an apply run with mismatched projections" >&2
  exit 1
fi

echo "attention reconcile dry-run/apply audit contract passed"
