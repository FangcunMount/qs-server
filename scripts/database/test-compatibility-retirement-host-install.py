import base64
import hashlib
import importlib.util
import json
import multiprocessing
import os
from pathlib import Path
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

SOURCE = Path(__file__).with_name("compatibility-retirement-host-install.py")
spec = importlib.util.spec_from_file_location("host_install_owned", SOURCE)
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

REQUIRED = {"authenticationmethods": "publickey", "authorizedkeyscommand": "none", "trustedusercakeys": "none",
            "passwordauthentication": "no", "kbdinteractiveauthentication": "no", "hostbasedauthentication": "no",
            "gssapiauthentication": "no", "permituserenvironment": "no", "permituserrc": "no", "disableforwarding": "yes",
            "permittty": "no", "forcecommand": "none", "pubkeyauthentication": "yes", "strictmodes": "yes", "acceptenv": "LANG LC_*"}


class Fixture:
    def __init__(self, directory, originals=True):
        self.anchor = Path(directory)
        self.anchor.chmod(0o700)
        self.root = self.anchor / "namespace"
        self.root.mkdir(mode=0o700)
        for name in ("inputs", "bin", "state"):
            (self.root / name).mkdir(mode=0o700)
        self.sshd = self.root / "inputs" / "sshd"
        self.calls = self.root / "sshd-calls.private"
        self.write(self.calls, b"", 0o600)
        body = ("#!" + sys.executable + "\nimport sys,json,pathlib\n"
                f"root=pathlib.Path({str(self.root)!r})\n"
                f"calls=pathlib.Path({str(self.calls)!r})\n"
                "with calls.open('a') as f:f.write(json.dumps(sys.argv[1:])+'\\n')\n"
                "if '-t' in sys.argv:sys.exit(0)\n"
                f"settings={REQUIRED!r}\n"
                "settings['authorizedkeysfile']=str(root/'authorized_keys')\n"
                "for k,v in settings.items():print(k+' '+v)\n")
        self.sshd.write_text(body)
        self.sshd.chmod(0o555)
        wire = len(b"ssh-ed25519").to_bytes(4, "big") + b"ssh-ed25519" + (32).to_bytes(4, "big") + b"x" * 32
        fp = "SHA256:" + base64.b64encode(hashlib.sha256(wire).digest()).decode().rstrip("=")
        policy = {"source_sha": "a" * 40, "operation_id": "123-1", "run_id": "456", "run_attempt": "1",
                  "request_sha256": "b" * 64, "ssh_key_fingerprint": fp}
        payloads = {"binary": b"approved fixed compiled-binary fixture bytes", "policy": m.canonical(policy), "sshd_config": b"approved isolated sshd fixture config\n"}
        cmd = (f"{self.root}/bin/qs-retirement-fence --policy {self.root}/policy.json --policy-sha256 {m.digest(payloads['policy'])} "
               f"--authenticated-key {fp} --github-token-file {self.root}/readonly-github-token")
        payloads["keyfile"] = f'restrict,command="{cmd}" ssh-ed25519 {base64.b64encode(wire).decode()}\n'.encode()
        marker = {"protocol": "qs-retirement-owned-install-namespace/v1", "namespace": str(self.root),
                  "source_sha": "a" * 40, "operation_id": "123-1", "run_id": "456-1"}
        self.write(self.root / "namespace.private.json", m.canonical(marker), 0o600)
        assets = {}
        self.originals = {}
        for name, (rel, _) in m.TARGETS.items():
            self.write(self.root / "inputs" / name, payloads[name], 0o600)
            old = ("original " + name).encode() if originals else None
            self.originals[name] = old
            target = self.root / rel
            if old is not None:
                self.write(target, old, 0o640)
                s = target.stat()
                original = {"present": True, "sha256": m.digest(old), "mode": 0o640, "uid": s.st_uid, "gid": s.st_gid}
            else:
                original = {"present": False, "sha256": "", "mode": None, "uid": None, "gid": None}
            assets[name] = {"sha256": m.digest(payloads[name]), "original": original}
        self.plan = {"protocol": m.PROTOCOL, "scope": "isolated_root_namespace", "source_sha": "a" * 40,
                     "operation_id": "123-1", "run_id": "456-1", "request_sha256": "b" * 64,
                     "namespace": str(self.root), "marker_sha256": m.digest(m.canonical(marker)),
                     "sshd_sha256": m.digest(self.sshd.read_bytes()), "assets": assets,
                     "matches": [{"user": "deploy", "host": "runner", "addr": "127.0.0.1", "laddr": "127.0.0.1", "lport": "2222"},
                                 {"user": "deploy", "host": "historical", "addr": "127.0.0.2", "laddr": "127.0.0.1", "lport": "2222"}]}

    def write(self, path, raw, mode):
        if path.exists():
            path.chmod(0o600)
        path.write_bytes(raw)
        path.chmod(mode)

    def run(self, operation="install", fault=None):
        raw = m.canonical(self.plan)
        return m._execute(raw, m.digest(raw), operation, str(self.anchor), os.geteuid(), self.sshd, fault)

    def sshd_body(self, body):
        self.write(self.sshd, ("#!" + sys.executable + "\n" + body).encode(), 0o555)
        self.plan["sshd_sha256"] = m.digest(self.sshd.read_bytes())


class HostInstallTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="qs-host-install-unit-")
        self.addCleanup(self.tmp.cleanup)
        self.f = Fixture(self.tmp.name)

    def rejected(self, category, callable):
        with self.assertRaises(m.Rejected) as result:
            callable()
        self.assertEqual(str(result.exception), category)

    def assert_original(self):
        for name, (rel, _) in m.TARGETS.items():
            target = self.f.root / rel
            self.assertEqual(target.read_bytes(), self.f.originals[name])
            self.assertEqual(stat.S_IMODE(target.stat().st_mode), 0o640)
            self.assertEqual(target.stat().st_uid, os.geteuid())
            self.assertEqual(target.stat().st_gid, os.getgid())

    def test_actual_install_inspect_and_exact_rollback(self):
        result = self.f.run()
        self.assertEqual(result["phase"], "installed")
        for name, (rel, mode) in m.TARGETS.items():
            target = self.f.root / rel
            self.assertEqual(target.read_bytes(), (self.f.root / "inputs" / name).read_bytes())
            self.assertEqual(stat.S_IMODE(target.stat().st_mode), mode)
        calls = [json.loads(x) for x in self.f.calls.read_text().splitlines()]
        self.assertEqual(len(calls), 3)
        self.assertEqual(calls[0], ["-t", "-f", str(self.f.root / "sshd_config")])
        self.assertEqual(calls[1][-2:], ["-C", "user=deploy,host=runner,addr=127.0.0.1,laddr=127.0.0.1,lport=2222"])
        self.assertEqual(calls[2][-2:], ["-C", "user=deploy,host=historical,addr=127.0.0.2,laddr=127.0.0.1,lport=2222"])
        self.assertEqual(self.f.run("inspect")["phase"], "installed")
        for field in ("production_installed", "sshd_reloaded", "whole_writer_fence_proven", "execution_authority", "drop_ready"):
            self.assertFalse(result[field])
        self.assertEqual(self.f.run("rollback")["phase"], "rolled_back")
        self.assert_original()
        self.assertEqual(self.f.run("inspect")["phase"], "rolled_back")
        self.rejected("host_install_existing_state", self.f.run)

    def test_actual_unset_targets_removed_on_rollback(self):
        directory = self.f.anchor / "second"
        directory.mkdir(mode=0o700)
        f = Fixture(directory, originals=False)
        f.run()
        f.run("rollback")
        self.assertTrue(all(not (f.root / rel).exists() for rel, _ in m.TARGETS.values()))

    def test_failed_sshd_validation_exact_automatic_rollback_no_raw_error(self):
        self.f.sshd_body("import sys\nsys.stderr.write('credential=PRIVATE_RAW_SENTINEL')\nsys.exit(17)\n")
        self.rejected("host_install_failed_and_rolled_back", self.f.run)
        self.assert_original()
        self.assertNotIn("PRIVATE_RAW_SENTINEL", self.f.run("inspect").__repr__())

    def test_each_required_effective_key_rejected(self):
        settings = dict(REQUIRED, authorizedkeysfile=str(self.f.root / "authorized_keys"))
        for key in settings:
            if key == "acceptenv":
                wrong = "LD_PRELOAD"
            else:
                wrong = "bad"
            with self.subTest(key=key):
                raw = ("\n".join(k + " " + (wrong if k == key else v) for k, v in settings.items()) + "\n").encode()
                self.rejected("host_install_sshd_projection_rejected", lambda: m._verify_effective(raw, self.f.root))

    def test_duplicate_effective_key_rejected(self):
        self.rejected("host_install_sshd_projection_rejected", lambda: m._verify_effective(b"strictmodes yes\nstrictmodes yes\n", self.f.root))

    def test_existing_content_or_mode_diff_refused(self):
        for variant in ("content", "mode"):
            with self.subTest(variant=variant):
                f = self.f
                target = f.root / "policy.json"
                if variant == "content":
                    target.write_bytes(b"independent change")
                else:
                    target.write_bytes(f.originals["policy"])
                    target.chmod(0o600)
                self.rejected("host_install_baseline_conflict", f.run)
        self.assertFalse(any((self.f.root / "state").glob("*.json")))

    def test_approved_original_gid_difference_refused(self):
        self.f.plan["assets"]["policy"]["original"]["gid"] += 1
        self.rejected("host_install_baseline_conflict", self.f.run)

    def test_original_source_hash_change_refused(self):
        (self.f.root / "inputs" / "binary").write_bytes(b"different upload")
        self.rejected("host_install_source_changed", self.f.run)

    def test_real_crash_leaves_unknown_no_adoption(self):
        process = multiprocessing.get_context("fork").Process(target=lambda: self.f.run(fault=lambda stage: os._exit(73) if stage == "binary" else None))
        process.start()
        process.join(10)
        self.assertFalse(process.is_alive())
        self.assertEqual(process.exitcode, 73)
        self.rejected("host_install_journal_unknown", lambda: self.f.run("inspect"))
        self.rejected("host_install_journal_unknown", lambda: self.f.run("rollback"))
        self.rejected("host_install_existing_state", self.f.run)
        self.assertEqual((self.f.root / "bin/qs-retirement-fence").read_bytes(), (self.f.root / "inputs/binary").read_bytes())

    def test_concurrent_real_process_refused_by_lock(self):
        ready, release = multiprocessing.get_context("fork").Event(), multiprocessing.get_context("fork").Event()
        def hook(stage):
            if stage == "intent":
                ready.set()
                if not release.wait(10):
                    raise RuntimeError("fixed fixture timeout")
        process = multiprocessing.get_context("fork").Process(target=lambda: self.f.run(fault=hook))
        process.start()
        self.assertTrue(ready.wait(10))
        try:
            self.rejected("host_install_operation_busy", self.f.run)
        finally:
            release.set()
            process.join(10)
        self.assertEqual(process.exitcode, 0)
        self.assertEqual(self.f.run("inspect")["phase"], "installed")

    def test_independent_change_after_install_blocks_rollback(self):
        self.f.run()
        (self.f.root / "policy.json").write_bytes(b"independent current config")
        self.rejected("host_install_rollback_conflict", lambda: self.f.run("rollback"))
        self.assertEqual((self.f.root / "policy.json").read_bytes(), b"independent current config")

    def test_in_process_failure_does_not_overwrite_other_writer(self):
        def hook(stage):
            if stage == "keyfile":
                (self.f.root / "policy.json").write_bytes(b"independent current config")
                raise RuntimeError("fixture injected failure")
        self.rejected("host_install_rollback_conflict", lambda: self.f.run(fault=hook))
        self.assertEqual((self.f.root / "policy.json").read_bytes(), b"independent current config")

    def test_unknown_journal_state_refused(self):
        self.f.run()
        path = sorted((self.f.root / "state").glob("*.json"))[-1]
        v = json.loads(path.read_bytes())
        v["phase"] = "operator_complete"
        path.write_bytes(m.canonical(v))
        self.rejected("host_install_journal_unknown", lambda: self.f.run("inspect"))

    def test_hardlink_and_symlink_and_fifo_sources_refused(self):
        for kind in ("hardlink", "symlink", "fifo"):
            with self.subTest(kind=kind):
                path = self.f.root / "inputs" / "binary"
                raw = path.read_bytes()
                path.unlink()
                spare = self.f.root / ("spare-" + kind)
                self.f.write(spare, raw, 0o600)
                if kind == "hardlink":
                    os.link(spare, path)
                elif kind == "symlink":
                    path.symlink_to(spare)
                else:
                    os.mkfifo(path, 0o600)
                self.rejected("host_install_file_rejected", self.f.run)
                path.unlink()
                self.f.write(path, raw, 0o600)
                spare.unlink()

    def test_writable_parent_refused(self):
        (self.f.root / "inputs").chmod(0o702)
        self.rejected("host_install_directory_rejected", self.f.run)

    def test_parent_swap_is_not_adopted(self):
        def hook(stage):
            if stage == "intent":
                old = self.f.root / "bin"
                old.rename(self.f.root / "old-bin")
                old.mkdir(mode=0o700)
        self.rejected("host_install_directory_changed", lambda: self.f.run(fault=hook))

    def test_policy_and_key_command_binding_refused(self):
        for name in ("policy", "keyfile"):
            with self.subTest(name=name):
                path = self.f.root / "inputs" / name
                raw = path.read_bytes()
                if name == "policy":
                    policy = json.loads(raw)
                    policy["operation_id"] = "999-1"
                    changed = m.canonical(policy)
                    category = "host_install_policy_binding_rejected"
                else:
                    changed = raw.replace(b" --policy ", b" --arbitrary-shell ")
                    category = "host_install_keyfile_rejected"
                path.write_bytes(changed)
                self.f.plan["assets"][name]["sha256"] = m.digest(changed)
                self.rejected(category, self.f.run)
                path.write_bytes(raw)
                self.f.plan["assets"][name]["sha256"] = m.digest(raw)

    def test_bad_approval_unknown_fields_duplicate_json_and_production_refused(self):
        raw = m.canonical(self.f.plan)
        self.rejected("host_install_approval_rejected", lambda: m._plan(raw, "0" * 64))
        p = dict(self.f.plan, production_complete=True)
        bad = m.canonical(p)
        self.rejected("host_install_schema_rejected", lambda: m._plan(bad, m.digest(bad)))
        self.rejected("host_install_json_rejected", lambda: m.decode(b'{"a":1,"a":2}'))
        self.rejected("host_install_production_boundary_unproven", lambda: m.execute(raw, m.digest(raw), "install"))
        self.f.plan["scope"] = "production"
        self.rejected("host_install_approval_rejected", self.f.run)

    def test_duplicate_or_missing_match_rejected(self):
        for matches in ([], [self.f.plan["matches"][0]] * 2):
            self.f.plan["matches"] = matches
            self.rejected("host_install_match_coverage_rejected", self.f.run)

    def test_original_asset_corruption_blocks_rollback_before_any_restore(self):
        self.f.run()
        target = self.f.root / "policy.json"
        installed = target.read_bytes()
        (self.f.root / "state" / "original-policy.private").write_bytes(b"different original")
        self.rejected("host_install_original_asset_changed", lambda: self.f.run("rollback"))
        self.assertEqual(target.read_bytes(), installed)

    def test_same_bytes_new_inode_conflict_is_actual(self):
        self.f.run()
        path = self.f.root / "policy.json"
        with path.open("rb") as held:
            before = os.fstat(held.fileno())
            raw = held.read()
            path.unlink()
            self.f.write(path, raw, 0o644)
            self.assertFalse(os.path.samestat(before, path.stat()))
            self.rejected("host_install_baseline_conflict", lambda: self.f.run("inspect"))
            self.rejected("host_install_rollback_conflict", lambda: self.f.run("rollback"))

    def test_rolled_back_inspect_does_not_adopt_new_config(self):
        self.f.run()
        self.f.run("rollback")
        (self.f.root / "policy.json").write_bytes(b"later independent config")
        self.rejected("host_install_baseline_conflict", lambda: self.f.run("inspect"))

    def test_complete_label_cannot_replace_four_actual_installed_assets(self):
        self.f.run()
        path = sorted((self.f.root / "state").glob("*.json"))[-1]
        v = json.loads(path.read_bytes())
        del v["installed"]["policy"]
        path.write_bytes(m.canonical(v))
        self.rejected("host_install_journal_unknown", lambda: self.f.run("inspect"))

    def test_unknown_preexisting_state_no_new_intent(self):
        self.f.write(self.f.root / "state" / "unrelated.private", b"unrelated", 0o600)
        self.rejected("host_install_journal_unknown", self.f.run)
        self.assertFalse((self.f.root / "state" / "0000.json").exists())

    def test_real_bounded_verifier_output_is_discarded(self):
        self.f.sshd_body("import sys\nsys.stdout.write('PRIVATE_OUTPUT_SENTINEL' * 150000)\n")
        self.rejected("host_install_failed_and_rolled_back", self.f.run)
        self.assert_original()
        self.assertNotIn("PRIVATE_OUTPUT_SENTINEL", str(self.f.run("inspect")))

    def test_marker_changed_no_actual_install(self):
        (self.f.root / "namespace.private.json").write_bytes(b"different marker")
        self.rejected("host_install_namespace_rejected", self.f.run)
        self.assert_original()

    def test_nonstring_hash_and_caller_complete_flags_refused(self):
        for key in ("source_sha", "request_sha256", "sshd_sha256", "namespace"):
            with self.subTest(key=key):
                old = self.f.plan[key]
                self.f.plan[key] = True
                self.rejected("host_install_approval_rejected", self.f.run)
                self.f.plan[key] = old

    def test_public_cli_unknown_arguments_never_echo_credentials(self):
        proc = subprocess.run([sys.executable, "-B", str(SOURCE), "--PRIVATE_SENTINEL_CREDENTIAL"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
        self.assertEqual(proc.returncode, 1)
        self.assertEqual(proc.stderr, b"")
        self.assertNotIn(b"PRIVATE_SENTINEL", proc.stdout)
        self.assertEqual(json.loads(proc.stdout)["category"], "host_install_input_rejected")

    def test_change_between_temp_write_and_second_cas_keeps_foreign_bytes(self):
        paths = m._Paths(str(self.f.anchor), os.geteuid())
        self.addCleanup(paths.close)
        target = self.f.root / "policy.json"
        expected = paths.snapshot(target)
        original = paths.snapshot
        calls = [0]
        def changed(path):
            if Path(path) == target:
                calls[0] += 1
                if calls[0] == 2:
                    target.write_bytes(b"foreign writer exact bytes")
            return original(path)
        paths.snapshot = changed
        self.rejected("host_install_baseline_conflict", lambda: paths.cas_write(target, expected, b"new approved bytes", 0o644, os.geteuid(), os.getgid()))
        self.assertEqual(target.read_bytes(), b"foreign writer exact bytes")
        self.assertFalse(list(self.f.root.glob(".qs-install-*")))
        paths.recheck()

    def test_rejected_directory_does_not_leak_owned_fd(self):
        descriptor_directory = Path("/proc/self/fd") if Path("/proc/self/fd").exists() else Path("/dev/fd")
        before = len(os.listdir(descriptor_directory))
        (self.f.root / "inputs").chmod(0o702)
        self.rejected("host_install_directory_rejected", self.f.run)
        self.assertEqual(len(os.listdir(descriptor_directory)), before)

    def test_unsupported_stage_refused_before_state_file(self):
        self.rejected("host_install_operation_rejected", lambda: self.f.run("apply"))
        self.assertEqual(list((self.f.root / "state").iterdir()), [])


class ApprovalFileTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="qs-host-install-approval-unit-")
        self.addCleanup(self.tmp.cleanup)
        # The public reader deliberately rejects symlink aliases such as /var.
        self.root = Path(self.tmp.name).resolve()
        self.path = self.root / "approval.private.json"
        self.raw = b'{"public_approval":"exact bytes"}\n'
        self.path.write_bytes(self.raw)
        self.path.chmod(0o600)

    def rejected(self, callable):
        with self.assertRaises(m.Rejected) as result:
            callable()
        self.assertEqual(str(result.exception), "host_install_approval_file_rejected")

    def cli_rejected(self, path):
        proc = subprocess.run([sys.executable, "-B", str(SOURCE), "--operation", "install",
                               "--approval-file", str(path), "--approval-sha256", "a" * 64],
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=2)
        self.assertEqual(proc.returncode, 1)
        self.assertEqual(proc.stderr, b"")
        result = json.loads(proc.stdout)
        self.assertEqual(result, {"category": "host_install_approval_file_rejected",
                                  "production_installed": False, "drop_ready": False})
        self.assertNotIn(str(path).encode(), proc.stdout)

    def test_actual_read_is_exact_and_each_open_is_nonblocking_nofollow(self):
        original, observed = os.open, []
        def opened(path, flags, *args, **kwargs):
            observed.append(flags)
            return original(path, flags, *args, **kwargs)
        with mock.patch.object(m.os, "open", opened):
            self.assertEqual(m.read_approval_file(str(self.path)), self.raw)
        self.assertGreater(len(observed), 1)
        self.assertTrue(all(flags & os.O_NOFOLLOW and flags & os.O_NONBLOCK for flags in observed))
        self.assertTrue(all(flags & os.O_DIRECTORY for flags in observed[:-1]))
        self.assertFalse(observed[-1] & os.O_DIRECTORY)

    def test_public_cli_fifo_without_writer_exits_within_subprocess_deadline(self):
        path = self.root / "PRIVATE_SENTINEL_FIFO"
        os.mkfifo(path, 0o600)
        self.cli_rejected(path)

    def test_public_cli_symlink_is_refused_without_echoing_target(self):
        path = self.root / "PRIVATE_SENTINEL_SYMLINK"
        path.symlink_to(self.path)
        self.cli_rejected(path)

    def test_hardlink_directory_and_oversize_are_rejected(self):
        link = self.root / "hardlink"
        os.link(self.path, link)
        self.rejected(lambda: m.read_approval_file(str(self.path)))
        link.unlink()
        self.rejected(lambda: m.read_approval_file(str(self.root)))
        self.path.write_bytes(b"x" * (m.APPROVAL_LIMIT + 1))
        self.cli_rejected(self.path)

    def test_exact_size_limit_is_accepted(self):
        raw = b"x" * m.APPROVAL_LIMIT
        self.path.write_bytes(raw)
        self.assertEqual(m.read_approval_file(str(self.path)), raw)

    def test_actual_same_bytes_new_inode_while_fd_held_is_rejected(self):
        original = m._approval_bytes
        def replaced(fd):
            raw = original(fd)
            held = os.fstat(fd)
            self.path.unlink()
            self.path.write_bytes(raw)
            self.path.chmod(0o600)
            self.assertFalse(os.path.samestat(held, self.path.stat()))
            return raw
        with mock.patch.object(m, "_approval_bytes", replaced):
            self.rejected(lambda: m.read_approval_file(str(self.path)))
        self.assertEqual(self.path.read_bytes(), self.raw)

    def test_actual_in_place_change_after_read_is_rejected(self):
        original = m._approval_bytes
        def changed(fd):
            raw = original(fd)
            held = os.fstat(fd)
            self.path.write_bytes(b"z" * len(raw))
            os.utime(self.path, ns=(held.st_atime_ns, held.st_mtime_ns + 1000000))
            return raw
        with mock.patch.object(m, "_approval_bytes", changed):
            self.rejected(lambda: m.read_approval_file(str(self.path)))

    def test_actual_parent_replacement_does_not_adopt_same_child_bytes(self):
        parent = self.root / "parent"
        parent.mkdir(mode=0o700)
        path = parent / "approval.json"
        path.write_bytes(self.raw)
        original = m._approval_bytes
        def replaced(fd):
            raw = original(fd)
            parent.rename(self.root / "old-parent")
            parent.mkdir(mode=0o700)
            path.write_bytes(raw)
            return raw
        with mock.patch.object(m, "_approval_bytes", replaced):
            self.rejected(lambda: m.read_approval_file(str(path)))

    def test_symlink_ancestor_and_noncanonical_paths_are_rejected(self):
        link = self.root / "parent-link"
        link.symlink_to(self.root, target_is_directory=True)
        self.rejected(lambda: m.read_approval_file(str(link / self.path.name)))
        for path in ("relative.json", str(self.root) + "/../approval.json", str(self.root) + "//approval.private.json", "/", "\x00"):
            with self.subTest(path=path):
                self.rejected(lambda: m.read_approval_file(path))

    def test_rejections_and_success_close_every_owned_fd(self):
        directory = Path("/proc/self/fd") if Path("/proc/self/fd").exists() else Path("/dev/fd")
        before = len(os.listdir(directory))
        for unused in range(10):
            self.assertEqual(m.read_approval_file(str(self.path)), self.raw)
            self.rejected(lambda: m.read_approval_file(str(self.root / "absent")))
        self.assertEqual(len(os.listdir(directory)), before)


if __name__ == "__main__":
    unittest.main()
