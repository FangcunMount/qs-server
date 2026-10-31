"""Run the exact canonical maintenance binary in its default read-only mode."""
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import subprocess

from m5_maintenance_container import resolve_inventory_target

source = os.environ["GITHUB_SHA"]
run_id = os.environ["GITHUB_RUN_ID"]
if not re.fullmatch(r"[0-9a-f]{40}", source) or not run_id.isdecimal():
    raise SystemExit("Explicit source and run required")
binary = Path("inventory-companion/qs-migrate-offline").read_bytes()
digest = hashlib.sha256(binary).hexdigest()
if digest != os.environ["EXPECTED_OFFLINE_DIGEST"]:
    raise SystemExit("Offline inspector identity mismatch")
temporary = "/tmp/qs-offline-preview-" + run_id
script = ("set -eu; umask 077; p=" + shlex.quote(temporary)
          + '; trap \'rm -f "$p"\' EXIT; cat > "$p"; chmod 0700 "$p"; "$p" '
          + "--application-env --mysql-database=qs --mongo-database=qs --timeout=30s")
try:
    target = resolve_inventory_target("qs.mysql", "qs-apiserver")
except (ValueError, KeyError, TypeError, OSError, subprocess.TimeoutExpired):
    raise SystemExit("Owned maintenance transport unavailable; no offline plan authorized") from None
command = ["sudo", "-n", "docker", "exec", "--user", "2000:2000", "-i", target, "sh", "-c", script]
report = dict(read_only_requested=True, inventory_complete=False, mutation=None,
              source_sha=source, binary_sha256=digest, workflow_run=run_id, snapshot=None)
try:
    result = subprocess.run(["ssh", "-F", os.environ["RUNNER_SSH_CONFIG"], os.environ["RUNNER_SSH_ALIAS"], shlex.join(command)],
                            input=binary, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=60)
    output = json.loads(result.stdout)
    assert result.returncode == 0 and output["read_only"] is True
    snapshot = output["snapshot"]
    assert snapshot["source_sha"] == source
    assert snapshot["mysql_database"] == snapshot["mongo_database"] == "qs"
    assert (snapshot["mysql_version"], snapshot["mongo_version"]) == (110, 44)
    assert (snapshot["target_mysql_version"], snapshot["target_mongo_version"]) == (111, 44)
    for key in ("migration_resources_sha256", "mysql_identity_sha256", "mongo_cluster_sha256"):
        assert re.fullmatch(r"[0-9a-f]{64}", snapshot[key])
    report.update(read_only=True, mutation=False, inventory_complete=True, snapshot=snapshot)
except (ValueError, KeyError, TypeError, AssertionError, subprocess.TimeoutExpired):
    Path("messaging-retirement-offline-preview.json").write_text(json.dumps(report, indent=2) + "\n")
    raise SystemExit("Canonical read-only preview unavailable; no execution authorized") from None
Path("messaging-retirement-offline-preview.json").write_text(json.dumps(report, indent=2) + "\n")
