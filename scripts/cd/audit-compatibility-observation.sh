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


def query(expression, start=None, end=None, step=None):
    parameters = {"query": expression}
    endpoint = "query"
    if start is not None:
        endpoint = "query_range"
        parameters.update(start=start, end=end, step=step)
    data = api(endpoint, parameters)
    if data.get("resultType") != ("matrix" if start is not None else "vector"):
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


def check_continuity(expression, expected, count, start, end, step):
    # Prometheus limits query_range points per series. Six-hour chunks retain
    # the actual scrape cadence and bound both request size and memory.
    cursor = start
    span = (21600 // step) * step
    while cursor <= end:
        chunk_end = min(cursor + span, end)
        series = keyed(query(expression, cursor, chunk_end, step), expected)
        size = (chunk_end - cursor) // step + 1
        for item in series.values():
            samples = item["values"]
            if len(samples) != size:
                fail("observation history incomplete or disappeared within the window")
            for index, (timestamp, value) in enumerate(samples):
                if abs(finite(timestamp) - (cursor + index * step)) > 0.01 or finite(value) != count:
                    fail("scrape/source/series coverage gap within the observation window")
        cursor += span + step


def retention_seconds(raw):
    units = {"y": 365 * 86400, "w": 7 * 86400, "d": 86400,
             "h": 3600, "m": 60, "s": 1}
    parts = re.findall(r"([0-9]+)([ywdhms])", raw)
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
    duration = str(max(1, end - start)) + "s"
    targets = api("targets")["activeTargets"]
    inventory = {}
    intervals = []
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
            interval = re.fullmatch(r"([0-9]+)s", target.get("scrapeInterval", ""))
            if interval is None or not 15 <= int(interval[1]) <= 60:
                fail("scrape interval must be explicitly bounded to 15-60 seconds")
            intervals.append(int(interval[1]))
            last = datetime.datetime.fromisoformat(target["lastScrape"].replace("Z", "+00:00")).timestamp()
            if not 0 <= end - last <= 2 * int(interval[1]):
                fail("current scrape sample is stale")
            started = datetime.datetime.fromisoformat(metadata["started"].replace("Z", "+00:00")).timestamp()
            if mode == "window" and started > start:
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
    step = min(intervals)
    max_age = 2 * min(intervals)
    for metric, expected, count, info in family_specs:
        jobs = "|".join(sorted({key[0] for key in expected}))
        selector = metric + '{job=~"' + jobs + '"' + (',source_sha="' + source + '",capability="1"' if info else '') + '}'
        if info:
            selector = "(" + selector + " == 1)"
        aggregate = "count by(job,instance)(" + selector + ")"
        exact_values(current_values(aggregate, expected), count, "required zero-valued source/series capability is absent or partial")
        fresh = "(max by(job,instance)(time()-timestamp(" + selector + ")) < bool " + str(max_age) + ")"
        exact_values(current_values("(" + aggregate + ") * on(job,instance) " + fresh, expected), count, "current capability or counter sample is stale")
        if mode == "window":
            check_continuity("(" + aggregate + ") * on(job,instance) " + fresh, expected, count, start, end, step)
            if not info:
                resets = "sum by(job,instance)(resets(" + selector + "[" + duration + "]))"
                exact_values(current_values(resets, expected), 0, "counter reset within the observation window; cannot classify as zero")
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
    admission_increase = query("sum(increase(" + admission + "[" + duration + "]))")
    if len(admission_increase) != 1:
        fail("legacy admission observation history is missing")
    admission_value = finite(admission_increase[0]["value"][1])
    if admission_value < 0:
        fail("negative legacy admission increase")
    print("M6\tevaluation_legacy_binding\t" + ("active_compatibility" if admission_value > 0 else "supplementary_zero"))
    up_fresh = "(" + up + " == 1) * on(job,instance) ((time()-timestamp(" + up + ")) < bool " + str(max_age) + ")"
    check_continuity(up_fresh, all_instances, 1, start, end, step)
    canonical = [
        ('qs_auth_token_source_total{job=~"qs-apiserver|qs-collection-server",source="bearer",result="accepted"}', all_instances),
        ('qs_plan_entry_route_total{job="qs-collection-server",route="canonical",result="accepted"}', collection_instances),
        ('qs_model_definition_write_total{job="qs-apiserver",input="canonical",result="accepted"}', api_instances),
    ]
    for field in ("user_id", "account_id"):
        canonical.append(('qs_auth_claim_source_total{job=~"qs-apiserver|qs-collection-server",field="' + field + '",source="canonical"}', all_instances))
    # OrgID is optional in current IAM sessions. Only the already verified
    # canonical or missing selection counts; Extra still belongs to legacy below.
    canonical.append(('qs_auth_claim_source_total{job=~"qs-apiserver|qs-collection-server",field="org_id",source=~"canonical|missing"}', all_instances))
    for selector, expected in canonical:
        expression = "sum by(job,instance)(increase(" + selector + "[" + duration + "]))"
        if any(value <= 0 for value in current_values(expression, expected).values()):
            fail("required verified traffic does not cover every deployed instance and contract")
    legacy = [
        ('claims_extra', 'qs_auth_claim_source_total{job=~"qs-apiserver|qs-collection-server",source=~"extra_.*"}', all_instances),
        ('bare_authorization', 'qs_auth_token_source_total{job=~"qs-apiserver|qs-collection-server",source="bare"}', all_instances),
        ('task_token_route', 'qs_plan_entry_route_total{job="qs-collection-server",route="legacy_token"}', collection_instances),
        ('definition_derived_input', 'qs_model_definition_write_total{job="qs-apiserver",input="derived"}', api_instances),
    ]
    for name, selector, expected in legacy:
        expression = "sum by(job,instance)(increase(" + selector + "[" + duration + "]))"
        values = current_values(expression, expected)
        if any(value < 0 for value in values.values()):
            fail("negative compatibility increase is invalid evidence")
        classification = "active_compatibility" if any(value > 0 for value in values.values()) else "zero_window_candidate"
        print("M6\t" + name + "\t" + classification)
    print("M6 window evidence audited read-only; client/data-owner and business exit approval remain separate")


try:
    main()
except (ValueError, KeyError, TypeError, json.JSONDecodeError, subprocess.SubprocessError) as error:
    print("M6 observation unavailable: " + str(error), file=sys.stderr)
    sys.exit(1)
PY_M6_OBSERVATION
