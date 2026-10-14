#!/usr/bin/env python3
"""Fixed-target Mongo FCV observation using the existing trusted ServerC SSH route.

No backup, retention, production mutation or privilege change is implemented.
Only this invocation's tunnel and native reader process are owned here.
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
import time

EXPECTED_TARGET = "ab3c60ccc9106a110d85946468f0cec60a18614e54fde49dadfa8701a362036f"
SHA = re.compile(r"^[0-9a-f]{40}$")
HASH = re.compile(r"^[0-9a-f]{64}$")
RUN = re.compile(r"^[0-9]{1,20}-[0-9]{1,4}$")
SSH_ALIAS = "003-ysj"
SSH_HOST = "81.70.102.15"
SSH_PORT = 22
REMOTE_PORT = 27017
UNOWNED_MATERIAL = frozenset({"key_unlinked", "known_hosts_unlinked", "temp_directory_released"})
OWNED_RELEASE = frozenset({"cursors_closed", "client_disconnected", "tunnel_terminated", "tunnel_waited", "owned_tunnel_process_released", "reader_process_released"})
INPUT_ENV = ("MONGO_PROFILE_SOURCE_SHA", "MONGO_PROFILE_CONFIRM_SHA", "MONGO_PROFILE_RUN_ID", "MONGO_PROFILE_EXPECTED_TARGET_HASH", "MONGO_PROFILE_OPERATION", "MONGO_PROFILE_DATABASE", "MONGO_PROFILE_ARCHIVE_INPUT", "GITHUB_SHA", "MONGODB_PORT", "MONGODB_USERNAME", "MONGODB_PASSWORD", "MONGODB_DBNAME")
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
# receipt has a distinct V2 boundary: existing keys/trust files are borrowed,
# never created or removed, and their owned-material release fields are null.
# No command, address or stderr is published.
TRANSPORT_STAGES = frozenset({"none", "preflight", "trusted_ssh", "port_reservation", "tunnel_start", "tunnel_listener", "tunnel_ownership", "native_reader", "source_recheck", "tunnel_recheck", "owned_resource_release", "total_budget"})
FINAL_RECEIPT_SCHEMA = {**RECEIPT_SCHEMA, "protocol": frozenset({"mongo_admin_metadata_transport_v2"}),
                        "release": {**RECEIPT_SCHEMA["release"], **{key: "nullable_bool" for key in UNOWNED_MATERIAL}},
                        "failure_stage": TRANSPORT_STAGES}


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


def final_receipt(value):
    value["protocol"] = "mongo_admin_metadata_transport_v2"
    for key in UNOWNED_MATERIAL:
        value["release"][key] = None
    value["failure_stage"] = "none"
    return value


def owned_released(value):
    return all(value["release"][key] for key in OWNED_RELEASE)


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
            if any(value["release"][key] is not None for key in UNOWNED_MATERIAL):
                raise ProfileError("receipt_invalid")
            eligible = observed and owned_released(value)
            if value["complete"] != eligible or value["eligible_fcv_readiness"] != eligible or (code == 0) != eligible:
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
    if not 1 <= len(env.get("MONGODB_USERNAME", "")) <= 256 or any(c in env.get("MONGODB_USERNAME", "") for c in "\0\r\n") or not 1 <= len(env.get("MONGODB_PASSWORD", "")) <= 4096 or port(env.get("MONGODB_PORT", ""), REMOTE_PORT) != REMOTE_PORT:
        raise ProfileError("input_invalid")
    return {"sha": sha, "run": run, "host": SSH_HOST, "account": "root", "ssh_port": SSH_PORT, "remote_port": REMOTE_PORT}


def child_env():
    return {"PATH": "/usr/bin:/bin", "LANG": "C", "LC_ALL": "C"}


def capture_private(args, *, payload=None, timeout=5, limit=65536, env=None):
    process = subprocess.Popen(args, env=child_env() if env is None else env, stdin=subprocess.PIPE if payload is not None else subprocess.DEVNULL,
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


def trusted_file(path, limit):
    before = path.lstat()
    if not stat.S_ISREG(before.st_mode) or before.st_uid != os.getuid() or before.st_mode & 0o022 or not 0 < before.st_size <= limit:
        raise ProfileError("ssh_route_invalid")
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd, "rb") as stream:
        if os.fstat(stream.fileno()) != before:
            raise ProfileError("ssh_route_invalid")
        raw = stream.read(limit + 1)
        if len(raw) > limit or os.fstat(stream.fileno()) != before:
            raise ProfileError("ssh_route_invalid")
    return hashlib.sha256(raw).hexdigest()


def validate_ssh(raw):
    try:
        values = {}
        for line in raw.decode("utf-8").splitlines():
            field, value = line.split(None, 1)
            values.setdefault(field, []).append(value)
        for field, expected in (("hostname", SSH_HOST), ("user", "root"), ("port", str(SSH_PORT))):
            if values.get(field) != [expected]:
                raise ProfileError("ssh_route_invalid")
        for field in ("proxycommand", "proxyjump", "hostkeyalias", "localcommand"):
            if values.get(field, ["none"]) != ["none"]:
                raise ProfileError("ssh_route_invalid")
        if any(field in values for field in ("localforward", "remoteforward", "dynamicforward")):
            raise ProfileError("ssh_route_invalid")
        return values
    except (ValueError, UnicodeError):
        raise ProfileError("ssh_route_invalid") from None


def trusted_ssh(home, remaining):
    known = home / ".ssh" / "known_hosts"
    known_hash = trusted_file(known, 4 << 20)
    local = {**child_env(), "HOME": str(home)}
    agent = os.environ.get("SSH_AUTH_SOCK", "")
    if agent:
        info = Path(agent).lstat()
        if not Path(agent).is_absolute() or not stat.S_ISSOCK(info.st_mode) or info.st_uid != os.getuid():
            raise ProfileError("ssh_route_invalid")
        local["SSH_AUTH_SOCK"] = agent
    options = ["-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "IdentitiesOnly=yes",
               "-o", "UserKnownHostsFile=" + str(known), "-o", "GlobalKnownHostsFile=/dev/null",
               "-o", "UpdateHostKeys=no", "-o", "VerifyHostKeyDNS=no", "-o", "CanonicalizeHostname=no",
               "-o", "AddKeysToAgent=no",
               "-o", "ConnectTimeout=5", "-o", "ConnectionAttempts=1", "-o", "PasswordAuthentication=no",
               "-o", "KbdInteractiveAuthentication=no", "-o", "ControlMaster=no", "-o", "ControlPath=none",
               "-o", "ControlPersist=no", "-o", "ForwardAgent=no", "-o", "ForwardX11=no",
               "-o", "ForwardX11Trusted=no", "-o", "PermitLocalCommand=no", "-o", "LogLevel=ERROR",
               "-o", "ExitOnForwardFailure=yes", "-o", "IdentityAgent=" + (agent or "none")]
    code, raw = capture_private(["/usr/bin/ssh", "-G", *options, "-l", "root", "-p", str(SSH_PORT), SSH_ALIAS], env=local, timeout=remaining())
    if code != 0:
        raise ProfileError("ssh_route_invalid")
    values = validate_ssh(raw)
    # Freeze only the approved alias's existing identity references. The final
    # connection does not re-parse mutable config or inherit any forwarding.
    identities = values.get("identityfile", [])
    if not identities or len(identities) > 32 or any(not path or "\0" in path or "%" in path or path == "none" for path in identities):
        raise ProfileError("ssh_route_invalid")
    identity_options = []
    for path in identities:
        identity_options.extend(["-i", path])
    # $SSH_SK_PROVIDER is OpenSSH's default token, but our SSH environment
    # never imports that variable. Freeze the native default, not a provider.
    if values.get("certificatefile", ["none"]) != ["none"] or values.get("pkcs11provider", ["none"]) != ["none"] or values.get("securitykeyprovider", ["internal"]) not in (["internal"], ["none"], ["$SSH_SK_PROVIDER"]):
        raise ProfileError("ssh_route_invalid")
    options.extend(["-o", "SecurityKeyProvider=internal"])
    code, raw = capture_private(["/usr/bin/ssh-keygen", "-F", SSH_HOST, "-f", str(known)], env=local, timeout=remaining())
    if code != 0 or not any(line and not line.startswith(b"#") for line in raw.splitlines()):
        raise ProfileError("ssh_route_invalid")
    if trusted_file(known, 4 << 20) != known_hash:
        raise ProfileError("ssh_route_invalid")
    base = ["/usr/bin/ssh", "-F", "/dev/null", "-T", *options, *identity_options, "-l", "root", "-p", str(SSH_PORT)]
    return base, local, known, known_hash


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
    value = final_receipt(initial_receipt(config["sha"], config["run"], tool_hash))
    tunnel = None
    child_code = 1
    source_hash = "0" * 64
    stage, failure_stage = "trusted_ssh", "none"

    def remaining():
        left = 40 - (time.monotonic() - start)
        if left <= 0:
            raise ProfileError("reading_budget_exceeded")
        return min(5, left)

    try:
        base, ssh_env, known, known_hash = trusted_ssh(Path.home(), remaining)
        binding = {key: config[key] for key in ("host", "account", "ssh_port", "remote_port")}
        binding.update(alias=SSH_ALIAS, known_hosts_sha256=known_hash, expected_target_hash=EXPECTED_TARGET)
        source_hash = hashlib.sha256(json.dumps(binding, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
        value["source_binding_hash"] = source_hash
        stage = "port_reservation"
        with socket.socket() as reservation:
            reservation.bind(("127.0.0.1", 0))
            local_port = reservation.getsockname()[1]
        # ExitOnForwardFailure handles a race after reservation release. Only
        # this new process is ever signalled; foreign listeners are not touched.
        stage = "tunnel_start"
        tunnel = subprocess.Popen([*base, "-N", "-L", "127.0.0.1:" + str(local_port) + ":127.0.0.1:" + str(config["remote_port"]), config["host"]], env=ssh_env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
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
        value = final_receipt(safe_receipt(raw, child_code, config["sha"], config["run"], tool_hash, source_hash))
        if child_code != 0:
            failure_stage = stage
        value["release"]["reader_process_released"] = True
        stage = "source_recheck"
        if tunnel.poll() is not None or binary_hash(binary) != tool_hash or trusted_file(known, 4 << 20) != known_hash:
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
        value["budget"]["elapsed_milliseconds"] = int((time.monotonic() - start) * 1000)
        if child_code == 0 and not owned_released(value):
            failure_stage = "owned_resource_release"
            value["error_category"] = "owned_resource_release_failed"
            value["fcv_observation_complete"] = False
            child_code = 1
        if value["budget"]["elapsed_milliseconds"] > 45000:
            failure_stage = "total_budget"
            value["error_category"] = "reading_budget_exceeded"
            value["fcv_observation_complete"] = False
            child_code = 1
        value["complete"] = value["fcv_observation_complete"] and owned_released(value)
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
        value = final_receipt(initial_receipt(sha, run))
        value["failure_stage"] = "preflight"
        value["error_category"] = str(exc) if str(exc) in ERRORS else "tool_failed"
        code = 1
    except Exception:
        value = final_receipt(initial_receipt(sha, run))
        value["failure_stage"] = "preflight"
        value["error_category"] = "input_invalid"
        code = 1
    secrets = [env.get(key, "") for key in ("MONGODB_USERNAME", "MONGODB_PASSWORD")]
    try:
        require_fields(value, FINAL_RECEIPT_SCHEMA)
        print(armor.encode_armored_receipt(value, schema=FINAL_RECEIPT_SCHEMA, secrets=secrets))
    except Exception:
        return 1
    return code


if __name__ == "__main__":
    sys.exit(main())
