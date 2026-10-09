"""Offline contracts and opt-in, owned-loopback MySQL snapshot tests.

Only this fixture host opens/ends transactions. The observer borrows them.
Native setup writes solely a randomly named test database, never production.
"""
import ast
import asyncio
import base64
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import unittest
from unittest.mock import AsyncMock, patch
from uuid import UUID, uuid4

PATH = Path(__file__).with_name("qs-ai-retirement-readonly-observer.py")
spec = importlib.util.spec_from_file_location("qs_ai_retirement_readonly_observer", PATH)
m = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = m
spec.loader.exec_module(m)


def encoded(**values):
    return {k: (None if v is None else str(v).encode()) for k, v in values.items()}


def full_row(table, **values):
    return {k: (None if values.get(k) is None else str(values[k]).encode())
            for k in m.SPECS[table][0].split()}


def fixture():
    rows = {table: [] for table in m.SPECS}
    targets = []
    for n in range(1, 9):
        cid, sid, rid = [str(UUID(int=n * 10 + k)) for k in (1, 2, 3)]
        target = dict(command_id=cid, request_id=cid, session_id=sid,
                      organization_id="1", subject_id="fixture-subject", testee_id="2",
                      assessment_ids=["3"], qs_ai_request_hash="a"*64,
                      source_row_sha256=hashlib.sha256(str(n).encode()).hexdigest())
        targets.append(target)
        rows["external_requests"].append(dict(request_id=cid, session_id=sid))
        rows["interpretation_sessions"].append(dict(id=sid, org_id="1", owner_subject_id="fixture-subject",
              testee_id="2", assessment_ids=["3"], status="completed", version="3", active_run_id=rid))
        rows["interpretation_runs"].append(dict(id=rid, session_id=sid, session_version="1", status="completed"))
        rows["execution_jobs"].append(dict(id=str(UUID(int=n*10+4)),run_id=rid,session_id=sid,status="done"))
        rows["model_calls"].append(dict(run_id=rid,status="response_received",digests={"response_json":"b"*64}))
        rows["idempotency_requests"].append(dict(scope_hash=m._sha(m._canon(["qs-server","external-start-v1"])),
              key=cid,request_hash="a"*64,original_receipt=dict(session_id=sid,run_id=rid,status="queued",version=1)))
    return rows, tuple(targets)


class PageReader:
    def __init__(self, rows):
        self.rows, self.calls = rows, []

    async def query(self, sql, params):
        self.calls.append((sql, params.copy()))
        selected = [r for r in self.rows if ("u0" not in params or r[0].decode() <= params["u0"])
                    and ("l0" not in params or r[0].decode() > params["l0"])]
        selected=selected[:params["page"]]
        return [(r[0],sum(len(v) for v in r if v is not None)) for r in selected] if " AS _source_row_bytes " in sql else selected


def lease_bound(upper):
    columns = [(k, "varchar(191)" if k=="thread_id" else "bigint unsigned", "NO", None, "", "utf8mb4_bin" if k=="thread_id" else None)
               for k in m.SPECS["execution_leases"][0].split()]
    return dict(columns=columns, columns_sha256=m._sha(m._canon(columns)),
                kinds={"thread_id":"varchar(191)"}, upper=None if upper is None else [base64.b64encode(upper).decode()])


class Offline(unittest.IsolatedAsyncioTestCase):
    def test_closed_complete_table_set(self):
        self.assertEqual(len(m.SPECS), 18)
        for columns, keys in m.SPECS.values():
            self.assertEqual(len(columns.split()),len(set(columns.split())))
            self.assertLessEqual(set(keys.split()),set(columns.split()))

    def test_helpers_do_not_own_lifecycle_or_dml(self):
        tree=ast.parse(PATH.read_text())
        prohibited={"commit","rollback","begin","close_all","create_async_engine","send","publish"}
        for node in ast.walk(tree):
            if isinstance(node,ast.Call):
                name=node.func.attr if isinstance(node.func,ast.Attribute) else getattr(node.func,"id","")
                self.assertNotIn(name,prohibited)

    def test_null_and_empty_source_digests_differ(self):
        a,b=hashlib.sha256(),hashlib.sha256()
        m._frame(a,None);m._frame(b,b"")
        self.assertNotEqual(a.digest(),b.digest())

    def test_python_original_fingerprint_golden(self):
        from qs_ai.application.interpretation.service import fingerprint
        value=[{"org_id":"1","subject_id":"fixture-subject"},"2",("3",),"目标",{"unknown":None}]
        self.assertEqual(m._sha(m._canon(value)), fingerprint(value))

    def test_numeric_cursor_is_not_lexical(self):
        self.assertLess(m._typed(b"9","bigint unsigned"),m._typed(b"10","bigint unsigned"))
        for invalid in (b"-1",b"01",b"1.0",None):
            with self.subTest(value=invalid),self.assertRaisesRegex(m.Rejected,"invalid_numeric_cursor"):
                m._typed(invalid,"bigint unsigned")

    def test_json_duplicate_and_nonfinite_are_rejected(self):
        for raw in (b'{"a":1,"a":2}',b'{"a":NaN}',b'{"a":Infinity}'):
            with self.subTest(raw=raw), self.assertRaises(m.Rejected):m._json(raw)

    def test_body_hashing_leaves_no_free_text(self):
        row=full_row("interpretation_sessions",id="private-id",goal="FAKE_PRIVATE_SECRET",assessment_ids='["3"]')
        minimal=m._minimal("interpretation_sessions",row)
        self.assertNotIn("FAKE_PRIVATE_SECRET",json.dumps(minimal));self.assertEqual(minimal["digests"]["goal"],m._sha(b"FAKE_PRIVATE_SECRET"))

    def test_idempotency_receipt_requires_known_shape(self):
        row=full_row("idempotency_requests",response='{"complete":true,"secret":"FAKE_PRIVATE_SECRET"}')
        with self.assertRaisesRegex(m.Rejected,"unsupported_original_receipt"):m._minimal("idempotency_requests",row)

    def test_protobuf_inner_kind_and_source_hash_are_independent(self):
        from qs_ai.contracts.workflow import messaging_pb2 as pb
        body=pb.MessagingBody(start={"request_id":str(UUID(int=1))})
        raw=body.SerializeToString(deterministic=True)
        row=full_row("ai_messaging_inbox",kind=1,body_sha256=m._sha(raw))
        row["body"]=raw
        self.assertEqual(m._minimal("ai_messaging_inbox",row)["body_refs"]["request_id"],str(UUID(int=1)))
        row["kind"]=b"2"
        with self.assertRaisesRegex(m.Rejected,"inner_outer_message_kind_conflict"):m._minimal("ai_messaging_inbox",row)
        row["kind"]=b"1";row["body_sha256"]=b"a"*64
        with self.assertRaisesRegex(m.Rejected,"stored_body_hash_conflict"):m._minimal("ai_messaging_inbox",row)

    def test_quarantine_wire_hash_not_assumed_trusted(self):
        row=full_row("ai_messaging_quarantine",wire_sha256="a"*64,wire="FAKE_PRIVATE_SECRET")
        with self.assertRaisesRegex(m.Rejected,"stored_wire_hash_conflict"):m._minimal("ai_messaging_quarantine",row)

    def test_exact_eight_and_canonical_original_identity(self):
        rows,targets=fixture()
        for invalid in (targets[:7],list(targets)):
            with self.subTest(case=type(invalid).__name__),self.assertRaisesRegex(m.Rejected,"exact_eight_start_scope_required"):m.analyze(rows,invalid)
        changed=copy.deepcopy(targets);changed[0]["command_id"]=str(uuid4())
        with self.assertRaisesRegex(m.Rejected,"original_start_scope_binding_required"):m.analyze(rows,changed)
        changed=copy.deepcopy(targets);changed[0]["session_id"]="00000000-0000-0000-0000-000000000000"
        with self.assertRaisesRegex(m.Rejected,"canonical_original_identity_required"):m.analyze(rows,changed)

    def test_targets_have_no_unknown_approval_or_complete_flag(self):
        rows,targets=fixture();changed=copy.deepcopy(targets);changed[0]["complete"]=True
        with self.assertRaisesRegex(m.Rejected,"original_start_scope_binding_required"):m.analyze(rows,changed)

    def test_duplicate_starts_and_invalid_business_identity(self):
        rows,targets=fixture();changed=list(copy.deepcopy(targets));changed[1]=changed[0].copy();changed=tuple(changed)
        with self.assertRaisesRegex(m.Rejected,"distinct_eight_original_starts_required"):m.analyze(rows,changed)
        for key,value in (("organization_id","01"),("assessment_ids",["3","3"]),("source_row_sha256","X"*64)):
            changed=copy.deepcopy(targets);changed[0][key]=value
            with self.subTest(key=key),self.assertRaises(m.Rejected):m.analyze(rows,changed)

    def test_delivered_and_completed_never_become_retirement_proof(self):
        rows,targets=fixture();out=m.analyze(rows,targets)
        self.assertFalse(out["drop_ready"]);self.assertFalse(out["business_retirement_proven"])
        self.assertTrue(all(not t["retirement_proven"] for t in out["targets"]))
        public=json.dumps(out)
        for t in targets:
            self.assertNotIn(t["command_id"],public);self.assertNotIn(t["subject_id"],public)

    def test_global_orphan_not_hidden_by_target_org(self):
        rows,targets=fixture();rows["execution_jobs"].append(dict(id="orphan",run_id="missing",session_id="foreign",status="done"))
        out=m.analyze(rows,targets)
        self.assertEqual(out["global_diagnostic_counts"]["orphan"],1)
        self.assertEqual(out["global_diagnostic_counts"]["binding_conflict"],1)

    def test_cancel_does_not_clear_provider_unknown(self):
        rows,targets=fixture();rows["interpretation_sessions"][0]["status"]="cancelled"
        rows["model_calls"][0]["status"]="unknown"
        out=m.analyze(rows,targets)
        self.assertIn("provider_execution_unknown",out["targets"][0]["blocking_categories"])

    def test_queue_claim_and_active_slot_block(self):
        rows,targets=fixture();rows["execution_jobs"][0]["status"]="leased"
        rows["participant_capacity_reservations"].append(dict(session_id=targets[0]["session_id"],run_id=rows["interpretation_runs"][0]["id"],organization_id="1",subject_id="fixture-subject",assessment_ids=["3"],active="1"))
        issues=m.analyze(rows,targets)["targets"][0]["blocking_categories"]
        self.assertIn("unfinished_execution",issues);self.assertIn("active_capacity_reservation",issues)

    def test_cross_org_and_changed_request_hash_block(self):
        rows,targets=fixture();rows["interpretation_sessions"][0]["org_id"]="999"
        rows["idempotency_requests"][0]["request_hash"]="b"*64
        issues=m.analyze(rows,targets)["targets"][0]["blocking_categories"]
        self.assertIn("business_ownership_conflict",issues);self.assertIn("original_start_receipt_unverified",issues)

    def test_result_original_actor_and_identity_are_checked(self):
        rows,targets=fixture();t=targets[0]
        event=dict(event_id="e",session_id=t["session_id"],version="2",delivered="1",event_actor={"org_id":"999","subject_id":t["subject_id"]},event_refs=dict(event_id="e",session_id=t["session_id"],request_id=t["request_id"],version=2,testee_id=t["testee_id"]))
        rows["result_outbox"].append(event)
        self.assertIn("legacy_result_binding_conflict",m.analyze(rows,targets)["targets"][0]["blocking_categories"])

    def test_mq_held_and_missing_original_receipt_block(self):
        rows,targets=fixture();t=targets[0]
        rows["ai_messaging_inbox"].append(dict(aggregate_key=t["request_id"],message_id=t["command_id"],decision="held",receipt_id=None,body_sha256="a"*64,body_refs={}))
        issues=m.analyze(rows,targets)["targets"][0]["blocking_categories"]
        self.assertIn("mq_command_processing_or_held",issues);self.assertIn("mq_command_receipt_orphan_or_conflicting",issues)

    async def test_page_boundary_and_complete_digest(self):
        raw=[(f"fixture-{i:04d}".encode(),b"1",b"2026-01-01 00:00:00.000000") for i in range(1001)]
        reader=PageReader(raw);bound=lease_bound(raw[-1][0])
        section,rows=await m._pass(reader,"execution_leases",bound,retain=True)
        self.assertEqual((section["rows"],section["pages"],len(rows)),(1001,2,1001))
        second,_=await m._pass(PageReader(raw),"execution_leases",bound,retain=False)
        self.assertEqual(section,second)
        self.assertEqual(reader.calls[2][1]["l0"],"fixture-0999")

    async def test_exact_page_needs_exhaustion_query(self):
        raw=[(f"fixture-{i:04d}".encode(),b"1",None) for i in range(1000)]
        section,_=await m._pass(PageReader(raw),"execution_leases",lease_bound(raw[-1][0]),retain=False)
        self.assertEqual(section["pages"],2)

    async def test_present_empty_is_read_not_skipped(self):
        reader=PageReader([])
        section,_=await m._pass(reader,"execution_leases",lease_bound(None),retain=False)
        self.assertEqual((section["rows"],section["pages"],len(reader.calls)),(0,1,2))
        with self.assertRaisesRegex(m.Rejected,"approved_empty_table_changed"):
            await m._pass(PageReader([(b"new",b"1",None)]),"execution_leases",lease_bound(None),retain=False)

    async def test_row_and_byte_caps_fail_closed(self):
        raw=[(b"a",b"1",None),(b"b",b"1",None)]
        for name,value in (("MAX_ROWS",1),("MAX_BYTES",1)):
            reader=PageReader(raw)
            with self.subTest(cap=name),patch.object(m,name,value),self.assertRaisesRegex(m.Rejected,"fixed_scan_budget_exceeded"):
                await m._pass(reader,"execution_leases",lease_bound(b"b"),retain=False)
            self.assertEqual(len(reader.calls),1)
            self.assertIn(" AS _source_row_bytes ",reader.calls[0][0])

    async def test_graph_budget_cannot_retain_unbounded_private_facts(self):
        with patch.object(m,"MAX_GRAPH_BYTES",1),self.assertRaisesRegex(m.Rejected,"fixed_minimal_graph_budget_exceeded"):
            await m._pass(PageReader([(b"a",b"1",None)]),"execution_leases",lease_bound(b"a"),retain=True)

    async def test_preview_and_actual_page_must_agree(self):
        class Changed(PageReader):
            async def query(self,sql,params):
                out=await super().query(sql,params)
                return out if " AS _source_row_bytes " in sql else [(b"changed",b"1",None)]
        with self.assertRaisesRegex(m.Rejected,"source_page_changed_after_length_check"):
            await m._pass(Changed([(b"a",b"1",None)]),"execution_leases",lease_bound(b"a"),retain=False)

    async def test_invalid_private_cursor_is_fixed_category(self):
        for upper in ([],["FAKE_PRIVATE_SECRET"],["YQ==","Yg=="],"YQ=="):
            approved=lease_bound(b"a");approved["upper"]=upper
            with self.subTest(shape=type(upper).__name__),self.assertRaisesRegex(m.Rejected,"unsupported_approved_cursor"):
                await m._pass(PageReader([]),"execution_leases",approved,retain=False)

    async def test_imported_bounds_complete_flag_has_no_authority(self):
        with self.assertRaisesRegex(m.Rejected,"independent_bounds_approval_required"):
            await m.observe(object(),{"complete":True},approved_bounds_sha256="a"*64)
        b=m.PrivateBounds("a"*64,"b"*40,m.HEAD,{})
        with self.assertRaisesRegex(m.Rejected,"independent_bounds_approval_required"):
            await m.observe(object(),b,approved_bounds_sha256=b.digest())

    def test_wrong_session_rejected_without_error_data(self):
        for s in (None,object(),{"password":"FAKE_PRIVATE_SECRET","active":True}):
            with self.subTest(kind=type(s).__name__),self.assertRaisesRegex(m.Rejected,"original_async_session_required"):
                m._Borrowed(s)

    def test_receipt_is_hashes_counts_and_explicit_gaps(self):
        b=m.PrivateBounds("a"*64,"b"*40,m.HEAD,{})
        with self.assertRaisesRegex(m.Rejected,"observed_scan_result_required"):
            m.PrivateObservation(b,{"complete":True},[],object())
        observation=m.PrivateObservation(b,{},[{"secret":"FAKE_PRIVATE_SECRET"}],object(),_seal=m._OBSERVATION_SEAL)
        public=observation.receipt()
        self.assertNotIn("FAKE_PRIVATE_SECRET",json.dumps(public))
        self.assertFalse(public["business_retirement_proven"])
        self.assertEqual(public["qs_ai_runtime_source_binding"],"not_observed")
        self.assertEqual(public["server_readonly_snapshot_mode"],"host_contract_not_independently_observed")
        self.assertNotIn("a"*10,repr(b))


@unittest.skipUnless(os.environ.get("QS_AI_RETIREMENT_OWNED_NATIVE")=="1","owned loopback native fixture is opt-in")
class OwnedNative(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        from sqlalchemy import text
        from sqlalchemy.engine import URL
        from sqlalchemy.ext.asyncio import create_async_engine
        from qs_ai.infrastructure.persistence.mysql.database import Base
        from qs_ai.infrastructure.persistence.mysql import schema  # loads source metadata, opens nothing
        self.name="qs_ai_retirement_observer_"+uuid4().hex
        self.admin=self.engine=None
        try:
            private=Path("/private/tmp/qs-compatibility-retirement")
            manifest=json.loads((private/"owned-mysql.json").read_text())
            env=json.loads((private/"mysql-native.env.json").read_text())
            if not (manifest["ready"] is True and str(manifest["loopback_port"])=="34306" and env["MYSQL_HOST"]=="127.0.0.1" and str(env["MYSQL_PORT"])=="34306"):
                raise ValueError("fixture boundary")
            raw=subprocess.run(["docker","inspect",manifest["container_id"]],capture_output=True,check=True,timeout=10)
            actual=json.loads(raw.stdout)[0]
            ports=actual["NetworkSettings"]["Ports"].get("3306/tcp",[])
            volumes=sorted(v.get("Name") for v in actual["Mounts"] if v.get("Type")=="volume")
            if not (actual["Id"]==manifest["container_id"] and actual["Name"].lstrip("/")==manifest["container_name"] and actual["Image"]==manifest["image_id"] and actual["Config"]["Labels"]==manifest["labels"] and volumes==sorted(manifest["volumes"]) and ports==[{"HostIp":"127.0.0.1","HostPort":"34306"}]):
                raise ValueError("fixture identity")
            url=URL.create("mysql+asyncmy",username=env["MYSQL_USERNAME"],password=env["MYSQL_PASSWORD"],host="127.0.0.1",port=34306)
            self.admin=create_async_engine(url,hide_parameters=True,echo=False)
            async with self.admin.begin() as c:
                await c.execute(text("CREATE DATABASE `"+self.name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"))
            self.engine=create_async_engine(url.set(database=self.name),hide_parameters=True,echo=False,isolation_level="REPEATABLE READ")
            async with self.engine.begin() as c:
                await c.run_sync(Base.metadata.create_all)
                path=Path(schema.__file__).parents[5]/"migrations/versions/0037_workflow_messaging.py"
                tree=ast.parse(path.read_text())
                ddl=next(ast.literal_eval(n.value) for n in tree.body if isinstance(n,ast.Assign) and any(isinstance(t,ast.Name) and t.id=="DDL" for t in n.targets))
                for statement in ddl:await c.execute(text(statement))
                await c.execute(text("CREATE TABLE alembic_version(version_num VARCHAR(64) PRIMARY KEY) ENGINE=InnoDB"))
                await c.execute(text("INSERT INTO alembic_version VALUES (:head)"),{"head":m.HEAD})
                bound=(await c.execute(text("SELECT CAST(@@server_uuid AS BINARY),CAST(DATABASE() AS BINARY)"))).one()
                self.identity=m._identity(*bound)
        except Exception:
            await self._clean()
            raise AssertionError("owned_native_fixture_setup_failed") from None

    async def _clean(self):
        from sqlalchemy import text
        if self.engine is not None:await self.engine.dispose();self.engine=None
        if self.admin is not None:
            try:
                async with self.admin.begin() as c:await c.execute(text("DROP DATABASE IF EXISTS `"+self.name+"`"))
            finally:await self.admin.dispose();self.admin=None

    async def asyncTearDown(self):await self._clean()

    async def host_snapshot(self):
        from sqlalchemy import text
        from sqlalchemy.ext.asyncio import AsyncSession
        session=AsyncSession(self.engine)
        await session.connection(execution_options={"isolation_level":"REPEATABLE READ"})
        await session.execute(text("START TRANSACTION WITH CONSISTENT SNAPSHOT, READ ONLY"))
        return session

    async def test_native_empty_schema_bound_scan_and_borrowed_ownership(self):
        from sqlalchemy import text
        s=await self.host_snapshot()
        try:
            original=s.get_transaction()
            bounds=await m.discover_bounds(s,identity_hash=self.identity,source_sha="b"*40)
            observation=await m.observe(s,bounds,approved_bounds_sha256=bounds.digest())
            receipt=observation.receipt()
            self.assertTrue(all(t["rows"]==0 and t["complete"] for t in receipt["sections"].values()))
            self.assertIs(original,s.get_transaction());self.assertTrue(original.is_active)
            self.assertFalse(receipt["drop_ready"])
            with self.assertRaisesRegex(m.Rejected,"original_snapshot_must_end_before_fresh_observation"):
                await m.fresh_after_upper(s,observation)
            with self.assertRaises(Exception):await s.execute(text("INSERT INTO execution_leases VALUES ('host-write-probe',1,UTC_TIMESTAMP(6))"))
            # Host handles the failed write probe; helper owns no rollback.
        finally:await s.rollback();await s.close()
        self.assertFalse(original.is_active)

    async def test_native_pages_upper_bound_and_fresh_after_snapshot(self):
        from sqlalchemy import text
        async with self.engine.begin() as c:
            await c.execute(text("INSERT INTO execution_leases(thread_id,fence,expires_at) VALUES (:id,1,UTC_TIMESTAMP(6))"),[{"id":f"fixture-{i:04d}"} for i in range(1001)])
            await c.execute(text("INSERT INTO participant_retries(organization_id,command_id,session_id,request_id,source_run_id,run_id,operator_user_id,expected_version,reason,accepted_unknown_risk,receipt) VALUES (:org,:cid,:sid,:request,:source,:run,1,2,'synthetic-fixture',0,'{}')"),
                            [{"org":n,"cid":str(UUID(int=n*10+1)),"sid":str(UUID(int=n*10+2)),"request":str(UUID(int=n*10+3)),"source":str(UUID(int=n*10+4)),"run":str(UUID(int=n*10+5))} for n in (9,10)])
        s=await self.host_snapshot()
        try:
            bounds=await m.discover_bounds(s,identity_hash=self.identity,source_sha="b"*40)
            async with self.engine.begin() as c:
                await c.execute(text("INSERT INTO execution_leases VALUES ('fixture-9999',1,UTC_TIMESTAMP(6))"))
            observation=await m.observe(s,bounds,approved_bounds_sha256=bounds.digest())
            section=observation.receipt()["sections"]["execution_leases"]
            self.assertEqual((section["rows"],section["pages"],section["equal_full_passes"]),(1001,2,2))
            self.assertEqual([r["organization_id"] for r in observation._rows["participant_retries"]],["9","10"])
        finally:await s.rollback();await s.close()
        fresh=await self.host_snapshot()
        try:
            report=await m.fresh_after_upper(fresh,observation)
            self.assertTrue(report["execution_leases"]["next_cycle_required"])
        finally:await fresh.rollback();await fresh.close()

    async def test_native_schema_source_head_inactive_nested_wrong_identity(self):
        from sqlalchemy import text
        from sqlalchemy.ext.asyncio import AsyncSession
        s=AsyncSession(self.engine)
        try:
            with self.assertRaisesRegex(m.Rejected,"original_active_mysql_root_transaction_required"):m._Borrowed(s)
        finally:await s.close()
        s=await self.host_snapshot()
        try:
            with self.assertRaisesRegex(m.Rejected,"database_binding_rejected"):
                await m.discover_bounds(s,identity_hash="a"*64,source_sha="b"*40)
            reader=m._Borrowed(s)
            async with s.begin_nested():
                with self.assertRaisesRegex(m.Rejected,"original_active_mysql_root_transaction_required"):m._Borrowed(s)
                with self.assertRaisesRegex(m.Rejected,"original_transaction_changed"):reader.validate()
            bounds=await m.discover_bounds(s,identity_hash=self.identity,source_sha="b"*40)
            with self.assertRaisesRegex(m.Rejected,"independent_bounds_approval_required"):
                await m.observe(s,bounds,approved_bounds_sha256="a"*64)
        finally:await s.rollback();await s.close()
        async with self.engine.begin() as c:await c.execute(text("ALTER TABLE execution_leases ADD extra_field INT"))
        s=await self.host_snapshot()
        try:
            with self.assertRaisesRegex(m.Rejected,"unsupported_complete_table_schema"):
                await m.observe(s,bounds,approved_bounds_sha256=bounds.digest())
        finally:await s.rollback();await s.close()

    async def test_native_transaction_deadline_driver_timeout_and_privacy(self):
        s=await self.host_snapshot()
        try:
            reader=m._Borrowed(s)
            with self.assertRaisesRegex(m.Rejected,"nonreadonly_query_rejected"):
                await reader.query("UPDATE interpretation_runs SET status='cancelled'")
            with patch.object(s,"execute",AsyncMock(side_effect=RuntimeError("FAKE_PRIVATE_SECRET"))):
                try:await reader.query("SELECT 1")
                except m.Rejected as error:self.assertEqual(str(error),"readonly_query_failed")
                else:self.fail("driver error was accepted")
            async def slow(*args,**kwargs):await asyncio.sleep(.05)
            with patch.object(s,"execute",slow),patch.object(m,"QUERY_SECONDS",.005),self.assertRaisesRegex(m.Rejected,"readonly_query_timeout"):
                await reader.query("SELECT 1")
            reader.deadline=0
            with self.assertRaisesRegex(m.Rejected,"scan_deadline_exceeded"):reader.validate()
            changed=m._Borrowed(s)
            await s.rollback()
            with self.assertRaisesRegex(m.Rejected,"original_transaction_changed"):changed.validate()
        finally:await s.rollback();await s.close()

    async def test_native_dirty_contract_head_engine_and_sorting_rejected(self):
        from sqlalchemy import text
        async with self.engine.begin() as c:await c.execute(text("UPDATE alembic_version SET version_num='unsupported-local-head'"))
        s=await self.host_snapshot()
        try:
            with self.assertRaisesRegex(m.Rejected,"unsupported_alembic_head"):
                await m.discover_bounds(s,identity_hash=self.identity,source_sha="b"*40)
        finally:await s.rollback();await s.close()
        async with self.engine.begin() as c:
            await c.execute(text("UPDATE alembic_version SET version_num=:head"),{"head":m.HEAD})
            await c.execute(text("ALTER TABLE execution_leases MODIFY thread_id VARCHAR(191) COLLATE utf8mb4_0900_ai_ci NOT NULL"))
        s=await self.host_snapshot()
        try:
            with self.assertRaisesRegex(m.Rejected,"unsupported_primary_key_type"):
                await m.discover_bounds(s,identity_hash=self.identity,source_sha="b"*40)
        finally:await s.rollback();await s.close()
        async with self.engine.begin() as c:await c.execute(text("ALTER TABLE execution_leases ENGINE=MyISAM"))
        s=await self.host_snapshot()
        try:
            with self.assertRaisesRegex(m.Rejected,"transactional_source_table_required"):
                await m.discover_bounds(s,identity_hash=self.identity,source_sha="b"*40)
        finally:await s.rollback();await s.close()


if __name__=="__main__":unittest.main()


class Observer0040(unittest.IsolatedAsyncioTestCase):
    def test_fixed_0040_registry_does_not_replace_legacy_specs(self):
        layout=m._layout()
        self.assertEqual(len(layout.CONTRACT),43);self.assertEqual(len(m._specs(layout.HEAD)),44)
        self.assertEqual(len(m.SPECS),18);self.assertEqual(m._specs(m.HEAD),m.SPECS)
        self.assertEqual(layout.SOURCE_SHA,"82ffa1b43308f23fbb1ebe669c3071e0486e105a")
        self.assertEqual(layout.CONTRACT_SHA256,"1920802a64a4410fbd863873231d69694bc92b1ed36102191fc922975e837e92")

    def test_bounds_head_and_complete_physical_scope_covered_by_approval_hash(self):
        layout=m._layout();tables={n:{"upper":None} for n in layout.SPECS}
        bound=m.PrivateBounds("a"*64,layout.SOURCE_SHA,layout.HEAD,tables)
        bad=dict(tables);bad.pop("alembic_version")
        self.assertNotEqual(bound.digest(),m.PrivateBounds("a"*64,layout.SOURCE_SHA,layout.HEAD,bad).digest())
        self.assertNotEqual(bound.digest(),m.PrivateBounds("a"*64,layout.SOURCE_SHA,m.HEAD,tables).digest())
        with self.assertRaises(m.Rejected):m.PrivateObservation(bound,{}, {},None,_seal=object())

    async def test_new_layout_keyset_reads_every_raw_column_before_projection(self):
        layout=m._layout();table="governance_asset_versions";spec=layout.SPECS[table]
        names=spec[0].split();cols=[(name,"bigint unsigned" if name=="asset_row_id" else "text","YES",None,"",None) for name in names]
        cells=[None]*len(names);cells[names.index("asset_row_id")]=b"1";cells[names.index("body_bytes")]=b"raw-byte-unchanged"
        class PhysicalReader:
            head=layout.HEAD;specs=layout.SPECS
            def __init__(self):self.calls=[]
            async def query(self,sql,params):
                self.calls.append(sql)
                return [(b"1",sum(len(c) for c in cells if c is not None))] if " AS _source_row_bytes " in sql else [tuple(cells)]
        reader=PhysicalReader();bound=dict(columns=cols,columns_sha256="a"*64,kinds={"asset_row_id":"bigint unsigned"},upper=[base64.b64encode(b"1").decode()])
        section,rows=await m._pass(reader,table,bound,retain=True)
        self.assertEqual(section["rows"],1);self.assertEqual(rows[0]["body_bytes"],b"raw-byte-unchanged")
        self.assertEqual(set(rows[0]),set(names));self.assertEqual(len(reader.calls),2)
        self.assertFalse(any(" JOIN " in sql or "asset_kind=" in sql for sql in reader.calls))
        self.assertTrue(all("CAST(`"+name+"` AS BINARY)" in reader.calls[1] for name in names))

    def test_adapter_and_observer_remain_no_io_no_lifecycle_no_execution_authorization(self):
        paths=(PATH,PATH.with_name("qs-ai-retirement-0040-layout.py"))
        for path in paths:
            tree=ast.parse(path.read_text())
            for n in ast.walk(tree):
                if isinstance(n,ast.Call):
                    name=n.func.attr if isinstance(n.func,ast.Attribute) else getattr(n.func,"id","")
                    self.assertNotIn(name,{"create_async_engine","commit","rollback","begin","send","publish","connect","open_connection"})
