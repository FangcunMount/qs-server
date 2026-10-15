#!/usr/bin/env python3
"""Fixed A maintenance receiver. Source preparation; never invoked on import.

Only a separately approved controller may open the framed stdin session. SQL
credentials stay in process memory/child environments. No standard CD, registry
operation, image tagging, config sync, arbitrary command hook, force or down.
"""
import base64
import copy
import datetime
import fcntl
import hashlib
import http.client
import json
import os
import pathlib
import platform
import re
import socket
import stat
import subprocess
import sys
import threading
import time
import urllib.parse

A = "51163edab7038adcdb6b8f1712d6dc434f361dc6"
OLD = "4a8642b045289423b75bca945f2b9b3b4365c01e"
RESOURCE = "77e11d801d270ef7b12f86261a05217a95bd8f8049f20d2e5f14ee0c0a20083b"
TOOL_SOURCE = "602710eb2f6ee383923230890c4bdb4a3bcada3a"
TOOL = "2c314163ee6931aa133f3537b418927c97cce9f8fd8f4f3b75f42be142e173fd"
EXECUTOR = "a5492be61236ddb08c819f64f39f38a0bbde1d5abbf20a43bd6f7c48c5f572d0"
LOCK = pathlib.Path("/var/lib/fangcun-image-retention/deploy.lock")
SERVICE = {"api": "qs-apiserver", "collection": "server", "worker": "runtime"}
COUNTS = {"api": 1, "collection": 2, "worker": 3}
PROGRAM = {"api": "/app/qs-apiserver", "collection": "/app/collection-server", "worker": "/app/qs-worker"}
CONFIG = {"api": "/app/configs/apiserver.prod.yaml", "collection": "/app/configs/collection-server.prod.yaml", "worker": "/app/configs/worker.prod.yaml"}
MAX_FRAME = 48 * 1024 * 1024
TEMPLATE = "49b7dc1d34296c1e99ff8df206e506f0b46b87fceabe9dd6ad09e016536cca06"
GENERATOR = "de0a3cbbf61e81bfdd7b1cc63b287e74fd72367064c71a8e7fbb598d4c1e52d9"
DECODER = "9baa4a9fac160969def1a84db56e2cb3a8ec2e1fd24f4f1d1661b80dafdb907b"
ENTRY = {
    "qs.fangcunmount.cn": ("/data/apps/nginx-configs/qs.fangcunmount.cn.conf", "26efeedd9f41e913b751f355977635efb8ab693eab179ec47b6f78a9ef85a156"),
    "collect.fangcunmount.cn": ("/data/apps/nginx-configs/collect.conf", "bb122f4c320c1f6f9532b0ee232dba4971e26409f63133accb01c8b0b513b8d5"),
}


def reject(reason):
    raise RuntimeError(reason)


def pairs(items):
    result = {}
    for k, v in items:
        if k in result:
            reject("duplicate_json_key")
        result[k] = v
    return result


def decode(raw):
    return json.loads(raw, object_pairs_hook=pairs)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def filehash(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(65536), b""):
            h.update(block)
    return h.hexdigest()


def durable(path, raw):
    """A marker is never overwritten or removed, including after failure."""
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "wb") as f:
        f.write(raw)
        f.flush()
        os.fsync(f.fileno())
    parent = os.open(pathlib.Path(path).parent, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(parent)
    finally:
        os.close(parent)


def encoded(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":")).encode()


def capture_receipt(raw_stdout, expected_material_sha256):
    """Validate actual successful Go stdout, before adding RPC-only metadata.

    SHA binds retained bytes; it is not a signature. Trust remains in this
    independently reviewed executor and the separate recovery approval.
    """
    if not isinstance(raw_stdout, bytes) or not 0 < len(raw_stdout) <= 64 * 1024:
        reject("capture_stdout_size_invalid")
    d = decode(raw_stdout)
    expected = {"action": "capture", "compiled_tool_source": TOOL_SOURCE,
                "a_execution_source": A, "a_execution_resources_sha256": RESOURCE,
                "a_execution_resource_files": 280, "complete": True,
                "database_read_only": True, "material_sha256": expected_material_sha256,
                "sql_connection_closed": True, "sql_pool_closed": True, "mongo_closed": True,
                "error_category": "none", "transaction_release_unproven": False,
                "production_freeze_certified": False, "total_budget_exceeded": False}
    if not isinstance(expected_material_sha256, str) or not re.fullmatch(r"[0-9a-f]{64}", expected_material_sha256) or expected_material_sha256 == "0" * 64:
        reject("capture_material_hash_invalid")
    # Canonical re-encoding is only a typed-tree check; original stdout is kept.
    if type(d) is not dict or encoded(d) != encoded(expected):
        reject("capture_stdout_not_successful_exact_receipt")
    return d


def capture_execution_receipt(raw, expected_sha256, expected_stdout_path):
    """Validate the fixed original envelope without rewriting either document."""
    if type(raw) is not bytes or not 0 < len(raw) <= 32 * 1024 or digest(raw) != expected_sha256:
        reject("capture_execution_original_bytes_invalid")
    proof = decode(raw)
    if type(proof) is not dict or set(proof) != {"format_version", "kind", "receipt_file", "receipt_sha256", "termination", "exit_code"}:
        reject("capture_execution_envelope_invalid")
    stdout_sha = proof.get("receipt_sha256")
    if not isinstance(stdout_sha, str) or not re.fullmatch(r"[0-9a-f]{64}", stdout_sha):
        reject("capture_execution_stdout_hash_invalid")
    expected = {"format_version": 1, "kind": "m4_a_capture_execution",
                "receipt_file": expected_stdout_path, "receipt_sha256": stdout_sha,
                "termination": "exited", "exit_code": 0}
    if encoded(proof) != encoded(expected):
        reject("capture_execution_envelope_invalid")
    return proof


def capture_private_bytes(path, expected_sha256, limit):
    """Read a fixed executor-owned file without following a substituted link."""
    p = pathlib.Path(path)
    parent = p.parent.lstat()
    if not p.is_absolute() or p.resolve() != p or not stat.S_ISDIR(parent.st_mode) or parent.st_uid != os.getuid() or stat.S_IMODE(parent.st_mode) != 0o700:
        reject("capture_evidence_private_parent_invalid")
    fd = os.open(p, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
    with os.fdopen(fd, "rb") as f:
        st = os.fstat(f.fileno())
        if not stat.S_ISREG(st.st_mode) or st.st_uid != os.getuid() or stat.S_IMODE(st.st_mode) != 0o600 or st.st_nlink != 1 or not 0 < st.st_size <= limit:
            reject("capture_evidence_private_file_invalid")
        raw = f.read(limit + 1)
        after = p.lstat()
        if (after.st_dev, after.st_ino) != (st.st_dev, st.st_ino) or len(raw) != st.st_size or digest(raw) != expected_sha256:
            reject("capture_evidence_bytes_changed")
    return raw


def network_request(d):
    """Retain explicit network inputs; never copy allocated IP/MAC/endpoint IDs.

    The exact 4a template has no static IP/MAC/links. An unreviewed static
    topology is refused instead of guessing what may be changed during create.
    Canonical name/service aliases remain; daemon-generated CID aliases do not.
    """
    result = {}
    for name, endpoint in d["NetworkSettings"]["Networks"].items():
        if endpoint.get("IPAMConfig") or endpoint.get("Links") or endpoint.get("DriverOpts") or endpoint.get("GwPriority", 0) or d["Config"].get("MacAddress"):
            reject("unreviewed_static_network_configuration")
        aliases = [a for a in (endpoint.get("Aliases") or []) if a not in [d["Id"], d["Id"][:12]]]
        result[name] = {"Aliases": aliases, "IPAMConfig": None, "Links": None, "DriverOpts": None}
    if not result:
        reject("actual_service_network_missing")
    return result


def candidate_spec(old, image):
    config = copy.deepcopy(old["Config"])
    config["Image"] = image
    # All actual Config fields (including Env, original service/Compose labels,
    # hostname, command, user and entrypoint) remain byte-for-byte equivalent.
    # Source is proved by real binary/version, never inherited label text.
    return dict(config, HostConfig=copy.deepcopy(old["HostConfig"]), NetworkingConfig={"EndpointsConfig": network_request(old)})


def host_semantics(host):
    value = copy.deepcopy(host)
    # Actual Engine1.44 create normalizes nullable legacy inspect default to
    # false. Only this named default is equivalent; true and every other field
    # remain distinct. Preserve the original raw request and audit both values.
    if value.get("OomKillDisable") is None:
        value["OomKillDisable"] = False
    return value


def mounts(value):
    return sorted(value, key=lambda m: (m.get("Destination", ""), m.get("Type", ""), m.get("Source", ""), encoded(m)))


def verify_candidate(old, new, image, name, started=False):
    expected = copy.deepcopy(old["Config"])
    expected["Image"] = image
    state = new["State"]
    never_started = state.get("StartedAt", "").startswith("0001-")
    if new["Config"] != expected or host_semantics(new["HostConfig"]) != host_semantics(old["HostConfig"]) or new["Image"] != image:
        reject("candidate_config_or_HostConfig_not_image_only")
    if mounts(new["Mounts"]) != mounts(old["Mounts"]) or network_request(new) != network_request(old) or new["Path"] != old["Path"] or new["Args"] != old["Args"] or new["Name"] != "/" + name:
        reject("candidate_mount_network_or_argv_changed")
    if not started and (state["Running"] or not never_started or state.get("Restarting") or state.get("OOMKilled")):
        reject("candidate_not_never_started")
    return {"complete_Config_equal_except_image": True, "complete_HostConfig_semantics_equal": True, "OomKillDisable_default_before_after": [old["HostConfig"].get("OomKillDisable"), new["HostConfig"].get("OomKillDisable")], "mounts_explicit_networks_and_argv_equal": True, "never_started": not started and never_started}


class EngineConnection(http.client.HTTPConnection):
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect("/var/run/docker.sock")


class Receiver:
    def __init__(self, request):
        self.b = request["binding"]
        self.host = self.b["host"]
        self.roles = ["api", "collection"] if self.host == "serverA" else ["worker"]
        self.root = pathlib.Path("/opt/qs-server/m4-cleanup-a") / request["operation_id"]
        self.deadline = time.monotonic() + request["remaining_seconds"]
        self.credentials = {}
        self.connection_evidence = None
        self.frozen = False
        self.window_started = False
        self.crossed = False
        self.old_runtime_started = False
        self.new_verified = False
        self.retired_old = set()
        self.allow_exact_old_cleanup = request["allow_exact_old_cleanup"] is True
        self.freeze_failure = None
        self.finished = threading.Event()
        self.journal = []
        self.original_entry = {}
        self.entry_mutating = threading.Event()
        self.candidate_mutating = threading.Event()
        self.entry_open = True
        self.material_sha = None
        self.capture_evidence_sha = None
        self.old = {}
        self.prepared = {}
        self.parked = {}
        self.owned_cli = {}
        self.native_carrier_attempt = 0
        self.before_first_create_ids = None
        self.images = {}
        self.lock = None
        if os.getuid() != 0 or platform.system() != "Linux" or socket.gethostname().lower() != self.host.lower() or platform.machine() not in ["x86_64", "amd64"]:
            reject("fixed_host_native_identity_mismatch")
        if not re.fullmatch(r"[0-9]{1,20}-[0-9]{1,4}", request["operation_id"]):
            reject("operation_id_invalid")
        if not 1 <= request["remaining_seconds"] <= 1800 or request["source_A"] != A:
            reject("source_or_window_invalid")
        if "connection_environment" in request:
            reject("external_or_fallback_credentials_not_supported")
        machine = pathlib.Path("/etc/machine-id").read_bytes().strip()
        if not re.fullmatch(rb"[0-9a-f]{32}", machine) or digest(machine) != self.b["machine_identity_sha256"]:
            reject("fresh_machine_identity_mismatch")
        self.bind_host_and_lock()
        self.root.parent.mkdir(mode=0o700, exist_ok=True)
        parent_stat = self.root.parent.lstat()
        if self.root.parent.is_symlink() or parent_stat.st_uid != 0 or stat.S_IMODE(parent_stat.st_mode) != 0o700:
            reject("private_operation_parent_untrusted")
        self.root.mkdir(mode=0o700)  # Existing operation is never silently reused.
        self.baseline = self.runtime_records(expect_running=True, expected_source=OLD)
        self.hash_inputs()
        self.verify_cached_images()
        self.verify_runtime(old=True)
        if self.host == "serverA":
            self.prefreeze_connection(request)
        self.monitor = threading.Thread(target=self.watch, daemon=True)
        self.monitor.start()

    def prefreeze_connection(self, request):
        raw = base64.b64decode(request["decoder_source"], validate=True)
        if digest(raw) != DECODER:
            reject("fixed_prefreeze_decoder_source_mismatch")
        # Reuse the exact reviewed decoder. Only its common import is supplied
        # by these identical strict JSON/require/error primitives; no decoder
        # logic or endpoint/account selection is configurable.
        common_import = b"from common import strict_json,require,Rejected\n"
        if raw.count(common_import) != 1:
            reject("fixed_decoder_import_shape_changed")
        def require(value, category):
            if not value:
                reject(category)
        namespace = {"strict_json": decode, "require": require, "Rejected": RuntimeError}
        exec(compile(raw.replace(common_import, b""), "fixed-reviewed-connection-decoder", "exec"), namespace)
        cid = self.b["services"]["api"]["old_container_ids"][0]
        d = self.inspect(cid)
        def fixed_read(argv):
            allowed = [["docker", "exec", cid, "cat", p] for p in ["/proc/1/cmdline", "/proc/1/environ", "/app/configs/apiserver.prod.yaml"]]
            if argv not in allowed:
                reject("prefreeze_decoder_command_outside_fixed_scope")
            return self.run(argv, 5, limit=2 * 1024 * 1024)
        connection, evidence = namespace["configuration"](d, fixed_read)
        expected = self.b["prefreeze_connection_source"]
        for key in ["config_sha256", "pid1_environment_sha256", "pid1_argv_sha256"]:
            if evidence[key] != expected[key]:
                reject("fresh_prefreeze_PID1_connection_source_changed")
        if evidence["task_opened_reminder_enabled"] is not False or connection.pop("database") != "qs":
            reject("fixed_connection_or_reminder_contract_changed")
        self.credentials = connection
        self.connection_evidence = {"decoder_source_sha256": DECODER, "old_API_cid": cid, "config_sha256": evidence["config_sha256"], "pid1_environment_sha256": evidence["pid1_environment_sha256"], "pid1_argv_sha256": evidence["pid1_argv_sha256"], "captured_before_fixed_six_stop": True, "credentials_remain_RAM_only": True, "task_opened_reminder_enabled": False, "not_T0_material": True}

    def run(self, argv, seconds=30, env=None, limit=4 * 1024 * 1024, input_raw=None):
        self.guard()
        remaining = self.deadline - time.monotonic()
        if remaining <= 0:
            reject("maintenance_budget_exhausted")
        try:
            effective = {"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "HOME": "/root", "DOCKER_HOST": "unix:///var/run/docker.sock"}
            if env:
                effective.update(env)
            p = subprocess.run(argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                               env=effective, input=input_raw, timeout=min(seconds, remaining), check=False)
        except subprocess.TimeoutExpired:
            reject("command_timeout_outcome_unknown")
        if len(p.stdout) > limit or len(p.stderr) > 65536 or p.returncode:
            reject("command_failed_or_output_exceeded")
        return p.stdout

    def guard(self):
        if self.freeze_failure:
            reject("continuous_fixed_writer_or_config_proof_lost")
        if self.lock:
            current = LOCK.lstat()
            held = os.fstat(self.lock.fileno())
            if (current.st_dev, current.st_ino) != (held.st_dev, held.st_ino):
                reject("shared_lock_inode_changed")

    def bind_host_and_lock(self):
        sock = pathlib.Path("/var/run/docker.sock").stat()
        if not stat.S_ISSOCK(sock.st_mode) or sock.st_uid != 0:
            reject("fixed_local_docker_socket_missing")
        st = LOCK.lstat()
        if not stat.S_ISREG(st.st_mode) or stat.S_ISLNK(st.st_mode) or st.st_uid != 0:
            reject("existing_shared_lock_unavailable")
        self.lock = open(LOCK, "r+b", buffering=0)
        fcntl.flock(self.lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        daemon = decode(self.run(["docker", "info", "--format", "{{json .}}"], 10))
        if daemon["ID"] != self.b["daemon_id"] or daemon["Name"].lower() != self.host.lower():
            reject("fresh_daemon_binding_mismatch")

    def inspect(self, cid):
        return decode(self.run(["docker", "inspect", cid], 10))[0]

    def runtime_records(self, expect_running, expected_source):
        result, expected_running = {}, set()
        for role in self.roles:
            b = self.b["services"][role]
            ids = b["old_container_ids"] if expected_source == OLD else [self.prepared[cid]["new_cid"] for cid in b["old_container_ids"]]
            project_ids = self.run(["docker", "ps", "-aq", "--no-trunc", "--filter", "label=com.docker.compose.project=" + b["project"], "--filter", "label=com.docker.compose.service=" + SERVICE[role]], 10).decode().split()
            allowed = (set(b["old_container_ids"]) - self.retired_old) | {v["new_cid"] for old, v in self.prepared.items() if old in b["old_container_ids"]}
            # A create response may be lost. Reconcile only a pre-registered
            # canonical name and exact NEVER-STARTED spec; no blind removal.
            for candidate_id in set(project_ids) - allowed:
                d = self.inspect(candidate_id)
                matching = [cid for cid, v in self.parked.items() if v["role"] == role and d["Name"] == "/" + v["name"]]
                if len(matching) != 1:
                    reject("unregistered_candidate_after_create")
                old_id = matching[0]
                verify_candidate(self.old[old_id], d, b["new_image"]["actual_engine_id"], self.parked[old_id]["name"])
                if d["Id"] != candidate_id:
                    reject("resolved_candidate_inspect_ID_mismatch")
                self.bind_created_identity(self.parked[old_id], candidate_id)
                self.prepared[old_id] = {"new_cid": candidate_id, "name": self.parked[old_id]["name"], "role": role}
                allowed.add(candidate_id)
            if len(ids) != COUNTS[role] or set(project_ids) != allowed:
                reject("complete_instance_set_mismatch")
            rows = []
            for cid in sorted(ids):
                d = self.inspect(cid)
                if d["State"]["Running"] is not expect_running:
                    reject("writer_running_state_mismatch")
                image = b["new_image"]["actual_engine_id"] if expected_source == A else b["old_image_id"]
                if d["Image"] != image or d["Config"]["User"] != "www" or d["Config"]["Entrypoint"] != [PROGRAM[role]] or d["Args"] != ["--config=" + CONFIG[role]]:
                    reject("image_user_or_entrypoint_mismatch")
                if expected_source == OLD and cid not in self.old:
                    # Never persist the create body/Env: secret values stay RAM.
                    self.old[cid] = copy.deepcopy(d)
                    if digest(encoded(candidate_spec(d, d["Config"]["Image"]))) != b["old_create_spec_sha256"][cid]:
                        reject("fresh_actual_container_config_binding_mismatch")
                if expect_running:
                    expected_running.add(cid)
                rows.append({"cid": cid, "image_id": d["Image"], "env": d["Config"]["Env"], "mounts": mounts(d["Mounts"]), "networks": sorted(d["NetworkSettings"]["Networks"]), "restart_policy": d["HostConfig"]["RestartPolicy"], "running": d["State"]["Running"]})
            result[role] = rows
        # This is fixed-process continuity, not inferred DB write absence. Keep
        # stopped old CIDs and NEVER-STARTED candidates in the allowed set.
        live = self.run(["docker", "ps", "--no-trunc", "--format", "{{.ID}} {{.Names}}"], 10).decode().splitlines()
        for line in live:
            cid, name = line.split(None, 1)
            if name.startswith(("qs-apiserver", "qs-collection", "qs-worker", "m4-a-old-")) and cid not in expected_running:
                reject("unregistered_or_restarted_qs_runtime_writer")
        return result

    def nginx_workers(self):
        raw = self.run(["docker", "top", self.b["nginx_container_id"], "-eo", "pid,args"], 5).decode()
        return {line.split()[0] for line in raw.splitlines() if "nginx: worker process" in line}

    def replace_entry(self, path, raw, original_stat):
        temporary = path.parent / (".m4-a-" + self.root.name + "-" + path.name + ".pending")
        durable(temporary, raw)
        os.chown(temporary, original_stat.st_uid, original_stat.st_gid)
        temporary.chmod(stat.S_IMODE(original_stat.st_mode))
        os.replace(temporary, path)
        parent = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(parent)
        finally:
            os.close(parent)

    def entry(self, open_entry=False):
        self.entry_mutating.set()
        try:
            return self.change_entry(open_entry)
        finally:
            self.entry_mutating.clear()

    def change_entry(self, open_entry):
        if self.host != "serverA":
            reject("entry_only_fixed_serverA")
        nginx = self.inspect(self.b["nginx_container_id"])
        if nginx["Name"] != "/nginx" or nginx["Image"] != self.b["nginx_image_id"] or not nginx["State"]["Running"]:
            reject("fresh_nginx_binding_mismatch")
        old_workers = self.nginx_workers()
        if not old_workers:
            reject("nginx_workers_unknown")
        for item in self.b["entry_files"]:
            path = pathlib.Path(item["path"])
            if path.parent != pathlib.Path("/data/apps/nginx-configs") or path.is_symlink():
                reject("fixed_entry_path_mismatch")
            if open_entry:
                original = self.original_entry.get(str(path))
                if original is None or filehash(path) != original["fenced_sha256"]:
                    reject("entry_restore_original_binding_missing")
                self.replace_entry(path, original["raw"], original["stat"])
            else:
                raw = path.read_bytes()
                if digest(raw) != item["sha256"]:
                    reject("fresh_entry_source_changed")
                names = re.findall(rb"(?m)^\s*server_name\s+([^;]+);", raw)
                if len(names) != item["server_blocks"] or any(n.decode().strip() != item["domain"] for n in names):
                    reject("complete_entry_server_scope_mismatch")
                fenced, count = re.subn(rb"(?m)^(\s*server\s*\{)", rb"\1\n    return 503; # fixed M4-A entry fence", raw)
                if count != item["server_blocks"]:
                    reject("entry_server_block_parser_refused")
                st = path.stat()
                durable(self.root / (path.name + ".original"), raw)
                self.original_entry[str(path)] = {"raw": raw, "stat": st, "fenced_sha256": digest(fenced)}
                self.replace_entry(path, fenced, st)
        self.entry_open = open_entry
        self.run(["docker", "exec", self.b["nginx_container_id"], "nginx", "-t"], 10)
        self.run(["docker", "exec", self.b["nginx_container_id"], "nginx", "-s", "reload"], 10)
        end = min(time.monotonic() + 60, self.deadline)
        while self.nginx_workers() & old_workers:
            if time.monotonic() >= end:
                reject("old_nginx_workers_not_drained_hold_entry")
            time.sleep(1)
        if not open_entry:
            for item in self.b["entry_files"]:
                code = self.run(["curl", "--silent", "--show-error", "--max-time", "5", "--resolve", item["domain"] + ":443:127.0.0.1", "--output", "/dev/null", "--write-out", "%{http_code}", "https://" + item["domain"] + "/"], 7).decode()
                if code != "503":
                    reject("actual_entry_fence_not_503")
        else:
            for item in self.b["entry_files"]:
                code = self.run(["curl", "--silent", "--show-error", "--max-time", "5", "--resolve", item["domain"] + ":443:127.0.0.1", "--output", "/dev/null", "--write-out", "%{http_code}", "https://" + item["domain"] + "/health"], 7).decode()
                if code != "200":
                    reject("post_open_public_health_failed_forward_only")
        return {"entry_open": open_entry, "all_bound_old_nginx_workers_drained": True, "actual_entry_503_verified": not open_entry, "actual_public_health_200_verified": open_entry}

    def hash_inputs(self):
        for item in self.b["immutable_files"]:
            p = pathlib.Path(item["path"])
            expected = item["sha256"]
            if not self.entry_open and str(p) in self.original_entry:
                expected = self.original_entry[str(p)]["fenced_sha256"]
            if not p.is_absolute() or p.is_symlink() or not p.is_file() or filehash(p) != expected:
                reject("fresh_config_certificate_or_compose_changed")

    def verify_cached_images(self):
        for role in self.roles:
            b = self.b["services"][role]
            if b["original_generation_source"] != OLD or b["original_template_sha256"] != TEMPLATE or b["original_generator_sha256"] != GENERATOR:
                reject("original_4a_generation_source_unbound")
            for image_id in [b["old_image_id"], b["new_image"]["actual_engine_id"]]:
                image = decode(self.run(["docker", "image", "inspect", image_id], 10))[0]
                if image["Id"] != image_id or image["Architecture"] != "amd64" or image["Os"] != "linux" or image["Config"]["User"] != "www" or image["Config"]["Entrypoint"] != [PROGRAM[role]]:
                    reject("cached_normal_image_identity_mismatch")
                self.images[image_id] = image
            # Removed /tmp Compose inputs are provenance, not required files.
            # Actual live Config/HostConfig and explicit topology are authoritative.
            durable(self.root / (role + "-reconstruction.json"), encoded({"original_generation_source": OLD, "original_template_sha256": TEMPLATE, "original_generator_sha256": GENERATOR, "actual_config_source": "fresh_bound_original_container_inspect_in_RAM", "retained_project": b["project"], "original_compose_paths": b["original_compose_paths"], "deleted_temporary_compose_recreated_from_actual_config": True, "new_image": b["new_image"]["actual_engine_id"], "secret_configuration_persisted": False, "old_container_ids": b["old_container_ids"]}))

    def capture_before_first_create(self):
        if self.before_first_create_ids is not None:
            return
        if self.lock is None or self.lock.closed:
            reject("locked_full_before_create_baseline_missing")
        ids = self.checked_CIDs(self.run(["docker", "ps", "-aq", "--no-trunc"], 10))
        durable(self.root / "before-first-create.json", encoded({"all_existing_CIDs": sorted(ids), "captured_under_existing_deploy_lock_before_first_create": True}))
        self.before_first_create_ids = frozenset(ids)

    @staticmethod
    def checked_CIDs(raw):
        ids = raw.decode().split()
        if len(ids) != len(set(ids)) or any(not re.fullmatch(r"[0-9a-f]{64}", cid) for cid in ids):
            reject("complete_container_identity_list_unproven")
        return ids

    def bind_created_identity(self, intent, cid):
        before = self.before_first_create_ids
        if not isinstance(before, frozenset) or not intent["dispatched"] or not re.fullmatch(r"[0-9a-f]{64}", cid) or cid in before:
            reject("exact_non_before_created_identity_unproven")
        if intent["known_cid"] is not None and intent["known_cid"] != cid:
            reject("known_created_identity_cannot_be_replaced")
        intent["known_cid"] = cid

    def prove_removed_absence(self, name, cid):
        named = self.checked_CIDs(self.run(["docker", "ps", "-aq", "--no-trunc", "--filter", "name=^/" + name + "$"], 10))
        all_ids = self.checked_CIDs(self.run(["docker", "ps", "-aq", "--no-trunc"], 10))
        if named or cid in all_ids:
            reject("removed_created_identity_absence_unproven")

    def engine_create(self, name, body, intent):
        self.guard()
        self.capture_before_first_create()
        remaining = self.deadline - time.monotonic()
        if remaining <= 0:
            reject("maintenance_budget_exhausted")
        connection = EngineConnection("localhost", timeout=min(20, remaining))
        try:
            path = "/v1.44/containers/create?" + urllib.parse.urlencode({"name": name, "platform": "linux/amd64"})
            durable(self.root / ("create-dispatched-" + digest(name.encode()) + ".json"), encoded({"name": name, "image": body["Image"], "dispatched": True, "outcome": "unknown"}))
            intent["dispatched"] = True
            connection.request("POST", path, encoded(body), {"Content-Type": "application/json"})
            response = connection.getresponse()
            raw = response.read(65537)
            if response.status != 201 or len(raw) > 65536:
                reject("fixed_local_container_create_failed_or_unknown")
            value = decode(raw)
            if not re.fullmatch(r"[0-9a-f]{64}", value.get("Id", "")) or value.get("Warnings"):
                reject("container_create_warning_or_invalid_identity")
            self.bind_created_identity(intent, value["Id"])
            return value["Id"]
        finally:
            connection.close()

    def prepare_new(self):
        self.candidate_mutating.set()
        try:
            return self.prepare_new_containers()
        finally:
            self.candidate_mutating.clear()

    def prepare_new_containers(self):
        if not self.frozen or self.crossed or (self.root / "first-runtime.json").exists() or self.prepared:
            reject("prepare_requires_prestart_frozen_originals")
        self.runtime_records(False, OLD)
        results = []
        for role in self.roles:
            image = self.b["services"][role]["new_image"]["actual_engine_id"]
            for cid in self.b["services"][role]["old_container_ids"]:
                old = self.old[cid]
                name = old["Name"].lstrip("/")
                parked = "m4-a-old-" + self.root.name + "-" + name
                # Register intent before rename/create; uncertain outcomes retain
                # both names for exact reconciliation, never arbitrary cleanup.
                self.parked[cid] = {"name": name, "parked": parked, "role": role, "dispatched": False, "known_cid": None, "remove_acknowledged": False}
                durable(self.root / ("create-intent-" + cid + ".json"), encoded({"old_cid": cid, "old_name": name, "parked_name": parked, "candidate_name": name, "image": image, "create_body_sha256": digest(encoded(candidate_spec(old, image)))}))
                self.run(["docker", "rename", cid, parked], 10)
                created = self.engine_create(name, candidate_spec(old, image), self.parked[cid])
                self.prepared[cid] = {"new_cid": created, "name": name, "role": role}
                # No start command has been issued. Config, complete HostConfig,
                # mounts, explicit network inputs and actual argv must match.
                proof = verify_candidate(old, self.inspect(created), image, name)
                results.append(dict(proof, old_cid=cid, new_cid=created, role=role, actual_engine_id=image, original_CID_retained=True))
                durable(self.root / ("created-" + created + ".json"), encoded(results[-1]))
        self.runtime_records(False, OLD)
        return {"complete": True, "prepared_instance_count": len(results), "instances": results, "business_start_command_issued": False}

    def unprepare(self):
        if self.crossed or (self.root / "first-runtime.json").exists():
            reject("candidate_removal_forbidden_after_first_runtime_intent")
        for cid, binding in self.parked.items():
            old = self.old[cid]
            image = self.b["services"][binding["role"]]["new_image"]["actual_engine_id"]
            if binding["dispatched"]:
                found = self.checked_CIDs(self.run(["docker", "ps", "-aq", "--no-trunc", "--filter", "name=^/" + binding["name"] + "$"], 10))
                if len(found) > 1:
                    reject("candidate_name_not_unique")
                if not found:
                    if binding["known_cid"] is None or not binding["remove_acknowledged"]:
                        reject("dispatched_candidate_create_or_release_unproven")
                    self.bind_created_identity(binding, binding["known_cid"])
                else:
                    candidate = found[0]
                    self.bind_created_identity(binding, candidate)
                    if binding["remove_acknowledged"]:
                        reject("candidate_still_present_after_remove_ACK")
                    actual = self.inspect(candidate)
                    if actual["Id"] != candidate:
                        reject("resolved_candidate_inspect_ID_mismatch")
                    verify_candidate(old, actual, image, binding["name"])
                    self.run(["docker", "rm", candidate], 15)  # no force/no volumes
                    binding["remove_acknowledged"] = True
                self.prove_removed_absence(binding["name"], binding["known_cid"])
            elif binding["known_cid"] is not None or binding["remove_acknowledged"]:
                reject("undispatched_candidate_intent_invalid")
            current = self.inspect(cid)
            if current["State"]["Running"] or current["Name"] not in ["/" + binding["parked"], "/" + binding["name"]]:
                reject("retained_old_container_identity_or_state_changed")
            if current["Name"] != "/" + binding["name"]:
                self.run(["docker", "rename", cid, binding["name"]], 10)
        self.prepared.clear()
        self.parked.clear()

    def watch(self):
        while not self.finished.wait(2):
            if self.entry_mutating.is_set() or self.candidate_mutating.is_set():
                continue
            try:
                self.hash_inputs()
                if self.frozen and not self.crossed and not self.old_runtime_started:
                    self.runtime_records(False, OLD)
                self.guard()
            except Exception:
                self.freeze_failure = True
                return

    def stop(self):
        ids = [cid for role in self.roles for cid in self.b["services"][role]["old_container_ids"]]
        # -1 deliberately avoids Docker's timeout->SIGKILL path. The caller is
        # bounded; interruption leaves outcome unknown and the entry fenced.
        self.run(["docker", "stop", "--time=-1"] + ids, 520)
        self.frozen = True
        self.runtime_records(False, OLD)
        return {"fixed_instance_count": len(ids), "all_stopped": True, "forced_kill_used": False, "direct_database_write_freeze_certified": False}

    def supply_tools(self, payload):
        if self.host != "serverA":
            reject("native_tools_only_serverA")
        for name, expected in [("protection", TOOL), ("offline", EXECUTOR)]:
            data = base64.b64decode(payload[name], validate=True)
            if digest(data) != expected or data[:5] != b"\x7fELF\x02":
                reject("exact_native_tool_bytes_mismatch")
            p = self.root / name
            durable(p, data)
            p.chmod(0o700)
        return {"exact_native_tools_supplied": True, "business_process_started": False}

    def run_native_carrier(self, args, env, seconds):
        if self.owned_cli:
            reject("prior_native_carrier_release_unproven")
        # Preserve the reviewed API connection namespace (mongo DNS/ExtraHosts),
        # without running its business entrypoint or borrowing its app mounts.
        api = self.old[self.b["services"]["api"]["old_container_ids"][0]]
        name = "m4-a-cli-" + self.root.name + "-" + str(self.native_carrier_attempt)
        host = {"Binds": [str(self.root) + ":" + str(self.root) + ":rw"], "ReadonlyRootfs": True,
                "NetworkMode": api["HostConfig"]["NetworkMode"], "CapDrop": ["ALL"], "SecurityOpt": ["no-new-privileges"],
                "LogConfig": {"Type": "none", "Config": {}}, "RestartPolicy": {"Name": "no", "MaximumRetryCount": 0},
                "Memory": 536870912, "NanoCpus": 1000000000, "PidsLimit": 64,
                "Tmpfs": {"/tmp": "rw,noexec,nosuid,nodev,size=8m"}}
        for key in ["ExtraHosts", "Dns", "DnsOptions", "DnsSearch"]:
            host[key] = copy.deepcopy(api["HostConfig"].get(key))
        labels = dict(self.images[self.b["services"]["api"]["old_image_id"]]["Config"].get("Labels") or {})
        labels.update({"qs.m4.a.operation": self.root.name, "qs.m4.a.purpose": "fixed-native-tool"})
        keys = [key for key in env if key != "PATH"]
        if len(keys) != 2 or any(not re.fullmatch(r"QS_(?:M4_OBJECT|OFFLINE)_(?:MYSQL_DSN|MONGO_URI)", key) for key in keys) or any(any(c in env[key] for c in "\x00\r\n") for key in keys):
            reject("fixed_stdin_native_environment_invalid")
        script = "set -eu; " + "; ".join("IFS= read -r " + key for key in keys) + "; export " + " ".join(keys) + "; exec \"$@\""
        body = {"Image": self.b["services"]["api"]["old_image_id"], "User": "0:0", "Entrypoint": ["/bin/sh"], "Cmd": ["-c", script, "m4-cli"] + args,
                "Env": ["PATH=" + env["PATH"]], "OpenStdin": True, "StdinOnce": True, "AttachStdin": True, "AttachStdout": True, "AttachStderr": True, "Tty": False, "WorkingDir": str(self.root),
                "Labels": labels,
                "HostConfig": host, "NetworkingConfig": {"EndpointsConfig": {key: {"Aliases": [name]} for key in api["NetworkSettings"]["Networks"]}}}
        # Pre-register the unique name before create; cleanup never touches an
        # unregistered name or calls force/rm -v. No raw create body is saved.
        self.native_carrier_attempt += 1  # reserved once; never roll back with a failed action
        self.owned_cli[name] = {"dispatched": False, "known_cid": None, "remove_acknowledged": False, "image": body["Image"], "labels": labels}
        cid = self.engine_create(name, body, self.owned_cli[name])
        d = self.inspect(cid)
        if d["Config"]["Labels"] != body["Labels"] or d["Config"]["Entrypoint"] != body["Entrypoint"] or d["Config"]["Cmd"] != body["Cmd"] or d["Config"]["User"] != "0:0" or any(key + "=" in value for key in keys for value in d["Config"]["Env"]) or d["State"]["Running"] or not d["State"]["StartedAt"].startswith("0001-"):
            reject("fixed_native_carrier_identity_failed")
        try:
            raw = self.run(["docker", "start", "-a", "-i", cid], seconds, input_raw=("\n".join(env[key] for key in keys) + "\n").encode())
            d = self.inspect(cid)
            if d["State"]["Running"] or d["State"]["ExitCode"] != 0 or d["State"]["OOMKilled"]:
                reject("fixed_native_carrier_outcome_unknown_or_failed")
            return raw, d["State"]["ExitCode"]
        finally:
            self.release_native_carriers()

    def release_native_carriers(self):
        for name, intent in list(self.owned_cli.items()):
            if not intent["dispatched"]:
                if intent["known_cid"] is not None or intent["remove_acknowledged"]:
                    reject("undispatched_native_intent_invalid")
                del self.owned_cli[name]
                continue
            found = self.checked_CIDs(self.run(["docker", "ps", "-aq", "--no-trunc", "--filter", "name=^/" + name + "$"], 5))
            if len(found) > 1:
                reject("native_carrier_name_not_unique")
            if not found:
                if intent["known_cid"] is None or not intent["remove_acknowledged"]:
                    reject("dispatched_native_create_or_release_unproven")
                self.bind_created_identity(intent, intent["known_cid"])
            else:
                cid = found[0]
                self.bind_created_identity(intent, cid)
                if intent["remove_acknowledged"]:
                    reject("native_carrier_still_present_after_remove_ACK")
                d = self.inspect(cid)
                if d["Id"] != cid or d["Name"] != "/" + name or d["Config"]["Labels"] != intent["labels"] or d["Image"] != intent["image"]:
                    reject("owned_native_carrier_binding_lost")
                if d["State"]["Running"]:
                    reject("native_carrier_still_running_hold_entry_and_lock")
                self.run(["docker", "rm", cid], 10)
                intent["remove_acknowledged"] = True
            self.prove_removed_absence(name, intent["known_cid"])
            del self.owned_cli[name]

    def capture_evidence(self, supplied_sha):
        if not self.capture_evidence_sha or supplied_sha != self.capture_evidence_sha:
            reject("independent_capture_evidence_binding_mismatch")
        raw = capture_private_bytes(self.root / "capture-execution.json", self.capture_evidence_sha, 32 * 1024)
        proof = capture_execution_receipt(raw, self.capture_evidence_sha, str(self.root / "capture-stdout.json"))
        stdout_sha = proof["receipt_sha256"]
        stdout = capture_private_bytes(self.root / "capture-stdout.json", stdout_sha, 64 * 1024)
        capture_receipt(stdout, self.material_sha)
        self.guard()
        if time.monotonic() >= self.deadline:
            reject("capture_evidence_original_deadline_exhausted")
        return raw, stdout

    def capture_proof_copy(self, payload):
        if self.host != "serverA" or not self.frozen:
            reject("capture_proof_copy_requires_frozen_serverA")
        if type(payload) is not dict or set(payload) != {"material_sha256", "capture_evidence_sha256"}:
            reject("capture_proof_copy_fixed_inputs_required")
        if not self.material_sha or payload["material_sha256"] != self.material_sha:
            reject("capture_proof_copy_material_binding_mismatch")
        self.guard()
        execution, stdout = self.capture_evidence(payload["capture_evidence_sha256"])
        return {"capture_execution_base64": base64.b64encode(execution).decode(),
                "capture_stdout_base64": base64.b64encode(stdout).decode()}

    def freeze_capture_evidence(self, raw, exit_code):
        if type(exit_code) is not int or exit_code != 0:
            reject("capture_actual_process_exit_not_successful")
        material_sha = filehash(self.root / "first-material.json")
        parsed = capture_receipt(raw, material_sha)
        stdout_path, proof_path = self.root / "capture-stdout.json", self.root / "capture-execution.json"
        durable(stdout_path, raw)
        stdout_sha = digest(raw)
        capture_private_bytes(stdout_path, stdout_sha, 64 * 1024)
        proof_raw = encoded({"format_version": 1, "kind": "m4_a_capture_execution",
                             "receipt_file": str(stdout_path), "receipt_sha256": stdout_sha,
                             "termination": "exited", "exit_code": exit_code})
        durable(proof_path, proof_raw)
        proof_sha = digest(proof_raw)
        capture_private_bytes(proof_path, proof_sha, 32 * 1024)
        self.guard()
        if time.monotonic() >= self.deadline:
            reject("capture_evidence_original_deadline_exhausted")
        # Partial files are retained on failure and never treated as success.
        self.material_sha, self.capture_evidence_sha = material_sha, proof_sha
        return dict(parsed, capture_evidence_sha256=proof_sha)

    def native(self, action, payload):
        if self.host != "serverA" or not self.frozen:
            reject("native_requires_fixed_frozen_serverA")
        self.guard()
        dbflags = ["--mysql-database=qs", "--mongo-database=qs", "--timeout=" + str(payload["timeout_seconds"]) + "s"]
        env = {"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
        if action in ["capture", "verify-target", "restore", "verify-restored"]:
            env.update(QS_M4_OBJECT_MYSQL_DSN=self.credentials["mysql_dsn"], QS_M4_OBJECT_MONGO_URI=self.credentials["mongo_uri"])
            args = [str(self.root / "protection"), "--action=" + action] + dbflags
            if action == "capture":
                args += ["--capture-file=" + str(self.root / "first-material.json")]
            else:
                if payload["material_sha256"] != self.material_sha or filehash(self.root / "first-material.json") != self.material_sha:
                    reject("independent_first_material_binding_mismatch")
                args += ["--material-file=" + str(self.root / "first-material.json"), "--material-sha256=" + self.material_sha]
            if action in ["restore", "verify-restored"]:
                if self.crossed or self.old_runtime_started or (self.root / "first-runtime.json").exists():
                    reject("restore_forbidden_after_first_runtime_intent")
                self.capture_evidence(payload.get("capture_evidence_sha256"))
                approval = base64.b64decode(payload["approval_raw"], validate=True)
                if digest(approval) != payload["approval_sha256"]:
                    reject("independent_restore_approval_changed")
                if decode(approval)["first_new_runtime_marker"] != str(self.root / "first-runtime.json"):
                    reject("actual_first_runtime_marker_path_mismatch")
                if action == "restore":
                    durable(self.root / "restore-approval.json", approval)
                elif filehash(self.root / "restore-approval.json") != payload["approval_sha256"]:
                    reject("original_restore_approval_not_retained")
                args += ["--approval-file=" + str(self.root / "restore-approval.json"), "--approval-sha256=" + payload["approval_sha256"]]
            if action == "verify-target":
                self.capture_evidence(payload.get("capture_evidence_sha256"))
            if action != "capture":
                args += ["--capture-evidence-file=" + str(self.root / "capture-execution.json"), "--capture-evidence-sha256=" + self.capture_evidence_sha]
        else:
            env.update(QS_OFFLINE_MYSQL_DSN=self.credentials["mysql_dsn"], QS_OFFLINE_MONGO_URI=self.credentials["mongo_uri"])
            args = [str(self.root / "offline")] + dbflags
            if action == "apply":
                approval = base64.b64decode(payload["approval_raw"], validate=True)
                if digest(approval) != payload["approval_sha256"]:
                    reject("external_offline_approval_changed")
                durable(self.root / "offline-approval.json", approval)
                args += ["--apply", "--approval-file=" + str(self.root / "offline-approval.json"), "--approval-sha256=" + payload["approval_sha256"]]
            elif action != "inspect":
                reject("native_action_invalid")
        raw, exit_code = self.run_native_carrier(args, env, payload["timeout_seconds"] + 10)
        self.guard()
        if action == "capture":
            return self.freeze_capture_evidence(raw, exit_code)
        return decode(raw)

    def material_chunk(self, payload):
        # Fixed named object only; never exports configs, DB business bodies,
        # Docker mounts or arbitrary paths. Bytes are the actual tool material.
        if not self.material_sha or payload["material_sha256"] != self.material_sha:
            reject("first_material_not_bound")
        offset = payload["offset"]
        if type(offset) is not int or offset < 0 or offset % 49152:
            reject("material_chunk_offset_invalid")
        p = self.root / "first-material.json"
        with p.open("rb") as f:
            f.seek(offset)
            raw = f.read(49152)
        return {"offset": offset, "data": base64.b64encode(raw).decode(), "eof": offset + len(raw) == p.stat().st_size, "bytes": p.stat().st_size}

    def first_runtime(self, payload):
        if not self.frozen or self.crossed or payload["source_A"] != A:
            reject("first_runtime_boundary_invalid")
        self.guard()
        self.verify_prepared()
        durable(self.root / "first-runtime.json", json.dumps(payload, sort_keys=True).encode())
        self.crossed = True
        return {"durable_first_runtime_intent": True, "marker_sha256": filehash(self.root / "first-runtime.json"), "started_or_ready_proven": False, "recovery_forward_only": True}

    def recovery_decision(self, payload):
        if self.crossed or self.old_runtime_started or not self.frozen or (self.root / "first-runtime.json").exists():
            reject("prestart_recovery_boundary_refused")
        self.guard()
        now = datetime.datetime.now(datetime.timezone.utc)
        decision = datetime.datetime.fromisoformat(payload["recovery_decision_at"].replace("Z", "+00:00"))
        deadline = datetime.datetime.fromisoformat(payload["restore_deadline"].replace("Z", "+00:00"))
        if not decision <= now < deadline or not datetime.timedelta(0) < deadline - decision <= datetime.timedelta(hours=1):
            reject("external_recovery_deadline_expired_or_over_budget")
        self.deadline = time.monotonic() + (deadline - now).total_seconds()
        self.runtime_records(False, OLD)
        durable(self.root / "prestart-recovery-decision.json", json.dumps(payload, sort_keys=True).encode())
        return {"prestart_only": True, "external_absolute_restore_deadline": payload["restore_deadline"], "first_runtime_marker_absent": True}

    def start(self, old=False):
        if not self.frozen or (not old and not self.crossed) or (old and (self.crossed or (self.root / "first-runtime.json").exists())):
            reject("runtime_start_boundary_invalid")
        self.guard()
        if old:
            self.candidate_mutating.set()
            try:
                self.unprepare()
            finally:
                self.candidate_mutating.clear()
            ids = [cid for role in self.roles for cid in self.b["services"][role]["old_container_ids"]]
        else:
            self.verify_prepared()
            ids = [self.prepared[cid]["new_cid"] for role in self.roles for cid in self.b["services"][role]["old_container_ids"]]
        if old:
            # Old runtime may create new facts too. Mark the dispatch boundary
            # immediately before start; never re-enter object restore afterwards.
            durable(self.root / "first-runtime.json", encoded({"kind": "original_runtime_start_intent", "source_A": A, "original_source": OLD, "original_container_ids": ids, "first_material_sha256": self.material_sha, "decision": "start_original_runtime_after_independent_original_state_verification"}))
            self.old_runtime_started = True
        self.run(["docker", "start"] + ids, 90)
        if old:
            self.frozen = False
        return {"runtime_start_commands_completed": True, "old_image_path": old, "original_container_CIDs_retained": True, "business_acceptance": False}

    def verify_prepared(self):
        expected = {cid for role in self.roles for cid in self.b["services"][role]["old_container_ids"]}
        if set(self.prepared) != expected:
            reject("complete_never_started_candidate_set_missing")
        for cid, candidate in self.prepared.items():
            old = self.inspect(cid)
            if old["State"]["Running"] or old["Image"] != self.old[cid]["Image"]:
                reject("original_writer_changed_during_preparation")
            image = self.b["services"][candidate["role"]]["new_image"]["actual_engine_id"]
            verify_candidate(self.old[cid], self.inspect(candidate["new_cid"]), image, candidate["name"])

    def verify_runtime(self, old=False):
        source = OLD if old else A
        rows = self.runtime_records(True, source)
        safe = []
        for role in self.roles:
            for row in rows[role]:
                if old:
                    original_cid = row["cid"]
                else:
                    original_cid = next(cid for cid, v in self.prepared.items() if v["new_cid"] == row["cid"])
                    candidate = self.prepared[original_cid]
                    verify_candidate(self.old[original_cid], self.inspect(row["cid"]), self.b["services"][role]["new_image"]["actual_engine_id"], candidate["name"], started=True)
                baseline = next(v for v in self.baseline[role] if v["cid"] == original_cid)
                if any(row[k] != baseline[k] for k in ["env", "mounts", "networks", "restart_policy"]):
                    reject("runtime_configuration_changed")
                cid = row["cid"]
                if self.run(["docker", "exec", cid, "readlink", "/proc/1/exe"], 5).decode().strip() != PROGRAM[role]:
                    reject("actual_PID1_program_mismatch")
                actual = self.run(["docker", "exec", cid, "sha256sum", PROGRAM[role]], 5).decode().split()[0]
                running = self.run(["docker", "exec", cid, "sha256sum", "/proc/1/exe"], 5).decode().split()[0]
                expected = self.b["services"][role]["old_ELF_sha256"] if old else self.b["services"][role]["new_image"]["actual_elf_sha256"]
                if actual != running or actual != expected:
                    reject("actual_running_ELF_mismatch")
                version = self.run(["docker", "exec", cid, PROGRAM[role], "--config=" + CONFIG[role], "--version"], 10).decode()
                if re.findall(r"(?m)^\s*gitCommit:\s*([0-9a-f]{40})\s*$", version) != [source]:
                    reject("actual_compiled_source_mismatch")
                status = self.run(["docker", "exec", cid, "cat", "/proc/1/status"], 5).decode()
                uid = re.findall(r"(?m)^Uid:\s+([0-9]+)\s+([0-9]+)\s+([0-9]+)\s+([0-9]+)\s*$", status)
                gid = re.findall(r"(?m)^Gid:\s+([0-9]+)\s+([0-9]+)\s+([0-9]+)\s+([0-9]+)\s*$", status)
                if uid != [("2000",) * 4] or gid != [("2000",) * 4]:
                    reject("actual_PID1_normal_user_mismatch")
                probes = ["/readyz", "/serve-readyz"] if role == "collection" else ["/readyz"]
                port = 9092 if role == "worker" else 8080
                for path in probes:
                    response = decode(self.run(["docker", "exec", cid, "wget", "-qO-", "http://127.0.0.1:" + str(port) + path], 5))
                    if response.get("status") != "ready" or (path == "/serve-readyz" and response.get("serve_ready") is not True):
                        reject("actual_ready_gate_failed")
                safe.append({"role": role, "container_id": cid, "actual_engine_id": row["image_id"], "actual_source": source, "actual_ELF_sha256": actual, "actual_PID1_UID_GID": [2000, 2000], "actual_config_environment_mounts_equal": True, "applicable_ready_passed": True})
        if not old:
            self.new_verified = True
        return {"instances": safe, "complete": True, "business_acceptance": False}

    def retire_exact_old(self, payload):
        if not self.crossed or not self.new_verified or not self.allow_exact_old_cleanup or payload.get("both_hosts_new_six_verified") is not True:
            reject("exact_old_cleanup_not_authorized_or_new_six_unverified")
        self.verify_runtime(old=False)
        removed = []
        for role in self.roles:
            for cid in self.b["services"][role]["old_container_ids"]:
                d = self.inspect(cid)
                if d["State"]["Running"] or d["Image"] != self.old[cid]["Image"] or d["Config"] != self.old[cid]["Config"] or d["HostConfig"] != self.old[cid]["HostConfig"] or d["Name"] != "/" + self.parked[cid]["parked"]:
                    reject("exact_stopped_original_CID_identity_changed")
                self.run(["docker", "rm", cid], 15)  # no force/no volumes/images
                self.retired_old.add(cid)
                removed.append(cid)
        self.verify_runtime(old=False)
        return {"complete": True, "exact_original_stopped_CIDs_removed": removed, "image_or_volume_removed": False, "duplicate_Compose_labels_retired": True}

    def dispatch(self, request):
        action = request["action"]
        payload = request.get("payload", {})
        if action == "begin-window":
            if self.window_started or self.frozen or self.crossed:
                reject("maintenance_window_already_started")
            now = datetime.datetime.now(datetime.timezone.utc)
            start = datetime.datetime.fromisoformat(payload["window_started_at"].replace("Z", "+00:00"))
            end = start + datetime.timedelta(seconds=1800)
            if not start <= now < end:
                reject("external_maintenance_window_invalid")
            self.deadline = time.monotonic() + (end - now).total_seconds()
            self.window_started = True
            result = {"maintenance_budget_seconds": 1800, "window_started_at": payload["window_started_at"]}
        elif action in ["entry-close", "entry-open"]:
            result = self.entry(open_entry=action == "entry-open")
        elif action == "recovery-decision":
            result = self.recovery_decision(payload)
        elif action == "stop":
            result = self.stop()
        elif action == "supply-tools":
            result = self.supply_tools(payload)
        elif action in ["capture", "inspect", "apply", "verify-target", "restore", "verify-restored"]:
            result = self.native(action, payload)
        elif action == "material-chunk":
            result = self.material_chunk(payload)
        elif action == "capture-proof-copy":
            if set(request) != {"action", "payload"}:
                reject("capture_proof_copy_fixed_frame_required")
            result = self.capture_proof_copy(payload)
        elif action == "prepare-new":
            result = self.prepare_new()
        elif action == "first-runtime":
            result = self.first_runtime(payload)
        elif action in ["start-new", "start-old"]:
            result = self.start(old=action == "start-old")
        elif action in ["verify-new", "verify-old"]:
            result = self.verify_runtime(old=action == "verify-old")
        elif action == "retire-exact-old":
            result = self.retire_exact_old(payload)
        elif action == "continuity":
            self.guard()
            self.hash_inputs()
            if self.frozen and not self.crossed and not self.old_runtime_started:
                self.runtime_records(False, OLD)
            result = {"complete": True, "fixed_process_freeze_continuity": self.frozen and not self.crossed, "database_write_absence_certified": False}
        else:
            reject("fixed_action_not_supported")
        self.journal.append({"action": action, "result_sha256": digest(json.dumps(result, sort_keys=True).encode())})
        return result

    def close(self):
        if any(v["dispatched"] and (not self.crossed or v["known_cid"] is None or v["remove_acknowledged"]) for v in self.parked.values()):
            reject("candidate_intent_release_unproven_hold_entry_and_lock")
        if self.owned_cli:
            self.release_native_carriers()
        self.finished.set()
        self.credentials.clear()
        if hasattr(self, "monitor"):
            self.monitor.join(3)
        if self.lock:
            fcntl.flock(self.lock, fcntl.LOCK_UN)
            self.lock.close()
        self.old.clear()
        self.baseline.clear()


def emit(value):
    print(json.dumps(value, separators=(",", ":")), flush=True)


def main():
    os.umask(0o077)
    receiver = None
    try:
        raw = sys.stdin.buffer.readline(MAX_FRAME + 1)
        if len(raw) > MAX_FRAME or not raw.endswith(b"\n"):
            reject("initial_frame_limit")
        receiver = Receiver(decode(raw))
        emit({"complete": True, "host": receiver.host, "existing_shared_lock_held": True, "fresh_bound_existing_config": True, "prefreeze_connection_handoff": receiver.connection_evidence})
        while True:
            raw = sys.stdin.buffer.readline(MAX_FRAME + 1)
            if not raw:
                break
            if len(raw) > MAX_FRAME or not raw.endswith(b"\n"):
                reject("frame_limit")
            request = decode(raw)
            if request.get("action") == "release":
                receiver.close()
                receiver = None
                emit({"complete": True, "shared_lock_released": True})
                return
            try:
                emit({"complete": True, "result": receiver.dispatch(request)})
            except Exception:
                # Keep the approved session/lock and unchanged credentials for
                # an independently authorized pre-start recovery decision.
                emit({"complete": False, "error_category": "maintenance_action_failed_or_unknown", "entry_must_remain_frozen": True, "recovery_forward_only": receiver.crossed})
        reject("controller_connection_lost_hold_maintenance")
    except Exception:
        # No exception text, CLI stderr, DSN, SQL/body or traceback crosses stdout.
        emit({"complete": False, "error_category": "maintenance_action_failed_or_unknown", "entry_must_remain_frozen": True, "recovery_forward_only": bool(receiver and receiver.crossed)})
        raise SystemExit(1)
    finally:
        if receiver:
            receiver.close()


if __name__ == "__main__":
    main()
