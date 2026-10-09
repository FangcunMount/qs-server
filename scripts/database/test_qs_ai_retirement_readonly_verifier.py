"""Offline contracts and owned-loopback native verifier tests; no production mode."""
import ast
import asyncio
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import unittest
from dataclasses import asdict, replace
from types import SimpleNamespace
from unittest.mock import patch
from uuid import uuid4

PATH = Path(__file__).with_name("qs-ai-retirement-readonly-verifier.py")
spec = importlib.util.spec_from_file_location("qs_ai_retirement_full_verifier", PATH)
m = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = m
spec.loader.exec_module(m)


def raw(value):
    if value is None:
        return None
    if isinstance(value, bytes):
        return value
    if isinstance(value, (dict, list)):
        return m.canonical(value)
    return str(value).encode()


def row(**kwargs):
    return {k: raw(v) for k, v in kwargs.items()}


def history():
    ai = {n: [] for n in m.AI_SPECS}
    peer = {n: [] for n in m.PEER_SPECS}
    cid, sid, rid, eid = [str(uuid4()) for _ in range(4)]
    start = dict(request_id=cid, actor=dict(org_id="1", subject_id="subject"),
                 testee_id="2", assessment_ids=["3"], goal="private-secret-goal")
    prepared, items = m._start(start)
    writer = m.sha(m._go_json(prepared))
    request_hash = m.sha(m.canonical([start["actor"], "2", ["3"], start["goal"]] + items))
    event = dict(event_id=eid, request_id=cid, session_id=sid, actor=start["actor"], testee_id="2",
                 version=4, status="blocked", question_id="", question="", can_skip=False,
                 failure_code="admission_input_invalid", artifact_json="")
    projection = m._event_projection(event)
    peer["ai_bridge_commands"] = [row(command_id=cid, request_id=cid, kind="start", payload=start,
          payload_hash=writer, delivered=1, attempts=0, available_at="2026-10-08 00:00:00.000000")]
    peer["ai_bridge_requests"] = [row(request_id=cid, request_hash=writer, payload=start, session_id=sid,
          version=4, status="blocked", projection=projection, organization_id=1, subject_id="subject", testee_id=2)]
    peer["ai_bridge_request_assessments"] = [row(request_id=cid, assessment_id=3)]
    peer["ai_bridge_events"] = [row(event_id=eid, request_id=cid, version=4, payload_hash=m.sha(m._go_json(projection)))]
    ai["interpretation_sessions"] = [row(id=sid, org_id=1, owner_subject_id="subject", testee_id=2,
          assessment_ids=["3"], goal=start["goal"], status="blocked", version=4, active_run_id=rid,
          current_question_id=None, evidence_set_id=None, workflow_version="qs-published-snapshot-v1",
          failure_code="admission_input_invalid")]
    ai["external_requests"] = [row(request_id=cid, session_id=sid)]
    ai["interpretation_runs"] = [row(id=rid, session_id=sid, session_version=4, status="blocked")]
    ai["result_outbox"] = [row(event_id=eid, session_id=sid, version=4, payload=event, delivered=1,
          mq_owned=0, attempts=0)]
    ai["idempotency_requests"] = [row(scope_hash=m.sha(m.canonical(["qs-server", "external-start-v1"])),
          key=cid, request_hash=request_hash, response=dict(session_id=sid, run_id=rid, status="blocked", version=4))]
    sections = {n: dict(rows=len(peer[n]), source_bytes=100, source_sha256="a"*64)
                for n in ("ai_bridge_commands", "ai_messaging_legacy_commands")}
    scan = SimpleNamespace(rows=peer, sections=sections)
    return ai, peer, scan, copy.deepcopy(sections)


def originals(scan, expected):
    return m._originals(scan, expected)


class Offline(unittest.IsolatedAsyncioTestCase):
    def test_closed_actual_catalog(self):
        self.assertEqual(len(m.AI_SPECS), 53)
        self.assertEqual(len(m.PEER_SPECS), 14)
        for columns, keys in (*m.AI_SPECS.values(), *m.PEER_SPECS.values()):
            self.assertEqual(len(columns.split()), len(set(columns.split())))
            self.assertLessEqual(set(keys.split()), set(columns.split()))

    async def test_no_lifecycle_no_dml_no_imported_completion(self):
        tree = ast.parse(PATH.read_text())
        forbidden = {"create_async_engine", "commit", "rollback", "begin", "close_all", "send", "publish"}
        for n in ast.walk(tree):
            if isinstance(n, ast.Call):
                name = n.func.attr if isinstance(n.func, ast.Attribute) else getattr(n.func, "id", "")
                self.assertNotIn(name, forbidden)
        for value in (None, {}, SimpleNamespace(complete=True)):
            with self.subTest(kind=type(value).__name__), self.assertRaises(m.Rejected):
                await m._scan(None, value, "a"*64)
        with self.assertRaises(m.Rejected):
            m.Qualification(None, None, None, seal=object())

    def test_original_source_scope_comes_from_full_eof_not_eight(self):
        ai, peer, scan, approved = history()
        values = originals(scan, approved)
        self.assertEqual(len(values), 1)
        self.assertEqual(values[0].source_tables, ("ai_bridge_commands",))
        self.assertEqual(m._mq(ai, peer, values, None)["authenticated_messages"], 0)

    def test_approval_cannot_be_automatic_complete_flag(self):
        _, _, scan, approved = history()
        for invalid in ({"complete": True}, {**approved, "complete": True},
                        {k: {**v, "complete": True} for k, v in approved.items()}):
            with self.subTest(case=len(invalid)), self.assertRaises(m.Rejected):
                originals(scan, invalid)
        approved["ai_bridge_commands"]["rows"] = 9
        with self.assertRaisesRegex(m.Rejected, "approved_original_source_changed"):
            originals(scan, approved)

    def test_original_writer_hash_is_not_sql_json_storage_bytes(self):
        _, peer, scan, approved = history()
        stored = peer["ai_bridge_commands"][0]
        self.assertNotEqual(m.sha(stored["payload"]), stored["payload_hash"].decode())
        originals(scan, approved)
        stored["payload_hash"] = m.sha(stored["payload"]).encode()
        with self.assertRaisesRegex(m.Rejected, "original_writer_digest"):
            originals(scan, approved)

    def test_go_writer_html_and_unicode_golden(self):
        from reliable_messaging.wire import go_json
        value = {"evidence": [{"facts": [{"ref": "中文", "value": "<>&\u2028\u2029"}]}]}
        self.assertEqual(m._go_json(value), go_json(value))
        self.assertNotIn(b"<", m._go_json(value))

    def test_handoff_original_hash_and_attempt_floor(self):
        _, peer, scan, approved = history()
        src = peer["ai_bridge_commands"][0]
        peer["ai_messaging_legacy_commands"].append(row(command_id=m.text(src["command_id"]),
          request_id=m.text(src["request_id"]), source_kind="start",
          source_payload=m._go_json(m._start(m.json_value(src["payload"]))[0]),
          source_payload_hash=m.text(src["payload_hash"]), source_attempts=0,
          source_available_at="2026-10-08 00:00:00.000000", source_original_time="",
          messaging_body_sha256="b"*64, transferred_at="2026-10-08 00:00:01.000000"))
        scan.sections["ai_messaging_legacy_commands"]["rows"] = 1
        approved = copy.deepcopy(scan.sections)
        self.assertEqual(len(originals(scan, approved)), 1)
        peer["ai_messaging_legacy_commands"][0]["source_attempts"] = b"1"
        with self.assertRaisesRegex(m.Rejected, "original_duplicate_source_conflict"):
            originals(scan, approved)

    def test_delivered_does_not_replace_business_acceptance(self):
        ai, peer, scan, approved = history()
        original = originals(scan, approved)[0]
        session = m._session(ai["interpretation_sessions"][0])
        m._legacy_event_chain(ai, peer, original, session)
        peer["ai_bridge_events"].clear()
        with self.assertRaisesRegex(m.Rejected, "original_peer_business_acceptance_missing"):
            m._legacy_event_chain(ai, peer, original, session)

    async def test_refusal_versions_are_not_fabricated_contiguous_events(self):
        ai, peer, scan, approved = history()
        original = originals(scan, approved)[0]
        session = m._session(ai["interpretation_sessions"][0])
        self.assertEqual(len(m._legacy_event_chain(ai, peer, original, session)), 1)
        self.assertEqual(await m._typed_execution(None, ai, original, session), "known_terminal_without_artifact")

    def test_cross_org_and_orphans_are_not_join_filtered(self):
        ai, peer, _, _ = history()
        m._reverse(ai, peer)
        ai["participant_capacity_reservations"].append(row(run_id=m.text(ai["interpretation_runs"][0]["id"]),
          session_id=m.text(ai["interpretation_sessions"][0]["id"]), organization_id=99, subject_id="subject",
          assessment_ids=["3"]))
        with self.assertRaisesRegex(m.Rejected, "global_capacity_owner_conflict"):
            m._reverse(ai, peer)
        ai["participant_capacity_reservations"].clear()
        ai["execution_jobs"].append(row(run_id=str(uuid4()), session_id=str(uuid4())))
        with self.assertRaisesRegex(m.Rejected, "global_orphan_participant_fact"):
            m._reverse(ai, peer)

    def test_unrelated_unknown_provider_has_real_global_owner(self):
        for status in ("unknown", "dispatched"):
            ai, peer, scan, approved = history()
            unrelated, current, _, _ = history()
            # Actual separate org/request/session/run graph, never supplied IDs.
            start = m.json_value(current["ai_bridge_requests"][0]["payload"])
            start["actor"]["org_id"] = "9"
            start["actor"]["subject_id"] = "outside-subject"
            owner = unrelated["interpretation_sessions"][0]
            owner["org_id"], owner["owner_subject_id"] = b"9", b"outside-subject"
            request = current["ai_bridge_requests"][0]
            request["payload"] = m.canonical(start)
            request["request_hash"] = m.sha(m._go_json(m._start(start)[0])).encode()
            request["organization_id"], request["subject_id"] = b"9", b"outside-subject"
            for name in ("interpretation_sessions", "interpretation_runs", "external_requests"):
                ai[name].extend(unrelated[name])
            peer["ai_bridge_requests"].extend(current["ai_bridge_requests"])
            rid = m.text(unrelated["interpretation_runs"][0]["id"])
            ai["execution_jobs"].append(row(id=str(uuid4()), run_id=rid,
                session_id=m.text(unrelated["interpretation_sessions"][0]["id"]), status="pending"))
            ai["model_calls"].append(row(run_id=rid, status=status))
            result = m._reverse(ai, peer, originals(scan, approved))
            self.assertEqual(result, {"outside_retirement_provider_result_unknown": 1})
            self.assertNotIn(rid, json.dumps(result))
            self.assertNotIn("private-secret-goal", json.dumps(result))

    def test_original_unknown_provider_and_unbound_scope_remain_blocked(self):
        for status in ("unknown", "dispatched"):
            ai, peer, scan, approved = history()
            rid = m.text(ai["interpretation_runs"][0]["id"])
            ai["model_calls"].append(row(run_id=rid, status=status))
            with self.subTest(status=status), self.assertRaisesRegex(m.Rejected, "global_participant_provider_result_unknown"):
                m._reverse(ai, peer, originals(scan, approved))
            with self.assertRaisesRegex(m.Rejected, "original_source_scope_required"):
                m._reverse(ai, peer)

    def test_unknown_provider_orphan_cross_session_and_fake_owner_block(self):
        for fault in ("orphan", "cross_session", "owner", "duplicate_owner", "duplicate_run", "missing_job"):
            ai, peer, scan, approved = history()
            unrelated, current, _, _ = history()
            for name in ("interpretation_sessions", "interpretation_runs", "external_requests"):
                ai[name].extend(unrelated[name])
            peer["ai_bridge_requests"].extend(current["ai_bridge_requests"])
            rid = m.text(unrelated["interpretation_runs"][0]["id"])
            call = row(run_id=rid, status="unknown")
            if fault != "missing_job":
                ai["execution_jobs"].append(row(id=str(uuid4()), run_id=rid,
                    session_id=m.text(unrelated["interpretation_sessions"][0]["id"]), status="pending"))
            if fault == "orphan":
                call["run_id"] = str(uuid4()).encode()
            elif fault == "cross_session":
                call["session_id"] = ai["interpretation_sessions"][0]["id"]
            elif fault == "owner":
                ai["interpretation_sessions"][-1]["org_id"] = b"99"
            elif fault == "duplicate_owner":
                ai["interpretation_sessions"].append(copy.deepcopy(unrelated["interpretation_sessions"][0]))
            elif fault == "duplicate_run":
                ai["interpretation_runs"].append(copy.deepcopy(unrelated["interpretation_runs"][0]))
            ai["model_calls"].append(call)
            with self.subTest(fault=fault), self.assertRaises(m.Rejected):
                m._reverse(ai, peer, originals(scan, approved))

    async def test_unrelated_evaluation_unknown_keeps_real_typed_evidence_validation(self):
        from datetime import datetime, timezone, timedelta
        from dataclasses import fields
        from unittest.mock import AsyncMock
        from qs_ai.domain.evaluation.identity import EvidenceReleaseIdentity, FrozenContractRef
        from qs_ai.domain.evaluation.checkpoint import ExecutionCheckpoint
        from qs_ai.domain.evaluation.completion import GenerationCompletion
        from qs_ai.domain.evaluation.failure import ClassifiedFailure
        from qs_ai.infrastructure.persistence.mysql.evaluation_checkpoints import encode
        from qs_ai.infrastructure.qs_server.evaluation_policies import load_execution_policy, load_gate_policy
        policy, gate = load_execution_policy(), load_gate_policy()
        ai, peer, scan, approved = history()
        source = originals(scan, approved)
        rid = str(uuid4())
        refs = {f.name: FrozenContractRef(f.name, "v1", "sha256:" + "a"*64)
                for f in fields(EvidenceReleaseIdentity)}
        refs.update(execution_policy=FrozenContractRef(policy.policy_id, policy.version, policy.fingerprint),
                    gate_policy=gate.reference)
        release = EvidenceReleaseIdentity(**refs)
        slots = [{"case_id": "case:" + str(i), "ordinal": j}
                 for i in range(1, policy.generation_cases+1)
                 for j in range(1, policy.candidates_per_case+1)]
        creation = dict(schema_version="qs-ai-evaluation-run-creation/v1", run_id=rid,
            release=asdict(release), release_fingerprint=release.fingerprint(),
            audit=dict(organization_id=9, requested_by="user:9", request_reason="offline scope fixture",
                       created_at="2026-10-08T00:00:00+00:00"),
            execution_policy_json=policy.definition_json, gate_policy_json=gate.definition_json,
            slots=slots, status="blocked", transitions=[])
        ai["evaluation_runs"] = [row(run_id=rid, organization_id=9, requested_by="user:9",
            definition_json=creation, progress_json=dict(status="blocked"), execution_mode="serial_v1")]
        ai["evaluation_run_policies"] = [row(run_id=rid, definition_json=policy.definition_json,
                                              fingerprint=policy.fingerprint)]
        ai["evaluation_checkpoints"] = [row(run_id=rid, checkpoint_json=None, version=1)]
        at = datetime(2026,10,8,tzinfo=timezone.utc)
        cp = ExecutionCheckpoint("exec:1", "generation", slots[0]["case_id"], slots[0]["ordinal"], "",
            1, "owner:9", "inv:1", "dispatching", at, at+timedelta(minutes=1), at)
        failure = ClassifiedFailure("generation_execution", "result_unknown", "provider_result_unknown",
            False, True, "manual_acknowledgement", "Provider result unknown", ("execution:exec:1",))
        completion = GenerationCompletion("exec:1", slots[0]["case_id"], slots[0]["ordinal"], 1,
            "inv:1", "result_unknown", at, at+timedelta(seconds=1), 1, failure=failure)
        value = asdict(completion)
        value["started_at"], value["finished_at"] = completion.started_at.isoformat(), completion.finished_at.isoformat()
        value.pop("raw_output"); value.pop("normalized_output")
        ai["evaluation_dispatches"] = [row(run_id=rid, invocation_id="inv:1", execution_id="exec:1",
            kind="generation", case_id=slots[0]["case_id"], slot_ordinal=slots[0]["ordinal"],
            candidate_id="", checkpoint_json=encode(cp))]
        ai["evaluation_generation_completions"] = [row(run_id=rid, invocation_id="inv:1", execution_id="exec:1",
            case_id=slots[0]["case_id"], slot_ordinal=slots[0]["ordinal"], execution_ordinal=1,
            candidate_id=None, candidate_json=None, evidence_json=value, raw_output=b"", normalized_output=b"")]
        suite = SimpleNamespace(slots=lambda: [(x["case_id"],x["ordinal"]) for x in slots],
            generation_case_ids=list(range(policy.generation_cases)), repetitions=policy.candidates_per_case)
        # Only host asset reads are mocked. Real creation, frozen policies,
        # checkpoint, terminal_dispatches and project_slots stay active.
        with patch("qs_ai.infrastructure.persistence.mysql.evaluation_assets.stored_run_suite", AsyncMock(return_value=suite)), \
             patch("qs_ai.infrastructure.persistence.mysql.evaluation_contracts.semantic_contract", AsyncMock()):
            m._reverse(ai, peer, source)
            result = await m._evaluations(ai, None, source, peer)
            self.assertEqual(result["outside_retirement_provider_result_unknown"], 1)
            self.assertEqual(result["legitimate_pending_runs"], 1)
            ai["evaluation_dispatches"][0]["invocation_id"] = b"conflicting:invocation"
            with self.assertRaises(Exception):
                await m._evaluations(ai, None, source, peer)
            ai["evaluation_dispatches"][0]["invocation_id"] = b"inv:1"
            # A real original ID collision is not proof of unrelated ownership.
            collision = source[0].request_id
            ai["evaluation_runs"][0]["run_id"] = collision.encode()
            creation["run_id"] = collision
            ai["evaluation_runs"][0]["definition_json"] = m.canonical(creation)
            for name in ("evaluation_run_policies", "evaluation_checkpoints", "evaluation_dispatches",
                         "evaluation_generation_completions"):
                ai[name][0]["run_id"] = collision.encode()
            with self.assertRaisesRegex(m.Rejected, "evaluation_provider_result_unknown"):
                await m._evaluations(ai, None, source, peer)
            with self.assertRaisesRegex(m.Rejected, "original_source_scope_required"):
                await m._evaluations(ai, None)

    def test_original_body_unknown_fields_hash_and_owner_block(self):
        for key, change in (("kind", b"PARTICIPANT_RETRY"), ("command_id", str(uuid4()).encode()),
                            ("payload_hash", b"a"*64)):
            _, peer, scan, approved = history()
            peer["ai_bridge_commands"][0][key] = change
            with self.subTest(field=key), self.assertRaises(m.Rejected):
                originals(scan, approved)
        _, peer, scan, approved = history()
        value = m.json_value(peer["ai_bridge_commands"][0]["payload"])
        value["complete"] = True
        peer["ai_bridge_commands"][0]["payload"] = m.canonical(value)
        with self.assertRaisesRegex(m.Rejected, "unsupported_original_start_schema"):
            originals(scan, approved)

    async def test_provider_unknown_stays_open_even_after_cancel(self):
        ai, peer, scan, approved = history()
        original = originals(scan, approved)[0]
        session = m._session(ai["interpretation_sessions"][0])
        rid = m.text(ai["interpretation_runs"][0]["id"])
        ai["execution_jobs"].append(row(id=str(uuid4()), run_id=rid, session_id=session.id,
          status="done", attempt=1, answer=None, skipped=0, question_id=None))
        ai["model_calls"].append(row(run_id=rid, status="unknown"))
        with self.assertRaisesRegex(m.Rejected, "provider_result_unknown"):
            await m._typed_execution(None, ai, original, session)

    def test_reverse_eval_orphan_does_not_disappear(self):
        ai, peer, _, _ = history()
        ai["evaluation_response_receipts"].append(row(run_id=str(uuid4()), invocation_id="orphan"))
        with self.assertRaisesRegex(m.Rejected, "global_orphan_evaluation_fact"):
            m._reverse(ai, peer)

    def test_live_mq_requires_real_keys_and_sdk(self):
        ai, peer, scan, approved = history()
        ai["ai_messaging_outbox"].append({})
        with self.assertRaisesRegex(m.Rejected, "host_message_protection_keys_required"):
            m._mq(ai, peer, originals(scan, approved), None)

    def test_safe_opaque_receipt_contains_no_body_or_private_ids(self):
        _, _, scan, approved = history()
        values = originals(scan, approved)
        qualification = m.Qualification((), values, {"known_terminal_without_artifact": 1}, seal=m._SEAL)
        receipt = json.dumps(qualification.receipt())
        self.assertNotIn("private-secret-goal", receipt)
        self.assertNotIn(values[0].command_id, receipt)
        self.assertFalse(qualification.receipt()["drop_ready"])

    def test_json_duplicates_nonfinite_and_private_repr(self):
        for raw in (b'{"x":1,"x":2}', b'{"x":NaN}'):
            with self.assertRaises(m.Rejected):
                m.json_value(raw)
        self.assertNotIn("private", repr(m.ProtectionKeys({"secret": "abc"}, {"secret": "def"})).replace("host-owned message protection keys", ""))

    async def test_historical_artifact_rebuilt_from_original_modelcall_schema(self):
        from qs_ai.infrastructure.persistence.mysql import schema as tables
        sys.path.insert(0, str(Path(tables.__file__).parents[5]))
        from tests.test_artifact import case
        from qs_ai.application.execution.artifact import build_artifact
        from qs_ai.application.interpretation.ports import Claim
        from qs_ai.application.interpretation.preparation import prepare_explanation
        from qs_ai.infrastructure.qs_server.output import QSOutputParser
        from qs_ai.infrastructure.qs_server.prompts import load_prompt
        from qs_ai.infrastructure.persistence.model_call_codec import JSONModelCallCodec
        old_claim, old_evidence, old_generated = case()
        sid, eid, rid, jid, cid = [str(uuid4()) for _ in range(5)]
        evidence = replace(old_evidence, id=eid, session_id=sid)
        session = replace(old_claim.session, id=sid, evidence_set_id=eid, active_run_id=rid,
                          status="completed", version=7)
        package = load_prompt(old_generated.request.prepared.release.render_policy.template_id,
                              old_generated.request.prepared.release.render_policy.version)
        prepared = prepare_explanation(session, evidence, old_generated.request.prepared.release, package)
        generated = replace(old_generated, request=replace(old_generated.request, prepared=prepared))
        claim = Claim(jid, rid, session, 1, None, False, None)
        candidate = build_artifact(claim, evidence, generated, QSOutputParser.from_schema(generated.request.schema))
        codec = JSONModelCallCodec()
        ai = {n: [] for n in m.AI_SPECS}
        ai["evidence_sets"] = [row(id=eid, session_id=sid, fingerprint=evidence.fingerprint,
          items=[asdict(v) for v in evidence.items], schema_version="evidence-v1")]
        ai["interpretation_runs"] = [row(id=rid, session_id=sid, session_version=7, status="completed")]
        ai["execution_jobs"] = [row(id=jid, run_id=rid, session_id=sid, status="done", attempt=1,
                                    answer=None, skipped=0, question_id=None)]
        ai["model_calls"] = [row(run_id=rid, session_id=sid, status="response_received", fence_token=1,
           invocation_id=generated.response.invocation_id, request_json=codec.encode_request(generated.request),
           response_json=codec.encode_response(generated.response))]
        ai["interpretation_artifacts"] = [row(id=candidate.id, run_id=rid, session_id=sid, payload=asdict(candidate))]
        original = SimpleNamespace(start={"evidence": [asdict(v) for v in evidence.items]})
        self.assertEqual(await m._typed_execution(None, ai, original, session),
                         "historical_original_configuration_not_retained")
        payload = asdict(candidate); payload["prompt_fingerprint"] = "sha256:"+"0"*64
        ai["interpretation_artifacts"][0]["payload"] = m.canonical(payload)
        with self.assertRaisesRegex(m.Rejected, "historical_original_artifact_typed_output_conflict"):
            await m._typed_execution(None, ai, original, session)

    async def test_public_helper_errors_never_include_driver_payload(self):
        with patch.object(m, "_verify", side_effect=RuntimeError("private-secret-body")):
            with self.assertRaisesRegex(m.Rejected, "^readonly_qualification_source_rejected$"):
                await m.verify()
        with patch.object(m, "_recheck", side_effect=RuntimeError("private-secret-body")):
            with self.assertRaisesRegex(m.Rejected, "^readonly_recheck_source_rejected$"):
                await m.recheck()
        with patch.object(m, "TOTAL_SECONDS", 0), patch.object(m, "_verify", side_effect=asyncio.sleep):
            with self.assertRaises(m.Rejected):
                await m.verify(1)

    def test_retry_original_writer_hash_receipt_and_inherited_budget(self):
        from qs_ai.application.execution.retry import ParticipantRetry
        from qs_ai.application.governance.prompt_drafts import DraftScope
        ai, peer, _, _ = history()
        sid = m.text(ai["interpretation_sessions"][0]["id"])
        source = m.text(ai["interpretation_runs"][0]["id"])
        cid, rid = str(uuid4()), str(uuid4())
        command = ParticipantRetry(DraftScope(1, 7), sid, cid, source, 4, "original reason", True, 1, False)
        receipt = dict(session_id=sid, run_id=rid, status="queued", version=5)
        retry = row(organization_id=1, command_id=cid, session_id=sid, request_id=m.text(ai["external_requests"][0]["request_id"]),
                    source_run_id=source, run_id=rid, operator_user_id=7, expected_version=4,
                    reason="original reason", accepted_unknown_risk=0, frozen_request_json=None, receipt=receipt)
        ai["participant_retries"].append(retry)
        ai["idempotency_requests"].append(row(scope_hash=m.sha(m.canonical(["participant-retry-v1", 1])),
            key=cid, request_hash=m.sha(m.canonical(asdict(command))), response=receipt))
        self.assertEqual(m._retry_acceptance(ai, retry)[0], command)
        retry["reason"] = b"changed original reason"
        with self.assertRaisesRegex(m.Rejected, "original_retry_acceptance_conflict"):
            m._retry_acceptance(ai, retry)

    def test_empty_transport_cannot_hide_orphan_sequence_or_projection(self):
        for table, side in (("ai_messaging_aggregates", "peer"), ("ai_messaging_evaluation_states", "peer"),
                            ("ai_messaging_evaluation_sequences", "ai")):
            ai, peer, scan, approved = history()
            (ai if side == "ai" else peer)[table].append(row(run_id=str(uuid4())))
            with self.subTest(table=table), self.assertRaisesRegex(m.Rejected, "global_orphan_mq_auxiliary_fact"):
                m._mq(ai, peer, originals(scan, approved), None)

    def test_full_scan_owned_rows_have_exact_indexes(self):
        rows = m._SourceRows([row(run_id=str(n), kind=n % 2) for n in range(10001)])
        self.assertEqual(m.match(rows, run_id="10000"), [rows[-1]])
        self.assertEqual(len(m.match(rows, kind=1)), 5000)
        self.assertEqual(m.match(rows, kind=1, run_id="10000"), [])
        self.assertEqual(m.match(rows, kind=0, run_id="10000"), [rows[-1]])

    def test_legacy_command_pending_remains_open_and_boolean_is_strict(self):
        _, peer, scan, approved = history()
        peer["ai_bridge_commands"][0]["delivered"] = b"0"
        with self.assertRaisesRegex(m.Rejected, "original_legacy_command_delivery_responsibility_open"):
            originals(scan, approved)
        with self.assertRaisesRegex(m.Rejected, "invalid_source_boolean"):
            m.boolean(b"2")




def signed_history(*, receipt_code="admission_input_invalid", grpc_status=3):
    from jwcrypto import jwk
    from reliable_messaging.protected import TrustedSigner
    from qs_ai.contracts.workflow import messaging_pb2 as pb
    from qs_ai.contracts.workflow import workflow_pb2 as workflow
    from qs_ai.infrastructure.workflow_transport.messaging import prepare
    ai, peer, scan, approved = history()
    source = originals(scan, approved)[0]
    cid = source.command_id
    signer_ai = jwk.JWK.generate(kty="EC", crv="P-256", kid="ai-sign")
    signer_peer = jwk.JWK.generate(kty="EC", crv="P-256", kid="peer-sign")
    encrypt_ai = jwk.JWK.generate(kty="EC", crv="P-256", kid="ai-encrypt")
    encrypt_peer = jwk.JWK.generate(kty="EC", crv="P-256", kid="peer-encrypt")
    keys = m.ProtectionKeys({k.get("kid"): k for k in (encrypt_ai, encrypt_peer)},
                           {"ai-sign": TrustedSigner("qs-ai", signer_ai),
                            "peer-sign": TrustedSigner("qs-server", signer_peer)})
    def stage(kind, mid, body, side):
        prepared = prepare(kind, mid, cid, body, organization_id="1",
                           signing_key=signer_ai if side == "ai" else signer_peer,
                           recipient_key=encrypt_peer if side == "ai" else encrypt_ai,
                           correlation=cid if kind == pb.COMMAND_RECEIPT else "")
        r = row(producer=prepared.envelope.producer, destination=prepared.envelope.destination,
                message_id=mid, body_sha256=prepared.envelope.body_sha256, body=prepared.body,
                wire=prepared.wire, wire_sha256=m.sha(prepared.wire), topic=prepared.topic, kind=kind,
                organization_id=1, aggregate_key=cid, aggregate_sequence=4 if side == "ai" else 1,
                ordered=1 if kind == pb.START else 0, requires_receipt=0 if kind == pb.EVENT_ACKNOWLEDGEMENT else 1,
                stage="confirmed", attempts=1, available_at="2026-10-08 00:00:00.000000",
                created_at="2026-10-08 00:00:00.000000", published_at="2026-10-08 00:00:00.000000",
                confirmed_at="2026-10-08 00:00:00.000000", error_code="")
        (ai if side == "ai" else peer)["ai_messaging_outbox"].append(r)
        return r
    start = stage(pb.START, cid, pb.MessagingBody(start=workflow.StartCommand(**source.start)), "peer")
    receipt_id = str(uuid4())
    original_receipt = m.doc(ai["idempotency_requests"][0], "response")
    receipt = stage(pb.COMMAND_RECEIPT, receipt_id, pb.MessagingBody(command_receipt=pb.MessagingCommandReceipt(
            command_id=cid, command_body_sha256=m.text(start["body_sha256"]), decision=pb.REJECTED,
            code=receipt_code, grpc_status_code=grpc_status,
            workflow_receipt=workflow.Receipt(**original_receipt))), "ai")
    # SDK prepare requires receipt correlation equal the actual command id.
    prepared = prepare(pb.COMMAND_RECEIPT, receipt_id, cid,
                       pb.MessagingBody.FromString(receipt["body"]), organization_id="1",
                       signing_key=signer_ai, recipient_key=encrypt_peer, correlation=cid)
    receipt.update(wire=prepared.wire, wire_sha256=raw(m.sha(prepared.wire)))
    event_value = m.doc(ai["result_outbox"][0], "payload")
    event = stage(pb.INTERPRETATION_STATE, event_value["event_id"],
                  pb.MessagingBody(interpretation_state=workflow.StateEvent(**event_value)), "ai")
    ai["result_outbox"][0]["mq_owned"] = b"1"
    ai["ai_messaging_inbox"].append(row(producer="qs-server", message_id=cid, destination="qs-ai",
            body_sha256=m.text(start["body_sha256"]), body=start["body"], wire_sha256=m.text(start["wire_sha256"]),
            kind=pb.START, aggregate_key=cid, reservation_token=str(uuid4()), decision="rejected",
            receipt_id=receipt_id, received_at="2026-10-08 00:00:00.000000"))
    peer["ai_messaging_operations"].append(row(command_id=cid, kind=pb.START, body_sha256=m.text(start["body_sha256"]),
            organization_id=1, subject_id="subject", resource_id=cid, aggregate_key=cid, aggregate_sequence=1,
            decision="rejected", code="admission_input_invalid", receipt_id=receipt_id, receipt=receipt["body"],
            created_at="2026-10-08 00:00:00.000000", decided_at="2026-10-08 00:00:00.000000",
            retired=0, retirement_evidence=None, retired_at=None))
    peer["ai_messaging_aggregates"].append(row(aggregate_key=cid, next_sequence=2))
    for result in (receipt, event):
        ack_id = str(uuid4())
        stage(pb.EVENT_ACKNOWLEDGEMENT, ack_id, pb.MessagingBody(event_acknowledgement=pb.MessagingEventAcknowledgement(
                event_id=m.text(result["message_id"]), event_body_sha256=m.text(result["body_sha256"]),
                event_kind=m.number(result["kind"]), outcome=pb.MessagingEventAcknowledgement.STORED)), "peer")
        peer["ai_messaging_inbox"].append(row(producer="qs-ai", message_id=m.text(result["message_id"]),
                body_sha256=m.text(result["body_sha256"]), body=result["body"], wire_sha256=m.text(result["wire_sha256"]),
                kind=m.number(result["kind"]), aggregate_key=cid, ack_id=ack_id,
                received_at="2026-10-08 00:00:00.000000", outcome="stored"))
    return ai, peer, scan, approved, keys


class SignedMQ(unittest.TestCase):
    def test_real_signature_original_command_first_receipt_peer_ack_chain(self):
        ai, peer, scan, approved, keys = signed_history()
        result = m._mq(ai, peer, originals(scan, approved), keys)
        self.assertEqual(result["authenticated_messages"], 5)
        self.assertEqual(result["target_mq_responsibilities"], 5)

    def test_signature_physical_identity_and_cross_org_negatives(self):
        for mode in ("signature", "row-kind", "owner", "wire-hash"):
            ai, peer, scan, approved, keys = signed_history()
            src = peer["ai_messaging_outbox"][0]
            if mode == "signature":
                from jwcrypto import jwk
                from reliable_messaging.protected import TrustedSigner
                keys.trusted_signers["peer-sign"] = TrustedSigner("qs-server", jwk.JWK.generate(kty="EC", crv="P-256", kid="peer-sign"))
            elif mode == "row-kind":
                src["kind"] = b"2"
            elif mode == "owner":
                src["organization_id"] = b"99"
            else:
                src["wire_sha256"] = b"a"*64
            with self.subTest(case=mode), self.assertRaises(m.Rejected):
                m._mq(ai, peer, originals(scan, approved), keys)

    def test_receipt_ack_operation_orphans_and_target_pending_block(self):
        for table, dataset in (("ai_messaging_inbox", "ai"), ("ai_messaging_operations", "peer"),
                               ("ai_messaging_inbox", "peer")):
            ai, peer, scan, approved, keys = signed_history()
            (ai if dataset == "ai" else peer)[table].clear()
            with self.subTest(table=table, side=dataset), self.assertRaises(m.Rejected):
                m._mq(ai, peer, originals(scan, approved), keys)
        ai, peer, scan, approved, keys = signed_history()
        peer["ai_messaging_outbox"][0]["stage"] = b"awaiting_receipt"
        peer["ai_messaging_outbox"][0]["confirmed_at"] = None
        with self.assertRaisesRegex(m.Rejected, "target_mq_delivery_responsibility_open"):
            m._mq(ai, peer, originals(scan, approved), keys)

    def test_unknown_quarantine_not_declared_non_target(self):
        ai, peer, scan, approved, keys = signed_history()
        ai["ai_messaging_quarantine"].append(row(wire=b"private-quarantine-body",
            wire_sha256=m.sha(b"private-quarantine-body"), logical_producer=None, logical_message_id=None))
        with self.assertRaisesRegex(m.Rejected, "global_unresolved_mq_quarantine"):
            m._mq(ai, peer, originals(scan, approved), keys)

    def test_signed_first_workflow_receipt_cannot_replace_original_acceptance(self):
        ai, peer, scan, approved, keys = signed_history()
        ai["idempotency_requests"][0]["request_hash"] = b"0"*64
        with self.assertRaisesRegex(m.Rejected, "mq_workflow_original_acceptance_conflict"):
            m._mq(ai, peer, originals(scan, approved), keys)
        ai, peer, scan, approved, keys = signed_history()
        ai["idempotency_requests"][0]["response"] = m.canonical({"session_id": str(uuid4()),
          "run_id": str(uuid4()), "status": "blocked", "version": 4})
        with self.assertRaisesRegex(m.Rejected, "mq_workflow_original_acceptance_conflict"):
            m._mq(ai, peer, originals(scan, approved), keys)

    def test_ordering_and_aggregate_counter_gaps_block(self):
        for mode in ("ordered", "counter", "missing"):
            ai, peer, scan, approved, keys = signed_history()
            if mode == "ordered":
                peer["ai_messaging_outbox"][0]["ordered"] = b"0"
            elif mode == "counter":
                peer["ai_messaging_aggregates"][0]["next_sequence"] = b"3"
            else:
                peer["ai_messaging_aggregates"].clear()
            with self.subTest(mode=mode), self.assertRaises(m.Rejected):
                m._mq(ai, peer, originals(scan, approved), keys)

    def test_signed_refusal_retains_original_business_code_and_grpc_outcome(self):
        for code, status in (("private-secret-code", 3), ("admission_input_invalid", 9)):
            ai, peer, scan, approved, keys = signed_history(receipt_code=code, grpc_status=status)
            with self.assertRaisesRegex(m.Rejected, "mq_original_start_refusal_conflict"):
                m._mq(ai, peer, originals(scan, approved), keys)

    def test_crypto_scan_fixed_total_deadline(self):
        ai, peer, scan, approved, keys = signed_history()
        with self.assertRaisesRegex(m.Rejected, "full_qualification_deadline_exceeded"):
            m._mq(ai, peer, originals(scan, approved), keys, deadline=0)

    def test_other_domain_same_aggregate_identity_is_ambiguous(self):
        ai, peer, scan, approved, keys = signed_history()
        cid = m.text(peer["ai_bridge_requests"][0]["request_id"])
        ai["evaluation_runs"].append(row(run_id=cid, organization_id=1))
        with self.assertRaisesRegex(m.Rejected, "cross_domain_aggregate_identity_ambiguous"):
            m._mq(ai, peer, originals(scan, approved), keys)


@unittest.skipUnless(os.environ.get("QS_AI_RETIREMENT_VERIFIER_NATIVE") == "1", "owned native opt-in")
class Native(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        from sqlalchemy import text
        from sqlalchemy.engine import URL
        from sqlalchemy.ext.asyncio import create_async_engine
        from qs_ai.infrastructure.persistence.mysql.database import Base
        from qs_ai.infrastructure.persistence.mysql import schema
        self.names = ["qs_ai_full_verifier_"+uuid4().hex, "qs_peer_full_verifier_"+uuid4().hex]
        self.admin = None; self.engines = []
        try:
            private = Path("/private/tmp/qs-compatibility-retirement")
            manifest = json.loads((private/"owned-mysql.json").read_text())
            env = json.loads((private/"mysql-native.env.json").read_text())
            actual = json.loads(subprocess.run(["docker", "inspect", manifest["container_id"]],
                               capture_output=True, check=True, timeout=10).stdout)[0]
            if not (manifest["ready"] is True and str(manifest["loopback_port"]) == "34306"
                and env["MYSQL_HOST"] == "127.0.0.1" and str(env["MYSQL_PORT"]) == "34306"
                and actual["Id"] == manifest["container_id"]
                and actual["Name"].lstrip("/") == manifest["container_name"]
                and actual["Image"] == manifest["image_id"] and actual["Config"]["Labels"] == manifest["labels"]
                and sorted(v["Name"] for v in actual["Mounts"] if v["Type"] == "volume") == sorted(manifest["volumes"])
                and actual["NetworkSettings"]["Ports"].get("3306/tcp") == [{"HostIp": "127.0.0.1", "HostPort": "34306"}]):
                raise ValueError
            url = URL.create("mysql+asyncmy", username=env["MYSQL_USERNAME"], password=env["MYSQL_PASSWORD"],
                             host="127.0.0.1", port=34306)
            self.admin = create_async_engine(url, hide_parameters=True, echo=False)
            for name in self.names:
                async with self.admin.begin() as c:
                    await c.execute(text("CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"))
                self.engines.append(create_async_engine(url.set(database=name), hide_parameters=True,
                                                       echo=False, isolation_level="REPEATABLE READ"))
            ai_root = Path(schema.__file__).parents[5]
            async with self.engines[0].begin() as c:
                await c.run_sync(Base.metadata.create_all)
                # SQLAlchemy's standalone BIGINT PK default adds AUTO_INCREMENT;
                # the actual admission migrations use explicitly assigned org IDs.
                for name in ("participant_admission_locks", "evaluation_admission_locks"):
                    await c.execute(text("ALTER TABLE `"+name+"` MODIFY organization_id BIGINT UNSIGNED NOT NULL"))
                tree = ast.parse((ai_root/"migrations/versions/0037_workflow_messaging.py").read_text())
                ddl = next(ast.literal_eval(n.value) for n in tree.body if isinstance(n, ast.Assign)
                           and any(isinstance(t, ast.Name) and t.id == "DDL" for t in n.targets))
                for sql in ddl:
                    await c.execute(text(sql))
                tree = ast.parse((ai_root/"migrations/versions/0038_messaging_observations.py").read_text())
                for n in ast.walk(tree):
                    if isinstance(n, ast.Call) and isinstance(n.func, ast.Attribute) and n.func.attr == "execute" and isinstance(n.args[0], ast.Constant):
                        await c.execute(text(ast.literal_eval(n.args[0])))
                await c.execute(text("CREATE TABLE alembic_version(version_num VARCHAR(64) PRIMARY KEY) ENGINE=InnoDB"))
                await c.execute(text("INSERT INTO alembic_version VALUES (:head)"), {"head": m.AI_HEAD})
            migrations = PATH.parents[2]/"internal/pkg/migration/migrations/mysql"
            async with self.engines[1].begin() as c:
                for number in (72, 83, 91, 92, 93, 94, 95, 97):
                    file = next(migrations.glob(f"{number:06}_*.up.sql"))
                    content = "\n".join(line for line in file.read_text().splitlines() if not line.lstrip().startswith("--"))
                    for statement in content.split(";"):
                        if statement.strip():
                            await c.execute(text(statement))
                await c.execute(text("CREATE TABLE schema_migrations(version BIGINT PRIMARY KEY,dirty BOOL NOT NULL) ENGINE=InnoDB"))
                await c.execute(text("INSERT INTO schema_migrations VALUES (99,0)"))
            self.identities = []
            for engine in self.engines:
                async with engine.connect() as c:
                    bound = (await c.execute(text("SELECT CAST(@@server_uuid AS BINARY),CAST(DATABASE() AS BINARY)"))).one()
                    self.identities.append(m._scanner("ai", m.AI_HEAD)._identity(*bound))
        except Exception:
            await self.clean()
            raise AssertionError("owned_full_verifier_setup_failed") from None

    async def clean(self):
        from sqlalchemy import text
        for e in self.engines:
            await e.dispose()
        self.engines = []
        if self.admin:
            try:
                for name in self.names:
                    async with self.admin.begin() as c:
                        await c.execute(text("DROP DATABASE IF EXISTS `"+name+"`"))
            finally:
                await self.admin.dispose(); self.admin = None

    async def asyncTearDown(self):
        await self.clean()

    async def snapshots(self, readonly=True):
        from sqlalchemy import text
        from sqlalchemy.ext.asyncio import AsyncSession
        sessions = []
        for engine in self.engines:
            session = AsyncSession(engine)
            await session.connection(execution_options={"isolation_level": "REPEATABLE READ"})
            await session.execute(text("START TRANSACTION WITH CONSISTENT SNAPSHOT" + (", READ ONLY" if readonly else "")))
            sessions.append(session)
        return sessions

    async def end(self, sessions):
        for s in sessions:
            await s.rollback(); await s.close()

    async def bounds(self, sessions):
        return [await m.discover_full_bounds(s, side=side, source_sha="a"*40, identity_hash=identity, head=head)
                for s, side, identity, head in zip(sessions, ("ai", "peer"), self.identities, (m.AI_HEAD, "99"))]

    async def qualify(self, sessions, bounds, protection_keys=None):
        peer = await m._scan(sessions[1], bounds[1], bounds[1].digest())
        expected = {n: {k: peer.sections[n][k] for k in ("rows", "source_bytes", "source_sha256")}
                    for n in ("ai_bridge_commands", "ai_messaging_legacy_commands")}
        # Native fixture explicitly approves the captured source descriptor; production
        # requires the separate human/host approval path and never calls this helper.
        return await m.verify(*sessions, ai_bounds=bounds[0], peer_bounds=bounds[1],
                              approved_ai_bounds_sha256=bounds[0].digest(), approved_peer_bounds_sha256=bounds[1].digest(),
                              approved_original_sections=expected, protection_keys=protection_keys)

    async def store_raw_fixture(self, ai, peer):
        from sqlalchemy import text
        for engine, dataset in zip(self.engines, (ai, peer)):
            async with engine.begin() as c:
                first = ["ai_bridge_requests"] if dataset is peer else ["interpretation_sessions", "interpretation_runs", "external_requests", "evidence_sets"]
                order = first + [n for n in dataset if n not in first]
                for table in order:
                    metadata = (await c.execute(text("SELECT COLUMN_NAME,DATA_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA FROM information_schema.columns WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=:t"), {"t": table})).all()
                    for original in dataset[table]:
                        values = dict(original)
                        for name, kind, nullable, default, extra in metadata:
                            if kind == "json" and values.get(name) is not None:
                                values[name] = values[name].decode("utf-8")
                            if name not in values and nullable == "NO" and default is None and "auto_increment" not in extra and kind in ("datetime", "timestamp"):
                                values[name] = b"2026-10-08 00:00:00.000000"
                        columns = list(values)
                        await c.execute(text("INSERT INTO `"+table+"` ("+",".join("`"+k+"`" for k in columns)+") VALUES ("+",".join(":"+k for k in columns)+")"), values)

    async def test_native_all_53_and_peer14_tables_two_passes_and_fresh_hashes(self):
        sessions = await self.snapshots()
        try:
            bounds = await self.bounds(sessions)
            q = await self.qualify(sessions, bounds)
            self.assertEqual(len(q._scans[0].sections), 53)
            self.assertEqual(len(q._scans[1].sections), 14)
            self.assertTrue(all(s.get_transaction().is_active for s in sessions))
            self.assertFalse(q.receipt()["drop_ready"])
            with self.assertRaisesRegex(m.Rejected, "original_snapshots_must_end"):
                await m.recheck(*sessions, q)
        finally:
            await self.end(sessions)
        fresh = await self.snapshots()
        try:
            self.assertTrue((await m.recheck(*fresh, q))["unchanged_full_source_hashes"])
        finally:
            await self.end(fresh)

    async def test_native_actual_read_write_transaction_rejected(self):
        sessions = await self.snapshots(readonly=False)
        try:
            with self.assertRaisesRegex(m.Rejected, "actual_readonly_snapshot_not_observed"):
                await self.bounds(sessions)
            self.assertTrue(all(s.get_transaction().is_active for s in sessions))
        finally:
            await self.end(sessions)

    async def test_native_changed_below_upper_config_or_owner_and_new_rows(self):
        from sqlalchemy import text
        async with self.engines[0].begin() as c:
            await c.execute(text("INSERT INTO participant_admission_locks VALUES (9)"))
        sessions = await self.snapshots()
        try:
            q = await self.qualify(sessions, await self.bounds(sessions))
        finally:
            await self.end(sessions)
        async with self.engines[0].begin() as c:
            await c.execute(text("INSERT INTO participant_admission_locks VALUES (10)"))
        fresh = await self.snapshots()
        try:
            with self.assertRaisesRegex(m.Rejected, "new_source_rows_require_next_cycle"):
                await m.recheck(*fresh, q)
        finally:
            await self.end(fresh)

    async def test_native_numeric_pk_pagination_full_eof_and_below_upper_change(self):
        from sqlalchemy import text
        async with self.engines[0].begin() as c:
            await c.execute(text("INSERT INTO participant_admission_locks VALUES (:n)"),
                            [{"n": n} for n in range(1, 1002)])
        sessions = await self.snapshots()
        try:
            q = await self.qualify(sessions, await self.bounds(sessions))
            self.assertEqual(q._scans[0].sections["participant_admission_locks"]["rows"], 1001)
            self.assertEqual([m.number(r["organization_id"]) for r in q._scans[0].rows["participant_admission_locks"]], list(range(1, 1002)))
        finally:
            await self.end(sessions)
        async with self.engines[0].begin() as c:
            await c.execute(text("DELETE FROM participant_admission_locks WHERE organization_id=500"))
        fresh = await self.snapshots()
        try:
            with self.assertRaisesRegex(m.Rejected, "full_source_or_owner_or_configuration_changed"):
                await m.recheck(*fresh, q)
        finally:
            await self.end(fresh)

    async def test_native_exact_empty_upper_and_private_page_budget(self):
        from sqlalchemy import text
        sessions = await self.snapshots()
        try:
            bounds = await self.bounds(sessions)
            q = await self.qualify(sessions, bounds)
            self.assertEqual(q._scans[0].sections["runtime_milestones"]["rows"], 0)
            # Changing an already approved private upper cannot self-approve a recheck.
            bounds[0].tables["participant_admission_locks"]["count"] = 1
        finally:
            await self.end(sessions)
        fresh = await self.snapshots()
        try:
            with self.assertRaisesRegex(m.Rejected, "original_approved_bounds_changed"):
                await m.recheck(*fresh, q)
        finally:
            await self.end(fresh)
        async with self.engines[0].begin() as c:
            await c.execute(text("INSERT INTO participant_admission_locks VALUES (1)"))
        sessions = await self.snapshots()
        try:
            bounds = await self.bounds(sessions)
            # The real query path must refuse before retaining oversized body pages.
            with patch.object(m, "MAX_PAGE_BYTES", 0), self.assertRaisesRegex(m.Rejected, "private_page_byte_budget_exceeded"):
                await m._scan(sessions[0], bounds[0], bounds[0].digest())
        finally:
            await self.end(sessions)

    async def test_native_actual_pending_evaluation_is_separate_domain(self):
        from datetime import datetime, timezone
        from dataclasses import fields
        from qs_ai.infrastructure.persistence.mysql.database import Database, Transactions
        from qs_ai.bootstrap.import_evaluation_assets import baseline_assets
        from qs_ai.infrastructure.persistence.mysql.evaluation_asset_registry import MySQLEvaluationAssets
        from qs_ai.infrastructure.persistence.mysql.schema_assets import MySQLSchemaAssets
        from qs_ai.bootstrap.import_evaluation_suites import baseline, install_baseline
        from qs_ai.bootstrap.import_routes import baseline_assets as route_baseline
        from qs_ai.infrastructure.persistence.mysql.route_assets import MySQLRouteAssets
        from qs_ai.domain.evaluation.identity import EvidenceReleaseIdentity, FrozenContractRef
        from qs_ai.infrastructure.qs_server.evaluation_suite import V6_PUBLISHED
        from qs_ai.infrastructure.qs_server.evaluation_policies import load_execution_policy, load_gate_policy
        from qs_ai.infrastructure.qs_server.semantic_assets import load_semantic_assets
        from qs_ai.infrastructure.persistence.mysql.evaluation_runs import create_run
        database = Database(None); database.engine = self.engines[0]
        tx = Transactions(database)
        source, policies, prompt, schema = baseline_assets()
        registry = MySQLEvaluationAssets(tx)
        for policy in policies:
            await registry.put_policy(policy, source, "owned-fixture")
        await registry.put_semantic_prompt(prompt, source, "owned-fixture")
        await MySQLSchemaAssets(tx).put(schema, source, "owned-fixture")
        source, suite, contracts = baseline()
        async with tx.open() as db:
            await install_baseline(db, source, suite, contracts, "owned-fixture")
            await db.commit()
        source, routes = route_baseline()
        for route in routes:
            await MySQLRouteAssets(tx).put(route, source, "owned-fixture")
        # The source's native storage fixture is a pending Run, not an eligible
        # published release or proof that provider execution occurred.
        refs = {f.name: FrozenContractRef(f.name, "v1", "sha256:"+"a"*64) for f in fields(EvidenceReleaseIdentity)}
        policy = load_execution_policy()
        route_ref = FrozenContractRef(routes[0].route, routes[0].revision, routes[0].fingerprint)
        refs.update(suite=V6_PUBLISHED, semantic_prompt=prompt.reference,
                    semantic_output_schema=load_semantic_assets().output_schema,
                    execution_policy=FrozenContractRef(policy.policy_id, policy.version, policy.fingerprint),
                    gate_policy=load_gate_policy().reference, generation_route=route_ref, semantic_route=route_ref)
        rid = uuid4()
        async with tx.open() as db:
            await create_run(db, rid, EvidenceReleaseIdentity(**refs), 1, "actor:1", "owned readonly fixture",
                             datetime(2026, 10, 8, tzinfo=timezone.utc))
            await db.commit()
        from qs_ai.infrastructure.workflow_transport.state_events import StateEventRecorder
        from qs_ai.infrastructure.persistence.mysql.messaging import MessagingStore
        _, _, _, _, keys = signed_history()
        recorder = StateEventRecorder(MessagingStore(), keys.trusted_signers["ai-sign"].key,
                                      keys.decrypt_keys["peer-encrypt"])
        async with tx.open() as db:
            await recorder.record_evaluation(db, str(rid))
            await db.commit()
        sessions = await self.snapshots()
        try:
            q = await self.qualify(sessions, await self.bounds(sessions), keys)
            self.assertEqual(q.receipt()["originals"], 0)
            self.assertEqual(q.receipt()["summary"]["evaluation"],
                             {"non_target_runs": 1, "legitimate_pending_runs": 1, "terminal_runs": 0})
            self.assertEqual(q.receipt()["summary"]["mq"]["non_target_pending_messages"], 1)
            self.assertEqual(q.receipt()["summary"]["mq"]["target_mq_responsibilities"], 0)
        finally:
            await self.end(sessions)

    async def test_native_real_signed_mq_full_original_receipt_and_ack_then_owner_change(self):
        from sqlalchemy import text
        ai, peer, _, _, keys = signed_history()
        await self.store_raw_fixture(ai, peer)
        sessions = await self.snapshots()
        try:
            q = await self.qualify(sessions, await self.bounds(sessions), keys)
            self.assertEqual(q.receipt()["originals"], 1)
            self.assertEqual(q.receipt()["summary"]["mq"]["authenticated_messages"], 5)
            self.assertFalse(q.receipt()["retirement_proven"])
        finally:
            await self.end(sessions)
        fresh = await self.snapshots()
        try:
            self.assertTrue((await m.recheck(*fresh, q, protection_keys=keys))["unchanged_full_source_hashes"])
        finally:
            await self.end(fresh)
        async with self.engines[1].begin() as c:
            await c.execute(text("UPDATE ai_messaging_operations SET organization_id=99"))
        changed = await self.snapshots()
        try:
            with self.assertRaisesRegex(m.Rejected, "full_source_or_owner_or_configuration_changed"):
                await m.recheck(*changed, q, protection_keys=keys)
        finally:
            await self.end(changed)

    async def test_native_borrowed_inactive_nested_wrong_identity_schema(self):
        sessions = await self.snapshots()
        try:
            with self.assertRaisesRegex(m.Rejected, "database_identity_or_isolation_changed"):
                await m.discover_full_bounds(sessions[0], side="ai", source_sha="a"*40,
                                             identity_hash="f"*64, head=m.AI_HEAD)
            nested = await sessions[0].begin_nested()
            with self.assertRaises(Exception):
                await m.discover_full_bounds(sessions[0], side="ai", source_sha="a"*40,
                                             identity_hash=self.identities[0], head=m.AI_HEAD)
            await nested.rollback()
        finally:
            await self.end(sessions)

    async def test_native_real_original_start_refusal_peer_business_receipt(self):
        from sqlalchemy import text
        from qs_ai.application.interpretation.service import InterpretationService
        from qs_ai.domain.interpretation.model import Actor
        from qs_ai.infrastructure.persistence.mysql.database import Database, Transactions
        from qs_ai.infrastructure.persistence.mysql.interpretation import MySQLUnitOfWorkFactory
        cid = str(uuid4())
        original = dict(request_id=cid, actor=dict(org_id="1", subject_id="fixture-subject"),
                        testee_id="2", assessment_ids=["3"], goal="private-secret-native")
        database = Database(None); database.engine = self.engines[0]
        service = InterpretationService(MySQLUnitOfWorkFactory(Transactions(database)), None)
        receipt = await service.start_external(Actor("1", "fixture-subject"), "2", ("3",), original["goal"], cid, ())
        self.assertEqual(receipt.status, "blocked")
        writer = m.sha(m._go_json(m._start(original)[0]))
        async with self.engines[0].begin() as c:
            event = (await c.execute(text("SELECT event_id,CAST(payload AS BINARY) FROM result_outbox"))).one()
            value = m.json_value(event[1])
            await c.execute(text("UPDATE result_outbox SET delivered=1,delivered_at=UTC_TIMESTAMP(6)"))
        projection = m._event_projection(value)
        async with self.engines[1].begin() as c:
            await c.execute(text("INSERT INTO ai_bridge_requests(request_id,request_hash,payload,session_id,version,status,projection,organization_id,subject_id,testee_id,created_at,updated_at) VALUES(:cid,:h,:p,:sid,:v,'blocked',:e,1,'fixture-subject',2,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))"),
                            {"cid": cid, "h": writer, "p": m.canonical(original).decode(), "sid": receipt.session_id,
                             "v": receipt.version, "e": m._go_json(projection).decode()})
            await c.execute(text("INSERT INTO ai_bridge_commands VALUES(:cid,:cid,'start',:p,:h,1,0,UTC_TIMESTAMP(6))"),
                            {"cid": cid, "p": m.canonical(original).decode(), "h": writer})
            await c.execute(text("INSERT INTO ai_bridge_request_assessments VALUES(:cid,3)"), {"cid": cid})
            await c.execute(text("INSERT INTO ai_bridge_events VALUES(:id,:cid,:v,:h)"),
                            {"id": event[0], "cid": cid, "v": receipt.version, "h": m.sha(m._go_json(projection))})
        sessions = await self.snapshots()
        try:
            q = await self.qualify(sessions, await self.bounds(sessions))
            self.assertEqual(q.receipt()["originals"], 1)
            self.assertEqual(q.receipt()["summary"]["known_terminal_without_artifact"], 1)
            self.assertEqual(q.receipt()["summary"]["mq"]["authenticated_messages"], 0)
            self.assertNotIn(cid, json.dumps(q.receipt()))
        finally:
            await self.end(sessions)


if __name__ == "__main__":
    unittest.main()


class Layout0040(unittest.TestCase):
    def setUp(self):
        self.layout = m._scanner("ai", "0040_module_table_names")._layout()
        self.rows = {n: [] for n in self.layout.SPECS}
        self.rows["alembic_version"] = [row(version_num=self.layout.HEAD)]
        self.schemas = {n: {"columns": [(c,) for c in spec[0].split()], "columns_sha256": m.sha(n.encode())}
                        for n, spec in self.layout.SPECS.items()}

    def project(self):
        return self.layout.project(self.rows, m.AI_SPECS, self.schemas)

    def physical(self, table, **values):
        contract = self.layout.CONTRACT[table]
        out = {}
        for c in contract["columns"]:
            if c["name"] in values:
                out[c["name"]] = raw(values[c["name"]])
            elif c["nullable"]:
                out[c["name"]] = None
            elif c["type"] == "JSON":
                out[c["name"]] = b"{}"
            elif c["type"].startswith(("DATETIME", "TIMESTAMP")):
                out[c["name"]] = b"2026-10-08 00:00:00.000000"
            elif c["type"].startswith(("BIGINT", "INTEGER", "BOOL")):
                out[c["name"]] = b"0"
            else:
                out[c["name"]] = b"fixture"
        self.rows[table].append(out)
        return out

    def asset(self, kind="profile", owner=0, identity="fixture-asset", version="1", body=b'{ "raw": "unchanged" }'):
        native = kind in ("profile", "prompt", "route", "schema")
        return self.physical("governance_asset_versions", asset_row_id=len(self.rows["governance_asset_versions"])+1,
            asset_kind=kind, owner_organization_id=owner, asset_id=identity, version=version,
            fingerprint="a"*64, created_at="2026-10-08 00:00:00.000000", body_format="prompt_package_json" if kind=="prompt" else "semantic_markdown" if kind=="semantic_prompt" else "definition_json",
            body_bytes=body, package_sha256="b"*64 if kind=="prompt" else None,
            native_id_key=identity if native else None, scoped_id_key=None if native else identity,
            profile_id_key=identity if kind=="profile" else None, catalog_version_key=version)

    def draft(self, kind="prompt", identity="00000000-0000-0000-0000-000000000001", owner=1):
        did=len(self.rows["governance_draft_heads"])+1
        self.physical("governance_draft_heads",draft_row_id=did,draft_kind=kind,organization_id=owner,draft_id=identity,revision=1,
            prompt_draft_id_key=identity if kind=="prompt" else None,semantic_draft_id_key=identity if kind=="semantic" else None)
        blob=b'{ "snapshot": 1 }'
        return self.physical("governance_draft_versions",draft_row_id=did,revision=1,draft_kind=kind,organization_id=owner,
            command_id=identity if kind=="prompt" else None,operator_user_id=1 if kind=="prompt" else None,
            request_bytes=b'{ "request": 1 }' if kind=="prompt" else None,
            snapshot_bytes=blob,snapshot_sha256=m.sha(blob),prompt_command_id_key=identity if kind=="prompt" else None)

    def completion(self, kind="generation"):
        if not self.rows["evaluation_runs"]:
            self.physical("evaluation_runs",run_id="fixture-run",frozen_execution_policy_fingerprint=None,frozen_execution_policy_json=None)
        return self.physical("evaluation_completions",kind=kind,run_id="fixture-run",execution_id=kind,
            candidate_id="candidate",case_id="case" if kind=="generation" else None,slot_ordinal=0 if kind=="generation" else None,
            result_json=b'{ "result": 1 }' if kind=="semantic" else None,candidate_json=None,
            raw_output=b'{ "raw": "kept" }',normalized_output=b' { "normalized": true } ',
            gen_candidate_key="candidate" if kind=="generation" else None,gen_case_key="case" if kind=="generation" else None,
            gen_slot_key=0 if kind=="generation" else None,sem_candidate_key="candidate" if kind=="semantic" else None)

    def test_empty_whole43_plus_system_to53_has_no_inferred_records(self):
        logical,lineage=self.project()
        self.assertEqual(len(self.rows),44);self.assertEqual(len(logical),53)
        self.assertTrue(all(not v for v in logical.values()));self.assertTrue(all(not v for v in lineage.values()))

    def test_all_six_merge_groups_preserve_raw_and_exact_source_pk(self):
        for kind in self.layout.ASSETS:self.asset(kind=kind,owner=1 if kind=="semantic_prompt" else 0,identity=kind)
        self.draft();self.draft("semantic",owner=2)
        self.completion();self.completion("semantic")
        session=self.physical("interpretation_sessions",id="session",request_id="request")
        run=self.rows["evaluation_runs"][0];run["frozen_execution_policy_fingerprint"]=b"a"*64;run["frozen_execution_policy_json"]=b' { "frozen": true } '
        logical,lineage=self.project()
        self.assertEqual(logical["profile_assets"][0]["definition_json"],self.rows["governance_asset_versions"][0]["body_bytes"])
        self.assertEqual(logical["semantic_draft_versions"][0]["organization_id"],b"2")
        self.assertEqual(logical["evaluation_semantic_completions"][0]["raw_output"],b'{ "raw": "kept" }')
        self.assertEqual(logical["evaluation_run_policies"][0]["definition_json"],run["frozen_execution_policy_json"])
        self.assertEqual(logical["external_requests"],[row(request_id="request",session_id="session")])
        self.assertEqual(lineage["external_requests"][0]["physical_pk"],(session["id"],))
        self.assertEqual(lineage["prompt_draft_revisions"][0]["physical_pk"],(b"1",b"1"))
        self.assertEqual(lineage["evaluation_generation_completions"][0]["physical_pk"],(b"generation",b"fixture-run",b"generation"))
        for name,values in logical.items():self.assertEqual(len(values),len(lineage[name]))

    def test_unknown_merge_kinds_block_even_without_original_targets(self):
        builders=(("asset_kind",lambda:self.asset()),("draft_kind",lambda:self.draft()),("kind",lambda:self.completion()))
        for field,builder in builders:
            self.setUp();target=builder();target[field]=b"unknown-private-do-not-echo"
            with self.subTest(field=field),self.assertRaises(self.layout.Rejected) as caught:self.project()
            self.assertNotIn("private",str(caught.exception))

    def test_full_raw_scope_and_head_required(self):
        for mutate in (lambda:self.rows.pop("messaging_observations"),lambda:self.rows["alembic_version"].clear(),lambda:self.rows["alembic_version"][0].update(version_num=b"0038_messaging_observations")):
            self.setUp();mutate()
            with self.assertRaises(self.layout.Rejected):self.project()

    def test_global_orphan_unrelated_to_original_command_not_hidden(self):
        self.physical("execution_model_calls",run_id="unbound-run")
        with self.assertRaisesRegex(self.layout.Rejected,"orphan"):self.project()

    def test_draft_wrong_parent_org_revision_and_null_audit_block(self):
        for field,value in (("organization_id",b"2"),("draft_row_id",b"90"),("revision",b"2"),("command_id",None),("prompt_command_id_key",b"wrong"),("snapshot_bytes",b"changed")):
            self.setUp();target=self.draft();target[field]=value
            with self.subTest(field=field),self.assertRaises(self.layout.Rejected):self.project()

    def test_policy_single_null_not_filled_from_current_policy(self):
        for fp,body in ((b"a"*64,None),(None,b"{}")):
            self.setUp();self.physical("evaluation_runs",run_id="r",frozen_execution_policy_fingerprint=fp,frozen_execution_policy_json=body)
            with self.assertRaisesRegex(self.layout.Rejected,"single_null"):self.project()

    def test_generated_owner_collision_null_shapes_block(self):
        for mutate in (lambda r:r.update(owner_organization_id=b"9"),lambda r:r.update(profile_id_key=b"wrong"),lambda r:r.update(package_sha256=b"a"*64),lambda r:r.update(body_bytes=None)):
            self.setUp();target=self.asset();mutate(target)
            with self.assertRaises(self.layout.Rejected):self.project()
        self.setUp();self.asset();self.asset()
        with self.assertRaisesRegex(self.layout.Rejected,"collision"):self.project()

    def test_completion_single_null_wrong_kind_or_generated_key_block(self):
        for field,value in (("case_id",None),("gen_case_key",b"other"),("result_json",b"{}"),("run_id",b"missing")):
            self.setUp();target=self.completion();target[field]=value
            with self.subTest(field=field),self.assertRaises(self.layout.Rejected):self.project()

    def test_physical_duplicate_pk_unknown_column_and_mutable_input_block(self):
        for mutate in (lambda r:self.rows["governance_asset_versions"].append(dict(r)),lambda r:r.update(unknown=None),lambda r:r.update(body_bytes="not-binary")):
            self.setUp();target=self.asset();mutate(target)
            with self.assertRaises(self.layout.Rejected):self.project()

    def test_physical_schema_isolated_from_legacy_registry_and_sealed_observer(self):
        current=m._scanner("ai",self.layout.HEAD);legacy=m._scanner("ai",m.AI_HEAD)
        self.assertEqual(len(current.SPECS),44);self.assertEqual(len(legacy.SPECS),53)
        self.assertNotEqual(current.SPECS,legacy.SPECS);self.assertEqual(len(m.AI_SPECS),53)
        changed=PATH.with_name("qs-ai-retirement-0040-layout.py").read_bytes()+b"\n"
        with patch.object(Path,"read_bytes",return_value=changed),self.assertRaisesRegex(current.Rejected,"fixed_0040_layout_changed"):current._layout()


@unittest.skipUnless(os.environ.get("QS_AI_RETIREMENT_VERIFIER_NATIVE") == "1", "owned native opt-in")
class Native0040(unittest.IsolatedAsyncioTestCase):
    clean=Native.clean
    asyncTearDown=Native.asyncTearDown
    snapshots=Native.snapshots
    end=Native.end
    qualify=Native.qualify

    async def asyncSetUp(self):
        from sqlalchemy import text
        await Native.asyncSetUp(self)
        self.layout=m._scanner("ai","0040_module_table_names")._layout()
        try:
            async with self.engines[0].begin() as c:
                names=(await c.execute(text("SELECT TABLE_NAME FROM information_schema.tables WHERE table_schema=DATABASE()"))).scalars().all()
                await c.execute(text("SET FOREIGN_KEY_CHECKS=0"))
                for n in names:await c.execute(text("DROP TABLE `"+n+"`"))
                # Exact pinned 82ffa/v0040 DDL, topological FK creation.
                pending=dict(self.layout.CONTRACT);done=set()
                while pending:
                    ready=[n for n,t in pending.items() if all(f["table"] in done for f in t["foreign_keys"])]
                    if not ready:raise AssertionError("0040_fixture_fk_cycle")
                    for n in ready:
                        t=pending.pop(n);await c.execute(text(t["create_sql"]))
                        for sql in t["index_sql"]:await c.execute(text(sql))
                        done.add(n)
                await c.execute(text("SET FOREIGN_KEY_CHECKS=1"))
                await c.execute(text("CREATE TABLE alembic_version(version_num VARCHAR(32) PRIMARY KEY) ENGINE=InnoDB"))
                await c.execute(text("INSERT INTO alembic_version VALUES (:head)"),{"head":self.layout.HEAD})
        except Exception:
            await self.clean();raise AssertionError("owned_0040_exact_schema_setup_failed") from None

    async def bounds(self,sessions):
        return [await m.discover_full_bounds(s,side=side,source_sha=self.layout.SOURCE_SHA if side=="ai" else "a"*40,identity_hash=identity,head=head)
                for s,side,identity,head in zip(sessions,("ai","peer"),self.identities,(self.layout.HEAD,"99"))]

    async def insert(self,table,values):
        from sqlalchemy import text
        async with self.engines[0].begin() as c:
            await c.execute(text("INSERT INTO `"+table+"` ("+",".join("`"+k+"`" for k in values)+") VALUES ("+",".join(":"+k for k in values)+")"),values)

    async def test_native0040_all43_actual_columns_pk_indexes_system_two_passes_fresh(self):
        sessions=await self.snapshots()
        try:
            bounds=await self.bounds(sessions);q=await self.qualify(sessions,bounds)
            scan=q._scans[0]
            self.assertEqual(len(scan.sections),44);self.assertEqual(len(scan.rows),53)
            self.assertEqual(scan.sections["alembic_version"]["rows"],1)
            self.assertTrue(all(section["equal_full_passes"]==2 for section in scan.sections.values()))
            self.assertTrue(all("indexes" in s and "foreign_keys" in s and "checks" in s for s in bounds[0].tables.values()))
            self.assertFalse(q.receipt()["drop_ready"])
            self.assertTrue(all(s.get_transaction().is_active for s in sessions))
            with self.assertRaisesRegex(m.Rejected,"original_snapshots_must_end"):await m.recheck(*sessions,q)
        finally:await self.end(sessions)
        fresh=await self.snapshots()
        try:self.assertTrue((await m.recheck(*fresh,q))["unchanged_full_source_hashes"])
        finally:await self.end(fresh)

    async def test_native0040_actual_blob_identity_lineage_and_below_upper_change(self):
        values=dict(asset_kind="profile",owner_organization_id=0,asset_id="fixture-profile",version="1",fingerprint="a"*64,
                    body_format="definition_json",body_bytes=b' { "private": "raw" } ',source_ref="fixture-source",imported_by=1,created_at=None)
        await self.insert("governance_asset_versions",values)
        sessions=await self.snapshots()
        try:
            bounds=await self.bounds(sessions);scan=await m._scan(sessions[0],bounds[0],bounds[0].digest())
            self.assertEqual(scan.rows["profile_assets"][0]["definition_json"],values["body_bytes"])
            self.assertEqual(scan.lineage["profile_assets"][0]["physical_table"],"governance_asset_versions")
            self.assertEqual(scan.lineage["profile_assets"][0]["physical_pk"],(b"1",))
        finally:await self.end(sessions)
        from sqlalchemy import text
        async with self.engines[0].begin() as c:await c.execute(text("UPDATE governance_asset_versions SET body_bytes=:v"),{"v":b' { "private": "changed" } '})
        fresh=await self.snapshots()
        try:
            self.assertFalse(any(v["next_cycle_required"] for v in (await scan.module.fresh_after_upper(fresh[0],scan.observation)).values()))
            changed=await m._scan(fresh[0],bounds[0],bounds[0].digest())
            self.assertNotEqual(changed.sections,scan.sections)
        finally:await self.end(fresh)

    async def test_native0040_schema_unknown_column_new_index_and_write_tx_rejected(self):
        from sqlalchemy import text
        write=await self.snapshots(False)
        try:
            with self.assertRaisesRegex(m.Rejected,"actual_readonly_snapshot_not_observed"):await self.bounds(write)
        finally:await self.end(write)
        for ddl in ("ALTER TABLE operations_runtime_milestones ADD unknown_private_column TEXT NULL","CREATE INDEX unknown_extra_index ON operations_runtime_milestones(kind)"):
            async with self.engines[0].begin() as c:await c.execute(text(ddl))
            sessions=await self.snapshots()
            try:
                with self.assertRaises(m.Rejected):await self.bounds(sessions)
            finally:await self.end(sessions)
            async with self.engines[0].begin() as c:await c.execute(text("ALTER TABLE operations_runtime_milestones DROP COLUMN unknown_private_column" if "ADD" in ddl else "DROP INDEX unknown_extra_index ON operations_runtime_milestones"))




    async def test_native0040_actual_ai_collation_mixed_unicode_and_numeric_keyset_complete(self):
        from sqlalchemy import text
        keys=["Z","a","é","Ω","ß","中","q"]
        numbers=[0,1,2,9,10]
        async with self.engines[0].begin() as c:
            for key in keys:
                await c.execute(text("INSERT INTO evaluation_runs(run_id,execution_mode,organization_id,requested_by,definition_json,progress_json) VALUES (:id,'serial_v1',1,'fixture','{}','{}')"),{"id":key})
                for number in numbers:
                    await c.execute(text("INSERT INTO evaluation_slot_claims(run_id,case_id,slot_ordinal,version,checkpoint_json) VALUES (:id,'case',:n,0,'{}')"),{"id":key,"n":number})
        sessions=await self.snapshots()
        try:
            with patch.object(m,"PAGE",2):
                bounds=await self.bounds(sessions)
                metadata={c[0]:c for c in bounds[0].tables["evaluation_runs"]["columns"]}
                self.assertEqual(metadata["run_id"][5],"utf8mb4_0900_ai_ci")
                actual=list((await sessions[0].execute(text("SELECT CAST(run_id AS BINARY) FROM evaluation_runs ORDER BY run_id"))).scalars().all())
                numeric=[tuple(x) for x in (await sessions[0].execute(text("SELECT CAST(run_id AS BINARY),CAST(slot_ordinal AS BINARY) FROM evaluation_slot_claims ORDER BY run_id,case_id,slot_ordinal"))).all()]
                self.assertNotEqual(actual,sorted(actual))
                self.assertNotEqual(numeric,sorted(numeric))
                scan=await m._scan(sessions[0],bounds[0],bounds[0].digest())
                self.assertEqual([r["run_id"] for r in scan.physical_rows["evaluation_runs"]],actual)
                self.assertEqual([(r["run_id"],r["slot_ordinal"]) for r in scan.physical_rows["evaluation_slot_claims"]],numeric)
                self.assertEqual(scan.sections["evaluation_runs"]["rows"],len(keys))
                self.assertEqual(scan.sections["evaluation_slot_claims"]["rows"],len(keys)*len(numbers))
                self.assertGreater(scan.sections["evaluation_slot_claims"]["pages"],1)
                self.assertTrue(all(x["equal_full_passes"]==2 for x in scan.sections.values()))
        finally:await self.end(sessions)

    async def test_native0040_all_six_nonempty_merges_keep_binary_null_and_original_identity(self):
        from sqlalchemy import text
        fixture=Layout0040();fixture.setUp()
        for kind in self.layout.ASSETS:fixture.asset(kind,1 if kind=="semantic_prompt" else 0,identity=kind)
        fixture.draft();fixture.draft("semantic",owner=2)
        fixture.completion();fixture.completion("semantic")
        policy=fixture.rows["evaluation_runs"][0]
        policy["frozen_execution_policy_fingerprint"]=b"a"*64
        policy["frozen_execution_policy_json"]=b' { "frozen": "original" } '
        fixture.physical("interpretation_sessions",id="session",request_id="request")
        async with self.engines[0].begin() as c:
            pending=dict(self.layout.CONTRACT);done=set()
            while pending:
                ready=[n for n,t in pending.items() if all(f["table"] in done for f in t["foreign_keys"])]
                for n in ready:
                    contract=pending.pop(n)
                    for original in fixture.rows[n]:
                        values={k:(value.decode("utf-8") if value is not None and next(col for col in contract["columns"] if col["name"]==k)["type"]=="JSON" else value) for k,value in original.items() if next(col for col in contract["columns"] if col["name"]==k)["computed"] is None}
                        await c.execute(text("INSERT INTO `"+n+"` ("+",".join("`"+k+"`" for k in values)+") VALUES ("+",".join(":"+k for k in values)+")"),values)
                    done.add(n)
        sessions=await self.snapshots()
        try:
            bounds=await self.bounds(sessions);scan=await m._scan(sessions[0],bounds[0],bounds[0].digest())
            self.assertEqual(scan.rows["profile_assets"][0]["definition_json"],fixture.rows["governance_asset_versions"][0]["body_bytes"])
            self.assertEqual(scan.rows["prompt_draft_revisions"][0]["snapshot_json"],b'{ "snapshot": 1 }')
            self.assertEqual(scan.rows["semantic_draft_versions"][0]["snapshot_json"],b'{ "snapshot": 1 }')
            self.assertIsNone(scan.physical_rows["governance_draft_versions"][1]["request_bytes"])
            self.assertEqual(scan.rows["evaluation_generation_completions"][0]["normalized_output"],b' { "normalized": true } ')
            self.assertEqual(scan.rows["evaluation_semantic_completions"][0]["raw_output"],b'{ "raw": "kept" }')
            self.assertEqual(scan.rows["evaluation_run_policies"][0]["definition_json"],policy["frozen_execution_policy_json"])
            self.assertEqual(scan.rows["external_requests"][0]["request_id"],b"request")
            physical={(n,tuple(r[k] for k in self.layout.SPECS[n][1].split())) for n,rows in scan.physical_rows.items() if n!="alembic_version" for r in rows}
            projected={(x["physical_table"],x["physical_pk"]) for refs in scan.lineage.values() for x in refs}
            self.assertEqual(physical,projected)
        finally:await self.end(sessions)

    async def test_native0040_standalone_observer_fresh_checks_below_upper_and_host_ownership(self):
        from sqlalchemy import text
        path=PATH.with_name("qs-ai-retirement-readonly-observer.py")
        name="_native_standalone_0040_"+uuid4().hex
        spec=importlib.util.spec_from_file_location(name,path);observer=importlib.util.module_from_spec(spec);sys.modules[name]=observer
        try:spec.loader.exec_module(observer)
        finally:sys.modules.pop(name,None)
        await self.insert("governance_asset_versions",dict(asset_kind="profile",owner_organization_id=0,asset_id="fixture-profile",version="1",fingerprint="a"*64,body_format="definition_json",body_bytes=b' { "raw": 1 } ',source_ref="fixture",imported_by=1))
        sessions=await self.snapshots()
        try:
            bound=await observer.discover_bounds(sessions[0],identity_hash=self.identities[0],source_sha=self.layout.SOURCE_SHA,head=self.layout.HEAD)
            observation=await observer.observe(sessions[0],bound,approved_bounds_sha256=bound.digest())
            self.assertEqual(len(observation._sections),44);self.assertEqual(len(observation._rows),18)
            self.assertEqual(observation.receipt()["server_readonly_snapshot_mode"],"actual_rr_readonly_observed")
            self.assertTrue(sessions[0].get_transaction().is_active)
            with self.assertRaisesRegex(observer.Rejected,"original_snapshot_must_end"):await observer.fresh_after_upper(sessions[0],observation)
        finally:await self.end(sessions)
        fresh=await self.snapshots()
        try:
            result=await observer.fresh_after_upper(fresh[0],observation)
            self.assertEqual(len(result),44);self.assertTrue(all(x["unchanged_full_source_hashes"] for x in result.values()))
            self.assertTrue(fresh[0].get_transaction().is_active)
        finally:await self.end(fresh)
        async with self.engines[0].begin() as c:await c.execute(text("UPDATE governance_asset_versions SET body_bytes=:body"),{"body":b' { "raw": 2 } '})
        changed=await self.snapshots()
        try:
            with self.assertRaisesRegex(observer.Rejected,"post_snapshot_full_source_changed"):await observer.fresh_after_upper(changed[0],observation)
            self.assertTrue(changed[0].get_transaction().is_active)
        finally:await self.end(changed)

    async def test_native0040_eight_originals_not_synthetic_targets_and_peer_corruption(self):
        from sqlalchemy import text
        fixture=Layout0040();fixture.setUp()
        peer={n:[] for n in m.PEER_SPECS}
        for _ in range(8):
            ai,p,_,_=history()
            request=ai["external_requests"][0]["request_id"]
            for old,rows in ai.items():
                if old=="external_requests":continue
                physical=self.layout.RENAMES.get(old,old)
                for original in rows:
                    values=dict(original)
                    if old=="interpretation_sessions":values["request_id"]=request
                    fixture.physical(physical,**values)
            for n,rows in p.items():peer[n].extend(rows)
        async with self.engines[0].begin() as c:
            pending=dict(self.layout.CONTRACT);done=set()
            while pending:
                ready=[n for n,t in pending.items() if all(f["table"] in done for f in t["foreign_keys"])]
                for n in ready:
                    contract=pending.pop(n)
                    for original in fixture.rows[n]:
                        values={k:(value.decode("utf-8") if value is not None and next(col for col in contract["columns"] if col["name"]==k)["type"]=="JSON" else value) for k,value in original.items() if next(col for col in contract["columns"] if col["name"]==k)["computed"] is None}
                        await c.execute(text("INSERT INTO `"+n+"` ("+",".join("`"+k+"`" for k in values)+") VALUES ("+",".join(":"+k for k in values)+")"),values)
                    done.add(n)
        async with self.engines[1].begin() as c:
            for n in ["ai_bridge_requests"]+[k for k in peer if k!="ai_bridge_requests"]:
                json_fields=set((await c.execute(text("SELECT COLUMN_NAME FROM information_schema.columns WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=:n AND DATA_TYPE='json'"),{"n":n})).scalars().all())
                for original in peer[n]:
                    values={k:(v.decode("utf-8") if k in json_fields and v is not None else v) for k,v in original.items()}
                    await c.execute(text("INSERT INTO `"+n+"` ("+",".join("`"+k+"`" for k in values)+") VALUES ("+",".join(":"+k for k in values)+")"),values)
        sessions=await self.snapshots()
        try:
            bounds=await self.bounds(sessions);q=await self.qualify(sessions,bounds)
            self.assertEqual(q.receipt()["originals"],8)
            self.assertEqual(q.receipt()["summary"]["known_terminal_without_artifact"],8)
            self.assertFalse(q.receipt()["drop_ready"])
            self.assertEqual(len(q._scans[0].lineage["external_requests"]),8)
        finally:await self.end(sessions)
        async with self.engines[1].begin() as c:await c.execute(text("UPDATE ai_bridge_events SET payload_hash=:bad LIMIT 1"),{"bad":"0"*64})
        fresh=await self.snapshots()
        try:
            with self.assertRaises(m.Rejected):await m.recheck(*fresh,q)
        finally:await self.end(fresh)

    async def test_native0040_unknown_kind_single_null_and_bad_constraints_refused(self):
        from sqlalchemy import text
        async with self.engines[0].begin() as c:
            for ck in self.layout.CONTRACT["governance_asset_versions"]["checks"]:
                await c.execute(text("ALTER TABLE governance_asset_versions ALTER CHECK `"+ck["name"]+"` NOT ENFORCED"))
            await c.execute(text("INSERT INTO governance_asset_versions(asset_kind,owner_organization_id,asset_id,version,fingerprint,body_format,body_bytes,source_ref,imported_by) VALUES ('unknown',0,'unknown','1',:fp,'definition_json',:body,'fixture',1)"),{"fp":"a"*64,"body":b"{}"})

        sessions=await self.snapshots()
        try:
            with self.assertRaisesRegex(m.Rejected,"check_constraint_conflict"):await self.bounds(sessions)
        finally:await self.end(sessions)
        async with self.engines[0].begin() as c:
            await c.execute(text("DELETE FROM governance_asset_versions"))
            for ck in self.layout.CONTRACT["governance_asset_versions"]["checks"]:
                await c.execute(text("ALTER TABLE governance_asset_versions ALTER CHECK `"+ck["name"]+"` ENFORCED"))
            await c.execute(text("ALTER TABLE evaluation_runs DROP CHECK ck_evaluation_runs_frozen_policy"))
        sessions=await self.snapshots()
        try:
            with self.assertRaisesRegex(m.Rejected,"complete_checks"):await self.bounds(sessions)
        finally:await self.end(sessions)

    async def test_native0040_global_orphan_cannot_hide_with_fk_off_at_import(self):
        from sqlalchemy import text
        async with self.engines[0].begin() as c:
            await c.execute(text("SET FOREIGN_KEY_CHECKS=0"))
            await c.execute(text("INSERT INTO execution_model_calls(run_id,invocation_id,fence_token,status,request_json) VALUES ('orphan','call',1,'failed','{}')"))
            await c.execute(text("SET FOREIGN_KEY_CHECKS=1"))
        sessions=await self.snapshots()
        try:
            bounds=await self.bounds(sessions)
            with self.assertRaisesRegex(m.Rejected,"orphan"):await m._scan(sessions[0],bounds[0],bounds[0].digest())
        finally:await self.end(sessions)


class Layout0040Metadata(unittest.TestCase):
    def setUp(self):self.layout=m._scanner("ai","0040_module_table_names")._layout()

    def metadata(self,table):
        t=self.layout.CONTRACT[table]
        columns=[];generated=[];indexes=[];fks=[];checks=[]
        for c in t["columns"]:
            auto=any(__import__('re').search(r"\b"+c["name"]+r"\b.*AUTO_INCREMENT",line,__import__('re').I) for line in t["create_sql"].splitlines())
            extra="auto_increment" if auto else "STORED GENERATED" if c["computed"] and c["persisted"] else "VIRTUAL GENERATED" if c["computed"] else ""
            columns.append((c["name"],self.layout._type(c["type"]),"YES" if c["nullable"] else "NO",c["default"],extra,c["collation"]))
            generated.append((c["name"],c["computed"] or ""))
        for name,keys,unique in [("PRIMARY",t["primary_key"],True)]+[(i["name"],i["columns"],i["unique"]) for i in t["indexes"]]:
            indexes.extend((name,n,k,0 if unique else 1,"A",None,"BTREE","YES",None) for n,k in enumerate(keys,1))
        for f in t["foreign_keys"]:
            fks.extend((f["name"],n,k,"fixture_db",f["table"],parent,"NO ACTION",f["ondelete"] or "NO ACTION") for n,(k,parent) in enumerate(zip(f["columns"],f["referred_columns"]),1))
        checks.extend((c["name"],c["sql"],"YES") for c in t["checks"])
        return columns,indexes,generated,fks,checks

    def test_all_fixed_contract_metadata_and_mysql_boolean_rendering(self):
        for n in self.layout.CONTRACT:
            columns,indexes,generated,fks,checks=self.metadata(n)
            self.layout.validate_schema(n,columns,indexes)
            self.layout.validate_constraints(n,generated,fks,checks,"fixture_db")
        a="(kind='generation' AND case_id IS NOT NULL) OR kind='semantic'"
        b="((`kind` = _utf8mb4'generation') and (`case_id` is not null)) or (`kind` = _utf8mb4'semantic')"
        self.assertEqual(self.layout._expression(a),self.layout._expression(b))
        self.assertEqual(self.layout._expression(a),self.layout._expression(b.replace("_utf8mb4'generation'", "_ascii\\'generation\\'").replace("_utf8mb4'semantic'", "_ascii\\'semantic\\'")))
        self.assertNotEqual(self.layout._expression("(a=1 OR b=1) AND c=1"),self.layout._expression("a=1 OR (b=1 AND c=1)"))

    def test_index_column_default_type_null_generated_collation_strict(self):
        n="governance_asset_versions"
        for mutate in (lambda c,i:c[0].__setitem__(1,"bigint"),lambda c,i:c[0].__setitem__(2,"YES"),lambda c,i:c[0].__setitem__(4,""),lambda c,i:c[1].__setitem__(3,"unknown"),lambda c,i:i.append(["unknown",1,"asset_id",1,"A",None,"BTREE","YES",None]),lambda c,i:i[0].__setitem__(7,"NO"),lambda c,i:i[0].__setitem__(4,"D")):
            columns,indexes,*_=self.metadata(n);columns=[list(x) for x in columns];indexes=[list(x) for x in indexes];mutate(columns,indexes)
            with self.assertRaises(self.layout.Rejected):self.layout.validate_schema(n,columns,indexes)

    def test_fk_check_and_generated_expressions_cannot_be_approved_wrong(self):
        n="governance_draft_versions"
        for part,mutate in (("fk",lambda g,f,c:f.clear()),("cross_schema",lambda g,f,c:f[0].__setitem__(3,"another_db")),("generated",lambda g,f,c:g[-1].__setitem__(1,"command_id")),("disabled_check",lambda g,f,c:c[0].__setitem__(2,"NO")),("changed_precedence",lambda g,f,c:c[0].__setitem__(1,"revision>=0"))):
            _,_,g,f,c=self.metadata(n);g=[list(x) for x in g];f=[list(x) for x in f];c=[list(x) for x in c];mutate(g,f,c)
            with self.subTest(part=part),self.assertRaises(self.layout.Rejected):self.layout.validate_constraints(n,g,f,c,"fixture_db")

    def test_system_schema_null_check_extra_generated_not_accepted(self):
        with self.assertRaises(self.layout.Rejected):self.layout.validate_schema("alembic_version",[("version_num","varchar(32)","YES",None,"","utf8mb4_bin")],[])
        with self.assertRaises(self.layout.Rejected):self.layout.validate_constraints("alembic_version",[("version_num","'fixed'")],[],[],"fixture_db")
