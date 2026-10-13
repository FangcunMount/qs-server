#!/usr/bin/env python3
"""Fast temporal/classification contracts; synthetic rows are not live evidence."""
import contextlib
import copy
import io
import json
import re
import runpy
import unittest
from pathlib import Path
from unittest import mock


DIRECTORY = Path(__file__).resolve().parent
script = DIRECTORY / "audit-compatibility-observation.sh"
embedded = script.read_text().split("<<'PY_M6_OBSERVATION'\n", 1)[1].rsplit("\nPY_M6_OBSERVATION", 1)[0]
AUDIT = {"__name__": "raw_window_contract"}
exec(compile(embedded.rsplit("\ntry:\n    main()", 1)[0], str(script), "exec"), AUDIT)
# Reuse the independent producer-shaped native fixture, not the helper's own
# expected-combination generator. This suite exercises logical raw-sample rules.
NATIVE = runpy.run_path(str(DIRECTORY / "test-compatibility-prometheus.py"))
SOURCE, INSTANCES, FAMILIES = (NATIVE[name] for name in ("SOURCE", "INSTANCES", "FAMILIES"))
CADENCE = {key: 15.0 for key in INSTANCES}
TOKEN = "qs_auth_token_source_total"
CLAIM = "qs_auth_claim_source_total"
DEFINITION = "qs_model_definition_write_total"
INFO = "qs_auth_compatibility_observation_info"


def frame(end=30, cadence=None, points=None):
    cadence = CADENCE if cadence is None else cadence
    families = {name: [] for name in FAMILIES}
    up = []
    for key in INSTANCES:
        interval = cadence[key]
        stamps = points[key] if points is not None else [
            index * interval for index in range(-2, int(end // interval) + 1)]
        labels = dict(zip(("job", "instance"), key))
        up.append({"metric": {"__name__": "up", **labels}, "values": [[stamp, "1"] for stamp in stamps]})
        for line in NATIVE["metrics"](0, key).decode().splitlines():
            name, encoded, value = re.fullmatch(r"(\w+)\{(.*)\} (\d+)", line).groups()
            if name not in families:
                continue
            metric = {name: json.loads(value) for name, value in re.findall(
                r'(\w+)=("(?:[^"\\]|\\.)*")', encoded)}
            families[name].append({"metric": {"__name__": name, **metric},
                                   "values": [[stamp, value] for stamp in stamps]})
    return up, families


def child(families, metric, key=INSTANCES[0], **dimensions):
    rows = [item for item in families[metric] if
            (item["metric"].get("job"), item["metric"].get("instance")) == key
            and all(item["metric"].get(name) == value for name, value in dimensions.items())]
    if len(rows) != 1:
        raise AssertionError("fixture selector must identify exactly one fixed child")
    return rows[0]


def growth(row, after=0):
    count = 0
    for point in row["values"]:
        count += point[0] > after
        point[1] = str(count)


def verified(families, org="canonical", user="canonical", account="canonical"):
    for key in INSTANCES:
        growth(child(families, TOKEN, key, source="bearer", result="accepted"))
        for field, source in (("user_id", user), ("account_id", account), ("org_id", org)):
            growth(child(families, CLAIM, key, field=field, source=source))
        if key[0] == "qs-collection-server":
            growth(child(families, "qs_plan_entry_route_total", key, route="canonical", result="accepted"))
        else:
            growth(child(families, DEFINITION, key, input="canonical", result="accepted"))


def crop(rows, begin, end):
    result = copy.deepcopy(rows)
    for row in result:
        row["values"] = [point for point in row["values"] if begin <= point[0] <= end]
    return result


def section(up, families, begin, end):
    return crop(up, begin, end), {name: crop(rows, begin, end) for name, rows in families.items()}


def audit(up, families, end=30, state=None, close=False, cadence=None, **kwargs):
    state = {} if state is None else state
    AUDIT["audit_raw_chunk"](up, families, CADENCE if cadence is None else cadence,
                             0, end, SOURCE, state, close=close, **kwargs)
    return state


class RawWindowContracts(unittest.TestCase):
    def test_verified_normal_and_optional_missing_org(self):
        required = {(name, key) for name in ("bearer", "user_id", "account_id", "org_id") for key in INSTANCES}
        required.update(("task", key) for key in INSTANCES[1:])
        required.add(("definition", INSTANCES[0]))
        for org in ("canonical", "missing"):
            with self.subTest(org=org):
                up, families = frame()
                verified(families, org=org)
                self.assertEqual(audit(up, families)["traffic"], required)

    def test_missing_user_or_account_cannot_satisfy_canonical(self):
        for field in ("user_id", "account_id"):
            with self.subTest(field=field):
                up, families = frame()
                verified(families, **{"user" if field == "user_id" else "account": "missing"})
                traffic = audit(up, families)["traffic"]
                for key in INSTANCES:
                    self.assertNotIn((field, key), traffic)
                    self.assertIn(("bearer", key), traffic)

    def test_extra_org_is_active_without_optional_org_coverage(self):
        up, families = frame()
        verified(families, org="extra_org_id")
        traffic = audit(up, families)["traffic"]
        for key in INSTANCES:
            self.assertIn(("claims_extra", key), traffic)
            self.assertNotIn(("org_id", key), traffic)

    def test_first_post_t0_hit_uses_previous_baseline(self):
        points = {key: [-30.5, -15.5, 0.5, 15.5, 30.5] for key in INSTANCES}
        up, families = frame(end=30.5, points=points)
        row = child(families, TOKEN, source="bare", result="accepted")
        row["values"] = [[stamp, "0" if stamp < 0 else "1"] for stamp, _ in row["values"]]
        self.assertIn(("bare_authorization", INSTANCES[0]), audit(up, families, end=30.5)["traffic"])
        # The first post-T0 sample is already 1; an unextended range has no
        # observed delta. The native suite separately proves Prom increase=0.
        post = [float(value) for stamp, value in row["values"] if stamp > 0]
        self.assertEqual(post[-1] - post[0], 0)

    def test_pre_t0_only_canonical_growth_does_not_count(self):
        up, families = frame()
        row = child(families, TOKEN, source="bearer", result="accepted")
        row["values"] = [[stamp, "0" if stamp < 0 else "1"] for stamp, _ in row["values"]]
        self.assertNotIn(("bearer", INSTANCES[0]), audit(up, families)["traffic"])

    def test_closing_captures_right_edge_legacy_and_excludes_post_e_canonical(self):
        up, families = frame(end=45)
        for source in ("bare", "bearer"):
            growth(child(families, TOKEN, source=source, result="accepted"), after=30)
        state = audit(up, families)
        self.assertEqual(state["traffic"], set())
        closing_up, closing_families = section(up, families, 15, 45)
        audit(closing_up, closing_families, state=state, close=True)
        self.assertIn(("bare_authorization", INSTANCES[0]), state["traffic"])
        self.assertNotIn(("bearer", INSTANCES[0]), state["traffic"])
        self.assertIn(("bare_authorization", INSTANCES[0]), state["boundary"])

    def test_closing_bracket_cannot_be_absent(self):
        up, families = frame()
        state = audit(up, families)
        closing_up, closing_families = section(up, families, 15, 30)
        with self.assertRaisesRegex(ValueError, "post-end scrape closing bracket"):
            audit(closing_up, closing_families, state=state, close=True)

    def test_fractional_millisecond_is_inclusive_for_raw_and_closing(self):
        self.assertEqual(AUDIT["raw_points"]({"values": [[1.001, "1"]]}, 1.001, 1.001), [(1001, 1)])
        cadence = {key: 1.0 for key in INSTANCES}
        points = {key: [-2, -1, 0, 1.001] for key in INSTANCES}
        up, families = frame(end=0.5, cadence=cadence, points=points)
        growth(child(families, TOKEN, source="bare", result="accepted"), after=0.5)
        state = audit(up, families, end=0.5, cadence=cadence)
        self.assertEqual(state["traffic"], set())
        closing_up, closing_families = section(up, families, 0, 1.001)
        audit(closing_up, closing_families, end=0.5, cadence=cadence, state=state, close=True)
        self.assertIn(("bare_authorization", INSTANCES[0]), state["traffic"])

    def test_closing_requires_every_source_counter_child(self):
        up, families = frame(end=45)
        state = audit(up, families)
        closing_up, closing_families = section(up, families, 15, 45)
        row = child(closing_families, TOKEN, source="bare", result="accepted")
        row["values"].pop()
        with self.assertRaisesRegex(ValueError, "actual up scrape timestamp"):
            audit(closing_up, closing_families, state=state, close=True)

    def test_one_actual_up_or_counter_sample_missing(self):
        for kind in ("up", "counter"):
            with self.subTest(kind=kind):
                up, families = frame()
                row = up[0] if kind == "up" else child(families, TOKEN, source="bare", result="accepted")
                row["values"] = [point for point in row["values"] if point[0] != 15]
                with self.assertRaisesRegex(ValueError, "cadence slot|actual up scrape timestamp"):
                    audit(up, families)

    def test_last_expected_up_slot_missing(self):
        up, families = frame()
        up[0]["values"].pop()
        with self.assertRaisesRegex(ValueError, "last expected scrape slot"):
            audit(up, families, end=32)

    def test_source_swap_unknown_combo_and_unknown_instance_fail(self):
        for kind in ("source", "combo", "instance"):
            with self.subTest(kind=kind):
                up, families = frame()
                if kind == "source":
                    row = child(families, INFO)
                    wrong = copy.deepcopy(row)
                    wrong["metric"]["source_sha"] = "f" * 40
                    wrong["values"] = [point for point in wrong["values"] if point[0] == 15]
                    families[INFO].append(wrong)
                else:
                    row = child(families, TOKEN, source="bare", result="accepted")
                    row["metric"]["source" if kind == "combo" else "instance"] = "unknown"
                with self.assertRaisesRegex(ValueError, "fixed label combination"):
                    audit(up, families)

    def test_reset_and_nonfinite_counter_fail(self):
        for kind in ("reset", "nan"):
            with self.subTest(kind=kind):
                up, families = frame()
                row = child(families, TOKEN, source="bare", result="accepted")
                row["values"][2][1] = "2" if kind == "reset" else "NaN"
                with self.assertRaisesRegex(ValueError, "reset|non-finite"):
                    audit(up, families)

    def test_overlap_seam_matches_single_window(self):
        up, families = frame(end=90)
        growth(child(families, TOKEN, source="bare", result="accepted"), after=45)
        verified(families, org="missing")
        single = audit(up, families, end=90)
        first_up, first_families = section(up, families, -30, 45)
        state = audit(first_up, first_families, end=45, window_end=90)
        second_up, second_families = section(up, families, 30, 90)
        audit(second_up, second_families, end=90, state=state)
        self.assertEqual(state["traffic"], single["traffic"])
        self.assertIn(("bare_authorization", INSTANCES[0]), state["traffic"])

    def test_overlap_value_conflict_and_seam_reset_fail(self):
        for kind in ("conflict", "reset"):
            with self.subTest(kind=kind):
                up, families = frame(end=90)
                row = child(families, TOKEN, source="bare", result="accepted")
                row["values"] = [[stamp, "2"] for stamp, _ in row["values"]]
                first_up, first_families = section(up, families, -30, 45)
                state = audit(first_up, first_families, end=45, window_end=90)
                second_up, second_families = section(up, families, 30, 90)
                row = child(second_families, TOKEN, source="bare", result="accepted")
                stamp = 30 if kind == "conflict" else 60
                for point in row["values"]:
                    if point[0] == stamp:
                        point[1] = "1"
                with self.assertRaisesRegex(ValueError, "overlap counter sample changed|counter reset"):
                    audit(second_up, second_families, end=90, state=state)

    def test_complete_extra_label_identity_cannot_change_at_seam(self):
        for kind in ("up", "counter"):
            with self.subTest(kind=kind):
                up, families = frame(end=90)
                row = up[0] if kind == "up" else child(families, TOKEN, source="bare", result="accepted")
                row["metric"]["deployment"] = "blue"
                first_up, first_families = section(up, families, -30, 45)
                state = audit(first_up, first_families, end=45, window_end=90)
                second_up, second_families = section(up, families, 30, 90)
                row = second_up[0] if kind == "up" else child(second_families, TOKEN, source="bare", result="accepted")
                row["metric"]["deployment"] = "green"
                with self.assertRaisesRegex(ValueError, "complete raw (up |series )label set changed"):
                    audit(second_up, second_families, end=90, state=state)

    def test_mixed_15_60_intervals(self):
        cadence = {key: 15.0 if key == INSTANCES[0] else 60.0 for key in INSTANCES}
        up, families = frame(end=120, cadence=cadence)
        self.assertEqual(audit(up, families, end=120, cadence=cadence)["traffic"], set())

    def test_bounded_jitter_and_fractional_baseline(self):
        for stamps, end in (([-31, -14, -1, 16, 29, 46, 59, 76, 89], 90),
                            ([-30.5, -15.5, 0.5], 0.5)):
            with self.subTest(stamps=stamps):
                up, families = frame(points={key: stamps for key in INSTANCES}, end=end)
                audit(up, families, end=end)

    def test_cadence_drift_and_old_15_now_60_fail(self):
        for kind in ("drift", "changed"):
            with self.subTest(kind=kind):
                cadence = CADENCE if kind == "drift" else {key: 60.0 for key in INSTANCES}
                stamps = [-30, -15, 0, 16, 32, 48, 64] if kind == "drift" else [-30, -15, 0, 15, 30]
                up, families = frame(points={key: stamps for key in INSTANCES}, end=64)
                with self.assertRaisesRegex(ValueError, "bounded phase contract|cadence slot"):
                    audit(up, families, end=64, cadence=cadence)

    def test_unreadable_definition_is_unknown_active_not_derived(self):
        for metric, dimensions in ((DEFINITION, {"input": "unreadable", "result": "accepted"}),
                                   ("qs_model_definition_input_field_total", {"field": "DecisionSpec", "presence": "unreadable", "result": "rejected"})):
            with self.subTest(metric=metric):
                up, families = frame()
                growth(child(families, metric, **dimensions))
                traffic = audit(up, families)["traffic"]
                self.assertIn(("definition_unreadable", INSTANCES[0]), traffic)
                self.assertNotIn(("definition_derived_input", INSTANCES[0]), traffic)
                self.assertNotIn(("definition", INSTANCES[0]), traffic)

    def test_main_classifies_legacy_and_blocks_unknown_or_incomplete_traffic(self):
        started = "2019-12-31T23:55:00Z"
        t0 = int(AUDIT["datetime"].datetime.fromisoformat("2020-01-01T00:00:00+00:00").timestamp())
        now = t0 + 14 * 86400 + 60
        last_scrape = AUDIT["datetime"].datetime.fromtimestamp(now - 1, AUDIT["datetime"].timezone.utc).isoformat()
        names = ("qs-apiserver", "qs-collection-server-1", "qs-collection-server-2")
        environment = {"OBSERVATION_MODE": "window", "EXPECTED_SOURCE_SHA": SOURCE,
                       "WINDOW_DAYS": "14", "OBSERVATION_STARTED_AT": "2020-01-01T00:00:00Z",
                       "EXPECTED_APISERVER_CONTAINERS": names[0],
                       "EXPECTED_COLLECTION_CONTAINERS": ",".join(names[1:])}
        _, healthy = frame()
        targets = [{"labels": dict(zip(("job", "instance"), key)), "health": "up",
                    "scrapeUrl": "http://" + key[1] + "/metrics", "scrapeInterval": "1m",
                    "scrapeTimeout": "10s", "lastScrape": last_scrape} for key in INSTANCES]

        def fake_api(endpoint, parameters=None):
            if endpoint == "status/flags":
                return {"storage.tsdb.retention.time": "400d"}
            self.assertEqual(endpoint, "targets")
            return {"activeTargets": targets}

        def fake_docker(*arguments):
            if arguments[0] == "ps":
                return names[0] if "label=prometheus.job=qs-apiserver" in arguments else "\n".join(names[1:])
            self.assertEqual(arguments[0], "inspect")
            index = names.index(arguments[1])
            job, instance = INSTANCES[index]
            return json.dumps({"running": True, "started": started,
                               "labels": {"prometheus.job": job, "prometheus.scrape": "true",
                                          "prometheus.metrics_port": "8080", "prometheus.metrics_path": "/metrics"},
                               "networks": {"proof": {"IPAddress": instance.split(":")[0]}}})

        def current_values(expression, expected):
            if expression.startswith("up{"):
                return {key: 1 for key in expected}
            name = next(name for name in FAMILIES if name in expression)
            return {key: sum((row["metric"]["job"], row["metric"]["instance"]) == key
                             for row in healthy[name]) for key in expected}

        def fake_query(expression, end=None):
            value = 4 if expression.startswith("count(") else 0
            return [{"metric": {}, "value": [now if end is None else end, str(value)]}]

        for case in ("known", "unknown", "user_id", "account_id", "org_id", "all_legacy"):
            with self.subTest(case=case):
                up, families = frame()
                verified(families, org="missing")
                if case == "unknown":
                    growth(child(families, DEFINITION, input="unreadable", result="rejected"))
                elif case in ("user_id", "account_id", "org_id"):
                    source = "missing" if case == "org_id" else "canonical"
                    row = child(families, CLAIM, INSTANCES[1], field=case, source=source)
                    row["values"] = [[stamp, "0"] for stamp, _ in row["values"]]
                elif case == "all_legacy":
                    growth(child(families, CLAIM, field="org_id", source="extra_org_id"))
                    growth(child(families, TOKEN, source="bare", result="rejected"))
                    growth(child(families, "qs_plan_entry_route_total", INSTANCES[1], route="legacy_token", result="rejected"))
                    growth(child(families, DEFINITION, input="derived", result="rejected"))
                    growth(child(families, "qs_model_definition_input_field_total", field="DecisionSpec", presence="present", result="rejected"))
                traffic = audit(up, families)["traffic"]
                output = io.StringIO()
                dependencies = {"api": fake_api, "docker": fake_docker, "current_values": current_values,
                                "query": fake_query, "check_raw_window": lambda *args: traffic}
                with mock.patch.dict(AUDIT, dependencies), mock.patch.dict(AUDIT["os"].environ, environment, clear=True), mock.patch.object(AUDIT["time"], "time", return_value=now), contextlib.redirect_stdout(output):
                    if case == "unknown":
                        with self.assertRaisesRegex(ValueError, "Definition unknown_input"):
                            AUDIT["main"]()
                    elif case in ("user_id", "account_id", "org_id"):
                        with self.assertRaisesRegex(ValueError, "required verified post-T0 traffic is insufficient"):
                            AUDIT["main"]()
                    else:
                        AUDIT["main"]()
                self.assertEqual(output.getvalue().count("zero_window_candidate"), 4 if case == "known" else 0)
                self.assertEqual(output.getvalue().count("active_compatibility"), 4 if case == "all_legacy" else 0)

    def test_explicit_query_time_and_later_closing_do_not_move_logical_end(self):
        up, families = frame(end=45)
        calls = []

        def fake_api(endpoint, parameters):
            self.assertEqual(endpoint, "query")
            calls.append(copy.deepcopy(parameters))
            self.assertIn("time", parameters)
            expression, end = parameters["query"], parameters["time"]
            if not expression.endswith("]"):
                return {"resultType": "vector", "result": []}
            duration = int(re.search(r"\[([0-9]+)s\]$", expression)[1])
            name = expression.split("{", 1)[0]
            rows = up if name == "up" else families[name]
            # Match the actual PromQL left-open, right-closed raw API interval.
            rows = copy.deepcopy(rows)
            for row in rows:
                row["values"] = [point for point in row["values"] if end - duration < point[0] <= end]
            return {"resultType": "matrix", "result": rows}

        selectors = {name: name + '{job=~"qs-apiserver|qs-collection-server"}' for name in FAMILIES}
        output = io.StringIO()
        with mock.patch.dict(AUDIT, {"api": fake_api}), mock.patch.object(AUDIT["time"], "time", return_value=45), contextlib.redirect_stdout(output):
            AUDIT["query"]("up", end=30)
            self.assertEqual(AUDIT["check_raw_window"](selectors, CADENCE, 0, 30, SOURCE), set())
        self.assertEqual({item["time"] for item in calls}, {30, 45})
        self.assertEqual(sum(item["time"] == 45 for item in calls), len(FAMILIES) + 1)
        self.assertIn("audited_end=30; closing_snapshot=45", output.getvalue())

    def test_31_day_scan_preserves_t0_and_bounds_the_closing_read(self):
        start, end = 17, 17 + 31 * 86400
        selectors = {name: name + '{job=~"qs-apiserver|qs-collection-server"}' for name in FAMILIES}
        for closing in (end + 15, end + 21601):
            with self.subTest(closing=closing):
                reads, chunks = [], []

                def fake_raw(selector, begin, bound):
                    reads.append((begin, bound))
                    return []

                def fake_audit(up, families, cadence, t0, bound, source, state, close=False, window_end=None):
                    chunks.append((t0, bound, close, window_end))
                    state.setdefault("traffic", set())
                    state.setdefault("boundary", set())

                with mock.patch.dict(AUDIT, {"raw_query": fake_raw, "audit_raw_chunk": fake_audit}), mock.patch.object(AUDIT["time"], "time", return_value=closing), contextlib.redirect_stdout(io.StringIO()):
                    if closing > end + 21600:
                        with self.assertRaisesRegex(ValueError, "bounded closing read"):
                            AUDIT["check_raw_window"](selectors, CADENCE, start, end, SOURCE)
                    else:
                        self.assertEqual(AUDIT["check_raw_window"](selectors, CADENCE, start, end, SOURCE), set())
                historical = [item for item in chunks if not item[2]]
                self.assertGreater(len(historical), 124)
                self.assertTrue(all(t0 == start and logical == end for t0, _, _, logical in chunks))
                self.assertLessEqual(reads[0][0], start - 2 * max(CADENCE.values()) - 1)
                self.assertEqual(historical[-1][1], end)
                self.assertTrue(all(0 < right[1] - left[1] <= 21600 for left, right in zip(historical, historical[1:])))
                if closing > end + 21600:
                    self.assertTrue(all(bound <= end for _, bound in reads))
                    self.assertFalse(any(item[2] for item in chunks))
                else:
                    self.assertEqual(chunks[-1], (start, end, True, end))
                    self.assertEqual(reads[-1], (end - 2 * max(CADENCE.values()), closing))


if __name__ == "__main__":
    unittest.main(verbosity=2)
