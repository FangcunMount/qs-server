#!/usr/bin/env bash
# One-shot production audit. Only aggregate categories are emitted to Actions.
set -Eeuo pipefail
python3 - <<'PY'
import hashlib
import json
import os
import re
import subprocess
from datetime import datetime, timedelta, timezone
from urllib.parse import quote


def fail(category):
    raise SystemExit("QS-03 read-only audit failed: " + category)


def run(args, *, input_text=None, timeout=15):
    return subprocess.run(args, input=input_text, text=True, capture_output=True, timeout=timeout)


expected = os.environ.get("EXPECTED_API_SHA", "")
binary_sha = os.environ.get("EXPECTED_BINARY_SHA256", "")
run_id = os.environ.get("AUDIT_RUN_ID", "")
raw_after = os.environ.get("AFTER_ID", "")
raw_upper = os.environ.get("UPPER_ID", "")
raw_pages = os.environ.get("MAX_PAGES", "")
cutoff_raw = os.environ.get("ACCEPTED_BEFORE", "")
if not re.fullmatch(r"[0-9a-f]{40}", expected):
    fail("invalid_api_sha")
if not re.fullmatch(r"[0-9a-f]{64}", binary_sha):
    fail("invalid_binary_sha")
if not re.fullmatch(r"[0-9]{1,20}", run_id):
    fail("invalid_run_id")
if not all(re.fullmatch(r"[0-9]{1,20}", value) for value in (raw_after, raw_upper)):
    fail("invalid_id_bounds")
after_id, upper_id = int(raw_after), int(raw_upper)
if not 0 <= after_id < upper_id <= 2**64 - 1:
    fail("invalid_id_bounds")
if not re.fullmatch(r"(?:[1-9]|1[0-9]|20)", raw_pages):
    fail("invalid_page_limit")
if not re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z", cutoff_raw):
    fail("invalid_cutoff")
try:
    cutoff = datetime.strptime(cutoff_raw, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=timezone.utc)
except ValueError:
    fail("invalid_cutoff")
if cutoff > datetime.now(timezone.utc) - timedelta(minutes=10):
    fail("cutoff_too_recent")

binary = "/tmp/qs-answersheet-gap-audit-" + run_id
try:
    checksum = hashlib.sha256()
    with open(binary, "rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            checksum.update(chunk)
    digest = checksum.hexdigest()
    if digest != binary_sha:
        fail("binary_hash_mismatch")
    os.chmod(binary, 0o755)

    docker = ["docker"]
    inspected = run([*docker, "inspect", "qs-apiserver"])
    if inspected.returncode != 0:
        docker = ["sudo", "-n", "docker"]
        inspected = run([*docker, "inspect", "qs-apiserver"])
    if inspected.returncode != 0:
        fail("api_inspect_failed")
    containers = json.loads(inspected.stdout)
    if len(containers) != 1:
        fail("api_count_changed")
    container = containers[0]
    if not container.get("State", {}).get("Running") or container.get("State", {}).get("Health", {}).get("Status") != "healthy":
        fail("api_unhealthy")
    if not container.get("Config", {}).get("Image", "").endswith(":" + expected):
        fail("api_image_changed")
    values = dict(item.split("=", 1) for item in container["Config"]["Env"] if "=" in item)
    mysql_host = values.get("QS_APISERVER_MYSQL_HOST", "")
    mysql_user = values.get("QS_APISERVER_MYSQL_USERNAME", "")
    mysql_password = values.get("QS_APISERVER_MYSQL_PASSWORD", "")
    mysql_db = values.get("QS_APISERVER_MYSQL_DATABASE", "")
    mongo_host = values.get("QS_APISERVER_MONGODB_HOST", "")
    mongo_user = values.get("QS_APISERVER_MONGODB_USERNAME", "")
    mongo_password = values.get("QS_APISERVER_MONGODB_PASSWORD", "")
    mongo_db = values.get("QS_APISERVER_MONGODB_DATABASE", "")
    if not all((mysql_host, mysql_user, mysql_password, mysql_db, mongo_host, mongo_user, mongo_password, mongo_db)):
        fail("database_environment_missing")
    if not re.fullmatch(r"[A-Za-z0-9_.-]+", mysql_db) or not re.fullmatch(r"[A-Za-z0-9_.-]+", mongo_db):
        fail("database_name_invalid")
    if not re.fullmatch(r"[A-Za-z0-9_.:-]+", mysql_host) or not re.fullmatch(r"[A-Za-z0-9_.:-]+", mongo_host):
        fail("database_host_invalid")
    if ":" not in mysql_host:
        mysql_host += ":3306"
    mysql_dsn = f"{mysql_user}:{mysql_password}@tcp({mysql_host})/{mysql_db}?parseTime=true&loc=UTC"
    mongo_uri = f"mongodb://{quote(mongo_user, safe='')}:{quote(mongo_password, safe='')}@{mongo_host}/{quote(mongo_db, safe='')}"
    command = [
        *docker, "run", "--rm", "-i", "--pull=never", "--read-only", "--cap-drop=ALL",
        "--security-opt=no-new-privileges", "--pids-limit=64", "--memory=512m",
        "--network", "container:qs-apiserver", "--mount", f"type=bind,source={binary},target=/audit,readonly",
        "--env", "RM_QS_GAP_READ_ONLY=1",
    ]
    command += [
        "--entrypoint", "/audit", "mongo:7.0",
        "--after-id", str(after_id), "--upper-id", str(upper_id),
        "--accepted-before", cutoff_raw, "--page-size", "100", "--max-pages", raw_pages,
        "--connections-stdin",
    ]
    credentials = json.dumps({"mysql_dsn": mysql_dsn, "mongo_uri": mongo_uri, "mongo_db_name": mongo_db})
    result = run(command, input_text=credentials, timeout=180)
    if result.returncode != 0:
        fail("scanner_nonzero")
    try:
        report = json.loads(result.stdout)
    except json.JSONDecodeError:
        fail("scanner_report_invalid")
    if not isinstance(report, dict) or report.get("after_id") != after_id or report.get("upper_id") != upper_id:
        fail("scanner_bounds_mismatch")
    if not isinstance(report.get("scanned"), int) or not isinstance(report.get("complete"), bool):
        fail("scanner_report_invalid")
    counts = report.get("counts")
    allowed = {"assessment_present", "not_required", "missing_confirmed", "delivery_pending", "unknown", "manual_required"}
    if not isinstance(counts, dict) or not set(counts).issubset(allowed) or any(not isinstance(value, int) or value < 0 for value in counts.values()):
        fail("scanner_categories_invalid")
    public_summary = {
        "accepted_before_utc": report.get("accepted_before_utc"),
        "pages": report.get("pages"),
        "scanned": report["scanned"],
        "complete": report["complete"],
        "counts": counts,
    }
    print("QS-03 read-only audit: " + json.dumps(public_summary, sort_keys=True))
    if not report["complete"]:
        fail("audit_incomplete_private_cursor_required")
    if any(counts.get(name, 0) for name in ("missing_confirmed", "unknown", "manual_required")):
        fail("actionable_gap_detected")
finally:
    try:
        os.remove(binary)
    except FileNotFoundError:
        pass
PY
