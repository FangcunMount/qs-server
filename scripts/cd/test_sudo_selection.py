#!/usr/bin/env python3
"""Exercise deployment sudo choice without contacting a deployment host."""

from pathlib import Path
import os
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("sudo-selection.sh")


class SudoSelectionTest(unittest.TestCase):
    def run_selection(
        self, password: str | None, allow_nopasswd: bool, allow_retention_nopasswd: bool
    ) -> subprocess.CompletedProcess[str]:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            sudo = root / "sudo"
            sudo.write_text(
                "#!/usr/bin/env bash\n"
                "if [[ $1 == -S ]]; then\n"
                "  read -r supplied\n"
                "  [[ $supplied == expected-password ]] || exit 1\n"
                "  shift\n"
                "  [[ $1 == -v || ( $1 == python3 && $2 == retention.py ) ]]\n"
                "elif [[ $1 == -n && $2 == true ]]; then\n"
                "  [[ ${ALLOW_NOPASSWD:-} == yes ]]\n"
                "elif [[ $1 == python3 && $2 == retention.py ]]; then\n"
                "  [[ ${ALLOW_RETENTION_NOPASSWD:-} == yes ]]\n"
                "else\n"
                "  exit 1\n"
                "fi\n"
            )
            sudo.chmod(0o755)
            env = os.environ.copy()
            env["PATH"] = f"{root}:{env['PATH']}"
            env["ALLOW_NOPASSWD"] = "yes" if allow_nopasswd else "no"
            env["ALLOW_RETENTION_NOPASSWD"] = "yes" if allow_retention_nopasswd else "no"
            if password is None:
                env.pop("SUDO_PASSWORD", None)
            else:
                env["SUDO_PASSWORD"] = password
            return subprocess.run(
                ["bash", "-c", 'set -e; . "$1"; select_deploy_sudo; printf "mode=%s\\n" "$SUDO"; $SUDO python3 retention.py', "bash", str(SCRIPT)],
                env=env,
                capture_output=True,
                text=True,
                check=False,
            )

    def test_password_is_used_even_if_cached_probe_would_succeed(self) -> None:
        result = self.run_selection("expected-password", allow_nopasswd=True, allow_retention_nopasswd=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("mode=sudo_pw", result.stdout)

    def test_without_password_uses_passwordless_route(self) -> None:
        result = self.run_selection(None, allow_nopasswd=True, allow_retention_nopasswd=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("mode=sudo", result.stdout)

    def test_missing_both_credential_paths_fails_before_deploy(self) -> None:
        result = self.run_selection(None, allow_nopasswd=False, allow_retention_nopasswd=False)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("sudo needs password", result.stderr)


if __name__ == "__main__":
    unittest.main()
