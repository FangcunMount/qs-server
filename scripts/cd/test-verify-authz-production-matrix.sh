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
    if [[ "$*" != *"/app/${FAKE_AUTHZ_COMMAND:-qs-authz verify} --config=/app/configs/apiserver.prod.yaml"* ]]; then
      echo "unexpected matrix command: $*" >&2
      exit 2
    fi
    printf '%s\n' "${FAKE_MATRIX_EVIDENCE:?}"
    ;;
  *)
    echo "unexpected fake docker command: $*" >&2
    exit 2
    ;;
esac
EOF
chmod +x "$FAKE_DOCKER"

evidence="$(python3 - <<'PYFIXTURE'
import json
subjects=[{"kind":k,"source":"production_operator" if k in ("admin","other") else "synthetic_iam_user","subject_fingerprint":str(i)*16} for i,k in enumerate(("admin","operator","plan_manager","other"),1)]
cases=[{"kind":k,"scenario":a,"action":a,"expected_allowed":ok,"allowed":ok,"policy_version":27,"passed":True} for k,a,ok in [("admin","retry",True),("operator","retry",True),("plan_manager","retry",True),("other","retry",False),("operator","force_retry",False),("admin","force_retry",True)]]
print(json.dumps({"schema_version":"iam-authz-production-matrix/v3","git_commit":"0123456789abcdef0123456789abcdef01234567","service_identity":"qs-apiserver.svc","policy_version":27,"subjects":subjects,"cases":cases,"passed":True},separators=(",",":")))
PYFIXTURE
)"

output="$(PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" FAKE_MATRIX_EVIDENCE="$evidence" \
  "$SCRIPT_DIR/verify-authz-production-matrix.sh")"
printf '%s\n' "$output" | grep -Fq 'Production AuthZ action matrix passed: subjects=4 cases=6 synthetic_subjects=2 policy_version=27'

# Retained rollback images use the original entrypoint only when it is absent.
output="$(FAKE_AUTHZ_PROBE_STATUS=42 FAKE_AUTHZ_COMMAND=qs-authz-matrix PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" FAKE_MATRIX_EVIDENCE="$evidence" \
  "$SCRIPT_DIR/verify-authz-production-matrix.sh")"
printf '%s\n' "$output" | grep -Fq 'Production AuthZ action matrix passed: subjects=4 cases=6 synthetic_subjects=2 policy_version=27'

bad_evidence="${evidence/\"passed\":true}/\"passed\":false}"
if PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" FAKE_MATRIX_EVIDENCE="$bad_evidence" \
  "$SCRIPT_DIR/verify-authz-production-matrix.sh" >/dev/null 2>&1; then
  echo "matrix verifier accepted failing evidence" >&2
  exit 1
fi

if FAKE_AUTHZ_PROBE_STATUS=125 FAKE_AUTHZ_OPERATIONS="$TEST_ROOT/probe-error-operations" PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" \
  "$SCRIPT_DIR/verify-authz-production-matrix.sh" >/dev/null 2>&1; then
  echo "AuthZ command probe error was accepted" >&2
  exit 1
fi

test ! -e "$TEST_ROOT/probe-error-operations"

# Generic exec/privilege status 1 is not the explicit missing-entrypoint signal.
probe_status=0
FAKE_AUTHZ_PROBE_STATUS=1 FAKE_AUTHZ_OPERATIONS="$TEST_ROOT/generic-probe-error-operations" PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" \
  "$SCRIPT_DIR/verify-authz-production-matrix.sh" >/dev/null 2>&1 || probe_status=$?
test "$probe_status" -eq 1
test ! -e "$TEST_ROOT/generic-probe-error-operations"

# Execution failure is returned directly, without retrying the legacy entrypoint.
command_status=0
FAKE_AUTHZ_COMMAND_STATUS=42 FAKE_AUTHZ_OPERATIONS="$TEST_ROOT/failed-operations" PRIVILEGE_RUNNER='' DOCKER_BIN="$FAKE_DOCKER" \
  "$SCRIPT_DIR/verify-authz-production-matrix.sh" >/dev/null 2>&1 || command_status=$?
test "$command_status" -eq 42
test "$(wc -l <"$TEST_ROOT/failed-operations" | tr -d ' ')" -eq 1

echo "production AuthZ matrix verification contract passed"
