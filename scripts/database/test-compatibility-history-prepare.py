#!/usr/bin/env python3
"""Synthetic Action/host contracts. No database or production calls."""
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
import stat
import subprocess
import tempfile
import textwrap
import unittest
import uuid
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]
def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, ROOT / "scripts/database" / filename)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value

tool = load("retirement_history_contract_parent", "compatibility-retirement.py")
history = load("retirement_history_contract_wrapper", "compatibility-history-prepare.py")
SOURCE = "a" * 40
OPERATION = "123-1"
RUN = "900-1"
CID = "c" * 64
IMAGE = "sha256:" + "d" * 64


def native_fixture_paths(env):
    binary_value = env.get("QS_HISTORY_WRAPPER_NATIVE_BINARY")
    receipt_value = env.get("QS_HISTORY_WRAPPER_NATIVE_RECEIPT_DIRECTORY")
    if not binary_value or not receipt_value:
        tool.fail("history_native_fixture_environment_required")
    binary, receipt_directory = Path(binary_value), Path(receipt_value)
    tool.private_directory(binary.parent)
    tool.private_directory(receipt_directory)
    if not binary.is_absolute() or ".." in binary.parts:
        tool.fail("history_native_binary_invalid")
    try:
        before = binary.lstat()
        fd = os.open(binary, os.O_RDONLY | os.O_NOFOLLOW)
    except OSError:
        tool.fail("history_native_binary_invalid")
    try:
        after = os.fstat(fd)
        if (not os.path.samestat(before, after) or not stat.S_ISREG(after.st_mode) or
            after.st_uid != os.getuid() or stat.S_IMODE(after.st_mode) != 0o700 or
            after.st_nlink != 1 or not 0 < after.st_size <= 256 << 20):
            tool.fail("history_native_binary_invalid")
    finally:
        os.close(fd)
    receipt = receipt_directory / "docker-native.json"
    if receipt.exists() or receipt.is_symlink():
        meta = receipt.lstat()
        if (not stat.S_ISREG(meta.st_mode) or meta.st_uid != os.getuid() or
            stat.S_IMODE(meta.st_mode) != 0o600 or meta.st_nlink != 1):
            tool.fail("history_native_receipt_invalid")
    return binary, receipt


def write_native_receipt(path, value):
    # The explicitly supplied fixture directory is private and checked above.
    # Preserve no-follow even when rewriting this producer's own prior receipt.
    tool.private_directory(path.parent)
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, "wb") as stream:
        meta = os.fstat(stream.fileno())
        if (not stat.S_ISREG(meta.st_mode) or meta.st_uid != os.getuid() or
            stat.S_IMODE(meta.st_mode) != 0o600 or meta.st_nlink != 1):
            tool.fail("history_native_receipt_invalid")
        os.ftruncate(stream.fileno(), 0)
        stream.write(tool.canonical_bytes(value))
        stream.flush()
        os.fsync(stream.fileno())


class HistoryPreparation(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name).resolve() / "backups/qs-server/compatibility-retirement"
        self.root.mkdir(parents=True, mode=0o700)
        self.directory = self.root / OPERATION
        self.directory.mkdir(mode=0o700)
        self.inventory_dir = self.directory / "inventory-700-1"
        self.inventory_dir.mkdir(mode=0o700)
        request_hash = self.store(self.directory, "inventory-request.json", {"synthetic_private_input": 1})
        self.assets = []
        self.targets = []
        for i, target in enumerate(tool.TARGETS):
            raw = b"" if i == 3 else b"synthetic-private-source-row-" + str(i).encode()
            p = self.inventory_dir / tool.SOURCE_FILENAMES[target[:2]]
            p.write_bytes(raw); p.chmod(0o600)
            self.assets.append({"database": target[0], "name": target[1], "path": str(p),
                "full_file_sha256": hashlib.sha256(raw).hexdigest(), "full_file_bytes": len(raw)})
            self.targets.append({"database": target[0], "name": target[1], "records": int(i != 3),
                "bytes": len(raw), "data_hash": str(i + 1) * 64})
        self.inventory = {"source_sha": SOURCE, "operation_id": OPERATION, "run_id": "700-1",
            "request_hash": request_hash, "complete": True, "drop_ready": False,
            "diagnostic_only": True, "error_category": "none", "targets": self.targets}
        report_hash = self.store(self.inventory_dir, "inventory.private.json", self.inventory)
        self.parent = {"format_version": 1, "kind": "readonly_compatibility_history_request",
            "source_sha": SOURCE, "operation_id": OPERATION, "run_id": "800-1",
            "inventory_request": {"path": str(self.directory / "inventory-request.json"), "sha256": request_hash},
            "inventory_report": {"path": str(self.inventory_dir / "inventory.private.json"), "sha256": report_hash},
            "assets": self.assets}
        self.parent_hash = self.store(self.directory, "history-request.json", self.parent)
        self.value = {"format_version": 1, "kind": "readonly_history_bootstrap_approval",
            "prepare_mode": history.MODE, "source_sha": SOURCE, "operation_id": OPERATION,
            "target_hash": tool.TARGET_HASH, "database_scope": "mysql-and-mongodb",
            "parent_request": {"run_id": "800-1", "sha256": self.parent_hash},
            "inventory_request_sha256": request_hash,
            "inventory_report": {"run_id": "700-1", "sha256": report_hash},
            "assets": [{k: v for k, v in a.items() if k != "path"} for a in self.assets],
            "process_limits": history.LIMITS.copy()}
        self.binary = Path(self.temp.name).resolve() / "history-binary"
        self.binary.write_text('#!/usr/bin/env python3\nprint(\'{"source_sha":"' + SOURCE + '"}\')\n')
        self.binary.chmod(0o700)
        self.args = argparse.Namespace(operation="prepare", root=str(self.root), operation_id=OPERATION,
            actual_source_sha=SOURCE, approved_source_sha=SOURCE, run_id=RUN, manifest_hash="",
            inventory_request_hash="", identity_request_hash="", prepare_mode=history.MODE,
            bootstrap_approval_json="", bootstrap_approval_hash="", history_binary=str(self.binary))
        self.approve()
        self.commands = []
        self.state = None
        self.failure = None
        self.original_capture = tool.capture_fixed
        self.env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"), "MYSQL_HOST": "synthetic-mysql",
            "MYSQL_USERNAME": "synthetic-reader", "MYSQL_PASSWORD": "synthetic-sql-secret",
            "MYSQL_DATABASE": "synthetic-db", "MONGODB_HOST": "synthetic-mongo",
            "MONGODB_USERNAME": "synthetic-app", "MONGODB_PASSWORD": "synthetic-app-secret",
            "MONGODB_DBNAME": "synthetic-db", "MONGODB_METADATA_ADMIN_USERNAME": "synthetic-admin",
            "MONGODB_METADATA_ADMIN_PASSWORD": "synthetic-admin-secret"}

    def tearDown(self):
        self.temp.cleanup()

    def store(self, directory, filename, value):
        raw = tool.canonical_bytes(value)
        p = directory / filename; p.write_bytes(raw); p.chmod(0o600)
        return hashlib.sha256(raw).hexdigest()

    def approve(self):
        raw = tool.canonical_bytes(self.value)
        self.args.bootstrap_approval_json = raw[:-1].decode()
        self.args.bootstrap_approval_hash = hashlib.sha256(raw).hexdigest()

    def blocked(self, category, fn, *args):
        with self.assertRaisesRegex(tool.Blocked, "^" + category + "$"):
            fn(*args)

    def readiness(self, derived):
        v = {key: False for key in history.BOOL_FIELDS}
        v.update({key: "e" * 64 for key in history.HASH_FIELDS})
        v.update({key: 0 for key in history.UINT_FIELDS})
        v.update(protocol="qs-compatibility-history-readonly/v1", source_sha=SOURCE,
            operation_id=OPERATION, run_id=RUN, request_sha256=derived,
            inventory_request_sha256=self.parent["inventory_request"]["sha256"],
            inventory_report_sha256=self.parent["inventory_report"]["sha256"],
            independent_epochs=2, full_source_file_sha256=[a["full_file_sha256"] for a in self.assets],
            sources=[{"protocol": "mysql_cast_binary_columns_pk_order_v2" if i != 3 else "mongodb_server_bson_pk_order_v2",
                      "records": t["records"], "source_bytes": t["bytes"], "data_hash": t["data_hash"],
                      "complete": True, "business_closure_verified": False, "drop_ready": False}
                     for i, t in enumerate(self.targets)],
            sql_global={"observed": 5, "retirement_related": 2, "outside_retirement": 2, "unknown": 1,
                "blocking": 1, "schema_coverage": "synthetic_fixed_scope"},
            mongo_global={"rows": 6, "classified_rows": 6, "class_counts": {"non_target": 6},
                "blocking_reasons": ["synthetic_gap"], "coverage_gaps": []},
            blocking_reasons={"external_ai_closure_required": 1}, required_adapters=["writer_fence_required"],
            error_category="none", local_candidates=3, locally_qualified=2, blocked_local=1, ai_blocked_pages=1)
        for key in ("completed_readonly_pipeline", "source_files_and_actual_origins_matched",
            "whole_four_source_coverage_complete", "business_and_responsibility_facts_unchanged"):
            v[key] = True
        return v

    def capture(self, command, **kwargs):
        self.commands.append(command)
        if command[0] == str(self.binary):
            return self.original_capture(command, **kwargs)
        tokens = command[3:]
        if tokens[:2] == ["image", "inspect"]:
            if tokens[-1] == "{{json .Config.Volumes}}":
                return 0, tool.canonical_bytes(history.IMAGE_VOLUMES)
            return (0, b"{}\n") if tokens[-1] == "{{json .Config.Labels}}" else (0, (IMAGE + "\n").encode())
        if tokens[:2] == ["network", "inspect"]:
            return 0, b"[]\n"
        if tokens[:2] == ["container", "ls"]:
            return 0, b"" if self.failure != "preexisting" else b"some-existing-id\n"
        if tokens[0] == "create":
            labels = dict(v.split("=", 1) for k, v in zip(tokens, tokens[1:]) if k == "--label")
            mounts = []
            for k, v in zip(tokens, tokens[1:]):
                if k == "--mount":
                    f = dict(x.split("=", 1) for x in v.split(",") if "=" in x)
                    mounts.append({"Type": "bind", "Source": f["source"], "Destination": f["target"], "RW": not v.endswith(",readonly")})
            self.state = {"id": CID, "name": "/qs-compatibility-history-" + RUN, "image": IMAGE,
                "labels": labels, "mounts": mounts, "status": "created", "running": False,
                "exit_code": 0, "oom": False, "memory": 3 << 30, "memory_swap": 3 << 30,
                "nano_cpus": 2_000_000_000, "network": "infra-network", "readonly": True,
                "user": str(os.getuid()) + ":" + str(os.getgid()), "privileged": False, "pids": 64,
                "cap_drop": ["ALL"], "security_opt": ["no-new-privileges:true"]}
            env_path = Path(tokens[tokens.index("--env-file") + 1])
            self.assertEqual(env_path.stat().st_mode & 0o777, 0o600)
            self.assertIn("MONGODB_USERNAME=synthetic-admin\n", env_path.read_text())
            self.assertNotIn("synthetic-admin-secret", " ".join(command))
            if self.failure == "create_unknown": tool.fail("inventory_runtime_failed")
            return 0, (CID + "\n").encode()
        if tokens[:2] == ["container", "inspect"]:
            state = copy.deepcopy(self.state)
            if self.failure == "identity": state["id"] = "b" * 64
            if self.failure == "mount": state["mounts"][0]["RW"] = True
            return 0, tool.canonical_bytes(state)
        if tokens[:2] == ["start", "--attach"]:
            if self.failure == "timeout":
                self.state.update(status="running", running=True)
                tool.fail("inventory_runtime_failed")
            request_path = self.directory / ("history-" + RUN) / "history.request.json"
            digest = hashlib.sha256(request_path.read_bytes()).hexdigest()
            summary = self.readiness(digest)
            if self.failure == "close": summary["error_category"] = "history_sql_close_failed"
            raw = tool.canonical_bytes(summary)
            if self.failure != "no_private":
                self.store(request_path.parent / "output", "history.readiness.json", summary)
            if self.failure == "parent_changed":
                self.assets[0]["path"] and Path(self.assets[0]["path"]).write_bytes(b"tampered")
            self.state.update(status="exited", running=False, exit_code=int(self.failure == "close"))
            return self.state["exit_code"], raw
        if tokens[:2] == ["container", "rm"]:
            self.assertEqual(tokens, ["container", "rm", CID])
            return 0, (CID + "\n").encode()
        raise AssertionError("unexpected synthetic command category")

    def run_prepare(self):
        with mock.patch.dict(os.environ, self.env, clear=True), mock.patch.object(tool, "capture_fixed", side_effect=self.capture):
            return history.prepare(self.args, tool)

    def test_actual_derivation_only_run_changes_and_diagnostic_not_authority(self):
        old = (self.directory / "history-request.json").read_bytes()
        receipt = self.run_prepare()
        self.assertEqual(old, (self.directory / "history-request.json").read_bytes())
        p = self.directory / ("history-" + RUN)
        derived = json.loads((p / "history.request.json").read_bytes())
        self.assertEqual(derived, dict(self.parent, run_id=RUN))
        registry = json.loads((p / "history.bootstrap.json").read_bytes())
        self.assertEqual(registry["approval"], self.value)
        self.assertEqual(registry["parent_request_sha256"], self.parent_hash)
        self.assertNotEqual(registry["derived_request_sha256"], self.parent_hash)
        self.assertTrue(receipt["history_readonly_complete"])
        self.assertTrue(receipt["diagnostic_only"])
        for key in ("complete", "execution_allowed", "drop_ready", "history_cas_complete", "history_process_budget_proven"):
            self.assertIs(receipt[key], False)
        self.assertEqual(receipt["history_global_sql"]["unknown"], 1)
        self.assertEqual(receipt["history_source_rows"], [1, 1, 1, 0])
        self.assertFalse(tool.CAPABILITIES["history_verifier"])
        self.assertEqual(len(self.state["mounts"]), 4)
        mysql = next(m for m in self.state["mounts"] if m["Destination"] == "/var/lib/mysql")
        self.assertFalse(mysql["RW"])
        self.assertEqual(list(Path(mysql["Source"]).iterdir()), [])

    def test_reused_actual_run_rejected_in_both_original_references(self):
        for key in ("parent_request", "inventory_report"):
            with self.subTest(key=key):
                self.value[key]["run_id"] = RUN; self.approve()
                self.blocked("history_actual_run_reused", history.approval, self.args, tool)
                self.value[key]["run_id"] = "800-1" if key == "parent_request" else "700-1"

    def test_canonical_encoding_unknown_fields_duplicate_keys_and_budget_rejected(self):
        self.args.bootstrap_approval_json += " "
        self.blocked("history_approval_encoding_or_hash_invalid", history.approval, self.args, tool)
        self.value["extra"] = True; self.approve()
        self.blocked("evidence_fields_invalid", history.approval, self.args, tool)
        del self.value["extra"]
        self.value["process_limits"]["memory_swap_bytes"] += 1; self.approve()
        self.blocked("history_process_limits_invalid", history.approval, self.args, tool)
        self.args.bootstrap_approval_json = '{"a":1,"a":1}'
        self.blocked("evidence_duplicate_key", history.approval, self.args, tool)

    def test_parent_bytes_changed_path_rewrite_and_hash_swap_rejected(self):
        self.parent["inventory_report"]["path"] = "/different/private/path"
        self.value["parent_request"]["sha256"] = self.store(self.directory, "history-request.json", self.parent)
        self.approve()
        self.blocked("history_parent_path_or_hash_invalid", self.run_prepare)

    def test_source_symlink_hardlink_and_mode_refused(self):
        p = Path(self.assets[0]["path"])
        p.chmod(0o644)
        self.blocked("history_private_asset_invalid", self.run_prepare)
        p.chmod(0o600)
        link = p.with_name("duplicate")
        os.link(p, link)
        self.blocked("history_private_asset_invalid", self.run_prepare)
        link.unlink(); p.rename(link); p.symlink_to(link)
        self.blocked("history_private_asset_unavailable", self.run_prepare)

    def test_preexisting_container_never_started_removed_or_retried(self):
        self.failure = "preexisting"
        self.blocked("history_container_name_not_available", self.run_prepare)
        self.assertFalse(any("create" in c or "start" in c or "rm" in c for c in self.commands))

    def test_unknown_declared_image_volume_refused_before_container_creation(self):
        def capture(command, **kwargs):
            if command[-1] == "{{json .Config.Volumes}}":
                return 0, tool.canonical_bytes({"/var/lib/mysql": {}, "/unapproved": {}})
            return self.capture(command, **kwargs)
        with mock.patch.dict(os.environ,self.env,clear=True), mock.patch.object(tool,"capture_fixed",side_effect=capture):
            self.blocked("history_runtime_image_volumes_unsupported",history.prepare,self.args,tool)
        self.assertFalse(any("create" in c or "start" in c or "rm" in c for c in self.commands))

    def test_history_incomplete_admin_pair_never_falls_back_or_starts_container(self):
        del self.env["MONGODB_METADATA_ADMIN_PASSWORD"]
        self.blocked("inventory_connection_input_invalid",self.run_prepare)
        self.assertFalse(any("create" in c or "start" in c for c in self.commands))

    def test_unknown_execution_inspected_retained_and_not_auto_stopped_or_removed(self):
        self.failure = "timeout"
        self.blocked("history_container_execution_unknown", self.run_prepare)
        self.assertEqual(sum(c[3:5] == ["container", "inspect"] for c in self.commands), 2)
        self.assertFalse(any("rm" in c or "stop" in c or "restart" in c for c in self.commands))
        self.assertTrue((self.directory / ("history-" + RUN) / "history.container.json").is_file())

    def test_unknown_creation_reconciles_actual_id_but_never_starts_or_removes(self):
        self.failure = "create_unknown"
        self.blocked("history_container_creation_unknown", self.run_prepare)
        p = self.directory / ("history-" + RUN)
        intent = json.loads((p / "history.creation.intent.json").read_bytes())
        unknown = json.loads((p / "history.creation.unknown.json").read_bytes())
        self.assertEqual(unknown["id"], CID)
        self.assertEqual(len(intent["labels"]["qs.compatibility-retirement.creation"]), 64)
        self.assertFalse(any("start" in c or "rm" in c or "restart" in c for c in self.commands))

    def test_unknown_prior_attempt_cannot_be_evaded_by_a_new_run(self):
        self.failure = "timeout"
        self.blocked("history_container_execution_unknown", self.run_prepare)
        self.args.run_id = "900-2"
        self.failure = None
        before = len(self.commands)
        self.blocked("history_prior_container_outcome_unresolved", self.run_prepare)
        self.assertFalse(any("create" in c or "start" in c or "rm" in c for c in self.commands[before:]))

    def test_prior_terminal_marker_requires_fresh_actual_container_absence(self):
        self.run_prepare()
        def captured(command, **kwargs):
            if command[3:5] == ["container", "ls"] and any(x == "id="+CID for x in command):
                return 0, (CID+"\n").encode()
            return self.capture(command, **kwargs)
        with mock.patch.object(tool,"capture_fixed",side_effect=captured):
            self.blocked("history_prior_container_outcome_unresolved",history._prior_runs,tool,
                ["sudo","-n","docker"],self.directory)

    def test_prior_complete_terminal_marker_revalidated_without_new_execution(self):
        self.run_prepare()
        before = len(self.commands)
        with mock.patch.object(tool,"capture_fixed",side_effect=self.capture):
            history._prior_runs(tool,["sudo","-n","docker"],self.directory)
        self.assertEqual(len(self.commands)-before,2)
        self.assertEqual(self.commands[-1][3:5],["container","ls"])

    def test_actual_handle_identity_and_mount_mismatch_stop_before_start(self):
        for failure, category in (("identity", "history_container_ownership_unproven"), ("mount", "history_container_mounts_unproven")):
            with self.subTest(failure=failure):
                self.failure = failure
                self.blocked(category, self.run_prepare)
                self.assertFalse(any("start" in c or "rm" in c for c in self.commands))
                shutil.rmtree(self.directory / ("history-" + RUN)); self.commands.clear()

    def test_source_postrun_change_missing_private_and_close_failure_never_complete(self):
        for failure, category in (("no_private", "evidence_unavailable"), ("close", "history_readiness_outcome_invalid"), ("parent_changed", "history_private_asset_hash_mismatch")):
            with self.subTest(failure=failure):
                self.failure = failure
                if failure == "parent_changed":
                    self.blocked(category, self.run_prepare)
                else:
                    with self.assertRaises(tool.Blocked): self.run_prepare()
                self.assertFalse(any("rm" in c for c in self.commands))
                shutil.rmtree(self.directory / ("history-" + RUN)); self.commands.clear()

    def test_binary_source_reply_exact_json_bytes_and_owner_mode(self):
        with mock.patch.object(tool, "capture_fixed", return_value=(0, b' {"source_sha":"'+SOURCE.encode()+b'"}\n')):
            self.blocked("history_binary_source_json_invalid", history._binary, tool, self.binary, SOURCE)
        self.binary.chmod(0o755)
        self.blocked("history_binary_invalid", history._binary, tool, self.binary, SOURCE)

    def test_readiness_wrong_run_single_epoch_slot_authority_and_private_bytes_reject(self):
        out = self.directory / "receipt-test"; out.mkdir(mode=0o700)
        request = dict(self.parent, run_id=RUN)
        derived = hashlib.sha256(tool.canonical_bytes(request)).hexdigest()
        mutations = [lambda v: v.update(run_id="800-1"), lambda v: v.update(independent_epochs=1),
            lambda v: v.update(cas_complete=True), lambda v: v["sources"][2].update(complete=False),
            lambda v: v.update(whole_four_source_coverage_complete=False), lambda v: v.update(extra="private-body")]
        for change in mutations:
            v = self.readiness(derived); change(v)
            raw = tool.canonical_bytes(v); self.store(out, "history.readiness.json", v)
            with self.assertRaises(tool.Blocked):
                history._receipt(tool, v, raw, 0, self.args, out, request, derived, "1"*64, self.parent_hash, self.inventory)
        v = self.readiness(derived); self.store(out, "history.readiness.json", v)
        with self.assertRaises(tool.Blocked):
            history._receipt(tool, v, tool.canonical_bytes(v)+b" ", 0, self.args, out, request, derived, "1"*64, self.parent_hash, self.inventory)

    def test_parent_main_actual_wrapper_armor_and_diagnostic_exit0(self):
        args = ["--operation","prepare","--root",str(self.root),"--operation-id",OPERATION,
            "--approved-source-sha",SOURCE,"--actual-source-sha",SOURCE,"--run-id",RUN,
            "--prepare-mode",history.MODE,"--bootstrap-approval-json",self.args.bootstrap_approval_json,
            "--bootstrap-approval-hash",self.args.bootstrap_approval_hash,"--history-binary",str(self.binary)]
        stdout = io.StringIO()
        with mock.patch.dict(os.environ,self.env,clear=True), mock.patch.object(tool,"capture_fixed",side_effect=self.capture), contextlib.redirect_stdout(stdout):
            code = tool.main(args)
        self.assertEqual(code, 0)
        decoded = json.loads(tool.transport().decode_armored_receipt(stdout.getvalue()))
        self.assertTrue(decoded["history_readonly_complete"])
        self.assertFalse(decoded["execution_allowed"])
        self.assertFalse(decoded["drop_ready"])
        self.assertNotIn("synthetic-admin-secret", stdout.getvalue())
        self.assertNotIn("synthetic-private-source-row", stdout.getvalue())

    def test_parent_armor_failure_never_returns_success(self):
        receipt = {"format_version": 1, "prepare_mode": history.MODE, "complete": False,
            "execution_allowed": False, "drop_ready": False, "history_readonly_complete": True,
            "error_category": "history_readonly_completed_diagnostic"}
        args = ["--operation","prepare","--operation-id",OPERATION,"--approved-source-sha",SOURCE,
            "--actual-source-sha",SOURCE,"--run-id",RUN]
        with mock.patch.object(tool,"execute",return_value=receipt), mock.patch.object(tool,"transport",side_effect=RuntimeError), contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(tool.main(args), 42)

    def test_older_identity_bounds_inventory_modes_still_exit42(self):
        for mode in ("identity", "bounds", "inventory"):
            with self.subTest(mode=mode):
                receipt = {"format_version": 1, "prepare_mode": mode, "complete": False,
                    "execution_allowed": False, "drop_ready": False, "history_readonly_complete": True,
                    "error_category": "preparation_blocked"}
                args = ["--operation","prepare","--operation-id",OPERATION,"--approved-source-sha",SOURCE,
                    "--actual-source-sha",SOURCE,"--run-id",RUN]
                with mock.patch.object(tool,"execute",return_value=receipt), contextlib.redirect_stdout(io.StringIO()):
                    self.assertEqual(tool.main(args), 42)

    def test_real_container_projection_rejects_budget_privilege_or_owner_drift(self):
        self.run_prepare()
        baseline = copy.deepcopy(self.state)
        mounts = {v["Destination"]: (v["Source"], v["RW"]) for v in baseline["mounts"]}
        for key, changed in (("memory_swap", 4<<30), ("privileged", True), ("pids", 0),
            ("user", "unowned:unowned"), ("readonly", False), ("cap_drop", []), ("nano_cpus", 0),
            ("memory", float(3<<30))):
            with self.subTest(key=key):
                state = dict(baseline, **{key: changed})
                with mock.patch.object(tool,"capture_fixed",return_value=(0,tool.canonical_bytes(state))):
                    self.blocked("history_container_ownership_unproven",history._inspect,tool,
                        ["sudo","-n","docker"],CID,"qs-compatibility-history-"+RUN,IMAGE,baseline["labels"],mounts)

    def test_actual_node_validation_positive_and_negative_approval_matrix(self):
        if not shutil.which("node"): self.skipTest("Node unavailable")
        workflow = (ROOT / ".github/workflows/compatibility-retirement.yml").read_text()
        script = textwrap.dedent(workflow.split("          script: |\n",1)[1].split("      - name:",1)[0])
        cases = [("valid", lambda v: None, True),
            ("old_run_as_actual", lambda v: v["parent_request"].update(run_id=RUN), False),
            ("extra_swap", lambda v: v["process_limits"].update(memory_swap_bytes=4<<30), False),
            ("numeric_string", lambda v: v["assets"][0].update(full_file_bytes="1"), False),
            ("missing_asset", lambda v: v["assets"].pop(), False),
            ("wrong_source", lambda v: v.update(source_sha="b"*40), False),
            ("extra_field", lambda v: v.update(unapproved=True), False),
            ("trusted_context_run_mismatch", lambda v: None, False),
            ("trusted_attempt_invalid", lambda v: None, False)]
        for name, change, allowed in cases:
            with self.subTest(name=name):
                value = copy.deepcopy(self.value); change(value)
                raw = tool.canonical_bytes(value)
                inputs = {"operation":"prepare","database":"mysql-and-mongodb","approved_source_sha":SOURCE,
                    "operation_id":OPERATION,"prepare_mode":history.MODE,"bootstrap_approval_json":raw[:-1].decode(),
                    "bootstrap_approval_sha256":hashlib.sha256(raw).hexdigest()}
                context = {"payload":{"inputs":inputs},"ref":"refs/heads/main","sha":SOURCE,"repo":{},"runId":900}
                if name == "trusted_context_run_mismatch": context["runId"] = 901
                attempt = "invalid" if name == "trusted_attempt_invalid" else "1"
                program = ("process.env.GITHUB_RUN_ID='900';process.env.GITHUB_RUN_ATTEMPT="+json.dumps(attempt)+";const script="+json.dumps(script)
                    +";const context="+json.dumps(context)+";const github={rest:{repos:{getCommit:async()=>({data:{sha:'"+SOURCE+"'}})}}};"
                    +"new (Object.getPrototypeOf(async function(){}).constructor)('context','github',script)(context,github).catch(()=>{process.exitCode=1;});")
                result = subprocess.run(["node","-e",program],capture_output=True,check=False)
                self.assertEqual(result.returncode,0 if allowed else 1)

    def test_native_fixture_requires_explicit_private_owned_inputs(self):
        values = {"QS_HISTORY_WRAPPER_NATIVE_BINARY": str(self.binary),
            "QS_HISTORY_WRAPPER_NATIVE_RECEIPT_DIRECTORY": str(self.directory)}
        self.assertEqual(native_fixture_paths(values), (self.binary, self.directory / "docker-native.json"))
        for key in values:
            changed = dict(values); del changed[key]
            self.blocked("history_native_fixture_environment_required", native_fixture_paths, changed)
        self.binary.chmod(0o600)
        self.blocked("history_native_binary_invalid", native_fixture_paths, values)
        self.binary.chmod(0o700)
        alias = self.binary.parent / "native-symlink"; alias.symlink_to(self.binary)
        self.blocked("history_native_binary_invalid", native_fixture_paths, dict(values, QS_HISTORY_WRAPPER_NATIVE_BINARY=str(alias)))
        self.directory.chmod(0o755)
        self.blocked("operation_directory_not_private", native_fixture_paths, values)
        self.directory.chmod(0o700)
        receipt = self.directory / "docker-native.json"; receipt.symlink_to(self.binary)
        self.blocked("history_native_receipt_invalid", native_fixture_paths, values)

    @unittest.skipUnless(os.environ.get("QS_HISTORY_WRAPPER_NATIVE") == "1", "explicit local Docker fixture opt-in required")
    def test_actual_docker_declared_volume_shadow_full_handle_and_terminal_cleanup(self):
        """Run only --source-sha, never the MySQL entrypoint or a DB pipeline."""
        binary, record_path = native_fixture_paths(os.environ)
        def docker(*args):
            value = subprocess.run(["docker", *args], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, check=False, timeout=30)
            self.assertLessEqual(len(value.stdout), tool.MAX_JSON)
            return value.returncode, value.stdout
        code, raw = docker("image", "inspect", "mysql:8.0", "--format", "{{.Id}}")
        self.assertEqual(code, 0); image = raw.decode().strip()
        code, raw = docker("image", "inspect", image, "--format", "{{json .Config.Volumes}}")
        self.assertEqual(code, 0); self.assertEqual(tool.decode(raw), history.IMAGE_VOLUMES)
        code, raw = docker("image", "inspect", image, "--format", "{{json .Config.Labels}}")
        self.assertEqual(code, 0); inherited = tool.decode(raw) if raw.strip() != b"null" else {}
        root = self.directory
        out = root / "actual-native-output"; out.mkdir(mode=0o700)
        shadow = root / "actual-native-shadow"; shadow.mkdir(mode=0o700)
        nonce = uuid.uuid4().hex
        name = "qs-history-action-native-" + nonce
        labels = dict(inherited, **{"qs.compatibility-retirement.operation": OPERATION,
            "qs.compatibility-retirement.run": RUN, "qs.compatibility-retirement.source": SOURCE,
            "qs.compatibility-retirement.kind": "history-readonly-native-test",
            "qs.compatibility-retirement.creation": nonce})
        mounts = {"/history-tool": (str(binary), False), str(root): (str(root), False),
            str(out): (str(out), True), "/var/lib/mysql": (str(shadow), False)}
        code, volumes_before = docker("volume", "ls", "--format", "{{.Name}}")
        self.assertEqual(code, 0)
        command = ["create", "--name", name, "--pull=never", "--network=none", "--read-only", "--cap-drop=ALL",
            "--security-opt=no-new-privileges", "--cpus=2", "--memory=3g", "--memory-swap=3g", "--pids-limit=64",
            "--user", str(os.getuid())+":"+str(os.getgid())]
        for key, v in labels.items(): command.extend(("--label", key+"="+v))
        for target, (source, rw) in mounts.items():
            command.extend(("--mount", "type=bind,source="+source+",target="+target+("" if rw else ",readonly")))
        command.extend(("--entrypoint", "/history-tool", image, "--source-sha"))
        record = {"name":name,"image":image,"labels":labels,"mounts":mounts,
            "declared_volumes":history.IMAGE_VOLUMES,"network":"none","no_database_calls":True,"cleanup_confirmed":False}
        write_native_receipt(record_path, record)
        cid = None; qualified = False; terminal = False
        try:
            code, raw = docker(*command)
            self.assertEqual(code, 0); cid = raw.decode().strip()
            self.assertRegex(cid, r"^[0-9a-f]{64}$")
            record["id"] = cid
            write_native_receipt(record_path, record)
            state = history._inspect(tool,["docker"],cid,name,image,labels,mounts,network="none")
            self.assertEqual(state["status"],"created")
            self.assertFalse(state["running"])
            qualified = True
            code, raw = docker("start","--attach",cid)
            self.assertEqual(code, 0)
            self.assertEqual(raw, ('{"source_sha":"'+SOURCE+'"}\n').encode())
            state = history._inspect(tool,["docker"],cid,name,image,labels,mounts,network="none")
            self.assertEqual(state["status"],"exited");self.assertFalse(state["running"])
            self.assertEqual(state["exit_code"],0);self.assertFalse(state["oom"])
            terminal = True
            record.update(actual_handle_verified=True,actual_mounts_all_bind=True,
                actual_declared_volume_shadow_readonly=True,source_json_exact=True,
                actual_memory_bytes=state["memory"],actual_memory_swap_bytes=state["memory_swap"],actual_nano_cpus=state["nano_cpus"])
        finally:
            if qualified and terminal:
                code, _ = docker("container","rm",cid)
                self.assertEqual(code,0)
                code, raw = docker("container","ls","--all","--filter","id="+cid,"--format","{{.ID}}")
                self.assertEqual(code,0);self.assertEqual(raw,b"")
                code, volumes_after = docker("volume","ls","--format","{{.Name}}")
                self.assertEqual(code,0);self.assertEqual(volumes_before,volumes_after)
                self.assertEqual(list(shadow.iterdir()),[])
                record.update(cleanup_confirmed=True,anonymous_volume_created=False,
                    volume_catalog_unchanged=True,remaining_owned_containers=0)
                write_native_receipt(record_path,record)


if __name__ == "__main__":
    unittest.main()
