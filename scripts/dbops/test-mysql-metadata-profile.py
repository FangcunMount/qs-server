import contextlib
import copy
import importlib.util
import io
import json
import os
from pathlib import Path
import stat
import sys
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('profile', Path(__file__).with_name('mysql-metadata-profile.py'))
profile = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(profile)
transport = profile.runtime_module()
SHA, RUN, HASH = 'a' * 40, '123-1', 'b' * 64
SECRET = 'fixture_password_NEVER_PRINT'


def environment():
    return {'MYSQL_HOST': 'mysql.fixture', 'MYSQL_PORT': '3306', 'MYSQL_USERNAME': 'fixture',
            'MYSQL_PASSWORD': SECRET, 'MYSQL_DATABASE': 'fixture_source',
            'PROFILE_SOURCE_SHA': SHA, 'PROFILE_RUN_ID': RUN, 'PROFILE_EXPECTED_TARGET_HASH': HASH}


def receipt():
    return {'format_version': 1, 'source_sha': SHA, 'run_id': RUN, 'expected_target_hash': HASH,
            'source_target_hash': HASH, 'current_unrestricted_metadata_grants': False,
            'rds_role_grants_available': True, 'rds_role_unrestricted_metadata_grants': True,
            'assigned_roles_present': False, 'mandatory_roles_present': False,
            'diagnostic_only': True, 'complete': True, 'error_category': 'none'}


class FakeRuntime:
    last = None
    fail_image = False
    fail_cleanup = False

    def __init__(self, binary, run, sha):
        self.owner = 'c' * 32
        self.label = transport.OWNER_LABEL + '=' + self.owner
        self.docker = ['sudo', '-n', 'docker']
        self.cleaned = False
        self.calls = []
        FakeRuntime.last = self

    def preflight_name(self, name):
        self.calls.append(('preflight', name))

    def image(self, tag):
        self.calls.append(('image', tag))
        if self.fail_image:
            raise ValueError(SECRET)
        return 'sha256:' + 'd' * 64

    def cleanup(self):
        self.cleaned = True
        if self.fail_cleanup:
            raise ValueError(SECRET)


class ProfileTests(unittest.TestCase):
    def validate(self, value, code=0):
        return profile.safe_receipt(transport, json.dumps(value), code, SHA, RUN, HASH)

    def setUp(self):
        FakeRuntime.last = None
        FakeRuntime.fail_image = False
        FakeRuntime.fail_cleanup = False

    def test_inactive_role_potential_is_not_current(self):
        value = self.validate(receipt())
        self.assertFalse(value['current_unrestricted_metadata_grants'])
        self.assertTrue(value['rds_role_unrestricted_metadata_grants'])
        self.assertTrue(value['diagnostic_only'])

    def test_role_unavailable_and_known_negative_are_complete(self):
        value = receipt()
        value.update(rds_role_grants_available=False, rds_role_unrestricted_metadata_grants=None)
        self.assertTrue(self.validate(value)['complete'])
        value.update(rds_role_grants_available=True, rds_role_unrestricted_metadata_grants=False)
        self.assertTrue(self.validate(value)['complete'])

    def test_extra_raw_fields_and_duplicate_json_rejected(self):
        for field in ('raw_grants', 'password', 'host', 'roles', 'uuid'):
            with self.subTest(field=field):
                value = receipt()
                value[field] = SECRET
                with self.assertRaises(Exception):
                    self.validate(value)
        raw = json.dumps(receipt())[:-1] + ', "complete": true}'
        with self.assertRaises(Exception):
            profile.safe_receipt(transport, raw, 0, SHA, RUN, HASH)

    def test_binding_mismatch_rejected(self):
        for field, value in (('source_sha', 'c' * 40), ('run_id', '124-1'),
                             ('expected_target_hash', 'c' * 64), ('source_target_hash', 'c' * 64)):
            with self.subTest(field=field):
                wrong = receipt()
                wrong[field] = value
                with self.assertRaises(Exception):
                    self.validate(wrong)

    def test_boolean_types_unknown_fields_and_role_conflicts_rejected(self):
        cases = [('diagnostic_only', 1), ('complete', 1), ('current_unrestricted_metadata_grants', 0),
                 ('rds_role_grants_available', 1), ('rds_role_unrestricted_metadata_grants', 'true'),
                 ('error_category', []), ('error_category', SECRET), ('format_version', True),
                 ('rds_role_grants_available', None), ('assigned_roles_present', None), ('assigned_roles_present', 1),
                 ('mandatory_roles_present', None), ('mandatory_roles_present', 'false'), ('source_target_hash', SECRET)]
        for field, value in cases:
            with self.subTest(field=field):
                wrong = receipt()
                wrong[field] = value
                with self.assertRaises(Exception):
                    self.validate(wrong)
        wrong = receipt()
        wrong['rds_role_grants_available'] = False
        with self.assertRaises(Exception):
            self.validate(wrong)

    def test_missing_fields_rejected(self):
        for field in profile.KEYS - {'source_target_hash'}:
            with self.subTest(field=field):
                value = receipt()
                del value[field]
                with self.assertRaises(Exception):
                    self.validate(value)

    def test_failed_query_has_no_positive_receipt(self):
        value = receipt()
        value.update(complete=False, error_category='rds_role_query_failed',
                     rds_role_grants_available=None, rds_role_unrestricted_metadata_grants=None,
                     assigned_roles_present=None, mandatory_roles_present=None)
        self.assertFalse(self.validate(value, 1)['complete'])
        for field in ('current_unrestricted_metadata_grants', 'rds_role_unrestricted_metadata_grants'):
            with self.subTest(field=field):
                wrong = copy.deepcopy(value)
                wrong[field] = True
                with self.assertRaises(Exception):
                    self.validate(wrong, 1)
        with self.assertRaises(Exception):
            self.validate(value, 0)
        with self.assertRaises(Exception):
            self.validate(receipt(), 1)

    def test_census_never_survives_failed_diagnostic(self):
        for field in ('rds_role_grants_available', 'rds_role_unrestricted_metadata_grants', 'assigned_roles_present', 'mandatory_roles_present'):
            for boolean in (False, True):
                with self.subTest(field=field, boolean=boolean):
                    value = receipt()
                    value.update(complete=False, error_category='mandatory_roles_query_failed',
                                 current_unrestricted_metadata_grants=False,
                                 rds_role_grants_available=None, rds_role_unrestricted_metadata_grants=None,
                                 assigned_roles_present=None, mandatory_roles_present=None)
                    value[field] = boolean
                    with self.assertRaises(Exception):
                        self.validate(value, 1)

    def execute_fixture(self, callback, env=None):
        with tempfile.TemporaryDirectory() as directory:
            binary = Path(directory) / 'profile'
            binary.write_bytes(b'fixture binary')
            with patch.object(transport, 'Runtime', FakeRuntime), patch.object(transport, 'capture', callback):
                return profile.execute(str(binary), env or environment(), transport)

    def test_container_boundary_env_private_and_removed(self):
        seen = []
        def capture(args, **kwargs):
            self.assertEqual(kwargs, {'timeout': 180})
            self.assertNotIn(SECRET, ' '.join(args))
            for value in ('mysql.fixture', 'fixture_source', SHA, HASH):
                self.assertNotIn(value, args)
            self.assertEqual(args[:4], ['sudo', '-n', 'docker', 'run'])
            for value in ('--rm', '--read-only', '--cap-drop=ALL', '--pull=never', '--security-opt=no-new-privileges'):
                self.assertIn(value, args)
            self.assertNotIn('--volume', args)
            self.assertNotIn('--env', args)
            self.assertEqual(args[args.index('--network') + 1], 'infra-network')
            self.assertEqual(args[-1], 'sha256:' + 'd' * 64)
            path = Path(args[args.index('--env-file') + 1])
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
            self.assertEqual(stat.S_IMODE(path.parent.stat().st_mode), 0o700)
            self.assertIn('MYSQL_PASSWORD=' + SECRET + '\n', path.read_text())
            self.assertIn('MYSQL_PORT=3306\n', path.read_text())
            seen.append(path)
            return 0, json.dumps(receipt())
        code, value = self.execute_fixture(capture)
        self.assertEqual(code, 0)
        self.assertTrue(value['complete'])
        self.assertTrue(FakeRuntime.last.cleaned)
        self.assertFalse(seen[0].exists())

    def test_transport_failure_removes_private_env_and_owned_container(self):
        seen = []
        def capture(args, **kwargs):
            seen.append(Path(args[args.index('--env-file') + 1]))
            raise TimeoutError(SECRET)
        with self.assertRaises(Exception):
            self.execute_fixture(capture)
        self.assertTrue(FakeRuntime.last.cleaned)
        self.assertFalse(seen[0].exists())

    def test_image_failure_still_cleans_owned_resources(self):
        FakeRuntime.fail_image = True
        with self.assertRaises(Exception):
            self.execute_fixture(lambda *args, **kwargs: self.fail('no capture expected'))
        self.assertTrue(FakeRuntime.last.cleaned)

    def test_cleanup_failure_never_reports_complete(self):
        FakeRuntime.fail_cleanup = True
        with self.assertRaises(Exception):
            self.execute_fixture(lambda *args, **kwargs: (0, json.dumps(receipt())))

    def test_invalid_binding_and_symlink_fail_before_runtime(self):
        with tempfile.TemporaryDirectory() as directory:
            binary = Path(directory) / 'profile'
            binary.write_bytes(b'fixture')
            link = Path(directory) / 'link'
            link.symlink_to(binary)
            with patch.object(transport, 'Runtime', FakeRuntime):
                for target in (str(link), 'relative'):
                    with self.assertRaises(Exception):
                        profile.execute(target, environment(), transport)
                for key in ('PROFILE_SOURCE_SHA', 'PROFILE_RUN_ID', 'PROFILE_EXPECTED_TARGET_HASH', 'MYSQL_DATABASE', 'MYSQL_PASSWORD'):
                    with self.subTest(key=key):
                        env = environment()
                        env[key] = 'invalid\n' + SECRET
                        with self.assertRaises(Exception):
                            profile.execute(str(binary), env, transport)
                self.assertIsNone(FakeRuntime.last)

    def test_main_error_output_suppresses_raw_secret(self):
        out = io.StringIO()
        with patch.object(sys, 'argv', ['profile', '--binary', '/fixture']), \
             patch.object(profile, 'runtime_module', return_value=transport), \
             patch.object(profile, 'execute', side_effect=RuntimeError(SECRET)), contextlib.redirect_stdout(out):
            self.assertEqual(profile.main(), 1)
        self.assertNotIn(SECRET, out.getvalue())
        value = json.loads(out.getvalue())
        self.assertFalse(value['complete'])
        self.assertTrue(value['diagnostic_only'])
        self.assertEqual(value['error_category'], 'profile_transport_or_receipt_failed')


if __name__ == '__main__':
    unittest.main()
