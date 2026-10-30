"""Read-only release and rollback inventory. Never export container environment values."""
import datetime
import hashlib
import json
from pathlib import Path
import re
import socket
import subprocess

NAMES = re.compile(r"(?:qs-(?:apiserver|collection.*|worker.*|ai|operating-system)|iam-apiserver|qs-ops|ops|prometheus|grafana|nginx)(?:-\d+)?$")


def command(*args):
    return subprocess.check_output(args, text=True, timeout=30, stderr=subprocess.DEVNULL)


def collect():
    docker = ("docker",)
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
            if source.is_file() and source.stat().st_size <= 4 * 1024 * 1024:
                item["file_sha256"] = hashlib.sha256(source.read_bytes()).hexdigest()
            if source.is_dir() and "config" in mount["Destination"].lower():
                config_files = sorted(f for f in source.rglob('*') if f.is_file() and f.suffix in {'.yaml', '.yml', '.json', '.toml', '.env'})
                if len(config_files) > 256 or any(f.stat().st_size > 4 * 1024 * 1024 for f in config_files):
                    raise ValueError("Configuration fingerprint exceeds bounds")
                fingerprints = {str(f.relative_to(source)): hashlib.sha256(f.read_bytes()).hexdigest() for f in config_files}
                item["config_files_sha256"] = fingerprints
            mounts.append(item)
        labels = image.get("Config", {}).get("Labels") or {}
        containers.append({"name": name, "container_id": cid, "image_ref": data["Config"]["Image"],
                           "image_id": data["Image"], "repo_digests": image.get("RepoDigests", []), "repo_tags": image.get("RepoTags", []),
                           "source_sha": labels.get("org.opencontainers.image.revision"),
                           "platform": image["Os"] + "/" + image["Architecture"],
                           "running": data["State"]["Running"],
                           "health": data["State"].get("Health", {}).get("Status"),
                           "mounts": mounts})
    if command(*docker, "ps", "--no-trunc", "--format", "{{.ID}} {{.Names}}") != ids:
        raise ValueError("Runtime changed during snapshot")
    if not containers:
        raise ValueError("No release containers found")
    result = {"host": socket.gethostname(), "observed_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
              "read_only": True, "business_acceptance_verified": False, "containers": containers}
    state = Path('/opt/qs-ai/state.json')
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
    return result


if __name__ == "__main__":
    try:
        print(json.dumps(collect(), sort_keys=True, indent=2))
    except Exception:
        raise SystemExit("Read-only release snapshot failed; no deployment performed") from None
