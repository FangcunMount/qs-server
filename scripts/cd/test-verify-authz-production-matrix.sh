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

evidence='{"schema_version":"iam-authz-production-matrix/v2","checked_at":"2026-08-26T00:00:00Z","git_commit":"0123456789abcdef0123456789abcdef01234567","service_identity":"qs-apiserver.svc","domain":"fangcun","resource":"qs:evaluation:collection:assessments","action":"retry","policy_version":27,"subjects":[{"kind":"admin","source":"production_staff","subject_fingerprint":"1111111111111111"},{"kind":"evaluator","source":"synthetic_iam_user","subject_fingerprint":"2222222222222222"},{"kind":"plan_manager","source":"synthetic_iam_user","subject_fingerprint":"3333333333333333"},{"kind":"other","source":"production_staff","subject_fingerprint":"4444444444444444"}],"cases":['
first=1
for origin in adhoc plan; do
  for kind in admin evaluator plan_manager other; do
    allowed=false
    if [ "$kind" = admin ] || { [ "$kind" = evaluator ] && [ "$origin" = adhoc ]; } || { [ "$kind" = plan_manager ] && [ "$origin" = plan ]; }; then
      allowed=true
    fi
    [ "$first" -eq 1 ] || evidence+=','
    first=0
    evidence+="{\"kind\":\"$kind\",\"scenario\":\"origin\",\"action\":\"retry\",\"origin_type\":\"$origin\",\"expected_allowed\":$allowed,\"allowed\":$allowed,\"policy_version\":27,\"passed\":true}"
  done
done
evidence+=',{"kind":"evaluator","scenario":"attribute_missing","action":"retry","expected_allowed":false,"allowed":false,"deny_code":"attribute_missing","missing_attribute_keys":["object.origin_type"],"policy_version":27,"passed":true}'
evidence+=',{"kind":"evaluator","scenario":"attribute_type_error","action":"retry","expected_allowed":false,"expected_error_code":"authorization_contract","allowed":false,"error_code":"authorization_contract","policy_version":27,"passed":true}'
evidence+=',{"kind":"evaluator","scenario":"force_retry","action":"force_retry","expected_allowed":false,"allowed":false,"deny_code":"policy_not_matched","policy_version":27,"passed":true}'
evidence+=',{"kind":"admin","scenario":"force_retry","action":"force_retry","expected_allowed":true,"allowed":true,"matched_role":"qs:admin","policy_version":27,"passed":true}'
evidence+='],"passed":true}'

output="$(PRIVILEGE_RUNNER= DOCKER_BIN="$FAKE_DOCKER" FAKE_MATRIX_EVIDENCE="$evidence" \
  "$SCRIPT_DIR/verify-authz-production-matrix.sh")"
printf '%s\n' "$output" | grep -Fq 'Production AuthZ v3 matrix passed: subjects=4 cases=12 synthetic_subjects=2 policy_version=27'

bad_evidence="${evidence/\"passed\":true}/\"passed\":false}"
if PRIVILEGE_RUNNER= DOCKER_BIN="$FAKE_DOCKER" FAKE_MATRIX_EVIDENCE="$bad_evidence" \
  "$SCRIPT_DIR/verify-authz-production-matrix.sh" >/dev/null 2>&1; then
  echo "matrix verifier accepted failing evidence" >&2
  exit 1
fi

echo "production AuthZ matrix verification contract passed"
