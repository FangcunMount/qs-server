import copy
import json
import tempfile
import unittest

import assign_unassigned_testees as script


class AssignmentTests(unittest.TestCase):
    def setUp(self):
        self.targets = [{"id": str(i + 10), "org_id": "1", "name": name, "code": str(i)}
                        for i, name in enumerate(script.STORE_NAMES)]
        self.rows = [{"testee_id": str(636809251561419310 + i), "expected_version": 1} for i in range(101)]
        self.manifest = script.make_plan("https://example.test/api/v1", "1", self.targets, self.rows, 42, "approved")

    def test_balanced_random_assignment_and_precise_ids(self):
        rows = self.manifest["plan"]["rows"]
        self.assertEqual({r["testee_id"] for r in rows}, {r["testee_id"] for r in self.rows})
        counts = script.Counter(r["store_id"] for r in rows)
        self.assertLessEqual(max(counts.values()) - min(counts.values()), 1)
        other = script.make_plan("https://example.test/api/v1", "1", self.targets, self.rows, 42, "approved")
        self.assertEqual([(r["testee_id"], r["store_id"]) for r in rows],
                         [(r["testee_id"], r["store_id"]) for r in other["plan"]["rows"]])
        self.assertTrue(all(len(r["request_id"]) <= 64 for r in rows))

    def test_manifest_tamper_and_wrong_environment_rejected(self):
        script.validate_manifest(self.manifest, "https://example.test/api/v1", "1")
        for base, org in [("https://other.test/api/v1", "1"), ("https://example.test/api/v1", "2")]:
            with self.assertRaises(ValueError):
                script.validate_manifest(self.manifest, base, org)
        changed = copy.deepcopy(self.manifest)
        changed["plan"]["rows"][0]["store_id"] = "999"
        with self.assertRaises(ValueError):
            script.validate_manifest(changed, "https://example.test/api/v1", "1")

    def test_pagination_detects_concurrent_change_and_duplicates(self):
        class Fake:
            def __init__(self, second):
                self.second = second
            def request(self, method, path, query):
                return {"total": 2, "items": [{"id": "1"}]} if query["page"] == 1 else self.second
        for second in ({"total": 3, "items": [{"id": "2"}]}, {"total": 2, "items": [{"id": "1"}]}, {"total": 2, "items": []}):
            with self.assertRaises(ValueError):
                script.list_all(Fake(second), "/testees")

    def test_missing_disabled_or_duplicate_store_rejected(self):
        class Fake:
            def __init__(self, rows):
                self.rows = rows
            def request(self, *a, **kw):
                return {"items": self.rows, "total": len(self.rows)}
        rows = [{**r, "is_active": True} for r in self.targets]
        self.assertEqual(script.stores(Fake(rows), "1"), self.targets)
        for invalid in (rows[:-1], rows + [rows[0]], [{**r, "is_active": False} for r in rows]):
            with self.assertRaises(ValueError):
                script.stores(Fake(invalid), "1")

    def test_inventory_rejects_assigned_and_cross_company(self):
        class Fake:
            def __init__(self, row):
                self.row = row
            def request(self, *a, **kw):
                return {"items": [self.row], "total": 1}
        for org, store in [("2", None), ("1", "10")]:
            with self.assertRaises(ValueError):
                script.inventory(Fake({"id": "20", "org_id": org, "store_id": store, "store_version": 1}), "1")

    def test_retry_reuses_request_and_stops_on_conflict(self):
        plan = self.manifest["plan"]
        plan["rows"] = plan["rows"][:2]
        class Fake:
            def __init__(self):
                self.requests = []
                self.fail = False
            def request(self, method, path, payload):
                self.requests.append((method, path, payload))
                if self.fail:
                    raise RuntimeError("HTTP 409")
                return {"testee_id": path.split('/')[2], "to_store_id": payload["store_id"],
                        "from_store_id": None, "org_id": "1", "kind": "initial", "version": 2,
                        "request_id": payload["request_id"], "reason": payload["reason"]}
        api = Fake()
        with tempfile.TemporaryFile(mode="w+") as journal:
            script.apply(api, plan, journal, 0)
            script.apply(api, plan, journal, 0)
            self.assertEqual(api.requests[:2], api.requests[2:])
            self.assertTrue(all(r[0] == "PUT" and r[1].endswith("/store") for r in api.requests))
            api.fail = True
            with self.assertRaises(RuntimeError):
                script.apply(api, plan, journal, 0)
            self.assertEqual(len(api.requests), 5)
            journal.seek(0)
            self.assertEqual(len([json.loads(line) for line in journal]), 4)

    def test_unexpected_existing_assignment_receipt_rejected(self):
        row, plan = self.manifest["plan"]["rows"][0], self.manifest["plan"]
        with self.assertRaises(ValueError):
            script.validate_receipt({"kind": "unchanged"}, row, plan)


if __name__ == "__main__":
    unittest.main()
