#!/usr/bin/env python3
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
    return dict(format_version=2, source_sha=SHA, run_id=RUN, expected_target_hash=HASH, source_target_hash=HASH,
                diagnostic_only=True, readiness_only=True, freeze_verified=False, complete=True, error_category='none',
                failure_stage='none', query_count=21, control_statement_count=3, transaction_released=True,
                connection_released=True, database_released=True, pool_closed_verified=True,
                measurements=dict(global_metadata_grant_rows_verified=True, migration_version=100, migration_dirty=False,
                                  performance_schema_enabled=True, event_scheduler='on',
                                  catalogs={key: dict(count=(3 if key == 'tables' else 2 if key in ('columns', 'checks') else 0), digest='c' * 64) for key in readiness.CATALOG_KEYS},
                                  enabled_events=0, threads=1, event_threads=0, transactions=1, modifying_transactions=1,
                                  metadata_locks=1, mdl_instrument_enabled=True, primary_key_shape_verified=True, consistent_snapshot=True,
                                  assessment_pages=1, outcome_pages=1, assessment_eof=True, outcome_eof=True,
                                  assessment_distribution=dict(zero), outcome_distribution=dict(zero)))

def with_totals(assessment, outcome):
    value = receipt()
    for table, total in (('assessment', assessment), ('outcome', outcome)):
        value['measurements'][table + '_distribution'].update(total=total, sql_null=total)
        value['measurements'][table + '_pages'] = 1 + (total + 999) // 1000
    value['query_count'] = 19 + value['measurements']['assessment_pages'] + value['measurements']['outcome_pages']
    return value

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

    def test_full_pagination_boundaries_include_empty_eof_for_both_tables(self):
        for assessment, outcome in ((0, 0), (1, 999), (999, 1000), (1000, 1001), (1001, 511999), (511999, 512000), (512000, 512000)):
            with self.subTest(assessment=assessment, outcome=outcome):
                value = with_totals(assessment, outcome)
                safe = self.validate(value)
                self.assertTrue(safe['complete'])
                self.assertEqual(safe['query_count'], 19 + safe['measurements']['assessment_pages'] + safe['measurements']['outcome_pages'])
                self.assertLessEqual(safe['query_count'], 1045)
                self.assertEqual(safe['control_statement_count'], 3)
                self.assertTrue(safe['transaction_released'])
        # All four non-SQL-NULL categories still contribute to conservation.
        value = with_totals(1001, 0)
        value['measurements']['assessment_distribution'].update(sql_null=1, non_null=1000, json_null=250,
                                                              empty_object=250, non_empty_object=250, other=250)
        self.assertTrue(self.validate(value)['complete'])

    def test_pagination_requires_fixed_page_size_full_eof_and_bounded_total(self):
        for table in ('assessment', 'outcome'):
            for total, pages in ((0, 0), (0, 2), (1, 1), (999, 3), (1000, 3), (1001, 2), (512000, 512), (512001, 513), (512001, 514)):
                with self.subTest(table=table, total=total, pages=pages):
                    value = with_totals(total if table == 'assessment' else 0, total if table == 'outcome' else 0)
                    value['measurements'][table + '_pages'] = pages
                    value['query_count'] = 19 + value['measurements']['assessment_pages'] + value['measurements']['outcome_pages']
                    with self.assertRaises(Exception):
                        self.validate(value)
            for eof in (False, None, 0, 1, 'true'):
                with self.subTest(table=table, eof=eof):
                    value = with_totals(1000, 1000)
                    value['measurements'][table + '_eof'] = eof
                    with self.assertRaises(Exception):
                        self.validate(value)

    def test_v2_is_required_for_success_and_failure_without_v1_fallback(self):
        for value, code in ((receipt(), 0), (readiness.stage_failure(environment(), 'query_failed'), 1)):
            for version in (0, 1, 3, True, 2.0, '2'):
                with self.subTest(version=version, code=code):
                    invalid = dict(value, format_version=version)
                    with self.assertRaises(Exception):
                        self.validate(invalid, code)
            for field in ('control_statement_count', 'transaction_released'):
                with self.subTest(field=field, code=code):
                    invalid = dict(value)
                    del invalid[field]
                    with self.assertRaises(Exception):
                        self.validate(invalid, code)
        for field in ('primary_key_shape_verified', 'consistent_snapshot', 'assessment_pages', 'outcome_pages', 'assessment_eof', 'outcome_eof'):
            value = receipt()
            del value['measurements'][field]
            with self.subTest(field=field), self.assertRaises(Exception):
                self.validate(value)

    def test_snapshot_and_transaction_proofs_are_required_for_success(self):
        for field in ('primary_key_shape_verified', 'consistent_snapshot'):
            value = receipt()
            value['measurements'][field] = False
            with self.subTest(field=field), self.assertRaises(Exception):
                self.validate(value)
        for transaction in (None, False):
            with self.subTest(transaction=transaction), self.assertRaises(Exception):
                self.validate(dict(receipt(), transaction_released=transaction))
        for controls in (0, 1, 2, 4):
            with self.subTest(controls=controls), self.assertRaises(Exception):
                self.validate(dict(receipt(), control_statement_count=controls))
        for queries in (0, 18, 20, 22, 1045, 1046):
            with self.subTest(queries=queries), self.assertRaises(Exception):
                self.validate(dict(receipt(), query_count=queries))

    def test_new_counts_are_strict_unsigned_and_failed_attempts_stay_bounded(self):
        for field in ('query_count', 'control_statement_count', 'assessment_pages', 'outcome_pages'):
            for invalid in (True, False, -1, 1.0, '1', None, 2 ** 64):
                value = receipt()
                target = value['measurements'] if field.endswith('_pages') else value
                target[field] = invalid
                with self.subTest(field=field, invalid=invalid), self.assertRaises(Exception):
                    self.validate(value)
        for queries, controls, transaction in ((0, 0, None), (1045, 3, True), (17, 3, False)):
            value = readiness.stage_failure(environment(), 'query_failed')
            value.update(query_count=queries, control_statement_count=controls, transaction_released=transaction)
            with self.subTest(queries=queries, controls=controls, transaction=transaction):
                self.assertFalse(self.validate(value, 1)['complete'])
        for field, invalid in (('query_count', 1046), ('control_statement_count', 4), ('transaction_released', 1)):
            value = readiness.stage_failure(environment(), 'query_failed')
            value[field] = invalid
            with self.subTest(field=field), self.assertRaises(Exception):
                self.validate(value, 1)

    def test_new_stages_and_failures_are_closed_and_discard_measurements(self):
        for stage in ('primary_columns', 'primary_indexes', 'snapshot_begin', 'snapshot_upper', 'snapshot_release'):
            value = readiness.stage_failure(environment(), 'query_failed')
            value['failure_stage'] = stage
            with self.subTest(stage=stage):
                self.assertEqual(self.validate(value, 1)['failure_stage'], stage)
        for category, stage in (('pagination_limit_exceeded', 'assessment_distribution'), ('transaction_release_failed', 'snapshot_release')):
            value = readiness.stage_failure(environment(), category)
            value.update(failure_stage=stage, query_count=17, control_statement_count=3, transaction_released=False)
            with self.subTest(category=category):
                safe = self.validate(value, 1)
                self.assertNotIn('measurements', safe)
                invalid = dict(value, measurements=receipt()['measurements'])
                with self.assertRaises(Exception):
                    self.validate(invalid, 1)
        failure = readiness.stage_failure(environment(), 'query_failed')
        self.assertEqual(failure['format_version'], 2)
        self.assertEqual(failure['control_statement_count'], 0)
        self.assertIsNone(failure['transaction_released'])

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
        for category in ('query_failed', 'query_server_timeout', 'query_server_interrupted', 'query_permission_denied', 'query_deadline_exceeded', 'query_canceled', 'metadata_truncated', 'pagination_limit_exceeded', 'transaction_release_failed', 'connection_release_failed', 'database_release_failed'):
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

    def test_typed_mysql_number_is_optional_bounded_and_never_positive(self):
        def failure(category='query_mysql_error', number=1064):
            value = readiness.stage_failure(environment(), category)
            value.update(failure_stage='outcome_distribution', query_count=16,
                         connection_released=True, database_released=True, pool_closed_verified=True)
            if number is not None:
                value['query_mysql_error_number'] = number
            return value
        for number in (0, 1064, 65535):
            self.assertEqual(self.validate(failure(number=number), 1)['query_mysql_error_number'], number)
        for number in (None, True, False, -1, 65536, 1064.0, '1064', 3024, 1317, 1142, 1143, 1227):
            with self.assertRaises(Exception):
                self.validate(failure(number=number), 1)
        for category, number in (('query_server_timeout', 3024), ('query_server_interrupted', 1317), ('query_permission_denied', 1142)):
            self.assertEqual(self.validate(failure(category, number), 1)['query_mysql_error_number'], number)
            with self.assertRaises(Exception):
                self.validate(failure(category, 1064), 1)
            self.assertFalse(self.validate(failure(category, None), 1)['complete'])  # No typed server number was observed.
        for category in ('query_network_timeout', 'query_network_error', 'query_invalid_connection', 'query_bad_connection', 'query_eof', 'query_unexpected_eof'):
            self.assertFalse(self.validate(failure(category, None), 1)['complete'])
            with self.assertRaises(Exception):
                self.validate(failure(category), 1)
        for category in ('metadata_release_failed', 'transaction_release_failed', 'connection_release_failed', 'database_release_failed'):
            value = failure(category)
            if category == 'transaction_release_failed':
                value.update(failure_stage='snapshot_release', control_statement_count=3, transaction_released=False)
            elif category != 'metadata_release_failed':
                value['failure_stage'] = category.removesuffix('_failed')
            self.assertFalse(self.validate(value, 1)['complete'])
        value = receipt()
        value['query_mysql_error_number'] = 0
        with self.assertRaises(Exception):
            self.validate(value)
        value = failure()
        value['query_count'] = 0
        with self.assertRaises(Exception):
            self.validate(value, 1)

    def test_unknown_raw_fields_and_tokens_cannot_cross_armor(self):
        for key in ('grants', 'roles', 'username', 'sql_text', 'body', 'connection_id', 'id', 'snapshot_upper', 'last_id'):
            value = receipt()
            value[key] = SECRET
            with self.assertRaises(Exception):
                self.validate(value)
        for key in ('id', 'snapshot_upper', 'last_id', 'body'):
            value = receipt()
            value['measurements'][key] = SECRET
            with self.subTest(key=key), self.assertRaises(Exception):
                self.validate(value)
        value = readiness.stage_failure(environment(), SECRET)
        self.assertNotIn(SECRET, json.dumps(value))
        self.assertEqual(value['error_category'], 'profile_transport_or_receipt_failed')

if __name__ == '__main__':
    unittest.main()
