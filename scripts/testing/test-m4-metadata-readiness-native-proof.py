import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("m4-metadata-readiness-native-proof.sh").resolve()


class NativeSelectorProofTests(unittest.TestCase):
    def run_fixture(self, mode, events, required=True, go_exit=0):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            marker = root / "go-invoked"
            fake_go = root / "go"
            fake_go.write_text(
                "#!/usr/bin/env python3\nfrom pathlib import Path\n"
                + "Path(" + repr(str(marker)) + ").write_text('owned fixture')\n"
                + "print(" + repr("\n".join(json.dumps(event) for event in events)) + ")\n"
                + "raise SystemExit(" + str(go_exit) + ")\n"
            )
            fake_go.chmod(0o700)
            env = {"PATH": str(root) + ":" + os.environ["PATH"],
                   "QS_METADATA_PROFILE_NATIVE_REQUIRED": "1" if required else "",
                   "QS_MONGO_METADATA_NATIVE_REQUIRED": "1" if required else ""}
            result = subprocess.run(["bash", str(SCRIPT), mode], env=env, capture_output=True, text=True)
            return result, marker.exists()

    def test_required_fixture_missing_stops_before_go(self):
        for mode in ("mysql", "mongodb"):
            with self.subTest(mode=mode):
                result, called = self.run_fixture(mode, [], required=False)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(called)

    def test_successful_go_exit_without_selected_tests_is_failure(self):
        result, _ = self.run_fixture("mysql", [{"Action": "pass"}])
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("RUN/PASS; zero SKIP", result.stdout)

    def test_every_mysql_selector_must_run_and_pass(self):
        names = ("TestMetadataProfileNativeOwnedMySQL", "TestMetadataReadinessNativeOwnedMySQL")
        events = [{"Action": action, "Test": name} for name in names for action in ("run", "pass")]
        result, _ = self.run_fixture("mysql", events)
        self.assertEqual(result.returncode, 0, result.stderr)
        missing, _ = self.run_fixture("mysql", events[:2])
        self.assertNotEqual(missing.returncode, 0)

    def test_nested_skip_or_failure_cannot_pass(self):
        name = "TestMongoMetadataProfileNativeOwnedReplicaSet"
        for action in ("skip", "fail"):
            with self.subTest(action=action):
                result, _ = self.run_fixture("mongodb", [
                    {"Action": "run", "Test": name},
                    {"Action": action, "Test": name + "/required_case"},
                    {"Action": "pass", "Test": name},
                ])
                self.assertNotEqual(result.returncode, 0)

    def test_go_failure_is_not_overridden_by_matching_events(self):
        name = "TestMongoMetadataProfileNativeOwnedReplicaSet"
        result, _ = self.run_fixture("mongodb", [
            {"Action": "run", "Test": name}, {"Action": "pass", "Test": name},
        ], go_exit=1)
        self.assertNotEqual(result.returncode, 0)


if __name__ == "__main__":
    unittest.main()
