"""Immutable Action derivation for the native, read-only ServerA AI host.

This wrapper validates constraints and private assets. It cannot deserialize an
opaque qualification or enable writes. The native executable owns operation.lock
during observation; the wrapper releases its registration lock before invoking it.
"""
import argparse
import hashlib
import importlib.util
import os
from pathlib import Path
import re
import stat
import time


MODES = frozenset({"bootstrap-ai-bounds", "bootstrap-ai-verify"})
REQUIRED = (
    "independent_production_descriptor_and_bounds_approval",
    "actual_host_process_memory_cpu_and_scan_budget",
    "original_exec_unknown_keeps_mutation_blocked",
    "whole_writer_fence_for_any_mutation", "historical_evidence_write_authority",
    "post_write_independent_readback", "backup_restore_acceptance_purge")
FLAGS_FALSE = frozenset({"complete", "independent_approval", "whole_writer_fence",
    "cas_authority", "retirement_written", "drop_ready"})
REPORT_FIELDS = FLAGS_FALSE | frozenset({"protocol", "mode", "source_sha", "operation_id",
    "actual_run_id", "external_run_id", "request_sha256", "descriptor_sha256",
    "expected_runtime_binding_sha256", "runtime_binding_sha256", "error_category",
    "diagnostic_only", "diagnostic_read_complete", "required_adapters"})
BOUNDS_FALSE = frozenset({"independent_approval", "business_closure", "writer_fence",
    "broker_coverage", "cas_authority", "retirement_written", "recovery_authority", "drop_ready"})
BOUNDS_FIELDS = BOUNDS_FALSE | frozenset({"scope", "facts_sha256", "runtime_binding_sha256",
    "ai_bounds_sha256", "peer_bounds_sha256", "ai_physical_objects", "ai_logical_objects",
    "peer_objects", "independent_epochs", "prior_ai_binding_matched", "next_cycle_required"})
VERIFY_FALSE = frozenset({"production_approval", "writer_fence", "broker_coverage", "cas_authority", "drop_ready"})
VERIFY_FIELDS = VERIFY_FALSE | frozenset({"scope", "originals", "facts_sha256",
    "independent_epochs", "external_database_facts_observed"})


def history_module():
    path = Path(__file__).with_name("compatibility-history-prepare.py")
    spec = importlib.util.spec_from_file_location("compatibility_ai_history_dependencies", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def approval(args, t, h):
    text = args.bootstrap_approval_json
    t.token(args.bootstrap_approval_hash, t.HASH)
    if type(text) is not str or not 0 < len(text) <= t.MAX_BOOTSTRAP_APPROVAL or not text.isascii():
        t.fail("ai_host_approval_invalid")
    value = t.decode(text.encode("ascii"))
    raw = t.canonical_bytes(value)
    if raw[:-1] != text.encode("ascii") or hashlib.sha256(raw).hexdigest() != args.bootstrap_approval_hash:
        t.fail("ai_host_approval_encoding_or_hash_invalid")
    optional = ("bounds", "protection_sha256") if args.prepare_mode == "bootstrap-ai-verify" else ()
    t.fields(value, ("format_version", "kind", "prepare_mode", "source_sha", "operation_id",
                     "target_hash", "database_scope", "history", "runtime"), optional)
    if (type(value["format_version"]) is not int or value["format_version"] != 1 or
        value["kind"] != "readonly_ai_host_bootstrap_approval" or value["prepare_mode"] != args.prepare_mode or
        args.prepare_mode not in MODES or value["source_sha"] != args.actual_source_sha or
        value["operation_id"] != args.operation_id or value["target_hash"] != t.TARGET_HASH or
        value["database_scope"] != "mysql-and-mongodb"):
        t.fail("ai_host_approval_binding_invalid")
    inner = argparse.Namespace(**vars(args))
    inner.prepare_mode = h.MODE
    inner.bootstrap_approval_json = t.canonical_bytes(value["history"])[:-1].decode("ascii")
    inner.bootstrap_approval_hash = hashlib.sha256(t.canonical_bytes(value["history"])).hexdigest()
    h.approval(inner, t)  # All original parent/report/run/asset/hash constraints.
    runtime = value["runtime"]
    t.fields(runtime, ("source_sha", "image_id", "container_id", "binding_sha256",
                       "expected_ai_identity_hash", "expected_ai_head"))
    t.token(runtime["source_sha"], t.SHA)
    t.token(runtime["container_id"], t.HASH); t.token(runtime["binding_sha256"], t.HASH)
    if type(runtime["image_id"]) is not str or not re.fullmatch(r"sha256:[0-9a-f]{64}", runtime["image_id"]):
        t.fail("ai_host_runtime_constraint_invalid")
    identity, head = runtime["expected_ai_identity_hash"], runtime["expected_ai_head"]
    if (type(identity) is not str or type(head) is not str or
        (identity == "") != (head == "") or
        (identity and (not re.fullmatch(t.HASH, identity) or head not in ("0038_messaging_observations", "0040_module_table_names")))):
        t.fail("ai_host_runtime_constraint_invalid")
    if args.prepare_mode == "bootstrap-ai-verify":
        if identity or head or set(optional) - value.keys():
            t.fail("ai_host_verify_approval_invalid")
        t.fields(value["bounds"], ("run_id", "ai_sha256", "peer_sha256"))
        t.token(value["bounds"]["run_id"], t.RUN)
        t.token(value["bounds"]["ai_sha256"], t.HASH); t.token(value["bounds"]["peer_sha256"], t.HASH)
        t.token(value["protection_sha256"], t.HASH)
        if value["bounds"]["run_id"] == args.run_id:
            t.fail("ai_host_bounds_run_reused")
    return value


def _credentials(t):
    # This new mode requires the explicit metadata pair. Legacy modes keep their
    # existing credential contract; an application account is never a fallback.
    if not all(os.environ.get(key, "") for key in
               ("MONGODB_METADATA_ADMIN_USERNAME", "MONGODB_METADATA_ADMIN_PASSWORD")):
        t.fail("ai_host_metadata_connection_pair_required")
    return t.inventory_connection_values(os.environ)


def _native(t, command, values):
    saved = {key: os.environ.get(key) for key in values}
    try:
        os.environ.update(values)
        return t.capture_fixed(command, timeout=1830, maximum=32768)
    finally:
        for key, previous in saved.items():
            if previous is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = previous


def _private_report(t, h, output, raw):
    # The original history helper has a closed filename list; its contract is
    # unchanged. This distinct producer reads only its one fixed output name.
    t.private_directory(output)
    path = output / "ai-host.readiness.json"
    digest = hashlib.sha256(raw).hexdigest()
    baseline = h._file_baseline(t, path, digest, 32768)
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    except OSError:
        t.fail("ai_host_private_stdout_mismatch")
    try:
        before = os.fstat(fd)
        if (not stat.S_ISREG(before.st_mode) or before.st_uid != os.getuid() or
            stat.S_IMODE(before.st_mode) != 0o600 or before.st_nlink != 1 or not 0 < before.st_size <= 32768):
            t.fail("ai_host_private_stdout_mismatch")
        stored = bytearray()
        while chunk := os.read(fd, 32769):
            stored.extend(chunk)
            if len(stored) > 32768:
                t.fail("ai_host_private_stdout_mismatch")
        if bytes(stored) != raw or h._file_baseline(t, path, digest, 32768) != baseline:
            t.fail("ai_host_private_stdout_mismatch")
        current = os.fstat(fd)
        if not os.path.samestat(before, current) or before.st_mtime_ns != current.st_mtime_ns or before.st_ctime_ns != current.st_ctime_ns:
            t.fail("ai_host_private_stdout_mismatch")
        return t.decode(bytes(stored)), digest
    finally:
        os.close(fd)


def _report(t, h, raw, output, descriptor, descriptor_hash, expected_rows):
    report = t.decode(raw)
    t.fields(report, REPORT_FIELDS, ("bounds", "verification"))
    if (report["protocol"] != "qs-compatibility-ai-host-readonly/v1" or
        any(report[key] != descriptor[field] for key, field in
            (("mode", "mode"), ("source_sha", "source_sha"), ("operation_id", "operation_id"),
             ("actual_run_id", "actual_run_id"), ("request_sha256", "request_sha256"),
             ("expected_runtime_binding_sha256", "approved_runtime_binding_sha256"))) or
        report["external_run_id"] != descriptor["actual_run_id"].split("-")[0] or
        report["descriptor_sha256"] != descriptor_hash or report["required_adapters"] != list(REQUIRED) or
        report["diagnostic_only"] is not True or type(report["diagnostic_read_complete"]) is not bool or
        any(report[key] is not False for key in FLAGS_FALSE)):
        t.fail("ai_host_readiness_binding_invalid")
    t.token(report["runtime_binding_sha256"], t.HASH)
    if report["runtime_binding_sha256"] != descriptor["approved_runtime_binding_sha256"]:
        t.fail("ai_host_readiness_binding_invalid")
    # Private and stdout are exact same bytes, not merely equal JSON objects.
    private, digest = _private_report(t, h, output, raw)
    if private != report or digest != hashlib.sha256(raw).hexdigest():
        t.fail("ai_host_private_stdout_mismatch")
    if report["error_category"] != "none" or report["diagnostic_read_complete"] is not True:
        t.fail("ai_host_native_diagnostic_failed")
    mode = report["mode"]
    if mode == "bounds":
        if "verification" in report or "bounds" not in report:
            t.fail("ai_host_readiness_summary_invalid")
        v = report["bounds"]; t.fields(v, BOUNDS_FIELDS)
        if (v["scope"] != "diagnostic-unapproved-bounds-only" or
            any(v[key] is not False for key in BOUNDS_FALSE) or
            v["runtime_binding_sha256"] != report["runtime_binding_sha256"] or
            type(v["prior_ai_binding_matched"]) is not bool or type(v["next_cycle_required"]) is not bool):
            t.fail("ai_host_readiness_summary_invalid")
        for key in ("facts_sha256", "runtime_binding_sha256", "ai_bounds_sha256", "peer_bounds_sha256"):
            t.token(v[key], t.HASH)
        for key in ("ai_physical_objects", "ai_logical_objects", "peer_objects", "independent_epochs"):
            t.uint(v[key])
        if (v["ai_physical_objects"] not in (43, 53) or v["ai_logical_objects"] != 53 or
            v["peer_objects"] != 14 or v["independent_epochs"] != 2):
            t.fail("ai_host_readiness_summary_invalid")
        for name, key in (("ai.bounds.json", "ai_bounds_sha256"), ("peer.bounds.json", "peer_bounds_sha256")):
            h._file_baseline(t, output / name, v[key], 4 << 20)
    else:
        if "bounds" in report or "verification" not in report:
            t.fail("ai_host_readiness_summary_invalid")
        v = report["verification"]; t.fields(v, VERIFY_FIELDS)
        t.token(v["facts_sha256"], t.HASH); t.uint(v["originals"]); t.uint(v["independent_epochs"])
        if (v["scope"] != "actual-full-qs-ai-and-peer-two-epoch-database-facts-only" or
            any(v[key] is not False for key in VERIFY_FALSE) or v["independent_epochs"] != 2 or
            v["external_database_facts_observed"] is not True or v["originals"] != expected_rows):
            t.fail("ai_host_readiness_summary_invalid")
    return report, digest


def prepare(args, t):
    h = history_module()
    value = approval(args, t, h)
    values = _credentials(t)
    directory = t.operation_directory(args.root, args.operation_id)
    mode = "bounds" if args.prepare_mode == "bootstrap-ai-bounds" else "verify"
    binary = Path(args.history_binary)
    with t.locked_operation(directory):
        h._metadata_prior_handles(t, ["sudo", "-n", "docker"], directory,
                                  time.monotonic() + h.METADATA_LIMITS["total_seconds"])
        parent, baselines, inventory = h._parent(t, directory, value["history"], args)
        binary_hash = h._binary(t, binary, args.actual_source_sha)
        binary_baseline = h._metadata_snapshot(binary.lstat())
        derived = dict(parent, run_id=args.run_id)
        request_raw = t.canonical_bytes(derived); request_hash = hashlib.sha256(request_raw).hexdigest()
        registration = directory / ("ai-bootstrap-" + mode + "-" + args.run_id)
        # O_EXCL, no adoption of a partial prior registration or unknown child.
        if registration.exists() or registration.is_symlink():
            t.fail("ai_host_registration_exists_or_unavailable")
        registration.mkdir(mode=0o700)
        t.private_directory(registration)
        runtime = value["runtime"]
        descriptor = {"format_version": 1, "kind": "readonly_ai_external_host_input",
            "source_sha": args.actual_source_sha, "operation_id": args.operation_id, "actual_run_id": args.run_id,
            "mode": mode, "request_sha256": request_hash, "runtime_source_sha": runtime["source_sha"],
            "image_id": runtime["image_id"], "container_id": runtime["container_id"],
            "approved_runtime_binding_sha256": runtime["binding_sha256"], "assets_directory": str(Path(__file__).parent),
            "expected_ai_identity_hash": runtime["expected_ai_identity_hash"], "expected_ai_head": runtime["expected_ai_head"]}
        if mode == "verify":
            bounds = value["bounds"]
            prior = directory / ("ai-host-bounds-" + bounds["run_id"])
            files = (("ai_bounds", prior / "ai.bounds.json", bounds["ai_sha256"], 4 << 20),
                     ("peer_bounds", prior / "peer.bounds.json", bounds["peer_sha256"], 4 << 20),
                     ("protection", directory / "ai-message-protection.json", value["protection_sha256"], t.MAX_JSON))
            for key, path, digest, limit in files:
                baseline = h._file_baseline(t, path, digest, limit)
                baselines.append((path, digest, limit, baseline))
                descriptor[key] = {"path": str(path), "sha256": digest}
        descriptor_raw = t.canonical_bytes(descriptor); descriptor_hash = hashlib.sha256(descriptor_raw).hexdigest()
        t.create_bootstrap_file(registration, "history.request.json", request_raw)
        t.create_bootstrap_file(registration, "ai-host.input.json", descriptor_raw)
        record = {"format_version": 1, "kind": "readonly_ai_host_derivation_registration",
            "source_sha": args.actual_source_sha, "operation_id": args.operation_id, "actual_run_id": args.run_id,
            "mode": mode, "approval_sha256": args.bootstrap_approval_hash,
            "parent_run_id": parent["run_id"], "parent_request_sha256": value["history"]["parent_request"]["sha256"],
            "derived_request_sha256": request_hash, "descriptor_sha256": descriptor_hash,
            "history_binary_sha256": binary_hash, "only_parent_field_replaced": "run_id"}
        t.create_bootstrap_file(registration, "ai-host.registration.json", t.canonical_bytes(record))
        for name, digest in (("history.request.json", request_hash), ("ai-host.input.json", descriptor_hash),
                             ("ai-host.registration.json", hashlib.sha256(t.canonical_bytes(record)).hexdigest())):
            path = registration / name
            baselines.append((path, digest, t.MAX_JSON, h._file_baseline(t, path, digest, t.MAX_JSON)))
        h._unchanged(t, baselines)
    # No outer flock is held. Native quiescence/exec journals reconcile actual
    # old handles; no unknown attempt is restarted or marked complete here.
    output = directory / ("ai-host-" + mode + "-" + args.run_id)
    command = [str(binary), "--ai-host-mode", mode, "--request", str(registration / "history.request.json"),
        "--request-sha256", request_hash, "--operation", args.operation_id, "--run", args.run_id,
        "--output", str(output), "--operation-directory", str(directory),
        "--ai-input", str(registration / "ai-host.input.json"), "--ai-input-sha256", descriptor_hash]
    code, raw = _native(t, command, values)
    h._unchanged(t, baselines)
    if h._binary(t, binary, args.actual_source_sha) != binary_hash or h._metadata_snapshot(binary.lstat()) != binary_baseline:
        t.fail("ai_host_binary_changed")
    if code:
        t.fail("ai_host_native_diagnostic_failed_or_execution_unknown")
    rows = inventory["targets"][1].get("records")
    t.uint(rows)
    report, digest = _report(t, h, raw, output, descriptor, descriptor_hash, rows)
    return {"format_version": 1, "operation": "prepare", "source_sha": args.actual_source_sha,
        "operation_id": args.operation_id, "run_id": args.run_id, "prepare_mode": args.prepare_mode,
        "complete": False, "execution_allowed": False, "diagnostic_only": True, "drop_ready": False,
        "ai_host_readonly_complete": True, "ai_host_process_budget_proven": False,
        "ai_host_mode": mode, "ai_host_private_readiness_sha256": digest,
        "ai_host_runtime_binding_sha256": report["runtime_binding_sha256"],
        "ai_host_facts_sha256": (report.get("bounds") or report.get("verification"))["facts_sha256"],
        "ai_host_independent_epochs": 2, "ai_host_originals": rows if mode == "verify" else 0,
        "bootstrap_approval_sha256": args.bootstrap_approval_hash, "derived_request_sha256": request_hash,
        "ai_host_descriptor_sha256": descriptor_hash, "history_parent_request_sha256": value["history"]["parent_request"]["sha256"],
        **({"ai_host_ai_bounds_sha256": report["bounds"]["ai_bounds_sha256"],
            "ai_host_peer_bounds_sha256": report["bounds"]["peer_bounds_sha256"]} if mode == "bounds" else {}),
        "error_category": "ai_host_readonly_diagnostic_only", "blockers": list(REQUIRED),
        "capabilities": {key: False for key in t.CAPABILITIES}}
