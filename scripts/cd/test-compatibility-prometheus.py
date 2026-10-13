#!/usr/bin/env python3
"""Validate observation expressions with isolated native Prometheus and samples.

The query artifact comes from the fail-closed shell contract test. Synthetic
history is not accepted as production evidence; this proves parser/API and
current sample semantics only.
"""
import argparse
import http.server
import itertools
import json
import os
import platform
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


SOURCE = "0123456789abcdef0123456789abcdef01234567"
INSTANCES = [("qs-apiserver", "qs-apiserver:8080"),
             ("qs-collection-server", "172.20.0.25:8080"),
             ("qs-collection-server", "172.20.0.26:8080")]


def metrics(accepted):
    lines = []

    def sample(name, job, instance, value, **labels):
        labels = {"job": job, "instance": instance, **labels}
        encoded = ",".join(key + "=" + json.dumps(value) for key, value in labels.items())
        lines.append(name + "{" + encoded + "} " + str(value))

    for job, instance in INSTANCES:
        sample("up", job, instance, 1)
        sample("qs_auth_compatibility_observation_info", job, instance, 1,
               source_sha=SOURCE, capability="1")
        for source, result in itertools.product(
                ("bearer", "bare", "query", "cookie", "missing", "unavailable"),
                ("accepted", "rejected")):
            sample("qs_auth_token_source_total", job, instance,
                   accepted if (source, result) == ("bearer", "accepted") else 0,
                   source=source, result=result)
        for field, sources in {
                "user_id": ("canonical", "missing", "extra_user_id", "extra_uid", "extra_sub"),
                "account_id": ("canonical", "missing", "extra_account_id", "extra_accountId", "extra_aid"),
                "org_id": ("canonical", "missing", "extra_org_id", "extra_organization_id")}.items():
            for source in sources:
                sample("qs_auth_claim_source_total", job, instance,
                       accepted if source == "canonical" else 0, field=field, source=source)
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


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("queries", type=Path)
    args = parser.parse_args()
    expressions = {json.loads(line)["parameters"]["query"][0] for line in args.queries.read_text().splitlines()}
    if not expressions:
        raise RuntimeError("query artifact is empty")
    state = {"accepted": 0}

    class Fixture(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            if self.path != "/metrics":
                self.send_error(404)
                return
            body = metrics(state["accepted"])
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
    try:
        with tempfile.TemporaryDirectory(prefix="qs-compat-prometheus-") as directory:
            config = Path(directory) / "prometheus.yml"
            config.write_text("global:\n  scrape_interval: 1s\nscrape_configs:\n"
                              "  - job_name: qs-observation-proof\n    honor_labels: true\n"
                              "    static_configs:\n      - targets: ['host.docker.internal:"
                              + str(fixture.server_port) + "']\n")
            command = ["docker", "run", "--detach", "--name", container,
                       "--publish", "127.0.0.1::9090", "--mount",
                       "type=bind,source=" + str(config) + ",target=/etc/prometheus/prometheus.yml,readonly"]
            if platform.system() != "Darwin":
                command += ["--add-host", "host.docker.internal:host-gateway"]
            command += ["prom/prometheus:v3.5.0", "--config.file=/etc/prometheus/prometheus.yml"]
            subprocess.run(command, check=True, capture_output=True, text=True)
            created = True
            port = subprocess.check_output(["docker", "port", container, "9090/tcp"], text=True).strip().split(":")[-1]
            base = "http://127.0.0.1:" + port
            deadline = time.monotonic() + 45
            while True:
                try:
                    result = request(base, "query", {"query": "qs_auth_compatibility_observation_info"})
                    if len(result["result"]) == 3:
                        break
                except (OSError, RuntimeError):
                    pass
                if time.monotonic() > deadline:
                    raise RuntimeError("native Prometheus did not scrape the fixture")
                time.sleep(0.2)
            time.sleep(2)
            state["accepted"] = 1
            time.sleep(2)
            now = int(time.time())
            for expression in sorted(expressions):
                data = request(base, "query", {"query": expression})
                if data["resultType"] != "vector":
                    raise RuntimeError("unexpected native vector type")
                matrix = request(base, "query_range", {
                    "query": expression, "start": now - 2, "end": now, "step": 1})
                if matrix["resultType"] != "matrix":
                    raise RuntimeError("unexpected native matrix type")
            for metric, expected in {
                    "qs_auth_token_source_total": 12, "qs_auth_claim_source_total": 14,
                    "qs_plan_entry_route_total": 4, "qs_model_definition_write_total": 6,
                    "qs_model_definition_input_field_total": 12}.items():
                result = request(base, "query", {"query": "count by(job,instance)(" + metric + ")"})
                expected_instances = 3 if metric.startswith("qs_auth_") else (2 if "plan_entry" in metric else 1)
                if len(result["result"]) != expected_instances or any(float(item["value"][1]) != expected for item in result["result"]):
                    raise RuntimeError("native fixed-series count differs for " + metric)
            result = request(base, "query", {"query": 'increase(qs_auth_token_source_total{source="bearer",result="accepted"}[10s])'})
            if len(result["result"]) != 3 or any(float(item["value"][1]) <= 0 for item in result["result"]):
                raise RuntimeError("native increase failed to retain real canonical traffic")
            print("native Prometheus: " + str(len(expressions)) + " expressions parsed and executed; fixed series and increase verified")
    finally:
        fixture.shutdown()
        fixture.server_close()
        if created:
            subprocess.run(["docker", "rm", "--force", container], check=True, capture_output=True)


if __name__ == "__main__":
    main()
