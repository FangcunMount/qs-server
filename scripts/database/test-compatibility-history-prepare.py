#!/usr/bin/env python3
"""Synthetic Action/host contracts. No database or production calls."""
import argparse
import base64
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
        fd = os.open(binary, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
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
            ai_reverse_global={"ledger_count": 14, "rows": 5, "retirement_related": 2,
                "outside_retirement": 2, "unknown": 1, "blocking": 1, "outside_active": 1,
                "data_sha256": "7"*64, "source_scope_sha256": "8"*64,
                "whole_ledger_eof": True, "independent_epoch_rechecked": True},
            blocking_reasons={"external_ai_closure_required": 1, "ai_reverse_global_unknown_responsibility": 1,
                "ai_reverse_global_blocking_responsibility": 1}, required_adapters=sorted(history.REQUIRED_ADAPTERS),
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

    def test_failed_readiness_accepts_fixed_ai_category_and_rejects_raw_or_false_success(self):
        out = self.directory / "category-test"; out.mkdir(mode=0o700)
        request = dict(self.parent, run_id=RUN)
        derived = hashlib.sha256(tool.canonical_bytes(request)).hexdigest()
        v = self.readiness(derived)
        v.update(completed_readonly_pipeline=False,
                 error_category="history_ai_readonly_resolver_failed")
        self.store(out, "history.readiness.json", v)
        result = history._receipt(tool, v, tool.canonical_bytes(v), 1, self.args,
            out, request, derived, "1"*64, self.parent_hash, self.inventory)
        self.assertFalse(result["history_readonly_complete"])
        self.assertFalse(result["execution_allowed"])
        self.assertFalse(result["drop_ready"])
        self.assertEqual(result["error_category"], "history_readonly_blocked")
        for category in ("none", "history_unknown_future_error", "private DSN and original body",
                         "history_failed\nprivate secret", 1, True, None, ["private"], {"private": "body"}):
            with self.subTest(category_type=type(category).__name__):
                v["error_category"] = category
                self.store(out, "history.readiness.json", v)
                self.blocked("history_readiness_category_invalid", history._receipt,
                    tool, v, tool.canonical_bytes(v), 1, self.args, out, request,
                    derived, "1"*64, self.parent_hash, self.inventory)
        v.update(completed_readonly_pipeline=True,
                 error_category="history_ai_readonly_resolver_failed")
        self.store(out, "history.readiness.json", v)
        self.blocked("history_readiness_category_invalid", history._receipt,
            tool, v, tool.canonical_bytes(v), 0, self.args, out, request,
            derived, "1"*64, self.parent_hash, self.inventory)

    def checked_readiness(self, change=None, code=0):
        out = self.directory / "ai-reverse-readiness"
        out.mkdir(mode=0o700, exist_ok=True)
        request = dict(self.parent, run_id=RUN)
        derived = hashlib.sha256(tool.canonical_bytes(request)).hexdigest()
        value = self.readiness(derived)
        if change:
            change(value)
        self.store(out, "history.readiness.json", value)
        return history._receipt(tool, value, tool.canonical_bytes(value), code, self.args,
            out, request, derived, "1"*64, self.parent_hash, self.inventory)

    def test_ai_reverse_completed_diagnostic_keeps_unknown_blocking_and_all_grants_false(self):
        result = self.checked_readiness()
        self.assertTrue(result["history_readonly_complete"])
        self.assertEqual(set(result["history_global_ai_reverse"]), history.AI_REVERSE_FIELDS)
        self.assertEqual(len(result["history_global_ai_reverse"]), 11)
        self.assertEqual(result["history_global_ai_reverse"]["unknown"], 1)
        self.assertEqual(result["history_global_ai_reverse"]["blocking"], 1)
        self.assertEqual(result["history_global_ai_reverse"]["outside_active"], 1)
        for key in ("complete", "execution_allowed", "drop_ready", "history_cas_complete", "history_process_budget_proven"):
            self.assertIs(result[key], False)

    def test_ai_reverse_empty_whole_scope_is_valid_and_not_missing_coverage(self):
        def empty(value):
            ai = value["ai_reverse_global"]
            for key in history.AI_REVERSE_UINT_FIELDS - {"ledger_count"}:
                ai[key] = 0
            value["blocking_reasons"] = {}
        result = self.checked_readiness(empty)
        self.assertTrue(result["history_readonly_complete"])
        self.assertEqual(result["history_global_ai_reverse"]["rows"], 0)
        self.assertEqual(result["history_global_ai_reverse"]["ledger_count"], 14)

    def test_ai_reverse_each_completed_coverage_requirement_is_mandatory(self):
        for key, wrong in (("ledger_count", 0), ("ledger_count", 13), ("ledger_count", 15),
            ("whole_ledger_eof", False), ("independent_epoch_rechecked", False),
            ("data_sha256", ""), ("source_scope_sha256", "")):
            with self.subTest(key=key, value=wrong):
                with self.assertRaises(tool.Blocked):
                    self.checked_readiness(lambda v: v["ai_reverse_global"].update({key: wrong}))

    def test_ai_reverse_every_counter_rejects_bool_negative_float_string_and_null(self):
        for key in sorted(history.AI_REVERSE_UINT_FIELDS):
            for wrong in (True, False, -1, 1.0, "1", None, 1 << 64, [], {}):
                with self.subTest(key=key, type=type(wrong).__name__):
                    self.blocked("evidence_type_invalid", self.checked_readiness,
                        lambda v: v["ai_reverse_global"].update({key: wrong}))

    def test_ai_reverse_booleans_are_actual_bools(self):
        for key in sorted(history.AI_REVERSE_BOOL_FIELDS):
            for wrong in (0, 1, "true", None, [], {}):
                with self.subTest(key=key, type=type(wrong).__name__):
                    self.blocked("history_ai_reverse_summary_invalid", self.checked_readiness,
                        lambda v: v["ai_reverse_global"].update({key: wrong}))

    def test_ai_reverse_hashes_are_empty_only_on_failure_or_exact_lowercase_hex(self):
        for key in sorted(history.AI_REVERSE_HASH_FIELDS):
            for wrong in ("a"*63, "a"*65, "A"*64, "g"*64, "a"*64+"\n", "sha256:"+"a"*64,
                          "synthetic-private-body", True, None, [], {}):
                with self.subTest(key=key, type=type(wrong).__name__):
                    self.blocked("evidence_type_invalid", self.checked_readiness,
                        lambda v: v["ai_reverse_global"].update({key: wrong}))

    def test_ai_reverse_missing_or_unknown_fields_and_nonobjects_cannot_carry_original_body(self):
        for key in sorted(history.AI_REVERSE_FIELDS):
            with self.subTest(missing=key):
                self.blocked("evidence_fields_invalid", self.checked_readiness,
                    lambda v: v["ai_reverse_global"].pop(key))
        for key in ("raw_body", "body", "wire", "projection", "request_id", "command_id", "global_reverse_qualified", "cas_authority", "drop_ready"):
            with self.subTest(unknown=key):
                self.blocked("evidence_fields_invalid", self.checked_readiness,
                    lambda v: v["ai_reverse_global"].update({key: "synthetic-private-body"}))
        for wrong in (None, [], True, "synthetic-private-body"):
            with self.subTest(type=type(wrong).__name__):
                self.blocked("evidence_fields_invalid", self.checked_readiness,
                    lambda v: v.update(ai_reverse_global=wrong))

    def test_ai_reverse_scope_counts_and_visible_blockers_cannot_be_forged(self):
        for change in (
            lambda v: v["ai_reverse_global"].update(rows=6),
            lambda v: v["ai_reverse_global"].update(outside_active=3),
            lambda v: v["ai_reverse_global"].update(blocking=0),
            lambda v: v["blocking_reasons"].pop("ai_reverse_global_unknown_responsibility"),
            lambda v: v["blocking_reasons"].update(ai_reverse_global_blocking_responsibility=2),
        ):
            with self.assertRaises(tool.Blocked):
                self.checked_readiness(change)

    def test_ai_reverse_structural_blockers_may_exceed_rows_without_becoming_a_grant(self):
        def structural(value):
            value["ai_reverse_global"]["blocking"] = 6
            value["blocking_reasons"]["ai_reverse_global_blocking_responsibility"] = 6
        result = self.checked_readiness(structural)
        self.assertEqual(result["history_global_ai_reverse"]["blocking"], 6)
        self.assertFalse(result["drop_ready"])

    def test_ai_reverse_all_four_new_fixed_failures_accept_honest_zero_or_partial_diagnostics(self):
        for category in ("history_ai_reverse_scan_failed", "history_ai_reverse_source_scope_failed",
                         "history_ai_reverse_coverage_incomplete", "history_ai_reverse_independent_epoch_failed"):
            for partial in (False, True):
                with self.subTest(category=category, partial=partial):
                    def failed(value):
                        value.update(completed_readonly_pipeline=False, independent_epochs=0 if not partial else 1,
                                     error_category=category, blocking_reasons={}, required_adapters=[])
                        value["ai_reverse_global"] = {key: 0 for key in history.AI_REVERSE_UINT_FIELDS}
                        value["ai_reverse_global"].update({key: "" for key in history.AI_REVERSE_HASH_FIELDS})
                        value["ai_reverse_global"].update({key: False for key in history.AI_REVERSE_BOOL_FIELDS})
                        if partial:
                            value["ai_reverse_global"].update(ledger_count=3, rows=2, unknown=2, blocking=2,
                                                              data_sha256="7"*64)
                    result = self.checked_readiness(failed, code=1)
                    self.assertFalse(result["history_readonly_complete"])
                    self.assertFalse(result["history_global_ai_reverse"]["independent_epoch_rechecked"])
                    self.assertEqual(result["error_category"], "history_readonly_blocked")
                    for key in ("complete", "execution_allowed", "drop_ready", "history_cas_complete"):
                        self.assertIs(result[key], False)

    def test_ai_reverse_failed_diagnostic_cannot_claim_inconsistent_positive_eof_or_fresh(self):
        for change in (
            lambda v: v["ai_reverse_global"].update(ledger_count=13),
            lambda v: v["ai_reverse_global"].update(data_sha256=""),
            lambda v: v["ai_reverse_global"].update(source_scope_sha256=""),
            lambda v: v["ai_reverse_global"].update(whole_ledger_eof=False),
            lambda v: v.update(independent_epochs=1),
        ):
            def failed(value):
                value.update(completed_readonly_pipeline=False, error_category="history_ai_reverse_independent_epoch_failed")
                change(value)
            self.blocked("history_ai_reverse_summary_invalid", self.checked_readiness, failed, 1)

    def test_ai_reverse_unfinished_read_may_report_whole_first_scope_without_fresh_claim(self):
        def failed(value):
            value.update(completed_readonly_pipeline=False, independent_epochs=1,
                         error_category="history_ai_reverse_independent_epoch_failed")
            value["ai_reverse_global"].update(independent_epoch_rechecked=False, source_scope_sha256="")
        result = self.checked_readiness(failed, 1)
        self.assertTrue(result["history_global_ai_reverse"]["whole_ledger_eof"])
        self.assertFalse(result["history_readonly_complete"])

    def test_ai_reverse_required_original_wire_adapter_is_retained_and_closed(self):
        required = "ai_stored_wire_original_jose_authentication"
        self.assertIn(required, history.BASE_REQUIRED_ADAPTERS)
        for change in (
            lambda v: v["required_adapters"].remove(required),
            lambda v: v["required_adapters"].append("future_unapproved_adapter"),
            lambda v: v["required_adapters"].append("synthetic_private_body"),
            lambda v: v["required_adapters"].append(required),
            lambda v: v.update(required_adapters=[{}]),
            lambda v: v.update(required_adapters=True),
        ):
            self.blocked("history_required_adapters_invalid", self.checked_readiness, change)

    def test_ai_reverse_cannot_upgrade_any_forbidden_pipeline_capability(self):
        for key in sorted(history.FORBIDDEN_TRUE):
            with self.subTest(key=key):
                self.blocked("history_readiness_authority_invalid", self.checked_readiness,
                    lambda v: v.update({key: True}))

    def test_ai_reverse_public_armor_has_exact_bodyfree_fields_and_rejects_illegal_types(self):
        good = self.checked_readiness()
        args = ["--operation", "prepare", "--operation-id", OPERATION,
                "--approved-source-sha", SOURCE, "--actual-source-sha", SOURCE, "--run-id", RUN]
        stdout = io.StringIO()
        with mock.patch.object(tool, "execute", return_value=good), contextlib.redirect_stdout(stdout):
            self.assertEqual(tool.main(args), 0)
        decoded = json.loads(tool.transport().decode_armored_receipt(stdout.getvalue()))
        self.assertEqual(decoded["history_global_ai_reverse"], good["history_global_ai_reverse"])
        for key, wrong in (("rows", True), ("whole_ledger_eof", 1), ("data_sha256", "A"*64),
                           ("source_scope_sha256", "synthetic-private-body"), ("body", "synthetic-private-body"),
                           ("global_reverse_qualified", True), ("drop_ready", True)):
            with self.subTest(key=key):
                value = copy.deepcopy(good); value["history_global_ai_reverse"][key] = wrong
                stdout, stderr = io.StringIO(), io.StringIO()
                with mock.patch.object(tool, "execute", return_value=value), contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
                    self.assertEqual(tool.main(args), 42)
                self.assertEqual(stdout.getvalue(), "")
                self.assertEqual(stderr.getvalue(), "compatibility_retirement_receipt_transport_failed\n")

    def test_ai_reverse_failed_public_armor_remains_exit42_with_empty_hashes(self):
        def failed(value):
            value.update(completed_readonly_pipeline=False, independent_epochs=0,
                         error_category="history_ai_reverse_scan_failed", blocking_reasons={})
            value["ai_reverse_global"] = {key: 0 for key in history.AI_REVERSE_UINT_FIELDS}
            value["ai_reverse_global"].update({key: "" for key in history.AI_REVERSE_HASH_FIELDS})
            value["ai_reverse_global"].update({key: False for key in history.AI_REVERSE_BOOL_FIELDS})
        result = self.checked_readiness(failed, 1)
        stdout = io.StringIO()
        args = ["--operation", "prepare", "--operation-id", OPERATION,
                "--approved-source-sha", SOURCE, "--actual-source-sha", SOURCE, "--run-id", RUN]
        with mock.patch.object(tool, "execute", return_value=result), contextlib.redirect_stdout(stdout):
            self.assertEqual(tool.main(args), 42)
        decoded = json.loads(tool.transport().decode_armored_receipt(stdout.getvalue()))
        self.assertFalse(decoded["history_readonly_complete"])
        self.assertEqual(decoded["history_global_ai_reverse"]["data_sha256"], "")
        self.assertFalse(decoded["drop_ready"])

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
        self.assertEqual(decoded["history_global_ai_reverse"]["ledger_count"], 14)
        self.assertIs(decoded["history_global_ai_reverse"]["whole_ledger_eof"], True)
        self.assertIs(decoded["history_global_ai_reverse"]["independent_epoch_rechecked"], True)
        self.assertEqual(decoded["history_global_ai_reverse"]["unknown"], 1)
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


class HistoryMetadata(unittest.TestCase):
    def setUp(self):
        self.fixture = HistoryPreparation(methodName="test_actual_derivation_only_run_changes_and_diagnostic_not_authority")
        self.fixture.setUp()
        self.directory = self.fixture.directory; self.output = self.fixture.inventory_dir
        (self.directory / "history-request.json").unlink()
        self.args = copy.deepcopy(self.fixture.args); self.args.prepare_mode = history.METADATA_MODE
        self.commands = []; self.failure = None
        def binding(database):
            v = {key: False for key in tool.INVENTORY_BINDING_FIELDS}
            v.update(identity_hash=("1" if database == "mysql" else "2")*64,
                database_anchor_hash=("1" if database == "mysql" else "3")*64,
                migration_generation_hash="" if database == "mysql" else "4"*64,
                expected_identity_match=True,migration_version=99 if database == "mysql" else 38,
                migration_dirty=False,expected_migration_match=True,catalog_hash="5"*64,non_target_schema_hash="6"*64,
                metadata_complete=True,permissions={"metadata_read":True},outside_dependencies=0,
                dependency_coverage_complete=False,inbound_foreign_key_coverage_complete=False,
                dependency_scope="selected_schema_outbound_only",dependency_text_review_required=True,error_category="none")
            return v
        self.bindings = {db:binding(db) for db in ("mysql","mongodb")}
        self.bounds = []
        for i,target in enumerate(tool.TARGETS):
            self.bounds.append({"database":target[0],"name":target[1],"kind":target[2],"present":True,"empty":i==3,
                "pk_type":"uint64" if target[0]=="mysql" else "","upper_token":base64.b64encode(b"1").decode() if i!=3 else "",
                "schema_hash":"7"*64,"identity_hash":self.bindings[target[0]]["identity_hash"]})
        bounds_dir=self.directory/"bounds-600-1";bounds_dir.mkdir(mode=0o700)
        bound_report={"format_version":2,"kind":"readonly_inventory_boundaries","source_sha":SOURCE,
            "operation_id":OPERATION,"run_id":"600-1","target_hash":tool.TARGET_HASH,"complete":True,
            "drop_ready":False,"diagnostic_only":True,"database_bindings":self.bindings,
            "targets":[{"boundary":b,"complete":True,"error_category":"none"} for b in self.bounds]}
        bound_hash=self.fixture.store(bounds_dir,"boundary.private.json",bound_report)
        self.request={"format_version":2,"kind":"readonly_inventory_request","source_sha":SOURCE,"operation_id":OPERATION,
            "target_hash":tool.TARGET_HASH,"database_scope":"mysql-and-mongodb",
            "identity_hashes":{db:v["identity_hash"] for db,v in self.bindings.items()},
            "expected_migrations":{db:v["migration_version"] for db,v in self.bindings.items()},"limits":tool.INVENTORY_V2_LIMITS.copy(),
            "boundary_run_id":"600-1","boundary_report_hash":bound_hash,"approved_boundaries":self.bounds}
        self.request_hash=self.fixture.store(self.directory,"inventory-request.json",self.request)
        targets=[]
        for i,(target,bound) in enumerate(zip(tool.TARGETS,self.bounds)):
            filename=tool.SOURCE_FILENAMES[target[:2]]
            # Raw database source bytes deliberately differ from encoded file bytes.
            targets.append({"database":target[0],"name":target[1],"kind":target[2],"present":True,"complete":True,
                "records":int(i!=3),"schema_hash":bound["schema_hash"],"data_hash":str(i+1)*64,"identity_hash":bound["identity_hash"],
                "bytes":5 if i!=3 else 0,"classification":{},"source_file":filename,"error_category":"none","boundary":bound,
                "equal_full_passes":2,"pages":2 if i!=3 else 0,"next_cycle_required":False})
            self.fixture.store(self.output,filename+".asset.json",{"format_version":1,"kind":"temporary_inventory_source_copy",
                "filename":filename,"source_sha":SOURCE,"operation_id":OPERATION,"run_id":"700-1","request_hash":self.request_hash,
                "protocol":"mysql_cast_binary_columns_pk_order_v2" if target[0]=="mysql" else "mongodb_server_bson_pk_order_v2",
                "boundary":bound,"contains_original_body":True,"retirement_proof":False,"purge_required_after_acceptance":True,"resume_existing_file_allowed":False})
        self.report={"format_version":2,"kind":"readonly_compatibility_inventory","source_sha":SOURCE,"operation_id":OPERATION,
            "run_id":"700-1","request_hash":self.request_hash,"target_hash":tool.TARGET_HASH,"observed_at":"2026-10-09T01:00:00Z",
            "complete":True,"drop_ready":False,"database_bindings":self.bindings,"targets":targets,
            "source_bytes_protocol":"mysql_cast_binary_columns_pk_order_v2+mongodb_server_bson_pk_order_v2",
            "consistency_semantics":"two_equal_complete_passes_within_independently_approved_upper;sql_same_readonly_snapshot;mongo_homogeneous_bson_id_simple_collation;after_upper_next_cycle_not_fenced",
            "error_category":"none","boundary_report_hash":bound_hash,"diagnostic_only":True}
        self.report_hash=self.fixture.store(self.output,"inventory.private.json",self.report)
        self.value={"format_version":1,"kind":"readonly_history_metadata_approval","prepare_mode":history.METADATA_MODE,
            "source_sha":SOURCE,"operation_id":OPERATION,"target_hash":tool.TARGET_HASH,"database_scope":"mysql-and-mongodb",
            "inventory_request_sha256":self.request_hash,"inventory_report":{"run_id":"700-1","sha256":self.report_hash},
            "metadata_limits":history.METADATA_LIMITS.copy()}
        self.approve()

    def tearDown(self): self.fixture.tearDown()

    def approve(self):
        raw=tool.canonical_bytes(self.value);self.args.bootstrap_approval_json=raw[:-1].decode()
        self.args.bootstrap_approval_hash=hashlib.sha256(raw).hexdigest()

    def capture(self, command, **kwargs):
        self.commands.append(command)
        self.assertEqual(command[:3],["sudo","-n","docker"])
        if command[3:5]==["container","ls"]:
            if self.failure=="unavailable":return 1,b""
            if self.failure=="name_hidden" and "name=^/qs-compatibility-inventory-700-1$" in command:return 0,(CID+"\n").encode()
            if self.failure=="whole_existing" and any(v.startswith("label=") for v in command):return 0,(CID+"\n").encode()
            return 0,b""
        if command[3]=="inspect": return 0,tool.canonical_bytes({"id":CID,"status":"exited","running":False})
        self.fail("metadata attempted non-readonly Docker call")

    def run_metadata(self):
        with mock.patch.object(tool,"capture_fixed",side_effect=self.capture),mock.patch.object(tool,"inventory_connection_values",side_effect=AssertionError("database values requested")),mock.patch.object(history,"_binary",side_effect=AssertionError("binary requested")):
            return history.prepare_metadata(self.args,tool)

    def test_two_complete_physical_passes_persisted_proposal_and_no_authority(self):
        result=self.run_metadata();self.assertTrue(result["history_metadata_complete"])
        for key in ("complete","execution_allowed","drop_ready","history_cas_complete","history_metadata_process_budget_proven"):self.assertIs(result[key],False)
        self.assertEqual(result["parent_proposal_run_id"],"700-1");self.assertEqual(result["metadata_created_run_id"],RUN)
        self.assertFalse((self.directory/"history-request.json").exists())
        private=self.directory/("history-metadata-"+RUN)
        proposal,digest=tool.read_private(private,"history-parent-proposal.json")
        self.assertEqual(digest,result["history_parent_proposal_sha256"]);self.assertEqual(proposal["run_id"],"700-1")
        self.assertNotEqual(result["history_metadata_assets"][0]["full_file_bytes"],self.report["targets"][0]["bytes"])
        self.assertEqual(result["history_metadata_assets"][3]["full_file_bytes"],0)
        self.assertEqual(sum(any(v.startswith("label=") for v in c) for c in self.commands),2)
        self.assertEqual(sum("name=^/qs-compatibility-inventory-700-1$" in c for c in self.commands),2)
        for p in private.iterdir():self.assertEqual(p.stat().st_mode&0o777,0o600)

    def test_canonical_descriptor_limits_run_source_and_unknown_fields_reject(self):
        for change in (lambda v:v.update(extra=True),lambda v:v.update(source_sha="b"*40),
            lambda v:v["inventory_report"].update(run_id=RUN),lambda v:v["metadata_limits"].update(passes=1),
            lambda v:v["metadata_limits"].update(total_seconds=True)):
            with self.subTest(change=change):
                changed=copy.deepcopy(self.value);change(changed);raw=tool.canonical_bytes(changed)
                args=copy.deepcopy(self.args);args.bootstrap_approval_json=raw[:-1].decode();args.bootstrap_approval_hash=hashlib.sha256(raw).hexdigest()
                with self.assertRaises(tool.Blocked):history.metadata_approval(args,tool)
        args=copy.deepcopy(self.args);args.bootstrap_approval_json+=' '
        with self.assertRaises(tool.Blocked):history.metadata_approval(args,tool)

    def test_existing_or_unknown_inventory_handles_not_owned_or_cleared(self):
        for failure in ("whole_existing","name_hidden","unavailable"):
            with self.subTest(failure=failure):
                self.failure=failure
                with self.assertRaises(tool.Blocked):self.run_metadata()
        self.assertTrue(any(c[3]=="inspect" for c in self.commands))
        self.assertFalse(any(c[3] in ("create","run","stop","rm","start") for c in self.commands))
        self.assertFalse((self.directory/("history-metadata-"+RUN)).exists())

    def test_old_history_unknown_attempt_also_blocks_metadata(self):
        prior=self.directory/"history-888-1";prior.mkdir(mode=0o700)
        self.fixture.store(prior,"history.creation.intent.json",{"unknown":True})
        with self.assertRaisesRegex(tool.Blocked,"history_prior_container_outcome_unresolved"):self.run_metadata()

    def test_prior_inventory_run_bound_rejects_without_docker_create(self):
        for i in range(128):(self.directory/("identity-"+str(1000+i)+"-1")).mkdir(mode=0o700)
        with self.assertRaisesRegex(tool.Blocked,"history_metadata_prior_runs_bound_exceeded"):self.run_metadata()
        self.assertEqual(self.commands,[])

    def test_report_or_sidecar_wrong_original_run_schema_null_and_unknown_reject(self):
        original=copy.deepcopy(self.report)
        for change in (lambda v:v.update(run_id=RUN),lambda v:v.update(source_sha="b"*40),lambda v:v.update(extra=0),
            lambda v:v["targets"][0].update(next_cycle_required=True),lambda v:v["targets"][1].update(complete=None),
            lambda v:v["database_bindings"]["mysql"].update(migration_dirty=True)):
            with self.subTest(change=change):
                self.report=copy.deepcopy(original);change(self.report)
                self.value["inventory_report"]["sha256"]=self.fixture.store(self.output,"inventory.private.json",self.report);self.approve()
                with self.assertRaises(tool.Blocked):self.run_metadata()
        self.report=original;self.value["inventory_report"]["sha256"]=self.fixture.store(self.output,"inventory.private.json",original);self.approve()
        filename=tool.SOURCE_FILENAMES[tool.TARGETS[0][:2]]+".asset.json"
        v,_=tool.read_private(self.output,filename);v["run_id"]=RUN;self.fixture.store(self.output,filename,v)
        with self.assertRaisesRegex(tool.Blocked,"inventory_source_asset_binding_invalid"):self.run_metadata()

    def test_file_symlink_hardlink_mode_and_source_bound_reject(self):
        path=self.output/tool.SOURCE_FILENAMES[tool.TARGETS[0][:2]];raw=path.read_bytes()
        path.chmod(0o644)
        with self.assertRaises(tool.Blocked):self.run_metadata()
        path.chmod(0o600);alias=self.output/"alias";os.link(path,alias)
        with self.assertRaises(tool.Blocked):self.run_metadata()
        alias.unlink();path.unlink();path.symlink_to(self.output/tool.SOURCE_FILENAMES[tool.TARGETS[1][:2]])
        with self.assertRaises(tool.Blocked):self.run_metadata()
        path.unlink();path.write_bytes(raw);path.chmod(0o600)
        with self.assertRaisesRegex(tool.Blocked,"history_metadata_file_invalid"):
            history._metadata_physical_file(tool,path,1,history.time.monotonic()+1)

    def test_same_size_mutation_between_passes_and_late_mutation_reject(self):
        original=history._metadata_physical_file;calls=0
        path=self.output/tool.SOURCE_FILENAMES[tool.TARGETS[0][:2]]
        def changed(*args):
            nonlocal calls
            row=original(*args);calls+=1
            if calls==4:path.write_bytes(b"X"*path.stat().st_size)
            return row
        with mock.patch.object(history,"_metadata_physical_file",side_effect=changed):
            with self.assertRaisesRegex(tool.Blocked,"history_metadata_file_changed"):self.run_metadata()
        self.assertFalse((self.directory/("history-metadata-"+RUN)).exists())

    def test_real_fifos_without_writer_are_immediately_rejected(self):
        fifo=self.output/"fifo.json";os.mkfifo(fifo,0o600)
        baseline=history._metadata_snapshot(fifo.lstat())
        started=history.time.monotonic()
        with self.assertRaisesRegex(tool.Blocked,"evidence_not_private"):tool.read_private(self.output,fifo.name)
        with self.assertRaisesRegex(tool.Blocked,"history_private_asset_invalid"):history._file_baseline(tool,fifo,hashlib.sha256(b"").hexdigest(),tool.MAX_JSON)
        with self.assertRaisesRegex(tool.Blocked,"history_metadata_file_invalid"):history._metadata_physical_file(tool,fifo,tool.MAX_JSON,history.time.monotonic()+1)
        with self.assertRaisesRegex(tool.Blocked,"history_metadata_file_changed"):history._metadata_current_stat(tool,fifo,baseline)
        private=self.directory/"fifo-readiness";private.mkdir(mode=0o700)
        os.mkfifo(private/"history.readiness.json",0o600)
        with self.assertRaisesRegex(tool.Blocked,"history_readiness_private_invalid"):history._readiness_file(tool,private)
        source=self.output/tool.SOURCE_FILENAMES[tool.TARGETS[0][:2]];source.unlink();os.mkfifo(source,0o600)
        original=copy.deepcopy(self.args);original.run_id="700-1"
        with self.assertRaisesRegex(tool.Blocked,"inventory_source_asset_invalid"):
            tool.validate_source_asset(self.output,self.report["targets"][0],original,self.request_hash,2<<30)
        self.assertLess(history.time.monotonic()-started,1.0)
        self.assertFalse((self.directory/("history-metadata-"+RUN)).exists())

    def test_source_or_private_record_changed_during_publication_cannot_complete(self):
        original=tool.create_bootstrap_file
        def mutate(directory,filename,raw):
            original(directory,filename,raw)
            if filename=="history-metadata.json":
                path=self.output/tool.SOURCE_FILENAMES[tool.TARGETS[0][:2]]
                path.write_bytes(b"Y"*path.stat().st_size)
        with mock.patch.object(tool,"create_bootstrap_file",side_effect=mutate):
            with self.assertRaisesRegex(tool.Blocked,"history_metadata_file_changed"):self.run_metadata()

    def test_private_readback_byte_hash_changed_cannot_complete(self):
        original=tool.create_bootstrap_file
        def mutate(directory,filename,raw):
            original(directory,filename,raw)
            if filename=="history-metadata.json":
                path=directory/filename;path.write_bytes(b" "+path.read_bytes())
        with mock.patch.object(tool,"create_bootstrap_file",side_effect=mutate):
            with self.assertRaisesRegex(tool.Blocked,"evidence_hash_mismatch"):self.run_metadata()

    def test_actual_source_stream_uses_bounded_chunks_and_two_eof_passes(self):
        source=self.output/tool.SOURCE_FILENAMES[tool.TARGETS[0][:2]]
        source.write_bytes(b"K"*(history.METADATA_LIMITS["stream_chunk_bytes"]*3+1));source.chmod(0o600)
        original=history._metadata_physical_file;calls=[]
        def observed(*args):
            result=original(*args);calls.append((args[1],result));return result
        with mock.patch.object(history,"_metadata_physical_file",side_effect=observed):result=self.run_metadata()
        self.assertEqual(len(calls),8)
        self.assertEqual(calls[:4],calls[4:])
        self.assertEqual(result["history_metadata_assets"][0]["full_file_sha256"],hashlib.sha256(source.read_bytes()).hexdigest())
        self.assertEqual(result["history_metadata_assets"][0]["full_file_bytes"],source.stat().st_size)

    def test_deadline_and_private_persistence_failure_never_complete(self):
        with mock.patch.object(history,"_metadata_time",side_effect=tool.Blocked("history_metadata_cooperative_deadline_exceeded")):
            with self.assertRaises(tool.Blocked):self.run_metadata()
        original=tool.create_bootstrap_file
        def fail_write(directory,filename,raw):
            if filename=="history-metadata.json":raise tool.Blocked("bootstrap_request_creation_incomplete")
            return original(directory,filename,raw)
        with mock.patch.object(tool,"create_bootstrap_file",side_effect=fail_write):
            with self.assertRaises(tool.Blocked):self.run_metadata()
        with self.assertRaisesRegex(tool.Blocked,"history_metadata_run_directory_exists_or_unavailable"):self.run_metadata()

    def test_parent_entrypoint_armored_metadata_success_with_no_db_environment_reads(self):
        argv=["--operation","prepare","--root",str(self.fixture.root),"--operation-id",OPERATION,
            "--approved-source-sha",SOURCE,"--actual-source-sha",SOURCE,"--run-id",RUN,"--prepare-mode",history.METADATA_MODE,
            "--bootstrap-approval-json",self.args.bootstrap_approval_json,"--bootstrap-approval-hash",self.args.bootstrap_approval_hash]
        output=io.StringIO()
        class NoDatabaseEnv(dict):
            def get(self,key,*args):
                if key.startswith(("MYSQL_","MONGODB_")):raise AssertionError("database env read")
                return super().get(key,*args)
        with contextlib.redirect_stdout(output),mock.patch.object(tool,"capture_fixed",side_effect=self.capture),mock.patch.object(tool.os,"environ",NoDatabaseEnv()),mock.patch.object(tool,"inventory_connection_values",side_effect=AssertionError("connection values")):
            self.assertEqual(tool.main(argv),0)
        decoded=json.loads(tool.transport().decode_armored_receipt(output.getvalue()))
        self.assertTrue(decoded["history_metadata_complete"]);self.assertFalse(decoded["complete"])
        self.assertEqual(decoded["parent_proposal_run_id"],"700-1")
        self.assertNotIn(str(self.directory),output.getvalue())

    def test_metadata_armor_failure_returns42_and_original_sources_unchanged(self):
        argv=["--operation","prepare","--root",str(self.fixture.root),"--operation-id",OPERATION,
            "--approved-source-sha",SOURCE,"--actual-source-sha",SOURCE,"--run-id",RUN,"--prepare-mode",history.METADATA_MODE,
            "--bootstrap-approval-json",self.args.bootstrap_approval_json,"--bootstrap-approval-hash",self.args.bootstrap_approval_hash]
        before=[p.read_bytes() for p in self.output.iterdir()]
        with mock.patch.object(tool,"capture_fixed",side_effect=self.capture),mock.patch.object(tool,"transport",return_value=argparse.Namespace(encode_armored_receipt=mock.Mock(side_effect=ValueError("safe failure")))),contextlib.redirect_stdout(io.StringIO()),contextlib.redirect_stderr(io.StringIO()):
            self.assertEqual(tool.main(argv),42)
        self.assertEqual(before,[p.read_bytes() for p in self.output.iterdir()])

    def test_source_reader_close_failure_returns42_without_private_completion(self):
        argv=["--operation","prepare","--root",str(self.fixture.root),"--operation-id",OPERATION,
            "--approved-source-sha",SOURCE,"--actual-source-sha",SOURCE,"--run-id",RUN,"--prepare-mode",history.METADATA_MODE,
            "--bootstrap-approval-json",self.args.bootstrap_approval_json,"--bootstrap-approval-hash",self.args.bootstrap_approval_hash]
        original_read=history._metadata_physical_file;original_close=os.close
        def read_with_close_failure(*args):
            def bad_close(fd):
                original_close(fd)
                raise OSError("synthetic close failure")
            with mock.patch.object(history.os,"close",side_effect=bad_close):return original_read(*args)
        output=io.StringIO()
        with mock.patch.object(tool,"capture_fixed",side_effect=self.capture),mock.patch.object(history,"_metadata_physical_file",side_effect=read_with_close_failure),contextlib.redirect_stdout(output):
            # Invoke this instance to inject the actual file-close failure, then
            # let the public entrypoint exercise its same fixed failure channel.
            with self.assertRaises(OSError):self.run_metadata()
        self.assertFalse((self.directory/("history-metadata-"+RUN)).exists())
        original_loader=tool.importlib.util.module_from_spec
        def loader(spec):
            loaded=original_loader(spec)
            if spec.name != "compatibility_history_prepare":return loaded
            # The CLI loads a separate module; apply the same failure inside
            # its own source reader, not by fabricating a complete DTO.
            original_exec=spec.loader.exec_module
            def execute(module):
                original_exec(module)
                read=module._metadata_physical_file
                def failing(*args):
                    def bad_close(fd):original_close(fd);raise OSError("synthetic close failure")
                    with mock.patch.object(module.os,"close",side_effect=bad_close):return read(*args)
                module._metadata_physical_file=failing
            spec.loader.exec_module=execute
            return loaded
        with mock.patch.object(tool,"capture_fixed",side_effect=self.capture),mock.patch.object(tool.importlib.util,"module_from_spec",side_effect=loader),contextlib.redirect_stdout(output):
            self.assertEqual(tool.main(argv),42)
        decoded=json.loads(tool.transport().decode_armored_receipt(output.getvalue()))
        self.assertNotIn("history_metadata_complete",decoded)
        self.assertFalse((self.directory/("history-metadata-"+RUN)).exists())

    def test_actual_node_metadata_strict_approval_matrix(self):
        workflow=(ROOT/".github/workflows/compatibility-retirement.yml").read_text()
        script=textwrap.dedent(workflow.split("          script: |\n",1)[1].split("      - name:",1)[0])
        cases=[("valid",lambda v:None,True),("wrong_originalrun",lambda v:v["inventory_report"].update(run_id=RUN),False),
            ("wrong_source",lambda v:v.update(source_sha="b"*40),False),("unapprovedhash",lambda v:v.update(inventory_request_sha256=""),False),
            ("unknown",lambda v:v.update(extra=True),False),("numericstring",lambda v:v["metadata_limits"].update(passes="2"),False),
            ("capacitychange",lambda v:v["metadata_limits"].update(max_encoded_file_bytes=4<<30),False),
            ("contextmismatch",lambda v:None,False),("badattempt",lambda v:None,False)]
        for name,change,allowed in cases:
            with self.subTest(name=name):
                value=copy.deepcopy(self.value);change(value);raw=tool.canonical_bytes(value)
                inputs={"operation":"prepare","database":"mysql-and-mongodb","approved_source_sha":SOURCE,"operation_id":OPERATION,
                    "prepare_mode":history.METADATA_MODE,"bootstrap_approval_json":raw[:-1].decode(),"bootstrap_approval_sha256":hashlib.sha256(raw).hexdigest()}
                context={"payload":{"inputs":inputs},"ref":"refs/heads/main","sha":SOURCE,"repo":{},"runId":901 if name=="contextmismatch" else 900}
                program="process.env.GITHUB_RUN_ID='900';process.env.GITHUB_RUN_ATTEMPT="+json.dumps("x" if name=="badattempt" else "1")+";const script="+json.dumps(script)+";const context="+json.dumps(context)+";const github={rest:{repos:{getCommit:async()=>({data:{sha:'"+SOURCE+"'}})}}};new (Object.getPrototypeOf(async function(){}).constructor)('context','github',script)(context,github).catch(()=>{process.exitCode=1;});"
                r=subprocess.run(["node","-e",program],capture_output=True,check=False)
                self.assertEqual(r.returncode,0 if allowed else 1)


if __name__ == "__main__":
    unittest.main()
