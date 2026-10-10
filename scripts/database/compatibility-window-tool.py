#!/usr/bin/env python3
"""Existing pinned management caller for an independent maintenance CLI.

It installs/calls only a hash-approved CLI. It does not deploy an API, alter a
schema, construct a fence/Window/restore proof, or activate missing adapters.
The same strict template supports A preparation and a separately approved B CLI.
"""
import argparse
import base64
import copy
import hashlib
import io
import importlib.util
import json
import os
from pathlib import Path
import platform
import re
import select
import signal
import socket
import stat
import subprocess
import sys
import tarfile
import time

SHA = re.compile(r"[0-9a-f]{40}")
HASH = re.compile(r"[0-9a-f]{64}")
RUN = re.compile(r"[1-9][0-9]{0,19}-[1-9][0-9]{0,3}")
TARGET = "4e09444409d1cfab3c9df383ed80a0999419fcbbd2f4e80d2e5dac1cf7dfcfb8"
STAGES = {"prepare", "apply", "verify", "recover", "purge"}
CREDENTIALS = ("MYSQL_HOST", "MYSQL_PORT", "MYSQL_USERNAME", "MYSQL_PASSWORD", "MYSQL_DATABASE", "MONGODB_HOST", "MONGODB_PORT", "MONGODB_USERNAME", "MONGODB_PASSWORD", "MONGODB_DBNAME", "MONGODB_METADATA_ADMIN_USERNAME", "MONGODB_METADATA_ADMIN_PASSWORD")

READ_TOKEN = "GITHUB_READ_TOKEN"
SERVICE_CREDENTIALS = ("RETIREMENT_SERVICE_SSH_HOST", "RETIREMENT_SERVICE_SSH_USERNAME", "RETIREMENT_SERVICE_SSH_PORT", "RETIREMENT_SERVICE_SSH_KEY", "RETIREMENT_SERVICE_SSH_FINGERPRINT")
SERVICE_KEY = "RETIREMENT_SERVICE_SSH_KEY"


def credential_names(stage):
    if stage not in STAGES:
        reject()
    # Preparation retains the exact original twelve-credential packet. Only a
    # future effectful caller may borrow this run's short-lived GET credential.
    return CREDENTIALS if stage == "prepare" else CREDENTIALS + (READ_TOKEN,) + SERVICE_CREDENTIALS


def validate_credentials(credentials, stage):
    if type(credentials) is not dict or set(credentials) != set(credential_names(stage)) or any(type(v) is not str or len(v) > 8192 or "\x00" in v or "\r" in v or ("\n" in v and k != SERVICE_KEY) for k, v in credentials.items()):
        reject("window_tool_credentials_rejected")


class Refused(Exception):
    pass


def reject(category="window_tool_binding_rejected"):
    raise Refused(category)


def exact(value, required, optional=()):
    if type(value) is not dict or not set(required) <= set(value) or set(value) - set(required) - set(optional):
        reject()


def token(value, pattern):
    if type(value) is not str or pattern.fullmatch(value) is None:
        reject()


def decode(raw):
    def unique(items):
        result = {}
        for key, value in items:
            if key in result:
                reject("window_tool_json_rejected")
            result[key] = value
        return result
    try:
        return json.loads(raw, object_pairs_hook=unique, parse_constant=lambda _: reject("window_tool_json_rejected"))
    except Refused:
        raise
    except (ValueError, UnicodeError):
        reject("window_tool_json_rejected")


def canonical(value):
    return (json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True) + "\n").encode("ascii")


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def approve(raw, expected, dispatcher, stage, operation, manifest, template):
    token(expected, HASH)
    token(dispatcher, SHA)
    token(operation, RUN)
    token(manifest, HASH)
    token(template, HASH)
    if stage not in STAGES or type(raw) is not str or not 1 <= len(raw) <= 4096:
        reject()
    try:
        supplied = (raw + "\n").encode("ascii")
    except UnicodeError:
        reject()
    a = decode(supplied)
    fields = ("format_version", "kind", "dispatcher_source_sha", "tool_source_sha", "original_source_sha", "operation_id", "original_run_id", "stage", "target_hash", "manifest_sha256", "request_template_sha256", "tool_binary_sha256", "b_image_id", "b_program_sha256")
    exact(a, fields if stage == "prepare" else fields + ("workflow_scope",), ("local_descriptor_sha256",) if stage == "prepare" else ())
    if canonical(a) != supplied or digest(supplied) != expected or type(a["format_version"]) is not int or a["format_version"] != 1 or a["kind"] != "independent_compatibility_window_tool_approval" or a["dispatcher_source_sha"] != dispatcher or a["stage"] != stage or a["operation_id"] != operation or a["manifest_sha256"] != manifest or a["request_template_sha256"] != template or a["target_hash"] != TARGET:
        reject()
    for key in ("tool_source_sha", "original_source_sha"):
        token(a[key], SHA)
    token(a["original_run_id"], RUN)
    exact(a["tool_binary_sha256"], ("amd64", "arm64"))
    for value in a["tool_binary_sha256"].values():
        token(value, HASH)
    if "local_descriptor_sha256" in a:
        token(a["local_descriptor_sha256"], HASH)
    # Earlier prepare-facts supplies actual cached B image/program identity.
    # Preparation and effectful stages both require that immutable pair.
    token(a["b_image_id"], re.compile(r"sha256:[0-9a-f]{64}"))
    token(a["b_program_sha256"], HASH)
    if stage != "prepare":
        validate_workflow_scope(a["workflow_scope"], a)
    return a


def validate_workflow_scope(scope, approval):
    """Approved identities only; no API state or execution result is imported."""
    exact(scope, ("format_version", "kind", "dispatcher_source_sha", "tool_source_sha", "original_source_sha", "operation_id", "original_run_id", "manifest_sha256", "repository_id", "owner_id", "actor_id", "workflow_id", "workflow_ids", "job_name", "runner_id"))
    if type(scope["format_version"]) is not int or scope["format_version"] != 1 or scope["kind"] != "approved_runner_workflow_quarantine_scope":
        reject("window_tool_workflow_scope_rejected")
    for key in ("dispatcher_source_sha", "tool_source_sha", "original_source_sha", "operation_id", "original_run_id", "manifest_sha256"):
        if scope[key] != approval[key]:
            reject("window_tool_workflow_scope_rejected")
    for key in ("repository_id", "owner_id", "actor_id"):
        token(scope[key], re.compile(r"[1-9][0-9]{0,19}"))
    for key in ("workflow_id", "runner_id"):
        if type(scope[key]) is not int or not 0 < scope[key] < 1 << 63:
            reject("window_tool_workflow_scope_rejected")
    ids = scope["workflow_ids"]
    if type(ids) is not list or not 1 <= len(ids) <= 1000 or any(type(i) is not int or not 0 < i < 1 << 63 for i in ids) or ids != sorted(set(ids)) or scope["workflow_id"] not in ids:
        reject("window_tool_workflow_scope_rejected")
    token(scope["job_name"], re.compile(r"[A-Za-z0-9 ()_.-]{1,200}"))


def derive_request(raw, approval, current_run):
    """Bind only a newly assigned run; preserve all original facts byte-for-value."""
    token(current_run, RUN)
    if current_run == approval["original_run_id"]:
        reject("window_tool_current_run_not_new")
    if digest(raw) != approval["request_template_sha256"]:
        reject("window_tool_template_hash_rejected")
    r = decode(raw)
    required = ("format_version", "kind", "tool_source_sha", "original_source_sha", "operation_id", "actual_run_id", "manifest_sha256", "archive_directory", "window_directory", "journal_directory", "archive_approval", "recovery")
    optional = ("source_directory", "restore_engines", "source_file_sha256", "service_control", "resume", "resume_kind", "deployment_control", "final_history", "writer_control", "source_copy_intent", "historical_write_report", "preparation_restore_zero")
    exact(r, required, optional)
    if type(r["format_version"]) is not int or r["format_version"] != 1 or r["kind"] != "compatibility_retirement_lifecycle_request" or r["tool_source_sha"] != approval["tool_source_sha"] or r["original_source_sha"] != approval["original_source_sha"] or r["operation_id"] != approval["operation_id"] or r["actual_run_id"] != "" or r["manifest_sha256"] != approval["manifest_sha256"]:
        reject("window_tool_template_binding_rejected")
    a = r["archive_approval"]
    exact(a, ("InventorySHA256", "SQLMetadataSHA256", "MongoMetadataSHA256", "OrderedMongoSchemaSHA256", "SourceSHA", "OperationID", "RunID", "RequestHash"))
    if a["SourceSHA"] != approval["original_source_sha"] or a["OperationID"] != approval["operation_id"] or a["RunID"] != approval["original_run_id"]:
        reject("window_tool_original_binding_rejected")
    q = r["recovery"]
    exact(q, ("source_sha", "operation_id", "original_run_id", "actual_run_id", "manifest_sha256", "archive_sha256", "mysql_non_target_sha256", "mongodb_non_target_sha256", "mysql_head", "mongodb_head"))
    if q["source_sha"] != approval["original_source_sha"] or q["operation_id"] != approval["operation_id"] or q["original_run_id"] != approval["original_run_id"] or q["manifest_sha256"] != approval["manifest_sha256"] or type(q["mysql_head"]) is not int or type(q["mongodb_head"]) is not int or q["mysql_head"] != 99 or q["mongodb_head"] != 38:
        reject("window_tool_original_binding_rejected")
    result = copy.deepcopy(r)
    result["actual_run_id"] = current_run
    if approval["stage"] == "prepare":
        if any(key in r for key in ("resume", "resume_kind", "service_control", "deployment_control", "final_history", "writer_control", "source_copy_intent", "historical_write_report", "preparation_restore_zero")) or q["archive_sha256"] != "":
            reject("window_tool_prepare_effect_fields_rejected")
    if "writer_control" in r:
        exact(r["writer_control"], ("workflow_scope_sha256",), ("database_input",))
        token(r["writer_control"]["workflow_scope_sha256"], HASH)
        if "database_input" in r["writer_control"]:
            file=r["writer_control"]["database_input"]
            exact(file,("path","sha256"));token(file["sha256"],HASH)
            root=Path('/opt/backups/qs-server/compatibility-retirement',approval['operation_id'])
            p=file['path']
            census_root='/opt/backups/qs-server/compatibility-retirement-root-prepare/'
            census_pattern=re.escape(census_root+approval['operation_id']+'-')+RUN.pattern+r'/db-writer-census\.private\.json'
            census=isinstance(p,str) and re.fullmatch(census_pattern,p) is not None
            if not isinstance(p,str) or not os.path.isabs(p) or os.path.normpath(p)!=p or not census and (not Path(p).is_relative_to(root) or p==str(root)):
                reject("window_tool_database_writer_input_rejected")
    if approval["stage"] != "prepare":
        validate_workflow_scope(approval.get("workflow_scope"), approval)
        if "writer_control" not in r or r["writer_control"]["workflow_scope_sha256"] != digest(canonical(approval["workflow_scope"])):
            reject("window_tool_workflow_scope_rejected")
    if "final_history" in r:
        validate_final_history(r["final_history"], approval["operation_id"])
    if "source_copy_intent" in r:
        value = r["source_copy_intent"]
        exact(value, ("path", "sha256"))
        token(value["sha256"], HASH)
        path = value["path"]
        if type(path) is not str or os.path.normpath(path) != path or Path(path).name != "source-copy.intent.private.json":
            reject("window_tool_source_copy_intent_rejected")
        prefix = approval["operation_id"] + "-"
        batch = Path(path).parent.name
        run = batch[len(prefix):] if batch.startswith(prefix) else ""
        token(run, RUN)
        if run in (current_run, approval["original_run_id"]) or path != str(Path("/opt/backups/qs-server/compatibility-retirement-root-prepare", batch, "source-copy.intent.private.json")):
            reject("window_tool_source_copy_intent_rejected")
    if "preparation_restore_zero" in r:
        value=r["preparation_restore_zero"]
        exact(value,("path","sha256"));token(value["sha256"],HASH)
        if "source_copy_intent" not in r or type(value["path"]) is not str:
            reject("window_tool_source_copy_intent_rejected")
        intent=Path(r["source_copy_intent"]["path"])
        batch=intent.parent.name
        run=batch[len(approval["operation_id"])+1:]
        if value["path"]!=str(intent.parent/("lifecycle-restore-"+run+".zero.private.json")):
            reject("window_tool_source_copy_intent_rejected")
    if "historical_write_report" in r:
        value=r["historical_write_report"]
        exact(value,("path","sha256"));token(value["sha256"],HASH)
        path=value["path"]
        if type(path) is not str or os.path.normpath(path)!=path or Path(path).name!="history.write.json":
            reject("window_tool_history_material_rejected")
        parent=Path(path).parent.name
        run=parent[len("history-write-"):] if parent.startswith("history-write-") else ""
        token(run,RUN)
        if run==current_run or path!=str(Path("/opt/backups/qs-server/compatibility-retirement",approval["operation_id"],"history-write-"+run,"history.write.json")):
            reject("window_tool_history_material_rejected")
    if "resume" not in r:
        if "resume_kind" in r or q["actual_run_id"] != "":
            reject("window_tool_current_run_not_empty")
        result["recovery"]["actual_run_id"] = current_run
    else:
        if approval["stage"] != "recover" or r.get("resume_kind") not in ("original", "b_complete", "b_sql_only"):
            reject("window_tool_resume_binding_rejected")
        resume = r["resume"]
        exact(resume, ("Recovery", "ApprovedBSourceSHA", "MigrationIntentSHA256", "MigrationResultSHA256"))
        exact(resume["Recovery"], ("Original", "CurrentRunID", "JournalSHA256", "WindowStartSHA256"))
        if resume["Recovery"]["Original"] != q or resume["Recovery"]["CurrentRunID"] != "":
            reject("window_tool_resume_binding_rejected")
        token(q["actual_run_id"], RUN)
        result["resume"]["Recovery"]["CurrentRunID"] = current_run
    return canonical(result)


def validate_final_history(value, operation):
    exact(value, ("assets_directory", "runtime_source_sha", "image_id", "container_id", "runtime_binding_sha256", "ai_bounds", "peer_bounds", "protection"), ("stop_constraints",))
    token(value["runtime_source_sha"], SHA)
    token(value["container_id"], HASH)
    token(value["runtime_binding_sha256"], HASH)
    if "stop_constraints" in value:
        constraints=value["stop_constraints"]
        exact(constraints,("settings_sha256","network_id"))
        token(constraints["settings_sha256"],HASH)
        token(constraints["network_id"],HASH)
    if not isinstance(value["image_id"], str) or not value["image_id"].startswith("sha256:"):
        reject("window_tool_final_history_rejected")
    token(value["image_id"][7:], HASH)
    assets=value["assets_directory"]
    if not isinstance(assets,str) or not os.path.isabs(assets) or os.path.normpath(assets)!=assets:
        reject("window_tool_final_history_rejected")
    root=Path('/opt/backups/qs-server/compatibility-retirement',operation)
    seen=set()
    for key in ("ai_bounds","peer_bounds","protection"):
        file=value[key]
        exact(file,("path","sha256"));token(file["sha256"], HASH)
        p=file["path"]
        if not isinstance(p,str) or os.path.normpath(p)!=p or not Path(p).is_relative_to(root) or p==str(root) or p in seen:
            reject("window_tool_final_history_rejected")
        seen.add(p)


def protected_directory(path, *, create=False):
    if create:
        path.mkdir(mode=0o700)
    st = path.lstat()
    if not stat.S_ISDIR(st.st_mode) or st.st_uid != 0 or stat.S_IMODE(st.st_mode) != 0o700:
        reject("window_tool_protected_directory_rejected")
    for parent in path.parents:
        info = parent.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or stat.S_IMODE(info.st_mode) & 0o022:
            reject("window_tool_protected_directory_rejected")


def read_owned(path, owner, expected, maximum, mode=0o600):
    token(expected, HASH)
    parent = path.parent.lstat()
    if not stat.S_ISDIR(parent.st_mode) or parent.st_uid != owner or stat.S_IMODE(parent.st_mode) != 0o700:
        reject("window_tool_source_owner_rejected")
    for ancestor in path.parent.parents:
        st = ancestor.lstat()
        if not stat.S_ISDIR(st.st_mode) or stat.S_IMODE(st.st_mode) & 0o022 and not st.st_mode & stat.S_ISVTX:
            reject("window_tool_source_owner_rejected")
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as f:
        before = os.fstat(f.fileno())
        if not stat.S_ISREG(before.st_mode) or before.st_uid != owner or before.st_nlink != 1 or stat.S_IMODE(before.st_mode) != mode or not 0 < before.st_size <= maximum:
            reject("window_tool_source_file_rejected")
        raw = f.read(maximum + 1)
        after = os.fstat(f.fileno())
    identity = lambda v: (v.st_dev, v.st_ino, v.st_uid, v.st_mode, v.st_nlink, v.st_size, v.st_mtime_ns, v.st_ctime_ns)
    if len(raw) > maximum or identity(before) != identity(after) or identity(before) != identity(path.lstat()) or digest(raw) != expected:
        reject("window_tool_source_changed")
    return raw


def sync_directory(path):
    fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def write_new(path, raw, mode=0o600):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, mode)
    with os.fdopen(fd, "wb") as f:
        f.write(raw)
        f.flush()
        os.fsync(f.fileno())
    sync_directory(path.parent)


# This owner concerns local physical children only. A daemon exec, remote SSH
# action or database statement with an unknown result remains unknown.
class LinuxChildOwner:
    def __init__(self):
        import ctypes
        if platform.system() != "Linux" or os.getuid() != 0 or os.geteuid() != 0 or not hasattr(os, "pidfd_open") or not hasattr(signal, "pidfd_send_signal"):
            reject("window_tool_local_owner_unavailable")
        self.pid = os.getpid()
        self.libc = ctypes.CDLL(None, use_errno=True)
        actual = ctypes.c_int()
        if self.libc.prctl(36, 1, 0, 0, 0) != 0 or self.libc.prctl(37, ctypes.byref(actual), 0, 0, 0) != 0 or actual.value != 1:
            reject("window_tool_local_owner_unavailable")
        if self.descendants():
            reject("window_tool_local_owner_unavailable")

    def descendants(self):
        if os.getpid() != self.pid:
            reject("window_tool_local_owner_changed")
        pending, found = [self.pid], set()
        while pending:
            parent = pending.pop()
            try:
                tasks = list(Path('/proc', str(parent), 'task').iterdir())
            except FileNotFoundError:
                continue
            if len(tasks) > 1024:
                reject("window_tool_local_owner_bound")
            for task in tasks:
                try:
                    children = (task / 'children').read_text().split()
                except FileNotFoundError:
                    continue
                for value in children:
                    child = int(value)
                    if child not in found:
                        found.add(child); pending.append(child)
                        if len(found) > 128:
                            reject("window_tool_local_owner_bound")
        return found

    def signal_descendants(self, value):
        # pidfd protects the observed kernel child from PID reuse. Subreaper
        # adoption preserves this actual ancestry after the native parent dies,
        # including its separately grouped SSH children.
        for pid in self.descendants():
            try:
                fd = os.pidfd_open(pid, 0)
            except ProcessLookupError:
                continue
            try:
                if pid in self.descendants():
                    signal.pidfd_send_signal(fd, value)
            except ProcessLookupError:
                pass
            finally:
                os.close(fd)

    def reap_adopted(self):
        while True:
            try:
                pid, _ = os.waitpid(-1, os.WNOHANG)
            except ChildProcessError:
                return
            if pid == 0:
                return


def group_present(pgid):
    try:
        os.killpg(pgid, 0)
        return True
    except ProcessLookupError:
        return False
    except PermissionError:
        return True


def terminate_owned(child, owner):
    deadline = time.monotonic() + 15
    owner_observation_failed = False
    last_signal = None
    while time.monotonic() < deadline:
        value = signal.SIGKILL if time.monotonic() > deadline - 12 else signal.SIGTERM
        # Always signal this Popen-owned session even when a /proc or pidfd
        # observation fails. Such a failure cannot become terminal evidence.
        if value != last_signal:
            try:
                os.killpg(child.pid, value)
            except ProcessLookupError:
                pass
            except PermissionError:
                owner_observation_failed = True
            last_signal = value
        if owner is not None:
            try:
                owner.signal_descendants(value)
            except (OSError, ValueError, Refused):
                owner_observation_failed = True
        if child.poll() is not None:
            child.wait()
            if owner is not None:
                owner.reap_adopted()
            try:
                descendants = owner is not None and owner.descendants()
            except (OSError, ValueError, Refused):
                owner_observation_failed = True
                descendants = True
            if not group_present(child.pid) and not descendants:
                return not owner_observation_failed
        time.sleep(0.02)
    return False


def live_control(fd):
    info = os.fstat(fd)
    if stat.S_ISSOCK(info.st_mode):
        duplicate = socket.socket(fileno=os.dup(fd))
        try:
            if duplicate.family != socket.AF_UNIX or duplicate.getsockopt(socket.SOL_SOCKET, socket.SO_TYPE) != socket.SOCK_STREAM or duplicate.getpeername() not in ("", b""):
                reject("window_tool_control_pipe_rejected")
        finally:
            duplicate.close()
    elif not stat.S_ISFIFO(info.st_mode):
        reject("window_tool_control_pipe_rejected")
    poll = select.poll()
    poll.register(fd, select.POLLIN | select.POLLHUP | select.POLLERR)
    if poll.poll(0):
        reject("window_tool_control_pipe_rejected")


def owned_process(command, environment, *, packet=None, control=None, timeout=110 * 60, owner=None):
    # The root supervisor owns its native session and its adopted descendants.
    # The unprivileged manager instead keeps a real stdin control pipe open;
    # closing it requests root-owned termination, never assumes it can signal
    # privileged sudo descendants itself.
    if control is not None:
        live_control(control)
    cancelled = [False]
    previous = {}
    for number in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
        previous[number] = signal.getsignal(number)
        signal.signal(number, lambda _n, _f: cancelled.__setitem__(0, True))
    child = None
    output = bytearray()
    try:
        child = subprocess.Popen(command, env=environment, stdin=subprocess.PIPE if packet is not None else subprocess.DEVNULL,
            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, start_new_session=True)
        if os.getpgid(child.pid) != child.pid:
            reject("window_tool_local_owner_changed")
        if packet is not None:
            child.stdin.write(packet); child.stdin.flush()
        poll = select.poll()
        poll.register(child.stdout.fileno(), select.POLLIN | select.POLLHUP | select.POLLERR)
        if control is not None:
            poll.register(control, select.POLLIN | select.POLLHUP | select.POLLERR)
        deadline = time.monotonic() + timeout
        stdout_done = False
        while True:
            if cancelled[0] or time.monotonic() >= deadline:
                reject("window_tool_local_execution_unknown")
            for fd, event in poll.poll(100):
                if control is not None and fd == control:
                    # A second message is forbidden; EOF/connection loss also
                    # terminates local execution. Neither grants recovery.
                    if event:
                        os.read(fd, 1)
                        reject("window_tool_local_execution_unknown")
                elif fd == child.stdout.fileno():
                    raw = os.read(fd, 4096)
                    if not raw:
                        stdout_done = True; poll.unregister(fd)
                    output.extend(raw)
                    if len(output) > 32768:
                        reject("window_tool_native_output_rejected")
            if stdout_done and child.poll() is not None:
                code = child.wait()
                if owner is not None:
                    owner.reap_adopted()
                if group_present(child.pid) or owner is not None and owner.descendants():
                    reject("window_tool_local_execution_unknown")
                return code, bytes(output)
    except BaseException:
        if child is not None:
            if packet is not None:
                # Production nonroot cannot kill the root native chain. Its
                # supervisor observes this actual EOF and performs root reap.
                try:
                    child.stdin.close()
                except (OSError, BrokenPipeError):
                    pass
                try:
                    child.wait(timeout=25)
                except subprocess.TimeoutExpired:
                    pass
                # Even a known sudo exit is not evidence about daemon exec.
            else:
                terminate_owned(child, owner)
        raise
    finally:
        if child is not None:
            if child.stdin is not None and not child.stdin.closed:
                child.stdin.close()
            child.stdout.close()
        for number, handler in previous.items():
            signal.signal(number, handler)


def validate_image_preload(value, approval, run):
    exact(value, ("kind", "tool_source_sha", "original_source_sha", "operation_id", "actual_run_id", "image_archive_sha256", "image_id", "os", "architecture", "revision", "program_sha256", "probe_id", "probe_absent", "temporary_files_zero", "capabilities"))
    if value["kind"] != "native_cached_api_image_observation" or any(value[k] != approval[k] for k in ("tool_source_sha", "original_source_sha", "operation_id")) or value["actual_run_id"] != run or value["revision"] != approval["tool_source_sha"] or value["os"] != "linux" or value["architecture"] != "amd64" or value["probe_absent"] is not True or value["temporary_files_zero"] is not True or value["capabilities"] != {"deployment": False, "writer_fence": False, "drop": False}:
        reject("window_tool_image_preload_rejected")
    token(value["image_id"], re.compile(r"sha256:[0-9a-f]{64}"))
    for key in ("image_archive_sha256", "program_sha256", "probe_id"): token(value[key], HASH)
    return value


def preload_api_image(raw, approval, run, batch, owner):
    # Fixed cached-image preparation only. This original root child never starts
    # an API, changes the original container, or grants a Window/writer lease.
    if approval["stage"] != "prepare" or platform.machine() != "x86_64" or not raw or len(raw) > 200 << 20:
        reject("window_tool_image_preload_rejected")
    reference = "qs-retirement/qs-apiserver:" + approval["tool_source_sha"]
    with tarfile.open(fileobj=io.BytesIO(raw), mode="r:gz") as image_tar:
        members = image_tar.getmembers()
        if len(members) > 2048 or any(not m.isfile() and not m.isdir() or m.name.startswith("/") or ".." in Path(m.name).parts or m.issym() or m.islnk() for m in members): reject("window_tool_image_preload_rejected")
        manifest = [m for m in members if m.name == "manifest.json"]
        if len(manifest) != 1 or manifest[0].size > 65536: reject("window_tool_image_preload_rejected")
        values = decode(image_tar.extractfile(manifest[0]).read(65537))
        if type(values) is not list or len(values) != 1 or type(values[0]) is not dict or values[0].get("RepoTags") != [reference]: reject("window_tool_image_preload_rejected")
        config_name = values[0].get("Config", "")
        if type(config_name) is not str or not re.fullmatch(r"(?:blobs/sha256/)?[0-9a-f]{64}(?:\.json)?", config_name): reject("window_tool_image_preload_rejected")
        configs = [m for m in members if m.name == config_name]
        if len(configs) != 1 or configs[0].size > 1 << 20: reject("window_tool_image_preload_rejected")
        config_raw = image_tar.extractfile(configs[0]).read((1 << 20)+1)
        config = decode(config_raw)
        expected_id = "sha256:"+digest(config_raw)
        if type(config) is not dict or config.get("os") != "linux" or config.get("architecture") != "amd64" or config.get("config",{}).get("Labels",{}).get("org.opencontainers.image.revision") != approval["tool_source_sha"]: reject("window_tool_image_preload_rejected")
    docker = Path("/usr/bin/docker")
    def identity(value): return (value.st_dev,value.st_ino,value.st_uid,value.st_mode,value.st_nlink,value.st_size,value.st_mtime_ns,value.st_ctime_ns)
    stamp = docker.stat()
    docker_identity = identity(stamp)
    sock = Path("/run/docker.sock").stat()
    if not stat.S_ISREG(stamp.st_mode) or stamp.st_uid != 0 or stamp.st_mode & 0o022 or not stat.S_ISSOCK(sock.st_mode) or sock.st_uid != 0: reject("window_tool_image_preload_rejected")
    directory = batch / "image-preload"
    protected_directory(directory, create=True)
    image_path = directory / "image.tar.gz"
    name = "qs-retirement-preload-"+approval["operation_id"]+"-"+run
    labels = {"codex.task":"qs-compatibility-retirement", "codex.operation":approval["operation_id"], "codex.run":run, "codex.kind":"preload-program"}
    files = {}
    def put(path, body):
        write_new(path, body)
        files[path] = identity(path.stat(follow_symlinks=False))
    put(directory / "intent.json",canonical({"source_sha":approval["tool_source_sha"],"image_archive_sha256":digest(raw),"image_id":expected_id,"name":name,"labels":labels,"network":"none","start":False}))
    put(image_path,raw)
    def command(*args):
        if identity(docker.stat()) != docker_identity: reject("window_tool_image_preload_rejected")
        argv=[str(docker),"--host","unix:///run/docker.sock",*args]
        if owner is None:
            # Existing prepare-facts root-once caller has an EOF credential pipe,
            # not a live Window. Only bounded cached-image preparation uses it.
            value=subprocess.run(argv,env={"PATH":"/usr/bin:/bin"},stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,timeout=300,check=False)
            code, output=value.returncode,value.stdout
        else:
            code, output=owned_process(argv,{"PATH":"/usr/bin:/bin"},control=sys.stdin.fileno(),timeout=300,owner=owner)
        if code or len(output)>1<<20 or identity(docker.stat()) != docker_identity: reject("window_tool_image_preload_rejected")
        return output
    command("load","--input",str(image_path))
    fmt = '{"id":{{json .Id}},"os":{{json .Os}},"architecture":{{json .Architecture}},"revision":{{json (index .Config.Labels "org.opencontainers.image.revision")}}}'
    first = decode(command("image","inspect","--format",fmt,reference))
    if first != {"id":expected_id,"os":"linux","architecture":"amd64","revision":approval["tool_source_sha"]}: reject("window_tool_image_preload_rejected")
    create = ["create","--name",name,"--network","none","--read-only","--entrypoint","/app/qs-apiserver"]
    for key in sorted(labels): create.extend(["--label",key+"="+labels[key]])
    cid = command(*create,expected_id).decode("ascii").strip(); token(cid,HASH)
    put(directory / "created.json",canonical({"id":cid,"image":expected_id}))
    probe_fmt = '{"id":{{json .Id}},"name":{{json .Name}},"image":{{json .Image}},"running":{{json .State.Running}},"pid":{{json .State.Pid}},"network":{{json .HostConfig.NetworkMode}},"mounts":{{json .Mounts}}}'
    probe = decode(command("inspect","--format",probe_fmt,cid))
    if probe != {"id":cid,"name":"/"+name,"image":expected_id,"running":False,"pid":0,"network":"none","mounts":[]}: reject("window_tool_image_preload_rejected")
    program = directory / "program"
    command("cp",cid+":/app/qs-apiserver",str(program))
    fd=os.open(program,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK)
    try:
        before=os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or before.st_uid != 0 or before.st_nlink != 1 or before.st_size <= 0 or before.st_size > 128 << 20 or before.st_mode & 0o022: reject("window_tool_image_preload_rejected")
        h=hashlib.sha256()
        while True:
            part=os.read(fd,65536)
            if not part:break
            h.update(part)
        if identity(os.fstat(fd)) != identity(before) or identity(program.stat(follow_symlinks=False)) != identity(before): reject("window_tool_image_preload_rejected")
        program_hash=h.hexdigest()
        files[program] = identity(before)
    finally:os.close(fd)
    second = decode(command("image","inspect","--format",fmt,reference))
    if second != first or decode(command("inspect","--format",probe_fmt,cid)) != probe: reject("window_tool_image_preload_rejected")
    command("rm",cid)
    if command("ps","--all","--no-trunc","--filter","id="+cid,"--format","{{.ID}}").strip(): reject("window_tool_image_preload_rejected")
    # Delete only this original caller's exact registered files, never an image
    # or another batch. Unknown/error work stays registered for investigation.
    expected = {"intent.json", "image.tar.gz", "created.json", "program"}
    if set(os.listdir(directory)) != expected: reject("window_tool_image_preload_rejected")
    for path in (directory/"intent.json",image_path,directory/"created.json",program):
        value=path.stat(follow_symlinks=False)
        if not stat.S_ISREG(value.st_mode) or value.st_uid != 0 or value.st_nlink != 1 or identity(value) != files[path]: reject("window_tool_image_preload_rejected")
        path.unlink()
    directory.rmdir()
    value = {"kind":"native_cached_api_image_observation","tool_source_sha":approval["tool_source_sha"],"original_source_sha":approval["original_source_sha"],"operation_id":approval["operation_id"],"actual_run_id":run,"image_archive_sha256":digest(raw),"image_id":expected_id,"os":"linux","architecture":"amd64","revision":approval["tool_source_sha"],"program_sha256":program_hash,"probe_id":cid,"probe_absent":True,"temporary_files_zero":not directory.exists(),"capabilities":{"deployment":False,"writer_fence":False,"drop":False}}
    return validate_image_preload(value,approval,run)


def root_execute(arguments, packet, source_uid, archive_raw):
    if os.getuid() != 0 or os.geteuid() != 0 or platform.system() != "Linux":
        reject("window_tool_actual_root_required")
    owner = LinuxChildOwner()
    live_control(sys.stdin.fileno())
    stage, operation, current_run, dispatcher, approval_hash, package_hash, manifest, template_hash = arguments
    a = approve(packet["approval"], approval_hash, dispatcher, stage, operation, manifest, template_hash)
    arch = {"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(platform.machine())
    if arch is None:
        reject("window_tool_architecture_rejected")
    with tarfile.open(fileobj=io.BytesIO(archive_raw), mode="r:gz") as tar:
        members = tar.getmembers()
        names = [m.name for m in members]
        base_names = {"compatibility-window-tool.py", "receipt-transport.py", "inventory-linux-amd64", "inventory-linux-arm64"}
        image_names = {"preload-image.tar.gz"} if stage == "prepare" and "preload-image.tar.gz" in names else set()
        if set(names) != base_names | image_names or len(names) != len(base_names | image_names) or len(names) != len(set(names)) or any(not m.isfile() or "/" in m.name or m.size > (200 << 20 if m.name == "preload-image.tar.gz" else 64 << 20) for m in members):
            reject("window_tool_package_rejected")
        selected = [m for m in members if m.name == "inventory-linux-" + arch]
        if len(selected) != 1:
            reject("window_tool_package_rejected")
        binary = tar.extractfile(selected[0]).read((64 << 20) + 1)
        preload_raw = tar.extractfile("preload-image.tar.gz").read((200 << 20)+1) if image_names else None
    if not binary or len(binary) > 64 << 20 or digest(binary) != a["tool_binary_sha256"][arch]:
        reject("window_tool_actual_binary_hash_rejected")
    original = Path("/opt/backups/qs-server/compatibility-retirement") / operation
    raw = read_owned(original / "lifecycle-request-template.json", source_uid, template_hash, 256 << 10)
    derived = derive_request(raw, a, current_run)
    frozen_manifest = read_owned(original / "manifest.json", source_uid, manifest, 256 << 10)
    invocation_base = Path("/opt/backups/qs-server/compatibility-retirement-invocations")
    try:
        invocation_base.mkdir(mode=0o700)
    except FileExistsError:
        pass
    protected_directory(invocation_base)
    invocation = invocation_base / (operation + "-" + current_run)
    protected_directory(invocation, create=True) # never overwrite/adopt unknown prior run
    if stage == "prepare":
        # A's preparation binary never occupies B's fixed maintenance slot.
        # Later apply can independently install the approved B CLI for this op.
        base = Path("/opt/backups/qs-server/compatibility-retirement-root-prepare")
        try:
            base.mkdir(mode=0o700)
        except FileExistsError:
            pass
        protected_directory(base)
        batch = base / (operation + "-" + current_run)
        protected_directory(batch, create=True)
        native = batch / "restore-native"
        binary_mode = 0o700
        mode = "lifecycle-prepare-root-once"
    else:
        tool_parent = Path("/opt/qs-server/qs-apiserver/compatibility-retirement")
        try:
            tool_parent.mkdir(mode=0o700)
        except FileExistsError:
            pass
        protected_directory(tool_parent)
        tool_root = tool_parent / operation
        try:
            tool_root.mkdir(mode=0o700)
        except FileExistsError:
            pass
        protected_directory(tool_root)
        native = tool_root / "qs-compatibility-retirement"
        binary_mode = 0o500
        mode = "lifecycle-" + stage
    intent = {"format_version": 1, "kind": "independent_window_tool_native_invocation", "dispatcher_source_sha": dispatcher,
              "tool_source_sha": a["tool_source_sha"], "original_source_sha": a["original_source_sha"], "operation_id": operation,
              "original_run_id": a["original_run_id"], "actual_run_id": current_run, "stage": stage,
              "approved_template_sha256": template_hash, "derived_request_sha256": digest(derived), "manifest_sha256": manifest,
              "package_sha256": package_hash, "tool_directory": packet["tool_directory"], "tool_program_sha256": packet["tool_program_sha256"], "native_sha256": digest(binary), "native_path": str(native), "b_image_id": a["b_image_id"],
              "b_program_sha256": a["b_program_sha256"], "source_uid": source_uid, "drop_authority": False}
    write_new(invocation / "native-call.intent.private.json", canonical(intent))
    if stage != "prepare" and "writer_control" in decode(derived):
        scope_hash = decode(derived)["writer_control"]["workflow_scope_sha256"]
        scope_raw = read_owned(original / "approved-workflow-scope.json", source_uid, scope_hash, 64 << 10)
        if scope_raw != canonical(a["workflow_scope"]):
            reject("window_tool_workflow_scope_rejected")
        scope_target = tool_root / "approved-workflow-scope.json"
        if not scope_target.exists():
            write_new(scope_target, scope_raw)
        elif read_owned(scope_target, 0, scope_hash, 64 << 10) != scope_raw:
            reject("window_tool_existing_scope_conflict")
    if stage == "prepare":
        write_new(batch / "tool.intent.private.json", canonical(intent))
    if not native.exists():
        write_new(native, binary, binary_mode)
    else:
        if len(read_owned(native, 0, digest(binary), len(binary), mode=binary_mode)) != len(binary):
            reject("window_tool_existing_native_conflict")
    check_code, check_raw = owned_process([str(native), "--source-sha"], {"PATH": "/usr/bin:/bin"},
        control=sys.stdin.fileno(), timeout=5, owner=owner)
    if check_code or check_raw != a["tool_source_sha"].encode() + b"\n":
        reject("window_tool_actual_source_rejected")
    if preload_raw is not None: reject("window_tool_package_rejected")
    if stage == "prepare":
        # The earlier original prepare-facts producer supplies the first actual
        # image facts. This invocation only re-reads that exact cached identity.
        image_fmt='{"id":{{json .Id}},"os":{{json .Os}},"architecture":{{json .Architecture}},"revision":{{json (index .Config.Labels "org.opencontainers.image.revision")}}}'
        for _ in range(2):
            image_code,image_raw=owned_process(["/usr/bin/docker","--host","unix:///run/docker.sock","image","inspect","--format",image_fmt,a["b_image_id"]],{"PATH":"/usr/bin:/bin"},control=sys.stdin.fileno(),timeout=10,owner=owner)
            if image_code or decode(image_raw)!={"id":a["b_image_id"],"os":"linux","architecture":arch,"revision":a["tool_source_sha"]}: reject("window_tool_image_preload_rejected")
    budget_result_hash = ""
    if stage == "prepare" and "local_descriptor_sha256" in a:
        budget_result_hash = prepare_budget_key(original, batch, native, source_uid, a, current_run, owner)
    if stage == "apply" and "service_control" in decode(derived):
        bootstrap_services(original, tool_root, native, source_uid, a, decode(derived), packet["credentials"], owner)
    write_new(invocation / "lifecycle-request.json", derived)
    write_new(invocation / "manifest.json", frozen_manifest)
    environment = {name: packet["credentials"][name] for name in CREDENTIALS + (() if stage == "prepare" else (READ_TOKEN,))}
    environment["PATH"] = "/usr/bin:/bin"
    environment["QS_RETIREMENT_SOURCE_UID"] = str(source_uid)
    if budget_result_hash:
        environment["QS_LIFECYCLE_BUDGET_KEY_DESCRIPTOR_SHA256"] = a["local_descriptor_sha256"]
        environment["QS_LIFECYCLE_BUDGET_KEY_RESULT_SHA256"] = budget_result_hash
    # The real native process owns every later connection/Window/proof. This
    # manager imports no completion/permit or guessed production authority.
    code, raw = owned_process([str(native), "--mode", mode, "--request", str(invocation / "lifecycle-request.json"),
               "--request-hash", digest(derived), "--operation-id", operation, "--run-id", current_run], environment, control=sys.stdin.fileno(), owner=owner)
    sys.stdout.buffer.write(raw)
    sys.stdout.buffer.flush()
    return code


def bootstrap_services(original, root, native, source_uid, approval, request, credentials, owner):
    """Install the approved actual inventories and open the preparation seed.

    Descriptor hashes remain the independently frozen request inputs. This
    caller cannot create a host census, stopped-service Lease or writer proof.
    The D public key is observed with the existing fingerprint before copying
    the existing deploy credential into this batch's exclusive root namespace.
    """
    validate_credentials(credentials, "apply")
    control = request["service_control"]
    exact(control, ("local_descriptor_sha256", "ssh_channel_sha256"))
    descriptor_raw = read_owned(original / "approved-services.json", source_uid, control["local_descriptor_sha256"], 256 << 10)
    channel_raw = read_owned(original / "ssh-channel.json", source_uid, control["ssh_channel_sha256"], 64 << 10)
    descriptor, channel = decode(descriptor_raw), decode(channel_raw)
    if type(descriptor) is not dict:
        reject("window_tool_service_inventory_rejected")
    token(descriptor.get("runtime_source_sha"), SHA)
    for key, expected in (("source_sha", approval["original_source_sha"]), ("tool_source_sha", approval["tool_source_sha"]), ("operation_id", approval["operation_id"]), ("original_run_id", approval["original_run_id"]), ("manifest_sha256", approval["manifest_sha256"]), ("host_role", "server-a")):
        if descriptor.get(key) != expected:
            reject("window_tool_service_inventory_rejected")
    exact(channel, ("format_version", "kind", "tool_source_sha", "original_source_sha", "operation_id", "manifest_sha256", "original_run_id", "actual_run_id", "ssh_executable_sha256", "host", "port", "user", "identity_sha256", "known_hosts_sha256", "host_key_fingerprint", "remote_request_sha256", "remote_descriptor_sha256"))
    for key in ("tool_source_sha", "original_source_sha", "operation_id", "manifest_sha256", "original_run_id"):
        if channel[key] != approval[key]:
            reject("window_tool_service_inventory_rejected")
    if type(channel["format_version"]) is not int or channel["format_version"] != 1 or not ((channel["kind"] == "qs_existing_pinned_service_ssh_channel_template" and channel["actual_run_id"] == "") or (channel["kind"] == "qs_existing_pinned_service_ssh_channel" and channel["actual_run_id"] == request["actual_run_id"])):
        reject("window_tool_service_inventory_rejected")
    for key in ("ssh_executable_sha256", "identity_sha256", "known_hosts_sha256", "remote_request_sha256", "remote_descriptor_sha256"):
        token(channel[key], HASH)
    if descriptor.get("remote_descriptor_sha256") != channel["remote_descriptor_sha256"]:
        reject("window_tool_service_inventory_rejected")
    host, user, port, key, pin = (credentials[name] for name in SERVICE_CREDENTIALS)
    token(host, re.compile(r"[A-Za-z0-9][A-Za-z0-9.-]{0,252}"))
    token(user, re.compile(r"[a-z_][a-z0-9_-]{0,31}"))
    token(pin, re.compile(r"SHA256:[A-Za-z0-9+/]{43}"))
    if ".." in host or not port.isdigit() or not 1 <= int(port) <= 65535 or type(channel["port"]) is not int or (host, user, int(port), pin) != (channel["host"], channel["user"], channel["port"], channel["host_key_fingerprint"]):
        reject("window_tool_service_route_rejected")
    key_raw = key.rstrip("\n").encode("ascii") + b"\n"
    if not key_raw.startswith(b"-----BEGIN ") or not key_raw.rstrip().endswith(b"-----") or digest(key_raw) != channel["identity_sha256"]:
        reject("window_tool_service_identity_rejected")
    code, observed = owned_process(["/usr/bin/ssh-keyscan", "-T", "5", "-p", port, "-t", "ed25519,rsa,ecdsa", host], {"PATH": "/usr/bin:/bin"}, control=sys.stdin.fileno(), timeout=20, owner=owner)
    known = pinned_service_known_host(observed, host, int(port), pin)
    if code or digest(known) != channel["known_hosts_sha256"]:
        reject("window_tool_service_peer_rejected")
    # An earlier/unknown bootstrap is not adopted, even if its bodies match.
    if set(os.listdir(root)) != {"qs-compatibility-retirement", "approved-workflow-scope.json", "budget-issuer"} or set(os.listdir(root / "budget-issuer")) != {"issuer-seed"}:
        reject("window_tool_service_namespace_conflict")
    session = {"format_version": 1, "kind": "qs_root_service_session", "tool_source_sha": approval["tool_source_sha"], "tool_binary_sha256": approval["tool_binary_sha256"][{"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}[platform.machine()]], "original_source_sha": approval["original_source_sha"], "operation_id": approval["operation_id"], "manifest_sha256": approval["manifest_sha256"], "original_run_id": approval["original_run_id"], "actual_run_id": request["actual_run_id"], "descriptor_sha256": control["local_descriptor_sha256"]}
    session_raw = canonical(session)
    protected_directory(root / "budget-issuer")
    for name in ("service-journal", "ssh"):
        protected_directory(root / name, create=True)
        sync_directory(root)
    for path, raw in ((root / "approved-services.json", descriptor_raw), (root / "ssh-channel.json", channel_raw), (root / "service-session.json", session_raw), (root / "ssh" / "identity", key_raw), (root / "ssh" / "known_hosts", known)):
        write_new(path, raw)
    # D's already frozen trust names the preparation public key. Never create
    # a replacement seed after its descriptor/channel were approved.
    code, raw = owned_process([str(native), "--mode", "host-budget-key-open", "--request", str(root / "service-session.json"), "--request-hash", digest(session_raw), "--operation-id", approval["operation_id"], "--run-id", request["actual_run_id"]], {"PATH": "/usr/bin:/bin"}, control=sys.stdin.fileno(), timeout=35, owner=owner)
    receipt = decode(raw)
    exact(receipt, ("kind", "tool_source_sha", "operation_id", "actual_run_id", "public_key", "key_available", "whole_writer_fence_proven", "drop_ready", "error_category"))
    if code or any(type(receipt[k]) is not bool for k in ("key_available", "whole_writer_fence_proven", "drop_ready")) or receipt != {"kind": "qs_native_temporary_budget_key_result", "tool_source_sha": approval["tool_source_sha"], "operation_id": approval["operation_id"], "actual_run_id": request["actual_run_id"], "public_key": receipt["public_key"], "key_available": True, "whole_writer_fence_proven": False, "drop_ready": False, "error_category": "none"}:
        reject("window_tool_actual_budget_key_rejected")
    token(receipt["public_key"], HASH)
    # The native constructor is the only writer of issuer-seed. The lifecycle
    # subsequently opens/holds these actual files through its original owner.
    seed = root / "budget-issuer" / "issuer-seed"
    info = seed.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_nlink != 1 or stat.S_IMODE(info.st_mode) != 0o600 or info.st_size != 32:
        reject("window_tool_actual_budget_key_rejected")


def prepare_budget_key(original, batch, native, source_uid, approval, run, owner):
    """One native key creation before D trust and final descriptors freeze."""
    descriptor_raw = read_owned(original / "budget-key.descriptor.private.json", source_uid, approval["local_descriptor_sha256"], 256 << 10)
    descriptor = decode(descriptor_raw)
    if type(descriptor) is not dict:
        reject("window_tool_service_inventory_rejected")
    token(descriptor.get("runtime_source_sha"), SHA)
    for key, expected in (("source_sha", approval["original_source_sha"]), ("tool_source_sha", approval["tool_source_sha"]), ("operation_id", approval["operation_id"]), ("original_run_id", approval["original_run_id"]), ("manifest_sha256", approval["manifest_sha256"]), ("host_role", "server-a")):
        if type(descriptor) is not dict or descriptor.get(key) != expected:
            reject("window_tool_service_inventory_rejected")
    if descriptor.get("remote_descriptor_sha256", "") != "" or descriptor.get("budget_trust_sha256", "") != "":
        reject("window_tool_service_inventory_rejected")
    parent = Path("/opt/qs-server/qs-apiserver/compatibility-retirement")
    for path in (parent, parent / approval["operation_id"]):
        try:
            path.mkdir(mode=0o700)
        except FileExistsError:
            pass
        protected_directory(path)
    root = parent / approval["operation_id"]
    if os.listdir(root):
        reject("window_tool_service_namespace_conflict")
    directory = root / "budget-issuer"
    protected_directory(directory, create=True)
    sync_directory(root)
    session = {"format_version": 1, "kind": "qs_root_service_session", "tool_source_sha": approval["tool_source_sha"], "tool_binary_sha256": digest(native.read_bytes()), "original_source_sha": approval["original_source_sha"], "operation_id": approval["operation_id"], "manifest_sha256": approval["manifest_sha256"], "original_run_id": approval["original_run_id"], "actual_run_id": run, "descriptor_sha256": approval["local_descriptor_sha256"]}
    session_raw = canonical(session)
    write_new(batch / "budget-key.basis.private.json", descriptor_raw)
    # Both files are actual root-created temporary inputs. Keep their original
    # FDs until native terminal; unknown native work retains them for recovery.
    files = []
    try:
        for name, raw in (("approved-services.json", descriptor_raw), ("service-session.json", session_raw)):
            path = directory / name
            write_new(path, raw)
            fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
            files.append((path, fd, os.fstat(fd), digest(raw)))
        code, raw = owned_process([str(native), "--mode", "host-budget-key-create", "--request", str(directory / "service-session.json"), "--request-hash", digest(session_raw), "--operation-id", approval["operation_id"], "--run-id", run], {"PATH": "/usr/bin:/bin"}, control=sys.stdin.fileno(), timeout=35, owner=owner)
        receipt = decode(raw)
        exact(receipt, ("kind", "tool_source_sha", "operation_id", "actual_run_id", "public_key", "key_available", "whole_writer_fence_proven", "drop_ready", "error_category"))
        if code or any(type(receipt[k]) is not bool for k in ("key_available", "whole_writer_fence_proven", "drop_ready")) or receipt != {"kind": "qs_native_temporary_budget_key_result", "tool_source_sha": approval["tool_source_sha"], "operation_id": approval["operation_id"], "actual_run_id": run, "public_key": receipt["public_key"], "key_available": True, "whole_writer_fence_proven": False, "drop_ready": False, "error_category": "none"}:
            reject("window_tool_actual_budget_key_rejected")
        token(receipt["public_key"], HASH)
        result_raw = canonical(receipt)
        write_new(batch / "budget-key.result.private.json", result_raw)
        identity = lambda v: (v.st_dev, v.st_ino, v.st_uid, v.st_mode, v.st_nlink, v.st_size, v.st_mtime_ns, v.st_ctime_ns)
        for path, fd, before, expected in files:
            if identity(before) != identity(os.fstat(fd)) or identity(before) != identity(path.lstat()) or read_owned(path, 0, expected, 256 << 10) == b"":
                reject("window_tool_service_namespace_conflict")
        if set(os.listdir(directory)) != {"approved-services.json", "service-session.json", "issuer-seed"}:
            reject("window_tool_service_namespace_conflict")
        for path, fd, before, expected in files:
            if identity(before) != identity(path.lstat()):
                reject("window_tool_service_namespace_conflict")
            path.unlink()
        sync_directory(directory)
        return digest(result_raw)
    finally:
        for unused, fd, unused, unused in files:
            os.close(fd)


def pinned_service_known_host(raw, host, port, fingerprint):
    matches = set()
    target = host if port == 22 else "[" + host + "]:" + str(port)
    for line in raw.decode("ascii").splitlines():
        if line.startswith("#") or not line.strip():
            continue
        fields = line.split()
        if len(fields) != 3 or fields[0] != target or fields[1] not in ("ssh-ed25519", "ssh-rsa", "ecdsa-sha2-nistp256"):
            reject("window_tool_service_peer_rejected")
        try:
            blob = base64.b64decode(fields[2], validate=True)
        except ValueError:
            reject("window_tool_service_peer_rejected")
        if not 0 < len(blob) <= 16384:
            reject("window_tool_service_peer_rejected")
        actual = "SHA256:" + base64.b64encode(hashlib.sha256(blob).digest()).decode("ascii").rstrip("=")
        if actual == fingerprint:
            matches.add((" ".join(fields) + "\n").encode("ascii"))
    if len(matches) != 1:
        reject("window_tool_service_peer_rejected")
    return matches.pop()


ROOT_BOOTSTRAP = r'''
import hashlib,io,json,os,re,stat,sys,tarfile
from pathlib import Path
try:
 if os.getuid()!=0 or os.geteuid()!=0 or len(sys.argv)!=10: raise ValueError()
 arguments=sys.argv[1:9]; channel=sys.argv[9]
 stage,operation,run,dispatcher,approval_hash,package_hash,manifest,template_hash=arguments
 if channel=='sudo-user':
  source_uid=int(os.environ['SUDO_UID'])
  if source_uid<1: raise ValueError()
 elif channel=='root-direct':
  if 'SUDO_UID' in os.environ: raise ValueError()
  source_uid=os.getuid()
 else: raise ValueError()
 if not re.fullmatch(r'[1-9][0-9]{0,19}-[1-9][0-9]{0,3}',run) or not re.fullmatch(r'[0-9a-f]{64}',package_hash): raise ValueError()
 raw=sys.stdin.buffer.readline(32769)
 if len(raw)>32768: raise ValueError()
 def unique(items):
  value={}
  for key,item in items:
   if key in value: raise ValueError()
   value[key]=item
  return value
 packet=json.loads(raw,object_pairs_hook=unique)
 if set(packet)!= {'credentials','approval','tool_directory','tool_program_sha256'} or type(packet['credentials']) is not dict or type(packet['approval']) is not str: raise ValueError()
 tool_directory=Path(packet['tool_directory'])
 if not re.fullmatch(r'/tmp/qs-independent-window-tool\.[a-zA-Z0-9]{6,16}',str(tool_directory)) or not re.fullmatch(r'[0-9a-f]{64}',packet['tool_program_sha256']): raise ValueError()
 path=Path('/tmp/qs-compatibility-retirement-'+run+'.tar.gz')
 fd=os.open(path,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK)
 with os.fdopen(fd,'rb') as f:
  st=os.fstat(f.fileno())
  if not stat.S_ISREG(st.st_mode) or st.st_nlink!=1 or st.st_uid!=source_uid or stat.S_IMODE(st.st_mode)!=0o600 or not 0<st.st_size<=256<<20: raise ValueError()
  archive_raw=f.read((256<<20)+1); after=os.fstat(f.fileno())
 identity=lambda v:(v.st_dev,v.st_ino,v.st_uid,v.st_mode,v.st_nlink,v.st_size,v.st_mtime_ns,v.st_ctime_ns)
 if identity(st)!=identity(after) or identity(st)!=identity(path.lstat()): raise ValueError()
 if len(archive_raw)>256<<20 or hashlib.sha256(archive_raw).hexdigest()!=package_hash: raise ValueError()
 with tarfile.open(fileobj=io.BytesIO(archive_raw),mode='r:gz') as tar:
  members=tar.getmembers(); names=[m.name for m in members]
  base={'compatibility-window-tool.py','receipt-transport.py','inventory-linux-amd64','inventory-linux-arm64'}
  extra={'preload-image.tar.gz'} if stage=='prepare' and 'preload-image.tar.gz' in names else set()
  if set(names)!=base|extra or len(names)!=len(base|extra) or len(names)!=len(set(names)) or any(not m.isfile() or '/' in m.name or m.size>(200<<20 if m.name=='preload-image.tar.gz' else 64<<20) for m in members): raise ValueError()
  candidates=[m for m in members if m.name=='compatibility-window-tool.py']
  if len(candidates)!=1 or candidates[0].size>1<<20: raise ValueError()
  program=tar.extractfile(candidates[0]).read((1<<20)+1)
 if hashlib.sha256(program).hexdigest()!=packet['tool_program_sha256']: raise ValueError()
 namespace={'__name__':'approved_window_tool_root_module'}
 exec(compile(program,'approved-hash-bound-window-tool','exec'),namespace)
 names=namespace['credential_names'](stage)
 namespace['validate_credentials'](packet['credentials'],stage)
 namespace['read_owned'](tool_directory/'compatibility-window-tool.py',source_uid,packet['tool_program_sha256'],1<<20)
 raise SystemExit(namespace['root_execute'](arguments,packet,source_uid,archive_raw))
except Exception:
 print('{"format_version":1,"complete":false,"execution_allowed":false,"drop_ready":false,"error_category":"window_tool_root_native_call_rejected"}')
 raise SystemExit(1)
'''



ADAPTERS = frozenset({"actual_four_source_historical_persistence_and_readback", "actual_production_bound_isolated_restore",
    "server_a_and_server_d_stop_drain_lease", "whole_writer_and_old_ref_fence", "prepared_inline_b_and_no_automigration_rollback",
    "actual_runtime_acceptance_and_private_purge"})
PUBLIC_ERRORS = frozenset({"none", "lifecycle_actual_host_adapters_missing", "lifecycle_whole_writer_and_old_ref_fence_missing",
    "lifecycle_parent_window_budget_rejected",
    "lifecycle_runtime_ledger_scan_rejected", "lifecycle_standard_audit_findings_present",
    "lifecycle_controlled_internal_resume_broker_and_complete_material_producers_missing",
    "lifecycle_final_historical_input_rejected", "lifecycle_final_historical_scope_rejected", "lifecycle_final_historical_scope_cleanup_failed",
    "lifecycle_final_historical_q_and_eof_missing", "lifecycle_actual_inline_b_deployment_missing",
    "lifecycle_actual_runtime_and_data_acceptance_missing", "lifecycle_actual_ddl_stopped_unproven",
    "lifecycle_writer_scope_binding_rejected", "lifecycle_platform_observation_unproven", "lifecycle_host_database_external_writer_isolation_unproven",
    "lifecycle_actual_no_migration_rollback_missing", "lifecycle_actual_batch_material_purge_missing",
    "lifecycle_actual_batch_material_zero_check_missing", "lifecycle_native_operation_failed",
    "window_tool_root_native_call_rejected", "window_tool_call_rejected"})
NATIVE_REQUIRED = ("format_version", "kind", "operation", "source_sha", "original_source_sha", "operation_id", "run_id",
    "manifest_sha256", "request_sha256", "archive_sha256", "target_hash", "target_count", "complete", "execution_allowed", "drop_ready",
    "archive_binding_complete", "recovery_attempted", "recovery_complete", "acceptance_complete", "purge_complete", "error_category",
    "required_adapters", "isolated_content_restore_complete", "restore_elapsed_millis")
BOOLS = ("complete", "execution_allowed", "drop_ready", "archive_binding_complete", "recovery_attempted", "recovery_complete",
    "acceptance_complete", "purge_complete", "isolated_content_restore_complete")


def validate_native(raw, exit_code, approval, current_run, derived_hash):
    n = decode(raw)
    if type(n) is not dict:
        reject("window_tool_native_output_rejected")
    if n.get("kind") != "compatibility_retirement_lifecycle_result":
        exact(n, ("format_version", "complete", "execution_allowed", "drop_ready", "error_category"))
        if n != {"format_version":1,"complete":False,"execution_allowed":False,"drop_ready":False,"error_category":"window_tool_root_native_call_rejected"} or exit_code == 0:
            reject("window_tool_native_output_rejected")
        return n
    exact(n, NATIVE_REQUIRED, ("recovery_error_category", "mysql_recovery_non_target_sha256", "source_copy_intent_sha256", "preparation_restore_zero_sha256"))
    if type(n["format_version"]) is not int or n["format_version"] != 1 or n["source_sha"] != approval["tool_source_sha"] or n["operation_id"] != approval["operation_id"] or n["run_id"] != current_run or n["operation"] != approval["stage"] or n["request_sha256"] != derived_hash or n["target_hash"] != TARGET or type(n["target_count"]) is not int or n["target_count"] != 4:
        reject("window_tool_native_binding_rejected")
    if any(type(n[k]) is not bool for k in BOOLS) or n["execution_allowed"] is not False or n["drop_ready"] is not False or type(n["required_adapters"]) is not list or len(n["required_adapters"]) > len(ADAPTERS) or set(n["required_adapters"]) - ADAPTERS:
        reject("window_tool_native_output_rejected")
    # The compile-time missing-adapter gate is intentionally before input/DB.
    # It can therefore have no original archive fields. It never completes.
    preflight = n["error_category"] == "lifecycle_actual_host_adapters_missing" and approval["stage"] != "prepare"
    if preflight:
        if n["original_source_sha"] != "" or n["manifest_sha256"] != "" or n["complete"] or n["archive_binding_complete"] or exit_code == 0:
            reject("window_tool_native_binding_rejected")
    elif n["original_source_sha"] != approval["original_source_sha"] or n["manifest_sha256"] != approval["manifest_sha256"]:
        reject("window_tool_native_binding_rejected")
    token(n["archive_sha256"], re.compile(r"(?:[0-9a-f]{64})?"))
    if "mysql_recovery_non_target_sha256" in n:
        token(n["mysql_recovery_non_target_sha256"], HASH)
    if "source_copy_intent_sha256" in n:
        token(n["source_copy_intent_sha256"], HASH)
        if approval["stage"] != "prepare":
            reject("window_tool_native_binding_rejected")
    if "preparation_restore_zero_sha256" in n:
        token(n["preparation_restore_zero_sha256"], HASH)
        if approval["stage"] != "prepare":
            reject("window_tool_native_binding_rejected")
    if type(n["restore_elapsed_millis"]) is not int or not 0 <= n["restore_elapsed_millis"] <= 600000 or n["complete"] != (exit_code == 0 and n["error_category"] == "none") or approval["stage"] == "prepare" and n["complete"] and not n["isolated_content_restore_complete"]:
        reject("window_tool_native_output_rejected")
    for key in ("error_category", "recovery_error_category"):
        if key in n:
            if type(n[key]) is not str or re.fullmatch(r"[a-z_]{1,128}", n[key]) is None:
                reject("window_tool_native_output_rejected")
            if n[key] not in PUBLIC_ERRORS:
                n[key] = "lifecycle_native_operation_failed"
    # No unrecognized field/body is carried into the public receipt.
    return n


def emit(result, secrets):
    spec = importlib.util.spec_from_file_location("window_receipt_transport", Path(__file__).with_name("receipt-transport.py"))
    transport = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(transport)
    native_schema = {"format_version":"uint", "kind":frozenset({"compatibility_retirement_lifecycle_result"}), "operation":frozenset(STAGES),
        "source_sha":"sha40", "original_source_sha":"hash64_or_empty", "operation_id":"run_id", "run_id":"run_id", "manifest_sha256":"hash64_or_empty",
        "request_sha256":"hash64", "archive_sha256":"hash64_or_empty", "target_hash":"hash64", "target_count":"uint",
        **{k:"bool" for k in BOOLS}, "restore_elapsed_millis":"uint", "mysql_recovery_non_target_sha256":"hash64", "source_copy_intent_sha256":"hash64", "preparation_restore_zero_sha256":"hash64",
        "required_adapters":[ADAPTERS], "error_category":PUBLIC_ERRORS, "recovery_error_category":PUBLIC_ERRORS}
    # Preserve the existing transport: original source uses a 40-character SHA,
    # with empty permitted only for the actual compile-time preflight rejection.
    if result.get("native_result", {}).get("original_source_sha"):
        native_schema["original_source_sha"] = "sha40"
    schema = {"format_version":"uint", "kind":frozenset({"independent_window_tool_call_result"}), "dispatcher_source_sha":"sha40", "tool_source_sha":"sha40",
        "approved_template_sha256":"hash64", "derived_request_sha256":"hash64", "native_result":native_schema,
        "complete":"bool", "execution_allowed":"bool", "drop_ready":"bool", "error_category":PUBLIC_ERRORS}
    print(transport.encode_armored_receipt(result, schema=schema, secrets=secrets))


def run_window_call(args, raw, approval_hash, package, credentials, *, control=None):
    """Actual caller, with credentials borrowed from the private live pipe.

    The runner caller passes only expected bindings and credentials here. This
    function still loads the protected original template and invokes the fixed
    root supervisor/native binary. It accepts no execution result or callback.
    """
    secrets = tuple(credentials.values()) if type(credentials) is dict else ()
    try:
        token(args.run_id, RUN)
        a = approve(raw, approval_hash, args.dispatcher_sha, args.operation, args.operation_id, args.manifest_hash, args.template_hash)
        validate_credentials(credentials, args.operation)
        token(package, HASH)
        directory = Path(__file__).resolve(strict=True).parent
        if not re.fullmatch(r"/tmp/qs-independent-window-tool\.[a-zA-Z0-9]{6,16}", str(directory)):
            reject("window_tool_parent_path_rejected")
        program_hash = digest(Path(__file__).read_bytes())
        packet = canonical({"approval": raw, "credentials": credentials, "tool_directory": str(directory), "tool_program_sha256": program_hash})
        template_path = Path("/opt/backups/qs-server/compatibility-retirement") / args.operation_id / "lifecycle-request-template.json"
        approved_template = read_owned(template_path, os.getuid(), args.template_hash, 256 << 10)
        derived_hash = digest(derive_request(approved_template, a, args.run_id))
        if len(packet) > 32768 or os.getuid() != os.geteuid():
            reject()
        bindings = [args.operation, args.operation_id, args.run_id, args.dispatcher_sha, approval_hash, package, args.manifest_hash, args.template_hash]
        if os.getuid() == 0:
            if "SUDO_UID" in os.environ:
                reject("window_tool_root_direct_rejected")
            command = ["/usr/bin/python3", "-I", "-c", ROOT_BOOTSTRAP, *bindings, "root-direct"]
            environment = {"PATH": "/usr/bin:/bin"}
        else:
            command = ["/usr/bin/sudo", "-n", "--", "/usr/bin/python3", "-I", "-c", ROOT_BOOTSTRAP, *bindings, "sudo-user"]
            environment = None
        # The live pipe requests root-owned cancellation if this manager loses
        # its caller. No saved JSON is turned into a live proof.
        code, native_raw = owned_process(command, environment, packet=packet, control=control, timeout=115 * 60)
        native = validate_native(native_raw, code, a, args.run_id, derived_hash)
        result = {"format_version": 1, "kind": "independent_window_tool_call_result", "dispatcher_source_sha": args.dispatcher_sha,
                  "tool_source_sha": a["tool_source_sha"], "approved_template_sha256": args.template_hash, "derived_request_sha256": derived_hash, "native_result": native}
        emit(result, secrets)
        return code
    except (Refused, OSError, ValueError, subprocess.SubprocessError):
        emit({'format_version':1,'complete':False,'execution_allowed':False,'drop_ready':False,'error_category':'window_tool_call_rejected'}, secrets)
        return 1


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for field in ("operation", "operation-id", "run-id", "dispatcher-sha", "manifest-hash", "template-hash"):
        p.add_argument("--" + field, required=True)
    args = p.parse_args()
    try:
        return run_window_call(args, os.environ.get("RETIREMENT_BOOTSTRAP_APPROVAL_JSON", ""),
            os.environ.get("RETIREMENT_BOOTSTRAP_APPROVAL_SHA256", ""), os.environ.get("RETIREMENT_PACKAGE_SHA256", ""),
            {key: os.environ.get(key, "") for key in credential_names(args.operation)})
    except Refused:
        emit({'format_version':1,'complete':False,'execution_allowed':False,'drop_ready':False,'error_category':'window_tool_call_rejected'},
            tuple(os.environ.get(key, "") for key in CREDENTIALS + (READ_TOKEN,)))
        return 1


if __name__ == "__main__":
    sys.exit(main())
