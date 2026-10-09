"""Finite full-ledger qualification; host-owned SQLAlchemy RR/RO transactions only.

No connection/transaction factory, DML, repair, resend, imported completion proof
or retirement verdict exists here. Body bytes are private, bounded and ephemeral.
The frozen diagnostic observer is loaded into an isolated module namespace.
"""
from __future__ import annotations

import asyncio
import base64
import hashlib
import importlib.util
import json
import re
import sys
import time
from dataclasses import asdict, dataclass, fields, replace
from datetime import datetime, timezone
from pathlib import Path
from uuid import UUID, uuid5, NAMESPACE_URL

_OBSERVER_SHA = "20c501d0930a21c1e8be12416156d5635430cf6171e01d28a6db789e83f06d5a"
PAGE = 1000
MAX_ROWS = 1_000_000
MAX_BYTES = 2 << 30
MAX_PRIVATE_BYTES = 256 << 20
MAX_PAGE_BYTES = 32 << 20
QUERY_SECONDS = 30
TOTAL_SECONDS = 1500
AI_HEAD = "0038_messaging_observations"
_SEAL = object()
# Source schema.py: all 48 Base tables. Additional MQ declarations are below.
AI_SPECS = {
    "clarifications": [
        "id session_id question_seq text can_skip answer skipped answered_by answered_at",
        "id"
    ],
    "configuration_publication_changes": [
        "command_id selector_key version organization_id operator_user_id request_json receipt_json",
        "command_id"
    ],
    "configuration_publication_pointers": [
        "selector_key selector_json version active_publication_id changed_at",
        "selector_key"
    ],
    "configuration_publications": [
        "publication_id selector_key run_id run_version organization_id content_json content_sha256",
        "publication_id"
    ],
    "evaluation_admission_locks": [
        "organization_id",
        "organization_id"
    ],
    "evaluation_capacity_reservations": [
        "quota_snapshot run_id organization_id budget_day provider_calls daily_limit requested_by reserved_at",
        "run_id"
    ],
    "evaluation_checkpoints": [
        "run_id version checkpoint_json",
        "run_id"
    ],
    "evaluation_dispatches": [
        "run_id invocation_id execution_id kind case_id slot_ordinal candidate_id checkpoint_json",
        "run_id invocation_id"
    ],
    "evaluation_generation_completions": [
        "run_id execution_id invocation_id case_id slot_ordinal execution_ordinal candidate_id candidate_json evidence_json raw_output normalized_output",
        "run_id execution_id"
    ],
    "evaluation_policy_assets": [
        "kind asset_id version fingerprint definition_json source_ref imported_by created_at",
        "kind asset_id version"
    ],
    "evaluation_response_receipts": [
        "run_id invocation_id execution_id claim_version definition_json sha256",
        "run_id invocation_id"
    ],
    "evaluation_run_policies": [
        "run_id fingerprint definition_json",
        "run_id"
    ],
    "evaluation_runs": [
        "run_id execution_mode organization_id requested_by definition_json progress_json",
        "run_id"
    ],
    "evaluation_semantic_completions": [
        "run_id execution_id invocation_id candidate_id execution_ordinal evidence_json result_json raw_output normalized_output",
        "run_id execution_id"
    ],
    "evaluation_slot_claims": [
        "run_id case_id slot_ordinal version checkpoint_json",
        "run_id case_id slot_ordinal"
    ],
    "evaluation_suites": [
        "suite_id suite_version fingerprint definition_json command_id organization_id operator_user_id receipt_json receipt_sha256 source_ref imported_by contracts_json contracts_sha256",
        "suite_id suite_version"
    ],
    "evidence_sets": [
        "id session_id fingerprint schema_version items frozen_at",
        "id"
    ],
    "execution_configurations": [
        "session_id evidence_set_id evidence_fingerprint publication_id publication_sha256 pointer_version selector_query",
        "session_id"
    ],
    "execution_jobs": [
        "id run_id session_id status available_at lease_until fence_token attempt answer skipped question_id",
        "id"
    ],
    "execution_leases": [
        "thread_id fence expires_at",
        "thread_id"
    ],
    "external_requests": [
        "request_id session_id",
        "request_id"
    ],
    "idempotency_requests": [
        "scope_hash key request_hash response created_at",
        "scope_hash key"
    ],
    "interpretation_artifacts": [
        "id session_id run_id payload created_at",
        "id"
    ],
    "interpretation_runs": [
        "id session_id session_version status checkpoint_ref",
        "id"
    ],
    "interpretation_sessions": [
        "id org_id owner_subject_id testee_id assessment_ids goal status version active_run_id current_question_id evidence_set_id workflow_version failure_code created_at updated_at created_at_utc updated_at_utc",
        "id"
    ],
    "model_calls": [
        "run_id invocation_id fence_token status request_json response_json failure_code created_at created_at_utc",
        "run_id"
    ],
    "organization_quota_commands": [
        "organization_id command_id operator_user_id request_json receipt_json receipt_sha256",
        "organization_id command_id"
    ],
    "organization_quota_pointers": [
        "organization_id revision",
        "organization_id"
    ],
    "organization_quota_versions": [
        "organization_id revision definition_json definition_sha256 operator_user_id reason created_at",
        "organization_id revision"
    ],
    "participant_admission_locks": [
        "organization_id",
        "organization_id"
    ],
    "participant_capacity_reservations": [
        "quota_snapshot run_id session_id organization_id subject_id assessment_ids budget_day reserved_at active acquired_at released_at",
        "run_id"
    ],
    "participant_retries": [
        "organization_id command_id session_id request_id source_run_id run_id operator_user_id expected_version reason accepted_unknown_risk source_failure_code frozen_request_json receipt created_at",
        "organization_id command_id"
    ],
    "profile_assets": [
        "profile_id version fingerprint definition_json source_ref imported_by created_at",
        "profile_id version"
    ],
    "profile_registrations": [
        "command_id organization_id operator_user_id profile_id profile_version receipt_json receipt_sha256",
        "command_id"
    ],
    "prompt_assets": [
        "template_id version fingerprint package_sha256 package_json source_ref imported_by created_at",
        "template_id version"
    ],
    "prompt_draft_freezes": [
        "command_id draft_id organization_id operator_user_id receipt_json receipt_sha256",
        "command_id"
    ],
    "prompt_draft_revisions": [
        "command_id draft_id revision organization_id operator_user_id request_json snapshot_json snapshot_sha256",
        "command_id"
    ],
    "prompt_drafts": [
        "draft_id organization_id revision",
        "draft_id"
    ],
    "result_outbox": [
        "event_id session_id version payload delivered mq_owned attempts created_at delivered_at available_at",
        "event_id"
    ],
    "route_assets": [
        "route revision fingerprint definition_json source_ref imported_by created_at",
        "route revision"
    ],
    "runtime_milestones": [
        "session_id dedupe_key run_id kind invocation_id attempt occurred_at expires_at",
        "session_id dedupe_key"
    ],
    "schema_assets": [
        "schema_id version fingerprint definition_json source_ref imported_by created_at",
        "schema_id version"
    ],
    "semantic_draft_commands": [
        "organization_id command_id operator_user_id draft_id request_json receipt_json receipt_sha256",
        "organization_id command_id"
    ],
    "semantic_draft_heads": [
        "organization_id draft_id revision",
        "organization_id draft_id"
    ],
    "semantic_draft_versions": [
        "organization_id draft_id revision snapshot_json snapshot_sha256",
        "organization_id draft_id revision"
    ],
    "semantic_prompt_assets": [
        "organization_id asset_id version fingerprint markdown source_ref imported_by created_at",
        "organization_id asset_id version"
    ],
    "solution_commands": [
        "command_id solution_id organization_id operator_user_id request_json receipt_json receipt_sha256",
        "command_id"
    ],
    "solution_revisions": [
        "solution_id organization_id revision draft_id state_json state_sha256",
        "solution_id"
    ]
}
AI_SPECS.update({
    "ai_messaging_outbox": ("producer destination message_id body_sha256 body wire wire_sha256 topic kind organization_id aggregate_key aggregate_sequence ordered requires_receipt stage attempts available_at created_at published_at confirmed_at error_code", "producer destination message_id"),
    "ai_messaging_inbox": ("producer message_id destination body_sha256 body wire_sha256 kind aggregate_key reservation_token decision receipt_id received_at", "producer message_id"),
    "ai_messaging_quarantine": ("wire_sha256 wire code logical_producer logical_message_id logical_body_sha256 attempts first_seen_at last_seen_at", "wire_sha256"),
    "ai_messaging_evaluation_sequences": ("run_id sequence version", "run_id"),
    "ai_messaging_observations": ("kind recorded_count recording_since last_observed_at", "kind"),
})
# QS-server migrations 72/83/91-95/97. Full AI responsibility sources, no org JOIN.
PEER_SPECS = {
    "ai_bridge_commands": ("command_id request_id kind payload payload_hash delivered attempts available_at", "command_id"),
    "ai_messaging_legacy_commands": ("command_id request_id source_kind source_payload source_payload_hash source_attempts source_available_at source_original_time messaging_body_sha256 transferred_at", "command_id"),
    "ai_bridge_requests": ("request_id request_hash payload session_id version status projection organization_id subject_id testee_id created_at updated_at", "request_id"),
    "ai_bridge_request_assessments": ("request_id assessment_id", "request_id assessment_id"),
    "ai_bridge_events": ("event_id request_id version payload_hash", "event_id"),
    "ai_messaging_outbox": ("producer destination message_id body_sha256 body wire wire_sha256 kind organization_id topic aggregate_key aggregate_sequence ordered requires_receipt stage attempts available_at created_at published_at confirmed_at error_code", "producer destination message_id"),
    "ai_messaging_inbox": ("producer message_id body_sha256 body wire_sha256 kind aggregate_key ack_id received_at outcome", "producer message_id"),
    "ai_messaging_operations": ("command_id kind body_sha256 organization_id subject_id resource_id aggregate_key aggregate_sequence decision code receipt_id receipt created_at decided_at retired retirement_evidence retired_at", "command_id"),
    "ai_messaging_aggregates": ("aggregate_key next_sequence", "aggregate_key"),
    "ai_messaging_evaluation_states": ("run_id organization_id event_sequence version state updated_at", "run_id"),
    "ai_messaging_quarantine": ("wire_sha256 wire code attempts first_seen_at last_seen_at", "wire_sha256"),
    "ai_messaging_failures": ("producer message_id body_sha256 kind aggregate_key wire attempts first_seen_at last_seen_at", "producer message_id"),
    "ai_messaging_admission": ("singleton closed revision updated_at", "singleton"),
    "ai_messaging_observations": ("kind recorded_count recording_since last_observed_at", "kind"),
}


class Rejected(Exception):
    """Fixed, body-free public categories."""


def fail(category):
    raise Rejected(category) from None


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def canonical(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"),
                      allow_nan=False).encode()


def json_value(raw):
    def pairs(items):
        d = {}
        for key, value in items:
            if key in d:
                fail("duplicate_json_field")
            d[key] = value
        return d
    try:
        return json.loads(raw, object_pairs_hook=pairs,
                          parse_constant=lambda _: fail("nonfinite_json"))
    except Rejected:
        raise
    except Exception:
        fail("invalid_source_json")


def text(raw):
    try:
        return None if raw is None else raw.decode("utf-8", errors="strict")
    except Exception:
        fail("invalid_source_text")


def number(raw):
    value = text(raw)
    if value is None or not re.fullmatch(r"0|[1-9][0-9]*", value):
        fail("invalid_source_integer")
    return int(value)


def boolean(raw):
    value = number(raw)
    if value not in (0, 1):
        fail("invalid_source_boolean")
    return bool(value)


def original_uuid(value):
    try:
        if not isinstance(value, str) or str(UUID(value)) != value or UUID(value).int == 0:
            fail("invalid_original_identity")
    except Rejected:
        raise
    except Exception:
        fail("invalid_original_identity")
    return value


def one(rows, category):
    if len(rows) != 1:
        fail(category)
    return rows[0]


class _SourceRows(list):
    """Immutable scan-owned row references; avoid quadratic full-ledger joins.

    This remains private to a sealed actual scan. Unit-test dictionaries are
    deliberately not cached because tests mutate them to model contradictions.
    """
    def __init__(self, rows):
        super().__init__(rows)
        self._indexes = {}

    def matching(self, keys):
        columns = tuple(sorted(keys))
        if columns not in self._indexes:
            indexed = {}
            for row in self:
                indexed.setdefault(tuple(text(row.get(k)) for k in columns), []).append(row)
            self._indexes[columns] = indexed
        return self._indexes[columns].get(tuple(str(keys[k]) for k in columns), [])


def match(rows, **keys):
    if isinstance(rows, _SourceRows):
        return rows.matching(keys)
    return [r for r in rows if all(text(r.get(k)) == str(v) for k, v in keys.items())]


def doc(row, key):
    return None if row.get(key) is None else json_value(row[key])


def fixed_fields(value, keys, category):
    if not isinstance(value, dict) or set(value) != set(keys.split()):
        fail(category)


def _private_json(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"),
                      default=lambda v: {"binary": base64.b64encode(v).decode()}
                      if isinstance(v, bytes) else fail("unsupported_private_value")).encode()


def _scanner(side, head):
    path = Path(__file__).with_name("qs-ai-retirement-readonly-observer.py")
    if sha(path.read_bytes()) != _OBSERVER_SHA:
        fail("frozen_observer_changed")
    name = "_qs_ai_full_scan_" + side + "_" + str(time.monotonic_ns())
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    try:
        spec.loader.exec_module(module)
    finally:
        # Functions retain their isolated globals, but do not accumulate modules.
        del sys.modules[name]
    module.SPECS = module._layout().SPECS if side == "ai" and head == "0040_module_table_names" else AI_SPECS if side == "ai" else PEER_SPECS
    module._RAW_LAYOUT_SCAN = True
    module.HEAD = head
    module.PAGE, module.MAX_ROWS, module.MAX_BYTES = PAGE, MAX_ROWS, MAX_BYTES
    module.MAX_GRAPH_BYTES, module.QUERY_SECONDS, module.TOTAL_SECONDS = MAX_PRIVATE_BYTES, QUERY_SECONDS, TOTAL_SECONDS
    module._canon = _private_json
    module._minimal = lambda _, row: row
    original_schema = module._schema
    module._schema = lambda reader, table: original_schema(reader, table) if side == "ai" and head == "0040_module_table_names" else _schema(module, reader, table)
    module._binding = lambda reader, identity, source: _binding(reader, identity, source, side, head, module)
    original_reader = module._Borrowed
    class BoundedReader(original_reader):
        async def query(self, sql, params=None):
            rows = await super().query(sql, params)
            if " AS _source_row_bytes " in sql and sum(r[-1] for r in rows) > MAX_PAGE_BYTES:
                fail("private_page_byte_budget_exceeded")
            return rows
    module._Borrowed = BoundedReader
    return module


async def _binding(reader, identity, source, side, head, module):
    if not re.fullmatch(r"[0-9a-f]{64}", identity) or not re.fullmatch(r"[0-9a-f]{40}", source):
        fail("independent_database_source_binding_required")
    row = await reader.query("SELECT CAST(@@server_uuid AS BINARY),CAST(DATABASE() AS BINARY),VERSION(),@@transaction_isolation")
    if len(row) != 1 or module._identity(row[0][0], row[0][1]) != identity or not row[0][2].startswith("8.") or row[0][3] != "REPEATABLE-READ":
        fail("database_identity_or_isolation_changed")
    if side == "ai" and head == "0040_module_table_names" and source != module._layout().SOURCE_SHA:
        fail("fixed_0040_source_revision_required")
    # Check the actual running server transaction, not only the session default.
    current = await reader.query("SELECT t.ACCESS_MODE,t.ISOLATION_LEVEL,t.STATE FROM performance_schema.events_transactions_current t JOIN performance_schema.threads p ON p.THREAD_ID=t.THREAD_ID WHERE p.PROCESSLIST_ID=CONNECTION_ID()")
    if current != [("READ ONLY", "REPEATABLE READ", "ACTIVE")]:
        fail("actual_readonly_snapshot_not_observed")
    state = await reader.query("SELECT version_num FROM alembic_version ORDER BY version_num") if side == "ai" else await reader.query("SELECT version,dirty FROM schema_migrations")
    if state != ([(head,)] if side == "ai" else [(int(head), 0)]):
        fail("database_clean_head_changed")


async def _schema(module, reader, table):
    engine = await reader.query("SELECT ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=:name", {"name": table})
    if engine != [("InnoDB",)]:
        fail("transactional_source_table_required")
    columns = await reader.query("SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=:name ORDER BY ORDINAL_POSITION", {"name": table})
    if set(c[0] for c in columns) != set(module.SPECS[table][0].split()) or len(columns) != len(module.SPECS[table][0].split()):
        fail("unsupported_complete_table_schema")
    keys = module.SPECS[table][1].split()
    pk = await reader.query("SELECT COLUMN_NAME FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=:name AND index_name='PRIMARY' ORDER BY SEQ_IN_INDEX", {"name": table})
    if [c[0] for c in pk] != keys:
        fail("unsupported_primary_key")
    ddl = await reader.query("SHOW CREATE TABLE `" + table + "`")
    kinds = {}
    for c in columns:
        if c[0] in keys:
            # Server collation is frozen; all comparisons run on the server.
            if not (c[1] in ("bigint unsigned", "bigint", "int", "integer", "tinyint unsigned")
                    or (re.fullmatch(r"(?:var)?char\([0-9]+\)", c[1]) and c[5] in
                        ("ascii_bin", "utf8mb4_bin", "utf8mb4_0900_bin", "utf8mb4_0900_ai_ci"))):
                fail("unsupported_primary_key_type")
            kinds[c[0]] = c[1]
    return {"columns": columns, "kinds": kinds, "ddl_sha256": sha(_private_json(ddl)),
            "columns_sha256": sha(_private_json(columns))}


async def _catalog(reader, side):
    names = await reader.query("SELECT TABLE_NAME FROM information_schema.tables WHERE table_schema=DATABASE() AND TABLE_TYPE='BASE TABLE' ORDER BY TABLE_NAME")
    actual = {r[0] for r in names}
    expected = set(reader.specs) | {"alembic_version"} if side == "ai" else set(PEER_SPECS)
    if side == "ai" and actual != expected:
        fail("unsupported_database_catalog")
    if side == "peer" and ({n for n in actual if n.startswith(("ai_", "ai_bridge_"))} != expected):
        fail("unsupported_peer_ai_catalog")
    return sha(canonical(sorted(actual)))


@dataclass(frozen=True, repr=False)
class FullBounds:
    side: str
    source_sha: str
    identity_hash: str
    head: str
    catalog_sha256: str
    tables: dict

    def private_bytes(self):
        return _private_json({"protocol": "qs-ai-full-ledger-bounds/v1", "side": self.side,
                              "source_sha": self.source_sha, "identity_hash": self.identity_hash,
                              "head": self.head, "catalog_sha256": self.catalog_sha256,
                              "tables": self.tables,
                              "profile": [PAGE, MAX_ROWS, MAX_BYTES, MAX_PRIVATE_BYTES, MAX_PAGE_BYTES,
                                          QUERY_SECONDS, TOTAL_SECONDS]})

    def digest(self):
        return sha(self.private_bytes())

    def __repr__(self):
        return "<private full-ledger bounds>"


async def discover_full_bounds(session, *, side, source_sha, identity_hash, head):
    if side not in ("ai", "peer") or (side == "ai" and head not in (AI_HEAD, "0040_module_table_names")) or (side == "peer" and (not isinstance(head, str) or not re.fullmatch(r"[1-9][0-9]{0,8}", head))):
        fail("unsupported_bound_head")
    module = _scanner(side, head)
    reader = module._Borrowed(session)
    catalog = await _catalog(reader, side)
    try:
        bounds = await module.discover_bounds(session, identity_hash=identity_hash, source_sha=source_sha, head=head)
        return FullBounds(side, source_sha, identity_hash, head, catalog, bounds.tables)
    except module.Rejected as e:
        fail(str(e))


def _go_json(value):
    # These typed original writers contain strings/integers/bools only. Preserve
    # declared field order and Go's HTML/U+2028/U+2029 escaping, never JSON-column bytes.
    raw = json.dumps(value, ensure_ascii=False, separators=(",", ":"), allow_nan=False)
    return raw.replace("&", "\\u0026").replace("<", "\\u003c").replace(">", "\\u003e").replace("\u2028", "\\u2028").replace("\u2029", "\\u2029").encode()


def _start(value):
    if not isinstance(value, dict) or set(value) - set("evidence request_id actor testee_id assessment_ids goal".split()) or not set("request_id actor testee_id assessment_ids goal".split()) <= set(value):
        fail("unsupported_original_start_schema")
    fixed_fields(value["actor"], "org_id subject_id", "unsupported_original_actor")
    for key in ("org_id",):
        if not re.fullmatch(r"[1-9][0-9]{0,19}", value["actor"][key]) or int(value["actor"][key]) >= 2**64:
            fail("invalid_original_owner")
    if not isinstance(value["actor"]["subject_id"], str) or not 1 <= len(value["actor"]["subject_id"]) <= 128:
        fail("invalid_original_owner")
    original_uuid(value["request_id"])
    if not isinstance(value["goal"], str) or not value["goal"].strip():
        fail("missing_original_goal")
    ids = value["assessment_ids"]
    if not isinstance(ids, list) or not 1 <= len(ids) <= 10 or len(ids) != len(set(ids)):
        fail("invalid_original_assessment_scope")
    for v in [value["testee_id"], *ids]:
        if not isinstance(v, str) or not re.fullmatch(r"[1-9][0-9]{0,19}", v) or int(v) >= 2**64:
            fail("invalid_original_business_identity")
    evidence = value.get("evidence", [])
    if evidence is None:
        evidence = []
    if not isinstance(evidence, list):
        fail("unsupported_original_evidence")
    ordered = []
    for item in evidence:
        fixed_fields(item, "assessment_id testee_id report_id source_version facts", "unsupported_original_evidence")
        facts = item["facts"]
        if not isinstance(facts, list):
            fail("unsupported_original_evidence")
        for f in facts:
            fixed_fields(f, "ref value", "unsupported_original_fact")
            if not all(isinstance(f[k], str) for k in f):
                fail("unsupported_original_fact")
        ordered.append({k: item[k] for k in ("assessment_id", "testee_id", "report_id", "source_version")} |
                       {"facts": [{"ref": f["ref"], "value": f["value"]} for f in facts]})
    result = {}
    if ordered:
        result["evidence"] = ordered
    result.update(request_id=value["request_id"], actor={"org_id": value["actor"]["org_id"], "subject_id": value["actor"]["subject_id"]},
                  testee_id=value["testee_id"], assessment_ids=ids, goal=value["goal"])
    return result, ordered


@dataclass(frozen=True, repr=False)
class _Original:
    command_id: str
    request_id: str
    session_id: str
    start: dict
    writer_sha256: str
    original_request_sha256: str
    source_tables: tuple
    attempts: int
    handoff_sha256: str | None

    def __repr__(self):
        return "<private original START binding>"


def _originals(peer, approved_sections):
    # Approval is for actual full-EOF frame hashes/counts, not a supplied target list.
    names = ("ai_bridge_commands", "ai_messaging_legacy_commands")
    if not isinstance(approved_sections, dict) or set(approved_sections) != set(names):
        fail("independent_original_sources_approval_required")
    sources = {}
    for table in names:
        section = approved_sections[table]
        fixed_fields(section, "rows source_bytes source_sha256", "unsupported_original_source_approval")
        actual = peer.sections[table]
        if section != {k: actual[k] for k in section}:
            fail("approved_original_source_changed")
        for row in peer.rows[table]:
            cid, rid = text(row["command_id"]), text(row["request_id"])
            original_uuid(cid); original_uuid(rid)
            kind = text(row["kind"] if table == names[0] else row["source_kind"])
            if kind != "start" or cid != rid:
                fail("unsupported_original_command_type")
            raw = row["payload"] if table == names[0] else row["source_payload"]
            parsed, items = _start(json_value(raw))
            stored = text(row["payload_hash"] if table == names[0] else row["source_payload_hash"])
            if parsed["request_id"] != rid or sha(_go_json(parsed)) != stored:
                fail("original_writer_digest_or_id_conflict")
            if table == names[1] and sha(raw) != stored:
                fail("original_handoff_blob_digest_conflict")
            request = one(match(peer.rows["ai_bridge_requests"], request_id=rid), "original_peer_request_missing_or_ambiguous")
            original, _ = _start(doc(request, "payload"))
            if original != parsed or text(request["request_hash"]) != stored:
                fail("original_peer_request_digest_conflict")
            if (text(request["organization_id"]), text(request["subject_id"]), text(request["testee_id"])) != (
                    parsed["actor"]["org_id"], parsed["actor"]["subject_id"], parsed["testee_id"]):
                fail("original_peer_owner_conflict")
            sid = original_uuid(text(request["session_id"]))
            assessments = match(peer.rows["ai_bridge_request_assessments"], request_id=rid)
            if sorted(text(v["assessment_id"]) for v in assessments) != sorted(parsed["assessment_ids"]):
                fail("original_peer_assessment_scope_conflict")
            attempts = number(row["attempts"] if table == names[0] else row["source_attempts"])
            if attempts > 2**32 - 1:
                fail("unsupported_original_attempt_budget")
            if table == names[0] and number(row["delivered"]) != 1:
                fail("original_legacy_command_delivery_responsibility_open")
            current = sources.get(cid)
            handoff = text(row["messaging_body_sha256"]) if table == names[1] else None
            if handoff is not None and not re.fullmatch(r"[0-9a-f]{64}", handoff):
                fail("invalid_original_handoff_digest")
            request_hash = sha(canonical([parsed["actor"], parsed["testee_id"], parsed["assessment_ids"], parsed["goal"]] + items))
            if current is not None and (current.start != parsed or current.writer_sha256 != stored or current.session_id != sid or current.attempts != attempts):
                fail("original_duplicate_source_conflict")
            sources[cid] = _Original(cid, rid, sid, parsed, stored, request_hash,
                                     tuple(sorted((current.source_tables if current else ()) + (table,))),
                                     attempts, handoff or (current.handoff_sha256 if current else None))
    return tuple(sources[k] for k in sorted(sources))


def _session(row):
    from qs_ai.domain.interpretation.model import Actor, Session, Status
    return Session(id=text(row["id"]), actor=Actor(str(number(row["org_id"])), text(row["owner_subject_id"])),
                   testee_id=str(number(row["testee_id"])), assessment_ids=tuple(doc(row, "assessment_ids")),
                   goal=text(row["goal"]), status=Status(text(row["status"])), version=number(row["version"]),
                   active_run_id=text(row["active_run_id"]), current_question_id=text(row["current_question_id"]),
                   evidence_set_id=text(row["evidence_set_id"]), workflow_version=text(row["workflow_version"]),
                   failure_code=text(row["failure_code"]))


def _evidence(row):
    from qs_ai.domain.interpretation.model import EvidenceSet, EvidenceItem, Fact
    if text(row["schema_version"]) != "evidence-v1":
        fail("unsupported_original_evidence_schema")
    items = doc(row, "items")
    result = EvidenceSet(text(row["id"]), text(row["session_id"]), text(row["fingerprint"]),
                         tuple(EvidenceItem(**{**v, "facts": tuple(Fact(**f) for f in v["facts"])}) for v in items))
    if result.fingerprint != sha(canonical(items)):
        fail("original_evidence_digest_conflict")
    return result


def _retry_acceptance(ai, row):
    from qs_ai.application.execution.retry import ParticipantRetry
    from qs_ai.application.governance.prompt_drafts import DraftScope
    command = ParticipantRetry(DraftScope(number(row["organization_id"]), number(row["operator_user_id"])),
        text(row["session_id"]), text(row["command_id"]), text(row["source_run_id"]),
        number(row["expected_version"]), text(row["reason"]), True, 1,
        boolean(row["accepted_unknown_risk"]))
    prior = one(match(ai["idempotency_requests"],
        scope_hash=sha(canonical(["participant-retry-v1", command.scope.organization_id])),
        key=command.command_id), "original_retry_acceptance_missing")
    receipt = doc(row, "receipt")
    fixed_fields(receipt, "session_id run_id status version", "unsupported_original_retry_receipt")
    if text(prior["request_hash"]) != sha(canonical(asdict(command))) or doc(prior, "response") != receipt or receipt["session_id"] != command.session_id or receipt["run_id"] != text(row["run_id"]) or receipt["version"] != command.expected_version + 1 or receipt["status"] != "queued":
        fail("original_retry_acceptance_conflict")
    raw = row["frozen_request_json"]
    sources = match(ai["model_calls"], run_id=command.expected_run_id)
    inherited = sources[0]["request_json"] if sources else (match(ai["participant_retries"], run_id=command.expected_run_id) or [{}])[0].get("frozen_request_json")
    if raw != inherited:
        fail("original_retry_frozen_request_conflict")
    return command, receipt


def _original_execution_scope(ai, peer, originals):
    # Only _verify passes originals constructed from both approved full-EOF
    # legacy sources. None is unknown scope; an actual empty tuple is empty.
    if originals is None:
        return None
    sessions = {text(r["id"]): r for r in ai["interpretation_sessions"]}
    if len(sessions) != len(ai["interpretation_sessions"]):
        fail("global_duplicate_participant_owner")
    runs = {text(r["id"]): r for r in ai["interpretation_runs"]}
    if len(runs) != len(ai["interpretation_runs"]):
        fail("global_duplicate_participant_run")
    targets, target_sessions = set(), set()
    for original in originals:
        if not isinstance(original, _Original) or original.command_id in targets:
            fail("original_source_scope_not_authenticated")
        request = one(match(peer["ai_bridge_requests"], request_id=original.request_id),
                      "original_peer_request_missing_or_ambiguous")
        owner = one(match(ai["interpretation_sessions"], id=original.session_id),
                    "original_session_missing_or_ambiguous")
        payload, _ = _start(doc(request, "payload"))
        if (original.command_id != original.request_id or
                payload != original.start or text(request["request_hash"]) != original.writer_sha256 or
                text(request["session_id"]) != original.session_id or
                (str(number(owner["org_id"])), text(owner["owner_subject_id"]),
                 str(number(owner["testee_id"])), doc(owner, "assessment_ids"), text(owner["goal"])) !=
                (original.start["actor"]["org_id"], original.start["actor"]["subject_id"],
                 original.start["testee_id"], original.start["assessment_ids"], original.start["goal"])):
            fail("original_session_owner_or_request_conflict")
        targets.add(original.request_id)
        target_sessions.add(original.session_id)
    target_runs = {rid for rid, row in runs.items() if text(row["session_id"]) in target_sessions}
    return targets, target_sessions, target_runs


def _reverse(ai, peer, originals=None):
    sessions = {text(r["id"]): r for r in ai["interpretation_sessions"]}
    runs = {text(r["id"]): r for r in ai["interpretation_runs"]}
    externals = {text(r["session_id"]): r for r in ai["external_requests"]}
    evaluations = {text(r["run_id"]): r for r in ai["evaluation_runs"]}
    for table, parent, field in (
        ("interpretation_runs", sessions, "session_id"), ("clarifications", sessions, "session_id"),
        ("evidence_sets", sessions, "session_id"), ("interpretation_artifacts", sessions, "session_id"),
        ("execution_configurations", sessions, "session_id"), ("runtime_milestones", sessions, "session_id"),
        ("result_outbox", sessions, "session_id"), ("external_requests", sessions, "session_id"),
        ("execution_jobs", runs, "run_id"), ("model_calls", runs, "run_id"),
        ("participant_capacity_reservations", runs, "run_id"), ("participant_retries", runs, "run_id"),
    ):
        for row in ai[table]:
            if text(row[field]) not in parent:
                fail("global_orphan_participant_fact")
            if field == "run_id" and "session_id" in row and text(row["session_id"]) != text(parent[text(row[field])]["session_id"]):
                fail("global_cross_session_fact")
    for row in ai["execution_leases"]:
        if text(row["thread_id"]) not in sessions:
            fail("global_orphan_execution_lease")
    for row in ai["interpretation_artifacts"]:
        run = runs.get(text(row["run_id"]))
        if run is None or text(run["session_id"]) != text(row["session_id"]):
            fail("global_cross_run_artifact")
    for row in ai["participant_retries"]:
        source = runs.get(text(row["source_run_id"]))
        owner = sessions.get(text(row["session_id"]))
        if source is None or text(source["session_id"]) != text(row["session_id"]) or number(row["organization_id"]) != number(owner["org_id"]) or text(row["request_id"]) != text(externals.get(text(row["session_id"]), {}).get("request_id")):
            fail("global_retry_owner_or_original_conflict")
        if text(source["status"]) != "blocked":
            fail("global_retry_source_business_state_conflict")
        _retry_acceptance(ai, row)
    for row in ai["participant_capacity_reservations"]:
        owner = sessions[text(row["session_id"])]
        if number(row["organization_id"]) != number(owner["org_id"]) or text(row["subject_id"]) != text(owner["owner_subject_id"]) or doc(row, "assessment_ids") != doc(owner, "assessment_ids"):
            fail("global_capacity_owner_conflict")
    for table in AI_SPECS:
        if table.startswith("evaluation_") and table not in ("evaluation_runs", "evaluation_admission_locks", "evaluation_policy_assets", "evaluation_suites"):
            for row in ai[table]:
                if text(row.get("run_id")) not in evaluations:
                    fail("global_orphan_evaluation_fact")
    for row in ai["ai_messaging_evaluation_sequences"]:
        if text(row["run_id"]) not in evaluations:
            fail("global_orphan_evaluation_sequence")
    requests = {text(r["request_id"]): r for r in peer["ai_bridge_requests"]}
    for row in ai["external_requests"]:
        request = requests.get(text(row["request_id"]))
        owner = sessions[text(row["session_id"])]
        if request is None or text(request["session_id"]) != text(owner["id"]):
            fail("global_external_request_peer_binding_missing")
        original, _ = _start(doc(request, "payload"))
        if text(request["request_hash"]) != sha(_go_json(original)) or (original["actor"]["org_id"], original["actor"]["subject_id"], original["testee_id"], original["assessment_ids"], original["goal"]) != (str(number(owner["org_id"])), text(owner["owner_subject_id"]), str(number(owner["testee_id"])), doc(owner, "assessment_ids"), text(owner["goal"])):
            fail("global_external_request_original_owner_conflict")
    scope = _original_execution_scope(ai, peer, originals)
    outside_unknown = 0
    for row in ai["model_calls"]:
        status = text(row["status"])
        if status not in ("failed", "response_received", "unknown", "dispatched"):
            fail("unsupported_global_model_call_status")
        if status in ("unknown", "dispatched"):
            if scope is None:
                fail("original_source_scope_required_for_provider_responsibility")
            # Global parent/session/org checks above have already proved the
            # current owner. Only exact original session-owned runs are target.
            if text(row["run_id"]) in scope[2]:
                fail("global_participant_provider_result_unknown")
            job = one(match(ai["execution_jobs"], run_id=text(row["run_id"])),
                      "global_provider_job_missing_or_ambiguous")
            if text(job["session_id"]) != text(runs[text(row["run_id"])]["session_id"]):
                fail("global_cross_session_fact")
            outside_unknown += 1
    for table in ("ai_bridge_events", "ai_bridge_request_assessments"):
        for row in peer[table]:
            if text(row["request_id"]) not in requests:
                fail("global_orphan_peer_request_fact")
    return {"outside_retirement_provider_result_unknown": outside_unknown}


def _event_projection(value):
    fixed_fields(value, "event_id request_id session_id actor testee_id version status question_id question can_skip failure_code artifact_json", "unsupported_original_state_event")
    fixed_fields(value["actor"], "org_id subject_id", "unsupported_original_actor")
    result = {k: value[k] for k in ("event_id", "request_id", "session_id", "actor", "testee_id", "version", "status", "question_id", "question", "can_skip", "failure_code")}
    if value["artifact_json"]:
        result["artifact_json"] = value["artifact_json"]
    return result


def _legacy_event_chain(ai, peer, original, session):
    events = sorted(match(ai["result_outbox"], session_id=original.session_id), key=lambda v: number(v["version"]))
    versions = [number(v["version"]) for v in events]
    # Session increments do not always stage an event: original admission
    # refusal runs queue/running/block before one save at version 4.
    if not events or versions != sorted(set(versions)) or versions[-1] != session.version:
        fail("original_state_sequence_gap")
    seen = set()
    for row in events:
        value = doc(row, "payload")
        projection = _event_projection(value)
        if value["event_id"] != text(row["event_id"]) or value["session_id"] != original.session_id or value["request_id"] != original.request_id or value["version"] != number(row["version"]) or value["actor"] != original.start["actor"] or value["testee_id"] != original.start["testee_id"]:
            fail("original_state_owner_or_identity_conflict")
        original_uuid(value["event_id"])
        if value["event_id"] in seen:
            fail("duplicate_original_state_identity")
        seen.add(value["event_id"])
        accepted = one(match(peer["ai_bridge_events"], event_id=value["event_id"]), "original_peer_business_acceptance_missing")
        if text(accepted["request_id"]) != original.request_id or number(accepted["version"]) != value["version"] or text(accepted["payload_hash"]) != sha(_go_json(projection)):
            fail("original_peer_business_acceptance_conflict")
        if not number(row["delivered"]):
            fail("original_result_delivery_responsibility_open")
        if boolean(row["mq_owned"]) and not match(ai["ai_messaging_outbox"], message_id=value["event_id"]):
            fail("original_result_handoff_responsibility_missing")
    # Reverse exact set, including events hidden by a mutable current projection.
    peer_events = match(peer["ai_bridge_events"], request_id=original.request_id)
    if {text(r["event_id"]) for r in peer_events} != seen:
        fail("orphan_original_peer_business_event")
    latest = events[-1]
    value = doc(latest, "payload")
    if value["status"] != str(session.status):
        fail("original_terminal_state_conflict")
    request = one(match(peer["ai_bridge_requests"], request_id=original.request_id), "original_peer_request_missing")
    expected = _event_projection(value)
    if doc(request, "projection") != expected or number(request["version"]) != session.version or text(request["status"]) != str(session.status):
        fail("original_peer_terminal_projection_conflict")
    return events


async def _typed_execution(db, ai, original, session):
    from qs_ai.application.execution.artifact import build_artifact
    from qs_ai.application.execution.generation import GeneratedExplanation
    from qs_ai.application.interpretation.ports import Claim
    from qs_ai.domain.interpretation.artifact import ArtifactCandidate, MBTIArtifactCandidate
    from qs_ai.infrastructure.persistence.model_call_codec import JSONModelCallCodec
    from qs_ai.infrastructure.persistence.mysql.execution_configurations import read_configuration, validate_generation
    evidence_rows = match(ai["evidence_sets"], session_id=session.id)
    evidence = _evidence(one(evidence_rows, "original_evidence_missing")) if evidence_rows else None
    if evidence is not None:
        evidence.validate(session.testee_id, session.assessment_ids)
        if tuple(asdict(v) for v in evidence.items) != tuple(original.start.get("evidence", [])):
            # asdict tuples and JSON lists differ; compare their canonical bytes.
            if canonical([asdict(v) for v in evidence.items]) != canonical(original.start.get("evidence", [])):
                fail("original_evidence_source_conflict")
    config_rows = match(ai["execution_configurations"], session_id=session.id)
    if config_rows and evidence is None:
        fail("original_configuration_without_evidence")
    # A native frozen configuration is validated through current read-only domain helpers.
    if config_rows:
        await read_configuration(db, session, evidence)
    records = match(ai["interpretation_runs"], session_id=session.id)
    if not records or text(one(match(records, id=session.active_run_id), "active_run_missing")["status"]) != str(session.status):
        fail("original_terminal_run_conflict")
    codec = JSONModelCallCodec()
    for run in records:
        rid = text(run["id"])
        status = text(run["status"])
        if status not in ("completed", "cancelled", "blocked", "awaiting_answer"):
            fail("original_execution_responsibility_open")
        jobs = match(ai["execution_jobs"], run_id=rid)
        calls = match(ai["model_calls"], run_id=rid)
        if not jobs:
            # Only known original pre-dispatch refusals have no execution Job.
            if status != "blocked" or calls:
                fail("original_execution_job_missing")
            state = [doc(v, "payload") for v in match(ai["result_outbox"], session_id=session.id)
                     if number(v["version"]) == number(run["session_version"])]
            if not state or any(v["failure_code"] not in ("configuration_unavailable", "admission_configuration_invalid", "admission_input_invalid", "participant_daily_capacity_exceeded") for v in state):
                fail("original_predispatch_refusal_not_proven")
            continue
        job = one(jobs, "duplicate_original_execution_job")
        if text(job["status"]) not in ("done", "dead", "cancelled"):
            fail("original_execution_job_open")
        if number(job["attempt"]) > 3:
            fail("original_execution_attempt_budget_conflict")
        if calls:
            call = one(calls, "duplicate_original_model_call")
            if text(call["status"]) in ("dispatched", "unknown"):
                fail("provider_result_unknown")
            if text(call["status"]) not in ("response_received", "failed"):
                fail("unsupported_original_model_call_status")
            frozen = codec.decode_request(text(call["request_json"]))
            if frozen.version != "qs-ai-generation/v1":
                fail("unsupported_original_generation_schema")
            if config_rows:
                _, verified = await validate_generation(db, session, evidence, text(call["request_json"]))
                if frozen != verified:
                    fail("original_generation_configuration_conflict")
            elif session.uses_published_snapshot:
                fail("original_generation_configuration_missing")
            if text(call["status"]) == "response_received":
                response = codec.decode_response(text(call["response_json"]))
                if response.invocation_id != text(call["invocation_id"]) or not response.request_id or response.model != frozen.route.model:
                    fail("original_provider_response_identity_conflict")
            elif not text(call["failure_code"]) or call["response_json"] is not None:
                fail("original_provider_failure_evidence_conflict")
    # Retry requests reuse exact stored frozen requests and first original receipts.
    for retry in match(ai["participant_retries"], session_id=session.id):
        receipt = doc(retry, "receipt")
        fixed_fields(receipt, "session_id run_id status version", "unsupported_original_retry_receipt")
        if receipt["session_id"] != session.id or receipt["run_id"] != text(retry["run_id"]) or receipt["version"] != number(retry["expected_version"]) + 1:
            fail("original_retry_receipt_conflict")
        source_calls = match(ai["model_calls"], run_id=text(retry["source_run_id"]))
        raw = retry["frozen_request_json"]
        if source_calls and raw != source_calls[0]["request_json"]:
            fail("original_retry_frozen_request_conflict")
        if raw is not None:
            codec.decode_request(text(raw))
        prior = one(match(ai["idempotency_requests"], scope_hash=sha(canonical(["participant-retry-v1", number(retry["organization_id"])])), key=text(retry["command_id"])), "original_retry_acceptance_missing")
        # These two values are fixed by ParticipantRetry.__post_init__ in the
        # original writer, not evidence that a provider attempt took place.
        from qs_ai.application.execution.retry import ParticipantRetry
        from qs_ai.application.governance.prompt_drafts import DraftScope
        command = ParticipantRetry(DraftScope(number(retry["organization_id"]), number(retry["operator_user_id"])),
            session.id, text(retry["command_id"]), text(retry["source_run_id"]),
            number(retry["expected_version"]), text(retry["reason"]), True, 1,
            boolean(retry["accepted_unknown_risk"]))
        if text(prior["request_hash"]) != sha(canonical(asdict(command))) or doc(prior, "response") != receipt:
            fail("original_retry_acceptance_conflict")
    reservations = match(ai["participant_capacity_reservations"], session_id=session.id)
    if any(number(v["active"]) for v in reservations):
        fail("original_capacity_responsibility_open")
    if any(v["acquired_at"] is not None and v["released_at"] is None for v in reservations):
        fail("original_acquired_capacity_unreleased")
    leases = match(ai["execution_leases"], thread_id=session.id)
    if any(datetime.fromisoformat(text(v["expires_at"])).replace(tzinfo=timezone.utc) > datetime.now(timezone.utc) for v in leases):
        fail("original_execution_lease_active")
    if str(session.status) != "completed":
        if match(ai["interpretation_artifacts"], session_id=session.id):
            fail("noncompleted_session_has_artifact")
        return "known_terminal_without_artifact"
    artifact_row = one(match(ai["interpretation_artifacts"], session_id=session.id), "original_completed_artifact_missing")
    value = doc(artifact_row, "payload")
    kind = MBTIArtifactCandidate if value.get("schema_version") == "qs-ai-artifact/v2" else ArtifactCandidate
    if value.get("schema_version") not in ("qs-ai-artifact/v1", "qs-ai-artifact/v2") or set(value) != {f.name for f in fields(kind)}:
        fail("unsupported_original_artifact_schema")
    candidate = kind(**value)
    call = one(match(ai["model_calls"], run_id=candidate.run_id), "original_artifact_modelcall_missing")
    job = one(match(ai["execution_jobs"], run_id=candidate.run_id), "original_artifact_job_missing")
    if evidence is None or candidate.session_id != session.id or candidate.id != text(artifact_row["id"]) or candidate.run_id != text(artifact_row["run_id"]) or candidate.run_id != session.active_run_id or candidate.evidence_set_id != evidence.id or candidate.evidence_fingerprint != evidence.fingerprint or candidate.invocation_id != text(call["invocation_id"]) or candidate.content_fingerprint != "sha256:" + sha(candidate.content_json.encode()):
        fail("original_artifact_business_binding_conflict")
    response = codec.decode_response(text(call["response_json"]))
    if candidate.provider_request_id != response.request_id or candidate.id != str(uuid5(NAMESPACE_URL, f"qs-ai:artifact:{candidate.run_id}:{candidate.invocation_id}")):
        fail("original_artifact_provider_binding_conflict")
    if config_rows:
        config, frozen = await validate_generation(db, session, evidence, text(call["request_json"]))
        claim = Claim(text(job["id"]), candidate.run_id, session, number(call["fence_token"]), text(job["answer"]), boolean(job["skipped"]), text(job["question_id"]))
        if build_artifact(claim, evidence, GeneratedExplanation(frozen, response), config.parser) != candidate:
            fail("original_artifact_typed_output_conflict")
    elif session.uses_published_snapshot:
        fail("original_artifact_configuration_missing")
    else:
        # Decode and validate against the original model-call schema and input,
        # never today's schema/configuration. This still cannot prove a missing
        # historical publication/configuration ledger.
        from qs_ai.infrastructure.qs_server.output import QSOutputParser
        frozen = codec.decode_request(text(call["request_json"]))
        claim = Claim(text(job["id"]), candidate.run_id, session, number(call["fence_token"]), text(job["answer"]), boolean(job["skipped"]), text(job["question_id"]))
        if build_artifact(claim, evidence, GeneratedExplanation(frozen, response), QSOutputParser.from_schema(frozen.schema)) != candidate:
            fail("historical_original_artifact_typed_output_conflict")
        return "historical_original_configuration_not_retained"
    return "frozen_configuration_and_artifact_verified"



class _Scan:
    def __init__(self, bounds, observation, module, *, seal):
        if seal is not _SEAL:
            fail("actual_scan_required")
        self.bounds, self.observation, self.module = bounds, observation, module
        self.approved_bounds_sha256 = bounds.digest()
        self.physical_rows = observation._rows
        self.lineage = None
        rows = observation._rows
        if bounds.side == "ai" and bounds.head == "0040_module_table_names":
            rows, self.lineage = module._project(rows, bounds.tables)
        self.rows = {name: _SourceRows(values) for name, values in rows.items()}
        self.sections = observation._sections

    def __repr__(self):
        return "<private full-ledger source scan>"


async def _scan(session, bounds, approved_sha):
    if not isinstance(bounds, FullBounds) or approved_sha != bounds.digest():
        fail("independently_approved_bounds_required")
    module = _scanner(bounds.side, bounds.head)
    reader = module._Borrowed(session)
    try:
        if await _catalog(reader, bounds.side) != bounds.catalog_sha256:
            fail("source_catalog_changed")
        original = module.PrivateBounds(bounds.identity_hash, bounds.source_sha, bounds.head, bounds.tables)
        observed = await module.observe(session, original, approved_bounds_sha256=original.digest())
        return _Scan(bounds, observed, module, seal=_SEAL)
    except module.Rejected as e:
        fail(str(e))


def _decoded_row(row, json_keys=(), binary_keys=()):
    result = {}
    for k, raw in row.items():
        if k in json_keys:
            result[k] = None if raw is None else json_value(raw)
        elif k in binary_keys:
            result[k] = raw
        else:
            value = text(raw)
            result[k] = int(value) if value is not None and re.fullmatch(r"0|[1-9][0-9]*", value) else value
    return result


async def _evaluations(ai, db, originals=None, peer=None):
    from qs_ai.infrastructure.persistence.mysql.evaluation_creation_receipt import creation_receipt
    from qs_ai.infrastructure.persistence.mysql.evaluation_frozen_policies import frozen_policies
    from qs_ai.infrastructure.persistence.mysql.evaluation_projection import project_slots
    from qs_ai.infrastructure.persistence.mysql.evaluation_checkpoints import decode
    from qs_ai.infrastructure.persistence.mysql.evaluation_slot_claims import SlotClaim, terminal_dispatches
    from qs_ai.infrastructure.persistence.mysql.evaluation_response_receipts import decode as decode_response
    from qs_ai.infrastructure.persistence.mysql.evaluation_assets import stored_run_suite
    from qs_ai.infrastructure.persistence.mysql.evaluation_contracts import semantic_contract
    from qs_ai.domain.evaluation.identity import EvidenceReleaseIdentity, FrozenContractRef
    scope = _original_execution_scope(ai, peer, originals) if peer is not None else None
    result = {"non_target_runs": 0, "legitimate_pending_runs": 0, "terminal_runs": 0,
              "outside_retirement_provider_result_unknown": 0}
    for row in ai["evaluation_runs"]:
        rid = text(row["run_id"])
        original_uuid(rid)
        typed = _decoded_row(row, ("progress_json",))
        creation_receipt(typed)  # actual original audit + release identity validation
        creation = json_value(row["definition_json"])
        policy, _ = frozen_policies(creation)
        suite = await stored_run_suite(db, creation)
        if [(s["case_id"], s["ordinal"]) for s in creation["slots"]] != list(suite.slots()) or len(suite.generation_case_ids) != policy.generation_cases or suite.repetitions != policy.candidates_per_case:
            fail("evaluation_frozen_slot_scope_conflict")
        release = EvidenceReleaseIdentity(**{k: FrozenContractRef(**v) for k, v in creation["release"].items()})
        await semantic_contract(db, release, number(row["organization_id"]), frozen=creation)
        stored_policy = one(match(ai["evaluation_run_policies"], run_id=rid), "evaluation_frozen_policy_missing")
        if text(stored_policy["definition_json"]) != policy.definition_json or text(stored_policy["fingerprint"]) != policy.fingerprint:
            fail("evaluation_frozen_policy_conflict")
        progress = doc(row, "progress_json")
        if progress is None:
            progress = {"status": creation["status"], "transitions": creation["transitions"]}
        status = progress.get("status")
        if status not in ("requested", "collecting", "awaiting_review", "blocked", "canceled", "approved", "rejected"):
            fail("unsupported_evaluation_lifecycle")
        cp = one(match(ai["evaluation_checkpoints"], run_id=rid), "evaluation_coordinator_missing")
        checkpoint = decode(doc(cp, "checkpoint_json"))
        claims = []
        for r in match(ai["evaluation_slot_claims"], run_id=rid):
            frozen = decode(doc(r, "checkpoint_json"))
            if frozen is None or frozen.case_id != text(r["case_id"]) or frozen.slot_ordinal != number(r["slot_ordinal"]):
                fail("evaluation_claim_binding_conflict")
            claims.append(SlotClaim(UUID(rid), number(r["version"]), frozen))
        mode = text(row["execution_mode"])
        if mode == "serial_v1":
            if claims:
                fail("serial_evaluation_has_candidate_claim")
            if checkpoint is not None:
                claims.append(SlotClaim(UUID(rid), number(cp["version"]), checkpoint))
        elif mode == "candidate_v2":
            if checkpoint is not None:
                fail("candidate_evaluation_has_serial_claim")
        else:
            fail("unsupported_evaluation_execution_mode")
        dispatches = [_decoded_row(r, ("checkpoint_json",)) for r in match(ai["evaluation_dispatches"], run_id=rid)]
        generations = [_decoded_row(r, ("candidate_json", "evidence_json"), ("raw_output", "normalized_output")) for r in match(ai["evaluation_generation_completions"], run_id=rid)]
        semantics = [_decoded_row(r, ("evidence_json", "result_json"), ("raw_output", "normalized_output")) for r in match(ai["evaluation_semantic_completions"], run_id=rid)]
        unknown = sum(r["evidence_json"].get("status") == "result_unknown" for r in generations + semantics)
        if unknown:
            if scope is None:
                fail("original_source_scope_required_for_provider_responsibility")
            # Evaluation is a separate actual owner graph, not an interpretation
            # execution. A colliding original identity cannot prove unrelated.
            if rid in scope[0] | scope[1] | scope[2]:
                fail("evaluation_provider_result_unknown")
            result["outside_retirement_provider_result_unknown"] += unknown
        terminal = terminal_dispatches(dispatches, generations + semantics, tuple(claims))
        project_slots(creation["slots"], generations, terminal, semantics,
                      progress.get("result_unknown_resolutions", []),
                      progress.get("semantic_contract_recoveries", []), policy=policy)
        if len(dispatches) > policy.generation_per_run + policy.semantic_per_run or len(generations) > policy.generation_per_run or len(semantics) > policy.semantic_per_run:
            fail("evaluation_frozen_budget_conflict")
        for response in match(ai["evaluation_response_receipts"], run_id=rid):
            value = decode_response(text(response["definition_json"]), text(response["sha256"]))
            entry = one([d for d in dispatches if d["invocation_id"] == text(response["invocation_id"])], "evaluation_response_dispatch_missing")
            if entry["execution_id"] != text(response["execution_id"]) or (value.response and value.response.invocation_id != entry["invocation_id"]):
                fail("evaluation_response_binding_conflict")
        if claims and status in ("canceled", "approved", "rejected", "awaiting_review"):
            fail("terminal_evaluation_has_execution_responsibility")
        for reservation in match(ai["evaluation_capacity_reservations"], run_id=rid):
            if number(reservation["organization_id"]) != number(row["organization_id"]) or text(reservation["requested_by"]) != text(row["requested_by"]):
                fail("evaluation_capacity_owner_conflict")
        for sequence in match(ai["ai_messaging_evaluation_sequences"], run_id=rid):
            if number(sequence["version"]) > number(cp["version"]):
                fail("evaluation_state_version_conflict")
        result["non_target_runs"] += 1
        if status in ("requested", "collecting", "blocked"):
            result["legitimate_pending_runs"] += 1
        else:
            result["terminal_runs"] += 1
    return result


@dataclass(frozen=True, repr=False)
class ProtectionKeys:
    decrypt_keys: dict
    trusted_signers: dict

    def __repr__(self):
        return "<host-owned message protection keys>"


def _mq(ai, peer, originals, keys, *, deadline=None):
    deadline = time.monotonic() + TOTAL_SECONDS if deadline is None else deadline
    def require_budget():
        if time.monotonic() >= deadline:
            fail("full_qualification_deadline_exceeded")
    # Import the real host/SDK contract. No local substitute is permitted.
    any_mq = any(ai[t] or peer[t] for t in ("ai_messaging_outbox", "ai_messaging_inbox", "ai_messaging_quarantine"))
    if not any_mq and not peer["ai_messaging_operations"] and not peer["ai_messaging_failures"]:
        if peer["ai_messaging_aggregates"] or peer["ai_messaging_evaluation_states"] or ai["ai_messaging_evaluation_sequences"]:
            fail("global_orphan_mq_auxiliary_fact")
        return {"authenticated_messages": 0, "target_mq_responsibilities": 0, "non_target_pending_messages": 0}
    if not isinstance(keys, ProtectionKeys) or not keys.decrypt_keys or not keys.trusted_signers:
        fail("host_message_protection_keys_required")
    try:
        from qs_ai.infrastructure.workflow_transport.messaging import authenticate, parse_body, FIELDS
        from qs_ai.contracts.workflow import messaging_pb2 as pb
    except ImportError:
        fail("real_sdk_protection_unavailable")
    messages = {}
    requests = {text(r["request_id"]): r for r in peer["ai_bridge_requests"]}
    sessions = {text(r["id"]): r for r in ai["interpretation_sessions"]}
    evaluations = {text(r["run_id"]): r for r in ai["evaluation_runs"]}
    if set(requests) & set(evaluations):
        fail("cross_domain_aggregate_identity_ambiguous")
    targets = {o.request_id for o in originals}
    target_sessions = {o.session_id for o in originals}
    counts = {"authenticated_messages": 0, "target_mq_responsibilities": 0, "non_target_pending_messages": 0}
    for side, dataset in (("ai", ai), ("peer", peer)):
        for row in dataset["ai_messaging_outbox"]:
            require_budget()
            if sha(row["wire"]) != text(row["wire_sha256"]) or sha(row["body"]) != text(row["body_sha256"]):
                fail("mq_original_physical_digest_conflict")
            try:
                env = authenticate(row["wire"], text(row["topic"]), decrypt_keys=keys.decrypt_keys,
                                   trusted_signers=keys.trusted_signers)
                body = parse_body(env, row["body"])
                clone = type(env)(); clone.CopyFrom(env); clone.DiscardUnknownFields()
                body_clone = type(body)(); body_clone.CopyFrom(body); body_clone.DiscardUnknownFields()
                if clone != env or body_clone != body or body.SerializeToString(deterministic=True) != row["body"]:
                    fail("unsupported_mq_protobuf_fields_or_encoding")
            except Exception:
                fail("mq_real_authentication_or_body_rejected")
            ident = (env.producer, env.destination, env.message_id)
            if ident in messages:
                fail("duplicate_global_message_identity")
            if ident != tuple(text(row[k]) for k in ("producer", "destination", "message_id")) or env.kind != number(row["kind"]) or env.aggregate_key != text(row["aggregate_key"]) or env.body_sha256 != text(row["body_sha256"]):
                fail("mq_outer_inner_identity_conflict")
            value = getattr(body, FIELDS[env.kind])
            org = None
            if env.kind in (pb.START, pb.CHANGE):
                org = value.actor.org_id
            elif env.kind in (pb.PARTICIPANT_RETRY, pb.EVALUATION_START, pb.EVALUATION_CANCEL):
                org = str(value.scope.organization_id)
            elif env.kind == pb.INTERPRETATION_STATE:
                org = value.actor.org_id
            elif env.kind == pb.EVALUATION_STATE:
                org = value.organization_id
            # ACK ownership is independently bound to its original event below.
            if org is not None and org != str(number(row["organization_id"])):
                fail("mq_cross_organization_fact")
            if side == "ai" and env.producer != "qs-ai" or side == "peer" and env.producer != "qs-server":
                fail("mq_wrong_database_owner")
            target = env.aggregate_key in targets
            if env.kind == pb.START:
                request = requests.get(value.request_id)
                if request is None or env.aggregate_key != value.request_id:
                    fail("mq_start_original_request_missing")
                parsed, _ = _start(doc(request, "payload"))
                if text(request["request_hash"]) != sha(_go_json(parsed)) or (text(request["organization_id"]), text(request["subject_id"]), text(request["testee_id"])) != (parsed["actor"]["org_id"], parsed["actor"]["subject_id"], parsed["testee_id"]):
                    fail("mq_start_original_owner_or_writer_conflict")
                actual = {"request_id": value.request_id, "actor": {"org_id": value.actor.org_id, "subject_id": value.actor.subject_id},
                          "testee_id": value.testee_id, "assessment_ids": list(value.assessment_ids), "goal": value.goal,
                          "evidence": [{"assessment_id": v.assessment_id, "testee_id": v.testee_id, "report_id": v.report_id,
                                        "source_version": v.source_version, "facts": [{"ref": f.ref, "value": f.value} for f in v.facts]} for v in value.evidence]}
                if _start(actual)[0] != parsed:
                    fail("mq_start_original_writer_conflict")
            elif env.kind in (pb.CHANGE, pb.PARTICIPANT_RETRY):
                owner = sessions.get(value.session_id)
                if owner is None or str(number(owner["org_id"])) != org or requests.get(env.aggregate_key) is None:
                    fail("mq_change_or_retry_owner_missing")
                if text(requests[env.aggregate_key]["session_id"]) != value.session_id:
                    fail("mq_change_or_retry_original_conflict")
                target |= value.session_id in target_sessions
            elif env.kind in (pb.EVALUATION_START, pb.EVALUATION_CANCEL, pb.EVALUATION_STATE):
                rid = value.run_id if env.kind == pb.EVALUATION_STATE else value.scope.run_id
                owner = evaluations.get(rid)
                if owner is None or str(number(owner["organization_id"])) != org or env.aggregate_key != rid:
                    fail("mq_evaluation_owner_missing")
            elif env.kind == pb.INTERPRETATION_STATE:
                result = one(match(ai["result_outbox"], event_id=value.event_id), "mq_original_state_event_missing")
                from qs_ai.contracts.workflow import workflow_pb2 as workflow
                if workflow.StateEvent(**doc(result, "payload")) != value or env.aggregate_key != value.request_id or number(row["aggregate_sequence"]) != value.version or not boolean(result["mq_owned"]):
                    fail("mq_original_state_binding_conflict")
            elif env.kind not in (pb.COMMAND_RECEIPT, pb.EVENT_ACKNOWLEDGEMENT):
                fail("unsupported_global_mq_kind")
            if number(row["attempts"]) >= 8 and text(row["stage"]) not in ("confirmed", "held"):
                fail("mq_attempt_budget_state_conflict")
            if text(row["stage"]) not in ("staged", "awaiting_receipt", "confirmed", "held"):
                fail("unsupported_global_mq_stage")
            is_command = env.kind in (pb.START, pb.CHANGE, pb.PARTICIPANT_RETRY, pb.EVALUATION_START, pb.EVALUATION_CANCEL)
            if boolean(row["ordered"]) != is_command or boolean(row["requires_receipt"]) != (env.kind != pb.EVENT_ACKNOWLEDGEMENT):
                fail("mq_ordering_or_receipt_contract_conflict")
            if target:
                counts["target_mq_responsibilities"] += 1
                if text(row["stage"]) != "confirmed" or row["confirmed_at"] is None:
                    fail("target_mq_delivery_responsibility_open")
            elif text(row["stage"]) != "confirmed":
                counts["non_target_pending_messages"] += 1
            messages[ident] = (row, env, body, target)
            counts["authenticated_messages"] += 1
    # Global command -> Inbox -> immutable first receipt -> peer Inbox -> ACK chain.
    inboxes = {}
    for side, dataset in (("ai", ai), ("peer", peer)):
        for row in dataset["ai_messaging_inbox"]:
            require_budget()
            producer, mid = text(row["producer"]), text(row["message_id"])
            destination = "qs-ai" if side == "ai" else "qs-server"
            message = messages.get((producer, destination, mid))
            if message is None:
                fail("global_orphan_mq_inbox")
            source, env, body, target = message
            if any(row[k] != source[k] for k in ("body", "body_sha256", "wire_sha256", "kind", "aggregate_key")):
                fail("mq_inbox_original_identity_conflict")
            inboxes[(producer, destination, mid)] = row
            if side == "ai":
                decision = text(row["decision"])
                if decision not in ("accepted", "rejected", "held", "processing"):
                    fail("unsupported_mq_inbox_decision")
                if target and decision not in ("accepted", "rejected"):
                    fail("target_mq_command_decision_open")
                receipt_id = text(row["receipt_id"])
                if decision in ("accepted", "rejected", "held"):
                    receipt = messages.get(("qs-ai", "qs-server", receipt_id))
                    if receipt is None or receipt[1].kind != pb.COMMAND_RECEIPT:
                        fail("mq_first_command_receipt_missing")
                    r = receipt[2].command_receipt
                    if r.command_id != mid or r.command_body_sha256 != env.body_sha256 or r.decision != {"accepted": pb.ACCEPTED, "rejected": pb.REJECTED, "held": pb.HELD}[decision] or number(receipt[0]["organization_id"]) != number(source["organization_id"]):
                        fail("mq_first_command_receipt_conflict")
                    if env.kind in (pb.START, pb.CHANGE, pb.PARTICIPANT_RETRY):
                        value = getattr(body, FIELDS[env.kind])
                        if env.kind == pb.START:
                            original = _start(doc(one(match(peer["ai_bridge_requests"], request_id=value.request_id), "mq_start_original_request_missing"), "payload"))[0]
                            scope = sha(canonical(["qs-server", "external-start-v1"]))
                            expected_hash = sha(canonical([original["actor"], original["testee_id"], original["assessment_ids"], original["goal"]] + original.get("evidence", [])))
                        elif env.kind == pb.CHANGE:
                            from qs_ai.application.interpretation.commands import AnswerCommand, CancelCommand
                            actor = {"org_id": value.actor.org_id, "subject_id": value.actor.subject_id}
                            scope = sha(canonical([actor, value.action, value.session_id]))
                            if value.action not in ("answer", "cancel"):
                                fail("unsupported_mq_change_action")
                            command = AnswerCommand(value.expected_version, value.question_id, value.answer if value.HasField("answer") else None, value.skip) if value.action == "answer" else CancelCommand(value.expected_version)
                            expected_hash = sha(canonical(asdict(command)))
                        else:
                            from qs_ai.application.execution.retry import ParticipantRetry
                            from qs_ai.application.governance.prompt_drafts import DraftScope
                            command = ParticipantRetry(DraftScope(value.scope.organization_id, value.scope.operator_user_id),
                                value.session_id, value.command_id, value.expected_run_id, value.expected_version,
                                value.reason, value.confirm, value.expected_provider_invocations, value.accept_result_unknown_risk)
                            scope = sha(canonical(["participant-retry-v1", command.scope.organization_id]))
                            expected_hash = sha(canonical(asdict(command)))
                            retry_rows = match(ai["participant_retries"], command_id=mid)
                            if decision == "accepted":
                                actual, _ = _retry_acceptance(ai, one(retry_rows, "mq_retry_original_fact_missing"))
                                if actual != command:
                                    fail("mq_retry_original_command_conflict")
                        prior = match(ai["idempotency_requests"], scope_hash=scope, key=mid)
                        has_workflow = r.HasField("workflow_receipt")
                        if decision == "accepted" and not has_workflow:
                            fail("mq_accepted_workflow_original_receipt_missing")
                        if has_workflow:
                            stored = one(prior, "mq_workflow_original_acceptance_missing")
                            from qs_ai.contracts.workflow import workflow_pb2 as workflow
                            if text(stored["request_hash"]) != expected_hash or workflow.Receipt(**doc(stored, "response")) != r.workflow_receipt:
                                fail("mq_workflow_original_acceptance_conflict")
                            if env.kind == pb.START:
                                first = r.workflow_receipt
                                if first.status == "blocked":
                                    state = one(match(ai["result_outbox"], session_id=first.session_id,
                                                      version=first.version), "mq_original_refusal_state_missing")
                                    code = doc(state, "payload")["failure_code"]
                                    statuses = {"configuration_unavailable": 9, "admission_configuration_invalid": 9,
                                                "admission_input_invalid": 3, "participant_daily_capacity_exceeded": 8}
                                    if decision != "rejected" or code not in statuses or r.code != code or r.grpc_status_code != statuses[code]:
                                        fail("mq_original_start_refusal_conflict")
                                elif first.status != "queued" or decision != "accepted" or r.code or r.grpc_status_code:
                                    fail("mq_original_start_acceptance_conflict")
            else:
                ack = messages.get(("qs-server", "qs-ai", text(row["ack_id"])))
                if ack is None or ack[1].kind != pb.EVENT_ACKNOWLEDGEMENT:
                    fail("mq_first_event_ack_missing")
                a = ack[2].event_acknowledgement
                outcome = text(row["outcome"])
                if a.event_id != mid or a.event_body_sha256 != env.body_sha256 or a.event_kind != env.kind or ack[1].aggregate_key != env.aggregate_key or number(ack[0]["organization_id"]) != number(source["organization_id"]) or a.outcome != {"stored": pb.MessagingEventAcknowledgement.STORED, "held": pb.MessagingEventAcknowledgement.TECHNICALLY_HELD}.get(outcome):
                    fail("mq_event_business_ack_conflict")
                if target and outcome != "stored":
                    fail("target_mq_peer_business_receipt_held")
    for ident, (row, env, body, target) in messages.items():
        require_budget()
        if env.kind in (pb.START, pb.CHANGE, pb.PARTICIPANT_RETRY, pb.EVALUATION_START, pb.EVALUATION_CANCEL):
            op = one(match(peer["ai_messaging_operations"], command_id=env.message_id), "mq_operation_missing")
            if number(op["retired"]) or number(op["kind"]) != env.kind or text(op["body_sha256"]) != env.body_sha256 or text(op["aggregate_key"]) != env.aggregate_key or number(op["organization_id"]) != number(row["organization_id"]) or number(op["aggregate_sequence"]) != number(row["aggregate_sequence"]):
                fail("mq_operation_original_binding_conflict")
            value = getattr(body, FIELDS[env.kind])
            subject = value.actor.subject_id if env.kind in (pb.START, pb.CHANGE) else str(value.scope.operator_user_id)
            resource = value.request_id if env.kind == pb.START else value.session_id if env.kind in (pb.CHANGE, pb.PARTICIPANT_RETRY) else value.scope.run_id
            if text(op["subject_id"]) != subject or text(op["resource_id"]) != resource:
                fail("mq_operation_subject_or_resource_conflict")
            received = inboxes.get(ident)
            if text(op["decision"]):
                receipt = messages.get(("qs-ai", "qs-server", text(op["receipt_id"])))
                if received is None or receipt is None or op["receipt"] != receipt[0]["body"] or text(op["decision"]) != text(received["decision"]):
                    fail("mq_peer_operation_first_receipt_conflict")
            if target and received is None:
                fail("target_mq_command_business_acceptance_missing")
        elif env.kind == pb.COMMAND_RECEIPT:
            r = body.command_receipt
            command = messages.get(("qs-server", "qs-ai", r.command_id))
            if command is None or command[1].body_sha256 != r.command_body_sha256 or command[1].aggregate_key != env.aggregate_key or number(command[0]["organization_id"]) != number(row["organization_id"]):
                fail("global_orphan_mq_command_receipt")
        elif env.kind == pb.EVENT_ACKNOWLEDGEMENT:
            a = body.event_acknowledgement
            event = messages.get(("qs-ai", "qs-server", a.event_id))
            if event is None or a.event_body_sha256 != event[1].body_sha256 or a.event_kind != event[1].kind:
                fail("global_orphan_mq_ack")
        if text(row["stage"]) == "confirmed" and env.kind in (pb.COMMAND_RECEIPT, pb.INTERPRETATION_STATE, pb.EVALUATION_STATE):
            received = inboxes.get(ident)
            if received is None or text(received["outcome"]) != "stored":
                fail("mq_confirmed_without_original_peer_business_ack")
    evaluation_events = {}
    accepted_evaluations = {}
    for ident, (row, env, body, target) in messages.items():
        require_budget()
        if env.kind != pb.EVALUATION_STATE:
            continue
        value = body.evaluation_state
        owner = evaluations[value.run_id]
        creation = doc(owner, "definition_json")
        cp = one(match(ai["evaluation_checkpoints"], run_id=value.run_id), "mq_evaluation_checkpoint_missing")
        if value.release_fingerprint != creation["release_fingerprint"] or not 1 <= value.version <= number(cp["version"]) or not value.event_sequence or number(row["aggregate_sequence"]) != value.event_sequence or env.message_id != str(uuid5(UUID(value.run_id), f"state:{value.version}")):
            fail("mq_original_evaluation_state_binding_conflict")
        evaluation_events.setdefault(value.run_id, []).append((row, env, body))
        received = inboxes.get(ident)
        if received is not None and text(received["outcome"]) == "stored":
            accepted_evaluations.setdefault(value.run_id, []).append((row, env, body))
    sequence_rows = {text(r["run_id"]): r for r in ai["ai_messaging_evaluation_sequences"]}
    if set(sequence_rows) != set(evaluation_events):
        fail("global_orphan_mq_evaluation_sequence")
    for rid, events in evaluation_events.items():
        states = sorted((body.evaluation_state for _, _, body in events), key=lambda s: s.event_sequence)
        last = states[-1]
        if [s.event_sequence for s in states] != list(range(1, len(states)+1)) or [s.version for s in states] != sorted(set(s.version for s in states)) or number(sequence_rows[rid]["sequence"]) != last.event_sequence or number(sequence_rows[rid]["version"]) != last.version:
            fail("mq_original_evaluation_sequence_gap")
    projections = {text(r["run_id"]): r for r in peer["ai_messaging_evaluation_states"]}
    if set(projections) != set(accepted_evaluations):
        fail("global_orphan_mq_evaluation_projection")
    for rid, events in accepted_evaluations.items():
        row, env, body = max(events, key=lambda v: v[2].evaluation_state.event_sequence)
        value, stored = body.evaluation_state, projections[rid]
        if stored["state"] != row["body"] or number(stored["organization_id"]) != int(value.organization_id) or number(stored["event_sequence"]) != value.event_sequence or number(stored["version"]) != value.version:
            fail("mq_original_evaluation_projection_conflict")
    # Failure/quarantine are responsibility facts, not broker delivery completion.
    for side, dataset in (("ai", ai), ("peer", peer)):
        for row in dataset["ai_messaging_quarantine"]:
            if sha(row["wire"]) != text(row["wire_sha256"]):
                fail("quarantine_physical_hash_conflict")
            # Untrusted logical identity cannot be safely declared unrelated.
            fail("global_unresolved_mq_quarantine")
    for row in peer["ai_messaging_failures"]:
        message = messages.get((text(row["producer"]), "qs-server", text(row["message_id"])))
        if message is None or text(row["body_sha256"]) != message[1].body_sha256 or number(row["kind"]) != message[1].kind or text(row["aggregate_key"]) != message[1].aggregate_key:
            fail("global_orphan_or_conflicting_mq_failure")
        if message[3] or number(row["attempts"]) >= 8:
            fail("unresolved_mq_failure_responsibility")
    for op in peer["ai_messaging_operations"]:
        if not number(op["retired"]) and ("qs-server", "qs-ai", text(op["command_id"])) not in messages:
            fail("global_orphan_mq_operation")
        if number(op["retired"]):
            fail("retired_operation_requires_original_retirement_verifier")
    aggregates = {text(r["aggregate_key"]): r for r in peer["ai_messaging_aggregates"]}
    command_groups = {}
    for row, env, body, target in messages.values():
        if env.producer == "qs-server" and env.kind != pb.EVENT_ACKNOWLEDGEMENT:
            command_groups.setdefault(env.aggregate_key, []).append(number(row["aggregate_sequence"]))
    if set(aggregates) != set(command_groups):
        fail("global_orphan_mq_aggregate")
    for key, sequences in command_groups.items():
        if sorted(sequences) != list(range(1, len(sequences)+1)) or number(aggregates[key]["next_sequence"]) != len(sequences)+1:
            fail("mq_original_aggregate_sequence_gap")
    for original in originals:
        if original.handoff_sha256:
            command = messages.get(("qs-server", "qs-ai", original.command_id))
            if command is None or command[1].body_sha256 != original.handoff_sha256 or number(command[0]["attempts"]) < min(original.attempts, 8):
                fail("original_handoff_identity_or_budget_conflict")
    return counts


class Qualification:
    def __init__(self, scans, originals, summary, *, seal):
        if seal is not _SEAL:
            fail("actual_full_qualification_required")
        self._scans, self._originals, self._summary = scans, originals, summary

    def __repr__(self):
        return "<private readonly full-ledger qualification>"

    def receipt(self):
        return {"protocol": "qs-ai-retirement-local-qualification/v1",
                "diagnostic_only": True, "drop_ready": False, "retirement_proven": False,
                "production_proof": False, "fence_verified": False, "broker_queue_coverage": False,
                "runtime_source_binding": "not_observed",
                "scope_from": "full_eof_original_source_scan",
                "originals": len(self._originals), "summary": self._summary,
                "snapshots": {s.bounds.side: {"source_sha": s.bounds.source_sha,
                    "identity_hash": s.bounds.identity_hash, "head": s.bounds.head,
                    "approved_bounds_sha256": s.approved_bounds_sha256, "catalog_sha256": s.bounds.catalog_sha256,
                    "sections": s.sections} for s in self._scans},
                "fresh_full_recheck_required": True,
                "cross_service_atomic_snapshot": False}


async def _verify(ai_session, peer_session, *, ai_bounds, peer_bounds,
                 approved_ai_bounds_sha256, approved_peer_bounds_sha256,
                 approved_original_sections, protection_keys=None):
    """Borrow two actual host RR/RO snapshots. Approval is independent of discovery."""
    if not isinstance(ai_bounds, FullBounds) or ai_bounds.side != "ai" or not isinstance(peer_bounds, FullBounds) or peer_bounds.side != "peer":
        fail("separately_bound_database_snapshots_required")
    started = time.monotonic()
    ai_scan = await _scan(ai_session, ai_bounds, approved_ai_bounds_sha256)
    peer_scan = await _scan(peer_session, peer_bounds, approved_peer_bounds_sha256)
    originals = _originals(peer_scan, approved_original_sections)
    ai, peer = ai_scan.rows, peer_scan.rows
    try:
        reverse = _reverse(ai, peer, originals)
        summary = {"historical_gap_candidates": 0, "typed_configuration_artifacts_verified": 0,
                   "known_terminal_without_artifact": 0, "participant_reverse": reverse}
        for original in originals:
            row = one(match(ai["interpretation_sessions"], id=original.session_id), "original_session_missing_or_ambiguous")
            session = _session(row)
            if (asdict(session.actor), session.testee_id, list(session.assessment_ids), session.goal) != (
                    original.start["actor"], original.start["testee_id"], original.start["assessment_ids"], original.start["goal"]):
                fail("original_session_owner_or_request_conflict")
            binding = one(match(ai["external_requests"], request_id=original.request_id), "original_external_request_missing")
            if text(binding["session_id"]) != session.id:
                fail("original_external_request_conflict")
            accepted = one(match(ai["idempotency_requests"], scope_hash=sha(canonical(["qs-server", "external-start-v1"])), key=original.request_id), "original_request_acceptance_missing")
            if text(accepted["request_hash"]) != original.original_request_sha256:
                fail("original_request_acceptance_hash_conflict")
            receipt = doc(accepted, "response")
            fixed_fields(receipt, "session_id run_id status version", "unsupported_original_request_receipt")
            original_uuid(receipt["run_id"])
            if receipt["session_id"] != session.id or receipt["run_id"] not in {text(v["id"]) for v in match(ai["interpretation_runs"], session_id=session.id)} or receipt["status"] not in ("queued", "blocked") or type(receipt["version"]) is not int or not 1 <= receipt["version"] <= session.version:
                fail("original_request_acceptance_identity_conflict")
            if str(session.status) not in ("completed", "cancelled", "blocked"):
                fail("original_business_not_terminal")
            events = _legacy_event_chain(ai, peer, original, session)
            if not any(doc(v, "payload")["version"] == receipt["version"] and doc(v, "payload")["status"] == receipt["status"] for v in events):
                fail("original_request_receipt_state_missing")
            remaining = TOTAL_SECONDS - (time.monotonic() - started)
            if remaining <= 0:
                fail("full_qualification_deadline_exceeded")
            async with asyncio.timeout(min(QUERY_SECONDS, remaining)):
                outcome = await _typed_execution(ai_session, ai, original, session)
            if outcome == "historical_original_configuration_not_retained":
                summary["historical_gap_candidates"] += 1
            elif outcome == "frozen_configuration_and_artifact_verified":
                summary["typed_configuration_artifacts_verified"] += 1
            else:
                summary["known_terminal_without_artifact"] += 1
        async with asyncio.timeout(min(QUERY_SECONDS, TOTAL_SECONDS - (time.monotonic() - started))):
            summary["evaluation"] = await _evaluations(ai, ai_session, originals, peer)
        summary["mq"] = _mq(ai, peer, originals, protection_keys, deadline=started + TOTAL_SECONDS)
        # Revalidate borrowed ownership after actual typed helper SELECTs.
        for scan, session in ((ai_scan, ai_session), (peer_scan, peer_session)):
            reader = scan.module._Borrowed(session)
            await scan.module._binding(reader, scan.bounds.identity_hash, scan.bounds.source_sha)
        return Qualification((ai_scan, peer_scan), originals, summary, seal=_SEAL)
    except Rejected:
        raise
    except TimeoutError:
        fail("typed_readonly_query_timeout")
    except Exception:
        fail("typed_original_fact_or_contract_rejected")


async def _recheck(ai_session, peer_session, qualification, *, protection_keys=None):
    """A new host snapshot repeats *all* original bytes, assets and reverse scans.

    Above-upper alone cannot notice changed owners, payloads or configuration below
    upper. The old snapshots must end before any fresh read is accepted.
    """
    if not isinstance(qualification, Qualification):
        fail("actual_full_qualification_required")
    old = qualification._scans
    if any(s.bounds.digest() != s.approved_bounds_sha256 for s in old):
        fail("original_approved_bounds_changed")
    if any(s.observation._original_tx.is_active for s in old):
        fail("original_snapshots_must_end_before_recheck")
    fresh = []
    for session, scan in zip((ai_session, peer_session), old):
        module = scan.module
        reader = module._Borrowed(session)
        if reader.tx is scan.observation._original_tx:
            fail("fresh_independent_snapshot_required")
        try:
            result = await module.fresh_after_upper(session, scan.observation)
            if any(v["next_cycle_required"] for v in result.values()):
                fail("new_source_rows_require_next_cycle")
            current = await _scan(session, scan.bounds, scan.approved_bounds_sha256)
            if current.sections != scan.sections:
                fail("full_source_or_owner_or_configuration_changed")
            fresh.append(current)
        except module.Rejected as e:
            fail(str(e))
    expected = {n: {k: fresh[1].sections[n][k] for k in ("rows", "source_bytes", "source_sha256")}
                for n in ("ai_bridge_commands", "ai_messaging_legacy_commands")}
    repeated = await verify(ai_session, peer_session, ai_bounds=old[0].bounds, peer_bounds=old[1].bounds,
                            approved_ai_bounds_sha256=old[0].approved_bounds_sha256,
                            approved_peer_bounds_sha256=old[1].approved_bounds_sha256,
                            approved_original_sections=expected, protection_keys=protection_keys)
    if repeated._summary != qualification._summary:
        fail("readonly_qualification_changed")
    return {"protocol": "qs-ai-full-ledger-fresh-recheck/v1", "unchanged_full_source_hashes": True,
            "both_original_snapshots_ended": True, "no_above_upper_rows": True,
            "drop_ready": False, "fence_verified": False, "broker_queue_coverage": False}


async def verify(*args, **kwargs):
    try:
        async with asyncio.timeout(TOTAL_SECONDS):
            return await _verify(*args, **kwargs)
    except TimeoutError:
        fail("full_qualification_deadline_exceeded")
    except Rejected:
        raise
    except Exception:
        fail("readonly_qualification_source_rejected")


async def recheck(*args, **kwargs):
    try:
        async with asyncio.timeout(TOTAL_SECONDS):
            return await _recheck(*args, **kwargs)
    except TimeoutError:
        fail("full_recheck_deadline_exceeded")
    except Rejected:
        raise
    except Exception:
        fail("readonly_recheck_source_rejected")
