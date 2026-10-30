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
QS_WORKER_MYSQL_TLS_MODE=VERIFY_IDENTITY
QS_WORKER_MYSQL_TLS_CA_FILE=/etc/qlume/mysql/ca-chain.pem
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
    if [ "${2:-}" = compose ]; then
      python3 - <<'PYMODEL'
import json, os
mount = dict(type='bind', source='/data/infra/ssl/mysql/ca-chain.pem',
             target='/etc/qlume/mysql/ca-chain.pem', read_only=True,
             bind=dict(create_host_path=False))
mode = os.environ.get('FAKE_CA_BIND', 'valid')
if mode == 'writable': mount['read_only'] = False
if mode == 'wrong-source': mount['source'] = '/wrong/ca.pem'
if mode == 'auto-create': mount['bind']['create_host_path'] = True
print(json.dumps(dict(services=dict(runtime=dict(volumes=[] if mode == 'missing' else [mount])))))
PYMODEL
      exit 0
    fi
    if [[ "$*" == *'--network none'* ]]; then
      [ "${FAKE_CA_READABLE:-1}" = 1 ]
      exit
    fi
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
    FAKE_CA_BIND="${FAKE_CA_BIND:-valid}" \
    FAKE_CA_READABLE="${FAKE_CA_READABLE:-1}" \
    WORKER_ENV_FILE="$ENV_FILE" \
    WORKER_COMPOSE_FILE="$TEST_ROOT/candidate.yml" \
    WORKER_COMPOSE_ENV_FILE="$TEST_ROOT/image.env" \
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
grep -Fq 'config --no-env-resolution --format json runtime' "$CALL_LOG"
grep -Fq 'docker run --rm --network none --read-only' "$CALL_LOG"
if grep -Fq 'must-not-appear-in-output' <<<"$success_output"; then
  echo 'worker dependency preflight leaked a credential' >&2
  exit 1
fi

for mode in missing writable wrong-source auto-create; do
  : > "$CALL_LOG"
  if FAKE_CA_BIND="$mode" run_verifier >"$TEST_ROOT/ca-${mode}.log" 2>&1; then
    echo "worker accepted an invalid CA bind: $mode" >&2
    exit 1
  fi
  grep -Fq 'Worker candidate MySQL CA mount invalid; no replacement' "$TEST_ROOT/ca-${mode}.log"
  if grep -Fq 'docker run' "$CALL_LOG"; then
    echo 'invalid CA bind reached a candidate container or network probe' >&2
    exit 1
  fi
done
: > "$CALL_LOG"
if FAKE_CA_READABLE=0 run_verifier >"$TEST_ROOT/ca-unreadable.log" 2>&1; then
  echo 'worker accepted an unreadable CA file' >&2
  exit 1
fi
grep -Fq 'Worker candidate MySQL CA unreadable; no replacement' "$TEST_ROOT/ca-unreadable.log"
if grep -Fq -- '--network infra-network' "$CALL_LOG"; then
  echo 'unreadable candidate CA reached the network probe' >&2
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
grep -Fq 'MYSQL_HOST: ${{ vars.MYSQL_PUBLIC_HOST }}' "$worker_job"
grep -Fq 'MYSQL_PORT: ${{ vars.MYSQL_PUBLIC_PORT }}' "$worker_job"

worker_deploy="$TEST_ROOT/worker-deploy.sh"
sed -n '/^deploy_worker()/,/^echo "=========================================="/p' \
  "$SCRIPT_DIR/remote-deploy.sh" >"$worker_deploy"
grep -Fq 'WORKER_ENV_FILE="$DEPLOY_TMP/configs/env/config.prod.env"' "$worker_deploy"
if grep -Fq 'WORKER_ENV_FILE="$COMPOSE_ENV_FILE"' "$worker_deploy"; then
  echo 'worker dependency preflight reads the image-only Compose environment' >&2
  exit 1
fi

echo '[OK] worker dependency preflight fails before replacement and does not expose credentials'
