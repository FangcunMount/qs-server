#!/usr/bin/env bash
set -Eeuo pipefail

WINDOW_DAYS="${WINDOW_DAYS:-14}"
APISERVER_CONTAINER="${APISERVER_CONTAINER:-qs-apiserver}"
PROMETHEUS_BASE_URL="${PROMETHEUS_BASE_URL:-http://prometheus:9090}"
QUERY_TIMEOUT_SECONDS="${QUERY_TIMEOUT_SECONDS:-10}"
PRIVILEGE_RUNNER="${PRIVILEGE_RUNNER-sudo}"
DOCKER_BIN="${DOCKER_BIN:-docker}"
PYTHON_BIN="${PYTHON_BIN:-python3}"
# Existing scheduled calls remain preflight. A window audit must explicitly bind
# the reviewed deployment inventory and compiled source; it never starts a timer.
OBSERVATION_MODE="${OBSERVATION_MODE:-preflight}"
EXPECTED_SOURCE_SHA="${EXPECTED_SOURCE_SHA:-}"
OBSERVATION_STARTED_AT="${OBSERVATION_STARTED_AT:-}"
EXPECTED_APISERVER_CONTAINERS="${EXPECTED_APISERVER_CONTAINERS:-qs-apiserver}"
EXPECTED_COLLECTION_CONTAINERS="${EXPECTED_COLLECTION_CONTAINERS:-qs-collection-server-1,qs-collection-server-2}"

require_positive_integer() {
  local name="$1"
  local value="$2"
  if ! [[ "$value" =~ ^[0-9]+$ ]] || [ "$value" -lt 1 ]; then
    echo "$name must be a positive integer, got: $value" >&2
    return 1
  fi
}

run_privileged() {
  if [ "$PRIVILEGE_RUNNER" = sudo ]; then
    if [ -n "${SUDO_PASSWORD:-}" ]; then
      printf '%s\n' "$SUDO_PASSWORD" | sudo -S -p '' -- "$@"
    else
      sudo -n -- "$@"
    fi
  elif [ -n "$PRIVILEGE_RUNNER" ]; then
    "$PRIVILEGE_RUNNER" "$@"
  else
    "$@"
  fi
}

require_positive_integer WINDOW_DAYS "$WINDOW_DAYS"
require_positive_integer QUERY_TIMEOUT_SECONDS "$QUERY_TIMEOUT_SECONDS"
if [ -n "$PRIVILEGE_RUNNER" ] && ! command -v "$PRIVILEGE_RUNNER" >/dev/null 2>&1; then
  echo "Privilege runner is unavailable: $PRIVILEGE_RUNNER" >&2
  exit 1
fi
if ! command -v "$DOCKER_BIN" >/dev/null 2>&1; then
  echo "Docker command is unavailable: $DOCKER_BIN" >&2
  exit 1
fi
if ! command -v "$PYTHON_BIN" >/dev/null 2>&1; then
  echo "Python command is unavailable: $PYTHON_BIN" >&2
  exit 1
fi
if [ "$(run_privileged "$DOCKER_BIN" inspect "$APISERVER_CONTAINER" --format '{{.State.Running}}' 2>/dev/null || true)" != "true" ]; then
  echo "Apiserver container is not running: $APISERVER_CONTAINER" >&2
  exit 1
fi

export WINDOW_DAYS OBSERVATION_MODE EXPECTED_SOURCE_SHA OBSERVATION_STARTED_AT EXPECTED_APISERVER_CONTAINERS EXPECTED_COLLECTION_CONTAINERS
export DOCKER_BIN PRIVILEGE_RUNNER APISERVER_CONTAINER PROMETHEUS_BASE_URL QUERY_TIMEOUT_SECONDS
export SUDO_PASSWORD="${SUDO_PASSWORD:-}"
"$PYTHON_BIN" - <<'PY_M6_OBSERVATION'
import datetime
import json
import math
import os
import re
import subprocess
import sys
import time
import urllib.parse


def fail(message):
    raise ValueError(message)


def docker(*arguments):
    command = [os.environ["DOCKER_BIN"], *arguments]
    password = None
    runner = os.environ["PRIVILEGE_RUNNER"]
    if runner == "sudo":
        if os.environ["SUDO_PASSWORD"]:
            command = [runner, "-S", "-p", "", "--", *command]
            password = os.environ["SUDO_PASSWORD"] + "\n"
        else:
            command = [runner, "-n", "--", *command]
    elif runner:
        command.insert(0, runner)
    result = subprocess.run(command, input=password, check=True, capture_output=True, text=True,
                            timeout=int(os.environ["QUERY_TIMEOUT_SECONDS"]) + 5)
    return result.stdout


def api(endpoint, parameters=None):
    arguments = ["exec", os.environ["APISERVER_CONTAINER"], "wget", "-qO-", "-T",
                 os.environ["QUERY_TIMEOUT_SECONDS"]]
    if parameters is not None:
        arguments.append("--post-data=" + urllib.parse.urlencode(parameters))
    arguments.append(os.environ["PROMETHEUS_BASE_URL"].rstrip("/") + "/api/v1/" + endpoint)
    payload = json.loads(docker(*arguments))
    if payload.get("status") != "success" or payload.get("warnings"):
        fail("Prometheus did not return complete successful evidence")
    return payload["data"]


def query(expression, end=None):
    parameters = {"query": expression}
    if end is not None:
        parameters["time"] = end
    data = api("query", parameters)
    if data.get("resultType") != "vector":
        fail("unexpected Prometheus result type")
    return data["result"]


def finite(value):
    result = float(value)
    if not math.isfinite(result):
        fail("non-finite Prometheus sample")
    return result


def keyed(series, expected):
    result = {}
    for item in series:
        labels = item["metric"]
        key = (labels.get("job"), labels.get("instance"))
        if key in result or key not in expected:
            fail("duplicate or unexpected service instance series")
        result[key] = item
    if set(result) != set(expected):
        fail("service instance series missing; absence is not zero")
    return result


def current_values(expression, expected):
    return {key: finite(item["value"][1]) for key, item in keyed(query(expression), expected).items()}


def exact_values(values, expected_value, description):
    if any(value != expected_value for value in values.values()):
        fail(description)


# These fixed combinations mirror the existing bounded producer registration.
def family_combinations(metric, source):
    if metric.endswith("_compatibility_observation_info"):
        return [{"source_sha": source, "capability": "1"}]
    if metric == "qs_auth_token_source_total":
        return [dict(source=s, result=r) for s in ("bearer", "bare", "query", "cookie", "missing", "unavailable") for r in ("accepted", "rejected")]
    if metric == "qs_auth_claim_source_total":
        return [dict(field=f, source=s) for f, sources in {
            "user_id": ("canonical", "missing", "extra_user_id", "extra_uid", "extra_sub"),
            "account_id": ("canonical", "missing", "extra_account_id", "extra_accountId", "extra_aid"),
            "org_id": ("canonical", "missing", "extra_org_id", "extra_organization_id")}.items() for s in sources]
    if metric == "qs_plan_entry_route_total":
        return [dict(route=r, result=s) for r in ("canonical", "legacy_token") for s in ("accepted", "rejected")]
    if metric == "qs_model_definition_write_total":
        return [dict(input=i, result=r) for i in ("canonical", "derived", "unreadable") for r in ("accepted", "rejected")]
    if metric == "qs_model_definition_input_field_total":
        return [dict(field=f, presence=p, result=r) for f in ("DecisionSpec", "InterpretationAssets") for p in ("present", "absent", "unreadable") for r in ("accepted", "rejected")]
    fail("unexpected raw metric family")


def traffic_contract(metric, labels):
    if metric == "qs_auth_token_source_total":
        if labels["source"] == "bare": return "bare_authorization", True
        if labels["source"] == "bearer" and labels["result"] == "accepted": return "bearer", False
    if metric == "qs_auth_claim_source_total":
        if labels["source"].startswith("extra_"): return "claims_extra", True
        if labels["source"] == "canonical" and labels["field"] in ("user_id", "account_id"): return labels["field"], False
        if labels["field"] == "org_id" and labels["source"] in ("canonical", "missing"): return "org_id", False
    if metric == "qs_plan_entry_route_total":
        if labels["route"] == "legacy_token": return "task_token_route", True
        if labels["result"] == "accepted": return "task", False
    if metric == "qs_model_definition_write_total":
        if labels["input"] == "unreadable": return "definition_unreadable", True
        if labels["input"] == "derived": return "definition_derived_input", True
        if labels["input"] == "canonical" and labels["result"] == "accepted": return "definition", False
    if metric == "qs_model_definition_input_field_total" and labels["presence"] == "unreadable":
        return "definition_unreadable", True
    return None, False


def raw_points(item, begin, end):
    result = []
    previous = None
    begin_stamp, end_stamp = round(begin * 1000), round(end * 1000)
    for timestamp, value in item["values"]:
        stamp = round(finite(timestamp) * 1000)
        value = finite(value)
        if previous is not None and stamp <= previous:
            fail("duplicate or out-of-order raw scrape sample")
        previous = stamp
        if begin_stamp <= stamp <= end_stamp:
            result.append((stamp, value))
    if not result:
        fail("raw scrape samples absent; absence is not zero")
    return result


def audit_raw_chunk(up_rows, family_rows, cadence, t0, end, source, state, close=False, window_end=None):
    """Validate only actual retained samples; state keeps bounded seam evidence."""
    timelines = state.setdefault("up", {})
    last = state.setdefault("last", {})
    recent = state.setdefault("recent", {})
    traffic = state.setdefault("traffic", set())
    boundary = state.setdefault("boundary", set())
    fixed_labels = state.setdefault("labels", {})
    begins = {key: t0 - 2 * interval - min(1.0, interval / 10) for key, interval in cadence.items()}
    window_end = end if window_end is None else window_end
    bounds = {}
    up_samples = {}
    for key, row in keyed(up_rows, set(cadence)).items():
        interval = cadence[key]
        jitter = min(1.0, interval / 10)
        bound = end
        if close:
            closing = [finite(ts) for ts, _ in row["values"] if finite(ts) > window_end]
            if not closing: fail("first post-end scrape closing bracket is missing")
            bound = min(closing)
        bounds[key] = bound
        points = raw_points(row, begins[key], bound)
        timeline = timelines.setdefault(key, {"last": None, "recent": {}, "baseline": set()})
        all_labels = tuple(sorted(row["metric"].items()))
        if timeline.setdefault("labels", all_labels) != all_labels:
            fail("complete raw up label set changed across chunks")
        for stamp, value in points:
            if value != 1:
                fail("an actual target scrape was not up")
            if stamp / 1000 <= t0: timeline["baseline"].add(stamp)
            if stamp in timeline["recent"]:
                if timeline["recent"][stamp] != value: fail("overlap up sample changed")
                continue
            if timeline["last"] is None:
                timeline.update(slot=0, low=stamp / 1000 - jitter, high=stamp / 1000 + jitter)
            else:
                if stamp <= timeline["last"]: fail("unverifiable overlap up sample")
                phase = (timeline["low"] + timeline["high"]) / 2
                slot = round((stamp / 1000 - phase) / interval)
                if slot != timeline["slot"] + 1: fail("an actual scrape cadence slot is missing or duplicated")
                timeline["low"] = max(timeline["low"], stamp / 1000 - slot * interval - jitter)
                timeline["high"] = min(timeline["high"], stamp / 1000 - slot * interval + jitter)
                if timeline["low"] > timeline["high"]: fail("actual scrape jitter exceeds the bounded phase contract")
                timeline["slot"] = slot
            timeline["last"] = stamp
            timeline["recent"][stamp] = value
        if len(timeline["baseline"]) < 2:
            fail("two successful actual baseline scrapes before T0 are required")
        if not close and timeline["high"] + (timeline["slot"] + 1) * interval < end - 0.001:
            fail("the last expected scrape slot before the fixed end is missing")
        up_samples[key] = points
        timeline["recent"] = {ts: v for ts, v in timeline["recent"].items() if ts >= (end - 2 * max(cadence.values()) - 2) * 1000}
    metrics = ("qs_auth_compatibility_observation_info", "qs_plan_entry_compatibility_observation_info", "qs_definition_compatibility_observation_info", "qs_auth_token_source_total", "qs_auth_claim_source_total", "qs_plan_entry_route_total", "qs_model_definition_write_total", "qs_model_definition_input_field_total")
    if set(family_rows) != set(metrics): fail("fixed raw metric family inventory differs")
    for metric in metrics:
        expected = {key for key in cadence if ("plan_entry" not in metric or key[0] == "qs-collection-server") and ("definition" not in metric or key[0] == "qs-apiserver")}
        combinations = [tuple(sorted(combo.items())) for combo in family_combinations(metric, source)]
        dimensions = set(dict(combinations[0]))
        rows = {}
        for item in family_rows[metric]:
            labels = item["metric"]
            key = (labels.get("job"), labels.get("instance"))
            combo = tuple(sorted((name, labels.get(name)) for name in dimensions))
            identity = (key, combo)
            if key not in expected or combo not in combinations or identity in rows:
                fail("raw source or fixed label combination missing, duplicate or unexpected")
            rows[identity] = item
        required = {(key, combo) for key in expected for combo in combinations}
        if set(rows) != required: fail("raw fixed label combination inventory incomplete")
        info = metric.endswith("_compatibility_observation_info")
        for (key, combo), item in rows.items():
            points = raw_points(item, begins[key], bounds[key])
            if [ts for ts, _ in points] != [ts for ts, _ in up_samples[key]]:
                fail("a fixed source/counter sample is missing at an actual up scrape timestamp")
            identity = (metric, key, combo)
            all_labels = tuple(sorted(item["metric"].items()))
            if fixed_labels.setdefault(identity, all_labels) != all_labels:
                fail("complete raw series label set changed across chunks")
            cache = recent.setdefault(identity, {})
            for stamp, value in points:
                if info and value != 1: fail("compiled source capability changed within the window")
                if not info and not 0 <= value < 2 ** 53: fail("counter value cannot safely prove unit increments")
                if stamp in cache:
                    if cache[stamp] != value: fail("overlap counter sample changed")
                    continue
                previous = last.get(identity)
                if previous is not None:
                    if stamp <= previous[0]: fail("unverifiable overlap counter sample")
                    if not info and value < previous[1]: fail("counter reset within baseline/window; cannot classify as zero")
                    contract, legacy = traffic_contract(metric, dict(combo))
                    if contract and value > previous[1] and (legacy or (previous[0] / 1000 >= t0 and stamp / 1000 <= window_end)):
                        traffic.add((contract, key))
                        if legacy and (previous[0] / 1000 < t0 or stamp / 1000 > window_end): boundary.add((contract, key))
                last[identity] = (stamp, value)
                cache[stamp] = value
            recent[identity] = {ts: v for ts, v in cache.items() if ts >= (end - 2 * max(cadence.values()) - 2) * 1000}


def raw_query(selector, begin, end):
    # Instant range vectors return retained sample timestamps, not an evaluation
    # grid filled by lookback. One extra second includes the left boundary.
    duration = math.ceil(end - begin) + 1
    data = api("query", {"query": selector + "[" + str(duration) + "s]", "time": end})
    if data.get("resultType") != "matrix": fail("raw Prometheus matrix unavailable")
    return data["result"]


def check_raw_window(selectors, cadence, start, end, source):
    state = {}
    overlap = max(cadence.values())
    cursor = start - 2 * overlap - 1
    while cursor < end:
        chunk_end = min(cursor + 21600, end)
        begin = cursor - overlap
        up = raw_query('up{job=~"qs-apiserver|qs-collection-server"}', begin, chunk_end)
        families = {metric: raw_query(selector, begin, chunk_end) for metric, selector in selectors.items()}
        audit_raw_chunk(up, families, cadence, start, chunk_end, source, state, window_end=end)
        cursor = chunk_end
    # Logical E never moves. After the historical scan, read a separate fixed
    # closing snapshot and require the first real post-E sample for every child.
    # Boundary legacy growth is conservatively active, never a zero candidate.
    closing_end = time.time()
    if closing_end <= end: fail("post-end closing snapshot unavailable")
    if closing_end - end > 21600 - overlap:
        fail("historical audit exceeded the bounded closing read; window remains incomplete")
    begin = end - 2 * overlap
    up = raw_query('up{job=~"qs-apiserver|qs-collection-server"}', begin, closing_end)
    families = {metric: raw_query(selector, begin, closing_end) for metric, selector in selectors.items()}
    audit_raw_chunk(up, families, cadence, start, end, source, state, close=True, window_end=end)
    print("M6 evidence bounds: T0=" + str(start) + "; audited_end=" + str(end) + "; closing_snapshot=" + str(closing_end))
    if state["boundary"]: print("M6 boundary_ambiguous: conservative_active=" + ",".join(sorted({name for name, _ in state["boundary"]})))
    return state["traffic"]


def retention_seconds(raw):
    units = {"y": 365 * 86400, "w": 7 * 86400, "d": 86400,
             "h": 3600, "m": 60, "s": 1, "ms": 0.001}
    parts = re.findall(r"([0-9]+)(ms|[ywdhms])", raw)
    if not parts or "".join(number + unit for number, unit in parts) != raw:
        fail("Prometheus retention duration unavailable or unsupported")
    return sum(int(number) * units[unit] for number, unit in parts)


def main():
    mode = os.environ["OBSERVATION_MODE"]
    if mode not in {"preflight", "window"}:
        fail("OBSERVATION_MODE must be preflight or window")
    source = os.environ["EXPECTED_SOURCE_SHA"]
    if re.fullmatch(r"[0-9a-f]{40}", source) is None:
        fail("compiled source approval missing: EXPECTED_SOURCE_SHA must be the reviewed full SHA; window not started")
    days = int(os.environ["WINDOW_DAYS"])
    if mode == "window" and days < 14:
        fail("retirement observation requires at least 14 complete days")
    end = int(time.time())
    start = end
    if mode == "window":
        approved_start = os.environ["OBSERVATION_STARTED_AT"]
        if re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z", approved_start) is None:
            fail("approved observation T0 is missing or is not a UTC timestamp")
        start = int(datetime.datetime.fromisoformat(approved_start.replace("Z", "+00:00")).timestamp())
        if end - start < days * 86400:
            fail("approved observation period has not elapsed since T0")
    # Retention is a prerequisite, not evidence that samples actually exist.
    retained = retention_seconds(api("status/flags").get("storage.tsdb.retention.time", ""))
    if retained < max(30, days + 7) * 86400:
        fail("Prometheus retention is shorter than max(30 days, approved period + 7 days)")
    targets = api("targets")["activeTargets"]
    targets_now = time.time()
    inventory = {}
    intervals = {}
    for job, variable in [("qs-apiserver", "EXPECTED_APISERVER_CONTAINERS"),
                          ("qs-collection-server", "EXPECTED_COLLECTION_CONTAINERS")]:
        names = os.environ[variable].split(",")
        if not names or len(names) > 16 or len(set(names)) != len(names) or any(re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]*", name) is None for name in names):
            fail("invalid reviewed deployment container inventory")
        running = set(docker("ps", "--filter", "status=running", "--filter",
                             "label=prometheus.job=" + job, "--format", "{{.Names}}").splitlines())
        if running != set(names):
            fail("running deployment instances differ from reviewed inventory")
        job_targets = [target for target in targets if target.get("labels", {}).get("job") == job]
        if len(job_targets) != len(names):
            fail("Prometheus target inventory does not cover every deployed instance")
        for name in names:
            template = '{"running":{{json .State.Running}},"started":{{json .State.StartedAt}},"labels":{{json .Config.Labels}},"networks":{{json .NetworkSettings.Networks}}}'
            metadata = json.loads(docker("inspect", name, "--format", template))
            labels = metadata["labels"]
            if not metadata["running"] or labels.get("prometheus.scrape") != "true" or labels.get("prometheus.job") != job or labels.get("prometheus.metrics_path") != "/metrics":
                fail("running service lacks its declared scrape capability")
            port = labels.get("prometheus.metrics_port", "")
            if re.fullmatch(r"[0-9]{1,5}", port) is None or not 1 <= int(port) <= 65535:
                fail("invalid service metrics port")
            candidates = {name + ":" + port}
            for network in metadata["networks"].values():
                if network.get("IPAddress"):
                    candidates.add(network["IPAddress"] + ":" + port)
            matches = [target for target in job_targets if target.get("labels", {}).get("instance") in candidates]
            if len(matches) != 1:
                fail("deployed container cannot be mapped uniquely to a Prometheus instance")
            target = matches[0]
            key = (job, target["labels"]["instance"])
            if key in inventory or target.get("health") != "up":
                fail("duplicate or unavailable scrape instance")
            scrape_url = urllib.parse.urlparse(target.get("scrapeUrl", ""))
            if scrape_url.netloc not in candidates or scrape_url.path != "/metrics":
                fail("scrape URL does not match deployed service endpoint")
            interval = retention_seconds(target.get("scrapeInterval", ""))
            timeout = retention_seconds(target.get("scrapeTimeout", ""))
            if not 15 <= interval <= 60 or not 0 < timeout < interval:
                fail("scrape interval/timeout must be bounded to 15-60 seconds and timeout below interval")
            intervals[key] = interval
            last = datetime.datetime.fromisoformat(target["lastScrape"].replace("Z", "+00:00")).timestamp()
            if not -0.001 <= targets_now - last <= 2 * interval:
                fail("current scrape sample is stale")
            started = datetime.datetime.fromisoformat(metadata["started"].replace("Z", "+00:00")).timestamp()
            if mode == "window" and started > start - 2 * interval - min(1.0, interval / 10):
                fail("service restarted or was deployed inside the requested window; window incomplete")
            inventory[key] = name
    api_instances = {key for key in inventory if key[0] == "qs-apiserver"}
    collection_instances = set(inventory) - api_instances
    all_instances = set(inventory)
    family_specs = [
        ("qs_auth_compatibility_observation_info", all_instances, 1, True),
        ("qs_plan_entry_compatibility_observation_info", collection_instances, 1, True),
        ("qs_definition_compatibility_observation_info", api_instances, 1, True),
        ("qs_auth_token_source_total", all_instances, 12, False),
        ("qs_auth_claim_source_total", all_instances, 14, False),
        ("qs_plan_entry_route_total", collection_instances, 4, False),
        ("qs_model_definition_write_total", api_instances, 6, False),
        ("qs_model_definition_input_field_total", api_instances, 12, False),
    ]
    selectors = {}
    for metric, expected, count, info in family_specs:
        jobs = "|".join(sorted({key[0] for key in expected}))
        broad = metric + '{job=~"' + jobs + '"}'
        selectors[metric] = broad
        selector = metric + '{job=~"' + jobs + '"' + (',source_sha="' + source + '",capability="1"' if info else '') + '}'
        if info:
            selector = "(" + selector + " == 1)"
        aggregate = "count by(job,instance)(" + selector + ")"
        exact_values(current_values(aggregate, expected), count, "required zero-valued source/series capability is absent or partial")
        for key in expected:
            interval = intervals[key]
            scoped = metric + '{job="' + key[0] + '",instance="' + key[1] + '"}'
            fresh = "max(time()-timestamp(" + scoped + "))"
            samples = query(fresh)
            if len(samples) != 1 or not 0 <= finite(samples[0]["value"][1]) < 2 * interval:
                fail("current capability or counter sample is stale")
    up = 'up{job=~"qs-apiserver|qs-collection-server"}'
    exact_values(current_values(up, all_instances), 1, "service scrape is not up")
    # The retained legacy admission counter is supplementary: it cannot settle
    # pending/hold/dead-letter/manual replay responsibility or attest Worker SHA.
    admission = "qs_evaluation_assessment_intake_legacy_binding_total"
    current_admission = query("count(" + admission + ")")
    if len(current_admission) != 1 or finite(current_admission[0]["value"][1]) < 1:
        fail("legacy admission metric is absent; absence is not zero")
    if mode == "preflight":
        print("M6 preflight ready: current deployment/source/zero-series coverage verified; observation_window=not_started")
        return
    traffic = check_raw_window(selectors, intervals, start, end, source)
    # Supplementary Worker intake is fixed to this same historical end. It does
    # not have a reviewed Worker inventory and cannot sign off its zero usage.
    admission_duration = str(math.ceil(end - start + 2 * max(intervals.values()))) + "s"
    admission_increase = query("sum(increase(" + admission + "[" + admission_duration + "]))", end=end)
    if len(admission_increase) != 1:
        fail("legacy admission observation history is missing")
    admission_value = finite(admission_increase[0]["value"][1])
    if admission_value < 0: fail("negative legacy admission increase")
    print("M6\tevaluation_legacy_binding\t" + ("active_compatibility" if admission_value > 0 else "supplementary_zero"))
    required = [("bearer", all_instances), ("user_id", all_instances), ("account_id", all_instances),
                ("org_id", all_instances), ("task", collection_instances), ("definition", api_instances)]
    for contract, expected in required:
        if any((contract, key) not in traffic for key in expected):
            fail("required verified post-T0 traffic is insufficient; baseline-only growth is not window coverage")
    if any(("definition_unreadable", key) in traffic for key in api_instances):
        fail("Definition unknown_input: unreadable request growth cannot prove absence of retired fields; no zero candidate")
    for name in ("claims_extra", "bare_authorization", "task_token_route", "definition_derived_input"):
        classification = "active_compatibility" if any((name, key) in traffic for key in all_instances) else "zero_window_candidate"
        print("M6\t" + name + "\t" + classification)
    print("M6 window evidence audited read-only; client/data-owner and business exit approval remain separate")


try:
    main()
except (ValueError, KeyError, TypeError, json.JSONDecodeError, subprocess.SubprocessError) as error:
    print("M6 observation unavailable: " + str(error), file=sys.stderr)
    sys.exit(1)
PY_M6_OBSERVATION
