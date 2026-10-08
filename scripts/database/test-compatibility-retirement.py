#!/usr/bin/env python3
"""Safety contracts: all fixtures are local synthetic test inputs, never evidence."""
import argparse
import base64
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


def inventory_binding(database, identity, version):
    return {"identity_hash": identity, "database_anchor_hash": identity if database == "mysql" else "7" * 64,
            "migration_generation_hash": "" if database == "mysql" else "8" * 64,
            "expected_identity_match": True, "migration_version": version, "migration_dirty": False,
            "expected_migration_match": True, "catalog_hash": "5" * 64, "non_target_schema_hash": "6" * 64,
            "metadata_complete": True, "permissions": {}, "outside_dependencies": 0,
            "dependency_coverage_complete": False, "inbound_foreign_key_coverage_complete": False,
            "dependency_scope": "metadata_only", "dependency_text_review_required": True, "error_category": "none"}


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

    def connection_environment(self):
        return {"MYSQL_HOST": "synthetic-mysql", "MYSQL_USERNAME": "synthetic-sql-reader",
                "MYSQL_PASSWORD": "synthetic-sql-secret", "MYSQL_DATABASE": "synthetic-business",
                "MONGODB_HOST": "synthetic-mongo", "MONGODB_USERNAME": "synthetic-app-user",
                "MONGODB_PASSWORD": "synthetic-app-secret", "MONGODB_DBNAME": "synthetic-business"}

    def test_inventory_legacy_mongo_pair_and_port_defaults(self):
        values = tool.inventory_connection_values(self.connection_environment())
        self.assertEqual(values["MONGODB_USERNAME"], "synthetic-app-user")
        self.assertEqual(values["MONGODB_PASSWORD"], "synthetic-app-secret")
        self.assertEqual(values["MYSQL_PORT"], "3306")
        self.assertEqual(values["MONGODB_PORT"], "27017")

    def test_inventory_metadata_mongo_pair_is_selected_together(self):
        environment = self.connection_environment()
        environment.update(MONGODB_METADATA_ADMIN_USERNAME="synthetic-meta-user",
                           MONGODB_METADATA_ADMIN_PASSWORD="synthetic-meta-secret")
        values = tool.inventory_connection_values(environment)
        self.assertEqual(values["MONGODB_USERNAME"], "synthetic-meta-user")
        self.assertEqual(values["MONGODB_PASSWORD"], "synthetic-meta-secret")
        self.assertEqual(environment["MONGODB_USERNAME"], "synthetic-app-user")
        self.assertEqual(environment["MONGODB_PASSWORD"], "synthetic-app-secret")
        self.assertNotIn("MONGODB_METADATA_ADMIN_USERNAME", values)
        self.assertNotIn("MONGODB_METADATA_ADMIN_PASSWORD", values)

    def test_inventory_metadata_pair_does_not_require_service_credentials(self):
        environment = self.connection_environment()
        del environment["MONGODB_USERNAME"]
        del environment["MONGODB_PASSWORD"]
        environment.update(MONGODB_METADATA_ADMIN_USERNAME="synthetic-meta-user",
                           MONGODB_METADATA_ADMIN_PASSWORD="synthetic-meta-secret")
        self.assertEqual(tool.inventory_connection_values(environment)["MONGODB_USERNAME"], "synthetic-meta-user")

    def test_inventory_partial_metadata_pair_never_mixes_credentials(self):
        for key in ("MONGODB_METADATA_ADMIN_USERNAME", "MONGODB_METADATA_ADMIN_PASSWORD"):
            with self.subTest(key=key):
                environment = self.connection_environment()
                environment[key] = "synthetic-partial-pair"
                self.assertBlocked("inventory_connection_input_invalid", tool.inventory_connection_values, environment)

    def test_inventory_metadata_pair_rejects_env_file_injection_with_fixed_error(self):
        for key in ("MONGODB_METADATA_ADMIN_USERNAME", "MONGODB_METADATA_ADMIN_PASSWORD"):
            for value in ("synthetic-private\nMYSQL_HOST=injected", "synthetic-private\r", "synthetic-private\x00", "x" * 4097, 123):
                with self.subTest(key=key, value_type=type(value).__name__):
                    environment = self.connection_environment()
                    environment.update(MONGODB_METADATA_ADMIN_USERNAME="synthetic-meta-user",
                                       MONGODB_METADATA_ADMIN_PASSWORD="synthetic-meta-secret")
                    environment[key] = value
                    self.assertBlocked("inventory_connection_input_invalid", tool.inventory_connection_values, environment)

    def test_inventory_metadata_pair_keeps_required_database_binding(self):
        for key in ("MONGODB_HOST", "MONGODB_DBNAME", "MYSQL_DATABASE"):
            with self.subTest(key=key):
                environment = self.connection_environment()
                environment.update(MONGODB_METADATA_ADMIN_USERNAME="synthetic-meta-user",
                                   MONGODB_METADATA_ADMIN_PASSWORD="synthetic-meta-secret")
                del environment[key]
                self.assertBlocked("inventory_connection_input_invalid", tool.inventory_connection_values, environment)

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

    def v2_request(self, boundary=False):
        value = {"format_version": 2, "kind": "readonly_inventory_boundary_request" if boundary else "readonly_inventory_request",
                 "operation_id": OPERATION, "source_sha": SOURCE, "target_hash": tool.TARGET_HASH,
                 "database_scope": "mysql-and-mongodb", "identity_hashes": {"mysql": "1" * 64, "mongodb": "2" * 64},
                 "expected_migrations": {"mysql": 98, "mongodb": 37}, "limits": tool.INVENTORY_V2_LIMITS.copy()}
        if not boundary:
            value.update(boundary_run_id="456-1", boundary_report_hash="a" * 64,
                         approved_boundaries=[{"database": database, "name": name, "kind": kind, "present": False,
                                               "empty": False, "pk_type": "", "upper_token": "",
                                               "schema_hash": "3" * 64, "identity_hash": "4" * 64}
                                              for database, name, kind in tool.TARGETS])
        return value

    def test_v2_exact_profile_and_independent_boundary_approval(self):
        value = self.v2_request()
        tool.validate_inventory_request(value, OPERATION, SOURCE)
        for mutate in (lambda v: v.pop("boundary_report_hash"), lambda v: v["limits"].update(page_size=2000),
                       lambda v: v["limits"].update(max_records=650000), lambda v: v["limits"].update(max_bytes=2147483649),
                       lambda v: v["limits"].update(query_seconds=31), lambda v: v["limits"].update(total_seconds=1501),
                       lambda v: v["limits"].update(max_pages=1002), lambda v: v.update(allow_unknown=True),
                       lambda v: v["approved_boundaries"][0].update(empty=True)):
            changed = copy.deepcopy(value); mutate(changed)
            with self.assertRaises(tool.Blocked):
                tool.validate_inventory_request(changed, OPERATION, SOURCE)
        discovery = self.v2_request(boundary=True)
        tool.validate_v2_request(discovery, OPERATION, SOURCE, boundary=True)
        discovery["approved_boundaries"] = value["approved_boundaries"]
        self.assertBlocked("evidence_fields_invalid", lambda: tool.validate_v2_request(discovery, OPERATION, SOURCE, boundary=True))

    def test_production_inventory_has_no_v1_bypass(self):
        tool.require_inventory_v2(self.v2_request())
        for version in (1, True, "2", 3, None):
            self.assertBlocked("inventory_v1_retired", tool.require_inventory_v2, {"format_version": version})

    def test_v2_present_empty_is_not_absence_and_sql_numbers_are_exact(self):
        value = self.v2_request()
        first = value["approved_boundaries"][0]
        first.update(present=True, empty=True, pk_type="uint64")
        tool.validate_inventory_request(value, OPERATION, SOURCE)
        first.update(empty=False, upper_token=base64.b64encode(b"18446744073709551615").decode())
        tool.validate_inventory_request(value, OPERATION, SOURCE)
        for raw in (b"18446744073709551616", b"01", b"+1", b"-1", b"PRIVATE_NOT_NUMERIC"):
            first["upper_token"] = base64.b64encode(raw).decode()
            self.assertBlocked("boundary_token_invalid", tool.validate_inventory_request, value, OPERATION, SOURCE)

    def test_boundary_report_is_exact_private_identity_head_and_never_auto_approved(self):
        value = self.v2_request()
        bounds = self.directory / "bounds-456-1"; bounds.mkdir(mode=0o700)
        observed = {"format_version": 2, "kind": "readonly_inventory_boundaries", "source_sha": SOURCE,
                    "operation_id": OPERATION, "run_id": "456-1", "target_hash": tool.TARGET_HASH,
                    "complete": True, "drop_ready": False, "diagnostic_only": True,
                    "targets": [{"boundary": b, "complete": True, "error_category": "none"} for b in value["approved_boundaries"]],
                    "database_bindings": {db: inventory_binding(db, value["identity_hashes"][db], value["expected_migrations"][db]) for db in ("mysql", "mongodb")}}
        def store(report):
            raw = json.dumps(report, separators=(",", ":")).encode()
            path = bounds / "boundary.private.json"; path.write_bytes(raw); path.chmod(0o600)
            value["boundary_report_hash"] = hashlib.sha256(raw).hexdigest()
        store(observed); tool.validate_approved_boundary_file(value, self.directory)
        for mutate in (lambda v: v.update(drop_ready=True), lambda v: v.update(source_sha="b" * 40),
                       lambda v: v.update(run_id="999-1"), lambda v: v["targets"][0]["boundary"].update(present=True),
                       lambda v: v["database_bindings"]["mysql"].update(migration_dirty=True),
                       lambda v: v["database_bindings"]["mongodb"].update(identity_hash="f" * 64)):
            changed = copy.deepcopy(observed); mutate(changed); store(changed)
            with self.assertRaises(tool.Blocked):
                tool.validate_approved_boundary_file(value, self.directory)
        store(observed); value["boundary_report_hash"] = "f" * 64
        with self.assertRaises(tool.Blocked):
            tool.validate_approved_boundary_file(value, self.directory)
        value["boundary_report_hash"] = hashlib.sha256((bounds / "boundary.private.json").read_bytes()).hexdigest()
        bounds.rename(self.directory / "real-bounds"); bounds.symlink_to(self.directory / "real-bounds", target_is_directory=True)
        with self.assertRaises(tool.Blocked):
            tool.validate_approved_boundary_file(value, self.directory)

    def test_bounds_cannot_mix_manifest_or_enable_nonprepare_stage(self):
        for operation in ("prepare", "apply", "verify", "recover", "purge"):
            args = self.arguments(operation); args.prepare_mode = "bounds"; args.inventory_request_hash = "a" * 64
            self.assertBlocked("input_classes_mixed", tool.execute, args)

    def test_inventory_v2_receipt_binds_every_target_and_never_unlocks_retirement(self):
        request = self.v2_request(); args = self.arguments()
        args.prepare_mode = "inventory"; args.inventory_request_hash = "a" * 64; args.run_id = "789-1"
        objects = [dict(database=db, name=name, kind=kind, present=False, complete=True, records=0,
                        bytes=0, schema_hash="3" * 64, data_hash="5" * 64, identity_hash="4" * 64,
                        classification={}, error_category="none", equal_full_passes=2, pages=0,
                        next_cycle_required=False, boundary=bound)
                   for (db, name, kind), bound in zip(tool.TARGETS, request["approved_boundaries"])]
        bindings = {db: inventory_binding(db, request["identity_hashes"][db], request["expected_migrations"][db])
                    for db in ("mysql", "mongodb")}
        report = {"format_version": 2, "kind": "readonly_compatibility_inventory", "source_sha": SOURCE,
                  "operation_id": OPERATION, "run_id": "789-1", "request_hash": "a" * 64, "target_hash": tool.TARGET_HASH,
                  "complete": True, "drop_ready": False, "diagnostic_only": True,
                  "boundary_report_hash": request["boundary_report_hash"], "database_bindings": bindings,
                  "targets": objects, "error_category": "none"}
        def receipt(value, mutate=None):
            raw = json.dumps(value, separators=(",", ":")).encode(); path = self.directory / "inventory.private.json"
            path.write_bytes(raw); path.chmod(0o600)
            summary = {key: value[key] for key in ("format_version", "kind", "source_sha", "operation_id", "run_id", "request_hash", "target_hash", "complete", "drop_ready", "diagnostic_only", "boundary_report_hash", "error_category", "database_bindings")}
            summary["private_report_hash"] = hashlib.sha256(raw).hexdigest()
            public_keys = ("database", "name", "present", "complete", "records", "bytes", "schema_hash", "data_hash", "identity_hash", "classification", "error_category", "equal_full_passes", "pages", "next_cycle_required")
            summary["targets"] = [dict({key: item[key] for key in public_keys}, boundary_hash=hashlib.sha256(json.dumps(item["boundary"], separators=(",", ":")).encode()).hexdigest()) for item in value["targets"]]
            if mutate:
                mutate(summary)
            return tool.validate_inventory_receipt(summary, 0, args, self.directory, request, "a" * 64, "b" * 64, "sha256:" + "c" * 64)
        safe = receipt(report)
        self.assertIs(safe["inventory_complete"], True); self.assertIs(safe["inventory_two_equal_scans"], True)
        for key in ("complete", "execution_allowed", "drop_ready"):
            self.assertIs(safe[key], False)
        self.assertIs(safe["capabilities"]["history_verifier"], False)
        self.assertNotIn("upper_token", json.dumps(safe)); self.assertNotIn("source_file", json.dumps(safe))
        self.assertBlocked("inventory_receipt_private_mismatch", lambda: receipt(report, lambda s: s["targets"][0].update(records=1)))
        self.assertBlocked("inventory_receipt_private_mismatch", lambda: receipt(report, lambda s: s.update(database_bindings={db: dict(b, database_anchor_hash="a" * 64) for db, b in s["database_bindings"].items()})))
        changed = copy.deepcopy(report); changed["targets"][0]["equal_full_passes"] = 1
        self.assertBlocked("inventory_receipt_boundary_invalid", receipt, changed)
        changed = copy.deepcopy(report); changed["targets"][0]["source_file"] = "PRIVATE_UNREGISTERED_BODY"
        self.assertBlocked("inventory_receipt_target_invalid", receipt, changed)
        output = io.StringIO()
        with mock.patch.object(tool, "execute", return_value=safe), contextlib.redirect_stdout(output):
            tool.main(["--operation", "prepare", "--operation-id", OPERATION, "--approved-source-sha", SOURCE,
                       "--actual-source-sha", SOURCE, "--run-id", "789-1"])
        decoded = json.loads(tool.transport().decode_armored_receipt(output.getvalue()))
        self.assertIs(decoded["drop_ready"], False); self.assertIs(decoded["inventory_two_equal_scans"], True)
        self.assertEqual(decoded["inventory_database_states"]["mongodb"]["migration_generation_hash"], "8" * 64)

    def test_database_anchor_contract_refuses_missing_old_or_invented_values(self):
        bindings = {db: inventory_binding(db, str(index) * 64, 95 if db == "mysql" else 36) for index, db in enumerate(("mysql", "mongodb"), 1)}
        tool.validate_inventory_bindings(bindings, True)
        mutations = (lambda b: b["mongodb"].pop("database_anchor_hash"),
                     lambda b: b["mongodb"].pop("migration_generation_hash"),
                     lambda b: b["mongodb"].update(database_anchor_hash=""),
                     lambda b: b["mongodb"].update(migration_generation_hash=""),
                     lambda b: b["mongodb"].update(database_anchor_hash="A" * 64),
                     lambda b: b["mongodb"].update(migration_generation_hash=None),
                     lambda b: b["mysql"].update(database_anchor_hash="a" * 64),
                     lambda b: b["mysql"].update(migration_generation_hash="a" * 64),
                     lambda b: b["mysql"].update(unapproved_anchor="a" * 64))
        for mutate in mutations:
            changed = copy.deepcopy(bindings); mutate(changed)
            with self.assertRaises(tool.Blocked):tool.validate_inventory_bindings(changed, True)

    def test_incomplete_anchor_observation_never_becomes_complete(self):
        bindings = {db: inventory_binding(db, "", 0) for db in ("mysql", "mongodb")}
        bindings["mongodb"].update(database_anchor_hash="", migration_generation_hash="")
        tool.validate_inventory_bindings(bindings, False)
        self.assertBlocked("database_anchor_missing", tool.validate_inventory_bindings, bindings, True)
        bindings["mysql"]["database_anchor_hash"] = "1" * 64
        self.assertBlocked("database_anchor_invalid", tool.validate_inventory_bindings, bindings, False)

    def test_private_source_copy_requires_exact_registry_and_safe_file(self):
        args = self.arguments(); filename = tool.SOURCE_FILENAMES[("mysql", "domain_event_outbox")]
        bound = self.v2_request()["approved_boundaries"][0]
        item = {"database": "mysql", "name": "domain_event_outbox", "source_file": filename, "boundary": bound}
        source = self.directory / filename; source.write_bytes(b"PRIVATE_BODY_SENTINEL"); source.chmod(0o600)
        registry = {"format_version": 1, "kind": "temporary_inventory_source_copy", "filename": filename,
                    "source_sha": SOURCE, "operation_id": OPERATION, "run_id": args.run_id, "request_hash": "a" * 64,
                    "protocol": "mysql_cast_binary_columns_pk_order_v2", "boundary": bound, "contains_original_body": True,
                    "retirement_proof": False, "purge_required_after_acceptance": True, "resume_existing_file_allowed": False}
        asset = self.directory / (filename + ".asset.json")
        asset.write_text(json.dumps(registry)); asset.chmod(0o600)
        tool.validate_source_asset(self.directory, item, args, "a" * 64, 1024)
        registry["request_hash"] = "b" * 64; asset.write_text(json.dumps(registry))
        self.assertBlocked("inventory_source_asset_binding_invalid", tool.validate_source_asset, self.directory, item, args, "a" * 64, 1024)
        registry["request_hash"] = "a" * 64; asset.write_text(json.dumps(registry))
        self.assertBlocked("inventory_source_asset_invalid", tool.validate_source_asset, self.directory, item, args, "a" * 64, 1)
        source.chmod(0o644)
        self.assertBlocked("inventory_source_asset_invalid", tool.validate_source_asset, self.directory, item, args, "a" * 64, 1024)
        source.chmod(0o600); linked = self.directory / "hardlinked-body"; os.link(source, linked)
        self.assertBlocked("inventory_source_asset_invalid", tool.validate_source_asset, self.directory, item, args, "a" * 64, 1024)
        source.unlink(); source.symlink_to(linked)
        self.assertBlocked("inventory_source_asset_unavailable", tool.validate_source_asset, self.directory, item, args, "a" * 64, 1024)

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
        state = {"identity_hash": "1" * 64, "database_anchor_hash": "1" * 64, "migration_generation_hash": "", "identity_observed": True, "migration_version": 95,
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
                   "database_states": {"mysql": state, "mongodb": dict(state, identity_hash="4" * 64, database_anchor_hash="7" * 64, migration_generation_hash="8" * 64, migration_version=36)},
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
        self.assertEqual(decoded["identity_database_states"]["mongodb"]["database_anchor_hash"], "7" * 64)
        self.assertIsNone(decoded["identity_database_states"]["mysql"]["migration_generation_hash"])
        for mutate in (lambda s: s["mysql"].update(database_anchor_hash="a" * 64),
                       lambda s: s["mysql"].update(migration_generation_hash="a" * 64),
                       lambda s: s["mongodb"].pop("database_anchor_hash"),
                       lambda s: s["mongodb"].update(migration_generation_hash="")):
            changed = copy.deepcopy(summary); mutate(changed["database_states"])
            with self.assertRaises(tool.Blocked):validate(changed)
        self.assertIs(decoded["execution_allowed"], False)
        self.assertIs(decoded["drop_ready"], False)
        self.assertEqual(decoded["identity_histogram_bucket_pages"]["page_a"][0]["type_label"], "footprint_entry_opened")
        changed = copy.deepcopy(summary); changed["diagnostic_histograms"][0]["present"] = 1
        self.assertBlocked("identity_histogram_target_invalid", validate, changed)
        changed = copy.deepcopy(summary); changed["diagnostic_histograms"][0]["buckets"].append(histogram[0]["buckets"][0])
        bounded = validate(changed)
        self.assertFalse(bounded["identity_histogram_bucket_pages"])
        self.assertTrue(all(h["error_category"] == "histogram_public_bound_exceeded" and not h["complete"] for h in bounded["identity_diagnostic_histograms"]))

    def test_identity_error_categories_are_bounded_database_specific_and_diagnostic_only(self):
        args, approval = self.bootstrap_fixture()
        directory = self.directory / "identity-701-1"
        baseline = json.loads((directory / "identity.private.json").read_bytes())
        args.run_id = baseline["run_id"]

        def validate(value, code):
            raw = tool.canonical_bytes(value)
            path = directory / "identity.private.json"
            path.write_bytes(raw); path.chmod(0o600)
            summary = dict(value, private_report_hash=hashlib.sha256(raw).hexdigest())
            return tool.validate_identity_receipt(summary, code, args, directory,
                                                 value["request_hash"], "b" * 64)

        for database, categories in tool.IDENTITY_ERRORS.items():
            for category in sorted(categories):
                with self.subTest(database=database, category=category):
                    changed = copy.deepcopy(baseline)
                    if category != "none":
                        changed["complete"] = False
                        changed["error_category"] = "identity_discovery_incomplete"
                        changed["database_states"][database].update(
                            identity_hash="", database_anchor_hash="", migration_generation_hash="",
                            identity_observed=False, migration_version=0, migration_head_observed=False,
                            migration_dirty=None, migration_clean=False, metadata_permissions_sufficient=False,
                            error_category=category)
                    receipt = validate(changed, 0 if changed["complete"] else 42)
                    self.assertEqual(receipt["identity_database_states"][database]["error_category"], category)
                    self.assertFalse(receipt["execution_allowed"])
                    self.assertFalse(receipt["drop_ready"])
                    output = io.StringIO()
                    with mock.patch.object(tool, "execute", return_value=receipt), contextlib.redirect_stdout(output):
                        self.assertEqual(tool.main(["--operation", "prepare", "--operation-id", OPERATION,
                                                   "--approved-source-sha", SOURCE, "--actual-source-sha", SOURCE,
                                                   "--run-id", "701-1"]), 42)
                    decoded = json.loads(tool.transport().decode_armored_receipt(output.getvalue()))
                    self.assertEqual(decoded["identity_database_states"][database]["error_category"], category)
                    self.assertFalse(decoded["execution_allowed"])
                    self.assertFalse(decoded["drop_ready"])

        for database, invalid in (("mysql", "mongo_replica_anchor_unavailable"),
                                  ("mongodb", "mysql_migration_head_invalid"),
                                  ("mongodb", "mongodb://private:password@private-host/database"),
                                  ("mongodb", "server_error\nPRIVATE_DO_NOT_PRINT"),
                                  ("mongodb", None), ("mongodb", True), ("mongodb", {})):
            with self.subTest(database=database, invalid_type=type(invalid).__name__):
                changed = copy.deepcopy(baseline)
                changed["complete"] = False
                changed["error_category"] = "identity_discovery_incomplete"
                changed["database_states"][database]["error_category"] = invalid
                self.assertBlocked("identity_error_category_invalid", validate, changed, 42)

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
        self.assertIn("inputs.operation == 'prepare' && !startsWith(inputs.prepare_mode, 'bootstrap-') && secrets.MYSQL_METADATA_ADMIN_PASSWORD", production)
        self.assertIn("inputs.operation == 'prepare' && !startsWith(inputs.prepare_mode, 'bootstrap-') && secrets.MONGODB_PASSWORD", production)
        for key in ("MONGODB_METADATA_ADMIN_USERNAME", "MONGODB_METADATA_ADMIN_PASSWORD"):
            self.assertNotIn(key, validation)
            self.assertIn("inputs.operation == 'prepare' && !startsWith(inputs.prepare_mode, 'bootstrap-') && secrets." + key, production)
            ssh_envs = production.split("          envs: ", 1)[1].split("\n", 1)[0].split(",")
            self.assertIn(key, ssh_envs)
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

    def test_history_bootstrap_wiring_preserves_old_credential_and_mutation_boundary(self):
        workflow = (SCRIPT.parents[2] / ".github/workflows/compatibility-retirement.yml").read_text()
        for key in ("MYSQL_METADATA_ADMIN_USERNAME", "MYSQL_METADATA_ADMIN_PASSWORD",
                    "MONGODB_METADATA_ADMIN_USERNAME", "MONGODB_METADATA_ADMIN_PASSWORD"):
            self.assertIn("inputs.prepare_mode == 'bootstrap-history' && secrets." + key, workflow)
        self.assertIn("!startsWith(inputs.prepare_mode, 'bootstrap-')", workflow)
        self.assertIn('history_binary="$tool_dir/history-linux-amd64"', workflow)
        self.assertIn('history_binary="$tool_dir/history-linux-arm64"', workflow)
        self.assertIn("CGO_ENABLED=0 GOOS=linux GOARCH=amd64", workflow)
        self.assertIn("CGO_ENABLED=0 GOOS=linux GOARCH=arm64", workflow)
        self.assertIn('main.sourceSHA=$GITHUB_SHA', workflow)
        self.assertIn("test-compatibility-history-prepare.py", workflow)
        self.assertFalse(tool.CAPABILITIES["history_verifier"])
        self.assertFalse(tool.CAPABILITIES["production_database_backend"])
        self.assertFalse(tool.CAPABILITIES["private_backup_restore_backend"])

    def test_actual_workflow_validation_normalizes_only_declared_optional_inputs(self):
        workflow = (SCRIPT.parents[2] / ".github/workflows/compatibility-retirement.yml").read_text()
        script = textwrap.dedent(workflow.split("          script: |\n", 1)[1].split("      - name:", 1)[0])
        supplied = {"operation": "prepare", "database": "mysql-and-mongodb", "approved_source_sha": SOURCE,
                    "operation_id": OPERATION, "prepare_mode": "identity", "identity_request_sha256": "1" * 64}
        bounds = {**supplied, "prepare_mode": "bounds", "inventory_request_sha256": "2" * 64}
        bounds.pop("identity_request_sha256")
        inventory = dict(bounds, prepare_mode="inventory")
        scenarios = [("omitted_empty_defaults", supplied, "refs/heads/main", SOURCE, SOURCE, True),
                     ("all_defaults_present", dict(supplied, manifest_sha256="", inventory_request_sha256=""), "refs/heads/main", SOURCE, SOURCE, True),
                     ("bounds_omitted_identity", bounds, "refs/heads/main", SOURCE, SOURCE, True),
                     ("inventory_omitted_identity", inventory, "refs/heads/main", SOURCE, SOURCE, True),
                     ("bounds_missing_request", {k: v for k, v in bounds.items() if k != "inventory_request_sha256"}, "refs/heads/main", SOURCE, SOURCE, False),
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

    def bootstrap_fixture(self, inventory=False):
        def store(directory, filename, value):
            raw = tool.canonical_bytes(value)
            path = directory / filename; path.write_bytes(raw); path.chmod(0o600)
            return hashlib.sha256(raw).hexdigest()
        identity_raw = tool.identity_request_bytes(OPERATION, SOURCE)
        (self.directory / "identity-request.json").write_bytes(identity_raw)
        (self.directory / "identity-request.json").chmod(0o600)
        identity_output = self.directory / "identity-701-1"; identity_output.mkdir(mode=0o700, exist_ok=True)
        state = {"identity_hash": "1" * 64, "database_anchor_hash": "1" * 64, "migration_generation_hash": "", "identity_observed": True, "migration_version": 95,
                 "migration_head_observed": True, "migration_dirty": False, "migration_clean": True,
                 "metadata_permissions_sufficient": True, "permission_scope": "identity_and_migration_head", "error_category": "none"}
        identity = {"format_version": 1, "kind": "readonly_identity_discovery", "source_sha": SOURCE,
                    "operation_id": OPERATION, "run_id": "701-1", "request_hash": hashlib.sha256(identity_raw).hexdigest(),
                    "target_hash": tool.TARGET_HASH, "diagnostic_only": True, "drop_ready": False, "complete": True,
                    "identity_protocols": {"mysql": "mysql_database_identity_v1", "mongodb": "mongodb_database_identity_v1"},
                    "database_states": {"mysql": state, "mongodb": dict(state, identity_hash="2" * 64, database_anchor_hash="7" * 64, migration_generation_hash="8" * 64, migration_version=36)},
                    "diagnostic_histograms": [{"database": db, "name": name, "present": True, "complete": True,
                                               "error_category": "none", "diagnostic_only": True, "buckets": []} for db, name, _ in tool.TARGETS],
                    "error_category": "none"}
        identity_hash = store(identity_output, "identity.private.json", identity)
        value = {"format_version": 1, "kind": "readonly_request_bootstrap_approval", "prepare_mode": "bootstrap-bounds",
                 "source_sha": SOURCE, "operation_id": OPERATION, "target_hash": tool.TARGET_HASH,
                 "database_scope": "mysql-and-mongodb", "identity_report": {"run_id": "701-1", "source_sha": SOURCE, "sha256": identity_hash},
                 "identity_hashes": {"mysql": "1" * 64, "mongodb": "2" * 64}, "expected_migrations": {"mysql": 95, "mongodb": 36},
                 "limits": tool.INVENTORY_V2_LIMITS.copy()}
        args = self.arguments(); args.manifest_hash = ""; args.prepare_mode = "bootstrap-bounds"; args.run_id = "703-1"
        def approve():
            args.bootstrap_approval_json = tool.canonical_bytes(value)[:-1].decode("ascii")
            args.bootstrap_approval_hash = hashlib.sha256(tool.canonical_bytes(value)).hexdigest()
        approve()
        if inventory:
            tool.execute(args)
            boundary_request, boundary_hash = tool.read_private(self.directory, "boundary-request.json")
            bounds = self.directory / "bounds-702-1"; bounds.mkdir(mode=0o700)
            targets = []
            for database, name, kind in tool.TARGETS:
                bound = {"database": database, "name": name, "kind": kind, "present": True, "empty": name == "ai_messaging_legacy_commands",
                         "pk_type": "long" if database == "mongodb" else "uint64", "upper_token": "" if name == "ai_messaging_legacy_commands" else base64.b64encode(b"PRIVATE_UPPER_TOKEN" if database == "mongodb" else b"42").decode(),
                         "schema_hash": "3" * 64, "identity_hash": "4" * 64}
                targets.append({"database": database, "name": name, "kind": kind, "present": True, "complete": True,
                                "records": 0, "bytes": 0, "schema_hash": "3" * 64, "data_hash": "", "identity_hash": "4" * 64,
                                "classification": {}, "error_category": "none", "equal_full_passes": 0, "pages": 0,
                                "next_cycle_required": False, "boundary": bound})
            bindings = {db: inventory_binding(db, value["identity_hashes"][db], value["expected_migrations"][db])
                        for db in ("mysql", "mongodb")}
            report = {"format_version": 2, "kind": "readonly_inventory_boundaries", "source_sha": SOURCE,
                      "operation_id": OPERATION, "run_id": "702-1", "request_hash": boundary_hash, "target_hash": tool.TARGET_HASH,
                      "observed_at": "2026-10-08T12:00:00Z", "complete": True, "drop_ready": False, "diagnostic_only": True,
                      "error_category": "none", "database_bindings": bindings, "targets": targets,
                      "source_bytes_protocol": "no_source_body_copy", "consistency_semantics": "diagnostic_upper_discovery_requires_independent_request_approval"}
            value.update(prepare_mode="bootstrap-inventory", boundary_report={"run_id": "702-1", "source_sha": SOURCE,
                                                                             "sha256": store(bounds, "boundary.private.json", report)})
            args.prepare_mode = "bootstrap-inventory"; args.run_id = "704-1"; approve()
        return args, value

    def reapprove_bootstrap(self, args, value):
        args.bootstrap_approval_json = tool.canonical_bytes(value)[:-1].decode("ascii")
        args.bootstrap_approval_hash = hashlib.sha256(tool.canonical_bytes(value)).hexdigest()

    def test_bootstrap_bounds_uses_real_identity_and_keeps_request_approval_separate(self):
        args, value = self.bootstrap_fixture()
        with mock.patch.object(tool, "live_inventory", side_effect=AssertionError("DB path called")), mock.patch.object(tool, "capture_fixed", side_effect=AssertionError("runtime called")), mock.patch.object(tool.subprocess, "run", side_effect=AssertionError("child called")):
            receipt = tool.execute(args)
        request, digest = tool.read_private(self.directory, "boundary-request.json", receipt["derived_request_sha256"])
        self.assertNotEqual(digest, args.bootstrap_approval_hash)
        tool.validate_v2_request(request, OPERATION, SOURCE, boundary=True)
        self.assertTrue(receipt["request_bootstrap_complete"])
        self.assertFalse(receipt["complete"]); self.assertFalse(receipt["execution_allowed"]); self.assertFalse(receipt["drop_ready"])
        args.run_id = "705-1"
        again = tool.execute(args)
        self.assertEqual(again["derived_request_sha256"], digest)
        self.assertEqual(again["request_created_run_id"], "703-1")
        self.assertTrue(all(not ready for key, ready in tool.CAPABILITIES.items() if key.endswith("backend")))

    def test_bootstrap_inventory_copies_only_approved_private_bounds_and_receipt_has_no_tokens(self):
        args, value = self.bootstrap_fixture(inventory=True)
        with mock.patch.object(tool, "live_inventory", side_effect=AssertionError("DB path called")), mock.patch.object(tool, "capture_fixed", side_effect=AssertionError("runtime called")):
            receipt = tool.execute(args)
        request, digest = tool.read_private(self.directory, "inventory-request.json", receipt["derived_request_sha256"])
        tool.validate_inventory_request(request, OPERATION, SOURCE)
        tool.validate_approved_boundary_file(request, self.directory)
        self.assertEqual(request["boundary_report_hash"], value["boundary_report"]["sha256"])
        self.assertEqual(request["approved_boundaries"][3]["upper_token"], base64.b64encode(b"PRIVATE_UPPER_TOKEN").decode())
        self.assertNotIn("PRIVATE_UPPER_TOKEN", json.dumps(receipt))
        self.assertNotIn(base64.b64encode(b"PRIVATE_UPPER_TOKEN").decode(), json.dumps(receipt))
        output = io.StringIO()
        with mock.patch.object(tool, "execute", return_value=receipt), contextlib.redirect_stdout(output):
            tool.main(["--operation", "prepare", "--operation-id", OPERATION, "--approved-source-sha", SOURCE, "--actual-source-sha", SOURCE, "--run-id", args.run_id])
        decoded = json.loads(tool.transport().decode_armored_receipt(output.getvalue()))
        self.assertEqual(decoded["derived_request_sha256"], digest)
        self.assertFalse(decoded["execution_allowed"])

    def test_bootstrap_public_descriptor_cannot_contain_private_or_unknown_fields(self):
        args, value = self.bootstrap_fixture()
        for change in (lambda v: v.update(password="TEST_PRIVATE_DO_NOT_PRINT"), lambda v: v.update(database_name="secret_database"),
                       lambda v: v.update(original_id="old-id"), lambda v: v.update(upper_token="token"), lambda v: v.update(complete=True),
                       lambda v: v["identity_report"].update(path="../identity.private.json"), lambda v: v["limits"].update(total_seconds=1501),
                       lambda v: v["expected_migrations"].update(mysql=True), lambda v: v["expected_migrations"].update(mysql=2**53),
                       lambda v: v.update(source_sha="b" * 40), lambda v: v.update(operation_id="999-1"),
                       lambda v: v["identity_report"].update(source_sha="b" * 40), lambda v: v["identity_report"].update(run_id=args.run_id)):
            changed = copy.deepcopy(value); change(changed); self.reapprove_bootstrap(args, changed)
            with self.assertRaises(tool.Blocked):tool.execute(args)
        self.assertFalse((self.directory / "boundary-request.json").exists())

    def test_bootstrap_canonical_hash_and_stage_are_required_before_files(self):
        args, value = self.bootstrap_fixture()
        for text in (json.dumps(value), args.bootstrap_approval_json + "\n", '{"x":1,"x":2}', '"text"', '[]', 'null', '{"x":"非ASCII"}', 'x' * 4097):
            args.bootstrap_approval_json = text
            with self.assertRaises(tool.Blocked):tool.execute(args)
        self.reapprove_bootstrap(args, value); args.bootstrap_approval_hash = "0" * 64
        with self.assertRaises(tool.Blocked):tool.execute(args)
        self.reapprove_bootstrap(args, value)
        for operation in ("apply", "verify", "recover", "purge"):
            args.operation = operation
            self.assertBlocked("input_classes_mixed", tool.execute, args)
        args.operation = "prepare"; args.inventory_request_hash = "1" * 64
        self.assertBlocked("input_classes_mixed", tool.execute, args)
        args.inventory_request_hash = ""; args.prepare_mode = "bounds"
        self.assertBlocked("input_classes_mixed", tool.execute, args)

    def test_bootstrap_identity_report_drift_dirty_incomplete_and_origin_block(self):
        args, value = self.bootstrap_fixture()
        path = self.directory / "identity-701-1" / "identity.private.json"
        original = tool.decode(path.read_bytes())
        for change in (lambda v: v.update(source_sha="b" * 40), lambda v: v.update(operation_id="999-1"), lambda v: v.update(run_id="999-1"),
                       lambda v: v.update(complete=False), lambda v: v.update(request_hash="0" * 64),
                       lambda v: v["database_states"]["mysql"].update(migration_dirty=True),
                       lambda v: v["database_states"]["mysql"].update(migration_version=96),
                       lambda v: v["database_states"]["mongodb"].update(identity_hash="3" * 64),
                       lambda v: v["diagnostic_histograms"][0].update(complete=False)):
            report = copy.deepcopy(original); change(report); path.write_bytes(tool.canonical_bytes(report))
            value["identity_report"]["sha256"] = hashlib.sha256(path.read_bytes()).hexdigest(); self.reapprove_bootstrap(args, value)
            with self.assertRaises(tool.Blocked):tool.execute(args)
        self.assertFalse((self.directory / "boundary-request.json").exists())

    def test_bootstrap_boundary_report_drift_unknown_body_and_dirty_block(self):
        args, value = self.bootstrap_fixture(inventory=True)
        path = self.directory / "bounds-702-1" / "boundary.private.json"
        original = tool.decode(path.read_bytes())
        for change in (lambda v: v.update(source_sha="b" * 40), lambda v: v.update(operation_id="999-1"), lambda v: v.update(run_id="999-1"),
                       lambda v: v.update(complete=False), lambda v: v.update(request_hash="0" * 64), lambda v: v.update(raw_body="TEST_PRIVATE_DO_NOT_PRINT"),
                       lambda v: v["targets"].reverse(), lambda v: v["targets"][0].update(records=1), lambda v: v["targets"][0].update(source_file="data.ndjson"),
                       lambda v: v["targets"][0]["boundary"].update(schema_hash="5" * 64), lambda v: v["targets"][0].update(present=False),
                       lambda v: v["database_bindings"]["mysql"].update(migration_dirty=True), lambda v: v["database_bindings"]["mongodb"].update(migration_version=37)):
            report = copy.deepcopy(original); change(report); path.write_bytes(tool.canonical_bytes(report))
            value["boundary_report"]["sha256"] = hashlib.sha256(path.read_bytes()).hexdigest(); self.reapprove_bootstrap(args, value)
            with self.assertRaises(tool.Blocked):tool.execute(args)
        self.assertFalse((self.directory / "inventory-request.json").exists())

    def test_bootstrap_existing_request_registry_symlink_and_partial_never_overwrite(self):
        args, value = self.bootstrap_fixture()
        receipt = tool.execute(args)
        request_path = self.directory / "boundary-request.json"; good = request_path.read_bytes()
        request_path.write_bytes(b'{"wrong":true}')
        with self.assertRaises(tool.Blocked):tool.execute(args)
        self.assertEqual(request_path.read_bytes(), b'{"wrong":true}')
        request_path.write_bytes(good)
        registry = self.directory / "boundary-request-bootstrap.json"
        value = tool.decode(registry.read_bytes()); value["approval_sha256"] = "0" * 64
        registry.write_bytes(tool.canonical_bytes(value))
        self.assertBlocked("bootstrap_request_binding_mismatch", tool.execute, args)
        registry.unlink(); registry.symlink_to(request_path)
        with self.assertRaises(tool.Blocked):tool.execute(args)
        registry.unlink(); request_path.unlink()
        partial = self.directory / "boundary-request.json.bootstrap.partial"; partial.write_bytes(b'incomplete'); partial.chmod(0o600)
        self.assertBlocked("bootstrap_request_creation_incomplete", tool.execute, args)
        self.assertEqual(partial.read_bytes(), b'incomplete')

    def test_bootstrap_interruption_keeps_explicit_checkpoint_and_refuses_resume(self):
        args, value = self.bootstrap_fixture()
        original = tool.create_bootstrap_file
        def interrupt(directory, filename, raw):
            if filename == "boundary-request.json":raise tool.Blocked("bootstrap_request_creation_incomplete")
            return original(directory, filename, raw)
        with mock.patch.object(tool, "create_bootstrap_file", side_effect=interrupt):
            self.assertBlocked("bootstrap_request_creation_incomplete", tool.execute, args)
        self.assertTrue((self.directory / "boundary-request-bootstrap.json").exists())
        self.assertFalse((self.directory / "boundary-request.json").exists())
        args.run_id = "705-1"
        self.assertBlocked("bootstrap_request_creation_incomplete", tool.execute, args)

    def test_bootstrap_native_lock_blocks_concurrent_process_and_idempotent_after_release(self):
        args, value = self.bootstrap_fixture()
        with tool.locked_operation(self.directory):
            command = ["python3", "-B", str(SCRIPT), "--operation", "prepare", "--root", str(self.root),
                       "--operation-id", OPERATION, "--approved-source-sha", SOURCE, "--actual-source-sha", SOURCE,
                       "--run-id", args.run_id, "--prepare-mode", args.prepare_mode,
                       "--bootstrap-approval-json", args.bootstrap_approval_json, "--bootstrap-approval-hash", args.bootstrap_approval_hash]
            process = subprocess.run(command, capture_output=True, text=True, check=False)
            self.assertEqual(process.returncode, 42)
            receipt = json.loads(tool.transport().decode_armored_receipt(process.stdout))
            self.assertEqual(receipt["error_category"], "operation_busy")
        self.assertFalse((self.directory / "boundary-request.json").exists())
        tool.execute(args); tool.execute(args)

    def test_bootstrap_native_process_exit_leaves_partial_and_never_resumes(self):
        args, value = self.bootstrap_fixture()
        program = ("import argparse,importlib.util,os;spec=importlib.util.spec_from_file_location('t',"+repr(str(SCRIPT))+");"
                   "t=importlib.util.module_from_spec(spec);spec.loader.exec_module(t);"
                   "args=argparse.Namespace(**"+repr(vars(args))+");t.os.fsync=lambda fd:os._exit(9);t.execute(args)")
        result = subprocess.run(["python3", "-B", "-c", program], capture_output=True, check=False)
        self.assertEqual(result.returncode, 9)
        partial = self.directory / "boundary-request-bootstrap.json.bootstrap.partial"
        self.assertTrue(partial.exists()); before = partial.read_bytes()
        self.assertEqual(partial.stat().st_mode & 0o777, 0o600)
        self.assertFalse((self.directory / "boundary-request.json").exists())
        self.assertBlocked("bootstrap_request_creation_incomplete", tool.execute, args)
        self.assertEqual(partial.read_bytes(), before)

    def test_bootstrap_concurrent_real_callers_do_not_clobber_request_or_origin(self):
        args, value = self.bootstrap_fixture()
        command = ["python3", "-B", str(SCRIPT), "--operation", "prepare", "--root", str(self.root),
                   "--operation-id", OPERATION, "--approved-source-sha", SOURCE, "--actual-source-sha", SOURCE,
                   "--run-id", args.run_id, "--prepare-mode", args.prepare_mode,
                   "--bootstrap-approval-json", args.bootstrap_approval_json, "--bootstrap-approval-hash", args.bootstrap_approval_hash]
        first = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        second_command = list(command); second_command[second_command.index("--run-id")+1] = "705-1"
        second = subprocess.Popen(second_command, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        receipts = []
        for process in (first, second):
            output, error = process.communicate(timeout=10)
            self.assertEqual(process.returncode, 42, error)
            receipts.append(json.loads(tool.transport().decode_armored_receipt(output.decode())))
        ready = [r for r in receipts if r.get("request_bootstrap_complete")]
        self.assertGreaterEqual(len(ready), 1)
        self.assertTrue(all(r.get("request_bootstrap_complete") or r["error_category"] == "operation_busy" for r in receipts))
        request, digest = tool.read_private(self.directory, "boundary-request.json")
        tool.validate_v2_request(request, OPERATION, SOURCE, boundary=True)
        self.assertTrue(all(r["derived_request_sha256"] == digest for r in ready))
        registry, _ = tool.read_private(self.directory, "boundary-request-bootstrap.json")
        self.assertIn(registry["created_run_id"], ("703-1", "705-1"))
        self.assertEqual(registry["request_sha256"], digest)

    def test_actual_bootstrap_workflow_validation_public_descriptor_and_hash(self):
        args, value = self.bootstrap_fixture()
        workflow = (SCRIPT.parents[2] / ".github/workflows/compatibility-retirement.yml").read_text()
        script = textwrap.dedent(workflow.split("          script: |\n", 1)[1].split("      - name:", 1)[0])
        base = {"operation": "prepare", "database": "mysql-and-mongodb", "approved_source_sha": SOURCE, "operation_id": OPERATION,
                "prepare_mode": args.prepare_mode, "bootstrap_approval_json": args.bootstrap_approval_json, "bootstrap_approval_sha256": args.bootstrap_approval_hash}
        scenarios = [(base, True), (dict(base, bootstrap_approval_sha256="0" * 64), False),
                     (dict(base, inventory_request_sha256="1" * 64), False), (dict(base, operation="apply"), False),
                     (dict(base, unknown=""), False), (dict(base, prepare_mode="inventory"), False),
                     (dict(base, bootstrap_approval_json=args.bootstrap_approval_json+"\n"), False)]
        inventory_value = dict(value, prepare_mode="bootstrap-inventory", boundary_report={"run_id": "702-1", "source_sha": SOURCE, "sha256": "3" * 64})
        inventory_input = dict(base, prepare_mode="bootstrap-inventory", bootstrap_approval_json=tool.canonical_bytes(inventory_value)[:-1].decode(), bootstrap_approval_sha256=hashlib.sha256(tool.canonical_bytes(inventory_value)).hexdigest())
        scenarios.append((inventory_input, True))
        invalid_origin = copy.deepcopy(inventory_value); invalid_origin["boundary_report"]["run_id"] = "701-1"
        scenarios.append((dict(inventory_input, bootstrap_approval_json=tool.canonical_bytes(invalid_origin)[:-1].decode(), bootstrap_approval_sha256=hashlib.sha256(tool.canonical_bytes(invalid_origin)).hexdigest()), False))
        for change in (lambda v: v.update(password="TEST_PRIVATE_DO_NOT_PRINT"), lambda v: v.update(complete=True),
                       lambda v: v["identity_report"].update(source_sha="b" * 40), lambda v: v["expected_migrations"].update(mysql=True),
                       lambda v: v["limits"].update(page_size=2000)):
            changed = copy.deepcopy(value); change(changed)
            scenarios.append((dict(base, bootstrap_approval_json=tool.canonical_bytes(changed)[:-1].decode(), bootstrap_approval_sha256=hashlib.sha256(tool.canonical_bytes(changed)).hexdigest()), False))
        for inputs, allowed in scenarios:
            context = {"payload": {"inputs": inputs}, "ref": "refs/heads/main", "sha": SOURCE, "repo": {}}
            program = ("const script="+json.dumps(script)+";const context="+json.dumps(context)+";const github={rest:{repos:{getCommit:async()=>({data:{sha:"+json.dumps(SOURCE)+"}})}}};"
                       +"new (Object.getPrototypeOf(async function(){}).constructor)('context','github','require',script)(context,github,require).catch(()=>{process.exitCode=1;});")
            result = subprocess.run(["node", "-e", program], capture_output=True, text=True, check=False)
            self.assertEqual(result.returncode, 0 if allowed else 1, result.stderr)


class MetadataWorkflowContracts(unittest.TestCase):
    def test_metadata_has_separate_no_database_environment_ssh_step(self):
        workflow=(SCRIPT.parents[2]/".github/workflows/compatibility-retirement.yml").read_text()
        step=workflow.split("      - name: Observe approved inventory file metadata without database credentials\n",1)[1]
        self.assertIn("if: inputs.prepare_mode == 'bootstrap-history-metadata'",step)
        for forbidden in ("MYSQL_","MONGODB_","inventory_binary","history_binary","uname -m","go build","docker create","docker rm"):
            self.assertNotIn(forbidden,step)
        envs=step.split("          envs: ",1)[1].split("\n",1)[0].split(",")
        self.assertEqual(len(envs),9);self.assertTrue(all(name.startswith("RETIREMENT_") for name in envs))
        old=workflow.split("      - name: Inventory source bytes or reject unavailable lifecycle stage\n",1)[1].split("      - name: Observe approved",1)[0]
        self.assertIn("if: inputs.prepare_mode != 'bootstrap-history-metadata'",old)
        self.assertIn("MONGODB_METADATA_ADMIN_PASSWORD",old)
        setup=workflow.split("      - name: Set up Go for immutable read-only inventory\n",1)[1].split("      - name:",1)[0]
        self.assertIn("if: inputs.prepare_mode != 'bootstrap-history-metadata'",setup)

    def test_actual_metadata_package_has_exact_four_files_and_never_executes_go(self):
        repository=SCRIPT.parents[2]
        workflow=(repository/".github/workflows/compatibility-retirement.yml").read_text()
        step=workflow.split("      - name: Package only immutable tooling\n",1)[1].split("      - name:",1)[0]
        body=textwrap.dedent(step.split("        run: |\n",1)[1])
        with tempfile.TemporaryDirectory() as temporary:
            root=Path(temporary).resolve();(root/"scripts/database").mkdir(parents=True);(root/"scripts/dbops").mkdir(parents=True)
            for rel in ("scripts/database/compatibility-retirement.py","scripts/database/compatibility-history-prepare.py",
                        "scripts/database/compatibility-retirement-entrypoints.json","scripts/dbops/receipt-transport.py"):
                (root/rel).write_bytes((repository/rel).read_bytes())
            tools=root/"tools";tools.mkdir(mode=0o700)
            sentinel=tools/"go";sentinel.write_text("#!/bin/sh\necho unexpected_go >&2\nexit 99\n");sentinel.chmod(0o700)
            runtemp=root/"runtime";runtemp.mkdir(mode=0o700);output=root/"output"
            env=dict(os.environ,RETIREMENT_PACKAGE_MODE="bootstrap-history-metadata",RUNNER_TEMP=str(runtemp),
                GITHUB_RUN_ID="900",GITHUB_RUN_ATTEMPT="1",GITHUB_SHA=SOURCE,GITHUB_OUTPUT=str(output),PATH=str(tools)+os.pathsep+os.environ["PATH"])
            result=subprocess.run(["bash","-c",body],cwd=root,env=env,capture_output=True,check=False)
            self.assertEqual(result.returncode,0,result.stderr.decode())
            archive=root/"qs-compatibility-retirement-900-1.tar.gz"
            listing=subprocess.run(["tar","-tzf",str(archive)],capture_output=True,check=True).stdout.decode().splitlines()
            self.assertEqual(listing,["compatibility-retirement.py","compatibility-history-prepare.py","compatibility-retirement-entrypoints.json","receipt-transport.py"])
            self.assertEqual(output.read_text().strip(),"sha256="+hashlib.sha256(archive.read_bytes()).hexdigest())
            self.assertEqual(list(runtemp.iterdir()),[])


if __name__ == "__main__":
    unittest.main()
