"""Fixed synchronous host for actual qs-ai/peer read-only execution facts.

Executed only by ai_external_execution.go in the independently bound qs-ai
container. Credentials and keys enter through bounded stdin, never argv/logs.
This host owns its engines/sessions; the frozen verifier continues to borrow
them. No imported Qualification, report, completion flag or retirement verdict
is accepted. Full writer isolation and Broker coverage remain outside this host.
"""
from __future__ import annotations

import asyncio
import base64
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import stat
import sys
import tempfile

INPUT_LIMIT = 16 << 20
OUTPUT_LIMIT = 32 << 20
ORIGINAL_LIMIT = 10000
TOTAL_SECONDS = 1500
MODULES = {
    "qs-ai-retirement-readonly-verifier.py": "ed85cf9419ac5c17b5c886fb3fc83230054e89ea8227fb537a29a900db9bd4ec",
    "qs-ai-retirement-readonly-observer.py": "20c501d0930a21c1e8be12416156d5635430cf6171e01d28a6db789e83f06d5a",
    "qs-ai-retirement-0040-layout.py": "690d68be91713641ad6ce13ad9ad126828c64fe780bda05522bd68697ead577f",
}
FIELDS = frozenset("protocol source_sha operation_id run_id runtime_source_sha runtime_binding_sha256 runtime_messaging image_id container_id ai_bounds ai_bounds_sha256 peer_bounds peer_bounds_sha256 original_sections peer_connection protection modules".split())
PROFILE = [1000, 1000000, 2 << 30, 256 << 20, 32 << 20, 30, 1500]


class Rejected(Exception):
    pass


class DiscardText:
    """Do not retain incidental stdout/stderr, including credential text."""
    def __init__(self):
        self.buffer = self

    def write(self, value):
        return len(value)

    def flush(self):
        pass

    def isatty(self):
        return False


def reject():
    raise Rejected() from None


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def canonical(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True,
                      separators=(",", ":"), allow_nan=False).encode()


def decode(raw):
    def pairs(items):
        value = {}
        for key, item in items:
            if key in value:
                reject()
            value[key] = item
        return value
    try:
        return json.loads(raw, object_pairs_hook=pairs,
                          parse_constant=lambda _: reject())
    except Exception:
        reject()


def exact(value, keys):
    if not isinstance(value, dict) or set(value) != set(keys.split()):
        reject()


def pattern(value, regex):
    if not isinstance(value, str) or not re.fullmatch(regex, value):
        reject()


def input_packet(raw):
    if not raw or len(raw) > INPUT_LIMIT:
        reject()
    value = decode(raw)
    if not isinstance(value, dict) or set(value) != FIELDS or value["protocol"] != "qs-ai-readonly-host-input/v2":
        reject()
    for key in ("source_sha", "runtime_source_sha"):
        pattern(value[key], r"[0-9a-f]{40}")
    pattern(value["operation_id"], r"[0-9]{1,20}-[0-9]{1,4}")
    pattern(value["run_id"], r"[1-9][0-9]{0,19}")
    pattern(value["image_id"], r"sha256:[0-9a-f]{64}")
    pattern(value["container_id"], r"[0-9a-f]{64}")
    pattern(value["runtime_binding_sha256"], r"[0-9a-f]{64}")
    if value["runtime_messaging"] is not None:
        exact(value["runtime_messaging"], "enabled nsqd signing_key_file decrypt_key_files qs_signer_files qs_recipient_key_file max_in_flight")
    for side in ("ai", "peer"):
        pattern(value[side + "_bounds_sha256"], r"[0-9a-f]{64}")
        raw_bounds = base64.b64decode(value[side + "_bounds"], validate=True)
        if len(raw_bounds) > 4 << 20 or digest(raw_bounds) != value[side + "_bounds_sha256"]:
            reject()
    exact(value["original_sections"], "ai_bridge_commands ai_messaging_legacy_commands")
    for section in value["original_sections"].values():
        exact(section, "rows source_bytes source_sha256")
        if any(type(section[k]) is not int or section[k] < 0 for k in ("rows", "source_bytes")):
            reject()
        pattern(section["source_sha256"], r"[0-9a-f]{64}")
    if sum(s["rows"] for s in value["original_sections"].values()) > ORIGINAL_LIMIT:
        reject()
    exact(value["peer_connection"], "host port database username password")
    for k in ("host", "database", "username", "password"):
        item = value["peer_connection"][k]
        if not isinstance(item, str) or not item or len(item.encode()) > 4096 or any(x in item for x in ("\x00", "\n", "\r")):
            reject()
    port = value["peer_connection"]["port"]
    if type(port) is not int or not 1 <= port <= 65535:
        reject()
    exact(value["protection"], "decrypt_keys trusted_signers")
    if any(not isinstance(v, dict) or len(v) > 32 for v in value["protection"].values()):
        reject()
    if not isinstance(value["modules"], dict) or set(value["modules"]) != set(MODULES):
        reject()
    return value


def load_verifier(packet, directory):
    for name, expected in MODULES.items():
        raw = base64.b64decode(packet["modules"][name], validate=True)
        if len(raw) > 1 << 20 or digest(raw) != expected:
            reject()
        fd = os.open(directory / name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, "wb") as output:
            output.write(raw)
            output.flush()
            os.fsync(output.fileno())
    spec = importlib.util.spec_from_file_location("_fixed_qs_ai_actual_host_verifier",
                                                 directory / "qs-ai-retirement-readonly-verifier.py")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def bounds(module, packet, side):
    raw = base64.b64decode(packet[side + "_bounds"], validate=True)
    value = decode(raw)
    exact(value, "protocol side source_sha identity_hash head catalog_sha256 tables profile")
    if value["protocol"] != "qs-ai-full-ledger-bounds/v1" or value["side"] != side or value["profile"] != PROFILE:
        reject()
    pattern(value["identity_hash"], r"[0-9a-f]{64}")
    pattern(value["source_sha"], r"[0-9a-f]{40}")
    pattern(value["catalog_sha256"], r"[0-9a-f]{64}")
    if not isinstance(value["tables"], dict):
        reject()
    if side == "peer" and value["source_sha"] != packet["source_sha"]:
        reject()
    if side == "ai" and value["head"] not in (module.AI_HEAD, "0040_module_table_names"):
        reject()
    if side == "peer" and not re.fullmatch(r"[1-9][0-9]{0,8}", value["head"]):
        reject()
    result = module.FullBounds(side, value["source_sha"], value["identity_hash"], value["head"],
                               value["catalog_sha256"], value["tables"])
    # Approval binds the original canonical bytes, not a reserialized substitute.
    if result.private_bytes() != raw or result.digest() != packet[side + "_bounds_sha256"]:
        reject()
    return result


def runtime_key(path):
    # Only actual frozen Settings paths derived from the deployment generator.
    # Source bytes never leave this process, and the named/FD identity is checked.
    pattern(path, r"/run/qs-ai-jose/(?:ai\.sign|ai\.encrypt|qs\.sign|qs\.encrypt)\.[a-z0-9][a-z0-9._-]{0,63}\.json")
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or before.st_nlink != 1 or before.st_size > 65536 or before.st_mode & 0o022:
            reject()
        raw = os.read(fd, 65537)
        after, visible = os.fstat(fd), os.stat(path, follow_symlinks=False)
        stamp = lambda s: (s.st_dev, s.st_ino, s.st_mode, s.st_uid, s.st_gid, s.st_size, s.st_mtime_ns, s.st_ctime_ns, s.st_nlink)
        if len(raw) != before.st_size or stamp(before) != stamp(after) or stamp(after) != stamp(visible):
            reject()
        return decode(raw), (stamp(before), digest(raw))
    finally:
        os.close(fd)


def protection(module, packet, settings):
    from jwcrypto import jwk
    from reliable_messaging.protected import TrustedSigner
    values = packet["protection"]
    decrypt, signers, signer_fingerprints = {}, {}, {}
    for kid, raw in values["decrypt_keys"].items():
        # The peer host may explicitly provide historical QS decrypt keys.
        # AI private keys must come from this image's actual mounted Settings.
        pattern(kid, r"qs\.encrypt\.[a-z0-9][a-z0-9._-]{0,63}")
        key = jwk.JWK.from_json(json.dumps(raw))
        if key.get("kid") != kid or not key.has_private or key.get("kty") != "EC" or key.get("crv") != "P-256":
            reject()
        decrypt[kid] = key
    for kid, value in values["trusted_signers"].items():
        exact(value, "producer key")
        producer = value["producer"]
        if producer != "qs-ai":
            reject()
        pattern(kid, r"ai\.sign\.[a-z0-9][a-z0-9._-]{0,63}")
        key = jwk.JWK.from_json(json.dumps(value["key"]))
        if key.get("kid") != kid or key.has_private or key.get("kty") != "EC" or key.get("crv") != "P-256":
            reject()
        signers[kid] = TrustedSigner(producer, key)
        signer_fingerprints[kid] = key.thumbprint()
    options = settings.messaging
    expected = packet["runtime_messaging"]
    if expected is None:
        if options.enabled:
            reject()
        return module.ProtectionKeys(decrypt, signers), {}
    if options.model_dump(mode="json") != expected or options.enabled is not True:
        reject()
    baseline = {}
    def configured(path, private, kid, role):
        value, physical = runtime_key(path)
        key = jwk.JWK.from_json(json.dumps(value))
        pattern(key.get("kid"), re.escape(role) + r"\.[a-z0-9][a-z0-9._-]{0,63}")
        if key.get("kid") != kid or Path(path).name != kid + ".json" or key.has_private != private or key.get("kty") != "EC" or key.get("crv") != "P-256":
            reject()
        baseline[path] = physical
        return key
    for kid, path in options.decrypt_key_files.items():
        decrypt[kid] = configured(path, True, kid, "ai.encrypt")
    for kid, path in options.qs_signer_files.items():
        signers[kid] = TrustedSigner("qs-server", configured(path, False, kid, "qs.sign"))
    signing_id = Path(options.signing_key_file).stem
    signing = configured(options.signing_key_file, True, signing_id, "ai.sign")
    # Export only the public signing key inside this process. Never send it or
    # private keys to stdout; old unmounted signers remain explicit peer input.
    public_signing = jwk.JWK.from_json(signing.export_public())
    if signing_id in signers and signer_fingerprints[signing_id] != public_signing.thumbprint():
        reject()
    signers[signing_id] = TrustedSigner("qs-ai", public_signing)
    recipient_id = Path(options.qs_recipient_key_file).stem
    recipient = configured(options.qs_recipient_key_file, False, recipient_id, "qs.encrypt")
    if recipient_id in decrypt and decrypt[recipient_id].thumbprint() != recipient.thumbprint():
        reject()
    # This public recipient cannot supply the corresponding private decrypt
    # key. If an actual peer wire needs a missing kid, the real SDK rejects it.
    return module.ProtectionKeys(decrypt, signers), baseline


def framed_digest(parts):
    # Exact Go sourceFrame framing. SQL NULL remains distinct from empty bytes.
    result = hashlib.sha256()
    for raw in parts:
        result.update(b"\x00" + (0).to_bytes(8, "big") if raw is None else
                      b"\x01" + len(raw).to_bytes(8, "big") + raw)
    return result.hexdigest()


def peer_row_facts(module, scan):
    facts = []
    for table, rows in scan.rows.items():
        columns = [c[0] for c in scan.bounds.tables[table]["columns"]]
        keys = module.PEER_SPECS[table][1].split()
        for row in rows:
            parts = [b"ai-reverse-native-row/v1"]
            for column in columns:
                parts.extend((column.encode(), row[column]))
            facts.append({"store": table,
                "primary_key_sha256": framed_digest([b"ai-reverse-native-pk/v1"] + [row[k] for k in keys]),
                "row_sha256": framed_digest(parts)})
    return facts


async def close_sessions(sessions):
    failed = False
    for session in sessions:
        try:
            await session.rollback()
        except Exception:
            failed = True
        try:
            await session.close()
        except Exception:
            failed = True
    if failed:
        reject()


async def open_snapshots(engines):
    from sqlalchemy import text
    from sqlalchemy.ext.asyncio import AsyncSession
    sessions = []
    try:
        for engine in engines:
            session = AsyncSession(engine, expire_on_commit=False)
            sessions.append(session)
            await session.connection(execution_options={"isolation_level": "REPEATABLE READ"})
            await session.execute(text("START TRANSACTION WITH CONSISTENT SNAPSHOT, READ ONLY"))
        return sessions
    except Exception:
        await close_sessions(sessions)
        raise


async def execute(packet, module):
    from sqlalchemy import URL
    from sqlalchemy.ext.asyncio import create_async_engine
    from qs_ai.config import Settings
    # Use the running image's actual settings. There is intentionally no ai URL
    # input, qs-server DB fallback or default namespace.
    if os.environ.get("QS_AI_RELEASE_SHA") != packet["runtime_source_sha"] or os.environ.get("QS_AI_ENVIRONMENT") != "production":
        reject()
    settings = Settings()
    if settings.database_url is None:
        reject()
    ai_url = settings.database_url.get_secret_value()
    peer = packet["peer_connection"]
    peer_url = URL.create("mysql+asyncmy", username=peer["username"], password=peer["password"],
                          host=peer["host"], port=peer["port"], database=peer["database"])
    ai_bound, peer_bound = bounds(module, packet, "ai"), bounds(module, packet, "peer")
    if ai_bound.identity_hash == peer_bound.identity_hash:
        reject()
    keys, key_baseline = protection(module, packet, settings)
    engines, sessions = [], []
    output = None
    try:
        for url in (ai_url, peer_url):
            engines.append(create_async_engine(url, hide_parameters=True, echo=False,
                            isolation_level="REPEATABLE READ", pool_size=1, max_overflow=0))
        sessions = await open_snapshots(engines)
        qualification = await module.verify(*sessions, ai_bounds=ai_bound, peer_bounds=peer_bound,
            approved_ai_bounds_sha256=packet["ai_bounds_sha256"],
            approved_peer_bounds_sha256=packet["peer_bounds_sha256"],
            approved_original_sections=packet["original_sections"], protection_keys=keys)
        # This object came only from the actual verifier call in this process.
        # It is never reconstructed from stdin, a report or a success flag.
        originals = []
        ai, peer_rows = qualification._scans[0].rows, qualification._scans[1].rows
        for original in qualification._originals:
            session = module._session(module.one(module.match(ai["interpretation_sessions"], id=original.session_id), "original_session_missing_or_ambiguous"))
            result = await module._typed_execution(sessions[0], ai, original, session)
            terminal_event = module._legacy_event_chain(ai, peer_rows, original, session)[-1]
            sources = []
            for name in original.source_tables:
                row = module.one(module.match(peer_rows[name], command_id=original.command_id), "original_source_missing")
                payload = row["payload"] if name == "ai_bridge_commands" else row["source_payload"]
                sources.append({"table": name, "payload_bytes_sha256": digest(payload)})
            originals.append({"command_id": original.command_id, "request_id": original.request_id,
                "session_id": original.session_id, "organization_id": original.start["actor"]["org_id"],
                "subject_id": original.start["actor"]["subject_id"], "resource_id": original.request_id,
                "testee_id": original.start["testee_id"], "active_run_id": session.active_run_id,
                "terminal_event_id": module.text(terminal_event["event_id"]),
                "writer_sha256": original.writer_sha256, "original_request_sha256": original.original_request_sha256,
                "attempts": original.attempts, "handoff_sha256": original.handoff_sha256,
                "status": str(session.status), "execution_result": result, "sources": sources})
        facts = [{"side": scan.bounds.side, "identity_hash": scan.bounds.identity_hash,
                  "head": scan.bounds.head, "catalog_sha256": scan.bounds.catalog_sha256,
                  "sections_sha256": digest(canonical(scan.sections))} for scan in qualification._scans]
        peer_facts = peer_row_facts(module, qualification._scans[1])
        await close_sessions(sessions)
        sessions = []
        # Old transactions truly ended before either new transaction begins.
        sessions = await open_snapshots(engines)
        await module.recheck(*sessions, qualification, protection_keys=keys)
        _, fresh_key_baseline = protection(module, packet, settings)
        if fresh_key_baseline != key_baseline:
            reject()
        output = {"protocol": "qs-ai-actual-execution-facts/v2", "source_sha": packet["source_sha"],
                  "operation_id": packet["operation_id"], "run_id": packet["run_id"],
                  "runtime_source_sha": packet["runtime_source_sha"], "runtime_binding_sha256": packet["runtime_binding_sha256"], "image_id": packet["image_id"],
                  "container_id": packet["container_id"], "ai_bounds_sha256": packet["ai_bounds_sha256"],
                  "peer_bounds_sha256": packet["peer_bounds_sha256"], "snapshots": facts,
                  "original_sections": packet["original_sections"], "originals": originals,
                  "peer_rows": peer_facts}
    finally:
        cleanup_failed = False
        try:
            await close_sessions(sessions)
        except Exception:
            cleanup_failed = True
        for engine in engines:
            try:
                await engine.dispose()
            except Exception:
                cleanup_failed = True
        if cleanup_failed:
            reject()
    if output is None:
        reject()
    return output


# This second protocol is discovery only. It never treats its freshly observed
# bounds hash as an independent approval, and never calls verify or _scan.
DISCOVERY_FIELDS = frozenset("protocol source_sha operation_id run_id runtime_source_sha runtime_binding_sha256 runtime_messaging image_id container_id original_sections peer_connection expected_ai expected_peer modules".split())


def discovery_input_packet(raw):
    if not raw or len(raw) > INPUT_LIMIT:
        reject()
    value = decode(raw)
    if not isinstance(value, dict) or set(value) != DISCOVERY_FIELDS or value["protocol"] != "qs-ai-readonly-bounds-discovery-input/v1":
        reject()
    for key in ("source_sha", "runtime_source_sha"):
        pattern(value[key], r"[0-9a-f]{40}")
    pattern(value["operation_id"], r"[0-9]{1,20}-[0-9]{1,4}")
    pattern(value["run_id"], r"[1-9][0-9]{0,19}")
    pattern(value["image_id"], r"sha256:[0-9a-f]{64}")
    pattern(value["container_id"], r"[0-9a-f]{64}")
    pattern(value["runtime_binding_sha256"], r"[0-9a-f]{64}")
    if value["runtime_messaging"] is not None:
        exact(value["runtime_messaging"], "enabled nsqd signing_key_file decrypt_key_files qs_signer_files qs_recipient_key_file max_in_flight")
    exact(value["original_sections"], "ai_bridge_commands ai_messaging_legacy_commands")
    for section in value["original_sections"].values():
        exact(section, "rows source_bytes source_sha256")
        if any(type(section[k]) is not int or section[k] < 0 for k in ("rows", "source_bytes")):
            reject()
        pattern(section["source_sha256"], r"[0-9a-f]{64}")
    if sum(s["rows"] for s in value["original_sections"].values()) > ORIGINAL_LIMIT:
        reject()
    exact(value["peer_connection"], "host port database username password")
    for key in ("host", "database", "username", "password"):
        item = value["peer_connection"][key]
        if not isinstance(item, str) or not item or len(item.encode()) > 4096 or any(c in item for c in ("\x00", "\n", "\r")):
            reject()
    port = value["peer_connection"]["port"]
    if type(port) is not int or not 1 <= port <= 65535:
        reject()
    for side in ("ai", "peer"):
        expected = value["expected_" + side]
        exact(expected, "identity_hash head")
        if side == "ai" and expected == {"identity_hash": "", "head": ""}:
            continue  # Unknown prior AI binding stays diagnostic, never approved.
        pattern(expected["identity_hash"], r"[0-9a-f]{64}")
        pattern(expected["head"], r"(?:0038_messaging_observations|0040_module_table_names)" if side == "ai" else r"[1-9][0-9]{0,8}")
    if not isinstance(value["modules"], dict) or set(value["modules"]) != set(MODULES):
        reject()
    return value


async def discovery_binding(session, module, side, packet):
    # Bootstrap reads are fixed and bounded; discover_full_bounds then checks
    # the true active READ ONLY / REPEATABLE READ transaction in P_S as well.
    scanner = module._scanner(side, module.AI_HEAD if side == "ai" else packet["expected_peer"]["head"])
    reader = scanner._Borrowed(session)
    rows = await reader.query("SELECT CAST(@@server_uuid AS BINARY),CAST(DATABASE() AS BINARY),VERSION(),@@transaction_isolation")
    if len(rows) != 1 or len(rows[0]) != 4 or any(not isinstance(v, bytes) or not v for v in rows[0][:2]) or not isinstance(rows[0][2], str) or not rows[0][2].startswith("8.") or rows[0][3] != "REPEATABLE-READ":
        reject()
    identity = scanner._identity(rows[0][0], rows[0][1])
    state = await reader.query("SELECT version_num FROM alembic_version ORDER BY version_num") if side == "ai" else await reader.query("SELECT version,dirty FROM schema_migrations")
    if side == "ai":
        if len(state) != 1 or len(state[0]) != 1 or state[0][0] not in (module.AI_HEAD, "0040_module_table_names"):
            reject()
        head = state[0][0]
        source = scanner._layout().SOURCE_SHA if head == "0040_module_table_names" else packet["runtime_source_sha"]
    else:
        if len(state) != 1 or len(state[0]) != 2 or type(state[0][0]) is not int or state[0][0] <= 0 or state[0][1] != 0:
            reject()
        head, source = str(state[0][0]), packet["source_sha"]
    expected = packet["expected_" + side]
    matched = expected != {"identity_hash": "", "head": ""}
    if matched and expected != {"identity_hash": identity, "head": head}:
        reject()
    bound = await module.discover_full_bounds(session, side=side, source_sha=source, identity_hash=identity, head=head)
    return bound, matched


async def discovery_original_sections(module, reader, bound, expected):
    sections = {}
    for table in ("ai_bridge_commands", "ai_messaging_legacy_commands"):
        approved = bound.tables[table]
        before = await reader._discovery_scanner._schema(reader, table)
        if any(before[k] != approved[k] for k in before):
            reject()
        first, _ = await reader._discovery_scanner._pass(reader, table, approved, retain=False)
        second, _ = await reader._discovery_scanner._pass(reader, table, approved, retain=False)
        section = {k: first[k] for k in ("rows", "source_bytes", "source_sha256")}
        if first != second or section != expected[table]:
            reject()
        after = await reader._discovery_scanner._schema(reader, table)
        if after != before:
            reject()
        sections[table] = section
    return sections


async def discovery_fresh_checks(session, module, bound, original_tx, expected_sections):
    # No PrivateObservation is forged for this diagnostic. All statements use
    # the frozen scanner's closed table catalog and exact typed PK protocol.
    if original_tx.is_active:
        reject()
    scanner = module._scanner(bound.side, bound.head)
    reader = scanner._Borrowed(session)
    if reader.tx is original_tx:
        reject()
    reader._discovery_scanner = scanner
    await scanner._binding(reader, bound.identity_hash, bound.source_sha)
    if await module._catalog(reader, bound.side) != bound.catalog_sha256:
        reject()
    after_upper = {}
    for table in reader.specs:
        schema = await scanner._schema(reader, table)
        old = bound.tables[table]
        if any(schema[k] != old[k] for k in schema):
            reject()
        params, clauses = {}, ""
        if old["upper"] is not None:
            keys = reader.specs[table][1].split()
            upper = scanner._upper_values(old, keys)
            params = {"u" + str(i): v for i, v in enumerate(upper)}
            clauses = " WHERE (" + scanner._pk_sql(table, reader.specs) + ") > (" + ",".join(":" + k for k in params) + ")"
        rows = await reader.query("SELECT 1 FROM `" + table + "`" + clauses + " LIMIT 1", params)
        if rows not in ([], [(1,)]):
            reject()
        after_upper[table] = bool(rows)
    sections = None
    if bound.side == "peer":
        sections = await discovery_original_sections(module, reader, bound, expected_sections)
    # Metadata must not change while the preceding checks/scans are running.
    for table in reader.specs:
        schema = await scanner._schema(reader, table)
        if any(schema[k] != bound.tables[table][k] for k in schema):
            reject()
    if await module._catalog(reader, bound.side) != bound.catalog_sha256:
        reject()
    await scanner._binding(reader, bound.identity_hash, bound.source_sha)
    return after_upper, sections


async def execute_discovery(packet, module):
    from sqlalchemy import URL
    from sqlalchemy.ext.asyncio import create_async_engine
    from qs_ai.config import Settings
    if os.environ.get("QS_AI_RELEASE_SHA") != packet["runtime_source_sha"] or os.environ.get("QS_AI_ENVIRONMENT") != "production":
        reject()
    settings = Settings()
    if settings.database_url is None:
        reject()
    actual_messaging = settings.messaging.model_dump(mode="json") if settings.messaging.enabled else None
    if actual_messaging != packet["runtime_messaging"]:
        reject()
    peer = packet["peer_connection"]
    urls = (settings.database_url.get_secret_value(), URL.create("mysql+asyncmy", username=peer["username"], password=peer["password"], host=peer["host"], port=peer["port"], database=peer["database"]))
    engines, sessions, output = [], [], None
    try:
        for url in urls:
            engines.append(create_async_engine(url, hide_parameters=True, echo=False, isolation_level="REPEATABLE READ", pool_size=1, max_overflow=0))
        sessions = await open_snapshots(engines)
        observed = [await discovery_binding(session, module, side, packet) for session, side in zip(sessions, ("ai", "peer"))]
        bounds_set = [value[0] for value in observed]
        if bounds_set[0].identity_hash == bounds_set[1].identity_hash:
            reject()
        scanner = module._scanner("peer", bounds_set[1].head)
        reader = scanner._Borrowed(sessions[1])
        reader._discovery_scanner = scanner
        await scanner._binding(reader, bounds_set[1].identity_hash, bounds_set[1].source_sha)
        sections = await discovery_original_sections(module, reader, bounds_set[1], packet["original_sections"])
        # Seal both first metadata views again after the complete source reads.
        for session, bound in zip(sessions, bounds_set):
            scan = module._scanner(bound.side, bound.head)
            view = scan._Borrowed(session)
            if await module._catalog(view, bound.side) != bound.catalog_sha256:
                reject()
            for table in view.specs:
                schema = await scan._schema(view, table)
                if any(schema[k] != bound.tables[table][k] for k in schema):
                    reject()
            await scan._binding(view, bound.identity_hash, bound.source_sha)
        original_txs = [session.get_transaction() for session in sessions]
        await close_sessions(sessions)
        sessions = []
        if any(tx is None or tx.is_active for tx in original_txs):
            reject()
        sessions = await open_snapshots(engines)
        fresh = [await discovery_fresh_checks(session, module, bound, tx, sections) for session, bound, tx in zip(sessions, bounds_set, original_txs)]
        packets = {bound.side: {"bytes": base64.b64encode(bound.private_bytes()).decode(), "sha256": bound.digest(), "objects": len(bound.tables), "prior_binding_matched": observed[i][1], "after_upper": fresh[i][0]} for i, bound in enumerate(bounds_set)}
        if fresh[1][1] != sections or len(bounds_set[1].tables) != 14:
            reject()
        logical_count = len(module.AI_SPECS)
        if bounds_set[0].head == "0040_module_table_names" and (len(bounds_set[0].tables) != 43 or logical_count != 53):
            reject()
        output = {"protocol": "qs-ai-readonly-bounds-discovery-facts/v1", **{k: packet[k] for k in ("source_sha", "operation_id", "run_id", "runtime_source_sha", "runtime_binding_sha256", "image_id", "container_id")}, "bounds": packets, "original_sections": sections, "ai_logical_objects": logical_count, "independent_epochs": 2, "scope": "diagnostic-unapproved-bounds-only", "independent_approval": False, "business_closure": False, "fence": False, "cas_authority": False, "drop_ready": False}
    finally:
        failed = False
        try:
            await close_sessions(sessions)
        except Exception:
            failed = True
        for engine in engines:
            try:
                await engine.dispose()
            except Exception:
                failed = True
        if failed:
            reject()
    if output is None:
        reject()
    return output


def main():
    os.umask(0o077)
    raw = sys.stdin.buffer.read(INPUT_LIMIT + 1)
    output = None
    # Dependencies may emit incidental text. It is discarded in memory and is
    # never copied to stderr or used as evidence. Only one protocol object exits.
    original_stdout, original_stderr = sys.stdout, sys.stderr
    sink = DiscardText()
    try:
        sys.stdout = sys.stderr = sink
        discovery = isinstance(decode(raw), dict) and decode(raw).get("protocol") == "qs-ai-readonly-bounds-discovery-input/v1"
        packet = discovery_input_packet(raw) if discovery else input_packet(raw)
        with tempfile.TemporaryDirectory(prefix="qs-ai-readonly-execution-") as directory:
            os.chmod(directory, 0o700)
            module = load_verifier(packet, Path(directory))
            async def bounded():
                async with asyncio.timeout(TOTAL_SECONDS):
                    return await execute_discovery(packet, module) if discovery else await execute(packet, module)
            output = asyncio.run(bounded())
        encoded = canonical(output)
        if len(encoded) > OUTPUT_LIMIT:
            reject()
    except Exception:
        encoded = canonical({"protocol": "qs-ai-actual-execution-failed/v1", "category": "execution_rejected"})
        output = None
    finally:
        sys.stdout, sys.stderr = original_stdout, original_stderr
    original_stdout.buffer.write(encoded + b"\n")
    original_stdout.buffer.flush()
    return 0 if output is not None else 1


if __name__ == "__main__":
    raise SystemExit(main())
