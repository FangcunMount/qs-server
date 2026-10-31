"""Exercise the real inventory transport without SSH or a Docker daemon."""
import hashlib
import contextlib
import io
import json
import os
from pathlib import Path
import runpy
import tempfile
import unittest
import types
import sys
from unittest.mock import patch
from subprocess import CompletedProcess

SCRIPT = Path(__file__).with_name("run-messaging-retirement-inventory.py")


class InventoryTransportTest(unittest.TestCase):
    def execute(self, *, owner="qs.mysql", available=True, authorized=False, corrupt=False, process_failed=False):
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
                          transaction_released=True, pool_released=True,
                          inventory_complete=available, source_sha="a" * 40, rows=None if not available else 0)
            with patch.dict(os.environ, env), patch("subprocess.run", return_value=CompletedProcess([], 0 if available and not process_failed else 1, json.dumps(report).encode())) as transport:
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
            self.assertIn("--user " + ("10001:10001" if owner == "ai.mysql" else "2000:2000"), command[-1])
            self.assertIn(container, command[-1])
            self.assertNotIn("PASSWORD", command[-1])
            if owner == "ai.mysql":
                self.assertIn("/app/.venv/bin/python -", command[-1])

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
        self.assertFalse(saved["inventory_complete"])
        self.assertFalse(saved["drop_authorized"])
        self.assertIsNone(saved["mutation"])
        self.assertFalse(saved["receipt_validated"])

    def test_process_failure_cannot_preserve_a_complete_claim(self):
        error, saved, _ = self.execute(process_failed=True)
        self.assertIn("incomplete", error)
        self.assertFalse(saved["inventory_complete"])
        self.assertFalse(saved["process_completed_successfully"])

    def test_actual_ai_payload_preserves_classified_errors_without_private_values(self):
        _, _, calls = self.execute(owner="ai.mysql")
        payload = calls[0].kwargs["input"]
        for failure in ["binding", "query", "close", "none"]:
            with self.subTest(failure=failure):
                closed = []
                class URL:
                    def get_secret_value(self):
                        return "mysql+asyncmy://private-user:private-password@host/db"
                class Settings:
                    database_url = None if failure == "binding" else URL()
                    database_tls_mode = "VERIFY_IDENTITY"
                    database_tls_ca_file = "/original/ca.pem"
                class Database:
                    def __init__(self, url, **kwargs):
                        self.url, self.kwargs = url, kwargs
                        assert kwargs["tls_mode"] == "VERIFY_IDENTITY"
                        assert kwargs["tls_ca_file"] == "/original/ca.pem"
                    async def close(self):
                        closed.append(True)
                        if failure == "close":
                            raise RuntimeError("private close details")
                async def inventory(db, **kwargs):
                    if failure == "query":
                        error = RuntimeError("private query parameters")
                        error.orig = Exception(1045, "private password")
                        raise error
                    return {"complete": True, "sections": {}}
                modules = {}
                for name in ["qs_ai", "qs_ai.config", "qs_ai.infrastructure", "qs_ai.infrastructure.persistence",
                             "qs_ai.infrastructure.persistence.mysql", "qs_ai.infrastructure.persistence.mysql.database",
                             "qs_ai.maintenance", "qs_ai.maintenance.messaging_audit"]:
                    modules[name] = types.ModuleType(name)
                    modules[name].__path__ = []
                modules["qs_ai.config"].Settings = Settings
                module = modules["qs_ai.infrastructure.persistence.mysql.database"]
                module.Database, module.Transactions = Database, lambda db: db
                modules["qs_ai.maintenance.messaging_audit"].inventory = inventory
                output, errors = io.StringIO(), io.StringIO()
                with patch.dict(sys.modules, modules), contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
                    try:
                        exec(compile(payload, "actual-ai-inventory-payload", "exec"), {})
                    except SystemExit as error:
                        self.assertEqual(error.code, 1)
                value = json.loads(output.getvalue())
                self.assertNotIn("private", output.getvalue() + errors.getvalue())
                self.assertFalse(value["drop_authorized"])
                if failure == "none":
                    self.assertTrue(value["inventory_complete"])
                    self.assertEqual(value["phase"], "complete")
                else:
                    self.assertFalse(value["inventory_complete"])
                    self.assertEqual(value["error_phase"], {"binding":"application_binding", "query":"inventory_query", "close":"pool_release"}[failure])
                if failure == "query":
                    self.assertEqual(value["driver_code"], 1045)
                    self.assertTrue(value["pool_released"])
                self.assertEqual(len(closed), 0 if failure == "binding" else 1)


if __name__ == "__main__":
    unittest.main()
