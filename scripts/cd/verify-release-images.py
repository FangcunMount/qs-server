"""Verify a fixed set of prepared registry images without building or publishing."""
import json
import os
from pathlib import Path
import re
import subprocess

ALLOWED = {'qs-apiserver', 'qs-worker', 'qs-collection-server', 'iam', 'qs-ai', 'qs-operating-system'}


def verify(images, registry, namespace, run=subprocess.check_output):
    if set(images) != ALLOWED:
        raise ValueError('Expected exactly six release images')
    for name, expected in images.items():
        if set(expected) != {'sha', 'digest', 'config'} or not re.fullmatch('[0-9a-f]{40}', expected['sha']) or any(not re.fullmatch('sha256:[0-9a-f]{64}', expected[k]) for k in ('digest', 'config')):
            raise ValueError('Invalid immutable image identity')
    rows = []
    for name, expected in sorted(images.items()):
        reference = registry + '/' + namespace + '/' + name + '@' + expected['digest']
        run(['docker', 'pull', '--platform', 'linux/amd64', reference], stderr=subprocess.DEVNULL, timeout=300)
        image = json.loads(run(['docker', 'image', 'inspect', reference], stderr=subprocess.DEVNULL, timeout=30))[0]
        if image['Id'] != expected['config'] or image['Os'] != 'linux' or image['Architecture'] != 'amd64':
            raise ValueError('Registry image configuration or platform mismatch')
        revision = (image.get('Config', {}).get('Labels') or {}).get('org.opencontainers.image.revision')
        if revision is not None and revision != expected['sha']:
            raise ValueError('Registry image revision mismatch')
        rows.append(dict(service=name, source_sha=expected['sha'], digest=expected['digest'], config_id=image['Id'],
                         platform='linux/amd64', revision_label_checked=revision is not None))
    return dict(images=rows, verified=True, deployment_verified=False)


if __name__ == '__main__':
    try:
        result = verify(json.loads(os.environ['CANDIDATE_IMAGES']), os.environ['ALIYUN_ACR_REGISTRY'], os.environ['ALIYUN_ACR_NAMESPACE'])
        Path('candidate-image-verification.json').write_text(json.dumps(result, indent=2) + '\n')
        print('Six immutable candidate images verified; no production deployment')
    except Exception:
        raise SystemExit('Candidate image verification failed; image identities remain unverified') from None
