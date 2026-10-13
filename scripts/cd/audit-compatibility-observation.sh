#!/usr/bin/env bash
set -Eeuo pipefail

WINDOW_DAYS="${WINDOW_DAYS:-30}"
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
  if [ -n "$PRIVILEGE_RUNNER" ]; then
    "$PRIVILEGE_RUNNER" "$@"
  else
    "$@"
  fi
}

parse_single_vector_value() {
  "$PYTHON_BIN" -c '
import json
import sys
from decimal import Decimal, InvalidOperation

payload = json.load(sys.stdin)
if payload.get("status") != "success":
    raise SystemExit("Prometheus query did not succeed")
result = payload.get("data", {}).get("result", [])
if not result:
    print("absent")
    raise SystemExit(0)
if len(result) != 1:
    raise SystemExit(f"Prometheus query returned {len(result)} series, want exactly one")
try:
    value = Decimal(result[0]["value"][1])
except (InvalidOperation, KeyError, IndexError, TypeError) as exc:
    raise SystemExit(f"Prometheus query returned an invalid value: {exc}")
print(format(value, "f"))
'
}

prometheus_query() {
  local query="$1"
  local response
  response="$(run_privileged "$DOCKER_BIN" exec "$APISERVER_CONTAINER" \
    wget -qO- -T "$QUERY_TIMEOUT_SECONDS" \
    --post-data="query=$query" \
    "${PROMETHEUS_BASE_URL%/}/api/v1/query")"
  printf '%s' "$response" | parse_single_vector_value
}

is_zero() {
  "$PYTHON_BIN" -c '
import sys
from decimal import Decimal
raise SystemExit(0 if Decimal(sys.argv[1]) == 0 else 1)
' "$1"
}

require_positive_integer WINDOW_DAYS "$WINDOW_DAYS"
require_positive_integer QUERY_TIMEOUT_SECONDS "$QUERY_TIMEOUT_SECONDS"
if [ "$WINDOW_DAYS" -gt 90 ]; then
  echo "WINDOW_DAYS must not exceed 90, got: $WINDOW_DAYS" >&2
  exit 1
fi
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

metrics=(
  "actor_practitioners|qs_actor_deprecated_practitioner_route_total"
  "statistics_validate_only|qs_statistics_deprecated_validate_only_total"
  "interpretation_generate_from_assessment|qs_interpretation_deprecated_generate_report_from_assessment_total"
  "survey_legacy_idempotency_lookup|qs_answersheet_legacy_idempotency_fallback_lookup_total"
  "survey_legacy_idempotency_hit|qs_answersheet_legacy_idempotency_fallback_hit_total"
  "evaluation_legacy_binding|qs_evaluation_assessment_intake_legacy_binding_total"
)

echo "Compatibility observation (Prometheus read-only):"
printf 'window_days\tmetric\tcurrent_series\tanchor_series\twindow_increase\tclassification\n'
for item in "${metrics[@]}"; do
  IFS='|' read -r name metric <<<"$item"
  current_series="$(prometheus_query "count(${metric})")"
  if [ "$current_series" = "absent" ] || is_zero "$current_series"; then
    echo "Compatibility metric is absent from the current Prometheus snapshot: $metric" >&2
    exit 1
  fi

  anchor_series="$(prometheus_query "count(${metric} offset ${WINDOW_DAYS}d)")"
  increase="$(prometheus_query "sum(increase(${metric}[${WINDOW_DAYS}d]))")"
  if [ "$increase" = "absent" ]; then
    echo "Compatibility metric has no samples in the requested window: $metric" >&2
    exit 1
  fi

  classification="active_compatibility"
  if [ "$anchor_series" = "absent" ] || is_zero "$anchor_series"; then
    classification="observation_window_incomplete"
  elif is_zero "$increase"; then
    classification="zero_window_candidate"
  fi
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$WINDOW_DAYS" "$name" "$current_series" "$anchor_series" "$increase" "$classification"
done

echo "Compatibility observation completed; counter semantics and caller/data-owner confirmation are still required before removal."

export WINDOW_DAYS OBSERVATION_MODE EXPECTED_SOURCE_SHA EXPECTED_APISERVER_CONTAINERS EXPECTED_COLLECTION_CONTAINERS
export DOCKER_BIN PRIVILEGE_RUNNER APISERVER_CONTAINER PROMETHEUS_BASE_URL QUERY_TIMEOUT_SECONDS
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
    if os.environ["PRIVILEGE_RUNNER"]:
        command.insert(0, os.environ["PRIVILEGE_RUNNER"])
    result = subprocess.run(command, check=True, capture_output=True, text=True,
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
    series = keyed(query(expression, start, end, step), expected)
    size = (end - start) // step + 1
    for item in series.values():
        samples = item["values"]
        if len(samples) != size:
            fail("observation history incomplete or disappeared within the window")
        for index, (timestamp, value) in enumerate(samples):
            if abs(finite(timestamp) - (start + index * step)) > 0.01 or finite(value) != count:
                fail("scrape/source/series coverage gap within the observation window")


def main():
    mode = os.environ["OBSERVATION_MODE"]
    if mode not in {"preflight", "window"}:
        fail("OBSERVATION_MODE must be preflight or window")
    source = os.environ["EXPECTED_SOURCE_SHA"]
    if re.fullmatch(r"[0-9a-f]{40}", source) is None:
        fail("compiled source approval missing: EXPECTED_SOURCE_SHA must be the reviewed full SHA; window not started")
    days = int(os.environ["WINDOW_DAYS"])
    if mode == "window" and not 14 <= days <= 30:
        fail("retirement observation requires 14-30 complete days")
    end = int(time.time())
    start = end - days * 86400
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
        if mode == "window":
            fresh = "(max by(job,instance)(time()-timestamp(" + selector + ")) < bool " + str(max_age) + ")"
            check_continuity("(" + aggregate + ") * on(job,instance) " + fresh, expected, count, start, end, step)
            if not info:
                resets = "sum by(job,instance)(resets(" + selector + "[" + str(days) + "d]))"
                exact_values(current_values(resets, expected), 0, "counter reset within the observation window; cannot classify as zero")
    up = 'up{job=~"qs-apiserver|qs-collection-server"}'
    exact_values(current_values(up, all_instances), 1, "service scrape is not up")
    if mode == "preflight":
        print("M6 preflight ready: current deployment/source/zero-series coverage verified; observation_window=not_started")
        return
    up_fresh = "(" + up + " == 1) * on(job,instance) ((time()-timestamp(" + up + ")) < bool " + str(max_age) + ")"
    check_continuity(up_fresh, all_instances, 1, start, end, step)
    canonical = [
        ('qs_auth_token_source_total{job=~"qs-apiserver|qs-collection-server",source="bearer",result="accepted"}', all_instances),
        ('qs_plan_entry_route_total{job="qs-collection-server",route="canonical",result="accepted"}', collection_instances),
        ('qs_model_definition_write_total{job="qs-apiserver",input="canonical",result="accepted"}', api_instances),
    ]
    for field in ("user_id", "account_id", "org_id"):
        canonical.append(('qs_auth_claim_source_total{job=~"qs-apiserver|qs-collection-server",field="' + field + '",source="canonical"}', all_instances))
    for selector, expected in canonical:
        expression = "sum by(job,instance)(increase(" + selector + "[" + str(days) + "d]))"
        if any(value <= 0 for value in current_values(expression, expected).values()):
            fail("canonical traffic does not cover every deployed instance and contract")
    legacy = [
        ('claims_extra', 'qs_auth_claim_source_total{job=~"qs-apiserver|qs-collection-server",source=~"extra_.*"}', all_instances),
        ('bare_authorization', 'qs_auth_token_source_total{job=~"qs-apiserver|qs-collection-server",source="bare"}', all_instances),
        ('task_token_route', 'qs_plan_entry_route_total{job="qs-collection-server",route="legacy_token"}', collection_instances),
        ('definition_derived_input', 'qs_model_definition_write_total{job="qs-apiserver",input="derived"}', api_instances),
    ]
    for name, selector, expected in legacy:
        expression = "sum by(job,instance)(increase(" + selector + "[" + str(days) + "d]))"
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
