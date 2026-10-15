#!/usr/bin/env python3
"""Eleven fixed retirement observations; no deletion, freeze or backup authority."""
import importlib.util
from pathlib import Path


def load_profile():
    spec = importlib.util.spec_from_file_location("inventory_profile_primitives", Path(__file__).with_name("mysql-metadata-profile.py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


profile = load_profile()
INVENTORY = True
SHA, RUN, HASH = profile.SHA, profile.RUN, profile.HASH
PROFILE_KEYS = profile.PROFILE_KEYS
runtime_module = profile.runtime_module
receipt_transport_module = profile.receipt_transport_module
capture_private = profile.capture_private

COUNT_FIELDS = {
    "testee_tags_partition": frozenset({"total", "sql_null", "non_null", "json_null", "empty_array", "non_empty_array", "other"}),
    "operators_role_duplication": frozenset({"total", "roles_sql_null", "effective_sql_null", "both_arrays", "same_canonical_json_bytes", "different_canonical_json_bytes"}),
    "five_table_counts": frozenset({"testee_store_migration_manifests", "testee_store_migration_items", "testee_store_migration_reversions", "operator_retirement_manifests", "clinician_operator_binding_archive"}),
    "testee_manifest_states": frozenset({"applied", "rolled_back", "unknown"}),
    "testee_item_dispositions": frozenset({"candidate", "deferred", "already_assigned", "unknown"}),
    "operator_manifest_stages": frozenset({"pending", "executing", "applied_unchanged", "applied_drifted", "invalid", "unknown"}),
    "historical_reference_gaps": frozenset({"items_without_manifest", "reversions_without_item", "archives_without_manifest"}),
    "ai_backfill_scope": frozenset({"total", "actual_backfill_pending", "subject_index_null", "testee_index_null", "nonempty_assessment_arrays", "invalid_assessment_array_shape", "invalid_actor_field_shape", "indexed_identity_mismatch"}),
    "ai_expected_relation_gaps": frozenset({"payload_relation_entries", "invalid_expected_id_entries", "valid_expected_entries_without_relation"}),
    "ai_actual_relation_gaps": frozenset({"actual_relation_rows", "orphan_relations", "zero_assessment_relations", "relation_not_in_request_payload"}),
}
STAGES = frozenset(COUNT_FIELDS) | {"none", "input", "connect", "identity_before", "migration_before", "snapshot_begin", "snapshot_release", "objects_present", "conservation", "migration_after", "identity_after", "connection_release", "database_release", "transport"}
ERRORS = frozenset({"none", "input_invalid", "connection_input_invalid", "connection_config_failed", "connection_failed", "target_identity_mismatch", "target_hash_mismatch", "identity_read_failed", "session_identity_changed", "server_version_unrecognized", "query_failed", "query_server_timeout", "query_server_interrupted", "query_permission_denied", "query_deadline_exceeded", "query_canceled", "query_mysql_error", "query_network_timeout", "query_network_error", "query_invalid_connection", "query_bad_connection", "query_unexpected_eof", "query_eof", "metadata_invalid", "metadata_truncated", "metadata_release_failed", "transaction_release_failed", "migration_pair_rejected", "object_shape_rejected", "counts_inconsistent", "connection_release_failed", "database_release_failed", "profile_input_preflight_failed", "profile_package_preflight_failed", "profile_trusted_ssh_preflight_failed", "profile_remote_or_receipt_failed", "profile_transport_or_receipt_failed"})
MEASUREMENTS_SCHEMA = {
    "mysql_version": {"major": "uint", "minor": "uint", "patch": "uint"},
    "migration_version": "uint", "migration_dirty": "bool", "nine_innodb_objects_verified": "bool", "consistent_snapshot": "bool",
    "counts": {family: {field: "uint" for field in fields} for family, fields in COUNT_FIELDS.items()},
}
RECEIPT_SCHEMA = {
    "format_version": "uint", "source_sha": "sha40", "run_id": "run_id", "expected_target_hash": "hash64", "source_target_hash": "hash64",
    "retirement_inventory_only": "bool", "freeze_verified": "bool", "complete": "bool", "error_category": ERRORS, "failure_stage": STAGES,
    "query_count": "uint", "control_statement_count": "uint", "transaction_released": "nullable_bool", "connection_released": "nullable_bool",
    "database_released": "nullable_bool", "pool_closed_verified": "nullable_bool", "measurements": MEASUREMENTS_SCHEMA, "query_mysql_error_number": "uint",
}
FAILURE_RECEIPT = {"format_version": 1, "retirement_inventory_only": True, "freeze_verified": False, "complete": False,
                   "error_category": "profile_transport_or_receipt_failed", "failure_stage": "transport", "query_count": 0,
                   "control_statement_count": 0, "transaction_released": None, "connection_released": None, "database_released": None, "pool_closed_verified": None}


def stage_failure(env, category):
    value = dict(FAILURE_RECEIPT)
    value["error_category"] = category if category in ERRORS else "profile_transport_or_receipt_failed"
    for key, source, expression in (("source_sha", "PROFILE_SOURCE_SHA", SHA), ("run_id", "PROFILE_RUN_ID", RUN), ("expected_target_hash", "PROFILE_EXPECTED_TARGET_HASH", HASH)):
        if expression.fullmatch(env.get(source, "")):
            value[key] = env[source]
    return value


def conserved_counts(c):
    if set(c) != set(COUNT_FIELDS) or any(set(c[key]) != fields for key, fields in COUNT_FIELDS.items()):
        return False
    tags, roles, tables = c["testee_tags_partition"], c["operators_role_duplication"], c["five_table_counts"]
    if tags["total"] != tags["sql_null"] + tags["non_null"] or tags["non_null"] != sum(tags[key] for key in ("json_null", "empty_array", "non_empty_array", "other")):
        return False
    if roles["total"] != roles["same_canonical_json_bytes"] + roles["different_canonical_json_bytes"] or any(roles[key] > roles["total"] for key in ("roles_sql_null", "effective_sql_null", "both_arrays")):
        return False
    for family, table in (("testee_manifest_states", "testee_store_migration_manifests"), ("testee_item_dispositions", "testee_store_migration_items"), ("operator_manifest_stages", "operator_retirement_manifests")):
        if sum(c[family].values()) != tables[table]:
            return False
    gaps = c["historical_reference_gaps"]
    if any(gaps[field] > tables[table] for field, table in (("items_without_manifest", "testee_store_migration_items"), ("reversions_without_item", "testee_store_migration_reversions"), ("archives_without_manifest", "clinician_operator_binding_archive"))):
        return False
    ai, expected, actual = (c[key] for key in ("ai_backfill_scope", "ai_expected_relation_gaps", "ai_actual_relation_gaps"))
    if ai["total"] != ai["nonempty_assessment_arrays"] + ai["invalid_assessment_array_shape"] or any(count > ai["total"] for count in ai.values()):
        return False
    return (ai["indexed_identity_mismatch"] <= ai["total"] - ai["invalid_actor_field_shape"]
            and expected["invalid_expected_id_entries"] <= expected["payload_relation_entries"]
            and expected["valid_expected_entries_without_relation"] <= expected["payload_relation_entries"] - expected["invalid_expected_id_entries"]
            and actual["orphan_relations"] <= actual["actual_relation_rows"]
            and actual["zero_assessment_relations"] <= actual["actual_relation_rows"]
            and actual["relation_not_in_request_payload"] <= actual["actual_relation_rows"] - actual["orphan_relations"])


def safe_receipt(module, raw, code, sha, run, expected):
    value = module.load_json_bytes(raw)
    receipt_transport_module().encode_armored_receipt(value, schema=RECEIPT_SCHEMA)
    required = set(RECEIPT_SCHEMA) - {"source_target_hash", "measurements", "query_mysql_error_number"}
    if not required <= set(value) or value["format_version"] != 1 or value["source_sha"] != sha or value["run_id"] != run or value["expected_target_hash"] != expected:
        raise ValueError("inventory_receipt_binding")
    if value["retirement_inventory_only"] is not True or value["freeze_verified"] is not False or value["query_count"] > 15 or value["control_statement_count"] > 3:
        raise ValueError("inventory_receipt_scope")
    category, number = value["error_category"], value.get("query_mysql_error_number")
    if "query_mysql_error_number" in value:
        if type(number) is not int or not 0 <= number <= 65535 or value["complete"] or value["query_count"] == 0:
            raise ValueError("inventory_mysql_number_invalid")
        known = {"query_server_timeout": {3024}, "query_server_interrupted": {1317}, "query_permission_denied": {1142, 1143, 1227}}
        if category in known:
            if number not in known[category]:
                raise ValueError("inventory_mysql_number_category")
        elif category == "query_mysql_error":
            if number in {3024, 1317, 1142, 1143, 1227}:
                raise ValueError("inventory_mysql_number_category")
        elif category not in {"metadata_release_failed", "transaction_release_failed", "connection_release_failed", "database_release_failed"}:
            raise ValueError("inventory_mysql_number_category")
    elif category == "query_mysql_error":
        raise ValueError("inventory_mysql_number_missing")
    if value["complete"]:
        if code != 0 or category != "none" or value["failure_stage"] != "none" or value.get("source_target_hash") != expected or value["query_count"] != 15 or value["control_statement_count"] != 3:
            raise ValueError("inventory_success_unverified")
        if any(value[key] is not True for key in ("transaction_released", "connection_released", "database_released", "pool_closed_verified")):
            raise ValueError("inventory_release_unverified")
        m = value.get("measurements")
        if type(m) is not dict or set(m) != set(MEASUREMENTS_SCHEMA) or set(m["mysql_version"]) != {"major", "minor", "patch"}:
            raise ValueError("inventory_measurements_incomplete")
        if m["migration_version"] != 100 or m["migration_dirty"] is not False or m["nine_innodb_objects_verified"] is not True or m["consistent_snapshot"] is not True:
            raise ValueError("inventory_observation_unverified")
        version = m["mysql_version"]
        if not 8 <= version["major"] <= 999 or any(component > 999 for component in version.values()) or not conserved_counts(m["counts"]):
            raise ValueError("inventory_counts_invalid")
    elif code == 0 or category == "none" or value["failure_stage"] == "none" or "measurements" in value:
        raise ValueError("inventory_failure_retained_measurements")
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
        raise ValueError("inventory_database_binding")
    code, raw = capture_private([str(path), "--retirement-inventory"], env=private)
    return code, safe_receipt(module, raw, code, sha, run, expected)
