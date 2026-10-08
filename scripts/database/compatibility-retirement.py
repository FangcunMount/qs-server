#!/usr/bin/env python3
"""Fail-closed preparation contract for the four compatibility namespaces.

This A-stage tool validates private, hash-bound evidence and provides a durable
DDL journal. It does not implement a production database backend. In particular,
operator-supplied booleans cannot turn an unimplemented fence/history/restore
verifier into an execution permit. No raw payload, credentials, or rejected
input is printed. See compatibility-retirement.md for the remaining adapters.
"""

import argparse
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
ROOT_SUFFIX = ("backups", "qs-server", "compatibility-retirement")
SHA = re.compile(r"^[0-9a-f]{40}$")
HASH = re.compile(r"^[0-9a-f]{64}$")
RUN = re.compile(r"^[0-9]{1,20}-[0-9]{1,4}$")
NAME = re.compile(r"^[a-z][a-z0-9_-]{0,80}\.json$")
MAX_JSON = 256 * 1024
MAX_WINDOW_SECONDS = 1800
FORWARD_STOP_SECONDS = 1200
CAPABILITIES = {
    "manifest_validation": True,
    "immutable_evidence_validation": True,
    "durable_ddl_journal": True,
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
JOURNAL_STATES = frozenset({"pending", "intent", "unknown", "dropped", "restored"})
HISTOGRAM_TYPES = frozenset({"unknown_type", "request", "change", "cancel", "prepare", "start", "answer",
    "answersheet.submitted", "evaluation.requested", "evaluation.retry.requested", "evaluation.outcome.committed", "evaluation.failed", "interpretation.report.generated", "interpretation.report.failed", "interpretation.retry.requested", "task.opened.reminder.requested",
    "footprint.entry_opened", "footprint.intake_confirmed", "footprint.testee_profile_created", "footprint.care_relationship_established", "footprint.care_relationship_transferred", "footprint.answersheet_submitted", "footprint.assessment_created", "footprint.report_generated",
    "interpretation.ai_explanation.requested", "interpretation.ai_explanation.retry.requested", "interpretation.ai_explanation.lease_recovery.requested", "interpretation.ai_explanation.generated", "interpretation.ai_explanation.failed", "interpretation.ai_explanation.prompt_evaluation.step_requested",
    "assessment.submitted", "assessment.evaluated", "assessment.interpreted", "assessment.failed", "report.generated", "questionnaire.changed", "scale.changed", "task.opened", "task.completed", "task.expired", "task.canceled"})
HISTOGRAM_STATES = frozenset({"unknown_state", "pending", "publishing", "published", "failed", "quarantined", "retry_wait", "0", "1", "historical_mapping"})
HISTOGRAM_ERRORS = frozenset({"none", "identity_metadata_not_ready", "histogram_namespace_kind_rejected", "histogram_schema_rejected", "histogram_query_failed_or_timed_out", "histogram_bucket_bound_exceeded", "histogram_public_bound_exceeded", "histogram_count_invalid", "histogram_metadata_query_failed", "mysql_metadata_read_failed", "metadata_bound_exceeded", "connection_input_invalid", "mongo_connection_failed", "mongo_close_failed"})


class Blocked(ValueError):
    """Fixed error categories; never include private input in the message."""


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
    if filename not in ("identity.private.json", "inventory.private.json"):
        token(filename, NAME)
    if expected_hash is not None:
        token(expected_hash, HASH)
    try:
        fd = os.open(directory / filename, os.O_RDONLY | os.O_NOFOLLOW)
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


def validate_identity_request(value, operation_id, source_sha):
    fields(value, ("format_version", "kind", "operation_id", "source_sha", "target_hash", "database_scope", "identity_protocols", "limits"))
    if type(value["format_version"]) is not int or value["format_version"] != 1 or value["kind"] != "readonly_identity_discovery_request":
        fail("identity_request_class_invalid")
    validate_binding(value, operation_id, source_sha)
    if value["database_scope"] != "mysql-and-mongodb" or value["target_hash"] != TARGET_HASH:
        fail("target_allowlist_mismatch")
    if value["identity_protocols"] != {"mysql": "mysql_database_identity_v1", "mongodb": "mongodb_database_identity_v1"} or value["limits"] != {"query_seconds": 15, "total_seconds": 90}:
        fail("identity_request_protocol_or_limits_invalid")


def identity_request_bytes(operation_id, source_sha):
    token(operation_id, RUN); token(source_sha, SHA)
    value = {"format_version": 1, "kind": "readonly_identity_discovery_request",
             "operation_id": operation_id, "source_sha": source_sha,
             "target_hash": TARGET_HASH, "database_scope": "mysql-and-mongodb",
             "identity_protocols": {"mysql": "mysql_database_identity_v1", "mongodb": "mongodb_database_identity_v1"},
             "limits": {"query_seconds": 15, "total_seconds": 90}}
    return json.dumps(value, sort_keys=True, separators=(",", ":"), allow_nan=False).encode("ascii") + b"\n"


def bootstrap_identity_request(args):
    # Request bytes contain no observed/expected identity and no credential.
    # The caller approves their exact hash independently before any directory
    # creation or connection. Never generate an inventory request from discovery.
    raw = identity_request_bytes(args.operation_id, args.actual_source_sha)
    token(args.identity_request_hash, HASH)
    if hashlib.sha256(raw).hexdigest() != args.identity_request_hash:
        fail("identity_request_hash_mismatch")
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


def capture_fixed(command, *, timeout, maximum=32768):
    # Child error output may contain connection strings; it is never relayed.
    try:
        result = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                                timeout=timeout, check=False)
    except (OSError, subprocess.TimeoutExpired):
        fail("inventory_runtime_failed")
    if len(result.stdout) > maximum:
        fail("inventory_output_bound_exceeded")
    return result.returncode, result.stdout


def live_inventory(args, directory):
    mode = getattr(args, "prepare_mode", "inventory")
    request_hash = args.identity_request_hash if mode == "identity" else args.inventory_request_hash
    request_name = "identity-request.json" if mode == "identity" else "inventory-request.json"
    token(request_hash, HASH)
    request, _ = read_private(directory, request_name, request_hash)
    validator = validate_identity_request if mode == "identity" else validate_inventory_request
    validator(request, args.operation_id, args.actual_source_sha)
    entrypoints, entrypoint_hash = read_private(Path(__file__).parent, "compatibility-retirement-entrypoints.json")
    if entrypoints.get("format_version") != 1 or entrypoints.get("kind") != "source_only_production_entrypoint_catalog" or entrypoints.get("live_fence_proven") is not False or entrypoints.get("historical_rerun_proven_denied") is not False or len(entrypoints.get("entrypoints", ())) != 12:
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
    keys = ("MYSQL_HOST", "MYSQL_PORT", "MYSQL_USERNAME", "MYSQL_PASSWORD", "MYSQL_DATABASE",
            "MONGODB_HOST", "MONGODB_PORT", "MONGODB_USERNAME", "MONGODB_PASSWORD", "MONGODB_DBNAME")
    values = {}
    for key in keys:
        value = os.environ.get(key, "") or ({"MYSQL_PORT": "3306", "MONGODB_PORT": "27017"}.get(key, ""))
        if not value or len(value) > 4096 or any(character in value for character in ("\n", "\r", "\x00")):
            fail("inventory_connection_input_invalid")
        values[key] = value
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
                       "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--cpus=0.5",
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
            code, raw = capture_fixed(command, timeout=210, maximum=MAX_JSON)
    summary = decode(raw)
    if mode == "identity":
        receipt = validate_identity_receipt(summary, code, args, output, request_hash, entrypoint_hash)
        receipt["runtime_image_id_sha256"] = image.removeprefix("sha256:")
        receipt["runtime_network"] = "infra_network"
        return receipt
    fields(summary, ("format_version", "kind", "source_sha", "operation_id", "run_id", "request_hash",
                     "target_hash", "complete", "drop_ready", "error_category", "database_bindings", "targets", "private_report_hash"))
    if summary["format_version"] != 1 or summary["kind"] != "readonly_compatibility_inventory" or summary["source_sha"] != args.actual_source_sha or summary["operation_id"] != args.operation_id or summary["run_id"] != args.run_id or summary["request_hash"] != args.inventory_request_hash or summary["target_hash"] != TARGET_HASH or summary["drop_ready"] is not False:
        fail("inventory_receipt_binding_mismatch")
    report, report_hash = read_private(output, "inventory.private.json")
    if summary["private_report_hash"] != report_hash:
        fail("inventory_receipt_private_mismatch")
    # Bind the exact private file and every public field; never trust stdout's
    # complete flag alone. Source bodies are never copied into the receipt.
    for key in ("format_version", "kind", "source_sha", "operation_id", "run_id", "request_hash", "target_hash", "complete", "drop_ready", "error_category", "database_bindings"):
        if report.get(key) != summary[key]:
            fail("inventory_receipt_private_mismatch")
    if type(summary["complete"]) is not bool or summary["complete"] != (code == 0) or (summary["complete"] and summary["error_category"] != "none"):
        fail("inventory_receipt_outcome_invalid")
    objects = report.get("targets")
    if type(objects) is not list or len(objects) > 4:
        fail("inventory_receipt_target_invalid")
    if summary["complete"] and len(objects) != 4:
        fail("inventory_receipt_target_invalid")
    identities = set()
    for item in objects:
        expected = next((target for target in TARGETS if target[:2] == (item.get("database"), item.get("name"))), None)
        if expected is None or item.get("kind") != expected[2]:
            fail("inventory_receipt_target_invalid")
        identity = expected[:2]
        if identity in identities:
            fail("inventory_receipt_target_invalid")
        identities.add(identity)
        for key in ("records", "bytes"):
            uint(item.get(key))
        if type(item.get("present")) is not bool or type(item.get("complete")) is not bool:
            fail("inventory_receipt_target_invalid")
        if summary["complete"]:
            if not item["complete"] or item["error_category"] != "none":
                fail("inventory_receipt_outcome_invalid")
            for key in ("schema_hash", "data_hash", "identity_hash"):
                token(item.get(key), HASH)
    if summary["complete"]:
        fields(report["database_bindings"], ("mysql", "mongodb"))
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
        database_states[database] = {"identity_hash": identity or None,
                                     "migration_version": version,
                                     "migration_head_observed": version > 0,
                                     "migration_dirty": binding.get("migration_dirty") if version > 0 else None,
                                     "metadata_complete": binding.get("metadata_complete") is True,
                                     "identity_match": binding.get("expected_identity_match") is True}
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
            "inventory_database_states": database_states,
            "blockers": ["history_verifier_not_implemented", "production_fence_unproven", "backup_restore_backend_not_implemented",
                         "prepared_release_backend_not_implemented", "live_acceptance_verifier_not_implemented"],
            "capabilities": CAPABILITIES.copy()}


def validate_identity_receipt(summary, code, args, output, request_hash, entrypoint_hash):
    keys = ("format_version", "kind", "source_sha", "operation_id", "run_id", "request_hash", "target_hash", "diagnostic_only", "drop_ready", "complete", "identity_protocols", "database_states", "diagnostic_histograms", "error_category")
    fields(summary, (*keys, "private_report_hash"))
    report, digest = read_private(output, "identity.private.json", summary["private_report_hash"])
    if any(report.get(key) != summary[key] for key in keys):
        fail("identity_receipt_private_mismatch")
    if summary["format_version"] != 1 or summary["kind"] != "readonly_identity_discovery" or summary["source_sha"] != args.actual_source_sha or summary["operation_id"] != args.operation_id or summary["run_id"] != args.run_id or summary["request_hash"] != request_hash or summary["target_hash"] != TARGET_HASH or summary["diagnostic_only"] is not True or summary["drop_ready"] is not False or summary["identity_protocols"] != {"mysql": "mysql_database_identity_v1", "mongodb": "mongodb_database_identity_v1"}:
        fail("identity_receipt_binding_mismatch")
    if type(summary["complete"]) is not bool or summary["complete"] != (code == 0):
        fail("identity_receipt_outcome_invalid")
    fields(summary["database_states"], ("mysql", "mongodb"))
    clean_states = {}
    for database, state in summary["database_states"].items():
        fields(state, ("identity_hash", "identity_observed", "migration_version", "migration_head_observed", "migration_dirty", "migration_clean", "metadata_permissions_sufficient", "permission_scope", "error_category"))
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
        clean_states[database] = {key: state[key] for key in state if key != "error_category"}
        clean_states[database]["identity_hash"] = state["identity_hash"] or None
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


def durable_json(directory, filename, value):
    if filename not in ("ddl-journal.json",):
        fail("journal_filename_invalid")
    private_directory(directory)
    raw = (json.dumps(value, sort_keys=True, separators=(",", ":"), allow_nan=False) + "\n").encode()
    temporary = directory / (filename + ".partial")
    fd = None
    try:
        fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, "wb", closefd=False) as stream:
            stream.write(raw)
            stream.flush()
            os.fsync(fd)
        os.close(fd)
        fd = None
        destination = directory / filename
        if destination.exists() or destination.is_symlink():
            read_private(directory, filename)
        os.replace(temporary, destination)
        parent = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            os.fsync(parent)
        finally:
            os.close(parent)
    except Blocked:
        raise
    except OSError:
        fail("journal_persistence_failed")
    finally:
        if fd is not None:
            os.close(fd)
        # Never remove an existing .partial belonging to another attempt.
        # A failed persist leaves its intent evidence for an operator.


class DDLJournal:
    """Exact-target, fsync-before-DDL state machine; requires an external lock.

    There is intentionally no automatic resume/retry for intent or unknown.
    Reconciliation must perform a new read-only observation. This journal is
    available to the future backend but is never invoked by the A-stage CLI.
    """

    def __init__(self, directory, manifest_hash, operation_id, source_sha):
        self.directory = private_directory(directory)
        token(manifest_hash, HASH)
        token(operation_id, RUN)
        token(source_sha, SHA)
        self.value = {"format_version": 1, "operation_id": operation_id, "source_sha": source_sha,
                      "manifest_hash": manifest_hash, "target_hash": TARGET_HASH,
                      "states": ["pending"] * 4}
        path = directory / "ddl-journal.json"
        if path.exists() or path.is_symlink():
            value, _ = read_private(directory, "ddl-journal.json")
            fields(value, self.value.keys())
            if any(value[key] != self.value[key] for key in self.value if key != "states"):
                fail("journal_binding_mismatch")
            if type(value["states"]) is not list or len(value["states"]) != 4 or any(type(state) is not str or state not in JOURNAL_STATES for state in value["states"]):
                fail("journal_state_invalid")
            self.value = value

    def transition(self, index, allowed, result):
        if type(index) is not int or not 0 <= index < 4 or result not in JOURNAL_STATES:
            fail("journal_state_invalid")
        if self.value["states"][index] not in allowed:
            fail("journal_transition_rejected")
        proposed = dict(self.value, states=list(self.value["states"]))
        proposed["states"][index] = result
        durable_json(self.directory, "ddl-journal.json", proposed)
        self.value = proposed

    def begin_drop(self, index):
        self.transition(index, {"pending"}, "intent")

    def mark_unknown(self, index):
        self.transition(index, {"intent"}, "unknown")

    def observe_absent(self, index):
        # Called only after a live read confirms absence. This does not permit
        # another DDL attempt, and a mixed journal does not permit purge.
        self.transition(index, {"intent", "unknown"}, "dropped")

    def observe_restored(self, index):
        self.transition(index, {"dropped"}, "restored")

    def require_all_dropped(self):
        if self.value["states"] != ["dropped"] * 4:
            fail("ddl_ledger_incomplete")


def deadline(started_monotonic, *, clock=time.monotonic, recovering=False):
    elapsed = clock() - started_monotonic
    if elapsed < 0:
        fail("maintenance_clock_invalid")
    ceiling = MAX_WINDOW_SECONDS if recovering else FORWARD_STOP_SECONDS
    if elapsed >= ceiling:
        fail("maintenance_deadline_exceeded")


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
    if mode not in ("identity", "inventory") or (identity_request and (args.operation != "prepare" or mode != "identity" or inventory_request or args.manifest_hash)) or (mode == "identity" and (args.operation != "prepare" or not identity_request)):
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
    parser.add_argument("--inventory-request-hash", default="")
    parser.add_argument("--inventory-binary", default="")
    parser.add_argument("--prepare-mode", default="inventory")
    parser.add_argument("--identity-request-hash", default="")
    receipt = {"format_version": 1, "complete": False, "execution_allowed": False,
               "error_category": "input_invalid"}
    try:
        args = parser.parse_args(argv)
        receipt = execute(args)
    except Blocked as error:
        receipt["error_category"] = str(error)
    except Exception:
        receipt["error_category"] = "unexpected_preparation_failure"
    schema = {"format_version": "uint", "complete": "bool", "execution_allowed": "bool",
              "operation": OPERATIONS, "source_sha": "sha40", "run_id": "run_id", "operation_id": "run_id",
              "manifest_hash": "hash64", "target_hash": "hash64", "target_count": "uint",
              "inventory_complete": "bool", "inventory_private_report_hash": "hash64",
              "prepare_mode": frozenset({"identity", "inventory"}), "diagnostic_only": "bool", "drop_ready": "bool",
              "identity_discovery_complete": "bool", "identity_private_report_hash": "hash64", "identity_request_hash": "hash64",
              "identity_database_states": {database: {"identity_hash": "nullable_hash64", "identity_observed": "bool", "migration_version": "uint", "migration_head_observed": "bool", "migration_dirty": "nullable_bool", "migration_clean": "bool", "metadata_permissions_sufficient": "bool", "permission_scope": frozenset({"identity_and_migration_head"})} for database in ("mysql", "mongodb")},
              "identity_diagnostic_histograms": [{"database": frozenset({"mysql", "mongodb"}), "name": frozenset(target[1] for target in TARGETS), "present": "nullable_bool", "complete": "bool", "diagnostic_only": "bool", "error_category": HISTOGRAM_ERRORS,
                   "bucket_count": "uint"}],
              "identity_histogram_bucket_pages": {"page_" + chr(97 + index): [{"object_index": "uint", "bucket_index": "uint", "type_label": frozenset(label.replace(".", "_") for label in HISTOGRAM_TYPES), "type_hash": "hash64", "state_label": HISTOGRAM_STATES, "state_hash": "hash64", "records": "uint"}] for index in range(4)},
              "inventory_entrypoint_catalog_hash": "hash64",
              "runtime_image_id_sha256": "hash64", "runtime_network": frozenset({"infra_network"}),
              "inventory_present_targets": "uint", "inventory_records": "uint", "inventory_source_bytes": "uint",
              "inventory_database_states": {database: {"identity_hash": "nullable_hash64", "migration_version": "uint",
                                                       "migration_head_observed": "bool", "migration_dirty": "nullable_bool",
                                                       "metadata_complete": "bool", "identity_match": "bool"}
                                            for database in ("mysql", "mongodb")},
              "error_category": frozenset({receipt["error_category"]}),
              "blockers": [frozenset(receipt.get("blockers", ()))],
              "capabilities": {key: "bool" for key in CAPABILITIES}}
    try:
        secrets = tuple(os.environ.get(key, "") for key in ("MYSQL_USERNAME", "MYSQL_PASSWORD", "MONGODB_USERNAME", "MONGODB_PASSWORD"))
        armor = transport().encode_armored_receipt(receipt, schema=schema, secrets=secrets)
        print(armor)
    except Exception:
        # Fixed ASCII fallback contains no input and cannot be mistaken for a
        # valid framed receipt. Never print a raw protocol/debug alternative.
        print("compatibility_retirement_receipt_transport_failed", file=sys.stderr)
    return 42


if __name__ == "__main__":
    sys.exit(main())
