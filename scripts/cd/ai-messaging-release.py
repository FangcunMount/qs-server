#!/usr/bin/env python3
"""Freeze and preflight the QS MQ release before replacing a running container.

Uses only local reviewed metadata and an offline, ordinary image entrypoint.
Never connects to a database, broker or model. No environment credentials are
copied into the frozen configuration. Invoke under the existing deploy lock.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

BINDING_ROOT = Path('/data/infra/qs-server-messaging')
RELEASE_ROOT = Path('/opt/qs-server/qs-apiserver/ai-mq-releases')
SHA = re.compile(r'[0-9a-f]{40}')
REVISION = re.compile(r'[a-z0-9][a-z0-9-]{0,63}')
ROLE = re.compile(r'(qs\.sign|qs\.encrypt|ai\.sign|ai\.encrypt)(?:\.[a-z0-9][a-z0-9._-]{0,63})?\.json')


class Refused(Exception):
    pass


def require(value):
    if not value:
        raise Refused('MQ release preflight failed; material withheld')


def unique(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result)
        result[key] = value
    return result


def read(path, limit):
    require(path.is_file() and not path.is_symlink())
    require(0 < path.stat().st_size <= limit)
    raw = path.read_bytes()
    require(len(raw) <= limit)
    return raw


def decoded(raw):
    return json.loads(raw, object_pairs_hook=unique)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def command(args):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            timeout=120, check=False)
    require(result.returncode == 0 and len(result.stdout) <= 32768)
    return result.stdout


def offline(image, args, mounts=()):
    run = ['docker', 'run', '--rm', '--platform', 'linux/amd64', '--network', 'none', '--read-only',
           '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
           '--entrypoint', '/app/qs-ai-messaging-preflight']
    for source, target, readonly in mounts:
        require(',' not in str(source) and ',' not in str(target))
        run += ['--mount', f'type=bind,src={source},dst={target}' + (',readonly' if readonly else '')]
    return command(run + [image] + args)


def key_mounts(binding, root):
    revision = binding.get('binding_revision')
    require(isinstance(revision, str) and REVISION.fullmatch(revision))
    require(binding.get('enabled') is True and binding.get('max_in_flight') == 1)
    require(type(binding['max_in_flight']) is int)
    require(set(binding) == {'binding_revision', 'enabled', 'nsqd', 'signing_key_file',
                             'decrypt_key_files', 'ai_signer_files', 'ai_recipient_key_file', 'max_in_flight'})
    entries = [(binding['signing_key_file'], 'qs.sign', None),
               (binding['ai_recipient_key_file'], 'ai.encrypt', None)]
    for field, role in [('decrypt_key_files', 'qs.encrypt'), ('ai_signer_files', 'ai.sign')]:
        values = binding[field]
        require(isinstance(values, dict) and 1 <= len(values) <= 8)
        entries += [(path, role, kid) for kid, path in values.items()]
    mounts = []
    for target, role, kid in entries:
        require(isinstance(target, str))
        path = Path(target)
        match = ROLE.fullmatch(path.name)
        require(path.parent == Path('/run/qs-server-jose') and match and match.group(1) == role)
        require(kid is None or kid == path.stem)
        source = root / 'versions' / revision / path.name
        require(source.parent.is_dir() and not source.parent.is_symlink())
        require(source.resolve() == source.absolute())
        read(source, 16384)  # No key bytes leave this process or enter metadata.
        require(source.stat().st_mode & 0o022 == 0)
        require(not role.startswith('qs.') or source.stat().st_mode & 0o007 == 0)
        mounts.append((source, target, True))
    return mounts


def overlay(release, mounts, image):
    volumes = [{'type': 'bind', 'source': str(release / 'apiserver.json'),
                'target': '/run/qs-server-messaging/apiserver.json', 'read_only': True}]
    volumes += [{'type': 'bind', 'source': str(source), 'target': target, 'read_only': readonly}
                for source, target, readonly in mounts]
    return {'services': {'qs-apiserver': {'image': image,
            'command': ['--config=/run/qs-server-messaging/apiserver.json'], 'volumes': volumes}}}


def write_new(path, raw, mode=0o640):
    with path.open('xb') as stream:
        stream.write(raw)
        stream.flush()
        os.fsync(stream.fileno())
    path.chmod(mode)


def encoded(value):
    return (json.dumps(value, sort_keys=True, indent=2) + '\n').encode()


def prepare(image, base, uid, gid, binding_root=BINDING_ROOT, release_root=RELEASE_ROOT):
    require(binding_root.is_dir() and not binding_root.is_symlink())
    source = offline(image, ['--source-sha']).decode().strip()
    require(SHA.fullmatch(source))  # Old images cannot pass the MQ ownership guard.
    image_info = decoded(command(['docker', 'image', 'inspect', image]))
    require(len(image_info) == 1)
    image_id = image_info[0]['Id']
    require(re.fullmatch(r'sha256:[0-9a-f]{64}', image_id))
    release_root.mkdir(parents=True, exist_ok=True, mode=0o750)
    require(not release_root.is_symlink())
    release = release_root / source
    existing = release.exists()
    require(not release.is_symlink())
    binding_path = release / 'binding.json' if existing else binding_root / 'current.json'
    binding_raw = read(binding_path, 16384)
    binding = decoded(binding_raw)
    mounts = key_mounts(binding, binding_root)
    base_raw = read(base, 1 << 20)
    if existing:
        metadata = decoded(read(release / 'metadata.json', 32768))
        require(metadata['source_sha'] == source and metadata['image_id'] == image_id)
        require(metadata['source_config_sha256'] == digest(base_raw))
        require(metadata['binding_file_sha256'] == digest(binding_raw))
        require(metadata['rendered_config_sha256'] == digest(read(release / 'apiserver.json', 1 << 20)))
        require(metadata['compose_sha256'] == digest(read(release / 'compose.json', 32768)))
        require(decoded(read(release / 'compose.json', 32768)) == overlay(release, mounts, image_id))
        check = decoded(offline(image_id, ['--binding=/preflight/binding.json'],
                         [(binding_path, '/preflight/binding.json', True)] + mounts))
        require(all(check[k] == metadata[k] for k in
                    ('source_sha', 'binding_revision', 'binding_sha256', 'public_key_fingerprints')))
    else:
        with tempfile.TemporaryDirectory(prefix='.mq-preflight-', dir=release_root) as temporary:
            staging = Path(temporary)
            staging.chmod(0o750)
            os.chown(staging, uid, gid)
            local_binding = staging / 'binding.json'
            write_new(local_binding, binding_raw)
            os.chown(local_binding, uid, gid)
            metadata = decoded(offline(image_id,
                ['--binding=/preflight/binding.json', '--base-config=/preflight/source.yaml',
                 '--output-config=/output/apiserver.json'],
                [(local_binding, '/preflight/binding.json', True),
                 (base, '/preflight/source.yaml', True), (staging, '/output', False)] + mounts))
            require(metadata['source_sha'] == source and metadata['offline'] is True)
            require(metadata['rendered_config_sha256'] == digest(read(staging / 'apiserver.json', 1 << 20)))
            composition = encoded(overlay(release, mounts, image_id))
            metadata.update(image_id=image_id, source_config_sha256=digest(base_raw),
                            binding_file_sha256=digest(binding_raw), compose_sha256=digest(composition))
            write_new(staging / 'compose.json', composition)
            write_new(staging / 'metadata.json', encoded(metadata))
            for path in staging.iterdir():
                os.chown(path, uid, gid)
            # Lock is held by remote-deploy; rename creates the permanent rollback asset.
            require(not release.exists())
            os.rename(staging, release)
            Path(temporary).mkdir()  # TemporaryDirectory cleans only this now-empty directory.
    retained = list(release_root.iterdir())
    require(len(retained) <= 128)
    protected = {image_id}
    for retained_release in retained:
        if SHA.fullmatch(retained_release.name):
            require(retained_release.is_dir() and not retained_release.is_symlink())
            retained_id = decoded(read(retained_release / 'metadata.json', 32768))['image_id']
            require(re.fullmatch(r'sha256:[0-9a-f]{64}', retained_id))
            protected.add(retained_id)
    return {'source_sha': source, 'image_id': image_id, 'protected_image_ids': sorted(protected),
            'compose_file': str(release / 'compose.json'), 'preflight': 'passed',
            'binding_sha256': metadata['binding_sha256']}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--image', required=True)
    parser.add_argument('--base-config', type=Path, required=True)
    parser.add_argument('--uid', type=int, required=True)
    parser.add_argument('--gid', type=int, required=True)
    args = parser.parse_args()
    require(args.uid > 0 and args.gid > 0)
    result = prepare(args.image, args.base_config, args.uid, args.gid)
    print(json.dumps(result, sort_keys=True))


if __name__ == '__main__':
    try:
        main()
    except Exception:
        raise SystemExit('MQ release preflight failed; material withheld') from None
