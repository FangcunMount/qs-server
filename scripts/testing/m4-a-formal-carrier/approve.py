#!/usr/bin/env python3
"""Separate fixed synthetic approver; cannot connect to a DB or start runtime."""
import argparse
import datetime as dt
import importlib.util
import json
import os
from pathlib import Path
import re

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("carrier_approval_primitives", HERE / "run.py")
fixed = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixed)


def approval_for(material, root, material_hash, responsibility, now):
    stamp = lambda value: value.astimezone(dt.timezone.utc).isoformat().replace("+00:00", "Z")
    return {"format_version": 1, "kind": "m4_a_pre_start_sql_object_restore", "operation_id": root.name,
            "compiled_tool_source": fixed.TOOL_SOURCE, "a_execution_source": fixed.A_SOURCE,
            "a_execution_resources_sha256": fixed.RESOURCE, "a_execution_resource_files": 280,
            "independently_frozen_material_sha256": material_hash,
            "complete_writer_freeze_receipt_sha256": responsibility, "responsibility_receipt_sha256": responsibility,
            "mysql_identity_sha256": material["mysql_identity_sha256"], "mongo_identity_sha256": material["mongo_identity_sha256"],
            "all_six_instances_and_other_writers_frozen": True, "first_new_runtime_command_not_issued": True,
            "first_new_runtime_marker": str(root / "first-runtime.json"),
            "window_started_at": stamp(dt.datetime.fromisoformat(material["captured_at"].replace("Z", "+00:00"))),
            "recovery_decision_at": stamp(now), "restore_deadline": stamp(now + dt.timedelta(hours=1)),
            "expires_at": stamp(now + dt.timedelta(minutes=20)), "protection_retention_days": 30,
            "data_owner": "system (10001)", "recovery_owner": "system (10001)"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--operation-root", type=Path, required=True)
    parser.add_argument("--evidence-root", type=Path, required=True)
    args = parser.parse_args()
    fixed.need(os.getuid() == 0 and os.environ.get("GITHUB_ACTIONS") == "true"
               and os.environ.get("QS_M4_A_CARRIER_ACK") == "isolated_original_V10_carrier_only", "fixed_CI_approver_only")
    root = args.operation_root
    fixed.need(root.is_absolute() and root.parent == Path("/opt/qs-server/m4-cleanup-a")
               and re.fullmatch(r"[0-9]{1,20}-[0-9]{1,4}", root.name) is not None
               and not (root / "first-runtime.json").exists(), "original_canonical_prestart_root_required")
    remote = fixed.load_remote()
    def read(path, maximum):
        raw = path.read_bytes()
        return remote.capture_private_bytes(path, fixed.digest(raw), maximum)
    state = fixed.decode(read(args.evidence_root / "capture-state.json", 32768))
    material_raw = remote.capture_private_bytes(root / "first-material.json", state["material_sha256"], 128 << 20)
    material = fixed.decode(material_raw)
    execution = remote.capture_private_bytes(root / "capture-execution.json", state["capture_evidence_sha256"], 32768)
    proof = remote.capture_execution_receipt(execution, state["capture_evidence_sha256"], str(root / "capture-stdout.json"))
    stdout = remote.capture_private_bytes(root / "capture-stdout.json", proof["receipt_sha256"], 65536)
    remote.capture_receipt(stdout, state["material_sha256"])
    partial = fixed.decode(read(args.evidence_root / "independent-partial.json", 131072))
    responsibility_raw = read(args.evidence_root / "synthetic-responsibility.json", 32768)
    responsibility = fixed.decode(responsibility_raw)
    fixed.need(responsibility.get("production") is False and responsibility.get("no_business_process_created") is True
               and partial.get("mysql_version") == 101 and partial.get("mysql_dirty") is True
               and partial.get("complete") is True and partial.get("action") == "partial"
               and partial.get("mysql_identity_sha256") == material["mysql_identity_sha256"]
               and partial.get("mongo_identity_sha256") == material["mongo_identity_sha256"]
               and "historical_lifecycle_evidence" not in partial["assessment_ddl"]
               and "historical_committed_evidence" in partial["evaluation_outcome_ddl"]
               and material["first_capture_path"] == str(root / "first-material.json")
               and material["compiled_tool_source"] == fixed.TOOL_SOURCE
               and material["a_execution_source"] == fixed.A_SOURCE
               and material["mysql_database"] == material["mongo_database"] == "qs",
               "independent_actual_T0_and_partial_relation_required")
    approval = approval_for(material, root, state["material_sha256"], fixed.digest(responsibility_raw), dt.datetime.now(dt.timezone.utc))
    raw = json.dumps(approval, separators=(",", ":")).encode()
    path = root / "independent-fixture-approval.json"
    remote.durable(path, raw)
    remote.capture_private_bytes(path, fixed.digest(raw), 32768)
    print(json.dumps({"approval_sha256": fixed.digest(raw), "production_authorization": False}))


if __name__ == "__main__":
    main()
