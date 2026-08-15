#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

FAKE_RUNNER="$TEST_ROOT/fake-privileged"
ENV_FILE="$TEST_ROOT/worker.env"
CALL_LOG="$TEST_ROOT/calls.log"

cat >"$ENV_FILE" <<'EOF'
QS_WORKER_MYSQL_HOST=172.17.0.227:3306
QS_WORKER_MYSQL_USERNAME=worker
QS_WORKER_MYSQL_PASSWORD=must-not-appear-in-output
EOF

cat >"$FAKE_RUNNER" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf '%s\n' "$*" >>"$FAKE_CALL_LOG"
case "${1:-}" in
  test)
    exit 0
    ;;
  sed)
    shift
    command sed "$@"
    ;;
  docker)
    if [ "${FAKE_DEPENDENCY_REACHABLE:-1}" = 1 ]; then
      exit 0
    fi
    exit 1
    ;;
  *)
    exit 64
    ;;
esac
EOF
chmod 0755 "$FAKE_RUNNER"

run_verifier() {
  env \
    PRIVILEGE_RUNNER="$FAKE_RUNNER" \
    FAKE_CALL_LOG="$CALL_LOG" \
    FAKE_DEPENDENCY_REACHABLE="${FAKE_DEPENDENCY_REACHABLE:-1}" \
    WORKER_ENV_FILE="$ENV_FILE" \
    WORKER_IMAGE_REF=ghcr.io/fangcunmount/qs-worker:new-sha \
    WORKER_DEPENDENCY_TIMEOUT_SECONDS=2 \
    "$SCRIPT_DIR/verify-worker-dependencies.sh" verify
}

success_output="$(run_verifier)"
grep -Fq 'preflight passed: QS_WORKER_MYSQL_HOST 172.17.0.227:3306' <<<"$success_output"
grep -Fq 'docker run --rm --network infra-network --entrypoint /bin/sh ghcr.io/fangcunmount/qs-worker:new-sha' "$CALL_LOG"
grep -Fq 'sleep "$3"' "$CALL_LOG"
grep -Fq 'nc -w "$3" "$1" "$2"' "$CALL_LOG"
grep -Fq 'od -An -tu1 -N5' "$CALL_LOG"
grep -Fq 'test "$protocol_byte" = 10' "$CALL_LOG"
if grep -Fq 'must-not-appear-in-output' <<<"$success_output"; then
  echo 'worker dependency preflight leaked a credential' >&2
  exit 1
fi

if FAKE_DEPENDENCY_REACHABLE=0 run_verifier >"$TEST_ROOT/failure.log" 2>&1; then
  echo 'worker dependency preflight unexpectedly accepted an unreachable MySQL endpoint' >&2
  exit 1
fi
grep -Fq 'QS_WORKER_MYSQL_HOST 172.17.0.227:3306 did not return a MySQL greeting' "$TEST_ROOT/failure.log"
if grep -Fq 'must-not-appear-in-output' "$TEST_ROOT/failure.log"; then
  echo 'worker dependency failure leaked a credential' >&2
  exit 1
fi

worker_job="$TEST_ROOT/worker-job.yml"
sed -n '/^  deploy-worker:/,/^  verify-worker-governance:/p' \
  "$SCRIPT_DIR/../../.github/workflows/cd.yml" >"$worker_job"
grep -Fq 'MYSQL_HOST: mysql-rds-proxy' "$worker_job"
grep -Fq 'MYSQL_PORT: 3306' "$worker_job"

worker_deploy="$TEST_ROOT/worker-deploy.sh"
sed -n '/^deploy_worker()/,/^echo "=========================================="/p' \
  "$SCRIPT_DIR/remote-deploy.sh" >"$worker_deploy"
grep -Fq 'WORKER_ENV_FILE="$DEPLOY_TMP/configs/env/config.prod.env"' "$worker_deploy"
if grep -Fq 'WORKER_ENV_FILE="$COMPOSE_ENV_FILE"' "$worker_deploy"; then
  echo 'worker dependency preflight reads the image-only Compose environment' >&2
  exit 1
fi

echo '[OK] worker dependency preflight fails before replacement and does not expose credentials'
