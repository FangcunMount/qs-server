import importlib.util
import json
from pathlib import Path
import unittest

SPEC = importlib.util.spec_from_file_location('receipt_transport', Path(__file__).with_name('receipt-transport.py'))
transport = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(transport)
SCHEMA = {'complete': 'bool', 'optional': 'nullable_bool', 'count': 'uint', 'source_sha': 'sha40',
          'hash': 'hash64', 'empty_hash': 'hash64_or_empty', 'run_id': 'run_id',
          'status': frozenset({'ok', 'blocked'}), 'stages': [{'complete': 'bool', 'count': 'uint'}]}


def sample():
    return {'complete': True, 'optional': None, 'count': 0, 'source_sha': 'a' * 40, 'hash': 'b' * 64,
            'empty_hash': '', 'run_id': '123-1', 'status': 'ok', 'stages': [{'complete': False, 'count': 22}]}


def frame(raw, base=0xE000):
    return chr(base + 0x100) + ''.join(chr(base + byte) for byte in raw) + chr(base + 0x101)


class ReceiptTransportTests(unittest.TestCase):
    def test_roundtrip_and_short_ascii_secret_masking(self):
        value = sample()
        encoded = transport.encode_armored_receipt(value, schema=SCHEMA, secrets=('0', 'a'))
        self.assertEqual(ord(encoded[0]), 0xE100)
        masked = encoded.replace('0', '***').replace('a', '***')
        decoded = transport.decode_armored_receipt('fixed timestamp\n' + masked + '\nfixed tail')
        self.assertEqual(json.loads(decoded), value)

    def test_alternate_blocks_avoid_full_frame_credential_collisions(self):
        value = sample()
        for secrets, expected in [(('\ue100',), 0xE300), (('\ue100', '\ue300'), 0xE500)]:
            with self.subTest(expected=expected):
                encoded = transport.encode_armored_receipt(value, schema=SCHEMA, secrets=secrets)
                self.assertEqual(ord(encoded[0]), expected)
                self.assertTrue(all(secret not in encoded for secret in secrets))
                self.assertEqual(json.loads(transport.decode_armored_receipt(encoded)), value)
        with self.assertRaisesRegex(transport.ReceiptTransportError, '^receipt_transport_credential_collision$'):
            transport.encode_armored_receipt(value, schema=SCHEMA, secrets=('\ue100', '\ue300', '\ue500'))

    def test_missing_duplicate_partial_and_wrong_markers_rejected(self):
        encoded = transport.encode_armored_receipt(sample(), schema=SCHEMA)
        for text in ['', json.dumps(sample()), encoded + encoded, encoded[:-1], encoded[1:],
                     '\ue101' + encoded[1:-1] + '\ue100', '\ue100' + encoded[1:-1] + '\ue301']:
            with self.subTest(text_length=len(text)):
                with self.assertRaises(transport.ReceiptTransportError):
                    transport.decode_armored_receipt(text)

    def test_mixed_alphabet_masked_body_and_outside_alphabet_rejected(self):
        encoded = transport.encode_armored_receipt(sample(), schema=SCHEMA)
        for text in [encoded[:2] + '\ue200' + encoded[3:], encoded[:2] + '***' + encoded[3:], '\ue000' + encoded]:
            with self.assertRaises(transport.ReceiptTransportError):
                transport.decode_armored_receipt(text)

    def test_body_and_log_limits_rejected(self):
        with self.assertRaises(transport.ReceiptTransportError):
            transport.decode_armored_receipt(frame(b' ' * (transport.MAX_RECEIPT_BYTES + 1)))
        with self.assertRaises(transport.ReceiptTransportError):
            transport.decode_armored_receipt('x' * (transport.MAX_LOG_CHARACTERS + 1))
        with self.assertRaises(transport.ReceiptTransportError):
            transport.encode_armored_receipt({'stages': [sample()] * 33}, schema={'stages': [SCHEMA]})
        # Each object is allowed, but aggregate encoding exceeds the hard body cap.
        large = {'hash_' + str(i): 'a' * 64 for i in range(40)}
        nested = {'stages': [large] * 32}
        large_schema = {'hash_' + str(i): 'hash64' for i in range(40)}
        with self.assertRaisesRegex(transport.ReceiptTransportError, '^receipt_transport_size$'):
            transport.encode_armored_receipt(nested, schema={'stages': [large_schema]})

    def test_invalid_utf8_json_duplicate_keys_and_nonobject_rejected(self):
        for raw in [b'\xc3', b'not json', b'[]', b'{"complete":true,"complete":false}',
                    b'{"stages":[{"count":1,"count":2}]}', b'{"count":NaN}']:
            with self.subTest(raw_length=len(raw)):
                with self.assertRaises(transport.ReceiptTransportError):
                    transport.decode_armored_receipt(frame(raw))

    def test_unknown_raw_and_credentials_fields_cannot_be_encoded(self):
        for key in ['raw_grants', 'raw_logs', 'password', 'username', 'host', 'source_server_uuid', 'roles']:
            value = sample(); value[key] = 'synthetic_NEVER_ENCODE'
            with self.subTest(key=key):
                with self.assertRaises(transport.ReceiptTransportError):
                    transport.encode_armored_receipt(value, schema=SCHEMA)
                with self.assertRaises(transport.ReceiptTransportError):
                    transport.encode_armored_receipt({key: 'ok'}, schema={key: frozenset({'ok'})})
        with self.assertRaises(transport.ReceiptTransportError):
            transport.encode_armored_receipt({'stages': [{'password': 'ok'}]}, schema={'stages': [{'password': frozenset({'ok'})}]})

    def test_scalar_types_tokens_hashes_and_counts_are_strict(self):
        cases = [('complete', 1), ('optional', 0), ('count', True), ('count', -1), ('count', 2 ** 64),
                 ('source_sha', '***'), ('hash', 'bad'), ('run_id', 'anything'), ('status', 'raw secret text'),
                 ('stages', [sample()])]
        for key, item in cases:
            with self.subTest(key=key, value_type=type(item).__name__):
                value = sample(); value[key] = item
                with self.assertRaises(transport.ReceiptTransportError):
                    transport.encode_armored_receipt(value, schema=SCHEMA)
        self.assertEqual(json.loads(transport.decode_armored_receipt(transport.encode_armored_receipt({'count': 2 ** 64 - 1}, schema=SCHEMA))), {'count': 2 ** 64 - 1})

    def test_optional_schema_keys_and_nullable_hash_accepted(self):
        value = {'complete': False, 'status': 'blocked'}
        self.assertEqual(json.loads(transport.decode_armored_receipt(transport.encode_armored_receipt(value, schema=SCHEMA))), value)
        encoded = transport.encode_armored_receipt({'hash': None}, schema={'hash': 'nullable_hash64'})
        self.assertEqual(json.loads(transport.decode_armored_receipt(encoded)), {'hash': None})


if __name__ == '__main__':
    unittest.main()
