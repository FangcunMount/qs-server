"""Exercise the Makefile preflight and the explicit legacy checks."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
DATA_CLIENTS = ("mysql", "redis-cli", "mongosh")


class InfrastructureCheck(unittest.TestCase):
    def run_check(self, args, failing_client, redis_reply="PONG"):
        with tempfile.TemporaryDirectory() as raw:
            directory = Path(raw)
            calls = directory / "calls"
            for client in (*DATA_CLIENTS, "curl"):
                stub = directory / client
                stub.write_text(
                    "#!/bin/sh\n"
                    'client=${0##*/}\n'
                    'printf "%s\\n" "$client" >> "$CHECK_CALLS"\n'
                    '[ "$client" != "$CHECK_FAILING_CLIENT" ] || exit 1\n'
                    'if [ "$client" = redis-cli ]; then printf "%s\\n" "$CHECK_REDIS_REPLY"; fi\n'
                )
                stub.chmod(0o700)
            result = subprocess.run(
                args,
                cwd=ROOT,
                env={
                    **os.environ,
                    "PATH": str(directory) + os.pathsep + os.environ["PATH"],
                    "CHECK_CALLS": str(calls),
                    "CHECK_FAILING_CLIENT": failing_client,
                    "CHECK_REDIS_REPLY": redis_reply,
                },
                capture_output=True,
                text=True,
                timeout=10,
            )
            return result, calls.read_text().splitlines()

    def test_runtime_preflight_omits_legacy_transport(self):
        result, calls = self.run_check(["make", "check-infra"], "curl")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(calls, list(DATA_CLIENTS))

    def test_runtime_preflight_rejects_unreachable_data_services(self):
        for client in DATA_CLIENTS:
            with self.subTest(client=client):
                result, calls = self.run_check(["make", "check-infra"], client)
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(calls, list(DATA_CLIENTS))

    def test_explicit_legacy_checks_still_probe_nsq(self):
        for args, expected in (
            (["make", "check-nsq"], ["curl"]),
            (["bash", "scripts/check-infra.sh", "all"], [*DATA_CLIENTS, "curl"]),
        ):
            with self.subTest(args=args):
                result, calls = self.run_check(args, "curl")
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(calls, expected)

    def test_redis_server_error_with_zero_exit_does_not_pass(self):
        result, calls = self.run_check(["make", "check-infra"], "", "NOAUTH Authentication required.")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(calls, list(DATA_CLIENTS))


if __name__ == "__main__":
    unittest.main()
