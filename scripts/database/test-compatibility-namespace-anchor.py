#!/usr/bin/env python3
"""Synthetic namespace identity contracts. This file grants no production scope."""
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, ROOT / "scripts/database" / filename)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


legacy = load("namespace_original_contract_fixtures", "test-compatibility-retirement.py")
tool = legacy.tool


def framed(parts):
    h = hashlib.sha256()
    for part in parts:
        raw = part.encode("utf-8")
        h.update(b"\x01" + len(raw).to_bytes(8, "big") + raw)
    return h.hexdigest()


def anchor():
    endpoint = framed(["mongodb-approved-seed-endpoint/v1", "fixture-host:27017", "admin", "fixture-business"])
    value = {"kind": tool.MONGO_NAMESPACE_PROFILE, "endpoint_sha256": endpoint,
             "database": "fixture-business", "replica_set_name": "fixture-rs", "collections": []}
    parts = [value["kind"], endpoint, value["database"], value["replica_set_name"]]
    for i, name in enumerate(tool.MONGO_KEPT_NAMES):
        item = {"name": name, "present": i == 0, "uuid": "31323334353637383930616263646566" if i == 0 else ""}
        value["collections"].append(item)
        parts.extend((name, "present" if item["present"] else "absent", item["uuid"]))
    value["hash"] = framed(parts)
    return value


class NamespaceAnchorContracts(unittest.TestCase):
    def setUp(self):
        self.fixture = legacy.SafetyContracts()
        self.fixture.setUp()
        self.addCleanup(self.fixture.tearDown)

    def new_bootstrap(self):
        args, value = self.fixture.bootstrap_fixture()
        a = anchor()
        raw = tool.identity_request_bytes(legacy.OPERATION, legacy.SOURCE, tool.MONGO_NAMESPACE_PROFILE)
        request = self.fixture.directory / "identity-request.json"
        request.write_bytes(raw); request.chmod(0o600)
        directory = self.fixture.directory / ("identity-" + value["identity_report"]["run_id"])
        report = tool.decode((directory / "identity.private.json").read_bytes())
        report["request_hash"] = hashlib.sha256(raw).hexdigest()
        report["database_states"]["mongodb"]["namespace_anchor"] = copy.deepcopy(a)
        report["database_states"]["mongodb"]["database_anchor_hash"] = a["hash"]
        encoded = tool.canonical_bytes(report)
        (directory / "identity.private.json").write_bytes(encoded)
        value["identity_report"]["sha256"] = hashlib.sha256(encoded).hexdigest()
        value["mongodb_namespace_anchor"] = copy.deepcopy(a)
        self.fixture.reapprove_bootstrap(args, value)
        return args, value

    def test_legacy_request_bytes_remain_unchanged_and_namespace_is_explicit(self):
        old = tool.identity_request_bytes(legacy.OPERATION, legacy.SOURCE)
        self.assertNotIn("mongo_anchor_profile", tool.decode(old))
        new = tool.identity_request_bytes(legacy.OPERATION, legacy.SOURCE, tool.MONGO_NAMESPACE_PROFILE)
        self.assertNotEqual(old, new)
        self.assertEqual(tool.decode(new)["identity_protocols"], tool.decode(old)["identity_protocols"])
        self.assertEqual(tool.decode(new)["mongo_anchor_profile"], tool.MONGO_NAMESPACE_PROFILE)
        tool.validate_identity_request(tool.decode(old), legacy.OPERATION, legacy.SOURCE)
        tool.validate_identity_request(tool.decode(new), legacy.OPERATION, legacy.SOURCE)
        with self.assertRaises(tool.Blocked): tool.identity_request_bytes(legacy.OPERATION, legacy.SOURCE, "unknown")

    def test_namespace_grammar_preserves_absence_and_rejects_aliases_duplicates_and_zero(self):
        original = anchor()
        tool.validate_mongo_namespace_anchor(original)
        mutations = [
            lambda a: a.update(kind=tool.MONGO_REPLICA_PROFILE),
            lambda a: a.update(endpoint_sha256="b" * 64),
            lambda a: a.update(database="other"),
            lambda a: a.update(collections=a["collections"][:-1]),
            lambda a: a["collections"][0].update(present=False, uuid=""),
            lambda a: a["collections"][0].update(uuid="0" * 32),
            lambda a: a["collections"][1].update(present=True, uuid=a["collections"][0]["uuid"]),
            lambda a: a["collections"][1].update(uuid="a" * 32),
            lambda a: a["collections"][0].pop("present"),
            lambda a: a["collections"].reverse(),
            lambda a: a.update(unapproved=True),
        ]
        for mutate in mutations:
            with self.subTest(mutation=mutations.index(mutate)):
                value = copy.deepcopy(original); mutate(value)
                with self.assertRaises(tool.Blocked): tool.validate_mongo_namespace_anchor(value)

    def test_bootstrap_consumes_approved_original_identity_and_does_not_connect(self):
        args, value = self.new_bootstrap()
        with mock.patch.object(tool, "live_inventory", side_effect=AssertionError("DB path forbidden")), mock.patch.object(tool, "capture_fixed", side_effect=AssertionError("runtime forbidden")):
            result = tool.execute(args)
        request, digest = tool.read_private(self.fixture.directory, "boundary-request.json", result["derived_request_sha256"])
        self.assertEqual(request["mongodb_namespace_anchor"], value["mongodb_namespace_anchor"])
        self.assertNotEqual(digest, args.bootstrap_approval_hash)
        self.assertFalse(result["complete"]); self.assertFalse(result["execution_allowed"]); self.assertFalse(result["drop_ready"])

    def test_approved_anchor_cannot_be_borrowed_from_different_report_source_or_operation(self):
        args, value = self.new_bootstrap()
        changed = copy.deepcopy(value); changed["mongodb_namespace_anchor"]["collections"][0]["uuid"] = "a" * 32
        a = changed["mongodb_namespace_anchor"]
        parts = [a["kind"], a["endpoint_sha256"], a["database"], a["replica_set_name"]]
        for row in a["collections"]: parts.extend((row["name"], "present" if row["present"] else "absent", row["uuid"]))
        a["hash"] = framed(parts)
        tool.validate_mongo_namespace_anchor(a)
        self.fixture.reapprove_bootstrap(args, changed)
        with self.assertRaises(tool.Blocked): tool.execute(args)
        for field, wrong in (("source_sha", "b" * 40), ("operation_id", "124-1")):
            other = copy.deepcopy(value); other[field] = wrong
            self.fixture.reapprove_bootstrap(args, other)
            with self.assertRaises(tool.Blocked): tool.execute(args)

    def test_binding_compare_does_not_treat_hash_only_or_rebuilt_namespace_as_same(self):
        a = anchor(); request = {"mongodb_namespace_anchor": a}
        observed = {"database_anchor_hash": a["hash"], "namespace_anchor": copy.deepcopy(a)}
        tool.validate_approved_namespace_anchor(request, observed)
        for wrong in ({"database_anchor_hash": a["hash"]},
                      {"database_anchor_hash": "b" * 64, "namespace_anchor": a},
                      {"database_anchor_hash": a["hash"], "namespace_anchor": dict(a, database="other")}):
            with self.assertRaises(tool.Blocked): tool.validate_approved_namespace_anchor(request, wrong)
        with self.assertRaises(tool.Blocked): tool.validate_approved_namespace_anchor({}, observed)

    def test_public_projection_contains_only_kind_hash_and_count(self):
        public = tool.public_namespace_anchor(anchor())
        self.assertEqual(set(public), {"database_anchor_kind", "database_anchor_uuid_set_sha256", "database_anchor_kept_count"})
        self.assertEqual(public["database_anchor_kept_count"], 1)
        self.assertNotIn("fixture-host", json.dumps(public)); self.assertNotIn("fixture-business", json.dumps(public))
        self.assertNotIn("31323334353637383930616263646566", json.dumps(public))

    def test_actual_action_namespace_descriptor_remains_closed_and_hash_bound(self):
        if not shutil.which("node"): self.skipTest("node_required_for_actual_workflow_contract")
        workflow = (ROOT / ".github/workflows/compatibility-retirement.yml").read_text()
        lines = workflow.split("          script: |\n", 1)[1].splitlines()
        actual = []
        for line in lines:
            if line.strip() and not line.startswith("            "): break
            actual.append(line[12:])
        script = "\n".join(actual)
        args, value = self.new_bootstrap()
        def check(approval):
            text = tool.canonical_bytes(approval)[:-1].decode("ascii")
            inputs = {"operation": "prepare", "database": "mysql-and-mongodb", "approved_source_sha": legacy.SOURCE,
                      "operation_id": legacy.OPERATION, "prepare_mode": "bootstrap-bounds",
                      "bootstrap_approval_json": text, "bootstrap_approval_sha256": hashlib.sha256(tool.canonical_bytes(approval)).hexdigest()}
            program = ("const context=JSON.parse(require('fs').readFileSync(0,'utf8'));const script="
                       + json.dumps(script) + ";const github={rest:{repos:{getCommit:async()=>({data:{sha:context.sha}})}}};"
                       + "new (Object.getPrototypeOf(async function(){}).constructor)('context','github',script)(context,github)"
                       + ".catch(()=>{process.exitCode=1;});")
            return subprocess.run(["node", "-e", program], input=json.dumps({"ref": "refs/heads/main", "sha": legacy.SOURCE, "repo": {}, "payload": {"inputs": inputs}}), text=True, capture_output=True, check=False).returncode
        self.assertEqual(check(value), 0)
        unicode_value = copy.deepcopy(value)
        unicode_anchor = unicode_value["mongodb_namespace_anchor"]
        unicode_anchor.update(database="测评库-😀", replica_set_name="副本-🧭")
        unicode_anchor["endpoint_sha256"] = framed([
            "mongodb-approved-seed-endpoint/v1", "fixture-host:27017", "admin",
            unicode_anchor["database"]])
        parts = [unicode_anchor["kind"], unicode_anchor["endpoint_sha256"],
                 unicode_anchor["database"], unicode_anchor["replica_set_name"]]
        for row in unicode_anchor["collections"]:
            parts.extend((row["name"], "present" if row["present"] else "absent", row["uuid"]))
        unicode_anchor["hash"] = framed(parts)
        tool.validate_mongo_namespace_anchor(unicode_anchor)
        self.assertEqual(check(unicode_value), 0)
        for mutate in (lambda a: a["mongodb_namespace_anchor"].update(kind=tool.MONGO_REPLICA_PROFILE),
                       lambda a: a["mongodb_namespace_anchor"].update(hash="b" * 64),
                       lambda a: a["mongodb_namespace_anchor"]["collections"][0].pop("present"),
                       lambda a: a["mongodb_namespace_anchor"]["collections"].reverse(),
                       lambda a: a.update(unapproved=True)):
            wrong = copy.deepcopy(value); mutate(wrong)
            self.assertNotEqual(check(wrong), 0)


if __name__ == "__main__":
    unittest.main()
