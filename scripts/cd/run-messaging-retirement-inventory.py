"""Existing SSH/Docker transport, fixed owners and read-only inventory only."""
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import subprocess

owner = os.environ["INVENTORY_OWNER"]
containers = {"iam.mysql": "iam-apiserver", "qs.mysql": "qs-apiserver", "ai.mysql": "qs-ai", "qs.mongo": "qs-apiserver"}
if owner not in containers or not re.fullmatch(r"[0-9a-f]{40}", os.environ["GITHUB_SHA"]):
    raise SystemExit("Explicit fixed owner and source required")

if owner == "ai.mysql":
    # Reuse the installed, tested historical messaging audit and its TLS policy.
    # It returns hashes/metadata, never body, wire, model responses or credentials.
    payload = b"""
import asyncio,json
phase='imports'
report=dict(read_only=True,mutation=False,owner='ai.mysql',inventory_complete=False,
    drop_authorized=False,original_body_export_verified=False,
    business_responsibility_closed=False,writers_excluded=False,
    transaction_released=None,pool_released=None,tables=None)
async def run():
    global phase
    from qs_ai.config import Settings
    from qs_ai.infrastructure.persistence.mysql.database import Database,Transactions
    from qs_ai.maintenance.messaging_audit import inventory
    phase='application_binding'
    settings=Settings()
    if settings.database_url is None or settings.database_tls_mode!='VERIFY_IDENTITY':
        raise ValueError('Explicit verified application database required')
    url=settings.database_url.get_secret_value()
    if not url.startswith('mysql+asyncmy://'):
        raise ValueError('Explicit application database required')
    phase='pool_configuration'
    db=Database(url,pool_size=1,max_overflow=0,
        tls_mode=settings.database_tls_mode,tls_ca_file=settings.database_tls_ca_file)
    try:
        phase='inventory_query'
        result=await inventory(Transactions(db),max_rows=10000)
    finally:
        before_release=phase
        phase='pool_release'
        await db.close()
        report['pool_released']=True
        phase=before_release
    report.update(inventory_complete=result['complete'],result=result)
    phase='complete' if result['complete'] else 'incomplete_inventory'
try:
    asyncio.run(run())
except Exception as error:
    driver=getattr(error,'orig',error)
    code=driver.args[0] if getattr(driver,'args',()) and type(driver.args[0]) is int else None
    report.update(error_phase=phase,error_type=type(error).__name__,driver_code=code)
    print(json.dumps(report))
    raise SystemExit(1) from None
report['phase']=phase
print(json.dumps(report))
"""
    args = ["sudo", "-n", "docker", "exec", "--user", "10001:10001", "-i", containers[owner], "/app/.venv/bin/python", "-"]
else:
    filename = "messaging-retirement-mongo-inventory" if owner == "qs.mongo" else "messaging-retirement-inventory"
    binary = (Path("inventory-companion") / filename).read_bytes()
    digest_key = "EXPECTED_MONGO_DIGEST" if owner == "qs.mongo" else "EXPECTED_DIGEST"
    if hashlib.sha256(binary).hexdigest() != os.environ[digest_key]:
        raise SystemExit("Inventory companion identity mismatch")
    run_id = os.environ["GITHUB_RUN_ID"]
    if not run_id.isdecimal():
        raise SystemExit("Explicit run identity required")
    temporary = "/tmp/messaging-retirement-inventory-" + run_id + "-" + owner.replace(".", "-")
    script = ("set -eu; umask 077; p=" + shlex.quote(temporary)
              + '; trap \'rm -f "$p"\' EXIT; cat > "$p"; chmod 0700 "$p"; "$p" '
              + shlex.quote("--messaging-retirement-inventory" if owner == "qs.mongo" else owner))
    args = ["sudo", "-n", "docker", "exec", "--user", "2000:2000", "-i", containers[owner], "sh", "-c", script]
    payload = binary

result = subprocess.run(["ssh", "-F", os.environ["RUNNER_SSH_CONFIG"],
                         os.environ["RUNNER_SSH_ALIAS"], shlex.join(args)],
                        input=payload, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=60)
try:
    value = json.loads(result.stdout)
    assert value["owner"] == owner and value["read_only"] is True and value["mutation"] is False
    assert value["drop_authorized"] is False
    if owner != "ai.mysql":
        assert value["source_sha"] == os.environ["GITHUB_SHA"]
        if value["inventory_complete"]:
            assert value["pool_released"] is True
            if owner == "qs.mongo":
                assert value["cursors_released"] is True and value["count_read_concern"] == "snapshot"
            else:
                assert value["transaction_released"] is True
    value["process_completed_successfully"] = result.returncode == 0
    if result.returncode:
        value["inventory_complete"] = False
    value["diagnostic_source_sha"] = os.environ["GITHUB_SHA"]
    value["workflow_run"] = os.environ["GITHUB_RUN_ID"]
    if owner == "qs.mysql" and value["inventory_complete"]:
        for table in value.get("tables", []):
            original = table.get("empty_publisher_export")
            if original is None:
                continue
            assert table["name"] == "ai_messaging_outbox" and table["rows"] == 0
            assert original["rows"] == 0 and original["body_wire_eof"] is True
            assert original["schema_dependency_visibility_verified"] is True
            ddl = original["original_ddl"].encode()
            assert hashlib.sha256(ddl).hexdigest() == table["show_create_table_sha256"]
            assert original["jsonl_sha256"] == hashlib.sha256(b"").hexdigest()
            # No nonempty body stream can reach an artifact. These two private
            # files are evidence of this exact empty source, not restore proof.
            for filename, data in [("messaging-retirement-original-ai-publisher.sql", ddl),
                                   ("messaging-retirement-original-ai-publisher.jsonl", b"")]:
                descriptor = os.open(filename, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
                with os.fdopen(descriptor, "wb") as exported:
                    exported.write(data)
    Path("messaging-retirement-inventory.json").write_text(json.dumps(value, indent=2) + "\n")
except (ValueError, KeyError, AssertionError, OSError):
    # Retain an unavailable receipt, without copying untrusted response contents.
    value = dict(owner=owner, read_only=None, mutation=None, read_only_requested=True,
                 inventory_complete=False, drop_authorized=False, tables=None,
                 receipt_validated=False, error_phase="transport_reply",
                 diagnostic_source_sha=os.environ["GITHUB_SHA"],
                 workflow_run=os.environ["GITHUB_RUN_ID"])
    Path("messaging-retirement-inventory.json").write_text(json.dumps(value, indent=2) + "\n")
    raise SystemExit("Read-only inventory unavailable; no complete or empty state inferred") from None
if result.returncode or not value["inventory_complete"]:
    raise SystemExit("Read-only inventory incomplete; no deletion authorized")
