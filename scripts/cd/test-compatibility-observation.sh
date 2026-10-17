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
import math
import os
import re
import sys
import time
import urllib.parse
from pathlib import Path

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


def raw_matrix(metric, end, duration):
    path = Path(os.environ["FAKE_AUDIT_PATH"])
    embedded = path.read_text().split("<<'PY_M6_OBSERVATION'\n", 1)[1].rsplit("\nPY_M6_OBSERVATION", 1)[0]
    namespace = {}
    exec(compile(embedded.rsplit("\ntry:\n    main()", 1)[0], str(path), "exec"), namespace)
    t0 = datetime.datetime.fromisoformat(os.environ["OBSERVATION_STARTED_AT"].replace("Z", "+00:00")).timestamp()
    stamps = range(math.floor((end-duration)/60)*60+60, math.floor(end/60)*60+1, 60)
    rows = []
    for job, instance in family[metric][1]:
        combos = [{}] if metric == "up" else namespace["family_combinations"](metric, source)
        for combo in combos:
            contract, legacy = namespace["traffic_contract"](metric, combo)
            # Real verified missing Org grows; canonical Org stays zero.
            if metric == "qs_auth_claim_source_total" and combo.get("field") == "org_id" and combo.get("source") == "canonical":
                contract = None
            values = [[stamp, "1" if metric == "up" or metric.endswith("_observation_info") else str(max(0, int((stamp-t0)//60)) if contract and not legacy else 0)] for stamp in stamps]
            rows.append({"metric": {"job": job, "instance": instance, **combo}, "values": values})
    return rows

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
                            "scrapeUrl": "http://" + instance + "/metrics", "scrapeInterval": "1m", "scrapeTimeout": "1s500ms", "lastScrape": datetime.datetime.fromtimestamp(last, datetime.timezone.utc).isoformat()})
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
        if url.endswith("/query_range"):
            raise SystemExit("raw evidence cannot use interpolated query_range")
        raw_match = re.fullmatch(r"[a-z_]+\{[^}]*\}\[([0-9]+)s\]", query)
        if raw_match:
            if "time" not in parameters:
                raise SystemExit("raw query lacks explicit historical end")
            emit({"resultType": "matrix", "result": raw_matrix(metric, float(parameters["time"][0]), int(raw_match[1]))})
        elif query.startswith("max(time()-timestamp("):
            emit({"resultType": "vector", "result": [{"metric": {}, "value": [now, "3600" if scenario == "stale_scrape" else "1"]}]})
        elif metric not in family:
            if metric != "qs_evaluation_assessment_intake_legacy_binding_total":
                raise SystemExit("unexpected observation query: " + query)
            if scenario == "admission_absent":
                emit({"resultType": "vector", "result": []})
            else:
                emit({"resultType": "vector", "result": [{"metric": {}, "value": [now, "4" if query.startswith("count(") else "0"]}]})
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
            for job, instance in keys:
                value = 1 if metric == "up" else count
                if scenario == "missing_zero_series" and metric == "qs_auth_claim_source_total":
                    value -= 1
                result.append({"metric": {"job": job, "instance": instance}, "value": [now, "NaN" if scenario == "nan" else str(value)]})
            emit({"resultType": "vector", "result": result})
else:
    raise SystemExit("unexpected fake docker command")
EOF
chmod +x "$FAKE_DOCKER"

# Only the first logical time is pinned in this test interpreter. Later target,
# current and closing reads use real time, reproducing an audit delayed past E.
FAKE_PYTHON="$TEST_ROOT/python"
cat >"$FAKE_PYTHON" <<'EOF'
#!/usr/bin/env python3
import os
import sys
import time
original = time.time
first = True


def clock():
    global first
    if first:
        first = False
        return float(os.environ["FAKE_LOGICAL_END"])
    return original()


time.time = clock
exec(compile(sys.stdin.read(), "synthetic_m6_cli", "exec"), {"__name__": "__main__"})
EOF
chmod +x "$FAKE_PYTHON"
logical_end=$(python3 -c 'import time; print(int(time.time())-120)')
COMMON_ENV=(
  WINDOW_DAYS=14
  PRIVILEGE_RUNNER=
  DOCKER_BIN="$FAKE_DOCKER"
  PYTHON_BIN="$FAKE_PYTHON"
  "FAKE_LOGICAL_END=$logical_end"
  "FAKE_AUDIT_PATH=$SCRIPT_DIR/audit-compatibility-observation.sh"
  EXPECTED_SOURCE_SHA=0123456789abcdef0123456789abcdef01234567
  "OBSERVATION_STARTED_AT=$(python3 -c 'import datetime,sys; print(datetime.datetime.fromtimestamp(int(sys.argv[1])-14*86400-2,datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"))' "$logical_end")"
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
printf '%s\n' "$zero_output" "$window_output"

# Short raw-sample matrices cover the counterexamples without replaying an
# invented 14-day history for every malformed point or label.
python3 "$SCRIPT_DIR/test-compatibility-raw-window.py"
for scenario in partial_deploy missing_target wrong_source down stale_scrape restarted missing_zero_series unexpected_instance nan retention_short admission_absent; do
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

python3 - "$query_log" "$logical_end" <<'PY'
import json
import re
import sys
ranges = []
logical_end = int(sys.argv[2])
for line in open(sys.argv[1]):
    query = json.loads(line)
    params = query['parameters']
    expression = params.get('query', [''])[0]
    if query['endpoint'].endswith('query_range'):
        raise SystemExit('window continuity used interpolated evaluation samples')
    match = re.fullmatch(r'[a-z_]+\{[^}]*\}\[([0-9]+)s\]', expression)
    if match:
        if 'time' not in params or int(match[1]) > 21600+61:
            raise SystemExit('raw matrix lacks explicit end or exceeds bounded six-hour overlap')
        ranges.append(float(params['time'][0]))
    if 'increase(' in expression and float(params.get('time', ['0'])[0]) != logical_end:
        raise SystemExit('historical instant increase moved beyond its fixed logical end')
if len(ranges) < 10 or max(ranges) <= logical_end or logical_end not in ranges:
    raise SystemExit('raw audit did not inspect bounded chunks and a separate closing snapshot')
PY

if [ -n "${M6_QUERY_ARTIFACT:-}" ]; then
  cp "$query_log" "$M6_QUERY_ARTIFACT"
fi

echo "compatibility observation contract passed"
