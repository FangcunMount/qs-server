#!/usr/bin/env python3
"""One bounded metadata diagnostic over the existing root@001-ysj SSH alias."""
import argparse
import base64
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import platform
import shlex
import signal
import socket
import stat
import sys
import tarfile
import tempfile

MEMBERS = frozenset(("profile-linux-amd64", "profile-linux-arm64", "mysql-metadata-profile.py",
                     "cbpt-cleanup.py", "receipt-transport.py"))
ARCHIVE_LIMIT = 32 * 1024 * 1024
HEADER_LIMIT = 32768
ALIAS = "001-ysj"


def load_profile(directory):
    spec = importlib.util.spec_from_file_location("owned_mysql_metadata_profile", directory / "mysql-metadata-profile.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def unique_object(pairs):
    value = {}
    for key, item in pairs:
        if key in value:
            raise ValueError("duplicate_input_key")
        value[key] = item
    return value


def validate_envelope(value):
    if not isinstance(value, dict) or set(value) != {"env", "package_sha256", "package_size"}:
        raise ValueError("envelope_fields")
    env = value["env"]
    keys = {"MYSQL_HOST", "MYSQL_PORT", "MYSQL_USERNAME", "MYSQL_PASSWORD", "MYSQL_DATABASE",
            "PROFILE_SOURCE_SHA", "PROFILE_RUN_ID", "PROFILE_EXPECTED_TARGET_HASH"}
    if not isinstance(env, dict) or set(env) != keys or any(not isinstance(v, str) or len(v) > 4096 for v in env.values()):
        raise ValueError("environment_fields")
    import re
    if not isinstance(value["package_sha256"], str) or not re.fullmatch(r"[0-9a-f]{64}", value["package_sha256"]) or type(value["package_size"]) is not int or not 0 < value["package_size"] <= ARCHIVE_LIMIT:
        raise ValueError("package_binding")
    for key, expression in (("PROFILE_SOURCE_SHA", r"[0-9a-f]{40}"), ("PROFILE_RUN_ID", r"[0-9]{1,20}-[0-9]{1,4}"),
                            ("PROFILE_EXPECTED_TARGET_HASH", r"[0-9a-f]{64}")):
        if not re.fullmatch(expression, env[key]):
            raise ValueError("source_binding")
    if any(any(c in v for c in ("\x00", "\r", "\n")) for v in env.values()):
        raise ValueError("environment_control_character")
    return env


def extract_archive(raw, directory):
    # Do not call tar.extract: only exact, unique plain files can enter this owned directory.
    with tarfile.open(fileobj=io.BytesIO(raw), mode="r:gz") as archive:
        seen, total = set(), 0
        for member in archive:
            if member.name not in MEMBERS or member.name in seen or not member.isfile() or member.size < 1:
                raise ValueError("package_member")
            total += member.size
            if total > ARCHIVE_LIMIT:
                raise ValueError("package_expanded_limit")
            seen.add(member.name)
            mode = 0o700 if member.name.startswith("profile-linux-") else 0o600
            fd = os.open(directory / member.name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, mode)
            with os.fdopen(fd, "wb") as target, archive.extractfile(member) as source:
                remaining = member.size
                while remaining:
                    block = source.read(min(65536, remaining))
                    if not block:
                        raise ValueError("package_truncated")
                    target.write(block)
                    remaining -= len(block)
                target.flush()
                os.fsync(target.fileno())
        if seen != MEMBERS:
            raise ValueError("package_missing_member")


def remove_owned(directory):
    # A symlink created by an unexpected child is unlinked, never traversed.
    for name in (*sorted(MEMBERS), "profile.env.json"):
        path = directory / name
        if path.exists() or path.is_symlink():
            path.unlink()
    directory.rmdir()


def receiver_arch():
    if os.getuid() != 0 or platform.system() != "Linux" or socket.gethostname().lower() != "servera":
        raise ValueError("receiver_host")
    arch = {"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(platform.machine())
    if not arch:
        raise ValueError("receiver_arch")
    return arch


def receive(stream):
    arch = receiver_arch()
    line = stream.readline(HEADER_LIMIT + 1)
    if len(line) > HEADER_LIMIT or not line.endswith(b"\n"):
        raise ValueError("envelope_limit")
    envelope = json.loads(line, object_pairs_hook=unique_object)
    env = validate_envelope(envelope)
    raw = stream.read(envelope["package_size"] + 1)
    if len(raw) != envelope["package_size"] or hashlib.sha256(raw).hexdigest() != envelope["package_sha256"]:
        raise ValueError("package_checksum")
    directory = Path(tempfile.mkdtemp(prefix="qs-mysql-metadata-profile-", dir="/tmp"))
    os.chmod(directory, 0o700)
    def interrupted(_signum, _frame):
        raise RuntimeError("receiver_interrupted")
    previous = {number: signal.getsignal(number) for number in (signal.SIGHUP, signal.SIGTERM, signal.SIGINT)}
    for number in previous:
        signal.signal(number, interrupted)
    try:
        extract_archive(raw, directory)
        profile = load_profile(directory)
        runtime = profile.runtime_module()
        runtime.validate_source(env)
        env_file = directory / "profile.env.json"
        runtime.json_private(env_file, env)
        # Re-read only our owned 0600 file. No shell source/eval and no credential argv.
        private = runtime.load_json_bytes(runtime.private_file(env_file, limit=HEADER_LIMIT))
        env_file.unlink()  # Credentials are RAM-only before any database connection.
        code, value = profile.execute_native(str(directory / ("profile-linux-" + arch)), private, runtime)
        output = profile.receipt_transport_module().encode_armored_receipt(
            value, schema=profile.RECEIPT_SCHEMA, secrets=(env["MYSQL_USERNAME"], env["MYSQL_PASSWORD"]))
    finally:
        # No positive output before all task-owned files have been released.
        try:
            remove_owned(directory)
        finally:
            for number, handler in previous.items():
                signal.signal(number, handler)
    return code, output


def trusted_ssh(profile, home):
    known = home / ".ssh" / "known_hosts"
    if known.is_symlink() or not known.is_file() or known.stat().st_uid != os.getuid() or stat.S_IMODE(known.stat().st_mode) & 0o022:
        raise ValueError("known_hosts_unavailable")
    options = ["-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "IdentitiesOnly=yes",
               "-o", "UserKnownHostsFile=" + str(known), "-o", "GlobalKnownHostsFile=/dev/null",
               "-o", "ConnectTimeout=10", "-o", "ConnectionAttempts=1", "-o", "PasswordAuthentication=no",
               "-o", "KbdInteractiveAuthentication=no", "-o", "ControlMaster=no", "-o", "ControlPath=none",
               "-o", "ClearAllForwardings=yes", "-o", "ForwardAgent=no", "-o", "ForwardX11=no",
               "-o", "ForwardX11Trusted=no", "-o", "PermitLocalCommand=no", "-o", "LogLevel=ERROR"]
    local = {"HOME": str(home), "PATH": "/usr/bin:/bin:/usr/sbin:/sbin"}
    agent = os.environ.get("SSH_AUTH_SOCK", "")
    if agent:
        info = Path(agent).stat()
        if not stat.S_ISSOCK(info.st_mode) or info.st_uid != os.getuid():
            raise ValueError("ssh_agent_binding")
        local["SSH_AUTH_SOCK"] = agent
    code, raw = profile.capture_private(["/usr/bin/ssh", "-G", *options, "-l", "root", ALIAS], env=local, timeout=10)
    if code != 0:
        raise ValueError("ssh_alias_unavailable")
    config = {}
    for line in raw.decode().splitlines():
        key, _, value = line.partition(" ")
        config.setdefault(key, []).append(value)
    if config.get("user") != ["root"] or config.get("hostname") in (None, [ALIAS]) or config.get("proxycommand", ["none"]) != ["none"] or config.get("proxyjump", ["none"]) != ["none"] or config.get("hostkeyalias", ["none"]) != ["none"]:
        raise ValueError("ssh_alias_binding")
    host = config["hostname"][0]
    port = config.get("port", ["22"])[0]
    host_key_name = host if port == "22" else "[" + host + "]:" + port
    code, raw = profile.capture_private(["/usr/bin/ssh-keygen", "-F", host_key_name, "-f", str(known)], env=local, timeout=10)
    if code != 0 or not any(line and not line.startswith(b"#") for line in raw.splitlines()):
        raise ValueError("ssh_host_key_unavailable")
    return ["/usr/bin/ssh", "-T", *options, "-l", "root", ALIAS], local


def dispatch(archive, env, profile, home):
    if env.get("PROFILE_OPERATION") not in ("metadata-visibility-profile", "metadata-visibility-admin-profile") or env.get("PROFILE_DATABASE") != "mysql" or env.get("PROFILE_ARCHIVE_INPUT", ""):
        raise ValueError("operation_binding")
    private = {key: env.get(key, "") for key in profile.PROFILE_KEYS}
    private["MYSQL_PORT"] = private["MYSQL_PORT"] or "3306"
    profile.runtime_module().validate_source(private)
    path = Path(archive)
    if path.is_symlink() or not path.is_file() or path.stat().st_uid != os.getuid() or stat.S_IMODE(path.stat().st_mode) != 0o600 or path.stat().st_size > ARCHIVE_LIMIT:
        raise ValueError("local_package_private")
    raw = path.read_bytes()
    envelope = {"env": private, "package_sha256": env.get("PROFILE_PACKAGE_SHA256", ""), "package_size": len(raw)}
    validate_envelope(envelope)
    if hashlib.sha256(raw).hexdigest() != envelope["package_sha256"]:
        raise ValueError("local_package_checksum")
    # Public receiver code only enters argv; package and secrets travel solely over stdin.
    receiver = base64.b64encode(Path(__file__).read_bytes()).decode("ascii")
    command = "/usr/bin/python3 -B -c " + shlex.quote("import base64;exec(compile(base64.b64decode('" + receiver + "'),'<metadata-receiver>','exec'))") + " --receive"
    ssh, local = trusted_ssh(profile, home)
    payload = json.dumps(envelope, separators=(",", ":"), ensure_ascii=True).encode() + b"\n" + raw
    if len(payload) - len(raw) > HEADER_LIMIT:
        raise ValueError("envelope_limit")
    code, sealed = profile.capture_private([*ssh, command], env=local, payload=payload, timeout=90, limit=262144)
    value = profile.safe_receipt(profile.runtime_module(), profile.receipt_transport_module().decode_armored_receipt(sealed.decode()),
                                 code, private["PROFILE_SOURCE_SHA"], private["PROFILE_RUN_ID"], private["PROFILE_EXPECTED_TARGET_HASH"])
    return code, profile.receipt_transport_module().encode_armored_receipt(value, schema=profile.RECEIPT_SCHEMA,
                                                                        secrets=(private["MYSQL_USERNAME"], private["MYSQL_PASSWORD"]))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--archive")
    parser.add_argument("--receive", action="store_true")
    args = parser.parse_args()
    if args.receive:
        try:
            code, output = receive(sys.stdin.buffer)
        except Exception:
            # Public failure is generated by the local caller; no exception/raw child output.
            return 1
    else:
        try:
            profile = load_profile(Path(__file__).parent)
        except Exception:
            return 1
        try:
            code, output = dispatch(args.archive, os.environ, profile, Path.home())
        except Exception:
            try:
                output = profile.receipt_transport_module().encode_armored_receipt(dict(profile.FAILURE_RECEIPT),
                    schema=profile.RECEIPT_SCHEMA, secrets=(os.environ.get("MYSQL_USERNAME", ""), os.environ.get("MYSQL_PASSWORD", "")))
            except Exception:
                return 1
            code = 1
    print(output)
    return code


if __name__ == "__main__":
    sys.exit(main())
