#!/usr/bin/env bash
set -Eeuo pipefail

APISERVER_CONTAINER="${APISERVER_CONTAINER:-}"
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

if [ "$CONFIRMATION" != "provision-isolated-authz-matrix-subjects-v2" ]; then
  echo "Exact matrix subject provisioning confirmation is required" >&2
  exit 1
fi
if [ -n "$PRIVILEGE_RUNNER" ] && ! command -v "$PRIVILEGE_RUNNER" >/dev/null 2>&1; then
  echo "Privilege runner is unavailable: $PRIVILEGE_RUNNER" >&2
  exit 1
fi
command -v "$DOCKER_BIN" >/dev/null 2>&1 || { echo "Docker command is unavailable: $DOCKER_BIN" >&2; exit 1; }
command -v "$PYTHON_BIN" >/dev/null 2>&1 || { echo "Python command is unavailable: $PYTHON_BIN" >&2; exit 1; }

m5_api_pattern='^qs-m5-[0-9a-f]{12}-apiserver-[1-9][0-9]*$'
if [ -z "$APISERVER_CONTAINER" ]; then
  running_names="$(run_privileged "$DOCKER_BIN" ps --format '{{.Names}}')"
  while IFS= read -r name; do
    if [[ "$name" == qs-apiserver || "$name" =~ $m5_api_pattern ]]; then
      if [ -n "$APISERVER_CONTAINER" ]; then
        echo "Multiple running QS apiserver candidates; a named target is required" >&2
        exit 1
      fi
      APISERVER_CONTAINER="$name"
    fi
  done <<<"$running_names"
fi
if ! [[ "$APISERVER_CONTAINER" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]]; then
  echo "No valid named QS apiserver target" >&2
  exit 1
fi

snapshot() {
  run_privileged "$DOCKER_BIN" inspect "$APISERVER_CONTAINER" --format \
    '{{.Name}}|{{.Id}}|{{.Image}}|{{.Config.Image}}|{{.State.Running}}|{{.State.StartedAt}}|{{.RestartCount}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}|{{index .Config.Labels "com.docker.compose.project"}}|{{index .Config.Labels "com.docker.compose.service"}}'
}
before="$(snapshot)"
IFS='|' read -r actual_name container_id image_id image_ref running started restarts health project service <<<"$before"
if [[ "$actual_name" != "/$APISERVER_CONTAINER" || "$running" != true ]]; then
  echo "Apiserver container is not running: $APISERVER_CONTAINER" >&2
  exit 1
fi
if ! [[ "$container_id" =~ ^[0-9a-f]{64}$ && "$image_id" =~ ^sha256:[0-9a-f]{64}$ && -n "$started" && "$restarts" =~ ^[0-9]+$ ]]; then
  echo "Apiserver container identity is incomplete" >&2
  exit 1
fi

image_identity="$(run_privileged "$DOCKER_BIN" image inspect "$image_id" --format '{{.Id}}|{{index .Config.Labels "org.opencontainers.image.revision"}}')"
IFS='|' read -r inspected_image revision <<<"$image_identity"
if [[ "$inspected_image" != "$image_id" ]]; then
  echo "Apiserver image identity changed" >&2
  exit 1
fi
if [[ "$image_ref" == "$image_id" ]]; then
  deployed_sha="$revision"
else
  deployed_sha="${image_ref##*:}"
  if [[ -n "$revision" && "$revision" != '<no value>' && "$revision" != "$deployed_sha" ]]; then
    echo "Apiserver tag and image revision differ" >&2
    exit 1
  fi
fi
if ! [[ "$deployed_sha" =~ ^[0-9a-f]{40}$ ]]; then
  echo "Apiserver image is not bound to a full Git SHA" >&2
  exit 1
fi
if [[ "$APISERVER_CONTAINER" =~ $m5_api_pattern ]]; then
  if [[ "$project" != "${APISERVER_CONTAINER%-*}" || "$project" != "qs-m5-${deployed_sha:0:12}-apiserver" ||
        "$service" != "apiserver-${APISERVER_CONTAINER##*-}" || "$health" != healthy ]]; then
    echo "Named M5 apiserver release identity or health differs" >&2
    exit 1
  fi
fi

# Select by the running image, so current containers and compatible rollback
# images remain usable before/after the command-family release. A probe error
# is not evidence that the old binary should be executed.
command=(/app/qs-authz provision)
legacy_command=false
probe_status=0
run_privileged "$DOCKER_BIN" exec "$container_id" sh -c 'if test -x /app/qs-authz; then exit 0; else exit 42; fi' >/dev/null 2>&1 || probe_status=$?
case "$probe_status" in
  0) ;;
  42) command=(/app/qs-authz-matrix-provision); legacy_command=true ;;
  *) echo "Cannot inspect AuthZ command availability" >&2; exit "$probe_status" ;;
esac

set +e
provision_output="$(run_privileged "$DOCKER_BIN" exec \
  -e QS_AUTHZ_MATRIX_PROVISION_CONFIRM="$CONFIRMATION" \
  "$container_id" "${command[@]}" --config="$CONFIG_FILE" 2>&1)"
provision_status=$?
set -e
if [[ "$(snapshot)" != "$before" ]]; then
  echo "Apiserver runtime changed during provisioning; reconcile the original outcome before retrying" >&2
  exit 1
fi
printf '%s\n' "$provision_output"
if [ "$provision_status" -ne 0 ]; then
  echo "Production AuthZ matrix subject provisioning failed with status $provision_status" >&2
  exit "$provision_status"
fi

evidence_file="$(mktemp /tmp/qs-authz-matrix-provision-evidence.XXXXXX)"
trap 'rm -f -- "$evidence_file"' EXIT
printf '%s\n' "$provision_output" | awk '/^\{"schema_version":"iam-authz-matrix-provision\/v2"/{evidence=$0} END{print evidence}' >"$evidence_file"
if [ ! -s "$evidence_file" ]; then
  echo "Production AuthZ matrix subject provisioning evidence JSON is missing" >&2
  exit 1
fi

DEPLOYED_SHA="$deployed_sha" LEGACY_AUTHZ_COMMAND="$legacy_command" "$PYTHON_BIN" - "$evidence_file" <<'PY'
import json
import os
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    evidence = json.load(handle)

if evidence.get("schema_version") != "iam-authz-matrix-provision/v2":
    raise SystemExit("unexpected matrix provisioning evidence schema")
if evidence.get("git_commit") != os.environ["DEPLOYED_SHA"]:
    raise SystemExit("matrix provisioner SHA does not match deployed image")
if evidence.get("service_identity") != "qs-apiserver.svc":
    raise SystemExit("matrix provisioner did not use qs-apiserver.svc")
expected = {
    "__qs_authz_matrix_operator_v2__": "qs:assessment_operator",
    "__qs_authz_matrix_plan_manager_v2__": "qs:evaluation_plan_manager",
}
subjects = evidence.get("subjects")
if not isinstance(subjects, list) or len(subjects) != len(expected):
    raise SystemExit("matrix provisioner returned an incomplete or duplicate subject list")
observed = {subject.get("nickname"): subject.get("role") for subject in subjects}
if os.environ["LEGACY_AUTHZ_COMMAND"] == "true" and "__qs_authz_matrix_evaluator_v2__" in observed:
    expected["__qs_authz_matrix_evaluator_v2__"] = expected.pop("__qs_authz_matrix_operator_v2__")
if observed != expected:
    raise SystemExit("matrix provisioner targeted unexpected identities or roles")
if evidence.get("passed") is not True:
    raise SystemExit("matrix provisioning did not pass")
for subject in subjects:
    if len(subject.get("subject_fingerprint", "")) != 16:
        raise SystemExit("matrix provisioning fingerprint is invalid")
    if subject.get("policy_version", 0) <= 0 or subject.get("passed") is not True:
        raise SystemExit("matrix subject did not become authoritative")
print(
    "Production AuthZ matrix subjects are ready: "
    f"subjects={len(subjects)} policy_version={max(s['policy_version'] for s in subjects)} "
    f"sha={evidence['git_commit']}"
)
PY
