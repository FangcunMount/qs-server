#!/usr/bin/env python3
"""Actual Action/CLI contracts for diagnostic parent registration, never DROP proof."""
import argparse
import contextlib
import copy
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import textwrap
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, ROOT / "scripts/database" / filename)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


fixtures = load("history_parent_action_fixtures", "test-compatibility-history-parent.py")
tool = fixtures.tool


class ParentAction(unittest.TestCase):
    def setUp(self):
        self.fixture = fixtures.ParentRegistration(methodName="test_actual_exclusive_publication_keeps_original_inventory_run_and_no_authority")
        self.fixture.setUp()
        self.args = self.fixture.args
        self.argv = ["--operation", "prepare", "--root", str(self.fixture.fixture.fixture.root),
            "--operation-id", self.args.operation_id, "--approved-source-sha", self.args.actual_source_sha,
            "--actual-source-sha", self.args.actual_source_sha, "--run-id", self.args.run_id,
            "--prepare-mode", "bootstrap-history-parent", "--bootstrap-approval-json", self.args.bootstrap_approval_json,
            "--bootstrap-approval-hash", self.args.bootstrap_approval_hash]

    def tearDown(self):
        self.fixture.tearDown()

    def invoke(self, argv=None):
        selected = self.argv if argv is None else argv
        parent_mode = selected[selected.index("--prepare-mode") + 1] == "bootstrap-history-parent"
        class NoDatabaseEnvironment(dict):
            def get(self, key, *args):
                if key.startswith(("MYSQL_", "MONGODB_")):
                    if parent_mode:
                        raise AssertionError("database environment read")
                    return ""
                return super().get(key, *args)
        output, error = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(error), mock.patch.object(tool, "capture_fixed", side_effect=self.fixture.fixture.capture), \
             mock.patch.object(tool.os, "environ", NoDatabaseEnvironment()), \
             mock.patch.object(tool, "inventory_connection_values", side_effect=AssertionError("database connection")):
            status = tool.main(selected)
        if not output.getvalue():
            self.assertEqual(status, 42)
            self.assertEqual(error.getvalue(), "compatibility_retirement_receipt_transport_failed\n")
            return status, {}, ""
        return status, json.loads(tool.transport().decode_armored_receipt(output.getvalue())), output.getvalue()

    def test_actual_main_armor_decode_registered_but_no_mutation_and_separate_three_runs(self):
        status, receipt, stdout = self.invoke()
        self.assertEqual(status, 0)
        self.assertIs(receipt["history_parent_registration_complete"], True)
        for name in ("complete", "execution_allowed", "drop_ready", "history_cas_complete", "history_parent_process_budget_proven"):
            self.assertIs(receipt[name], False)
        self.assertIs(receipt["diagnostic_only"], True)
        self.assertEqual(receipt["run_id"], "1000-1")
        self.assertEqual(receipt["request_created_run_id"], "1000-1")
        self.assertEqual(receipt["approved_metadata_report"]["run_id"], "900-1")
        self.assertEqual(receipt["approved_inventory_report"]["run_id"], "700-1")
        self.assertEqual(receipt["parent_proposal_run_id"], "700-1")
        parent, digest = tool.read_private(self.fixture.directory, "history-request.json")
        self.assertEqual(parent["run_id"], "700-1")
        self.assertEqual(receipt["history_parent_request_sha256"], digest)
        self.assertNotIn(str(self.fixture.directory), stdout)
        self.assertFalse(any(c[3] in ("create", "run", "start", "stop", "rm") for c in self.fixture.fixture.commands))

    def test_armor_failure_returns42_even_if_actual_registration_was_written(self):
        output, error = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(error), \
             mock.patch.object(tool, "capture_fixed", side_effect=self.fixture.fixture.capture), \
             mock.patch.object(tool, "transport", return_value=argparse.Namespace(encode_armored_receipt=mock.Mock(side_effect=ValueError("failure")))):
            self.assertEqual(tool.main(self.argv), 42)
        self.assertEqual(output.getvalue(), "")
        self.assertEqual(error.getvalue(), "compatibility_retirement_receipt_transport_failed\n")
        self.assertTrue((self.fixture.directory / "history-request.json").is_file())

    def test_actual_registration_interruption_and_retry_return42_never_overwrite(self):
        original = tool.create_bootstrap_file
        def interrupted(directory, filename, raw):
            if filename == "history-request.json":
                raise tool.Blocked("bootstrap_request_creation_incomplete")
            return original(directory, filename, raw)
        with mock.patch.object(tool, "create_bootstrap_file", side_effect=interrupted):
            status, receipt, _ = self.invoke()
        self.assertEqual(status, 42)
        self.assertNotIn("history_parent_registration_complete", receipt)
        recorded = (self.fixture.directory / "history-request-bootstrap.json").read_bytes()
        status, receipt, _ = self.invoke()
        self.assertEqual(status, 42)
        self.assertEqual(receipt["error_category"], "history_parent_registration_incomplete")
        self.assertEqual(recorded, (self.fixture.directory / "history-request-bootstrap.json").read_bytes())
        self.assertFalse((self.fixture.directory / "history-request.json").exists())

    def test_parent_input_classes_actual_source_and_invalid_run_rejected_before_publication(self):
        changes = (("--operation", "apply"), ("--approved-source-sha", "b" * 40), ("--run-id", "700-1"),
                   ("--run-id", "900-1"), ("--run-id", "not-a-run"), ("--prepare-mode", "bootstrap-inventory"))
        for key, value in changes:
            with self.subTest(key=key, value=value):
                argv = self.argv.copy()
                argv[argv.index(key) + 1] = value
                status, receipt, _ = self.invoke(argv)
                self.assertEqual(status, 42)
                self.assertNotIn("history_parent_registration_complete", receipt)
                self.fixture.assert_unpublished()
        for option in ("--manifest-hash", "--inventory-request-hash", "--identity-request-hash"):
            with self.subTest(option=option):
                self.assertEqual(self.invoke(self.argv + [option, "e" * 64])[0], 42)
                self.fixture.assert_unpublished()

    def test_parent_whitelist_and_mutation_flags_never_allow_diagnostic_zero(self):
        receipt = self.fixture.run_parent()
        for name, value in (("complete", True), ("execution_allowed", True), ("drop_ready", True),
                            ("history_cas_complete", True), ("history_parent_process_budget_proven", True),
                            ("diagnostic_only", False), ("history_parent_registration_complete", False),
                            ("unsafe_path", "/private/source/body")):
            with self.subTest(name=name):
                altered = dict(receipt, **{name: value})
                with mock.patch.object(tool, "execute", return_value=altered), contextlib.redirect_stderr(io.StringIO()):
                    self.assertEqual(self.invoke()[0], 42)

    def test_actual_node_parent_strict_approval_matrix(self):
        if not shutil.which("node"):
            self.fail("Node required for Action contract validation")
        workflow = (ROOT / ".github/workflows/compatibility-retirement.yml").read_text()
        script = textwrap.dedent(workflow.split("          script: |\n", 1)[1].split("      - name:", 1)[0])
        cases = [("valid", lambda v: None, True), ("empty_mongo", lambda v: v["assets"][3].update(full_file_bytes=0), True),
            ("extra", lambda v: v.update(extra=True), False), ("wrong_kind", lambda v: v.update(kind="readonly_history_metadata_approval"), False),
            ("wrong_mode", lambda v: v.update(prepare_mode="bootstrap-history"), False), ("wrong_source", lambda v: v.update(source_sha="b" * 40), False),
            ("wrong_operation", lambda v: v.update(operation_id="124-1"), False), ("wrong_target", lambda v: v.update(target_hash="f" * 64), False),
            ("metadata_actual", lambda v: v["metadata_report"].update(run_id="1000-1"), False),
            ("inventory_actual", lambda v: v["inventory_report"].update(run_id="1000-1"), False),
            ("same_original_runs", lambda v: v["metadata_report"].update(run_id="700-1"), False),
            ("bad_metadata_hash", lambda v: v["metadata_report"].update(sha256=""), False),
            ("bad_proposal_hash", lambda v: v.update(parent_proposal_sha256=""), False),
            ("request_null", lambda v: v.update(inventory_request_sha256=None), False),
            ("ref_unknown", lambda v: v["inventory_report"].update(path="/private/source"), False),
            ("source_hash", lambda v: v["assets"][0].update(full_file_sha256="z" * 64), False),
            ("sidecar_hash", lambda v: v["assets"][0].update(source_asset_sha256=""), False),
            ("zero_sql", lambda v: v["assets"][0].update(full_file_bytes=0), False),
            ("over_cap", lambda v: v["assets"][0].update(full_file_bytes=2147483649), False),
            ("boolean_bytes", lambda v: v["assets"][0].update(full_file_bytes=True), False),
            ("float_bytes", lambda v: v["assets"][0].update(full_file_bytes=1.5), False),
            ("asset_path", lambda v: v["assets"][0].update(path="/private/source"), False),
            ("asset_missing", lambda v: v["assets"].pop(), False), ("asset_reverse", lambda v: v["assets"].reverse(), False),
            ("limit_bool", lambda v: v["metadata_limits"].update(passes=True), False),
            ("limit_change", lambda v: v["metadata_limits"].update(total_seconds=901), False),
            ("context_mismatch", lambda v: None, False), ("bad_attempt", lambda v: None, False),
            ("branch", lambda v: None, False), ("main_advanced", lambda v: None, False),
            ("noncanonical", lambda v: None, False), ("approval_hash", lambda v: None, False),
            ("duplicate_key", lambda v: None, False)]
        for name, change, allowed in cases:
            with self.subTest(name=name):
                value = copy.deepcopy(self.fixture.value)
                change(value)
                raw = tool.canonical_bytes(value)
                line = raw[:-1].decode("ascii")
                if name == "noncanonical": line += " "
                if name == "duplicate_key": line = line[:-1] + ',"format_version":1}'
                descriptor_hash = hashlib.sha256((line + "\n").encode("ascii")).hexdigest()
                inputs = {"operation": "prepare", "database": "mysql-and-mongodb", "approved_source_sha": self.args.actual_source_sha,
                    "operation_id": self.args.operation_id, "prepare_mode": "bootstrap-history-parent",
                    "bootstrap_approval_json": line, "bootstrap_approval_sha256": "f" * 64 if name == "approval_hash" else descriptor_hash}
                context = {"payload": {"inputs": inputs}, "ref": "refs/heads/feature" if name == "branch" else "refs/heads/main",
                    "sha": self.args.actual_source_sha, "repo": {}, "runId": 1001 if name == "context_mismatch" else 1000}
                main_sha = "b" * 40 if name == "main_advanced" else self.args.actual_source_sha
                program = "process.env.GITHUB_RUN_ID='1000';process.env.GITHUB_RUN_ATTEMPT=" + json.dumps("x" if name == "bad_attempt" else "1") + ";const script=" + json.dumps(script) + ";const context=" + json.dumps(context) + ";const github={rest:{repos:{getCommit:async()=>({data:{sha:" + json.dumps(main_sha) + "}})}}};new (Object.getPrototypeOf(async function(){}).constructor)('context','github',script)(context,github).catch(()=>{process.exitCode=1;});"
                result = subprocess.run(["node", "-e", program], capture_output=True, check=False)
                self.assertEqual(result.returncode, 0 if allowed else 1, name)


class ParentWorkflow(unittest.TestCase):
    def test_separate_parent_ssh_has_no_db_go_arch_or_binary_environment(self):
        workflow = (ROOT / ".github/workflows/compatibility-retirement.yml").read_text()
        step = workflow.split("      - name: Register independently approved history parent without database credentials\n", 1)[1]
        self.assertIn("if: inputs.prepare_mode == 'bootstrap-history-parent'", step)
        for forbidden in ("MYSQL_", "MONGODB_", "inventory_binary", "history_binary", "uname -m", "go build", "docker create", "docker rm"):
            self.assertNotIn(forbidden, step)
        envs = step.split("          envs: ", 1)[1].split("\n", 1)[0].split(",")
        self.assertEqual(len(envs), 9)
        self.assertTrue(all(name.startswith("RETIREMENT_") for name in envs))
        old = workflow.split("      - name: Inventory source bytes or reject unavailable lifecycle stage\n", 1)[1].split("      - name: Observe approved", 1)[0]
        self.assertIn("inputs.prepare_mode != 'bootstrap-history-parent'", old)
        self.assertIn("MONGODB_METADATA_ADMIN_PASSWORD", old)
        setup = workflow.split("      - name: Set up Go for immutable read-only inventory\n", 1)[1].split("      - name:", 1)[0]
        self.assertIn("inputs.prepare_mode != 'bootstrap-history-parent'", setup)
        self.assertIn("python3 -B scripts/database/test-compatibility-history-parent-action.py", workflow)

    def test_actual_parent_package_exact_five_without_any_go_call_metadata_stays_four(self):
        workflow = (ROOT / ".github/workflows/compatibility-retirement.yml").read_text()
        step = workflow.split("      - name: Package only immutable tooling\n", 1)[1].split("      - name:", 1)[0]
        body = textwrap.dedent(step.split("        run: |\n", 1)[1])
        for mode in ("bootstrap-history-parent", "bootstrap-history-metadata"):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary).resolve()
                (root / "scripts/database").mkdir(parents=True)
                (root / "scripts/dbops").mkdir(parents=True)
                for rel in ("scripts/database/compatibility-retirement.py", "scripts/database/compatibility-history-prepare.py",
                            "scripts/database/compatibility-history-parent.py", "scripts/database/compatibility-retirement-entrypoints.json", "scripts/dbops/receipt-transport.py"):
                    (root / rel).write_bytes((ROOT / rel).read_bytes())
                tools = root / "tools"
                tools.mkdir(mode=0o700)
                sentinel = tools / "go"
                sentinel.write_text("#!/bin/sh\necho unexpected_go >&2\nexit 99\n")
                sentinel.chmod(0o700)
                runtime = root / "runtime"
                runtime.mkdir(mode=0o700)
                output = root / "output"
                env = dict(os.environ, RETIREMENT_PACKAGE_MODE=mode, RUNNER_TEMP=str(runtime), GITHUB_RUN_ID="1000",
                    GITHUB_RUN_ATTEMPT="1", GITHUB_SHA="a" * 40, GITHUB_OUTPUT=str(output), PATH=str(tools) + os.pathsep + os.environ["PATH"])
                result = subprocess.run(["bash", "-c", body], cwd=root, env=env, capture_output=True, check=False)
                self.assertEqual(result.returncode, 0, result.stderr.decode())
                archive = root / "qs-compatibility-retirement-1000-1.tar.gz"
                names = subprocess.run(["tar", "-tzf", str(archive)], capture_output=True, check=True).stdout.decode().splitlines()
                expected = ["compatibility-retirement.py", "compatibility-history-prepare.py"]
                if mode == "bootstrap-history-parent": expected += ["compatibility-history-parent.py"]
                expected += ["compatibility-retirement-entrypoints.json", "receipt-transport.py"]
                self.assertEqual(names, expected)
                self.assertEqual(output.read_text().strip(), "sha256=" + hashlib.sha256(archive.read_bytes()).hexdigest())
                self.assertEqual(list(runtime.iterdir()), [])
        # Existing compiled package is unchanged; this assertion does not run Go.
        self.assertIn("compatibility-retirement.py compatibility-history-prepare.py compatibility-retirement-entrypoints.json receipt-transport.py inventory-linux-amd64 inventory-linux-arm64 history-linux-amd64 history-linux-arm64", body)


if __name__ == "__main__":
    unittest.main()
