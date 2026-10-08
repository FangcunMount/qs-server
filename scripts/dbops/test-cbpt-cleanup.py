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


def full_receipt(operation, operation_id="123-1"):
    # All emitted Go struct fields, including the successful audit/archive "none" reason.
    value = receipt(operation, operation_id)
    value.update(manifest_sha256="", dump_sha256="", proof_sha256="", drop_block_reason="",
                 dropped_count=0, pending_count=22, unknown_count=0, failed_count=0, ledger_complete=False)
    if operation in {"audit", "archive"}:
        value["drop_block_reason"] = "none"
    if operation in {"archive", "verify-restored", "apply", "verify-removed"}:
        value.update(manifest_sha256=HASH, dump_sha256=HASH)
    if operation in {"verify-restored", "apply", "verify-removed"}:
        value["proof_sha256"] = HASH
    if operation == "verify-restored":
        value["drop_eligible"] = False
    if operation in {"apply", "verify-removed"}:
        value.update(dropped_count=22, pending_count=0, ledger_complete=True)
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
        self.temporary = tempfile.TemporaryDirectory(dir=str(Path(tempfile.gettempdir()).resolve()), prefix="cbpt-test-")
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

    def test_docker_space_uses_unprivileged_df_on_actual_docker_root(self):
        runtime = cbpt.Runtime(self.binary, "123-1", SHA, ["docker"])
        root = "/var/lib/docker"
        output = "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/fixture 20000000 5000000 15000000 25% /var/lib\n"
        def ordinary_df_only(command, **kwargs):
            # Simulate a host that rejects every extra sudo command.
            if command[0] == "sudo":
                cbpt.fail("fixture_sudo_denied")
            self.assertEqual(command, ["env", "LC_ALL=C", "df", "-Pk", "--", root])
            self.assertEqual(kwargs["timeout"], 15)
            return 0, output
        with patch.object(runtime, "invoke", return_value=root) as invoked, patch.object(cbpt, "capture", side_effect=ordinary_df_only):
            runtime.docker_space(8 * cbpt.GIB)
        invoked.assert_called_once_with(["info", "--format", "{{.DockerRootDir}}"])

    def test_docker_space_nonzero_never_reports_low_or_accepts_capacity(self):
        runtime = cbpt.Runtime(self.binary, "123-1", SHA, ["docker"])
        header = "Filesystem 1024-blocks Used Available Capacity Mounted on\n"
        for output in ("", header + "/dev/fixture 20 19 1 95% /\n", header + "/dev/fixture 20000000 0 20000000 0% /\n"):
            with self.subTest(output=output), patch.object(runtime, "invoke", return_value="/var/lib/docker"), patch.object(cbpt, "capture", return_value=(1, output)), self.assertRaisesRegex(cbpt.CleanupError, "^docker_disk_unknown$"):
                runtime.docker_space(8 * cbpt.GIB)

    def test_docker_space_malformed_or_ambiguous_posix_output_is_unknown(self):
        runtime = cbpt.Runtime(self.binary, "123-1", SHA, ["docker"])
        header = "Filesystem 1024-blocks Used Available Capacity Mounted on\n"
        row = "/dev/fixture 20000000 5000000 15000000 25% /\n"
        for output in ("", row, header, header + row.rstrip("\n"), header + row + row,
                       header + "/dev/fixture\n20000000 5000000 15000000 25% /\n",
                       (header + row).replace("\n", "\r\n"), header.replace("Available", "Libre") + row,
                       header + row.replace(" /\n", " relative\n"), header + row.replace("25%", "101%"),
                       header + row.replace("25%", "25"), header + row.replace(" /\n", " /bad\x00path\n"),
                       header + row.replace("/dev/fixture", "/dev/bad\x00fixture"),
                       header + row.replace("/dev/fixture", "/dev/bad\x7ffixture")):
            with self.subTest(output=output), patch.object(runtime, "invoke", return_value="/var/lib/docker"), patch.object(cbpt, "capture", return_value=(0, output)), self.assertRaisesRegex(cbpt.CleanupError, "^docker_disk_unknown$"):
                runtime.docker_space(8 * cbpt.GIB)

    def test_docker_space_numeric_bounds_and_consistency_are_checked(self):
        runtime = cbpt.Runtime(self.binary, "123-1", SHA, ["docker"])
        header = "Filesystem 1024-blocks Used Available Capacity Mounted on\n"
        for total, used, available in (("20", "0", "-1"), ("20", "0", "+1"), ("20", "0", "1.5"),
                                       ("20", "0", "1e3"), ("20", "0", "9" * 40),
                                       (str(2 ** 63), "0", "1"), (str((2 ** 63 - 1) // 1024 + 1), "0", "1"),
                                       ("0", "0", "0"),
                                       ("20", "21", "0"), ("20", "10", "11")):
            output = header + f"/dev/fixture {total} {used} {available} 0% /\n"
            with self.subTest(values=(total, used, available)), patch.object(runtime, "invoke", return_value="/var/lib/docker"), patch.object(cbpt, "capture", return_value=(0, output)), self.assertRaisesRegex(cbpt.CleanupError, "^docker_disk_unknown$"):
                runtime.docker_space(8 * cbpt.GIB)

    def test_docker_space_capacity_threshold_is_in_kibibytes(self):
        runtime = cbpt.Runtime(self.binary, "123-1", SHA, ["docker"])
        output = "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/fixture 10 9 1 90% /\n"
        with patch.object(runtime, "invoke", return_value="/var/lib/docker"), patch.object(cbpt, "capture", return_value=(0, output)):
            runtime.docker_space(1024)
            with self.assertRaisesRegex(cbpt.CleanupError, "^docker_disk_low$"):
                runtime.docker_space(1025)
        blocks = (2 ** 63 - 1) // 1024
        output = f"Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/fixture {blocks} 0 {blocks} 0% /\n"
        with patch.object(runtime, "invoke", return_value="/var/lib/docker"), patch.object(cbpt, "capture", return_value=(0, output)):
            runtime.docker_space(blocks * 1024)

    def test_docker_space_invalid_storage_identity_never_runs_df(self):
        runtime = cbpt.Runtime(self.binary, "123-1", SHA, ["docker"])
        for root in ("relative", "", "/var/lib/docker\n/another", "/var/lib/do\x00cker", "/var/lib/docker\r"):
            with self.subTest(root=root), patch.object(runtime, "invoke", return_value=root), patch.object(cbpt, "capture") as captured, self.assertRaisesRegex(cbpt.CleanupError, "^docker_storage_identity_invalid$"):
                runtime.docker_space(8 * cbpt.GIB)
            captured.assert_not_called()

    def test_actual_docker_space_failure_retains_archive_and_stops_before_restore(self):
        self.args.operation = "archive-verify"
        class SpaceRuntime(FakeRuntime):
            docker_space = cbpt.Runtime.docker_space
            def invoke(self, args, **kwargs):
                if args[0] == "info":
                    return "/var/lib/docker"
                return super().invoke(args, **kwargs)
        with patch.object(cbpt, "capture", return_value=(1, "")), self.assertRaisesRegex(cbpt.CleanupError, "^docker_disk_unknown$") as caught:
            cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=SpaceRuntime)
        self.assertEqual(caught.exception.result["stage"], "archive_validation")
        self.assertTrue(FakeRuntime.last.cleaned)
        self.assertFalse(any(call == ["restore_container"] for call in FakeRuntime.last.calls))
        archive = self.archives / "123-1"
        self.assertTrue((archive / "dump.sql.gz").exists())
        self.assertTrue((archive / "manifest.json").exists())
        self.assertFalse((archive / "restore-proof.json").exists())
        self.assertFalse((archive / "drop-ledger.json").exists())

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
        error = cbpt.CleanupError("tool_drop_ledger_unconfirmed")
        error.result = {"format_version": 1, "complete": False, "operation": "apply", "source_sha": SHA,
                        "run_id": "123-1", "archive_id": "122-1", "stages": [receipt("apply", "122-1")],
                        "failure": dict(receipt("verify-removed", "122-1"), status="unknown", error_category="drop_ledger_unconfirmed",
                                        unknown_count=1, dropped_count=21, ledger_complete=False)}
        output = io.StringIO()
        with patch.object(cbpt, "perform", side_effect=error), contextlib.redirect_stdout(output):
            status = cbpt.main(["--operation", "apply", "--tool-binary", str(self.binary), "--source-sha", SHA,
                                "--run-id", "123-1", "--archive-id", "122-1"], env())
        self.assertEqual(status, 1)
        raw = output.getvalue()
        self.assertNotIn(SECRET, raw)
        self.assertNotIn(SOURCE_UUID, raw)
        decoded = cbpt.load_json_bytes(cbpt.receipt_transport_module().decode_armored_receipt(raw.splitlines()[1]))
        self.assertEqual(decoded["failure"]["unknown_count"], 1)
        self.assertEqual(decoded["failure"]["dropped_count"], 21)
        self.assertFalse(decoded["failure"]["ledger_complete"])
        self.assertTrue(raw.startswith("QS_CBPT_CLEANUP_BEGIN\n"))
        self.assertTrue(raw.endswith("QS_CBPT_CLEANUP_END\n"))


    def output_args(self, operation="audit", archive_id=""):
        return ["--operation", operation, "--tool-binary", str(self.binary), "--source-sha", SHA,
                "--run-id", "123-1", "--archive-id", archive_id]

    def decoded_summary(self, raw):
        lines = raw.splitlines()
        self.assertEqual(len(lines), 3)
        self.assertEqual(lines[0], "QS_CBPT_CLEANUP_BEGIN")
        self.assertEqual(lines[2], "QS_CBPT_CLEANUP_END")
        self.assertFalse(lines[1].startswith("{"))
        return cbpt.load_json_bytes(cbpt.receipt_transport_module().decode_armored_receipt(lines[1]))

    def test_encoded_archive_success_roundtrips_all_verified_stages_and_counts(self):
        self.args.operation = "archive-verify"
        result = cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=FakeRuntime)
        output = io.StringIO()
        with patch.object(cbpt, "perform", return_value=result), contextlib.redirect_stdout(output):
            status = cbpt.main(self.output_args("archive-verify"), env())
        decoded = self.decoded_summary(output.getvalue())
        self.assertEqual(status, 0)
        self.assertEqual(decoded, result)
        self.assertEqual([x["operation"] for x in decoded["stages"]], ["audit", "archive", "verify-restored"])
        self.assertEqual(decoded["total_archived_rows"], 22)
        self.assertNotIn(SECRET, output.getvalue())
        self.assertNotIn(SOURCE_UUID, output.getvalue())

    def test_real_go_field_shape_archive_full_chain_encodes_success_none_reason(self):
        class CompleteReceiptRuntime(FakeRuntime):
            def tool(self, operation, connection, archive, operation_id, image, **kwargs):
                super().tool(operation, connection, archive, operation_id, image, **kwargs)
                return full_receipt(operation, operation_id)

        self.args.operation = "archive-verify"
        result = cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=CompleteReceiptRuntime)
        output = io.StringIO()
        with patch.object(cbpt, "perform", return_value=result), contextlib.redirect_stdout(output):
            self.assertEqual(cbpt.main(self.output_args("archive-verify"), env()), 0)
        decoded = self.decoded_summary(output.getvalue())
        self.assertEqual(decoded, result)
        self.assertEqual([item["drop_block_reason"] for item in decoded["stages"]], ["none", "none", ""])
        self.assertFalse(decoded["stages"][2]["drop_eligible"])
        self.assertEqual(decoded["total_archived_rows"], 22)
        self.assertTrue(FakeRuntime.last.cleaned)

    def test_full_apply_and_verify_summaries_encode_confirmed_ledger(self):
        class CompleteReceiptRuntime(FakeRuntime):
            def tool(self, operation, connection, archive, operation_id, image, **kwargs):
                super().tool(operation, connection, archive, operation_id, image, **kwargs)
                return full_receipt(operation, operation_id)

        archive = self.archives / "122-1"
        archive.mkdir(mode=0o700)
        make_archive(archive, "122-1")
        make_proof(archive, "122-1")
        for operation in ("apply", "verify"):
            self.args.operation, self.args.archive_id = operation, "122-1"
            result = cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=CompleteReceiptRuntime)
            output = io.StringIO()
            with self.subTest(operation=operation), patch.object(cbpt, "perform", return_value=result), contextlib.redirect_stdout(output):
                self.assertEqual(cbpt.main(self.output_args(operation, "122-1"), env()), 0)
            decoded = self.decoded_summary(output.getvalue())
            self.assertEqual(decoded, result)
            self.assertEqual(decoded["stages"][-1]["remaining_target_count"], 0)
            self.assertEqual(decoded["stages"][-1]["dropped_count"], 22)
            self.assertTrue(decoded["stages"][-1]["ledger_complete"])

    def test_archive_only_envelope_keeps_drop_blocked_without_rewriting_gate(self):
        class ArchiveOnlyRuntime(FakeRuntime):
            def tool(self, operation, connection, archive, operation_id, image, **kwargs):
                super().tool(operation, connection, archive, operation_id, image, **kwargs)
                return dict(full_receipt(operation, operation_id), status="archive_only", drop_eligible=False,
                            drop_block_reason="metadata_visibility_blocked")

        result = cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=ArchiveOnlyRuntime)
        output = io.StringIO()
        with patch.object(cbpt, "perform", return_value=result), contextlib.redirect_stdout(output):
            self.assertEqual(cbpt.main(self.output_args(), env()), 0)
        decoded = self.decoded_summary(output.getvalue())
        self.assertEqual(decoded, result)
        self.assertFalse(decoded["stages"][0]["drop_eligible"])
        self.assertEqual(decoded["stages"][0]["drop_block_reason"], "metadata_visibility_blocked")

    def test_legacy_empty_stage_category_and_late_cleanup_failure_keep_safe_progress(self):
        result = cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=FakeRuntime)
        result["stages"][0]["error_category"] = ""
        output = io.StringIO()
        with patch.object(cbpt, "perform", return_value=result), contextlib.redirect_stdout(output):
            self.assertEqual(cbpt.main(self.output_args(), env()), 0)
        self.assertEqual(self.decoded_summary(output.getvalue()), result)
        error = cbpt.CleanupError("resource_cleanup_unconfirmed")
        error.result = dict(result, complete=False, stage="source_audit")
        output = io.StringIO()
        with patch.object(cbpt, "perform", side_effect=error), contextlib.redirect_stdout(output):
            self.assertEqual(cbpt.main(self.output_args(), env()), 1)
        value = self.decoded_summary(output.getvalue())
        self.assertFalse(value["complete"])
        self.assertEqual(value["stages"], result["stages"])
        self.assertEqual(value["error_category"], "resource_cleanup_unconfirmed")
        self.assertNotIn("failure", value)

    def test_failure_receipt_rechecks_binding_scope_and_private_fields_before_encoding(self):
        failure = dict(full_receipt("apply", "122-1"), status="unknown", error_category="drop_execution_unknown",
                       dropped_count=0, pending_count=21, unknown_count=1, ledger_complete=False)
        for changes in ({"source_sha": "f" * 40}, {"operation": "audit"},
                        {"target_table_count": 23}, {"raw_error": SECRET}):
            error = cbpt.CleanupError("tool_drop_execution_unknown")
            error.result = {"format_version": 1, "complete": False, "operation": "apply", "source_sha": SHA,
                            "run_id": "123-1", "archive_id": "122-1", "stage": "source_apply", "stages": [],
                            "failure": dict(failure, **changes)}
            output = io.StringIO()
            with self.subTest(keys=tuple(changes)), patch.object(cbpt, "perform", side_effect=error), contextlib.redirect_stdout(output):
                self.assertEqual(cbpt.main(self.output_args("apply", "122-1"), env()), 1)
            value = self.decoded_summary(output.getvalue())
            self.assertEqual(value["error_category"], "unsafe_cleanup_summary")
            self.assertNotIn("failure", value)
            self.assertNotIn(SECRET, json.dumps(value))

    def test_transport_resists_ascii_boolean_and_hash_masking_without_changing_plain_api(self):
        result = cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=FakeRuntime)
        output = io.StringIO()
        with patch.object(cbpt, "perform", return_value=result), contextlib.redirect_stdout(output):
            self.assertEqual(cbpt.main(self.output_args(), dict(env(), MYSQL_USERNAME="true", MYSQL_PASSWORD="false")), 0)
        masked = output.getvalue()
        for token in ("true", "false", "1", SHA, HASH):
            masked = masked.replace(token, "***")
        self.assertEqual(self.decoded_summary(masked), result)
        plain = json.dumps(receipt("audit"))
        self.assertEqual(cbpt.safe_output(plain, "audit", SHA, "123-1"), receipt("audit"))

    def test_final_summary_rejects_untrusted_fields_bindings_counts_and_stage_order(self):
        result = cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=FakeRuntime)
        bad = []
        value = dict(result, raw_error=SECRET)
        bad.append(value)
        for changes in ({"password": SECRET}, {"source_sha": "f" * 40}, {"target_table_count": 21}, {"unknown_count": True}):
            bad.append(dict(result, stages=[dict(result["stages"][0], **changes)]))
        bad.append(dict(result, stages=[receipt("apply")]))
        bad.append(dict(result, total_archived_rows=2 ** 64))
        bad.append(dict(result, archive_compressed_bytes=True))
        for value in bad:
            with self.subTest(keys=tuple(value)), patch.object(cbpt, "perform", return_value=value), contextlib.redirect_stdout(io.StringIO()) as output:
                self.assertEqual(cbpt.main(self.output_args(), env()), 1)
            decoded = self.decoded_summary(output.getvalue())
            self.assertFalse(decoded["complete"])
            self.assertEqual(decoded["error_category"], "unsafe_cleanup_summary")
            self.assertNotIn("stages", decoded)
            self.assertNotIn(SECRET, cbpt.receipt_transport_module().decode_armored_receipt(output.getvalue().splitlines()[1]))

    def test_unknown_exception_tokens_and_untrusted_failure_body_never_get_encoded(self):
        for error in (RuntimeError(SECRET), cbpt.CleanupError("private_database_name_must_never_leak")):
            output = io.StringIO()
            with patch.object(cbpt, "perform", side_effect=error), contextlib.redirect_stdout(output):
                self.assertEqual(cbpt.main(self.output_args(), env()), 1)
            decoded = self.decoded_summary(output.getvalue())
            self.assertEqual(decoded["error_category"], "unexpected_cleanup_failure")
            self.assertNotIn(SECRET, json.dumps(decoded))
            self.assertNotIn("private_database_name_must_never_leak", json.dumps(decoded))
        error = cbpt.CleanupError("docker_disk_unknown")
        error.result = {"format_version": 1, "complete": False, "operation": "audit", "raw_error": SECRET}
        output = io.StringIO()
        with patch.object(cbpt, "perform", side_effect=error), contextlib.redirect_stdout(output):
            self.assertEqual(cbpt.main(self.output_args(), env()), 1)
        self.assertEqual(self.decoded_summary(output.getvalue())["error_category"], "unsafe_cleanup_summary")

    def test_missing_transport_stops_before_any_cleanup_and_has_no_plain_fallback(self):
        output, errors = io.StringIO(), io.StringIO()
        with patch.object(cbpt, "receipt_transport_module", side_effect=OSError(SECRET)), patch.object(cbpt, "perform") as perform, contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
            self.assertEqual(cbpt.main(self.output_args(), env()), 1)
        perform.assert_not_called()
        self.assertEqual(output.getvalue(), "")
        self.assertEqual(errors.getvalue(), "QS_CBPT_CLEANUP_TRANSPORT_UNAVAILABLE\n")

    def test_encoding_refusal_has_no_secret_error_or_plain_fallback(self):
        result = cbpt.perform(self.args, env(), archive_root=self.archives, runtime_class=FakeRuntime)
        transport = cbpt.receipt_transport_module()
        output, errors = io.StringIO(), io.StringIO()
        with patch.object(cbpt, "receipt_transport_module", return_value=transport), patch.object(transport, "encode_armored_receipt", side_effect=RuntimeError(SECRET)), patch.object(cbpt, "perform", return_value=result), contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
            self.assertEqual(cbpt.main(self.output_args(), env()), 1)
        self.assertEqual(output.getvalue(), "")
        self.assertEqual(errors.getvalue(), "QS_CBPT_CLEANUP_TRANSPORT_FAILED\n")

    def test_early_input_failure_encodes_only_valid_bindings_and_fixed_error(self):
        output = io.StringIO()
        argv = self.output_args()
        argv[argv.index("--source-sha") + 1] = SECRET
        with contextlib.redirect_stdout(output):
            self.assertEqual(cbpt.main(argv, env()), 1)
        value = self.decoded_summary(output.getvalue())
        self.assertFalse(value["complete"])
        self.assertEqual(value["error_category"], "invalid_cleanup_binding")
        self.assertNotIn("source_sha", value)
        self.assertNotIn(SECRET, json.dumps(value))


if __name__ == "__main__":
    unittest.main()
