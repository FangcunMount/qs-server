"""Exercise the real shell bootstrap with a restricted sudo command boundary.

chown is simulated (no root required); file/link semantics and held locks are real.
"""
import fcntl
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


HELPER = Path(__file__).with_name("image-retention.sh")


class DeploymentLockTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.package = self.root / "package"
        self.package.mkdir()
        self.directory = self.root / "retention"
        self.lock = self.directory / "deploy.lock"
        self.log = self.root / "commands"
        self.sudo = self.root / "sudo"
        self.sudo.write_text('''#!/usr/bin/env bash
set -eu
printf '%s\\n' "$*" >> "$COMMAND_LOG"
case "$1" in
  mkdir|chmod) exec "$@" ;;
  chown)
    [ "$2" = root:root ] || exit 91
    [ "${FAIL_OWNER:-0}" = 0 ] || exit 92
    ;;
  ln)
    [ "${FAIL_LINK:-0}" = 0 ] || exit 93
    if [ "${RACE_WINNER:-0}" = 1 ]; then
      printf winner > "${@: -1}"
      exit 1
    fi
    exec "$@"
    ;;
  *) echo "sudo command denied" >&2; exit 94 ;;
esac
''')
        self.sudo.chmod(0o700)

    def bootstrap(self, **overrides):
        env = dict(os.environ, SUDO=str(self.sudo), SCRIPT_DIR=str(self.package),
                   COMMAND_LOG=str(self.log), **overrides)
        return subprocess.run(
            ["bash", "-c", 'set -eu; source "$1"; initialize_image_deploy_lock "$2"',
             "bootstrap", str(HELPER), str(self.directory)],
            env=env, text=True, capture_output=True, timeout=10,
        )

    def assert_cleaned(self):
        self.assertEqual(list(self.package.iterdir()), [])

    def test_initialization_uses_permitted_commands(self):
        result = self.bootstrap()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.lock.stat().st_mode & 0o777, 0o666)
        self.assertEqual(self.directory.stat().st_mode & 0o777, 0o755)
        commands = self.log.read_text().splitlines()
        self.assertEqual([line.split()[0] for line in commands],
                         ["mkdir", "chmod", "chown", "chmod", "ln"])
        self.assert_cleaned()

    def test_existing_held_lock_is_not_replaced_or_truncated(self):
        self.directory.mkdir()
        self.lock.write_text("existing")
        with self.lock.open("r+") as held:
            fcntl.flock(held, fcntl.LOCK_EX | fcntl.LOCK_NB)
            inode = self.lock.stat().st_ino
            result = self.bootstrap()
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(self.lock.stat().st_ino, inode)
            self.assertEqual(self.lock.read_text(), "existing")
            with self.lock.open("r+") as contender:
                with self.assertRaises(BlockingIOError):
                    fcntl.flock(contender, fcntl.LOCK_EX | fcntl.LOCK_NB)
        self.assert_cleaned()

    def test_concurrent_winner_is_preserved(self):
        result = self.bootstrap(RACE_WINNER="1")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.lock.read_text(), "winner")
        self.assert_cleaned()

    def test_link_failure_stops_and_cleans_up(self):
        result = self.bootstrap(FAIL_LINK="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.lock.exists())
        self.assert_cleaned()

    def test_owner_failure_stops_before_publishing(self):
        result = self.bootstrap(FAIL_OWNER="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.lock.exists())
        self.assertNotIn("ln ", self.log.read_text())
        self.assert_cleaned()

    def test_nonregular_or_symlink_lock_is_rejected(self):
        self.directory.mkdir()
        self.lock.mkdir()
        self.assertNotEqual(self.bootstrap().returncode, 0)
        self.lock.rmdir()
        target = self.root / "target"
        target.write_text("protected")
        self.lock.symlink_to(target)
        self.assertNotEqual(self.bootstrap().returncode, 0)
        self.assertEqual(target.read_text(), "protected")
        self.assert_cleaned()


if __name__ == "__main__":
    unittest.main()
