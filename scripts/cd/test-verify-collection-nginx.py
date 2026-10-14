#!/usr/bin/env python3
"""Exercise the actual verifier with local commands and bounded fake dependencies."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("verify-collection-nginx.sh")
VALID = """upstream collect-api {
    zone collect_api 64k;
    resolver 127.0.0.11 valid=10s ipv6=off;
    resolver_timeout 5s;
    server qs-collection-server:8080 resolve;
}
server {
    listen 443 ssl;
    server_name collect.fangcunmount.cn;
    proxy_next_upstream error timeout http_502 http_504;
}
"""
# Unrelated blocks must not be included in the first matching block's policy.
LARGE_TAIL = """upstream unrelated {
    ip_hash;
}
server {
    listen 443 ssl;
    server_name unrelated.example;
    proxy_next_upstream http_503;
}
""" + "# synthetic remainder of nginx -T\n" * 40000

FAKE_COMMAND = r'''#!/usr/bin/env python3
import json, os, shutil, sys
from pathlib import Path
root = Path(os.environ["FAKE_ROOT"]).resolve()
args = sys.argv[1:]
def record(action):
    with (root / "actions.jsonl").open("a") as stream:
        stream.write(json.dumps(action) + "\n")
def path(value):
    result = Path(value)
    if root not in result.resolve().parents and result.resolve() != root:
        raise SystemExit("fake mutation outside test directory")
    return result
if Path(sys.argv[0]).name == "curl":
    count_file = root / "requests"
    count = int(count_file.read_text()) if count_file.exists() else 0
    count_file.write_text(str(count + 1))
elif args[0] == "docker":
    if args[1] == "ps":
        print("cid1\ncid2")
    elif args[1] == "inspect":
        if args[2] == "nginx":
            print("true")
        elif "IPAddress" in args[-1]:
            print("172.20.0." + ("10" if args[2] == "cid1" else "11"))
        else:
            print("/qs-collection-server-" + args[2][-1])
    elif args[1] == "exec" and args[2] == "nginx":
        if args[3:] == ["nginx", "-v"]:
            print("nginx version: nginx/1.27.5", file=sys.stderr)
        elif args[3:] == ["nginx", "-T"]:
            if os.environ.get("FAKE_DUMP_FAIL") == "1":
                raise SystemExit(42)
            sys.stdout.write(Path(os.environ["FAKE_EFFECTIVE"]).read_text())
        elif args[3:] == ["nginx", "-s", "reload"]:
            record(["reload"])
        elif args[3:] == ["nginx", "-t"]:
            pass
        elif args[3:5] == ["getent", "ahostsv4"]:
            print("172.20.0.10 STREAM upstream\n172.20.0.11 STREAM upstream")
        else:
            raise SystemExit(65)
    elif args[1] == "exec" and args[2] in ["cid1", "cid2"]:
        count_file = root / "requests"
        count = int(count_file.read_text()) if count_file.exists() else 0
        count = (count + (1 if args[2] == "cid1" else 0)) // 2
        print('gin_requests_total{code="200",method="GET",url="/health"} ' + str(count))
    else:
        raise SystemExit(66)
elif args[0] == "rsync":
    source, dest = path(args[-2]), path(args[-1])
    record(["copy", str(source), str(dest)])
    shutil.copyfile(source, dest)
elif args[0] == "chmod":
    path(args[2]).chmod(int(args[1], 8))
elif args[0] == "mkdir":
    for value in args[2:]:
        path(value).mkdir(parents=True, exist_ok=True)
elif args[0] == "rm":
    path(args[-1]).unlink(missing_ok=True)
else:
    raise SystemExit(67)
'''


class CollectionNginxVerificationTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="collection-nginx-test-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        for name in ["fake-privileged", "curl"]:
            target = self.bin / name
            target.write_text(FAKE_COMMAND)
            target.chmod(0o755)
        self.config = self.root / "collect.conf"
        self.candidate = self.root / "candidate.conf"
        self.candidate.write_text(VALID)
        self.config.write_text(VALID + LARGE_TAIL)

    def run_verifier(self, mode="verify-only", **overrides):
        env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ["PATH"],
                   FAKE_ROOT=str(self.root), FAKE_EFFECTIVE=str(self.config),
                   PRIVILEGE_RUNNER=str(self.bin / "fake-privileged"),
                   NGINX_CONFIG_DEST=str(self.config),
                   NGINX_CONFIG_LEGACY_DEST=str(self.root / "legacy.conf"),
                   NGINX_CONFIG_BACKUP_DIR=str(self.root / "backups"),
                   NGINX_CONFIG_SOURCE=str(self.candidate),
                   EXPECTED_COLLECTION_REPLICAS="2")
        env.update(overrides)
        return subprocess.run(["bash", str(SCRIPT), mode], env=env,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                              text=True, timeout=20)

    def test_large_dump_preserves_first_blocks_and_routes_to_both_replicas(self):
        result = self.run_verifier()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Nginx request distribution passed (40 observed requests across all replicas)", result.stdout)

    def test_large_dump_rejects_invalid_upstream(self):
        self.config.write_text(VALID.replace("valid=10s", "valid=5s") + LARGE_TAIL)
        result = self.run_verifier()
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("Effective collect-api upstream is missing", result.stderr)

    def test_large_dump_rejects_invalid_protective_policy(self):
        self.config.write_text(VALID.replace("http_504;", "http_504 http_503;") + LARGE_TAIL)
        result = self.run_verifier()
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("does not preserve protective 503 responses", result.stderr)

    def test_missing_upstream_remains_a_failure(self):
        self.config.write_text(LARGE_TAIL)
        result = self.run_verifier()
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("0 collect-api upstream blocks, want 1", result.stderr)

    def assert_single_restore(self, previous):
        old = b"# exact old configuration\n"
        if previous != self.config:
            self.config.unlink()
        previous.write_bytes(old)
        result = self.run_verifier("install-and-verify", FAKE_DUMP_FAIL="1")
        self.assertEqual(result.returncode, 42, result.stderr)
        actions = [json.loads(line) for line in (self.root / "actions.jsonl").read_text().splitlines()]
        restores = [item for item in actions if item[0] == "copy" and item[2] == str(previous)
                    and Path(item[1]).parent == self.root / "backups"]
        self.assertEqual(len(restores), 1, result.stderr)
        self.assertEqual(result.stderr.count("Collection Nginx verification failed; restoring previous config"), 1)
        self.assertEqual(previous.read_bytes(), old)
        if previous != self.config:
            self.assertFalse(self.config.exists())
        backup = list((self.root / "backups").glob("collect-nginx-*.conf"))
        self.assertEqual(len(backup), 1)
        self.assertEqual(backup[0].read_bytes(), old)
        self.assertIn(hashlib.sha256(old).hexdigest(), result.stdout)

    def test_subshell_dump_failure_restores_old_config_once(self):
        self.assert_single_restore(self.config)

    def test_subshell_dump_failure_restores_old_legacy_path_once(self):
        self.assert_single_restore(self.root / "legacy.conf")


if __name__ == "__main__":
    unittest.main()
