#!/usr/bin/env python3
"""Armor only schema-validated, non-secret receipts; this is not encryption."""
import json
import re

MAX_RECEIPT_BYTES = 64 * 1024
MAX_LOG_CHARACTERS = 4 * 1024 * 1024
ALPHABET_BASES = (0xE000, 0xE200, 0xE400)
MAX_DEPTH = 4
MAX_LIST_ITEMS = 32
FIELD = re.compile(r"^[a-z][a-z0-9_]{0,63}$")
TOKEN = re.compile(r"^[a-z0-9_-]{0,96}$")
SHA40 = re.compile(r"^[0-9a-f]{40}$")
HASH64 = re.compile(r"^[0-9a-f]{64}$")
RUN_ID = re.compile(r"^[0-9]{1,20}-[0-9]{1,4}$")
FORBIDDEN_FIELDS = frozenset({"credential", "credentials", "password", "username", "user", "host", "uri", "dsn", "connection", "uuid", "source_server_uuid", "restore_server_uuid", "roles", "grants", "log", "logs"})


class ReceiptTransportError(ValueError):
    """A fixed-category transport failure, never containing a rejected value."""


def _fail(category):
    raise ReceiptTransportError(category)


def _validate(value, schema, depth=0):
    if depth > MAX_DEPTH:
        _fail("receipt_transport_depth")
    if isinstance(schema, dict):
        if type(value) is not dict or set(value) - set(schema):
            _fail("receipt_transport_fields")
        for key, item in value.items():
            if type(key) is not str or not FIELD.fullmatch(key) or key in FORBIDDEN_FIELDS or key.startswith("raw_"):
                _fail("receipt_transport_fields")
            _validate(item, schema[key], depth + 1)
        return
    if isinstance(schema, list):
        if len(schema) != 1 or type(value) is not list or len(value) > MAX_LIST_ITEMS:
            _fail("receipt_transport_list")
        for item in value:
            _validate(item, schema[0], depth + 1)
        return
    if isinstance(schema, frozenset):
        if type(value) is not str or not TOKEN.fullmatch(value) or value not in schema:
            _fail("receipt_transport_token")
        return
    if schema == "bool":
        valid = type(value) is bool
    elif schema == "nullable_bool":
        valid = value is None or type(value) is bool
    elif schema == "uint":
        valid = type(value) is int and 0 <= value <= 2 ** 64 - 1
    elif schema == "sha40":
        valid = type(value) is str and SHA40.fullmatch(value)
    elif schema == "hash64":
        valid = type(value) is str and HASH64.fullmatch(value)
    elif schema == "nullable_hash64":
        valid = value is None or (type(value) is str and HASH64.fullmatch(value))
    elif schema == "hash64_or_empty":
        valid = type(value) is str and (value == "" or HASH64.fullmatch(value))
    elif schema == "run_id":
        valid = type(value) is str and RUN_ID.fullmatch(value)
    else:
        _fail("receipt_transport_schema")
    if not valid:
        _fail("receipt_transport_type")


def encode_armored_receipt(value, *, schema, secrets=()):
    """Encode a previously protocol-validated dict under an explicit schema.

    The schema is a field whitelist, not a required-field list. Fixed tokens
    use frozensets, nested dicts and single-element lists use nested schemas.
    Binding, required fields and operation semantics remain caller-owned.
    """
    if type(value) is not dict or not value or not isinstance(schema, dict):
        _fail("receipt_transport_object")
    _validate(value, schema)
    if not isinstance(secrets, (tuple, list)) or len(secrets) > 32 or any(type(s) is not str or len(s) > 65536 for s in secrets):
        _fail("receipt_transport_credentials")
    raw = json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True, allow_nan=False).encode("utf-8")
    if not 0 < len(raw) <= MAX_RECEIPT_BYTES:
        _fail("receipt_transport_size")
    for base in ALPHABET_BASES:
        frame = chr(base + 0x100) + "".join(chr(base + byte) for byte in raw) + chr(base + 0x101)
        if not any(secret and secret in frame for secret in secrets):
            return frame
    _fail("receipt_transport_credential_collision")


def _unique_object(pairs):
    value = {}
    for key, item in pairs:
        if key in value:
            _fail("receipt_transport_duplicate_key")
        value[key] = item
    return value


def _reject_constant(_value):
    _fail("receipt_transport_json")


def decode_armored_receipt(text):
    """Return UTF-8 JSON text from exactly one valid frame in bounded log text.

    No legacy-JSON or damaged-frame repair is attempted. Callers must still
    apply their original safe-receipt validator and source/run/target binding.
    """
    if type(text) is not str or len(text) > MAX_LOG_CHARACTERS:
        _fail("receipt_transport_log_size")
    marker_base = {base + offset: base for base in ALPHABET_BASES for offset in (0x100, 0x101)}
    markers = []
    for index, char in enumerate(text):
        if ord(char) in marker_base:
            markers.append((index, ord(char)))
            if len(markers) > 2:
                _fail("receipt_transport_frame_count")
    if len(markers) != 2:
        _fail("receipt_transport_frame_count")
    (start, first), (end, last) = markers
    base = marker_base[first]
    if first != base + 0x100 or last != base + 0x101:
        _fail("receipt_transport_frame_markers")
    if not 0 < end - start - 1 <= MAX_RECEIPT_BYTES:
        _fail("receipt_transport_size")
    for index, char in enumerate(text):
        code = ord(char)
        if start < index < end:
            if not base <= code <= base + 0xFF:
                _fail("receipt_transport_alphabet")
        elif any(candidate <= code <= candidate + 0xFF for candidate in ALPHABET_BASES):
            _fail("receipt_transport_outside_frame")
    raw = bytes(ord(char) - base for char in text[start + 1:end])
    try:
        decoded = raw.decode("utf-8", errors="strict")
        value = json.loads(decoded, object_pairs_hook=_unique_object, parse_constant=_reject_constant)
    except ReceiptTransportError:
        raise
    except (ValueError, UnicodeError, RecursionError):
        _fail("receipt_transport_json")
    if type(value) is not dict:
        _fail("receipt_transport_object")
    return decoded
