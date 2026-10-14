import copy
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import stat
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch

DIRECTORY = Path(__file__).parent
SPEC = importlib.util.spec_from_file_location('profile_ssh', DIRECTORY / 'mysql-metadata-profile-ssh.py')
ssh = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(ssh)
profile = ssh.load_profile(DIRECTORY)
armor = profile.receipt_transport_module()
SECRET = 'test-secret-never-argv-or-log'
SHA, RUN, HASH = 'a' * 40, '123-1', 'b' * 64


def environment():
    return {'MYSQL_HOST': 'mysql.fixture', 'MYSQL_PORT': '3306', 'MYSQL_USERNAME': 'fixture',
            'MYSQL_PASSWORD': SECRET, 'MYSQL_DATABASE': 'fixture_source',
            'PROFILE_SOURCE_SHA': SHA, 'PROFILE_RUN_ID': RUN, 'PROFILE_EXPECTED_TARGET_HASH': HASH,
            'PROFILE_OPERATION': 'metadata-visibility-admin-profile', 'PROFILE_DATABASE': 'mysql', 'PROFILE_ARCHIVE_INPUT': ''}


def receipt():
    return {'format_version': 1, 'source_sha': SHA, 'run_id': RUN, 'expected_target_hash': HASH,
            'source_target_hash': HASH, 'current_unrestricted_metadata_grants': False,
            'rds_role_grants_available': False, 'rds_role_unrestricted_metadata_grants': None,
            'assigned_roles_present': False, 'mandatory_roles_present': False,
            'diagnostic_only': True, 'complete': True, 'error_category': 'none'}


def package(members=None):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode='w:gz') as archive:
        for name, content, kind in members or [(n, b'owned-fixture', tarfile.REGTYPE) for n in sorted(ssh.MEMBERS)]:
            member = tarfile.TarInfo(name)
            member.type, member.size = kind, len(content)
            member.linkname = '/etc/passwd' if kind == tarfile.SYMTYPE else ''
            archive.addfile(member, io.BytesIO(content) if kind == tarfile.REGTYPE else None)
    return output.getvalue()


def envelope(raw, env=None):
    env = env or environment()
    return {'env': {key: env[key] for key in profile.PROFILE_KEYS},
            'package_sha256': hashlib.sha256(raw).hexdigest(), 'package_size': len(raw)}


class SSHProfileTests(unittest.TestCase):
    def test_dispatch_keeps_secret_only_in_stdin_and_binds_sealed_receipt(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'package.tar.gz'
            path.write_bytes(package())
            path.chmod(0o600)
            env = environment()
            env['PROFILE_PACKAGE_SHA256'] = hashlib.sha256(path.read_bytes()).hexdigest()
            calls = []
            def capture(args, **kw):
                self.assertNotIn(SECRET, ' '.join(args))
                self.assertNotIn(SECRET, str(kw['env']))
                head, raw = kw['payload'].split(b'\n', 1)
                private = json.loads(head)
                self.assertEqual(private['env']['MYSQL_PASSWORD'], SECRET)
                self.assertEqual(hashlib.sha256(raw).hexdigest(), private['package_sha256'])
                self.assertEqual(kw['timeout'], 90)
                calls.append(args)
                return 0, armor.encode_armored_receipt(receipt(), schema=profile.RECEIPT_SCHEMA).encode()
            with patch.object(ssh, 'trusted_ssh', return_value=(['ssh', '-l', 'root', '001-ysj'], {})), patch.object(profile, 'capture_private', side_effect=capture):
                code, sealed = ssh.dispatch(path, env, profile, Path(directory))
            self.assertEqual(code, 0)
            self.assertTrue(json.loads(armor.decode_armored_receipt(sealed))['complete'])
            self.assertEqual(len(calls), 1)

    def test_operation_missing_admin_secret_source_or_checksum_never_calls_ssh(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'package.tar.gz'
            path.write_bytes(package())
            path.chmod(0o600)
            env = environment()
            env['PROFILE_PACKAGE_SHA256'] = hashlib.sha256(path.read_bytes()).hexdigest()
            for key, value in (('PROFILE_OPERATION', 'restore'), ('PROFILE_DATABASE', 'all'), ('PROFILE_ARCHIVE_INPUT', 'archive'),
                               ('MYSQL_PASSWORD', ''), ('MYSQL_USERNAME', ''), ('PROFILE_SOURCE_SHA', 'main'),
                               ('PROFILE_RUN_ID', '123'), ('PROFILE_EXPECTED_TARGET_HASH', 'unknown'), ('PROFILE_PACKAGE_SHA256', 'c' * 64)):
                with self.subTest(key=key), patch.object(ssh, 'trusted_ssh') as trusted:
                    invalid = dict(env, **{key: value})
                    with self.assertRaises(Exception):
                        ssh.dispatch(path, invalid, profile, Path(directory))
                    trusted.assert_not_called()

    def test_raw_or_wrong_bound_remote_output_cannot_pass(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'package.tar.gz'
            path.write_bytes(package())
            path.chmod(0o600)
            env = environment()
            env['PROFILE_PACKAGE_SHA256'] = hashlib.sha256(path.read_bytes()).hexdigest()
            wrong = dict(receipt(), source_sha='d' * 40)
            for raw in (SECRET.encode(), armor.encode_armored_receipt(wrong, schema=profile.RECEIPT_SCHEMA).encode(),
                        (armor.encode_armored_receipt(receipt(), schema=profile.RECEIPT_SCHEMA) * 2).encode()):
                with patch.object(ssh, 'trusted_ssh', return_value=(['ssh'], {})), patch.object(profile, 'capture_private', return_value=(0, raw)):
                    with self.assertRaises(Exception):
                        ssh.dispatch(path, env, profile, Path(directory))

    def test_wrong_source_node_stops_before_secret_stream_read(self):
        class UnreadStream:
            def readline(self, *_):
                raise AssertionError('secret_envelope_was_read')
        with patch.object(ssh.os, 'getuid', return_value=0), patch.object(ssh.platform, 'system', return_value='Linux'), patch.object(ssh.socket, 'gethostname', return_value='serverB'):
            with self.assertRaisesRegex(ValueError, '^receiver_host$'):
                ssh.receive(UnreadStream())
        with patch.object(ssh.os, 'getuid', return_value=0), patch.object(ssh.platform, 'system', return_value='Linux'), patch.object(ssh.socket, 'gethostname', return_value='serverA'), patch.object(ssh.platform, 'machine', return_value='x86_64'):
            self.assertEqual(ssh.receiver_arch(), 'amd64')

    def test_receiver_cleans_owned_files_before_positive_output(self):
        raw = package()
        payload = json.dumps(envelope(raw)).encode() + b'\n' + raw
        seen = []
        def execute(binary, env, module):
            path = Path(binary)
            seen.append(path.parent)
            self.assertEqual(stat.S_IMODE(path.parent.stat().st_mode), 0o700)
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o700)
            self.assertFalse((path.parent / 'profile.env.json').exists())
            self.assertEqual(env['MYSQL_PASSWORD'], SECRET)
            return 0, receipt()
        with patch.object(ssh, 'receiver_arch', return_value='amd64'), patch.object(ssh, 'load_profile', return_value=profile), patch.object(profile, 'execute_native', side_effect=execute):
            code, sealed = ssh.receive(io.BytesIO(payload))
        self.assertEqual(code, 0)
        self.assertTrue(json.loads(armor.decode_armored_receipt(sealed))['complete'])
        self.assertFalse(seen[0].exists())

    def test_receiver_interrupt_cleans_without_positive_output(self):
        raw = package()
        payload = json.dumps(envelope(raw)).encode() + b'\n' + raw
        seen = []
        def interrupted(binary, _env, _module):
            import signal
            seen.append(Path(binary).parent)
            signal.getsignal(signal.SIGHUP)(signal.SIGHUP, None)
        with patch.object(ssh, 'receiver_arch', return_value='amd64'), patch.object(ssh, 'load_profile', return_value=profile), patch.object(profile, 'execute_native', side_effect=interrupted):
            with self.assertRaises(RuntimeError):
                ssh.receive(io.BytesIO(payload))
        self.assertFalse(seen[0].exists())

    def test_receiver_cleanup_failure_discards_success(self):
        raw = package()
        payload = json.dumps(envelope(raw)).encode() + b'\n' + raw
        removed = []
        cleanup = ssh.remove_owned
        def fail_after_cleanup(path):
            cleanup(path)
            removed.append(path)
            raise OSError(SECRET)
        with patch.object(ssh, 'receiver_arch', return_value='amd64'), patch.object(ssh, 'load_profile', return_value=profile), patch.object(profile, 'execute_native', return_value=(0, receipt())), patch.object(ssh, 'remove_owned', side_effect=fail_after_cleanup):
            with self.assertRaises(Exception):
                ssh.receive(io.BytesIO(payload))
        self.assertFalse(removed[0].exists())

    def test_traversal_symlink_duplicate_and_missing_members_rejected(self):
        normal = [(n, b'fixture', tarfile.REGTYPE) for n in sorted(ssh.MEMBERS)]
        for members in (normal + [('../escape', b'fixture', tarfile.REGTYPE)], normal + [normal[0]],
                        [('profile-linux-amd64', b'', tarfile.SYMTYPE)], normal[:-1]):
            with self.subTest(members=[m[0] for m in members]), tempfile.TemporaryDirectory() as directory:
                with self.assertRaises(Exception):
                    ssh.extract_archive(package(members), Path(directory))
                self.assertFalse((Path(directory).parent / 'escape').exists())

    def test_receiver_rejects_unknown_environment_checksum_duplicate_and_limits(self):
        raw = package()
        base = envelope(raw)
        cases = [dict(base, package_sha256='e' * 64), dict(base, package_size=True), dict(base, package_size=ssh.ARCHIVE_LIMIT + 1),
                 dict(base, env=dict(base['env'], LD_PRELOAD=SECRET))]
        lines = [json.dumps(v).encode() for v in cases] + [b'{"env":{},"env":{}}', b'x' * (ssh.HEADER_LIMIT + 1)]
        with patch.object(ssh, 'receiver_arch', return_value='amd64'):
            for line in lines:
                with self.subTest(length=len(line)), patch.object(profile, 'execute_native') as execute:
                    with self.assertRaises(Exception):
                        ssh.receive(io.BytesIO(line + b'\n' + raw))
                    execute.assert_not_called()

    def test_existing_known_host_required_and_strict_per_call_options(self):
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            with self.assertRaises(Exception):
                ssh.trusted_ssh(profile, home)
            (home / '.ssh').mkdir()
            (home / '.ssh' / 'known_hosts').write_text('trusted fixture\n')
            calls = []
            def capture(args, **kw):
                calls.append(args)
                return (0, b'user root\nhostname fixture.example\nport 22\n') if '-G' in args else (0, b'# found\nfixture key\n')
            with patch.object(profile, 'capture_private', side_effect=capture):
                args, _ = ssh.trusted_ssh(profile, home)
            for option in ('StrictHostKeyChecking=yes', 'BatchMode=yes', 'UserKnownHostsFile=' + str(home / '.ssh/known_hosts'), 'ControlPath=none', 'ClearAllForwardings=yes', 'ForwardAgent=no', 'ForwardX11=no', 'ForwardX11Trusted=no'):
                self.assertIn(option, args)
            self.assertEqual(args[-3:], ['-l', 'root', '001-ysj'])
            for config in (b'user root\nhostname 001-ysj\n', b'user wrong\nhostname fixture\n', b'user root\nhostname fixture\nproxycommand dangerous\n'):
                with patch.object(profile, 'capture_private', return_value=(0, config)), self.assertRaises(Exception):
                    ssh.trusted_ssh(profile, home)

    def test_real_offline_ssh_config_rejects_hostkeyalias_before_key_or_secret_transport(self):
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            (home / '.ssh').mkdir()
            (home / '.ssh/known_hosts').write_text('owned fixture, not a real host key\n')
            config = home / 'owned-ssh-config'
            config.write_text('Host 001-ysj\n  HostName example.invalid\n  User root\n  HostKeyAlias unexpected-key-identity\n')
            capture = profile.capture_private
            calls = []
            def offline(args, **kwargs):
                calls.append(args)
                self.assertIn('-G', args)
                self.assertEqual(args[0], '/usr/bin/ssh')
                # Real OpenSSH parses only this owned config; -G never connects.
                return capture([args[0], '-F', str(config), *args[1:]], **kwargs)
            with patch.object(profile, 'capture_private', side_effect=offline), self.assertRaisesRegex(ValueError, '^ssh_alias_binding$'):
                ssh.trusted_ssh(profile, home)
            self.assertEqual(len(calls), 1)
            self.assertNotIn('-F', calls[0])  # Fixture injection never alters production arguments.

    def test_native_capture_discards_stderr_and_limits_timeout_and_output(self):
        code, raw = profile.capture_private([sys.executable, '-c', 'import sys;print("safe");sys.stderr.write("' + SECRET + '")'], env={})
        self.assertEqual((code, raw), (0, b'safe\n'))
        with self.assertRaises(TimeoutError):
            profile.capture_private([sys.executable, '-c', 'import time;time.sleep(10)'], env={}, timeout=0.05)
        with self.assertRaises(ValueError):
            profile.capture_private([sys.executable, '-c', 'print("x"*100000)'], env={}, limit=1024)

    def test_native_environment_does_not_inherit_other_secrets(self):
        with tempfile.TemporaryDirectory() as directory:
            binary = Path(directory) / 'profile'
            binary.write_text('owned')
            env = dict(environment(), LD_PRELOAD=SECRET, UNRELATED_TOKEN=SECRET)
            def capture(args, **kw):
                self.assertEqual(args, [str(binary)])
                self.assertEqual(set(kw['env']), set(profile.PROFILE_KEYS))
                self.assertNotIn('LD_PRELOAD', kw['env'])
                return 0, json.dumps(receipt()).encode()
            with patch.object(profile, 'capture_private', side_effect=capture):
                self.assertTrue(profile.execute_native(str(binary), env, profile.runtime_module())[1]['complete'])

    def test_release_failure_categories_are_closed_and_remove_facts(self):
        for category in ('connection_release_failed', 'database_release_failed'):
            value = dict(receipt(), complete=False, error_category=category, rds_role_grants_available=None,
                         assigned_roles_present=None, mandatory_roles_present=None)
            self.assertFalse(profile.safe_receipt(profile.runtime_module(), json.dumps(value), 1, SHA, RUN, HASH)['complete'])
            invalid = dict(value, rds_role_unrestricted_metadata_grants=True)
            with self.assertRaises(Exception):
                profile.safe_receipt(profile.runtime_module(), json.dumps(invalid), 1, SHA, RUN, HASH)


if __name__ == '__main__':
    unittest.main()
