#!/usr/bin/env python3
"""Release image retention v1. Standard library; dry run unless --apply.

Identical vendored file in each deploy repository. Never prune containers/volumes.
"""

import argparse
import contextlib
import fcntl
import json
import os
import re
import socket
import subprocess
import time
from pathlib import Path

STATE = Path("/var/lib/fangcun-image-retention")
LOCK = STATE / "deploy.lock"
HOSTS = {
    "iam": "serverB",
    "qs-operating-system": "serverB",
    "qs-ai": "serverA",
    "qs-apiserver": "serverA",
    "qs-collection-server": "serverA",
    "qs-worker": "serverD",
}
ACR = "crpi-x3r4i68cpfmwtcll.cn-beijing.personal.cr.aliyuncs.com/fangcunmount/"


def docker(*args):
    return subprocess.check_output(["docker", *args], text=True, timeout=120).strip()


def inventory():
    ids = sorted(set(docker("image", "ls", "-aq", "--no-trunc").split()))
    return json.loads(docker("image", "inspect", *ids)) if ids else []


def containers():
    ids = sorted(docker("ps", "-aq").split())
    raw = json.loads(docker("inspect", *ids)) if ids else []
    return [
        (
            x["Id"],
            x["Image"],
            x["State"]["Status"],
            x["State"].get("Health", {}).get("Status"),
            x["State"]["StartedAt"],
            x["RestartCount"],
        )
        for x in raw
    ]


def repo(tag):
    return tag.rsplit(":", 1)[0]


def select(images, refs, service, image_id, image_ref, state, previous=()):
    allowed = {repo(image_ref), ACR + service, "ghcr.io/fangcunmount/" + service}
    if service == "qs-ai":
        allowed.add("qs-ai")
    scoped = [
        x for x in images if x.get("RepoTags") and {repo(t) for t in x["RepoTags"]} <= allowed
    ]
    if image_id not in {x["Id"] for x in scoped}:
        raise ValueError("Current image has unrecognized aliases; refusing cleanup")
    # Successful deployment order, not build time. Repeated deploys/rollbacks deduplicate.
    history = [image_id] + [x for x in state.get("successful", []) if x != image_id]
    history = history[:3]
    bootstrap = state.get("bootstrap")
    if bootstrap is None:
        bootstrap = [x["Id"] for x in sorted(scoped, key=lambda x: x["Created"], reverse=True)[:3]]
        bootstrap = list(
            dict.fromkeys(bootstrap + [x for x in previous if x in {i["Id"] for i in scoped}])
        )
    if len(history) >= 3:
        bootstrap = []
    keep = set(history) | set(bootstrap) | refs
    candidates = [x for x in scoped if x["Id"] not in keep]
    return {"version": 1, "successful": history, "bootstrap": bootstrap}, keep, candidates


def write_json(path, value):
    temporary = path.with_suffix(".tmp")
    temporary.write_text(json.dumps(value, indent=2) + "\n")
    temporary.chmod(0o600)
    temporary.replace(path)


@contextlib.contextmanager
def transaction(already_locked):
    # CI caller owns the same host lock across load, replacement, verification and cleanup.
    if already_locked:
        yield
    else:
        with LOCK.open("r+") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            yield


def execute(args):
    if socket.gethostname().split(".")[0] != HOSTS[args.service]:
        raise ValueError("Unexpected deployment host")
    if "/" in args.service or not args.image_ref:
        raise ValueError("Invalid service/image")
    with transaction(args.deployment_locked):
        images, before = inventory(), containers()
        image_id = json.loads(docker("image", "inspect", args.image_ref))[0]["Id"]
        current = [x for x in before if x[1] == image_id]
        if not current or any(x[2] != "running" or x[3] not in (None, "healthy") for x in current):
            raise ValueError("Successful image is not running and healthy")
        path = STATE / (args.service + ".json")
        state = json.loads(path.read_text()) if path.exists() else {}
        if state and state.get("version") != 1:
            raise ValueError("Unsupported retention state")
        for key in ("successful", "bootstrap"):
            if key in state and (
                not isinstance(state[key], list)
                or any(
                    not isinstance(x, str) or not re.fullmatch(r"sha256:[0-9a-f]{64}", x)
                    for x in state[key]
                )
            ):
                raise ValueError("Invalid retention state")
        new_state, keep, candidates = select(
            images,
            {x[1] for x in before},
            args.service,
            image_id,
            args.image_ref,
            state,
            args.protect_image_id,
        )
        audit = {
            "service": args.service,
            "keep": sorted(keep),
            "candidates": [{"id": x["Id"], "tags": x["RepoTags"]} for x in candidates],
            "removed_tags": [],
            "free_before": os.statvfs("/").f_bavail * os.statvfs("/").f_frsize,
        }
        if not args.apply:
            print(json.dumps(audit))
            return
        # Commit the successful release even if reclamation later fails.
        write_json(path, new_state)
        audit_path = STATE / (args.service + "-last-cleanup.json")
        try:
            for candidate in candidates:
                if containers() != before:
                    raise RuntimeError("Container state changed during cleanup")
                live = json.loads(docker("image", "inspect", candidate["Id"]))[0]
                if set(live.get("RepoTags") or []) != set(candidate["RepoTags"]):
                    raise RuntimeError("Image aliases changed during cleanup")
                for tag in sorted(candidate["RepoTags"]):
                    if json.loads(docker("image", "inspect", tag))[0]["Id"] != candidate["Id"]:
                        raise RuntimeError("Image tag changed during cleanup")
                    docker("image", "rm", "--no-prune", tag)
                    audit["removed_tags"].append(tag)
            if containers() != before:
                raise RuntimeError("Container state changed during cleanup")
            remaining = {x["Id"] for x in inventory()}
            if not (keep & {x["Id"] for x in images}) <= remaining:
                raise RuntimeError("Protected image missing")
            audit["status"] = "success"
        except Exception:
            audit["status"] = "failed"
            raise
        finally:
            audit["free_after"] = os.statvfs("/").f_bavail * os.statvfs("/").f_frsize
            audit["timestamp"] = int(time.time())
            write_json(audit_path, audit)
        print(json.dumps(audit))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--service", required=True, choices=HOSTS)
    parser.add_argument("--image-ref", required=True)
    parser.add_argument("--apply", action="store_true")
    parser.add_argument(
        "--protect-image-id",
        action="append",
        default=[],
        help="Previously running/rollback image to protect during bootstrap",
    )
    parser.add_argument(
        "--deployment-locked",
        action="store_true",
        help="Internal: parent deployment already holds the host lock",
    )
    args = parser.parse_args()
    try:
        execute(args)
    except Exception as error:
        print("Image retention failed: " + type(error).__name__)
        raise SystemExit(1) from None


if __name__ == "__main__":
    main()
