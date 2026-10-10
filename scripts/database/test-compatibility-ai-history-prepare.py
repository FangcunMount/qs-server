"""Offline filesystem/Node/package protocol tests, never a native DB proof."""
import argparse
import copy
import fcntl
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import textwrap
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


fixture = load("ai_host_filesystem_fixture", ROOT / "scripts/database/test-compatibility-history-prepare.py")
tool = fixture.tool
host = load("ai_host_action_candidate", ROOT / "scripts/database/compatibility-ai-history-prepare.py")
SOURCE, RUN, OPERATION = fixture.SOURCE, fixture.RUN, fixture.OPERATION


class AIHostAction(unittest.TestCase):
    def setUp(self):
        self.fx = fixture.HistoryPreparation(); self.fx.setUp()
        self.args = self.fx.args
        self.args.prepare_mode = "bootstrap-ai-bounds"
        self.value = {"format_version": 1, "kind": "readonly_ai_host_bootstrap_approval",
            "prepare_mode": self.args.prepare_mode, "source_sha": SOURCE, "operation_id": OPERATION,
            "target_hash": tool.TARGET_HASH, "database_scope": "mysql-and-mongodb", "history": copy.deepcopy(self.fx.value),
            "runtime": {"source_sha": "b" * 40, "image_id": "sha256:" + "c" * 64,
                "container_id": "d" * 64, "binding_sha256": "e" * 64,
                "expected_ai_identity_hash": "", "expected_ai_head": ""}}
        self.env = self.fx.env.copy()
        self.calls = []
        self.change = None
        self.child_code = 0
        self.approve()

    def tearDown(self):
        self.fx.tearDown()

    def approve(self):
        raw = tool.canonical_bytes(self.value)
        self.args.bootstrap_approval_json = raw[:-1].decode()
        self.args.bootstrap_approval_hash = hashlib.sha256(raw).hexdigest()

    def verify(self):
        self.args.prepare_mode = self.value["prepare_mode"] = "bootstrap-ai-verify"
        prior = self.fx.directory / "ai-host-bounds-600-1"; prior.mkdir(mode=0o700)
        ai = self.fx.store(prior, "ai.bounds.json", {"synthetic_unapproved_bounds": "ai"})
        peer = self.fx.store(prior, "peer.bounds.json", {"synthetic_unapproved_bounds": "peer"})
        protection = self.fx.store(self.fx.directory, "ai-message-protection.json", {"synthetic_private_input": True})
        self.value.update(bounds={"run_id": "600-1", "ai_sha256": ai, "peer_sha256": peer}, protection_sha256=protection)
        self.approve()

    def capture(self, command, **kwargs):
        self.calls.append(command)
        if command[-1] == "--source-sha":
            return self.fx.original_capture(command, **kwargs)
        if command[:3] == ["sudo", "-n", "docker"]:
            self.assertEqual(command[3:5], ["container", "ls"])
            return 0, b""
        self.assertEqual(command[1], "--ai-host-mode")
        # Actual filesystem lock acquisition proves no outer Python flock is
        # retained when the native caller takes ownership. No DB call occurs.
        with open(self.fx.directory / "operation.lock", "r+b") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        self.assertEqual(os.environ["MONGODB_USERNAME"], self.env["MONGODB_METADATA_ADMIN_USERNAME"])
        self.assertEqual(os.environ["MONGODB_PASSWORD"], self.env["MONGODB_METADATA_ADMIN_PASSWORD"])
        flags = dict(zip(command[1::2], command[2::2]))
        descriptor = tool.decode(Path(flags["--ai-input"]).read_bytes())
        output = Path(flags["--output"]); output.mkdir(mode=0o700)
        r = {key: False for key in host.FLAGS_FALSE}
        r.update(protocol="qs-compatibility-ai-host-readonly/v1", mode=flags["--ai-host-mode"], source_sha=SOURCE,
            operation_id=OPERATION, actual_run_id=RUN, external_run_id=RUN.split("-")[0],
            request_sha256=flags["--request-sha256"], descriptor_sha256=flags["--ai-input-sha256"],
            expected_runtime_binding_sha256="e" * 64, runtime_binding_sha256="e" * 64,
            error_category="none", diagnostic_only=True, diagnostic_read_complete=True, required_adapters=list(host.REQUIRED))
        if r["mode"] == "bounds":
            ai = self.fx.store(output, "ai.bounds.json", {"synthetic_observation": "ai"})
            peer = self.fx.store(output, "peer.bounds.json", {"synthetic_observation": "peer"})
            v = {key: False for key in host.BOUNDS_FALSE}
            v.update(scope="diagnostic-unapproved-bounds-only", facts_sha256="f" * 64,
                runtime_binding_sha256="e" * 64, ai_bounds_sha256=ai, peer_bounds_sha256=peer,
                ai_physical_objects=44, ai_logical_objects=53, peer_objects=14, independent_epochs=2,
                prior_ai_binding_matched=False, next_cycle_required=True)
            r["bounds"] = v
            self.assertNotIn("ai_bounds", descriptor)
        else:
            v = {key: False for key in host.VERIFY_FALSE}
            v.update(scope="actual-full-qs-ai-and-peer-two-epoch-database-facts-only", originals=1,
                facts_sha256="f" * 64, independent_epochs=2, external_database_facts_observed=True)
            r["verification"] = v
            self.assertIn("ai_bounds", descriptor)
        if self.change:
            self.change(r, descriptor, output)
        raw = tool.canonical_bytes(r)
        (output / "ai-host.readiness.json").write_bytes(raw)
        (output / "ai-host.readiness.json").chmod(0o600)
        return self.child_code, raw

    def prepare(self):
        with mock.patch.dict(os.environ, self.env, clear=True), mock.patch.object(tool, "capture_fixed", self.capture):
            return host.prepare(self.args, argparse.Namespace(**vars(tool)))

    def arguments(self):
        a = self.args
        return ["--operation", "prepare", "--root", a.root, "--operation-id", OPERATION,
            "--approved-source-sha", SOURCE, "--actual-source-sha", SOURCE, "--run-id", RUN,
            "--prepare-mode", a.prepare_mode, "--history-binary", a.history_binary,
            "--bootstrap-approval-json", a.bootstrap_approval_json, "--bootstrap-approval-hash", a.bootstrap_approval_hash]

    def test_bounds_registration_derives_only_actual_run_and_releases_lock(self):
        result = self.prepare()
        self.assertTrue(result["ai_host_readonly_complete"])
        self.assertFalse(result["ai_host_process_budget_proven"])
        self.assertTrue(all(v is False for v in result["capabilities"].values()))
        original = (self.fx.directory / "history-request.json").read_bytes()
        self.assertEqual(hashlib.sha256(original).hexdigest(), self.fx.parent_hash)
        registered = self.fx.directory / ("ai-bootstrap-bounds-" + RUN)
        derived = tool.decode((registered / "history.request.json").read_bytes())
        self.assertEqual(derived, dict(self.fx.parent, run_id=RUN))
        self.assertNotEqual(result["derived_request_sha256"], self.fx.parent_hash)
        self.assertTrue(all(c[3:5] == ["container", "ls"] for c in self.calls if "docker" in c))
        self.assertEqual(os.environ.get("MONGODB_USERNAME"), None)

    def test_verify_actual_private_bounds_and_protection_are_hash_bound(self):
        self.verify()
        r = self.prepare()
        self.assertEqual(r["ai_host_mode"], "verify")
        self.assertEqual(r["ai_host_originals"], 1)

    def test_no_mongo_application_fallback_or_half_metadata_pair(self):
        for key in ("MONGODB_METADATA_ADMIN_USERNAME", "MONGODB_METADATA_ADMIN_PASSWORD"):
            with self.subTest(key=key):
                saved = self.env.pop(key)
                with self.assertRaisesRegex(tool.Blocked, "^ai_host_metadata_connection_pair_required$"):
                    self.prepare()
                self.env[key] = saved
        self.assertEqual(self.calls, [])

    def test_nested_source_or_current_run_or_hash_drift_rejected_before_child(self):
        changes = [lambda v: v["history"].update(source_sha="9" * 40),
                   lambda v: v["history"]["parent_request"].update(run_id=RUN),
                   lambda v: v["history"]["assets"][0].update(full_file_sha256="a" * 64),
                   lambda v: v["runtime"].update(container_id="unexpected/path"),
                   lambda v: v.update(unapproved_complete=True)]
        initial = copy.deepcopy(self.value)
        for change in changes:
            with self.subTest(change=changes.index(change)):
                self.value = copy.deepcopy(initial); change(self.value); self.approve()
                with self.assertRaises(tool.Blocked): self.prepare()
        self.assertFalse(any("--ai-host-mode" in c for c in self.calls))

    def test_unknown_readiness_body_or_authority_true_rejected(self):
        for label, mutate in (
            ("extra", lambda r, d, o: r.update(raw_body="not-permitted")),
            ("cap", lambda r, d, o: r.update(cas_authority=True)),
            ("epoch", lambda r, d, o: r["bounds"].update(independent_epochs=1)),
            ("missing_system_head", lambda r, d, o: r["bounds"].update(ai_physical_objects=43)),
            ("extra_physical_object", lambda r, d, o: r["bounds"].update(ai_physical_objects=45)),
            ("type", lambda r, d, o: r["bounds"].update(peer_objects="14")),
            ("binding", lambda r, d, o: r.update(request_sha256="a" * 64)),
            ("incomplete", lambda r, d, o: r.update(diagnostic_read_complete=False)),
        ):
            with self.subTest(label=label):
                # A separate real private filesystem fixture prevents retries
                # or overwriting a failed producer's registration/output.
                case = AIHostAction(); case.setUp()
                try:
                    case.change = mutate
                    with self.assertRaises(tool.Blocked): case.prepare()
                finally: case.tearDown()

    def test_child_failure_never_succeeds_or_retries(self):
        self.child_code = 1
        with self.assertRaisesRegex(tool.Blocked, "^ai_host_native_diagnostic_failed_or_execution_unknown$"):
            self.prepare()
        with self.assertRaisesRegex(tool.Blocked, "^ai_host_registration_exists_or_unavailable$"):
            self.prepare()
        self.assertEqual(sum("--ai-host-mode" in c for c in self.calls), 1)

    def test_missing_private_verify_asset_rejected_before_native(self):
        self.verify()
        (self.fx.directory / "ai-message-protection.json").unlink()
        with self.assertRaises(tool.Blocked): self.prepare()
        self.assertFalse(any("--ai-host-mode" in c for c in self.calls))

    def test_existing_inventory_handle_unknown_is_not_adopted_or_removed(self):
        def unknown(command, **kwargs):
            self.calls.append(command)
            if command[3:5] == ["container", "ls"]:
                return 0, ("d" * 64 + "\n").encode()
            self.assertEqual(command[3], "inspect")
            return 0, tool.canonical_bytes({"id": "d" * 64})
        with mock.patch.dict(os.environ, self.env, clear=True), mock.patch.object(tool, "capture_fixed", unknown):
            with self.assertRaisesRegex(tool.Blocked, "^history_metadata_inventory_outcome_unresolved$"):
                host.prepare(self.args, argparse.Namespace(**vars(tool)))
        self.assertFalse(any("--ai-host-mode" in c for c in self.calls))
        self.assertTrue(all(c[3] in ("container", "inspect") for c in self.calls))

    def test_driver_armor_and_zero_only_actual_diagnostic(self):
        stdout = io.StringIO()
        with mock.patch.dict(os.environ, self.env, clear=True), mock.patch.object(tool, "capture_fixed", self.capture), mock.patch("sys.stdout", stdout):
            self.assertEqual(tool.main(self.arguments()), 0)
        decoded = json.loads(tool.transport().decode_armored_receipt(stdout.getvalue()))
        self.assertTrue(decoded["ai_host_readonly_complete"])
        self.assertFalse(decoded["complete"])
        self.assertFalse(decoded["execution_allowed"])
        self.assertNotIn(self.env["MYSQL_PASSWORD"], stdout.getvalue())
        self.assertNotIn(str(self.fx.directory), stdout.getvalue())

    def test_armor_failure_stays_42(self):
        broken = mock.Mock(); broken.encode_armored_receipt.side_effect = ValueError("fixed synthetic failure")
        with mock.patch.dict(os.environ, self.env, clear=True), mock.patch.object(tool, "capture_fixed", self.capture), mock.patch.object(tool, "transport", return_value=broken), mock.patch("sys.stdout", io.StringIO()), mock.patch("sys.stderr", io.StringIO()):
            self.assertEqual(tool.main(self.arguments()), 42)

    def test_actual_node_nested_binding_matrix(self):
        self.assertIsNotNone(shutil.which("node"), "Node is required for this Action contract test")
        workflow = (ROOT / ".github/workflows/compatibility-retirement.yml").read_text()
        script = textwrap.dedent(workflow.split("          script: |\n", 1)[1].split("      - name:", 1)[0])
        cases = [("valid_bounds", lambda v: None, True),
            ("wrong_nested_source", lambda v: v["history"].update(source_sha="9" * 40), False),
            ("old_current_run", lambda v: v["history"]["parent_request"].update(run_id="900-1"), False),
            ("asset_unknown", lambda v: v["history"]["assets"][0].update(body=True), False),
            ("false_type", lambda v: v["runtime"].update(expected_ai_head=False), False),
            ("extra_authority", lambda v: v.update(drop_ready=True), False),
            ("context_run_mismatch", lambda v: None, False),
            ("hash_mismatch", lambda v: None, False),
        ]
        for name, change, allowed in cases:
            with self.subTest(name=name):
                v = copy.deepcopy(self.value); change(v); raw = tool.canonical_bytes(v)
                inputs = {"operation": "prepare", "database": "mysql-and-mongodb", "approved_source_sha": SOURCE,
                    "operation_id": OPERATION, "prepare_mode": self.args.prepare_mode, "bootstrap_approval_json": raw[:-1].decode(),
                    "bootstrap_approval_sha256": hashlib.sha256(raw).hexdigest() if name != "hash_mismatch" else "0" * 64}
                context = {"payload": {"inputs": inputs}, "ref": "refs/heads/main", "sha": SOURCE, "repo": {}, "runId": 901 if name == "context_run_mismatch" else 900}
                program = "process.env.GITHUB_RUN_ID='900';process.env.GITHUB_RUN_ATTEMPT='1';const context="+json.dumps(context)+";const script="+json.dumps(script)+";const github={rest:{repos:{getCommit:async()=>({data:{sha:'"+SOURCE+"'}})}}};new(Object.getPrototypeOf(async function(){}).constructor)('context','github',script)(context,github).catch(()=>{process.exitCode=1;});"
                result = subprocess.run(["node", "-e", program], stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
                self.assertEqual(result.returncode, 0 if allowed else 1)
        self.verify()
        # The exact verify descriptor has a separate current run and approved
        # prior bounds hashes. Reuse the same actual Node validator.
        raw = tool.canonical_bytes(self.value)
        context["runId"] = 900; inputs.update(prepare_mode=self.args.prepare_mode, bootstrap_approval_json=raw[:-1].decode(), bootstrap_approval_sha256=hashlib.sha256(raw).hexdigest())
        context["payload"]["inputs"] = inputs
        program = "process.env.GITHUB_RUN_ID='900';process.env.GITHUB_RUN_ATTEMPT='1';const context="+json.dumps(context)+";const script="+json.dumps(script)+";const github={rest:{repos:{getCommit:async()=>({data:{sha:'"+SOURCE+"'}})}}};new(Object.getPrototypeOf(async function(){}).constructor)('context','github',script)(context,github).catch(()=>{process.exitCode=1;});"
        self.assertEqual(subprocess.run(["node", "-e", program], capture_output=True, timeout=10).returncode, 0)

    def test_real_shell_package_exact_11_with_stub_go_no_actual_compilation(self):
        workflow = (ROOT / ".github/workflows/compatibility-retirement.yml").read_text()
        body = workflow.split("      - name: Package only immutable tooling", 1)[1].split("      - name:", 1)[0]
        script = textwrap.dedent(body.split("        run: |\n", 1)[1])
        with tempfile.TemporaryDirectory() as scratch:
            base = Path(scratch); runner = base / "runner"; runner.mkdir(); bin_dir = base / "bin"; bin_dir.mkdir()
            go = bin_dir / "go"
            go.write_text("#!/usr/bin/env python3\nimport os,sys,pathlib\na=sys.argv[1:]\nwith open(os.environ['GO_CALLS'],'a') as f:f.write(repr(a)+'\\n')\nif a and a[0]=='build':pathlib.Path(a[a.index('-o')+1]).write_text('synthetic-package-binary')\n")
            go.chmod(0o700)
            # Actual source cp/tar/trap is executed; only Go is an offline stub.
            source = base / "source"; source.mkdir()
            shutil.copytree(ROOT / "scripts", source / "scripts")
            env = dict(os.environ, COPYFILE_DISABLE="1", PATH=str(bin_dir)+":"+os.environ["PATH"], RUNNER_TEMP=str(runner), GITHUB_SHA=SOURCE,
                GITHUB_RUN_ID="900", GITHUB_RUN_ATTEMPT="1", GITHUB_OUTPUT=str(base / "output"), GO_CALLS=str(base / "calls"), RETIREMENT_PACKAGE_MODE="bootstrap-ai-bounds")
            result = subprocess.run(["bash", "-c", script], cwd=source, env=env, capture_output=True, timeout=30)
            self.assertEqual(result.returncode, 0, "offline package protocol failed")
            with tarfile.open(source / "qs-compatibility-retirement-900-1.tar.gz") as archive:
                names = archive.getnames()
            self.assertEqual(len(names), 11)
            self.assertNotIn("inventory-linux-amd64", names)
            self.assertIn("compatibility-ai-history-prepare.py", names)
            self.assertEqual((base / "calls").read_text().count("main.sourceSHA="+SOURCE), 2)
            self.assertEqual(list(runner.iterdir()), [])

    def test_dedicated_ssh_branch_native_explicit_pair_and_no_double_flock(self):
        workflow = (ROOT / ".github/workflows/compatibility-retirement.yml").read_text()
        step = workflow.split("      - name: Read actual AI bounds or verification", 1)[1].split("      - name:", 1)[0]
        self.assertIn("MONGODB_USERNAME: ${{ secrets.MONGODB_METADATA_ADMIN_USERNAME }}", step)
        self.assertIn("MONGODB_PASSWORD: ${{ secrets.MONGODB_METADATA_ADMIN_PASSWORD }}", step)
        self.assertNotIn("secrets.MONGODB_USERNAME", step)
        self.assertNotIn("secrets.MONGODB_PASSWORD", step)
        self.assertNotIn("flock", step); self.assertNotIn("docker", step)
        self.assertNotIn("inventory_binary", step); self.assertNotIn("go build", step)
        generic = workflow.split("      - name: Inventory source bytes", 1)[1].split("      - name:", 1)[0]
        self.assertIn("inputs.prepare_mode != 'bootstrap-ai-bounds'", generic)
        self.assertIn("inputs.prepare_mode != 'bootstrap-ai-verify'", generic)


if __name__ == "__main__":
    unittest.main()
