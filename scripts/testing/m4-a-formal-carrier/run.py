#!/usr/bin/env python3
"""One opt-in Ubuntu/root carrier qualification; never a production executor."""
import argparse
import base64
import fcntl
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import platform
import re
import secrets
import signal
import stat
import subprocess
import tarfile
import time

HERE = Path(__file__).resolve().parent
REMOTE_HASH = "5ac739cc4b6e2f6a7b3ca1b453d0dddfd2c1d0735a72c1b1aca53b1301454d6e"
TOOL_SOURCE = "602710eb2f6ee383923230890c4bdb4a3bcada3a"
TOOL_HASH = "2c314163ee6931aa133f3537b418927c97cce9f8fd8f4f3b75f42be142e173fd"
A_SOURCE = "51163edab7038adcdb6b8f1712d6dc434f361dc6"
RESOURCE = "77e11d801d270ef7b12f86261a05217a95bd8f8049f20d2e5f14ee0c0a20083b"
# This is the digest actually pulled by the qualified original f8f carrier.
MYSQL = "docker.io/library/mysql@sha256:a532724022429812ec797c285c1b540a644c15e248579c6bfdf12a8fbaab4964"
MONGO = "mongo@sha256:43fddee7e532a920f3dfdee9e8f4834398c155c26bcb92d790cc1cd3c630fc40"
LABEL = "qs.m4.a.formal-carrier.fixture"
STEPS = ("capture", "restore", "verify-restored", "verify-target")
# Build2x600 + setup240 + seed180 + native1800 + proof120 + cleanup120 =3660.
TOTAL_SECONDS = 3660
RUNTIME_SECONDS = 240 + 180 + 1800 + 120
CLEANUP_SECONDS = 120


def need(value, category):
    if not value:
        raise RuntimeError(category)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def pairs(items):
    value = {}
    for key, item in items:
        need(key not in value, "duplicate_JSON_member")
        value[key] = item
    return value


def decode(raw):
    return json.loads(raw, object_pairs_hook=pairs)


def save(path, value):
    raw = value if isinstance(value, bytes) else (json.dumps(value, indent=2) + "\n").encode()
    fd = os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    with os.fdopen(fd, "wb") as output:
        output.write(raw)
        output.flush()
        os.fsync(output.fileno())
    fd = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)
    need(path.read_bytes() == raw, "private_readback_failed")
    return digest(raw)


def load_remote():
    path = HERE / "remote-maintenance.py"
    need(digest(path.read_bytes()) == REMOTE_HASH, "original_V10_source_changed")
    spec = importlib.util.spec_from_file_location("fixed_original_V10_receiver", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def check_receipt(row, action):
    need(type(row) is dict and row.get("action") == action and row.get("complete") is True,
         "native_action_missing_or_negative")
    need(row.get("compiled_tool_source") == TOOL_SOURCE and row.get("a_execution_source") == A_SOURCE
         and row.get("a_execution_resources_sha256") == RESOURCE
         and row.get("a_execution_resource_files") == 280, "native_source_binding_failed")
    for flag in ("sql_connection_closed", "sql_pool_closed", "mongo_closed"):
        need(row.get(flag) is True, "native_physical_release_missing")
    need(row.get("error_category") == "none" and row.get("transaction_release_unproven") is False
         and row.get("total_budget_exceeded") is False, "native_unknown_or_budget_failure")


def require_mysql_image(image):
    need(type(image) is dict and image.get("Os") == "linux" and image.get("Architecture") == "amd64"
         and type(image.get("Id")) is str and re.fullmatch(r"sha256:[0-9a-f]{64}", image["Id"]),
         "fixed_MySQL_image_identity_required")
    repos = image.get("RepoDigests")
    # Docker may display its default registry/repository in familiar form.
    need(type(repos) is list and 1 <= len(repos) <= 32
         and all(type(item) is str and len(item) <= 256 for item in repos)
         and any(item in (MYSQL, MYSQL.removeprefix("docker.io/library/")) for item in repos),
         "fixed_MySQL_RepoDigest_required")


def qualify(result):
    """The actual final predicate, also exercised directly by pure tests."""
    need(type(result) is dict and result.get("kind") == "M4_A_original_native_carrier",
         "qualification_kind")
    need(result.get("tool_sha256") == TOOL_HASH and result.get("tool_exact_build") is True,
         "qualification_tool")
    mysql = result.get("mysql_image_binding")
    need(type(mysql) is dict and mysql.get("registry_ref") == MYSQL,
         "qualification_MySQL_binding")
    require_mysql_image(mysql.get("image"))
    need(mysql.get("container_image_id") == mysql["image"]["Id"]
         and mysql.get("actual_server_version") == "8.0.36", "qualification_MySQL_actual_identity")
    actions = result.get("actions")
    need(type(actions) is list and [item.get("action") for item in actions] == list(STEPS),
         "qualification_exact_actions")
    for row, action in zip(actions, STEPS):
        check_receipt(row, action)
    need(result.get("material_counts") == [247463, 247463]
         and type(result.get("material_bytes")) is int
         and 49_500_000 <= result["material_bytes"] <= 50_500_000, "qualification_scale")
    carriers = result.get("carriers")
    need(type(carriers) is list and len(carriers) == len(STEPS), "qualification_carrier_set")
    for carrier, action in zip(carriers, STEPS):
        need(carrier.get("action") == action and carrier.get("image_content_verified") is True
             and carrier.get("uid") == "0:0" and carrier.get("memory") == 536870912
             and carrier.get("native_bind_verified") is True and type(carrier.get("same_CID_exit")) is int
             and carrier.get("same_CID_exit") == 0
             and carrier.get("oom") is False and carrier.get("release_proven") is True,
             "qualification_actual_carrier")
    for flag in ("partial_dirty101_proven", "restored10039_witness", "target10139_witness",
                 "original_three_files_verified", "all_native_carriers_released",
                 "owned_cleanup_proven", "source_unchanged"):
        need(result.get(flag) is True, "qualification_" + flag)
    need(result.get("cleanup_errors") in (None, []) and not result.get("cleanup_failure")
         and not result.get("evidence_failure"), "qualification_cleanup_failure")
    need(result.get("production") is False and result.get("Mac_custody_executed") is False
         and result.get("business_runtime_started") is False, "qualification_scope")
    return True


class Fixture:
    def __init__(self, build, output):
        self.build, self.output = build, output
        output.mkdir(mode=0o700)
        self.began = time.monotonic()
        self.phase_deadline = self.began + 240
        self.cleanup_started = None
        self.owner = "m4-a-carrier-" + secrets.token_hex(8)
        self.network = self.owner + "-network"
        self.sqlvolume = self.owner + "-mysql"
        self.created = {}
        self.network_created = False
        self.volume_created = False
        self.bound = False
        self.sequence = 0
        self.remote = load_remote()
        self.receiver = None
        self.active_action = None
        self.observed = []
        self.source_hashes = {str(p.relative_to(HERE)): digest(p.read_bytes())
                              for p in HERE.iterdir() if p.is_file()}
        self.binding = decode((HERE / "original-api-binding.json").read_bytes())
        self.result = {"kind": "M4_A_original_native_carrier", "production": False,
                       "Mac_custody_executed": False, "business_runtime_started": False,
                       "full_six_instance_rehearsal": False, "SSH_Nginx_executed": False,
                       "production_30_60_RPO_business_proven": False,
                       "qualified": False, "owner": self.owner, "actions": [], "carriers": [],
                       "cleanup_errors": [], "tool_sha256": TOOL_HASH}

    def remaining(self):
        if self.cleanup_started is not None:
            return CLEANUP_SECONDS - (time.monotonic() - self.cleanup_started)
        remaining = RUNTIME_SECONDS - (time.monotonic() - self.began)
        if self.phase_deadline is not None:
            remaining = min(remaining, self.phase_deadline - time.monotonic())
        return remaining

    def cmd(self, argv, seconds=15, raw=None, child_env=None, output_limit=150 << 20):
        need(self.remaining() > 0, "fixed_fixture_budget_exhausted")
        # Fixed synthetic credentials only; no process environment is inherited.
        process = subprocess.run(argv, input=raw, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                 timeout=min(seconds, self.remaining()),
                                 env=dict({"PATH": os.environ["PATH"], "HOME": "/root",
                                           "DOCKER_HOST": "unix:///var/run/docker.sock"}, **(child_env or {})), check=False)
        need(len(process.stdout) <= output_limit and len(process.stderr) <= 65536,
             "fixture_output_bound")
        row = {"argv": argv, "exit_code": process.returncode,
               "stdout_sha256": digest(process.stdout), "stderr_sha256": digest(process.stderr)}
        save(self.output / ("command-%04d.json" % self.sequence), row)
        self.sequence += 1
        need(process.returncode == 0, "fixed_fixture_command_failed")
        return process.stdout

    def docker(self, *args, **kwargs):
        return self.cmd(["docker"] + list(args), **kwargs)

    def inspect(self, cid):
        return decode(self.docker("inspect", "--type=container", cid))[0]

    def create(self, role, image, args):
        name = self.owner + "-" + role
        # Intent remains known even if create's ACK is lost.
        self.created[name] = {"image": image, "cid": None}
        raw = self.docker("create", "--name", name, "--label", LABEL + "=" + self.owner,
                          "--network", self.network, "--restart=no", *args, image)
        cid = raw.decode().strip()
        need(re.fullmatch(r"[0-9a-f]{64}", cid), "fixture_create_CID_unknown")
        self.created[name]["cid"] = cid
        actual = self.inspect(cid)
        need(actual["Name"] == "/" + name and actual["Config"]["Labels"].get(LABEL) == self.owner,
             "fixture_created_identity")
        return cid

    def ready(self, cid, args):
        end = time.monotonic() + 90
        while time.monotonic() < end:
            state = self.inspect(cid)["State"]
            need(state["Running"] and not state["OOMKilled"], "owned_dependency_exited")
            try:
                return self.docker("exec", cid, *args, seconds=5)
            except RuntimeError:
                time.sleep(2)
        raise RuntimeError("owned_dependency_readiness_exhausted")

    def setup(self):
        need(os.getuid() == 0 and platform.system() == "Linux" and platform.machine() in ("x86_64", "amd64"),
             "native_Linux_amd64_root_required")
        need(os.environ.get("GITHUB_ACTIONS") == "true"
             and os.environ.get("GITHUB_REPOSITORY", "").lower() == "fangcunmount/qs-server"
             and os.environ.get("QS_M4_A_CARRIER_ACK") == "isolated_original_V10_carrier_only",
             "fixed_opt_in_CI_required")
        info = decode(self.docker("info", "--format", "{{json .}}"))
        need(info["OSType"] == "linux" and info["Architecture"] in ("x86_64", "amd64"),
             "native_host_Docker_required")
        self.before_cids = set(self.docker("ps", "-aq", "--no-trunc").decode().split())
        need(not self.before_cids, "exclusive_ephemeral_runner_required")
        self.bound = True
        build = decode((self.build / "build.json").read_bytes())
        need(build.get("tool_exact_bytes") is True and build.get("tool_source") == TOOL_SOURCE
             and digest((self.build / "protection").read_bytes()) == TOOL_HASH
             and build.get("fixture_sha256") == digest((self.build / "fixture.test").read_bytes()),
             "exact_build_inputs_required")
        self.result["tool_exact_build"] = True
        # setup-go builds as runner UID. A cap-drop root container cannot
        # execute another UID's0700 bind; stage exact helper bytes as root.
        self.helper_binary = self.output / "fixture.test"
        save(self.helper_binary, (self.build / "fixture.test").read_bytes())
        self.helper_binary.chmod(0o700)
        need(self.helper_binary.stat().st_uid == 0
             and digest(self.helper_binary.read_bytes()) == build["fixture_sha256"], "root_owned_helper_binding_required")
        image = decode(self.docker("image", "inspect", self.binding["registry_ref"]))[0]
        self.api_image = image["Id"]
        need(image["Os"] == "linux" and image["Architecture"] == "amd64"
             and image["RootFS"]["Layers"] == self.binding["rootfs_diff_ids"]
             and image["Config"]["User"] == "www"
             and image["Config"]["Entrypoint"] == ["/app/qs-apiserver"], "old_M2_image_mismatch")
        # Digest/config are different authorities on containerd-backed daemons.
        # Fixed original M2 image content is230144789B plus tar metadata.
        # Only this archive read gets256MiB; other command output stays150MiB.
        archive = self.docker("image", "save", self.binding["registry_ref"], seconds=60,
                              output_limit=256 << 20)
        with tarfile.open(fileobj=io.BytesIO(archive)) as stream:
            manifest = decode(stream.extractfile("manifest.json").read())
            need(len(manifest) == 1, "single_original_image_required")
            config = stream.extractfile(manifest[0]["Config"]).read(1 << 20)
            need(digest(config) == self.binding["config_sha256"], "old_M2_raw_config_mismatch")
        mysql_images = decode(self.docker("image", "inspect", MYSQL))
        need(type(mysql_images) is list and len(mysql_images) == 1, "single_fixed_MySQL_image_required")
        require_mysql_image(mysql_images[0])
        self.result["mysql_image_binding"] = {
            "registry_ref": MYSQL,
            "image": {key: mysql_images[0][key] for key in ("Id", "RepoDigests", "Os", "Architecture")},
        }
        self.network_created = True
        self.network_id = self.docker("network", "create", "--internal", "--label", LABEL + "=" + self.owner,
                                      self.network).decode().strip()
        need(os.statvfs(self.output).f_bavail * os.statvfs(self.output).f_frsize >= 8 << 30,
             "owned_fixture_disk_reserve_required")
        self.volume_created = True  # creation ACK may be unknown
        self.docker("volume", "create", "--label", LABEL + "=" + self.owner, self.sqlvolume)
        self.mysql = self.create_command("mysql", MYSQL, ["--memory=1g", "--memory-swap=1g", "--cpus=1",
                                "--network-alias=mysql", "--env=MYSQL_ALLOW_EMPTY_PASSWORD=yes", "--env=MYSQL_DATABASE=qs",
                                "--mount=type=volume,src=" + self.sqlvolume + ",dst=/var/lib/mysql",
                                "--entrypoint=/usr/local/bin/docker-entrypoint.sh"], ["mysqld", "--innodb-buffer-pool-size=64M"])
        self.result["mysql_image_binding"]["container_image_id"] = self.inspect(self.mysql)["Image"]
        need(self.result["mysql_image_binding"]["container_image_id"]
             == self.result["mysql_image_binding"]["image"]["Id"], "fixed_MySQL_container_image_mismatch")
        self.docker("start", self.mysql)
        self.ready(self.mysql, ["mysqladmin", "ping", "-uroot", "--silent"])
        actual = self.docker("exec", self.mysql, "mysql", "-uroot", "-N", "-e", "SELECT VERSION()")
        need(actual.strip() == b"8.0.36", "actual_MySQL8.0.36_required")
        self.result["mysql_image_binding"]["actual_server_version"] = actual.strip().decode("ascii")
        self.mongo = self.create_command("mongo", MONGO, ["--memory=2g", "--memory-swap=2g", "--cpus=1",
                              "--network-alias=mongo", "--tmpfs=/data/db:rw,size=1g",
                              "--tmpfs=/data/configdb:rw,size=128m", "--entrypoint=mongod"],
                              ["--replSet", "rs0", "--bind_ip_all", "--wiredTigerCacheSizeGB", "0.25"])
        self.docker("start", self.mongo)
        self.ready(self.mongo, ["mongosh", "--quiet", "--eval", "quit(db.adminCommand({ping:1}).ok===1?0:1)"])
        self.docker("exec", self.mongo, "mongosh", "--quiet", "--eval",
                    "if(db.adminCommand({buildInfo:1}).version!=='7.0.31')quit(1);rs.initiate({_id:'rs0',members:[{_id:0,host:'mongo:27017'}]})")
        self.ready(self.mongo, ["mongosh", "--quiet", "--eval", "quit(db.adminCommand({hello:1}).isWritablePrimary?0:1)"])
        self.anchor = self.create("old-api-anchor", self.api_image, ["--entrypoint=/bin/false", "--read-only"])
        raw = self.docker("cp", self.anchor + ":/app/qs-apiserver", "-", seconds=30)
        with tarfile.open(fileobj=io.BytesIO(raw)) as stream:
            members = stream.getmembers()
            need(len(members) == 1 and members[0].isfile() and members[0].size == self.binding["ELF_bytes"],
                 "exact_ELF_archive_required")
            need(digest(stream.extractfile(members[0]).read()) == self.binding["ELF_sha256"], "old_M2_ELF_mismatch")
        self.result["image_binding"] = {"registry_ref": self.binding["registry_ref"], "actual_engine_id": self.api_image,
                                         "config_sha256": self.binding["config_sha256"], "ELF_sha256": self.binding["ELF_sha256"]}

    def create_command(self, role, image, args, command):
        name = self.owner + "-" + role
        self.created[name] = {"image": image, "cid": None}
        raw = self.docker("create", "--name", name, "--label", LABEL + "=" + self.owner,
                          "--network", self.network, "--restart=no", *args, image, *command)
        cid = raw.decode().strip()
        need(re.fullmatch(r"[0-9a-f]{64}", cid), "fixture_create_CID_unknown")
        self.created[name]["cid"] = cid
        actual = self.inspect(cid)
        need(actual["Name"] == "/" + name and actual["Image"] == decode(self.docker("image", "inspect", image))[0]["Id"]
             and actual["Config"]["Labels"].get(LABEL) == self.owner and not actual["State"]["Running"],
             "fixed_created_command_identity")
        return cid

    def helper(self, action):
        role = "helper-" + action + "-" + str(self.sequence)
        cid = self.create_command(role, self.api_image, ["--user=0:0", "--memory=1g", "--memory-swap=1g",
                  "--cpus=1", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges",
                  "--tmpfs=/tmp:rw,size=64m", "--mount=type=bind,src=" + str(self.helper_binary) + ",dst=/fixture.test,readonly",
                  "--env=QS_M4_CARRIER_FIXTURE_ACK=owned_synthetic_only", "--env=QS_M4_CARRIER_FIXTURE_ACTION=" + action,
                  "--entrypoint=/fixture.test"], ["-test.run=^TestM4AFormalCarrierFixture$", "-test.count=1", "-test.timeout=180s", "-test.v"])
        raw = self.docker("start", "--attach", cid, seconds=185)
        state = self.inspect(cid)["State"]
        need(not state["Running"] and not state["OOMKilled"] and state["ExitCode"] == 0, "fixture_helper_exit_unknown")
        rows = [decode(line[len(b"A_FORMAL_CARRIER_FIXTURE "):]) for line in raw.splitlines()
                if line.startswith(b"A_FORMAL_CARRIER_FIXTURE ")]
        need(len(rows) == 1 and rows[0].get("action") == action and rows[0].get("complete") is True,
             "fixture_helper_unique_success_required")
        save(self.output / (role + ".json"), rows[0])
        return rows[0]

    def construct_receiver(self):
        remote = self.remote
        operation = str(time.time_ns()) + "-1"
        root = Path("/opt/qs-server/m4-cleanup-a") / operation
        root.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        need(root.parent.stat().st_uid == 0 and stat.S_IMODE(root.parent.stat().st_mode) == 0o700,
             "canonical_private_Linux_parent_required")
        root.mkdir(mode=0o700)
        remote.LOCK.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        fd = os.open(remote.LOCK, os.O_RDWR | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        lock = os.fdopen(fd, "r+")
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        receiver = remote.Receiver.__new__(remote.Receiver)
        receiver.host, receiver.root, receiver.frozen = "serverA", root, True
        receiver.crossed = receiver.old_runtime_started = False
        receiver.freeze_failure = None
        receiver.lock, receiver.deadline = lock, time.monotonic() + 1800
        receiver.owned_cli, receiver.native_carrier_attempt = {}, 0
        receiver.before_first_create_ids = None
        receiver.material_sha = receiver.capture_evidence_sha = None
        self.receiver = receiver  # retain the actual lock even if later setup fails
        self.result["operation_root"] = str(root)
        receiver.b = {"services": {"api": {"old_container_ids": [self.anchor], "old_image_id": self.api_image}}}
        receiver.old = {self.anchor: self.inspect(self.anchor)}
        receiver.images = {self.api_image: decode(self.docker("image", "inspect", self.api_image))[0]}
        receiver.credentials = {"mysql_dsn": "root@tcp(mysql:3306)/qs?parseTime=true", "mongo_uri": "mongodb://mongo:27017/?replicaSet=rs0&directConnection=true"}
        # Observation wraps only inspect; original method still performs actual
        # guard/run/Docker reads. No carrier creation, run, release or guard seam.
        def observed_inspect(cid):
            actual = remote.Receiver.inspect(receiver, cid)
            if actual["Config"].get("Labels", {}).get("qs.m4.a.purpose") == "fixed-native-tool":
                self.observed.append(actual)
            return actual
        receiver.inspect = observed_inspect
        for name in ("protection",):
            remote.durable(root / name, (self.build / name).read_bytes())
            (root / name).chmod(0o700)

    def action(self, action, extra=None):
        receiver = self.receiver
        limit = min(300, int(receiver.deadline - time.monotonic()) - 300)
        need(limit > 0, "original_startup_reserve_required")
        start = len(self.observed)
        payload = {"timeout_seconds": limit}
        if action != "capture":
            payload.update(material_sha256=receiver.material_sha, capture_evidence_sha256=receiver.capture_evidence_sha)
        payload.update(extra or {})
        result = receiver.native(action, payload)
        check_receipt(result, action)
        self.result["actions"].append(result)
        observations = self.observed[start:]
        terminal = [row for row in observations if row["State"]["StartedAt"][:5] != "0001-" and not row["State"]["Running"]]
        need(terminal, "actual_carrier_exit_not_observed")
        row = terminal[-1]
        host = row["HostConfig"]
        mounts = row["Mounts"]
        native_bind = [item for item in mounts if item["Destination"] == str(receiver.root)]
        need(len(native_bind) == 1 and native_bind[0]["Type"] == "bind"
             and native_bind[0]["Source"] == str(receiver.root) and native_bind[0]["RW"] is True,
             "same_canonical_native_bind_required")
        need(host["Memory"] == 536870912 and host["NanoCpus"] == 1000000000 and host["PidsLimit"] == 64
             and host["ReadonlyRootfs"] and host["CapDrop"] == ["ALL"]
             and "no-new-privileges" in host["SecurityOpt"] and host["Tmpfs"] == {"/tmp": "rw,noexec,nosuid,nodev,size=8m"}
             and row["Image"] == self.api_image and row["Config"]["User"] == "0:0", "formal_carrier_configuration_drift")
        need(not receiver.owned_cli, "original_carrier_release_unproven")
        self.result["carriers"].append({"action": action, "cid": row["Id"], "image_content_verified": True,
                    "uid": row["Config"]["User"], "memory": host["Memory"], "native_bind_verified": True,
                    "same_CID_exit": row["State"]["ExitCode"], "oom": row["State"]["OOMKilled"], "release_proven": True})
        save(self.output / ("carrier-" + action + ".json"), {"receipt": result, "observations": observations})
        return result

    def exercise(self):
        self.phase_deadline = time.monotonic() + 180
        self.helper("seed")
        self.phase_deadline = None
        self.construct_receiver()
        self.action("capture")
        receiver, remote = self.receiver, self.remote
        raw = remote.capture_private_bytes(receiver.root / "first-material.json", receiver.material_sha, 128 << 20)
        material = decode(raw)
        self.result["material_bytes"] = len(raw)
        self.result["material_counts"] = [len(table["primary_key_retained_hashes"]) for table in material["tables"]]
        proof = receiver.capture_proof_copy({"material_sha256": receiver.material_sha,
                                             "capture_evidence_sha256": receiver.capture_evidence_sha})
        self.result["material_sha256"] = receiver.material_sha
        self.result["capture_evidence_sha256"] = receiver.capture_evidence_sha
        custody = self.output / "originals"
        custody.mkdir(mode=0o700)
        save(custody / "first-material.json", raw)
        for key, name in [("capture_execution_base64", "capture-execution.json"), ("capture_stdout_base64", "capture-stdout.json")]:
            save(custody / name, base64.b64decode(proof[key], validate=True))
        self.result["original_three_files_verified"] = True
        partial = self.helper("partial")
        need(partial["mysql_version"] == 101 and partial["mysql_dirty"] is True
             and "historical_lifecycle_evidence" not in partial["assessment_ddl"]
             and "historical_committed_evidence" in partial["evaluation_outcome_ddl"], "actual_partial_dirty101_required")
        self.result["partial_dirty101_proven"] = True
        save(self.output / "synthetic-responsibility.json", {"production": False, "no_business_process_created": True, "fixed_owner": self.owner})
        save(self.output / "capture-state.json", {"material_sha256": receiver.material_sha,
                                                   "capture_evidence_sha256": receiver.capture_evidence_sha})
        save(self.output / "independent-partial.json", partial)
        reply = decode(self.cmd(["python3", "-B", str(HERE / "approve.py"),
                                 "--operation-root", str(receiver.root), "--evidence-root", str(self.output)], seconds=30,
                                child_env={"GITHUB_ACTIONS": "true", "QS_M4_A_CARRIER_ACK": "isolated_original_V10_carrier_only"}))
        approval_raw = remote.capture_private_bytes(receiver.root / "independent-fixture-approval.json", reply["approval_sha256"], 32768)
        save(self.output / "fixture-restore-approval.json", approval_raw)
        extra = {"approval_raw": base64.b64encode(approval_raw).decode(), "approval_sha256": digest(approval_raw)}
        self.action("restore", extra)
        self.action("verify-restored", extra)
        restored = self.helper("witness")
        need(restored["mysql_version"] == 100 and restored["mysql_dirty"] is False
             and restored["mongo_version"] == 39 and restored["mongo_dirty"] is False
             and restored["mysql_identity_sha256"] == material["mysql_identity_sha256"]
             and restored["mongo_identity_sha256"] == material["mongo_identity_sha256"], "restored_identity_and_version")
        self.result["restored10039_witness"] = True
        target = self.helper("target")
        need(target["mysql_version"] == 101 and target["mysql_dirty"] is False
             and target["mongo_version"] == 39 and target["mongo_dirty"] is False, "target_version")
        self.action("verify-target")
        self.result["target10139_witness"] = True
        self.result["all_native_carriers_released"] = not receiver.owned_cli

    def cleanup(self):
        self.cleanup_started = time.monotonic()
        errors = []
        if not self.bound:
            self.result["owned_cleanup_proven"] = False
            return
        receiver = self.receiver
        if receiver is not None:
            try:
                receiver.release_native_carriers()
            except Exception as error:
                errors.append(type(error).__name__ + ":native_release_unproven")
            # Unknown native release stays negative. Only isolated owned labels
            # may be forcibly reaped; it cannot turn original release into PASS.
            for name, intent in list(receiver.owned_cli.items()):
                try:
                    ids = self.docker("ps", "-aq", "--no-trunc", "--filter", "name=^/" + name + "$").decode().split()
                    for cid in ids:
                        row = self.inspect(cid)
                        need(row["Config"]["Labels"].get("qs.m4.a.operation") == receiver.root.name
                             and row["Config"]["Labels"].get("qs.m4.a.purpose") == "fixed-native-tool", "owned_native_cleanup_scope")
                        self.docker("rm", "--force", cid)
                except Exception as error:
                    errors.append(type(error).__name__ + ":native_cleanup")
        for name, intent in reversed(list(self.created.items())):
            try:
                ids = self.docker("ps", "-aq", "--no-trunc", "--filter", "name=^/" + name + "$").decode().split()
                need(len(ids) <= 1, "owned_cleanup_name_ambiguous")
                for cid in ids:
                    row = self.inspect(cid)
                    need(row["Name"] == "/" + name and row["Config"]["Labels"].get(LABEL) == self.owner
                         and (intent["cid"] is None or intent["cid"] == cid), "owned_cleanup_identity")
                    save(self.output / ("cleanup-state-" + cid + ".json"), {"cid": cid, "state": row["State"], "memory": row["HostConfig"]["Memory"]})
                    self.docker("rm", "--force", "--volumes", cid)
                need(not self.docker("ps", "-aq", "--no-trunc", "--filter", "name=^/" + name + "$").strip(), "owned_CID_still_present")
            except Exception as error:
                errors.append(type(error).__name__ + ":owned_container_cleanup")
        if self.network_created:
            try:
                networks = decode(self.docker("network", "inspect", self.network))
                need(len(networks) == 1 and networks[0]["Labels"].get(LABEL) == self.owner
                     and not networks[0]["Containers"], "owned_network_cleanup_scope")
                self.docker("network", "rm", self.network)
                need(not self.docker("network", "ls", "--filter", "label=" + LABEL + "=" + self.owner, "--format", "{{.ID}}").strip(), "owned_network_still_present")
            except Exception as error:
                errors.append(type(error).__name__ + ":network_cleanup")
        if self.volume_created:
            try:
                volume = decode(self.docker("volume", "inspect", self.sqlvolume))
                need(len(volume) == 1 and volume[0]["Labels"].get(LABEL) == self.owner,
                     "owned_volume_cleanup_scope")
                self.docker("volume", "rm", self.sqlvolume)
                need(not self.docker("volume", "ls", "--filter", "label=" + LABEL + "=" + self.owner,
                                     "--format", "{{.Name}}").strip(), "owned_volume_still_present")
            except Exception as error:
                errors.append(type(error).__name__ + ":volume_cleanup")
        if receiver is not None and receiver.lock is not None:
            try:
                fcntl.flock(receiver.lock, fcntl.LOCK_UN)
                receiver.lock.close()
            except Exception as error:
                errors.append(type(error).__name__ + ":local_lock_release")
        if receiver is not None:
            try:
                # Failure files are diagnostic originals, never new proof. Keep
                # existing claims/journal/partial capture as well as successes.
                members = list(receiver.root.iterdir())
                need(len(members) <= 128, "operation_diagnostic_member_bound")
                size = 0
                for path in members:
                    status = path.lstat()
                    need(stat.S_ISREG(status.st_mode) and status.st_uid == 0 and status.st_nlink == 1,
                         "operation_diagnostic_private_regular_required")
                    size += status.st_size
                need(size <= 150 << 20, "operation_diagnostic_size_bound")
                archive = self.output / "original-operation-files.tar"
                with archive.open("xb") as output:
                    with tarfile.open(fileobj=output, mode="w") as stream:
                        for path in sorted(members):
                            stream.add(path, arcname=path.name, recursive=False)
                    output.flush()
                    os.fsync(output.fileno())
                self.result["original_operation_archive_sha256"] = digest(archive.read_bytes())
                self.result["failure_archive_grants_capture_success"] = False
            except Exception as error:
                errors.append(type(error).__name__ + ":original_operation_preservation")
        self.result["cleanup_errors"] = errors
        self.result["owned_cleanup_proven"] = not errors and not self.docker("ps", "-aq", "--no-trunc").strip()
        self.result["source_unchanged"] = False
        try:
            self.result["source_unchanged"] = all(digest((HERE / name).read_bytes()) == value for name, value in self.source_hashes.items()) and digest((self.build / "protection").read_bytes()) == TOOL_HASH
        except Exception:
            pass  # A failed input read never becomes a success certificate.


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--build", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--run", action="store_true")
    args = parser.parse_args()
    need(args.run, "default_is_source_only_explicit_run_required")
    os.umask(0o077)
    fixture = Fixture(args.build.resolve(), args.output.resolve())
    primary = None
    def interrupted(signum, _frame):
        raise RuntimeError("fixture_interrupted_" + str(signum))
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    signal.signal(signal.SIGALRM, interrupted)
    signal.alarm(RUNTIME_SECONDS)
    try:
        fixture.setup()
        fixture.phase_deadline = None
        fixture.exercise()
    except Exception as error:
        primary = type(error).__name__ + ":" + str(error)
        fixture.result["primary_failure"] = primary
    finally:
        signal.alarm(CLEANUP_SECONDS)
        try:
            save(fixture.output / "all-native-carrier-observations.json", fixture.observed)
        except Exception as error:
            fixture.result["evidence_failure"] = type(error).__name__
        try:
            fixture.cleanup()
        except Exception as error:
            fixture.result["cleanup_failure"] = type(error).__name__
        try:
            fixture.result["qualified"] = qualify(fixture.result) if primary is None else False
        except Exception as error:
            fixture.result["qualification_failure"] = str(error)
        fixture.result["elapsed_seconds"] = time.monotonic() - fixture.began
        save(fixture.output / "result.json", fixture.result)
        signal.alarm(0)
    summary = {key: fixture.result.get(key) for key in (
        "kind", "qualified", "production", "Mac_custody_executed", "business_runtime_started",
        "tool_sha256", "tool_exact_build", "mysql_image_binding", "material_counts", "material_bytes", "material_sha256",
        "capture_evidence_sha256", "actions", "carriers", "partial_dirty101_proven", "restored10039_witness",
        "target10139_witness", "original_three_files_verified", "all_native_carriers_released",
        "owned_cleanup_proven", "source_unchanged", "primary_failure", "cleanup_failure", "cleanup_errors",
        "qualification_failure", "original_operation_archive_sha256", "elapsed_seconds")}
    summary["observed_native_terminal_states"] = [{"cid": row["Id"], "running": row["State"]["Running"],
        "exit_code": row["State"]["ExitCode"], "oom": row["State"]["OOMKilled"]} for row in fixture.observed][-16:]
    summary["result_sha256"] = digest((fixture.output / "result.json").read_bytes())
    print("A_FORMAL_CARRIER_RESULT " + json.dumps(summary, separators=(",", ":")))
    return 0 if fixture.result["qualified"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
