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
  inspect)
    if [[ "$*" == *State.Running* ]]; then
      printf 'true\n'
    else
      printf 'ghcr.io/fangcunmount/qs-apiserver:0123456789abcdef0123456789abcdef01234567\n'
    fi
    ;;
  exec)
    if [[ "$*" != *'/app/qs-authz-matrix --config=/app/configs/apiserver.prod.yaml'* ]]; then
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
subjects=[{"kind":k,"source":"production_staff" if k in ("admin","other") else "synthetic_iam_user","subject_fingerprint":str(i)*16} for i,k in enumerate(("admin","operator","plan_manager","other"),1)]
cases=[{"kind":k,"scenario":a,"action":a,"expected_allowed":ok,"allowed":ok,"policy_version":27,"passed":True} for k,a,ok in [("admin","retry",True),("operator","retry",True),("plan_manager","retry",True),("other","retry",False),("operator","force_retry",False),("admin","force_retry",True)]]
print(json.dumps({"schema_version":"iam-authz-production-matrix/v3","git_commit":"0123456789abcdef0123456789abcdef01234567","service_identity":"qs-apiserver.svc","policy_version":27,"subjects":subjects,"cases":cases,"passed":True},separators=(",",":")))
PYFIXTURE
)"

output="$(PRIVILEGE_RUNNER= DOCKER_BIN="$FAKE_DOCKER" FAKE_MATRIX_EVIDENCE="$evidence" \
  "$SCRIPT_DIR/verify-authz-production-matrix.sh")"
printf '%s\n' "$output" | grep -Fq 'Production AuthZ action matrix passed: subjects=4 cases=6 synthetic_subjects=2 policy_version=27'

bad_evidence="${evidence/\"passed\":true}/\"passed\":false}"
if PRIVILEGE_RUNNER= DOCKER_BIN="$FAKE_DOCKER" FAKE_MATRIX_EVIDENCE="$bad_evidence" \
  "$SCRIPT_DIR/verify-authz-production-matrix.sh" >/dev/null 2>&1; then
  echo "matrix verifier accepted failing evidence" >&2
  exit 1
fi

echo "production AuthZ matrix verification contract passed"
