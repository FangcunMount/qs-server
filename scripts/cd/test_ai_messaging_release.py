#!/usr/bin/env python3
"""Release risks only: original keys, immutable configuration, fail-closed CD."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('mq_release', Path(__file__).with_name('ai-messaging-release.py'))
module = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(module)
SOURCE = 'a' * 40
IMAGE = 'sha256:' + 'b' * 64


class MQReleaseTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name).resolve()
        self.binding_root = self.root / 'keys'
        self.versions = self.binding_root / 'versions' / 'mq-v1'
        self.versions.mkdir(parents=True)
        self.releases = self.root / 'releases'
        self.base = self.root / 'source.yaml'
        self.base.write_text('ai_workflow:\n  enabled: true\n')
        self.binding = {'binding_revision': 'mq-v1', 'enabled': True,
            'nsqd': {'nsqd:4150': 'http://nsqd:4151'}, 'max_in_flight': 1,
            'signing_key_file': '/run/qs-server-jose/qs.sign.v1.json',
            'decrypt_key_files': {'qs.encrypt': '/run/qs-server-jose/qs.encrypt.json'},
            'ai_signer_files': {'ai.sign': '/run/qs-server-jose/ai.sign.json'},
            'ai_recipient_key_file': '/run/qs-server-jose/ai.encrypt.v1.json'}
        for name in ['qs.sign.v1.json', 'qs.encrypt.json', 'ai.sign.json', 'ai.encrypt.v1.json']:
            file = self.versions / name
            file.write_text('{"fixture":"never logged"}')
            file.chmod(0o600 if name.startswith('qs.') else 0o644)
        self.write_binding()
        self.calls = []
        self.fail_keys = False
        self.source = SOURCE
        self.image_id = IMAGE

    def write_binding(self):
        (self.binding_root / 'current.json').write_text(json.dumps(self.binding))

    def fake_offline(self, image, args, mounts=()):
        self.calls.append((image, args, mounts))
        if args == ['--source-sha']:
            return (self.source + '\n').encode()
        if self.fail_keys:
            raise module.Refused('key unavailable')
        data = {'source_sha': self.source, 'offline': True,
                'binding_revision': 'mq-v1', 'binding_sha256': 'c' * 64,
                'public_key_fingerprints': {'qs.sign.v1': 'd' * 64}}
        if any(arg.startswith('--output-config') for arg in args):
            directory = next(src for src, dst, _ in mounts if dst == '/output')
            raw = b'{"ai_workflow":{"messaging":{"enabled":true}}}\n'
            (directory / 'apiserver.json').write_bytes(raw)
            data['rendered_config_sha256'] = hashlib.sha256(raw).hexdigest()
        return json.dumps(data).encode()

    def prepare(self):
        with patch.object(module, 'offline', self.fake_offline), patch.object(module, 'command',
                return_value=json.dumps([{'Id': self.image_id}]).encode()):
            return module.prepare('selected-tag', self.base, os.getuid(), os.getgid(),
                                  self.binding_root, self.releases)

    def test_mq_freezes_exact_image_and_readonly_original_key_paths(self):
        result = self.prepare()
        self.assertEqual(result['source_sha'], SOURCE)
        config = json.loads(Path(result['compose_file']).read_text())['services']['qs-apiserver']
        self.assertEqual(config['image'], IMAGE)
        self.assertEqual(config['command'], ['--config=/app/configs/apiserver.prod.yaml'])
        self.assertEqual(len(config['volumes']), 5)
        self.assertTrue(all(v['read_only'] for v in config['volumes']))
        self.assertTrue(any(v['target'].endswith('/qs.encrypt.json') for v in config['volumes']))
        self.assertNotIn('never logged', (self.releases / SOURCE / 'metadata.json').read_text())
        self.assertEqual(result['protected_image_ids'], [IMAGE])

    def test_mq_frozen_main_keeps_original_relative_policy_directory(self):
        # Production's cache policy resolves relative to the main config file.
        # Relocating only the main file to /run breaks startup before storage.
        config = module.overlay(self.root / 'release', [], IMAGE)['services']['qs-apiserver']
        main = Path(config['command'][0].split('=', 1)[1])
        self.assertEqual(main.parent / 'cache/apiserver.prod.yaml',
                         Path('/app/configs/cache/apiserver.prod.yaml'))
        mount = next(v for v in config['volumes'] if v['target'] == str(main))
        self.assertEqual(mount['source'], str(self.root / 'release/apiserver.json'))
        self.assertTrue(mount['read_only'])

    def test_mq_frozen_compatible_images_remain_protected_from_retention(self):
        self.prepare()
        self.source = 'f' * 40
        self.image_id = 'sha256:' + 'e' * 64
        next_release = self.prepare()
        self.assertEqual(set(next_release['protected_image_ids']), {IMAGE, self.image_id})

    def test_mq_same_release_and_rollback_reuse_frozen_binding_not_new_pointer(self):
        original = self.prepare()
        fixed = (self.releases / SOURCE / 'binding.json').read_bytes()
        self.binding['binding_revision'] = 'invalid-new-pointer'
        self.write_binding()
        repeated = self.prepare()
        self.assertEqual(original, repeated)
        self.assertEqual(fixed, (self.releases / SOURCE / 'binding.json').read_bytes())
        self.assertTrue(self.calls[-1][2][0][0] == self.releases / SOURCE / 'binding.json')

    def test_mq_refuses_frozen_config_overlay_and_image_drift(self):
        self.prepare()
        for name in ['binding.json', 'apiserver.json', 'compose.json']:
            file = self.releases / SOURCE / name
            raw = file.read_bytes()
            file.write_bytes(raw + b' ')
            with self.assertRaises(module.Refused):
                self.prepare()
            file.write_bytes(raw)
        self.image_id = 'sha256:' + 'e' * 64
        with self.assertRaises(module.Refused):
            self.prepare()
        self.image_id = IMAGE
        self.base.write_text('changed: true\n')
        with self.assertRaises(module.Refused):
            self.prepare()

    def test_mq_key_failure_keeps_old_frozen_release_and_creates_no_new_asset(self):
        self.prepare()
        metadata = (self.releases / SOURCE / 'metadata.json').read_bytes()
        self.source = 'f' * 40
        self.fail_keys = True
        with self.assertRaises(module.Refused):
            self.prepare()
        self.assertFalse((self.releases / self.source).exists())
        self.assertEqual(metadata, (self.releases / SOURCE / 'metadata.json').read_bytes())
        self.assertFalse(list(self.releases.glob('.mq-preflight-*')))

    def test_mq_private_permissions_role_mapping_symlink_and_missing_keys_refused(self):
        private = self.versions / 'qs.sign.v1.json'
        private.chmod(0o644)
        with self.assertRaises(module.Refused):
            self.prepare()
        private.chmod(0o600)
        private.unlink()
        private.symlink_to(self.versions / 'qs.encrypt.json')
        with self.assertRaises(module.Refused):
            self.prepare()
        private.unlink()
        with self.assertRaises(module.Refused):
            self.prepare()
        self.binding['signing_key_file'] = '/etc/unreviewed.json'
        self.write_binding()
        with self.assertRaises(module.Refused):
            self.prepare()

    def test_mq_duplicate_binding_keys_and_legacy_source_refused(self):
        (self.binding_root / 'current.json').write_text('{"enabled":true,"enabled":false}')
        with self.assertRaises(module.Refused):
            self.prepare()
        self.source = 'development'
        with self.assertRaises(module.Refused):
            self.prepare()
        self.assertFalse(list(self.releases.iterdir()) if self.releases.exists() else [])

    def test_mq_preflight_has_no_network_privileges_environment_or_normal_entrypoint(self):
        with patch.object(module, 'command', return_value=b'{}') as command:
            module.offline(IMAGE, ['--source-sha'])
        args = command.call_args.args[0]
        self.assertEqual(args[args.index('--platform') + 1], 'linux/amd64')
        self.assertEqual(args[args.index('--network') + 1], 'none')
        self.assertIn('--read-only', args)
        self.assertEqual(args[args.index('--cap-drop') + 1], 'ALL')
        self.assertEqual(args[args.index('--entrypoint') + 1], '/app/qs-ai-messaging-preflight')
        self.assertNotIn('--env-file', args)
        self.assertNotIn('--env', args)

    def test_mq_preflight_precedes_config_sync_and_stop_without_changing_default_flow(self):
        text = Path(__file__).with_name('remote-deploy.sh').read_text()
        body = text[text.index('acquire_image_deploy_lock\n'):]
        self.assertLess(body.index('ai-messaging-release.py'), body.index('\nsync_configs\n'))
        self.assertLess(body.index('ai-messaging-release.py'), body.index('\n    deploy_http_service'))
        self.assertIn('if [ "$MQ_IMAGE_SELECTED" != "1" ]; then select_image; fi', body)
        service = text[text.index('deploy_http_service()'):text.index('collection_container_ids()')]
        self.assertLess(service.index('/ai-mq-releases/required'), service.index('stop_single_container'))
        self.assertIn('compose_args+=(-f "$MQ_COMPOSE_OVERRIDE")', service)


if __name__ == '__main__':
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(MQReleaseTests)
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    print(json.dumps({'passed': result.testsRun - len(result.failures) - len(result.errors),
                      'failed': len(result.failures), 'errors': len(result.errors), 'skipped': len(result.skipped)}))
    raise SystemExit(0 if result.wasSuccessful() and result.testsRun and not result.skipped else 1)
