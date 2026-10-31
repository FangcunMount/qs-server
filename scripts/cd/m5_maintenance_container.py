"""Fixed QS maintenance transport; default only inspects/preflights Docker rights.

No database client, application entrypoint, migration or approval is run here.
An explicitly staged container retains its private files until reviewed cleanup.
"""
import argparse
import ipaddress
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile


SOURCE = "qs-apiserver"
LABEL = "com.fangcunmount.qs-m5.operation"
SLEEP = "exec sleep 2147483647"
ID = re.compile(r"[0-9a-f]{64}")
IMAGE = re.compile(r"sha256:[0-9a-f]{64}")
OPERATION = re.compile(r"[0-9]{1,20}-[0-9]{1,4}")
NAME = re.compile(r"qs-m5-maintenance-[0-9]{1,20}-[0-9]{1,4}")


def selected_environment(values):
    result = {}
    for value in values:
        if not isinstance(value, str):
            raise ValueError("invalid original environment")
        key, separator, content = value.partition("=")
        if not re.fullmatch(r"QS_APISERVER_(MYSQL|MONGODB)_[A-Z0-9_]+", key):
            continue
        if not separator or key in result or any(c in content for c in "\x00\r\n"):
            raise ValueError("database environment cannot be copied losslessly")
        result[key] = content
    for family in ("MYSQL", "MONGODB"):
        if result.get("QS_APISERVER_" + family + "_DATABASE") != "qs":
            raise ValueError("explicit original QS database required")
    return result


def source_configuration(source, expected_image, operation):
    if (not IMAGE.fullmatch(expected_image) or not OPERATION.fullmatch(operation)
            or not ID.fullmatch(source.get("Id", ""))
            or source.get("Name") != "/" + SOURCE or source.get("Image") != expected_image):
        raise ValueError("reviewed QS source identity required")
    networks = list(source.get("NetworkSettings", {}).get("Networks", {}))
    if (len(networks) != 1 or networks[0] in {"host", "none", "bridge"}
            or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,127}", networks[0])):
        raise ValueError("one explicit original application network required")
    if any(m.get("Destination") in {"/", "/tmp"} for m in source.get("Mounts", [])):
        raise ValueError("private maintenance filesystem must not be borrowed")
    return ("qs-m5-maintenance-" + operation, networks[0],
            selected_environment(source["Config"]["Env"]))


def create_arguments(source, image, operation, env_file):
    name, network, _ = source_configuration(source, image, operation)
    arguments = ["create", "--name", name, "--label", LABEL + "=" + operation,
                 "--label", "com.fangcunmount.qs-m5.source=" + source["Id"],
                 "--user", "2000:2000", "--network", network,
                 "--volumes-from", source["Id"] + ":ro", "--env-file", str(env_file),
                 "--entrypoint", "/bin/sh"]
    host = source.get("HostConfig", {})
    for address in host.get("Dns") or []:
        ipaddress.ip_address(address)
        arguments += ["--dns", address]
    for domain in host.get("DnsSearch") or []:
        if not re.fullmatch(r"[A-Za-z0-9.][A-Za-z0-9.-]{0,252}", domain):
            raise ValueError("unrecognized original DNS search domain")
        arguments += ["--dns-search", domain]
    for entry in host.get("ExtraHosts") or []:
        if not re.fullmatch(r"[A-Za-z0-9_.-]+:(?:[0-9a-fA-F:.]+|host-gateway)", entry):
            raise ValueError("unrecognized original host mapping")
        arguments += ["--add-host", entry]
    arguments += [image, "-c", SLEEP]
    return arguments


def inspect(execute, name, optional=False):
    result = execute(["inspect", name], optional=optional)
    if result.returncode:
        return None
    value = json.loads(result.stdout)
    if not isinstance(value, list) or len(value) != 1:
        raise ValueError("one Docker identity required")
    return value[0]


def prepare(image, operation, *, stage=False, docker_prefix=None):
    prefix = docker_prefix or ["sudo", "-n", shutil.which("docker") or "/usr/bin/docker"]

    def execute(arguments, optional=False):
        value = subprocess.run(prefix + arguments, stdout=subprocess.PIPE,
                               stderr=subprocess.DEVNULL, timeout=30, check=False)
        if value.returncode and not optional:
            raise ValueError("Docker maintenance operation refused")
        return value

    source = inspect(execute, SOURCE)
    name, network, environment = source_configuration(source, image, operation)
    if inspect(execute, name, optional=True) is not None:
        raise ValueError("existing maintenance state must be preserved")
    with tempfile.TemporaryDirectory(prefix="qs-m5-private-") as private:
        env_file = Path(private) / "database.env"
        descriptor = os.open(env_file, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "w") as out:
            out.write("".join(k + "=" + v + "\n" for k, v in sorted(environment.items())))
        arguments = create_arguments(source, image, operation, env_file)
        if not stage:
            # Rights checks execute no Docker create/start/exec operation. They
            # do not certify actual staging, application freeze or DB readiness.
            for command in (arguments, ["start", name],
                            ["exec", "--user", "2000:2000", name, "sh", "-c", "true"]):
                permission = subprocess.run(["sudo", "-n", "-l", "--", prefix[-1]] + command,
                                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                            timeout=10, check=False)
                if permission.returncode:
                    raise ValueError("required maintenance Docker permission unavailable")
        else:
            created = execute(arguments).stdout.decode().strip()
            if not ID.fullmatch(created):
                raise ValueError("maintenance creation outcome unknown; preserve state")
            # Never remove/reuse an old operation or erase an approval marker.
            # A staging error deliberately retains this newly owned container.
            actual = inspect(execute, created)
            if (actual["Image"] != image or actual["Config"]["User"] != "2000:2000"
                    or actual["Config"]["Entrypoint"] != ["/bin/sh"]
                    or actual["Config"]["Cmd"] != ["-c", SLEEP]
                    or actual["Config"]["Labels"].get(LABEL) != operation
                    or any(m.get("RW") for m in actual.get("Mounts", []))):
                raise ValueError("maintenance shape mismatch; preserve owned container")
            if inspect(execute, SOURCE)["Id"] != source["Id"]:
                raise ValueError("original source changed; preserve owned container")
            execute(["start", created])
            if not inspect(execute, created)["State"]["Running"]:
                raise ValueError("maintenance shell not running; preserve owned container")
    return dict(read_only_preflight=not stage, container_created=stage,
                maintenance_container=name, source_container_id=source["Id"],
                source_image_id=image, network=network,
                database_environment_names=sorted(environment),
                application_started=False, database_queried=False,
                database_mutation=False, runtime_frozen=False,
                source_was_running=source["State"]["Running"])


def resolve_inventory_target(owner, default):
    """Explicit optional transport, validated with the existing SSH identity."""
    name = os.environ.get("QS_MAINTENANCE_CONTAINER", "")
    if not name:
        return default
    if not NAME.fullmatch(name):
        raise ValueError("owned maintenance name required")
    if owner not in {"qs.mysql", "qs.mongo"}:
        return default
    expected = os.environ.get("QS_MAINTENANCE_IMAGE_ID", "")
    if not IMAGE.fullmatch(expected):
        raise ValueError("reviewed original image required")
    import shlex
    value = subprocess.run(["ssh", "-F", os.environ["RUNNER_SSH_CONFIG"],
                            os.environ["RUNNER_SSH_ALIAS"],
                            shlex.join(["sudo", "-n", "docker", "inspect", name])],
                           stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=30)
    if value.returncode:
        raise ValueError("maintenance identity unavailable")
    actual = json.loads(value.stdout)
    if not isinstance(actual, list) or len(actual) != 1:
        raise ValueError("exact maintenance identity required")
    actual = actual[0]
    operation = name.removeprefix("qs-m5-maintenance-")
    if not isinstance(actual, dict) or not isinstance(actual.get("Config"), dict):
        raise ValueError("invalid maintenance metadata")
    if (actual.get("Name") != "/" + name or actual.get("Image") != expected
            or not actual.get("State", {}).get("Running")
            or actual.get("Config", {}).get("User") != "2000:2000"
            or actual["Config"].get("Entrypoint") != ["/bin/sh"]
            or actual["Config"].get("Cmd") != ["-c", SLEEP]
            or actual["Config"].get("Labels", {}).get(LABEL) != operation
            or any(m.get("RW") for m in actual.get("Mounts", []))):
        raise ValueError("maintenance process/image/ownership mismatch")
    return name


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--expected-image-id", required=True)
    parser.add_argument("--operation-id", required=True)
    parser.add_argument("--stage", action="store_true",
                        help="explicitly create/start only an idle maintenance container")
    args = parser.parse_args()
    try:
        result = prepare(args.expected_image_id, args.operation_id, stage=args.stage)
    except (ValueError, KeyError, TypeError, OSError, subprocess.TimeoutExpired):
        raise SystemExit("QS maintenance transport refused; preserve any existing operation") from None
    print(json.dumps(result, sort_keys=True))


if __name__ == "__main__":
    main()
