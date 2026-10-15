#!/usr/bin/env python3
"""Build only the fixed A executables. No Docker, DB or production connection."""
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
import sys
import tarfile

TOOL_SOURCE = "602710eb2f6ee383923230890c4bdb4a3bcada3a"
A_SOURCE = "51163edab7038adcdb6b8f1712d6dc434f361dc6"
TOOL_HASH = "2c314163ee6931aa133f3537b418927c97cce9f8fd8f4f3b75f42be142e173fd"
HERE = Path(__file__).resolve().parent
REPO = HERE.parents[2]


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def prepare(root):
    root.mkdir(mode=0o700)  # never reuse another build
    inputs = {}
    env = dict(os.environ, GOOS="linux", GOARCH="amd64", CGO_ENABLED="0",
               GOMAXPROCS="2", GOMEMLIMIT="1GiB", GOTOOLCHAIN="local",
               GOWORK="off", GOPROXY="off", GOSUMDB="off", GOFLAGS="")
    version = subprocess.check_output(["go", "version"], text=True).strip()
    if not version.startswith("go version go1.25.12 "):
        raise RuntimeError("exact_Go1.25.12_required")
    for name, source in [("tool-source", TOOL_SOURCE), ("a-source", A_SOURCE)]:
        archive = root / (name + ".tar")
        with archive.open("xb") as output:
            subprocess.run(["git", "archive", source], cwd=REPO, stdout=output, check=True, timeout=60)
        destination = root / name
        destination.mkdir(mode=0o700)
        with tarfile.open(archive) as stream:
            for member in stream.getmembers():
                path = PurePosixPath(member.name)
                if path.is_absolute() or ".." in path.parts or not (member.isfile() or member.isdir()):
                    raise RuntimeError("fixed_git_archive_regular_members_required")
            stream.extractall(destination)
        for file in destination.rglob("*"):
            if file.is_file():
                inputs[str(file.relative_to(root))] = sha(file.read_bytes())
    source = root / "tool-source"
    binary = root / "protection"
    argv = ["go", "build", "-p=2", "-trimpath", "-ldflags",
            "-s -w -X github.com/FangcunMount/qs-server/pkg/version.GitCommit=" + TOOL_SOURCE,
            "-o", str(binary), "./scripts/oneoff/m4_sql_historical_protection"]
    subprocess.run(argv, cwd=source, env=env, check=True, timeout=600)
    if sha(binary.read_bytes()) != TOOL_HASH:
        raise RuntimeError("reproduced_Tool_bytes_differ_stop_no_substitute")
    binary.chmod(0o700)
    source = root / "a-source"
    target = source / "internal/pkg/migration/zz_private_formal_carrier_test.go"
    fixture = HERE / "fixture_test.go.txt"
    overlay = root / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str(target): str(fixture)}}))
    subprocess.run(["go", "test", "-p=2", "-c", "-tags=integration", "-trimpath",
                    "-overlay=" + str(overlay), "-ldflags",
                    "-X github.com/FangcunMount/qs-server/pkg/version.GitCommit=" + A_SOURCE,
                    "-o", str(root / "fixture.test"), "./internal/pkg/migration"],
                   cwd=source, env=env, check=True, timeout=600)
    (root / "fixture.test").chmod(0o700)
    for name, expected in inputs.items():
        if sha((root / name).read_bytes()) != expected:
            raise RuntimeError("immutable_archived_source_changed_during_build")
    receipt = {"kind": "fixed_A_formal_carrier_build", "tool_source": TOOL_SOURCE,
               "a_source": A_SOURCE, "tool_sha256": TOOL_HASH, "tool_exact_bytes": True,
               "fixture_sha256": sha((root / "fixture.test").read_bytes()),
               "fixture_source_sha256": sha(fixture.read_bytes()), "go_version": version,
               "source_inputs": inputs, "production": False, "runtime_executed": False}
    (root / "build.json").write_text(json.dumps(receipt, indent=2) + "\n")
    print(json.dumps({key: value for key, value in receipt.items() if key != "source_inputs"}))


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("one fresh build directory required")
    prepare(Path(sys.argv[1]).resolve())
