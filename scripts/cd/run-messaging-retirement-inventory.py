"""Existing SSH/Docker transport, fixed owners and read-only inventory only."""
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import subprocess

owner = os.environ["INVENTORY_OWNER"]
containers = {"iam.mysql": "iam-apiserver", "qs.mysql": "qs-apiserver", "ai.mysql": "qs-ai"}
if owner not in containers or not re.fullmatch(r"[0-9a-f]{40}", os.environ["GITHUB_SHA"]):
    raise SystemExit("Explicit fixed owner and source required")

if owner == "ai.mysql":
    # Reuse the installed, tested historical messaging audit and its TLS policy.
    # It returns hashes/metadata, never body, wire, model responses or credentials.
    payload = b"""
import asyncio,json,os
from qs_ai.infrastructure.persistence.mysql.database import Database,Transactions
from qs_ai.maintenance.messaging_audit import inventory
async def run():
    url=os.environ.get('QS_AI_DATABASE_URL')
    if not url or not url.startswith('mysql+asyncmy://'):
        raise ValueError('Explicit application database required')
    db=Database(url,pool_size=1,max_overflow=0)
    try:
        result=await inventory(Transactions(db),max_rows=10000)
    finally:
        await db.close()
    print(json.dumps(dict(read_only=True,mutation=False,owner='ai.mysql',
        inventory_complete=result['complete'],drop_authorized=False,
        original_body_export_verified=False,business_responsibility_closed=False,
        writers_excluded=False,result=result)))
try:
    asyncio.run(run())
except Exception:
    raise SystemExit('Read-only AI inventory unavailable; no deletion authorized') from None
"""
    args = ["sudo", "-n", "docker", "exec", "--user", "2000:2000", "-i", containers[owner], "python", "-"]
else:
    binary = Path("inventory-companion/messaging-retirement-inventory").read_bytes()
    if hashlib.sha256(binary).hexdigest() != os.environ["EXPECTED_DIGEST"]:
        raise SystemExit("Inventory companion identity mismatch")
    run_id = os.environ["GITHUB_RUN_ID"]
    if not run_id.isdecimal():
        raise SystemExit("Explicit run identity required")
    temporary = "/tmp/messaging-retirement-inventory-" + run_id
    script = ("set -eu; umask 077; p=" + shlex.quote(temporary)
              + '; trap \'rm -f "$p"\' EXIT; cat > "$p"; chmod 0700 "$p"; "$p" '
              + shlex.quote(owner))
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
            assert value["transaction_released"] is True and value["pool_released"] is True
    value["process_completed_successfully"] = result.returncode == 0
    if result.returncode:
        value["inventory_complete"] = False
    value["diagnostic_source_sha"] = os.environ["GITHUB_SHA"]
    value["workflow_run"] = os.environ["GITHUB_RUN_ID"]
    Path("messaging-retirement-inventory.json").write_text(json.dumps(value, indent=2) + "\n")
except (ValueError, KeyError, AssertionError):
    raise SystemExit("Read-only inventory unavailable; no complete or empty state inferred") from None
if result.returncode or not value["inventory_complete"]:
    raise SystemExit("Read-only inventory incomplete; no deletion authorized")
