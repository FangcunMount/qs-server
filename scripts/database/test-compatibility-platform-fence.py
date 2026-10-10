#!/usr/bin/env python3
"""Offline ordering/file/API protocol tests; no GitHub or full-fence proof."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from urllib.parse import parse_qs, urlsplit

PATH = Path(__file__).with_name("compatibility-platform-fence.py")
SPEC = importlib.util.spec_from_file_location("platform_fence", PATH)
M = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(M)


def scope():
    return {"format_version": 1, "kind": "approved_runner_workflow_quarantine_scope",
        "dispatcher_source_sha": "a" * 40, "tool_source_sha": "b" * 40, "original_source_sha": "c" * 40,
        "operation_id": "800-1", "original_run_id": "801-1", "manifest_sha256": "d" * 64,
        "repository_id": "200", "owner_id": "201", "actor_id": "202", "workflow_id": 10,
        "workflow_ids": [10, 20, 30], "job_name": "Mutation fixture", "runner_id": 203}


class API:
    def __init__(self, s):
        self.s = s
        self.calls = []
        self.closed = False
        self.mutation = None
        self.workflows = {10: {"id": 10, "path": M.WORKFLOW, "state": "active"},
            20: {"id": 20, "path": ".github/workflows/cd.yml", "state": "active"},
            30: {"id": 30, "path": ".github/workflows/historical.yml", "state": "disabled_inactivity"}}

    def call(self, method, path, expected=200):
        self.calls.append((method, path))
        p = urlsplit(path)
        base = "/repos/" + M.REPO
        if method == "PUT":
            wid = int(p.path.split("/")[-2])
            self.workflows[wid]["state"] = "disabled_manually" if p.path.endswith("/disable") else "active"
            if self.mutation == "lost_put_result":
                M.fail("platform_fence_remote_unknown")
            return None
        if p.path == base:
            return {"id": 200, "full_name": M.REPO, "owner": {"id": 201}}
        if p.path == base + "/commits/main":
            return {"sha": "f" * 40 if self.mutation == "old_main" else self.s["dispatcher_source_sha"]}
        if p.path == base + "/actions/runs/900/attempts/1":
            repo = {"id": 200, "full_name": M.REPO}
            return {"id": 900, "run_attempt": 2 if self.mutation == "old_attempt" else 1, "workflow_id": 10,
                "head_sha": self.s["dispatcher_source_sha"], "head_branch": "main", "event": "workflow_dispatch",
                "status": "in_progress", "path": M.WORKFLOW, "actor": {"id": 202}, "triggering_actor": {"id": 202},
                "repository": repo, "head_repository": repo}
        if p.path == base + "/actions/workflows":
            rows = list(self.workflows.values())
            if self.mutation == "extra_workflow":
                rows.append({"id": 40, "path": "other", "state": "active"})
            if self.mutation == "unknown_state":
                rows = copy.deepcopy(rows); rows[2]["state"] = "new_unknown_state"
            return {"total_count": len(rows), "workflows": copy.deepcopy(rows)}
        if p.path.startswith(base + "/actions/workflows/"):
            return copy.deepcopy(self.workflows[int(p.path.rsplit("/", 1)[1])])
        if p.path == base + "/actions/runs":
            status = parse_qs(p.query)["status"][0]
            rows = [{"id": 900, "run_attempt": 1, "workflow_id": 10, "status": "in_progress",
                "head_sha": self.s["dispatcher_source_sha"], "repository": {"id": 200, "full_name": M.REPO}}] if status == "in_progress" else []
            if self.mutation == status:
                rows.append({"id": 901, "run_attempt": 1, "workflow_id": 20, "status": status, "head_sha": "c" * 40,
                    "repository": {"id": 200, "full_name": M.REPO}})
            return {"total_count": len(rows), "workflow_runs": rows}
        if p.path == base + "/actions/runs/900/attempts/1/jobs":
            rows = [{"id": 910, "run_id": 900, "name": self.s["job_name"], "head_sha": self.s["dispatcher_source_sha"],
                "runner_id": 999 if self.mutation == "wrong_runner" else 203, "status": "in_progress"}]
            if self.mutation == "other_job":
                rows.append({**rows[0], "id": 911, "name": "other"})
            return {"total_count": len(rows), "jobs": rows}
        raise AssertionError("unexpected fixture endpoint")

    def close(self):
        self.closed = True


class WorkflowQuarantineTests(unittest.TestCase):
    def fixture(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        path = Path(temporary.name).resolve(); path.chmod(0o700)
        s = scope(); api = API(s); store = M._Store(path)
        lease = M.WorkflowQuarantine(api, store, s, "900-1")
        self.addCleanup(lease.close)
        return lease, api, path

    def test_install_and_explicit_restore_original_states(self):
        lease, api, path = self.fixture()
        lease.install()
        r = lease.check()
        self.assertFalse(r["whole_writer_fence_proven"])
        self.assertFalse(r["drop_ready"])
        self.assertEqual(api.workflows[20]["state"], "disabled_manually")
        self.assertEqual(api.workflows[30]["state"], "disabled_inactivity")
        lease.restore()
        self.assertEqual(api.workflows[20]["state"], "active")
        self.assertEqual(api.workflows[30]["state"], "disabled_inactivity")
        puts = [c for c in api.calls if c[0] == "PUT"]
        self.assertEqual([c[1].rsplit("/", 1)[1] for c in puts], ["disable", "enable"])
        self.assertTrue((path / "platform-restore.result.json").exists())

    def test_close_does_not_enable_or_remove_materials(self):
        lease, api, path = self.fixture(); lease.install()
        puts = [c for c in api.calls if c[0] == "PUT"]
        lease.close()
        self.assertEqual(api.workflows[20]["state"], "disabled_manually")
        self.assertEqual([c for c in api.calls if c[0] == "PUT"], puts)
        self.assertTrue((path / "platform-install.intent.json").exists())
        self.assertTrue(api.closed)

    def test_unknown_put_is_not_retried_or_restored(self):
        lease, api, path = self.fixture(); api.mutation = "lost_put_result"
        with self.assertRaises(M.Refused): lease.install()
        self.assertTrue((path / "workflow-20.disable.intent.json").exists())
        self.assertFalse((path / "workflow-20.disable.result.json").exists())
        for action in (lease.install, lease.check, lease.restore):
            with self.assertRaises(M.Refused): action()
        self.assertEqual(sum(c[0] == "PUT" for c in api.calls), 1)
        lease.close(); self.assertEqual(api.workflows[20]["state"], "disabled_manually")

    def test_existing_registration_never_adopted(self):
        lease, api, path = self.fixture(); lease.install(); lease.close()
        other = M.WorkflowQuarantine(api, M._Store(path), scope(), "900-1")
        self.addCleanup(other.close)
        puts = sum(c[0] == "PUT" for c in api.calls)
        with self.assertRaises(M.Refused): other.install()
        self.assertEqual(sum(c[0] == "PUT" for c in api.calls), puts)

    def test_other_runs_jobs_old_refs_and_coverage_block_before_put(self):
        for mutation in ("old_main", "old_attempt", "extra_workflow", "unknown_state", "queued", "waiting", "pending", "requested", "in_progress", "wrong_runner", "other_job"):
            with self.subTest(mutation=mutation):
                lease, api, _ = self.fixture(); api.mutation = mutation
                with self.assertRaises(M.Refused): lease.install()
                self.assertFalse(any(c[0] == "PUT" for c in api.calls))

    def test_concurrent_state_change_is_not_overwritten(self):
        lease, api, _ = self.fixture(); lease.install()
        api.workflows[20]["state"] = "disabled_fork"
        puts = sum(c[0] == "PUT" for c in api.calls)
        with self.assertRaises(M.Refused): lease.restore()
        self.assertEqual(sum(c[0] == "PUT" for c in api.calls), puts)

    def test_held_registration_changed_blocks_enable(self):
        lease, api, path = self.fixture(); lease.install()
        target = path / "platform-install.intent.json"
        raw = target.read_bytes(); target.unlink(); target.write_bytes(raw); target.chmod(0o600)
        with self.assertRaises(M.Refused): lease.restore()
        self.assertFalse(any(c[1].endswith("/enable") for c in api.calls))

    def test_scope_rejects_extra_boolean_duplicate_and_bad_ids(self):
        for change in ({"drop_ready": True}, {"whole_writer_fence_proven": True}, {"workflow_ids": [10, 10, 30]}, {"workflow_id": True}, {"workflow_ids": [30, 20, 10]}):
            with self.subTest(change=change):
                with self.assertRaises(M.Refused): M.validate_scope({**scope(), **change})
        with self.assertRaises(M.Refused): M.decode(b'{"id":1,"id":1}')

    def test_real_store_modes_and_no_credentials_in_its_files(self):
        lease, _, path = self.fixture(); lease.install()
        for file in path.iterdir():
            self.assertEqual(file.stat().st_mode & 0o777, 0o600)
            self.assertNotIn(b"Authorization", file.read_bytes())
            self.assertNotIn(b"GITHUB_TOKEN", file.read_bytes())
        (path / "platform-install.result.json").chmod(0o644)
        with self.assertRaises(M.Refused): lease.check()

    def test_native_token_factory_rejects_endpoint_before_network(self):
        api = M._NativeAPI(b"private-fixture-token", 0)
        for method, path in (("POST", "/repos/" + M.REPO + "/actions/runs"), ("PUT", "/repos/" + M.REPO + "/actions/runs/10/cancel"), ("GET", "/repos/other/project/actions/workflows")):
            with self.subTest(method=method, path=path):
                with self.assertRaises(M.Refused): api.call(method, path)
        with self.assertRaises(M.Refused): api.call("GET", "/repos/" + M.REPO + "/commits/main")
        api.close(); self.assertFalse(any(api._token))


if __name__ == "__main__":
    unittest.main()
