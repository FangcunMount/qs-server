"""Register an independently approved, immutable read-only history request.

Physical metadata is an observation. This stage requires a separate descriptor,
checks the original inputs again and publishes the request exclusively. It never
connects to a database, starts a container or enables a retirement mutation.
"""
import argparse
import hashlib
import importlib.util
from pathlib import Path
import time


MODE = "bootstrap-history-parent"
METADATA_FIELDS = ("format_version", "kind", "source_sha", "operation_id", "run_id", "metadata_limits",
    "approved_inventory_report", "inventory_request_sha256", "approval_sha256", "parent_proposal_run_id",
    "parent_proposal_sha256", "assets", "equal_full_physical_passes", "metadata_complete", "input_baseline_sha256",
    "semantic_source_coverage_verified", "production_process_budget_proven", "complete", "execution_allowed",
    "drop_ready", "cas_complete")
METADATA_REGISTRATION_FIELDS = ("format_version", "kind", "source_sha", "operation_id", "created_run_id",
    "approval_sha256", "approval", "parent_proposal_run_id", "parent_proposal_sha256")
PROPOSAL_FIELDS = ("format_version", "kind", "source_sha", "operation_id", "run_id", "inventory_request",
    "inventory_report", "assets")
ASSET_FIELDS = ("database", "name", "full_file_sha256", "full_file_bytes", "source_asset_sha256")


def history_api():
    path = Path(__file__).with_name("compatibility-history-prepare.py")
    spec = importlib.util.spec_from_file_location("compatibility_history_parent_inputs", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def approval(args, t, h):
    t.token(args.actual_source_sha, t.SHA)
    t.token(args.operation_id, t.RUN)
    t.token(args.run_id, t.RUN)
    text = args.bootstrap_approval_json
    t.token(args.bootstrap_approval_hash, t.HASH)
    if type(text) is not str or not 0 < len(text) <= t.MAX_BOOTSTRAP_APPROVAL or not text.isascii():
        t.fail("history_parent_approval_invalid")
    value = t.decode(text.encode("ascii"))
    raw = t.canonical_bytes(value)
    if raw[:-1] != text.encode("ascii") or hashlib.sha256(raw).hexdigest() != args.bootstrap_approval_hash:
        t.fail("history_parent_approval_encoding_or_hash_invalid")
    t.fields(value, ("format_version", "kind", "prepare_mode", "source_sha", "operation_id", "target_hash",
        "database_scope", "metadata_report", "parent_proposal_sha256", "inventory_request_sha256",
        "inventory_report", "assets", "metadata_limits"))
    if (type(value["format_version"]) is not int or value["format_version"] != 1 or
        value["kind"] != "readonly_history_parent_registration_approval" or value["prepare_mode"] != MODE or
        value["source_sha"] != args.actual_source_sha or value["operation_id"] != args.operation_id or
        value["target_hash"] != t.TARGET_HASH or value["database_scope"] != "mysql-and-mongodb"):
        t.fail("history_parent_approval_binding_invalid")
    for key in ("metadata_report", "inventory_report"):
        h._reference(t, value[key])
    for key in ("parent_proposal_sha256", "inventory_request_sha256"):
        t.token(value[key], t.HASH)
    reference_runs = {value[key]["run_id"] for key in ("metadata_report", "inventory_report")}
    if len(reference_runs) != 2 or args.run_id in reference_runs:
        t.fail("history_parent_actual_run_reused")
    t.fields(value["metadata_limits"], h.METADATA_LIMITS)
    if any(type(value["metadata_limits"][key]) is not int or value["metadata_limits"][key] != expected
           for key, expected in h.METADATA_LIMITS.items()):
        t.fail("history_parent_limits_invalid")
    if type(value["assets"]) is not list or len(value["assets"]) != 4:
        t.fail("history_parent_approved_assets_invalid")
    for asset, target in zip(value["assets"], t.TARGETS):
        t.fields(asset, ASSET_FIELDS)
        for key in ("full_file_sha256", "source_asset_sha256"):
            t.token(asset[key], t.HASH)
        t.uint(asset["full_file_bytes"])
        if ((asset["database"], asset["name"]) != target[:2] or
            asset["full_file_bytes"] > h.METADATA_LIMITS["max_encoded_file_bytes"] or
            (asset["full_file_bytes"] == 0 and target[0] != "mongodb")):
            t.fail("history_parent_approved_assets_invalid")
    return value


def _inputs(args, t, h, directory, value):
    output = t.private_directory(directory / ("history-metadata-" + value["metadata_report"]["run_id"]))
    metadata, metadata_hash = t.read_private(output, "history-metadata.json", value["metadata_report"]["sha256"])
    registration, registration_hash = t.read_private(output, "history-metadata-bootstrap.json")
    proposal, proposal_hash = t.read_private(output, "history-parent-proposal.json", value["parent_proposal_sha256"])
    t.fields(metadata, METADATA_FIELDS)
    t.fields(registration, METADATA_REGISTRATION_FIELDS)
    t.fields(proposal, PROPOSAL_FIELDS)
    original_run = value["inventory_report"]["run_id"]
    if (type(metadata["format_version"]) is not int or metadata["format_version"] != 1 or
        metadata["kind"] != "readonly_history_file_metadata" or metadata["source_sha"] != args.actual_source_sha or
        metadata["operation_id"] != args.operation_id or metadata["run_id"] != value["metadata_report"]["run_id"] or
        t.canonical_bytes(metadata["metadata_limits"]) != t.canonical_bytes(value["metadata_limits"]) or
        t.canonical_bytes(metadata["assets"]) != t.canonical_bytes(value["assets"]) or
        metadata["approved_inventory_report"] != value["inventory_report"] or
        metadata["inventory_request_sha256"] != value["inventory_request_sha256"] or
        metadata["parent_proposal_run_id"] != original_run or metadata["parent_proposal_sha256"] != proposal_hash or
        type(metadata["equal_full_physical_passes"]) is not int or metadata["equal_full_physical_passes"] != 2 or
        metadata["metadata_complete"] is not True or
        any(metadata[key] is not False for key in ("semantic_source_coverage_verified", "production_process_budget_proven",
                                                  "complete", "execution_allowed", "drop_ready", "cas_complete"))):
        t.fail("history_parent_metadata_binding_invalid")
    for key in ("approval_sha256", "input_baseline_sha256"):
        t.token(metadata[key], t.HASH)
    if (type(registration["format_version"]) is not int or registration["format_version"] != 1 or
        registration["kind"] != "readonly_history_metadata_binding" or registration["source_sha"] != args.actual_source_sha or
        registration["operation_id"] != args.operation_id or registration["created_run_id"] != metadata["run_id"] or
        registration["approval_sha256"] != metadata["approval_sha256"] or
        registration["parent_proposal_run_id"] != original_run or registration["parent_proposal_sha256"] != proposal_hash):
        t.fail("history_parent_metadata_registration_invalid")
    metadata_args = argparse.Namespace(**vars(args))
    metadata_args.run_id = metadata["run_id"]
    metadata_args.prepare_mode = h.METADATA_MODE
    approval_raw = t.canonical_bytes(registration["approval"])
    metadata_args.bootstrap_approval_json = approval_raw[:-1].decode("ascii")
    metadata_args.bootstrap_approval_hash = registration["approval_sha256"]
    approved_metadata = h.metadata_approval(metadata_args, t)
    if (approved_metadata["inventory_report"] != value["inventory_report"] or
        approved_metadata["inventory_request_sha256"] != value["inventory_request_sha256"] or
        approved_metadata["metadata_limits"] != value["metadata_limits"]):
        t.fail("history_parent_metadata_registration_invalid")
    request, report, paths, json_baselines = h._metadata_inputs(t, directory, approved_metadata, metadata_args)
    expected_assets = [dict({key: asset[key] for key in ("database", "name", "full_file_sha256", "full_file_bytes")},
                            path=str(path)) for asset, path in zip(value["assets"], paths)]
    expected_proposal = {"format_version": 1, "kind": "readonly_compatibility_history_request",
        "source_sha": args.actual_source_sha, "operation_id": args.operation_id, "run_id": original_run,
        "inventory_request": {"path": str(directory / "inventory-request.json"), "sha256": value["inventory_request_sha256"]},
        "inventory_report": {"path": str(paths[0].parent / "inventory.private.json"), "sha256": value["inventory_report"]["sha256"]},
        "assets": expected_assets}
    if (t.canonical_bytes(proposal) != t.canonical_bytes(expected_proposal) or
        hashlib.sha256(t.canonical_bytes(proposal)).hexdigest() != proposal_hash):
        t.fail("history_parent_proposal_binding_invalid")
    if [entry[1] for entry in json_baselines[3:]] != [asset["source_asset_sha256"] for asset in value["assets"]]:
        t.fail("history_parent_source_sidecars_changed")
    for filename, digest in (("history-metadata.json", metadata_hash),
                             ("history-metadata-bootstrap.json", registration_hash),
                             ("history-parent-proposal.json", proposal_hash)):
        path = output / filename
        json_baselines.append((path, digest, t.MAX_JSON, h._file_baseline(t, path, digest, t.MAX_JSON)))
    return metadata, proposal, paths, json_baselines


def _publish(args, t, h, directory, value, proposal):
    raw = t.canonical_bytes(proposal)
    digest = hashlib.sha256(raw).hexdigest()
    registration = {"format_version": 1, "kind": "readonly_history_parent_registration",
        "source_sha": args.actual_source_sha, "operation_id": args.operation_id, "created_run_id": args.run_id,
        "approval_sha256": args.bootstrap_approval_hash, "approval": value, "request_sha256": digest,
        "parent_run_id": proposal["run_id"], "metadata_report": value["metadata_report"],
        "inventory_report": value["inventory_report"]}
    names = ("history-request-bootstrap.json", "history-request.json")
    if any((directory / (name + ".bootstrap.partial")).exists() or
           (directory / (name + ".bootstrap.partial")).is_symlink() for name in names):
        t.fail("history_parent_registration_incomplete")
    present = [(directory / name).exists() or (directory / name).is_symlink() for name in names]
    if any(present):
        if not all(present):
            t.fail("history_parent_registration_incomplete")
        recorded, _ = t.read_private(directory, names[0])
        existing, _ = t.read_private(directory, names[1], digest)
        t.fields(recorded, registration)
        t.token(recorded["created_run_id"], t.RUN)
        if recorded["created_run_id"] in {value[key]["run_id"] for key in ("metadata_report", "inventory_report")}:
            t.fail("history_parent_registration_binding_invalid")
        expected = dict(registration, created_run_id=recorded["created_run_id"])
        if recorded != expected or existing != proposal or t.canonical_bytes(recorded) != t.canonical_bytes(expected):
            t.fail("history_parent_registration_binding_invalid")
        created_run = recorded["created_run_id"]
    else:
        t.create_bootstrap_file(directory, names[0], t.canonical_bytes(registration))
        t.create_bootstrap_file(directory, names[1], raw)
        created_run = args.run_id
    published, published_hash = t.read_private(directory, names[1], digest)
    recorded, registry_hash = t.read_private(directory, names[0])
    if published != proposal or recorded != dict(registration, created_run_id=created_run):
        t.fail("history_parent_registration_binding_invalid")
    return created_run, published_hash, registry_hash


def prepare(args, t):
    h = history_api()
    value = approval(args, t, h)
    directory = t.operation_directory(args.root, args.operation_id)
    deadline = time.monotonic() + h.METADATA_LIMITS["total_seconds"]
    docker = ["sudo", "-n", "docker"]
    with t.locked_operation(directory):
        h._metadata_prior_handles(t, docker, directory, deadline)
        metadata, proposal, paths, json_baselines = _inputs(args, t, h, directory, value)
        observed = []
        for pass_index in range(h.METADATA_LIMITS["passes"]):
            current = []
            total = 0
            for path, asset in zip(paths, value["assets"]):
                item = h._metadata_physical_file(t, path, h.METADATA_LIMITS["max_encoded_file_bytes"], deadline)
                total += item[1]
                if (item[:2] != (asset["full_file_sha256"], asset["full_file_bytes"]) or
                    total > h.METADATA_LIMITS["max_total_encoded_bytes"]):
                    t.fail("history_parent_physical_source_changed")
                current.append(item)
            if pass_index and current != observed:
                t.fail("history_parent_physical_source_changed")
            observed = current
        # The producer's private stat baseline also binds replacement with the
        # same bytes. It is not replaced by a newly generated approval here.
        original_json = json_baselines[:-3]
        baseline = [(str(path), item) for path, _, _, item in original_json]
        baseline += [(str(path), item[2]) for path, item in zip(paths, observed)]
        if hashlib.sha256(t.canonical_bytes(baseline)).hexdigest() != metadata["input_baseline_sha256"]:
            t.fail("history_parent_input_baseline_changed")
        h._unchanged(t, json_baselines)
        for path, item in zip(paths, observed):
            h._metadata_current_stat(t, path, item[2])
        h._metadata_prior_handles(t, docker, directory, deadline)
        h._metadata_time(t, deadline)
        created_run, request_hash, registry_hash = _publish(args, t, h, directory, value, proposal)
        h._unchanged(t, json_baselines)
        for path, item in zip(paths, observed):
            h._metadata_current_stat(t, path, item[2])
        h._metadata_time(t, deadline)
    return {"format_version": 1, "operation": "prepare", "prepare_mode": MODE,
        "source_sha": args.actual_source_sha, "operation_id": args.operation_id, "run_id": args.run_id,
        "target_hash": t.TARGET_HASH, "target_count": 4, "complete": False, "execution_allowed": False,
        "drop_ready": False, "diagnostic_only": True, "history_cas_complete": False,
        "history_parent_registration_complete": True, "history_parent_process_budget_proven": False,
        "bootstrap_approval_sha256": args.bootstrap_approval_hash, "request_created_run_id": created_run,
        "history_parent_request_sha256": request_hash, "history_parent_registration_sha256": registry_hash,
        "approved_metadata_report": value["metadata_report"], "approved_inventory_report": value["inventory_report"],
        "inventory_request_sha256": value["inventory_request_sha256"], "parent_proposal_run_id": proposal["run_id"],
        "error_category": "history_parent_registered_requires_independent_history_run_approval"}
