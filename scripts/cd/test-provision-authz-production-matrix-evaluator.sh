#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

FAKE_DOCKER="$TEST_ROOT/docker"
cat >"$FAKE_DOCKER" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
case "${1:-}" in
  inspect)
    if [[ "$*" == *State.Running* ]]; then
      printf 'true\n'
    else
      printf 'ghcr.io/fangcunmount/qs-apiserver:0123456789abcdef0123456789abcdef01234567\n'
    fi
    ;;
  exec)
    if [[ "$*" == *"sh -c if test -x /app/qs-authz; then exit 0; else exit 42; fi"* ]]; then
      exit "${FAKE_AUTHZ_PROBE_STATUS:-0}"
    fi
    if [ -n "${FAKE_AUTHZ_OPERATIONS:-}" ]; then
      printf '%s\n' "$*" >>"$FAKE_AUTHZ_OPERATIONS"
    fi
    if [ "${FAKE_AUTHZ_COMMAND_STATUS:-0}" -ne 0 ]; then
      exit "$FAKE_AUTHZ_COMMAND_STATUS"
    fi
    if [[ "$*" != *'QS_AUTHZ_MATRIX_PROVISION_CONFIRM=provision-isolated-authz-matrix-subjects-v2'* ]] ||
       [[ "$*" != *"/app/${FAKE_AUTHZ_COMMAND:-qs-authz provision} --config=/app/configs/apiserver.prod.yaml"* ]]; then
      echo "unexpected provision command: $*" >&2
      exit 2
    fi
    printf '%s\n' "${FAKE_PROVISION_EVIDENCE:?}"
    ;;
  *)
    echo "unexpected fake docker command: $*" >&2
    exit 2
    ;;
esac
EOF
chmod +x "$FAKE_DOCKER"

evidence='{"schema_version":"iam-authz-matrix-provision/v2","provisioned_at":"2026-08-26T00:00:00Z","git_commit":"0123456789abcdef0123456789abcdef01234567","service_identity":"qs-apiserver.svc","subjects":[{"schema_version":"iam-authz-matrix-subject/v2","nickname":"__qs_authz_matrix_evaluator_v2__","subject_fingerprint":"2222222222222222","role":"qs:assessment_operator","user_created":true,"assignment_created":true,"policy_version":28,"passed":true},{"schema_version":"iam-authz-matrix-subject/v2","nickname":"__qs_authz_matrix_plan_manager_v2__","subject_fingerprint":"3333333333333333","role":"qs:evaluation_plan_manager","user_created":true,"assignment_created":true,"policy_version":29,"passed":true}],"passed":true}'

output="$(PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" FAKE_PROVISION_EVIDENCE="$evidence" \
  QS_AUTHZ_MATRIX_PROVISION_CONFIRM=provision-isolated-authz-matrix-subjects-v2 \
  "$SCRIPT_DIR/provision-authz-production-matrix-evaluator.sh")"
printf '%s\n' "$output" | grep -Fq 'Production AuthZ matrix subjects are ready: subjects=2 policy_version=29'

# Retained rollback images use the original entrypoint only when it is absent.
output="$(FAKE_AUTHZ_PROBE_STATUS=42 FAKE_AUTHZ_COMMAND=qs-authz-matrix-provision PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" FAKE_PROVISION_EVIDENCE="$evidence" \
  QS_AUTHZ_MATRIX_PROVISION_CONFIRM=provision-isolated-authz-matrix-subjects-v2 \
  "$SCRIPT_DIR/provision-authz-production-matrix-evaluator.sh")"
printf '%s\n' "$output" | grep -Fq 'Production AuthZ matrix subjects are ready: subjects=2 policy_version=29'

if PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" FAKE_PROVISION_EVIDENCE="$evidence" \
  QS_AUTHZ_MATRIX_PROVISION_CONFIRM=wrong \
  "$SCRIPT_DIR/provision-authz-production-matrix-evaluator.sh" >/dev/null 2>&1; then
  echo "provisioner accepted an invalid confirmation" >&2
  exit 1
fi

if FAKE_AUTHZ_PROBE_STATUS=125 FAKE_AUTHZ_OPERATIONS="$TEST_ROOT/probe-error-operations" PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" QS_AUTHZ_MATRIX_PROVISION_CONFIRM=provision-isolated-authz-matrix-subjects-v2 \
  "$SCRIPT_DIR/provision-authz-production-matrix-evaluator.sh" >/dev/null 2>&1; then
  echo "AuthZ command probe error was accepted" >&2
  exit 1
fi

test ! -e "$TEST_ROOT/probe-error-operations"

# Generic exec/privilege status 1 is not the explicit missing-entrypoint signal.
probe_status=0
FAKE_AUTHZ_PROBE_STATUS=1 FAKE_AUTHZ_OPERATIONS="$TEST_ROOT/generic-probe-error-operations" PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" QS_AUTHZ_MATRIX_PROVISION_CONFIRM=provision-isolated-authz-matrix-subjects-v2 \
  "$SCRIPT_DIR/provision-authz-production-matrix-evaluator.sh" >/dev/null 2>&1 || probe_status=$?
test "$probe_status" -eq 1
test ! -e "$TEST_ROOT/generic-probe-error-operations"

# Execution failure is returned directly, without retrying the legacy entrypoint.
command_status=0
FAKE_AUTHZ_COMMAND_STATUS=42 FAKE_AUTHZ_OPERATIONS="$TEST_ROOT/failed-operations" PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" QS_AUTHZ_MATRIX_PROVISION_CONFIRM=provision-isolated-authz-matrix-subjects-v2 \
  "$SCRIPT_DIR/provision-authz-production-matrix-evaluator.sh" >/dev/null 2>&1 || command_status=$?
test "$command_status" -eq 42
test "$(wc -l <"$TEST_ROOT/failed-operations" | tr -d ' ')" -eq 1

echo "production AuthZ matrix subject provisioning contract passed"
