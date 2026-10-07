import argparse
import contextlib
import gzip
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("cbpt", Path(__file__).with_name("cbpt-cleanup.py"))
cbpt = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(cbpt)
SECRET = "fixture_password_NEVER_PRINT"
SHA = "a" * 40
HASH = "b" * 64
SOURCE_UUID = "11111111-1111-1111-1111-111111111111"
RESTORE_UUID = "22222222-2222-2222-2222-222222222222"


def env():
    return {"MYSQL_HOST": "mysql.fixture", "MYSQL_PORT": "3306", "MYSQL_USERNAME": "fixture",
            "MYSQL_PASSWORD": SECRET, "MYSQL_DATABASE": "fixture_source"}


def receipt(operation, operation_id="123-1"):
    value = {"format_version": 1, "operation": operation, "stage": operation, "status": "ok",
             "error_category": "none", "error_code": 0, "archive_eligible": True, "drop_eligible": True,
             "source_target_hash": HASH, "source_sha": SHA, "operation_id": operation_id,
             "target_table_count": 22, "non_target_count": 66}
    if operation == "verify-removed":
        value.update(remaining_target_count=0, ledger_complete=True, dropped_count=22, unknown_count=0)
    return value


def manifest(operation_id, digest):
    return {"format_version": 1, "operation_id": operation_id, "source_sha": SHA,
            "source_server_uuid": SOURCE_UUID, "source_target_hash": HASH, "migration_version": 95,
            "migration_dirty": False, "archive_drop_eligible": True,
            "tables": [{"name": name, "rows": 1} for name in cbpt.TARGETS], "non_targets": [],
            "dump_file": "dump.sql.gz", "dump_sha256": digest}


def make_archive(path, operation_id):
    raw = gzip.compress(b"CREATE TABLE fixture (id bigint primary key);\nINSERT INTO fixture VALUES (7);\n")
    cbpt.write_private(path / "dump.sql.gz", raw)
    cbpt.json_private(path / "manifest.json", manifest(operation_id, hashlib.sha256(raw).hexdigest()))


def make_proof(path, operation_id):
    value = cbpt.manifest_binding(path, operation_id, SHA)
    proof = {"format_version": 1, "operation_id": operation_id, "source_sha": SHA,
             "source_target_hash": value["source_target_hash"], "dump_sha256": value["dump_sha256"],
             "manifest_sha256": hashlib.sha256(cbpt.private_file(path / "manifest.json")).hexdigest(),
             "verified": True, "target_table_count": 22, "restore_server_uuid": RESTORE_UUID,
             "marker_sha256": "c" * 64}
    cbpt.json_private(path / "restore-proof.json", proof)


class FakeRuntime:
    last = None
    failed_operation = None

    def __init__(self, binary, run_id, source_sha):
        self.owner = "d" * 32
        self.deadline = time.monotonic() + 4200
        self.last_failure_output = None
        self.calls = []
        self.cleaned = False
        self.source = None
        self.isolated = None
        FakeRuntime.last = self

    def invoke(self, args, **kwargs):
        self.calls.append(args)
        return "infra-network"

    def image(self, tag, **kwargs):
        self.calls.append(["image", tag])
        return "sha256:" + "e" * 64

    def tool(self, operation, connection, archive, operation_id, image, **kwargs):
        self.calls.append(["tool", operation])
        values = json.loads(cbpt.private_file(connection / "mysql.json"))
        if operation == "audit":
            self.source = values
        if operation == self.failed_operation:
            self.last_failure_output = dict(receipt(operation, operation_id), status="blocked", error_category="fixture_failure")
            cbpt.fail("tool_fixture_failure")
        if operation == "archive":
            make_archive(archive, operation_id)
        if operation == "verify-restored":
            self.isolated = values
            marker = json.loads(cbpt.private_file(connection / "restore-target.json"))
            assert marker["source_server_uuid"] == SOURCE_UUID
            assert marker["restore_server_uuid"] == RESTORE_UUID
            assert marker["restore_database"] == values["database"]
            assert marker["container_owner"] == self.owner
            make_proof(archive, operation_id)
        return receipt(operation, operation_id)

    def docker_space(self, required):
        self.calls.append(["space", required])

    def restore_container(self, connection, archive, operation_id, image):
        self.calls.append(["restore_container"])
        assert SECRET not in (connection / "root-password").read_text()
        assert SECRET not in (connection / "mysql.cnf").read_text()
        return "f" * 64

    def wait_restore(self, container, source_uuid):
        assert source_uuid == SOURCE_UUID
        return RESTORE_UUID

    def restore_sql(self, container, sql, **kwargs):
        self.calls.append(["restore_sql", sql])
        return 0, ""

    def stream_restore(self, container, archive):
        self.calls.append(["stream_restore"])
        assert gzip.decompress(archive["path"].read_bytes()).startswith(b"CREATE TABLE")

    def cleanup(self):
        self.cleaned = True


class Contracts(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(dir="/private/tmp", prefix="cbpt-test-")
        self.root = Path(self.temporary.name)
        self.binary = self.root / "tool"
        self.binary.write_bytes(b"fixture")
        self.binary.chmod(0o700)
        self.archives = self.root / "archives"
        self.archives.mkdir(mode=0o700)
        self.args = argparse.Namespace(operation="audit", tool_binary=str(self.binary),
                                       source_sha=SHA, run_id="123-1", archive_id="")
        FakeRuntime.failed_operation = None

    def tearDown(self):
        self.temporary.cleanup()

    def test_config_special_characters_and_modes_without_database_option(self):
        values = cbpt.validate_source(dict(env(), MYSQL_PASSWORD='a"b\\c\td'))
        with cbpt.connections(values) as root:
            self.assertEqual(stat.S_IMODE(root.stat().st_mode), 0o700)
            for path in root.iterdir():
                self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
            self.assertNotIn("database=", (root / "mysql.cnf").read_text())
            self.assertIn('password="a\\"b\\\\c\\td"', (root / "mysql.cnf").read_text())
            self.assertEqual(json.loads((root / "mysql.json").read_text()), values)
        self.assertFalse(root.exists())

    def test_environment_controls_and_system_database_fail_closed(self):
        for changes in ({"MYSQL_PASSWORD": "bad\nsecret"}, {"MYSQL_HOST": "bad;host"},
                        {"MYSQL_PORT": "0"}, {"MYSQL_DATABASE": "mysql"}, {"MYSQL_USERNAME": ""}):
            with self.subTest(changes=tuple(changes)), self.assertRaises(cbpt.CleanupError):
                cbpt.validate_source(dict(env(), **changes))

    def test_private_root_lock_collision_and_symlink_are_rejected(self):
        with cbpt.archive_lock(self.archives), self.assertRaisesRegex(cbpt.CleanupError, "already_running"):
            with cbpt.archive_lock(self.archives):
                pass
        link = self.root / "alias"
        link.symlink_to(self.archives)
        with self.assertRaises(cbpt.CleanupError):
            cbpt.directory_check(link)
        self.archives.chmod(0o750)
        with self.assertRaises(cbpt.CleanupError):
            cbpt.directory_check(self.archives)

    def test_safe_envelope_rejects_pii_duplicate_key_bool_and_unknown_hash(self):
        for value in (dict(receipt("audit"), database=SECRET),
                      dict(receipt("audit"), target_table_count=True),
                      dict(receipt("audit"), source_target_hash=SECRET),
                      dict(receipt("audit"), unknown_count=23)):
            with self.assertRaises(cbpt.CleanupError) as caught:
                cbpt.safe_output(json.dumps(value), "audit", SHA, "123-1")
            self.assertNotIn(SECRET, str(caught.exception))
        with self.assertRaises(cbpt.CleanupError):
            cbpt.safe_output('{"format_version":1,"format_version":1}', "audit", SHA, "123-1")

    def test_tool_success_none_and_legal_archive_only_and_apply_boundary(self):
        runtime = cbpt.Runtime(self.binary, "123-1", SHA, ["docker"])
        archive = self.archives
        with patch.object(runtime, "preflight_name"), patch.object(cbpt, "capture", return_value=(0, json.dumps(receipt("audit")))):
            self.assertEqual(runtime.tool("audit", self.root, archive, "123-1", "image")["status"], "ok")
        only = dict(receipt("audit"), status="archive_only", drop_eligible=False,
                    error_category="metadata_visibility_blocked")
        with patch.object(runtime, "preflight_name"), patch.object(cbpt, "capture", return_value=(0, json.dumps(only))):
            self.assertEqual(runtime.tool("audit", self.root, archive, "123-1", "image")["status"], "archive_only")
        for operation in ("apply", "verify-restored"):
            value = dict(receipt(operation), status="archive_only", drop_eligible=False,
                         error_category="metadata_visibility_blocked")
            with patch.object(runtime, "preflight_name"), patch.object(cbpt, "capture", return_value=(0, json.dumps(value))), self.assertRaises(cbpt.CleanupError):
                runtime.tool(operation, self.root, archive, "123-1", "image")

    def test_removed_needs_complete_ledger_even_when_absent(self):
        runtime = cbpt.Runtime(self.binary, "123-1", SHA, ["docker"])
        for changes in ({"remaining_target_count": 1}, {"ledger_complete": False},
                        {"dropped_count": 21}, {"unknown_count": 1}):
            value = dict(receipt("verify-removed"), **changes)
            with patch.object(runtime, "preflight_name"), patch.object(cbpt, "capture", return_value=(0, json.dumps(value))), self.assertRaisesRegex(cbpt.CleanupError, "completion"):
                runtime.tool("verify-removed", self.root, self.archives, "123-1", "image")
        with patch.object(runtime, "preflight_name"), patch.object(cbpt, "capture", return_value=(0, json.dumps(receipt("verify-removed")))):
            self.assertTrue(runtime.tool("verify-removed", self.root, self.archives, "123-1", "image")["ledger_complete"])

    def test_capture_sanitizes_stderr_and_timeout(self):
        status, output = cbpt.capture([sys.executable, "-c", "import sys;sys.stderr.write(sys.argv[1]);print('safe')", SECRET])
        self.assertEqual((status, output.strip()), (0, "safe"))
        start = time.monotonic()
        with self.assertRaises(cbpt.CleanupError) as caught:
            cbpt.capture([sys.executable, "-c", "import time;time.sleep(10)"], timeout=0.15)
        self.assertLess(time.monotonic() - start, 3)
        self.assertNotIn(SECRET, str(caught.exception))

    def test_stream_real_pipe_success_failure_timeout_and_size_bound(self):
        dump = self.root / "stream.gz"
        raw = b"x\x00\xff" * 100000
        dump.write_bytes(gzip.compress(raw))
        output = self.root / "restored"
        cbpt.restore_stream([sys.executable, "-c", "import pathlib,sys;pathlib.Path(sys.argv[1]).write_bytes(sys.stdin.buffer.read())", str(output)], dump, timeout=5)
        self.assertEqual(output.read_bytes(), raw)
        with self.assertRaises(cbpt.CleanupError):
            cbpt.restore_stream([sys.executable, "-c", "import sys;sys.stderr.write(sys.argv[1]);sys.exit(1)", SECRET], dump, timeout=5)
        with self.assertRaises(cbpt.CleanupError):
            cbpt.restore_stream([sys.executable, "-c", "import time;time.sleep(10)"], dump, timeout=0.1)
        with patch.object(cbpt, "RESTORE_LIMIT", 10), self.assertRaisesRegex(cbpt.CleanupError, "size_limit"):
            cbpt.restore_stream([sys.executable, "-c", "import sys;sys.stdin.buffer.read()"], dump, timeout=5)

    def test_dump_hash_corruption_truncation_and_private_permissions(self):
        archive = self.archives / "123-1"
        archive.mkdir(mode=0o700)
        make_archive(archive, "123-1")
        value = cbpt.manifest_binding(archive, "123-1", SHA)
        self.assertGreater(cbpt.dump_check(archive / "dump.sql.gz", value["dump_sha256"]), 0)
        with self.assertRaises(cbpt.CleanupError):
            cbpt.dump_check(archive / "dump.sql.gz", "f" * 64)
        raw = (archive / "dump.sql.gz").read_bytes()[:-8]
        (archive / "dump.sql.gz").write_bytes(raw)
        with self.assertRaises(cbpt.CleanupError):
            cbpt.dump_check(archive / "dump.sql.gz", hashlib.sha256(raw).hexdigest())
        (archive / "dump.sql.gz").chmod(0o640)
        with self.assertRaises(cbpt.CleanupError):
            cbpt.dump_check(archive / "dump.sql.gz", hashlib.sha256(raw).hexdigest())

    def test_manifest_and_proof_wrong_identity_or_hash_are_rejected(self):
        archive = self.archives / "123-1"
        archive.mkdir(mode=0o700)
        make_archive(archive, "123-1")
        make_proof(archive, "123-1")
        cbpt.proof_binding(archive, "123-1", SHA)
        path = archive / "restore-proof.json"
        original = json.loads(path.read_text())
        for changes in ({"source_sha": "f" * 40}, {"manifest_sha256": "e" * 64},
                        {"restore_server_uuid": SOURCE_UUID}, {"verified": False}):
            path.write_text(json.dumps(dict(original, **changes)))
            with self.assertRaises(cbpt.CleanupError):
                cbpt.proof_binding(archive, "123-1", SHA)

    def test_archive_verify_uses_distinct_credentials_and_retains_artifacts(self):
        self.args.operation = "archive-verify"
        result = cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=FakeRuntime)
        runtime = FakeRuntime.last
        self.assertTrue(result["complete"])
        self.assertEqual(result["total_archived_rows"], 22)
        self.assertGreater(result["archive_compressed_bytes"], 0)
        self.assertGreater(result["archive_uncompressed_bytes"], 0)
        self.assertTrue(runtime.cleaned)
        self.assertNotEqual(runtime.source["password"], runtime.isolated["password"])
        self.assertEqual(runtime.source["host"], "mysql.fixture")
        self.assertEqual(runtime.isolated["host"], "127.0.0.1")
        self.assertTrue(runtime.isolated["database"].startswith("cbpt_restore_"))
        self.assertTrue((self.archives / "123-1" / "dump.sql.gz").exists())
        self.assertTrue((self.archives / "123-1" / "restore-proof.json").exists())
        self.assertNotIn(SECRET, json.dumps(result))
        self.assertNotIn(SOURCE_UUID, json.dumps(result))

    def test_archive_collision_does_not_overwrite_and_failed_restore_retains_archive(self):
        self.args.operation = "archive-verify"
        FakeRuntime.failed_operation = "verify-restored"
        with self.assertRaises(cbpt.CleanupError) as caught:
            cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=FakeRuntime)
        self.assertTrue(FakeRuntime.last.cleaned)
        self.assertTrue((self.archives / "123-1" / "dump.sql.gz").exists())
        self.assertEqual([s["operation"] for s in caught.exception.result["stages"]], ["audit", "archive"])
        with self.assertRaisesRegex(cbpt.CleanupError, "exists"):
            cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=FakeRuntime)

    def test_apply_needs_prior_id_and_verified_proof_then_verifies_removed(self):
        self.args.operation = "apply"
        with self.assertRaisesRegex(cbpt.CleanupError, "archive_id_required"):
            cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=FakeRuntime)
        archive = self.archives / "122-1"
        archive.mkdir(mode=0o700)
        self.args.archive_id = "122-1"
        make_archive(archive, "122-1")
        with self.assertRaises(cbpt.CleanupError):
            cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=FakeRuntime)
        make_proof(archive, "122-1")
        result = cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=FakeRuntime)
        self.assertEqual([s["operation"] for s in result["stages"]], ["apply", "verify-removed"])
        self.assertTrue((archive / "dump.sql.gz").exists())

    def test_runtime_commands_are_bounded_and_never_mount_source_credentials_into_restore(self):
        runtime = cbpt.Runtime(self.binary, "123-1", SHA, ["docker"])
        runtime.deadline = time.monotonic() + 121
        self.assertLessEqual(runtime.budget(1800), 1.1)
        runtime.deadline = time.monotonic()
        with self.assertRaisesRegex(cbpt.CleanupError, "budget"):
            runtime.budget(30)
        runtime.cleaning = True
        self.assertEqual(runtime.budget(300), 30)

    def test_source_command_does_not_put_credentials_in_argv(self):
        runtime = cbpt.Runtime(self.binary, "123-1", SHA, ["docker"])
        with cbpt.connections(cbpt.validate_source(env())) as connection:
            with patch.object(runtime, "preflight_name"), patch.object(cbpt, "capture", return_value=(0, json.dumps(receipt("audit")))) as captured:
                runtime.tool("audit", connection, self.archives, "123-1", "sha256:" + "e" * 64)
            command = captured.call_args.args[0]
            self.assertNotIn(SECRET, json.dumps(command))
            self.assertIn("infra-network", command)
            self.assertIn("--cap-drop=ALL", command)
            self.assertIn("--read-only", command)
            self.assertNotIn("MYSQL_PASSWORD", json.dumps(command))
            self.assertNotIn("MYSQL_USERNAME", json.dumps(command))

    def test_restore_command_isolated_and_owner_checked_before_mount(self):
        runtime = cbpt.Runtime(self.binary, "123-1", SHA, ["docker"])
        container = "e" * 64
        volume = "qs-cbpt-restore-" + runtime.owner + "-data"
        source_connection = self.root / "source-credentials-never-mounted"
        restore_connection = self.root / "restore-only"
        restore_connection.mkdir(mode=0o700)
        calls = []
        def invoke(args, **kwargs):
            calls.append(args)
            if args[:2] == ["volume", "create"]:
                return volume
            if args[:2] == ["volume", "inspect"]:
                return json.dumps({cbpt.OWNER_LABEL: runtime.owner})
            if args[0] == "run":
                return container
            if args[:2] == ["container", "inspect"]:
                return json.dumps([{"Id": container, "Config": {"Labels": {cbpt.OWNER_LABEL: runtime.owner}},
                                    "HostConfig": {"NetworkMode": "none", "PortBindings": {}, "Privileged": False}}])
            raise AssertionError("unexpected command")
        with patch.object(runtime, "preflight_name"), patch.object(cbpt, "capture", return_value=(1, "")), patch.object(runtime, "invoke", side_effect=invoke):
            self.assertEqual(runtime.restore_container(restore_connection, self.archives, "123-1", "image"), container)
        run = next(args for args in calls if args[0] == "run")
        self.assertEqual(run[run.index("--network") + 1], "none")
        self.assertNotIn("-p", run)
        self.assertNotIn("--publish", run)
        self.assertNotIn(str(source_connection), json.dumps(run))
        self.assertIn("MYSQL_ROOT_PASSWORD_FILE=/connection/root-password", run)
        self.assertNotIn(SECRET, json.dumps(run))
        self.assertLess(next(i for i,args in enumerate(calls) if args[:2] == ["volume", "inspect"]),
                        next(i for i,args in enumerate(calls) if args[0] == "run"))

    def test_manifest_exact_scope_and_boolean_counts_fail_closed(self):
        archive = self.archives / "123-1"
        archive.mkdir(mode=0o700)
        make_archive(archive, "123-1")
        path = archive / "manifest.json"
        original = json.loads(path.read_text())
        for tables in (original["tables"][:-1], original["tables"] + [original["tables"][0]],
                       [original["tables"][0]] * 22,
                       [dict(table, rows=True) for table in original["tables"]]):
            path.write_text(json.dumps(dict(original, tables=tables)))
            with self.assertRaises(cbpt.CleanupError):
                cbpt.manifest_binding(archive, "123-1", SHA)

    def test_failed_apply_stops_before_verify_and_retains_safe_progress(self):
        archive = self.archives / "122-1"
        archive.mkdir(mode=0o700)
        make_archive(archive, "122-1")
        make_proof(archive, "122-1")
        self.args.operation = "apply"
        self.args.archive_id = "122-1"
        FakeRuntime.failed_operation = "apply"
        with self.assertRaises(cbpt.CleanupError) as caught:
            cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=FakeRuntime)
        self.assertTrue(FakeRuntime.last.cleaned)
        self.assertEqual(caught.exception.result["stage"], "source_apply")
        self.assertEqual(caught.exception.result["failure"]["error_category"], "fixture_failure")
        self.assertFalse(any(call == ["tool", "verify-removed"] for call in FakeRuntime.last.calls))
        self.assertTrue((archive / "dump.sql.gz").exists())

    def test_volume_create_race_blocks_before_container_run(self):
        runtime = cbpt.Runtime(self.binary, "123-1", SHA, ["docker"])
        volume = "qs-cbpt-restore-" + runtime.owner + "-data"
        calls = []
        def invoke(args, **kwargs):
            calls.append(args)
            if args[:2] == ["volume", "create"]:
                return volume
            if args[:2] == ["volume", "inspect"]:
                return json.dumps({cbpt.OWNER_LABEL: "foreign"})
            raise AssertionError("container run must not happen")
        with patch.object(runtime, "preflight_name"), patch.object(cbpt, "capture", return_value=(1, "")), patch.object(runtime, "invoke", side_effect=invoke), self.assertRaisesRegex(cbpt.CleanupError, "ownership"):
            runtime.restore_container(self.root, self.archives, "123-1", "image")
        self.assertFalse(any(args[0] == "run" for args in calls))

    def test_cleanup_only_exact_owned_resources_and_no_prune(self):
        runtime = cbpt.Runtime(self.binary, "123-1", SHA, ["docker"])
        container = "a" * 64
        volume = "qs-cbpt-restore-" + runtime.owner + "-data"
        calls = []
        counters = {"ps": 0, "volume_ls": 0}
        def invoke(args, **kwargs):
            calls.append(args)
            if args[0] == "ps":
                counters["ps"] += 1
                return container if counters["ps"] == 1 else ""
            if args[:2] == ["volume", "ls"]:
                counters["volume_ls"] += 1
                return volume if counters["volume_ls"] == 1 else ""
            if args[:2] == ["volume", "inspect"]:
                return json.dumps({cbpt.OWNER_LABEL: runtime.owner})
            return ""
        with patch.object(runtime, "invoke", side_effect=invoke):
            runtime.cleanup()
        self.assertIn(["rm", "--force", container], calls)
        self.assertIn(["volume", "rm", volume], calls)
        self.assertNotIn("prune", json.dumps(calls))
        self.assertFalse(any("archive" in str(item) for args in calls for item in args))
        with patch.object(runtime, "invoke", return_value="foreign"), self.assertRaisesRegex(cbpt.CleanupError, "cleanup_unconfirmed"):
            runtime.cleanup()

    def test_main_never_prints_source_secrets_and_preserves_safe_partial_progress(self):
        error = cbpt.CleanupError("tool_fixture_failure")
        error.result = {"format_version": 1, "complete": False, "operation": "apply", "source_sha": SHA,
                        "run_id": "123-1", "archive_id": "122-1", "stages": [receipt("apply", "122-1")],
                        "failure": dict(receipt("verify-removed", "122-1"), status="blocked", unknown_count=1)}
        output = io.StringIO()
        with patch.object(cbpt, "perform", side_effect=error), contextlib.redirect_stdout(output):
            status = cbpt.main(["--operation", "apply", "--tool-binary", str(self.binary), "--source-sha", SHA,
                                "--run-id", "123-1", "--archive-id", "122-1"], env())
        self.assertEqual(status, 1)
        raw = output.getvalue()
        self.assertNotIn(SECRET, raw)
        self.assertNotIn(SOURCE_UUID, raw)
        self.assertIn('"unknown_count": 1', raw)
        self.assertTrue(raw.startswith("QS_CBPT_CLEANUP_BEGIN\n"))
        self.assertTrue(raw.endswith("QS_CBPT_CLEANUP_END\n"))


if __name__ == "__main__":
    unittest.main()
