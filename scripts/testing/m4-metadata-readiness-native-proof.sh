#!/usr/bin/env bash
set -Eeuo pipefail

case "${1:-}" in
  mysql)
    [[ "${QS_METADATA_PROFILE_NATIVE_REQUIRED:-}" == 1 ]] || { echo 'MySQL native fixture is required' >&2; exit 1; }
    package=./scripts/oneoff/mysql_metadata_profile
    selector='^(TestMetadataProfileNativeOwnedMySQL|TestMetadataReadinessNativeOwnedMySQL|TestRetirementInventoryNativeOwnedMySQL)$'
    expected='["TestMetadataProfileNativeOwnedMySQL", "TestMetadataReadinessNativeOwnedMySQL", "TestRetirementInventoryNativeOwnedMySQL"]'
    ;;
  mongodb)
    [[ "${QS_MONGO_METADATA_NATIVE_REQUIRED:-}" == 1 ]] || { echo 'MongoDB native fixture is required' >&2; exit 1; }
    package=./scripts/oneoff/mongodb_metadata_profile
    selector='^TestMongoMetadataProfileNativeOwnedReplicaSet$'
    expected='["TestMongoMetadataProfileNativeOwnedReplicaSet"]'
    ;;
  *) echo 'native proof requires mysql or mongodb' >&2; exit 1 ;;
esac

proof_log=$(mktemp)
trap 'rm -f -- "$proof_log"' EXIT
GOMAXPROCS=2 go test -p=2 -race -tags=integration -count=1 -timeout=8m -json -run "$selector" "$package" | tee "$proof_log"
python3 - "$proof_log" "$expected" <<'PY_NATIVE_PROOF'
import json
import sys

expected = set(json.loads(sys.argv[2]))
ran, passed = set(), set()
with open(sys.argv[1], encoding="utf-8") as stream:
    for line in stream:
        event = json.loads(line)
        action, name = event.get("Action"), event.get("Test")
        if action in ("skip", "fail"):
            raise SystemExit("required native proof cannot skip or fail")
        if name in expected:
            if action == "run":
                ran.add(name)
            if action == "pass":
                passed.add(name)
if ran != expected or passed != expected:
    raise SystemExit("required native selectors did not all run and pass")
print("required native selectors RUN/PASS; zero SKIP")
PY_NATIVE_PROOF
