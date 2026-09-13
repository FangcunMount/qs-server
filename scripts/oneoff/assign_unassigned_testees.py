#!/usr/bin/env python3
"""Random, balanced initial store assignment via the audited headquarters API."""
from __future__ import annotations

import argparse
from collections import Counter
from datetime import datetime, timezone
import fcntl
import hashlib
import json
import os
from pathlib import Path
import random
import secrets
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

STORE_NAMES = ("武汉店", "西安店", "南京店")


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, ensure_ascii=False,
                                     separators=(",", ":")).encode()).hexdigest()


class API:
    def __init__(self, base_url, token):
        parsed = urllib.parse.urlsplit(base_url)
        if parsed.scheme != "https" and not (parsed.scheme == "http" and parsed.hostname in ("localhost", "127.0.0.1")):
            raise ValueError("HTTPS required except for local tests")
        self.base_url, self.token = base_url.rstrip("/"), token

    def request(self, method, path, payload=None, query=None):
        url = self.base_url + path
        if query:
            url += "?" + urllib.parse.urlencode(query)
        req = urllib.request.Request(url, method=method,
            data=None if payload is None else json.dumps(payload).encode(),
            headers={"Authorization": "Bearer " + self.token, "Content-Type": "application/json"})
        # Never forward a bearer token through a redirect.
        class NoRedirect(urllib.request.HTTPRedirectHandler):
            def redirect_request(self, req, fp, code, msg, headers, newurl):
                return None
        try:
            with urllib.request.build_opener(NoRedirect).open(req, timeout=30) as resp:
                body = json.load(resp)
        except urllib.error.HTTPError as exc:
            raise RuntimeError(f"{method} {path}: HTTP {exc.code}; stopped, reconcile before resuming") from None
        if body.get("code") != 0 or "data" not in body:
            raise RuntimeError(f"{method} {path}: API rejected request (code={body.get('code')})")
        return body["data"]


def list_all(api, path, **query):
    result, seen, total = [], set(), None
    page = 1
    while True:
        data = api.request("GET", path, query={**query, "page": page, "page_size": 100})
        if total is None:
            total = data["total"]
        if data["total"] != total:
            raise ValueError("Listing changed during pagination; rerun preflight")
        items = data["items"]
        for row in items:
            key = str(row["id"])
            if key in seen:
                raise ValueError("Duplicate ID during pagination; rerun preflight")
            seen.add(key)
            result.append(row)
        if len(result) == total:
            return result
        if not items or len(result) > total:
            raise ValueError("List/count mismatch")
        page += 1


def stores(api, org_id):
    rows = list_all(api, "/stores")
    targets = []
    for name in STORE_NAMES:
        matches = [s for s in rows if s["name"] == name and str(s["org_id"]) == org_id]
        if len(matches) != 1 or matches[0]["is_active"] is not True:
            raise ValueError(f"{name}: expected exactly one active store in company {org_id}")
        s = matches[0]
        targets.append({"id": str(s["id"]), "name": name, "code": s["code"], "org_id": org_id})
    return targets


def inventory(api, org_id):
    rows = list_all(api, "/testees", unassigned_store="true")
    result = []
    for row in rows:
        if str(row["org_id"]) != org_id or row.get("store_id") is not None or row["store_version"] < 1:
            raise ValueError("Unexpected company, ownership or version in unassigned inventory")
        result.append({"testee_id": str(row["id"]), "expected_version": row["store_version"]})
    return sorted(result, key=lambda r: int(r["testee_id"]))


def make_plan(base_url, org_id, targets, rows, seed, reason):
    rows = [dict(r) for r in rows]
    random.Random(seed).shuffle(rows)
    batch = secrets.token_hex(12)
    for i, row in enumerate(rows):
        row["store_id"] = targets[i % len(targets)]["id"]
        row["request_id"] = f"random-store-{batch}-{row['testee_id']}"
    plan = {"version": 1, "batch_id": batch, "created_at": datetime.now(timezone.utc).isoformat(),
            "base_url": base_url.rstrip("/"), "org_id": org_id, "seed": seed,
            "reason": reason, "stores": targets, "rows": rows}
    return {"plan": plan, "sha256": digest(plan)}


def validate_manifest(manifest, base_url, org_id):
    p = manifest["plan"]
    if digest(p) != manifest["sha256"] or p["version"] != 1:
        raise ValueError("Manifest checksum/version invalid")
    if p["base_url"] != base_url.rstrip("/") or p["org_id"] != org_id:
        raise ValueError("Manifest API/company mismatch")
    ids = set()
    targets = {s["id"] for s in p["stores"]}
    if len(targets) != 3 or [s["name"] for s in p["stores"]] != list(STORE_NAMES):
        raise ValueError("Unexpected target stores")
    for r in p["rows"]:
        if r["testee_id"] in ids or r["store_id"] not in targets or r["expected_version"] < 1:
            raise ValueError("Invalid/duplicate manifest row")
        ids.add(r["testee_id"])
    return p


def validate_receipt(receipt, row, plan):
    if (str(receipt.get("testee_id")) != row["testee_id"] or
        str(receipt.get("to_store_id")) != row["store_id"] or
        str(receipt.get("org_id")) != plan["org_id"] or
        receipt.get("from_store_id") is not None or receipt.get("kind") != "initial" or
        receipt.get("version") != row["expected_version"] + 1 or
        receipt.get("request_id") != row["request_id"] or receipt.get("reason") != plan["reason"]):
        raise ValueError("Unexpected assignment receipt; inspect history before resuming")


def apply(api, plan, journal, delay):
    # Always replay the same request, including after a timeout/crash. The server
    # validates request identity before version and returns the original history.
    for i, row in enumerate(plan["rows"], 1):
        receipt = api.request("PUT", f"/testees/{row['testee_id']}/store", payload={
            "store_id": row["store_id"], "expected_version": row["expected_version"],
            "request_id": row["request_id"], "reason": plan["reason"]})
        validate_receipt(receipt, row, plan)
        journal.write(json.dumps({"batch_id": plan["batch_id"], "receipt": receipt}, ensure_ascii=False) + "\n")
        journal.flush()
        os.fsync(journal.fileno())
        if i % 100 == 0 or i == len(plan["rows"]):
            print(json.dumps({"processed": i, "total": len(plan["rows"])}), flush=True)
        time.sleep(delay)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("preflight", "apply", "verify"))
    parser.add_argument("--base-url", required=True, help="QS API root ending /api/v1")
    parser.add_argument("--org-id", required=True)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--seed", type=int)
    parser.add_argument("--reason", default="总部批准：未归属受试者随机均分至武汉店、西安店、南京店")
    parser.add_argument("--delay", type=float, default=0.2, help="Seconds between writes (default 0.2)")
    args = parser.parse_args()
    if args.delay < 0 or not 1 <= len(args.reason.strip()) <= 500:
        parser.error("Invalid delay/reason")
    token_file = os.environ.get("QS_TOKEN_FILE")
    if not token_file:
        parser.error("Set QS_TOKEN_FILE to a restricted file containing a headquarters bearer token")
    api = API(args.base_url, Path(token_file).read_text().strip())
    os.umask(0o077)
    args.manifest.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    with args.manifest.with_suffix(args.manifest.suffix + ".lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        if args.command == "preflight":
            if args.manifest.exists():
                raise ValueError("Manifest exists; use apply/verify or choose a NEW filename")
            targets = stores(api, args.org_id)
            rows = inventory(api, args.org_id)
            if rows != inventory(api, args.org_id):
                raise ValueError("Inventory changed between scans; rerun preflight")
            manifest = make_plan(args.base_url, args.org_id, targets, rows,
                                 args.seed if args.seed is not None else secrets.randbits(64), args.reason.strip())
            with args.manifest.open("x") as out:
                json.dump(manifest, out, ensure_ascii=False, indent=2)
                out.flush()
                os.fsync(out.fileno())
            counts = Counter(r["store_id"] for r in manifest["plan"]["rows"])
            print(json.dumps({"total": len(rows), "stores": {s["name"]: counts[s["id"]] for s in targets},
                              "sha256": manifest["sha256"], "writes": 0}, ensure_ascii=False))
            return
        plan = validate_manifest(json.loads(args.manifest.read_text()), args.base_url, args.org_id)
        if stores(api, args.org_id) != plan["stores"]:
            raise ValueError("Target store configuration changed; stop and review")
        if args.command == "apply":
            with args.manifest.with_suffix(args.manifest.suffix + ".receipts.jsonl").open("a") as journal:
                apply(api, plan, journal, args.delay)
        else:
            # Read current ownership only; never regard historical receipts as current truth.
            for row in plan["rows"]:
                current = api.request("GET", f"/testees/{row['testee_id']}")
                if (str(current["org_id"]) != args.org_id or str(current.get("store_id")) != row["store_id"] or
                    current["store_version"] != row["expected_version"] + 1):
                    raise ValueError(f"Current ownership drift: {row['testee_id']}")
                time.sleep(args.delay)
            print(json.dumps({"verified": len(plan["rows"]), "writes": 0}))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, RuntimeError) as exc:
        print(f"Stopped: {exc}", file=sys.stderr)
        sys.exit(1)
