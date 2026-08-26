#!/usr/bin/env bash
set -Eeuo pipefail

APISERVER_CONTAINER="${APISERVER_CONTAINER:-qs-apiserver}"
PRIVILEGE_RUNNER="${PRIVILEGE_RUNNER-sudo}"
DOCKER_BIN="${DOCKER_BIN:-docker}"
PYTHON_BIN="${PYTHON_BIN:-python3}"
CONFIG_FILE="${CONFIG_FILE:-/app/configs/apiserver.prod.yaml}"
CONFIRMATION="${QS_AUTHZ_MATRIX_PROVISION_CONFIRM:-}"

run_privileged() {
  if [ -n "$PRIVILEGE_RUNNER" ]; then
    "$PRIVILEGE_RUNNER" "$@"
  else
    "$@"
  fi
}

if [ "$CONFIRMATION" != "provision-isolated-authz-matrix-evaluator" ]; then
  echo "Exact evaluator provisioning confirmation is required" >&2
  exit 1
fi
if [ -n "$PRIVILEGE_RUNNER" ] && ! command -v "$PRIVILEGE_RUNNER" >/dev/null 2>&1; then
  echo "Privilege runner is unavailable: $PRIVILEGE_RUNNER" >&2
  exit 1
fi
command -v "$DOCKER_BIN" >/dev/null 2>&1 || { echo "Docker command is unavailable: $DOCKER_BIN" >&2; exit 1; }
command -v "$PYTHON_BIN" >/dev/null 2>&1 || { echo "Python command is unavailable: $PYTHON_BIN" >&2; exit 1; }

if [ "$(run_privileged "$DOCKER_BIN" inspect "$APISERVER_CONTAINER" --format '{{.State.Running}}' 2>/dev/null || true)" != "true" ]; then
  echo "Apiserver container is not running: $APISERVER_CONTAINER" >&2
  exit 1
fi

container_image="$(run_privileged "$DOCKER_BIN" inspect "$APISERVER_CONTAINER" --format '{{.Config.Image}}')"
deployed_sha="${container_image##*:}"
if [ "${#deployed_sha}" -ne 40 ] || [ -n "${deployed_sha//[0-9a-f]/}" ]; then
  echo "Apiserver image is not pinned to a full Git SHA: $container_image" >&2
  exit 1
fi

set +e
provision_output="$(run_privileged "$DOCKER_BIN" exec \
  -e QS_AUTHZ_MATRIX_PROVISION_CONFIRM="$CONFIRMATION" \
  "$APISERVER_CONTAINER" /app/qs-authz-matrix-provision --config="$CONFIG_FILE" 2>&1)"
provision_status=$?
set -e
printf '%s\n' "$provision_output"
if [ "$provision_status" -ne 0 ]; then
  echo "Production AuthZ evaluator provisioning failed with status $provision_status" >&2
  exit "$provision_status"
fi

evidence_file="$(mktemp /tmp/qs-authz-matrix-provision-evidence.XXXXXX)"
trap 'rm -f -- "$evidence_file"' EXIT
printf '%s\n' "$provision_output" | awk '/^\{"schema_version":"iam-authz-matrix-provision\/v1"/{evidence=$0} END{print evidence}' >"$evidence_file"
if [ ! -s "$evidence_file" ]; then
  echo "Production AuthZ evaluator provisioning evidence JSON is missing" >&2
  exit 1
fi

DEPLOYED_SHA="$deployed_sha" "$PYTHON_BIN" - "$evidence_file" <<'PY'
import json
import os
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    evidence = json.load(handle)

if evidence.get("schema_version") != "iam-authz-matrix-provision/v1":
    raise SystemExit("unexpected evaluator provisioning evidence schema")
if evidence.get("git_commit") != os.environ["DEPLOYED_SHA"]:
    raise SystemExit("evaluator provisioner SHA does not match deployed image")
if evidence.get("service_identity") != "qs-apiserver.svc":
    raise SystemExit("evaluator provisioner did not use qs-apiserver.svc")
if evidence.get("nickname") != "__qs_authz_matrix_evaluator_v1__" or evidence.get("role") != "qs:evaluator":
    raise SystemExit("evaluator provisioner targeted an unexpected identity or role")
if len(evidence.get("subject_fingerprint", "")) != 16:
    raise SystemExit("evaluator provisioning fingerprint is invalid")
if evidence.get("policy_version", 0) <= 0 or evidence.get("passed") is not True:
    raise SystemExit("evaluator provisioning did not become authoritative")
print(
    "Production AuthZ matrix evaluator is ready: "
    f"user_created={evidence['user_created']} assignment_created={evidence['assignment_created']} "
    f"policy_version={evidence['policy_version']} sha={evidence['git_commit']}"
)
PY
