#!/usr/bin/env python3
"""Private host orchestration for the fixed cbpt cleanup tool."""
import argparse
import contextlib
import fcntl
import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import time
import uuid

ARCHIVE_ROOT = Path("/opt/backups/qs-server/mysql-cbpt-archive")
OWNER_LABEL = "com.fangcunmount.qs-server.cbpt-cleanup-owner"
SHA = re.compile(r"^[0-9a-f]{40}$")
HASH = re.compile(r"^[0-9a-f]{64}$")
RUN_ID = re.compile(r"^[0-9]{1,20}-[0-9]{1,4}$")
SERVER_UUID = re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
GIB = 1024 ** 3
MIN_FREE = GIB
ARCHIVE_LIMIT = 20 * GIB
RESTORE_LIMIT = 64 * GIB
CAPTURE_LIMIT = 256 * 1024
SAFE_KEYS = {
    "format_version", "operation", "stage", "status", "error_category", "error_code",
    "archive_eligible", "drop_eligible", "source_target_hash", "source_sha",
    "operation_id", "target_table_count", "non_target_count", "manifest_sha256",
    "dump_sha256", "proof_sha256", "drop_block_reason", "ledger_complete", "remaining_target_count",
}
HASH_KEYS = {"source_target_hash", "manifest_sha256", "dump_sha256", "proof_sha256"}
COUNT_KEYS = {"dropped_count", "pending_count", "unknown_count", "failed_count"}
SAFE_KEYS |= COUNT_KEYS
ARCHIVE_ONLY_REASONS = {
    "metadata_visibility_blocked", "dependency_read_failed", "metadata_limit",
    "metadata_definition_hidden", "foreign_key_dependency", "trigger_dependency",
    "definition_dependency", "dynamic_definition_unknown",
}
TARGETS = tuple(sorted("cbpt_" + table + "_" + suffix for table in (
    "assessment", "assessment_score", "domain_event_outbox", "evaluation_outcome",
    "interpretation_admission_failure", "interpretation_attention_projection",
    "retry_event_hold", "runtime_checkpoint", "statistics_assessment_daily",
    "statistics_assessment_fact", "statistics_org_snapshot",
) for suffix in ("20260827114947", "20260827131756")))


class CleanupError(Exception):
    pass


def fail(category):
    raise CleanupError(category)


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            fail("duplicate_json_key")
        result[key] = value
    return result


def load_json_bytes(raw):
    try:
        return json.loads(raw, object_pairs_hook=unique_object)
    except (ValueError, TypeError, UnicodeError):
        fail("invalid_private_json")


def private_file(path, *, limit=4 * 1024 * 1024):
    try:
        descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
        with os.fdopen(descriptor, "rb") as stream:
            info = os.fstat(stream.fileno())
            if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o600 or info.st_size > limit:
                fail("private_file_invalid")
            raw = stream.read(limit + 1)
            if len(raw) > limit:
                fail("private_file_limit")
            return raw
    except CleanupError:
        raise
    except OSError:
        fail("private_file_unavailable")


def write_private(path, payload):
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(descriptor, "wb") as stream:
        stream.write(payload)
        stream.flush()
        os.fsync(stream.fileno())


def json_private(path, value):
    write_private(path, json.dumps(value, sort_keys=True, ensure_ascii=True).encode() + b"\n")


def mysql_option(value):
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"').replace("\t", "\\t").replace("\b", "\\b") + '"'


def connection_payload(values):
    # database is positional for mysqldump and in the private Go JSON only.
    return ("[client]\n" + "".join(key + "=" + mysql_option(str(values[key])) + "\n"
                                    for key in ("host", "port", "user", "password"))).encode()


def validate_source(env):
    keys = ("MYSQL_HOST", "MYSQL_USERNAME", "MYSQL_PASSWORD", "MYSQL_DATABASE")
    if any(not env.get(key) for key in keys):
        fail("database_environment_missing")
    if any(any(char in env[key] for char in ("\x00", "\r", "\n")) for key in keys):
        fail("database_environment_control_character")
    if not re.fullmatch(r"[A-Za-z0-9_.:-]+", env["MYSQL_HOST"]):
        fail("database_host_invalid")
    database = env["MYSQL_DATABASE"]
    if not re.fullmatch(r"[A-Za-z0-9_]{1,64}", database):
        fail("database_name_invalid")
    if database.lower() in {"mysql", "sys", "information_schema", "performance_schema"}:
        fail("system_database_not_allowed")
    port = env.get("MYSQL_PORT", "") or "3306"
    if not re.fullmatch(r"[0-9]{1,5}", port) or not 1 <= int(port) <= 65535:
        fail("database_port_invalid")
    return {"host": env["MYSQL_HOST"], "port": int(port), "user": env["MYSQL_USERNAME"],
            "password": env["MYSQL_PASSWORD"], "database": database}


@contextlib.contextmanager
def connections(values, *, marker=None):
    with tempfile.TemporaryDirectory(prefix="qs-cbpt-connection-") as temporary:
        root = Path(temporary)
        os.chmod(root, 0o700)
        json_private(root / "mysql.json", values)
        write_private(root / "mysql.cnf", connection_payload(values))
        if marker is not None:
            json_private(root / "restore-target.json", marker)
            write_private(root / "root-password", values["password"].encode())
        yield root


def directory_check(root):
    # Never repair permissions/ownership on an existing backup directory.
    try:
        for component in (root, *root.parents):
            info = component.lstat()
            if not stat.S_ISDIR(info.st_mode) or stat.S_ISLNK(info.st_mode):
                fail("archive_path_invalid")
            if info.st_mode & 0o022:
                # A root-owned sticky /tmp ancestor is safe for a private child.
                if not (info.st_uid == 0 and info.st_mode & stat.S_ISVTX):
                    fail("archive_path_writable")
        info = root.lstat()
        if info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o700:
            fail("archive_root_permissions")
    except OSError:
        fail("archive_root_unavailable")


@contextlib.contextmanager
def archive_lock(root):
    directory_check(root)
    descriptor = os.open(root / ".cbpt-cleanup.lock", os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    try:
        info = os.fstat(descriptor)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o600:
            fail("archive_lock_invalid")
        try:
            fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            fail("cleanup_already_running")
        yield
    finally:
        os.close(descriptor)


def stop_process(process):
    # Reap an exited leader first; still terminate its process group so that
    # descendants cannot inherit and keep the capture pipes alive.
    process.poll()
    denied = False
    for sig in (signal.SIGTERM, signal.SIGKILL):
        try:
            os.killpg(process.pid, sig)
        except ProcessLookupError:
            pass
        except PermissionError:
            denied = True
        if sig == signal.SIGTERM:
            try:
                process.wait(timeout=0.5)
            except subprocess.TimeoutExpired:
                pass
    try:
        process.wait(timeout=2)
    except subprocess.TimeoutExpired:
        fail("client_termination_unconfirmed")
    if denied:
        fail("client_termination_unconfirmed")


def archive_guard(path):
    try:
        size = 0
        for child in path.iterdir():
            info = child.lstat()
            if not stat.S_ISREG(info.st_mode) or stat.S_ISLNK(info.st_mode):
                fail("archive_object_invalid")
            size += info.st_size
        if size > ARCHIVE_LIMIT:
            fail("archive_size_limit")
        if shutil.disk_usage(path).free < MIN_FREE:
            fail("archive_disk_low")
    except OSError:
        fail("archive_disk_unknown")


def capture(args, *, timeout=30, guard=None):
    # Child stderr is drained but never retained or printed.
    try:
        process = subprocess.Popen(args, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE, start_new_session=True, bufsize=0)
    except OSError:
        fail("client_start_failed")
    selector = selectors.DefaultSelector()
    output = bytearray()
    total = 0
    deadline = time.monotonic() + timeout
    try:
        for stream in (process.stdout, process.stderr):
            os.set_blocking(stream.fileno(), False)
            selector.register(stream, selectors.EVENT_READ)
        while selector.get_map():
            if time.monotonic() >= deadline:
                fail("client_timeout")
            if guard is not None:
                guard()
            for key, _ in selector.select(0.2):
                block = os.read(key.fileobj.fileno(), 65536)
                if not block:
                    selector.unregister(key.fileobj)
                else:
                    total += len(block)
                    if total > CAPTURE_LIMIT:
                        fail("client_output_limit")
                    if key.fileobj is process.stdout:
                        output.extend(block)
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            fail("client_timeout")
        status = process.wait(timeout=remaining)
        return status, output.decode("utf-8", errors="strict")
    except (subprocess.TimeoutExpired, UnicodeError):
        fail("client_timeout_or_output_invalid")
    finally:
        try:
            stop_process(process)
        finally:
            selector.close()
            for stream in (process.stdout, process.stderr):
                stream.close()


def safe_output(raw, operation, source_sha, operation_id):
    value = load_json_bytes(raw)
    if not isinstance(value, dict) or not set(value) <= SAFE_KEYS:
        fail("unexpected_tool_output")
    if value.get("format_version") != 1 or value.get("operation") != operation:
        fail("tool_operation_mismatch")
    if value.get("source_sha") != source_sha or value.get("operation_id") != operation_id:
        fail("tool_binding_mismatch")
    for key, item in value.items():
        if key in HASH_KEYS:
            if item != "" and (not isinstance(item, str) or not HASH.fullmatch(item)):
                fail("invalid_tool_hash")
        elif key in {"archive_eligible", "drop_eligible", "ledger_complete"}:
            if not isinstance(item, bool):
                fail("invalid_tool_boolean")
        elif key in {"format_version", "error_code", "target_table_count", "non_target_count", "remaining_target_count"} | COUNT_KEYS:
            if isinstance(item, bool) or not isinstance(item, int) or not 0 <= item <= 2147483647:
                fail("invalid_tool_number")
            if key in COUNT_KEYS | {"remaining_target_count"} and item > 22:
                fail("invalid_tool_count")
        elif key not in {"source_sha", "operation_id"}:
            if not isinstance(item, str) or not re.fullmatch(r"[a-z0-9_-]{0,80}", item):
                fail("invalid_tool_token")
    if value.get("target_table_count") != 22:
        fail("tool_target_count_mismatch")
    return value


class Runtime:
    def __init__(self, binary, run_id, source_sha, docker=None):
        self.binary = binary
        self.run_id = run_id
        self.source_sha = source_sha
        self.docker = docker or ["sudo", "-n", "docker"]
        self.owner = uuid.uuid4().hex
        self.label = OWNER_LABEL + "=" + self.owner
        self.deadline = time.monotonic() + 4200
        self.cleaning = False
        self.last_failure_output = None

    def budget(self, desired):
        if self.cleaning:
            return min(desired, 30)
        remaining = self.deadline - time.monotonic() - 120
        if remaining <= 0:
            fail("operation_time_budget_exhausted")
        return min(desired, remaining)

    def invoke(self, args, **kwargs):
        kwargs["timeout"] = self.budget(kwargs.get("timeout", 30))
        status, output = capture([*self.docker, *args], **kwargs)
        if status != 0:
            fail("docker_operation_failed")
        return output.strip()

    def image(self, tag, *, pull=False):
        status, output = capture([*self.docker, "image", "inspect", "--format", "{{.Id}}", tag])
        if status != 0:
            if not pull:
                fail("client_image_unavailable")
            self.invoke(["pull", "--quiet", tag], timeout=300)
            output = self.invoke(["image", "inspect", "--format", "{{.Id}}", tag])
        value = output.strip()
        if not re.fullmatch(r"sha256:[0-9a-f]{64}", value):
            fail("image_identity_invalid")
        return value

    def preflight_name(self, name):
        status, _ = capture([*self.docker, "container", "inspect", "--format", "{{.Id}}", name])
        if status == 0:
            fail("container_name_conflict")
        if status != 1:
            fail("container_preflight_failed")

    def tool_flags(self, operation, operation_id, marker=False):
        flags = ["--operation", operation, "--connection", "/connection/mysql.json",
                 "--defaults-file", "/connection/mysql.cnf", "--archive-dir", "/archive/" + operation_id,
                 "--source-sha", self.source_sha]
        if marker:
            flags += ["--restore-target-marker", "/connection/restore-target.json"]
        return flags

    def tool(self, operation, connection, archive, operation_id, image, *, restore_id=None):
        flags = self.tool_flags(operation, operation_id, restore_id is not None)
        if restore_id is None:
            name = "qs-cbpt-client-" + self.run_id + "-" + uuid.uuid4().hex[:12]
            self.preflight_name(name)
            command = [
                *self.docker, "run", "--rm", "--name", name, "--pull=never", "--network", "infra-network",
                "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges",
                "--cpus=2", "--memory=1g", "--pids-limit=128", "--user", str(os.getuid()) + ":" + str(os.getgid()),
                "--tmpfs", "/tmp:rw,noexec,nosuid,size=32m,mode=1777", "--env", "HOME=/tmp",
                "--label", self.label, "--mount", "type=bind,source=" + str(self.binary) + ",target=/cbpt-tool,readonly",
                "--mount", "type=bind,source=" + str(connection) + ",target=/connection,readonly",
                "--mount", "type=bind,source=" + str(archive) + ",target=/archive/" + operation_id,
                "--entrypoint", "/cbpt-tool", image, *flags,
            ]
        else:
            command = [*self.docker, "exec", "--user", str(os.getuid()) + ":" + str(os.getgid()),
                       restore_id, "/cbpt-tool", *flags]
        desired = 3600 if operation == "apply" else (1800 if operation in {"archive", "verify-restored"} else 600)
        status, raw = capture(command, timeout=self.budget(desired),
                              guard=lambda: archive_guard(archive))
        if status != 0:
            # Parse only the fixed failure envelope; untrusted text never escapes.
            try:
                output = safe_output(raw, operation, self.source_sha, operation_id)
                self.last_failure_output = output
                category = output.get("error_category", "")
                if re.fullmatch(r"[a-z0-9_]{1,80}", category):
                    fail("tool_" + category)
            except CleanupError as error:
                if str(error).startswith("tool_"):
                    raise
            fail("tool_execution_failed")
        output = safe_output(raw, operation, self.source_sha, operation_id)
        status_value = output.get("status")
        category = output.get("error_category", "")
        if status_value == "archive_only":
            reason = output.get("drop_block_reason", "") or category
            valid = (operation in {"audit", "archive"} and output.get("archive_eligible") is True
                     and output.get("drop_eligible") is False and reason in ARCHIVE_ONLY_REASONS
                     and category in {"", "none", reason})
        else:
            valid = status_value == "ok" and category in {"", "none"}
        if not valid or (operation == "apply" and output.get("drop_eligible") is not True):
            fail("tool_result_not_success")
        if operation == "verify-removed" and (output.get("remaining_target_count") != 0
                or output.get("ledger_complete") is not True or output.get("dropped_count") != 22
                or output.get("unknown_count") != 0):
            fail("cleanup_completion_unconfirmed")
        return output

    def docker_space(self, required):
        root = self.invoke(["info", "--format", "{{.DockerRootDir}}"])
        if not root.startswith("/") or any(char in root for char in ("\x00", "\r", "\n")):
            fail("docker_storage_identity_invalid")
        # Filesystem metadata needs no extra sudo grant when ancestors are searchable.
        status, output = capture(["env", "LC_ALL=C", "df", "-Pk", "--", root], timeout=15)
        if status != 0 or not output.endswith("\n") or "\r" in output:
            fail("docker_disk_unknown")
        lines = output.splitlines()
        if len(lines) != 2 or lines[0].split() != ["Filesystem", "1024-blocks", "Used", "Available", "Capacity", "Mounted", "on"]:
            fail("docker_disk_unknown")
        fields = lines[1].split(maxsplit=5)
        if (any(ord(char) < 32 and char != "\t" or ord(char) == 127 for char in lines[1])
                or len(fields) != 6 or any(not re.fullmatch(r"[0-9]{1,16}", token) for token in fields[1:4])
                or not re.fullmatch(r"[0-9]{1,3}%", fields[4]) or int(fields[4][:-1]) > 100
                or not fields[5].startswith("/") or any(ord(char) < 32 for char in fields[5])):
            fail("docker_disk_unknown")
        total, used, available = (int(token) for token in fields[1:4])
        if total <= 0 or total > (2 ** 63 - 1) // 1024 or used + available > total:
            fail("docker_disk_unknown")
        if available * 1024 < required:
            fail("docker_disk_low")

    def restore_container(self, connection, archive, operation_id, image):
        name = "qs-cbpt-restore-" + self.owner
        volume = name + "-data"
        self.preflight_name(name)
        status, _ = capture([*self.docker, "volume", "inspect", volume])
        if status == 0:
            fail("restore_volume_name_conflict")
        if status != 1:
            fail("restore_volume_preflight_failed")
        created = self.invoke(["volume", "create", "--label", self.label, volume])
        if created != volume:
            fail("restore_volume_identity_invalid")
        labels = load_json_bytes(self.invoke(["volume", "inspect", "--format", "{{json .Labels}}", volume]))
        if not isinstance(labels, dict) or labels.get(OWNER_LABEL) != self.owner:
            fail("restore_volume_ownership_unconfirmed")
        result = self.invoke([
            "run", "-d", "--name", name, "--pull=never", "--network", "none", "--read-only",
            "--cap-drop=ALL", "--cap-add=CHOWN", "--cap-add=SETUID", "--cap-add=SETGID", "--cap-add=DAC_OVERRIDE",
            "--security-opt=no-new-privileges", "--cpus=2", "--memory=2g", "--pids-limit=256",
            "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m,mode=1777",
            "--tmpfs", "/var/run/mysqld:rw,nosuid,size=16m,mode=1777",
            "--label", self.label, "--mount", "type=volume,source=" + volume + ",target=/var/lib/mysql",
            "--mount", "type=bind,source=" + str(connection) + ",target=/connection,readonly",
            "--mount", "type=bind,source=" + str(archive) + ",target=/archive/" + operation_id,
            "--mount", "type=bind,source=" + str(self.binary) + ",target=/cbpt-tool,readonly",
            "--env", "MYSQL_ROOT_PASSWORD_FILE=/connection/root-password",
            image, "--bind-address=127.0.0.1", "--port=3306", "--max-allowed-packet=256M",
        ], timeout=60)
        if not re.fullmatch(r"[0-9a-f]{64}", result):
            fail("restore_container_identity_invalid")
        info = load_json_bytes(self.invoke(["container", "inspect", result]))
        try:
            actual = info[0]
            safe = (actual["Id"] == result and actual["Config"]["Labels"].get(OWNER_LABEL) == self.owner
                    and actual["HostConfig"]["NetworkMode"] == "none" and not actual["HostConfig"].get("PortBindings")
                    and not actual["HostConfig"].get("Privileged"))
        except (KeyError, IndexError, TypeError):
            safe = False
        if not safe:
            fail("restore_container_isolation_unconfirmed")
        return result

    def restore_sql(self, container, sql, *, timeout=30):
        return capture([*self.docker, "exec", "--user", str(os.getuid()) + ":" + str(os.getgid()),
                        container, "mysql", "--defaults-file=/connection/mysql.cnf", "--protocol=tcp",
                        "--connect-timeout=5", "--batch", "--skip-column-names", "-e", sql], timeout=self.budget(timeout))

    def wait_restore(self, container, source_uuid):
        deadline = time.monotonic() + self.budget(180)
        while time.monotonic() < deadline:
            status, raw = self.restore_sql(container, "SELECT @@server_uuid, VERSION();", timeout=10)
            if status == 0:
                pieces = raw.strip().split("\t")
                if len(pieces) != 2 or not SERVER_UUID.fullmatch(pieces[0]) or pieces[1] != "8.0.36":
                    fail("restore_server_identity_invalid")
                if pieces[0] == source_uuid:
                    fail("restore_source_identity_collision")
                return pieces[0]
            time.sleep(1)
        fail("restore_server_not_ready")

    def stream_restore(self, container, archive):
        command = [*self.docker, "exec", "-i", "--user", str(os.getuid()) + ":" + str(os.getgid()), container,
                   "mysql", "--defaults-file=/connection/mysql.cnf", "--protocol=tcp",
                   "--connect-timeout=5", "--binary-mode=1", archive["restore_database"]]
        restore_stream(command, archive["path"], timeout=self.budget(1800))

    def cleanup(self):
        # Only exact IDs/names bearing this operation's random label may be removed.
        self.cleaning = True
        try:
            ids = self.invoke(["ps", "--all", "--quiet", "--no-trunc", "--filter", "label=" + self.label]).splitlines()
            if len(ids) > 4 or any(not re.fullmatch(r"[0-9a-f]{64}", value) for value in ids):
                fail("resource_cleanup_unconfirmed")
            for container in ids:
                self.invoke(["rm", "--force", container])
            if self.invoke(["ps", "--all", "--quiet", "--no-trunc", "--filter", "label=" + self.label]):
                fail("resource_cleanup_unconfirmed")
            volumes = self.invoke(["volume", "ls", "--quiet", "--filter", "label=" + self.label]).splitlines()
            expected = "qs-cbpt-restore-" + self.owner + "-data"
            if len(volumes) > 1 or any(value != expected for value in volumes):
                fail("resource_cleanup_unconfirmed")
            for volume in volumes:
                labels = load_json_bytes(self.invoke(["volume", "inspect", "--format", "{{json .Labels}}", volume]))
                if not isinstance(labels, dict) or labels.get(OWNER_LABEL) != self.owner:
                    fail("resource_cleanup_unconfirmed")
                self.invoke(["volume", "rm", volume])
            if self.invoke(["volume", "ls", "--quiet", "--filter", "label=" + self.label]):
                fail("resource_cleanup_unconfirmed")
        except CleanupError:
            fail("resource_cleanup_unconfirmed")
        finally:
            self.cleaning = False


def dump_check(path, expected, *, deadline=None):
    if not HASH.fullmatch(expected):
        fail("dump_hash_invalid")
    try:
        info = path.lstat()
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o600 or info.st_size == 0 or info.st_size > ARCHIVE_LIMIT:
            fail("dump_file_invalid")
        digest = hashlib.sha256()
        with path.open("rb") as stream:
            for block in iter(lambda: stream.read(1024 * 1024), b""):
                if deadline is not None and time.monotonic() >= deadline:
                    fail("operation_time_budget_exhausted")
                digest.update(block)
        if digest.hexdigest() != expected:
            fail("dump_hash_mismatch")
        total = 0
        with gzip.open(path, "rb") as stream:
            for block in iter(lambda: stream.read(1024 * 1024), b""):
                if deadline is not None and time.monotonic() >= deadline:
                    fail("operation_time_budget_exhausted")
                total += len(block)
                if total > RESTORE_LIMIT:
                    fail("restore_size_limit")
        if total == 0:
            fail("dump_empty")
        return total
    except (OSError, EOFError):
        fail("dump_archive_invalid")


def restore_stream(command, path, *, timeout):
    """Bounded nonblocking gzip-to-client stream; never print client output."""
    try:
        process = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE, start_new_session=True, bufsize=0)
    except OSError:
        fail("restore_client_start_failed")
    selector = selectors.DefaultSelector()
    deadline = time.monotonic() + timeout
    total = 0
    output_total = 0
    pending = b""
    eof = False
    try:
        for stream in (process.stdin, process.stdout, process.stderr):
            os.set_blocking(stream.fileno(), False)
            selector.register(stream, selectors.EVENT_WRITE if stream is process.stdin else selectors.EVENT_READ)
        with gzip.open(path, "rb") as dump:
            while selector.get_map():
                if time.monotonic() >= deadline:
                    fail("restore_client_timeout")
                for key, _ in selector.select(0.2):
                    if key.fileobj is process.stdin:
                        if not pending and not eof:
                            pending = dump.read(65536)
                            total += len(pending)
                            if total > RESTORE_LIMIT:
                                fail("restore_size_limit")
                            eof = not pending
                        if eof:
                            selector.unregister(process.stdin)
                            process.stdin.close()
                        else:
                            try:
                                sent = os.write(process.stdin.fileno(), pending)
                                pending = pending[sent:]
                            except BlockingIOError:
                                pass
                    else:
                        block = os.read(key.fileobj.fileno(), 65536)
                        if not block:
                            selector.unregister(key.fileobj)
                        else:
                            output_total += len(block)
                            if output_total > CAPTURE_LIMIT:
                                fail("restore_client_output_limit")
            if process.wait(timeout=max(0.01, deadline - time.monotonic())) != 0 or not eof:
                fail("restore_client_failed")
    except (OSError, EOFError, subprocess.TimeoutExpired):
        fail("restore_client_failed")
    finally:
        try:
            stop_process(process)
        finally:
            selector.close()
            for stream in (process.stdin, process.stdout, process.stderr):
                if not stream.closed:
                    stream.close()


def manifest_binding(archive, operation_id, source_sha):
    raw = private_file(archive / "manifest.json")
    value = load_json_bytes(raw)
    if not isinstance(value, dict) or type(value.get("format_version")) is not int or value.get("format_version") != 1 or value.get("operation_id") != operation_id or value.get("source_sha") != source_sha:
        fail("manifest_binding_mismatch")
    if not isinstance(value.get("source_server_uuid"), str) or not SERVER_UUID.fullmatch(value["source_server_uuid"]) or not isinstance(value.get("source_target_hash"), str) or not HASH.fullmatch(value["source_target_hash"]):
        fail("manifest_source_identity_invalid")
    if value.get("migration_version") != 95 or isinstance(value.get("migration_version"), bool) or value.get("migration_dirty") is not False or not isinstance(value.get("archive_drop_eligible"), bool):
        fail("manifest_migration_binding_invalid")
    if value.get("dump_file") != "dump.sql.gz" or not isinstance(value.get("dump_sha256"), str) or not HASH.fullmatch(value["dump_sha256"]) or not isinstance(value.get("tables"), list) or len(value["tables"]) != 22:
        fail("manifest_archive_invalid")
    tables = value["tables"]
    if any(not isinstance(table, dict) for table in tables) or sorted(table.get("name", "") for table in tables) != list(TARGETS):
        fail("manifest_target_scope_invalid")
    for table in tables:
        rows = table.get("rows")
        if isinstance(rows, bool) or not isinstance(rows, int) or not 0 <= rows <= 2 ** 64 - 1:
            fail("manifest_row_count_invalid")
    if sum(table["rows"] for table in tables) > 2 ** 64 - 1:
        fail("manifest_row_count_invalid")
    return value


def proof_binding(archive, operation_id, source_sha):
    manifest = manifest_binding(archive, operation_id, source_sha)
    raw_manifest = private_file(archive / "manifest.json")
    proof = load_json_bytes(private_file(archive / "restore-proof.json"))
    expected = {"format_version": 1, "operation_id": operation_id, "source_sha": source_sha,
                "source_target_hash": manifest["source_target_hash"], "dump_sha256": manifest["dump_sha256"],
                "manifest_sha256": hashlib.sha256(raw_manifest).hexdigest(), "verified": True,
                "target_table_count": 22}
    if not isinstance(proof, dict) or any(proof.get(key) != value for key, value in expected.items()) or type(proof.get("format_version")) is not int or type(proof.get("target_table_count")) is not int or proof.get("verified") is not True:
        fail("restore_proof_binding_mismatch")
    restored = proof.get("restore_server_uuid", "")
    if not isinstance(restored, str) or not SERVER_UUID.fullmatch(restored) or restored == manifest["source_server_uuid"]:
        fail("restore_proof_identity_invalid")
    if not isinstance(proof.get("marker_sha256"), str) or not HASH.fullmatch(proof["marker_sha256"]):
        fail("restore_proof_marker_invalid")
    return proof


def perform(args, env, *, archive_root=None, runtime_class=Runtime):
    if args.operation not in {"audit", "archive-verify", "apply", "verify"} or not SHA.fullmatch(args.source_sha) or not RUN_ID.fullmatch(args.run_id):
        fail("invalid_cleanup_binding")
    if args.operation in {"apply", "verify"}:
        if not RUN_ID.fullmatch(args.archive_id or ""):
            fail("archive_id_required")
    elif args.archive_id:
        fail("unexpected_archive_id")
    binary = Path(args.tool_binary)
    try:
        info = binary.lstat()
        if not binary.is_absolute() or any(char in str(binary) for char in (",", "\x00", "\r", "\n")) or not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or not info.st_mode & stat.S_IXUSR or info.st_mode & 0o022:
            fail("tool_binary_invalid")
    except OSError:
        fail("tool_binary_unavailable")
    values = validate_source(env)
    root = archive_root or ARCHIVE_ROOT
    result = {"format_version": 1, "operation": args.operation, "source_sha": args.source_sha,
              "run_id": args.run_id, "complete": False, "archive_private": True,
              "archive_retention": "preserve_no_automatic_prune", "stage": "preflight", "stages": []}
    runtime = runtime_class(binary, args.run_id, args.source_sha)
    operation_id = args.archive_id or args.run_id
    result["archive_id"] = operation_id
    with archive_lock(root):
        if args.operation == "audit":
            temporary = tempfile.TemporaryDirectory(prefix="qs-cbpt-audit-")
            archive = Path(temporary.name)
        elif args.operation == "archive-verify":
            archive = root / operation_id
            try:
                archive.mkdir(mode=0o700)
            except FileExistsError:
                fail("archive_operation_exists")
            except OSError:
                fail("archive_create_failed")
            temporary = None
        else:
            archive = root / operation_id
            directory_check(archive)
            temporary = None
            manifest_binding(archive, operation_id, args.source_sha)
        try:
            if shutil.disk_usage(archive).free < (8 * GIB if args.operation == "archive-verify" else MIN_FREE):
                fail("archive_disk_low")
            runtime.invoke(["network", "inspect", "--format", "{{.Name}}", "infra-network"])
            source_image = runtime.image("mysql:8.0")
            with connections(values) as connection:
                try:
                    if args.operation == "audit":
                        result["stage"] = "source_audit"
                        result["stages"].append(runtime.tool("audit", connection, archive, operation_id, source_image))
                    elif args.operation == "archive-verify":
                        result["stage"] = "source_audit"
                        audit = runtime.tool("audit", connection, archive, operation_id, source_image)
                        result["stages"].append(audit)
                        if not audit.get("archive_eligible"):
                            fail("archive_not_eligible")
                        result["stage"] = "source_archive"
                        result["stages"].append(runtime.tool("archive", connection, archive, operation_id, source_image))
                        result["stage"] = "archive_validation"
                        manifest = manifest_binding(archive, operation_id, args.source_sha)
                        if manifest["source_target_hash"] != audit.get("source_target_hash"):
                            fail("archive_audit_binding_mismatch")
                        size = dump_check(archive / "dump.sql.gz", manifest["dump_sha256"], deadline=runtime.deadline - 120)
                        runtime.docker_space(max(8 * GIB, size * 3 + MIN_FREE))
                        result["stage"] = "restore_runtime"
                        restore_image = runtime.image("docker.io/library/mysql:8.0.36", pull=True)
                        nonce = uuid.uuid4().hex
                        database = "cbpt_restore_" + nonce
                        isolated = {"host": "127.0.0.1", "port": 3306, "user": "root",
                                    "password": secrets.token_hex(32), "database": database}
                        # Credentials/marker belong only to this isolated runtime.
                        with connections(isolated, marker={}) as restore_connection:
                            container = runtime.restore_container(restore_connection, archive, operation_id, restore_image)
                            restore_uuid = runtime.wait_restore(container, manifest["source_server_uuid"])
                            marker = {"format_version": 1, "operation_id": operation_id, "nonce": nonce,
                                      "container_owner": runtime.owner, "source_server_uuid": manifest["source_server_uuid"],
                                      "restore_server_uuid": restore_uuid, "restore_database": database,
                                      "source_target_hash": manifest["source_target_hash"]}
                            (restore_connection / "restore-target.json").unlink()
                            json_private(restore_connection / "restore-target.json", marker)
                            status, _ = runtime.restore_sql(container, "CREATE DATABASE `" + database + "` CHARACTER SET utf8mb4;")
                            if status != 0:
                                fail("restore_database_create_failed")
                            result["stage"] = "restore_import"
                            runtime.stream_restore(container, {"path": archive / "dump.sql.gz", "restore_database": database})
                            result["stage"] = "restore_validation"
                            result["stages"].append(runtime.tool("verify-restored", restore_connection, archive, operation_id,
                                                                 restore_image, restore_id=container))
                            proof_binding(archive, operation_id, args.source_sha)
                            result["archive_compressed_bytes"] = (archive / "dump.sql.gz").lstat().st_size
                            result["archive_uncompressed_bytes"] = size
                            result["total_archived_rows"] = sum(table["rows"] for table in manifest["tables"])
                    elif args.operation == "apply":
                        proof_binding(archive, operation_id, args.source_sha)
                        result["stage"] = "source_apply"
                        result["stages"].append(runtime.tool("apply", connection, archive, operation_id, source_image))
                        result["stage"] = "source_verify"
                        result["stages"].append(runtime.tool("verify-removed", connection, archive, operation_id, source_image))
                    else:
                        result["stage"] = "source_verify"
                        result["stages"].append(runtime.tool("verify-removed", connection, archive, operation_id, source_image))
                finally:
                    runtime.cleanup()
        except CleanupError as error:
            result["error_category"] = str(error)
            if runtime.last_failure_output is not None:
                result["failure"] = runtime.last_failure_output
            error.result = result
            raise
        except Exception:
            error = CleanupError("unexpected_cleanup_failure")
            error.result = result
            raise error from None
        finally:
            if temporary is not None:
                temporary.cleanup()
    result["complete"] = True
    result["stage"] = "complete"
    return result


def main(argv=None, env=None):
    parser = argparse.ArgumentParser(description="Private fixed-scope cbpt cleanup")
    parser.add_argument("--operation", required=True, choices=("audit", "archive-verify", "apply", "verify"))
    parser.add_argument("--tool-binary", required=True)
    parser.add_argument("--source-sha", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--archive-id", default="")
    args = parser.parse_args(argv)
    try:
        result = perform(args, dict(os.environ) if env is None else env)
    except CleanupError as error:
        category = str(error)
        if not re.fullmatch(r"[a-z0-9_]{1,100}", category):
            category = "unexpected_cleanup_failure"
        base = {"format_version": 1, "complete": False, "operation": args.operation}
        if SHA.fullmatch(args.source_sha):
            base["source_sha"] = args.source_sha
        if RUN_ID.fullmatch(args.run_id):
            base["run_id"] = args.run_id
        archive_id = args.archive_id or args.run_id
        if RUN_ID.fullmatch(archive_id):
            base["archive_id"] = archive_id
        result = getattr(error, "result", base)
        result["error_category"] = category
    except Exception:
        result = {"format_version": 1, "complete": False, "error_category": "unexpected_cleanup_failure"}
    print("QS_CBPT_CLEANUP_BEGIN")
    print(json.dumps(result, ensure_ascii=True, sort_keys=True))
    print("QS_CBPT_CLEANUP_END")
    return 0 if result["complete"] else 1


if __name__ == "__main__":
    sys.exit(main())
