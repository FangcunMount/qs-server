#!/usr/bin/env python3
"""Real file installation in an independently owned SSH test namespace only.

This adapter never reloads system sshd, installs into /etc, reads a token, or
grants an execution/writer-fence capability. Production management, activation,
session drain and external writers require a separate host integration.
"""
import argparse
import base64
import contextlib
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import stat
import subprocess
import sys
import time

PROTOCOL = "qs-retirement-isolated-host-install/v1"
NAMESPACE = re.compile(r"^/opt/qs-retirement-install-[0-9a-f]{24}$")
SHA = re.compile(r"^[0-9a-f]{64}$")
SOURCE = re.compile(r"^[0-9a-f]{40}$")
RUN = re.compile(r"^[1-9][0-9]{0,19}-[1-9][0-9]{0,3}$")
TARGETS = {"binary": ("bin/qs-retirement-fence", 0o555),
           "policy": ("policy.json", 0o644),
           "keyfile": ("authorized_keys", 0o600),
           "sshd_config": ("sshd_config", 0o600)}
LIMIT = 64 << 20
APPROVAL_LIMIT = 1 << 20
FORBIDDEN = ("production_management_channel_unproven", "actual_account_shell_and_sudo_docker_boundary_unproven",
             "existing_sessions_local_runner_and_external_writers_unproven", "sshd_activation_unproven",
             "fixed_executor_and_database_mutations_not_integrated")


class Rejected(Exception):
    """Only fixed categories may leave the process."""


def refuse(category):
    raise Rejected(category)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def canonical(value):
    return (json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True) + "\n").encode()


def decode(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                refuse("host_install_json_rejected")
            result[key] = value
        return result
    try:
        return json.loads(raw, object_pairs_hook=pairs)
    except Rejected:
        raise
    except (ValueError, UnicodeError):
        refuse("host_install_json_rejected")


def fields(value, names):
    if type(value) is not dict or set(value) != set(names):
        refuse("host_install_schema_rejected")


def matches(pattern, value):
    return type(value) is str and pattern.fullmatch(value) is not None


def stamp(s):
    return {"dev": s.st_dev, "ino": s.st_ino, "mode": stat.S_IMODE(s.st_mode),
            "uid": s.st_uid, "gid": s.st_gid, "nlink": s.st_nlink,
            "size": s.st_size, "mtime_ns": s.st_mtime_ns, "ctime_ns": s.st_ctime_ns}


def _approval_bytes(fd):
    """Bound the real regular-file read; never resolve the supplied path again."""
    chunks, count = [], 0
    while True:
        raw = os.read(fd, min(65536, APPROVAL_LIMIT + 1 - count))
        if not raw:
            return b"".join(chunks)
        count += len(raw)
        if count > APPROVAL_LIMIT:
            refuse("host_install_approval_file_rejected")
        chunks.append(raw)


def read_approval_file(path):
    """Read public metadata through pinned, no-follow directories and one FD.

    Ownership of this public file is not an installation permission. The exact
    canonical bytes still require the independent approval hash in execute().
    """
    if (type(path) is not str or not path.startswith("/") or len(path) > 4096
            or "\x00" in path or os.path.normpath(path) != path or path == "/"):
        refuse("host_install_approval_file_rejected")
    parts = path.split("/")[1:]
    if any(part in ("", ".", "..") for part in parts):
        refuse("host_install_approval_file_rejected")
    descriptors, directories = [], []
    directory_flags = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_NONBLOCK

    def directory_identity(s):
        return (s.st_dev, s.st_ino, s.st_mode, s.st_uid, s.st_gid)

    def regular(s):
        if not stat.S_ISREG(s.st_mode) or s.st_nlink != 1 or s.st_size > APPROVAL_LIMIT:
            refuse("host_install_approval_file_rejected")

    try:
        parent = os.open("/", directory_flags)
        descriptors.append(parent)
        root_before = os.fstat(parent)
        if not stat.S_ISDIR(root_before.st_mode):
            refuse("host_install_approval_file_rejected")
        for part in parts[:-1]:
            before = os.stat(part, dir_fd=parent, follow_symlinks=False)
            if not stat.S_ISDIR(before.st_mode):
                refuse("host_install_approval_file_rejected")
            child = os.open(part, directory_flags, dir_fd=parent)
            descriptors.append(child)
            actual = os.fstat(child)
            if directory_identity(before) != directory_identity(actual):
                refuse("host_install_approval_file_rejected")
            directories.append((parent, part, child, directory_identity(actual)))
            parent = child
        before = os.stat(parts[-1], dir_fd=parent, follow_symlinks=False)
        regular(before)
        fd = os.open(parts[-1], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=parent)
        descriptors.append(fd)
        opened = os.fstat(fd)
        regular(opened)
        baseline = stamp(opened)
        if stamp(before) != baseline:
            refuse("host_install_approval_file_rejected")
        raw = _approval_bytes(fd)
        after, named = os.fstat(fd), os.stat(parts[-1], dir_fd=parent, follow_symlinks=False)
        regular(after)
        regular(named)
        if stamp(after) != baseline or stamp(named) != baseline or len(raw) != opened.st_size:
            refuse("host_install_approval_file_rejected")
        for parent, name, child, expected in directories:
            if (directory_identity(os.fstat(child)) != expected
                    or directory_identity(os.stat(name, dir_fd=parent, follow_symlinks=False)) != expected):
                refuse("host_install_approval_file_rejected")
        if (directory_identity(os.fstat(descriptors[0])) != directory_identity(root_before)
                or directory_identity(os.stat("/", follow_symlinks=False)) != directory_identity(root_before)):
            refuse("host_install_approval_file_rejected")
        return raw
    except OSError:
        refuse("host_install_approval_file_rejected")
    finally:
        for fd in reversed(descriptors):
            os.close(fd)


class _Paths:
    """All operations remain relative to validated and pinned directory FDs."""
    def __init__(self, anchor, owner):
        self.anchor, self.owner = Path(anchor), owner
        self.fds = {}
        self.dirs = {}
        self.parents = []
        flags = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_NONBLOCK
        if str(self.anchor) != os.path.normpath(str(self.anchor)) or not self.anchor.is_absolute():
            refuse("host_install_path_rejected")
        try:
            fd = os.open(str(self.anchor), flags)
            self._directory(fd)
            self.fds[str(self.anchor)] = fd
            self.dirs[str(self.anchor)] = stamp(os.fstat(fd))
        except OSError:
            self.close()
            refuse("host_install_directory_rejected")
        except Rejected:
            if 'fd' in locals() and fd not in self.fds.values():
                os.close(fd)
            self.close()
            raise

    def _directory(self, fd):
        s = os.fstat(fd)
        if not stat.S_ISDIR(s.st_mode) or s.st_uid != self.owner or s.st_mode & 0o022:
            refuse("host_install_directory_rejected")

    def directory(self, path):
        path = str(path)
        if path == str(self.anchor):
            return self.fds[path]
        if not path.startswith(str(self.anchor).rstrip("/") + "/") or os.path.normpath(path) != path:
            refuse("host_install_path_rejected")
        cursor = str(self.anchor)
        for part in Path(path).relative_to(self.anchor).parts:
            parent = self.fds[cursor]
            child = str(Path(cursor) / part)
            if child not in self.fds:
                try:
                    before = os.stat(part, dir_fd=parent, follow_symlinks=False)
                    fd = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=parent)
                    try:
                        self._directory(fd)
                    except Rejected:
                        os.close(fd)
                        raise
                    actual = os.fstat(fd)
                    if (before.st_dev, before.st_ino) != (actual.st_dev, actual.st_ino):
                        os.close(fd)
                        refuse("host_install_directory_changed")
                except OSError:
                    refuse("host_install_directory_rejected")
                self.fds[child], self.dirs[child] = fd, stamp(actual)
                self.parents.append((cursor, part, child))
            cursor = child
        self.recheck()
        return self.fds[path]

    def recheck(self):
        for path, fd in self.fds.items():
            self._directory(fd)
            s = os.fstat(fd)
            saved = self.dirs[path]
            if (s.st_dev, s.st_ino, stat.S_IMODE(s.st_mode), s.st_uid, s.st_gid, s.st_nlink) != tuple(saved[k] for k in ("dev", "ino", "mode", "uid", "gid", "nlink")):
                refuse("host_install_directory_changed")
        for parent, name, child in self.parents:
            s = os.stat(name, dir_fd=self.fds[parent], follow_symlinks=False)
            if (s.st_dev, s.st_ino) != (self.dirs[child]["dev"], self.dirs[child]["ino"]):
                refuse("host_install_directory_changed")
        actual = os.lstat(self.anchor)
        if (actual.st_dev, actual.st_ino) != (self.dirs[str(self.anchor)]["dev"], self.dirs[str(self.anchor)]["ino"]):
            refuse("host_install_directory_changed")

    def read(self, path, maximum=LIMIT, absent=False):
        p = Path(path)
        parent = self.directory(p.parent)
        try:
            before = os.stat(p.name, dir_fd=parent, follow_symlinks=False)
        except FileNotFoundError:
            if absent:
                return None, None
            refuse("host_install_file_missing")
        if not stat.S_ISREG(before.st_mode) or before.st_uid != self.owner or before.st_nlink != 1 or before.st_mode & 0o022 or before.st_size > maximum:
            refuse("host_install_file_rejected")
        try:
            fd = os.open(p.name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=parent)
            with os.fdopen(fd, "rb") as f:
                opened = os.fstat(f.fileno())
                if stamp(opened) != stamp(before):
                    refuse("host_install_file_changed")
                raw = f.read(maximum + 1)
                after = os.fstat(f.fileno())
            named = os.stat(p.name, dir_fd=parent, follow_symlinks=False)
            if len(raw) > maximum or stamp(after) != stamp(opened) or stamp(named) != stamp(opened):
                refuse("host_install_file_changed")
        except OSError:
            refuse("host_install_file_rejected")
        self.recheck()
        return raw, stamp(opened)

    def create(self, path, raw, mode=0o600, uid=None, gid=None):
        p = Path(path)
        parent = self.directory(p.parent)
        self.recheck()
        try:
            fd = os.open(p.name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_NONBLOCK, 0o600, dir_fd=parent)
            with os.fdopen(fd, "wb") as f:
                f.write(raw)
                f.flush()
                if uid is not None and (os.fstat(f.fileno()).st_uid != uid or os.fstat(f.fileno()).st_gid != gid):
                    os.fchown(f.fileno(), uid, gid)
                os.fchmod(f.fileno(), mode)
                os.fsync(f.fileno())
            os.fsync(parent)
        except FileExistsError:
            refuse("host_install_existing_state")
        except OSError:
            refuse("host_install_persistence_failed")
        self._own_directory_write(p.parent)
        return self.read(p)

    def _own_directory_write(self, path):
        # APFS counts regular directory entries in st_nlink; Linux usually
        # counts subdirectories only. Our known entry change updates only this
        # field after verifying every remaining protected directory attribute.
        saved = self.dirs[str(path)]
        actual = os.fstat(self.fds[str(path)])
        if (actual.st_dev, actual.st_ino, stat.S_IMODE(actual.st_mode), actual.st_uid, actual.st_gid) != tuple(saved[k] for k in ("dev", "ino", "mode", "uid", "gid")):
            refuse("host_install_directory_changed")
        saved["nlink"] = actual.st_nlink

    def snapshot(self, path):
        raw, meta = self.read(path, absent=True)
        return {"present": raw is not None, "sha256": digest(raw) if raw is not None else "", "stat": meta}

    def cas_write(self, path, expected, raw, mode, uid, gid):
        if self.snapshot(path) != expected:
            refuse("host_install_baseline_conflict")
        p = Path(path)
        parent = self.directory(p.parent)
        temp = p.parent / (".qs-install-" + secrets.token_hex(16))
        self.create(temp, raw, mode, uid, gid)
        # Host owns the operation lock and all root-protected parent directories.
        # An independent root writer is not fenced by this file primitive.
        if self.snapshot(path) != expected:
            os.unlink(temp.name, dir_fd=parent)
            os.fsync(parent)
            self._own_directory_write(p.parent)
            refuse("host_install_baseline_conflict")
        os.rename(temp.name, p.name, src_dir_fd=parent, dst_dir_fd=parent)
        os.fsync(parent)
        self._own_directory_write(p.parent)
        installed = self.snapshot(path)
        if installed["sha256"] != digest(raw) or installed["stat"]["mode"] != mode or installed["stat"]["uid"] != uid or installed["stat"]["gid"] != gid:
            refuse("host_install_readback_failed")
        return installed

    def cas_remove(self, path, expected):
        if self.snapshot(path) != expected:
            refuse("host_install_rollback_conflict")
        p = Path(path)
        parent = self.directory(p.parent)
        os.unlink(p.name, dir_fd=parent)
        os.fsync(parent)
        self._own_directory_write(p.parent)
        if self.snapshot(path)["present"]:
            refuse("host_install_rollback_conflict")

    def close(self):
        for fd in reversed(list(self.fds.values())):
            os.close(fd)
        self.fds.clear()


def _plan(raw, approved_sha):
    if type(raw) is not bytes or not matches(SHA, approved_sha) or digest(raw) != approved_sha:
        refuse("host_install_approval_rejected")
    p = decode(raw)
    fields(p, ("protocol", "scope", "source_sha", "operation_id", "run_id", "request_sha256", "namespace", "marker_sha256", "sshd_sha256", "assets", "matches"))
    if raw != canonical(p) or p["protocol"] != PROTOCOL or p["scope"] != "isolated_root_namespace" or not matches(SOURCE, p["source_sha"]) or not matches(RUN, p["operation_id"]) or not matches(RUN, p["run_id"]) or not matches(SHA, p["request_sha256"]) or not matches(SHA, p["marker_sha256"]) or not matches(SHA, p["sshd_sha256"]) or type(p["namespace"]) is not str or not 1 <= len(p["namespace"]) <= 512:
        refuse("host_install_approval_rejected")
    fields(p["assets"], TARGETS)
    for name in TARGETS:
        a = p["assets"][name]
        fields(a, ("sha256", "original"))
        if not matches(SHA, a["sha256"]):
            refuse("host_install_approval_rejected")
        o = a["original"]
        fields(o, ("present", "sha256", "mode", "uid", "gid"))
        if type(o["present"]) is not bool or (not o["present"] and o != {"present": False, "sha256": "", "mode": None, "uid": None, "gid": None}) or (o["present"] and (not matches(SHA, o["sha256"]) or any(type(o[k]) is not int or o[k] < 0 for k in ("mode", "uid", "gid")) or o["mode"] > 0o777 or o["mode"] & 0o022)):
            refuse("host_install_approval_rejected")
    if type(p["matches"]) is not list or not 1 <= len(p["matches"]) <= 64:
        refuse("host_install_match_coverage_rejected")
    seen = set()
    for m in p["matches"]:
        fields(m, ("user", "host", "addr", "laddr", "lport"))
        if any(type(v) is not str or not re.fullmatch(r"[A-Za-z0-9_.:-]{1,128}", v) for v in m.values()) or not m["lport"].isdigit() or not 1 <= int(m["lport"]) <= 65535 or canonical(m) in seen:
            refuse("host_install_match_coverage_rejected")
        seen.add(canonical(m))
    return p


def _scope(p):
    if sys.platform != "linux" or os.geteuid() != 0 or not NAMESPACE.fullmatch(p["namespace"]):
        refuse("host_install_production_boundary_unproven")
    # A caller's isolated=true cannot authorize any system asset path. The exact
    # namespace is a root-owned tmpfs mount and all targets are fixed beneath it.
    try:
        rows = Path("/proc/self/mountinfo").read_text().splitlines()
        found = [r for r in rows if r.split()[4] == p["namespace"]]
        if len(found) != 1 or found[0].split(" - ", 1)[1].split()[0] != "tmpfs":
            refuse("host_install_namespace_rejected")
    except (OSError, IndexError, ValueError):
        refuse("host_install_namespace_rejected")
    return "/", 0, "/usr/sbin/sshd"


def _payloads(paths, p, root):
    marker, _ = paths.read(root / "namespace.private.json", 4096)
    expected = {"protocol": "qs-retirement-owned-install-namespace/v1", "namespace": str(root), "source_sha": p["source_sha"], "operation_id": p["operation_id"], "run_id": p["run_id"]}
    if digest(marker) != p["marker_sha256"] or marker != canonical(expected):
        refuse("host_install_namespace_rejected")
    payloads = {}
    for name in TARGETS:
        raw, _ = paths.read(root / "inputs" / name)
        if digest(raw) != p["assets"][name]["sha256"]:
            refuse("host_install_source_changed")
        payloads[name] = raw
    policy = decode(payloads["policy"])
    if not isinstance(policy, dict) or policy.get("source_sha") != p["source_sha"] or policy.get("operation_id") != p["operation_id"] or policy.get("run_id", "") + "-" + policy.get("run_attempt", "") != p["run_id"] or policy.get("request_sha256") != p["request_sha256"]:
        refuse("host_install_policy_binding_rejected")
    if any(root != q and root not in q.parents for q in [root / rel for rel, _ in TARGETS.values()]):
        refuse("host_install_path_rejected")
    # Fixed test key grammar; certificates, options that loosen restrict, loader
    # environment, arbitrary commands and alternate key paths are not accepted.
    lines = payloads["keyfile"].decode("ascii", errors="strict").splitlines()
    seen = set()
    for line in lines:
        match = re.fullmatch(r'restrict,command="([^"\r\n]+)" (ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp256) ([A-Za-z0-9+/=]+)', line)
        if not match:
            refuse("host_install_keyfile_rejected")
        try:
            wire = base64.b64decode(match[3], validate=True)
            length = int.from_bytes(wire[:4], "big")
            if len(wire) < 4 + length or wire[4:4 + length].decode("ascii") != match[2]:
                refuse("host_install_keyfile_rejected")
        except (ValueError, UnicodeError):
            refuse("host_install_keyfile_rejected")
        fp = "SHA256:" + base64.b64encode(hashlib.sha256(wire).digest()).decode().rstrip("=")
        command = (f"{root}/bin/qs-retirement-fence --policy {root}/policy.json --policy-sha256 {digest(payloads['policy'])} "
                   f"--authenticated-key {fp} --github-token-file {root}/readonly-github-token")
        if match[1] != command or fp in seen:
            refuse("host_install_keyfile_rejected")
        seen.add(fp)
    if not seen or policy.get("ssh_key_fingerprint") not in seen:
        refuse("host_install_keyfile_rejected")
    return payloads


def _verify_effective(raw, root):
    settings = {}
    try:
        for line in raw.decode("ascii").splitlines():
            key, value = line.split(" ", 1)
            if key in settings:
                refuse("host_install_sshd_projection_rejected")
            settings[key] = value.strip()
    except (ValueError, UnicodeError):
        refuse("host_install_sshd_projection_rejected")
    required = {"authenticationmethods": "publickey", "authorizedkeysfile": str(root / "authorized_keys"),
                "authorizedkeyscommand": "none", "trustedusercakeys": "none", "passwordauthentication": "no",
                "kbdinteractiveauthentication": "no", "hostbasedauthentication": "no", "gssapiauthentication": "no",
                "permituserenvironment": "no", "permituserrc": "no", "disableforwarding": "yes", "permittty": "no",
                "forcecommand": "none", "pubkeyauthentication": "yes", "strictmodes": "yes"}
    if any(settings.get(k) != v for k, v in required.items()) or any(x not in ("LANG", "LC_*") for x in settings.get("acceptenv", "").split()):
        refuse("host_install_sshd_projection_rejected")


def _sshd(paths, p, root, executable):
    before, baseline = paths.read(executable)
    if digest(before) != p["sshd_sha256"] or not baseline["mode"] & 0o111:
        refuse("host_install_sshd_binary_rejected")
    for match in [None] + p["matches"]:
        args = [str(executable), "-t" if match is None else "-T", "-f", str(root / "sshd_config")]
        if match is not None:
            args += ["-C", ",".join(k + "=" + match[k] for k in ("user", "host", "addr", "laddr", "lport"))]
        code, stdout, stderr = _capture(args)
        if code or stderr:
            refuse("host_install_sshd_validation_failed")
        if match is not None:
            _verify_effective(stdout, root)
        after, meta = paths.read(executable)
        if after != before or meta != baseline:
            refuse("host_install_sshd_binary_changed")


def _capture(args):
    # stdout/stderr are bounded in memory, never forwarded or written to logs.
    process = None
    completed = False
    try:
        process = subprocess.Popen(args, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                   start_new_session=True, env={"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LANG": "C"})
        chunks = [bytearray(), bytearray()]
        deadline = time.monotonic() + 10
        with selectors.DefaultSelector() as selector:
            for index, pipe in enumerate((process.stdout, process.stderr)):
                os.set_blocking(pipe.fileno(), False)
                selector.register(pipe, selectors.EVENT_READ, index)
            while selector.get_map():
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    refuse("host_install_sshd_validation_failed")
                for key, _ in selector.select(min(remaining, 0.2)):
                    raw = os.read(key.fd, 65536)
                    if not raw:
                        selector.unregister(key.fileobj)
                    else:
                        chunks[key.data].extend(raw)
                        if sum(map(len, chunks)) > 2 << 20:
                            refuse("host_install_sshd_validation_failed")
        code = process.wait(timeout=max(0.001, deadline - time.monotonic()))
        completed = True
        return code, bytes(chunks[0]), bytes(chunks[1])
    except (OSError, subprocess.TimeoutExpired):
        refuse("host_install_sshd_validation_failed")
    finally:
        if process is not None:
            if not completed:
                # This adapter owns this fixed verifier process, never a borrowed
                # daemon, shared shell, or system sshd service lifecycle.
                try:
                    os.killpg(process.pid, 9)
                except ProcessLookupError:
                    pass
                process.wait()
            for pipe in (process.stdout, process.stderr):
                if pipe is not None:
                    pipe.close()


class _Journal:
    def __init__(self, paths, root, approved):
        self.paths, self.root, self.approved = paths, root / "state", approved
        self.previous, self.seq, self.last = "", 0, None

    def append(self, phase, original, installed):
        value = {"protocol": PROTOCOL, "approval_sha256": self.approved, "sequence": self.seq,
                 "previous_sha256": self.previous, "phase": phase, "original": original, "installed": installed}
        raw = canonical(value)
        self.paths.create(self.root / f"{self.seq:04d}.json", raw)
        actual, _ = self.paths.read(self.root / f"{self.seq:04d}.json")
        if actual != raw:
            refuse("host_install_journal_unknown")
        self.previous, self.seq, self.last = digest(raw), self.seq + 1, decode(raw)

    def load(self):
        names = os.listdir(self.paths.directory(self.root))
        allowed = {"operation.lock"} | {f"original-{x}.private" for x in TARGETS}
        logs = sorted(n for n in names if re.fullmatch(r"[0-9]{4}\.json", n))
        if not logs or any(n not in allowed and n not in logs for n in names):
            refuse("host_install_journal_unknown")
        for index, name in enumerate(logs):
            if name != f"{index:04d}.json":
                refuse("host_install_journal_unknown")
            raw, _ = self.paths.read(self.root / name, 1 << 20)
            v = decode(raw)
            fields(v, ("protocol", "approval_sha256", "sequence", "previous_sha256", "phase", "original", "installed"))
            if raw != canonical(v) or v["protocol"] != PROTOCOL or v["approval_sha256"] != self.approved or v["sequence"] != self.seq or v["previous_sha256"] != self.previous or v["phase"] not in ("intent", "installing", "installed", "rollback_intent", "rolling_back", "rolled_back", "rollback_conflict"):
                refuse("host_install_journal_unknown")
            _snapshots(v["original"], complete=True)
            _snapshots(v["installed"], complete=False)
            if self.last is None:
                if v["phase"] != "intent" or v["installed"]:
                    refuse("host_install_journal_unknown")
            else:
                before, current = list(self.last["installed"]), list(v["installed"])
                phase, prior = v["phase"], self.last["phase"]
                if phase == "installing":
                    valid = prior in ("intent", "installing") and len(current) == len(before) + 1 and set(before) < set(current)
                elif phase == "installed":
                    valid = prior == "installing" and set(current) == set(TARGETS) and v["installed"] == self.last["installed"]
                elif phase == "rollback_intent":
                    valid = prior in ("intent", "installing", "installed") and v["installed"] == self.last["installed"]
                elif phase == "rolling_back":
                    valid = prior in ("rollback_intent", "rolling_back") and len(current) == len(before) - 1 and set(current) < set(before)
                elif phase == "rolled_back":
                    valid = prior in ("rollback_intent", "rolling_back") and not current
                elif phase == "rollback_conflict":
                    valid = prior in ("rollback_intent", "rolling_back") and v["installed"] == self.last["installed"]
                else:
                    valid = False
                if not valid or any(v["installed"][k] != self.last["installed"][k] for k in set(current) & set(before)):
                    refuse("host_install_journal_unknown")
            if self.last is not None and (v["original"] != self.last["original"] or self.last["phase"] in ("rolled_back", "rollback_conflict")):
                refuse("host_install_journal_unknown")
            self.previous, self.seq, self.last = digest(raw), self.seq + 1, v
        return self.last


def _snapshots(value, complete):
    if type(value) is not dict or (set(value) != set(TARGETS) if complete else not set(value) <= set(TARGETS)):
        refuse("host_install_journal_unknown")
    for snapshot in value.values():
        fields(snapshot, ("present", "sha256", "stat"))
        if type(snapshot["present"]) is not bool:
            refuse("host_install_journal_unknown")
        if not snapshot["present"]:
            if snapshot != {"present": False, "sha256": "", "stat": None}:
                refuse("host_install_journal_unknown")
            continue
        if type(snapshot["sha256"]) is not str or not SHA.fullmatch(snapshot["sha256"]):
            refuse("host_install_journal_unknown")
        fields(snapshot["stat"], ("dev", "ino", "mode", "uid", "gid", "nlink", "size", "mtime_ns", "ctime_ns"))
        if any(type(x) is not int or x < 0 for x in snapshot["stat"].values()) or snapshot["stat"]["nlink"] != 1 or snapshot["stat"]["mode"] > 0o777 or snapshot["stat"]["mode"] & 0o022:
            refuse("host_install_journal_unknown")


@contextlib.contextmanager
def _lock(paths, root):
    path = root / "state" / "operation.lock"
    fd_dir = paths.directory(path.parent)
    fd = None
    try:
        if not paths.snapshot(path)["present"]:
            try:
                paths.create(path, b"")
            except Rejected as error:
                if str(error) != "host_install_existing_state":
                    raise
        fd = os.open(path.name, os.O_RDWR | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd_dir)
        s = os.fstat(fd)
        if not stat.S_ISREG(s.st_mode) or s.st_uid != paths.owner or s.st_nlink != 1 or stat.S_IMODE(s.st_mode) != 0o600:
            refuse("host_install_lock_rejected")
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        if stamp(os.stat(path.name, dir_fd=fd_dir, follow_symlinks=False)) != stamp(s):
            refuse("host_install_lock_rejected")
    except BlockingIOError:
        os.close(fd)
        refuse("host_install_operation_busy")
    except OSError:
        if fd is not None:
            os.close(fd)
        refuse("host_install_lock_rejected")
    except Rejected:
        if fd is not None:
            os.close(fd)
        raise
    try:
        yield
    finally:
        os.close(fd)


def _original_matches(actual, approved):
    if actual["present"] != approved["present"] or actual["sha256"] != approved["sha256"]:
        return False
    return not actual["present"] or all(actual["stat"][k] == approved[k] for k in ("mode", "uid", "gid"))


def _rollback(paths, root, journal, originals, installed):
    journal.append("rollback_intent", originals, installed)
    for name in reversed(list(installed)):
        target = root / TARGETS[name][0]
        if paths.snapshot(target) != installed[name]:
            journal.append("rollback_conflict", originals, installed)
            refuse("host_install_rollback_conflict")
        old = originals[name]
        if old["present"]:
            raw, _ = paths.read(root / "state" / f"original-{name}.private")
            if digest(raw) != old["sha256"]:
                refuse("host_install_original_asset_changed")
            meta = old["stat"]
            paths.cas_write(target, installed[name], raw, meta["mode"], meta["uid"], meta["gid"])
        else:
            paths.cas_remove(target, installed[name])
        del installed[name]
        journal.append("rolling_back", originals, installed)
    for name, old in originals.items():
        actual = paths.snapshot(root / TARGETS[name][0])
        if actual["present"] != old["present"] or actual["sha256"] != old["sha256"] or old["present"] and any(actual["stat"][k] != old["stat"][k] for k in ("mode", "uid", "gid")):
            refuse("host_install_rollback_conflict")
    journal.append("rolled_back", originals, installed)


def _execute(raw, approved_sha, operation, anchor, owner, executable, fault=None):
    if operation not in ("install", "inspect", "rollback"):
        refuse("host_install_operation_rejected")
    p = _plan(raw, approved_sha)
    root = Path(p["namespace"])
    paths = _Paths(anchor, owner)
    try:
        paths.directory(root / "bin")
        paths.directory(root / "inputs")
        paths.directory(root / "state")
        if stat.S_IMODE(os.fstat(paths.directory(root)).st_mode) != 0o700 or stat.S_IMODE(os.fstat(paths.directory(root / "state")).st_mode) != 0o700:
            refuse("host_install_directory_rejected")
        payloads = _payloads(paths, p, root)
        journal = _Journal(paths, root, approved_sha)
        with _lock(paths, root):
            if operation in ("inspect", "rollback"):
                last = journal.load()
                if last["phase"] not in ("installed", "rolled_back"):
                    refuse("host_install_journal_unknown")
                for name, original in last["original"].items():
                    if not _original_matches(original, p["assets"][name]["original"]):
                        refuse("host_install_journal_unknown")
                    if original["present"]:
                        raw_old, _ = paths.read(root / "state" / f"original-{name}.private")
                        if digest(raw_old) != original["sha256"]:
                            refuse("host_install_original_asset_changed")
                if last["phase"] == "installed" and set(last["installed"]) != set(TARGETS):
                    refuse("host_install_journal_unknown")
                for name, installed in last["installed"].items():
                    if not installed["present"] or installed["sha256"] != p["assets"][name]["sha256"] or installed["stat"]["mode"] != TARGETS[name][1] or installed["stat"]["uid"] != owner:
                        refuse("host_install_journal_unknown")
                if operation == "rollback" and last["phase"] == "installed":
                    _rollback(paths, root, journal, last["original"], dict(last["installed"]))
                elif last["phase"] == "installed":
                    for name, expected in last["installed"].items():
                        if paths.snapshot(root / TARGETS[name][0]) != expected:
                            refuse("host_install_baseline_conflict")
                    _sshd(paths, p, root, executable)
                else:
                    for name, original in last["original"].items():
                        if not _original_matches(paths.snapshot(root / TARGETS[name][0]), p["assets"][name]["original"]):
                            refuse("host_install_baseline_conflict")
                return _receipt(journal, p)
            if operation != "install":
                refuse("host_install_operation_rejected")
            existing = set(os.listdir(paths.directory(root / "state")))
            if any(re.fullmatch(r"[0-9]{4}\.json", n) for n in existing):
                refuse("host_install_existing_state")
            if existing != {"operation.lock"}:
                refuse("host_install_journal_unknown")
            originals, installed = {}, {}
            for name in TARGETS:
                actual = paths.snapshot(root / TARGETS[name][0])
                if not _original_matches(actual, p["assets"][name]["original"]):
                    refuse("host_install_baseline_conflict")
                originals[name] = actual
            journal.append("intent", originals, installed)
            try:
                for name, actual in originals.items():
                    if actual["present"]:
                        old, _ = paths.read(root / TARGETS[name][0])
                        if paths.snapshot(root / TARGETS[name][0]) != actual:
                            refuse("host_install_baseline_conflict")
                        paths.create(root / "state" / f"original-{name}.private", old)
                if fault:
                    fault("intent")
                for name, (relative, mode) in TARGETS.items():
                    installed[name] = paths.cas_write(root / relative, originals[name], payloads[name], mode, owner, os.getgid())
                    journal.append("installing", originals, installed)
                    if fault:
                        fault(name)
                _sshd(paths, p, root, executable)
                paths.recheck()
                for name in TARGETS:
                    if paths.snapshot(root / TARGETS[name][0]) != installed[name]:
                        refuse("host_install_readback_failed")
                journal.append("installed", originals, installed)
            except Exception:
                _rollback(paths, root, journal, originals, installed)
                refuse("host_install_failed_and_rolled_back")
            return _receipt(journal, p)
    except Rejected:
        raise
    except (OSError, ValueError, UnicodeError, TypeError, KeyError):
        refuse("host_install_fixed_failure")
    finally:
        paths.close()


def _receipt(journal, plan):
    return {"protocol": PROTOCOL, "source_sha": plan["source_sha"], "operation_id": plan["operation_id"],
            "run_id": plan["run_id"], "approval_sha256": journal.approved, "journal_sha256": journal.previous,
            "phase": journal.last["phase"], "asset_count": 4, "scope": "isolated_root_namespace",
            "production_installed": False, "sshd_reloaded": False, "whole_writer_fence_proven": False,
            "execution_authority": False, "drop_ready": False, "required": list(FORBIDDEN)}


def execute(raw, approved_sha, operation):
    plan = _plan(raw, approved_sha)
    anchor, owner, executable = _scope(plan)
    return _execute(raw, approved_sha, operation, anchor, owner, executable)


def main():
    class FixedParser(argparse.ArgumentParser):
        def error(self, message):
            refuse("host_install_input_rejected")
    parser = FixedParser(description=__doc__)
    parser.add_argument("--operation", required=True, choices=("install", "inspect", "rollback"))
    parser.add_argument("--approval-file", required=True)
    parser.add_argument("--approval-sha256", required=True)
    try:
        args = parser.parse_args()
        raw = read_approval_file(args.approval_file)
        result = execute(raw, args.approval_sha256, args.operation)
        print(canonical(result).decode(), end="")
        return 0
    except Rejected as error:
        print(canonical({"category": str(error), "production_installed": False, "drop_ready": False}).decode(), end="")
        return 1
    except OSError:
        print('{"category":"host_install_approval_unavailable","production_installed":false,"drop_ready":false}')
        return 1


if __name__ == "__main__":
    sys.exit(main())
