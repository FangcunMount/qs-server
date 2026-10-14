#!/usr/bin/env python3
"""Only local protocol/owned-child fixtures; never an SSH connection or DB read."""
import copy
from contextlib import nullcontext
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location("mongo_profile", Path(__file__).with_name("mongodb-metadata-profile.py"))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
SHA, RUN, TOOL, SOURCE = "a" * 40, "12345-1", "b" * 64, "c" * 64


def positive():
    value = m.initial_receipt(SHA, RUN, TOOL, SOURCE)
    value["fcv_observation_complete"] = True
    value["target_hash"] = m.EXPECTED_TARGET
    value["target_check"] = {key: True for key in value["target_check"]}
    value["target_check"].update(head_version=39, dirty=False)
    value["authorization"] = {key: True for key in value["authorization"]}
    value["server_build"].update(known=True, major=7, minor=0, patch=31, matches_expected=True)
    # Actual observed FCV6.0 remains6.0 even with binary7.0.31.
    value["feature_compatibility"].update(known=True, major=6, minor=0)
    value["ttl_monitor"].update(known=True, enabled=True)
    value["release"].update(cursors_closed=True, client_disconnected=True)
    value["budget"].update(elapsed_milliseconds=100, logical_queries=10)
    return value


def encoded(value):
    return json.dumps(value, separators=(",", ":")).encode()


def validate(value, code=0, final=False):
    return m.safe_receipt(encoded(value), code, SHA, RUN, TOOL, SOURCE, final=final)


def env_fixture():
    return {"MONGO_PROFILE_SOURCE_SHA": SHA, "MONGO_PROFILE_CONFIRM_SHA": SHA,
            "MONGO_PROFILE_RUN_ID": RUN, "MONGO_PROFILE_EXPECTED_TARGET_HASH": m.EXPECTED_TARGET,
            "MONGO_PROFILE_OPERATION": "metadata-visibility-admin-profile", "MONGO_PROFILE_DATABASE": "mongodb",
            "MONGO_BACKUP_HOST": "127.0.0.1", "MONGO_BACKUP_USERNAME": "owned_fixture",
            "MONGO_BACKUP_SSH_KEY": "fixture-private-key-body", "MONGO_BACKUP_SSH_FINGERPRINT": "SHA256:" + "a" * 43,
            "MONGODB_USERNAME": "fixture-db-account", "MONGODB_PASSWORD": "fixture-db-password", "MONGODB_DBNAME": "qs"}


class ReceiptTests(unittest.TestCase):
    def test_private_child_has_no_transport_or_backup_claim(self):
        value = validate(positive())
        self.assertFalse(value["complete"])
        self.assertFalse(value["eligible_fcv_readiness"])
        self.assertEqual(value["feature_compatibility"]["major"], 6)
        self.assertFalse(value["backup_restore_proven"])

    def test_final_requires_every_owned_release(self):
        value = positive()
        value["release"] = {key: True for key in value["release"]}
        value["complete"] = value["eligible_fcv_readiness"] = True
        value["failure_stage"] = "none"
        validate(value, final=True)
        for key in value["release"]:
            with self.subTest(key=key):
                bad = copy.deepcopy(value)
                bad["release"][key] = False
                with self.assertRaises(m.ProfileError):
                    validate(bad, final=True)

    def test_release_failure_can_report_only_incomplete(self):
        value = positive()
        value["fcv_observation_complete"] = False
        value["error_category"] = "owned_resource_release_failed"
        value["failure_stage"] = "owned_resource_release"
        validate(value, code=1, final=True)

    def test_final_stage_is_required_fixed_and_consistent(self):
        value = positive()
        value["release"] = {key: True for key in value["release"]}
        value["complete"] = value["eligible_fcv_readiness"] = True
        for stage in (None, "ssh_keyscan", "private host or stderr"):
            bad = copy.deepcopy(value)
            if stage is not None:
                bad["failure_stage"] = stage
            with self.subTest(stage=stage), self.assertRaises(m.ProfileError):
                validate(bad, final=True)
        value["failure_stage"] = "none"
        with self.assertRaises(m.ProfileError):
            validate(value)  # transport fields are not accepted from the native child

        failed = m.initial_receipt(SHA, RUN, TOOL, SOURCE)
        failed["error_category"] = "reading_budget_exceeded"
        failed["failure_stage"] = "ssh_keyscan"
        validate(failed, code=1, final=True)
        failed["failure_stage"] = "none"
        with self.assertRaises(m.ProfileError):
            validate(failed, code=1, final=True)

    def test_required_nested_fields_and_forbidden_fields(self):
        for change in (lambda v: v.pop("target_hash"), lambda v: v["release"].pop("client_disconnected"), lambda v: v.update(username="secret"), lambda v: v["authorization"].update(roles=[])):
            value = positive()
            change(value)
            with self.assertRaises(m.ProfileError):
                validate(value)

    def test_every_binding_and_numeric_type_is_exact(self):
        for key, replacement in (("source_sha", "f" * 40), ("run_id", "12345-2"), ("expected_target_hash", "0" * 64), ("target_hash", "0" * 64), ("tool_sha256", "0" * 64), ("source_binding_hash", "0" * 64)):
            value = positive()
            value[key] = replacement
            with self.assertRaises(m.ProfileError):
                validate(value)
        value = positive()
        value["budget"]["logical_queries"] = True
        with self.assertRaises(m.ProfileError):
            validate(value)
        with self.assertRaises(m.ProfileError):
            validate(positive(), code=False)

    def test_dirty_transition_oversized_and_bad_protocol_rejected(self):
        for change in (lambda v: v["target_check"].update(dirty=True), lambda v: v["feature_compatibility"].update(target_present=True, target_major=7, transition_observed=True), lambda v: v["server_build"].update(patch=25), lambda v: v["budget"].update(logical_queries=11), lambda v: v["budget"].update(elapsed_milliseconds=45001), lambda v: v.update(protocol="other"), lambda v: v.update(backup_restore_proven=True)):
            value = positive()
            change(value)
            with self.assertRaises(m.ProfileError):
                validate(value)

    def test_unknown_version_cannot_retain_guessed_components(self):
        value = m.initial_receipt(SHA, RUN, TOOL, SOURCE)
        value["error_category"] = "fcv_permission_denied"
        validate(value, code=1)
        value["feature_compatibility"]["major"] = 7
        with self.assertRaises(m.ProfileError):
            validate(value, code=1)

    def test_ttl_known_not_disabled_is_required_for_positive(self):
        value=positive()
        value["ttl_monitor"]["enabled"]=False
        validate(value)
        for ttl in ({"known":False,"enabled":False},{"known":False,"enabled":True},{"known":True,"enabled":"false"}):
            value=positive()
            value["ttl_monitor"]=ttl
            with self.assertRaises(m.ProfileError):
                validate(value)

    def test_duplicate_json_and_raw_fallback_rejected(self):
        raw = encoded(positive()).replace(b'"dirty":false', b'"dirty":false,"dirty":false')
        with self.assertRaises(m.ProfileError):
            m.safe_receipt(raw, 0, SHA, RUN, TOOL, SOURCE)
        with self.assertRaises(ValueError):
            m.transport().decode_armored_receipt(encoded(positive()).decode())

    def test_short_secret_mask_does_not_damage_hashes(self):
        armor = m.transport()
        value = positive()
        frame = armor.encode_armored_receipt(value, schema=m.RECEIPT_SCHEMA, secrets=["a", "1", "admin", "read"])
        masked = frame
        for secret in ("a", "1", "admin", "read"):
            masked = masked.replace(secret, "***")
        validate(json.loads(armor.decode_armored_receipt(masked)))
        for bad in (frame + frame, frame[1:], frame[:-1], frame[:10] + "***" + frame[10:]):
            with self.assertRaises(ValueError):
                armor.decode_armored_receipt(bad)


class TransportTests(unittest.TestCase):
    def test_input_rejects_non_metadata_or_rebinding(self):
        m.inputs(env_fixture())
        for key, value in (("MONGO_PROFILE_OPERATION", "backup"), ("MONGO_PROFILE_DATABASE", "all"), ("MONGO_PROFILE_ARCHIVE_INPUT", "archive"), ("MONGO_PROFILE_EXPECTED_TARGET_HASH", "0" * 64), ("MONGO_PROFILE_CONFIRM_SHA", "f" * 40), ("MONGODB_DBNAME", "admin"), ("MONGO_BACKUP_HOST", "-oProxyCommand=x"), ("MONGO_BACKUP_USERNAME", "root;echo"), ("MONGO_BACKUP_SSH_PORT", "0")):
            env = env_fixture()
            env[key] = value
            with self.assertRaises(m.ProfileError):
                m.inputs(env)

    def test_real_offline_ssh_g_is_fixed_and_no_forwards(self):
        config = m.inputs(env_fixture())
        with tempfile.TemporaryDirectory() as directory:
            key, known = Path(directory) / "key", Path(directory) / "known"
            m.write_owned(key, b"fixture-key-not-used-for-auth\n")
            m.write_owned(known, b"")
            base = m.ssh_args(config, key, known)
            code, raw = m.capture_private([*base, "-G", config["host"]])
            self.assertEqual(code, 0)
            m.validate_ssh(raw, config, key, known)
            for key, replacement in ((b"hostname 127.0.0.1", b"hostname other"), (b"user owned_fixture", b"user root"), (b"port 22", b"port 2222")):
                with self.assertRaises(m.ProfileError):
                    m.validate_ssh(raw.replace(key, replacement), config, Path(directory) / "key", known)
            for addition in (b"proxycommand bad\n", b"proxyjump other\n", b"hostkeyalias other\n", b"localforward 127.0.0.1:1\n"):
                with self.assertRaises(m.ProfileError):
                    m.validate_ssh(raw + addition, config, Path(directory) / "key", known)

    def test_binary_is_owned_regular_file_not_link_or_world_writable(self):
        with tempfile.TemporaryDirectory() as directory:
            p = Path(directory) / "binary"
            p.write_bytes(b"ownedbinary")
            p.chmod(0o700)
            self.assertEqual(m.binary_hash(str(p)), hashlib.sha256(b"ownedbinary").hexdigest())
            link = Path(directory) / "link"
            link.symlink_to(p)
            with self.assertRaises(m.ProfileError):
                m.binary_hash(str(link))
            p.chmod(0o777)
            with self.assertRaises(m.ProfileError):
                m.binary_hash(str(p))

    def test_private_child_stdin_env_and_stderr_are_not_published(self):
        # Both sides are owned local Python. Payload/secret is not in argv or a file.
        program = "import os,sys;v=sys.stdin.buffer.read();sys.stderr.buffer.write(v);sys.stdout.buffer.write(b'clean' if 'MONGODB_PASSWORD' not in os.environ else b'leak')"
        with mock.patch.dict(os.environ, {"MONGODB_PASSWORD": "foreign-secret"}):
            code, raw = m.capture_private([sys_executable(), "-c", program], payload=b"private-stdin-marker")
        self.assertEqual((code, raw), (0, b"clean"))

    def test_child_output_limit_and_timeout_fail_closed(self):
        with self.assertRaises(m.ProfileError):
            m.capture_private([sys_executable(), "-c", "print('x'*1000)"], limit=64)
        with self.assertRaises(m.ProfileError):
            m.capture_private([sys_executable(), "-c", "import time;time.sleep(10)"], timeout=0.05)

    def test_owned_file_exclusive_and_private(self):
        with tempfile.TemporaryDirectory() as directory:
            p = Path(directory) / "key"
            m.write_owned(p, b"private-key-fixture")
            self.assertEqual(p.stat().st_mode & 0o777, 0o600)
            with self.assertRaises(FileExistsError):
                m.write_owned(p, b"replacement")

    def test_fingerprint_mismatch_leaves_no_file(self):
        with tempfile.TemporaryDirectory() as directory:
            with mock.patch.object(m, "capture_private", return_value=(0, b"256 SHA256:wrong fixture (ED25519)\n")):
                with self.assertRaises(m.ProfileError):
                    m.match_known_hosts(b"fixture ssh-ed25519 QUJD\n", "SHA256:" + "a" * 43, Path(directory), lambda: 1)
            self.assertEqual(list(Path(directory).iterdir()), [])

    def test_listener_must_belong_to_exact_owned_pid_and_port(self):
        for raw in (b"p101\nn127.0.0.1:37018\nTST=LISTEN\n", b"p100\nn127.0.0.1:37019\nTST=LISTEN\n", b"p100\nn127.0.0.1:37018\nTST=CLOSED\n"):
            with mock.patch.object(m, "capture_private", return_value=(0, raw)):
                with self.assertRaises(m.ProfileError):
                    m.owned_listener(100, 37018, 1)
        with mock.patch.object(m, "capture_private", return_value=(0, b"p100\nn127.0.0.1:37018\nTST=LISTEN\n")):
            m.owned_listener(100, 37018, 1)

    def test_full_owned_protocol_and_cleanup(self):
        self.owned_flow("success")

    def test_interruption_keeps_only_negative_and_releases_owned(self):
        self.owned_flow("interrupted")

    def test_directory_release_failure_cannot_complete(self):
        self.owned_flow("release_failure")

    def test_early_tunnel_exit_does_not_connect_database(self):
        self.owned_flow("early_exit")

    def test_keyscan_timeout_records_pre_database_stage(self):
        self.owned_flow("keyscan_timeout")

    def test_fingerprint_timeout_records_pre_database_stage(self):
        self.owned_flow("fingerprint_timeout")

    def owned_flow(self, mode):
        # SSH, keyscan and DB process responses are protocol fixtures. The real
        # filesystem permissions/deletion and offline SSH-G are exercised;
        # there is no network SSH or database call in this suite.
        real_capture, real_popen = m.capture_private, subprocess.Popen
        env, seen, directories = env_fixture(), [], []

        class OwnedTunnel:
            pid = 2147483000
            ended = False

            def poll(self):
                return 1 if mode == "early_exit" else (0 if self.ended else None)

            def wait(self, timeout):
                self.ended = True
                return 0

        tunnel = OwnedTunnel()

        def popen(args, **kw):
            if args[0] == "/usr/bin/ssh" and "-N" in args:
                self.assertNotIn("MONGODB_PASSWORD", kw["env"])
                seen.append("tunnel")
                return tunnel
            return real_popen(args, **kw)

        def capture(args, **kw):
            if args[0] == "/usr/bin/ssh-keyscan":
                if mode == "keyscan_timeout":
                    raise m.ProfileError("reading_budget_exceeded")
                return 0, b"127.0.0.1 ssh-ed25519 QUJD\n"
            if args[0] == "/usr/bin/ssh-keygen":
                if mode == "fingerprint_timeout":
                    raise m.ProfileError("reading_budget_exceeded")
                return 0, ("256 " + env["MONGO_BACKUP_SSH_FINGERPRINT"] + " fixture (ED25519)\n").encode()
            if args[0] in ("/usr/sbin/lsof", "/usr/bin/lsof"):
                port = next(arg.split(":")[-1] for arg in args if arg.startswith("-iTCP@"))
                return 0, ("p" + str(tunnel.pid) + "\nn127.0.0.1:" + port + "\nTST=LISTEN\n").encode()
            if args[0] == "/usr/bin/ssh" and "-G" in args:
                directory = Path(args[args.index("-i") + 1]).parent
                directories.append(directory)
                self.assertEqual(directory.stat().st_mode & 0o777, 0o700)
                self.assertEqual({p.name for p in directory.iterdir()}, {"ssh_key", "known_hosts"})
                self.assertFalse(any(env["MONGODB_PASSWORD"].encode() in p.read_bytes() for p in directory.iterdir()))
                return real_capture(args, **kw)
            seen.append("database")
            if mode == "interrupted":
                raise m.ProfileError("interrupted")
            payload = json.loads(kw["payload"])
            self.assertEqual(payload["username"], env["MONGODB_USERNAME"])
            self.assertEqual(payload["password"], env["MONGODB_PASSWORD"])
            self.assertEqual(set(args), {str(binary)})
            value = positive()
            value["tool_sha256"] = payload["tool_sha256"]
            value["source_binding_hash"] = payload["source_binding_hash"]
            return 0, encoded(value)

        with tempfile.TemporaryDirectory() as local:
            binary = Path(local) / "owned-tool"
            binary.write_bytes(b"owned-protocol-tool")
            binary.chmod(0o700)
            original_rmdir = Path.rmdir

            def rmdir(path):
                if mode == "release_failure" and path in directories:
                    raise OSError("fixture-private-error")
                return original_rmdir(path)

            with mock.patch.object(m, "source_check"), mock.patch.object(m, "capture_private", side_effect=capture), mock.patch.object(m.subprocess, "Popen", side_effect=popen), mock.patch.object(m.socket, "create_connection", return_value=nullcontext()), mock.patch.object(m.os, "killpg") as killed, mock.patch.object(Path, "rmdir", rmdir):
                code, value = m.execute(str(binary), env)
            if mode == "success":
                self.assertEqual(code, 0)
                self.assertTrue(value["complete"])
                self.assertTrue(all(value["release"].values()))
            else:
                self.assertEqual(code, 1)
                self.assertFalse(value["complete"])
                self.assertFalse(value["eligible_fcv_readiness"])
            if mode in ("keyscan_timeout", "fingerprint_timeout"):
                self.assertNotIn("database", seen)
                self.assertNotIn("tunnel", seen)
                killed.assert_not_called()
                self.assertEqual(value["failure_stage"], "ssh_keyscan" if mode == "keyscan_timeout" else "ssh_fingerprint")
                self.assertEqual(value["error_category"], "reading_budget_exceeded")
                self.assertEqual(value["budget"]["logical_queries"], 0)
                self.assertTrue(value["release"]["key_unlinked"])
                self.assertTrue(value["release"]["known_hosts_unlinked"])
                self.assertTrue(value["release"]["temp_directory_released"])
            elif mode == "early_exit":
                self.assertNotIn("database", seen)
                self.assertEqual(value["failure_stage"], "tunnel_listener")
            else:
                killed.assert_called_once_with(tunnel.pid, m.signal.SIGTERM)
            if mode == "success":
                self.assertEqual(value["failure_stage"], "none")
            if mode == "interrupted":
                self.assertEqual(value["failure_stage"], "native_reader")
            if mode == "release_failure":
                self.assertEqual(value["error_category"], "owned_resource_release_failed")
                self.assertEqual(value["failure_stage"], "owned_resource_release")
                self.assertFalse(value["release"]["temp_directory_released"])
            for path in directories:
                if path.exists():
                    self.assertEqual(list(path.iterdir()), [])
                    path.rmdir()
                self.assertFalse(path.exists())


def sys_executable():
    import sys
    return sys.executable


if __name__ == "__main__":
    unittest.main()
