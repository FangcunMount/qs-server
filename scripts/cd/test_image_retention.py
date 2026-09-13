import contextlib
import importlib.util
import io
import json
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

spec = importlib.util.spec_from_file_location(
    "retention", Path(__file__).with_name("image-retention.py")
)
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


def image(n, extra=()):
    return {
        "Id": "sha256:" + f"{n:064x}",
        "Created": f"2026-09-{n:02d}",
        "RepoTags": [f"qs-ai:v{n}", *extra],
    }


class RetentionTests(unittest.TestCase):
    def setUp(self):
        self.images = [image(n) for n in range(1, 7)]

    def select(self, current=4, refs=(), state=None):
        return m.select(
            self.images, set(refs), "qs-ai", image(current)["Id"], f"qs-ai:v{current}", state or {}
        )

    def test_success_order_beats_new_failed_builds(self):
        state = {"successful": [image(2)["Id"], image(1)["Id"]], "bootstrap": []}
        new, keep, delete = self.select(state=state)
        self.assertEqual(new["successful"], [image(n)["Id"] for n in (4, 2, 1)])
        self.assertEqual({x["Id"] for x in delete}, {image(n)["Id"] for n in (3, 5, 6)})

    def test_bootstrap_does_not_claim_old_images_succeeded(self):
        new, keep, _ = self.select()
        self.assertEqual(new["successful"], [image(4)["Id"]])
        self.assertEqual(set(new["bootstrap"]), {image(n)["Id"] for n in (4, 5, 6)})

    def test_bootstrap_expires_after_three_successes(self):
        state, _, _ = self.select()
        state, _, _ = self.select(current=2, state=state)
        state, keep, _ = self.select(current=1, state=state)
        self.assertEqual(state["bootstrap"], [])
        self.assertEqual(keep, {image(n)["Id"] for n in (1, 2, 4)})

    def test_rollback_deduplicates_and_preserves_prior_current(self):
        new, _, _ = self.select(
            current=1, state={"successful": [image(n)["Id"] for n in (4, 2, 1)], "bootstrap": []}
        )
        self.assertEqual(new["successful"], [image(n)["Id"] for n in (1, 4, 2)])

    def test_stopped_container_reference_protected(self):
        _, keep, delete = self.select(refs=[image(1)["Id"]])
        self.assertIn(image(1)["Id"], keep)
        self.assertNotIn(image(1)["Id"], {x["Id"] for x in delete})

    def test_aliases_deduplicate_and_unknown_repository_is_untouched(self):
        self.images[0]["RepoTags"].append(m.ACR + "qs-ai:old")
        self.images[1]["RepoTags"].append("unrelated/tool:old")
        _, _, delete = self.select()
        self.assertIn(self.images[0], delete)
        self.assertNotIn(self.images[1], delete)

    def test_untagged_is_not_blindly_pruned(self):
        self.images[0]["RepoTags"] = []
        self.assertNotIn(self.images[0], self.select()[2])

    def test_global_lock_contention(self):
        with tempfile.TemporaryDirectory() as directory:
            lockpath = Path(directory) / "lock"
            lockpath.touch()
            with patch.object(m, "LOCK", lockpath), m.transaction(False):
                with self.assertRaises(BlockingIOError):
                    with m.transaction(False):
                        self.fail("second process must not enter")

    def execute(self, apply=True, unhealthy=False, fail_delete=False, change=False):
        args = SimpleNamespace(
            service="qs-ai",
            image_ref="qs-ai:v4",
            apply=apply,
            deployment_locked=True,
            protect_image_id=[],
        )
        cs = [
            (
                "container",
                image(4)["Id"],
                "running",
                "unhealthy" if unhealthy else "healthy",
                "start",
                0,
            )
        ]
        self.commands = []

        def docker(*args):
            self.commands.append(args)
            if args[:2] == ("image", "inspect"):
                for x in self.images:
                    if args[2] == x["Id"] or args[2] in x["RepoTags"]:
                        return json.dumps([x])
                raise RuntimeError("missing image")
            if args[:3] == ("image", "rm", "--no-prune"):
                if fail_delete:
                    raise RuntimeError("Docker conflict")
                for x in list(self.images):
                    if args[3] in x["RepoTags"]:
                        x["RepoTags"].remove(args[3])
                        if not x["RepoTags"]:
                            self.images.remove(x)
                return ""
            self.fail("Unexpected Docker mutation: " + str(args))

        with (
            patch.object(m, "docker", docker),
            patch.object(m, "inventory", lambda: self.images.copy()),
            patch.object(m, "containers", side_effect=[cs, []] if change else lambda: cs),
            patch.object(m.socket, "gethostname", return_value="serverA"),
            contextlib.redirect_stdout(io.StringIO()),
        ):
            m.execute(args)

    def test_dry_run_has_no_state_or_deletes(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(m, "STATE", Path(directory)):
            self.execute(apply=False)
            self.assertEqual(list(Path(directory).iterdir()), [])
            self.assertFalse(any("rm" in c for c in self.commands))

    def test_success_records_and_deletes_only_old_scoped_tags(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(m, "STATE", Path(directory)):
            self.execute()
            self.assertEqual(len(self.images), 3)
            audit = json.loads((Path(directory) / "qs-ai-last-cleanup.json").read_text())
            self.assertEqual(audit["status"], "success")
            self.assertEqual(set(audit["removed_tags"]), {"qs-ai:v1", "qs-ai:v2", "qs-ai:v3"})

    def test_unhealthy_release_never_updates_history(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(m, "STATE", Path(directory)):
            with self.assertRaises(ValueError):
                self.execute(unhealthy=True)
            self.assertEqual(list(Path(directory).iterdir()), [])

    def test_delete_failure_records_success_but_reports_cleanup_failure(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(m, "STATE", Path(directory)):
            with self.assertRaises(RuntimeError):
                self.execute(fail_delete=True)
            self.assertEqual(
                json.loads((Path(directory) / "qs-ai.json").read_text())["successful"],
                [image(4)["Id"]],
            )
            self.assertEqual(
                json.loads((Path(directory) / "qs-ai-last-cleanup.json").read_text())["status"],
                "failed",
            )

    def test_concurrent_container_change_stops_deletion(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(m, "STATE", Path(directory)):
            with self.assertRaises(RuntimeError):
                self.execute(change=True)
            self.assertFalse(any("rm" in c for c in self.commands))

    def test_first_run_preserves_actual_previous_even_if_many_newer_failed_builds(self):
        state, keep, _ = m.select(
            self.images, set(), "qs-ai", image(4)["Id"], "qs-ai:v4", {}, [image(1)["Id"]]
        )
        self.assertIn(image(1)["Id"], keep)
        self.assertIn(image(1)["Id"], state["bootstrap"])

    def test_delivery_includes_helper_and_success_gate(self):
        root = Path(__file__).resolve().parents[2]
        remote = root / "scripts/cd/remote-deploy.sh"
        if remote.exists():
            text = remote.read_text()
            self.assertIn("acquire_image_deploy_lock", text)
            self.assertIn("retain_successful_image", text)
            if "verify_running_image" in text:
                self.assertLess(
                    text.rindex("\nverify_running_image\n"),
                    text.rindex("\nretain_successful_image"),
                )
            elif "verify_service" in text:
                self.assertLess(
                    text.rindex("\nverify_service\n"), text.rindex("\nretain_successful_image")
                )
            else:
                self.assertLess(
                    text.rindex("\nverify_health\n"), text.rindex("\nretain_successful_image")
                )
            package = root / "scripts/cd/prepare-package.sh"
            delivery = (
                package if package.exists() else root / "scripts/cd/runner-upload-and-deploy.sh"
            )
            self.assertIn("image-retention.py", delivery.read_text())
            self.assertIn("image-retention.sh", delivery.read_text())
        else:
            self.assertIn("remote_retention", (root / "scripts/cd/deploy.py").read_text())
            self.assertIn(
                "retain_successful_image(release)", (root / "deploy/serverA/deploy.py").read_text()
            )

    def test_shell_cleanup_failure_is_warning_not_failed_deploy(self):
        helper = Path(__file__).with_name("image-retention.sh")
        if not helper.exists():
            self.skipTest("Python deploy has a separate nonfatal cleanup test")
        import subprocess

        result = subprocess.run(
            [
                "bash",
                "-c",
                'set -e; source "$1"; SUDO=false; SCRIPT_DIR=/unused; IMAGE_NAME=iam; '
                "RETENTION_PREVIOUS_IDS=(); retain_successful_image repo:tag; echo deployed",
                "test",
                str(helper),
            ],
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0)
        self.assertIn("::warning::", result.stderr)
        self.assertIn("deployed", result.stdout)


if __name__ == "__main__":
    unittest.main()
