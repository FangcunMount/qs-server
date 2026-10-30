"""Execute the actual frozen-revision gate and config-sync functions offline."""
import os
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest

ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = (ROOT / '.github/workflows/cd.yml').read_text()
GATE = textwrap.dedent(WORKFLOW.split('      - name: Validate frozen service revision\n', 1)[1]
    .split('        run: |\n', 1)[1].split('\n      - name:', 1)[0])
REMOTE = (ROOT / 'scripts/cd/remote-deploy.sh').read_text()
SYNC = REMOTE[REMOTE.index('sync_configs() {'):REMOTE.index('\nensure_networks() {')]


class FrozenRevision(unittest.TestCase):
    def execute(self, **overrides):
        with tempfile.TemporaryDirectory() as temporary:
            binpath = Path(temporary)
            for name, script in {
                'git': '#!/bin/sh\nexit "${ANCESTOR_STATUS:-0}"\n',
                'gh': '#!/bin/sh\nprintf "%s\\n" "${CI_RESULT:-true}"\n',
            }.items():
                path = binpath / name
                path.write_text(script)
                path.chmod(0o700)
            env = dict(os.environ, PATH=str(binpath)+os.pathsep+os.environ['PATH'],
                       REQUESTED_REVISION='a'*40, GITHUB_EVENT_NAME='workflow_dispatch',
                       GITHUB_REF='refs/heads/main', REUSE_IMAGES='true',
                       DEPLOY_OPERATION='deploy', GITHUB_REPOSITORY='example/qs',
                       ANCESTOR_STATUS='0', CI_RESULT='true')
            env.update(overrides)
            return subprocess.run(['bash', '-c', GATE], env=env, capture_output=True)

    def test_frozen_successful_main_ancestor(self):
        self.assertEqual(self.execute().returncode, 0)

    def test_rejects_other_branch(self):
        self.assertNotEqual(self.execute(GITHUB_REF='refs/heads/example').returncode, 0)

    def test_rejects_automatic_override(self):
        self.assertNotEqual(self.execute(GITHUB_EVENT_NAME='workflow_run').returncode, 0)

    def test_rejects_nonreuse(self):
        self.assertNotEqual(self.execute(REUSE_IMAGES='false').returncode, 0)

    def test_rejects_build_override(self):
        self.assertNotEqual(self.execute(DEPLOY_OPERATION='build').returncode, 0)

    def test_rejects_short_revision(self):
        self.assertNotEqual(self.execute(REQUESTED_REVISION='a'*9).returncode, 0)

    def test_rejects_nonancestor(self):
        self.assertNotEqual(self.execute(ANCESTOR_STATUS='1').returncode, 0)

    def test_rejects_missing_successful_source_ci(self):
        self.assertNotEqual(self.execute(CI_RESULT='false').returncode, 0)

    def test_existing_automatic_path_does_not_require_override(self):
        self.assertEqual(self.execute(REQUESTED_REVISION='', GITHUB_EVENT_NAME='workflow_run',
                                      ANCESTOR_STATUS='1', CI_RESULT='false').returncode, 0)


class ConfigSync(unittest.TestCase):
    def execute(self, service):
        # A Collection host log directory deliberately does not exist. The
        # actual sync function must work without touching it; no host IO occurs.
        script = '''set -Eeuo pipefail
mock_sudo() {
  if [[ "$*" == *'/data/logs/'* ]]; then
    [[ "$SERVICE" != collection ]] || return 72
    echo log_ownership_updated
  fi
}
SUDO=mock_sudo
APP_UID=2000 APP_GID=2000 DEPLOY_TMP=/unused CONTAINER_NAME=qs-test
''' + SYNC + '\nsync_configs\n'
        return subprocess.run(['bash', '-c', script],
                              env=dict(os.environ, SERVICE=service), capture_output=True)

    def test_collection_without_host_log_directory(self):
        result = self.execute('collection')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn(b'log_ownership_updated', result.stdout)

    def test_worker_retains_host_log_ownership(self):
        result = self.execute('worker')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(b'log_ownership_updated', result.stdout)


if __name__ == '__main__':
    unittest.main()
