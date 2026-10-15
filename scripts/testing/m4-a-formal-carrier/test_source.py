#!/usr/bin/env python3
"""Pure tests of the actual qualifier, fixed source seam and failed cleanup."""
import datetime as dt
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("fixed_carrier", HERE / "run.py")
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)
spec = importlib.util.spec_from_file_location("fixed_approver", HERE / "approve.py")
approver = importlib.util.module_from_spec(spec)
spec.loader.exec_module(approver)


def valid():
    actions = []
    for action in runner.STEPS:
        actions.append({"action": action, "complete": True, "compiled_tool_source": runner.TOOL_SOURCE,
                        "a_execution_source": runner.A_SOURCE, "a_execution_resources_sha256": runner.RESOURCE,
                        "a_execution_resource_files": 280, "sql_connection_closed": True,
                        "sql_pool_closed": True, "mongo_closed": True, "error_category": "none",
                        "transaction_release_unproven": False, "total_budget_exceeded": False})
    row = {"kind": "M4_A_original_native_carrier", "tool_sha256": runner.TOOL_HASH, "tool_exact_build": True,
           "mysql_image_binding": {"registry_ref": runner.MYSQL,
                                   "image": {"Id": "sha256:" + "a" * 64, "RepoDigests": [runner.MYSQL],
                                             "Os": "linux", "Architecture": "amd64"},
                                   "container_image_id": "sha256:" + "a" * 64,
                                   "actual_server_version": "8.0.36"},
           "actions": actions, "material_counts": [247463, 247463], "material_bytes": 50_014_038,
           "production": False, "Mac_custody_executed": False, "business_runtime_started": False,
           "carriers": [{"action": a, "image_content_verified": True, "uid": "0:0", "memory": 536870912,
                         "native_bind_verified": True, "same_CID_exit": 0, "oom": False,
                         "release_proven": True} for a in runner.STEPS]}
    for flag in ("partial_dirty101_proven", "restored10039_witness", "target10139_witness",
                 "original_three_files_verified", "all_native_carriers_released", "owned_cleanup_proven", "source_unchanged"):
        row[flag] = True
    return row


class QualificationTests(unittest.TestCase):
    def test_full_actual_predicate_accepts_only_complete_fixed_scope(self):
        self.assertTrue(runner.qualify(valid()))

    def test_fixed_mysql_content_and_actual_container_identity_are_mandatory(self):
        mutations = {
            "missing_binding": lambda row: row.pop("mysql_image_binding"),
            "wrong_requested_ref": lambda row: row["mysql_image_binding"].update(registry_ref="mysql:8.0.36"),
            "missing_digest": lambda row: row["mysql_image_binding"]["image"].pop("RepoDigests"),
            "empty_digests": lambda row: row["mysql_image_binding"]["image"].update(RepoDigests=[]),
            "wrong_digest": lambda row: row["mysql_image_binding"]["image"].update(RepoDigests=["mysql@sha256:" + "b" * 64]),
            "foreign_repo": lambda row: row["mysql_image_binding"]["image"].update(RepoDigests=[runner.MYSQL.replace("library/mysql", "foreign/mysql")]),
            "wrong_os": lambda row: row["mysql_image_binding"]["image"].update(Os="windows"),
            "wrong_arch": lambda row: row["mysql_image_binding"]["image"].update(Architecture="arm64"),
            "missing_engine_id": lambda row: row["mysql_image_binding"]["image"].pop("Id"),
            "bad_engine_id": lambda row: row["mysql_image_binding"]["image"].update(Id="actual-unknown"),
            "missing_container_id": lambda row: row["mysql_image_binding"].pop("container_image_id"),
            "different_container_id": lambda row: row["mysql_image_binding"].update(container_image_id="sha256:" + "b" * 64),
            "wrong_version": lambda row: row["mysql_image_binding"].update(actual_server_version="8.0.37"),
        }
        for name, mutate in mutations.items():
            with self.subTest(name=name):
                row = valid(); mutate(row)
                with self.assertRaises(RuntimeError): runner.qualify(row)

    def test_mysql_registry_digest_is_not_an_assumed_engine_config_id(self):
        row = valid()
        row["mysql_image_binding"]["image"]["RepoDigests"] = [runner.MYSQL.removeprefix("docker.io/library/")]
        self.assertTrue(runner.qualify(row))
        self.assertNotEqual(row["mysql_image_binding"]["image"]["Id"], runner.MYSQL.split("@", 1)[1])

    def test_workflow_pulls_the_same_fixed_amd64_mysql_as_the_fixture(self):
        import shlex
        import yaml
        workflow = yaml.safe_load((HERE.parents[2] / ".github/workflows/ci.yml").read_text())
        step = next(item for item in workflow["jobs"]["m4-a-formal-carrier"]["steps"]
                    if item.get("name") == "Read only the fixed images without replacement")
        pulls = [shlex.split(line) for line in step["run"].splitlines() if line.strip().startswith("docker pull ")]
        selected = [args for args in pulls if args[-1] == runner.MYSQL]
        self.assertEqual(len(selected), 1)
        self.assertIn("--platform=linux/amd64", selected[0])

    def test_missing_duplicate_foreign_and_reordered_actual_actions_rejected(self):
        for mutate in (lambda r: r["actions"].pop(), lambda r: r["actions"].append(r["actions"][0]),
                       lambda r: r["actions"][0].update(action="foreign"), lambda r: r["actions"].reverse()):
            row = valid(); mutate(row)
            with self.assertRaises(RuntimeError): runner.qualify(row)

    def test_negative_unknown_release_and_source_cannot_be_qualified(self):
        for field, value in [("complete", False), ("complete", 1), ("sql_pool_closed", False),
                             ("mongo_closed", None), ("transaction_release_unproven", True),
                             ("total_budget_exceeded", True), ("error_category", "unknown"),
                             ("compiled_tool_source", "0" * 40), ("a_execution_resource_files", 279)]:
            with self.subTest(field=field):
                row = valid(); row["actions"][2][field] = value
                with self.assertRaises(RuntimeError): runner.qualify(row)

    def test_formal_carrier_uid_memory_bind_exit_and_oom_are_required(self):
        for field, value in [("uid", "501:501"), ("memory", 1073741824), ("native_bind_verified", False),
                             ("same_CID_exit", 1), ("oom", True), ("release_proven", False),
                             ("image_content_verified", False)]:
            with self.subTest(field=field):
                row = valid(); row["carriers"][0][field] = value
                with self.assertRaises(RuntimeError): runner.qualify(row)

    def test_material_scale_and_custody_flags_cannot_borrow_A14(self):
        for field, value in [("material_counts", [2, 2]), ("material_bytes", 8_000_000),
                             ("original_three_files_verified", False), ("Mac_custody_executed", True),
                             ("source_unchanged", False), ("tool_exact_build", False),
                             ("owned_cleanup_proven", False), ("production", True)]:
            with self.subTest(field=field):
                row = valid(); row[field] = value
                with self.assertRaises(RuntimeError): runner.qualify(row)

    def test_duplicate_JSON_refused(self):
        with self.assertRaisesRegex(RuntimeError, "duplicate"):
            runner.decode('{"complete":true,"complete":false}')

    def test_separate_approver_uses_original_contract_and_literal_UTC_Z(self):
        now = dt.datetime(2026, 10, 16, tzinfo=dt.timezone.utc)
        material = {"captured_at": "2026-10-15T23:59:59.123456789Z", "mysql_identity_sha256": "a" * 64,
                    "mongo_identity_sha256": "b" * 64}
        result = approver.approval_for(material, Path("/opt/qs-server/m4-cleanup-a/123-1"), "c" * 64, "d" * 64, now)
        for field in ("window_started_at", "recovery_decision_at", "restore_deadline", "expires_at"):
            self.assertTrue(result[field].endswith("Z"))
        self.assertEqual(result["data_owner"], "system (10001)")
        self.assertEqual(result["recovery_owner"], "system (10001)")
        self.assertEqual(result["compiled_tool_source"], runner.TOOL_SOURCE)
        self.assertEqual(result["first_new_runtime_marker"], "/opt/qs-server/m4-cleanup-a/123-1/first-runtime.json")
        self.assertEqual(dt.datetime.fromisoformat(result["restore_deadline"].replace("Z", "+00:00")) - now, dt.timedelta(hours=1))
        source = (HERE / "approve.py").read_text()
        self.assertNotIn("sql.Open", source)
        self.assertNotIn("subprocess.run", source)

    def test_original_receiver_bytes_and_methods_are_the_actual_producer(self):
        module = runner.load_remote()
        self.assertEqual(runner.digest((HERE / "remote-maintenance.py").read_bytes()), runner.REMOTE_HASH)
        import inspect
        body = inspect.getsource(runner.Fixture.action)
        self.assertIn("receiver.native(action, payload)", body)
        for method in ("native", "run_native_carrier", "engine_create", "run", "guard", "release_native_carriers"):
            self.assertNotIn("receiver." + method + " =", (HERE / "run.py").read_text())
            self.assertTrue(callable(getattr(module.Receiver, method)))

    def test_unknown_native_release_stays_negative_even_after_owned_force_cleanup(self):
        fixture = runner.Fixture.__new__(runner.Fixture)
        fixture.bound, fixture.result, fixture.created, fixture.network_created, fixture.volume_created = True, {}, {}, False, False
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        fixture.source_hashes, fixture.output = {}, Path(directory.name)
        receiver = mock.Mock()
        receiver.release_native_carriers.side_effect = RuntimeError("unknown")
        receiver.owned_cli, receiver.lock = {}, None
        receiver.root.iterdir.return_value = []
        fixture.receiver = receiver
        fixture.docker = mock.Mock(return_value=b"")
        fixture.cleanup()
        self.assertFalse(fixture.result["owned_cleanup_proven"])
        self.assertIn("RuntimeError:native_release_unproven", fixture.result["cleanup_errors"])

    def test_unknown_create_resolves_only_owned_name_and_foreign_identity_is_retained(self):
        fixture = runner.Fixture.__new__(runner.Fixture)
        fixture.bound, fixture.result, fixture.network_created, fixture.volume_created = True, {}, False, False
        fixture.created = {"owned-name": {"cid": None, "image": "image"}}
        fixture.receiver, fixture.owner, fixture.source_hashes = None, "owner", {}
        fixture.docker = mock.Mock(side_effect=[b"a" * 64 + b"\n", b""])
        fixture.inspect = mock.Mock(return_value={"Name": "/owned-name", "Config": {"Labels": {runner.LABEL: "foreign"}}})
        fixture.cleanup()
        self.assertFalse(fixture.result["owned_cleanup_proven"])
        self.assertFalse(any(call.args[:1] == ("rm",) for call in fixture.docker.call_args_list))

    def test_unbound_runner_cleanup_never_touches_Docker(self):
        fixture = runner.Fixture.__new__(runner.Fixture)
        fixture.bound, fixture.result = False, {}
        fixture.docker = mock.Mock(side_effect=AssertionError("no Docker before bind"))
        fixture.cleanup()
        fixture.docker.assert_not_called()

    def test_setup_and_seed_cannot_borrow_original_native_budget(self):
        fixture = runner.Fixture.__new__(runner.Fixture)
        fixture.began, fixture.cleanup_started, fixture.phase_deadline = 100, None, 340
        with mock.patch.object(runner.time, "monotonic", return_value=341):
            self.assertLess(fixture.remaining(), 0)
            fixture.phase_deadline = None
            self.assertGreater(fixture.remaining(), 0)
            fixture.phase_deadline = 520  # independent180s seed cap
            with mock.patch.object(runner.time, "monotonic", return_value=521):
                self.assertLess(fixture.remaining(), 0)

    def test_private_original_writer_never_overwrites_existing_or_fsync_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "original"
            runner.save(path, b"original")
            with self.assertRaises(FileExistsError): runner.save(path, b"substitution")
            self.assertEqual(path.read_bytes(), b"original")
            failed = Path(directory) / "partial"
            with mock.patch.object(runner.os, "fsync", side_effect=OSError("unknown")):
                with self.assertRaises(OSError): runner.save(failed, b"partial")
            self.assertEqual(failed.read_bytes(), b"partial")

    def test_workflow_opt_in_default_and_existing_jobs_are_isolated(self):
        # Parse as BaseLoader because YAML1.1 otherwise treats 'on' as boolean.
        import yaml
        workflow = yaml.load((HERE.parents[2] / ".github/workflows/ci.yml").read_text(), Loader=yaml.BaseLoader)
        input = workflow["on"]["workflow_dispatch"]["inputs"]["m4_a_formal_carrier"]
        self.assertEqual(input["default"], "false")
        job = workflow["jobs"]["m4-a-formal-carrier"]
        self.assertEqual(job["if"], "github.event_name == 'workflow_dispatch' && inputs.m4_a_formal_carrier == true && github.repository == 'FangcunMount/qs-server'")
        self.assertEqual(job["timeout-minutes"], "85")
        self.assertEqual(sum(int(step["timeout-minutes"]) for step in job["steps"]), 80)
        self.assertEqual(job["runs-on"], "ubuntu-latest")
        self.assertNotIn("m4-a-formal-carrier", json.dumps(workflow["jobs"]["test"]))
        self.assertEqual(workflow["permissions"], {"contents": "read"})


if __name__ == "__main__":
    unittest.main()
