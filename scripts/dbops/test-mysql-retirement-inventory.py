#!/usr/bin/env python3
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

HERE = Path(__file__).parent
spec = importlib.util.spec_from_file_location('inventory_fixtures', HERE / 'mysql-retirement-inventory.py')
inventory = importlib.util.module_from_spec(spec)
spec.loader.exec_module(inventory)
SHA, RUN, HASH = 'a' * 40, '12345-1', 'b' * 64
SECRET = 'private-inventory-marker'


def environment():
    return dict(MYSQL_HOST='selected-source', MYSQL_PORT='3306', MYSQL_USERNAME='private-user', MYSQL_PASSWORD=SECRET,
                MYSQL_DATABASE='qs', PROFILE_SOURCE_SHA=SHA, PROFILE_RUN_ID=RUN, PROFILE_EXPECTED_TARGET_HASH=HASH)


def receipt():
    return dict(format_version=1, source_sha=SHA, run_id=RUN, expected_target_hash=HASH, source_target_hash=HASH,
                retirement_inventory_only=True, freeze_verified=False, complete=True, error_category='none', failure_stage='none',
                query_count=15, control_statement_count=3, transaction_released=True, connection_released=True,
                database_released=True, pool_closed_verified=True,
                measurements=dict(mysql_version=dict(major=8, minor=0, patch=36), migration_version=100, migration_dirty=False,
                                  nine_innodb_objects_verified=True, consistent_snapshot=True,
                                  counts={family: {field: 0 for field in fields} for family, fields in inventory.COUNT_FIELDS.items()}))


class InventoryTests(unittest.TestCase):
    def validate(self, value, code=0):
        return inventory.safe_receipt(inventory.runtime_module(), json.dumps(value), code, SHA, RUN, HASH)

    def test_complete_is_observation_even_with_nonzero_findings(self):
        value = receipt()
        c = value['measurements']['counts']
        c['testee_tags_partition'].update(total=2, non_null=2, non_empty_array=1, other=1)
        c['operators_role_duplication'].update(total=1, different_canonical_json_bytes=1, both_arrays=1)
        c['five_table_counts'].update(testee_store_migration_manifests=1, testee_store_migration_items=1, operator_retirement_manifests=1)
        c['testee_manifest_states']['unknown'] = 1
        c['testee_item_dispositions']['unknown'] = 1
        c['operator_manifest_stages']['unknown'] = 1
        c['historical_reference_gaps']['items_without_manifest'] = 1
        c['ai_backfill_scope'].update(total=2, actual_backfill_pending=1, invalid_assessment_array_shape=1,
                                     nonempty_assessment_arrays=1, invalid_actor_field_shape=1, indexed_identity_mismatch=1)
        c['ai_expected_relation_gaps'].update(payload_relation_entries=2, invalid_expected_id_entries=1, valid_expected_entries_without_relation=1)
        c['ai_actual_relation_gaps'].update(actual_relation_rows=2, orphan_relations=1, zero_assessment_relations=1, relation_not_in_request_payload=1)
        safe = self.validate(value)
        self.assertTrue(safe['complete'])
        self.assertFalse(safe['freeze_verified'])
        self.assertNotIn('eligible', json.dumps(safe))
        self.assertEqual(safe['measurements']['counts'], c)

    def test_only_exact_query_control_and_release_proofs_can_complete(self):
        for key, invalid in (('query_count', 14), ('query_count', 16), ('control_statement_count', 2),
                             ('control_statement_count', 4), ('transaction_released', False), ('connection_released', None),
                             ('database_released', False), ('pool_closed_verified', False), ('freeze_verified', True),
                             ('retirement_inventory_only', False), ('format_version', 2), ('source_sha', 'c' * 40)):
            with self.subTest(key=key, invalid=invalid), self.assertRaises(Exception):
                self.validate(dict(receipt(), **{key: invalid}))
        for key in ('transaction_released', 'query_count', 'freeze_verified'):
            value = receipt()
            del value[key]
            with self.subTest(missing=key), self.assertRaises(Exception):
                self.validate(value)

    def test_measurements_have_closed_types_families_and_state_labels(self):
        for path, invalid in ((('migration_version',), 101), (('migration_dirty',), True),
                              (('nine_innodb_objects_verified',), False), (('consistent_snapshot',), False),
                              (('mysql_version', 'major'), 7), (('mysql_version', 'patch'), 1000),
                              (('counts', 'ai_backfill_scope', 'total'), True),
                              (('counts', 'ai_backfill_scope', 'total'), -1),
                              (('counts', 'ai_backfill_scope', 'total'), 1.0),
                              (('counts', 'ai_backfill_scope', 'total'), 2 ** 64)):
            value = receipt()
            target = value['measurements']
            for item in path[:-1]:
                target = target[item]
            target[path[-1]] = invalid
            with self.subTest(path=path, invalid=invalid), self.assertRaises(Exception):
                self.validate(value)
        for path in (('counts', 'ai_backfill_scope'), ('mysql_version',), ('counts', 'testee_manifest_states')):
            value = receipt()
            target = value['measurements']
            for item in path:
                target = target[item]
            target['unreviewed'] = 0
            with self.subTest(path=path), self.assertRaises(Exception):
                self.validate(value)
        value = receipt()
        del value['measurements']['counts']['operators_role_duplication']
        with self.assertRaises(Exception):
            self.validate(value)

    def test_all_partition_and_cross_family_conservation_is_required(self):
        for family, field in (('testee_tags_partition', 'non_null'), ('operators_role_duplication', 'both_arrays'),
                              ('five_table_counts', 'testee_store_migration_items'), ('operator_manifest_stages', 'unknown'),
                              ('historical_reference_gaps', 'archives_without_manifest'), ('ai_backfill_scope', 'indexed_identity_mismatch'),
                              ('ai_expected_relation_gaps', 'valid_expected_entries_without_relation'),
                              ('ai_actual_relation_gaps', 'relation_not_in_request_payload')):
            value = receipt()
            value['measurements']['counts'][family][field] = 1
            with self.subTest(family=family, field=field), self.assertRaises(Exception):
                self.validate(value)

    def test_every_fixed_failure_discards_all_measurements(self):
        env = environment()
        for category in inventory.ERRORS - {'none', 'query_mysql_error'}:
            value = inventory.stage_failure(env, category)
            safe = self.validate(value, 1)
            self.assertFalse(safe['complete'])
            self.assertNotIn('measurements', safe)
            value['measurements'] = receipt()['measurements']
            with self.subTest(category=category), self.assertRaises(Exception):
                self.validate(value, 1)
        for invalid in ('unreviewed', SECRET):
            value = inventory.stage_failure(env, invalid)
            self.assertEqual(value['error_category'], 'profile_transport_or_receipt_failed')

    def test_typed_mysql_numbers_are_bounded_and_cannot_sign_success(self):
        for category, number in (('query_mysql_error', 0), ('query_mysql_error', 65535), ('query_mysql_error', 1064),
                                 ('query_server_timeout', 3024), ('query_server_interrupted', 1317), ('query_permission_denied', 1142),
                                 ('metadata_release_failed', 1064), ('transaction_release_failed', 1064)):
            value = inventory.stage_failure(environment(), category)
            value.update(query_count=7, query_mysql_error_number=number)
            self.assertEqual(self.validate(value, 1)['query_mysql_error_number'], number)
        for number in (None, True, -1, 65536, '1064', 1064.0, 3024):
            value = inventory.stage_failure(environment(), 'query_mysql_error')
            value.update(query_count=7, query_mysql_error_number=number)
            with self.subTest(number=number), self.assertRaises(Exception):
                self.validate(value, 1)
        for category in ('query_failed', 'query_invalid_connection', 'query_network_timeout', 'none'):
            value = inventory.stage_failure(environment(), category)
            value.update(query_count=7, query_mysql_error_number=1064)
            with self.subTest(category=category), self.assertRaises(Exception):
                self.validate(value, 1)
        value = inventory.stage_failure(environment(), 'query_mysql_error')
        with self.assertRaises(Exception):
            self.validate(value, 1)

    def test_wrong_mode_or_raw_identifying_fields_are_rejected(self):
        for key in ('readiness_only', 'diagnostic_only', 'account', 'message', 'uuid', 'sql', 'body', 'request_id'):
            with self.subTest(key=key), self.assertRaises(Exception):
                self.validate(dict(receipt(), **{key: SECRET}))
        value = receipt()
        raw = json.dumps(value).replace('"query_count": 15', '"query_count": 15, "query_count": 15')
        with self.assertRaises(Exception):
            inventory.safe_receipt(inventory.runtime_module(), raw, 0, SHA, RUN, HASH)

    def test_fixed_native_argument_and_private_environment_only(self):
        with tempfile.TemporaryDirectory() as directory:
            binary = Path(directory) / 'reader'
            binary.write_bytes(b'owned')
            def capture(args, **kwargs):
                self.assertEqual(args, [str(binary), '--retirement-inventory'])
                self.assertEqual(set(kwargs['env']), set(inventory.PROFILE_KEYS))
                self.assertNotIn('LD_PRELOAD', kwargs['env'])
                return 0, json.dumps(receipt()).encode()
            with patch.object(inventory, 'capture_private', side_effect=capture):
                self.assertTrue(inventory.execute_native(str(binary), dict(environment(), LD_PRELOAD=SECRET), inventory.runtime_module())[1]['complete'])
            with patch.object(inventory, 'capture_private') as capture, self.assertRaises(Exception):
                inventory.execute_native(str(binary), dict(environment(), MYSQL_DATABASE='other'), inventory.runtime_module())
            capture.assert_not_called()


if __name__ == '__main__':
    unittest.main()
