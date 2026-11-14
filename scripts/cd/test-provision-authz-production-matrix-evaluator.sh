#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

FAKE_DOCKER="$TEST_ROOT/docker"
cat >"$FAKE_DOCKER" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
container_id="$(printf '%064d' 1)"
image_id="sha256:$(printf '%064d' 2)"
revision=0123456789abcdef0123456789abcdef01234567
case "${1:-}" in
  ps)
    printf '%s\n' "${FAKE_RUNNING_NAMES-qs-apiserver}"
    ;;
  inspect)
    name="$2"
    image_ref="ghcr.io/fangcunmount/qs-apiserver:$revision"
    project=qs
    service=apiserver
    if [[ "$name" == qs-m5-* ]]; then
      image_ref="$image_id"
      project="${name%-*}"
      service="apiserver-${name##*-}"
    fi
    started=2026-10-31T19:48:01Z
    if [[ -n "${FAKE_DRIFT_MARKER:-}" && -e "$FAKE_DRIFT_MARKER" ]]; then
      started=2026-11-01T19:48:01Z
    fi
    printf '/%s|%s|%s|%s|true|%s|0|%s|%s|%s\n' "$name" "$container_id" "$image_id" "$image_ref" \
      "$started" "${FAKE_HEALTH-healthy}" "$project" "$service"
    ;;
  image)
    [[ "$2" == inspect && "$3" == "$image_id" ]]
    printf '%s|%s\n' "$image_id" "${FAKE_REVISION-$revision}"
    ;;
  exec)
    [[ "$*" == *"$container_id"* ]]
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
    if [[ -n "${FAKE_DRIFT_MARKER:-}" ]]; then
      touch "$FAKE_DRIFT_MARKER"
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

evidence='{"schema_version":"iam-authz-matrix-provision/v2","provisioned_at":"2026-08-26T00:00:00Z","git_commit":"0123456789abcdef0123456789abcdef01234567","service_identity":"qs-apiserver.svc","subjects":[{"schema_version":"iam-authz-matrix-subject/v2","nickname":"__qs_authz_matrix_operator_v2__","subject_fingerprint":"2222222222222222","role":"qs:assessment_operator","user_created":true,"assignment_created":true,"policy_version":28,"passed":true},{"schema_version":"iam-authz-matrix-subject/v2","nickname":"__qs_authz_matrix_plan_manager_v2__","subject_fingerprint":"3333333333333333","role":"qs:evaluation_plan_manager","user_created":true,"assignment_created":true,"policy_version":29,"passed":true}],"passed":true}'

output="$(PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" FAKE_PROVISION_EVIDENCE="$evidence" \
  QS_AUTHZ_MATRIX_PROVISION_CONFIRM=provision-isolated-authz-matrix-subjects-v2 \
  "$SCRIPT_DIR/provision-authz-production-matrix-evaluator.sh")"
printf '%s\n' "$output" | grep -Fq 'Production AuthZ matrix subjects are ready: subjects=2 policy_version=29'

m5=qs-m5-0123456789ab-apiserver-1
output="$(FAKE_RUNNING_NAMES="$m5" PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" FAKE_PROVISION_EVIDENCE="$evidence" \
  QS_AUTHZ_MATRIX_PROVISION_CONFIRM=provision-isolated-authz-matrix-subjects-v2 \
  "$SCRIPT_DIR/provision-authz-production-matrix-evaluator.sh")"
printf '%s\n' "$output" | grep -Fq 'Production AuthZ matrix subjects are ready'

for evidence_input in "${evidence/operator_v2/evaluator_v2}" \
  "$(python3 -c 'import json,sys; v=json.loads(sys.argv[1]); v["subjects"].append(v["subjects"][0]); print(json.dumps(v, separators=(",", ":")))' "$evidence")"; do
  if PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" FAKE_PROVISION_EVIDENCE="$evidence_input" \
    QS_AUTHZ_MATRIX_PROVISION_CONFIRM=provision-isolated-authz-matrix-subjects-v2 \
      "$SCRIPT_DIR/provision-authz-production-matrix-evaluator.sh" >/dev/null 2>&1; then
    echo "current provisioner accepted unexpected identities" >&2
    exit 1
  fi
done

for invalid in "FAKE_RUNNING_NAMES=qs-apiserver"$'\n'"$m5" 'FAKE_HEALTH=unhealthy' 'FAKE_REVISION='; do
  if env FAKE_RUNNING_NAMES="$m5" "$invalid" FAKE_AUTHZ_OPERATIONS="$TEST_ROOT/invalid-operations" \
    PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" FAKE_PROVISION_EVIDENCE="$evidence" \
    QS_AUTHZ_MATRIX_PROVISION_CONFIRM=provision-isolated-authz-matrix-subjects-v2 \
      "$SCRIPT_DIR/provision-authz-production-matrix-evaluator.sh" >/dev/null 2>&1; then
    echo "unexpected M5 runtime accepted" >&2
    exit 1
  fi
done
test ! -e "$TEST_ROOT/invalid-operations"

if FAKE_RUNNING_NAMES="$m5" FAKE_DRIFT_MARKER="$TEST_ROOT/drift" FAKE_AUTHZ_OPERATIONS="$TEST_ROOT/drift-operations" \
    PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" FAKE_PROVISION_EVIDENCE="$evidence" \
    QS_AUTHZ_MATRIX_PROVISION_CONFIRM=provision-isolated-authz-matrix-subjects-v2 \
      "$SCRIPT_DIR/provision-authz-production-matrix-evaluator.sh" >/dev/null 2>&1; then
  echo "identity drift accepted" >&2
  exit 1
fi
test "$(wc -l <"$TEST_ROOT/drift-operations" | tr -d ' ')" -eq 1

# Retained rollback images use the original entrypoint only when it is absent.
output="$(FAKE_AUTHZ_PROBE_STATUS=42 FAKE_AUTHZ_COMMAND=qs-authz-matrix-provision PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" FAKE_PROVISION_EVIDENCE="${evidence/operator_v2/evaluator_v2}" \
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
