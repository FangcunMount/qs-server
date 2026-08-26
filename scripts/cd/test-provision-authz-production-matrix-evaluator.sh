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
    if [[ "$*" != *'QS_AUTHZ_MATRIX_PROVISION_CONFIRM=provision-isolated-authz-matrix-evaluator'* ]] ||
       [[ "$*" != *'/app/qs-authz-matrix-provision --config=/app/configs/apiserver.prod.yaml'* ]]; then
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

evidence='{"schema_version":"iam-authz-matrix-provision/v1","provisioned_at":"2026-08-26T00:00:00Z","git_commit":"0123456789abcdef0123456789abcdef01234567","service_identity":"qs-apiserver.svc","nickname":"__qs_authz_matrix_evaluator_v1__","subject_fingerprint":"2222222222222222","role":"qs:evaluator","user_created":true,"assignment_created":true,"policy_version":28,"passed":true}'

output="$(PRIVILEGE_RUNNER= DOCKER_BIN="$FAKE_DOCKER" FAKE_PROVISION_EVIDENCE="$evidence" \
  QS_AUTHZ_MATRIX_PROVISION_CONFIRM=provision-isolated-authz-matrix-evaluator \
  "$SCRIPT_DIR/provision-authz-production-matrix-evaluator.sh")"
printf '%s\n' "$output" | grep -Fq 'Production AuthZ matrix evaluator is ready: user_created=True assignment_created=True policy_version=28'

if PRIVILEGE_RUNNER= DOCKER_BIN="$FAKE_DOCKER" FAKE_PROVISION_EVIDENCE="$evidence" \
  QS_AUTHZ_MATRIX_PROVISION_CONFIRM=wrong \
  "$SCRIPT_DIR/provision-authz-production-matrix-evaluator.sh" >/dev/null 2>&1; then
  echo "provisioner accepted an invalid confirmation" >&2
  exit 1
fi

echo "production AuthZ evaluator provisioning contract passed"
