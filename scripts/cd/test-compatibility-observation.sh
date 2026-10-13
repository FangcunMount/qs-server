#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

FAKE_DOCKER="$TEST_ROOT/docker"
cat >"$FAKE_DOCKER" <<'EOF'
#!/usr/bin/env python3
import datetime
import json
import os
import re
import sys
import time
import urllib.parse

arguments = sys.argv[1:]
scenario = os.environ.get("FAKE_M6_SCENARIO", "")
now = int(time.time())
source = "0123456789abcdef0123456789abcdef01234567"
instances = [("qs-apiserver", "qs-apiserver:8080"),
             ("qs-collection-server", "172.20.0.25:8080"),
             ("qs-collection-server", "172.20.0.26:8080")]
family = {
    "qs_auth_compatibility_observation_info": (1, instances),
    "qs_plan_entry_compatibility_observation_info": (1, instances[1:]),
    "qs_definition_compatibility_observation_info": (1, instances[:1]),
    "qs_auth_token_source_total": (12, instances),
    "qs_auth_claim_source_total": (14, instances),
    "qs_plan_entry_route_total": (4, instances[1:]),
    "qs_model_definition_write_total": (6, instances[:1]),
    "qs_model_definition_input_field_total": (12, instances[:1]),
    "up": (1, instances),
}


def emit(data):
    print(json.dumps({"status": "success", "data": data}, separators=(",", ":")))


if arguments[0] == "ps":
    names = ["qs-apiserver"] if "label=prometheus.job=qs-apiserver" in arguments else ["qs-collection-server-1", "qs-collection-server-2"]
    if scenario == "partial_deploy" and len(names) == 2:
        names.pop()
    print("\n".join(names))
elif arguments[0] == "inspect":
    if "networks" not in arguments[-1]:
        print("true")
    else:
        name = arguments[1]
        job = "qs-apiserver" if name == "qs-apiserver" else "qs-collection-server"
        ip = {"qs-apiserver": "172.20.0.16", "qs-collection-server-1": "172.20.0.25", "qs-collection-server-2": "172.20.0.26"}[name]
        started = now - (1 if scenario == "restarted" else 100 * 86400)
        print(json.dumps({"running": True, "started": datetime.datetime.fromtimestamp(started, datetime.timezone.utc).isoformat(),
                          "labels": {"prometheus.job": job, "prometheus.scrape": "true", "prometheus.metrics_port": "8080", "prometheus.metrics_path": "/metrics"},
                          "networks": {"infra-network": {"IPAddress": ip}}}))
elif arguments[0] == "exec":
    url = arguments[-1]
    if url.endswith("/status/flags"):
        emit({"storage.tsdb.retention.time": "15d" if scenario == "retention_short" else "400d"})
    elif url.endswith("/targets"):
        targets = []
        for index, (job, instance) in enumerate(instances):
            if scenario == "missing_target" and index == 2:
                continue
            # The audit pins its snapshot before this later fake API call.
            # A one-second margin races runner scheduling; use a clearly stale
            # sample while leaving the production two-interval gate unchanged.
            last = now - (3600 if scenario == "stale_scrape" else 1)
            targets.append({"labels": {"job": job, "instance": instance}, "health": "down" if scenario == "down" and index == 2 else "up",
                            "scrapeUrl": "http://" + instance + "/metrics", "scrapeInterval": "60s", "lastScrape": datetime.datetime.fromtimestamp(last, datetime.timezone.utc).isoformat()})
        emit({"activeTargets": targets})
    else:
        raw = next((arg[len("--post-data="):] for arg in arguments if arg.startswith("--post-data=")), "")
        parameters = urllib.parse.parse_qs(raw)
        query = parameters.get("query", [""])[0]
        if os.environ.get("FAKE_QUERY_LOG"):
            with open(os.environ["FAKE_QUERY_LOG"], "a") as stream:
                stream.write(json.dumps({"endpoint": urllib.parse.urlparse(url).path, "parameters": parameters}) + "\n")
        metric_match = re.search(r"qs_[a-z_]+|up\{", query)
        metric = metric_match[0].rstrip("{") if metric_match else ""
        if metric not in family:
            if metric != "qs_evaluation_assessment_intake_legacy_binding_total":
                raise SystemExit("unexpected observation query: " + query)
            if scenario == "admission_absent":
                emit({"resultType": "vector", "result": []})
                sys.exit(0)
            value = 4 if query.startswith("count(") else (2 if scenario == "admission_active" else 0)
            emit({"resultType": "vector", "result": [{"metric": {}, "value": [now, str(value)]}]})

        else:
            count, keys = family[metric]
            keys = list(keys)
            if scenario == "wrong_source" and metric == "qs_auth_compatibility_observation_info":
                keys.pop()
            if scenario == "unexpected_instance":
                keys.append(("qs-collection-server", "unexpected:8080"))
            if "source_sha" in query and source not in query:
                keys = []
            result = []
            for index, (job, instance) in enumerate(keys):
                value = count
                if "resets(" in query:
                    value = 1 if scenario == "counter_reset" and index == 0 else 0
                elif "increase(" in query:
                    legacy = any(part in query for part in ['source=~"extra_.*"', 'source="bare"', 'route="legacy_token"', 'input="derived"'])
                    value = (2 if scenario == "active" else 0) if legacy else (0 if scenario == "canonical_empty" and index == 0 else 1)
                    if metric == "qs_auth_claim_source_total":
                        field_match = re.search(r'field="([a-z_]+)"', query)
                        field = field_match[1] if field_match else ""
                        if scenario in ("org_missing", "org_extra") and field == "org_id":
                            # Only the real verified missing-Org branch increments;
                            # requiring canonical Org must fail this fixture.
                            value = 1 if 'source=~"canonical|missing"' in query else 0
                        if scenario == "org_extra" and legacy:
                            value = 2
                        if scenario == "org_empty" and field == "org_id" and index == 1:
                            value = 0
                        if ((scenario == "user_missing" and field == "user_id") or
                                (scenario == "account_missing" and field == "account_id")):
                            value = 0 if 'source="canonical"' in query else 1
                elif scenario == "missing_zero_series" and metric == "qs_auth_claim_source_total":
                    value -= 1
                labels = {"job": job, "instance": instance}
                if url.endswith("/query_range"):
                    start, end, step = (int(parameters[key][0]) for key in ("start", "end", "step"))
                    samples = [[point, str(value)] for point in range(start, end + 1, step)]
                    if metric == "qs_auth_compatibility_observation_info" and index == 0:
                        if scenario == "history_gap": samples.pop(10)
                        if scenario == "stale_history": samples[10][1] = "0"
                    result.append({"metric": labels, "values": samples})
                else:
                    result.append({"metric": labels, "value": [now, "NaN" if scenario == "nan" else str(value)]})
            emit({"resultType": "matrix" if url.endswith("/query_range") else "vector", "result": result})
else:
    raise SystemExit("unexpected fake docker command")
EOF
chmod +x "$FAKE_DOCKER"

COMMON_ENV=(
  WINDOW_DAYS=14
  PRIVILEGE_RUNNER=
  DOCKER_BIN="$FAKE_DOCKER"
  EXPECTED_SOURCE_SHA=0123456789abcdef0123456789abcdef01234567
  "OBSERVATION_STARTED_AT=$(python3 -c 'import datetime; print((datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(days=14,seconds=2)).strftime("%Y-%m-%dT%H:%M:%SZ"))')"
)

query_log="$TEST_ROOT/queries.jsonl"
zero_output="$(env "${COMMON_ENV[@]}" FAKE_QUERY_LOG="$query_log" "$SCRIPT_DIR/audit-compatibility-observation.sh")"
python3 - "$query_log" <<'PY'
import json
import sys
for line in open(sys.argv[1]):
    query = json.loads(line)
    if query['endpoint'].endswith('query_range') or any('increase(' in value or 'resets(' in value for value in query['parameters'].get('query', [])):
        raise SystemExit('preflight inspected historical samples or claimed an observation window')
PY

if env "${COMMON_ENV[@]}" WINDOW_DAYS=0 \
  "$SCRIPT_DIR/audit-compatibility-observation.sh" >/dev/null 2>&1; then
  echo "observation accepted an invalid window" >&2
  exit 1
fi

if ! printf '%s\n' "$zero_output" | grep -Fq 'observation_window=not_started'; then
  echo "preflight falsely started an observation window" >&2
  exit 1
fi

window_output="$(env "${COMMON_ENV[@]}" OBSERVATION_MODE=window FAKE_QUERY_LOG="$query_log" "$SCRIPT_DIR/audit-compatibility-observation.sh")"
if [ "$(printf '%s\n' "$window_output" | awk -F '\t' '$1 == "M6" && $3 == "zero_window_candidate" {count++} END {print count + 0}')" -ne 4 ]; then
  echo "complete window did not classify all four M6 contracts" >&2
  exit 1
fi
missing_org_output="$(env "${COMMON_ENV[@]}" OBSERVATION_MODE=window FAKE_M6_SCENARIO=org_missing FAKE_QUERY_LOG="$query_log" "$SCRIPT_DIR/audit-compatibility-observation.sh")"
if [ "$(printf '%s\n' "$missing_org_output" | awk -F '\t' '$1 == "M6" && $3 == "zero_window_candidate" {count++} END {print count + 0}')" -ne 4 ]; then
  echo "verified missing optional Org claim did not allow the otherwise canonical window" >&2
  exit 1
fi
extra_org_output="$(env "${COMMON_ENV[@]}" OBSERVATION_MODE=window FAKE_M6_SCENARIO=org_extra "$SCRIPT_DIR/audit-compatibility-observation.sh")"
if ! printf '%s\n' "$extra_org_output" | grep -Fq $'M6\tclaims_extra\tactive_compatibility'; then
  echo "legacy Extra Org traffic was falsely classified as an exit candidate" >&2
  exit 1
fi
active_m6="$(env "${COMMON_ENV[@]}" OBSERVATION_MODE=window WINDOW_DAYS=14 FAKE_M6_SCENARIO=active "$SCRIPT_DIR/audit-compatibility-observation.sh")"
if [ "$(printf '%s\n' "$active_m6" | awk -F '\t' '$1 == "M6" && $3 == "active_compatibility" {count++} END {print count + 0}')" -ne 4 ]; then
  echo "actual legacy traffic was lost" >&2
  exit 1
fi
for scenario in partial_deploy missing_target wrong_source down stale_scrape restarted missing_zero_series unexpected_instance history_gap stale_history counter_reset canonical_empty user_missing account_missing org_empty nan retention_short admission_absent; do
  if env "${COMMON_ENV[@]}" OBSERVATION_MODE=window WINDOW_DAYS=14 FAKE_M6_SCENARIO="$scenario" \
    "$SCRIPT_DIR/audit-compatibility-observation.sh" >"$TEST_ROOT/$scenario.log" 2>&1; then
    echo "M6 observation accepted incomplete evidence: $scenario" >&2
    exit 1
  fi
done
for option in 'EXPECTED_SOURCE_SHA=' 'OBSERVATION_MODE=invalid' 'EXPECTED_APISERVER_CONTAINERS=wrong' 'EXPECTED_COLLECTION_CONTAINERS=qs-collection-server-1'; do
  if env "${COMMON_ENV[@]}" "$option" "$SCRIPT_DIR/audit-compatibility-observation.sh" >/dev/null 2>&1; then
    echo "M6 preflight accepted invalid source/mode/inventory: $option" >&2
    exit 1
  fi
done
if env "${COMMON_ENV[@]}" OBSERVATION_MODE=window WINDOW_DAYS=13 "$SCRIPT_DIR/audit-compatibility-observation.sh" >/dev/null 2>&1; then
  echo "M6 accepted a shorter retirement observation window" >&2
  exit 1
fi


for start in '' 'invalid' '2099-01-01T00:00:00Z'; do
  if env "${COMMON_ENV[@]}" OBSERVATION_MODE=window "OBSERVATION_STARTED_AT=$start" "$SCRIPT_DIR/audit-compatibility-observation.sh" >/dev/null 2>&1; then
    echo "M6 accepted missing, invalid or unelapsed approved T0" >&2
    exit 1
  fi
done

python3 - "$query_log" <<'PY'
import json
import sys
ranges = []
for line in open(sys.argv[1]):
    query = json.loads(line)
    if query['endpoint'].endswith('query_range'):
        params = query['parameters']
        start, end, step = (int(params[key][0]) for key in ('start', 'end', 'step'))
        if end-start > 21600 or (end-start)//step+1 > 11000:
            raise SystemExit('Prometheus range query exceeded the bounded chunk')
        ranges.append(query)
if len(ranges) < 10:
    raise SystemExit('window continuity did not inspect multiple bounded chunks')
PY
# A longer approved period is supported without truncating it to 30 days.
long_start=$(python3 -c 'import datetime; print((datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(days=31,seconds=2)).strftime("%Y-%m-%dT%H:%M:%SZ"))')
env "${COMMON_ENV[@]}" OBSERVATION_MODE=window WINDOW_DAYS=31 "OBSERVATION_STARTED_AT=$long_start" "$SCRIPT_DIR/audit-compatibility-observation.sh" > "$TEST_ROOT/long-window.log"

if [ -n "${M6_QUERY_ARTIFACT:-}" ]; then
  cp "$query_log" "$M6_QUERY_ARTIFACT"
fi

echo "compatibility observation contract passed"
