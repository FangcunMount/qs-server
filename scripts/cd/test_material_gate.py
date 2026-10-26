"""Host call-order tests with a controlled external material verifier boundary."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


def host_blocks():
    text=(ROOT/'scripts/cd/remote-deploy.sh').read_text()
    main=text[text.index('acquire_image_deploy_lock\n'):]
    if (ROOT/'scripts/cd/render-messaging.py').exists():
        start=main.index('ROCKETMQ_BINDING_REVISION=$(sed')
        return {'iam':main[start:main.index('\nsync_package\n',start)]}
    start=main.index('if [ "$SERVICE" = worker ]; then')
    api=main.index('  mq_material_revision=')
    return {'worker':main[start:main.index('\n# Verify the public CA',start)],
            'qs-api':main[api:main.index('  while IFS= read -r mq_retained_image',api)]}


class MaterialGateTests(unittest.TestCase):
    def exercise(self,role,block,*,refused=False,missing=False):
        with tempfile.TemporaryDirectory() as raw:
            root=Path(raw);(root/'configs/env').mkdir(parents=True)
            (root/'configs/env/config.prod.env').write_text('' if missing else
                'ROCKETMQ_BINDING_REVISION=m4-host-test\nROCKETMQ_WORKER_BINDING_REVISION=m4-host-test\n')
            (root/'ai-mq-preflight.json').write_text(json.dumps({} if missing else {'binding_revision':'m4-host-test'}))
            marker=root/'would-sync';calls=root/'calls'
            shell='''set -Eeuo pipefail
m4_material_preflight() { printf '%s:%s\\n' "$1" "$2" >> "$CALLS"; return "$REFUSED"; }
'''+block+'\nprintf synced > "$MARKER"\n'
            result=subprocess.run(['bash','-c',shell],env={**os.environ,'DEPLOY_TMP':str(root),
                'SERVICE':'worker' if role=='worker' else 'apiserver','REFUSED':'1' if refused else '0',
                'MARKER':str(marker),'CALLS':str(calls)},capture_output=True,text=True,timeout=10)
            return result.returncode,marker.exists(),calls.read_text() if calls.exists() else ''

    def test_external_material_refusal_prevents_configuration_sync(self):
        for role,block in host_blocks().items():
            with self.subTest(role=role):
                code,marker,calls=self.exercise(role,block,refused=True)
                self.assertNotEqual(code,0);self.assertFalse(marker)
                self.assertEqual(calls,role+':m4-host-test\n')

    def test_verified_revision_is_required_before_sync(self):
        for role,block in host_blocks().items():
            with self.subTest(role=role):
                code,marker,calls=self.exercise(role,block)
                self.assertEqual(code,0);self.assertTrue(marker)
                self.assertEqual(calls,role+':m4-host-test\n')

    def test_missing_revision_cannot_silently_skip_material_verification(self):
        for role,block in host_blocks().items():
            with self.subTest(role=role):
                code,marker,calls=self.exercise(role,block,missing=True)
                self.assertNotEqual(code,0);self.assertFalse(marker);self.assertEqual(calls,'')

    def test_guard_is_packaged_and_called_in_the_real_sync_path(self):
        prepare=(ROOT/'scripts/cd/prepare-package.sh').read_text()
        remote=(ROOT/'scripts/cd/remote-deploy.sh').read_text()
        self.assertIn('scripts/cd/material-guard.sh',prepare)
        self.assertIn('. "$SCRIPT_DIR/material-guard.sh"',remote)
        main=remote[remote.index('acquire_image_deploy_lock\n'):]
        sync='\nsync_package\n' if 'iam' in host_blocks() else '\nsync_configs\n'
        for role in host_blocks():
            self.assertLess(main.index('m4_material_preflight '+role+' '),main.index(sync))


if __name__=='__main__':unittest.main()
