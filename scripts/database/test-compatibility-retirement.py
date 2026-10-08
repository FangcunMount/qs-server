#!/usr/bin/env python3
"""Safety contracts: all fixtures are local synthetic test inputs, never evidence."""
import argparse
import contextlib
import copy
import datetime
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

SCRIPT = Path(__file__).with_name("compatibility-retirement.py")
SPEC = importlib.util.spec_from_file_location("compatibility_retirement", SCRIPT)
tool = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(tool)
NOW = datetime.datetime(2026, 10, 8, 12, 0, tzinfo=datetime.timezone.utc)
SOURCE = "a" * 40
OPERATION = "123-1"


def manifest():
    bindings = {database: {"identity_hash": digit * 64, "migration_version": version,
                           "migration_dirty": False, "catalog_hash": "d" * 64,
                           "non_target_schema_hash": "e" * 64}
                for database, digit, version in (("mysql", "1", 95), ("mongodb", "2", 36))}
    targets = [{"database": database, "name": name, "kind": kind, "identity_hash": "3" * 64,
                "schema_hash": "4" * 64, "data_hash": "5" * 64, "records": 0}
               for database, name, kind in tool.TARGETS]
    return {"format_version": 1, "operation_id": OPERATION, "source_sha": SOURCE,
            "target_hash": tool.TARGET_HASH, "database_bindings": bindings, "targets": targets,
            "evidence": {}, "maintenance": {"max_seconds": 1800, "forward_stop_seconds": 1200,
                                             "rollback_seconds": 600}}


def proof(kind, value):
    identities = {database: binding["identity_hash"] for database, binding in value["database_bindings"].items()}
    summaries = {
        "inventory": {"count_semantics": "exact", "target_count": 4, "identity_hashes": identities,
                      "snapshots_hash": hashlib.sha256(json.dumps(value["targets"], sort_keys=True, separators=(",", ":")).encode()).hexdigest(),
                      "dependencies_complete": True, "outside_dependencies": 0},
        "history": {"classified": 3, "verified_live": 1, "verified_retired": 1, "unverifiable_closed": 1,
                    "unresolved": 0, "ambiguous": 0, "hash_conflicts": 0, "unknown_execution": 0,
                    "unexplained_high": 0, "retirement_references": 3, "references_hash": "6" * 64},
        "fence": {"approved_main_sha": SOURCE, "original_workflows_disabled": True,
                  "old_runs_terminal": True, "pending_environments_zero": True,
                  "historical_rerun_denied_before_credentials": True, "snapshot_hash": "7" * 64},
        "backup_restore": {"archive_hash": "8" * 64, "restore_proof_hash": "9" * 64,
                           "restored_target_count": 4, "source_identity_hashes": identities,
                           "restore_identity_hashes": {"mysql": "b" * 64, "mongodb": "c" * 64},
                           "network_none": True, "no_published_ports": True, "complete_content_equal": True,
                           "complete_schema_equal": True, "assets_registered": True},
        "release": {"application_a_sha": "b" * 40, "release_b_sha": SOURCE, "safe_rollback_sha": "c" * 40,
                    "prepared_images_hash": "d" * 64, "a_no_legacy_dependencies": True,
                    "four_presence_states_tested": True, "rollback_dirty_head_tested": True,
                    "rollback_seconds": 400, "b_runtime_matches_a": True},
        "acceptance": {"release_b_sha": SOURCE, "four_targets_absent": True, "heads_clean": True,
                       "non_target_schema_equal": True, "retirement_references_complete": True,
                       "standard_live_checks_passed": True, "missing_namespace_errors": 0,
                       "unexplained_high": 0, "operation_ledger_hash": "e" * 64},
    }
    return {"format_version": 1, "kind": kind, "operation_id": OPERATION, "source_sha": SOURCE,
            "target_hash": tool.TARGET_HASH, "producer": {"protocol": "qs_compatibility_retirement_" + kind + "_v1",
            "source_sha": SOURCE, "run_id": "123-1"}, "complete": True,
            "observed_at": "2026-10-08T11:59:00Z", "valid_until": "2026-10-08T13:00:00Z",
            "summary": summaries[kind]}


class SafetyContracts(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.base = Path(self.temporary.name).resolve()
        self.root = self.base / "backups" / "qs-server" / "compatibility-retirement"
        self.root.mkdir(parents=True, mode=0o700)
        self.directory = self.root / OPERATION
        self.directory.mkdir(mode=0o700)
        self.value = manifest()

    def tearDown(self):
        self.temporary.cleanup()

    def write(self, name, value):
        raw = json.dumps(value, separators=(",", ":"), allow_nan=False).encode()
        path = self.directory / name
        path.write_bytes(raw)
        path.chmod(0o600)
        return hashlib.sha256(raw).hexdigest()

    def assertBlocked(self, category, function, *args):
        with self.assertRaisesRegex(tool.Blocked, "^" + category + "$"):
            function(*args)

    def arguments(self, operation="prepare"):
        return argparse.Namespace(operation=operation, root=str(self.root), operation_id=OPERATION,
                                  approved_source_sha=SOURCE, actual_source_sha=SOURCE, run_id="456-1",
                                  manifest_hash=self.write("manifest.json", self.value))

    def test_fixed_four_targets_exclude_cbpt_and_other_legacy(self):
        self.assertEqual(len(tool.TARGETS), 4)
        self.assertEqual([target[0] for target in tool.TARGETS], ["mysql"] * 3 + ["mongodb"])
        for replacement in ("assessment__cbpt_backup_v1", "operator_recovery_archives", "rm_outbox"):
            changed = copy.deepcopy(self.value)
            changed["targets"][0]["name"] = replacement
            self.assertBlocked("target_allowlist_mismatch", tool.validate_manifest, changed, OPERATION, SOURCE)

    def test_manifest_binds_source_operation_and_exact_order(self):
        tool.validate_manifest(self.value, OPERATION, SOURCE)
        for key, replacement in (("source_sha", "b" * 40), ("operation_id", "124-1"), ("target_hash", "f" * 64)):
            changed = copy.deepcopy(self.value); changed[key] = replacement
            with self.assertRaises(tool.Blocked):
                tool.validate_manifest(changed, OPERATION, SOURCE)
        self.value["targets"].reverse()
        self.assertBlocked("target_allowlist_mismatch", tool.validate_manifest, self.value, OPERATION, SOURCE)

    def test_strict_counts_dirty_heads_unknown_fields_and_identity(self):
        for mutate in (
            lambda value: value.update(password="TEST_PRIVATE_DO_NOT_PRINT"),
            lambda value: value["targets"][0].update(records=True),
            lambda value: value["targets"][0].update(records=-1),
            lambda value: value["database_bindings"]["mysql"].update(migration_dirty=True),
            lambda value: value["database_bindings"]["mysql"].update(migration_version=False),
            lambda value: value["database_bindings"]["mongodb"].update(identity_hash="1" * 64),
            lambda value: value["targets"][3].update(kind="view"),
            lambda value: value["targets"].append(copy.deepcopy(value["targets"][0])),
        ):
            changed = copy.deepcopy(self.value); mutate(changed)
            with self.assertRaises(tool.Blocked):
                tool.validate_manifest(changed, OPERATION, SOURCE)

    def test_maintenance_budget_cannot_extend_or_remove_recovery_reserve(self):
        for budget in ({"max_seconds": 3600, "forward_stop_seconds": 1200, "rollback_seconds": 2400},
                       {"max_seconds": 1800, "forward_stop_seconds": 1800, "rollback_seconds": 0}):
            self.value["maintenance"] = budget
            self.assertBlocked("maintenance_budget_invalid", tool.validate_manifest, self.value, OPERATION, SOURCE)

    def test_json_duplicate_keys_nonfinite_and_utf8_are_rejected(self):
        for raw in (b'{"complete":true,"complete":false}', b'{"count":NaN}', b'\xff', b'not json'):
            with self.assertRaises(tool.Blocked):
                tool.decode(raw)

    def test_private_files_reject_symlinks_hardlinks_modes_and_changed_hash(self):
        digest = self.write("manifest.json", self.value)
        self.assertEqual(tool.read_private(self.directory, "manifest.json", digest)[1], digest)
        self.assertBlocked("evidence_hash_mismatch", tool.read_private, self.directory, "manifest.json", "0" * 64)
        path = self.directory / "manifest.json"
        path.chmod(0o644)
        self.assertBlocked("evidence_not_private", tool.read_private, self.directory, "manifest.json")
        path.chmod(0o600)
        os.link(path, self.directory / "duplicate.json")
        self.assertBlocked("evidence_not_private", tool.read_private, self.directory, "manifest.json")
        (self.directory / "duplicate.json").unlink()
        (self.directory / "link.json").symlink_to(path)
        self.assertBlocked("evidence_unavailable", tool.read_private, self.directory, "link.json")

    def test_directory_and_root_never_traverse_cbpt_or_symlinks(self):
        self.assertEqual(tool.operation_directory(self.root, OPERATION), self.directory)
        self.assertBlocked("operation_root_invalid", tool.operation_directory, self.base / "mysql-cbpt-archive", OPERATION)
        self.assertBlocked("evidence_type_invalid", tool.operation_directory, self.root, "../123-1")
        self.directory.chmod(0o755)
        self.assertBlocked("operation_directory_not_private", tool.operation_directory, self.root, OPERATION)
        self.directory.chmod(0o700)
        alias = self.base / "alias"; alias.symlink_to(self.root, target_is_directory=True)
        self.assertBlocked("operation_path_invalid", tool.private_directory, alias / OPERATION)

    def test_evidence_references_are_unique_private_basenames(self):
        for refs in ({"inventory": {"filename": "../proof.json", "sha256": "f" * 64}},
                     {"inventory": {"filename": "manifest.json", "sha256": "f" * 64}},
                     {"inventory": {"filename": "proof.json", "sha256": "f" * 64},
                      "history": {"filename": "proof.json", "sha256": "e" * 64}},
                     {"unknown": {"filename": "proof.json", "sha256": "f" * 64}}):
            self.value["evidence"] = refs
            with self.assertRaises(tool.Blocked):
                tool.validate_manifest(self.value, OPERATION, SOURCE)

    def test_writable_ancestor_cannot_replace_the_private_operation_root(self):
        parent = self.root.parent
        parent.chmod(0o777)
        self.assertBlocked("operation_ancestor_writable", tool.operation_directory, self.root, OPERATION)
        parent.chmod(0o755)

    def test_proof_binds_producer_source_target_and_time(self):
        for kind in tool.PROOF_KINDS:
            value = proof(kind, self.value)
            tool.validate_proof(value, kind, self.value, NOW)
            for mutate in (lambda item: item.update(complete=1),
                           lambda item: item.update(valid_until="2026-10-08T11:00:00Z"),
                           lambda item: item.update(valid_until="2026-10-10T11:00:00Z"),
                           lambda item: item["producer"].update(source_sha="f" * 40),
                           lambda item: item.update(target_hash="f" * 64),
                           lambda item: item.update(raw_payload="TEST_PRIVATE_DO_NOT_PRINT")):
                changed = copy.deepcopy(value); mutate(changed)
                with self.subTest(kind=kind), self.assertRaises(tool.Blocked):
                    tool.validate_proof(changed, kind, self.value, NOW)

    def test_inventory_estimates_incomplete_dependencies_or_drift_block(self):
        for key, replacement in (("count_semantics", "metadata_estimate"), ("dependencies_complete", False),
                                 ("outside_dependencies", 1), ("snapshots_hash", "a" * 64), ("target_count", True)):
            value = proof("inventory", self.value); value["summary"][key] = replacement
            with self.assertRaises(tool.Blocked):
                tool.validate_proof(value, "inventory", self.value, NOW)

    def test_unverifiable_closed_is_separate_but_unsettled_or_ambiguous_blocks(self):
        value = proof("history", self.value)
        self.assertEqual(value["summary"]["unverifiable_closed"], 1)
        tool.validate_proof(value, "history", self.value, NOW)
        for key in ("unresolved", "ambiguous", "hash_conflicts", "unknown_execution", "unexplained_high"):
            changed = copy.deepcopy(value); changed["summary"][key] = 1
            self.assertBlocked("history_blocked", tool.validate_proof, changed, "history", self.value, NOW)
        for key in ("classified", "retirement_references"):
            changed = copy.deepcopy(value); changed["summary"][key] += 1
            self.assertBlocked("history_coverage_incomplete", tool.validate_proof, changed, "history", self.value, NOW)

    def test_disabled_workflows_and_main_alone_do_not_prove_fence(self):
        value = proof("fence", self.value)
        value["summary"]["historical_rerun_denied_before_credentials"] = False
        self.assertBlocked("production_fence_unproven", tool.validate_proof, value, "fence", self.value, NOW)

    def test_dry_run_partial_restore_same_database_network_or_ports_blocks(self):
        for key, replacement in (("restored_target_count", 3), ("network_none", False),
                                 ("no_published_ports", False), ("complete_content_equal", False),
                                 ("complete_schema_equal", False), ("assets_registered", False)):
            value = proof("backup_restore", self.value); value["summary"][key] = replacement
            with self.assertRaises(tool.Blocked):
                tool.validate_proof(value, "backup_restore", self.value, NOW)
        value = proof("backup_restore", self.value)
        value["summary"]["restore_identity_hashes"]["mysql"] = "1" * 64
        self.assertBlocked("restore_not_isolated", tool.validate_proof, value, "backup_restore", self.value, NOW)

    def test_rollback_over_ten_minutes_or_dirty_head_untested_blocks(self):
        for key, replacement in (("rollback_seconds", 601), ("rollback_dirty_head_tested", False),
                                 ("b_runtime_matches_a", False), ("four_presence_states_tested", False)):
            value = proof("release", self.value); value["summary"][key] = replacement
            self.assertBlocked("release_preparation_incomplete", tool.validate_proof, value, "release", self.value, NOW)

    def test_acceptance_requires_real_b_absence_schema_and_business_checks(self):
        for key, replacement in (("release_b_sha", "f" * 40), ("four_targets_absent", False),
                                 ("heads_clean", False), ("standard_live_checks_passed", False),
                                 ("retirement_references_complete", False), ("non_target_schema_equal", False),
                                 ("missing_namespace_errors", 1), ("unexplained_high", 1)):
            value = proof("acceptance", self.value); value["summary"][key] = replacement
            with self.assertRaises(tool.Blocked):
                tool.validate_proof(value, "acceptance", self.value, NOW)

    def test_imported_all_true_claims_never_enable_unimplemented_adapters(self):
        for kind in tool.PROOF_KINDS:
            filename = kind + "-proof.json"
            self.value["evidence"][kind] = {"filename": filename,
                                           "sha256": self.write(filename, proof(kind, self.value))}
        blockers = tool.preparation(self.directory, self.value, NOW)
        self.assertIn("historical_rerun_fence_verifier_not_implemented", blockers)
        self.assertIn("history_verifier_not_implemented", blockers)
        self.assertIn("private_backup_restore_backend_not_implemented", blockers)
        self.assertIn("production_database_backend_not_implemented", blockers)

    def test_all_cli_stages_fail_closed_without_filesystem_or_db_side_effects(self):
        for operation in tool.OPERATIONS:
            args = self.arguments(operation)
            before = sorted(path.name for path in self.directory.iterdir())
            value = tool.execute(args)
            self.assertIs(value["complete"], False)
            self.assertIs(value["execution_allowed"], False)
            self.assertEqual(before, sorted(path.name for path in self.directory.iterdir()))
            if operation != "prepare":
                self.assertIn(operation + "_stage_not_implemented", value["blockers"])

    def test_cli_unknown_inputs_emit_only_safe_armored_failure(self):
        for argv in (["--password", "TEST_PRIVATE_DO_NOT_PRINT"],
                     ["--operation", "arbitrary"]):
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                status = tool.main(argv)
            self.assertEqual(status, 42)
            self.assertNotIn("TEST_PRIVATE_DO_NOT_PRINT", output.getvalue())
            decoded = json.loads(tool.transport().decode_armored_receipt(output.getvalue()))
            self.assertIs(decoded["complete"], False)
            self.assertIs(decoded["execution_allowed"], False)

    def test_journal_persists_intent_then_unknown_and_rejects_blind_retry(self):
        journal = tool.DDLJournal(self.directory, "f" * 64, OPERATION, SOURCE)
        with tool.locked_operation(self.directory):
            journal.begin_drop(0)
            reloaded = tool.DDLJournal(self.directory, "f" * 64, OPERATION, SOURCE)
            self.assertEqual(reloaded.value["states"][0], "intent")
            journal.mark_unknown(0)
            self.assertBlocked("journal_transition_rejected", journal.begin_drop, 0)
            self.assertBlocked("ddl_ledger_incomplete", journal.require_all_dropped)
            journal.observe_absent(0)
            for index in range(1, 4):
                journal.begin_drop(index); journal.observe_absent(index)
            journal.require_all_dropped()
            journal.observe_restored(0)
            self.assertBlocked("ddl_ledger_incomplete", journal.require_all_dropped)
        self.assertEqual((self.directory / "ddl-journal.json").stat().st_mode & 0o777, 0o600)

    def test_journal_binding_bad_states_and_partial_file_fail_closed(self):
        journal = tool.DDLJournal(self.directory, "f" * 64, OPERATION, SOURCE)
        journal.begin_drop(0)
        self.assertBlocked("journal_binding_mismatch", tool.DDLJournal, self.directory, "e" * 64, OPERATION, SOURCE)
        value = copy.deepcopy(journal.value); value["states"][0] = {}
        self.write("ddl-journal.json", value)
        self.assertBlocked("journal_state_invalid", tool.DDLJournal, self.directory, "f" * 64, OPERATION, SOURCE)
        self.write("ddl-journal.json", journal.value)
        partial = self.directory / "ddl-journal.json.partial"; partial.write_text("existing-owned-intent")
        self.assertBlocked("journal_persistence_failed", journal.begin_drop, 1)
        self.assertEqual(partial.read_text(), "existing-owned-intent")
        self.assertEqual(journal.value["states"][1], "pending")

    def test_operation_lock_rejects_second_owner_and_symlink(self):
        with tool.locked_operation(self.directory):
            with self.assertRaisesRegex(tool.Blocked, "^operation_busy$"):
                with tool.locked_operation(self.directory):
                    self.fail("second lock accepted")
        path = self.directory / "operation.lock"; path.unlink()
        path.symlink_to(self.directory / "manifest.json")
        with self.assertRaisesRegex(tool.Blocked, "^operation_lock_unavailable$"):
            with tool.locked_operation(self.directory):
                self.fail("symlink accepted")

    def test_deadline_stops_forward_at_twenty_recovery_at_thirty(self):
        tool.deadline(0, clock=lambda: 1199)
        self.assertBlocked("maintenance_deadline_exceeded", lambda: tool.deadline(0, clock=lambda: 1200))
        tool.deadline(0, clock=lambda: 1799, recovering=True)
        self.assertBlocked("maintenance_deadline_exceeded", lambda: tool.deadline(0, clock=lambda: 1800, recovering=True))
        self.assertBlocked("maintenance_clock_invalid", lambda: tool.deadline(10, clock=lambda: 9))

    def test_inventory_request_is_separate_class_and_exact_bounded_scope(self):
        value = {"format_version": 1, "kind": "readonly_inventory_request", "operation_id": OPERATION,
                 "source_sha": SOURCE, "target_hash": tool.TARGET_HASH, "database_scope": "mysql-and-mongodb",
                 "identity_hashes": {"mysql": "1" * 64, "mongodb": "2" * 64},
                 "expected_migrations": {"mysql": 95, "mongodb": 36},
                 "limits": {"query_seconds": 15, "total_seconds": 180, "max_records": 100000, "max_bytes": 134217728}}
        tool.validate_inventory_request(value, OPERATION, SOURCE)
        for mutate in (lambda v: v.update(kind="retirement_manifest"),
                       lambda v: v.update(database_scope="all"),
                       lambda v: v["limits"].update(total_seconds=3600),
                       lambda v: v["expected_migrations"].update(mysql=True),
                       lambda v: v.update(source_sha="b" * 40),
                       lambda v: v.update(extra="TEST_PRIVATE_DO_NOT_PRINT")):
            changed = copy.deepcopy(value); mutate(changed)
            with self.assertRaises(tool.Blocked):
                tool.validate_inventory_request(changed, OPERATION, SOURCE)

    def test_prepare_request_and_mutation_manifest_classes_cannot_mix(self):
        for operation in ("prepare", "apply", "verify", "recover", "purge"):
            args = self.arguments(operation)
            args.inventory_request_hash = "a" * 64
            self.assertBlocked("input_classes_mixed", tool.execute, args)

    def test_identity_bootstrap_hash_precedes_creation_and_never_overwrites(self):
        args = self.arguments()
        args.prepare_mode = "identity"; args.inventory_request_hash = ""; args.manifest_hash = ""
        raw = tool.identity_request_bytes(OPERATION, SOURCE)
        args.identity_request_hash = hashlib.sha256(raw).hexdigest()
        tool.validate_identity_request(json.loads(raw), OPERATION, SOURCE)
        directory = tool.bootstrap_identity_request(args)
        self.assertEqual((directory / "identity-request.json").read_bytes(), raw)
        self.assertEqual((directory / "identity-request.json").stat().st_mode & 0o777, 0o600)
        tool.bootstrap_identity_request(args)
        before = (directory / "identity-request.json").stat().st_ino
        args.identity_request_hash = "f" * 64
        self.assertBlocked("identity_request_hash_mismatch", tool.bootstrap_identity_request, args)
        self.assertEqual((directory / "identity-request.json").stat().st_ino, before)
        args.root = str(self.base / "missing" / "backups" / "qs-server" / "compatibility-retirement")
        self.assertBlocked("identity_request_hash_mismatch", tool.bootstrap_identity_request, args)
        self.assertFalse(Path(args.root).exists())
        args.identity_request_hash = hashlib.sha256(raw).hexdigest()
        created = tool.bootstrap_identity_request(args)
        self.assertEqual((created / "identity-request.json").read_bytes(), raw)
        self.assertEqual(created.stat().st_mode & 0o777, 0o700)
        changed = json.loads(raw); changed["identity_hashes"] = {"mysql": "1" * 64}
        self.assertBlocked("evidence_fields_invalid", tool.validate_identity_request, changed, OPERATION, SOURCE)
        changed = json.loads(raw); changed["limits"]["total_seconds"] = 30
        self.assertBlocked("identity_request_protocol_or_limits_invalid", tool.validate_identity_request, changed, OPERATION, SOURCE)

    def test_identity_mode_cannot_mix_request_classes_or_enable_mutation(self):
        for operation in ("prepare", "apply", "verify", "recover", "purge"):
            args = self.arguments(operation)
            args.prepare_mode = "identity"; args.identity_request_hash = "1" * 64
            self.assertBlocked("input_classes_mixed", tool.execute, args)

    def test_identity_histogram_receipt_transport_is_bounded_and_payload_free(self):
        args = self.arguments()
        state = {"identity_hash": "1" * 64, "identity_observed": True, "migration_version": 95,
                 "migration_head_observed": True, "migration_dirty": False, "migration_clean": True,
                 "metadata_permissions_sufficient": True, "permission_scope": "identity_and_migration_head", "error_category": "none"}
        histogram = [{"database": database, "name": name, "present": True, "complete": True,
                      "error_category": "none", "diagnostic_only": True,
                      "buckets": [{"type_label": "footprint.entry_opened", "type_hash": "2" * 64,
                                   "state_label": "published", "state_hash": "3" * 64, "records": 1}
                                  for _ in range(32)]} for database, name, _ in tool.TARGETS]
        summary = {"format_version": 1, "kind": "readonly_identity_discovery", "source_sha": SOURCE,
                   "operation_id": OPERATION, "run_id": "456-1", "request_hash": "a" * 64,
                   "target_hash": tool.TARGET_HASH, "diagnostic_only": True, "drop_ready": False, "complete": True,
                   "identity_protocols": {"mysql": "mysql_database_identity_v1", "mongodb": "mongodb_database_identity_v1"},
                   "database_states": {"mysql": state, "mongodb": dict(state, identity_hash="4" * 64, migration_version=36)},
                   "diagnostic_histograms": histogram, "error_category": "none"}
        def validate(value):
            raw = json.dumps(value, separators=(",", ":")).encode()
            (self.directory / "identity.private.json").write_bytes(raw)
            (self.directory / "identity.private.json").chmod(0o600)
            value = dict(value, private_report_hash=hashlib.sha256(raw).hexdigest())
            return tool.validate_identity_receipt(value, 0, args, self.directory, "a" * 64, "b" * 64)
        receipt = validate(summary)
        self.assertEqual(len(receipt["identity_histogram_bucket_pages"]), 4)
        output = io.StringIO()
        with mock.patch.object(tool, "execute", return_value=receipt), contextlib.redirect_stdout(output):
            self.assertEqual(tool.main(["--operation", "prepare", "--operation-id", OPERATION,
                                       "--approved-source-sha", SOURCE, "--actual-source-sha", SOURCE, "--run-id", "456-1"]), 42)
        decoded = json.loads(tool.transport().decode_armored_receipt(output.getvalue()))
        self.assertIs(decoded["execution_allowed"], False)
        self.assertIs(decoded["drop_ready"], False)
        self.assertEqual(decoded["identity_histogram_bucket_pages"]["page_a"][0]["type_label"], "footprint_entry_opened")
        changed = copy.deepcopy(summary); changed["diagnostic_histograms"][0]["present"] = 1
        self.assertBlocked("identity_histogram_target_invalid", validate, changed)
        changed = copy.deepcopy(summary); changed["diagnostic_histograms"][0]["buckets"].append(histogram[0]["buckets"][0])
        bounded = validate(changed)
        self.assertFalse(bounded["identity_histogram_bucket_pages"])
        self.assertTrue(all(h["error_category"] == "histogram_public_bound_exceeded" and not h["complete"] for h in bounded["identity_diagnostic_histograms"]))

    def test_entrypoint_catalog_is_source_only_and_includes_unprotected_ssh(self):
        repository = SCRIPT.parents[2]
        value = json.loads((repository / "scripts/database/compatibility-retirement-entrypoints.json").read_text())
        self.assertIs(value["live_fence_proven"], False)
        self.assertIs(value["historical_rerun_proven_denied"], False)
        entries = value["entrypoints"]
        self.assertEqual(len(entries), 12)
        self.assertEqual(len({entry["workflow"] for entry in entries}), 12)
        self.assertEqual(len(value["historical_workflows_reported_active_not_in_current_source"]), 9)
        self.assertTrue(all(entry["credential_path_proven_denied"] is False for entry in value["historical_workflows_reported_active_not_in_current_source"]))
        self.assertIs(value["remote_metadata_report"]["observation_is_fence_proof"], False)
        for entry in entries:
            for path in [entry["workflow"], *entry["source"]]:
                self.assertTrue((repository / path).is_file(), path)
        by_name = {entry["name"]: entry for entry in entries}
        self.assertEqual(by_name["Provision Production AuthZ Matrix Subjects"]["classification"], "production_writer")
        for name in ("M5 AuthZ Outage Read-only Preflight", "M5 AuthZ Ephemeral Read-only Postcheck", "Ping Runner"):
            self.assertIs(by_name[name]["production_environment"], False)

    def test_workflow_exact_stage_lock_and_credential_boundary(self):
        repository = SCRIPT.parents[2]
        workflow = (repository / ".github/workflows/compatibility-retirement.yml").read_text()
        self.assertIn("group: production-deploy", workflow)
        self.assertIn("cancel-in-progress: false", workflow)
        self.assertIn("options: [prepare, apply, verify, recover, purge]", workflow)
        self.assertIn("options: [mysql-and-mongodb]", workflow)
        self.assertIn("context.ref !== 'refs/heads/main'", workflow)
        self.assertIn("current.data.sha !== input.approved_source_sha", workflow)
        validation, production = workflow.split("  controlled-stage:")
        self.assertNotIn("environment: production", validation)
        self.assertIn("needs: validate", production)
        self.assertNotIn("MYSQL_PASSWORD", validation)
        self.assertNotIn("MONGODB_PASSWORD", validation)
        self.assertIn("inputs.operation == 'prepare' && secrets.MYSQL_METADATA_ADMIN_PASSWORD", production)
        self.assertIn("inputs.operation == 'prepare' && secrets.MONGODB_PASSWORD", production)
        self.assertIn("inventory_request_sha256", workflow)
        self.assertIn("${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}.tar.gz", workflow)
        self.assertIn("qs-compatibility-retirement-${RETIREMENT_RUN_ID}.tar.gz", workflow)
        for key in ("cbpt-cleanup", "mongorestore", "gh workflow run"):
            self.assertNotIn(key, workflow)

    def test_existing_status_uses_standard_state_without_legacy_dependency(self):
        repository = SCRIPT.parents[2]
        workflow = (repository / ".github/workflows/db-ops.yml").read_text()
        self.assertNotIn("domain_event_outbox", workflow)
        self.assertIn('targetDB.getCollection("rm_outbox")', workflow)
        self.assertIn("FROM rm_outbox", workflow)
        self.assertIn("WHERE state <> 'published'", workflow)
        self.assertIn("STR_TO_DATE('${AUDIT_FROM}'", workflow)
        self.assertIn("|| github.event.inputs.operation == 'inventory'", workflow)
        self.assertNotIn("qs-server-db-ops-production", workflow)

    def test_current_production_provisioner_shares_writer_lock(self):
        repository = SCRIPT.parents[2]
        workflow = (repository / ".github/workflows/authz-production-matrix-provision.yml").read_text()
        self.assertIn("group: production-deploy", workflow)
        self.assertIn("cancel-in-progress: false", workflow)
        self.assertNotIn("group: qs-server-authz-production-matrix-provision", workflow)

    def test_actual_workflow_validation_normalizes_only_declared_optional_inputs(self):
        repository = SCRIPT.parents[2]
        workflow = (repository / ".github/workflows/compatibility-retirement.yml").read_text()
        script = textwrap.dedent(workflow.split("          script: |\n", 1)[1].split("      - name:", 1)[0])
        supplied = {"operation": "prepare", "database": "mysql-and-mongodb",
                    "approved_source_sha": SOURCE, "operation_id": OPERATION,
                    "prepare_mode": "identity", "identity_request_sha256": "1" * 64}
        scenarios = [("omitted_empty_defaults", supplied, "refs/heads/main", SOURCE, SOURCE, True),
                     ("all_defaults_present", dict(supplied, manifest_sha256="", inventory_request_sha256=""), "refs/heads/main", SOURCE, SOURCE, True),
                     ("unknown_input", dict(supplied, unknown=""), "refs/heads/main", SOURCE, SOURCE, False),
                     ("mixed_request", dict(supplied, inventory_request_sha256="2" * 64), "refs/heads/main", SOURCE, SOURCE, False),
                     ("missing_required", {k: v for k, v in supplied.items() if k != "operation"}, "refs/heads/main", SOURCE, SOURCE, False),
                     ("wrong_ref", supplied, "refs/heads/old", SOURCE, SOURCE, False),
                     ("wrong_runtime_sha", supplied, "refs/heads/main", "b" * 40, SOURCE, False),
                     ("main_advanced", supplied, "refs/heads/main", SOURCE, "b" * 40, False)]
        for name, inputs, ref, actual_sha, current_sha, allowed in scenarios:
            with self.subTest(name=name):
                context = {"payload": {"inputs": inputs}, "ref": ref, "sha": actual_sha, "repo": {}}
                program = ("const script=" + json.dumps(script) + ";const context=" + json.dumps(context)
                           + ";const current=" + json.dumps(current_sha)
                           + ";const github={rest:{repos:{getCommit:async()=>({data:{sha:current}})}}};"
                           + "new (Object.getPrototypeOf(async function(){}).constructor)('context','github',script)(context,github)"
                           + ".catch(()=>{process.exitCode=1;});")
                result = subprocess.run(["node", "-e", program], capture_output=True, text=True, check=False)
                self.assertEqual(result.returncode, 0 if allowed else 1, result.stderr)

    def test_workflow_embedded_javascript_parses(self):
        if not shutil.which("node"):
            self.skipTest("Node parser unavailable; no code is executed")
        repository = SCRIPT.parents[2]
        workflow = (repository / ".github/workflows/db-ops.yml").read_text()
        script = workflow.split("<<'MONGOJS'\n", 1)[1].split("\n            MONGOJS", 1)[0]
        result = subprocess.run(["node", "--check"], input=script, text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        workflow = (repository / ".github/workflows/compatibility-retirement.yml").read_text()
        script = workflow.split("          script: |\n", 1)[1].split("      - name:", 1)[0]
        result = subprocess.run(["node", "--check"], input="async function validate() {\n" + script + "\n}", text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == "__main__":
    unittest.main()
