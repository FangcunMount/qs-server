#!/usr/bin/env python3
"""Filesystem and synthetic receipt contracts for immutable parent registration.

Database and Docker mutations are forbidden in every case. These tests prove
request registration, not production history qualification or retirement.
"""
import argparse
import copy
import hashlib
import importlib.util
import os
from pathlib import Path
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, ROOT / "scripts/database" / filename)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


fixtures = load("history_parent_synthetic_fixtures", "test-compatibility-history-prepare.py")
parent = load("history_parent_registration_contract", "compatibility-history-parent.py")
tool = fixtures.tool
history = fixtures.history


class ParentRegistration(unittest.TestCase):
    def setUp(self):
        self.fixture = fixtures.HistoryMetadata(methodName="test_two_complete_physical_passes_persisted_proposal_and_no_authority")
        self.fixture.setUp()
        self.result = self.fixture.run_metadata()
        self.directory = self.fixture.directory
        self.metadata_dir = self.directory / ("history-metadata-" + self.fixture.args.run_id)
        self.args = argparse.Namespace(**vars(self.fixture.args))
        self.args.run_id = "1000-1"
        self.args.prepare_mode = parent.MODE
        self.value = {"format_version": 1, "kind": "readonly_history_parent_registration_approval",
            "prepare_mode": parent.MODE, "source_sha": fixtures.SOURCE, "operation_id": fixtures.OPERATION,
            "target_hash": tool.TARGET_HASH, "database_scope": "mysql-and-mongodb",
            "metadata_report": {"run_id": self.fixture.args.run_id,
                                "sha256": self.result["metadata_private_report_sha256"]},
            "parent_proposal_sha256": self.result["history_parent_proposal_sha256"],
            "inventory_request_sha256": self.result["inventory_request_sha256"],
            "inventory_report": self.result["approved_inventory_report"],
            "assets": copy.deepcopy(self.result["history_metadata_assets"]),
            "metadata_limits": history.METADATA_LIMITS.copy()}
        self.approve()
        self.fixture.commands.clear()

    def tearDown(self):
        self.fixture.tearDown()

    def approve(self):
        raw = tool.canonical_bytes(self.value)
        self.args.bootstrap_approval_json = raw[:-1].decode("ascii")
        self.args.bootstrap_approval_hash = hashlib.sha256(raw).hexdigest()

    def run_parent(self):
        with mock.patch.object(parent, "history_api", return_value=history), \
             mock.patch.object(tool, "capture_fixed", side_effect=self.fixture.capture), \
             mock.patch.object(tool, "inventory_connection_values", side_effect=AssertionError("database access")), \
             mock.patch.object(history, "_binary", side_effect=AssertionError("binary execution")):
            return parent.prepare(self.args, tool)

    def assert_unpublished(self):
        self.assertFalse((self.directory / "history-request.json").exists())
        self.assertFalse((self.directory / "history-request-bootstrap.json").exists())

    def rewrite_metadata(self, change):
        value, _ = tool.read_private(self.metadata_dir, "history-metadata.json")
        change(value)
        self.value["metadata_report"]["sha256"] = self.fixture.fixture.store(self.metadata_dir, "history-metadata.json", value)
        self.approve()

    def test_actual_exclusive_publication_keeps_original_inventory_run_and_no_authority(self):
        originals = {p: p.read_bytes() for p in self.fixture.output.iterdir()}
        result = self.run_parent()
        self.assertIs(result["history_parent_registration_complete"], True)
        for key in ("complete", "execution_allowed", "drop_ready", "history_cas_complete", "history_parent_process_budget_proven"):
            self.assertIs(result[key], False)
        self.assertEqual(result["run_id"], "1000-1")
        self.assertEqual(result["parent_proposal_run_id"], "700-1")
        request, digest = tool.read_private(self.directory, "history-request.json")
        proposal, proposal_digest = tool.read_private(self.metadata_dir, "history-parent-proposal.json")
        self.assertEqual(request, proposal)
        self.assertEqual(digest, proposal_digest)
        self.assertEqual(digest, result["history_parent_request_sha256"])
        self.assertEqual(request["run_id"], "700-1")
        registered, registry_digest = tool.read_private(self.directory, "history-request-bootstrap.json")
        self.assertEqual(registered["created_run_id"], "1000-1")
        self.assertEqual(registered["metadata_report"], self.value["metadata_report"])
        self.assertEqual(registry_digest, result["history_parent_registration_sha256"])
        self.assertEqual(originals, {p: p.read_bytes() for p in self.fixture.output.iterdir()})
        self.assertFalse(any(p.name.endswith(".bootstrap.partial") for p in self.directory.iterdir()))
        self.assertEqual((self.directory / "history-request.json").stat().st_mode & 0o777, 0o600)
        self.assertEqual((self.directory / "history-request.json").stat().st_nlink, 1)
        self.assertEqual(sum(any(v.startswith("label=") for v in command) for command in self.fixture.commands), 2)

    def test_new_attempt_exact_idempotence_preserves_original_created_run(self):
        first = self.run_parent()
        before = {name: (self.directory / name).read_bytes() for name in ("history-request.json", "history-request-bootstrap.json")}
        self.args.run_id = "1001-2"
        second = self.run_parent()
        self.assertEqual(second["run_id"], "1001-2")
        self.assertEqual(second["request_created_run_id"], "1000-1")
        self.assertEqual(first["history_parent_request_sha256"], second["history_parent_request_sha256"])
        self.assertEqual(before, {name: (self.directory / name).read_bytes() for name in before})

    def test_descriptor_requires_independent_class_hash_source_run_and_exact_assets(self):
        changes = (lambda v: v.update(kind="readonly_history_metadata_approval"),
            lambda v: v.update(source_sha="b" * 40), lambda v: v.update(extra=True),
            lambda v: v["metadata_report"].update(run_id="1000-1"),
            lambda v: v["metadata_report"].update(run_id="700-1"),
            lambda v: v["metadata_limits"].update(total_seconds=True),
            lambda v: v["assets"].reverse(), lambda v: v["assets"][0].update(full_file_bytes="28"),
            lambda v: v["assets"][0].update(path="/private/source"))
        for change in changes:
            with self.subTest(change=change):
                value = copy.deepcopy(self.value)
                change(value)
                args = argparse.Namespace(**vars(self.args))
                raw = tool.canonical_bytes(value)
                args.bootstrap_approval_json = raw[:-1].decode("ascii")
                args.bootstrap_approval_hash = hashlib.sha256(raw).hexdigest()
                with self.assertRaises(tool.Blocked):
                    parent.approval(args, tool, history)
        self.args.bootstrap_approval_json += " "
        with self.assertRaisesRegex(tool.Blocked, "history_parent_approval_encoding_or_hash_invalid"):
            self.run_parent()
        self.assert_unpublished()

    def test_metadata_observation_flags_cannot_enable_mutations_or_semantic_proof(self):
        for key in ("semantic_source_coverage_verified", "production_process_budget_proven", "complete", "execution_allowed", "drop_ready", "cas_complete"):
            with self.subTest(key=key):
                previous = (self.metadata_dir / "history-metadata.json").read_bytes()
                self.rewrite_metadata(lambda v: v.update({key: True}))
                with self.assertRaisesRegex(tool.Blocked, "history_parent_metadata_binding_invalid"):
                    self.run_parent()
                (self.metadata_dir / "history-metadata.json").write_bytes(previous)
                self.value["metadata_report"]["sha256"] = hashlib.sha256(previous).hexdigest()
                self.approve()
        self.assert_unpublished()

    def test_metadata_false_complete_two_eofs_original_run_and_strict_types_required(self):
        changes = (lambda v: v.update(metadata_complete=False), lambda v: v.update(equal_full_physical_passes=1),
            lambda v: v.update(format_version=True), lambda v: v.update(run_id="1000-1"),
            lambda v: v.update(parent_proposal_run_id="900-1"),
            lambda v: v["assets"][0].update(full_file_bytes=True))
        original = (self.metadata_dir / "history-metadata.json").read_bytes()
        for change in changes:
            with self.subTest(change=change):
                (self.metadata_dir / "history-metadata.json").write_bytes(original)
                self.rewrite_metadata(change)
                with self.assertRaises(tool.Blocked):
                    self.run_parent()
        self.assert_unpublished()

    def test_metadata_registration_hash_and_original_approval_cannot_be_forged(self):
        registration, _ = tool.read_private(self.metadata_dir, "history-metadata-bootstrap.json")
        registration["approval"]["metadata_limits"]["passes"] = 1
        self.fixture.fixture.store(self.metadata_dir, "history-metadata-bootstrap.json", registration)
        with self.assertRaisesRegex(tool.Blocked, "history_metadata_approval_encoding_or_hash_invalid"):
            self.run_parent()
        self.assert_unpublished()

    def test_proposal_path_or_typed_schema_cannot_be_substituted_even_when_hash_approved(self):
        proposal, _ = tool.read_private(self.metadata_dir, "history-parent-proposal.json")
        proposal["assets"][0]["path"] = str(self.fixture.output / "foreign-source")
        digest = self.fixture.fixture.store(self.metadata_dir, "history-parent-proposal.json", proposal)
        self.value["parent_proposal_sha256"] = digest
        self.rewrite_metadata(lambda v: v.update(parent_proposal_sha256=digest))
        registration, _ = tool.read_private(self.metadata_dir, "history-metadata-bootstrap.json")
        registration["parent_proposal_sha256"] = digest
        self.fixture.fixture.store(self.metadata_dir, "history-metadata-bootstrap.json", registration)
        with self.assertRaisesRegex(tool.Blocked, "history_parent_proposal_binding_invalid"):
            self.run_parent()
        self.assert_unpublished()

    def test_same_size_source_mutation_and_same_bytes_inode_replacement_both_block(self):
        path = self.fixture.output / tool.SOURCE_FILENAMES[tool.TARGETS[0][:2]]
        original = path.read_bytes()
        path.write_bytes(b"X" * len(original))
        with self.assertRaisesRegex(tool.Blocked, "history_parent_physical_source_changed"):
            self.run_parent()
        path.unlink()
        path.write_bytes(original)
        path.chmod(0o600)
        with self.assertRaisesRegex(tool.Blocked, "history_parent_input_baseline_changed"):
            self.run_parent()
        self.assert_unpublished()

    def test_source_private_permissions_and_single_link_enforced(self):
        path = self.fixture.output / tool.SOURCE_FILENAMES[tool.TARGETS[0][:2]]
        path.chmod(0o644)
        with self.assertRaises(tool.Blocked):
            self.run_parent()
        path.chmod(0o600)
        os.link(path, self.fixture.output / "alias")
        with self.assertRaises(tool.Blocked):
            self.run_parent()
        self.assert_unpublished()

    def test_interrupted_two_file_publication_blocks_retry_without_overwrite(self):
        original = tool.create_bootstrap_file
        def interrupt(directory, filename, raw):
            if filename == "history-request.json":
                raise tool.Blocked("bootstrap_request_creation_incomplete")
            return original(directory, filename, raw)
        with mock.patch.object(tool, "create_bootstrap_file", side_effect=interrupt):
            with self.assertRaises(tool.Blocked):
                self.run_parent()
        record = (self.directory / "history-request-bootstrap.json").read_bytes()
        with self.assertRaisesRegex(tool.Blocked, "history_parent_registration_incomplete"):
            self.run_parent()
        self.assertEqual((self.directory / "history-request-bootstrap.json").read_bytes(), record)
        self.assertFalse((self.directory / "history-request.json").exists())

    def test_existing_unregistered_request_and_partial_never_adopted(self):
        path = self.directory / "history-request.json"
        path.write_bytes((self.metadata_dir / "history-parent-proposal.json").read_bytes())
        path.chmod(0o600)
        with self.assertRaisesRegex(tool.Blocked, "history_parent_registration_incomplete"):
            self.run_parent()
        path.unlink()
        partial = self.directory / "history-request.json.bootstrap.partial"
        partial.write_bytes(b"interrupted")
        partial.chmod(0o600)
        with self.assertRaisesRegex(tool.Blocked, "history_parent_registration_incomplete"):
            self.run_parent()
        self.assertEqual(partial.read_bytes(), b"interrupted")

    def test_other_complete_registration_approval_is_not_overwritten(self):
        self.run_parent()
        registry, _ = tool.read_private(self.directory, "history-request-bootstrap.json")
        registry["approval_sha256"] = "e" * 64
        self.fixture.fixture.store(self.directory, "history-request-bootstrap.json", registry)
        before = (self.directory / "history-request-bootstrap.json").read_bytes()
        with self.assertRaisesRegex(tool.Blocked, "history_parent_registration_binding_invalid"):
            self.run_parent()
        self.assertEqual(before, (self.directory / "history-request-bootstrap.json").read_bytes())

    def test_inventory_unknown_handle_and_unavailable_observation_block_before_publication(self):
        for failure in ("whole_existing", "name_hidden", "unavailable"):
            with self.subTest(failure=failure):
                self.fixture.failure = failure
                with self.assertRaises(tool.Blocked):
                    self.run_parent()
                self.assert_unpublished()
        self.assertFalse(any(command[3] in ("create", "run", "start", "stop", "rm") for command in self.fixture.commands))

    def test_late_source_change_after_publication_never_returns_registration_success(self):
        original = parent._publish
        path = self.fixture.output / tool.SOURCE_FILENAMES[tool.TARGETS[0][:2]]
        def changed(*args):
            result = original(*args)
            path.write_bytes(b"X" * path.stat().st_size)
            return result
        with mock.patch.object(parent, "_publish", side_effect=changed):
            with self.assertRaisesRegex(tool.Blocked, "history_metadata_file_changed"):
                self.run_parent()
        self.assertTrue((self.directory / "history-request.json").exists())
        with self.assertRaisesRegex(tool.Blocked, "history_parent_physical_source_changed"):
            self.run_parent()

    def test_no_database_environment_read_and_no_binary_execution(self):
        class NoDatabaseEnvironment(dict):
            def get(self, key, *args):
                if key.startswith(("MYSQL_", "MONGODB_")):
                    raise AssertionError("database environment read")
                return super().get(key, *args)
        with mock.patch.object(tool.os, "environ", NoDatabaseEnvironment()):
            self.assertTrue(self.run_parent()["history_parent_registration_complete"])

    def test_registration_observes_all_four_real_files_to_eof_twice(self):
        calls = []
        original = history._metadata_physical_file
        def observed(t, path, maximum, deadline):
            result = original(t, path, maximum, deadline)
            calls.append((path.name, result[0], result[1]))
            return result
        with mock.patch.object(history, "_metadata_physical_file", side_effect=observed):
            self.run_parent()
        self.assertEqual(len(calls), 8)
        self.assertEqual(calls[:4], calls[4:])
        self.assertEqual([item[0] for item in calls[:4]],
                         [tool.SOURCE_FILENAMES[target[:2]] for target in tool.TARGETS])
        self.assertEqual([item[1] for item in calls[:4]], [asset["full_file_sha256"] for asset in self.value["assets"]])
        self.assertEqual([item[2] for item in calls[:4]], [asset["full_file_bytes"] for asset in self.value["assets"]])

    def test_real_partial_link_failure_is_not_automatically_resumed(self):
        with mock.patch.object(tool.os, "link", side_effect=OSError("synthetic interruption")):
            with self.assertRaisesRegex(tool.Blocked, "bootstrap_request_creation_incomplete"):
                self.run_parent()
        partial = self.directory / "history-request-bootstrap.json.bootstrap.partial"
        self.assertTrue(partial.is_file())
        original = partial.read_bytes()
        with self.assertRaisesRegex(tool.Blocked, "history_parent_registration_incomplete"):
            self.run_parent()
        self.assertEqual(partial.read_bytes(), original)
        self.assert_unpublished()

    def test_direct_parent_call_rejects_invalid_current_source_operation_and_run(self):
        for key, invalid in (("actual_source_sha", "invalid-source"),
                             ("operation_id", "../operation"), ("run_id", "invalid-run")):
            with self.subTest(key=key):
                original = getattr(self.args, key)
                setattr(self.args, key, invalid)
                with self.assertRaises(tool.Blocked):
                    self.run_parent()
                setattr(self.args, key, original)
        self.assert_unpublished()
        self.assertEqual(self.fixture.commands, [])

    def test_concurrent_operation_lock_refuses_second_caller_before_source_scan(self):
        with tool.locked_operation(self.directory):
            with mock.patch.object(history, "_metadata_physical_file", side_effect=AssertionError("scanned despite held lock")):
                with self.assertRaisesRegex(tool.Blocked, "operation_busy"):
                    self.run_parent()
        self.assertEqual(self.fixture.commands, [])
        self.assert_unpublished()


if __name__ == "__main__":
    unittest.main()
