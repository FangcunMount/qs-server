"""Offline host grammar/privacy/framing tests; never database qualification."""
import base64
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import unittest

PATH = Path(__file__).with_name("qs-ai-retirement-readonly-host.py")
SPEC = importlib.util.spec_from_file_location("actual_external_host_under_test", PATH)
m = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(m)


class ActualHostContract(unittest.TestCase):
    def packet(self):
        bounds = m.canonical({"not": "a bound or qualification"})
        return {"protocol": "qs-ai-readonly-host-input/v2", "source_sha": "a" * 40,
            "operation_id": "123-1", "run_id": "123", "runtime_source_sha": "b" * 40,
            "image_id": "sha256:" + "c" * 64, "container_id": "d" * 64,
            "runtime_binding_sha256": "f" * 64, "runtime_messaging": None,
            "ai_bounds": base64.b64encode(bounds).decode(), "ai_bounds_sha256": m.digest(bounds),
            "peer_bounds": base64.b64encode(bounds).decode(), "peer_bounds_sha256": m.digest(bounds),
            "original_sections": {name: {"rows": 0, "source_bytes": 0, "source_sha256": "e" * 64}
                for name in ("ai_bridge_commands", "ai_messaging_legacy_commands")},
            "peer_connection": {"host": "fixture.invalid", "port": 3306, "database": "synthetic_peer",
                "username": "synthetic", "password": "do-not-publish"},
            "protection": {"decrypt_keys": {}, "trusted_signers": {}},
            "modules": {name: "" for name in m.MODULES}}

    def test_bound_packets_are_not_completion_proofs(self):
        packet = self.packet()
        self.assertEqual(m.input_packet(m.canonical(packet)), packet)
        for field in ("complete", "production_proof", "fence_verified", "drop_ready", "executable", "ai_database_url"):
            with self.subTest(field=field), self.assertRaises(m.Rejected):
                m.input_packet(m.canonical({**packet, field: True}))

    def test_missing_source_identity_and_unknown_modules_rejected(self):
        packet = self.packet()
        for field in ("runtime_source_sha", "image_id", "ai_bounds_sha256", "peer_bounds_sha256"):
            broken = {**packet, field: ""}
            with self.subTest(field=field), self.assertRaises(m.Rejected):
                m.input_packet(m.canonical(broken))
        packet["modules"]["arbitrary.py"] = ""
        with self.assertRaises(m.Rejected):
            m.input_packet(m.canonical(packet))

    def test_duplicate_noncanonical_and_body_budgets_fail_closed(self):
        with self.assertRaises(m.Rejected):
            m.decode(b'{"closed":0,"closed":1}')
        with self.assertRaises(m.Rejected):
            m.decode(b'{"value":NaN}')
        with self.assertRaises(m.Rejected):
            m.input_packet(b" " * (m.INPUT_LIMIT + 1))
        packet = self.packet()
        packet["original_sections"]["ai_bridge_commands"]["rows"] = True
        with self.assertRaises(m.Rejected):
            m.input_packet(m.canonical(packet))

    def test_sql_null_empty_unicode_and_numeric_identity_framing(self):
        self.assertNotEqual(m.framed_digest([None]), m.framed_digest([b""]))
        self.assertNotEqual(m.framed_digest([b"9"]), m.framed_digest([b"10"]))
        self.assertEqual(m.framed_digest(["测评<&>".encode(), None, b""]),
            "60b205a0587d1d5a20caa51f92d5164c98267864210b0a4442397f17013d7e6c")

    def test_discarded_sensitive_text_has_no_retained_storage(self):
        sink = m.DiscardText()
        self.assertEqual(sink.write("do-not-publish"), 14)
        self.assertEqual(sink.buffer.write(b"do-not-publish"), 14)
        self.assertEqual(set(vars(sink)), {"buffer"})

    def test_actual_cli_bad_input_has_fixed_output_and_zero_stderr(self):
        result = subprocess.run([sys.executable, "-I", "-B", str(PATH)],
            input=b'{"password":"do-not-publish","complete":true}',
            capture_output=True, timeout=3, check=False)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stderr, b"")
        self.assertEqual(json.loads(result.stdout),
            {"protocol": "qs-ai-actual-execution-failed/v1", "category": "execution_rejected"})
        self.assertNotIn(b"do-not-publish", result.stdout)



class DiscoveryHostContract(unittest.TestCase):
    def packet(self):
        original = ActualHostContract().packet()
        keep = m.DISCOVERY_FIELDS - {"expected_ai", "expected_peer"}
        value = {key: original[key] for key in keep}
        value["protocol"] = "qs-ai-readonly-bounds-discovery-input/v1"
        value["expected_ai"] = {"identity_hash": "", "head": ""}
        value["expected_peer"] = {"identity_hash": "1" * 64, "head": "99"}
        return value

    def test_explicit_unknown_prior_ai_is_diagnostic_not_approved_bounds(self):
        packet = self.packet()
        self.assertEqual(m.discovery_input_packet(m.canonical(packet)), packet)
        for key in ("complete", "approved", "ai_bounds_sha256", "ai_database_url", "tables", "protection", "retired", "drop_ready"):
            with self.subTest(key=key), self.assertRaises(m.Rejected):
                m.discovery_input_packet(m.canonical({**packet, key: True}))

    def test_prior_binding_is_a_pair_and_peer_binding_cannot_be_unknown(self):
        for key, values in (("expected_ai", {"identity_hash": "a" * 64, "head": ""}),
                            ("expected_ai", {"identity_hash": "", "head": "0040_module_table_names"}),
                            ("expected_ai", {"identity_hash": "a" * 64, "head": "0041_unknown"}),
                            ("expected_peer", {"identity_hash": "", "head": ""}),
                            ("expected_peer", {"identity_hash": "a" * 64, "head": "099"})):
            packet = self.packet()
            packet[key] = values
            with self.subTest(key=key, values=values), self.assertRaises(m.Rejected):
                m.discovery_input_packet(m.canonical(packet))

    def test_discovery_does_not_accept_caller_target_list_or_relabel_v2(self):
        packet = self.packet()
        packet["original_sections"]["ai_bridge_commands"]["rows"] = 10001
        with self.assertRaises(m.Rejected):
            m.discovery_input_packet(m.canonical(packet))
        packet = self.packet()
        packet["protocol"] = "qs-ai-readonly-host-input/v2"
        with self.assertRaises(m.Rejected):
            m.input_packet(m.canonical(packet))

    def test_invalid_discovery_cli_has_fixed_output_and_no_private_input(self):
        packet = self.packet()
        del packet["expected_peer"]
        result = subprocess.run([sys.executable, "-I", "-B", str(PATH)],
            input=m.canonical(packet), capture_output=True, timeout=3, check=False)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stderr, b"")
        self.assertNotIn(b"do-not-publish", result.stdout)
        self.assertEqual(json.loads(result.stdout),
            {"protocol": "qs-ai-actual-execution-failed/v1", "category": "execution_rejected"})


class DiscoveryFreshProtocol(unittest.IsolatedAsyncioTestCase):
    def fixture(self, *, source_changes=False, schema_changes=False, catalog_changes=False):
        import types
        schema = {"columns": [["id", "bigint unsigned", "NO", None, "", None]],
                  "kinds": {"id": "bigint unsigned"}, "ddl_sha256": "a" * 64,
                  "columns_sha256": "b" * 64}
        tables = {name: {**schema, "upper": [base64.b64encode(b"10").decode()]}
                  for name in ("ai_bridge_commands", "ai_messaging_legacy_commands")}
        tables["ai_messaging_legacy_commands"]["upper"] = None
        expected = {name: {"rows": 0, "source_bytes": 0, "source_sha256": "e" * 64} for name in tables}
        query_calls, state = [], {"schema_calls": 0, "passes": 0, "binds": 0}
        class Reader:
            def __init__(self, session):
                self.tx = session.tx
                self.specs = {name: ("id", "id") for name in tables}
            async def query(self, sql, params=None):
                query_calls.append((sql, params))
                if sql == "SELECT 1 FROM `ai_bridge_commands` WHERE (`id`) > (:u0) LIMIT 1":
                    return [(1,)]
                if sql == "SELECT 1 FROM `ai_messaging_legacy_commands` LIMIT 1":
                    return []
                raise AssertionError("unexpected fixed read")
        async def binding(reader, identity, source):
            state["binds"] += 1
        async def read_schema(reader, table):
            state["schema_calls"] += 1
            return {**schema, "ddl_sha256": "c" * 64} if schema_changes else dict(schema)
        async def read_pass(reader, table, old, *, retain):
            self.assertFalse(retain)
            state["passes"] += 1
            result = {**expected[table], "pages": 1, "upper_token_sha256": "f" * 64}
            if source_changes:
                result["source_sha256"] = "0" * 64
            return result, []
        scanner = types.SimpleNamespace(_Borrowed=Reader, _binding=binding,
            _schema=read_schema, _pass=read_pass,
            _upper_values=lambda old, keys: tuple(int(base64.b64decode(v)) for v in old["upper"]),
            _pk_sql=lambda table, specs: "`id`")
        async def catalog(reader, side):
            return "0" * 64 if catalog_changes else "c" * 64
        module = types.SimpleNamespace(_scanner=lambda side, head: scanner, _catalog=catalog)
        bound = types.SimpleNamespace(side="peer", head="99", identity_hash="d" * 64,
            source_sha="a" * 40, catalog_sha256="c" * 64, tables=tables)
        old_tx = types.SimpleNamespace(is_active=False)
        session = types.SimpleNamespace(tx=types.SimpleNamespace(is_active=True))
        return module, bound, old_tx, session, expected, query_calls, state

    async def test_fresh_typed_after_upper_present_empty_and_full_source_eof(self):
        module, bound, old_tx, session, expected, calls, state = self.fixture()
        after_upper, actual = await m.discovery_fresh_checks(session, module, bound, old_tx, expected)
        self.assertEqual(after_upper, {"ai_bridge_commands": True, "ai_messaging_legacy_commands": False})
        self.assertEqual(actual, expected)
        self.assertEqual(state["passes"], 4)
        self.assertEqual(state["binds"], 2)
        self.assertEqual(calls[0][1], {"u0": 10})
        self.assertEqual(calls[1][1], {})

    async def test_actual_original_transaction_must_end_and_new_tx_must_differ(self):
        module, bound, old_tx, session, expected, calls, state = self.fixture()
        old_tx.is_active = True
        with self.assertRaises(m.Rejected):
            await m.discovery_fresh_checks(session, module, bound, old_tx, expected)
        self.assertEqual(calls, [])
        old_tx.is_active = False
        session.tx = old_tx
        with self.assertRaises(m.Rejected):
            await m.discovery_fresh_checks(session, module, bound, old_tx, expected)
        self.assertEqual(calls, [])

    async def test_source_catalog_and_schema_drift_refuse_without_qualification(self):
        for option in ("source_changes", "schema_changes", "catalog_changes"):
            with self.subTest(option=option):
                module, bound, old_tx, session, expected, _, _ = self.fixture(**{option: True})
                with self.assertRaises(m.Rejected):
                    await m.discovery_fresh_checks(session, module, bound, old_tx, expected)

    async def test_rows_after_upper_are_next_cycle_hint_not_business_closure(self):
        module, bound, old_tx, session, expected, _, _ = self.fixture()
        after_upper, _ = await m.discovery_fresh_checks(session, module, bound, old_tx, expected)
        self.assertTrue(after_upper["ai_bridge_commands"])
        self.assertFalse(hasattr(module, "verify"))
        self.assertFalse(hasattr(module, "_SEAL"))


if __name__ == "__main__":
    unittest.main()
