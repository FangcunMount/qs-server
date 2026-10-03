"""Read actual original results/resources; missing observations fail, never zero."""
import base64
import hashlib
import json
import re
import sys
from datetime import datetime
from pathlib import Path

root = Path(sys.argv[1])
errors = []
report = {"passed": False, "errors": errors, "formal_acceptance_closed": False}


def require(condition, message):
    if not condition:
        errors.append(message)


def stamp(value):
    return datetime.fromisoformat(value)


def bytes_value(value):
    m = re.fullmatch(r"([0-9.]+)(B|kB|KB|KiB|MB|MiB|GB|GiB)", value.strip())
    if not m:
        raise ValueError("unknown memory unit: " + value)
    factors = {"B": 1, "kB": 1000, "KB": 1000, "KiB": 1024, "MB": 10**6,
               "MiB": 2**20, "GB": 10**9, "GiB": 2**30}
    return float(m[1]) * factors[m[2]]


try:
    contract = json.loads((root / "contract.json").read_text())
    result = json.loads((root / "business-result.json").read_text())
    log = (root / "execution.log").read_text()
    require("--- PASS: TestM4ConfirmSustainedSQLAndSuccessfulOutcomes" in log
            and "--- FAIL:" not in log and "--- SKIP:" not in log, "real complete PASS missing")
    require((root / "runtime-head.txt").read_text().strip() == contract["runtime_head"], "runtime mismatch")
    require((root / "runtime-sdk.txt").read_text().strip() == "github.com/FangcunMount/reliable-messaging " + contract["sdk"], "SDK mismatch")
    for key in ("count", "spacing_ms"):
        require(result[key] == contract[key], key + " mismatch")
    require(result["input_span_seconds"] >= contract["input_span_min_seconds"], "input span")
    require(result["input_rate"] >= contract["input_rate_min"], "input rate")
    require(result["fault_duration_seconds"] >= contract["actual_confirm_fault_min_seconds"], "fault duration")
    require(result["recovery_ms"] <= 1000 * contract["recovery_max_seconds"], "recovery duration")
    ids = result["original_ids"]
    require(len(ids) == len(set(ids)) == contract["count"], "original IDs")
    for key in ("runs", "outcomes", "outcome_intents", "original_request_intents", "resolver_calls"):
        require(result[key] == contract["count"], key + " uniqueness")
    drops = result["actual_drops"]
    require(len(drops) >= contract["actual_dropped_packets_min"], "actual drops")
    require((stamp(drops[-1]["at"]) - stamp(drops[0]["at"])).total_seconds() >= contract["actual_dropped_packet_span_min_seconds"], "actual packet fault span")
    for drop in drops:
        require(stamp(result["fault_at"]) <= stamp(drop["at"]) <= stamp(result["restored_at"]), "drop outside original fault window")
        require("UPDATE rm_outbox" in drop["actual_sql"] and "state='published'" in drop["actual_sql"], "not actual Confirm SQL")
    rows = result["intent_rows"]
    deliveries = result["deliveries"]
    require(len(rows) == len(deliveries) == len(set(row["MessageID"] for row in rows)) == 2 * contract["count"], "logical intent/receipt identities")
    for event_type in ("evaluation.requested", "evaluation.outcome.committed"):
        require(sum(row["EventType"] == event_type for row in rows) == contract["count"], "per-event intent count: " + event_type)
    physical = 0
    assessment_ids = set()
    for row in rows:
        wire = json.loads(base64.b64decode(row["Payload"]))
        body = base64.b64decode(wire["payload"])
        require(wire["uuid"] == row["MessageID"], "stored envelope identity")
        require(wire["metadata"]["event_type"] == row["EventType"], "stored event type")
        event = json.loads(body)
        require(event["id"] == row["MessageID"], "domain identity")
        if row["EventType"] == "evaluation.requested":
            assessment_ids.add(event["data"]["assessment_id"])
        ds = deliveries[row["MessageID"]]
        require(len(ds) > 0 and len({d["PhysicalID"] for d in ds}) == len(ds), "physical receipt identity")
        for d in ds:
            require(d["ID"] == row["MessageID"] and d["EventType"] == row["EventType"] and base64.b64decode(d["Payload"]) == body, "decoded receipt differs from original stored envelope")
        physical += len(ds)
    require(assessment_ids == set(ids), "original Assessment mapping")
    require(physical == result["physical_deliveries"] and result["duplicated_event_ids"] > 0, "physical/duplicate counts")
    channel = result["final_channel"]
    require(channel["message_count"] == physical and channel["e2e_processing_latency"]["count"] == physical, "physical FIN samples")
    require(all(channel[k] == 0 for k in ("depth", "in_flight_count", "deferred_count", "requeue_count", "timeout_count")), "native broker final state")
    require(len(channel["e2e_processing_latency"]["percentiles"]) == 3, "FIN latency percentiles")

    metrics = root / "metrics"
    containers = [json.loads(line) for line in (metrics / "containers.jsonl").read_text().splitlines()]
    mysql = next(c for c in containers if c["Config"]["Labels"].get("com.docker.compose.service") == "mysql")
    mysql_name = mysql["Name"].lstrip("/")
    expected_names = {c["Name"].lstrip("/") for c in containers}
    driver = next(c for c in containers if c["Name"].endswith("-driver-1"))
    require(driver["HostConfig"]["Memory"] == driver["HostConfig"]["NanoCpus"] == 0, "driver newly capped")
    groups = {}
    resources = {}
    for line in (metrics / "docker-stats.jsonl").read_text().splitlines():
        at, encoded = line.split("\t", 1)
        row = json.loads(encoded)
        groups.setdefault(at, set()).add(row["Name"])
        resources.setdefault(row["Name"], []).append(row)
    sql = {}
    for line in (metrics / "mysql-status.tsv").read_text().splitlines():
        at, key, value = line.split("\t")
        sql.setdefault(at, {})[key] = int(value)
    nsq = {}
    for line in (metrics / "nsqd-status.jsonl").read_text().splitlines():
        at, encoded = line.split("\t", 1)
        nsq[at] = json.loads(encoded)
    complete = sorted(stamp(at) for at, names in groups.items() if names == expected_names and at in sql and at in nsq
                      and {"Threads_connected", "Innodb_row_lock_waits"}.issubset(sql[at]))
    require(len(complete) >= contract["resource_complete_groups_min"], "complete resource groups")
    span = (complete[-1] - complete[0]).total_seconds() if complete else None
    gap = max((b-a).total_seconds() for a, b in zip(complete, complete[1:])) if len(complete) > 1 else None
    require(span is not None and span >= contract["resource_complete_span_min_seconds"], "complete resource span")
    require(gap is not None and gap <= contract["resource_complete_gap_max_seconds"], "complete resource gap")
    cpu = max(float(row["CPUPerc"].rstrip("%")) for row in resources[mysql_name])
    memory = max(bytes_value(row["MemUsage"].split("/")[0]) for row in resources[mysql_name])
    connections = max(row["Threads_connected"] for row in sql.values())
    ordered_sql = [sql[at] for at in sorted(sql)]
    lock_delta = ordered_sql[-1]["Innodb_row_lock_waits"] - ordered_sql[0]["Innodb_row_lock_waits"]
    require(cpu <= contract["mysql_cpu_max_percent"], "MySQL CPU")
    require(memory <= contract["mysql_memory_max_bytes"], "MySQL memory")
    require(connections <= contract["mysql_connections_max"], "MySQL connections")
    require(0 <= lock_delta <= contract["mysql_row_lock_waits_delta_max"], "MySQL row lock waits")
    report.update(resources={"complete_groups": len(complete), "span_seconds": span, "max_gap_seconds": gap,
                             "mysql_cpu_max_percent": cpu, "mysql_memory_max_bytes": memory,
                             "mysql_connections_max": connections, "mysql_row_lock_waits_delta": lock_delta},
                  business={k: result[k] for k in ("count", "input_span_seconds", "input_rate", "recovery_ms", "physical_deliveries", "duplicated_event_ids")})
except Exception as exc:
    errors.append(type(exc).__name__ + ": " + str(exc))
report["passed"] = not errors
(root / "verification.json").write_text(json.dumps(report, indent=2) + "\n")
print(json.dumps(report, indent=2))
sys.exit(0 if report["passed"] else 1)
