#!/usr/bin/env python3
"""Fixed-target Mongo FCV observation using the declared backup SSH route.

No backup, retention, production mutation or privilege change is implemented.
Only this invocation's tunnel, temporary key and known_hosts are owned here.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import selectors
import signal
import socket
import stat
import subprocess
import sys
import tempfile
import time

EXPECTED_TARGET = "ab3c60ccc9106a110d85946468f0cec60a18614e54fde49dadfa8701a362036f"
SHA = re.compile(r"^[0-9a-f]{40}$")
HASH = re.compile(r"^[0-9a-f]{64}$")
RUN = re.compile(r"^[0-9]{1,20}-[0-9]{1,4}$")
HOST = re.compile(r"^(?:[A-Za-z0-9][A-Za-z0-9.-]{0,252}|[0-9A-Fa-f:]{2,64})$")
ACCOUNT = re.compile(r"^[A-Za-z_][A-Za-z0-9_-]{0,63}$")
FINGERPRINT = re.compile(r"^SHA256:[A-Za-z0-9+/]{43}=?$")
INPUT_ENV = ("MONGO_PROFILE_SOURCE_SHA", "MONGO_PROFILE_CONFIRM_SHA", "MONGO_PROFILE_RUN_ID", "MONGO_PROFILE_EXPECTED_TARGET_HASH", "MONGO_PROFILE_OPERATION", "MONGO_PROFILE_DATABASE", "MONGO_PROFILE_ARCHIVE_INPUT", "GITHUB_SHA", "MONGO_BACKUP_HOST", "MONGO_BACKUP_USERNAME", "MONGO_BACKUP_SSH_PORT", "MONGO_BACKUP_SSH_KEY", "MONGO_BACKUP_SSH_FINGERPRINT", "MONGODB_PORT", "MONGODB_USERNAME", "MONGODB_PASSWORD", "MONGODB_DBNAME")
ERRORS = frozenset({"none", "input_invalid", "source_binding_failed", "ssh_route_invalid", "fingerprint_mismatch", "tunnel_port_busy", "tunnel_unavailable", "tool_failed", "receipt_invalid", "authorization_unknown", "authentication_mismatch", "target_identity_mismatch", "identity_changed", "migration_state_invalid", "build_metadata_invalid", "build_version_mismatch", "fcv_permission_denied", "fcv_read_failed", "fcv_shape_invalid", "fcv_transition_observed", "ttl_metadata_invalid", "reading_budget_exceeded", "cursor_release_failed", "client_release_failed", "owned_resource_release_failed", "interrupted"})

RECEIPT_SCHEMA = {
    "protocol": frozenset({"mongo_admin_metadata_v1"}),
    "operation": frozenset({"metadata-visibility-admin-profile"}), "database": frozenset({"mongodb"}),
    "source_sha": "sha40", "run_id": "run_id", "tool_sha256": "hash64", "source_binding_hash": "hash64",
    "expected_target_hash": "hash64", "target_hash": "hash64_or_empty",
    "fcv_observation_complete": "bool", "complete": "bool", "eligible_fcv_readiness": "bool",
    "backup_restore_proven": "bool", "business_acceptance_proven": "bool", "error_category": ERRORS,
    "target_check": {"namespace_matches_expected": "bool", "replica_set_matches_expected": "bool", "advertised_members_match_expected": "bool", "catalog_fingerprint_matches_expected": "bool", "identity_unchanged": "bool", "process_identity_unchanged": "bool", "writable_primary": "bool", "head_version": "uint", "dirty": "bool", "retired_namespace_absent": "bool"},
    "authorization": {"authenticated_as_requested": "bool", "authentication_database_matches_expected": "bool", "privileges_observed": "bool", "get_parameter_hint": "nullable_bool", "catalog_metadata_hint": "nullable_bool", "migration_read_hint": "nullable_bool"},
    "server_build": {"known": "bool", "major": "uint", "minor": "uint", "patch": "uint", "matches_expected": "bool"},
    "feature_compatibility": {"known": "bool", "major": "uint", "minor": "uint", "target_present": "bool", "target_major": "uint", "target_minor": "uint", "previous_present": "bool", "previous_major": "uint", "previous_minor": "uint", "cleanup_present": "bool", "cleanup_value": "bool", "change_timestamp_present": "bool", "change_timestamp_seconds": "uint", "change_timestamp_increment": "uint", "transition_observed": "bool"},
    "ttl_monitor": {"known":"bool","enabled":"bool"},
    "release": {"cursors_closed": "bool", "client_disconnected": "bool", "tunnel_terminated": "bool", "tunnel_waited": "bool", "key_unlinked": "bool", "known_hosts_unlinked": "bool", "temp_directory_released": "bool", "owned_tunnel_process_released": "bool", "reader_process_released": "bool"},
    "budget": {"elapsed_milliseconds": "uint", "logical_queries": "uint"},
}

# The native reader retains its existing protocol. Only the final transport
# receipt adds a fixed stage, so a pre-database timeout cannot be mistaken for
# an observed database failure. No command, address or stderr is published.
TRANSPORT_STAGES = frozenset({"none", "preflight", "temporary_material", "ssh_keyscan", "ssh_fingerprint", "ssh_route", "port_reservation", "tunnel_start", "tunnel_listener", "tunnel_ownership", "native_reader", "source_recheck", "tunnel_recheck", "owned_resource_release", "total_budget"})
FINAL_RECEIPT_SCHEMA = {**RECEIPT_SCHEMA, "failure_stage": TRANSPORT_STAGES}


class ProfileError(ValueError):
    pass


def transport():
    spec = importlib.util.spec_from_file_location("mongo_receipt_transport", Path(__file__).with_name("receipt-transport.py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def unique_object(pairs):
    value = {}
    for key, item in pairs:
        if key in value:
            raise ProfileError("receipt_invalid")
        value[key] = item
    return value


def require_fields(value, schema):
    if type(value) is not dict or set(value) != set(schema):
        raise ProfileError("receipt_invalid")
    for key, spec in schema.items():
        if isinstance(spec, dict):
            require_fields(value[key], spec)


def blank(schema):
    value = {}
    for key, spec in schema.items():
        if isinstance(spec, dict):
            value[key] = blank(spec)
        elif spec == "bool":
            value[key] = False
        elif spec == "nullable_bool":
            value[key] = None
        elif spec == "uint":
            value[key] = 0
        elif isinstance(spec, frozenset):
            value[key] = "none" if key == "error_category" else next(iter(spec))
        else:
            value[key] = ""
    return value


def initial_receipt(sha, run, tool_hash="0" * 64, source_hash="0" * 64):
    value = blank(RECEIPT_SCHEMA)
    value.update(source_sha=sha, run_id=run, tool_sha256=tool_hash,
                 source_binding_hash=source_hash, expected_target_hash=EXPECTED_TARGET)
    return value


def safe_receipt(raw, code, sha, run, tool_hash, source_hash, *, final=False):
    try:
        if type(code) is not int or code not in (0, 1) or type(raw) is not bytes or not 0 < len(raw) <= 65536:
            raise ProfileError("receipt_invalid")
        value = json.loads(raw, object_pairs_hook=unique_object,
                           parse_constant=lambda _v: (_ for _ in ()).throw(ProfileError("receipt_invalid")))
        schema = FINAL_RECEIPT_SCHEMA if final else RECEIPT_SCHEMA
        require_fields(value, schema)
        transport()._validate(value, schema)
        if (value["source_sha"], value["run_id"], value["tool_sha256"], value["source_binding_hash"], value["expected_target_hash"]) != (sha, run, tool_hash, source_hash, EXPECTED_TARGET):
            raise ProfileError("receipt_invalid")
        if value["backup_restore_proven"] or value["business_acceptance_proven"] or value["budget"]["logical_queries"] > 10:
            raise ProfileError("receipt_invalid")
        if value["budget"]["elapsed_milliseconds"] > 45000:
            # Preserve actual elapsed/release facts only in a final negative
            # budget receipt. Native and every positive receipt keep the
            # original 45s gate; this does not extend any execution timeout.
            budget_failure = (final and code == 1 and value["error_category"] == "reading_budget_exceeded" and value["failure_stage"] == "total_budget" and not any(value[key] for key in ("fcv_observation_complete", "complete", "eligible_fcv_readiness")))
            if not budget_failure:
                raise ProfileError("receipt_invalid")
        f = value["feature_compatibility"]
        ttl=value["ttl_monitor"]
        if not ttl["known"] and ttl["enabled"]:
            raise ProfileError("receipt_invalid")
        for present, fields in (("known", ("major", "minor")), ("target_present", ("target_major", "target_minor")), ("previous_present", ("previous_major", "previous_minor")), ("change_timestamp_present", ("change_timestamp_seconds", "change_timestamp_increment"))):
            if not f[present] and any(f[key] != 0 for key in fields):
                raise ProfileError("receipt_invalid")
        if not f["cleanup_present"] and f["cleanup_value"] or f["transition_observed"] != (f["target_present"] or f["previous_present"] or f["cleanup_value"]):
            raise ProfileError("receipt_invalid")
        if not f["known"] and any(f[key] for key in ("target_present", "previous_present", "cleanup_present", "change_timestamp_present")):
            raise ProfileError("receipt_invalid")
        build = value["server_build"]
        if not build["known"] and any(build[key] for key in ("major", "minor", "patch", "matches_expected")):
            raise ProfileError("receipt_invalid")
        if any(f[key] > 999 for key in ("major", "minor", "target_major", "target_minor", "previous_major", "previous_minor")):
            raise ProfileError("receipt_invalid")
        observed = value["fcv_observation_complete"]
        if observed:
            tc, auth, build, released = (value[key] for key in ("target_check", "authorization", "server_build", "release"))
            target_flags = set(tc) - {"head_version", "dirty"}
            if code != 0 or value["error_category"] != "none" or value["target_hash"] != EXPECTED_TARGET or not all(tc[key] for key in target_flags) or tc["head_version"] != 39 or tc["dirty"] or not all(auth[key] for key in ("authenticated_as_requested", "authentication_database_matches_expected", "privileges_observed")) or any(auth[key] is None for key in ("get_parameter_hint", "catalog_metadata_hint", "migration_read_hint")) or not build["known"] or not build["matches_expected"] or (build["major"], build["minor"], build["patch"]) != (7, 0, 31) or not f["known"] or f["transition_observed"] or not ttl["known"] or not released["cursors_closed"] or not released["client_disconnected"] or value["budget"]["logical_queries"] != 10:
                raise ProfileError("receipt_invalid")
        elif code == 0 or value["error_category"] == "none":
            raise ProfileError("receipt_invalid")
        if final:
            if (code == 0) != (value["failure_stage"] == "none"):
                raise ProfileError("receipt_invalid")
            eligible = observed and all(value["release"].values())
            if value["complete"] != eligible or value["eligible_fcv_readiness"] != eligible:
                raise ProfileError("receipt_invalid")
        elif value["complete"] or value["eligible_fcv_readiness"] or any(value["release"][key] for key in set(value["release"]) - {"cursors_closed", "client_disconnected"}):
            raise ProfileError("receipt_invalid")
        return value
    except (ValueError, TypeError, KeyError, UnicodeError, RecursionError):
        raise ProfileError("receipt_invalid") from None


def port(text, default):
    text = text or str(default)
    if not re.fullmatch(r"[0-9]{1,5}", text) or not 1 <= int(text) <= 65535:
        raise ProfileError("input_invalid")
    return int(text)


def inputs(env):
    sha, run = env.get("MONGO_PROFILE_SOURCE_SHA", ""), env.get("MONGO_PROFILE_RUN_ID", "")
    if not SHA.fullmatch(sha) or not RUN.fullmatch(run) or env.get("MONGO_PROFILE_CONFIRM_SHA") != sha or env.get("MONGO_PROFILE_EXPECTED_TARGET_HASH") != EXPECTED_TARGET or env.get("MONGO_PROFILE_OPERATION") != "metadata-visibility-admin-profile" or env.get("MONGO_PROFILE_DATABASE") != "mongodb" or env.get("MONGO_PROFILE_ARCHIVE_INPUT", "") or env.get("MONGODB_DBNAME") != "qs":
        raise ProfileError("input_invalid")
    if (env.get("GITHUB_SHA") or sha) != sha:
        raise ProfileError("source_binding_failed")
    host, account, fingerprint = (env.get(key, "") for key in ("MONGO_BACKUP_HOST", "MONGO_BACKUP_USERNAME", "MONGO_BACKUP_SSH_FINGERPRINT"))
    if not HOST.fullmatch(host) or not ACCOUNT.fullmatch(account) or not FINGERPRINT.fullmatch(fingerprint):
        raise ProfileError("input_invalid")
    if not env.get("MONGO_BACKUP_SSH_KEY") or len(env["MONGO_BACKUP_SSH_KEY"]) > 65536 or not 1 <= len(env.get("MONGODB_USERNAME", "")) <= 256 or any(c in env.get("MONGODB_USERNAME", "") for c in "\0\r\n") or not 1 <= len(env.get("MONGODB_PASSWORD", "")) <= 4096:
        raise ProfileError("input_invalid")
    return {"sha": sha, "run": run, "host": host, "account": account, "fingerprint": fingerprint,
            "ssh_port": port(env.get("MONGO_BACKUP_SSH_PORT", ""), 22), "remote_port": port(env.get("MONGODB_PORT", ""), 27017)}


def child_env():
    return {"PATH": "/usr/bin:/bin", "LANG": "C", "LC_ALL": "C"}


def capture_private(args, *, payload=None, timeout=5, limit=65536):
    process = subprocess.Popen(args, env=child_env(), stdin=subprocess.PIPE if payload is not None else subprocess.DEVNULL,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True, bufsize=0)
    selector = selectors.DefaultSelector()
    output, offset, total = bytearray(), 0, 0
    deadline = time.monotonic() + timeout
    try:
        for stream in (process.stdout, process.stderr):
            os.set_blocking(stream.fileno(), False)
            selector.register(stream, selectors.EVENT_READ)
        if payload is not None:
            os.set_blocking(process.stdin.fileno(), False)
            selector.register(process.stdin, selectors.EVENT_WRITE)
        while selector.get_map():
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise ProfileError("reading_budget_exceeded")
            for key, _ in selector.select(min(0.2, remaining)):
                if key.fileobj is process.stdin:
                    offset += os.write(process.stdin.fileno(), payload[offset:offset + 65536])
                    if offset == len(payload):
                        selector.unregister(process.stdin)
                        process.stdin.close()
                    continue
                block = os.read(key.fileobj.fileno(), 65536)
                if not block:
                    selector.unregister(key.fileobj)
                else:
                    total += len(block)
                    if total > limit:
                        raise ProfileError("receipt_invalid")
                    if key.fileobj is process.stdout:
                        output.extend(block)
        return process.wait(timeout=max(0.01, deadline - time.monotonic())), bytes(output)
    finally:
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=2)
        selector.close()
        for stream in (process.stdin, process.stdout, process.stderr):
            if stream is not None:
                stream.close()


def write_owned(path, payload):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    try:
        with os.fdopen(fd, "wb") as handle:
            handle.write(payload)
    except BaseException:
        try:
            os.close(fd)
        except OSError:
            pass
        raise


def binary_hash(binary):
    path = Path(binary)
    if not path.is_absolute():
        raise ProfileError("source_binding_failed")
    st = path.lstat()
    if not stat.S_ISREG(st.st_mode) or st.st_uid != os.getuid() or st.st_mode & 0o022 or st.st_size > 64 << 20:
        raise ProfileError("source_binding_failed")
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd, "rb") as handle:
        if os.fstat(handle.fileno()) != st:
            raise ProfileError("source_binding_failed")
        return hashlib.sha256(handle.read()).hexdigest()


def ssh_args(config, key, known):
    return ["/usr/bin/ssh", "-F", "/dev/null", "-i", str(key), "-l", config["account"], "-p", str(config["ssh_port"]),
            "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-o", "IdentityAgent=none",
            "-o", "PasswordAuthentication=no", "-o", "KbdInteractiveAuthentication=no",
            "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + str(known),
            "-o", "GlobalKnownHostsFile=/dev/null", "-o", "ForwardAgent=no", "-o", "ForwardX11=no",
            "-o", "ControlMaster=no", "-o", "ControlPath=none", "-o", "PermitLocalCommand=no",
            "-o", "ExitOnForwardFailure=yes", "-o", "ConnectTimeout=5", "-o", "ConnectionAttempts=1"]


def validate_ssh(raw, config, key, known):
    try:
        values = {}
        for line in raw.decode("ascii").splitlines():
            field, value = line.split(None, 1)
            if field in values:
                raise ProfileError("ssh_route_invalid")
            values[field] = value
        if values.get("hostname") != config["host"] or values.get("user") != config["account"] or values.get("port") != str(config["ssh_port"]):
            raise ProfileError("ssh_route_invalid")
        for field in ("proxycommand", "proxyjump", "hostkeyalias", "localcommand"):
            if values.get(field, "none") != "none":
                raise ProfileError("ssh_route_invalid")
        if values.get("identityfile") != str(key) or values.get("userknownhostsfile") != str(known) or values.get("identityagent") != "none" or values.get("globalknownhostsfile") != "/dev/null":
            raise ProfileError("ssh_route_invalid")
        if values.get("stricthostkeychecking") != "true" or values.get("forwardagent") != "no" or values.get("forwardx11") != "no":
            raise ProfileError("ssh_route_invalid")
        if any(key in values for key in ("localforward", "remoteforward", "dynamicforward")):
            raise ProfileError("ssh_route_invalid")
    except (ValueError, UnicodeError):
        raise ProfileError("ssh_route_invalid") from None


def match_known_hosts(raw, fingerprint, directory, remaining):
    if not raw or len(raw) > 65536:
        raise ProfileError("fingerprint_mismatch")
    candidates = []
    for line in raw.splitlines():
        if line.startswith(b"#"):
            continue
        parts = line.split()
        if len(parts) != 3 or parts[1] not in (b"ssh-ed25519", b"ssh-rsa", b"ecdsa-sha2-nistp256", b"ecdsa-sha2-nistp384", b"ecdsa-sha2-nistp521") or len(candidates) >= 3:
            raise ProfileError("fingerprint_mismatch")
        candidates.append(line)
    matched = []
    path = directory / "known_hosts"
    for line in candidates:
        write_owned(path, line + b"\n")
        try:
            code, metadata = capture_private(["/usr/bin/ssh-keygen", "-E", "sha256", "-lf", str(path)], timeout=remaining())
            fields = metadata.decode("ascii").split()
            if code == 0 and len(fields) >= 2 and fields[1] == fingerprint:
                matched.append(line)
        finally:
            path.unlink()
    if not matched:
        raise ProfileError("fingerprint_mismatch")
    write_owned(path, b"\n".join(matched) + b"\n")
    return path


def source_check(sha):
    code, raw = capture_private(["/usr/bin/git", "rev-parse", "HEAD"], timeout=5)
    if code != 0 or raw.decode("ascii", errors="ignore").strip() != sha:
        raise ProfileError("source_binding_failed")


def owned_listener(pid, local_port, timeout):
    # A reachable port alone might belong to a competing process after the
    # ephemeral reservation is released. Bind it to this exact owned SSH PID.
    executable = "/usr/sbin/lsof" if sys.platform == "darwin" else "/usr/bin/lsof"
    code, raw = capture_private([executable, "-nP", "-a", "-p", str(pid),
                                 "-iTCP@127.0.0.1:" + str(local_port),
                                 "-sTCP:LISTEN", "-FpnT"], timeout=timeout)
    if code != 0:
        raise ProfileError("tunnel_unavailable")
    fields = raw.decode("ascii").splitlines()
    if [line for line in fields if line.startswith("p")] != ["p" + str(pid)] or [line for line in fields if line.startswith("n")] != ["n127.0.0.1:" + str(local_port)] or "TST=LISTEN" not in fields:
        raise ProfileError("tunnel_unavailable")


def execute(binary, env):
    start = time.monotonic()
    config = inputs(env)
    source_check(config["sha"])
    tool_hash = binary_hash(binary)
    binding = {key: config[key] for key in ("host", "ssh_port", "remote_port", "fingerprint")}
    binding["expected_target_hash"] = EXPECTED_TARGET
    source_hash = hashlib.sha256(json.dumps(binding, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
    value = initial_receipt(config["sha"], config["run"], tool_hash, source_hash)
    directory = None
    tunnel = None
    child_code = 1
    stage, failure_stage = "temporary_material", "none"

    def remaining():
        left = 40 - (time.monotonic() - start)
        if left <= 0:
            raise ProfileError("reading_budget_exceeded")
        return min(5, left)

    try:
        directory = Path(tempfile.mkdtemp(prefix="qs-mongo-profile-"))
        os.chmod(directory, 0o700)
        key = directory / "ssh_key"
        write_owned(key, env["MONGO_BACKUP_SSH_KEY"].encode())
        stage = "ssh_keyscan"
        code, raw = capture_private(["/usr/bin/ssh-keyscan", "-T", "3", "-p", str(config["ssh_port"]), "-t", "ed25519,ecdsa,rsa", config["host"]], timeout=remaining())
        if code != 0:
            raise ProfileError("fingerprint_mismatch")
        stage = "ssh_fingerprint"
        known = match_known_hosts(raw, config["fingerprint"], directory, remaining)
        base = ssh_args(config, key, known)
        stage = "ssh_route"
        code, raw = capture_private([*base, "-G", config["host"]], timeout=remaining())
        if code != 0:
            raise ProfileError("ssh_route_invalid")
        validate_ssh(raw, config, key, known)
        stage = "port_reservation"
        with socket.socket() as reservation:
            reservation.bind(("127.0.0.1", 0))
            local_port = reservation.getsockname()[1]
        # ExitOnForwardFailure handles a race after reservation release. Only
        # this new process is ever signalled; foreign listeners are not touched.
        stage = "tunnel_start"
        tunnel = subprocess.Popen([*base, "-N", "-L", "127.0.0.1:" + str(local_port) + ":127.0.0.1:" + str(config["remote_port"]), config["host"]], env=child_env(), stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        stage = "tunnel_listener"
        deadline = time.monotonic() + remaining()
        while True:
            if tunnel.poll() is not None:
                raise ProfileError("tunnel_unavailable")
            try:
                with socket.create_connection(("127.0.0.1", local_port), timeout=0.1):
                    break
            except OSError:
                if time.monotonic() >= deadline:
                    raise ProfileError("tunnel_unavailable")
                time.sleep(0.05)
        stage = "tunnel_ownership"
        owned_listener(tunnel.pid, local_port, remaining())
        payload = json.dumps({"source_sha": config["sha"], "run_id": config["run"], "expected_target_hash": EXPECTED_TARGET, "source_binding_hash": source_hash, "tool_sha256": tool_hash, "port": local_port, "username": env["MONGODB_USERNAME"], "password": env["MONGODB_PASSWORD"], "database": "qs"}, separators=(",", ":")).encode()
        available = 40 - (time.monotonic() - start)
        if available <= 0:
            raise ProfileError("reading_budget_exceeded")
        stage = "native_reader"
        child_code, raw = capture_private([str(Path(binary))], payload=payload, timeout=min(35, available))
        value = safe_receipt(raw, child_code, config["sha"], config["run"], tool_hash, source_hash)
        if child_code != 0:
            failure_stage = stage
        value["release"]["reader_process_released"] = True
        stage = "source_recheck"
        if tunnel.poll() is not None or binary_hash(binary) != tool_hash:
            raise ProfileError("source_binding_failed")
        stage = "tunnel_recheck"
        owned_listener(tunnel.pid, local_port, remaining())
    except ProfileError as exc:
        failure_stage = stage
        value["error_category"] = str(exc) if str(exc) in ERRORS else "tool_failed"
        value["fcv_observation_complete"] = False
        child_code = 1
    except (OSError, ValueError, subprocess.SubprocessError, UnicodeError):
        failure_stage = stage
        value["error_category"] = "tool_failed"
        value["fcv_observation_complete"] = False
        child_code = 1
    finally:
        releases = value["release"]
        if tunnel is not None:
            try:
                if tunnel.poll() is None:
                    os.killpg(tunnel.pid, signal.SIGTERM)
                try:
                    tunnel.wait(timeout=1)
                except subprocess.TimeoutExpired:
                    os.killpg(tunnel.pid, signal.SIGKILL)
                    tunnel.wait(timeout=1)
                releases["tunnel_terminated"] = True
                releases["tunnel_waited"] = True
                releases["owned_tunnel_process_released"] = True
            except (OSError, subprocess.SubprocessError):
                pass
        if directory is not None:
            for filename, flag in (("ssh_key", "key_unlinked"), ("known_hosts", "known_hosts_unlinked")):
                try:
                    path = directory / filename
                    if path.exists():
                        path.unlink()
                    releases[flag] = not path.exists()
                except OSError:
                    pass
            try:
                directory.rmdir()
                releases["temp_directory_released"] = True
            except OSError:
                pass
        value["budget"]["elapsed_milliseconds"] = int((time.monotonic() - start) * 1000)
        if child_code == 0 and not all(releases.values()):
            failure_stage = "owned_resource_release"
            value["error_category"] = "owned_resource_release_failed"
            value["fcv_observation_complete"] = False
            child_code = 1
        if value["budget"]["elapsed_milliseconds"] > 45000:
            failure_stage = "total_budget"
            value["error_category"] = "reading_budget_exceeded"
            value["fcv_observation_complete"] = False
            child_code = 1
        value["complete"] = value["fcv_observation_complete"] and all(releases.values())
        value["eligible_fcv_readiness"] = value["complete"]
        value["failure_stage"] = failure_stage
    raw = json.dumps(value, separators=(",", ":")).encode()
    value = safe_receipt(raw, child_code, config["sha"], config["run"], tool_hash, source_hash, final=True)
    return (0 if value["complete"] else 1), value


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", required=True)
    args = parser.parse_args()
    env = {key: os.environ.get(key, "") for key in INPUT_ENV}
    sha, run = env.get("MONGO_PROFILE_SOURCE_SHA", ""), env.get("MONGO_PROFILE_RUN_ID", "")
    # A malformed binding is never published as an eligible receipt.
    if not SHA.fullmatch(sha) or not RUN.fullmatch(run):
        return 1
    try:
        armor = transport()
    except Exception:
        return 1
    def interrupted(_signum, _frame):
        raise ProfileError("interrupted")
    for signum in (signal.SIGHUP, signal.SIGTERM, signal.SIGINT):
        signal.signal(signum, interrupted)
    try:
        code, value = execute(args.binary, env)
    except ProfileError as exc:
        value = initial_receipt(sha, run)
        value["failure_stage"] = "preflight"
        value["error_category"] = str(exc) if str(exc) in ERRORS else "tool_failed"
        code = 1
    except Exception:
        value = initial_receipt(sha, run)
        value["failure_stage"] = "preflight"
        value["error_category"] = "input_invalid"
        code = 1
    secrets = [env.get(key, "") for key in ("MONGO_BACKUP_HOST", "MONGO_BACKUP_USERNAME", "MONGO_BACKUP_SSH_KEY", "MONGO_BACKUP_SSH_FINGERPRINT", "MONGODB_USERNAME", "MONGODB_PASSWORD")]
    try:
        require_fields(value, FINAL_RECEIPT_SCHEMA)
        print(armor.encode_armored_receipt(value, schema=FINAL_RECEIPT_SCHEMA, secrets=secrets))
    except Exception:
        return 1
    return code


if __name__ == "__main__":
    sys.exit(main())
