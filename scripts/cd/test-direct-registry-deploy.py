"""Exercise production registry selection with a fake Docker boundary."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SOURCE = (ROOT / 'scripts/cd/remote-deploy.sh').read_text()
FUNCTIONS = SOURCE[SOURCE.index('docker_login_with_token()'):SOURCE.index('prepare_dirs_and_backup()')]
FUNCTIONS += SOURCE[SOURCE.index('write_compose_image_env()'):SOURCE.index('load_image_from_tarball()')]
FUNCTIONS += SOURCE[SOURCE.index('select_image_from_registry()'):SOURCE.index('select_image()')]


class RegistryDeployTests(unittest.TestCase):
    def run_selection(self, failure=''):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            docker = root / 'docker'
            docker.write_text('''#!/bin/bash
set -eu
printf '%s\\n' "$*" >> "$CALL_LOG"
if [ "${1:-}" = --config ]; then
  printf '%s' "$2" > "$AUTH_PATH"
  shift 2
fi
case "$1" in
  login) cat > /dev/null; printf '{}' > "$(cat "$AUTH_PATH")/config.json" ;;
esac
[ "$1" != "${FAIL_COMMAND:-}" ] || exit 41
''')
            docker.chmod(0o700)
            env = dict(os.environ, PATH=f'{root}:{os.environ["PATH"]}',
                       SUDO='', DEPLOY_PULL_REGISTRY='acr',
                       ALIYUN_ACR_REGISTRY='acr.example', ALIYUN_ACR_NAMESPACE='qs',
                       ALIYUN_ACR_USERNAME='reader', ALIYUN_ACR_PASSWORD='fake-secret',
                       DOCKER_REGISTRY='ghcr.io', DOCKER_REPOSITORY='fangcunmount',
                       IMAGE_NAME='qs-apiserver', IMAGE_TAG='a' * 40,
                       IMAGE_ENV_VAR='DOCKER_IMAGE_QS_APISERVER', DEPLOY_TMP=str(root),
                       CALL_LOG=str(root / 'calls'), AUTH_PATH=str(root / 'auth'),
                       FAIL_COMMAND=failure)
            result = subprocess.run(['bash', '-c', 'set -Eeuo pipefail\n' + FUNCTIONS +
                                     '\nselect_image_from_registry'], env=env,
                                    capture_output=True, text=True, timeout=10)
            calls = (root / 'calls').read_text()
            self.assertNotIn('fake-secret', calls + result.stdout + result.stderr)
            self.assertFalse(Path((root / 'auth').read_text()).exists())
            composed = root / 'compose-image.env'
            return result, calls, composed.read_text() if composed.exists() else None

    def test_direct_pull_keeps_canonical_compose_identity_and_cleans_credentials(self):
        result, calls, composed = self.run_selection()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('pull --platform linux/amd64 acr.example/qs/qs-apiserver:' + 'a' * 40, calls)
        self.assertIn('tag acr.example/qs/qs-apiserver:', calls)
        self.assertEqual(composed, 'DOCKER_IMAGE_QS_APISERVER=ghcr.io/fangcunmount/qs-apiserver:' + 'a' * 40 + '\n')

    def test_login_pull_tag_or_inspect_failure_never_selects_image(self):
        for failure in ('login', 'pull', 'tag', 'image'):
            with self.subTest(failure=failure):
                result, calls, composed = self.run_selection(failure)
                self.assertEqual(result.returncode, 41, result.stderr)
                self.assertIsNone(composed)
                if failure in ('login', 'pull'):
                    self.assertNotIn('\ntag ', calls)


class UploadContractTests(unittest.TestCase):
    def test_registry_upload_needs_only_config_package_and_keeps_ssh_entry(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            log = root / 'calls'
            for tool in ('ssh', 'scp'):
                executable = root / tool
                executable.write_text("#!/bin/bash\nset -eu\nprintf '%s %s\\n' " + tool + " \"$*\" >> \"$CALL_LOG\"\n" +
                                      ("if [[ \"$*\" == *'-G '* ]]; then echo 'hostname 192.0.2.1'; fi\n" if tool == 'ssh' else ''))
                executable.chmod(0o700)
            (root / 'package.tar.gz').write_bytes(b'config-package')
            env = dict(os.environ, PATH=f'{root}:{os.environ["PATH"]}', CALL_LOG=str(log),
                       SERVICE='apiserver', IMAGE_TAG='a' * 40, CD_SOURCE_SHA='a' * 40,
                       CD_RUN_ID='123', CD_RUN_ATTEMPT='1', WWW_UID='2000', WWW_GID='2000',
                       RUNNER_SSH_ALIAS='production', DOCKER_REGISTRY='ghcr.io',
                       DOCKER_REPOSITORY='fangcunmount', DEPLOY_IMAGE_SOURCE='registry',
                       DEPLOY_PACKAGE=str(root / 'package.tar.gz'))
            result = subprocess.run(['bash', str(ROOT / 'scripts/cd/runner-upload-and-deploy.sh')],
                                    env=env, cwd=root, capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stderr)
            calls = log.read_text()
            self.assertIn('scp ' + str(root / 'package.tar.gz'), calls)
            self.assertIn('bash /tmp/qs-cd-bootstrap-apiserver-', calls)
            self.assertNotIn('deploy-image-', calls)
            # The tarball recovery path still refuses a missing image package.
            result = subprocess.run(['bash', str(ROOT / 'scripts/cd/runner-upload-and-deploy.sh')],
                                    env=dict(env, DEPLOY_IMAGE_SOURCE='tarball'), cwd=root,
                                    capture_output=True, text=True, timeout=10)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('Missing file:', result.stderr)


if __name__ == '__main__':
    unittest.main()
