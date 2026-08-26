#!/usr/bin/env bash
set -Eeuo pipefail

APISERVER_CONTAINER="${APISERVER_CONTAINER:-qs-apiserver}"
PRIVILEGE_RUNNER="${PRIVILEGE_RUNNER-sudo}"
DOCKER_BIN="${DOCKER_BIN:-docker}"
PYTHON_BIN="${PYTHON_BIN:-python3}"
CONFIG_FILE="${CONFIG_FILE:-/app/configs/apiserver.prod.yaml}"

run_privileged() {
  if [ -n "$PRIVILEGE_RUNNER" ]; then
    "$PRIVILEGE_RUNNER" "$@"
  else
    "$@"
  fi
}

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
matrix_output="$(run_privileged "$DOCKER_BIN" exec "$APISERVER_CONTAINER" \
  /app/qs-authz-matrix --config="$CONFIG_FILE" 2>&1)"
matrix_status=$?
set -e
printf '%s\n' "$matrix_output"
if [ "$matrix_status" -ne 0 ]; then
  echo "Production AuthZ matrix command failed with status $matrix_status" >&2
  exit "$matrix_status"
fi

evidence_file="$(mktemp /tmp/qs-authz-matrix-evidence.XXXXXX)"
trap 'rm -f -- "$evidence_file"' EXIT
printf '%s\n' "$matrix_output" | awk '/^\{"schema_version":"iam-authz-production-matrix\/v1"/{evidence=$0} END{print evidence}' >"$evidence_file"
if [ ! -s "$evidence_file" ]; then
  echo "Production AuthZ matrix evidence JSON is missing" >&2
  exit 1
fi

DEPLOYED_SHA="$deployed_sha" "$PYTHON_BIN" - "$evidence_file" <<'PY'
import json
import os
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    evidence = json.load(handle)

expected = {
    ("admin", "adhoc"): True,
    ("admin", "plan"): True,
    ("evaluator", "adhoc"): True,
    ("evaluator", "plan"): False,
    ("plan_manager", "adhoc"): False,
    ("plan_manager", "plan"): True,
    ("other", "adhoc"): False,
    ("other", "plan"): False,
}
if evidence.get("schema_version") != "iam-authz-production-matrix/v1":
    raise SystemExit("unexpected AuthZ matrix evidence schema")
if evidence.get("git_commit") != os.environ["DEPLOYED_SHA"]:
    raise SystemExit("AuthZ matrix binary SHA does not match deployed image")
if evidence.get("service_identity") != "qs-apiserver.svc":
    raise SystemExit("AuthZ matrix did not use qs-apiserver.svc")
if evidence.get("policy_version", 0) <= 0 or evidence.get("passed") is not True:
    raise SystemExit("AuthZ matrix evidence is not a passing loaded policy")
if len(evidence.get("subjects", [])) != 4 or len(evidence.get("cases", [])) != 12:
    raise SystemExit("AuthZ matrix evidence is incomplete")

subjects = {subject.get("kind"): subject for subject in evidence["subjects"]}
if set(subjects) != {"admin", "evaluator", "plan_manager", "other"}:
    raise SystemExit("AuthZ matrix subject kinds are incomplete")
for kind in ("admin", "plan_manager", "other"):
    if subjects[kind].get("source") != "production_staff":
        raise SystemExit(f"AuthZ matrix {kind} subject is not production staff")
if subjects["evaluator"].get("source") not in {"production_staff", "synthetic_iam_user"}:
    raise SystemExit("AuthZ matrix evaluator subject source is invalid")
for subject in subjects.values():
    if len(subject.get("subject_fingerprint", "")) != 16:
        raise SystemExit("AuthZ matrix subject fingerprint is invalid")

observed = {}
for case in evidence["cases"]:
    if case.get("origin_type"):
        observed[(case["kind"], case["origin_type"])] = case.get("allowed")
    if case.get("passed") is not True:
        raise SystemExit(f"AuthZ matrix case failed: {case.get('kind')}/{case.get('origin_type')}")
if observed != expected:
    raise SystemExit(f"AuthZ role x origin matrix mismatch: {observed}")
print(
    "Production AuthZ v3 matrix passed: "
    f"subjects=4 cases=12 evaluator_source={subjects['evaluator']['source']} "
    f"policy_version={evidence['policy_version']} sha={evidence['git_commit']}"
)
PY
