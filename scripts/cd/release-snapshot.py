"""Read-only release and rollback inventory. Never export container environment values."""
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import socket
import subprocess
import sys

NAMES = re.compile(r"(?:qs-(?:apiserver|collection.*|worker.*|ai|operating-system)|iam-apiserver|qs-ops|ops|prometheus|grafana|nginx)(?:-\d+)?$")


def command(*args):
    return subprocess.check_output(args, text=True, timeout=30, stderr=subprocess.DEVNULL)


def collect(sudo_docker=False):
    docker = ("sudo", "-n", "docker") if sudo_docker else ("docker",)
    command(*docker, "version", "--format", "{{.Server.Version}}")
    ids = command(*docker, "ps", "--no-trunc", "--format", "{{.ID}} {{.Names}}")
    containers = []
    for row in ids.splitlines():
        cid, name = row.split(maxsplit=1)
        if not NAMES.fullmatch(name):
            continue
        # Fixed projections exclude credentials, Env, logs and application data.
        data = json.loads(command(*docker, "inspect", "--format", '{"Image":{{json .Image}},"Config":{"Image":{{json .Config.Image}}},"State":{"Running":{{json .State.Running}}},"Mounts":{{json .Mounts}}}', cid))
        image = json.loads(command(*docker, "image", "inspect", "--format", '{"Os":{{json .Os}},"Architecture":{{json .Architecture}},"RepoDigests":{{json .RepoDigests}},"RepoTags":{{json .RepoTags}},"Config":{"Labels":{{json .Config.Labels}}}}', data["Image"]))
        mounts = []
        for mount in data.get("Mounts", []):
            item = {k.lower(): mount.get(k) for k in ("Type", "Source", "Destination", "RW")}
            source = Path(mount["Source"])
            item["fingerprint_available"] = False
            try:
                if source.is_file() and source.stat().st_size <= 4 * 1024 * 1024:
                    item["file_sha256"] = hashlib.sha256(source.read_bytes()).hexdigest()
                    item["fingerprint_available"] = True
                if source.is_dir() and "config" in mount["Destination"].lower():
                    def unreadable(error):
                        raise error
                    config_files = sorted(Path(directory) / name for directory, _, names in os.walk(source, onerror=unreadable)
                                          for name in names if Path(name).suffix in {'.yaml', '.yml', '.json', '.toml', '.env'})
                    if len(config_files) > 256 or any(f.stat().st_size > 4 * 1024 * 1024 for f in config_files):
                        raise ValueError("Configuration fingerprint exceeds bounds")
                    fingerprints = {str(f.relative_to(source)): hashlib.sha256(f.read_bytes()).hexdigest() for f in config_files}
                    item["config_files_sha256"] = fingerprints
                    item["fingerprint_available"] = True
            except PermissionError:
                item["fingerprint_available"] = False
            mounts.append(item)
        labels = image.get("Config", {}).get("Labels") or {}
        containers.append({"name": name, "container_id": cid, "image_ref": data["Config"]["Image"],
                           "image_id": data["Image"], "repo_digests": image.get("RepoDigests", []), "repo_tags": image.get("RepoTags", []),
                           "source_sha": labels.get("org.opencontainers.image.revision"),
                           "platform": image["Os"] + "/" + image["Architecture"],
                           "running": data["State"]["Running"],
                           "health": data["State"].get("Health", {}).get("Status"),
                           "mounts": mounts})
        # The same fixed early-exit version and readiness commands used by runtime-evidence.py.
        role = "worker" if name.startswith("qs-worker") else "collection-server" if name.startswith("qs-collection") else "apiserver" if name == "qs-apiserver" else None
        if role:
            binary = "/app/" + ("collection-server" if role == "collection-server" else "qs-" + role)
            container = containers[-1]
            container.update(reported_git_commit=None, binary_matches_process=False, ready=False)
            try:
                raw = command(*docker, "exec", cid, binary, "--version=true", "--config=/app/configs/" + role + ".prod.yaml")
                commits = re.findall(r"^\s*gitCommit:\s*([0-9a-f]{40})\s*$", raw, re.MULTILINE)
                hashes = command(*docker, "exec", cid, "sha256sum", "/proc/1/exe", binary).splitlines()
                container["binary_matches_process"] = len(hashes) == 2 and all(re.fullmatch(r"[0-9a-f]{64}\s+\S+", x) for x in hashes) and hashes[0].split()[0] == hashes[1].split()[0]
                if len(commits) == 1 and container["binary_matches_process"]:
                    container["reported_git_commit"] = commits[0]
            except subprocess.SubprocessError:
                pass
            port, ready_path = (9092, "/readyz") if role == "worker" else (8080, "/serve-readyz") if role == "collection-server" else (8080, "/readyz")
            try:
                command(*docker, "exec", cid, "wget", "-q", "-O", "/dev/null", "-T", "5", "http://127.0.0.1:" + str(port) + ready_path)
                container["ready"] = True
            except subprocess.SubprocessError:
                pass
    if command(*docker, "ps", "--no-trunc", "--format", "{{.ID}} {{.Names}}") != ids:
        raise ValueError("Runtime changed during snapshot")
    if not containers:
        raise ValueError("No release containers found")
    result = {"host": socket.gethostname(), "observed_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
              "read_only": True, "business_acceptance_verified": False, "containers": containers}
    state = Path('/opt/qs-ai/state.json')
    result["ai_release_state_available"] = False
    try:
        if state.is_file():
            values = json.loads(state.read_text())
            result["ai_release_state"] = {k: values.get(k) for k in ("current", "previous")}
            result["ai_release_manifests"] = {}
            for key in ("current", "previous"):
                release = values.get(key)
                if release and re.fullmatch(r"[0-9a-f]{40}-[0-9]+-[0-9]+", release):
                    manifest = Path('/opt/qs-ai/releases') / release / 'manifest.json'
                    if manifest.is_file():
                        values2 = json.loads(manifest.read_text())
                        result["ai_release_manifests"][key] = {k: values2.get(k) for k in
                            ("revision", "image_id", "image_ref", "schema_policy")}
            result["ai_release_state_available"] = True
    except PermissionError:
        pass
    return result


if __name__ == "__main__":
    try:
        if sys.argv[1:] not in ([], ["--sudo-docker"]):
            raise ValueError("Unsupported snapshot mode")
        print(json.dumps(collect(sudo_docker=bool(sys.argv[1:])), sort_keys=True, indent=2))
    except Exception:
        raise SystemExit("Read-only release snapshot failed; no deployment performed") from None
