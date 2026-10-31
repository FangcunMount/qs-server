"""Exercise the real inventory transport without SSH or a Docker daemon."""
import hashlib
import json
import os
from pathlib import Path
import runpy
import tempfile
import unittest
from unittest.mock import patch
from subprocess import CompletedProcess

SCRIPT = Path(__file__).with_name("run-messaging-retirement-inventory.py")


class InventoryTransportTest(unittest.TestCase):
    def execute(self, *, owner="qs.mysql", available=True, authorized=False, corrupt=False):
        with tempfile.TemporaryDirectory() as temporary:
            previous = Path.cwd()
            self.addCleanup(os.chdir, previous)
            os.chdir(temporary)
            Path("inventory-companion").mkdir()
            binary = b"isolated-fixture"
            Path("inventory-companion/messaging-retirement-inventory").write_bytes(binary)
            env = dict(INVENTORY_OWNER=owner, EXPECTED_DIGEST="0" * 64 if corrupt else hashlib.sha256(binary).hexdigest(),
                       GITHUB_SHA="a" * 40, GITHUB_RUN_ID="12345", RUNNER_SSH_CONFIG="isolated-config", RUNNER_SSH_ALIAS="isolated-alias")
            report = dict(owner=owner, read_only=True, mutation=False, drop_authorized=authorized,
                          inventory_complete=available, source_sha="a" * 40, rows=None if not available else 0)
            with patch.dict(os.environ, env), patch("subprocess.run", return_value=CompletedProcess([], 0 if available else 1, json.dumps(report).encode())) as transport:
                error = None
                try:
                    runpy.run_path(str(SCRIPT), run_name="__main__")
                except SystemExit as exc:
                    error = str(exc)
                output = Path("messaging-retirement-inventory.json")
                saved = json.loads(output.read_text()) if output.exists() else None
                calls = transport.call_args_list
            os.chdir(previous)
        return error, saved, calls

    def test_fixed_owners_use_application_uid_without_database_arguments(self):
        for owner, container in [("qs.mysql", "qs-apiserver"), ("iam.mysql", "iam-apiserver"), ("ai.mysql", "qs-ai")]:
            error, saved, calls = self.execute(owner=owner)
            self.assertIsNone(error)
            self.assertFalse(saved["drop_authorized"])
            self.assertEqual(len(calls), 1)
            command = calls[0].args[0]
            self.assertEqual(command[:3], ["ssh", "-F", "isolated-config"])
            self.assertIn("--user 2000:2000", command[-1])
            self.assertIn(container, command[-1])
            self.assertNotIn("PASSWORD", command[-1])

    def test_partial_evidence_is_preserved_but_fails(self):
        error, saved, _ = self.execute(available=False)
        self.assertIn("incomplete", error)
        self.assertIsNone(saved["rows"])
        self.assertFalse(saved["inventory_complete"])

    def test_companion_drift_and_unknown_owner_refuse_before_ssh(self):
        for options in [dict(corrupt=True), dict(owner="unreviewed")]:
            error, saved, calls = self.execute(**options)
            self.assertIsNotNone(error)
            self.assertIsNone(saved)
            self.assertFalse(calls)

    def test_transport_cannot_return_deletion_approval(self):
        error, saved, _ = self.execute(authorized=True)
        self.assertIn("unavailable", error)
        self.assertIsNone(saved)


if __name__ == "__main__":
    unittest.main()
