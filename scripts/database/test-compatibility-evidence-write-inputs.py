"""Offline public-input and actual Node validation; no database/proof producer."""
import argparse
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import textwrap
import tempfile
import os
from unittest import mock
import contextlib
import io
import unittest

ROOT = Path(__file__).resolve().parents[2]


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, ROOT / "scripts/database" / filename)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


tool = load("evidence_input_transport", "compatibility-retirement.py")
host = load("evidence_input_host", "compatibility-ai-history-prepare.py")
history = host.history_module()
ORIGINAL, TOOL = "a" * 40, "b" * 40


def approval(mode):
    parent = {"format_version": 1, "kind": "readonly_history_bootstrap_approval", "prepare_mode": "bootstrap-history",
        "source_sha": ORIGINAL, "operation_id": "100-1", "target_hash": tool.TARGET_HASH,
        "database_scope": "mysql-and-mongodb", "parent_request": {"run_id": "101-1", "sha256": "1" * 64},
        "inventory_request_sha256": "2" * 64, "inventory_report": {"run_id": "102-1", "sha256": "3" * 64},
        "assets": [{"database": db, "name": name, "full_file_sha256": "4" * 64, "full_file_bytes": 1}
            for db, name, _ in tool.TARGETS], "process_limits": dict(history.LIMITS)}
    value = {"format_version": 1, "kind": "historical_evidence_write_approval" if mode == host.WRITE_MODE else "readonly_ai_original_source_bounds_approval",
        "prepare_mode": mode, "source_sha": TOOL, "original_source_sha": ORIGINAL, "operation_id": "100-1",
        "target_hash": tool.TARGET_HASH, "database_scope": "mysql-and-mongodb", "history": parent,
        "runtime": {"source_sha": "c" * 40, "image_id": "sha256:" + "5" * 64, "container_id": "6" * 64,
            "binding_sha256": "7" * 64, "expected_ai_identity_hash": "", "expected_ai_head": ""}}
    if mode == host.WRITE_MODE:
        value.update(bounds={"run_id": "103-1", "ai_sha256": "8" * 64, "peer_sha256": "9" * 64}, protection_sha256="0" * 64)
    return value


class EvidenceInputContract(unittest.TestCase):
    def args(self, value):
        raw = tool.canonical_bytes(value)
        return argparse.Namespace(prepare_mode=value["prepare_mode"], actual_source_sha=TOOL, operation_id="100-1",
            run_id="900-1", bootstrap_approval_json=raw[:-1].decode(), bootstrap_approval_hash=hashlib.sha256(raw).hexdigest())

    def test_python_requires_distinct_original_inventory_binding_and_no_proof(self):
        for mode in (host.WRITE_MODE, host.ORIGINAL_BOUNDS_MODE):
            original = approval(mode)
            self.assertEqual(host.approval(self.args(original), tool, history), original)
            for change in (lambda v: v["history"].update(source_sha=TOOL),
                           lambda v: v.update(whole_writer_fence=True),
                           lambda v: v.update(original_source_sha=False),
                           lambda v: v["history"]["parent_request"].update(run_id="900-1")):
                value = copy.deepcopy(original); change(value)
                with self.assertRaises(tool.Blocked):
                    host.approval(self.args(value), tool, history)

    def test_actual_node_validates_both_new_modes_before_credentials(self):
        self.assertIsNotNone(shutil.which("node"))
        workflow = (ROOT / ".github/workflows/compatibility-retirement.yml").read_text()
        script = textwrap.dedent(workflow.split("          script: |\n", 1)[1].split("      - name:", 1)[0])
        for mode in (host.WRITE_MODE, host.ORIGINAL_BOUNDS_MODE):
            for label, change, valid in (("valid", lambda v: None, True),
                ("source", lambda v: v["history"].update(source_sha=TOOL), False),
                ("proof", lambda v: v.update(cas_authority=True), False)):
                with self.subTest(mode=mode, label=label):
                    value = approval(mode); change(value); args = self.args(value)
                    context = {"payload": {"inputs": {"operation": "prepare", "database": "mysql-and-mongodb",
                        "approved_source_sha": TOOL, "operation_id": "100-1", "prepare_mode": mode,
                        "bootstrap_approval_json": args.bootstrap_approval_json,
                        "bootstrap_approval_sha256": args.bootstrap_approval_hash}}, "ref": "refs/heads/main",
                        "sha": TOOL, "repo": {}, "runId": 900}
                    program = ("process.env.GITHUB_RUN_ID='900';process.env.GITHUB_RUN_ATTEMPT='1';const context="+json.dumps(context)+
                        ";const script="+json.dumps(script)+";const github={rest:{repos:{getCommit:async()=>({data:{sha:'"+TOOL+
                        "'}})}}};new(Object.getPrototypeOf(async function(){}).constructor)('context','github',script)(context,github).catch(()=>{process.exitCode=1;});")
                    result = subprocess.run(["node", "-e", program], capture_output=True, timeout=10)
                    self.assertEqual(result.returncode, 0 if valid else 1)


class EvidenceWriteResultContract(unittest.TestCase):
    def report(self):
        value = {key: False for key in host.WRITE_BOOL_FIELDS}
        value.update({key: 0 for key in host.WRITE_UINT_FIELDS})
        value.update(protocol="qs-compatibility-evidence-write/v1", source_sha=ORIGINAL,
            tool_source_sha=TOOL, operation_id="100-1", actual_run_id="900-1",
            request_sha256="1" * 64, descriptor_sha256="2" * 64,
            commit_state="sql_committed_mongo_not_required", mongo_commit_requirement="not_required",
            error_category="none", actual_sql_commit_response=True, evidence_write_finished=True,
            ai_original_commands=2, ai_source_references=3, ai_command_persistence_complete=True)
        return value

    def observe(self, report, rows=(0, 2, 1, 0), code=0):
        # The actual private-file validator reads the same bounded, protected
        # bytes as stdout. This local receipt test does not execute a writer.
        raw = tool.canonical_bytes(report)
        with tempfile.TemporaryDirectory() as d:
            output = Path(d).resolve(); output.chmod(0o700)
            path = output / "history.write.json"
            path.write_bytes(raw); path.chmod(0o600)
            descriptor = {key: report[key] for key in ("source_sha", "tool_source_sha", "operation_id", "actual_run_id", "request_sha256")}
            return host._write_report(tool, history, raw, output, descriptor, "2" * 64,
                {"targets": [{"records": count} for count in rows]}, code)

    def test_ai_only_reports_no_mongo_response_with_complete_readback(self):
        report, _ = self.observe(self.report())
        self.assertFalse(report["actual_mongo_commit_response"])
        self.assertEqual(report["mongo_commit_requirement"], "not_required")

    def test_event_batch_requires_actual_mongo_response(self):
        report = self.report()
        report.update(mongo_commit_requirement="required", commit_state="both_responses_success_non_atomic",
            actual_mongo_commit_response=True, event_references=2, prepared_pages=1,
            readback_pages=1, event_persistence_observed=True)
        self.observe(report, rows=(1, 2, 1, 1))
        report["actual_mongo_commit_response"] = False
        with self.assertRaises(tool.Blocked): self.observe(report, rows=(1, 2, 1, 1))

    def test_contradictory_or_incomplete_success_is_rejected(self):
        for change in (dict(actual_mongo_commit_response=True), dict(commit_state="both_responses_success_non_atomic"),
            dict(ai_command_persistence_complete=False), dict(mongo_commit_requirement="undetermined"),
            dict(readback_pages=1)):
            report = self.report(); report.update(change)
            with self.subTest(change=change), self.assertRaises(tool.Blocked): self.observe(report)

    def test_unknown_actual_commit_facts_are_kept_without_success(self):
        report = self.report()
        report.update(mongo_commit_requirement="required", commit_state="sql_committed_mongo_unknown",
            event_references=2, prepared_pages=1, evidence_write_finished=False,
            error_category="history_write_commit_unknown", ai_command_persistence_complete=False)
        observed, _ = self.observe(report, code=1)
        self.assertTrue(observed["actual_sql_commit_response"])
        self.assertFalse(observed["actual_mongo_commit_response"])
        self.assertFalse(observed["evidence_write_finished"])

    def test_public_receipt_whitelist_preserves_not_required_semantics(self):
        receipt = {"format_version":1, "operation":"prepare", "source_sha":TOOL, "original_source_sha":ORIGINAL,
            "operation_id":"100-1", "run_id":"900-1", "prepare_mode":host.WRITE_MODE,
            "complete":False, "execution_allowed":False, "drop_ready":False, "diagnostic_only":False,
            "history_evidence_write_finished":True, "history_actual_sql_commit_response":True,
            "history_actual_mongo_commit_response":False, "history_mongo_commit_requirement":"not_required",
            "history_commit_state":"sql_committed_mongo_not_required", "history_prepared_pages":0,
            "history_readback_pages":0, "history_event_references":0, "history_ai_original_commands":2,
            "history_ai_source_references":3, "history_ai_command_persistence_complete":True,
            "error_category":"none", "capabilities":{key:False for key in tool.CAPABILITIES}}
        argv=["--operation","prepare","--operation-id","100-1","--approved-source-sha",TOOL,
            "--actual-source-sha",TOOL,"--run-id","900-1","--prepare-mode",host.WRITE_MODE]
        for bad in (False, True):
            current = dict(receipt)
            if bad: current["history_actual_mongo_commit_response"] = True
            out = io.StringIO()
            with mock.patch.object(tool, "execute", return_value=current), contextlib.redirect_stdout(out):
                code = tool.main(argv)
            self.assertEqual(code, 42 if bad else 0)
            decoded = json.loads(tool.transport().decode_armored_receipt(out.getvalue().strip()))
            self.assertEqual(decoded["history_mongo_commit_requirement"], "not_required")
            self.assertFalse(decoded["drop_ready"])


if __name__ == "__main__":
    unittest.main()
