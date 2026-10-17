#!/usr/bin/env python3
"""Prove raw observation sample semantics with isolated native Prometheus.

The query artifact comes from the fail-closed shell contract test. Three actual
scrape targets produce independent, Prometheus-generated up samples. This proves
the parser/API and short native sampling regressions, not production history or
an approved 14-day retirement window.
"""
import argparse
import http.server
import itertools
import json
import math
import os
import platform
import subprocess
import tempfile
import threading
import time
import urllib.parse
import urllib.request
from pathlib import Path


SOURCE = "0123456789abcdef0123456789abcdef01234567"
INSTANCES = [("qs-apiserver", "qs-apiserver:8080"),
             ("qs-collection-server", "172.20.0.25:8080"),
             ("qs-collection-server", "172.20.0.26:8080")]
FAMILIES = ("qs_auth_compatibility_observation_info",
            "qs_plan_entry_compatibility_observation_info",
            "qs_definition_compatibility_observation_info",
            "qs_auth_token_source_total", "qs_auth_claim_source_total",
            "qs_plan_entry_route_total", "qs_model_definition_write_total",
            "qs_model_definition_input_field_total")
CADENCE = {key: 1.0 for key in INSTANCES}
JOBS = '{job=~"qs-apiserver|qs-collection-server"}'
# Native Docker requests may need most of an interval under the 1-CPU limit.
# Read only a settled cutoff, independently of the logical end being proved.
QUERY_LAG = 1.0


def metrics(accepted, key, bare=0, defect=None):
    lines = []

    def sample(name, job, instance, value, **labels):
        if name == "qs_auth_token_source_total" and labels == {"source": "bare", "result": "accepted"}:
            if defect == "missing_counter":
                return
            if defect == "unknown_combo":
                labels["source"] = "unknown"
        labels = {"job": job, "instance": instance, **labels}
        encoded = ",".join(name + "=" + json.dumps(value) for name, value in labels.items())
        lines.append(name + "{" + encoded + "} " + str(value))

    job, instance = key
    auth_source = "f" * 40 if defect == "wrong_source" else SOURCE
    accepted = 0 if defect == "counter_reset" else accepted
    sample("qs_auth_compatibility_observation_info", job, instance, 1,
           source_sha=auth_source, capability="1")
    for source, result in itertools.product(
            ("bearer", "bare", "query", "cookie", "missing", "unavailable"),
            ("accepted", "rejected")):
        value = accepted if (source, result) == ("bearer", "accepted") else 0
        if (source, result) == ("bare", "accepted"):
            value = bare
        sample("qs_auth_token_source_total", job, instance, value,
               source=source, result=result)
    for field, sources in {
            "user_id": ("canonical", "missing", "extra_user_id", "extra_uid", "extra_sub"),
            "account_id": ("canonical", "missing", "extra_account_id", "extra_accountId", "extra_aid"),
            "org_id": ("canonical", "missing", "extra_org_id", "extra_organization_id")}.items():
        for source in sources:
            # Fresh verified IAM identities legitimately omit optional OrgID.
            selected = "missing" if field == "org_id" else "canonical"
            sample("qs_auth_claim_source_total", job, instance,
                   accepted if source == selected else 0, field=field, source=source)
    if job == "qs-collection-server":
        sample("qs_plan_entry_compatibility_observation_info", job, instance, 1,
               source_sha=SOURCE, capability="1")
        for route, result in itertools.product(("canonical", "legacy_token"), ("accepted", "rejected")):
            sample("qs_plan_entry_route_total", job, instance,
                   accepted if (route, result) == ("canonical", "accepted") else 0,
                   route=route, result=result)
    else:
        sample("qs_definition_compatibility_observation_info", job, instance, 1,
               source_sha=SOURCE, capability="1")
        for input_kind, result in itertools.product(("canonical", "derived", "unreadable"), ("accepted", "rejected")):
            sample("qs_model_definition_write_total", job, instance,
                   accepted if (input_kind, result) == ("canonical", "accepted") else 0,
                   input=input_kind, result=result)
        for field, presence, result in itertools.product(
                ("DecisionSpec", "InterpretationAssets"), ("present", "absent", "unreadable"),
                ("accepted", "rejected")):
            sample("qs_model_definition_input_field_total", job, instance,
                   accepted if (presence, result) == ("absent", "accepted") else 0,
                   field=field, presence=presence, result=result)
        # This retained Worker metric remains supplementary to the new contracts.
        for result in ("resolved", "not_found", "dependency_error", "unavailable"):
            sample("qs_evaluation_assessment_intake_legacy_binding_total", "qs-worker", "worker:8081", 0,
                   result=result)
    return ("\n".join(lines) + "\n").encode()


def request(base, endpoint, params=None):
    data = urllib.parse.urlencode(params).encode() if params else None
    with urllib.request.urlopen(base + "/api/v1/" + endpoint, data=data, timeout=20) as response:
        payload = json.load(response)
    if payload.get("status") != "success" or payload.get("warnings"):
        raise RuntimeError("native Prometheus query returned incomplete evidence")
    return payload["data"]


def raw(base, metric, begin, end):
    # The production helper filters this wider left-open API range to each
    # target's exact baseline; all points retain actual scrape timestamps.
    duration = math.ceil(end - begin) + 1
    result = request(base, "query", {"query": metric + JOBS + "[" + str(duration) + "s]", "time": end})
    if result["resultType"] != "matrix":
        raise RuntimeError("native raw selector did not return a matrix")
    return result["result"]


def matrices(base, begin, end):
    return raw(base, "up", begin, end), {name: raw(base, name, begin, end) for name in FAMILIES}


def audit_helper():
    path = Path(__file__).with_name("audit-compatibility-observation.sh")
    embedded = path.read_text().split("<<'PY_M6_OBSERVATION'\n", 1)[1].rsplit("\nPY_M6_OBSERVATION", 1)[0]
    prefix = embedded.rsplit("\ntry:\n    main()", 1)[0]
    namespace = {"__name__": "native_observation_audit"}
    exec(compile(prefix, str(path), "exec"), namespace)
    return namespace["audit_raw_chunk"]


def rejected(helper, up, families, t0, end, name):
    try:
        helper(up, families, CADENCE, t0, end, SOURCE, {})
    except ValueError:
        return
    raise RuntimeError("native raw audit accepted " + name)


def wait_until(predicate, description, timeout=10):
    deadline = time.monotonic() + timeout
    while not predicate():
        if time.monotonic() > deadline:
            raise RuntimeError("timed out waiting for " + description)
        time.sleep(0.1)


def close_window(helper, base, t0, end, state):
    closing = {}

    def ready():
        closing["time"] = time.time() - QUERY_LAG
        closing["up"] = raw(base, "up", end - 1, closing["time"])
        return len(closing["up"]) == 3 and all(
            any(timestamp > end for timestamp, _ in item["values"]) for item in closing["up"])

    wait_until(ready, "a real closing scrape after the pinned logical end")
    families = {name: raw(base, name, end - 1, closing["time"]) for name in FAMILIES}
    helper(closing["up"], families, CADENCE, t0, end, SOURCE, state, close=True)
    return closing["up"], families


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("queries", type=Path)
    args = parser.parse_args()
    expressions = {json.loads(line)["parameters"]["query"][0] for line in args.queries.read_text().splitlines()}
    if not expressions:
        raise RuntimeError("query artifact is empty")
    helper = audit_helper()
    fixture_state = {"accepted": 0, "bare": 0, "defect": None, "seen_defect": 0,
                     "accepted_after": None}
    lock = threading.Lock()

    class Fixture(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            if self.path not in {"/metrics/" + str(index) for index in range(len(INSTANCES))}:
                self.send_error(404)
                return
            index = int(self.path.rsplit("/", 1)[1])
            with lock:
                after = fixture_state["accepted_after"]
                if index == 0 and after is not None and time.time() > after:
                    fixture_state["accepted"] = 4
                    fixture_state["accepted_after"] = None
                defect = fixture_state["defect"] if index == 0 else None
                body = metrics(fixture_state["accepted"], INSTANCES[index], fixture_state["bare"], defect)
                if defect:
                    fixture_state["defect"] = None
                    fixture_state["seen_defect"] += 1
            self.send_response(200)
            self.send_header("Content-Type", "text/plain; version=0.0.4")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *_):
            pass

    fixture = http.server.ThreadingHTTPServer(("0.0.0.0", 0), Fixture)
    threading.Thread(target=fixture.serve_forever, daemon=True).start()
    container = "qs-compat-prometheus-" + str(os.getpid())
    created = False
    paused = False
    try:
        with tempfile.TemporaryDirectory(prefix="qs-compat-prometheus-") as directory:
            config = Path(directory) / "prometheus.yml"
            settings = ["global:", "  scrape_interval: 1s", "  scrape_timeout: 800ms", "scrape_configs:"]
            for index, (job, instance) in enumerate(INSTANCES):
                settings.extend(["  - job_name: qs-observation-proof-" + str(index),
                                 "    metrics_path: /metrics/" + str(index), "    honor_labels: true",
                                 "    static_configs:", "      - targets: ['host.docker.internal:" + str(fixture.server_port) + "']",
                                 "        labels:", "          job: " + json.dumps(job),
                                 "          instance: " + json.dumps(instance)])
            config.write_text("\n".join(settings) + "\n")
            command = ["docker", "run", "--detach", "--name", container,
                       "--label", "qs.cleanup.owner=compat-observation-native",
                       "--cpus", "1", "--memory", "512m",
                       "--tmpfs", "/prometheus:rw,nosuid,size=134217728,mode=1777",
                       "--publish", "127.0.0.1::9090", "--mount",
                       "type=bind,source=" + str(config) + ",target=/etc/prometheus/prometheus.yml,readonly"]
            if platform.system() != "Darwin":
                command += ["--add-host", "host.docker.internal:host-gateway"]
            command += ["prom/prometheus:v3.5.0", "--config.file=/etc/prometheus/prometheus.yml"]
            subprocess.run(command, check=True, capture_output=True, text=True)
            created = True
            port = subprocess.check_output(["docker", "port", container, "9090/tcp"], text=True).strip().split(":")[-1]
            base = "http://127.0.0.1:" + port

            def ready():
                try:
                    rows = raw(base, "up", time.time() - 10, time.time() - QUERY_LAG)
                    return len(rows) == 3 and all(len(item["values"]) >= 3 for item in rows)
                except (OSError, RuntimeError):
                    return False

            wait_until(ready, "three independent native scrape baselines", timeout=45)
            build_info = request(base, "status/buildinfo")
            if build_info.get("version") != "3.5.0":
                raise RuntimeError("unexpected isolated Prometheus version")
            transition = time.time()
            with lock:
                fixture_state["bare"] = 1
            time.sleep(1.4)
            for value in (1, 2, 3):
                with lock:
                    fixture_state["accepted"] = value
                time.sleep(1.3)
            pinned = time.time() - QUERY_LAG
            initial = raw(base, "qs_auth_token_source_total", transition - 4, pinned)
            bare = [item for item in initial if item["metric"]["instance"] == INSTANCES[0][1]
                    and item["metric"].get("source") == "bare" and item["metric"].get("result") == "accepted"][0]
            first_hit = min(timestamp for timestamp, value in bare["values"] if float(value) == 1)
            t0 = first_hit - 0.001
            span = math.floor(pinned - t0)
            if span < 3:
                raise RuntimeError("native first-hit proof has insufficient post-T0 samples")
            end = t0 + span
            selector = 'qs_auth_token_source_total{job="qs-apiserver",instance="qs-apiserver:8080",source="bare",result="accepted"}'
            old = request(base, "query", {"query": "increase(" + selector + "[" + str(span) + "s])", "time": end})
            extended = request(base, "query", {"query": "increase(" + selector + "[" + str(span + 2) + "s])", "time": end})
            if len(old["result"]) != 1 or float(old["result"][0]["value"][1]) != 0:
                raise RuntimeError("native unextended increase did not reproduce first-hit loss")
            if len(extended["result"]) != 1 or float(extended["result"][0]["value"][1]) <= 0:
                raise RuntimeError("native extended baseline did not retain the first legacy hit")
            up, families = matrices(base, t0 - 2, end)
            audit_state = {}
            helper(up, families, CADENCE, t0, end, SOURCE, audit_state)
            required = {(name, key) for name in ("bearer", "user_id", "account_id", "org_id") for key in INSTANCES}
            required.update(("task", key) for key in INSTANCES[1:])
            required.add(("definition", INSTANCES[0]))
            if not required <= audit_state["traffic"] or ("bare_authorization", INSTANCES[0]) not in audit_state["traffic"]:
                raise RuntimeError("native raw audit lost post-T0 canonical or first-hit legacy traffic")
            # Native API chunks overlap by one actual interval. The same helper
            # must retain their seam evidence and recover exactly the same flags.
            split = t0 + 2
            chunk_state = {}
            chunk_up, chunk_families = matrices(base, t0 - 2, split)
            helper(chunk_up, chunk_families, CADENCE, t0, split, SOURCE, chunk_state, window_end=end)
            chunk_up, chunk_families = matrices(base, split - 1, end)
            helper(chunk_up, chunk_families, CADENCE, t0, end, SOURCE, chunk_state)
            if chunk_state["traffic"] != audit_state["traffic"]:
                raise RuntimeError("native overlap chunks changed the observed traffic flags")
            close_window(helper, base, t0, end, chunk_state)
            org = families["qs_auth_claim_source_total"]
            canonical_org = [item for item in org if item["metric"].get("field") == "org_id" and item["metric"].get("source") == "canonical"]
            missing_org = [item for item in org if item["metric"].get("field") == "org_id" and item["metric"].get("source") == "missing"]
            if len(canonical_org) != 3 or any(float(value) != 0 for item in canonical_org for _, value in item["values"]):
                raise RuntimeError("native fixture contains unexpected canonical Org traffic")
            if len(missing_org) != 3 or any(float(item["values"][-1][1]) <= float(item["values"][0][1]) for item in missing_org):
                raise RuntimeError("native verified missing-Org counter did not actually increase")
            for expression in sorted(expressions):
                data = request(base, "query", {"query": expression, "time": end})
                if data["resultType"] not in {"vector", "matrix"}:
                    raise RuntimeError("unexpected native observation expression type")

            # Put a legacy increment strictly between the latest APIServer
            # scrape and logical E. No sample at/before E sees it; the genuine
            # first post-E scrape must conservatively prevent a zero candidate.
            time.sleep(3)

            def latest_api():
                rows = raw(base, "up", time.time() - 3, time.time() - 0.02)
                return max(timestamp for item in rows if item["metric"]["instance"] == INSTANCES[0][1]
                           for timestamp, _ in item["values"])

            old_api = latest_api()
            fresh_api = {}

            def fresh():
                stamp = latest_api()
                if stamp <= old_api:
                    return False
                # A query can finish well after the newly observed sample was
                # scraped. Retry on the next real tick rather than assuming a
                # 0.5-second deadline remains available after the API call.
                with lock:
                    if time.time() >= stamp + 0.3:
                        return False
                    fixture_state["bare"] = 2
                    fixture_state["accepted_after"] = stamp + 0.5
                    fresh_api["timestamp"] = stamp
                return True

            wait_until(fresh, "a fresh APIServer sample with pre-E mutation margin")
            edge_t0 = fresh_api["timestamp"] - 0.5
            edge_end = fresh_api["timestamp"] + 0.5
            # The fixture arms canonical growth for its first actual API
            # scrape after E. A delayed test thread cannot miss that scrape.
            time.sleep(max(0, edge_end + 0.25 - time.time()))
            edge_up, edge_families = matrices(base, edge_t0 - 2, edge_end)
            edge_state = {}
            helper(edge_up, edge_families, CADENCE, edge_t0, edge_end, SOURCE, edge_state)
            if ("bare_authorization", INSTANCES[0]) in edge_state["traffic"]:
                raise RuntimeError("native APIServer legacy hit was sampled before logical E")
            _, edge_closing = close_window(helper, base, edge_t0, edge_end, edge_state)
            if ("bare_authorization", INSTANCES[0]) not in edge_state["traffic"]:
                raise RuntimeError("native closing scrape lost the right-edge legacy hit")
            closing_bearer = [item for item in edge_closing["qs_auth_token_source_total"]
                              if item["metric"]["instance"] == INSTANCES[0][1]
                              and item["metric"].get("source") == "bearer"
                              and item["metric"].get("result") == "accepted"][0]
            first_post = next(float(value) for timestamp, value in closing_bearer["values"] if timestamp > edge_end)
            if first_post != 4 or any((name, INSTANCES[0]) in edge_state["traffic"]
                                      for name in ("bearer", "user_id", "account_id", "org_id", "definition")):
                raise RuntimeError("native post-E-only canonical growth was accepted as in-window traffic")

            # Each mutation is exported by one actual successful HTTP scrape;
            # fresh per-case baselines exclude earlier defective samples.
            for defect in ("missing_counter", "counter_reset", "wrong_source", "unknown_combo"):
                time.sleep(4)
                control_end = time.time() - QUERY_LAG
                control_t0 = control_end - 1
                control_up, control_families = matrices(base, control_t0 - 2, control_end)
                helper(control_up, control_families, CADENCE, control_t0, control_end, SOURCE, {})
                case_t0 = time.time()
                with lock:
                    seen = fixture_state["seen_defect"]
                    fixture_state["defect"] = defect
                wait_until(lambda: fixture_state["seen_defect"] > seen, "one native " + defect + " scrape")
                time.sleep(1.4)
                case_end = time.time() - QUERY_LAG
                case_up, case_families = matrices(base, case_t0 - 2, case_end)
                if any(float(value) != 1 for item in case_up for _, value in item["values"]):
                    targets = request(base, "targets")["activeTargets"]
                    context = {"defect": defect,
                               "bad_up": [{"labels": item["metric"], "point": point} for item in case_up
                                          for point in item["values"] if float(point[1]) != 1],
                               "targets": [{name: target.get(name) for name in
                                            ("labels", "lastError", "lastScrape", "lastScrapeDuration", "health")}
                                           for target in targets],
                               "durations": raw(base, "scrape_duration_seconds", case_t0 - 2, case_end)}
                    raise RuntimeError("native counter/source fixture expected up=1: " + json.dumps(context, sort_keys=True))
                rejected(helper, case_up, case_families, case_t0, case_end, defect)

            # Pause only the just-created isolated container. No exporter-authored
            # up metric or client-side point deletion simulates this actual gap.
            time.sleep(3)
            gap_t0 = time.time()
            subprocess.run(["docker", "pause", container], check=True, capture_output=True)
            paused = True
            time.sleep(2.2)
            subprocess.run(["docker", "unpause", container], check=True, capture_output=True)
            paused = False
            time.sleep(1.5)
            gap_end = time.time() - QUERY_LAG
            gap_up, gap_families = matrices(base, gap_t0 - 2, gap_end)
            pair = next(((left[0], right[0]) for item in gap_up
                         for left, right in zip(item["values"], item["values"][1:])
                         if right[0] - left[0] > 1.2), None)
            if pair is None:
                raise RuntimeError("isolated pause did not create a native up timestamp gap")
            grid = pair[0] + min(1.1, (pair[1] - pair[0]) / 2)
            interpolated = request(base, "query_range", {"query": "up" + JOBS, "start": grid, "end": grid, "step": 1})
            if len(interpolated["result"]) != 3 or any(float(item["values"][0][1]) != 1 for item in interpolated["result"]):
                raise RuntimeError("native range query did not reproduce up lookback interpolation")
            rejected(helper, gap_up, gap_families, gap_t0, gap_end, "actual omitted up scrape")
            print("native Prometheus: " + str(len(expressions)) + " pinned expressions parsed; version=" + build_info["version"]
                  + "; independent raw timestamps, first-T0-hit baseline, overlap seams, closing right-edge hit, post-E-only canonical excluded, post-T0 canonical/missing Org traffic, actual counter/up gaps, reset/source/label rejection verified")
    finally:
        fixture.shutdown()
        fixture.server_close()
        if created:
            if paused:
                subprocess.run(["docker", "unpause", container], check=True, capture_output=True)
            subprocess.run(["docker", "rm", "--force", "--volumes", container], check=True, capture_output=True)


if __name__ == "__main__":
    main()
