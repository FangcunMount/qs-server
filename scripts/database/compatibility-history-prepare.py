"""Exact, diagnostic-only Action host for the read-only history executable.

The immutable descriptor authorizes derivation of one current-run request. It
does not authenticate production approval, fence writers or enable mutations.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import tempfile


MODE = "bootstrap-history"
LIMITS = {"memory_bytes": 3 << 30, "cpus": 2, "memory_swap_bytes": 3 << 30,
          "timeout_seconds": 1830}
MAX_SOURCE_FILE = (4 << 30) + (64 << 20)
IMAGE_VOLUMES = {"/var/lib/mysql": {}}
BOOL_FIELDS = frozenset({"completed_readonly_pipeline", "source_files_and_actual_origins_matched",
    "whole_four_source_coverage_complete", "business_and_responsibility_facts_unchanged",
    "independent_production_approval_verified", "ordered_mongo_source_metadata_approved",
    "host_process_budget_proven", "full_external_ai_closure_verified", "distributed_atomic_snapshot",
    "writer_fence_proven", "cas_complete", "post_cas_readback_complete", "backup_restore_qualified",
    "mutation_backend_enabled", "drop_ready"})
FORBIDDEN_TRUE = BOOL_FIELDS - {"completed_readonly_pipeline", "source_files_and_actual_origins_matched",
    "whole_four_source_coverage_complete", "business_and_responsibility_facts_unchanged"}
HASH_FIELDS = frozenset({"whole_source_index_sha256", "candidate_sha256", "sql_current_facts_sha256",
    "mongo_current_facts_sha256"})
UINT_FIELDS = frozenset({"local_candidates", "locally_qualified", "blocked_local", "joint_event_pages",
    "ai_blocked_pages", "sql_ledger_count", "mongo_collection_count", "elapsed_milliseconds"})
READINESS_FIELDS = BOOL_FIELDS | HASH_FIELDS | UINT_FIELDS | frozenset({"protocol", "source_sha",
    "operation_id", "run_id", "request_sha256", "inventory_request_sha256", "inventory_report_sha256",
    "independent_epochs", "sources", "full_source_file_sha256", "sql_global", "mongo_global",
    "blocking_reasons", "required_adapters", "error_category"})


def _reference(t, value):
    t.fields(value, ("run_id", "sha256"))
    t.token(value["run_id"], t.RUN); t.token(value["sha256"], t.HASH)


def approval(args, t):
    text = args.bootstrap_approval_json
    t.token(args.bootstrap_approval_hash, t.HASH)
    if type(text) is not str or not 0 < len(text) <= t.MAX_BOOTSTRAP_APPROVAL or not text.isascii():
        t.fail("history_approval_invalid")
    value = t.decode(text.encode("ascii"))
    raw = t.canonical_bytes(value)
    if raw[:-1] != text.encode("ascii") or hashlib.sha256(raw).hexdigest() != args.bootstrap_approval_hash:
        t.fail("history_approval_encoding_or_hash_invalid")
    t.fields(value, ("format_version", "kind", "prepare_mode", "source_sha", "operation_id",
                     "target_hash", "database_scope", "parent_request", "inventory_request_sha256",
                     "inventory_report", "assets", "process_limits"))
    if (type(value["format_version"]) is not int or value["format_version"] != 1 or
        value["kind"] != "readonly_history_bootstrap_approval" or value["prepare_mode"] != MODE or
        value["source_sha"] != args.actual_source_sha or value["operation_id"] != args.operation_id or
        value["target_hash"] != t.TARGET_HASH or value["database_scope"] != "mysql-and-mongodb"):
        t.fail("history_approval_binding_invalid")
    _reference(t, value["parent_request"]); _reference(t, value["inventory_report"])
    t.token(value["inventory_request_sha256"], t.HASH)
    if args.run_id in (value["parent_request"]["run_id"], value["inventory_report"]["run_id"]):
        t.fail("history_actual_run_reused")
    t.fields(value["process_limits"], LIMITS)
    if any(type(value["process_limits"][key]) is not int or value["process_limits"][key] != expected
           for key, expected in LIMITS.items()):
        t.fail("history_process_limits_invalid")
    if type(value["assets"]) is not list or len(value["assets"]) != 4:
        t.fail("history_approval_assets_invalid")
    for asset, target in zip(value["assets"], t.TARGETS):
        t.fields(asset, ("database", "name", "full_file_sha256", "full_file_bytes"))
        t.token(asset["full_file_sha256"], t.HASH); t.uint(asset["full_file_bytes"])
        if ((asset["database"], asset["name"]) != target[:2] or
            asset["full_file_bytes"] > MAX_SOURCE_FILE or
            (asset["full_file_bytes"] == 0 and target[0] != "mongodb")):
            t.fail("history_approval_assets_invalid")
    return value


def _file_baseline(t, path, expected, maximum):
    """Hash an exact owned regular file without retaining its payload."""
    t.private_directory(path.parent)
    try:
        before = path.lstat()
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    except OSError:
        t.fail("history_private_asset_unavailable")
    try:
        opened = os.fstat(fd)
        if (not stat.S_ISREG(opened.st_mode) or opened.st_uid != os.getuid() or
            stat.S_IMODE(opened.st_mode) != 0o600 or opened.st_nlink != 1 or
            opened.st_size > maximum or not os.path.samestat(before, opened)):
            t.fail("history_private_asset_invalid")
        digest = hashlib.sha256()
        count = 0
        while chunk := os.read(fd, 128 * 1024):
            count += len(chunk)
            if count > maximum:
                t.fail("history_private_asset_bound_exceeded")
            digest.update(chunk)
        after = os.fstat(fd)
        visible = path.lstat()
        baseline = lambda s: (s.st_dev, s.st_ino, s.st_size, s.st_mtime_ns, s.st_ctime_ns,
                              s.st_uid, s.st_mode, s.st_nlink)
        if baseline(before) != baseline(after) or baseline(after) != baseline(visible):
            t.fail("history_private_asset_changed")
        if digest.hexdigest() != expected:
            t.fail("history_private_asset_hash_mismatch")
        return baseline(after)
    finally:
        os.close(fd)


def _parent(t, directory, value, args):
    request, _ = t.read_private(directory, "history-request.json", value["parent_request"]["sha256"])
    t.fields(request, ("format_version", "kind", "source_sha", "operation_id", "run_id",
                       "inventory_request", "inventory_report", "assets"))
    if (type(request["format_version"]) is not int or request["format_version"] != 1 or
        request["kind"] != "readonly_compatibility_history_request" or
        request["source_sha"] != args.actual_source_sha or request["operation_id"] != args.operation_id or
        request["run_id"] != value["parent_request"]["run_id"]):
        t.fail("history_parent_binding_invalid")
    report_dir = directory / ("inventory-" + value["inventory_report"]["run_id"])
    expected_files = [(directory / "history-request.json", value["parent_request"]["sha256"], t.MAX_JSON),
        (directory / "inventory-request.json", value["inventory_request_sha256"], t.MAX_JSON),
        (report_dir / "inventory.private.json", value["inventory_report"]["sha256"], t.MAX_JSON)]
    for key, (path, digest, _) in zip(("inventory_request", "inventory_report"), expected_files[1:]):
        t.fields(request[key], ("path", "sha256"))
        if request[key] != {"path": str(path), "sha256": digest}:
            t.fail("history_parent_path_or_hash_invalid")
    if type(request["assets"]) is not list or len(request["assets"]) != 4:
        t.fail("history_parent_assets_invalid")
    for actual, approved, target in zip(request["assets"], value["assets"], t.TARGETS):
        t.fields(actual, ("database", "name", "path", "full_file_sha256", "full_file_bytes"))
        path = report_dir / t.SOURCE_FILENAMES[target[:2]]
        if actual != dict(approved, path=str(path)):
            t.fail("history_parent_assets_invalid")
        expected_files.append((path, approved["full_file_sha256"], MAX_SOURCE_FILE))
    baselines = [(path, digest, limit, _file_baseline(t, path, digest, limit))
                 for path, digest, limit in expected_files]
    if any(b[3][2] != a["full_file_bytes"] for b, a in zip(baselines[3:], value["assets"])):
        t.fail("history_parent_asset_size_invalid")
    report, _ = t.read_private(report_dir, "inventory.private.json", value["inventory_report"]["sha256"])
    if (report.get("source_sha") != args.actual_source_sha or report.get("operation_id") != args.operation_id or
        report.get("run_id") != value["inventory_report"]["run_id"] or
        report.get("request_hash") != value["inventory_request_sha256"] or
        report.get("complete") is not True or report.get("drop_ready") is not False or
        report.get("diagnostic_only") is not True or report.get("error_category") != "none" or
        type(report.get("targets")) is not list or len(report["targets"]) != 4):
        t.fail("history_inventory_parent_invalid")
    return request, baselines, report


def _unchanged(t, baselines):
    for path, digest, limit, original in baselines:
        if _file_baseline(t, path, digest, limit) != original:
            t.fail("history_parent_files_changed")


def _readiness_file(t, directory, filename="history.readiness.json"):
    # The existing evidence filename grammar intentionally excludes dots. Do
    # not broaden it for old entrypoints; this producer owns one exact filename.
    t.private_directory(directory)
    if filename not in {"history.readiness.json", "history.creation.intent.json", "history.terminal.json", "history.container.json"}:
        t.fail("history_private_filename_invalid")
    path = directory / filename
    try:
        before = path.lstat()
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    except OSError:
        t.fail("history_readiness_private_unavailable")
    try:
        opened = os.fstat(fd)
        if (not os.path.samestat(before, opened) or not stat.S_ISREG(opened.st_mode) or
            opened.st_uid != os.getuid() or stat.S_IMODE(opened.st_mode) != 0o600 or
            opened.st_nlink != 1 or not 0 < opened.st_size <= t.MAX_JSON):
            t.fail("history_readiness_private_invalid")
        with os.fdopen(fd, "rb", closefd=False) as stream:
            raw = stream.read(t.MAX_JSON + 1)
        after = os.fstat(fd)
        visible = path.lstat()
        snapshot = lambda s: (s.st_dev, s.st_ino, s.st_size, s.st_mtime_ns, s.st_ctime_ns,
                              s.st_uid, s.st_mode, s.st_nlink)
        if snapshot(before) != snapshot(after) or snapshot(after) != snapshot(visible) or len(raw) != opened.st_size:
            t.fail("history_readiness_private_changed")
        return t.decode(raw), hashlib.sha256(raw).hexdigest()
    finally:
        os.close(fd)


def _prior_runs(t, docker, directory):
    # A different GitHub attempt/name cannot evade an unresolved creation from
    # this same operation. Only the producer's exact terminal/removal marker,
    # followed by a new actual absence read, permits another diagnostic run.
    for prior in directory.iterdir():
        if not re.fullmatch(r"history-[0-9]{1,20}-[0-9]{1,4}", prior.name):
            continue
        t.private_directory(prior)
        intent_path = prior / "history.creation.intent.json"
        partial = prior / "history.creation.intent.json.bootstrap.partial"
        if partial.exists() or partial.is_symlink():
            t.fail("history_prior_container_outcome_unresolved")
        if not intent_path.exists() and not intent_path.is_symlink():
            continue  # No creation was attempted by this producer.
        terminal_path = prior / "history.terminal.json"
        if not terminal_path.exists() or terminal_path.is_symlink():
            t.fail("history_prior_container_outcome_unresolved")
        intent, _ = _readiness_file(t, prior, "history.creation.intent.json")
        terminal, _ = _readiness_file(t, prior, "history.terminal.json")
        container, _ = _readiness_file(t, prior, "history.container.json")
        saved, saved_hash = _readiness_file(t, prior / "output")
        t.fields(terminal, ("id", "name", "image", "actual_run_id", "source_sha", "request_sha256",
                            "creation_nonce", "owner_uid", "owner_gid", "status", "exit_code",
                            "container_removed", "private_readiness_sha256"))
        labels = intent.get("labels")
        if type(labels) is not dict or type(terminal["id"]) is not str or not re.fullmatch(r"[0-9a-f]{64}", terminal["id"]):
            t.fail("history_prior_container_outcome_unresolved")
        if (terminal["id"] != container.get("id") or container.get("name") != intent.get("name") or
            container.get("image") != intent.get("image") or container.get("labels") != labels or
            container.get("mounts") != intent.get("mounts") or container.get("limits") != LIMITS or
            terminal["private_readiness_sha256"] != saved_hash or saved.get("run_id") != terminal["actual_run_id"] or
            saved.get("source_sha") != terminal["source_sha"] or saved.get("request_sha256") != terminal["request_sha256"]):
            t.fail("history_prior_container_outcome_unresolved")
        if (terminal["status"] != "exited" or terminal["container_removed"] is not True or
            type(terminal["exit_code"]) is not int or terminal["owner_uid"] != os.getuid() or
            terminal["owner_gid"] != os.getgid() or terminal["name"] != intent.get("name") or
            terminal["image"] != intent.get("image") or terminal["actual_run_id"] != prior.name.removeprefix("history-") or
            terminal["actual_run_id"] != labels.get("qs.compatibility-retirement.run") or
            terminal["source_sha"] != labels.get("qs.compatibility-retirement.source") or
            terminal["request_sha256"] != labels.get("qs.compatibility-retirement.request") or
            terminal["creation_nonce"] != labels.get("qs.compatibility-retirement.creation")):
            t.fail("history_prior_container_outcome_unresolved")
        for key in ("request_sha256", "creation_nonce", "private_readiness_sha256"):
            t.token(terminal[key], t.HASH)
        t.token(terminal["source_sha"], t.SHA)
        for filter_value in ("id="+terminal["id"], "name=^/"+terminal["name"]+"$"):
            code, raw = t.capture_fixed([*docker, "container", "ls", "--all", "--filter", filter_value, "--format", "{{.ID}}"], timeout=15, maximum=128)
            if code or raw:
                t.fail("history_prior_container_outcome_unresolved")


def _binary(t, path, expected):
    t.private_directory(path.parent)
    try:
        meta = path.lstat()
    except OSError:
        t.fail("history_binary_unavailable")
    if (not path.is_absolute() or not stat.S_ISREG(meta.st_mode) or meta.st_uid != os.getuid()
        or stat.S_IMODE(meta.st_mode) != 0o700 or meta.st_nlink != 1 or meta.st_size > (256 << 20)):
        t.fail("history_binary_invalid")
    code, raw = t.capture_fixed([str(path), "--source-sha"], timeout=5, maximum=128)
    if code or raw != (json.dumps({"source_sha": expected}, separators=(",", ":")) + "\n").encode("ascii"):
        t.fail("history_binary_source_json_invalid")
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    except OSError:
        t.fail("history_binary_changed")
    try:
        opened = os.fstat(fd)
        digest = hashlib.sha256(); count = 0
        while chunk := os.read(fd, 128 * 1024):
            count += len(chunk)
            if count > (256 << 20):
                t.fail("history_binary_changed")
            digest.update(chunk)
        after = os.fstat(fd)
        visible = path.lstat()
        snapshot = lambda s: (s.st_dev, s.st_ino, s.st_size, s.st_mtime_ns, s.st_ctime_ns,
                              s.st_uid, s.st_mode, s.st_nlink)
        if snapshot(meta) != snapshot(opened) or snapshot(opened) != snapshot(after) or snapshot(after) != snapshot(visible):
            t.fail("history_binary_changed")
        return digest.hexdigest()
    finally:
        os.close(fd)


INSPECT_FORMAT = ('{"id":{{json .Id}},"name":{{json .Name}},"image":{{json .Image}},'
    '"labels":{{json .Config.Labels}},"mounts":{{json .Mounts}},'
    '"status":{{json .State.Status}},"running":{{json .State.Running}},'
    '"exit_code":{{json .State.ExitCode}},"oom":{{json .State.OOMKilled}},'
    '"memory":{{json .HostConfig.Memory}},"memory_swap":{{json .HostConfig.MemorySwap}},'
    '"nano_cpus":{{json .HostConfig.NanoCpus}},"network":{{json .HostConfig.NetworkMode}},'
    '"readonly":{{json .HostConfig.ReadonlyRootfs}},"user":{{json .Config.User}},'
    '"privileged":{{json .HostConfig.Privileged}},"pids":{{json .HostConfig.PidsLimit}},'
    '"cap_drop":{{json .HostConfig.CapDrop}},"security_opt":{{json .HostConfig.SecurityOpt}}}')


def _inspect(t, docker, cid, name, image, labels, mounts, *, network="infra-network"):
    code, raw = t.capture_fixed([*docker, "container", "inspect", cid, "--format", INSPECT_FORMAT], timeout=15)
    if code:
        t.fail("history_container_state_unknown")
    data = t.decode(raw)
    t.fields(data, ("id", "name", "image", "labels", "mounts", "status", "running", "exit_code",
                    "oom", "memory", "memory_swap", "nano_cpus", "network", "readonly", "user",
                    "privileged", "pids", "cap_drop", "security_opt"))
    if (data["id"] != cid or data["name"] != "/" + name or data["image"] != image or
        data["labels"] != labels or data["memory"] != LIMITS["memory_bytes"] or
        data["memory_swap"] != LIMITS["memory_swap_bytes"] or data["nano_cpus"] != 2_000_000_000 or
        data["network"] != network or data["readonly"] is not True or data["privileged"] is not False or
        data["pids"] != 64 or data["cap_drop"] != ["ALL"] or
        data["security_opt"] not in (["no-new-privileges"], ["no-new-privileges:true"]) or
        data["user"] != str(os.getuid()) + ":" + str(os.getgid()) or
        any(type(data[key]) is not int for key in ("memory", "memory_swap", "nano_cpus", "pids")) or
        type(data["running"]) is not bool or type(data["oom"]) is not bool or
        type(data["exit_code"]) is not int or type(data["status"]) is not str):
        t.fail("history_container_ownership_unproven")
    if type(data["mounts"]) is not list or len(data["mounts"]) != len(mounts):
        t.fail("history_container_mounts_unproven")
    actual = {}
    for mount in data["mounts"]:
        if (type(mount) is not dict or mount.get("Type") != "bind" or
            type(mount.get("RW")) is not bool or mount.get("Destination") in actual):
            t.fail("history_container_mounts_unproven")
        actual[mount.get("Destination")] = (mount.get("Source"), mount["RW"])
    if actual != mounts:
        t.fail("history_container_mounts_unproven")
    return data


def _creation_unknown(t, docker, name, image, labels, mounts, output):
    # A creation timeout may have created a container even without a returned
    # ID. Observe by this unique intent name, then pin its actual full ID. This
    # never makes it safe to start/remove/retry the uncertain creation.
    code, raw = t.capture_fixed([*docker, "container", "inspect", name, "--format", INSPECT_FORMAT], timeout=15)
    if code:
        t.fail("history_container_state_unknown")
    observed = t.decode(raw)
    cid = observed.get("id") if type(observed) is dict else None
    if type(cid) is not str or not re.fullmatch(r"[0-9a-f]{64}", cid):
        t.fail("history_container_state_unknown")
    state = _inspect(t, docker, cid, name, image, labels, mounts)
    t.create_bootstrap_file(output, "history.creation.unknown.json", t.canonical_bytes({
        "id": cid, "name": name, "status": state["status"], "running": state["running"],
        "creation_outcome": "unknown", "execution_permitted": False}))
    t.fail("history_container_creation_unknown")


def _receipt(t, summary, raw, code, args, directory, request, derived_hash, approval_hash, parent_hash, inventory):
    t.fields(summary, READINESS_FIELDS)
    if (summary["protocol"] != "qs-compatibility-history-readonly/v1" or
        summary["source_sha"] != args.actual_source_sha or summary["operation_id"] != args.operation_id or
        summary["run_id"] != args.run_id or summary["request_sha256"] != derived_hash or
        summary["inventory_request_sha256"] != request["inventory_request"]["sha256"] or
        summary["inventory_report_sha256"] != request["inventory_report"]["sha256"] or
        summary["full_source_file_sha256"] != [a["full_file_sha256"] for a in request["assets"]]):
        t.fail("history_readiness_binding_invalid")
    for key in BOOL_FIELDS:
        if type(summary[key]) is not bool or (key in FORBIDDEN_TRUE and summary[key]):
            t.fail("history_readiness_authority_invalid")
    for key in HASH_FIELDS:
        if summary[key] != "":
            t.token(summary[key], t.HASH)
    for key in UINT_FIELDS:
        t.uint(summary[key])
    sql = summary["sql_global"]
    t.fields(sql, ("observed", "retirement_related", "outside_retirement", "unknown", "blocking", "schema_coverage"))
    for key in sql:
        if key != "schema_coverage":
            t.uint(sql[key])
    if type(sql["schema_coverage"]) is not str:
        t.fail("history_global_summary_invalid")
    mongo = summary["mongo_global"]
    t.fields(mongo, ("rows", "classified_rows", "class_counts", "blocking_reasons", "coverage_gaps"))
    t.uint(mongo["rows"]); t.uint(mongo["classified_rows"])
    for collection in (summary["blocking_reasons"], mongo["class_counts"]):
        if type(collection) is not dict or len(collection) > 1024:
            t.fail("history_global_summary_invalid")
        for key, v in collection.items():
            if type(key) is not str or not re.fullmatch(r"[a-z][a-z0-9_.-]{0,127}", key):
                t.fail("history_global_summary_invalid")
            t.uint(v)
    for collection in (summary["required_adapters"], mongo["blocking_reasons"], mongo["coverage_gaps"]):
        if type(collection) is not list or len(collection) > 1024 or any(
            type(v) is not str or not re.fullmatch(r"[a-z][a-z0-9_.-]{0,127}", v) for v in collection):
            t.fail("history_global_summary_invalid")
    if type(summary["sources"]) is not list or len(summary["sources"]) != 4:
        t.fail("history_sources_invalid")
    for source, target, original in zip(summary["sources"], t.TARGETS, inventory["targets"]):
        t.fields(source, ("protocol", "records", "source_bytes", "data_hash", "complete", "business_closure_verified", "drop_ready"))
        t.uint(source["records"]); t.uint(source["source_bytes"])
        if (type(source["complete"]) is not bool or source["business_closure_verified"] is not False or
            source["drop_ready"] is not False):
            t.fail("history_sources_invalid")
        if summary["completed_readonly_pipeline"] and (source["protocol"] != (
            "mysql_cast_binary_columns_pk_order_v2" if target[0] == "mysql" else "mongodb_server_bson_pk_order_v2") or
            source["complete"] is not True or source["records"] != original.get("records") or
            source["source_bytes"] != original.get("bytes") or source["data_hash"] != original.get("data_hash")):
            t.fail("history_sources_invalid")
    if (type(summary["independent_epochs"]) is not int or not 0 <= summary["independent_epochs"] <= 2 or
        summary["local_candidates"] != summary["locally_qualified"] + summary["blocked_local"]):
        t.fail("history_readiness_counts_invalid")
    saved, digest = _readiness_file(t, directory)
    # Both producer channels must match. A failed CLI that did not durably write
    # this exact private receipt is not promoted into any completion result.
    if saved != summary or hashlib.sha256(raw).hexdigest() != digest:
        t.fail("history_readiness_private_mismatch")
    completed = summary["completed_readonly_pipeline"]
    if completed != (code == 0) or (completed and (summary["independent_epochs"] != 2 or
        summary["error_category"] != "none" or not all(summary[key] for key in
        ("source_files_and_actual_origins_matched", "whole_four_source_coverage_complete",
         "business_and_responsibility_facts_unchanged")) or any(not summary[key] for key in HASH_FIELDS))):
        t.fail("history_readiness_outcome_invalid")
    return {"format_version": 1, "operation": "prepare", "prepare_mode": MODE,
        "source_sha": args.actual_source_sha, "operation_id": args.operation_id, "run_id": args.run_id,
        "target_hash": t.TARGET_HASH, "target_count": 4, "complete": False,
        "execution_allowed": False, "drop_ready": False, "diagnostic_only": True,
        "history_readonly_complete": completed, "history_independent_epochs": summary["independent_epochs"],
        "history_private_readiness_sha256": digest, "bootstrap_approval_sha256": approval_hash,
        "history_parent_request_sha256": parent_hash, "derived_request_sha256": derived_hash,
        "history_local_candidates": summary["local_candidates"], "history_locally_qualified": summary["locally_qualified"],
        "history_blocked_local": summary["blocked_local"], "history_ai_blocked_pages": summary["ai_blocked_pages"],
        "history_cas_complete": False, "history_process_budget_proven": False,
        "history_source_rows": [source["records"] for source in summary["sources"]],
        "history_global_sql": {key: v for key, v in sql.items() if key != "schema_coverage"},
        "history_global_mongodb": {"rows": mongo["rows"], "classified_rows": mongo["classified_rows"],
            "blocking_reason_count": len(mongo["blocking_reasons"]), "coverage_gap_count": len(mongo["coverage_gaps"])},
        "error_category": "history_readonly_completed_diagnostic" if completed else "history_readonly_blocked"}


def prepare(args, t):
    value = approval(args, t)
    directory = t.operation_directory(args.root, args.operation_id)
    binary = Path(args.history_binary)
    binary_hash = _binary(t, binary, args.actual_source_sha)
    values = t.inventory_connection_values(os.environ)
    docker = ["sudo", "-n", "docker"]
    with t.locked_operation(directory):
        _prior_runs(t, docker, directory)
        parent, baselines, inventory = _parent(t, directory, value, args)
        output = directory / ("history-" + args.run_id)
        try:
            output.mkdir(mode=0o700)
            result_directory = output / "output"
            result_directory.mkdir(mode=0o700)
        except OSError:
            t.fail("history_run_directory_exists_or_unavailable")
        request = dict(parent, run_id=args.run_id)
        derived_raw = t.canonical_bytes(request)
        derived_hash = hashlib.sha256(derived_raw).hexdigest()
        registration = {"format_version": 1, "kind": "immutable_history_run_derivation",
            "source_sha": args.actual_source_sha, "operation_id": args.operation_id, "actual_run_id": args.run_id,
            "approval_sha256": args.bootstrap_approval_hash, "approval": value,
            "parent_request_sha256": value["parent_request"]["sha256"],
            "parent_run_id": parent["run_id"], "derived_request_sha256": derived_hash,
            "only_changed_field": "run_id", "binary_sha256": binary_hash}
        t.create_bootstrap_file(output, "history.bootstrap.json", t.canonical_bytes(registration))
        t.create_bootstrap_file(output, "history.request.json", derived_raw)
        for filename in ("history.bootstrap.json", "history.request.json"):
            path = output / filename
            digest = hashlib.sha256(path.read_bytes()).hexdigest()
            baselines.append((path, digest, t.MAX_JSON, _file_baseline(t, path, digest, t.MAX_JSON)))
        request_path = output / "history.request.json"
        code, raw = t.capture_fixed([*docker, "image", "inspect", "mysql:8.0", "--format", "{{.Id}}"], timeout=15, maximum=256)
        image = raw.decode("ascii", errors="strict").strip()
        if code or not re.fullmatch(r"sha256:[0-9a-f]{64}", image):
            t.fail("history_runtime_image_unavailable")
        code, raw = t.capture_fixed([*docker, "image", "inspect", image, "--format", "{{json .Config.Volumes}}"], timeout=15)
        if code or t.decode(raw) != IMAGE_VOLUMES:
            t.fail("history_runtime_image_volumes_unsupported")
        code, _ = t.capture_fixed([*docker, "network", "inspect", "infra-network"], timeout=15)
        if code:
            t.fail("history_runtime_network_unavailable")
        name = "qs-compatibility-history-" + args.run_id
        code, raw = t.capture_fixed([*docker, "container", "ls", "--all", "--filter", "name=^/"+name+"$", "--format", "{{.ID}}"], timeout=15, maximum=256)
        if code or raw:
            t.fail("history_container_name_not_available")
        labels = {"qs.compatibility-retirement.operation": args.operation_id,
            "qs.compatibility-retirement.run": args.run_id, "qs.compatibility-retirement.source": args.actual_source_sha,
            "qs.compatibility-retirement.request": derived_hash, "qs.compatibility-retirement.kind": "history-readonly"}
        labels["qs.compatibility-retirement.creation"] = os.urandom(32).hex()
        code, raw = t.capture_fixed([*docker, "image", "inspect", image, "--format", "{{json .Config.Labels}}"], timeout=15)
        inherited = t.decode(raw) if raw.strip() != b"null" else {}
        if code or type(inherited) is not dict or any(type(k) is not str or type(v) is not str for k, v in inherited.items()) or set(inherited) & set(labels):
            t.fail("history_image_labels_unproven")
        labels = dict(inherited, **labels)
        shadow = output / "mysql-volume-shadow"
        try:
            shadow.mkdir(mode=0o700)
        except OSError:
            t.fail("history_image_volume_shadow_unavailable")
        t.private_directory(shadow)
        mounts = {"/history-tool": (str(binary), False), str(directory): (str(directory), False),
                  str(result_directory): (str(result_directory), True), "/var/lib/mysql": (str(shadow), False)}
        with tempfile.TemporaryDirectory(prefix="qs-history-env-") as temporary:
            os.chmod(temporary, 0o700)
            env_file = Path(temporary) / "history.env"
            fd = os.open(env_file, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
            with os.fdopen(fd, "w") as stream:
                stream.write("".join(key+"="+v+"\n" for key, v in values.items()))
                stream.flush(); os.fsync(stream.fileno())
            command = [*docker, "create", "--name", name, "--pull=never", "--network", "infra-network",
                "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--cpus=2",
                "--memory=3g", "--memory-swap=3g", "--pids-limit=64", "--user", str(os.getuid())+":"+str(os.getgid())]
            for key, v in labels.items():
                command.extend(("--label", key+"="+v))
            for target, (source, rw) in mounts.items():
                command.extend(("--mount", "type=bind,source="+source+",target="+target+("" if rw else ",readonly")))
            command.extend(("--env-file", str(env_file), "--entrypoint", "/history-tool", image,
                "--request", str(request_path), "--request-sha256", derived_hash,
                "--operation", args.operation_id, "--run", args.run_id, "--output", str(result_directory)))
            t.create_bootstrap_file(output, "history.creation.intent.json", t.canonical_bytes({
                "name": name, "image": image, "labels": labels,
                "declared_image_volumes": IMAGE_VOLUMES,
                "mounts": {key: {"source": v[0], "writable": v[1]} for key, v in mounts.items()},
                "limits": LIMITS, "owner_uid": os.getuid(), "owner_gid": os.getgid()}))
            try:
                code, raw = t.capture_fixed(command, timeout=30, maximum=128)
            except t.Blocked:
                _creation_unknown(t, docker, name, image, labels, mounts, output)
            try:
                cid = raw.decode("ascii", errors="strict").strip()
            except UnicodeError:
                _creation_unknown(t, docker, name, image, labels, mounts, output)
            if code or not re.fullmatch(r"[0-9a-f]{64}", cid):
                _creation_unknown(t, docker, name, image, labels, mounts, output)
            initial = _inspect(t, docker, cid, name, image, labels, mounts)
            if initial["status"] != "created" or initial["running"] or initial["oom"] or initial["exit_code"] != 0:
                t.fail("history_container_execution_unknown")
            t.create_bootstrap_file(output, "history.container.json", t.canonical_bytes({
                "id": cid, "name": name, "image": image, "labels": labels,
                "mounts": {key: {"source": v[0], "writable": v[1]} for key, v in mounts.items()},
                "limits": LIMITS, "owner_uid": os.getuid(), "owner_gid": os.getgid()}))
            try:
                code, raw = t.capture_fixed([*docker, "start", "--attach", cid], timeout=LIMITS["timeout_seconds"], maximum=t.MAX_JSON)
            except t.Blocked:
                # Inspect first, retain the owned handle for reconciliation. No
                # automatic stop/remove/restart, even if an attach timed out.
                _inspect(t, docker, cid, name, image, labels, mounts)
                t.fail("history_container_execution_unknown")
        state = _inspect(t, docker, cid, name, image, labels, mounts)
        if state["running"] or state["status"] != "exited" or state["exit_code"] != code or state["oom"]:
            t.fail("history_container_execution_unknown")
        _unchanged(t, baselines)
        if _binary(t, binary, args.actual_source_sha) != binary_hash:
            t.fail("history_binary_changed")
        summary = t.decode(raw)
        receipt = _receipt(t, summary, raw, code, args, result_directory, request,
                           derived_hash, args.bootstrap_approval_hash, value["parent_request"]["sha256"], inventory)
        # This is the exact, terminal, previously inspected self-created ID;
        # no name lookup, wildcard, force removal or shared-resource cleanup.
        code, _ = t.capture_fixed([*docker, "container", "rm", cid], timeout=15, maximum=128)
        if code:
            t.fail("history_owned_container_cleanup_unconfirmed")
        code, remaining = t.capture_fixed([*docker, "container", "ls", "--all", "--filter", "id="+cid, "--format", "{{.ID}}"], timeout=15, maximum=128)
        if code or remaining:
            t.fail("history_owned_container_cleanup_unconfirmed")
        t.create_bootstrap_file(output, "history.terminal.json", t.canonical_bytes({
            "id": cid, "name": name, "image": image, "actual_run_id": args.run_id,
            "source_sha": args.actual_source_sha, "request_sha256": derived_hash,
            "creation_nonce": labels["qs.compatibility-retirement.creation"],
            "owner_uid": os.getuid(), "owner_gid": os.getgid(), "status": "exited",
            "exit_code": state["exit_code"], "container_removed": True,
            "private_readiness_sha256": receipt["history_private_readiness_sha256"]}))
        return receipt
