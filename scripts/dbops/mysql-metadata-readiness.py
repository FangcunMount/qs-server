#!/usr/bin/env python3
"""Fixed MySQL metadata/scalar readiness; no freeze, backup or mutation authority."""
import importlib.util
import json
from pathlib import Path


def load_profile():
    spec = importlib.util.spec_from_file_location("readiness_profile_primitives", Path(__file__).with_name("mysql-metadata-profile.py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


profile = load_profile()
READINESS = True
SHA, RUN, HASH = profile.SHA, profile.RUN, profile.HASH
PROFILE_KEYS = profile.PROFILE_KEYS
runtime_module = profile.runtime_module
receipt_transport_module = profile.receipt_transport_module
capture_private = profile.capture_private

STAGES = frozenset({"none", "input", "connect", "identity_before", "global_grants", "migration_before", "tables", "views", "routines", "triggers", "events", "columns", "checks", "threads", "transactions", "locks", "instrument", "assessment_distribution", "outcome_distribution", "migration_after", "identity_after", "connection_release", "database_release", "transport"})
ERRORS = frozenset({"none", "input_invalid", "connection_input_invalid", "connection_config_failed", "connection_failed", "target_identity_mismatch", "target_hash_mismatch", "identity_read_failed", "identity_final_failed", "session_identity_changed", "query_failed", "metadata_invalid", "metadata_truncated", "metadata_release_failed", "grants_unverified", "migration_pair_rejected", "storage_observation_unavailable", "connection_release_failed", "database_release_failed", "profile_input_preflight_failed", "profile_package_preflight_failed", "profile_trusted_ssh_preflight_failed", "profile_remote_or_receipt_failed", "profile_transport_or_receipt_failed"})
CATALOG_KEYS = frozenset({"tables", "views", "routines", "triggers", "events", "columns", "checks"})
PARTITION_KEYS = frozenset({"total", "sql_null", "non_null", "json_null", "empty_object", "non_empty_object", "other"})
MEASUREMENTS_SCHEMA = {
    "global_metadata_grant_rows_verified": "bool", "migration_version": "uint", "migration_dirty": "bool",
    "performance_schema_enabled": "bool", "event_scheduler": frozenset({"on", "off", "disabled"}),
    "catalogs": {key: {"count": "uint", "digest": "hash64"} for key in CATALOG_KEYS},
    "enabled_events": "uint", "threads": "uint", "event_threads": "uint", "transactions": "uint",
    "modifying_transactions": "uint", "metadata_locks": "uint", "mdl_instrument_enabled": "bool",
    "assessment_distribution": {key: "uint" for key in PARTITION_KEYS},
    "outcome_distribution": {key: "uint" for key in PARTITION_KEYS},
}
RECEIPT_SCHEMA = {
    "format_version": "uint", "source_sha": "sha40", "run_id": "run_id", "expected_target_hash": "hash64",
    "source_target_hash": "hash64", "diagnostic_only": "bool", "readiness_only": "bool", "freeze_verified": "bool",
    "complete": "bool", "error_category": ERRORS, "failure_stage": STAGES, "query_count": "uint",
    "connection_released": "nullable_bool", "database_released": "nullable_bool", "pool_closed_verified": "nullable_bool",
    "measurements": MEASUREMENTS_SCHEMA,
}
FAILURE_RECEIPT = {"format_version": 1, "diagnostic_only": True, "readiness_only": True, "freeze_verified": False,
                   "complete": False, "error_category": "profile_transport_or_receipt_failed", "failure_stage": "transport",
                   "query_count": 0, "connection_released": None, "database_released": None, "pool_closed_verified": None}


def stage_failure(env, category):
    value = dict(FAILURE_RECEIPT)
    value["error_category"] = category if category in ERRORS else "profile_transport_or_receipt_failed"
    for key, expression in (("source_sha", SHA), ("run_id", RUN), ("expected_target_hash", HASH)):
        source = {"source_sha": "PROFILE_SOURCE_SHA", "run_id": "PROFILE_RUN_ID", "expected_target_hash": "PROFILE_EXPECTED_TARGET_HASH"}[key]
        if expression.fullmatch(env.get(source, "")):
            value[key] = env[source]
    return value


def conserved_partition(value):
    if type(value) is not dict or set(value) != PARTITION_KEYS:
        return False
    return value["total"] == value["sql_null"] + value["non_null"] and value["non_null"] == sum(value[key] for key in ("json_null", "empty_object", "non_empty_object", "other"))


def safe_receipt(module, raw, code, sha, run, expected):
    value = module.load_json_bytes(raw)
    # Existing armor's nested field/type/size bounds also validate native JSON.
    receipt_transport_module().encode_armored_receipt(value, schema=RECEIPT_SCHEMA)
    required = set(RECEIPT_SCHEMA) - {"source_target_hash", "measurements"}
    if not required <= set(value) or value["format_version"] != 1 or value["source_sha"] != sha or value["run_id"] != run or value["expected_target_hash"] != expected:
        raise ValueError("readiness_receipt_binding")
    if value["diagnostic_only"] is not True or value["readiness_only"] is not True or value["freeze_verified"] is not False or value["query_count"] > 18:
        raise ValueError("readiness_receipt_scope")
    if value["complete"]:
        if code != 0 or value["error_category"] != "none" or value["failure_stage"] != "none" or value.get("source_target_hash") != expected or value["query_count"] != 18:
            raise ValueError("readiness_receipt_success")
        if any(value[key] is not True for key in ("connection_released", "database_released", "pool_closed_verified")):
            raise ValueError("readiness_release_unverified")
        m = value.get("measurements")
        if type(m) is not dict or set(m) != set(MEASUREMENTS_SCHEMA) or set(m["catalogs"]) != CATALOG_KEYS:
            raise ValueError("readiness_measurements_incomplete")
        if m["migration_version"] != 100 or m["migration_dirty"] is not False or any(m[key] is not True for key in ("global_metadata_grant_rows_verified", "performance_schema_enabled", "mdl_instrument_enabled")):
            raise ValueError("readiness_capability_unverified")
        if any(set(summary) != {"count", "digest"} or summary["count"] > 1000 for summary in m["catalogs"].values()) or m["catalogs"]["columns"]["count"] != 2 or m["catalogs"]["checks"]["count"] != 2 or m["catalogs"]["tables"]["count"] < 3 or m["catalogs"]["views"]["count"] > m["catalogs"]["tables"]["count"]:
            raise ValueError("readiness_catalog_incomplete")
        if any(m[key] > 1000 for key in ("threads", "transactions", "metadata_locks")) or m["event_threads"] > m["threads"] or m["modifying_transactions"] > m["transactions"] or m["enabled_events"] > m["catalogs"]["events"]["count"]:
            raise ValueError("readiness_observation_invalid")
        if not all(conserved_partition(m[key]) for key in ("assessment_distribution", "outcome_distribution")):
            raise ValueError("readiness_partition_invalid")
    elif code == 0 or value["error_category"] == "none" or value["failure_stage"] == "none" or "measurements" in value:
        raise ValueError("readiness_failure_retained_measurements")
    return value


def execute_native(binary, env, module):
    sha, run, expected = (env.get(key, "") for key in ("PROFILE_SOURCE_SHA", "PROFILE_RUN_ID", "PROFILE_EXPECTED_TARGET_HASH"))
    if not SHA.fullmatch(sha) or not RUN.fullmatch(run) or not HASH.fullmatch(expected):
        raise ValueError("input_invalid")
    module.validate_source(env)
    path = Path(binary)
    if not path.is_absolute() or path.is_symlink() or not path.is_file():
        raise ValueError("binary_invalid")
    private = {key: env.get(key, "") for key in PROFILE_KEYS}
    private["MYSQL_PORT"] = private["MYSQL_PORT"] or "3306"
    if private["MYSQL_DATABASE"] != "qs":
        raise ValueError("readiness_database_binding")
    code, raw = capture_private([str(path), "--metadata-readiness"], env=private)
    return code, safe_receipt(module, raw, code, sha, run, expected)
