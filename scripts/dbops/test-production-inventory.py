import contextlib
import importlib.util
import io
import json
import shutil
from pathlib import Path
import subprocess
import stat
import sys
import os
import re
import tempfile
import time
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("inventory", Path(__file__).with_name("production-inventory.py"))
inventory = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(inventory)
SECRET = "FIXTURE_PASSWORD_MUST_NEVER_BE_OUTPUT"
MYSQL = "server\tqs\t8.0.36\ntables\tschema_migrations\tBASE TABLE\tInnoDB\t1\t16384\t0\ntables\thistorical_view\tVIEW\tNULL\tNULL\tNULL\tNULL\ncolumns\tschema_migrations\t1\tversion\tbigint\tNO\tNULL\t20\nindexes\tschema_migrations\tPRIMARY\t0\t1\tversion\tNULL\tBTREE\tYES\nmigration\t95\t0\n"


def mongo_fixture():
    return {"database": "qs", "metadata_complete": True, "migration_state": [{"version": "36", "dirty": False}], "namespaces": [{"name": "schema_migrations", "type": "collection", "validator_fields": [], "metadata_error": [], "storage": {"estimated_documents": "1", "data_bytes": "22", "storage_bytes": "16384", "index_bytes": "16384"}, "indexes": [{"name": "_id_", "keys": {"_id": 1}, "unique": True, "sparse": False, "hidden": False, "partial": False, "expire_after_seconds": None}]}]}


def environment():
    return {"INVENTORY_SOURCE_SHA": "a" * 40, "INVENTORY_DATABASE": "all", "INVENTORY_RUN_ID": "123-1", "MYSQL_HOST": "mysql.fixture", "MYSQL_USERNAME": "fixture", "MYSQL_PASSWORD": SECRET, "MYSQL_DATABASE": "qs", "MONGODB_HOST": "mongo.fixture", "MONGODB_USERNAME": "fixture", "MONGODB_PASSWORD": SECRET, "MONGODB_DBNAME": "qs"}


class InventoryContract(unittest.TestCase):
    def test_sql_template_is_read_only_and_metadata_only(self):
        sql = Path(__file__).with_name("production-inventory.sql").read_text()
        sql = re.sub(r"--[^\n]*", "", sql)
        statements = [statement.strip().lower() for statement in sql.split(";") if statement.strip()]
        self.assertEqual(statements[:3], ["set session max_execution_time = 5000", "set session lock_wait_timeout = 5", "start transaction read only"])
        self.assertEqual(statements[-1], "rollback")
        allowed = {"information_schema.tables", "information_schema.columns", "information_schema.statistics", "schema_migrations"}
        for statement in statements[3:-1]:
            self.assertTrue(statement.startswith("select "), "non-read statement in inventory")
            self.assertTrue(set(re.findall(r"\b(?:from|join)\s+([a-z_.]+)", statement)) <= allowed, "business table read in metadata inventory")
        self.assertNotRegex(sql.lower(), r"\b(?:count|json_extract|json_table)\s*\(")

    def test_mysql_fixture_keeps_view_and_estimates(self):
        result = inventory.parse_mysql(MYSQL, "qs")
        self.assertEqual(result["migration_state"], [{"version": 95, "dirty": False}])
        self.assertEqual(result["tables"][1]["type"], "VIEW")
        self.assertIsNone(result["tables"][1]["estimated_rows"])
        self.assertNotIn(SECRET, json.dumps(result))

    def test_mysql_rejects_unexpected_text_duplicate_head_and_truncation(self):
        for raw in (MYSQL + "payload\t" + SECRET + "\n", MYSQL + "migration\t94\t0\n", MYSQL.replace("migration\t95\t0\n", ""), MYSQL.replace("server\tqs", "server\twrong")):
            with self.subTest(raw=raw[:15]), self.assertRaises(inventory.InventoryError) as caught:
                inventory.parse_mysql(raw, "qs")
            self.assertNotIn(SECRET, str(caught.exception))
        raw = MYSQL + "tables\tx\tBASE TABLE\tInnoDB\t0\t0\t0\n" * 1000
        with self.assertRaisesRegex(inventory.InventoryError, "limit_exceeded"):
            inventory.parse_mysql(raw, "qs")

    def test_mongo_rejects_extra_body_error_and_partial_head(self):
        fixture = mongo_fixture()
        self.assertEqual(inventory.parse_mongo(json.dumps(fixture), "qs")["namespaces"][0]["storage"]["estimated_documents"], 1)
        fixture["namespaces"][0]["body"] = SECRET
        with self.assertRaises(inventory.InventoryError) as caught:
            inventory.parse_mongo(json.dumps(fixture), "qs")
        self.assertNotIn(SECRET, str(caught.exception))
        for raw in (json.dumps({"error": SECRET}), SECRET, json.dumps(dict(mongo_fixture(), migration_state=[]))):
            with self.assertRaises(inventory.InventoryError) as caught:
                inventory.parse_mongo(raw, "qs")
            self.assertNotIn(SECRET, str(caught.exception))

    def test_mongo_nonzero_fixed_stage_tokens_only(self):
        stages = ("connect", "auth", "listCollections", "collStats", "listIndexes", "migration", "head")
        for stage in stages:
            for code in ("none", "0", "18", "2147483647"):
                token = "mongo_inventory_" + stage + "_code_" + code
                raw = json.dumps({"error": token})
                self.assertEqual(inventory.mongo_failure_category(raw), token)
                with patch.object(inventory, "capture", return_value=(1, raw)), self.assertRaises(inventory.InventoryError) as caught:
                    inventory.mongo_client_result(["fixture"], "qs")
                self.assertEqual(str(caught.exception), token)
        for raw in (SECRET, json.dumps({"error": SECRET}), json.dumps({"error": "mongo_inventory_auth_code_18", "body": SECRET}),
                    '{"error":"mongo_inventory_auth_code_18","error":"mongo_inventory_auth_code_13"}',
                    '{"error":"mongo_inventory_auth_code_18"} trailing', '{"error":"mongo_inventory_auth_code_01"}',
                    '{"error":"mongo_inventory_auth_code_-1"}', '{"error":"mongo_inventory_auth_code_2147483648"}',
                    '{"error":"mongo_inventory_auth_code_1.5"}', '{"error":"mongo_inventory_unknown_code_18"}',
                    json.dumps(mongo_fixture())):
            self.assertEqual(inventory.mongo_failure_category(raw), "mongo_client_exit_status_none")
            expected = "inconsistent_mongo_client_exit" if raw == json.dumps(mongo_fixture()) else "mongo_client_exit_status_1"
            with patch.object(inventory, "capture", return_value=(1, raw)), self.assertRaisesRegex(inventory.InventoryError, "^" + expected + "$"):
                inventory.mongo_client_result(["fixture"], "qs")
        for status in (0, 1, 125, 126, 127, 137, 255, -1, -9, -64):
            self.assertEqual(inventory.mongo_failure_category(SECRET, status), "mongo_client_exit_status_" + str(status))
        for status in (256, -65, True, "1", None):
            self.assertEqual(inventory.mongo_failure_category(SECRET, status), "mongo_client_exit_status_none")
        raw = '{"error":"mongo_inventory_auth_code_18"}'
        with patch.object(inventory, "capture", return_value=(0, raw)), self.assertRaises(inventory.InventoryError):
            inventory.mongo_client_result(["fixture"], "qs")
        with patch.object(inventory, "capture", return_value=(1, raw)), self.assertRaisesRegex(inventory.InventoryError, "^client_execution_failed$"):
            inventory.invoke(["fixture"])
        with self.assertRaisesRegex(inventory.InventoryError, "^mongo_inventory_auth_code_18$"):
            inventory.mongo_client_result([sys.executable, "-c", "import sys; print(sys.argv[1]); sys.exit(1)", raw], "qs")

    def test_mongo_partial_unauthorized_metadata_and_nonzero_job(self):
        for token, field in (("collStats_unauthorized_code_13", "storage"), ("listIndexes_unauthorized_code_13", "indexes")):
            fixture = mongo_fixture()
            fixture["metadata_complete"] = False
            fixture["namespaces"][0]["metadata_error"] = [token]
            fixture["namespaces"][0][field] = None
            raw = json.dumps(fixture)
            parsed = inventory.parse_mongo(raw, "qs")
            self.assertIsNone(parsed["namespaces"][0][field])
            self.assertEqual(parsed["migration_state"], [{"version": 36, "dirty": False}])
            with patch.object(inventory, "capture", return_value=(1, raw)):
                self.assertFalse(inventory.mongo_client_result(["fixture"], "qs")["metadata_complete"])
            for status in (0, 2, 137):
                with patch.object(inventory, "capture", return_value=(status, raw)), self.assertRaises(inventory.InventoryError):
                    inventory.mongo_client_result(["fixture"], "qs")
            env = dict(environment(), INVENTORY_DATABASE="mongodb")
            with patch.object(inventory, "invoke", return_value="network"), patch.object(inventory, "collect", return_value=parsed):
                result = inventory.run(env)
            self.assertFalse(result["complete"])
            self.assertFalse(result["databases"]["mongodb"]["metadata_complete"])
            output = io.StringIO()
            with patch.object(inventory, "run", return_value=result), contextlib.redirect_stdout(output):
                self.assertEqual(inventory.main(), 1)
            self.assertNotIn(SECRET, output.getvalue())

    def test_mongo_partial_consistency_errors_and_unknown_head_failclosed(self):
        fixture = mongo_fixture()
        fixture["metadata_complete"] = False
        fixture["namespaces"][0]["metadata_error"] = ["collStats_unauthorized_code_13"]
        fixture["namespaces"][0]["storage"] = None
        mutations = (lambda f: f.update(metadata_complete=True), lambda f: f["namespaces"][0].update(metadata_error=[]),
                     lambda f: f["namespaces"][0].update(metadata_error=[SECRET]), lambda f: f["namespaces"][0].update(metadata_error=["collStats_unauthorized_code_13", "collStats_unauthorized_code_13"]),
                     lambda f: f["namespaces"][0].update(indexes=None), lambda f: f["namespaces"][0].update(metadata_error=[{}]), lambda f: f["namespaces"][0].update(type="view"), lambda f: f.update(migration_state=[]), lambda f: f["migration_state"][0].update(dirty=None))
        for mutate in mutations:
            changed = json.loads(json.dumps(fixture)); mutate(changed)
            with self.assertRaises(inventory.InventoryError) as caught:
                inventory.parse_mongo(json.dumps(changed), "qs")
            self.assertNotIn(SECRET, str(caught.exception))
        with patch.object(inventory, "capture", return_value=(1, json.dumps(mongo_fixture()))), self.assertRaisesRegex(inventory.InventoryError, "inconsistent_mongo_client_exit"):
            inventory.mongo_client_result(["fixture"], "qs")
        with patch.object(inventory, "capture", return_value=(0, json.dumps(mongo_fixture()))):
            self.assertTrue(inventory.mongo_client_result(["fixture"], "qs")["metadata_complete"])

    def test_invalid_scope_and_system_database_rejected_before_docker(self):
        for changed in ({"INVENTORY_DATABASE": "redis"}, {"INVENTORY_SOURCE_SHA": SECRET}, {"MYSQL_DATABASE": "mysql"}, {"MONGODB_DBNAME": "admin"}, {"MONGODB_PORT": "70000"}):
            env = dict(environment(), **changed)
            with patch.object(inventory.subprocess, "Popen") as run, self.assertRaises(inventory.InventoryError):
                inventory.run(env)
            run.assert_not_called()

    def test_all_runs_only_mysql_and_mongo(self):
        with patch.object(inventory, "invoke", return_value="infra-network"), patch.object(inventory, "collect", return_value={}) as collect:
            result = inventory.run(environment())
        self.assertEqual(list(result["databases"]), ["mysql", "mongodb"])
        self.assertEqual([call.args[0] for call in collect.call_args_list], ["mysql", "mongodb"])
        self.assertEqual(result["source_sha"], "a" * 40)
        self.assertTrue(result["read_only"])

    def test_docker_args_never_contain_password_and_have_resource_limits(self):
        for kind in ("mysql", "mongodb"):
            calls = []
            private_files = []
            def invoke(args, **kwargs):
                calls.append((args, kwargs))
                if "run" in args:
                    config = Path(args[args.index("--env-file") + 1]) if kind == "mongodb" else Path(next(arg.split("source=", 1)[1].split(",target=", 1)[0] for arg in args if "target=/connection,readonly" in arg)) / "mysql.cnf"
                    self.assertEqual(stat.S_IMODE(config.parent.stat().st_mode), 0o700)
                    self.assertEqual(stat.S_IMODE(config.stat().st_mode), 0o600)
                    self.assertIn(SECRET, config.read_text())
                    private_files.append(config)
                return MYSQL if kind == "mysql" and "run" in args else json.dumps(mongo_fixture()) if "run" in args else "image-fixture"
            with patch.object(inventory, "invoke", side_effect=invoke), patch.object(inventory, "mongo_client_result", side_effect=lambda args, db: inventory.parse_mongo(invoke(args, timeout=75), db)), patch.object(inventory, "capture", side_effect=[(1, ""), (0, "b" * 64 + "\n"), (0, ""), (0, "")]) as capture:
                inventory.collect(kind, environment(), ["docker"], Path(__file__).parent, "123-1")
            args, options = next(call for call in calls if "run" in call[0])
            self.assertNotIn(SECRET, " ".join(args))
            self.assertNotIn(SECRET, options.get("input_text", ""))
            for flag in ("--read-only", "--pull=never", "--cpus=0.5", "--memory=512m", "--pids-limit=64", "--cap-drop=ALL"):
                self.assertIn(flag, args)
            self.assertEqual(options["timeout"], 75)
            self.assertIn("--defaults-file=/connection/mysql.cnf" if kind == "mysql" else "--env-file", args)
            self.assertFalse(any(file.exists() for file in private_files))
            self.assertEqual(capture.call_args_list[2].args[0], ["docker", "rm", "--force", "b" * 64])

    def test_name_collision_does_not_remove_existing_container(self):
        with patch.object(inventory, "invoke", return_value="image"), patch.object(inventory, "capture", return_value=(0, "container")) as run:
            with self.assertRaisesRegex(inventory.InventoryError, "name_conflict"):
                inventory.collect("mysql", environment(), ["docker"], Path(__file__).parent, "123-1")
        self.assertEqual(run.call_count, 1)

    def test_failed_container_creation_never_removes_another_owner(self):
        def invoke(args, **kwargs):
            if "run" in args:
                raise inventory.InventoryError("client_execution_failed")
            return "image"
        with patch.object(inventory, "invoke", side_effect=invoke), patch.object(inventory, "capture", side_effect=[(1, ""), (0, "")]) as capture:
            with self.assertRaises(inventory.InventoryError):
                inventory.collect("mysql", environment(), ["docker"], Path(__file__).parent, "123-1")
        self.assertEqual(capture.call_count, 2)
        self.assertFalse(any("rm" in call.args[0] for call in capture.call_args_list))

    def test_cleanup_is_by_owned_id_and_failure_is_explicit(self):
        with patch.object(inventory, "capture", side_effect=[(0, "c" * 64 + "\n"), (1, ""), (0, "")]) as capture:
            inventory.cleanup_client(["docker"], "fixture-owner")
        self.assertEqual(capture.call_args_list[1].args[0], ["docker", "rm", "--force", "c" * 64])
        for result in ((1, ""), (0, "not-an-id"), (0, "c" * 64 + "\n" + "d" * 64)):
            with patch.object(inventory, "capture", return_value=result), self.assertRaisesRegex(inventory.InventoryError, "cleanup_unconfirmed"):
                inventory.cleanup_client(["docker"], "fixture-owner")

    def test_private_connection_files_special_chars_and_cleanup(self):
        env = dict(environment(), MYSQL_PASSWORD='space # dollar$ quote" slash\\ end')
        with inventory.connection_file("mysql", env, "qs", "3306") as (root, config):
            self.assertEqual(stat.S_IMODE(root.stat().st_mode), 0o700)
            self.assertEqual(stat.S_IMODE(config.stat().st_mode), 0o600)
            self.assertIn('password="space # dollar$ quote\\" slash\\\\ end"', config.read_text())
        self.assertFalse(root.exists())
        for password in ("evil\nMONGODB_HOST=attacker", "bad\rvalue", "bad\x00value"):
            with self.assertRaisesRegex(inventory.InventoryError, "control_character"):
                inventory.validate_binding(dict(environment(), MONGODB_PASSWORD=password), "mongodb")

    @unittest.skipUnless(shutil.which("my_print_defaults"), "optional local MySQL option parser unavailable")
    def test_mysql_quoted_credentials_roundtrip_with_real_option_parser(self):
        password = 'space # dollar$ quote" single\' slash\\ tab\t end'
        env = dict(environment(), MYSQL_PASSWORD=password)
        with inventory.connection_file("mysql", env, "qs", "3306") as (root, config):
            # MySQL 8.0 lacks --no-login-paths. Isolate its login-file lookup
            # without reading a developer/runner's real login configuration.
            login_file = root / "absent-fixture-login.cnf"
            self.assertFalse(login_file.exists())
            with patch.dict(os.environ, {"MYSQL_TEST_LOGIN_FILE": str(login_file)}):
                status, output = inventory.capture([shutil.which("my_print_defaults"), "--defaults-file=" + str(config), "--show", "client"])
        self.assertEqual(status, 0)
        self.assertTrue("--password=" + password in output.splitlines(), "MySQL option encoding roundtrip failed")

    def test_raw_client_error_timeout_and_unexpected_exception_not_printed(self):
        with self.assertRaises(inventory.InventoryError) as caught:
            inventory.invoke([sys.executable, "-c", "import sys; print(sys.argv[1], file=sys.stderr); sys.exit(1)", SECRET])
        self.assertNotIn(SECRET, str(caught.exception))
        with self.assertRaisesRegex(inventory.InventoryError, "timed_out"):
            inventory.invoke([sys.executable, "-c", "import time; time.sleep(2)"], timeout=0.05)
        output = io.StringIO()
        with patch.object(inventory, "run", side_effect=RuntimeError(SECRET)), contextlib.redirect_stdout(output):
            self.assertEqual(inventory.main(), 1)
        self.assertNotIn(SECRET, output.getvalue())
        self.assertIn("unexpected_inventory_failure", output.getvalue())

    def test_timeout_terminates_owned_client_descendants(self):
        with tempfile.TemporaryDirectory() as directory:
            pid_file = Path(directory) / "child.pid"
            fixture = "import pathlib,signal,subprocess,sys,time; child=subprocess.Popen([sys.executable,'-c','import time; time.sleep(10)']); pathlib.Path(sys.argv[1]).write_text(str(child.pid)); signal.signal(signal.SIGTERM,lambda *_: (child.wait(timeout=2),sys.exit(0))); time.sleep(10)"
            with self.assertRaisesRegex(inventory.InventoryError, "timed_out"):
                inventory.invoke([sys.executable, "-c", fixture, str(pid_file)], timeout=0.2)
            pid = int(pid_file.read_text())
            with self.assertRaises(ProcessLookupError):
                os.kill(pid, 0)

    def test_exited_client_leader_does_not_leave_owned_descendants(self):
        with patch.object(inventory.os, "killpg") as kill:
            self.assertEqual(inventory.capture([sys.executable, "-c", "pass"])[0], 0)
        self.assertEqual([call.args[1] for call in kill.call_args_list], [inventory.signal.SIGTERM, inventory.signal.SIGKILL])

    def test_exited_leader_actual_descendant_is_terminated(self):
        with tempfile.TemporaryDirectory() as directory:
            pid_file = Path(directory) / "orphan.pid"
            fixture = "import pathlib,subprocess,sys; child=subprocess.Popen([sys.executable,'-c','import time; time.sleep(10)'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL); pathlib.Path(sys.argv[1]).write_text(str(child.pid))"
            self.assertEqual(inventory.capture([sys.executable, "-c", fixture, str(pid_file)])[0], 0)
            pid = int(pid_file.read_text())
            stopped = False
            try:
                for _ in range(100):
                    try:
                        os.kill(pid, 0)
                    except ProcessLookupError:
                        stopped = True
                        break
                    stat_file = Path(f"/proc/{pid}/stat")
                    if stat_file.is_file() and stat_file.read_text().rsplit(")", 1)[1].strip().split()[0] == "Z":
                        stopped = True
                        break
                    time.sleep(0.01)
                self.assertTrue(stopped, "exited client leader left an active descendant")
            finally:
                if not stopped:
                    with contextlib.suppress(ProcessLookupError):
                        os.kill(pid, inventory.signal.SIGKILL)

    def test_stdout_and_stderr_are_bounded_while_reading(self):
        for descriptor in (1, 2):
            with self.subTest(stream=descriptor), self.assertRaisesRegex(inventory.InventoryError, "output_limit_exceeded"):
                inventory.invoke([sys.executable, "-c", f"import os; os.write({descriptor}, b'x' * (4 * 1024 * 1024 + 1))"])

    def test_partial_failure_is_not_reported_as_zero_or_complete(self):
        with patch.object(inventory, "invoke", return_value="network"), patch.object(inventory, "collect", side_effect=[{"database": "qs"}, inventory.InventoryError("client_execution_failed")]):
            result = inventory.run(environment())
        self.assertFalse(result["complete"])
        self.assertEqual(result["databases"]["mysql"]["database"], "qs")
        self.assertEqual(result["databases"]["mongodb"], {"error": "client_execution_failed"})


if __name__ == "__main__":
    unittest.main()
