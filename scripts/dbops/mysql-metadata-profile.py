#!/usr/bin/env python3
"""Read-only current versus fixed RDS role grants diagnostic; never a drop gate."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import sys
import tempfile

SHA = re.compile(r"^[0-9a-f]{40}$")
HASH = re.compile(r"^[0-9a-f]{64}$")
RUN = re.compile(r"^[0-9]{1,20}-[0-9]{1,4}$")
KEYS = {"format_version", "source_sha", "run_id", "expected_target_hash", "source_target_hash", "current_unrestricted_metadata_grants", "rds_role_grants_available", "rds_role_unrestricted_metadata_grants", "assigned_roles_present", "mandatory_roles_present", "diagnostic_only", "complete", "error_category"}
FACT_KEYS = {"current_global_select_grant", "current_global_show_view_grant", "current_global_trigger_grant", "current_global_event_grant", "current_partial_revocations_present"}
ALLOWED_KEYS = KEYS | FACT_KEYS

ERRORS = {"none", "input_invalid", "connection_input_invalid", "connection_config_failed", "connection_failed", "identity_read_failed", "target_identity_mismatch", "target_hash_mismatch", "current_grants_query_failed", "current_grants_rejected", "rds_role_query_failed", "rds_role_grants_rejected", "identity_final_failed", "session_identity_changed", "mandatory_roles_query_failed"}


FAILURE_RECEIPT = {"format_version": 1, "diagnostic_only": True, "complete": False,
                   "error_category": "profile_transport_or_receipt_failed"}
RECEIPT_SCHEMA = {
    "format_version": "uint", "source_sha": "sha40", "run_id": "run_id",
    "expected_target_hash": "hash64", "source_target_hash": "nullable_hash64",
    "current_unrestricted_metadata_grants": "bool", "rds_role_grants_available": "nullable_bool",
    "rds_role_unrestricted_metadata_grants": "nullable_bool", "assigned_roles_present": "nullable_bool",
    "mandatory_roles_present": "nullable_bool", "diagnostic_only": "bool", "complete": "bool",
    "error_category": frozenset(ERRORS | {"profile_transport_or_receipt_failed"}),
}

RECEIPT_SCHEMA.update({key: "nullable_bool" for key in FACT_KEYS})

def receipt_transport_module():
    spec = importlib.util.spec_from_file_location("bounded_receipt_transport", Path(__file__).with_name("receipt-transport.py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def runtime_module():
    spec = importlib.util.spec_from_file_location("bounded_cbpt_transport", Path(__file__).with_name("cbpt-cleanup.py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def safe_receipt(module, raw, code, sha, run, expected):
    value = module.load_json_bytes(raw)
    if not isinstance(value, dict) or set(value) - ALLOWED_KEYS:
        raise ValueError("receipt_fields")
    required = KEYS - {"source_target_hash"}
    if not required <= set(value):
        raise ValueError("receipt_fields")
    if type(value["format_version"]) is not int or value["format_version"] != 1:
        raise ValueError("receipt_version")
    if value["source_sha"] != sha or value["run_id"] != run or value["expected_target_hash"] != expected:
        raise ValueError("receipt_binding")
    for name in ("diagnostic_only", "complete", "current_unrestricted_metadata_grants"):
        if type(value[name]) is not bool:
            raise ValueError("receipt_type")
    for name in ("rds_role_grants_available", "rds_role_unrestricted_metadata_grants", "assigned_roles_present", "mandatory_roles_present"):
        if value[name] is not None and type(value[name]) is not bool:
            raise ValueError("receipt_type")
    target = value.get("source_target_hash")
    if target is not None and (not isinstance(target, str) or not HASH.fullmatch(target)):
        raise ValueError("receipt_target")
    if value["diagnostic_only"] is not True or (not isinstance(value["error_category"], str) or value["error_category"] not in ERRORS):
        raise ValueError("receipt_category")
    if value["complete"]:
        if code != 0 or value["error_category"] != "none" or target != expected:
            raise ValueError("receipt_success")
        if type(value["rds_role_grants_available"]) is not bool or any(type(value[name]) is not bool for name in ("assigned_roles_present", "mandatory_roles_present")):
            raise ValueError("receipt_role_unknown")
        if value["rds_role_grants_available"] is True and type(value["rds_role_unrestricted_metadata_grants"]) is not bool:
            raise ValueError("receipt_role_unknown")
        if value["rds_role_grants_available"] is False and value["rds_role_unrestricted_metadata_grants"] is not None:
            raise ValueError("receipt_role_conflict")
    elif code == 0 or value["error_category"] == "none" or value["current_unrestricted_metadata_grants"] or value["rds_role_unrestricted_metadata_grants"] is True:
        raise ValueError("receipt_failure")
    if not value["complete"] and any(value[name] is not None for name in ("rds_role_grants_available", "rds_role_unrestricted_metadata_grants", "assigned_roles_present", "mandatory_roles_present")):
        raise ValueError("receipt_census_failure")
    facts = set(value) & FACT_KEYS
    if facts:
        if facts != FACT_KEYS:
            raise ValueError("receipt_grant_facts_missing")
        if value["complete"]:
            if any(type(value[key]) is not bool for key in FACT_KEYS):
                raise ValueError("receipt_grant_facts_type")
            full = not value["current_partial_revocations_present"] and all(value[key] for key in FACT_KEYS - {"current_partial_revocations_present"})
            if full != value["current_unrestricted_metadata_grants"]:
                raise ValueError("receipt_grant_facts_conflict")
        elif any(value[key] is not None for key in FACT_KEYS):
            raise ValueError("receipt_grant_facts_failure")
    return value


def execute(binary, env, module):
    sha, run, expected = (env.get(k, "") for k in ("PROFILE_SOURCE_SHA", "PROFILE_RUN_ID", "PROFILE_EXPECTED_TARGET_HASH"))
    if not SHA.fullmatch(sha) or not RUN.fullmatch(run) or not HASH.fullmatch(expected):
        raise ValueError("input_invalid")
    module.validate_source(env)
    path = Path(binary)
    if not path.is_absolute() or path.is_symlink() or not path.is_file():
        raise ValueError("binary_invalid")
    runtime = module.Runtime(str(path), run, sha)
    name = "qs-mysql-metadata-profile-" + runtime.owner
    runtime.preflight_name(name)
    value = None
    code = 1
    try:
        image = runtime.image("mysql:8.0")
        command = [*runtime.docker, "run", "--rm", "--name", name, "--pull=never", "--network", "infra-network", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--cpus=0.5", "--memory=256m", "--pids-limit=64", "--user", str(os.getuid()) + ":" + str(os.getgid()), "--label", runtime.label, "--mount", "type=bind,source=" + str(path) + ",target=/metadata-profile,readonly", "--entrypoint", "/metadata-profile"]
        # sudo may reset inherited environment. Pass a private, temporary env file;
        # no credential values enter the command line or diagnostic output.
        keys = ("MYSQL_HOST", "MYSQL_PORT", "MYSQL_USERNAME", "MYSQL_PASSWORD", "MYSQL_DATABASE", "PROFILE_SOURCE_SHA", "PROFILE_RUN_ID", "PROFILE_EXPECTED_TARGET_HASH")
        with tempfile.TemporaryDirectory(prefix="qs-mysql-profile-env-") as temporary:
            directory = Path(temporary)
            os.chmod(directory, 0o700)
            env_file = directory / "profile.env"
            payload = "".join(key + "=" + (env.get(key, "") or ("3306" if key == "MYSQL_PORT" else "")) + "\n" for key in keys)
            module.write_private(env_file, payload.encode())
            command += ["--env-file", str(env_file), image]
            code, raw = module.capture(command, timeout=180)
            value = safe_receipt(module, raw, code, sha, run, expected)
    finally:
        runtime.cleanup()
    return code, value


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", required=True)
    args = parser.parse_args()
    # Transport must be available before any database diagnostic starts.
    try:
        armor = receipt_transport_module()
    except Exception:
        return 1
    try:
        # execute returns only the dictionary accepted by safe_receipt.
        code, value = execute(args.binary, os.environ, runtime_module())
    except Exception:
        value = dict(FAILURE_RECEIPT)
        code = 1
    secrets = tuple(os.environ.get(key, "") for key in ("MYSQL_USERNAME", "MYSQL_PASSWORD"))
    try:
        output = armor.encode_armored_receipt(value, schema=RECEIPT_SCHEMA, secrets=secrets)
    except Exception:
        code = 1
        try:
            output = armor.encode_armored_receipt(dict(FAILURE_RECEIPT), schema=RECEIPT_SCHEMA, secrets=secrets)
        except Exception:
            # No unarmored fallback if all alphabets collide or armor is invalid.
            return 1
    print(output)
    return code


if __name__ == "__main__":
    sys.exit(main())
