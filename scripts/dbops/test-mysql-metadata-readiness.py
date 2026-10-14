#!/usr/bin/env python3
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

HERE = Path(__file__).parent

def load(name, file):
    spec = importlib.util.spec_from_file_location(name, HERE / file)
    value = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(value)
    return value

readiness = load('readiness_test_module', 'mysql-metadata-readiness.py')
profile = readiness.profile
SHA, RUN, HASH = 'a' * 40, '12345-1', 'b' * 64
SECRET = 'private-readiness-marker'

def receipt():
    zero = dict(total=0, sql_null=0, non_null=0, json_null=0, empty_object=0, non_empty_object=0, other=0)
    return dict(format_version=1, source_sha=SHA, run_id=RUN, expected_target_hash=HASH, source_target_hash=HASH,
                diagnostic_only=True, readiness_only=True, freeze_verified=False, complete=True, error_category='none',
                failure_stage='none', query_count=18, connection_released=True, database_released=True, pool_closed_verified=True,
                measurements=dict(global_metadata_grant_rows_verified=True, migration_version=100, migration_dirty=False,
                                  performance_schema_enabled=True, event_scheduler='on',
                                  catalogs={key: dict(count=(3 if key == 'tables' else 2 if key in ('columns', 'checks') else 0), digest='c' * 64) for key in readiness.CATALOG_KEYS},
                                  enabled_events=0, threads=1, event_threads=0, transactions=1, modifying_transactions=1,
                                  metadata_locks=1, mdl_instrument_enabled=True, assessment_distribution=dict(zero), outcome_distribution=dict(zero)))

def environment():
    return dict(MYSQL_HOST='selected-source', MYSQL_PORT='3306', MYSQL_USERNAME='private-user', MYSQL_PASSWORD=SECRET,
                MYSQL_DATABASE='qs', PROFILE_SOURCE_SHA=SHA, PROFILE_RUN_ID=RUN, PROFILE_EXPECTED_TARGET_HASH=HASH)

class ReadinessTests(unittest.TestCase):
    def validate(self, value, code=0):
        return readiness.safe_receipt(readiness.runtime_module(), json.dumps(value), code, SHA, RUN, HASH)

    def test_positive_counts_are_observation_not_freeze(self):
        value = receipt()
        self.assertTrue(self.validate(value)['complete'])
        self.assertFalse(self.validate(value)['freeze_verified'])
        value['measurements']['catalogs']['events']['count'] = 1
        value['measurements']['enabled_events'] = 1
        self.assertTrue(self.validate(value)['complete'])

    def test_missing_fields_wrong_source_truncation_or_release_cannot_pass(self):
        failures = []
        for path, change in [(['measurements', 'performance_schema_enabled'], False), (['measurements', 'mdl_instrument_enabled'], False),
                             (['measurements', 'migration_version'], 101), (['measurements', 'migration_dirty'], True),
                             (['measurements', 'catalogs', 'tables', 'count'], 1001), (['connection_released'], False),
                             (['database_released'], None), (['pool_closed_verified'], False), (['source_sha'], 'd' * 40),
                             (['query_count'], 17), (['freeze_verified'], True), (['measurements', 'assessment_distribution', 'non_null'], 1)]:
            value = receipt()
            target = value
            for part in path[:-1]:
                target = target[part]
            target[path[-1]] = change
            failures.append(value)
        value = receipt()
        del value['measurements']['catalogs']['events']
        failures.append(value)
        value = receipt()
        del value['connection_released']
        failures.append(value)
        for value in failures:
            with self.subTest(value=value), self.assertRaises(Exception):
                self.validate(value)

    def test_native_mode_environment_and_argument_are_fixed(self):
        with tempfile.TemporaryDirectory() as directory:
            binary = Path(directory) / 'profile'
            binary.write_bytes(b'owned')
            env = dict(environment(), LD_PRELOAD=SECRET, UNRELATED_TOKEN=SECRET)
            def captured(args, **kwargs):
                self.assertEqual(args, [str(binary), '--metadata-readiness'])
                self.assertEqual(set(kwargs['env']), set(readiness.PROFILE_KEYS))
                self.assertNotIn('LD_PRELOAD', kwargs['env'])
                return 0, json.dumps(receipt()).encode()
            with patch.object(readiness, 'capture_private', side_effect=captured):
                self.assertTrue(readiness.execute_native(str(binary), env, readiness.runtime_module())[1]['complete'])
            env['MYSQL_DATABASE'] = 'unapproved'
            with patch.object(readiness, 'capture_private') as capture, self.assertRaises(Exception):
                readiness.execute_native(str(binary), env, readiness.runtime_module())
            capture.assert_not_called()

    def test_partial_query_or_release_failure_retains_no_measurements(self):
        for category in ('query_failed', 'query_server_timeout', 'query_server_interrupted', 'query_permission_denied', 'query_deadline_exceeded', 'query_canceled', 'metadata_truncated', 'connection_release_failed', 'database_release_failed'):
            value = readiness.stage_failure(environment(), category)
            value['failure_stage'] = 'transactions'
            self.assertFalse(self.validate(value, 1)['complete'])
            value['measurements'] = receipt()['measurements']
            with self.assertRaises(Exception):
                self.validate(value, 1)

    def test_new_query_categories_do_not_authorize_success_or_raw_error_fields(self):
        for category in ('query_server_timeout', 'query_server_interrupted', 'query_permission_denied', 'query_deadline_exceeded', 'query_canceled'):
            value = readiness.stage_failure(environment(), category)
            value.update(failure_stage='outcome_distribution', query_count=16,
                         connection_released=True, database_released=True, pool_closed_verified=True)
            safe = self.validate(value, 1)
            self.assertFalse(safe['complete'])
            self.assertNotIn('measurements', safe)
            frame = readiness.receipt_transport_module().encode_armored_receipt(safe, schema=readiness.RECEIPT_SCHEMA, secrets=(SECRET,))
            self.assertNotIn(SECRET, frame)
            for key in ('message', 'mysql_error_number', 'sqlstate', 'account'):
                raw = dict(value)
                raw[key] = SECRET
                with self.assertRaises(Exception):
                    self.validate(raw, 1)
            value['complete'] = True
            with self.assertRaises(Exception):
                self.validate(value, 0)

    def test_unknown_raw_fields_and_tokens_cannot_cross_armor(self):
        for key in ('grants', 'roles', 'username', 'sql_text', 'body', 'connection_id'):
            value = receipt()
            value[key] = SECRET
            with self.assertRaises(Exception):
                self.validate(value)
        value = readiness.stage_failure(environment(), SECRET)
        self.assertNotIn(SECRET, json.dumps(value))
        self.assertEqual(value['error_category'], 'profile_transport_or_receipt_failed')

if __name__ == '__main__':
    unittest.main()
