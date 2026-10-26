#!/usr/bin/env python3
"""Required CI: actual standard image, actual host freeze tool, isolated files.

Only the offline preflight entrypoint runs, with network=none. No application,
DB, Broker, model, registry push, global prune or production path is used.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--fixture', type=Path, required=True)
    parser.add_argument('--image', required=True)
    parser.add_argument('--report', type=Path, required=True)
    args = parser.parse_args()
    assert os.geteuid() == 0 and os.environ.get('GITHUB_ACTIONS') == 'true'
    root = args.fixture.resolve()
    runner_temp = Path(os.environ['RUNNER_TEMP']).resolve()
    assert root.is_relative_to(runner_temp) and root != runner_temp
    assert re.fullmatch(r'qs-mq-qs-apiserver:ci', args.image)
    spec = importlib.util.spec_from_file_location('reviewed_freeze', Path(__file__).with_name('ai-messaging-release.py'))
    tool = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(tool)
    for path in root.rglob('*'):
        assert not path.is_symlink()
        os.chown(path, 0, 2000)
        path.chmod(0o750 if path.is_dir() else 0o640)
    releases = root / 'releases'
    binding_root = root / 'bindings'
    current = binding_root / 'current.json'
    binding_original = current.read_bytes()
    version = binding_root / 'versions/m4-native-fixture'
    cases = []

    def prepare():
        return tool.prepare(args.image, root / 'source.yaml', 2000, 2000, binding_root, releases)

    result = prepare()
    source = result['source_sha']
    assert source == os.environ['GITHUB_SHA']
    release = releases / source
    metadata_original = (release / 'metadata.json').read_bytes()
    metadata = json.loads(metadata_original)
    assert metadata['final_transaction_binding'] is True
    assert set(metadata['transport_files_sha256']) == {'credentials.json', 'mongo-credentials.json', 'ca.crt', 'routes.json'}
    overlay = json.loads(Path(result['compose_file']).read_bytes())['services']['qs-apiserver']
    assert len(overlay['volumes']) == 9
    assert all(v['read_only'] and not v['bind']['create_host_path'] for v in overlay['volumes'])
    config = json.loads((release / 'apiserver.json').read_bytes())
    domain = config['messaging']['rocketmq']['transactions']
    assert domain['qs.mysql']['store_id'] == 'Native-$主'
    assert domain['qs.mysql']['writer_epoch'] == 'Native-$$-Epoch'
    assert domain['qs.mongo']['credentials_file'] != domain['qs.mysql']['credentials_file']
    assert config['iam']['authz-sync']['rocketmq']['publish_enabled'] is False
    cases.append('actual-image-final-binding-and-four-transport-files')
    assert prepare() == result
    cases.append('same-image-frozen-config-revalidated')

    def refused(name, path, mutate):
        raw, mode = path.read_bytes(), path.stat().st_mode & 0o777
        try:
            mutate(path)
            try:
                prepare()
            except tool.Refused:
                pass
            else:
                raise AssertionError('invalid material accepted: ' + name)
            assert (release / 'metadata.json').read_bytes() == metadata_original
            assert not list(releases.glob('.mq-preflight-*'))
            cases.append(name)
        finally:
            if path.is_symlink() or path.exists():
                path.unlink()
            path.write_bytes(raw)
            os.chown(path, 0, 2000)
            path.chmod(mode)

    mongo = version / 'rocketmq/mongo-credentials.json'
    refused('missing-mongo-credential', mongo, lambda p: p.unlink())
    refused('world-readable-mongo-credential', mongo, lambda p: p.chmod(0o644))
    def link_sql(p):
        p.unlink()
        p.symlink_to(version / 'rocketmq/credentials.json')
    refused('mutable-mongo-symlink', mongo, link_sql)
    refused('duplicate-broker-principal', mongo, lambda p: p.write_bytes((version / 'rocketmq/credentials.json').read_bytes()))
    routes = version / 'rocketmq/routes.json'
    refused('canonical-but-wrong-event-owner', routes, lambda p: p.write_bytes((root / 'wrong-events.routes.json').read_bytes()))
    def missing_iam(p):
        value = json.loads(p.read_bytes())
        value['routes'] = [r for r in value['routes'] if r['topic'] != 'iam.authz.version.v2']
        p.write_text(json.dumps(value))
    refused('missing-iam-transaction-route', routes, missing_iam)
    refused('missing-jose-key', version / 'qs.sign.fixture.json', lambda p: p.unlink())
    assert prepare() == result
    cases.append('restored-owned-fixtures-reuse-original-release')
    assert current.read_bytes() == binding_original
    assert sorted(p.name for p in releases.iterdir()) == [source]
    args.report.parent.mkdir(parents=True, exist_ok=True)
    report = {'source_sha': source, 'image_id': result['image_id'], 'passed': len(cases), 'failed': 0,
              'errors': 0, 'skipped': 0, 'case_ids': cases, 'offline': True, 'network': 'none',
              'normal_application_started': False, 'production': False,
              'metadata_sha256': hashlib.sha256(metadata_original).hexdigest()}
    args.report.write_text(json.dumps(report, indent=2) + '\n')
    args.report.chmod(0o600)
    print(json.dumps({k: v for k, v in report.items() if k != 'case_ids'}))


if __name__ == '__main__':
    main()
