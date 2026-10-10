#!/usr/bin/env python3
"""Bounded ordinary-user Linux observation; no installation or writer permit.

No process environment, shadow, private key, container Env, arbitrary shell,
container exec, sudo Python, daemon reload or mutation is read or executed.
The request approval binds public metadata, not an authenticated host channel.
"""
import argparse
import base64
import fnmatch
import glob
import hashlib
import ipaddress
import json
import os
import pwd
from pathlib import Path
import re
import selectors
import shlex
import signal
import stat
import subprocess
import sys
import time

PROTOCOL = "qs-retirement-linux-host-inventory/v1"
OBSERVATION_PROTOCOL = "qs-retirement-linux-host-inventory/v2"
CONTEXT_MODE = "current_ssh_session_observation"
AUDITED_SOURCE = "0d5d75046328e6d4a415380f4ed249a6d3c9714c"
SHA = re.compile(r"[0-9a-f]{64}")
SOURCE = re.compile(r"[0-9a-f]{40}")
RUN = re.compile(r"[1-9][0-9]{0,19}-[1-9][0-9]{0,3}")
UNIT = re.compile(r"[A-Za-z0-9_.@:-]{1,200}\.(?:service|timer)")
SESSION = re.compile(r"[A-Za-z0-9]{1,32}")
FILE_CAP, OUTPUT_CAP, FILE_COUNT, PROCESS_CAP = 2 << 20, 2 << 20, 512, 32768
TOTAL_BYTES, TOTAL_SECONDS, QUERY_SECONDS = 32 << 20, 120, 5
BINARY_CAP, TOTAL_BINARY_BYTES = 128 << 20, 512 << 20
SSH_PROPERTIES = ("authenticationmethods", "authorizedkeysfile", "authorizedkeyscommand", "authorizedkeyscommanduser",
                  "authorizedprincipalsfile", "authorizedprincipalscommand", "trustedusercakeys", "passwordauthentication",
                  "kbdinteractiveauthentication", "hostbasedauthentication", "gssapiauthentication", "pubkeyauthentication",
                  "permituserenvironment", "permituserrc", "permittty", "disableforwarding", "forcecommand", "strictmodes",
                  "acceptenv", "allowusers", "allowgroups", "denyusers", "denygroups")
CAPABILITIES = ("management_channel_authenticated", "production_installed", "sshd_reloaded", "writer_fence_proven",
                "historical_rerun_denied", "execution_authority", "cas_ready", "drop_ready")
ALWAYS_UNKNOWN = ("management_channel_and_request_origin_unproven", "all_match_contexts_not_enumerated",
                  "nss_external_subject_coverage_unknown", "all_writers_and_external_services_unproven",
                  "historical_refs_reruns_queues_approvals_not_fenced", "local_runner_bypass_not_fenced",
                  "existing_sessions_not_drained", "effective_acl_visibility_unknown",
                  "systemd_user_socket_and_transient_activation_coverage_unknown", "ssh_key_option_semantics_and_authentication_unproven")
ERROR_CATEGORIES = frozenset(("account_schema_unknown", "command_denied_or_failed", "command_executable_changed",
    "command_executable_unprotected", "command_not_in_closed_read_set", "command_output_budget_exceeded", "command_timeout",
    "command_unavailable", "cron_directory_budget_exceeded", "directory_changed_during_read", "directory_missing",
    "directory_permission_unknown", "directory_read_unknown", "docker_mount_schema_unknown", "docker_projection_schema_unknown",
    "docker_roster_schema_or_budget_unknown", "file_budget_exceeded", "file_changed_during_read", "file_link_or_type_unsupported",
    "file_missing", "file_path_unsupported", "file_permission_unknown", "file_read_unknown", "file_content_forbidden", "inventory_deadline_exceeded",
    "inventory_input_rejected", "inventory_request_rejected", "inventory_fixed_failure", "linux_host_required",
    "process_budget_exceeded", "process_link_visibility_unknown", "process_schema_unknown", "property_projection_schema_unknown",
    "session_listing_schema_unknown", "ssh_authorized_key_schema_unknown", "ssh_config_syntax_unknown", "ssh_effective_duplicate_key",
    "ssh_effective_projection_incomplete", "ssh_include_cycle_or_depth_unknown", "ssh_include_directory_changed",
    "ssh_include_path_unsupported", "systemd_listing_schema_unknown", "systemd_unit_budget_exceeded"))


class Unknown(Exception):
    """Only a fixed category, never captured diagnostics or file paths."""
    def __init__(self, category):
        super().__init__(category if category in ERROR_CATEGORIES else "inventory_fixed_failure")


def reject(category):
    raise Unknown(category)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def canonical(value):
    return (json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True) + "\n").encode()


def validate_session(v):
    """Observed login environment is not an authenticated management channel.

    SSH_CONNECTION has four fields (OpenSSH session.c). It has no hostname.
    Never invent a hostname, resolve DNS here, or use USER/LOGNAME as identity.
    Source: https://github.com/openssh/openssh-portable/blob/master/session.c
    """
    keys = {"uid", "euid", "username", "ssh_connection", "identity_sha256", "connection_sha256", "origin"}
    if (type(v) is not dict or set(v) != keys or any(type(v[k]) is not int or not 0 <= v[k] < (1 << 32) for k in ("uid", "euid"))
            or type(v["username"]) is not str or not re.fullmatch(r"[A-Za-z0-9_.-]{1,64}", v["username"])
            or type(v["ssh_connection"]) is not str or len(v["ssh_connection"]) > 1024
            or v["origin"] != "ordinary_ssh_session_environment"):
        reject("inventory_request_rejected")
    fields = v["ssh_connection"].split(" ")
    if len(fields) != 4 or any(not x for x in fields):
        reject("inventory_request_rejected")
    try:
        for x in (fields[0], fields[2]):
            if not re.fullmatch(r"[0-9A-Fa-f:.]{1,45}", x):
                reject("inventory_request_rejected")
            ipaddress.ip_address(x)
        for x in (fields[1], fields[3]):
            if not re.fullmatch(r"[1-9][0-9]{0,4}", x) or int(x) > 65535:
                reject("inventory_request_rejected")
    except ValueError:
        reject("inventory_request_rejected")
    identity = {k: v[k] for k in ("uid", "euid", "username")}
    if v["identity_sha256"] != sha(canonical(identity)) or v["connection_sha256"] != sha(v["ssh_connection"].encode("ascii")):
        reject("inventory_request_rejected")
    return fields


def observe_current_session():
    """Public finite observation API; ordinary shell may alter its environment."""
    try:
        uid, euid = os.getuid(), os.geteuid()
        subject, effective = pwd.getpwuid(uid), pwd.getpwuid(euid)
        user = subject.pw_name
        # A differing effective principal cannot be silently attributed to login.
        if subject.pw_uid != uid or effective.pw_uid != euid or effective.pw_name != user or uid != euid:
            reject("inventory_request_rejected")
        identity = {"uid": uid, "euid": euid, "username": user}
        connection = os.environ.get("SSH_CONNECTION", "")
        v = dict(identity, ssh_connection=connection, identity_sha256=sha(canonical(identity)),
                 connection_sha256=sha(connection.encode("ascii")), origin="ordinary_ssh_session_environment")
        validate_session(v)
        return v
    except (KeyError, UnicodeError, OSError):
        reject("inventory_request_rejected")


def request(raw, approved):
    def pairs(items):
        out = {}
        for k, v in items:
            if k in out:
                reject("inventory_request_rejected")
            out[k] = v
        return out
    try:
        v = json.loads(raw, object_pairs_hook=pairs)
        observation = type(v) is dict and v.get("protocol") == OBSERVATION_PROTOCOL
        keys = {"protocol", "source_sha", "operation_id", "run_id", "host_role"} | ({"context_mode", "session"} if observation else {"matches"})
        if (type(approved) is not str or not SHA.fullmatch(approved) or sha(raw) != approved or type(v) is not dict
                or set(v) != keys or canonical(v) != raw or v["protocol"] not in (PROTOCOL, OBSERVATION_PROTOCOL)
                or type(v["source_sha"]) is not str or not SOURCE.fullmatch(v["source_sha"])
                or any(type(v[k]) is not str or not RUN.fullmatch(v[k]) for k in ("operation_id", "run_id"))
                or v["host_role"] not in ("server_a", "server_b", "server_d", "runner")):
            reject("inventory_request_rejected")
        if observation:
            if v["context_mode"] != CONTEXT_MODE:
                reject("inventory_request_rejected")
            validate_session(v["session"])
            return v
        if type(v["matches"]) is not list or not 1 <= len(v["matches"]) <= 32:
            reject("inventory_request_rejected")
        seen = set()
        for m in v["matches"]:
            if (type(m) is not dict or set(m) != {"user", "host", "addr", "laddr", "lport"}
                    or any(type(x) is not str or not re.fullmatch(r"[A-Za-z0-9_.:-]{1,128}", x) for x in m.values())
                    or not m["lport"].isdigit() or not 1 <= int(m["lport"]) <= 65535 or canonical(m) in seen):
                reject("inventory_request_rejected")
            seen.add(canonical(m))
        return v
    except Unknown:
        raise
    except (ValueError, UnicodeError, TypeError, KeyError):
        reject("inventory_request_rejected")


def stamp(s):
    return (s.st_dev, s.st_ino, s.st_mode, s.st_uid, s.st_gid, s.st_nlink, s.st_size, s.st_mtime_ns, s.st_ctime_ns)


class _Files:
    """Private test root seam. Public inventory always selects actual '/'."""
    def __init__(self, root="/"):
        self.root = Path(root)
        self.witnesses, self.bytes, self.binary_bytes, self.binary_paths = {}, 0, 0, set()
        self.metadata_witnesses, self.directory_witnesses = {}, {}
        self.deadline = time.monotonic() + TOTAL_SECONDS

    def physical(self, path):
        if type(path) is not str or not path.startswith("/") or os.path.normpath(path) != path or "\x00" in path or "//" in path:
            reject("file_path_unsupported")
        return self.root / path.lstrip("/") if str(self.root) != "/" else Path(path)

    def read(self, path, proc=False, binary=False):
        name = os.path.basename(path)
        if (name in ("shadow", "gshadow", "environ", ".env") or name.endswith(".env") or name.startswith("id_")
                or (path.startswith("/etc/ssh/ssh_host_") and not name.endswith(".pub"))
                or path.startswith(("/etc/cron", "/var/spool/cron", "/etc/systemd/", "/usr/lib/systemd/"))):
            reject("file_content_forbidden")
        p = self.physical(path)
        flags = os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK
        fds, parents = [], []
        try:
            parent = os.open("/", flags | os.O_DIRECTORY)
            fds.append(parent)
            root_directory = os.fstat(parent)
            if not stat.S_ISDIR(root_directory.st_mode) or not os.path.samestat(root_directory, os.stat("/", follow_symlinks=False)):
                reject("file_changed_during_read")
            for part in p.parts[1:-1]:
                before = os.stat(part, dir_fd=parent, follow_symlinks=False)
                if not stat.S_ISDIR(before.st_mode):
                    reject("file_link_or_type_unsupported")
                child = os.open(part, flags | os.O_DIRECTORY, dir_fd=parent)
                fds.append(child)
                s = os.fstat(child)
                identity = (s.st_dev, s.st_ino, s.st_mode, s.st_uid, s.st_gid)
                if identity != (before.st_dev, before.st_ino, before.st_mode, before.st_uid, before.st_gid):
                    reject("file_changed_during_read")
                parents.append((parent, part, child, identity))
                parent = child
            before = os.stat(p.name, dir_fd=parent, follow_symlinks=False)
            cap = BINARY_CAP if binary else FILE_CAP
            if not stat.S_ISREG(before.st_mode) or before.st_nlink != 1 or before.st_size > cap:
                reject("file_link_or_type_unsupported")
            fd = os.open(p.name, flags, dir_fd=parent)
            fds.append(fd)
            actual = os.fstat(fd)
            if stamp(actual) != stamp(before):
                reject("file_changed_during_read")
            chunks, count, hasher = [], 0, hashlib.sha256()
            while True:
                if time.monotonic() > self.deadline:
                    reject("inventory_deadline_exceeded")
                b = os.read(fd, min(65536, cap + 1 - count))
                if not b:
                    break
                count += len(b)
                if count > cap:
                    reject("file_budget_exceeded")
                hasher.update(b)
                if not binary:
                    chunks.append(b)
            raw = None if binary else b"".join(chunks)
            if stamp(os.fstat(fd)) != stamp(before) or stamp(os.stat(p.name, dir_fd=parent, follow_symlinks=False)) != stamp(before):
                reject("file_changed_during_read")
            for parent, name, child, expected in parents:
                s, named = os.fstat(child), os.stat(name, dir_fd=parent, follow_symlinks=False)
                if any((x.st_dev, x.st_ino, x.st_mode, x.st_uid, x.st_gid) != expected for x in (s, named)):
                    reject("file_changed_during_read")
            root_after, root_named = os.fstat(fds[0]), os.stat("/", follow_symlinks=False)
            root_identity = (root_directory.st_dev, root_directory.st_ino, root_directory.st_mode, root_directory.st_uid, root_directory.st_gid)
            if any((s.st_dev, s.st_ino, s.st_mode, s.st_uid, s.st_gid) != root_identity for s in (root_after, root_named)):
                reject("file_changed_during_read")
            if not proc and count != before.st_size:
                reject("file_changed_during_read")
            if binary:
                self.binary_paths.add(path)
                self.binary_bytes += count
            else:
                self.bytes += count
            if self.bytes > TOTAL_BYTES or self.binary_bytes > TOTAL_BINARY_BYTES:
                reject("file_budget_exceeded")
            if not proc:
                if len(self.witnesses) >= FILE_COUNT and path not in self.witnesses:
                    reject("file_budget_exceeded")
                current = (stamp(before), hasher.hexdigest())
                if path in self.witnesses and self.witnesses[path] != current:
                    reject("file_changed_during_read")
                self.witnesses[path] = current
            return raw, {"path_sha256": sha(path.encode()), "raw_sha256": hasher.hexdigest(), "bytes": count,
                         "stat_sha256": sha(canonical(stamp(before))), "uid": before.st_uid,
                         "gid": before.st_gid, "mode": stat.S_IMODE(before.st_mode), "single_link": True,
                         "root_mode_protected": before.st_uid == 0 and not before.st_mode & 0o022,
                         "ancestor_root_mode_protected": root_directory.st_uid == 0 and not root_directory.st_mode & 0o022
                             and all(x[3][3] == 0 and not x[3][2] & 0o022 for x in parents),
                         "acl_complete": False}
        except FileNotFoundError:
            reject("file_missing")
        except PermissionError:
            reject("file_permission_unknown")
        except OSError:
            reject("file_read_unknown")
        finally:
            for fd in reversed(fds):
                os.close(fd)

    def list(self, path):
        p = self.physical(path)
        descriptors, parents = [], []
        try:
            flags = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_NONBLOCK
            fd = os.open("/", flags)
            descriptors.append(fd)
            root_directory = os.fstat(fd)
            for part in p.parts[1:]:
                before = os.stat(part, dir_fd=fd, follow_symlinks=False)
                if not stat.S_ISDIR(before.st_mode):
                    reject("file_link_or_type_unsupported")
                child = os.open(part, flags, dir_fd=fd)
                descriptors.append(child)
                actual = os.fstat(child)
                expected = (before.st_dev, before.st_ino, before.st_mode, before.st_uid, before.st_gid)
                if (actual.st_dev, actual.st_ino, actual.st_mode, actual.st_uid, actual.st_gid) != expected:
                    reject("directory_changed_during_read")
                parents.append((fd, part, child, expected))
                fd = child
            before = os.fstat(fd)
            result = sorted(os.listdir(fd))
            if stamp(os.fstat(fd)) != stamp(before):
                reject("directory_changed_during_read")
            for parent, name, child, expected in parents:
                actual, named = os.fstat(child), os.stat(name, dir_fd=parent, follow_symlinks=False)
                if any((s.st_dev, s.st_ino, s.st_mode, s.st_uid, s.st_gid) != expected for s in (actual, named)):
                    reject("directory_changed_during_read")
            root_after, root_named = os.fstat(descriptors[0]), os.stat("/", follow_symlinks=False)
            expected_root = (root_directory.st_dev, root_directory.st_ino, root_directory.st_mode, root_directory.st_uid, root_directory.st_gid)
            if any((s.st_dev, s.st_ino, s.st_mode, s.st_uid, s.st_gid) != expected_root for s in (root_after, root_named)):
                reject("directory_changed_during_read")
            baseline = (before.st_dev, before.st_ino, before.st_mode, before.st_uid, before.st_gid, tuple(result))
            if path in self.directory_witnesses and self.directory_witnesses[path] != baseline:
                reject("directory_changed_during_read")
            self.directory_witnesses[path] = baseline
            return result
        except FileNotFoundError:
            reject("directory_missing")
        except PermissionError:
            reject("directory_permission_unknown")
        except OSError:
            reject("directory_read_unknown")
        finally:
            for fd in reversed(descriptors):
                os.close(fd)

    def metadata(self, path):
        try:
            p = self.physical(path)
            s = os.stat(p, follow_symlinks=False)
            baseline = (stamp(s), os.access(p, os.W_OK))
            if path in self.metadata_witnesses and self.metadata_witnesses[path] != baseline:
                reject("file_changed_during_read")
            self.metadata_witnesses[path] = baseline
            return {"path_sha256": sha(path.encode()), "stat_sha256": sha(canonical(stamp(s))),
                    "uid": s.st_uid, "gid": s.st_gid, "mode": stat.S_IMODE(s.st_mode), "is_link": stat.S_ISLNK(s.st_mode),
                    "current_user_write_access": os.access(p, os.W_OK), "acl_complete": False}
        except FileNotFoundError:
            reject("file_missing")
        except PermissionError:
            reject("file_permission_unknown")
        except OSError:
            reject("file_read_unknown")

    def link(self, path):
        try:
            return os.readlink(self.physical(path))
        except OSError:
            reject("process_link_visibility_unknown")

    def recheck(self):
        expected = dict(self.witnesses)
        for path, baseline in expected.items():
            self.read(path, binary=path in self.binary_paths)
            if self.witnesses[path] != baseline:
                reject("file_changed_during_read")
        for path in tuple(self.metadata_witnesses):
            self.metadata(path)
        for path in tuple(self.directory_witnesses):
            self.list(path)


DOCKER_FORMAT = ('{"id":{{json .Id}},"name":{{json .Name}},"component":{{json (index .Config.Labels "prometheus.component")}},"image":{{json .Image}},"status":{{json .State.Status}},'
                 '"running":{{json .State.Running}},"started":{{json .State.StartedAt}},"restarts":{{json .RestartCount}},'
                 '"privileged":{{json .HostConfig.Privileged}},"readonly":{{json .HostConfig.ReadonlyRootfs}},'
                 '"user":{{json .Config.User}},"project":{{json (index .Config.Labels "com.docker.compose.project")}},'
                 '"service":{{json (index .Config.Labels "com.docker.compose.service")}},'
                 '"mounts":[{{range $i,$m := .Mounts}}{{if $i}},{{end}}'
                 '{"source":{{json $m.Source}},"target":{{json $m.Destination}},"rw":{{json $m.RW}},"type":{{json $m.Type}}}{{end}}]}')
QS_SERVICE_FORMAT = ('{"id":{{json .Id}},"name":{{json .Name}},"image":{{json .Image}},'
 '"component":{{json (index .Config.Labels "prometheus.component")}},'
 '"project":{{json (index .Config.Labels "com.docker.compose.project")}},'
 '"service":{{json (index .Config.Labels "com.docker.compose.service")}},'
 '"entrypoint":{{json .Config.Entrypoint}},"command":{{json .Config.Cmd}},'
 '"running":{{json .State.Running}},"started_at":{{json .State.StartedAt}},'
 '"restart_policy":{{json .HostConfig.RestartPolicy.Name}},"restart_maximum":{{json .HostConfig.RestartPolicy.MaximumRetryCount}}}')
QS_RESTORE_IMAGE_FORMAT = '{"id":{{json .Id}},"os":{{json .Os}},"architecture":{{json .Architecture}}}'
QS_RESTORE_IMAGES = ("mysql:8.0", "mongo:7.0")
QS_COMPONENTS = ("qs-apiserver", "qs-collection-server", "qs-worker")

def validate_qs_service_observation(value, role):
    expected = {"kind", "host_role", "machine_id_sha256", "docker_path", "docker_sha256", "containers", "restore_images", "observation_sha256", "recheck_equal"}
    if (type(value) is not dict or set(value) != expected or role not in ("server_a", "server_d") or
            value["kind"] != "readonly_qs_service_descriptor_observation" or value["host_role"] != role.replace("_", "-") or
            value["docker_path"] != "/usr/bin/docker" or value["recheck_equal"] is not True or
            any(type(value[k]) is not str or not SHA.fullmatch(value[k]) for k in ("machine_id_sha256", "docker_sha256", "observation_sha256")) or
            type(value["containers"]) is not list or not 1 <= len(value["containers"]) <= 32):
        reject("docker_projection_schema_unknown")
    images = value["restore_images"]
    if role == "server_d":
        if images is not None: reject("docker_projection_schema_unknown")
    else:
        if type(images) is not dict or set(images) != {"status", "images", "missing"} or type(images["images"]) is not list or type(images["missing"]) is not list:
            reject("docker_projection_schema_unknown")
        refs = []
        for row in images["images"]:
            if (type(row) is not dict or set(row) != {"reference", "id", "os", "architecture"} or row["reference"] not in QS_RESTORE_IMAGES or type(row["id"]) is not str or not re.fullmatch(r"sha256:[0-9a-f]{64}", row["id"]) or row["os"] != "linux" or row["architecture"] not in ("amd64", "arm64")):
                reject("docker_projection_schema_unknown")
            refs.append(row["reference"])
        if (refs != [v for v in QS_RESTORE_IMAGES if v not in images["missing"]] or images["missing"] != [v for v in QS_RESTORE_IMAGES if v not in refs] or images["status"] != ("cache_missing" if images["missing"] else "cached") or len({r["architecture"] for r in images["images"]}) > 1 or len({r["id"] for r in images["images"]}) != len(refs)):
            reject("docker_projection_schema_unknown")
    expected_components = set(QS_COMPONENTS[:2] if role == "server_a" else QS_COMPONENTS[2:])
    keys = {"id", "name", "image", "component", "project", "service", "entrypoint", "command", "running", "started_at", "restart_policy", "restart_maximum"}
    seen = set()
    for v in value["containers"]:
        if type(v) is not dict or set(v) != keys or type(v["id"]) is not str or not SHA.fullmatch(v["id"]) or v["id"] in seen or type(v["component"]) is not str or v["component"] not in expected_components:
            reject("docker_projection_schema_unknown")
        seen.add(v["id"])
        binaries = {"qs-apiserver": "qs-apiserver", "qs-collection-server": "collection-server", "qs-worker": "qs-worker"}
        configs = {"qs-apiserver": "apiserver", "qs-collection-server": "collection-server", "qs-worker": "worker"}
        if (type(v["name"]) is not str or type(v["image"]) is not str or not re.fullmatch(r"/[A-Za-z0-9][A-Za-z0-9_.-]{0,127}", v["name"]) or not re.fullmatch(r"sha256:[0-9a-f]{64}", v["image"]) or
                any(type(v[k]) is not str or not re.fullmatch(r"[A-Za-z0-9_.-]{0,128}", v[k]) for k in ("project", "service")) or
                v["entrypoint"] != ["/app/" + binaries[v["component"]]] or v["command"] != ["--config=/app/configs/" + configs[v["component"]] + ".prod.yaml"] or
                type(v["running"]) is not bool or type(v["started_at"]) is not str or not re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+Z", v["started_at"]) or
                v["restart_policy"] != "unless-stopped" or type(v["restart_maximum"]) is not int or v["restart_maximum"] != 0):
            reject("docker_projection_schema_unknown")
        if (v["component"] == "qs-apiserver" and (v["name"] != "/qs-apiserver" or v["service"] != "qs-apiserver") or
                v["component"] == "qs-collection-server" and (v["project"] != "qs-collection" or v["service"] != "server") or
                v["component"] == "qs-worker" and (v["project"] != "qs-worker" or v["service"] != "runtime")):
            reject("docker_projection_schema_unknown")
    if {v["component"] for v in value["containers"]} != expected_components or sum(v["component"] == "qs-apiserver" for v in value["containers"]) > 1:
        reject("docker_projection_schema_unknown")
    base = {k:v for k,v in value.items() if k not in ("observation_sha256", "recheck_equal")}
    if sha(canonical(base)) != value["observation_sha256"]: reject("docker_projection_schema_unknown")
    return value


COMMANDS = {"sudo_list": ("/usr/bin/sudo", ("-n", "-l")),
            "docker_list": ("/usr/bin/docker", ("ps", "-aq", "--no-trunc")),
            "sessions": ("/usr/bin/loginctl", ("list-sessions", "--no-legend", "--no-pager")),
            "units": ("/usr/bin/systemctl", ("list-units", "--all", "--type=service", "--type=timer", "--no-legend", "--plain", "--no-pager")),
            "unit_files": ("/usr/bin/systemctl", ("list-unit-files", "--type=service", "--type=timer", "--no-legend", "--no-pager")),
            "timers": ("/usr/bin/systemctl", ("list-timers", "--all", "--no-legend", "--no-pager"))}


def _argv(kind, arg=None):
    if kind in COMMANDS and arg is None:
        exe, tail = COMMANDS[kind]
        return [exe, *tail]
    if kind == "sshd_global" and type(arg) is str and arg.startswith("/etc/ssh/") and os.path.normpath(arg) == arg:
        return ["/usr/sbin/sshd", "-T", "-f", arg]
    if kind == "sshd" and type(arg) is tuple and len(arg) == 2:
        path, match = arg
        if (type(path) is str and path.startswith("/etc/ssh/") and os.path.normpath(path) == path
                and type(match) is dict and set(match) == {"user", "host", "addr", "laddr", "lport"}
                and all(type(v) is str and re.fullmatch(r"[A-Za-z0-9_.:-]{1,128}", v) for v in match.values())):
            return ["/usr/sbin/sshd", "-T", "-f", path, "-C", ",".join(k + "=" + match[k] for k in ("user", "host", "addr", "laddr", "lport"))]
    if kind == "docker_inspect" and type(arg) is str and re.fullmatch(r"[0-9a-f]{64}", arg):
        return ["/usr/bin/docker", "inspect", "--format", DOCKER_FORMAT, arg]
    if kind == "qs_restore_image_inspect" and arg in QS_RESTORE_IMAGES:
        return ["/usr/bin/docker", "image", "inspect", "--format", QS_RESTORE_IMAGE_FORMAT, arg]
    if kind == "qs_restore_image_present" and arg in QS_RESTORE_IMAGES:
        return ["/usr/bin/docker", "image", "ls", "--no-trunc", "--quiet", arg]
    if kind == "qs_service_inspect" and type(arg) is str and SHA.fullmatch(arg):
        return ["/usr/bin/docker", "inspect", "--format", QS_SERVICE_FORMAT, arg]
    if kind == "unit" and type(arg) is str and UNIT.fullmatch(arg):
        return ["/usr/bin/systemctl", "show", "--no-pager", "--property=Id,ActiveState,SubState,MainPID,FragmentPath,DropInPaths,User,Group,WorkingDirectory", arg]
    if kind == "session" and type(arg) is str and SESSION.fullmatch(arg):
        return ["/usr/bin/loginctl", "show-session", "--no-pager", "--property=Id,User,Leader,Remote,Type,State,Active,Service", arg]
    reject("command_not_in_closed_read_set")


def _capture(argv, timeout):
    try:
        p = subprocess.Popen(argv, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                             env={"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LANG": "C", "LC_ALL": "C"}, start_new_session=True)
    except (OSError, ValueError):
        reject("command_unavailable")
    poll = selectors.DefaultSelector()
    buffers = {p.stdout: [], p.stderr: []}
    count, end, done = 0, time.monotonic() + timeout, False
    try:
        for pipe in buffers:
            os.set_blocking(pipe.fileno(), False)
            poll.register(pipe, selectors.EVENT_READ)
        while poll.get_map():
            left = end - time.monotonic()
            if left <= 0:
                reject("command_timeout")
            for key, unused in poll.select(min(left, .1)):
                raw = os.read(key.fileobj.fileno(), 65536)
                if not raw:
                    poll.unregister(key.fileobj)
                    continue
                count += len(raw)
                if count > OUTPUT_CAP:
                    reject("command_output_budget_exceeded")
                buffers[key.fileobj].append(raw)
        left = end - time.monotonic()
        if left <= 0:
            reject("command_timeout")
        try:
            code = p.wait(timeout=left)
        except subprocess.TimeoutExpired:
            reject("command_timeout")
        done = True
        if code or buffers[p.stderr]:
            reject("command_denied_or_failed")
        return b"".join(buffers[p.stdout])
    finally:
        if not done:
            try:
                os.killpg(p.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            p.wait()
        poll.close()
        p.stdout.close()
        p.stderr.close()


class _Runner:
    def __init__(self, files):
        self.files, self.end, self.calls = files, time.monotonic() + TOTAL_SECONDS, []
        self.executables = {}

    def run(self, kind, arg=None):
        argv = _argv(kind, arg)
        # An untrusted PATH or a deploy-user replacement executable cannot
        # turn this observer into arbitrary execution. No sudo prefix exists.
        meta = self.files.metadata(argv[0])
        if meta["is_link"] or meta["uid"] != 0 or meta["mode"] & 0o022:
            reject("command_executable_unprotected")
        if argv[0] not in self.executables:
            unused, proof = self.files.read(argv[0], binary=True)
            if not proof["ancestor_root_mode_protected"]:
                reject("command_executable_unprotected")
            self.executables[argv[0]] = (proof["stat_sha256"], proof["raw_sha256"])
        if meta["stat_sha256"] != self.executables[argv[0]][0]:
            reject("command_executable_changed")
        left = self.end - time.monotonic()
        if left <= 0:
            reject("inventory_deadline_exceeded")
        raw = _capture(argv, min(QUERY_SECONDS, left))
        self.calls.append({"kind": kind, "argv_sha256": sha(canonical(argv)), "raw_sha256": sha(raw), "bytes": len(raw),
                           "executable_sha256": self.executables[argv[0]][1]})
        if self.files.metadata(argv[0])["stat_sha256"] != self.executables[argv[0]][0]:
            reject("command_executable_changed")
        return raw


class _Observer:
    def __init__(self, files, runner):
        self.files, self.runner, self.unknown = files, runner, set(ALWAYS_UNKNOWN)
        self.files_seen, self.includes, self.account_rows = [], [], {}
        self.started = time.monotonic()

    def guard(self):
        if time.monotonic() - self.started > TOTAL_SECONDS:
            reject("inventory_deadline_exceeded")

    def attempt(self, category, fn):
        self.guard()
        try:
            return fn()
        except Unknown as e:
            self.unknown.add(category + ":" + str(e))
            return None
        except (ValueError, UnicodeError, TypeError, KeyError, IndexError):
            self.unknown.add(category + ":observation_schema_unknown")
            return None

    def read(self, path, proc=False):
        self.guard()
        raw, proof = self.files.read(path, proc)
        self.files_seen.append(proof)
        return raw

    def accounts(self):
        raw = self.read("/etc/passwd")
        out = []
        for line in raw.decode("utf-8").splitlines():
            parts = line.split(":")
            if len(parts) != 7 or not parts[2].isdigit() or not parts[3].isdigit():
                reject("account_schema_unknown")
            # Password/gecos values are neither retained nor emitted.
            name, uid, gid, home, shell = parts[0], int(parts[2]), int(parts[3]), parts[5], parts[6]
            self.account_rows[name] = (uid, gid, home, shell)
            out.append({"subject_sha256": sha(name.encode()), "uid": uid, "gid": gid,
                        "home_sha256": sha(home.encode()), "shell_sha256": sha(shell.encode()),
                        "interactive_shell_candidate": shell not in ("/usr/sbin/nologin", "/sbin/nologin", "/bin/false")})
        groups = self.read("/etc/group")
        return {"local_subjects": out, "passwd_raw_sha256": sha(raw), "group_raw_sha256": sha(groups), "nss_complete": False}

    def processes(self):
        names = [x for x in self.files.list("/proc") if re.fullmatch(r"[1-9][0-9]*", x)]
        if len(names) > PROCESS_CAP:
            reject("process_budget_exceeded")
        rows, daemons = [], []
        for name in names:
            def one():
                raw = self.read("/proc/" + name + "/stat", True)
                text = raw.decode("utf-8")
                tail = text.rsplit(")", 1)[1].split()
                start = tail[19]
                status = self.read("/proc/" + name + "/status", True)
                uid = re.search(rb"^Uid:\s+(\d+)\s+(\d+)", status, re.M)
                if not start.isdigit() or not uid:
                    reject("process_schema_unknown")
                exe = self.files.link("/proc/" + name + "/exe")
                if not tail[1].isdigit():
                    reject("process_schema_unknown")
                row = {"pid": int(name), "ppid": int(tail[1]), "start_ticks": start, "uid": int(uid[1]), "euid": int(uid[2]),
                       "exe_sha256": sha(exe.encode()), "cgroup_sha256": sha(self.read("/proc/" + name + "/cgroup", True))}
                if exe == "/usr/sbin/sshd":
                    row["sshd"] = True
                    command = self.read("/proc/" + name + "/cmdline", True)
                    row["sshd_original_argv_sha256"] = sha(command)
                    args = command.split(b"\x00")
                    daemons.append(args)
                return row
            row = self.attempt("process_visibility", one)
            if row is not None:
                rows.append(row)
        return {"rows": rows, "set_sha256": sha(canonical(rows)), "observed": len(rows)}, daemons

    def config(self, path, stack=()):
        if not path.startswith("/etc/ssh/") or os.path.normpath(path) != path:
            reject("ssh_include_path_unsupported")
        if path in stack or len(stack) >= 16:
            reject("ssh_include_cycle_or_depth_unknown")
        raw = self.read(path)
        self.includes.append({"path_sha256": sha(path.encode()), "raw_sha256": sha(raw), "sequence": len(self.includes)})
        for line in raw.decode("utf-8").splitlines():
            try:
                tokens = shlex.split(line, comments=True)
            except ValueError:
                reject("ssh_config_syntax_unknown")
            if not tokens or tokens[0].lower() != "include":
                continue
            for pattern in tokens[1:]:
                if not pattern.startswith("/"):
                    pattern = "/etc/ssh/" + pattern
                parent, name = os.path.split(pattern)
                if glob.has_magic(parent) or os.path.normpath(pattern) != pattern or not pattern.startswith("/etc/ssh/"):
                    reject("ssh_include_path_unsupported")
                entries = self.files.list(parent)
                expanded = sorted(x for x in entries if fnmatch.fnmatchcase(x, name))
                self.includes.append({"pattern_sha256": sha(pattern.encode()), "directory_listing_sha256": sha(canonical(entries)), "matched_files": len(expanded)})
                for entry in expanded:
                    self.config(parent + "/" + entry, (*stack, path))
                if self.files.list(parent) != entries:
                    reject("ssh_include_directory_changed")

    def session_context(self, session, processes, pid):
        """Only an actual sshd ancestor and protected config can supply UseDNS.

        OpenSSH sshd.8 defines -C host as resolved source hostname; auth.c
        returns the numeric peer address only when UseDNS is false. A fresh
        -T observation cannot prove the daemon's originally loaded bytes.
        This remaining gap is explicit and this method confers no authority.
        https://man.openbsd.org/sshd.8
        https://github.com/openssh/openssh-portable/blob/master/auth.c
        """
        fields = validate_session(session)
        empty = {"matches": [], "daemon": None, "usedns": "unknown", "host_status": "host_unobserved", "binding_sha256": None}
        self.unknown.update(("session_environment_origin_unproven", "ssh_daemon_loaded_configuration_unproven"))
        rows = {r["pid"]: r for r in processes.get("rows", [])}
        visited, row = set(), rows.get(pid)
        if row is None or row["uid"] != session["uid"] or row["euid"] != session["euid"]:
            self.unknown.update(("ssh_session_daemon_binding_unknown", "ssh_session_host_unobserved"))
            return empty
        while row is not None and row["pid"] not in visited and len(visited) < 64:
            visited.add(row["pid"])
            if row.get("sshd"):
                raw = self.read("/proc/" + str(row["pid"]) + "/cmdline", True)
                args = raw.split(b"\x00")
                if args and args[0] == b"/usr/sbin/sshd":
                    if row["uid"] != 0 or row["euid"] != 0 or sha(raw) != row["sshd_original_argv_sha256"]:
                        self.unknown.add("ssh_session_daemon_binding_unknown")
                        return empty
                    path, index, seen_f = "/etc/ssh/sshd_config", 1, False
                    args = [x.decode("utf-8") for x in args if x]
                    while index < len(args):
                        if args[index] in ("-D", "-e", "-q", "-4", "-6"):
                            index += 1
                        elif args[index] == "-f" and index + 1 < len(args) and not seen_f:
                            path, index, seen_f = args[index + 1], index + 2, True
                        else:
                            self.unknown.add("ssh_session_daemon_binding_unknown")
                            return empty
                    if not path.startswith("/etc/ssh/") or os.path.normpath(path) != path:
                        self.unknown.add("ssh_session_daemon_binding_unknown")
                        return empty
                    before = len(self.files_seen)
                    self.includes = []
                    self.config(path)
                    proofs = self.files_seen[before:]
                    if not proofs or any(not r["root_mode_protected"] or not r["ancestor_root_mode_protected"] for r in proofs):
                        self.unknown.add("ssh_session_daemon_binding_unknown")
                        return empty
                    output = self.runner.run("sshd_global", path)
                    settings = {}
                    for line in output.decode("utf-8").splitlines():
                        key, value = line.split(" ", 1)
                        if key in settings:
                            reject("ssh_effective_duplicate_key")
                        settings[key] = value
                    usedns = settings.get("usedns", "unknown")
                    if usedns not in ("yes", "no"):
                        self.unknown.add("ssh_session_host_unobserved")
                        return empty
                    binding = sha(canonical({"pid": row["pid"], "start": row["start_ticks"], "argv": sha(raw),
                                             "config": self.includes, "effective": sha(output)}))
                    result = dict(empty, daemon=raw.split(b"\x00"), usedns=usedns, binding_sha256=binding)
                    if usedns == "no":
                        result["matches"] = [{"user": session["username"], "host": fields[0], "addr": fields[0], "laddr": fields[2], "lport": fields[3]}]
                        result["host_status"] = "numeric_peer_from_usedns_no"
                    else:
                        self.unknown.add("ssh_session_host_unobserved")
                    return result
            row = rows.get(row["ppid"])
        self.unknown.add("ssh_session_daemon_binding_unknown")
        self.unknown.add("ssh_session_host_unobserved")
        return empty

    def ssh(self, matches, daemons):
        self.includes = []
        configs = set()
        for raw in daemons:
            if not raw or raw[0] != b"/usr/sbin/sshd":
                self.unknown.add("ssh_daemon_original_argv_unknown")
                continue
            args = [x.decode("utf-8") for x in raw[1:] if x]
            path, i = "/etc/ssh/sshd_config", 0
            while i < len(args):
                if args[i] in ("-D", "-e", "-q", "-4", "-6"):
                    i += 1
                elif args[i] == "-f" and i + 1 < len(args):
                    path, i = args[i + 1], i + 2
                else:
                    self.unknown.add("ssh_daemon_cli_override_unproven")
                    break
            configs.add(path)
        if not configs:
            self.unknown.add("ssh_actual_daemon_config_unknown")
            configs.add("/etc/ssh/sshd_config")
        results = []
        for path in sorted(configs):
            self.attempt("ssh_config_visibility", lambda: self.config(path))
            for match in matches:
                def one():
                    raw = self.runner.run("sshd", (path, match))
                    settings = {}
                    for line in raw.decode("utf-8").splitlines():
                        key, value = line.split(" ", 1)
                        if key in settings:
                            reject("ssh_effective_duplicate_key")
                        settings[key] = value
                    if any(k not in settings for k in SSH_PROPERTIES[:18]):
                        reject("ssh_effective_projection_incomplete")
                    projection = {k: {"sha256": sha(settings[k].encode()), "state": settings[k] if settings[k] in ("yes", "no", "none") else "other"}
                                  for k in SSH_PROPERTIES if k in settings}
                    result = {"context_sha256": sha(canonical(match)), "raw_sha256": sha(raw), "projection": projection, "key_files": []}
                    if settings["authorizedkeyscommand"] != "none" or settings["authorizedprincipalscommand"] != "none":
                        self.unknown.add("dynamic_ssh_key_or_principal_provider_unknown")
                    account = self.account_rows.get(match["user"])
                    if account is None:
                        self.unknown.add("ssh_context_subject_not_in_local_accounts")
                        return result
                    uid, gid, home, unused = account
                    paths = shlex.split(settings["authorizedkeysfile"])
                    for key_path in paths:
                        if key_path == "none":
                            continue
                        key_path = key_path.replace("%%", "\x00").replace("%h", home).replace("%u", match["user"]).replace("%U", str(uid)).replace("\x00", "%")
                        if "%" in key_path or glob.has_magic(key_path):
                            self.unknown.add("ssh_key_path_expansion_unsupported")
                            continue
                        if not key_path.startswith("/"):
                            key_path = home.rstrip("/") + "/" + key_path
                        if os.path.basename(key_path) not in ("authorized_keys", "authorized_keys2"):
                            self.unknown.add("ssh_public_key_file_name_unsupported")
                            continue
                        def key_file():
                            content = self.read(key_path)
                            keys = []
                            for line in content.decode("utf-8").splitlines():
                                if not line.strip() or line.lstrip().startswith("#"):
                                    continue
                                tokens = shlex.split(line)
                                idx = next((i for i in range(min(len(tokens), 2)) if tokens[i].startswith(("ssh-", "ecdsa-", "sk-"))), None)
                                if idx is None or idx + 1 >= len(tokens):
                                    reject("ssh_authorized_key_schema_unknown")
                                wire = base64.b64decode(tokens[idx + 1], validate=True)
                                _public_wire(wire, tokens[idx])
                                keys.append({"wire_sha256": sha(wire), "options_sha256": sha(canonical(tokens[:idx])),
                                             "certificate": tokens[idx].endswith("-cert-v01@openssh.com")})
                            return {"path_sha256": sha(key_path.encode()), "raw_sha256": sha(content), "keys": keys}
                        key_result = self.attempt("ssh_authorized_key_visibility", key_file)
                        if key_result is not None:
                            result["key_files"].append(key_result)
                    for property_name in ("trustedusercakeys", "authorizedprincipalsfile"):
                        if settings.get(property_name, "none") != "none":
                            self.unknown.add("additional_ssh_ca_or_principal_source_unproven")
                    return result
                self.attempt("ssh_effective_visibility", lambda: results.append(one()))
        return {"include_records": list(self.includes), "matches": results, "all_match_contexts_complete": False}

    def docker(self):
        ids = self.runner.run("docker_list").decode("ascii").splitlines()
        if len(ids) > 256 or len(set(ids)) != len(ids) or any(not re.fullmatch(r"[0-9a-f]{64}", x) for x in ids):
            reject("docker_roster_schema_or_budget_unknown")
        expected = {"id", "name", "component", "image", "status", "running", "started", "restarts", "privileged", "readonly", "user", "project", "service", "mounts"}
        rows, initial, candidates = [], {}, {}
        for cid in sorted(ids):
            raw = self.runner.run("docker_inspect", cid)
            v = json.loads(raw)
            if set(v) != expected or v["id"] != cid or not re.fullmatch(r"sha256:[0-9a-f]{64}", v["image"]) or type(v["mounts"]) is not list:
                reject("docker_projection_schema_unknown")
            if (any(type(v[k]) is not bool for k in ("running", "privileged", "readonly"))
                    or type(v["restarts"]) is not int or v["restarts"] < 0
                    or any(v[k] is not None and type(v[k]) is not str for k in ("user", "project", "service", "component"))
                    or type(v["name"]) is not str
                    or type(v["status"]) is not str or type(v["started"]) is not str):
                reject("docker_projection_schema_unknown")
            initial[cid] = sha(raw)
            if v["component"] in QS_COMPONENTS or v["name"] in ("/qs-apiserver", "/qs-worker", "/qs-collection-server") or v["project"] in ("qs-collection", "qs-worker"):
                candidates[cid] = {k: v[k] for k in ("name", "component", "project", "service")}

            mounts = []
            for m in v["mounts"]:
                if type(m) is not dict or set(m) != {"source", "target", "rw", "type"} or type(m["rw"]) is not bool:
                    reject("docker_mount_schema_unknown")
                if any(type(m[k]) is not str for k in ("source", "target", "type")):
                    reject("docker_mount_schema_unknown")
                mounts.append({"source_sha256": sha(m["source"].encode()), "target_sha256": sha(m["target"].encode()), "rw": m["rw"], "type_sha256": sha(m["type"].encode())})
                if m["rw"] and m["source"].startswith("/"):
                    self.attempt("container_writable_path_visibility", lambda: self.files_seen.append(self.files.metadata(m["source"])))
            rows.append({"id": cid, "image_config_digest": v["image"], "inspect_sha256": sha(raw),
                         "status_sha256": sha(str(v["status"]).encode()), "running": v["running"], "privileged": v["privileged"],
                         "readonly_rootfs": v["readonly"], "mounts": mounts,
                         "project_sha256": sha(canonical(v["project"])), "service_sha256": sha(canonical(v["service"]))})
        return {"containers": rows, "before_ids": sorted(ids), "before_inspects": initial, "qs_candidates": candidates}

    def qs_services(self, role, docker):
        # The same finite relevance rule as native stop.isRelevant: names and
        # compose projects also expose missing/mismatched component labels.
        if role not in ("server_a", "server_d") or docker is None or not docker["qs_candidates"] or len(docker["qs_candidates"]) > 32:
            reject("docker_projection_schema_unknown")
        expected_components = set(QS_COMPONENTS[:2] if role == "server_a" else QS_COMPONENTS[2:])
        keys = {"id", "name", "image", "component", "project", "service", "entrypoint", "command", "running", "started_at", "restart_policy", "restart_maximum"}
        def read_rows():
            rows = []
            for cid, selector in sorted(docker["qs_candidates"].items()):
                v = json.loads(self.runner.run("qs_service_inspect", cid))
                if type(v) is not dict or set(v) != keys or v["id"] != cid:
                    reject("docker_projection_schema_unknown")
                for key in ("component", "project", "service"):
                    if v[key] is None: v[key] = ""
                if ({k: (selector[k] or "") for k in ("component", "project", "service")} != {k: v[k] for k in ("component", "project", "service")} or v["name"] != selector["name"] or v["component"] not in expected_components):
                    reject("docker_projection_schema_unknown")
                rows.append(v)
            if {v["component"] for v in rows} != expected_components or sum(v["component"] == "qs-apiserver" for v in rows) > 1:
                reject("docker_projection_schema_unknown")
            return rows
        def restore_images():
            rows, missing = [], []
            for reference in QS_RESTORE_IMAGES:
                present = self.runner.run("qs_restore_image_present", reference).decode("ascii").strip()
                if not present:
                    missing.append(reference) # Successful exact native image ls, not an inspect permission failure.
                    continue
                if not re.fullmatch(r"sha256:[0-9a-f]{64}", present): reject("docker_projection_schema_unknown")
                row = json.loads(self.runner.run("qs_restore_image_inspect", reference))
                if type(row) is not dict or set(row) != {"id", "os", "architecture"} or row["id"] != present:
                    reject("docker_projection_schema_unknown")
                rows.append(dict(reference=reference, **row))
            return {"status": "cache_missing" if missing else "cached", "images": rows, "missing": missing}
        machine = self.read("/etc/machine-id").strip()
        if not re.fullmatch(rb"[0-9a-f]{32}", machine): reject("docker_projection_schema_unknown")
        before = read_rows()
        restored = restore_images() if role == "server_a" else None
        ids = sorted(self.runner.run("docker_list").decode("ascii").splitlines())
        if ids != docker["before_ids"] or read_rows() != before or (restore_images() if role == "server_a" else None) != restored or self.read("/etc/machine-id").strip() != machine:
            reject("command_executable_changed")
        executable = [v["executable_sha256"] for v in self.runner.calls if v["kind"] == "qs_service_inspect"]
        if not executable or len(set(executable)) != 1 or not SHA.fullmatch(executable[0]): reject("command_executable_unprotected")
        actual = {"kind": "readonly_qs_service_descriptor_observation", "host_role": role.replace("_", "-"),
                  "machine_id_sha256": sha(machine), "docker_path": "/usr/bin/docker", "docker_sha256": executable[0], "containers": before, "restore_images": restored}
        return validate_qs_service_observation(dict(actual, observation_sha256=sha(canonical(actual)), recheck_equal=True), role)

    def systemd(self):
        sources = {k: self.runner.run(k) for k in ("units", "unit_files", "timers")}
        units = set()
        for kind in ("units", "unit_files"):
            for line in sources[kind].decode("utf-8").splitlines():
                tokens = line.split()
                if not tokens or not UNIT.fullmatch(tokens[0]):
                    reject("systemd_listing_schema_unknown")
                units.add(tokens[0])
        if len(units) > 256:
            reject("systemd_unit_budget_exceeded")
        rows = []
        for unit in sorted(units):
            raw = self.runner.run("unit", unit)
            v = _properties(raw, {"Id", "ActiveState", "SubState", "MainPID", "FragmentPath", "DropInPaths", "User", "Group", "WorkingDirectory"})
            rows.append({"unit_sha256": sha(unit.encode()), "raw_sha256": sha(raw), "property_sha256": sha(canonical(v))})
            working = v.get("WorkingDirectory", "")
            if working.startswith("/"):
                self.attempt("systemd_writable_path_visibility", lambda: self.files_seen.append(self.files.metadata(working)))
        self.unknown.add("systemd_execution_and_secret_environment_not_read")
        return {"rows": rows, "listing_hashes": {k: sha(v) for k, v in sources.items()}}

    def sessions(self):
        raw = self.runner.run("sessions")
        ids = [line.split()[0] for line in raw.decode("utf-8").splitlines() if line.strip()]
        if len(ids) > 256 or len(set(ids)) != len(ids) or any(not SESSION.fullmatch(x) for x in ids):
            reject("session_listing_schema_unknown")
        rows = [{"session_sha256": sha(x.encode()), "properties_sha256": sha(canonical(_properties(self.runner.run("session", x), {"Id", "User", "Leader", "Remote", "Type", "State", "Active", "Service"})))} for x in sorted(ids)]
        return {"listing_sha256": sha(raw), "rows": rows}

    def cron(self):
        roots = ("/etc/cron.d", "/etc/cron.hourly", "/etc/cron.daily", "/etc/cron.weekly", "/etc/cron.monthly",
                 "/var/spool/cron", "/var/spool/cron/crontabs")
        out = []
        for path in ("/etc/crontab",):
            self.attempt("cron_visibility", lambda: out.append(self.files.metadata(path)))
        for path in roots:
            def one():
                entries = self.files.list(path)
                if len(entries) > 256:
                    reject("cron_directory_budget_exceeded")
                for name in entries:
                    out.append(self.files.metadata(path + "/" + name))
                return {"directory_sha256": sha(path.encode()), "listing_sha256": sha(canonical(entries))}
            result = self.attempt("cron_visibility", one)
            if result:
                out.append(result)
        # Crontabs may embed credentials in environment assignments. Do not
        # read their content or claim a writer-free schedule from file metadata.
        self.unknown.add("cron_contents_and_indirect_scripts_not_read")
        return out


def _properties(raw, expected):
    out = {}
    for line in raw.decode("utf-8").splitlines():
        key, value = line.split("=", 1)
        if key in out or not re.fullmatch(r"[A-Za-z]+", key):
            reject("property_projection_schema_unknown")
        out[key] = value
    if set(out) != expected:
        reject("property_projection_schema_unknown")
    return out


def _public_wire(raw, key_type):
    cursor = 0
    def string():
        nonlocal cursor
        if cursor + 4 > len(raw):
            reject("ssh_authorized_key_schema_unknown")
        size = int.from_bytes(raw[cursor:cursor + 4], "big")
        cursor += 4
        if size > 16384 or cursor + size > len(raw):
            reject("ssh_authorized_key_schema_unknown")
        result = raw[cursor:cursor + size]
        cursor += size
        return result
    if string() != key_type.encode():
        reject("ssh_authorized_key_schema_unknown")
    if key_type == "ssh-ed25519":
        if len(string()) != 32:
            reject("ssh_authorized_key_schema_unknown")
    elif key_type == "ssh-rsa":
        for unused in range(2):
            value = string()
            if not value or value[0] & 0x80 or value == b"\x00" or (len(value) > 1 and value[0] == 0 and not value[1] & 0x80):
                reject("ssh_authorized_key_schema_unknown")
    elif key_type == "ecdsa-sha2-nistp256":
        curve, point = string(), string()
        if curve != b"nistp256" or len(point) != 65 or point[0] != 4:
            reject("ssh_authorized_key_schema_unknown")
    else:
        # Certificates/security-key/vendor types require their real parser;
        # header-only bytes must never count as an accepted public key.
        reject("ssh_authorized_key_schema_unknown")
    if cursor != len(raw):
        reject("ssh_authorized_key_schema_unknown")


def _collect(v, approved, files, runner, identity):
    o = _Observer(files, runner)
    accounts = o.attempt("accounts_visibility", o.accounts)
    initial = o.attempt("process_visibility", o.processes)
    processes, daemons = initial if initial is not None else ({"observed": 0, "set_sha256": ""}, [])
    observation = v["protocol"] == OBSERVATION_PROTOCOL
    context = None
    if observation:
        context = o.attempt("session_context_visibility", lambda: o.session_context(v["session"], processes, identity["pid"]))
        if context is None:
            context = {"matches": [], "daemon": None, "usedns": "unknown", "host_status": "host_unobserved", "binding_sha256": None}
            o.unknown.update(("session_environment_origin_unproven", "ssh_daemon_loaded_configuration_unproven", "ssh_session_host_unobserved"))
        matches = context["matches"]
        ssh_daemons = [context["daemon"]] if context["daemon"] is not None else daemons
    else:
        matches, ssh_daemons = v["matches"], daemons
    ssh = o.ssh(matches, ssh_daemons)
    docker = o.attempt("docker_visibility", o.docker)
    systemd = o.attempt("systemd_visibility", o.systemd)
    sessions = o.attempt("sessions_visibility", o.sessions)
    cron = o.cron()
    sudo = o.attempt("sudo_list_visibility", lambda: {"raw_sha256": sha(runner.run("sudo_list")), "actual_command_denial_proven": False})
    o.attempt("docker_socket_visibility", lambda: o.files_seen.append(files.metadata("/var/run/docker.sock")))
    boot = o.attempt("host_identity_visibility", lambda: sha(o.read("/proc/sys/kernel/random/boot_id", True)))
    namespaces = {}
    for kind in ("mnt", "pid", "user"):
        value = o.attempt("namespace_visibility", lambda: files.link("/proc/" + str(identity["pid"]) + "/ns/" + kind))
        if value is not None:
            namespaces[kind] = sha(value.encode())
    services = o.attempt("qs_service_visibility", lambda: o.qs_services(v["host_role"], docker)) if v["host_role"] in ("server_a", "server_d") else None
    checks = []
    def end_check(name, fn):
        result = o.attempt(name, fn)
        checks.append({"kind": name, "unchanged": result is True})
        if result is not True:
            o.unknown.add(name + ":end_recheck_unproven")
    end_check("process_end_recheck", lambda: o.processes()[0]["set_sha256"] == processes["set_sha256"])
    if docker is not None:
        def docker_end():
            ids = sorted(runner.run("docker_list").decode("ascii").splitlines())
            return ids == docker["before_ids"] and all(sha(runner.run("docker_inspect", cid)) == digest for cid, digest in docker["before_inspects"].items())
        end_check("docker_end_recheck", docker_end)
        docker = {"containers": docker["containers"]}
    if sessions is not None:
        end_check("sessions_end_recheck", lambda: o.sessions() == sessions)
    if systemd is not None:
        end_check("systemd_end_recheck", lambda: o.systemd() == systemd)
    # Re-run the real effective-config query after its files and daemon roster
    # have been independently re-read. This is still a point observation.
    def ssh_end():
        if observation:
            current = o.session_context(v["session"], o.processes()[0], identity["pid"])
            if current != context:
                return False
        return o.ssh(matches, ssh_daemons) == ssh
    end_check("ssh_end_recheck", ssh_end)
    end_check("boot_end_recheck", lambda: sha(o.read("/proc/sys/kernel/random/boot_id", True)) == boot)
    end_check("files_end_recheck", lambda: (files.recheck() is None))
    result = {"protocol": v["protocol"], "audited_source_sha": AUDITED_SOURCE, "requested_source_sha": v["source_sha"],
            "operation_id": v["operation_id"], "run_id": v["run_id"], "request_sha256": approved,
            "host_role": v["host_role"], "identity": {"uid": identity["uid"], "euid": identity["euid"],
                "groups_sha256": sha(canonical(identity["groups"])), "os_sha256": sha(canonical(identity.get("os", {}))),
                "boot_id_sha256": boot, "namespaces": namespaces},
            "observations": {"accounts": accounts, "processes": processes, "ssh": ssh, "docker": docker,
                             "systemd": systemd, "sessions": sessions, "cron": cron, "sudo_list": sudo, "files": o.files_seen, "qs_services": services},
            "end_rechecks": checks, "unknown": sorted(o.unknown), "read_only_command_receipts": runner.calls,
            "observed_budgets": {"read_calls": len(runner.calls), "content_bytes": files.bytes, "binary_hash_bytes": files.binary_bytes,
                                 "distinct_content_files": len(files.witnesses), "elapsed_seconds": round(time.monotonic() - o.started, 6)},
            "required_readonly_host_visibility": ["effective_sshd_config_and_protected_include_key_metadata", "root_cron_and_other_subject_sessions",
                                                 "full_proc_visibility_and_acl_metadata", "docker_socket_and_selected_inspect"],
            "root_python_or_permission_changes_requested": False, "raw_credentials_or_configuration_output": False,
            "capabilities": {k: False for k in CAPABILITIES}}
    if observation:
        result["context_mode"] = CONTEXT_MODE
        result["session_observation"] = {"identity_sha256": v["session"]["identity_sha256"], "connection_sha256": v["session"]["connection_sha256"],
            "request_session_sha256": sha(canonical(v["session"])), "uid": v["session"]["uid"], "euid": v["session"]["euid"],
            "origin_proven": False, "partial": True, "host_status": context["host_status"], "usedns": context["usedns"],
            "daemon_binding_sha256": context["binding_sha256"], "derived_matches_sha256": sha(canonical(matches)), "match_context_count": len(matches),
            "identity_connection_rechecked": False}
    return result


def inventory(raw, approved):
    v = request(raw, approved)
    if sys.platform != "linux":
        reject("linux_host_required")
    observed = observe_current_session() if v["protocol"] == OBSERVATION_PROTOCOL else None
    if observed is not None and observed != v["session"]:
        reject("inventory_request_rejected")
    files = _Files()
    result = _collect(v, approved, files, _Runner(files), {"uid": os.getuid(), "euid": os.geteuid(), "groups": sorted(os.getgroups()),
                     "pid": os.getpid(), "os": list(os.uname())})
    if observed is not None and observe_current_session() != observed:
        reject("inventory_request_rejected")
    if observed is not None:
        result["session_observation"]["identity_connection_rechecked"] = True
    return result


def main():
    class Parser(argparse.ArgumentParser):
        def error(self, unused):
            reject("inventory_input_rejected")
    error_protocol = PROTOCOL
    try:
        p = Parser(description=__doc__)
        p.add_argument("--request-file", required=True)
        p.add_argument("--request-sha256", required=True)
        args = p.parse_args()
        input_files = _Files()
        raw, unused = input_files.read(args.request_file)
        source_path = str(Path(__file__).absolute())
        tool_raw, unused = input_files.read(source_path)
        if len(raw) > 1 << 20:
            reject("inventory_request_rejected")
        error_protocol = request(raw, args.request_sha256)["protocol"]
        result = inventory(raw, args.request_sha256)
        input_files.recheck()
        result["tool_sha256"] = sha(tool_raw)
        print(canonical(result).decode(), end="")
        return 0
    except Unknown as e:
        print(canonical({"protocol": error_protocol, "category": str(e), "capabilities": {k: False for k in CAPABILITIES}}).decode(), end="")
        return 1
    except (OSError, ValueError, UnicodeError, TypeError, KeyError, IndexError):
        print(canonical({"protocol": error_protocol, "category": "inventory_fixed_failure", "capabilities": {k: False for k in CAPABILITIES}}).decode(), end="")
        return 1


if __name__ == "__main__":
    sys.exit(main())
