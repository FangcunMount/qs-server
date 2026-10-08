"""Finite diagnostic qs-ai observer; the host owns connections and transactions.

There is deliberately no CLI, connection factory, DML, recovery, proof import,
message send, or final retirement verdict. Private source bytes stay in memory.
"""
from __future__ import annotations

import asyncio
import base64
import hashlib
import json
import re
import struct
import time
from dataclasses import dataclass
from uuid import UUID

PAGE = 1000
MAX_ROWS = 1_000_000
MAX_BYTES = 2 << 30
MAX_GRAPH_BYTES = 256 << 20
QUERY_SECONDS = 30
TOTAL_SECONDS = 1500
HEAD = "0038_messaging_observations"
_OBSERVATION_SEAL = object()

# Complete current named columns; ordinal/type/default/collation metadata and
# SHOW CREATE bytes are additionally frozen by the independently approved bound.
# Declaration order is not assumed to equal historical Alembic append order.
SPECS = {
    "external_requests": ("request_id session_id", "request_id"),
    "interpretation_sessions": ("id org_id owner_subject_id testee_id assessment_ids goal status version active_run_id current_question_id evidence_set_id workflow_version failure_code created_at updated_at created_at_utc updated_at_utc", "id"),
    "idempotency_requests": ("scope_hash key request_hash response created_at", "scope_hash key"),
    "interpretation_runs": ("id session_id session_version status checkpoint_ref", "id"),
    "execution_jobs": ("id run_id session_id status available_at lease_until fence_token attempt answer skipped question_id", "id"),
    "model_calls": ("run_id invocation_id fence_token status request_json response_json failure_code created_at created_at_utc", "run_id"),
    "clarifications": ("id session_id question_seq text can_skip answer skipped answered_by answered_at", "id"),
    "evidence_sets": ("id session_id fingerprint schema_version items frozen_at", "id"),
    "interpretation_artifacts": ("id session_id run_id payload created_at", "id"),
    "execution_configurations": ("session_id evidence_set_id evidence_fingerprint publication_id publication_sha256 pointer_version selector_query", "session_id"),
    "participant_capacity_reservations": ("quota_snapshot run_id session_id organization_id subject_id assessment_ids budget_day reserved_at active acquired_at released_at", "run_id"),
    "participant_retries": ("organization_id command_id session_id request_id source_run_id run_id operator_user_id expected_version reason accepted_unknown_risk source_failure_code frozen_request_json receipt created_at", "organization_id command_id"),
    "execution_leases": ("thread_id fence expires_at", "thread_id"),
    "result_outbox": ("event_id session_id version payload delivered mq_owned attempts created_at delivered_at available_at", "event_id"),
    "ai_messaging_outbox": ("producer destination message_id body_sha256 body wire wire_sha256 topic kind organization_id aggregate_key aggregate_sequence ordered requires_receipt stage attempts available_at created_at published_at confirmed_at error_code", "producer destination message_id"),
    "ai_messaging_inbox": ("producer message_id destination body_sha256 body wire_sha256 kind aggregate_key reservation_token decision receipt_id received_at", "producer message_id"),
    "ai_messaging_quarantine": ("wire_sha256 wire code logical_producer logical_message_id logical_body_sha256 attempts first_seen_at last_seen_at", "wire_sha256"),
    "runtime_milestones": ("session_id dedupe_key run_id kind invocation_id attempt occurred_at expires_at", "session_id dedupe_key"),
}


class Rejected(Exception):
    """Only fixed categories; never wrap a driver, SQL, body or credential error."""


def _fail(category):
    raise Rejected(category) from None


def _json(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                _fail("duplicate_json_field")
            result[key] = value
        return result
    try:
        return json.loads(raw, object_pairs_hook=pairs,
                          parse_constant=lambda _: _fail("nonfinite_json"))
    except Rejected:
        raise
    except Exception:
        _fail("unsupported_json")


def _canon(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True,
                      separators=(",", ":")).encode()


def _sha(raw):
    return hashlib.sha256(raw).hexdigest()


def _frame(h, raw):
    h.update(b"\x00" + struct.pack(">Q", 0) if raw is None else
             b"\x01" + struct.pack(">Q", len(raw)) + raw)


def _identity(uuid, name):
    h = hashlib.sha256()
    for part in (b"mysql_database_identity_v1", uuid, name):
        _frame(h, part)
    return h.hexdigest()


def _text(raw):
    try:
        return None if raw is None else raw.decode("utf-8", errors="strict")
    except Exception:
        _fail("invalid_text_fact")


def _typed(raw, kind):
    value = _text(raw)
    if kind in ("bigint unsigned", "bigint", "int", "integer", "tinyint unsigned"):
        if value is None or not re.fullmatch(r"0|[1-9][0-9]*", value):
            _fail("invalid_numeric_cursor")
        return int(value)
    if value is None:
        _fail("missing_primary_key")
    return value


@dataclass(frozen=True, repr=False)
class PrivateBounds:
    identity_hash: str
    source_sha: str
    head: str
    tables: dict

    def __repr__(self):
        return "<private qs-ai readonly bounds>"

    def private_bytes(self):
        return _canon({"protocol": "qs-ai-retirement-bounds/v1",
                       "identity_hash": self.identity_hash, "source_sha": self.source_sha,
                       "head": self.head, "profile": [PAGE, MAX_ROWS, MAX_BYTES, MAX_GRAPH_BYTES, QUERY_SECONDS, TOTAL_SECONDS],
                       "tables": self.tables})

    def digest(self):
        return _sha(self.private_bytes())


class PrivateObservation:
    def __init__(self, bounds, sections, rows, original_tx, *, _seal=None):
        if _seal is not _OBSERVATION_SEAL:
            _fail("observed_scan_result_required")
        self._bounds, self._sections, self._rows, self._original_tx = bounds, sections, rows, original_tx

    def __repr__(self):
        return "<private qs-ai readonly observation>"

    def receipt(self):
        return {"protocol": "qs-ai-retirement-observation/v1", "diagnostic_only": True,
                "database_identity_hash": self._bounds.identity_hash,
                "qs_ai_source_sha": self._bounds.source_sha,
                "schema_head": self._bounds.head, "approved_bounds_sha256": self._bounds.digest(),
                "sections": self._sections, "database_bound_scan_complete": True,
                "business_retirement_proven": False, "drop_ready": False,
                "fence_verified": False, "peer_receipts_verified": False,
                "message_reauthentication": "not_performed",
                "qs_ai_runtime_source_binding": "not_observed",
                "server_readonly_snapshot_mode": "host_contract_not_independently_observed",
                "catalogue_coverage": "closed_18_responsibility_tables",
                "other_business_tables_semantic_scope": "not_qualified",
                "fresh_after_upper_observation": "host_required",
                "milestone_history": "retained_diagnostics_only"}

    def facts(self, targets):
        return analyze(self._rows, targets)


class _Borrowed:
    def __init__(self, session):
        # Real SQLAlchemy transaction validation, never a caller's complete flag.
        from sqlalchemy.ext.asyncio import AsyncSession
        if not isinstance(session, AsyncSession):
            _fail("original_async_session_required")
        self.session, self.tx = session, session.get_transaction()
        if (self.tx is None or not self.tx.is_active or session.get_nested_transaction() is not None
                or session.get_bind().dialect.name != "mysql"):
            _fail("original_active_mysql_root_transaction_required")
        self.deadline = time.monotonic() + TOTAL_SECONDS

    def validate(self):
        if (not self.tx.is_active or self.session.get_transaction() is not self.tx
                or self.session.get_nested_transaction() is not None):
            _fail("original_transaction_changed")
        if time.monotonic() >= self.deadline:
            _fail("scan_deadline_exceeded")

    async def query(self, sql, params=None):
        self.validate()
        from sqlalchemy import text
        # Queries are produced exclusively from the closed SPECS and fixed
        # metadata statements. No user SQL or identifier is accepted.
        if not sql.startswith(("SELECT ", "SHOW CREATE TABLE ")):
            _fail("nonreadonly_query_rejected")
        try:
            async with asyncio.timeout(min(QUERY_SECONDS, self.deadline - time.monotonic())):
                result = await self.session.execute(text(sql), params or {})
                try:
                    rows = [tuple(v) for v in result.fetchall()]
                finally:
                    result.close()
        except Rejected:
            raise
        except TimeoutError:
            _fail("readonly_query_timeout")
        except Exception:
            _fail("readonly_query_failed")
        self.validate()
        return rows


async def _binding(reader, identity, source):
    if not re.fullmatch(r"[0-9a-f]{64}", identity) or not re.fullmatch(r"[0-9a-f]{40}", source):
        _fail("explicit_source_identity_binding_required")
    row = await reader.query("SELECT CAST(@@server_uuid AS BINARY),CAST(DATABASE() AS BINARY),VERSION(),@@transaction_isolation")
    if len(row) != 1 or row[0][0] is None or row[0][1] is None or _identity(row[0][0], row[0][1]) != identity or not row[0][2].startswith("8.") or row[0][3] != "REPEATABLE-READ":
        _fail("database_binding_rejected")
    head = await reader.query("SELECT version_num FROM alembic_version ORDER BY version_num")
    if head != [(HEAD,)]:
        _fail("unsupported_alembic_head")


async def _schema(reader, table):
    engine = await reader.query("SELECT ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=:name", {"name": table})
    if engine != [("InnoDB",)]:
        _fail("transactional_source_table_required")
    columns = await reader.query("SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=:name ORDER BY ORDINAL_POSITION", {"name": table})
    if set(c[0] for c in columns) != set(SPECS[table][0].split()) or len(columns) != len(SPECS[table][0].split()):
        _fail("unsupported_complete_table_schema")
    keys = await reader.query("SELECT COLUMN_NAME FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=:name AND index_name='PRIMARY' ORDER BY SEQ_IN_INDEX", {"name": table})
    if [c[0] for c in keys] != SPECS[table][1].split():
        _fail("unsupported_primary_key")
    ddl = await reader.query("SHOW CREATE TABLE `" + table + "`")
    if len(ddl) != 1 or len(ddl[0]) != 2 or ddl[0][0] != table:
        _fail("missing_source_table")
    kinds = {}
    for c in columns:
        if c[0] in SPECS[table][1].split():
            if (not (c[1] in ("bigint unsigned", "bigint", "int", "integer", "tinyint unsigned")
                     or (re.fullmatch(r"(?:var)?char\([0-9]+\)", c[1]) and c[5] in ("ascii_bin", "utf8mb4_bin")))):
                _fail("unsupported_primary_key_type")
            kinds[c[0]] = c[1]
    return {"columns": columns, "kinds": kinds, "ddl_sha256": _sha(_canon(ddl)),
            "columns_sha256": _sha(_canon(columns))}


def _pk_sql(table):
    return ",".join("`" + k + "`" for k in SPECS[table][1].split())


def _upper_values(approved, keys):
    upper = approved["upper"]
    if upper is None:
        return None
    if not isinstance(upper, list) or len(upper) != len(keys):
        _fail("unsupported_approved_cursor")
    try:
        return [_typed(base64.b64decode(v, validate=True), approved["kinds"][k])
                for v, k in zip(upper, keys)]
    except Rejected:
        raise
    except Exception:
        _fail("unsupported_approved_cursor")


async def discover_bounds(session, *, identity_hash, source_sha):
    """Independent diagnostic discovery; discovery is never approval."""
    reader = _Borrowed(session)
    await _binding(reader, identity_hash, source_sha)
    tables = {}
    for table in SPECS:
        schema = await _schema(reader, table)
        keys = SPECS[table][1].split()
        upper = await reader.query("SELECT " + ",".join("CAST(`"+k+"` AS BINARY)" for k in keys) + " FROM `"+table+"` ORDER BY " + ",".join("`"+k+"` DESC" for k in keys) + " LIMIT 1")
        schema["upper"] = [base64.b64encode(v).decode() for v in upper[0]] if upper else None
        tables[table] = schema
    await _binding(reader, identity_hash, source_sha)
    return PrivateBounds(identity_hash, source_sha, HEAD, tables)


async def _pass(reader, table, approved, *, retain, retained_budget=None):
    keys = SPECS[table][1].split()
    columns = [c[0] for c in approved["columns"]]
    upper = approved["upper"]
    upper_values = _upper_values(approved, keys)
    select = "SELECT " + ",".join("CAST(`"+c+"` AS BINARY)" for c in columns) + " FROM `"+table+"`"
    preview_select = "SELECT " + ",".join("CAST(`"+k+"` AS BINARY)" for k in keys) + "," + "+".join("COALESCE(OCTET_LENGTH(CAST(`"+c+"` AS BINARY)),0)" for c in columns) + " AS _source_row_bytes FROM `"+table+"`"
    h = hashlib.sha256(); _frame(h, approved["columns_sha256"].encode())
    count = size = pages = 0; last = None; minimal = []
    retained_budget = [0] if retained_budget is None else retained_budget
    while True:
        params = {"page": PAGE}
        clauses = []
        if upper is None:
            clauses.append("1=1")  # Must query exact present-empty proof, not skip.
        else:
            for i, k in enumerate(keys):
                params["u"+str(i)] = upper_values[i]
            clauses.append("("+_pk_sql(table)+") <= ("+",".join(":u"+str(i) for i in range(len(keys)))+")")
        if last is not None:
            for i, k in enumerate(keys): params["l"+str(i)] = _typed(last[i], approved["kinds"][k])
            clauses.append("("+_pk_sql(table)+") > ("+",".join(":l"+str(i) for i in range(len(keys)))+")")
        suffix = " WHERE " + " AND ".join(clauses) + " ORDER BY " + _pk_sql(table) + " LIMIT :page"
        preview = await reader.query(preview_select + suffix, params)
        if (len(preview) > PAGE or any(len(row) != len(keys)+1 or type(row[-1]) is not int or row[-1] < 0 for row in preview)):
            _fail("source_page_length_shape_changed")
        if count+len(preview) > MAX_ROWS or size+sum(row[-1] for row in preview) > MAX_BYTES:
            _fail("fixed_scan_budget_exceeded")
        if upper is None and preview:
            _fail("approved_empty_table_changed")
        rows = await reader.query(select + suffix, params)
        pages += 1
        if len(rows) != len(preview): _fail("source_page_changed_after_length_check")
        if upper is None and rows:
            _fail("approved_empty_table_changed")
        for cells, measured in zip(rows, preview):
            if len(cells) != len(columns): _fail("source_row_shape_changed")
            count += 1; size += sum(len(v) for v in cells if v is not None)
            if count > MAX_ROWS or size > MAX_BYTES: _fail("fixed_scan_budget_exceeded")
            cursor = tuple(cells[columns.index(k)] for k in keys)
            if cursor != measured[:-1] or sum(len(v) for v in cells if v is not None) != measured[-1]:
                _fail("source_page_changed_after_length_check")
            if last == cursor: _fail("nonadvancing_primary_key")
            last = cursor
            for v in cells: _frame(h, v)
            if retain:
                fact = _minimal(table, dict(zip(columns, cells)))
                retained_budget[0] += len(_canon(fact))
                if retained_budget[0] > MAX_GRAPH_BYTES: _fail("fixed_minimal_graph_budget_exceeded")
                minimal.append(fact)
        if len(rows) < PAGE: break
    return {"rows": count, "source_bytes": size, "pages": pages,
            "source_sha256": h.hexdigest(), "upper_token_sha256": _sha(_canon(upper))}, minimal


async def observe(session, bounds, *, approved_bounds_sha256):
    if (not isinstance(bounds, PrivateBounds) or set(bounds.tables) != set(SPECS)
            or bounds.head != HEAD or approved_bounds_sha256 != bounds.digest()):
        _fail("independent_bounds_approval_required")
    reader = _Borrowed(session)
    await _binding(reader, bounds.identity_hash, bounds.source_sha)
    sections = {}; dataset = {}; retained_budget = [0]
    for table in SPECS:
        schema = await _schema(reader, table)
        approved = bounds.tables.get(table)
        if approved is None or any(schema[k] != approved[k] for k in schema):
            _fail("approved_schema_changed")
        first, rows = await _pass(reader, table, approved, retain=True, retained_budget=retained_budget)
        second, _ = await _pass(reader, table, approved, retain=False)
        if first != second: _fail("bound_passes_disagree")
        current = await _schema(reader, table)
        if any(current[k] != approved[k] for k in current):
            _fail("source_schema_changed_during_scan")
        sections[table] = {**first, "equal_full_passes": 2, "complete": True}
        dataset[table] = rows
    await _binding(reader, bounds.identity_hash, bounds.source_sha)
    return PrivateObservation(bounds, sections, dataset, reader.tx, _seal=_OBSERVATION_SEAL)


async def fresh_after_upper(session, observation):
    """Call only on a distinct newly started host snapshot after the first ends."""
    if not isinstance(observation, PrivateObservation) or observation._original_tx.is_active:
        _fail("original_snapshot_must_end_before_fresh_observation")
    bounds=observation._bounds
    reader = _Borrowed(session)
    if reader.tx is observation._original_tx:
        _fail("fresh_snapshot_required")
    await _binding(reader, bounds.identity_hash, bounds.source_sha)
    result = {}
    for table in SPECS:
        schema = await _schema(reader, table)
        approved = bounds.tables[table]
        if any(schema[k] != approved[k] for k in schema): _fail("post_snapshot_schema_changed")
        params = {}; clauses = ""
        if approved["upper"] is not None:
            keys = SPECS[table][1].split()
            upper_values = _upper_values(approved, keys)
            for i, k in enumerate(keys): params["u"+str(i)] = upper_values[i]
            clauses = " WHERE ("+_pk_sql(table)+") > ("+",".join(":u"+str(i) for i in range(len(keys)))+")"
        rows = await reader.query("SELECT 1 FROM `"+table+"`" + clauses + " LIMIT 1", params)
        result[table] = {"next_cycle_required": bool(rows)}
    return result


def _minimal(table, row):
    # References remain private; free text/answer/config/provider/output/wire
    # bodies are replaced with hashes before retaining a graph observation.
    body_fields = {"goal", "answer", "text", "items", "payload", "response", "request_json", "response_json", "selector_query", "reason", "frozen_request_json", "receipt", "body", "wire", "quota_snapshot"}
    out = {k: _text(v) for k, v in row.items() if k not in body_fields}
    out["digests"] = {k: _sha(v) if v is not None else None for k, v in row.items() if k in body_fields}
    for field in ("assessment_ids",):
        if row.get(field) is not None: out[field] = _json(row[field])
    if table == "idempotency_requests" and row["response"] is not None:
        response = _json(row["response"])
        if not isinstance(response, dict) or set(response) != {"session_id", "run_id", "status", "version"}: _fail("unsupported_original_receipt")
        out["original_receipt"] = response
    if table == "result_outbox":
        value = _json(row["payload"])
        if not isinstance(value, dict): _fail("unsupported_result_event")
        out["event_refs"] = {k: value.get(k) for k in ("event_id", "request_id", "session_id", "testee_id", "version", "status")}
        out["event_actor"] = value.get("actor")
    if table.startswith("ai_messaging_") and row.get("body") is not None:
        if _sha(row["body"]) != out["body_sha256"]: _fail("stored_body_hash_conflict")
        from qs_ai.contracts.workflow import messaging_pb2 as pb
        try: body = pb.MessagingBody.FromString(row["body"])
        except Exception: _fail("unsupported_protobuf_body")
        name = body.WhichOneof("value")
        expected = {1:"start",2:"change",3:"participant_retry",4:"evaluation_start",5:"evaluation_cancel",6:"command_receipt",7:"interpretation_state",8:"evaluation_state",9:"event_acknowledgement"}
        if expected.get(int(out["kind"])) != name: _fail("inner_outer_message_kind_conflict")
        value = getattr(body, name)
        fields = {f.name for f in value.DESCRIPTOR.fields}
        out["body_refs"] = {k: getattr(value,k) for k in ("request_id","session_id","command_id","event_id","run_id","command_body_sha256","event_body_sha256") if k in fields}
        if name == "command_receipt": out["body_refs"]["decision"] = value.decision
        if name == "event_acknowledgement": out["body_refs"]["outcome"] = value.outcome
        if "actor" in fields: out["body_refs"]["actor"] = {"org_id": value.actor.org_id, "subject_id": value.actor.subject_id}
        if "workflow_receipt" in fields and value.HasField("workflow_receipt"):
            out["body_refs"]["workflow_receipt"] = {"session_id":value.workflow_receipt.session_id,"run_id":value.workflow_receipt.run_id,"version":value.workflow_receipt.version,"status":value.workflow_receipt.status}
        if "scope" in fields: out["body_refs"]["scope_org_id"] = str(value.scope.organization_id)
    if row.get("wire") is not None and table in ("ai_messaging_outbox", "ai_messaging_quarantine") and _sha(row["wire"]) != out["wire_sha256"]: _fail("stored_wire_hash_conflict")
    return out


def analyze(rows, targets):
    """Finite graph diagnostics; full semantic/peer/fence verdict is not emitted."""
    if set(rows) != set(SPECS) or not isinstance(targets, tuple) or len(targets)!=8:
        _fail("exact_eight_start_scope_required")
    required={"command_id","request_id","session_id","organization_id","subject_id","testee_id","assessment_ids","qs_ai_request_hash","source_row_sha256"}
    if any(set(t)!=required or t["command_id"]!=t["request_id"] for t in targets): _fail("original_start_scope_binding_required")
    for target in targets:
        try:
            for key in ("command_id","request_id","session_id"):
                value=target[key];parsed=UUID(value)
                if not parsed.int or str(parsed)!=value: _fail("canonical_original_identity_required")
            for value in [target["organization_id"],target["testee_id"],*target["assessment_ids"]]:
                if not isinstance(value,str) or not re.fullmatch(r"[1-9][0-9]{0,19}",value) or int(value)>=2**64: _fail("canonical_business_identity_required")
            if (not isinstance(target["assessment_ids"],list) or not 1<=len(target["assessment_ids"])<=10 or len(set(target["assessment_ids"]))!=len(target["assessment_ids"]) or not isinstance(target["subject_id"],str) or not 1<=len(target["subject_id"].encode())<=128): _fail("canonical_business_identity_required")
            for key in ("qs_ai_request_hash","source_row_sha256"):
                if not re.fullmatch(r"[0-9a-f]{64}",target[key]): _fail("original_source_digest_required")
        except Rejected: raise
        except Exception: _fail("original_start_scope_binding_required")
    if len({t["command_id"] for t in targets})!=8 or len({t["session_id"] for t in targets})!=8: _fail("distinct_eight_original_starts_required")
    counters={"orphan":0,"binding_conflict":0,"unfinished_execution":0,"unknown_provider":0,"unfinished_delivery":0,"untrusted_quarantine":len(rows["ai_messaging_quarantine"])}
    sessions={r["id"]:r for r in rows["interpretation_sessions"]};runs={r["id"]:r for r in rows["interpretation_runs"]}
    jobs={r["run_id"]:r for r in rows["execution_jobs"]};calls={r["run_id"]:r for r in rows["model_calls"]}
    requests={r["request_id"]:r for r in rows["external_requests"]}
    for table in ("interpretation_runs","execution_jobs","clarifications","evidence_sets","interpretation_artifacts","execution_configurations","participant_capacity_reservations","participant_retries","result_outbox","runtime_milestones"):
        for r in rows[table]:
            if r["session_id"] not in sessions: counters["orphan"]+=1
            for field in ("run_id","source_run_id"):
                if r.get(field) and (r[field] not in runs or runs[r[field]]["session_id"]!=r["session_id"]): counters["binding_conflict"]+=1
    for r in rows["model_calls"]:
        if r["run_id"] not in runs: counters["orphan"]+=1
    for r in rows["external_requests"]:
        if r["session_id"] not in sessions: counters["orphan"]+=1
    for r in rows["execution_leases"]:
        if r["thread_id"] not in sessions: counters["orphan"]+=1
    scope=_sha(_canon(["qs-server","external-start-v1"]))
    summaries=[]
    for target in targets:
        sid=target["session_id"];session=sessions.get(sid);request=requests.get(target["request_id"]);issues=set()
        if session is None or request is None or request["session_id"]!=sid: issues.add("original_request_session_missing_or_conflicting")
        if session and (session["org_id"]!=target["organization_id"] or session["owner_subject_id"]!=target["subject_id"] or session["testee_id"]!=target["testee_id"] or session["assessment_ids"]!=target["assessment_ids"]): issues.add("business_ownership_conflict")
        receipt=[r for r in rows["idempotency_requests"] if r["key"]==target["request_id"] and r["scope_hash"]==scope]
        if len(receipt)!=1 or receipt[0]["request_hash"]!=target["qs_ai_request_hash"] or receipt[0].get("original_receipt",{}).get("session_id")!=sid: issues.add("original_start_receipt_unverified")
        linked_runs=[r for r in rows["interpretation_runs"] if r["session_id"]==sid]
        if session and (session["status"] not in ("completed","cancelled") or session["active_run_id"] not in runs or runs[session["active_run_id"]]["session_id"]!=sid): issues.add("business_terminal_unproven")
        for run in linked_runs:
            job=jobs.get(run["id"]);call=calls.get(run["id"])
            if run["status"] in ("queued","running") or (job and job["status"] not in ("done","dead","cancelled")): counters["unfinished_execution"]+=1;issues.add("unfinished_execution")
            if call and call["status"] in ("dispatched","unknown"): counters["unknown_provider"]+=1;issues.add("provider_execution_unknown")
            if call and call["status"] not in ("response_received","failed","dispatched","unknown"): issues.add("unsupported_model_call_status")
            if call and call["status"]=="response_received" and call["digests"].get("response_json") is None: issues.add("persistent_response_missing")
        for reservation in rows["participant_capacity_reservations"]:
            if reservation["session_id"]==sid and (reservation["organization_id"]!=target["organization_id"] or reservation["subject_id"]!=target["subject_id"] or reservation["assessment_ids"]!=target["assessment_ids"]): issues.add("reservation_owner_conflict")
            if reservation["session_id"]==sid and reservation["active"]=="1": issues.add("active_capacity_reservation")
        for event in rows["result_outbox"]:
            if event["session_id"]==sid:
                ref=event["event_refs"]
                if (ref.get("request_id")!=target["request_id"] or ref.get("session_id")!=sid or ref.get("event_id")!=event["event_id"] or str(ref.get("version"))!=event["version"] or ref.get("testee_id")!=target["testee_id"] or event["event_actor"]!={"org_id":target["organization_id"],"subject_id":target["subject_id"]}): issues.add("legacy_result_binding_conflict")
                if event["delivered"]!="1": counters["unfinished_delivery"]+=1;issues.add("unfinished_legacy_result_delivery")
        for box in rows["ai_messaging_outbox"]:
            ref=box["body_refs"]
            if box["aggregate_key"]==target["request_id"] or any(v in (target["request_id"],sid) for v in ref.values() if isinstance(v,str)):
                if box["organization_id"]!=target["organization_id"] or box["aggregate_key"]!=target["request_id"]: issues.add("mq_body_owner_or_aggregate_conflict")
                if box["stage"]!="confirmed" or box["confirmed_at"] is None: counters["unfinished_delivery"]+=1;issues.add("unfinished_mq_event_confirmation")
        for inbox in rows["ai_messaging_inbox"]:
            if inbox["aggregate_key"]==target["request_id"] or inbox["message_id"]==target["command_id"]:
                if inbox["decision"] not in ("accepted","rejected") or inbox["receipt_id"] is None: issues.add("mq_command_processing_or_held")
                refs=inbox["body_refs"];actor=refs.get("actor")
                if inbox["aggregate_key"]!=target["request_id"] or (actor and actor["org_id"]!=target["organization_id"]): issues.add("mq_inbox_ownership_conflict")
                decisions=[b for b in rows["ai_messaging_outbox"] if b["message_id"]==inbox["receipt_id"]]
                if len(decisions)!=1 or decisions[0]["body_refs"].get("command_id")!=inbox["message_id"] or decisions[0]["body_refs"].get("command_body_sha256")!=inbox["body_sha256"]: issues.add("mq_command_receipt_orphan_or_conflicting")
        summaries.append({"original_source_row_sha256":target["source_row_sha256"],"reference_sha256":_sha(_canon([target["command_id"],target["request_id"],sid])),"retained_runs":len(linked_runs),"blocking_categories":sorted(issues),"retirement_proven":False})
    return {"targets":summaries,"global_diagnostic_counts":counters,"semantic_coverage":"prototype_partial_explicit",
            "remaining_gates":["typed_artifact_configuration_response_full_validation","all_reverse_mq_and_retry_semantics","qs_server_peer_receipt_exact_cross_check","writer_fence_and_fresh_source_recheck","historical_writer_contract_version_binding"],
            "business_retirement_proven":False,"drop_ready":False}
