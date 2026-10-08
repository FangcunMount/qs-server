#!/usr/bin/env python3
"""Read fixed Docker projections and readiness; never emits raw process output."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import posixpath
import re
import subprocess
import sys

SHA = re.compile(r"^[0-9a-f]{40}$")
HASH = re.compile(r"^[0-9a-f]{64}$")
IMAGE = re.compile(r"^sha256:[0-9a-f]{64}$")
RUN = re.compile(r"^[1-9][0-9]{0,19}$")
ATTEMPT = re.compile(r"^[1-9][0-9]{0,3}$")
ROLES = {
    "apiserver": ("qs-apiserver", "qs-apiserver", "/app/qs-apiserver", 8080, "/readyz"),
    "collection": ("qs-collection", "server", "/app/collection-server", 8080, "/serve-readyz"),
    "worker": ("qs-worker", "runtime", "/app/qs-worker", 9092, "/readyz"),
}
VERSION_CONFIGS = {"apiserver": "/app/configs/apiserver.prod.yaml", "collection": "/app/configs/collection-server.prod.yaml", "worker": "/app/configs/worker.prod.yaml"}
ERRORS = frozenset({"none", "input_binding_invalid", "docker_command_failed", "docker_command_refused", "docker_output_rejected",
    "instance_set_invalid", "container_projection_invalid", "container_not_running", "topology_mismatch",
    "image_tag_mismatch", "image_config_binding_mismatch", "receipt_binding_invalid", "entrypoint_mismatch", "binary_shadowed", "binary_modified", "binary_hash_mismatch",
    "version_output_rejected", "version_mismatch", "readiness_failed", "runtime_changed", "transport_failed"})
LIST_TEMPLATE = '{"container_id":{{json .ID}},"name":{{json .Names}},"project":{{json (.Label "com.docker.compose.project")}},"service":{{json (.Label "com.docker.compose.service")}}}'
INSPECT_TEMPLATE = '{"container_id":{{json .Id}},"name":{{json .Name}},"image_id":{{json .Image}},"image_reference":{{json .Config.Image}},"entrypoint":{{json .Config.Entrypoint}},"path":{{json .Path}},"status":{{json .State.Status}},"running":{{json .State.Running}},"dead":{{json .State.Dead}},"restarting":{{json .State.Restarting}},"started_at":{{json .State.StartedAt}},"finished_at":{{json .State.FinishedAt}},"restart_count":{{json .RestartCount}},"project":{{json (index .Config.Labels "com.docker.compose.project")}},"service":{{json (index .Config.Labels "com.docker.compose.service")}},"number":{{json (index .Config.Labels "com.docker.compose.container-number")}},"oneoff":{{json (index .Config.Labels "com.docker.compose.oneoff")}},"mount_destinations":[{{range $i,$m := .Mounts}}{{if $i}},{{end}}{{json $m.Destination}}{{end}}]}'
INSPECT_KEYS = frozenset(("container_id", "name", "image_id", "image_reference", "entrypoint", "path", "status", "running", "dead", "restarting", "started_at", "finished_at", "restart_count", "project", "service", "number", "oneoff", "mount_destinations"))
IMAGE_TEMPLATE = '{"image_id":{{json .Id}},"entrypoint":{{json .Config.Entrypoint}}}'


class Refused(ValueError):
    pass


def refuse(category):
    raise Refused(category)


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode()).hexdigest()


def unique(pairs):
    result = {}
    for key, value in pairs:
        if key in result:refuse("docker_output_rejected")
        result[key] = value
    return result


def load(raw):
    try:return json.loads(raw, object_pairs_hook=unique)
    except Refused:raise
    except Exception:refuse("docker_output_rejected")


class Docker:
    def __init__(self, sudo_docker=False):
        if type(sudo_docker) is not bool:refuse("input_binding_invalid")
        self.sudo_docker = sudo_docker

    def capture(self, args, *, timeout=15, maximum=256*1024, failure="docker_command_failed", discard_stderr=False):
        if not readonly_docker_command(args):refuse("docker_command_refused")
        prefix, private_input = ["docker"], {}
        if self.sudo_docker:
            password = os.environ.get("SUDO_PASSWORD", "")
            if password:
                try:encoded = password.encode("utf-8", errors="strict")
                except UnicodeError:refuse("input_binding_invalid")
                if len(encoded) > 4096 or any(value in encoded for value in (b"\n", b"\r", b"\x00")):
                    refuse("input_binding_invalid")
                # Preserve the host's password-based sudo mode without relying
                # on the parent shell's ticket. Only fixed read-only Docker
                # commands receive stdin; neither Python nor argv is privileged.
                prefix = ["sudo", "-S", "-p", "", "docker"]
                private_input = {"input": encoded + b"\n"}
            else:
                prefix = ["sudo", "-n", "docker"]
        try:
            result = subprocess.run([*prefix, *args], stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout, check=False, **private_input)
        except Exception:refuse(failure)
        if result.returncode or (result.stderr and not discard_stderr):refuse(failure)
        if len(result.stdout) + len(result.stderr) > maximum:refuse("docker_output_rejected")
        try:return result.stdout.decode("utf-8", errors="strict")
        except UnicodeError:refuse("docker_output_rejected")

    def listing(self):
        raw = self.capture(["ps", "--all", "--no-trunc", "--format", LIST_TEMPLATE])
        rows = [load(line) for line in raw.splitlines()]
        if len(rows) > 4096:refuse("instance_set_invalid")
        for row in rows:
            if type(row) is not dict or set(row) != {"container_id", "name", "project", "service"} or any(type(v) is not str for v in row.values()) or not HASH.fullmatch(row["container_id"]):refuse("docker_output_rejected")
        if len({r["container_id"] for r in rows}) != len(rows):refuse("instance_set_invalid")
        return rows

    def inspect(self, container):
        value = load(self.capture(["inspect", "--type", "container", "--format", INSPECT_TEMPLATE, container], maximum=64*1024))
        if type(value) is not dict or set(value) != INSPECT_KEYS:refuse("container_projection_invalid")
        return value


def readonly_docker_command(args):
    # This executor cannot run a mutation or an arbitrary root command even if
    # a future caller accidentally tries to pass one through capture().
    if type(args) is not list or any(type(arg) is not str for arg in args):return False
    if args == ["ps", "--all", "--no-trunc", "--format", LIST_TEMPLATE]:return True
    if len(args) == 6 and args[:5] == ["inspect", "--type", "container", "--format", INSPECT_TEMPLATE]:return bool(HASH.fullmatch(args[5]))
    if len(args) == 5 and args[:4] == ["image", "inspect", "--format", IMAGE_TEMPLATE]:return bool(IMAGE.fullmatch(args[4]))
    if len(args) == 2 and args[0] == "diff":return bool(HASH.fullmatch(args[1]))
    if len(args) == 7 and args[:3] == ["exec", "--user", "0"] and HASH.fullmatch(args[3]) and args[4:6] == ["sha256sum", "/proc/1/exe"]:
        return args[6] in {role[2] for role in ROLES.values()}
    if len(args) >= 2 and args[0] == "exec" and HASH.fullmatch(args[1]):
        for role, spec in ROLES.items():
            if args[2:] == [spec[2], "--version=true", "--config="+VERSION_CONFIGS[role]]:return True
            if args[2:] == ["wget", "-q", "-O", "/dev/null", "-T", "5", "http://127.0.0.1:"+str(spec[3])+spec[4]]:return True
    return False


def select_instances(rows, role, expected):
    project, service, _, _, _ = ROLES[role]
    def relevant(row):
        if role == "apiserver":return row["name"] == "qs-apiserver" or row["service"] == service
        if role == "collection":return row["project"] == project or row["name"] == "qs-collection-server" or row["name"].startswith("qs-deploy-collection-")
        return row["project"] == project or row["name"] == "qs-worker" or row["name"].startswith("qs-deploy-worker-")
    chosen = sorted((row for row in rows if relevant(row)), key=lambda row: row["container_id"])
    if len(chosen) != expected or len({r["name"] for r in chosen}) != expected:refuse("instance_set_invalid")
    for row in chosen:
        if row["service"] != service or (role != "apiserver" and row["project"] != project) or (role == "apiserver" and row["name"] != "qs-apiserver"):refuse("topology_mismatch")
    return chosen


def validate_projection(value, row, role, source, expected_image_config_id=None):
    project, service, binary, _, _ = ROLES[role]
    strings = INSPECT_KEYS - {"entrypoint", "running", "dead", "restarting", "restart_count", "mount_destinations"}
    if any(type(value[k]) is not str for k in strings) or any(type(value[k]) is not bool for k in ("running", "dead", "restarting")) or type(value["restart_count"]) is not int or not 0 <= value["restart_count"] < 2**64:refuse("container_projection_invalid")
    if value["container_id"] != row["container_id"] or value["name"] != "/"+row["name"] or value["project"] != row["project"] or value["service"] != service or not IMAGE.fullmatch(value["image_id"]):refuse("topology_mismatch")
    if not value["running"] or value["dead"] or value["restarting"] or value["status"] != "running":refuse("container_not_running")
    if expected_image_config_id is not None:
        if value["image_id"] != expected_image_config_id or value["image_reference"] != expected_image_config_id:refuse("image_config_binding_mismatch")
    elif not value["image_reference"].endswith(":" + source) or "@" in value["image_reference"]:refuse("image_tag_mismatch")
    if value["entrypoint"] != [binary] or value["path"] != binary:refuse("entrypoint_mismatch")
    if value["oneoff"].lower() != "false" or (role != "apiserver" and not re.fullmatch(r"[1-9][0-9]{0,2}", value["number"])):refuse("topology_mismatch")
    mounts = value["mount_destinations"]
    if type(mounts) is not list or any(type(m) is not str or not m.startswith("/") for m in mounts):refuse("container_projection_invalid")
    for mount in mounts:
        path = posixpath.normpath("/"+mount.lstrip("/"))
        if path == binary or binary.startswith(path.rstrip("/")+"/") or path == "/proc" or path.startswith("/proc/"):refuse("binary_shadowed")
    for key in ("started_at", "finished_at"):
        if not re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+Z", value[key]):refuse("container_projection_invalid")


def version_commit(raw):
    lines = raw.splitlines()
    keys = ("gitVersion", "gitCommit", "gitTreeState", "buildDate", "goVersion", "compiler", "platform")
    # pkg/app emits Info/flag output before verflag. Discard everything outside
    # the unique, contiguous pkg/version block; never log or return those bytes.
    if sum(bool(re.match(r"^[ \t]*gitCommit:", line)) for line in lines) != 1:refuse("version_output_rejected")
    blocks = []
    for start, line in enumerate(lines):
        if not re.match(r"^[ \t]*gitVersion:", line):continue
        values = {}
        for key, candidate in zip(keys, lines[start:start+len(keys)]):
            match = re.fullmatch(r"[ \t]*"+key+r":[ \t]*([^\x00-\x1f\x7f]*)", candidate)
            if not match:break
            values[key] = match[1].strip()
        if len(values) == len(keys):blocks.append(values)
    if len(blocks) != 1:refuse("version_output_rejected")
    values = blocks[0]
    if not SHA.fullmatch(values["gitCommit"]):refuse("version_output_rejected")
    # Only the full GitCommit participates in proof. Branch/version/date/tree
    # formatting varies by build; all other values are discarded in memory.
    return values["gitCommit"]


def inspect_program(docker, container, binary, source, port, path, version_config):
    # Do not expose stderr/stdout, even on failure. These commands never read Env.
    image_check = load(docker.capture(["image", "inspect", "--format", IMAGE_TEMPLATE, container["image_id"]], maximum=1024))
    if image_check != {"image_id": container["image_id"], "entrypoint": [binary]}:refuse("entrypoint_mismatch")
    changes = docker.capture(["diff", container["container_id"]])
    for line in changes.splitlines():
        match = re.fullmatch(r"([ACD]) (/[\x20-\x7e]+)", line)
        if not match:refuse("docker_output_rejected")
        changed = match[2].rstrip("/") or "/"
        if changed == binary or (match[1] in "AD" and binary.startswith(changed.rstrip("/")+"/")):refuse("binary_modified")
    raw = docker.capture(["exec", "--user", "0", container["container_id"], "sha256sum", "/proc/1/exe", binary], maximum=512)
    hashes = []
    for line, filename in zip(raw.splitlines(), ("/proc/1/exe", binary)):
        match = re.fullmatch(r"([0-9a-f]{64})  " + re.escape(filename), line)
        if not match:refuse("binary_hash_mismatch")
        hashes.append(match[1])
    if len(raw.splitlines()) != 2 or len(hashes) != 2 or hashes[0] != hashes[1]:refuse("binary_hash_mismatch")
    # Cobra OnInitialize reads its config before verflag. Use the fixed existing
    # role path; verflag then exits before Complete or dependency initialization.
    commit = version_commit(docker.capture(["exec", container["container_id"], binary, "--version=true", "--config="+version_config], maximum=64*1024, failure="version_output_rejected", discard_stderr=True))
    if commit != source:refuse("version_mismatch")
    docker.capture(["exec", container["container_id"], "wget", "-q", "-O", "/dev/null", "-T", "5", "http://127.0.0.1:"+str(port)+path], timeout=8, maximum=0, failure="readiness_failed")
    return {"container_id": container["container_id"], "image_config_sha256": container["image_id"][7:],
            "program_sha256": hashes[0], "build_git_commit": commit, "ready": True,
            "restart_count": container["restart_count"], "state_sha256": digest(container)}


def validate_inputs(role, expected, source, run, attempt, image_tag, expected_image_config_id):
    if type(role) is not str or role not in ROLES or type(expected) is not int or not 1 <= expected <= 32 or (role == "apiserver" and expected != 1) or type(source) is not str or not SHA.fullmatch(source) or image_tag != source or type(run) is not str or not RUN.fullmatch(run) or type(attempt) is not str or not ATTEMPT.fullmatch(attempt):refuse("input_binding_invalid")
    if expected_image_config_id is not None and (role != "apiserver" or expected != 1 or type(expected_image_config_id) is not str or not IMAGE.fullmatch(expected_image_config_id)):refuse("input_binding_invalid")


def collect(docker, role, expected, source, run, attempt, image_tag, expected_image_config_id=None):
    validate_inputs(role, expected, source, run, attempt, image_tag, expected_image_config_id)
    before_list = docker.listing()
    rows = select_instances(before_list, role, expected)
    before = [docker.inspect(row["container_id"]) for row in rows]
    for value, row in zip(before, rows):validate_projection(value, row, role, source, expected_image_config_id)
    if role != "apiserver" and {int(v["number"]) for v in before} != set(range(1, expected+1)):refuse("instance_set_invalid")
    _, _, binary, port, path = ROLES[role]
    instances = [inspect_program(docker, value, binary, source, port, path, VERSION_CONFIGS[role]) for value in before]
    after = [docker.inspect(row["container_id"]) for row in rows]
    after_list = docker.listing()
    if select_instances(after_list, role, expected) != rows or after != before:refuse("runtime_changed")
    # Compare the complete role inventory twice, including stopped containers.
    receipt = {"format_version": 2, "kind": "runtime_instance_evidence", "source_sha": source, "image_tag": image_tag,
            "run_id": run+"-"+attempt, "role": role, "expected_instances": expected, "observed_instances": len(instances),
            "complete": True, "health_verified": True, "business_acceptance_verified": False,
            "tool_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
            "topology_sha256": digest(ROLES[role]), "instance_set_sha256": digest(rows), "instances": instances,
            "image_binding_kind": "preflight_config_id" if expected_image_config_id is not None else "source_tag",
            "expected_image_config_sha256": expected_image_config_id[7:] if expected_image_config_id is not None else None,
            "error_category": "none"}
    receipt["receipt_body_sha256"] = digest(receipt)
    validate_receipt(receipt, role=role, expected=expected, source=source, run=run, attempt=attempt,
                     tool_sha256=receipt["tool_sha256"], expected_image_config_id=expected_image_config_id,
                     expected_container_ids=[row["container_id"] for row in rows],
                     expected_state_sha256=[digest(value) for value in before], expected_instance_set_sha256=digest(rows))
    return receipt


def transport():
    path = Path(__file__).resolve().parents[1] / "dbops/receipt-transport.py"
    spec = importlib.util.spec_from_file_location("runtime_receipt_transport", path)
    module = importlib.util.module_from_spec(spec); spec.loader.exec_module(module)
    return module


INSTANCE_SCHEMA = {"container_id": "hash64", "image_config_sha256": "hash64", "program_sha256": "hash64", "build_git_commit": "sha40", "ready": "bool", "restart_count": "uint", "state_sha256": "hash64"}
SCHEMA = {"format_version": "uint", "kind": frozenset({"runtime_instance_evidence"}), "source_sha": "sha40", "image_tag": "sha40", "run_id": "run_id", "role": frozenset(ROLES), "expected_instances": "uint", "observed_instances": "uint", "complete": "bool", "health_verified": "bool", "business_acceptance_verified": "bool", "tool_sha256": "hash64", "topology_sha256": "hash64", "instance_set_sha256": "hash64", "instances": [INSTANCE_SCHEMA], "image_binding_kind": frozenset({"source_tag", "preflight_config_id"}), "expected_image_config_sha256": "nullable_hash64", "receipt_body_sha256": "hash64", "error_category": ERRORS}


def validate_receipt(receipt, *, role, expected, source, run, attempt, tool_sha256, expected_image_config_id=None,
                     expected_container_ids=None, expected_state_sha256=None, expected_instance_set_sha256=None):
    """Bind a v2 receipt to trusted caller inputs; armor is not authentication.

    Optional instance/state bindings must come from an independent observation,
    never be copied from an untrusted decoded receipt. Without them this checks
    receipt semantics, not a new or independent observation of live containers.
    """
    validate_inputs(role, expected, source, run, attempt, source, expected_image_config_id)
    if type(tool_sha256) is not str or not HASH.fullmatch(tool_sha256):refuse("input_binding_invalid")
    if type(receipt) is not dict or set(receipt) != set(SCHEMA):refuse("receipt_binding_invalid")
    try:transport().encode_armored_receipt(receipt, schema=SCHEMA)
    except Exception:refuse("receipt_binding_invalid")
    if receipt["format_version"] != 2 or receipt["source_sha"] != source or receipt["image_tag"] != source or receipt["run_id"] != run+"-"+attempt or receipt["role"] != role or receipt["expected_instances"] != expected or receipt["observed_instances"] != expected or receipt["tool_sha256"] != tool_sha256 or receipt["topology_sha256"] != digest(ROLES[role]):refuse("receipt_binding_invalid")
    if receipt["complete"] is not True or receipt["health_verified"] is not True or receipt["business_acceptance_verified"] is not False or receipt["error_category"] != "none":refuse("receipt_binding_invalid")
    kind = "preflight_config_id" if expected_image_config_id is not None else "source_tag"
    config_hash = expected_image_config_id[7:] if expected_image_config_id is not None else None
    if receipt["image_binding_kind"] != kind or receipt["expected_image_config_sha256"] != config_hash:refuse("receipt_binding_invalid")
    instances = receipt["instances"]
    if len(instances) != expected or any(set(instance) != set(INSTANCE_SCHEMA) or instance["build_git_commit"] != source or instance["ready"] is not True or (config_hash is not None and instance["image_config_sha256"] != config_hash) for instance in instances):refuse("receipt_binding_invalid")
    ids = [instance["container_id"] for instance in instances]
    if ids != sorted(set(ids)):refuse("receipt_binding_invalid")
    if receipt["receipt_body_sha256"] != digest({key:value for key,value in receipt.items() if key != "receipt_body_sha256"}):refuse("receipt_binding_invalid")
    for supplied, actual in ((expected_container_ids, ids), (expected_state_sha256, [instance["state_sha256"] for instance in instances])):
        if supplied is not None and (type(supplied) is not list or len(supplied) != expected or any(type(value) is not str or not HASH.fullmatch(value) for value in supplied) or supplied != actual):refuse("receipt_binding_invalid")
    if expected_instance_set_sha256 is not None and (type(expected_instance_set_sha256) is not str or not HASH.fullmatch(expected_instance_set_sha256) or receipt["instance_set_sha256"] != expected_instance_set_sha256):refuse("receipt_binding_invalid")
    return receipt


def receive_armored_receipt(text, **trusted_bindings):
    """Decode exactly one armored frame, then enforce all semantic bindings."""
    try:receipt = load(transport().decode_armored_receipt(text))
    except Exception:refuse("receipt_binding_invalid")
    return validate_receipt(receipt, **trusted_bindings)


class SafeArgumentParser(argparse.ArgumentParser):
    def error(self, message):
        # argparse normally echoes rejected input, which could be sensitive.
        refuse("input_binding_invalid")


def main(argv=None):
    parser = SafeArgumentParser(description=__doc__)
    for key in ("role", "expected-instances", "source-sha", "run-id", "run-attempt", "image-tag"):parser.add_argument("--"+key, required=True)
    parser.add_argument("--sudo-docker", action="store_true", help="Use the fixed noninteractive sudo Docker executor only.")
    parser.add_argument("--expected-image-config-id", help="API-only immutable image ID established by the successful MQ deployment preflight.")
    try:
        args = parser.parse_args(argv)
        try:count = int(args.expected_instances)
        except ValueError:refuse("input_binding_invalid")
        if str(count) != args.expected_instances:refuse("input_binding_invalid")
        receipt = collect(Docker(sudo_docker=args.sudo_docker), args.role, count, args.source_sha, args.run_id, args.run_attempt, args.image_tag, args.expected_image_config_id)
        validate_receipt(receipt, role=args.role, expected=count, source=args.source_sha, run=args.run_id,
                         attempt=args.run_attempt, tool_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                         expected_image_config_id=args.expected_image_config_id)
        print(transport().encode_armored_receipt(receipt, schema=SCHEMA))
        return 0
    except Exception as error:
        category = str(error) if type(error) is Refused and str(error) in ERRORS else "docker_command_failed"
        # This fixed category never includes a rejected value or raw output.
        print("runtime_instance_evidence_refused:"+category, file=sys.stderr)
        return 1


if __name__ == "__main__":sys.exit(main())
