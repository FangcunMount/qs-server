#!/usr/bin/env python3
"""Fail-closed preparation contract for the four compatibility namespaces.

This A-stage tool validates private, hash-bound evidence. It does not implement
a production database backend. In particular,
operator-supplied booleans cannot turn an unimplemented fence/history/restore
verifier into an execution permit. No raw payload, credentials, or rejected
input is printed. See compatibility-retirement.md for the remaining adapters.
"""

import argparse
import base64
import contextlib
import datetime
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tempfile
import time


OPERATIONS = frozenset({"prepare", "apply", "verify", "recover", "purge"})
TARGETS = (
    ("mysql", "domain_event_outbox", "base_table"),
    ("mysql", "ai_bridge_commands", "base_table"),
    ("mysql", "ai_messaging_legacy_commands", "base_table"),
    ("mongodb", "domain_event_outbox", "collection"),
)
TARGET_HASH = hashlib.sha256(json.dumps(TARGETS, separators=(",", ":")).encode()).hexdigest()
SOURCE_FILENAMES = {("mysql", name): "mysql-" + name + ".source.ndjson" for _, name, _ in TARGETS[:3]}
SOURCE_FILENAMES[("mongodb", "domain_event_outbox")] = "mongodb-domain_event_outbox.source.bsonframes"
ASSET_FILENAMES = frozenset(filename + ".asset.json" for filename in SOURCE_FILENAMES.values())
ROOT_SUFFIX = ("backups", "qs-server", "compatibility-retirement")
SHA = re.compile(r"^[0-9a-f]{40}$")
HASH = re.compile(r"^[0-9a-f]{64}$")
RUN = re.compile(r"^[0-9]{1,20}-[0-9]{1,4}$")
NAME = re.compile(r"^[a-z][a-z0-9_-]{0,80}\.json$")
MAX_JSON = 256 * 1024
INVENTORY_V2_LIMITS = {"query_seconds": 30, "total_seconds": 1500, "max_records": 1000000,
                       "max_bytes": 2147483648, "page_size": 10000, "max_pages": 1001}
BOOTSTRAP_MODES = frozenset({"bootstrap-bounds", "bootstrap-inventory", "bootstrap-history", "bootstrap-history-metadata", "bootstrap-history-parent", "bootstrap-ai-bounds", "bootstrap-ai-verify", "historical-evidence-write", "historical-ai-bounds"})
MAX_BOOTSTRAP_APPROVAL = 4096
MAX_WINDOW_SECONDS = 1800
FORWARD_STOP_SECONDS = 1200
CAPABILITIES = {
    "manifest_validation": True,
    "immutable_evidence_validation": True,
    "durable_ddl_journal": False,
    "live_inventory_verifier": False,
    "history_verifier": False,
    "historical_rerun_fence_verifier": False,
    "private_backup_restore_backend": False,
    "production_database_backend": False,
    "prepared_release_backend": False,
    "live_acceptance_verifier": False,
    "private_asset_purge_backend": False,
}
PROOF_KINDS = frozenset({"inventory", "history", "fence", "backup_restore", "release", "acceptance"})
HISTOGRAM_TYPES = frozenset({"unknown_type", "request", "change", "cancel", "prepare", "start", "answer",
    "answersheet.submitted", "evaluation.requested", "evaluation.retry.requested", "evaluation.outcome.committed", "evaluation.failed", "interpretation.report.generated", "interpretation.report.failed", "interpretation.retry.requested", "task.opened.reminder.requested",
    "footprint.entry_opened", "footprint.intake_confirmed", "footprint.testee_profile_created", "footprint.care_relationship_established", "footprint.care_relationship_transferred", "footprint.answersheet_submitted", "footprint.assessment_created", "footprint.report_generated",
    "interpretation.ai_explanation.requested", "interpretation.ai_explanation.retry.requested", "interpretation.ai_explanation.lease_recovery.requested", "interpretation.ai_explanation.generated", "interpretation.ai_explanation.failed", "interpretation.ai_explanation.prompt_evaluation.step_requested",
    "assessment.submitted", "assessment.evaluated", "assessment.interpreted", "assessment.failed", "report.generated", "questionnaire.changed", "scale.changed", "task.opened", "task.completed", "task.expired", "task.canceled"})
HISTOGRAM_STATES = frozenset({"unknown_state", "pending", "publishing", "published", "failed", "quarantined", "retry_wait", "0", "1", "historical_mapping"})
HISTOGRAM_ERRORS = frozenset({"none", "identity_metadata_not_ready", "histogram_namespace_kind_rejected", "histogram_schema_rejected", "histogram_query_failed_or_timed_out", "histogram_bucket_bound_exceeded", "histogram_public_bound_exceeded", "histogram_count_invalid", "histogram_metadata_query_failed", "mysql_metadata_read_failed", "metadata_bound_exceeded", "connection_input_invalid", "mongo_connection_failed", "mongo_close_failed"})


# These are producer-owned, fixed categories, never raw driver exceptions.
# Keep the allowlists scoped per database so failed identity discovery can be
# diagnosed without leaking names, connection strings or server error text.
IDENTITY_ERRORS = {
    "mysql": frozenset({"none", "connection_input_invalid", "mysql_connection_invalid",
        "mysql_connection_failed", "mysql_close_failed", "mysql_readonly_transaction_failed",
        "mysql_readonly_close_failed", "mysql_identity_or_version_rejected",
        "mysql_metadata_read_failed", "metadata_bound_exceeded",
        "mysql_global_metadata_visibility_unproven", "mysql_migration_head_invalid",
        "migration_head_rejected"}),
    "mongodb": frozenset({"none", "connection_input_invalid", "mongo_connection_failed",
        "mongo_close_failed", "mongo_identity_read_failed", "mongo_version_rejected",
        "mongo_identity_metadata_permission_or_missing", "mongo_database_uuid_unavailable",
        "mongo_replica_anchor_permission_or_read_failed", "mongo_replica_anchor_not_authorized",
        "mongo_replica_anchor_replication_not_enabled", "mongo_replica_anchor_metadata_rejected",
        "mongo_replica_anchor_topology_rejected", "mongo_replica_anchor_unavailable",
        "mongo_migration_generation_rejected", "mongo_privileges_read_failed",
        "mongo_namespace_anchor_read_failed", "mongo_namespace_anchor_rejected", "mongo_namespace_anchor_mismatch",
        "mongo_migration_head_invalid", "migration_head_rejected"}),
}


# Only producer-owned fixed categories may enter a public inventory receipt.
# An unknown private category is reduced to one closed token, never relayed.
INVENTORY_DIAGNOSTIC_ERRORS = {
    database: IDENTITY_ERRORS[database] | frozenset({
        "database_identity_mismatch", "metadata_bound_exceeded",
        "inventory_database_error_unrecognized",
    }) | (frozenset({"mongo_catalog_read_failed", "mongo_schema_decode_failed",
                    "mongo_index_visibility_incomplete"}) if database == "mongodb" else frozenset())
    for database in ("mysql", "mongodb")
}


def inventory_diagnostic_error_categories(bindings):
    return {database: (binding["error_category"]
                      if type(binding["error_category"]) is str and
                      binding["error_category"] in INVENTORY_DIAGNOSTIC_ERRORS[database]
                      else "inventory_database_error_unrecognized")
            for database, binding in bindings.items()}


# Existing boundary diagnostics intentionally keep their original allowlist.
# These additional literal categories belong to the Go v2 inventory producer
# (main.go/paging.go); no driver text or dynamically derived token is public.
INVENTORY_REPORT_SHARED_ERRORS = frozenset({
    "approved_boundary_missing", "approved_boundary_mismatch", "boundary_token_invalid",
    "target_type_rejected", "target_boundary_read_failed", "target_page_bound_exceeded",
    "target_record_bound_exceeded", "target_byte_bound_exceeded", "target_output_byte_bound_exceeded",
    "private_output_exists_or_unavailable", "private_output_failed", "source_asset_binding_missing",
})
INVENTORY_REPORT_DIAGNOSTIC_ERRORS = {
    "mysql": INVENTORY_DIAGNOSTIC_ERRORS["mysql"] | INVENTORY_REPORT_SHARED_ERRORS | frozenset({
        "target_primary_key_rejected", "target_primary_key_type_unsupported", "target_columns_unavailable",
        "target_read_failed_or_timed_out", "cursor_did_not_advance", "mysql_source_changed_during_scan",
        "mysql_migration_head_changed_during_scan", "mysql_schema_changed_during_scan",
    }),
    "mongodb": INVENTORY_DIAGNOSTIC_ERRORS["mongodb"] | INVENTORY_REPORT_SHARED_ERRORS | frozenset({
        "mongo_cursor_invalid", "mongo_id_type_unsupported", "mongo_id_type_proof_failed",
        "mongo_mixed_id_types_unsupported", "mongo_source_changed_during_bounds",
        "mongo_source_changed_during_scan", "mongo_target_read_failed_or_timed_out",
        "mongo_target_decode_failed", "mongo_target_collation_unsupported", "mongo_target_uuid_unavailable",
        "mongo_schema_changed_during_scan", "mongo_migration_head_changed_during_scan",
    }),
}


def inventory_report_error_categories(bindings):
    return {database: (binding["error_category"]
                      if type(binding["error_category"]) is str and
                      binding["error_category"] in INVENTORY_REPORT_DIAGNOSTIC_ERRORS[database]
                      else "inventory_database_error_unrecognized")
            for database, binding in bindings.items()}


class Blocked(ValueError):
    """Fixed error categories; never include private input in the message."""


class NativeReceiptBlocked(Blocked):
    """Actual child diagnostics contain only fixed scalars and a bounded digest."""
    def __init__(self, category, diagnostic):
        super().__init__(category)
        self.native_diagnostic = diagnostic


def fail(category):
    raise Blocked(category)


def fields(value, required, optional=()):
    if type(value) is not dict or set(value) != set(required) | (set(value) & set(optional)):
        fail("evidence_fields_invalid")


def uint(value):
    if type(value) is not int or not 0 <= value <= 2 ** 64 - 1:
        fail("evidence_type_invalid")


def token(value, pattern):
    if type(value) is not str or not pattern.fullmatch(value):
        fail("evidence_type_invalid")


def utc(value):
    if type(value) is not str or len(value) != 20 or not value.endswith("Z"):
        fail("evidence_time_invalid")
    try:
        parsed = datetime.datetime.strptime(value, "%Y-%m-%dT%H:%M:%SZ")
    except ValueError:
        fail("evidence_time_invalid")
    return parsed.replace(tzinfo=datetime.timezone.utc)


def unique(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            fail("evidence_duplicate_key")
        result[key] = value
    return result


def decode(raw):
    if not 0 < len(raw) <= MAX_JSON:
        fail("evidence_size_invalid")
    try:
        return json.loads(raw.decode("utf-8"), object_pairs_hook=unique,
                          parse_constant=lambda _: fail("evidence_json_invalid"))
    except Blocked:
        raise
    except (UnicodeError, ValueError, RecursionError):
        fail("evidence_json_invalid")


def private_directory(path):
    path = Path(path)
    if not path.is_absolute() or ".." in path.parts:
        fail("operation_path_invalid")
    # Every ancestor is inspected, including the root itself. Do not resolve a
    # symlink first and accidentally accept its destination instead.
    for component in reversed((path, *path.parents)):
        try:
            metadata = component.lstat()
        except OSError:
            fail("operation_directory_missing")
        if not stat.S_ISDIR(metadata.st_mode):
            fail("operation_path_invalid")
        if metadata.st_mode & 0o022 and not (metadata.st_mode & stat.S_ISVTX and metadata.st_uid in (0, os.getuid())):
            fail("operation_ancestor_writable")
    metadata = path.lstat()
    if metadata.st_uid != os.getuid() or stat.S_IMODE(metadata.st_mode) != 0o700:
        fail("operation_directory_not_private")
    return path


def operation_directory(root, operation_id):
    token(operation_id, RUN)
    root = Path(root)
    if tuple(root.parts[-3:]) != ROOT_SUFFIX:
        fail("operation_root_invalid")
    private_directory(root)
    return private_directory(root / operation_id)


def read_private(directory, filename, expected_hash=None):
    if filename not in ("identity.private.json", "inventory.private.json", "boundary.private.json") and filename not in ASSET_FILENAMES:
        token(filename, NAME)
    if expected_hash is not None:
        token(expected_hash, HASH)
    try:
        fd = os.open(directory / filename, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    except OSError:
        fail("evidence_unavailable")
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or before.st_uid != os.getuid() or stat.S_IMODE(before.st_mode) != 0o600 or before.st_nlink != 1:
            fail("evidence_not_private")
        if not 0 < before.st_size <= MAX_JSON:
            fail("evidence_size_invalid")
        with os.fdopen(fd, "rb", closefd=False) as stream:
            raw = stream.read(MAX_JSON + 1)
        after = os.fstat(fd)
        if (before.st_ino, before.st_dev, before.st_size, before.st_mtime_ns, before.st_ctime_ns) != (after.st_ino, after.st_dev, after.st_size, after.st_mtime_ns, after.st_ctime_ns):
            fail("evidence_changed")
        digest = hashlib.sha256(raw).hexdigest()
        if expected_hash is not None and digest != expected_hash:
            fail("evidence_hash_mismatch")
        return decode(raw), digest
    finally:
        os.close(fd)


def validate_binding(value, operation_id, source_sha):
    if value["operation_id"] != operation_id or value["source_sha"] != source_sha:
        fail("evidence_binding_mismatch")


def validate_manifest(value, operation_id, source_sha):
    fields(value, ("format_version", "operation_id", "source_sha", "target_hash", "database_bindings", "targets", "evidence", "maintenance"))
    if type(value["format_version"]) is not int or value["format_version"] != 1:
        fail("manifest_version_unsupported")
    token(value["operation_id"], RUN)
    token(value["source_sha"], SHA)
    validate_binding(value, operation_id, source_sha)
    if value["target_hash"] != TARGET_HASH:
        fail("target_allowlist_mismatch")
    bindings = value["database_bindings"]
    fields(bindings, ("mysql", "mongodb"))
    for binding in bindings.values():
        fields(binding, ("identity_hash", "migration_version", "migration_dirty", "catalog_hash", "non_target_schema_hash"))
        for key in ("identity_hash", "catalog_hash", "non_target_schema_hash"):
            token(binding[key], HASH)
        uint(binding["migration_version"])
        if type(binding["migration_dirty"]) is not bool or binding["migration_dirty"]:
            fail("database_dirty")
    if bindings["mysql"]["identity_hash"] == bindings["mongodb"]["identity_hash"]:
        fail("database_identity_invalid")
    snapshots = value["targets"]
    if type(snapshots) is not list or len(snapshots) != len(TARGETS):
        fail("target_allowlist_mismatch")
    for snapshot, expected in zip(snapshots, TARGETS):
        fields(snapshot, ("database", "name", "kind", "identity_hash", "schema_hash", "data_hash", "records"))
        if tuple(snapshot[key] for key in ("database", "name", "kind")) != expected:
            fail("target_allowlist_mismatch")
        for key in ("identity_hash", "schema_hash", "data_hash"):
            token(snapshot[key], HASH)
        uint(snapshot["records"])
    refs = value["evidence"]
    if type(refs) is not dict or set(refs) - PROOF_KINDS:
        fail("evidence_fields_invalid")
    filenames = set()
    for ref in refs.values():
        fields(ref, ("filename", "sha256"))
        token(ref["filename"], NAME)
        token(ref["sha256"], HASH)
        if ref["filename"] == "manifest.json" or ref["filename"] in filenames:
            fail("evidence_reference_invalid")
        filenames.add(ref["filename"])
    window = value["maintenance"]
    fields(window, ("max_seconds", "forward_stop_seconds", "rollback_seconds"))
    if window != {"max_seconds": MAX_WINDOW_SECONDS, "forward_stop_seconds": FORWARD_STOP_SECONDS,
                  "rollback_seconds": MAX_WINDOW_SECONDS - FORWARD_STOP_SECONDS}:
        fail("maintenance_budget_invalid")
    return value


def validate_proof(proof, kind, manifest, now):
    fields(proof, ("format_version", "kind", "operation_id", "source_sha", "target_hash", "producer", "complete", "observed_at", "valid_until", "summary"))
    if type(proof["format_version"]) is not int or proof["format_version"] != 1 or proof["kind"] != kind:
        fail("proof_protocol_unsupported")
    validate_binding(proof, manifest["operation_id"], manifest["source_sha"])
    if proof["target_hash"] != TARGET_HASH:
        fail("target_allowlist_mismatch")
    if type(proof["complete"]) is not bool or not proof["complete"]:
        fail("proof_incomplete")
    start, end = utc(proof["observed_at"]), utc(proof["valid_until"])
    if not start <= now <= end or end - start > datetime.timedelta(hours=24):
        fail("proof_expired")
    producer = proof["producer"]
    fields(producer, ("protocol", "source_sha", "run_id"))
    if producer["protocol"] != "qs_compatibility_retirement_" + kind + "_v1":
        fail("proof_protocol_unsupported")
    token(producer["source_sha"], SHA)
    token(producer["run_id"], RUN)
    if producer["source_sha"] != manifest["source_sha"]:
        fail("proof_producer_source_mismatch")
    summary = proof["summary"]
    # Deliberately reject unknown summaries rather than copy them into output.
    if kind == "inventory":
        fields(summary, ("count_semantics", "target_count", "identity_hashes", "snapshots_hash", "dependencies_complete", "outside_dependencies"))
        if summary["count_semantics"] != "exact" or type(summary["target_count"]) is not int or summary["target_count"] != 4:
            fail("inventory_not_exact")
        fields(summary["identity_hashes"], ("mysql", "mongodb"))
        for database in ("mysql", "mongodb"):
            if summary["identity_hashes"][database] != manifest["database_bindings"][database]["identity_hash"]:
                fail("evidence_binding_mismatch")
        wanted = hashlib.sha256(json.dumps(manifest["targets"], sort_keys=True, separators=(",", ":")).encode()).hexdigest()
        if summary["snapshots_hash"] != wanted:
            fail("snapshot_binding_mismatch")
        if type(summary["dependencies_complete"]) is not bool or not summary["dependencies_complete"]:
            fail("dependency_visibility_incomplete")
        uint(summary["outside_dependencies"])
        if summary["outside_dependencies"]:
            fail("outside_dependencies_present")
    elif kind == "history":
        keys = ("classified", "verified_live", "verified_retired", "unverifiable_closed", "unresolved", "ambiguous", "hash_conflicts", "unknown_execution", "unexplained_high", "retirement_references", "references_hash")
        fields(summary, keys)
        for key in keys[:-1]:
            uint(summary[key])
        token(summary["references_hash"], HASH)
        if summary["classified"] != sum(summary[key] for key in ("verified_live", "verified_retired", "unverifiable_closed")) or summary["retirement_references"] != summary["classified"]:
            fail("history_coverage_incomplete")
        if any(summary[key] for key in ("unresolved", "ambiguous", "hash_conflicts", "unknown_execution", "unexplained_high")):
            fail("history_blocked")
    elif kind == "fence":
        fields(summary, ("approved_main_sha", "original_workflows_disabled", "old_runs_terminal", "pending_environments_zero", "historical_rerun_denied_before_credentials", "snapshot_hash"))
        if summary["approved_main_sha"] != manifest["source_sha"]:
            fail("evidence_binding_mismatch")
        token(summary["snapshot_hash"], HASH)
        for key in ("original_workflows_disabled", "old_runs_terminal", "pending_environments_zero", "historical_rerun_denied_before_credentials"):
            if type(summary[key]) is not bool or not summary[key]:
                fail("production_fence_unproven")
    elif kind == "backup_restore":
        fields(summary, ("archive_hash", "restore_proof_hash", "restored_target_count", "source_identity_hashes", "restore_identity_hashes", "network_none", "no_published_ports", "complete_content_equal", "complete_schema_equal", "assets_registered"))
        token(summary["archive_hash"], HASH)
        token(summary["restore_proof_hash"], HASH)
        if type(summary["restored_target_count"]) is not int or summary["restored_target_count"] != 4:
            fail("restore_scope_incomplete")
        for key in ("source_identity_hashes", "restore_identity_hashes"):
            fields(summary[key], ("mysql", "mongodb"))
            for identity in summary[key].values():
                token(identity, HASH)
        for database in ("mysql", "mongodb"):
            if summary["source_identity_hashes"][database] != manifest["database_bindings"][database]["identity_hash"] or summary["restore_identity_hashes"][database] in summary["source_identity_hashes"].values():
                fail("restore_not_isolated")
        for key in ("network_none", "no_published_ports", "complete_content_equal", "complete_schema_equal", "assets_registered"):
            if type(summary[key]) is not bool or not summary[key]:
                fail("restore_proof_incomplete")
    elif kind == "release":
        fields(summary, ("application_a_sha", "release_b_sha", "safe_rollback_sha", "prepared_images_hash", "a_no_legacy_dependencies", "four_presence_states_tested", "rollback_dirty_head_tested", "rollback_seconds", "b_runtime_matches_a"))
        for key in ("application_a_sha", "release_b_sha", "safe_rollback_sha"):
            token(summary[key], SHA)
        token(summary["prepared_images_hash"], HASH)
        uint(summary["rollback_seconds"])
        if summary["release_b_sha"] != manifest["source_sha"] or summary["rollback_seconds"] > 600:
            fail("release_preparation_incomplete")
        for key in ("a_no_legacy_dependencies", "four_presence_states_tested", "rollback_dirty_head_tested", "b_runtime_matches_a"):
            if type(summary[key]) is not bool or not summary[key]:
                fail("release_preparation_incomplete")
    else:
        fields(summary, ("release_b_sha", "four_targets_absent", "heads_clean", "non_target_schema_equal", "retirement_references_complete", "standard_live_checks_passed", "missing_namespace_errors", "unexplained_high", "operation_ledger_hash"))
        if summary["release_b_sha"] != manifest["source_sha"]:
            fail("evidence_binding_mismatch")
        token(summary["operation_ledger_hash"], HASH)
        for key in ("four_targets_absent", "heads_clean", "non_target_schema_equal", "retirement_references_complete", "standard_live_checks_passed"):
            if type(summary[key]) is not bool or not summary[key]:
                fail("production_acceptance_incomplete")
        for key in ("missing_namespace_errors", "unexplained_high"):
            uint(summary[key])
            if summary[key]:
                fail("production_acceptance_incomplete")


def preparation(directory, manifest, now):
    blockers = []
    required = PROOF_KINDS - {"acceptance"}
    for kind in sorted(required):
        ref = manifest["evidence"].get(kind)
        if ref is None:
            blockers.append(kind + "_proof_missing")
            continue
        try:
            proof, _ = read_private(directory, ref["filename"], ref["sha256"])
            validate_proof(proof, kind, manifest, now)
        except Blocked as error:
            blockers.append(str(error))
    # A structurally valid imported claim is not a live, trusted verifier.
    # No CLI switch exists to bypass this capability barrier.
    blockers.extend(name + "_not_implemented" for name, ready in CAPABILITIES.items() if not ready)
    return sorted(set(blockers))


def validate_inventory_request(value, operation_id, source_sha):
    if type(value) is dict and value.get("format_version") == 2:
        validate_v2_request(value, operation_id, source_sha, boundary=False)
        return
    fields(value, ("format_version", "kind", "operation_id", "source_sha", "target_hash",
                   "database_scope", "identity_hashes", "expected_migrations", "limits"))
    if type(value["format_version"]) is not int or value["format_version"] != 1 or value["kind"] != "readonly_inventory_request":
        fail("inventory_request_class_invalid")
    validate_binding(value, operation_id, source_sha)
    if value["database_scope"] != "mysql-and-mongodb" or value["target_hash"] != TARGET_HASH:
        fail("target_allowlist_mismatch")
    fields(value["identity_hashes"], ("mysql", "mongodb"))
    fields(value["expected_migrations"], ("mysql", "mongodb"))
    for database in ("mysql", "mongodb"):
        token(value["identity_hashes"][database], HASH)
        uint(value["expected_migrations"][database])
        if not value["expected_migrations"][database]:
            fail("inventory_request_head_invalid")
    if value["identity_hashes"]["mysql"] == value["identity_hashes"]["mongodb"]:
        fail("database_identity_invalid")
    if value["limits"] != {"query_seconds": 15, "total_seconds": 180, "max_records": 100000, "max_bytes": 134217728}:
        fail("inventory_request_limits_invalid")


def validate_v2_request(value, operation_id, source_sha, *, boundary):
    core = ("format_version", "kind", "operation_id", "source_sha", "target_hash", "database_scope",
            "identity_hashes", "expected_migrations", "limits")
    fields(value, core if boundary else (*core, "boundary_run_id", "boundary_report_hash", "approved_boundaries"), ("mongodb_namespace_anchor",))
    if "mongodb_namespace_anchor" in value: validate_mongo_namespace_anchor(value["mongodb_namespace_anchor"])
    if type(value["format_version"]) is not int or value["format_version"] != 2 or value["kind"] != ("readonly_inventory_boundary_request" if boundary else "readonly_inventory_request"):
        fail("inventory_request_class_invalid")
    validate_binding(value, operation_id, source_sha)
    if value["target_hash"] != TARGET_HASH or value["database_scope"] != "mysql-and-mongodb":
        fail("target_allowlist_mismatch")
    fields(value["identity_hashes"], ("mysql", "mongodb")); fields(value["expected_migrations"], ("mysql", "mongodb"))
    for database in ("mysql", "mongodb"):
        token(value["identity_hashes"][database], HASH); uint(value["expected_migrations"][database])
        if not value["expected_migrations"][database]:
            fail("inventory_request_head_invalid")
    if value["identity_hashes"]["mysql"] == value["identity_hashes"]["mongodb"]:
        fail("database_identity_invalid")
    if value["limits"] != INVENTORY_V2_LIMITS or any(type(v) is not int for v in value["limits"].values()):
        fail("inventory_request_limits_invalid")
    if boundary:
        return
    token(value["boundary_run_id"], RUN); token(value["boundary_report_hash"], HASH)
    bounds = value["approved_boundaries"]
    if type(bounds) is not list or len(bounds) != 4:
        fail("approved_boundary_mismatch")
    for bound, target in zip(bounds, TARGETS):
        fields(bound, ("database", "name", "kind", "present", "empty", "pk_type", "upper_token", "schema_hash", "identity_hash"))
        if tuple(bound[key] for key in ("database", "name", "kind")) != target or type(bound["present"]) is not bool or type(bound["empty"]) is not bool:
            fail("approved_boundary_mismatch")
        for key in ("schema_hash", "identity_hash"):
            token(bound[key], HASH)
        if type(bound["upper_token"]) is not str or type(bound["pk_type"]) is not str:
            fail("boundary_token_invalid")
        if not bound["present"] or bound["empty"]:
            if bound["upper_token"] or (not bound["present"] and (bound["empty"] or bound["pk_type"])):
                fail("boundary_token_invalid")
            if bound["present"] and (bound["pk_type"] not in ({"uint64", "int64", "ascii_string"} if target[0] == "mysql" else {""})):
                fail("boundary_token_invalid")
            continue
        try:
            raw = base64.b64decode(bound["upper_token"], validate=True)
        except Exception:
            fail("boundary_token_invalid")
        if not 0 < len(raw) <= 1024 or base64.b64encode(raw).decode("ascii") != bound["upper_token"] or bound["pk_type"] not in ({"uint64", "int64", "ascii_string"} if target[0] == "mysql" else {"string", "objectId", "int", "long"}):
            fail("boundary_token_invalid")
        if target[0] == "mysql":
            if bound["pk_type"] == "ascii_string":
                if len(raw) > 128 or any(byte < 33 or byte > 126 for byte in raw):
                    fail("boundary_token_invalid")
            else:
                try:
                    text = raw.decode("ascii"); number = int(text)
                except Exception:
                    fail("boundary_token_invalid")
                if str(number) != text or not ((0 <= number <= 2**64-1) if bound["pk_type"] == "uint64" else (-2**63 <= number <= 2**63-1)):
                    fail("boundary_token_invalid")


MONGO_NAMESPACE_PROFILE = "selected_namespace_kept_uuids_v1"
MONGO_REPLICA_PROFILE = "replica_set_uuid_v1"
MONGO_KEPT_NAMES = ("answersheets", "interpret_report_artifacts", "interpretation_runs", "report_generations")


def _mongo_framed_hash(parts):
    digest = hashlib.sha256()
    for value in parts:
        if type(value) is not str: fail("database_anchor_invalid")
        try: raw = value.encode("utf-8", "strict")
        except UnicodeError: fail("database_anchor_invalid")
        digest.update(b"\x01" + len(raw).to_bytes(8, "big") + raw)
    return digest.hexdigest()


def validate_mongo_namespace_anchor(anchor):
    # Metadata DTO validation only. Actual before/after observations are made by
    # the native connection owner; accepting this shape is never execution proof.
    fields(anchor, ("kind", "hash", "endpoint_sha256", "database", "replica_set_name", "collections"))
    if anchor["kind"] != MONGO_NAMESPACE_PROFILE: fail("database_anchor_invalid")
    for key in ("hash", "endpoint_sha256"): token(anchor[key], HASH)
    for key in ("database", "replica_set_name"):
        value = anchor[key]
        if type(value) is not str or not value or any(ch in value for ch in ("\x00", "\r", "\n")): fail("database_anchor_invalid")
        try: encoded = value.encode("utf-8", "strict")
        except UnicodeError: fail("database_anchor_invalid")
        if len(encoded) > 128: fail("database_anchor_invalid")
    rows = anchor["collections"]
    if type(rows) is not list or len(rows) != len(MONGO_KEPT_NAMES): fail("database_anchor_invalid")
    parts = [anchor["kind"], anchor["endpoint_sha256"], anchor["database"], anchor["replica_set_name"]]
    seen = set()
    for name, row in zip(MONGO_KEPT_NAMES, rows):
        fields(row, ("name", "present", "uuid"))
        if row["name"] != name or type(row["present"]) is not bool or type(row["uuid"]) is not str: fail("database_anchor_invalid")
        if row["present"]:
            if not re.fullmatch(r"[0-9a-f]{32}", row["uuid"]) or row["uuid"] == "0" * 32 or row["uuid"] in seen: fail("database_anchor_invalid")
            seen.add(row["uuid"])
        elif row["uuid"] != "": fail("database_anchor_invalid")
        parts.extend((name, "present" if row["present"] else "absent", row["uuid"]))
    if not seen or _mongo_framed_hash(parts) != anchor["hash"]: fail("database_anchor_invalid")


def validate_mongo_namespace_approval(anchor):
    # A public kind/hash reference binds approval to an existing private
    # observation. It is not a complete v2 request or metadata authority.
    if type(anchor) is dict and set(anchor) == {"kind", "hash"}:
        if anchor["kind"] != MONGO_NAMESPACE_PROFILE: fail("database_anchor_invalid")
        token(anchor["hash"], HASH)
        return True
    validate_mongo_namespace_anchor(anchor)
    return False


def expand_approved_namespace_anchor(approval, binding):
    expected = approval.get("mongodb_namespace_anchor")
    if expected is None:
        validate_approved_namespace_anchor(approval, binding)
        return None
    reference = validate_mongo_namespace_approval(expected)
    observed = binding.get("namespace_anchor")
    if observed is None: fail("database_anchor_profile_mismatch")
    validate_mongo_namespace_anchor(observed)
    if reference:
        if observed["kind"] != expected["kind"] or observed["hash"] != expected["hash"] or binding["database_anchor_hash"] != expected["hash"]:
            fail("database_anchor_profile_mismatch")
    else:
        validate_approved_namespace_anchor(approval, binding)
    # Independent copy: never alias/mutate the approved descriptor or original
    # private report. The caller must FIRST validate the exact report chain.
    return decode(canonical_bytes(observed))


def validate_approved_namespace_anchor(request, binding):
    expected = request.get("mongodb_namespace_anchor")
    observed = binding.get("namespace_anchor")
    if expected is None:
        if observed is not None: fail("database_anchor_profile_mismatch")
        return
    validate_mongo_namespace_anchor(expected)
    if observed is None: fail("database_anchor_profile_mismatch")
    validate_mongo_namespace_anchor(observed)
    if observed != expected or binding["database_anchor_hash"] != expected["hash"]: fail("database_anchor_profile_mismatch")


def public_namespace_anchor(anchor):
    validate_mongo_namespace_anchor(anchor)
    return {"database_anchor_kind": MONGO_NAMESPACE_PROFILE,
            "database_anchor_uuid_set_sha256": hashlib.sha256(canonical_bytes(anchor["collections"])).hexdigest(),
            "database_anchor_kept_count": sum(row["present"] for row in anchor["collections"])}


INVENTORY_BINDING_FIELDS = ("identity_hash", "database_anchor_hash", "migration_generation_hash", "expected_identity_match", "migration_version", "migration_dirty", "expected_migration_match", "catalog_hash", "non_target_schema_hash", "metadata_complete", "permissions", "outside_dependencies", "dependency_coverage_complete", "inbound_foreign_key_coverage_complete", "dependency_scope", "dependency_text_review_required", "error_category")


def validate_database_anchors(database, state, complete):
    if "namespace_anchor" in state:
        if database != "mongodb": fail("database_anchor_invalid")
        validate_mongo_namespace_anchor(state["namespace_anchor"])
        if state["database_anchor_hash"] != state["namespace_anchor"]["hash"]: fail("database_anchor_invalid")
    # Anchors are observations, never derived from a request or a name here.
    for key in ("database_anchor_hash", "migration_generation_hash"):
        if type(state[key]) is not str:
            fail("database_anchor_invalid")
        if state[key]:
            token(state[key], HASH)
    if database == "mysql":
        if state["database_anchor_hash"] != state["identity_hash"] or state["migration_generation_hash"] != "":
            fail("database_anchor_invalid")
    elif complete and (not state["database_anchor_hash"] or not state["migration_generation_hash"]):
        fail("database_anchor_missing")


def validate_inventory_bindings(bindings, complete):
    fields(bindings, ("mysql", "mongodb"))
    for database, binding in bindings.items():
        fields(binding, INVENTORY_BINDING_FIELDS, ("namespace_anchor",))
        validate_database_anchors(database, binding, complete)


def validate_approved_boundary_file(request, directory):
    # This is a supplied approval, never a bootstrap from newly observed state.
    bounds_directory = private_directory(directory / ("bounds-" + request["boundary_run_id"]))
    observed, _ = read_private(bounds_directory,
                               "boundary.private.json", request["boundary_report_hash"])
    if observed.get("format_version") != 2 or observed.get("kind") != "readonly_inventory_boundaries" or observed.get("complete") is not True or observed.get("drop_ready") is not False or observed.get("diagnostic_only") is not True or any(observed.get(key) != request[key] for key in ("source_sha", "operation_id", "target_hash")) or observed.get("run_id") != request["boundary_run_id"]:
        fail("boundary_report_binding_invalid")
    objects = observed.get("targets")
    if type(objects) is not list or len(objects) != 4 or [item.get("boundary") for item in objects] != request["approved_boundaries"] or any(item.get("complete") is not True or item.get("error_category") != "none" for item in objects):
        fail("approved_boundary_mismatch")
    validate_inventory_bindings(observed.get("database_bindings"), True)
    validate_approved_namespace_anchor(request, observed["database_bindings"]["mongodb"])
    for database in ("mysql", "mongodb"):
        state = observed.get("database_bindings", {}).get(database, {})
        if state.get("identity_hash") != request["identity_hashes"][database] or state.get("migration_version") != request["expected_migrations"][database] or state.get("migration_dirty") is not False or any(state.get(key) is not True for key in ("metadata_complete", "expected_identity_match", "expected_migration_match")):
            fail("boundary_report_binding_invalid")


def validate_identity_request(value, operation_id, source_sha):
    fields(value, ("format_version", "kind", "operation_id", "source_sha", "target_hash", "database_scope", "identity_protocols", "limits"), ("mongo_anchor_profile",))
    if "mongo_anchor_profile" in value and value["mongo_anchor_profile"] not in (MONGO_NAMESPACE_PROFILE, MONGO_REPLICA_PROFILE): fail("identity_request_protocol_or_limits_invalid")
    if type(value["format_version"]) is not int or value["format_version"] != 1 or value["kind"] != "readonly_identity_discovery_request":
        fail("identity_request_class_invalid")
    validate_binding(value, operation_id, source_sha)
    if value["database_scope"] != "mysql-and-mongodb" or value["target_hash"] != TARGET_HASH:
        fail("target_allowlist_mismatch")
    if value["identity_protocols"] != {"mysql": "mysql_database_identity_v1", "mongodb": "mongodb_database_identity_v1"} or value["limits"] != {"query_seconds": 15, "total_seconds": 90}:
        fail("identity_request_protocol_or_limits_invalid")


def identity_request_bytes(operation_id, source_sha, anchor_profile=""):
    if anchor_profile not in ("", MONGO_NAMESPACE_PROFILE, MONGO_REPLICA_PROFILE): fail("identity_request_protocol_or_limits_invalid")
    token(operation_id, RUN); token(source_sha, SHA)
    value = {"format_version": 1, "kind": "readonly_identity_discovery_request",
             "operation_id": operation_id, "source_sha": source_sha,
             "target_hash": TARGET_HASH, "database_scope": "mysql-and-mongodb",
             "identity_protocols": {"mysql": "mysql_database_identity_v1", "mongodb": "mongodb_database_identity_v1"},
             "limits": {"query_seconds": 15, "total_seconds": 90}}
    if anchor_profile: value["mongo_anchor_profile"] = anchor_profile
    return json.dumps(value, sort_keys=True, separators=(",", ":"), allow_nan=False).encode("ascii") + b"\n"


def bootstrap_identity_request(args):
    # Request bytes contain no observed/expected identity and no credential.
    # The caller approves their exact hash independently before any directory
    # creation or connection. Never generate an inventory request from discovery.
    token(args.identity_request_hash, HASH)
    # Choose only among two exact pre-approved raw forms, before any connection.
    # No server error or newly observed state can select/fallback a profile.
    candidates = [identity_request_bytes(args.operation_id, args.actual_source_sha, profile)
                  for profile in ("", MONGO_NAMESPACE_PROFILE, MONGO_REPLICA_PROFILE)]
    matched = [raw for raw in candidates if hashlib.sha256(raw).hexdigest() == args.identity_request_hash]
    if len(matched) != 1: fail("identity_request_hash_mismatch")
    raw = matched[0]
    root = Path(args.root)
    if not root.is_absolute() or ".." in root.parts or tuple(root.parts[-3:]) != ROOT_SUFFIX:
        fail("operation_root_invalid")
    for path in reversed((root, *root.parents)):
        if path.exists() or path.is_symlink():
            info = path.lstat()
            if not stat.S_ISDIR(info.st_mode) or (info.st_mode & 0o022 and not (info.st_mode & stat.S_ISVTX and info.st_uid in (0, os.getuid()))):
                fail("operation_path_invalid")
            continue
        try:
            path.mkdir(mode=0o700)
        except PermissionError:
            # Only create new fixed production ancestors. mkdir is exclusive;
            # chown is never called for a pre-existing directory. Existing
            # inaccessible or unsafe ancestors remain an explicit blocker.
            if str(root) != "/opt/backups/qs-server/compatibility-retirement" or str(path) not in ("/opt/backups", "/opt/backups/qs-server", str(root)):
                fail("operation_directory_creation_failed")
            code, _ = capture_fixed(["sudo", "-n", "mkdir", "-m", "0700", str(path)], timeout=10)
            if code:
                fail("operation_directory_creation_failed")
            code, _ = capture_fixed(["sudo", "-n", "chown", str(os.getuid()) + ":" + str(os.getgid()), str(path)], timeout=10)
            if code:
                fail("operation_directory_creation_failed")
        except OSError:
            fail("operation_directory_creation_failed")
    private_directory(root)
    directory = root / args.operation_id
    try:
        directory.mkdir(mode=0o700)
    except FileExistsError:
        pass
    except OSError:
        fail("operation_directory_creation_failed")
    private_directory(directory)
    with locked_operation(directory):
        path = directory / "identity-request.json"
        try:
            fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        except FileExistsError:
            value, _ = read_private(directory, path.name, args.identity_request_hash)
            validate_identity_request(value, args.operation_id, args.actual_source_sha)
        except OSError:
            fail("identity_request_creation_failed")
        else:
            with os.fdopen(fd, "wb") as stream:
                stream.write(raw); stream.flush(); os.fsync(stream.fileno())
        for parent in (directory, root):
            fd = os.open(parent, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
            try:
                os.fsync(fd)
            finally:
                os.close(fd)
    return directory


def canonical_bytes(value):
    return (json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True,
                       allow_nan=False) + "\n").encode("ascii")


def validate_bootstrap_approval(args):
    text = getattr(args, "bootstrap_approval_json", "")
    approved_hash = getattr(args, "bootstrap_approval_hash", "")
    token(approved_hash, HASH)
    if type(text) is not str or not 0 < len(text) <= MAX_BOOTSTRAP_APPROVAL or not text.isascii():
        fail("bootstrap_approval_invalid")
    value = decode(text.encode("ascii"))
    raw = canonical_bytes(value)
    # The input is one canonical JSON line; its approved digest includes LF.
    if text.encode("ascii") != raw[:-1] or hashlib.sha256(raw).hexdigest() != approved_hash:
        fail("bootstrap_approval_hash_or_encoding_invalid")
    mode = args.prepare_mode
    core = ("format_version", "kind", "prepare_mode", "operation_id", "source_sha", "target_hash",
            "database_scope", "identity_report", "identity_hashes", "expected_migrations", "limits")
    fields(value, core if mode == "bootstrap-bounds" else (*core, "boundary_report"), ("mongodb_namespace_anchor",))
    namespace_ref = "mongodb_namespace_anchor" in value and validate_mongo_namespace_approval(value["mongodb_namespace_anchor"])
    if type(value["format_version"]) is not int or value["format_version"] != 1 or value["kind"] != "readonly_request_bootstrap_approval" or value["prepare_mode"] != mode:
        fail("bootstrap_approval_class_invalid")
    validate_binding(value, args.operation_id, args.actual_source_sha)
    request = {key: value[key] for key in ("operation_id", "source_sha", "target_hash", "database_scope", "identity_hashes", "expected_migrations", "limits")}
    if "mongodb_namespace_anchor" in value and not namespace_ref: request["mongodb_namespace_anchor"] = value["mongodb_namespace_anchor"]
    request.update(format_version=2, kind="readonly_inventory_boundary_request")
    validate_v2_request(request, args.operation_id, args.actual_source_sha, boundary=True)
    # JSON numbers must have the same exact representation in Node and Python.
    if any(head > 2**53-1 for head in value["expected_migrations"].values()):
        fail("bootstrap_approval_head_invalid")
    for key in (("identity_report",) if mode == "bootstrap-bounds" else ("identity_report", "boundary_report")):
        reference = value[key]
        fields(reference, ("run_id", "source_sha", "sha256"))
        token(reference["run_id"], RUN); token(reference["source_sha"], SHA); token(reference["sha256"], HASH)
        if reference["source_sha"] != args.actual_source_sha or reference["run_id"] == args.run_id:
            fail("bootstrap_report_origin_invalid")
    if mode == "bootstrap-inventory" and value["identity_report"]["run_id"] == value["boundary_report"]["run_id"]:
        fail("bootstrap_report_origin_invalid")
    return value, request


def bootstrap_identity_report(directory, value, args):
    reference = value["identity_report"]
    output = private_directory(directory / ("identity-" + reference["run_id"]))
    report, report_hash = read_private(output, "identity.private.json", reference["sha256"])
    profiles = (MONGO_NAMESPACE_PROFILE,) if "mongodb_namespace_anchor" in value else ("", MONGO_REPLICA_PROFILE)
    matched = [(profile, hashlib.sha256(identity_request_bytes(args.operation_id, args.actual_source_sha, profile)).hexdigest()) for profile in profiles if hashlib.sha256(identity_request_bytes(args.operation_id, args.actual_source_sha, profile)).hexdigest() == report.get("request_hash")]
    if len(matched) != 1: fail("bootstrap_identity_binding_mismatch")
    profile, expected_hash = matched[0]
    original_request, _ = read_private(directory, "identity-request.json", expected_hash)
    validate_identity_request(original_request, args.operation_id, args.actual_source_sha)
    original_args = argparse.Namespace(actual_source_sha=args.actual_source_sha, operation_id=args.operation_id,
                                       run_id=reference["run_id"])
    summary = dict(report, private_report_hash=report_hash)
    validate_identity_receipt(summary, 0, original_args, output, expected_hash, "0" * 64, profile)
    if report["complete"] is not True or any(item["complete"] is not True or item["error_category"] != "none" for item in report["diagnostic_histograms"]):
        fail("bootstrap_identity_report_incomplete")
    for database, state in report["database_states"].items():
        if state["identity_hash"] != value["identity_hashes"][database] or state["migration_version"] != value["expected_migrations"][database]:
            fail("bootstrap_identity_binding_mismatch")

    # Expand only AFTER original request/report/source/run, full identity
    # outcome, exact heads and complete diagnostic histograms were validated.
    return expand_approved_namespace_anchor(value, report["database_states"]["mongodb"])


def bootstrap_boundary_report(directory, value, request, args):
    reference = value["boundary_report"]
    output = private_directory(directory / ("bounds-" + reference["run_id"]))
    report, _ = read_private(output, "boundary.private.json", reference["sha256"])
    fields(report, ("format_version", "kind", "source_sha", "operation_id", "run_id", "request_hash", "target_hash", "observed_at", "complete", "drop_ready", "diagnostic_only", "error_category", "database_bindings", "targets", "source_bytes_protocol", "consistency_semantics"), ("boundary_report_hash",))
    if type(report["format_version"]) is not int or report["format_version"] != 2 or report["kind"] != "readonly_inventory_boundaries" or report["source_sha"] != args.actual_source_sha or report["operation_id"] != args.operation_id or report["run_id"] != reference["run_id"] or report["target_hash"] != TARGET_HASH or report["complete"] is not True or report["drop_ready"] is not False or report["diagnostic_only"] is not True or report["error_category"] != "none" or report.get("boundary_report_hash", "") != "" or report["source_bytes_protocol"] != "no_source_body_copy" or report["consistency_semantics"] != "diagnostic_upper_discovery_requires_independent_request_approval":
        fail("bootstrap_boundary_report_invalid")
    utc(report["observed_at"])
    token(report["request_hash"], HASH)
    original_request, _ = read_private(directory, "boundary-request.json", report["request_hash"])
    validate_v2_request(original_request, args.operation_id, args.actual_source_sha, boundary=True)
    if original_request != request:
        fail("bootstrap_boundary_request_mismatch")
    objects = report["targets"]
    if type(objects) is not list or len(objects) != 4:
        fail("bootstrap_boundary_report_invalid")
    boundaries = []
    for item, target in zip(objects, TARGETS):
        fields(item, ("database", "name", "kind", "present", "complete", "records", "bytes", "schema_hash", "data_hash", "identity_hash", "classification", "error_category", "equal_full_passes", "pages", "next_cycle_required", "boundary"))
        expected_data_hash = "" if item["present"] is True else hashlib.sha256(b"null").hexdigest()
        if tuple(item[key] for key in ("database", "name", "kind")) != target or type(item["present"]) is not bool or item["complete"] is not True or item["error_category"] != "none" or type(item["next_cycle_required"]) is not bool or item["next_cycle_required"] or item["classification"] != {} or item["data_hash"] != expected_data_hash:
            fail("bootstrap_boundary_target_invalid")
        for key in ("records", "bytes", "equal_full_passes", "pages"):
            uint(item[key])
            if item[key]:
                fail("bootstrap_boundary_copied_source_body")
        bound = item["boundary"]
        if type(bound) is not dict or any(bound.get(key) != item[key] for key in ("database", "name", "kind", "present", "schema_hash", "identity_hash")):
            fail("bootstrap_boundary_target_invalid")
        boundaries.append(bound)
    request.update(kind="readonly_inventory_request", boundary_run_id=reference["run_id"],
                   boundary_report_hash=reference["sha256"], approved_boundaries=boundaries)
    validate_v2_request(request, args.operation_id, args.actual_source_sha, boundary=False)
    validate_approved_boundary_file(request, directory)
    for binding in report["database_bindings"].values():
        fields(binding, INVENTORY_BINDING_FIELDS, ("namespace_anchor",))
        uint(binding["migration_version"]); uint(binding["outside_dependencies"])
        for key in ("identity_hash", "catalog_hash", "non_target_schema_hash"):
            token(binding[key], HASH)
        for key in ("expected_identity_match", "migration_dirty", "expected_migration_match", "metadata_complete", "dependency_coverage_complete", "inbound_foreign_key_coverage_complete", "dependency_text_review_required"):
            if type(binding[key]) is not bool:
                fail("bootstrap_boundary_report_invalid")
        if type(binding["permissions"]) is not dict or any(type(flag) is not bool for flag in binding["permissions"].values()) or type(binding["dependency_scope"]) is not str or binding["error_category"] != "none":
            fail("bootstrap_boundary_report_invalid")


def create_bootstrap_file(directory, filename, raw):
    """Publish complete bytes exclusively; interrupted partial files block retry."""
    partial = directory / (filename + ".bootstrap.partial")
    try:
        fd = os.open(partial, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    except OSError:
        fail("bootstrap_request_creation_incomplete")
    try:
        with os.fdopen(fd, "wb", closefd=False) as stream:
            stream.write(raw); stream.flush(); os.fsync(stream.fileno())
        os.close(fd)
        fd = None
        os.link(partial, directory / filename, follow_symlinks=False)
        partial.unlink()
        parent_fd = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            os.fsync(parent_fd)
        finally:
            os.close(parent_fd)
    except OSError:
        # Preserve the exact interruption for review; never resume/overwrite it.
        fail("bootstrap_request_creation_incomplete")
    finally:
        if fd is not None:
            os.close(fd)


def bootstrap_private_request(args):
    value, request = validate_bootstrap_approval(args)
    directory = operation_directory(args.root, args.operation_id)
    filename = "boundary-request.json" if args.prepare_mode == "bootstrap-bounds" else "inventory-request.json"
    registry_name = filename.removesuffix(".json") + "-bootstrap.json"
    with locked_operation(directory):
        namespace_anchor = bootstrap_identity_report(directory, value, args)
        if namespace_anchor is not None:
            request["mongodb_namespace_anchor"] = namespace_anchor
        validate_v2_request(request, args.operation_id, args.actual_source_sha, boundary=True)
        if args.prepare_mode == "bootstrap-inventory":
            bootstrap_boundary_report(directory, value, request, args)
        raw = canonical_bytes(request)
        request_hash = hashlib.sha256(raw).hexdigest()
        registration = {"format_version": 1, "kind": "private_request_bootstrap_binding", "source_sha": args.actual_source_sha,
                        "operation_id": args.operation_id, "created_run_id": args.run_id, "prepare_mode": args.prepare_mode,
                        "approval_sha256": args.bootstrap_approval_hash, "request_sha256": request_hash,
                        "identity_report": value["identity_report"], "boundary_report": value.get("boundary_report")}
        paths = [directory / name for name in (filename, registry_name)]
        if any((directory / (name + ".bootstrap.partial")).exists() or (directory / (name + ".bootstrap.partial")).is_symlink() for name in (filename, registry_name)):
            fail("bootstrap_request_creation_incomplete")
        present = [path.exists() or path.is_symlink() for path in paths]
        if any(present):
            if not all(present):
                fail("bootstrap_request_creation_incomplete")
            existing, _ = read_private(directory, filename, request_hash)
            recorded, _ = read_private(directory, registry_name)
            fields(recorded, registration.keys())
            token(recorded["created_run_id"], RUN)
            if recorded["created_run_id"] in {value[key]["run_id"] for key in ("identity_report", "boundary_report") if key in value}:
                fail("bootstrap_request_binding_mismatch")
            expected = dict(registration, created_run_id=recorded["created_run_id"])
            if existing != request or recorded != expected or canonical_bytes(recorded) != canonical_bytes(expected):
                fail("bootstrap_request_binding_mismatch")
            created_run = recorded["created_run_id"]
        else:
            create_bootstrap_file(directory, registry_name, canonical_bytes(registration))
            create_bootstrap_file(directory, filename, raw)
            created_run = args.run_id
    receipt = {"format_version": 1, "operation": "prepare", "prepare_mode": args.prepare_mode,
            "source_sha": args.actual_source_sha, "run_id": args.run_id, "operation_id": args.operation_id,
            "target_hash": TARGET_HASH, "target_count": 4, "complete": False, "execution_allowed": False,
            "diagnostic_only": True, "drop_ready": False, "request_bootstrap_complete": True,
            "bootstrap_approval_sha256": args.bootstrap_approval_hash, "derived_request_sha256": request_hash,
            "request_created_run_id": created_run, "approved_identity_report": value["identity_report"],
            "error_category": "request_bootstrap_requires_independent_request_approval"}
    if "boundary_report" in value:
        receipt["approved_boundary_report"] = value["boundary_report"]
    return receipt


MONGO_INDEX_DIAGNOSTIC = re.compile(
    rb"QS_MONGO_INDEX_DIAGNOSTIC phase=(?:list|iterate|close) "
    rb"kind=(?:context_deadline|context_cancelled|server|network_timeout|other) "
    rb"code=(?:0|-?[1-9][0-9]{0,9}) namespace_sha256=[0-9a-f]{64} "
    rb"elapsed_ms=(?:0|[1-9][0-9]{0,15})")


def _forward_mongo_index_diagnostic(raw):
    # Only the immutable inventory caller enables this path. Unknown stderr
    # stays in the private temporary fd and is never put in logs or receipts.
    if len(raw) > 8192:
        return
    lines = []
    for line in raw.split(b"\n")[:-1]:
        if not MONGO_INDEX_DIAGNOSTIC.fullmatch(line):
            continue
        fields = dict(token.split(b"=", 1) for token in line.split(b" ")[1:])
        if not -(1 << 31) <= int(fields[b"code"]) < (1 << 31) or int(fields[b"elapsed_ms"]) > (1 << 63) - 1:
            continue
        lines.append(line)
    # One failed catalog call emits one diagnostic. Multiple candidate lines
    # are ambiguous, and none may be adopted as the actual failure.
    if len(lines) == 1:
        print(lines[0].decode("ascii"), file=sys.stderr)


MONGO_TARGET_DIAGNOSTIC = re.compile(
    rb"QS_MONGO_TARGET_DIAGNOSTIC phase=(?:find|iterate|close) "
    rb"kind=(?:context_deadline|context_cancelled|server|network_timeout|other) "
    rb"code=(?:0|-?[1-9][0-9]{0,9}) "
    rb"run_ctx=(?:active|deadline|cancelled|other) page_ctx=(?:active|deadline|cancelled|other) "
    rb"pass=[12] page=[1-9][0-9]{0,3} records=(?:0|[1-9][0-9]{0,6}) "
    rb"source_bytes=(?:0|[1-9][0-9]{0,9}) "
    rb"page_elapsed_ms=(?:0|[1-9][0-9]{0,15}) "
    rb"run_elapsed_ms=(?:0|[1-9][0-9]{0,15}) "
    rb"mysql_elapsed_ms=(?:0|[1-9][0-9]{0,15})")


def _forward_mongo_target_diagnostic(raw):
    if len(raw) > 8192:
        return
    lines = []
    for line in raw.split(b"\n")[:-1]:
        if not MONGO_TARGET_DIAGNOSTIC.fullmatch(line):
            continue
        fields = dict(token.split(b"=", 1) for token in line.split(b" ")[1:])
        numbers = {key: int(fields[key]) for key in
                   (b"code", b"page", b"records", b"source_bytes", b"page_elapsed_ms", b"run_elapsed_ms", b"mysql_elapsed_ms")}
        if (not -(1 << 31) <= numbers[b"code"] < (1 << 31) or numbers[b"page"] > 1001 or
                numbers[b"records"] > 1000000 or numbers[b"source_bytes"] > 2 << 30 or
                numbers[b"run_elapsed_ms"] > (1 << 63) - 1 or
                numbers[b"page_elapsed_ms"] > numbers[b"run_elapsed_ms"] or
                numbers[b"mysql_elapsed_ms"] > numbers[b"run_elapsed_ms"]):
            continue
        lines.append(line)
    # A pagination failure has one primary phase. Separate index diagnostics
    # and unrecognized stderr never become a target-failure classification.
    if len(lines) == 1:
        print(lines[0].decode("ascii"), file=sys.stderr)


def _forward_inventory_diagnostic_fd(private_stderr):
    size = os.fstat(private_stderr.fileno()).st_size
    if size <= 8192:
        private_stderr.seek(0)
        raw = private_stderr.read(8193)
        _forward_mongo_index_diagnostic(raw)
    else:
        # Keep only a bounded tail. Inspect its preceding byte so a truncated
        # private line cannot acquire a valid marker merely at the cutoff.
        private_stderr.seek(size - 8193)
        tail = private_stderr.read(8193)
        raw = tail[1:] if tail[:1] == b"\n" else tail[1:].partition(b"\n")[2]
    _forward_mongo_target_diagnostic(raw)


def capture_fixed(command, *, timeout, maximum=32768, mongo_index_diagnostics=False):
    # Raw child errors can contain connection strings. Capturing into a
    # private temporary fd avoids retaining an unbounded stderr PIPE in RAM.
    # The normal path continues to discard stderr; no other mode opts in.
    try:
        with (tempfile.TemporaryFile() if mongo_index_diagnostics else contextlib.nullcontext()) as private_stderr:
            result = subprocess.run(command, stdout=subprocess.PIPE,
                                    stderr=private_stderr if private_stderr is not None else subprocess.DEVNULL,
                                    timeout=timeout, check=False)
            if private_stderr is not None:
                _forward_inventory_diagnostic_fd(private_stderr)
    except (OSError, subprocess.TimeoutExpired):
        fail("inventory_runtime_failed")
    if len(result.stdout) > maximum:
        fail("inventory_output_bound_exceeded")
    return result.returncode, result.stdout


def inventory_connection_values(environment):
    # Select the Mongo pair together. An incomplete metadata pair must never
    # combine an administrator username with an application password.
    keys = ("MYSQL_HOST", "MYSQL_PORT", "MYSQL_USERNAME", "MYSQL_PASSWORD", "MYSQL_DATABASE",
            "MONGODB_HOST", "MONGODB_PORT", "MONGODB_USERNAME", "MONGODB_PASSWORD", "MONGODB_DBNAME")
    mongo_pair = tuple(environment.get(key, "") for key in
                       ("MONGODB_METADATA_ADMIN_USERNAME", "MONGODB_METADATA_ADMIN_PASSWORD"))
    if any(mongo_pair) and not all(mongo_pair):
        fail("inventory_connection_input_invalid")
    selected = {key: environment.get(key, "") for key in keys}
    if all(mongo_pair):
        selected["MONGODB_USERNAME"], selected["MONGODB_PASSWORD"] = mongo_pair
    values = {}
    for key in keys:
        value = selected.get(key, "") or ({"MYSQL_PORT": "3306", "MONGODB_PORT": "27017"}.get(key, ""))
        if not isinstance(value, str) or not value or len(value) > 4096 or any(character in value for character in ("\n", "\r", "\x00")):
            fail("inventory_connection_input_invalid")
        values[key] = value
    return values


def live_inventory(args, directory):
    mode = getattr(args, "prepare_mode", "inventory")
    request_hash = args.identity_request_hash if mode == "identity" else args.inventory_request_hash
    request_name = {"identity": "identity-request.json", "bounds": "boundary-request.json", "inventory": "inventory-request.json"}[mode]
    token(request_hash, HASH)
    request, _ = read_private(directory, request_name, request_hash)
    if mode != "identity":
        require_inventory_v2(request)
    if mode == "bounds":
        validate_v2_request(request, args.operation_id, args.actual_source_sha, boundary=True)
    else:
        validator = validate_identity_request if mode == "identity" else validate_inventory_request
        validator(request, args.operation_id, args.actual_source_sha)
    if mode == "inventory" and request["format_version"] == 2:
        validate_approved_boundary_file(request, directory)
    entrypoints, entrypoint_hash = read_private(Path(__file__).parent, "compatibility-retirement-entrypoints.json")
    if entrypoints.get("format_version") != 1 or entrypoints.get("kind") != "source_only_production_entrypoint_catalog" or entrypoints.get("live_fence_proven") is not False or entrypoints.get("historical_rerun_proven_denied") is not False or len(entrypoints.get("entrypoints", ())) != 13 or entrypoints.get("current_source_entrypoint_workflow_total") != 13:
        fail("inventory_entrypoint_catalog_invalid")
    binary = Path(args.inventory_binary)
    try:
        info = binary.lstat()
    except OSError:
        fail("inventory_binary_unavailable")
    if not binary.is_absolute() or not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o700:
        fail("inventory_binary_invalid")
    code, raw = capture_fixed([str(binary), "--source-sha"], timeout=5, maximum=128)
    if code or raw.decode("ascii", errors="ignore").strip() != args.actual_source_sha:
        fail("inventory_binary_source_mismatch")
    values = inventory_connection_values(os.environ)
    keys = tuple(values)
    docker = ["sudo", "-n", "docker"]
    code, raw = capture_fixed([*docker, "image", "inspect", "mysql:8.0", "--format", "{{.Id}}"], timeout=15, maximum=256)
    image = raw.decode("ascii", errors="ignore").strip()
    if code or not re.fullmatch(r"sha256:[0-9a-f]{64}", image):
        fail("inventory_runtime_image_unavailable")
    code, _ = capture_fixed([*docker, "network", "inspect", "infra-network"], timeout=15)
    if code:
        fail("inventory_network_unavailable")
    output = directory / (mode + "-" + args.run_id)
    name = "qs-compatibility-inventory-" + args.run_id
    with locked_operation(directory):
        try:
            output.mkdir(mode=0o700)
        except OSError:
            fail("inventory_run_directory_exists_or_unavailable")
        parent = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            os.fsync(parent)
        finally:
            os.close(parent)
        entrypoint_envelope = {"format_version": 1, "kind": "source_only_production_entrypoint_catalog",
                               "source_sha": args.actual_source_sha, "operation_id": args.operation_id,
                               "run_id": args.run_id, "request_hash": request_hash,
                               "catalog_hash": entrypoint_hash, "live_fence_proven": False,
                               "catalog": entrypoints}
        fd = os.open(output / "entrypoints.private.json", os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, "w") as stream:
            json.dump(entrypoint_envelope, stream, sort_keys=True, separators=(",", ":"), allow_nan=False)
            stream.write("\n"); stream.flush(); os.fsync(stream.fileno())
        with tempfile.TemporaryDirectory(prefix="qs-compatibility-env-") as temporary:
            env_file = Path(temporary) / "inventory.env"
            os.chmod(temporary, 0o700)
            fd = os.open(env_file, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
            with os.fdopen(fd, "w") as stream:
                stream.write("".join(key + "=" + values[key] + "\n" for key in keys))
                stream.flush(); os.fsync(stream.fileno())
            command = [*docker, "run", "--rm", "--name", name, "--pull=never", "--network", "infra-network",
                       "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--cpus=2",
                       "--memory=512m", "--pids-limit=64", "--user", str(os.getuid()) + ":" + str(os.getgid()),
                       "--label", "qs.compatibility-retirement.operation=" + args.operation_id,
                       "--label", "qs.compatibility-retirement.run=" + args.run_id,
                       "--label", "qs.compatibility-retirement.source=" + args.actual_source_sha,
                       "--label", "qs.compatibility-retirement.request=" + request_hash,
                       "--mount", "type=bind,source=" + str(binary) + ",target=/inventory-tool,readonly",
                       "--mount", "type=bind,source=" + str(directory) + ",target=/operation,readonly",
                       "--mount", "type=bind,source=" + str(output) + ",target=/inventory",
                       "--env-file", str(env_file), "--entrypoint", "/inventory-tool", image,
                       "--mode", mode, "--request", "/operation/" + request_name, "--request-hash", request_hash,
                       "--operation-id", args.operation_id, "--run-id", args.run_id, "--output-directory", "/inventory"]
            # Only prospective exact name/labels are supplied here; no actual
            # container ID is observed. A timeout must first reconcile live
            # ID/labels/image/mounts by read-only inspection, without automatic
            # removal or retry of a potentially pre-existing container.
            code, raw = capture_fixed(command, timeout=request["limits"]["total_seconds"] + 30, maximum=MAX_JSON,
                                      mongo_index_diagnostics=mode in ("bounds", "inventory"))
    summary = decode(raw)
    if mode == "identity":
        receipt = validate_identity_receipt(summary, code, args, output, request_hash, entrypoint_hash, request.get("mongo_anchor_profile", ""))
        receipt["runtime_image_id_sha256"] = image.removeprefix("sha256:")
        receipt["runtime_network"] = "infra_network"
        return receipt
    return validate_inventory_receipt(summary, code, args, output, request, request_hash, entrypoint_hash, image)


def require_inventory_v2(request):
    # V1 is parseable only for historical fixtures. Production preparation must
    # use the approved-bound/page protocol; no legacy entrypoint can bypass it.
    if type(request.get("format_version")) is not int or request["format_version"] != 2:
        fail("inventory_v1_retired")


def validate_inventory_receipt(summary, code, args, output, request, request_hash, entrypoint_hash, image):
    mode = getattr(args, "prepare_mode", "inventory")
    fields(summary, ("format_version", "kind", "source_sha", "operation_id", "run_id", "request_hash",
                     "target_hash", "complete", "drop_ready", "diagnostic_only", "boundary_report_hash", "error_category", "database_bindings", "targets", "private_report_hash"))
    if summary["format_version"] != request["format_version"] or summary["kind"] != ("readonly_inventory_boundaries" if mode == "bounds" else "readonly_compatibility_inventory") or summary["source_sha"] != args.actual_source_sha or summary["operation_id"] != args.operation_id or summary["run_id"] != args.run_id or summary["request_hash"] != args.inventory_request_hash or summary["target_hash"] != TARGET_HASH or summary["drop_ready"] is not False or summary["diagnostic_only"] is not True or summary["boundary_report_hash"] != request.get("boundary_report_hash", ""):
        fail("inventory_receipt_binding_mismatch")
    report, report_hash = read_private(output, "boundary.private.json" if mode == "bounds" else "inventory.private.json")
    if summary["private_report_hash"] != report_hash:
        fail("inventory_receipt_private_mismatch")
    # Bind the exact private file and every public field; never trust stdout's
    # complete flag alone. Source bodies are never copied into the receipt.
    for key in ("format_version", "kind", "source_sha", "operation_id", "run_id", "request_hash", "target_hash", "complete", "drop_ready", "diagnostic_only", "error_category", "database_bindings"):
        if report.get(key) != summary[key]:
            fail("inventory_receipt_private_mismatch")
    if report.get("boundary_report_hash", "") != summary["boundary_report_hash"]:
        fail("inventory_receipt_private_mismatch")
    if type(summary["complete"]) is not bool or summary["complete"] != (code == 0) or (summary["complete"] and summary["error_category"] != "none"):
        fail("inventory_receipt_outcome_invalid")
    objects = report.get("targets")
    if type(objects) is not list or len(objects) > 4:
        fail("inventory_receipt_target_invalid")
    if summary["complete"] and len(objects) != 4:
        fail("inventory_receipt_target_invalid")
    identities = set()
    if type(summary["targets"]) is not list or len(summary["targets"]) != len(objects):
        fail("inventory_receipt_target_invalid")
    for item, public in zip(objects, summary["targets"]):
        expected = next((target for target in TARGETS if target[:2] == (item.get("database"), item.get("name"))), None)
        if expected is None or item.get("kind") != expected[2]:
            fail("inventory_receipt_target_invalid")
        identity = expected[:2]
        if identity in identities:
            fail("inventory_receipt_target_invalid")
        identities.add(identity)
        public_keys = ("database", "name", "present", "complete", "records", "bytes", "schema_hash", "data_hash", "identity_hash", "classification", "error_category", "equal_full_passes", "pages", "next_cycle_required")
        fields(public, (*public_keys, "boundary_hash"))
        if any(public[key] != item.get(key) for key in public_keys):
            fail("inventory_receipt_private_mismatch")
        boundary_json = json.dumps(item.get("boundary"), separators=(",", ":"), ensure_ascii=False).encode()
        if public["boundary_hash"] != hashlib.sha256(boundary_json).hexdigest():
            fail("inventory_receipt_private_mismatch")
        for key in ("records", "bytes"):
            uint(item.get(key))
        if type(item.get("present")) is not bool or type(item.get("complete")) is not bool:
            fail("inventory_receipt_target_invalid")
        if summary["complete"]:
            if not item["complete"] or item["error_category"] != "none":
                fail("inventory_receipt_outcome_invalid")
            for key in (("schema_hash", "identity_hash") if mode == "bounds" else ("schema_hash", "data_hash", "identity_hash")):
                token(item.get(key), HASH)
            if request["format_version"] == 2 and mode == "inventory":
                if item.get("equal_full_passes") != 2 or item.get("boundary") != request["approved_boundaries"][len(identities)-1]:
                    fail("inventory_receipt_boundary_invalid")
                if item["records"] > request["limits"]["max_records"] or item["bytes"] > request["limits"]["max_bytes"]:
                    fail("inventory_receipt_limit_invalid")
                if item["present"]:
                    validate_source_asset(output, item, args, request_hash, request["limits"]["max_bytes"])
                elif item["records"] or item["bytes"] or item.get("source_file"):
                    fail("inventory_receipt_target_invalid")
            elif mode == "bounds" and (item["records"] or item["bytes"] or item.get("source_file") or item.get("equal_full_passes") or item.get("pages")):
                fail("boundary_receipt_copied_source_body")
    validate_inventory_bindings(report["database_bindings"], summary["complete"])
    if summary["complete"]:
        validate_approved_namespace_anchor(request, report["database_bindings"]["mongodb"])
        for database, binding in report["database_bindings"].items():
            if binding.get("identity_hash") != request["identity_hashes"][database] or binding.get("migration_version") != request["expected_migrations"][database] or binding.get("migration_dirty") is not False or binding.get("metadata_complete") is not True or binding.get("expected_identity_match") is not True or binding.get("expected_migration_match") is not True:
                fail("inventory_receipt_database_invalid")
    database_states = {}
    for database in ("mysql", "mongodb"):
        binding = report["database_bindings"].get(database, {})
        identity = binding.get("identity_hash", "")
        if identity:
            token(identity, HASH)
        version = binding.get("migration_version", 0)
        uint(version)
        if "namespace_anchor" in binding: validate_mongo_namespace_anchor(binding["namespace_anchor"])
        database_states[database] = {"identity_hash": identity or None,
                                     "database_anchor_hash": binding["database_anchor_hash"] or None,
                                     "migration_generation_hash": binding["migration_generation_hash"] or None,
                                     "migration_version": version,
                                     "migration_head_observed": version > 0,
                                     "migration_dirty": binding.get("migration_dirty") if version > 0 else None,
                                     "metadata_complete": binding.get("metadata_complete") is True,
                                     "identity_match": binding.get("expected_identity_match") is True}
        if "namespace_anchor" in binding: database_states[database].update(public_namespace_anchor(binding["namespace_anchor"]))
    database_errors = inventory_diagnostic_error_categories(report["database_bindings"])
    if mode == "bounds":
        return {"format_version": 1, "operation": "prepare", "prepare_mode": "bounds", "source_sha": args.actual_source_sha,
                "run_id": args.run_id, "operation_id": args.operation_id, "target_hash": TARGET_HASH, "target_count": 4,
                "complete": False, "execution_allowed": False, "diagnostic_only": True, "drop_ready": False,
                "boundary_discovery_complete": summary["complete"], "boundary_private_report_hash": report_hash,
                "boundary_request_hash": request_hash, "inventory_entrypoint_catalog_hash": entrypoint_hash,
                "runtime_image_id_sha256": image.removeprefix("sha256:"), "runtime_network": "infra_network",
                "inventory_database_states": database_states, "inventory_database_error_categories": database_errors,
                "error_category": "boundary_discovery_requires_independent_approval"}
    return {"format_version": 1, "operation": "prepare", "source_sha": args.actual_source_sha,
            "run_id": args.run_id, "operation_id": args.operation_id, "target_hash": TARGET_HASH,
            "target_count": 4, "complete": False, "execution_allowed": False,
            "error_category": "preparation_gate_not_ready", "inventory_complete": summary["complete"],
            "inventory_private_report_hash": report_hash,
            "inventory_entrypoint_catalog_hash": entrypoint_hash,
            "runtime_image_id_sha256": image.removeprefix("sha256:"), "runtime_network": "infra_network",
            "inventory_present_targets": sum(item["present"] for item in objects),
            "inventory_records": sum(item["records"] for item in objects),
            "inventory_source_bytes": sum(item["bytes"] for item in objects),
            "inventory_next_cycle_required": any(item.get("next_cycle_required") is True for item in objects),
            "inventory_boundary_report_hash": request.get("boundary_report_hash") or None,
            "inventory_two_equal_scans": request["format_version"] == 2 and summary["complete"],
            "diagnostic_only": True, "drop_ready": False,
            "inventory_database_states": database_states, "inventory_database_error_categories": database_errors,
            "blockers": ["history_verifier_not_implemented", "production_fence_unproven", "backup_restore_backend_not_implemented",
                         "prepared_release_backend_not_implemented", "live_acceptance_verifier_not_implemented"],
            "capabilities": CAPABILITIES.copy()}


def validate_source_asset(output, item, args, request_hash, maximum):
    filename = SOURCE_FILENAMES[(item["database"], item["name"])]
    if item.get("source_file") != filename:
        fail("inventory_source_asset_invalid")
    try:
        fd = os.open(output / filename, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    except OSError:
        fail("inventory_source_asset_unavailable")
    try:
        st = os.fstat(fd)
        if not stat.S_ISREG(st.st_mode) or st.st_uid != os.getuid() or stat.S_IMODE(st.st_mode) != 0o600 or st.st_nlink != 1 or not 0 <= st.st_size <= maximum or (item["database"] == "mysql" and not st.st_size):
            fail("inventory_source_asset_invalid")
    finally:
        os.close(fd)
    registry, _ = read_private(output, filename + ".asset.json")
    expected = {"format_version": 1, "kind": "temporary_inventory_source_copy", "filename": filename,
                "source_sha": args.actual_source_sha, "operation_id": args.operation_id, "run_id": args.run_id,
                "request_hash": request_hash, "protocol": ("mysql_cast_binary_columns_pk_order_v2" if item["database"] == "mysql" else "mongodb_server_bson_pk_order_v2"),
                "boundary": item["boundary"], "contains_original_body": True, "retirement_proof": False,
                "purge_required_after_acceptance": True, "resume_existing_file_allowed": False}
    if registry != expected:
        fail("inventory_source_asset_binding_invalid")


def validate_identity_receipt(summary, code, args, output, request_hash, entrypoint_hash, anchor_profile=""):
    if anchor_profile not in ("", MONGO_REPLICA_PROFILE, MONGO_NAMESPACE_PROFILE): fail("database_anchor_profile_mismatch")
    keys = ("format_version", "kind", "source_sha", "operation_id", "run_id", "request_hash", "target_hash", "diagnostic_only", "drop_ready", "complete", "identity_protocols", "database_states", "diagnostic_histograms", "error_category")
    fields(summary, (*keys, "private_report_hash"))
    report, digest = read_private(output, "identity.private.json", summary["private_report_hash"])
    fields(report, keys)
    if any(report.get(key) != summary[key] for key in keys):
        fail("identity_receipt_private_mismatch")
    if summary["format_version"] != 1 or summary["kind"] != "readonly_identity_discovery" or summary["source_sha"] != args.actual_source_sha or summary["operation_id"] != args.operation_id or summary["run_id"] != args.run_id or summary["request_hash"] != request_hash or summary["target_hash"] != TARGET_HASH or summary["diagnostic_only"] is not True or summary["drop_ready"] is not False or summary["identity_protocols"] != {"mysql": "mysql_database_identity_v1", "mongodb": "mongodb_database_identity_v1"}:
        fail("identity_receipt_binding_mismatch")
    if type(summary["complete"]) is not bool or summary["complete"] != (code == 0):
        fail("identity_receipt_outcome_invalid")
    fields(summary["database_states"], ("mysql", "mongodb"))
    clean_states = {}
    for database, state in summary["database_states"].items():
        fields(state, ("identity_hash", "database_anchor_hash", "migration_generation_hash", "identity_observed", "migration_version", "migration_head_observed", "migration_dirty", "migration_clean", "metadata_permissions_sufficient", "permission_scope", "error_category"), ("namespace_anchor",))
        if database == "mongodb" and ((anchor_profile != MONGO_NAMESPACE_PROFILE and "namespace_anchor" in state) or (summary["complete"] and anchor_profile == MONGO_NAMESPACE_PROFILE and "namespace_anchor" not in state)): fail("database_anchor_profile_mismatch")
        validate_database_anchors(database, state, summary["complete"])
        if state["permission_scope"] != "identity_and_migration_head":
            fail("identity_receipt_permission_scope_invalid")
        for key in ("identity_observed", "migration_head_observed", "migration_clean", "metadata_permissions_sufficient"):
            if type(state[key]) is not bool:
                fail("identity_receipt_type_invalid")
        uint(state["migration_version"])
        if state["identity_hash"]:
            token(state["identity_hash"], HASH)
        if state["migration_dirty"] is not None and type(state["migration_dirty"]) is not bool:
            fail("identity_receipt_type_invalid")
        if summary["complete"] and (not all(state[key] for key in ("identity_observed", "migration_head_observed", "migration_clean", "metadata_permissions_sufficient")) or state["migration_dirty"] is not False or not state["identity_hash"] or state["error_category"] != "none"):
            fail("identity_receipt_outcome_invalid")
        if type(state["error_category"]) is not str or state["error_category"] not in IDENTITY_ERRORS[database]:
            fail("identity_error_category_invalid")
        clean_states[database] = {key: item for key, item in state.items() if key != "namespace_anchor"}
        if "namespace_anchor" in state: clean_states[database].update(public_namespace_anchor(state["namespace_anchor"]))
        clean_states[database]["identity_hash"] = state["identity_hash"] or None
        clean_states[database]["database_anchor_hash"] = state["database_anchor_hash"] or None
        clean_states[database]["migration_generation_hash"] = state["migration_generation_hash"] or None
    if summary["complete"] and summary["error_category"] != "none":
        fail("identity_receipt_outcome_invalid")
    histograms = summary["diagnostic_histograms"]
    if type(histograms) is not list or len(histograms) != 4:
        fail("identity_histogram_target_invalid")
    public_histograms = []
    public_buckets = []
    total_buckets = sum(len(item.get("buckets", ())) for item in histograms)
    for object_index, (histogram, target) in enumerate(zip(histograms, TARGETS)):
        fields(histogram, ("database", "name", "present", "complete", "buckets", "error_category", "diagnostic_only"))
        if (histogram["database"], histogram["name"]) != target[:2] or histogram["diagnostic_only"] is not True or type(histogram["complete"]) is not bool or (histogram["present"] is not None and type(histogram["present"]) is not bool) or type(histogram["buckets"]) is not list or len(histogram["buckets"]) > 128 or histogram["error_category"] not in HISTOGRAM_ERRORS:
            fail("identity_histogram_target_invalid")
        if histogram["complete"] and (histogram["present"] is None or histogram["error_category"] != "none"):
            fail("identity_histogram_outcome_invalid")
        if histogram["present"] is False and histogram["buckets"]:
            fail("identity_histogram_outcome_invalid")
        for bucket in histogram["buckets"]:
            fields(bucket, ("type_label", "type_hash", "state_label", "state_hash", "records"))
            if bucket["type_label"] not in HISTOGRAM_TYPES or bucket["state_label"] not in HISTOGRAM_STATES:
                fail("identity_histogram_label_invalid")
            token(bucket["type_hash"], HASH); token(bucket["state_hash"], HASH); uint(bucket["records"])
        public = {key: histogram[key] for key in histogram if key != "buckets"}
        public["bucket_count"] = len(histogram["buckets"])
        if total_buckets > 128:
            public["complete"] = False
            public["error_category"] = "histogram_public_bound_exceeded"
        else:
            for bucket_index, bucket in enumerate(histogram["buckets"]):
                public_buckets.append(dict(bucket, object_index=object_index, bucket_index=bucket_index,
                                           type_label=bucket["type_label"].replace(".", "_")))
        public_histograms.append(public)
    bucket_pages = {"page_" + chr(97 + offset // 32): public_buckets[offset:offset + 32] for offset in range(0, len(public_buckets), 32)}
    return {"format_version": 1, "operation": "prepare", "prepare_mode": "identity", "source_sha": args.actual_source_sha,
            "run_id": args.run_id, "operation_id": args.operation_id, "target_hash": TARGET_HASH, "target_count": 4,
            "complete": False, "execution_allowed": False, "diagnostic_only": True, "drop_ready": False,
            "identity_discovery_complete": summary["complete"], "identity_private_report_hash": digest,
            "identity_request_hash": request_hash, "inventory_entrypoint_catalog_hash": entrypoint_hash,
            "identity_diagnostic_histograms": public_histograms,
            "identity_histogram_bucket_pages": bucket_pages,
            "identity_database_states": clean_states, "error_category": "identity_discovery_requires_independent_approval"}


@contextlib.contextmanager
def locked_operation(directory):
    """Serializes this private manifest/journal, not all production db access."""
    private_directory(directory)
    path = directory / "operation.lock"
    try:
        fd = os.open(path, os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
    except OSError:
        fail("operation_lock_unavailable")
    try:
        metadata = os.fstat(fd)
        if not stat.S_ISREG(metadata.st_mode) or metadata.st_uid != os.getuid() or stat.S_IMODE(metadata.st_mode) != 0o600 or metadata.st_nlink != 1:
            fail("operation_lock_invalid")
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except OSError:
            fail("operation_busy")
        yield
    finally:
        os.close(fd)


class SafeParser(argparse.ArgumentParser):
    def error(self, message):
        fail("input_invalid")


def report_diagnostic(args):
    """Observe an exact existing boundary report, never repeat its DB scan."""
    text = args.bootstrap_approval_json
    token(args.bootstrap_approval_hash, HASH)
    if type(text) is not str or not 0 < len(text) <= MAX_BOOTSTRAP_APPROVAL or not text.isascii():
        fail("report_diagnostic_approval_invalid")
    value = decode(text.encode("ascii"))
    raw = canonical_bytes(value)
    if text.encode("ascii") != raw[:-1] or hashlib.sha256(raw).hexdigest() != args.bootstrap_approval_hash:
        fail("report_diagnostic_approval_hash_invalid")
    if type(value) is dict and value.get("kind") == "cleanup_only_failed_inventory_baseline_approval":
        return failed_inventory_cleanup_baseline(args, value)
    if type(value) is dict and value.get("kind") == "readonly_existing_inventory_report_approval":
        return inventory_report_diagnostic(args, value)
    fields(value, ("format_version", "kind", "prepare_mode", "source_sha", "operation_id",
                   "target_hash", "database_scope", "boundary_report"))
    if type(value["format_version"]) is not int or value["format_version"] != 1 or value["kind"] != "readonly_existing_boundary_report_approval" or value["prepare_mode"] != "report-diagnostic" or value["target_hash"] != TARGET_HASH or value["database_scope"] != "mysql-and-mongodb":
        fail("report_diagnostic_approval_invalid")
    validate_binding(value, args.operation_id, args.actual_source_sha)
    reference = value["boundary_report"]
    fields(reference, ("run_id", "source_sha", "sha256", "request_sha256"))
    token(reference["run_id"], RUN); token(reference["source_sha"], SHA)
    token(reference["sha256"], HASH); token(reference["request_sha256"], HASH)
    if reference["run_id"] == args.run_id:
        fail("report_diagnostic_origin_invalid")
    # A historical source may differ from the current approved TOOL source
    # only in this diagnostic branch. It cannot mint an inventory approval.
    directory = operation_directory(args.root, args.operation_id)
    original_request, _ = read_private(directory, "boundary-request.json", reference["request_sha256"])
    validate_v2_request(original_request, args.operation_id, reference["source_sha"], boundary=True)
    output = private_directory(directory / ("bounds-" + reference["run_id"]))
    report, report_hash = read_private(output, "boundary.private.json", reference["sha256"])
    fields(report, ("format_version", "kind", "source_sha", "operation_id", "run_id", "request_hash", "target_hash", "observed_at", "complete", "drop_ready", "diagnostic_only", "error_category", "database_bindings", "targets", "source_bytes_protocol", "consistency_semantics"), ("boundary_report_hash",))
    if type(report["format_version"]) is not int or report["format_version"] != 2 or report["kind"] != "readonly_inventory_boundaries" or report["source_sha"] != reference["source_sha"] or report["operation_id"] != args.operation_id or report["run_id"] != reference["run_id"] or report["request_hash"] != reference["request_sha256"] or report["target_hash"] != TARGET_HASH or type(report["complete"]) is not bool or report["drop_ready"] is not False or report["diagnostic_only"] is not True or report.get("boundary_report_hash", "") != "" or report["source_bytes_protocol"] != "no_source_body_copy" or report["consistency_semantics"] != "diagnostic_upper_discovery_requires_independent_request_approval":
        fail("report_diagnostic_report_binding_invalid")
    utc(report["observed_at"])
    validate_inventory_bindings(report["database_bindings"], report["complete"])
    # Exact report bytes are independently bound above; inspect no source body.
    # Unknown error strings stay private and reduce to a fixed unknown token.
    categories = inventory_diagnostic_error_categories(report["database_bindings"])
    if type(report["targets"]) is not list or len(report["targets"]) > 4:
        fail("report_diagnostic_report_binding_invalid")
    seen = set()
    for item in report["targets"]:
        if type(item) is not dict:
            fail("report_diagnostic_report_binding_invalid")
        target = tuple(item.get(key) for key in ("database", "name", "kind"))
        if any(type(part) is not str for part in target) or target not in TARGETS or target in seen:
            fail("report_diagnostic_report_binding_invalid")
        seen.add(target)
    if report["complete"] and (len(seen) != 4 or any(category != "none" for category in categories.values())):
        fail("report_diagnostic_report_binding_invalid")
    # Recheck real file bytes and directory protections before emitting. No
    # operation lock/file, registry, request, report or checkpoint is written.
    operation_directory(args.root, args.operation_id)
    private_directory(output)
    read_private(directory, "boundary-request.json", reference["request_sha256"])
    read_private(output, "boundary.private.json", reference["sha256"])
    return {"format_version": 1, "operation": "prepare", "prepare_mode": "report-diagnostic",
            "source_sha": args.actual_source_sha, "run_id": args.run_id, "operation_id": args.operation_id,
            "target_hash": TARGET_HASH, "target_count": 4, "complete": False, "execution_allowed": False,
            "diagnostic_only": True, "drop_ready": False, "report_diagnostic_complete": True,
            "report_diagnostic_approval_sha256": args.bootstrap_approval_hash,
            "observed_boundary_report": reference.copy(), "boundary_discovery_complete": report["complete"],
            "boundary_private_report_hash": report_hash, "boundary_request_hash": reference["request_sha256"],
            "inventory_database_error_categories": categories,
            "error_category": "existing_report_diagnostic_only", "capabilities": {key: False for key in CAPABILITIES}}


def inventory_report_diagnostic(args, value):
    """Read an exact existing native inventory report, never its source assets."""
    fields(value, ("format_version", "kind", "prepare_mode", "source_sha", "operation_id",
                   "target_hash", "database_scope", "inventory_report"))
    if type(value["format_version"]) is not int or value["format_version"] != 1 or value["kind"] != "readonly_existing_inventory_report_approval" or value["prepare_mode"] != "report-diagnostic" or value["target_hash"] != TARGET_HASH or value["database_scope"] != "mysql-and-mongodb":
        fail("report_diagnostic_approval_invalid")
    validate_binding(value, args.operation_id, args.actual_source_sha)
    reference = value["inventory_report"]
    fields(reference, ("run_id", "source_sha", "sha256", "request_sha256"))
    token(reference["run_id"], RUN); token(reference["source_sha"], SHA)
    token(reference["sha256"], HASH); token(reference["request_sha256"], HASH)
    if reference["run_id"] == args.run_id:
        fail("report_diagnostic_origin_invalid")
    directory = operation_directory(args.root, args.operation_id)
    request, _ = read_private(directory, "inventory-request.json", reference["request_sha256"])
    validate_v2_request(request, args.operation_id, reference["source_sha"], boundary=False)
    # Exact existing path used by native_inventory/live_inventory. A diagnostic
    # never creates this directory, a lock, request, checkpoint or permit.
    output = private_directory(directory / ("inventory-" + reference["run_id"]))
    report, report_hash = read_private(output, "inventory.private.json", reference["sha256"])
    fields(report, ("format_version", "kind", "source_sha", "operation_id", "run_id", "request_hash", "target_hash", "observed_at", "complete", "drop_ready", "diagnostic_only", "error_category", "database_bindings", "targets", "source_bytes_protocol", "consistency_semantics", "boundary_report_hash"))
    if type(report["format_version"]) is not int or report["format_version"] != 2 or report["kind"] != "readonly_compatibility_inventory" or report["source_sha"] != reference["source_sha"] or report["operation_id"] != args.operation_id or report["run_id"] != reference["run_id"] or report["request_hash"] != reference["request_sha256"] or report["target_hash"] != TARGET_HASH or type(report["complete"]) is not bool or report["drop_ready"] is not False or report["diagnostic_only"] is not True or report["boundary_report_hash"] != request["boundary_report_hash"] or report["source_bytes_protocol"] != "mysql_cast_binary_columns_pk_order_v2+mongodb_server_bson_pk_order_v2" or report["consistency_semantics"] != "two_equal_complete_passes_within_independently_approved_upper;sql_same_readonly_snapshot;mongo_homogeneous_bson_id_simple_collation;after_upper_next_cycle_not_fenced":
        fail("report_diagnostic_report_binding_invalid")
    utc(report["observed_at"])
    validate_inventory_bindings(report["database_bindings"], report["complete"])
    categories = inventory_report_error_categories(report["database_bindings"])
    if type(report["targets"]) is not list or len(report["targets"]) > 4:
        fail("report_diagnostic_report_binding_invalid")
    seen = set()
    for item in report["targets"]:
        fields(item, ("database", "name", "kind", "present", "complete", "records", "schema_hash", "data_hash", "identity_hash", "bytes", "classification", "error_category", "equal_full_passes", "pages", "next_cycle_required"), ("source_file", "boundary"))
        target = tuple(item[key] for key in ("database", "name", "kind"))
        if any(type(part) is not str for part in target) or target not in TARGETS or target in seen:
            fail("report_diagnostic_report_binding_invalid")
        seen.add(target)
        for key in ("records", "bytes", "pages", "equal_full_passes"): uint(item[key])
        if any(type(item[key]) is not bool for key in ("present", "complete", "next_cycle_required")):
            fail("report_diagnostic_report_binding_invalid")
        if report["complete"] and (item["complete"] is not True or item["error_category"] != "none" or item["equal_full_passes"] != 2 or item.get("boundary") != request["approved_boundaries"][TARGETS.index(target)]):
            fail("report_diagnostic_report_binding_invalid")
    if report["complete"] and (len(seen) != 4 or report["error_category"] != "none" or any(category != "none" for category in categories.values())):
        fail("report_diagnostic_report_binding_invalid")
    # Recheck protected original files before publishing only fixed categories.
    # Reading a report cannot prove that its source assets or history passed.
    operation_directory(args.root, args.operation_id)
    private_directory(output)
    read_private(directory, "inventory-request.json", reference["request_sha256"])
    read_private(output, "inventory.private.json", reference["sha256"])
    return {"format_version": 1, "operation": "prepare", "prepare_mode": "report-diagnostic",
            "source_sha": args.actual_source_sha, "run_id": args.run_id, "operation_id": args.operation_id,
            "target_hash": TARGET_HASH, "target_count": 4, "complete": False, "execution_allowed": False,
            "diagnostic_only": True, "drop_ready": False, "report_diagnostic_complete": True,
            "report_diagnostic_approval_sha256": args.bootstrap_approval_hash,
            "observed_inventory_report": reference.copy(), "inventory_complete": report["complete"],
            "inventory_private_report_hash": report_hash, "inventory_request_hash": reference["request_sha256"],
            "inventory_database_error_categories": categories,
            "error_category": "existing_report_diagnostic_only", "capabilities": {key: False for key in CAPABILITIES}}


# One failed, already terminal producer only. This is temporary-file disposition,
# never an alternate inventory parser, history proof, or DROP capability.
FAILED_INVENTORY_OPERATION = "38019009876-1"
FAILED_INVENTORY_REFERENCE = {
    "source_sha": "ae807219ffee37c1f060f1bfca25dd7c408ef9fd",
    "run_id": "38025045551-1",
    "request_sha256": "e5d29b674698dbfe4d8f85932fad0d8bba445e3b6823028cd78db4a150f16962",
    "sha256": "3983779a5fab8be6e027390491a3c9215cdf1270c03316ed8ebcc614793777c8",
}
FAILED_INVENTORY_LIMITS = dict(INVENTORY_V2_LIMITS, page_size=1000)
FAILED_INVENTORY_MONGO_PAGES = 281
FAILED_INVENTORY_BASELINE = "failed-inventory-cleanup-baseline.json"
FAILED_INVENTORY_BASELINE_MAX = 3 * 1024 * 1024


def failed_inventory_container_absent(deadline):
    # Check both the exact prospective name and any original op/run-labelled
    # producer. An inspect error is never interpreted as container absence.
    filters = (("name=^/qs-compatibility-inventory-" + FAILED_INVENTORY_REFERENCE["run_id"] + "$",),
               ("label=qs.compatibility-retirement.operation=" + FAILED_INVENTORY_OPERATION,
                "label=qs.compatibility-retirement.run=" + FAILED_INVENTORY_REFERENCE["run_id"]))
    for selection in filters:
        command = ["sudo", "-n", "docker", "container", "ls", "--all", "--no-trunc"]
        for condition in selection: command.extend(("--filter", condition))
        command.extend(("--format", "{{.ID}}"))
        code, raw = capture_fixed(command, timeout=min(15, max(0.01, deadline-time.monotonic())))
        if code or raw.strip() or time.monotonic() >= deadline:
            fail("failed_inventory_producer_not_absent")


def failed_inventory_stat(st):
    return [st.st_dev, st.st_ino, st.st_size, st.st_mtime_ns, st.st_ctime_ns,
            st.st_uid, stat.S_IMODE(st.st_mode), st.st_nlink]


def failed_inventory_file(dirfd, name, deadline, *, decode_json=False):
    # Names come only from the closed original producer list below. Hold the
    # actual file FD while hashing; no source body enters a JSON receipt.
    try: fd = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=dirfd)
    except OSError: fail("failed_inventory_file_unavailable")
    try:
        before = os.fstat(fd)
        maximum = MAX_JSON if decode_json else FAILED_INVENTORY_LIMITS["max_bytes"]
        if not stat.S_ISREG(before.st_mode) or before.st_uid != os.getuid() or stat.S_IMODE(before.st_mode) != 0o600 or before.st_nlink != 1 or not 0 <= before.st_size <= maximum:
            fail("failed_inventory_file_not_private")
        digest = hashlib.sha256(); chunks = []
        while True:
            if time.monotonic() >= deadline: fail("failed_inventory_cleanup_timeout")
            chunk = os.read(fd, 128 * 1024)
            if not chunk: break
            digest.update(chunk)
            if decode_json: chunks.append(chunk)
        after = os.fstat(fd)
        visible = os.stat(name, dir_fd=dirfd, follow_symlinks=False)
        if failed_inventory_stat(before) != failed_inventory_stat(after) or failed_inventory_stat(after) != failed_inventory_stat(visible):
            fail("failed_inventory_file_changed")
        return {"sha256": digest.hexdigest(), "stat": failed_inventory_stat(after)}, decode(b"".join(chunks)) if decode_json else None
    finally: os.close(fd)


def failed_inventory_inputs(args):
    directory = operation_directory(args.root, FAILED_INVENTORY_OPERATION)
    ref = FAILED_INVENTORY_REFERENCE
    request, _ = read_private(directory, "inventory-request.json", ref["request_sha256"])
    if request.get("limits") != FAILED_INVENTORY_LIMITS or any(type(v) is not int for v in request["limits"].values()):
        fail("failed_inventory_old_profile_mismatch")
    # Validate all other existing request semantics without permitting the old
    # profile in the ordinary/current inventory entry points.
    validate_v2_request(dict(request, limits=dict(INVENTORY_V2_LIMITS)), FAILED_INVENTORY_OPERATION, ref["source_sha"], boundary=False)
    output = private_directory(directory / ("inventory-" + ref["run_id"]))
    report, _ = read_private(output, "inventory.private.json", ref["sha256"])
    if report.get("kind") != "readonly_compatibility_inventory" or report.get("format_version") != 2 or report.get("source_sha") != ref["source_sha"] or report.get("operation_id") != FAILED_INVENTORY_OPERATION or report.get("run_id") != ref["run_id"] or report.get("request_hash") != ref["request_sha256"] or report.get("target_hash") != TARGET_HASH or report.get("complete") is not False or report.get("drop_ready") is not False or report.get("diagnostic_only") is not True:
        fail("failed_inventory_report_mismatch")
    targets = report.get("targets")
    if type(targets) is not list or len(targets) != 4 or [tuple(i.get(k) for k in ("database","name","kind")) for i in targets] != list(TARGETS):
        fail("failed_inventory_report_mismatch")
    names = set(SOURCE_FILENAMES.values()) | ASSET_FILENAMES | {"inventory.private.json", "entrypoints.private.json", "mysql-metadata.private.json"}
    checkpoints = {}
    for item in targets[:3]:
        for key in ("records", "pages", "equal_full_passes"): uint(item.get(key))
        if item.get("complete") is not True or item["equal_full_passes"] != 2 or item.get("error_category") != "none" or item["pages"] % 2 or item["pages"] > 2002:
            fail("failed_inventory_sql_checkpoint_scope_invalid")
        for pass_id in (1,2):
            for page in range(1, item["pages"]//2+1):
                checkpoints[f"mysql-{item['name']}-pass-{pass_id}-page-{page:06d}.checkpoint.json"] = (pass_id,page)
    if targets[3].get("complete") is not False or targets[3].get("equal_full_passes") != 0:
        fail("failed_inventory_mongo_incomplete_mismatch")
    for page in range(1, FAILED_INVENTORY_MONGO_PAGES+1):
        checkpoints[f"mongodb-domain_event_outbox-pass-1-page-{page:06d}.checkpoint.json"] = (1,page)
    names.update(checkpoints)
    return directory, output, request, names, checkpoints


def failed_inventory_cleanup_baseline(args, value):
    fields(value, ("format_version", "kind", "prepare_mode", "source_sha", "operation_id",
                   "target_hash", "database_scope", "inventory_report"))
    if type(value["format_version"]) is not int or value["format_version"] != 1 or value["kind"] != "cleanup_only_failed_inventory_baseline_approval" or value["prepare_mode"] != "report-diagnostic" or value["target_hash"] != TARGET_HASH or value["database_scope"] != "mysql-and-mongodb" or value["operation_id"] != FAILED_INVENTORY_OPERATION or value["inventory_report"] != FAILED_INVENTORY_REFERENCE:
        fail("failed_inventory_cleanup_approval_invalid")
    validate_binding(value, args.operation_id, args.actual_source_sha)
    if args.run_id == FAILED_INVENTORY_REFERENCE["run_id"]: fail("report_diagnostic_origin_invalid")
    directory, output, request, names, checkpoints = failed_inventory_inputs(args)
    deadline = time.monotonic()+100
    with locked_operation(directory):
        failed_inventory_container_absent(deadline)
        dirfd = os.open(output, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            st = os.fstat(dirfd)
            if st.st_uid != os.getuid() or stat.S_IMODE(st.st_mode) != 0o700:
                fail("failed_inventory_directory_not_private")
            if set(os.listdir(dirfd)) != names: fail("failed_inventory_unknown_or_missing_member")
            entries = {}
            for name in sorted(names):
                item, content = failed_inventory_file(dirfd, name, deadline, decode_json=name.endswith(".json") and name != "mysql-metadata.private.json")
                if name == "inventory.private.json" and item["sha256"] != FAILED_INVENTORY_REFERENCE["sha256"]:
                    fail("failed_inventory_report_mismatch")
                if name in checkpoints:
                    fields(content, ("format_version","kind","source_sha","pass","page","cursor_token","records","source_bytes","prefix_hash","diagnostic_only","resume_existing_file_allowed"))
                    pass_id,page = checkpoints[name]
                    if content["format_version"] != 1 or type(content["format_version"]) is not int or content["kind"] != "readonly_inventory_page_checkpoint" or content["source_sha"] != FAILED_INVENTORY_REFERENCE["source_sha"] or type(content["pass"]) is not int or type(content["page"]) is not int or (content["pass"],content["page"]) != (pass_id,page) or content["diagnostic_only"] is not True or content["resume_existing_file_allowed"] is not False or type(content["cursor_token"]) is not str or len(content["cursor_token"]) > 2048:
                        fail("failed_inventory_checkpoint_mismatch")
                    uint(content["records"]); uint(content["source_bytes"]); token(content["prefix_hash"], HASH)
                elif name in ASSET_FILENAMES:
                    filename = name.removesuffix(".asset.json")
                    target = next(target for target,path in SOURCE_FILENAMES.items() if path == filename)
                    boundary = request["approved_boundaries"][next(i for i,t in enumerate(TARGETS) if t[:2] == target)]
                    expected = {"format_version":1,"kind":"temporary_inventory_source_copy","filename":filename,"source_sha":FAILED_INVENTORY_REFERENCE["source_sha"],"operation_id":FAILED_INVENTORY_OPERATION,"run_id":FAILED_INVENTORY_REFERENCE["run_id"],"request_hash":FAILED_INVENTORY_REFERENCE["request_sha256"],"protocol":"mysql_cast_binary_columns_pk_order_v2" if target[0] == "mysql" else "mongodb_server_bson_pk_order_v2","boundary":boundary,"contains_original_body":True,"retirement_proof":False,"purge_required_after_acceptance":True,"resume_existing_file_allowed":False}
                    if content != expected: fail("failed_inventory_asset_mismatch")
                elif name == "entrypoints.private.json":
                    fields(content, ("format_version","kind","source_sha","operation_id","run_id","request_hash","catalog_hash","live_fence_proven","catalog"))
                    if content["format_version"] != 1 or type(content["format_version"]) is not int or content["kind"] != "source_only_production_entrypoint_catalog" or content["source_sha"] != FAILED_INVENTORY_REFERENCE["source_sha"] or content["operation_id"] != FAILED_INVENTORY_OPERATION or content["run_id"] != FAILED_INVENTORY_REFERENCE["run_id"] or content["request_hash"] != FAILED_INVENTORY_REFERENCE["request_sha256"] or content["live_fence_proven"] is not False:
                        fail("failed_inventory_catalog_mismatch")
                    token(content["catalog_hash"], HASH)
                entries[name] = item
            failed_inventory_container_absent(deadline)
            if set(os.listdir(dirfd)) != names or os.stat(output, follow_symlinks=False).st_ino != st.st_ino or os.stat(output, follow_symlinks=False).st_dev != st.st_dev:
                fail("failed_inventory_directory_changed")
            read_private(directory, "inventory-request.json", FAILED_INVENTORY_REFERENCE["request_sha256"])
            # These are current observed bytes/inodes, NOT original source hash
            # proof. The hard-coded producer disposition is the narrow scope.
            baseline = {"format_version":1,"kind":"cleanup_only_failed_inventory_baseline","original_operation_id":FAILED_INVENTORY_OPERATION,"original_inventory_report":FAILED_INVENTORY_REFERENCE.copy(),"observing_source_sha":args.actual_source_sha,"observing_run_id":args.run_id,"approval_sha256":args.bootstrap_approval_hash,"directory_identity":[st.st_dev,st.st_ino,st.st_uid,stat.S_IMODE(st.st_mode)],"files":entries,"inventory_complete":False,"retirement_proof":False,"drop_authority":False,"producer_container_absent":True}
            raw = canonical_bytes(baseline)
            if len(raw) > FAILED_INVENTORY_BASELINE_MAX: fail("failed_inventory_baseline_bound_exceeded")
            create_bootstrap_file(directory, FAILED_INVENTORY_BASELINE, raw)
        finally: os.close(dirfd)
    return {"format_version":1,"operation":"prepare","prepare_mode":"report-diagnostic","source_sha":args.actual_source_sha,"run_id":args.run_id,"operation_id":args.operation_id,"target_hash":TARGET_HASH,"target_count":4,"complete":False,"execution_allowed":False,"drop_ready":False,"diagnostic_only":True,"inventory_complete":False,"cleanup_only":True,"cleanup_baseline_complete":True,"cleanup_baseline_sha256":hashlib.sha256(raw).hexdigest(),"cleanup_file_count":len(entries),"cleanup_source_file_bytes":sum(entries[n]["stat"][2] for n in SOURCE_FILENAMES.values()),"observed_inventory_report":FAILED_INVENTORY_REFERENCE.copy(),"original_content_verified":False,"purge_executed":False,"error_category":"failed_inventory_cleanup_baseline_only","capabilities":{key:False for key in CAPABILITIES}}


LIFECYCLE_ADAPTERS = frozenset({"actual_four_source_historical_persistence_and_readback",
    "actual_production_bound_isolated_restore", "server_a_and_server_d_stop_drain_lease",
    "whole_writer_and_old_ref_fence", "prepared_inline_b_and_no_automigration_rollback",
    "actual_runtime_acceptance_and_private_purge"})


# This fixed inline root once program is part of the already hash-verified
# immutable Action package. The same pinned SSH connection supplies sudo;
# inability to execute it fails prepare before a database/engine operation.
ROOT_PREPARE_ONCE = r"""
import hashlib, io, json, os, platform, re, stat, subprocess, sys, tarfile
from pathlib import Path

def stop():
    print('{"format_version":1,"complete":false,"execution_allowed":false,"drop_ready":false,"error_category":"lifecycle_root_once_bootstrap_rejected"}')
    raise SystemExit(1)

def private_directory(path):
    info = path.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or stat.S_IMODE(info.st_mode) != 0o700: stop()
    for ancestor in path.parents:
        v = ancestor.lstat()
        if not stat.S_ISDIR(v.st_mode) or (stat.S_IMODE(v.st_mode) & 0o022 and not v.st_mode & stat.S_ISVTX): stop()

try:
    if os.getuid() != 0 or os.geteuid() != 0 or len(sys.argv) not in (8,9): stop()
    stage = 'lifecycle' if len(sys.argv) == 8 else sys.argv[-1]
    if stage not in ('lifecycle','prepare-facts','host-writer-scope','db-writer-census') or (stage == 'lifecycle' and len(sys.argv) != 8): stop()
    operation, run, tool_sha, request_hash, package_hash, manifest_hash, source_channel = sys.argv[1:8]
    if not all(re.fullmatch(r'[0-9]{1,20}-[0-9]{1,4}',v) for v in (operation,run)): stop()
    if not re.fullmatch(r'[0-9a-f]{40}',tool_sha) or not all(re.fullmatch(r'[0-9a-f]{64}',v) for v in (request_hash,package_hash)): stop()
    if (stage == 'lifecycle' and not re.fullmatch(r'[0-9a-f]{64}',manifest_hash)) or (stage in ('prepare-facts','host-writer-scope','db-writer-census') and manifest_hash != ''): stop()
    if source_channel == 'sudo-user':
        source_uid = int(os.environ['SUDO_UID'])
        if source_uid < 1: stop()
    elif source_channel == 'root-direct':
        # The fixed root caller supplies a cleaned environment. Never adopt
        # another user's identity from an inherited SUDO_UID or input DTO.
        if 'SUDO_UID' in os.environ: stop()
        source_uid = os.getuid() # actual root identity, already jointly checked
    else:
        stop()
    allowed = {'MYSQL_HOST','MYSQL_PORT','MYSQL_USERNAME','MYSQL_PASSWORD','MYSQL_DATABASE','MONGODB_HOST','MONGODB_PORT','MONGODB_USERNAME','MONGODB_PASSWORD','MONGODB_DBNAME','MONGODB_METADATA_ADMIN_USERNAME','MONGODB_METADATA_ADMIN_PASSWORD'}
    packet = sys.stdin.buffer.read(32769)
    if len(packet)>32768: stop()
    def unique_credentials(items):
        value={}
        for key,item in items:
            if key in value: stop()
            value[key]=item
        return value
    credentials=json.loads(packet,object_pairs_hook=unique_credentials)
    if stage == 'host-writer-scope': allowed=set()
    if not isinstance(credentials,dict) or set(credentials)!=allowed or any(not isinstance(v,str) or '\x00' in v or '\r' in v or '\n' in v for v in credentials.values()): stop()
    archive=Path('/tmp/qs-compatibility-retirement-'+run+'.tar.gz')
    fd=os.open(archive,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK)
    with os.fdopen(fd,'rb') as f:
        info=os.fstat(f.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_nlink!=1 or info.st_uid!=source_uid or info.st_size<1 or info.st_size>256<<20: stop()
        raw=f.read((256<<20)+1)
    if len(raw)>256<<20 or hashlib.sha256(raw).hexdigest()!=package_hash: stop()
    arch={'x86_64':'amd64','aarch64':'arm64','arm64':'arm64'}.get(platform.machine())
    if arch is None: stop()
    name='inventory-linux-'+arch
    with tarfile.open(fileobj=io.BytesIO(raw),mode='r:gz') as tar:
        members=tar.getmembers()
        names=[m.name for m in members]
        if len(names)!=len(set(names)) or any(not m.isfile() or '/' in m.name or m.size>(200<<20 if stage=='prepare-facts' and m.name=='preload-image.tar.gz' else 64<<20) for m in members): stop()
        selected=[m for m in members if m.name==name]
        if len(selected)!=1: stop()
        binary=tar.extractfile(selected[0]).read((64<<20)+1)
        if not binary or len(binary)>64<<20: stop()
        preload_program=preload_archive=None
        if stage=='prepare-facts':
            programs=[m for m in members if m.name=='compatibility-window-tool.py']
            images=[m for m in members if m.name=='preload-image.tar.gz']
            if len(programs)!=1 or programs[0].size>1<<20 or len(images)!=1: stop()
            preload_program=tar.extractfile(programs[0]).read((1<<20)+1)
            preload_archive=tar.extractfile(images[0]).read((200<<20)+1)
    base=Path('/opt/backups/qs-server/compatibility-retirement-root-prepare')
    try: base.mkdir(mode=0o700)
    except FileExistsError: pass
    private_directory(base)
    if stage in ('host-writer-scope','db-writer-census'):
        # The new observer executable is installed only below an actual root
        # namespace; original user-owned request sources are not re-owned.
        for ancestor in (base, *base.parents):
            v=ancestor.lstat()
            if v.st_uid != 0 or stat.S_IMODE(v.st_mode) & 0o022: stop()
    batch=base/(operation+'-'+run)
    batch.mkdir(mode=0o700) # once only; unknown earlier work never silently adopted
    private_directory(batch)
    native=batch/'restore-native'
    request_name = 'lifecycle-request.json' if stage == 'lifecycle' else stage+'-request-'+run+'.json'
    request_path='/opt/backups/qs-server/compatibility-retirement/'+operation+'/'+request_name
    registry={'format_version':1,'kind':'approved_root_once_tool_staging','stage':stage,'operation_id':operation,'actual_run_id':run,'tool_source_sha':tool_sha,'request_path':request_path,'request_sha256':request_hash,'manifest_sha256':manifest_hash,'package_sha256':package_hash,'native_sha256':hashlib.sha256(binary).hexdigest(),'source_uid':source_uid,'drop_authority':False,'purge_after_acceptance_required':True}
    fd=os.open(batch/'tool.intent.private.json',os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
    with os.fdopen(fd,'wb') as f: f.write(json.dumps(registry,sort_keys=True,separators=(',',':')).encode()+b'\n'); f.flush(); os.fsync(f.fileno())
    fd=os.open(batch,os.O_RDONLY|os.O_DIRECTORY)
    os.fsync(fd);os.close(fd)
    fd=os.open(base,os.O_RDONLY|os.O_DIRECTORY)
    os.fsync(fd);os.close(fd)
    fd=os.open(native,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o700)
    with os.fdopen(fd,'wb') as f: f.write(binary); f.flush(); os.fsync(f.fileno())
    fd=os.open(batch,os.O_RDONLY|os.O_DIRECTORY)
    os.fsync(fd);os.close(fd)
    import subprocess
    check=subprocess.run([str(native),'--source-sha'],stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,timeout=5,env={'PATH':'/usr/bin:/bin'})
    if check.returncode or check.stdout!=tool_sha.encode()+b'\n': stop()
    credentials['PATH']='/usr/bin:/bin'
    credentials['QS_RETIREMENT_SOURCE_UID']=str(source_uid)
    native_mode = stage+'-root-once' if stage != 'lifecycle' else 'lifecycle-prepare-root-once'
    native_argv=[str(native),'--mode',native_mode,'--request',request_path,'--request-hash',request_hash,'--operation-id',operation,'--run-id',run]
    if stage=='prepare-facts':
        module={'__name__':'approved_cached_image_preparation'}
        exec(compile(preload_program,'approved-package-cached-image-caller','exec'),module)
        request_raw=module['read_owned'](Path(request_path),source_uid,request_hash,256<<10)
        request=module['decode'](request_raw)
        if request.get('source_sha')!=tool_sha or request.get('operation_id')!=operation or request.get('actual_run_id')!=run: stop()
        approval={'stage':'prepare','tool_source_sha':tool_sha,'original_source_sha':request['inventory_report']['source_sha'],'operation_id':operation}
        observed=module['preload_api_image'](preload_archive,approval,run,batch,None)
        result=subprocess.run(native_argv,env=credentials,stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,timeout=85*60,check=False)
        if len(result.stdout)>32768: stop()
        native_result=json.loads(result.stdout,object_pairs_hook=unique_credentials)
        print(json.dumps({'native_receipt':native_result,'image_preload':observed},sort_keys=True,separators=(',',':')))
        raise SystemExit(result.returncode)
    os.execve(native,[str(native),'--mode',native_mode,'--request',request_path,'--request-hash',request_hash,'--operation-id',operation,'--run-id',run],credentials)
except (OSError,ValueError,KeyError,tarfile.TarError,subprocess.SubprocessError):
    stop()
"""


def root_native_diagnostic(private_stderr, completed, stdout=b'', returncode=0):
    size = os.fstat(private_stderr.fileno()).st_size
    private_stderr.seek(0)
    sample = private_stderr.read(8192)
    return {'process_completed': completed,
            'exit_code': max(returncode, 0), 'termination_signal': max(-returncode, 0),
            'stdout_bytes': len(stdout), 'stderr_bytes': size,
            'stderr_sample_bytes': len(sample),
            'stderr_sample_sha256': hashlib.sha256(sample).hexdigest(),
            'stderr_sample_truncated': size > len(sample)}


def root_once_lifecycle_prepare(args):
    if args.operation != 'prepare' or args.prepare_mode not in ('lifecycle','prepare-facts','host-writer-scope','db-writer-census'):
        fail('lifecycle_root_host_channel_required')
    package_hash=os.environ.get('RETIREMENT_PACKAGE_SHA256','')
    token(package_hash,HASH)
    names=('MYSQL_HOST','MYSQL_PORT','MYSQL_USERNAME','MYSQL_PASSWORD','MYSQL_DATABASE','MONGODB_HOST','MONGODB_PORT','MONGODB_USERNAME','MONGODB_PASSWORD','MONGODB_DBNAME','MONGODB_METADATA_ADMIN_USERNAME','MONGODB_METADATA_ADMIN_PASSWORD')
    packet=json.dumps({} if args.prepare_mode == 'host-writer-scope' else {name:os.environ.get(name,'') for name in names},separators=(',',':')).encode()
    if len(packet)>32768: fail('lifecycle_connection_input_rejected')
    uid, euid = os.getuid(), os.geteuid()
    if uid != euid:
        fail('lifecycle_root_host_channel_required')
    facts = args.prepare_mode in ('prepare-facts','host-writer-scope','db-writer-census')
    request_hash = args.db_census_request_hash if args.prepare_mode == 'db-writer-census' else args.host_scope_request_hash if args.prepare_mode == 'host-writer-scope' else args.prepare_facts_request_hash if facts else args.lifecycle_request_hash
    token(request_hash,HASH)
    if facts and args.manifest_hash: fail('prepare_facts_input_classes_mixed')
    bindings=[args.operation_id,args.run_id,args.actual_source_sha,request_hash,package_hash,args.manifest_hash]
    suffix = [args.prepare_mode] if facts else []
    # Credentials remain on this bounded private pipe, not argv/stdout/logs.
    # Keep stderr on a private temporary FD. Only length and a bounded sample
    # digest may leave this scope; child text can contain credentials/URIs.
    with tempfile.TemporaryFile() as private_stderr:
        try:
            if uid == 0:
                result=subprocess.run(['/usr/bin/python3','-I','-c',ROOT_PREPARE_ONCE,*bindings,'root-direct',*suffix],env={'PATH':'/usr/bin:/bin'},input=packet,stdout=subprocess.PIPE,stderr=private_stderr,timeout=91*60,check=False)
            elif args.prepare_mode in ('host-writer-scope','db-writer-census'):
                result=subprocess.run(['/usr/bin/sudo','-n','--','/usr/bin/python3','-I','-c',ROOT_PREPARE_ONCE,*bindings,'sudo-user',*suffix],env={'PATH':'/usr/bin:/bin'},input=packet,stdout=subprocess.PIPE,stderr=private_stderr,timeout=3*60,check=False)
            else:
                result=subprocess.run(['sudo','-n','python3','-I','-c',ROOT_PREPARE_ONCE,*bindings,'sudo-user',*suffix],input=packet,stdout=subprocess.PIPE,stderr=private_stderr,timeout=91*60,check=False)
        except OSError:
            raise NativeReceiptBlocked('lifecycle_native_start_failed', root_native_diagnostic(private_stderr, False)) from None
        except subprocess.TimeoutExpired as error:
            output = error.output if isinstance(error.output, bytes) else b''
            raise NativeReceiptBlocked('lifecycle_native_receipt_timed_out', root_native_diagnostic(private_stderr, False, output)) from None
        if len(result.stdout)>32768:
            raise NativeReceiptBlocked('lifecycle_native_receipt_invalid', root_native_diagnostic(private_stderr, True, result.stdout, result.returncode))
        if not result.stdout:
            raise NativeReceiptBlocked('lifecycle_native_receipt_missing', root_native_diagnostic(private_stderr, True, result.stdout, result.returncode))
    if args.prepare_mode == 'prepare-facts':
        wrapped=decode(result.stdout)
        fields(wrapped,('native_receipt','image_preload'))
        args.actual_preloaded_image=wrapped['image_preload']
        return result.returncode,canonical_bytes(wrapped['native_receipt'])
    return result.returncode,result.stdout


def live_lifecycle(args, directory):
    """Invoke the fixed native caller; a DTO never supplies its host adapters."""
    if args.operation != "prepare":
        fail("lifecycle_actual_host_adapters_missing")
    token(args.lifecycle_request_hash, HASH)
    token(args.manifest_hash, HASH)
    request, request_hash = read_private(directory, "lifecycle-request.json", args.lifecycle_request_hash)
    fields(request, ("format_version", "kind", "tool_source_sha", "original_source_sha",
        "operation_id", "actual_run_id", "manifest_sha256", "archive_directory",
        "window_directory", "journal_directory", "archive_approval", "recovery"),
        ("source_directory", "restore_engines", "source_file_sha256"))
    token(request["original_source_sha"], SHA)
    if (request["format_version"] != 1 or request["kind"] != "compatibility_retirement_lifecycle_request" or
        request["tool_source_sha"] != args.actual_source_sha or request["operation_id"] != args.operation_id or
        request["actual_run_id"] != args.run_id or request["manifest_sha256"] != args.manifest_hash):
        fail("lifecycle_request_binding_rejected")
    manifest, _ = read_private(directory, "manifest.json", args.manifest_hash)
    # Original A inventory/archive/window bindings are retained when the fixed
    # tool and B image are approved at another SHA. Never rewrite either value.
    validate_manifest(manifest, args.operation_id, request["original_source_sha"])
    binary = Path(args.inventory_binary)
    try:
        metadata = binary.lstat()
    except OSError:
        fail("lifecycle_binary_unavailable")
    if (not binary.is_absolute() or not stat.S_ISREG(metadata.st_mode) or
        metadata.st_nlink != 1 or metadata.st_uid != os.getuid() or stat.S_IMODE(metadata.st_mode) != 0o700):
        fail("lifecycle_binary_rejected")
    code, raw = capture_fixed([str(binary), "--source-sha"], timeout=5, maximum=128)
    if code or raw.decode("ascii", errors="ignore").strip() != args.actual_source_sha:
        fail("lifecycle_binary_source_mismatch")
    # This host-native process requires the root management channel and actual
    # fixed runtime adapters. Do not reuse the read-only inventory container or
    # serialize a Window/RestoreVerification/stop lease into its arguments.
    with locked_operation(directory):
        # Both legal privilege identities use the same actual once staging.
        # No root branch can invoke the unstaged lifecycle-prepare mode.
        code, raw = root_once_lifecycle_prepare(args)
    result = decode(raw)
    required = ("format_version", "kind", "operation", "source_sha", "original_source_sha",
        "operation_id", "run_id", "manifest_sha256", "request_sha256", "archive_sha256",
        "target_hash", "target_count", "complete", "execution_allowed", "drop_ready",
        "archive_binding_complete", "recovery_attempted", "recovery_complete", "acceptance_complete",
        "purge_complete", "error_category", "required_adapters", "isolated_content_restore_complete", "restore_elapsed_millis")
    fields(result, required, ("recovery_error_category", "mysql_recovery_non_target_sha256", "source_copy_intent_sha256", "preparation_restore_zero_sha256"))
    if (result["format_version"] != 1 or result["kind"] != "compatibility_retirement_lifecycle_result" or
        result["operation"] != args.operation or result["source_sha"] != args.actual_source_sha or
        result["original_source_sha"] != request["original_source_sha"] or result["operation_id"] != args.operation_id or
        result["run_id"] != args.run_id or result["manifest_sha256"] != args.manifest_hash or
        result["request_sha256"] != request_hash or result["target_hash"] != TARGET_HASH or result["target_count"] != 4 or
        type(result["required_adapters"]) is not list or set(result["required_adapters"]) - LIFECYCLE_ADAPTERS):
        fail("lifecycle_native_receipt_binding_rejected")
    for key in ("complete", "execution_allowed", "drop_ready", "archive_binding_complete", "recovery_attempted",
                "recovery_complete", "acceptance_complete", "purge_complete", "isolated_content_restore_complete"):
        if type(result[key]) is not bool:
            fail("lifecycle_native_receipt_rejected")
    for key in ("manifest_sha256", "request_sha256"):
        token(result[key], HASH)
    token(result["archive_sha256"], HASH if result["complete"] else re.compile(r"(?:[0-9a-f]{64})?"))
    if "mysql_recovery_non_target_sha256" in result:
        token(result["mysql_recovery_non_target_sha256"], HASH)
    if "source_copy_intent_sha256" in result:
        token(result["source_copy_intent_sha256"], HASH)
        if args.operation != "prepare":
            fail("lifecycle_native_receipt_binding_rejected")
    if "preparation_restore_zero_sha256" in result:
        token(result["preparation_restore_zero_sha256"], HASH)
        if args.operation != "prepare":
            fail("lifecycle_native_receipt_binding_rejected")
    if type(result["restore_elapsed_millis"]) is not int or not 0 <= result["restore_elapsed_millis"] <= 600000:
        fail("lifecycle_native_restore_budget_rejected")
    if args.operation == "prepare" and result["complete"] and not result["isolated_content_restore_complete"]:
        fail("lifecycle_native_restore_missing")
    if (result["execution_allowed"] is not False or result["drop_ready"] is not False or
        (result["complete"] is True) != (code == 0 and result["error_category"] == "none") or
        not re.fullmatch(r"[a-z_]{1,128}", result["error_category"]) or
        ("recovery_error_category" in result and not re.fullmatch(r"[a-z_]{1,128}", result["recovery_error_category"]))):
        fail("lifecycle_native_receipt_rejected")
    result["capabilities"] = {key: False for key in CAPABILITIES}
    return result


PREPARE_SOURCE_NAMES = ("inventory.private.json", "mysql-metadata.private.json", "mongodb-metadata.private.json",
    "mysql-domain_event_outbox.source.ndjson", "mysql-ai_bridge_commands.source.ndjson",
    "mysql-ai_messaging_legacy_commands.source.ndjson", "mongodb-domain_event_outbox.source.bsonframes")



def prepare_facts_request(args):
    """Approve only where to observe; never approve the returned schema facts."""
    token(args.operation_id, RUN); token(args.run_id, RUN)
    text = args.bootstrap_approval_json
    token(args.bootstrap_approval_hash, HASH)
    if type(text) is not str or not 0 < len(text) <= MAX_BOOTSTRAP_APPROVAL or not text.isascii():
        fail("prepare_facts_descriptor_invalid")
    value = decode(text.encode("ascii"))
    raw = canonical_bytes(value)
    if text.encode("ascii") != raw[:-1] or hashlib.sha256(raw).hexdigest() != args.bootstrap_approval_hash:
        fail("prepare_facts_descriptor_hash_invalid")
    fields(value, ("format_version", "kind", "prepare_mode", "source_sha", "operation_id", "target_hash",
        "database_scope", "inventory_report", "restore_engines", "archive_directory"))
    if (type(value["format_version"]) is not int or value["format_version"] != 1 or
        value["kind"] != "readonly_prepare_facts_observation_descriptor" or value["prepare_mode"] != "prepare-facts" or
        value["source_sha"] != args.actual_source_sha or value["operation_id"] != args.operation_id or
        value["target_hash"] != TARGET_HASH or value["database_scope"] != "mysql-and-mongodb"):
        fail("prepare_facts_descriptor_binding_rejected")
    producer = value["inventory_report"]
    fields(producer, ("operation_id", "run_id", "source_sha", "sha256", "request_sha256"))
    token(producer["operation_id"], RUN); token(producer["run_id"], RUN); token(producer["source_sha"], SHA)
    token(producer["sha256"], HASH); token(producer["request_sha256"], HASH)
    if producer["operation_id"] != args.operation_id or producer["run_id"] == args.run_id:
        fail("prepare_facts_producer_rejected")
    engines = value["restore_engines"]
    fields(engines, ("mysql_image_id", "mongodb_image_id", "architecture"))
    image = re.compile(r"sha256:[0-9a-f]{64}")
    token(engines["mysql_image_id"], image); token(engines["mongodb_image_id"], image)
    if engines["mysql_image_id"] == engines["mongodb_image_id"] or engines["architecture"] not in ("amd64", "arm64"):
        fail("prepare_facts_images_rejected")
    path = value["archive_directory"]
    root = Path("/opt/backups/qs-server/compatibility-retirement") / args.operation_id
    if (type(path) is not str or not 1 <= len(path) <= 1024 or any(v in path for v in ("\x00", "\r", "\n")) or
        not Path(path).is_absolute() or ".." in Path(path).parts or str(Path(path)) != path or
        not Path(path).is_relative_to(root) or Path(path) == root or
        Path(path).is_relative_to(root / ("inventory-" + producer["run_id"]))):
        fail("prepare_facts_archive_path_rejected")
    return {"format_version": 1, "kind": "readonly_prepare_facts_request", "source_sha": args.actual_source_sha,
        "operation_id": args.operation_id, "actual_run_id": args.run_id, "target_hash": TARGET_HASH,
        "database_scope": "mysql-and-mongodb", "observation_approval_sha256": args.bootstrap_approval_hash,
        "inventory_report": producer, "restore_engines": engines, "archive_directory": path}



def validate_prepare_facts_result(result, args, request, request_hash, code):
    fields(result, ("format_version", "kind", "operation", "prepare_mode", "source_sha", "operation_id", "run_id",
        "request_sha256", "observation_approval_sha256", "target_hash", "complete", "prepare_facts_observation_complete",
        "diagnostic_only", "execution_allowed", "drop_ready", "observed_inventory_producer", "prepare_source_files",
        "observed_ordered_mongo_schema_sha256", "observed_filesystems", "observed_socket_kind",
        "observation_elapsed_millis", "error_category"), ("observed_restore_engines", "prepare_facts_private_observation_sha256", "observed_ai_runtime"))
    if (type(result["format_version"]) is not int or result["format_version"] != 1 or
        result["kind"] != "readonly_prepare_facts_observation" or result["operation"] != "prepare" or
        result["prepare_mode"] != "prepare-facts" or result["source_sha"] != args.actual_source_sha or
        result["operation_id"] != args.operation_id or result["run_id"] != args.run_id or
        result["request_sha256"] != request_hash or result["observation_approval_sha256"] != args.bootstrap_approval_hash or
        result["target_hash"] != TARGET_HASH or result["observed_inventory_producer"] != request["inventory_report"] or
        any(result[k] is not False for k in ("complete", "execution_allowed", "drop_ready")) or
        result["diagnostic_only"] is not True or type(result["prepare_facts_observation_complete"]) is not bool or
        (result["prepare_facts_observation_complete"] is True) != (code == 0 and result["error_category"] == "none") or
        not re.fullmatch(r"(?:prepare_facts|lifecycle)_[a-z_]{1,100}|none", result["error_category"])):
        fail("prepare_facts_native_binding_rejected")
    uint(result["observation_elapsed_millis"])
    ai_runtime = result.get("observed_ai_runtime")
    if ai_runtime is not None:
        fields(ai_runtime, ("source_sha", "image_id", "container_id", "binding_sha256", "stop_constraints"))
        token(ai_runtime["source_sha"], SHA)
        token(ai_runtime["image_id"], re.compile(r"sha256:[0-9a-f]{64}"))
        for key in ("container_id", "binding_sha256"): token(ai_runtime[key], HASH)
        fields(ai_runtime["stop_constraints"], ("settings_sha256", "network_id"))
        for value in ai_runtime["stop_constraints"].values(): token(value, HASH)
    files = result["prepare_source_files"]
    if type(files) is not list or len(files) > 7:
        fail("prepare_facts_native_files_rejected")
    for index, value in enumerate(files):
        fields(value, ("name", "sha256", "bytes"))
        if value["name"] != PREPARE_SOURCE_NAMES[index]: fail("prepare_facts_native_files_rejected")
        token(value["sha256"], HASH); uint(value["bytes"])
    capacity = result["observed_filesystems"]
    scopes = ("source", "staging", "archive", "docker")
    if type(capacity) is not list or len(capacity) > 4: fail("prepare_facts_native_capacity_rejected")
    for index, value in enumerate(capacity):
        fields(value, ("scope", "path_sha256", "total_bytes", "available_bytes", "free_bytes"))
        if value["scope"] != scopes[index]: fail("prepare_facts_native_capacity_rejected")
        token(value["path_sha256"], HASH)
        for k in ("total_bytes", "available_bytes", "free_bytes"): uint(value[k])
        if value["available_bytes"] > value["total_bytes"] or value["free_bytes"] > value["total_bytes"]: fail("prepare_facts_native_capacity_rejected")
    if result["prepare_facts_observation_complete"]:
        token(result["observed_ordered_mongo_schema_sha256"], HASH)
        token(result.get("prepare_facts_private_observation_sha256"), HASH)
        if (ai_runtime is None or len(files) != 7 or len(capacity) != 4 or result.get("observed_restore_engines") != request["restore_engines"] or
            result["observed_socket_kind"] != "fixed_root_owned_unix_docker" or files[0]["sha256"] != request["inventory_report"]["sha256"]):
            fail("prepare_facts_native_incomplete")
    else:
        token(result["observed_ordered_mongo_schema_sha256"], re.compile(r"(?:[0-9a-f]{64})?"))
        if result.get("observed_restore_engines") is not None and result["observed_restore_engines"] != request["restore_engines"]:
            fail("prepare_facts_native_images_rejected")
        if result["observed_socket_kind"] not in ("", "fixed_root_owned_unix_docker"):
            fail("prepare_facts_native_socket_rejected")
    # Only allowlisted tokens go through the existing armored public transport.
    # The root-owned raw observation keeps the exact original filenames/IDs.
    for value in files: value["name"] = value["name"].replace(".", "_")
    if ai_runtime is not None:
        ai_runtime["image_id_sha256"] = ai_runtime.pop("image_id")[7:]
    if result.get("observed_restore_engines") is not None:
        engines = result["observed_restore_engines"]
        result["observed_restore_engines"] = {"mysql_image_id_sha256": engines["mysql_image_id"][7:],
            "mongodb_image_id_sha256": engines["mongodb_image_id"][7:], "architecture": engines["architecture"]}
    result["capabilities"] = {key: False for key in CAPABILITIES}
    return result



def live_prepare_facts(args):
    request = prepare_facts_request(args)
    directory = operation_directory(args.root, args.operation_id)
    raw = canonical_bytes(request); request_hash = hashlib.sha256(raw).hexdigest()
    with locked_operation(directory):
        create_bootstrap_file(directory, "prepare-facts-request-" + args.run_id + ".json", raw)
        args.prepare_facts_request_hash = request_hash
        code, native = root_once_lifecycle_prepare(args)
    result=validate_prepare_facts_result(decode(native), args, request, request_hash, code)
    observed=args.actual_preloaded_image
    fields(observed,('kind','tool_source_sha','original_source_sha','operation_id','actual_run_id','image_archive_sha256','image_id','os','architecture','revision','program_sha256','probe_id','probe_absent','temporary_files_zero','capabilities'))
    if observed['kind']!='native_cached_api_image_observation' or observed['tool_source_sha']!=args.actual_source_sha or observed['original_source_sha']!=request['inventory_report']['source_sha'] or observed['operation_id']!=args.operation_id or observed['actual_run_id']!=args.run_id or observed['revision']!=args.actual_source_sha or observed['os']!='linux' or observed['architecture']!='amd64' or observed['probe_absent'] is not True or observed['temporary_files_zero'] is not True or observed['capabilities']!={'deployment':False,'writer_fence':False,'drop':False}: fail('prepare_facts_image_preload_rejected')
    token(observed['image_id'],re.compile(r'sha256:[0-9a-f]{64}'))
    for key in ('image_archive_sha256','program_sha256','probe_id'):token(observed[key],HASH)
    result['observed_cached_api_image']={'image_id_sha256':observed['image_id'][7:],'program_sha256':observed['program_sha256'],'source_sha':observed['tool_source_sha'],'original_source_sha':observed['original_source_sha'],'operation_id':observed['operation_id'],'run_id':observed['actual_run_id'],'image_archive_sha256':observed['image_archive_sha256'],'probe_id':observed['probe_id'],'probe_absent':True,'temporary_files_zero':True,'os':'linux','architecture':'amd64'}
    return result


HOST_SCOPE_GAPS = frozenset(('absent_source_end_recheck_changed_or_unread', 'absent_source_end_recheck_unknown', 'account_home_source_unsupported', 'account_startup_indirect_execution_not_proven', 'account_startup_or_public_key_unread', 'activation_source_unread', 'all_match_authentication_domains_not_exhaustively_proven', 'directory_end_recheck_changed_or_unread', 'docker_socket_and_other_container_writer_admission_not_fenced', 'dynamic_key_or_principal_provider_not_exhaustively_proven', 'existing_sessions_not_drained_or_admission_fenced', 'external_database_and_qs_ai_writers_not_observed', 'external_or_conditional_nss_backend_not_exhaustively_proven', 'host_identity_read_unknown', 'host_namespace_read_unknown', 'indirect_activation_scripts_and_arbitrary_commands_unproven', 'local_accounts_schema_unknown', 'local_accounts_unread', 'local_groups_unread', 'local_runner_workflow_admission_not_fenced', 'login_session_listing_unread', 'login_session_schema_unknown', 'native_command_end_recheck_changed_or_unread', 'native_docker_roster_budget_exceeded', 'native_docker_roster_schema_unknown', 'native_docker_roster_unread', 'native_docker_selected_inspect_schema_unknown', 'native_docker_selected_inspect_unread', 'native_nss_accounts_schema_unknown', 'native_nss_differs_from_local_accounts', 'native_nss_enumeration_unread', 'native_systemd_listing_schema_unknown', 'native_systemd_listing_unread', 'native_systemd_properties_unread', 'native_systemd_unit_budget_exceeded', 'nss_sources_unread', 'observation_handle_close_failed', 'pam_and_dynamic_authentication_modules_not_exhaustively_proven', 'pam_authentication_source_unread', 'proc_roster_unread', 'process_cgroup_unread', 'process_changed_during_read', 'process_disappeared_or_stat_unread', 'process_end_recheck_changed_or_unread', 'process_executable_hash_unread', 'process_executable_unread', 'process_namespace_unread', 'process_stat_schema_unknown', 'process_status_unread', 'process_uid_schema_unknown', 'public_key_options_and_certificate_semantics_not_proven', 'source_activation_directory_unread', 'source_activation_symlink_target_not_followed', 'source_activation_symlink_unread', 'source_activation_tree_budget_exceeded', 'source_activation_tree_unread', 'source_end_recheck_changed_or_unread', 'source_sshd_configuration_unread', 'source_sshd_include_cycle_or_duplicate', 'source_sshd_include_directory_unread', 'source_sshd_include_path_unsupported', 'source_sshd_include_pattern_unknown', 'source_sshd_include_scope_or_budget_unknown', 'source_sshd_original_config_from_process_title_unproven', 'source_sshd_syntax_unknown', 'ssh_authorization_path_scope_unknown', 'ssh_key_path_expansion_requires_effective_subject', 'ssh_public_authorization_unread', 'sshd_argv_end_recheck_changed_or_unread', 'sshd_command_line_override_semantics_unproven', 'sshd_daemon_not_observed', 'sshd_loaded_configuration_snapshot_unproven', 'sshd_original_argv_unread', 'symlink_source_end_recheck_changed_or_unread', 'unclassified_processes_require_independent_writer_catalog', 'user_manager_runtime_socket_activation_not_exhaustively_proven', 'writer_admission_and_historical_platform_fence_not_installed'))
HOST_SCOPE_NAMES = frozenset({'ssh_configuration_sources','nss_accounts_and_key_sources','existing_sessions_and_processes','local_activation_sources'})
HOST_SCOPE_ERRORS = frozenset({'none','host_scope_incomplete','host_scope_root_once_required','host_scope_request_read_rejected','host_scope_schema_rejected','host_scope_binding_rejected','host_scope_private_observation_write_failed','host_scope_observation_incomplete','host_scope_read_budget_exceeded'})
# The native adapter has not read/validated approval before these finite refusals.
HOST_SCOPE_EARLY_ERRORS = frozenset({'host_scope_root_once_required','host_scope_request_read_rejected','host_scope_schema_rejected','host_scope_binding_rejected','host_scope_read_budget_exceeded'})

def host_scope_request(args):
    approval = decode(args.bootstrap_approval_json.encode('ascii') + b'\n')
    if hashlib.sha256(canonical_bytes(approval)).hexdigest() != args.bootstrap_approval_hash:
        fail('host_scope_approval_hash_rejected')
    fields(approval, ('format_version','kind','prepare_mode','source_sha','operation_id','target_hash','host_role'))
    if (type(approval['format_version']) is not int or approval['format_version'] != 1 or
        approval['kind'] != 'readonly_host_writer_scope_descriptor' or approval['prepare_mode'] != 'host-writer-scope' or
        approval['source_sha'] != args.actual_source_sha or approval['operation_id'] != args.operation_id or
        approval['target_hash'] != TARGET_HASH or approval['host_role'] != 'server_a'):
        fail('host_scope_approval_binding_rejected')
    return {'format_version':1,'kind':'readonly_host_writer_scope_request','source_sha':args.actual_source_sha,
        'operation_id':args.operation_id,'actual_run_id':args.run_id,'host_role':approval['host_role'],
        'target_hash':TARGET_HASH,'observation_approval_sha256':args.bootstrap_approval_hash}


def validate_host_scope_result(result,args,request,request_hash,code):
    fields(result, ('format_version','kind','operation','prepare_mode','source_sha','operation_id','run_id','host_role','source_uid',
        'request_sha256','observation_approval_sha256','target_hash','complete','diagnostic_only','execution_allowed','drop_ready',
        'host_observation_complete','writer_scope_complete','observed_machine_id_sha256','observed_boot_id_sha256',
        'observed_namespace_sha256','host_scope_private_observation_sha256','host_scope_catalog_sha256',
        'process_count','file_count','entry_count','observation_elapsed_millis','observed_scopes','unknown','error_category'))
    # Keep the native early diagnostic without inventing an unread approval.
    # Partial observations or successful calls still require the exact digest.
    early_refusal = (type(code) is int and code == 1 and result['error_category'] in HOST_SCOPE_EARLY_ERRORS and
        result['observation_approval_sha256'] == '' and result['host_observation_complete'] is False and
        all(result[k] == 0 for k in ('source_uid','process_count','file_count','entry_count')) and
        all(result[k] == '' for k in ('observed_machine_id_sha256','observed_boot_id_sha256','observed_namespace_sha256',
            'host_scope_private_observation_sha256','host_scope_catalog_sha256')) and
        result['observed_scopes'] == [] and result['unknown'] == [])
    if (result['format_version'] != 1 or type(result['format_version']) is not int or result['kind'] != 'readonly_host_writer_scope_observation' or
        result['operation'] != 'prepare' or result['prepare_mode'] != 'host-writer-scope' or result['source_sha'] != args.actual_source_sha or
        result['operation_id'] != args.operation_id or result['run_id'] != args.run_id or result['host_role'] != request['host_role'] or
        result['request_sha256'] != request_hash or (result['observation_approval_sha256'] != args.bootstrap_approval_hash and not early_refusal) or result['target_hash'] != TARGET_HASH or
        any(result[k] is not False for k in ('complete','execution_allowed','drop_ready','writer_scope_complete')) or result['diagnostic_only'] is not True or
        type(result['host_observation_complete']) is not bool or result['error_category'] not in HOST_SCOPE_ERRORS or
        result['host_observation_complete'] != (code == 0 and result['error_category'] == 'none')):
        fail('host_scope_native_binding_rejected')
    for k in ('source_uid','process_count','file_count','entry_count','observation_elapsed_millis'): uint(result[k])
    for k in ('observed_machine_id_sha256','observed_boot_id_sha256','observed_namespace_sha256','host_scope_private_observation_sha256','host_scope_catalog_sha256'):
        token(result[k], re.compile(r'(?:[0-9a-f]{64})?'))
    if result['host_observation_complete'] and result['observation_elapsed_millis'] >= 120000: fail('host_scope_native_budget_exceeded')
    if result['host_observation_complete'] and any(not result[k] for k in ('observed_machine_id_sha256','observed_boot_id_sha256','observed_namespace_sha256','host_scope_private_observation_sha256','host_scope_catalog_sha256')):
        fail('host_scope_native_incomplete')
    scopes=result['observed_scopes']
    if type(scopes) is not list or len(scopes)>4: fail('host_scope_native_schema_rejected')
    seen=set()
    for value in scopes:
        fields(value, ('name','enumeration_complete','recheck_equal','items','catalog_sha256','unknown'))
        if value['name'] not in HOST_SCOPE_NAMES or value['name'] in seen or any(type(value[k]) is not bool for k in ('enumeration_complete','recheck_equal')): fail('host_scope_native_schema_rejected')
        seen.add(value['name']);uint(value['items']);token(value['catalog_sha256'],HASH)
        if type(value['unknown']) is not list or len(value['unknown'])>32 or any(v not in HOST_SCOPE_GAPS for v in value['unknown']): fail('host_scope_native_schema_rejected')
    if type(result['unknown']) is not list or len(result['unknown'])>32 or any(v not in HOST_SCOPE_GAPS for v in result['unknown']): fail('host_scope_native_schema_rejected')
    if result['host_observation_complete'] and (len(scopes)!=4 or any(not v['enumeration_complete'] or not v['recheck_equal'] for v in scopes)): fail('host_scope_native_incomplete')
    result['capabilities']={key:False for key in CAPABILITIES}
    return result


def live_host_scope(args):
    request=host_scope_request(args)
    directory=operation_directory(args.root,args.operation_id)
    raw=canonical_bytes(request);request_hash=hashlib.sha256(raw).hexdigest()
    with locked_operation(directory):
        create_bootstrap_file(directory,'host-writer-scope-request-'+args.run_id+'.json',raw)
        args.host_scope_request_hash=request_hash
        code,native=root_once_lifecycle_prepare(args)
    return validate_host_scope_result(decode(native),args,request,request_hash,code)


DB_CENSUS_SECTION_NAMES = frozenset(('mysql_accounts','mysql_role_edges','mysql_default_roles','mysql_dynamic_grants','mysql_proxy_grants','mysql_observer_grants','mysql_connections','mysql_account_grants','mongodb_observer_privileges','mongodb_users','mongodb_databases','mongodb_stored_role_definitions','mongodb_roles','mongodb_authentication_configuration','mongodb_authentication_parameters','mongodb_connections_and_idle_operations','mongodb_local_logical_sessions','mongodb_persisted_logical_sessions'))
DB_CENSUS_GAPS = frozenset(name+'_unread' for name in DB_CENSUS_SECTION_NAMES) | frozenset(('mysql_full_process_privilege_unproven','mysql_external_authentication_and_direct_writer_admission_not_fenced','mongodb_other_nodes_sessions_and_external_authentication_unobserved','mongodb_external_direct_writer_admission_not_fenced','mongodb_startup_authorization_and_external_provider_configuration_unobserved'))
DB_CENSUS_SECTION_ERRORS = frozenset(('none','db_census_sql_query_failed_or_bounded','db_census_sql_account_grants_incomplete','db_census_sql_full_process_permission_unproven','db_census_mongo_query_failed_or_bounded','db_census_second_enumeration_missing'))
DB_CENSUS_ERRORS = frozenset(('none','db_census_incomplete','db_census_read_budget_exceeded','db_census_root_once_required','db_census_request_read_rejected','db_census_request_binding_rejected','db_census_original_identity_read_rejected','db_census_original_identity_binding_rejected','db_census_original_connection_failed','db_census_owner_close_failed','db_census_actual_identity_rejected','db_census_actual_identity_changed','db_census_original_identity_changed','db_census_private_catalog_write_failed','db_census_catalog_or_session_permissions_incomplete'))

def db_census_request(args):
    approval=decode(args.bootstrap_approval_json.encode('ascii')+b'\n')
    if hashlib.sha256(canonical_bytes(approval)).hexdigest()!=args.bootstrap_approval_hash: fail('db_census_approval_hash_rejected')
    fields(approval,('format_version','kind','prepare_mode','source_sha','operation_id','target_hash','database_scope','identity_report'))
    reference=approval['identity_report'];fields(reference,('operation_id','run_id','source_sha','sha256','request_sha256'))
    if (type(approval['format_version']) is not int or approval['format_version']!=1 or approval['kind']!='readonly_db_writer_census_descriptor' or approval['prepare_mode']!='db-writer-census' or approval['source_sha']!=args.actual_source_sha or approval['operation_id']!=args.operation_id or approval['target_hash']!=TARGET_HASH or approval['database_scope']!='mysql-and-mongodb' or reference['operation_id']!=args.operation_id or reference['run_id']==args.run_id): fail('db_census_approval_binding_rejected')
    token(reference['run_id'],RUN);token(reference['source_sha'],SHA);token(reference['sha256'],HASH);token(reference['request_sha256'],HASH)
    return {'format_version':1,'kind':'readonly_db_writer_census_request','source_sha':args.actual_source_sha,'operation_id':args.operation_id,'actual_run_id':args.run_id,'target_hash':TARGET_HASH,'observation_approval_sha256':args.bootstrap_approval_hash,'identity_report':reference}

def validate_db_census_result(result,args,request,request_hash,code):
    fields(result,('format_version','kind','operation','prepare_mode','source_sha','operation_id','run_id','source_uid','target_hash','request_sha256','observation_approval_sha256','observed_identity_producer','mysql_identity_sha256','mongodb_identity_sha256','mongodb_namespace_anchor_sha256','mysql_migration_version','mongodb_migration_version','complete','diagnostic_only','execution_allowed','drop_ready','db_census_observation_complete','mysql_all_connections_permission_proven','mongodb_local_all_sessions_permission_proven','all_nodes_sessions_coverage_complete','external_writer_coverage_complete','writer_scope_complete','db_census_private_catalog_sha256','db_census_catalog_sha256','observed_sections','unknown','observation_elapsed_millis','error_category'))
    if (type(result['format_version']) is not int or result['format_version']!=1 or result['kind']!='readonly_db_writer_census_observation' or result['operation']!='prepare' or result['prepare_mode']!='db-writer-census' or result['source_sha']!=args.actual_source_sha or result['operation_id']!=args.operation_id or result['run_id']!=args.run_id or result['request_sha256']!=request_hash or result['target_hash']!=TARGET_HASH or result['diagnostic_only'] is not True or any(result[k] is not False for k in ('complete','execution_allowed','drop_ready','writer_scope_complete','all_nodes_sessions_coverage_complete','external_writer_coverage_complete')) or any(type(result[k]) is not bool for k in ('mysql_all_connections_permission_proven','mongodb_local_all_sessions_permission_proven')) or type(result['db_census_observation_complete']) is not bool or result['error_category'] not in DB_CENSUS_ERRORS or result['db_census_observation_complete']!=(code==0 and result['error_category']=='none')): fail('db_census_native_binding_rejected')
    for key in ('source_uid','mysql_migration_version','mongodb_migration_version','observation_elapsed_millis'): uint(result[key])
    for key in ('mysql_identity_sha256','mongodb_identity_sha256','mongodb_namespace_anchor_sha256','db_census_private_catalog_sha256','db_census_catalog_sha256','observation_approval_sha256'): token(result[key],re.compile(r'(?:[0-9a-f]{64})?'))
    # A pre-identity rejection keeps empty producer facts; successful observations
    # require the exact original report reference, never a relabeled current run.
    observed=result['observed_identity_producer'];fields(observed,('operation_id','run_id','source_sha','sha256','request_sha256'))
    if observed!=request['identity_report'] and any(observed.values()): fail('db_census_native_identity_rejected')
    if result['observation_approval_sha256'] not in ('',args.bootstrap_approval_hash): fail('db_census_native_approval_rejected')
    sections=result['observed_sections']
    if type(sections) is not list or len(sections)>18: fail('db_census_native_schema_rejected')
    seen=set()
    for section in sections:
        fields(section,('name','enumeration_complete','recheck_equal','items','sha256','error_category'))
        if section['name'] not in DB_CENSUS_SECTION_NAMES or section['name'] in seen or any(type(section[k]) is not bool for k in ('enumeration_complete','recheck_equal')) or section['error_category'] not in DB_CENSUS_SECTION_ERRORS: fail('db_census_native_schema_rejected')
        seen.add(section['name']);uint(section['items']);token(section['sha256'],re.compile(r'(?:[0-9a-f]{64})?'))
        if section['enumeration_complete'] and (section['error_category']!='none' or not section['sha256']): fail('db_census_native_schema_rejected')
    if type(result['unknown']) is not list or len(result['unknown'])>32 or any(value not in DB_CENSUS_GAPS for value in result['unknown']): fail('db_census_native_schema_rejected')
    if result['db_census_observation_complete'] and (not result['mysql_all_connections_permission_proven'] or not result['mongodb_local_all_sessions_permission_proven'] or result['mysql_migration_version']!=99 or result['mongodb_migration_version']!=38 or result['observation_elapsed_millis']>=120000 or len(sections)!=18 or any(not s['enumeration_complete'] for s in sections) or observed!=request['identity_report'] or result['observation_approval_sha256']!=args.bootstrap_approval_hash or any(not result[k] for k in ('mysql_identity_sha256','mongodb_identity_sha256','mongodb_namespace_anchor_sha256','db_census_private_catalog_sha256','db_census_catalog_sha256'))): fail('db_census_native_incomplete')
    result['capabilities']={key:False for key in CAPABILITIES}
    return result

def live_db_census(args):
    request=db_census_request(args);directory=operation_directory(args.root,args.operation_id)
    raw=canonical_bytes(request);request_hash=hashlib.sha256(raw).hexdigest()
    with locked_operation(directory):
        create_bootstrap_file(directory,'db-writer-census-request-'+args.run_id+'.json',raw)
        args.db_census_request_hash=request_hash
        code,native=root_once_lifecycle_prepare(args)
    return validate_db_census_result(decode(native),args,request,request_hash,code)


def execute(args):
    if args.operation not in OPERATIONS:
        fail("operation_unsupported")
    token(args.approved_source_sha, SHA)
    token(args.actual_source_sha, SHA)
    token(args.run_id, RUN)
    if args.approved_source_sha != args.actual_source_sha:
        fail("source_revision_mismatch")
    mode = getattr(args, "prepare_mode", "inventory")
    identity_request = getattr(args, "identity_request_hash", "")
    inventory_request = getattr(args, "inventory_request_hash", "")
    bootstrap_json = getattr(args, "bootstrap_approval_json", "")
    bootstrap_hash = getattr(args, "bootstrap_approval_hash", "")
    lifecycle_request = getattr(args, "lifecycle_request_hash", "")
    if lifecycle_request:
        if identity_request or inventory_request or bootstrap_json or bootstrap_hash or mode != "lifecycle":
            fail("input_classes_mixed")
        return live_lifecycle(args, operation_directory(args.root, args.operation_id))
    if mode == "lifecycle":
        fail("lifecycle_request_approval_missing")
    if mode == "db-writer-census":
        if args.operation != "prepare" or args.manifest_hash or identity_request or inventory_request or not bootstrap_json or not bootstrap_hash: fail("input_classes_mixed")
        return live_db_census(args)
    if mode == "host-writer-scope":
        if args.operation != "prepare" or args.manifest_hash or identity_request or inventory_request or not bootstrap_json or not bootstrap_hash:
            fail("input_classes_mixed")
        return live_host_scope(args)
    if mode == "prepare-facts":
        if args.operation != "prepare" or args.manifest_hash or identity_request or inventory_request or not bootstrap_json or not bootstrap_hash:
            fail("input_classes_mixed")
        return live_prepare_facts(args)
    if mode == "report-diagnostic":
        if args.operation != "prepare" or args.manifest_hash or identity_request or inventory_request or not bootstrap_json or not bootstrap_hash:
            fail("input_classes_mixed")
        return report_diagnostic(args)
    if mode in BOOTSTRAP_MODES:
        if args.operation != "prepare" or args.manifest_hash or identity_request or inventory_request or not bootstrap_json or not bootstrap_hash:
            fail("input_classes_mixed")
        if mode in ("bootstrap-ai-bounds", "bootstrap-ai-verify", "historical-evidence-write", "historical-ai-bounds"):
            path = Path(__file__).with_name("compatibility-ai-history-prepare.py")
            spec = importlib.util.spec_from_file_location("compatibility_ai_history_prepare", path)
            module = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)
            return module.prepare(args, argparse.Namespace(**globals()))
        if mode in ("bootstrap-history", "bootstrap-history-metadata", "bootstrap-history-parent"):
            filename = "compatibility-history-parent.py" if mode == "bootstrap-history-parent" else "compatibility-history-prepare.py"
            module_name = "compatibility_history_parent" if mode == "bootstrap-history-parent" else "compatibility_history_prepare"
            path = Path(__file__).with_name(filename)
            spec = importlib.util.spec_from_file_location(module_name, path)
            module = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)
            entrypoint = module.prepare_metadata if mode == "bootstrap-history-metadata" else module.prepare
            return entrypoint(args, argparse.Namespace(**globals()))
        return bootstrap_private_request(args)
    if bootstrap_json or bootstrap_hash:
        fail("input_classes_mixed")
    if mode not in ("identity", "bounds", "inventory") or (identity_request and (args.operation != "prepare" or mode != "identity" or inventory_request or args.manifest_hash)) or (mode == "identity" and (args.operation != "prepare" or not identity_request)) or (mode == "bounds" and (args.operation != "prepare" or not inventory_request or args.manifest_hash)):
        fail("input_classes_mixed")
    if args.operation == "prepare" and mode == "identity":
        directory = bootstrap_identity_request(args)
        return live_inventory(args, directory)
    directory = operation_directory(args.root, args.operation_id)
    if args.operation == "prepare" and getattr(args, "inventory_request_hash", ""):
        if args.manifest_hash:
            fail("input_classes_mixed")
        return live_inventory(args, directory)
    if args.operation != "prepare" and getattr(args, "inventory_request_hash", ""):
        fail("input_classes_mixed")
    token(args.manifest_hash, HASH)
    manifest, digest = read_private(directory, "manifest.json", args.manifest_hash)
    validate_manifest(manifest, args.operation_id, args.approved_source_sha)
    now = datetime.datetime.now(datetime.timezone.utc)
    blockers = preparation(directory, manifest, now)
    if args.operation != "prepare":
        blockers.append(args.operation + "_stage_not_implemented")
    # Reaching here never means readiness. The live adapters are deliberately
    # unavailable until independently reviewed and wired to real producers.
    return {"format_version": 1, "operation": args.operation, "source_sha": args.actual_source_sha,
            "run_id": args.run_id, "operation_id": args.operation_id, "manifest_hash": digest,
            "target_hash": TARGET_HASH, "target_count": 4, "complete": False,
            "execution_allowed": False, "error_category": "preparation_blocked",
            "blockers": sorted(set(blockers)), "capabilities": CAPABILITIES.copy()}


def transport():
    candidates = (Path(__file__).with_name("receipt-transport.py"),
                  Path(__file__).parent.parent / "dbops" / "receipt-transport.py")
    path = next((path for path in candidates if path.is_file()), None)
    if path is None:
        fail("receipt_transport_unavailable")
    spec = importlib.util.spec_from_file_location("compatibility_receipt_transport", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def main(argv=None):
    parser = SafeParser(description=__doc__)
    parser.add_argument("--operation", required=True)
    parser.add_argument("--root", default="/opt/backups/qs-server/compatibility-retirement")
    parser.add_argument("--operation-id", required=True)
    parser.add_argument("--approved-source-sha", required=True)
    parser.add_argument("--actual-source-sha", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--manifest-hash", default="")
    parser.add_argument("--lifecycle-request-hash", default="")
    parser.add_argument("--inventory-request-hash", default="")
    parser.add_argument("--inventory-binary", default="")
    parser.add_argument("--history-binary", default="")
    parser.add_argument("--prepare-mode", default="inventory")
    parser.add_argument("--identity-request-hash", default="")
    parser.add_argument("--bootstrap-approval-json", default="")
    parser.add_argument("--bootstrap-approval-hash", default="")
    receipt = {"format_version": 1, "complete": False, "execution_allowed": False,
               "error_category": "input_invalid"}
    args = None
    try:
        args = parser.parse_args(argv)
        receipt = execute(args)
    except Blocked as error:
        receipt["error_category"] = str(error)
        if isinstance(error, NativeReceiptBlocked):
            receipt["native_diagnostic"] = error.native_diagnostic
    except Exception:
        receipt["error_category"] = "unexpected_preparation_failure"
    schema = {"format_version": "uint", "complete": "bool", "execution_allowed": "bool",
              "operation": OPERATIONS, "source_sha": "sha40", "run_id": "run_id", "operation_id": "run_id",
              "manifest_hash": "hash64", "target_hash": "hash64", "target_count": "uint",
              "kind": frozenset({"compatibility_retirement_lifecycle_result", "readonly_prepare_facts_observation", "readonly_host_writer_scope_observation", "readonly_db_writer_census_observation"}),
              "original_source_sha": "sha40", "manifest_sha256": "hash64", "request_sha256": "hash64", "archive_sha256": "hash64_or_empty",
              "isolated_content_restore_complete": "bool", "restore_elapsed_millis": "uint", "mysql_recovery_non_target_sha256": "hash64", "source_copy_intent_sha256": "hash64", "preparation_restore_zero_sha256": "hash64",
              "archive_binding_complete": "bool", "recovery_attempted": "bool", "recovery_complete": "bool",
              "acceptance_complete": "bool", "purge_complete": "bool", "required_adapters": [LIFECYCLE_ADAPTERS],
              "recovery_error_category": frozenset({receipt.get("recovery_error_category", "none")}),
              "prepare_facts_private_observation_sha256": "hash64", "prepare_facts_observation_complete": "bool", "observation_approval_sha256": "hash64_or_empty",
              "observed_inventory_producer": {"operation_id": "run_id", "run_id": "run_id", "source_sha": "sha40", "sha256": "hash64", "request_sha256": "hash64"},
              "prepare_source_files": [{"name": frozenset(name.replace(".", "_") for name in PREPARE_SOURCE_NAMES), "sha256": "hash64", "bytes": "uint"}],
              "observed_ordered_mongo_schema_sha256": "hash64_or_empty",
              "observed_restore_engines": {"mysql_image_id_sha256": "hash64", "mongodb_image_id_sha256": "hash64", "architecture": frozenset({"amd64", "arm64"})},
              "observed_cached_api_image": {"image_id_sha256":"hash64","program_sha256":"hash64","source_sha":"sha40","original_source_sha":"sha40","operation_id":"run_id","run_id":"run_id","image_archive_sha256":"hash64","probe_id":"hash64","probe_absent":"bool","temporary_files_zero":"bool","os":frozenset({"linux"}),"architecture":frozenset({"amd64"})},
              "observed_ai_runtime": {"source_sha": "sha40", "image_id_sha256": "hash64", "container_id": "hash64", "binding_sha256": "hash64", "stop_constraints": {"settings_sha256": "hash64", "network_id": "hash64"}},
              "observed_filesystems": [{"scope": frozenset({"source", "staging", "archive", "docker"}), "path_sha256": "hash64", "total_bytes": "uint", "available_bytes": "uint", "free_bytes": "uint"}],
              "db_census_observation_complete":"bool", "mysql_all_connections_permission_proven":"bool", "mongodb_local_all_sessions_permission_proven":"bool", "all_nodes_sessions_coverage_complete":"bool", "external_writer_coverage_complete":"bool", "db_census_private_catalog_sha256":"hash64_or_empty", "db_census_catalog_sha256":"hash64_or_empty",
              "observed_identity_producer":({key:frozenset({""}) for key in ("operation_id","run_id","source_sha","sha256","request_sha256")} if not any(receipt.get("observed_identity_producer",{}).values()) else {"operation_id":"run_id", "run_id":"run_id", "source_sha":"sha40", "sha256":"hash64", "request_sha256":"hash64"}),
              "mysql_identity_sha256":"hash64_or_empty", "mongodb_identity_sha256":"hash64_or_empty", "mongodb_namespace_anchor_sha256":"hash64_or_empty", "mysql_migration_version":"uint", "mongodb_migration_version":"uint",
              "observed_sections":[{"name":DB_CENSUS_SECTION_NAMES,"enumeration_complete":"bool","recheck_equal":"bool","items":"uint","sha256":"hash64_or_empty","error_category":DB_CENSUS_SECTION_ERRORS}],
              "host_role": frozenset({"server_a"}), "source_uid": "uint", "host_observation_complete": "bool", "writer_scope_complete": "bool",
              "observed_machine_id_sha256": "hash64_or_empty", "observed_boot_id_sha256": "hash64_or_empty", "observed_namespace_sha256": "hash64_or_empty",
              "host_scope_private_observation_sha256": "hash64_or_empty", "host_scope_catalog_sha256": "hash64_or_empty",
              "process_count": "uint", "file_count": "uint", "entry_count": "uint", "unknown": [HOST_SCOPE_GAPS | DB_CENSUS_GAPS],
              "observed_scopes": [{"name":HOST_SCOPE_NAMES, "enumeration_complete":"bool", "recheck_equal":"bool", "items":"uint", "catalog_sha256":"hash64", "unknown":[HOST_SCOPE_GAPS]}],
              "observed_socket_kind": frozenset({"", "fixed_root_owned_unix_docker"}), "observation_elapsed_millis": "uint",
              "inventory_complete": "bool", "inventory_private_report_hash": "hash64",
              "prepare_mode": frozenset({"identity", "bounds", "inventory", "report-diagnostic", "prepare-facts", "host-writer-scope", "db-writer-census"}) | BOOTSTRAP_MODES, "diagnostic_only": "bool", "drop_ready": "bool",
              "request_bootstrap_complete": "bool", "bootstrap_approval_sha256": "hash64", "derived_request_sha256": "hash64", "request_created_run_id": "run_id",
              "history_metadata_complete": "bool", "history_metadata_process_budget_proven": "bool",
              "metadata_private_report_sha256": "hash64", "metadata_created_run_id": "run_id",
              "approved_inventory_report": {"run_id": "run_id", "sha256": "hash64"}, "inventory_request_sha256": "hash64",
              "history_parent_proposal_sha256": "hash64", "parent_proposal_run_id": "run_id",
              "history_metadata_assets": [{"database": frozenset({"mysql", "mongodb"}), "name": frozenset(target[1] for target in TARGETS),
                   "full_file_sha256": "hash64", "full_file_bytes": "uint", "source_asset_sha256": "hash64"}],
              "history_parent_registration_complete": "bool", "history_parent_process_budget_proven": "bool",
              "history_parent_registration_sha256": "hash64", "approved_metadata_report": {"run_id": "run_id", "sha256": "hash64"},
              "history_parent_request_sha256": "hash64", "history_private_readiness_sha256": "hash64",
              "ai_host_readonly_complete": "bool", "ai_host_process_budget_proven": "bool",
              "original_source_sha": "sha40", "history_evidence_write_finished": "bool", "history_write_private_result_sha256": "hash64",
              "history_actual_sql_commit_response": "bool", "history_actual_mongo_commit_response": "bool",
              "history_commit_state": frozenset({"not_attempted", "sql_unknown_mongo_not_attempted", "sql_response_success_journal_unknown",
                  "sql_committed_mongo_not_attempted", "sql_committed_mongo_unknown", "both_responses_success_non_atomic", "sql_committed_mongo_not_required"}),
              "history_mongo_commit_requirement": frozenset({"undetermined", "required", "not_required"}),
              "history_prepared_pages": "uint", "history_readback_pages": "uint", "history_event_references": "uint",
              "history_ai_original_commands": "uint", "history_ai_source_references": "uint", "history_ai_command_persistence_complete": "bool",
              "ai_host_mode": frozenset({"bounds", "verify"}), "ai_host_private_readiness_sha256": "hash64",
              "ai_host_runtime_binding_sha256": "hash64", "ai_host_facts_sha256": "hash64",
              "ai_host_independent_epochs": "uint", "ai_host_originals": "uint", "ai_host_descriptor_sha256": "hash64",
              "ai_host_ai_bounds_sha256": "hash64", "ai_host_peer_bounds_sha256": "hash64",
              "history_readonly_complete": "bool", "history_independent_epochs": "uint",
              "history_local_candidates": "uint", "history_locally_qualified": "uint", "history_blocked_local": "uint",
              "history_ai_blocked_pages": "uint", "history_cas_complete": "bool", "history_process_budget_proven": "bool",
              "history_source_rows": ["uint"],
              "history_global_sql": {key: "uint" for key in ("observed", "retirement_related", "outside_retirement", "unknown", "blocking")},
              "history_global_mongodb": {key: "uint" for key in ("rows", "classified_rows", "blocking_reason_count", "coverage_gap_count")},
              "history_global_ai_reverse": {
                  **{key: "uint" for key in ("ledger_count", "rows", "retirement_related", "outside_retirement", "unknown", "blocking", "outside_active")},
                  **{key: "hash64_or_empty" for key in ("data_sha256", "source_scope_sha256")},
                  **{key: "bool" for key in ("whole_ledger_eof", "independent_epoch_rechecked")}},
              "approved_identity_report": {"run_id": "run_id", "source_sha": "sha40", "sha256": "hash64"},
              "approved_boundary_report": {"run_id": "run_id", "source_sha": "sha40", "sha256": "hash64"},
              "report_diagnostic_complete": "bool", "report_diagnostic_approval_sha256": "hash64",
              "observed_boundary_report": {"run_id": "run_id", "source_sha": "sha40", "sha256": "hash64", "request_sha256": "hash64"},
              "observed_inventory_report": {"run_id": "run_id", "source_sha": "sha40", "sha256": "hash64", "request_sha256": "hash64"},
              "inventory_request_hash": "hash64",
              "boundary_discovery_complete": "bool", "boundary_private_report_hash": "hash64", "boundary_request_hash": "hash64",
              "inventory_next_cycle_required": "bool", "inventory_boundary_report_hash": "nullable_hash64", "inventory_two_equal_scans": "bool",
              "identity_discovery_complete": "bool", "identity_private_report_hash": "hash64", "identity_request_hash": "hash64",
              "identity_database_states": {database: {**({"database_anchor_kind": frozenset({MONGO_NAMESPACE_PROFILE}), "database_anchor_uuid_set_sha256": "hash64", "database_anchor_kept_count": "uint"} if database == "mongodb" else {}), "identity_hash": "nullable_hash64", "database_anchor_hash": "nullable_hash64", "migration_generation_hash": "nullable_hash64", "identity_observed": "bool", "migration_version": "uint", "migration_head_observed": "bool", "migration_dirty": "nullable_bool", "migration_clean": "bool", "metadata_permissions_sufficient": "bool", "permission_scope": frozenset({"identity_and_migration_head"}), "error_category": IDENTITY_ERRORS[database]} for database in ("mysql", "mongodb")},
              "identity_diagnostic_histograms": [{"database": frozenset({"mysql", "mongodb"}), "name": frozenset(target[1] for target in TARGETS), "present": "nullable_bool", "complete": "bool", "diagnostic_only": "bool", "error_category": HISTOGRAM_ERRORS,
                   "bucket_count": "uint"}],
              "identity_histogram_bucket_pages": {"page_" + chr(97 + index): [{"object_index": "uint", "bucket_index": "uint", "type_label": frozenset(label.replace(".", "_") for label in HISTOGRAM_TYPES), "type_hash": "hash64", "state_label": HISTOGRAM_STATES, "state_hash": "hash64", "records": "uint"}] for index in range(4)},
              "inventory_database_error_categories": INVENTORY_REPORT_DIAGNOSTIC_ERRORS,
              "inventory_entrypoint_catalog_hash": "hash64",
              "runtime_image_id_sha256": "hash64", "runtime_network": frozenset({"infra_network"}),
              "inventory_present_targets": "uint", "inventory_records": "uint", "inventory_source_bytes": "uint",
              "inventory_database_states": {database: {**({"database_anchor_kind": frozenset({MONGO_NAMESPACE_PROFILE}), "database_anchor_uuid_set_sha256": "hash64", "database_anchor_kept_count": "uint"} if database == "mongodb" else {}), "identity_hash": "nullable_hash64", "database_anchor_hash": "nullable_hash64", "migration_generation_hash": "nullable_hash64", "migration_version": "uint",
                                                       "migration_head_observed": "bool", "migration_dirty": "nullable_bool",
                                                       "metadata_complete": "bool", "identity_match": "bool"}
                                            for database in ("mysql", "mongodb")},
              "native_diagnostic": {"process_completed":"bool", "exit_code":"uint", "termination_signal":"uint",
                  "stdout_bytes":"uint", "stderr_bytes":"uint", "stderr_sample_bytes":"uint",
                  "stderr_sample_sha256":"hash64", "stderr_sample_truncated":"bool"},
              "error_category": frozenset({receipt["error_category"]}),
              "blockers": [frozenset(receipt.get("blockers", ()))],
              "capabilities": {key: "bool" for key in CAPABILITIES}}
    emitted = False
    try:
        secrets = () if getattr(args, "prepare_mode", "") in ("bootstrap-history-metadata", "bootstrap-history-parent", "report-diagnostic") else tuple(os.environ.get(key, "") for key in ("MYSQL_USERNAME", "MYSQL_PASSWORD", "MONGODB_USERNAME", "MONGODB_PASSWORD",
                                                                          "MONGODB_METADATA_ADMIN_USERNAME", "MONGODB_METADATA_ADMIN_PASSWORD"))
        armor = transport().encode_armored_receipt(receipt, schema=schema, secrets=secrets)
        print(armor)
        emitted = True
    except Exception:
        # Fixed ASCII fallback contains no input and cannot be mistaken for a
        # valid framed receipt. Never print a raw protocol/debug alternative.
        print("compatibility_retirement_receipt_transport_failed", file=sys.stderr)
    diagnostic_complete = ((receipt.get("prepare_mode") == "prepare-facts" and receipt.get("prepare_facts_observation_complete") is True and receipt.get("diagnostic_only") is True and all(value is False for value in receipt.get("capabilities", {}).values())) or (receipt.get("prepare_mode") == "report-diagnostic" and receipt.get("report_diagnostic_complete") is True and receipt.get("diagnostic_only") is True and all(value is False for value in receipt.get("capabilities", {}).values())) or
        (receipt.get("prepare_mode") == "bootstrap-history" and receipt.get("history_readonly_complete") is True) or
        (receipt.get("prepare_mode") == "bootstrap-history-metadata" and receipt.get("history_metadata_complete") is True) or
        (receipt.get("prepare_mode") == "bootstrap-history-parent" and receipt.get("history_parent_registration_complete") is True and
         receipt.get("diagnostic_only") is True and receipt.get("history_cas_complete") is False and receipt.get("history_parent_process_budget_proven") is False) or
        (receipt.get("prepare_mode") in ("bootstrap-ai-bounds", "bootstrap-ai-verify", "historical-ai-bounds") and receipt.get("ai_host_readonly_complete") is True and
         receipt.get("diagnostic_only") is True and receipt.get("ai_host_process_budget_proven") is False and
         all(value is False for value in receipt.get("capabilities", {}).values())))
    evidence_finished = (receipt.get("prepare_mode") == "historical-evidence-write" and
        receipt.get("history_evidence_write_finished") is True and receipt.get("history_actual_sql_commit_response") is True and
        ((receipt.get("history_mongo_commit_requirement") == "required" and receipt.get("history_actual_mongo_commit_response") is True and
          receipt.get("history_commit_state") == "both_responses_success_non_atomic" and receipt.get("history_event_references", 0) > 0 and
          receipt.get("history_prepared_pages", 0) > 0) or
         (receipt.get("history_mongo_commit_requirement") == "not_required" and receipt.get("history_actual_mongo_commit_response") is False and
          receipt.get("history_commit_state") == "sql_committed_mongo_not_required" and receipt.get("history_event_references") == 0 and
          receipt.get("history_prepared_pages") == 0 and receipt.get("history_ai_original_commands", 0) > 0)) and
        receipt.get("history_prepared_pages") == receipt.get("history_readback_pages") and
        (receipt.get("history_ai_original_commands") == 0 or receipt.get("history_ai_command_persistence_complete") is True) and
        all(value is False for value in receipt.get("capabilities", {}).values()))
    lifecycle_complete = (receipt.get("kind") == "compatibility_retirement_lifecycle_result" and
        receipt.get("operation") == "prepare" and receipt.get("complete") is True and
        receipt.get("error_category") == "none" and receipt.get("isolated_content_restore_complete") is True and
        receipt.get("execution_allowed") is False and receipt.get("drop_ready") is False and
        all(value is False for value in receipt.get("capabilities", {}).values()))
    return 0 if emitted and (lifecycle_complete or ((diagnostic_complete or evidence_finished) and receipt.get("complete") is False and receipt.get("execution_allowed") is False and receipt.get("drop_ready") is False)) else 42


if __name__ == "__main__":
    sys.exit(main())
