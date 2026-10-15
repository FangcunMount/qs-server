#!/usr/bin/env python3
"""Only local protocol/owned-child fixtures; never an SSH connection or DB read."""
import copy
import contextlib
import io
from contextlib import nullcontext
import hashlib
import importlib.util
import json
import os
import socket
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


def final_positive():
    value = m.final_receipt(positive())
    value["release"].update({key: True for key in m.OWNED_RELEASE})
    value["complete"] = value["eligible_fcv_readiness"] = True
    return value


def encoded(value):
    return json.dumps(value, separators=(",", ":")).encode()


def validate(value, code=0, final=False):
    return m.safe_receipt(encoded(value), code, SHA, RUN, TOOL, SOURCE, final=final)


def env_fixture():
    return {"MONGO_PROFILE_SOURCE_SHA": SHA, "MONGO_PROFILE_CONFIRM_SHA": SHA,
            "MONGO_PROFILE_RUN_ID": RUN, "MONGO_PROFILE_EXPECTED_TARGET_HASH": m.EXPECTED_TARGET,
            "MONGO_PROFILE_OPERATION": "metadata-visibility-admin-profile", "MONGO_PROFILE_DATABASE": "mongodb",
            "MONGODB_USERNAME": "fixture-db-account", "MONGODB_PASSWORD": "fixture-db-password", "MONGODB_DBNAME": "qs"}


class ReceiptTests(unittest.TestCase):
    def test_private_child_has_no_transport_or_backup_claim(self):
        value = validate(positive())
        self.assertFalse(value["complete"])
        self.assertFalse(value["eligible_fcv_readiness"])
        self.assertEqual(value["feature_compatibility"]["major"], 6)
        self.assertFalse(value["backup_restore_proven"])

    def test_final_requires_every_owned_release(self):
        value = final_positive()
        validate(value, final=True)
        for key in m.OWNED_RELEASE:
            with self.subTest(key=key):
                bad = copy.deepcopy(value)
                bad["release"][key] = False
                with self.assertRaises(m.ProfileError):
                    validate(bad, final=True)
                bad["complete"] = bad["eligible_fcv_readiness"] = False
                with self.assertRaises(m.ProfileError):
                    validate(bad, final=True)  # zero exit cannot hide incomplete release

    def test_release_failure_can_report_only_incomplete(self):
        value = positive()
        value["fcv_observation_complete"] = False
        value["error_category"] = "owned_resource_release_failed"
        value = m.final_receipt(value)
        value["failure_stage"] = "owned_resource_release"
        validate(value, code=1, final=True)

    def test_final_stage_is_required_fixed_and_consistent(self):
        value = final_positive()
        for stage in (None, "ssh_keyscan", "private host or stderr"):
            bad = copy.deepcopy(value)
            if stage is None:
                bad.pop("failure_stage")
            else:
                bad["failure_stage"] = stage
            with self.subTest(stage=stage), self.assertRaises(m.ProfileError):
                validate(bad, final=True)
        value["failure_stage"] = "none"
        with self.assertRaises(m.ProfileError):
            validate(value)  # transport fields are not accepted from the native child

        failed = m.final_receipt(m.initial_receipt(SHA, RUN, TOOL, SOURCE))
        failed["error_category"] = "reading_budget_exceeded"
        failed["failure_stage"] = "trusted_ssh"
        validate(failed, code=1, final=True)
        failed["failure_stage"] = "none"
        with self.assertRaises(m.ProfileError):
            validate(failed, code=1, final=True)

    def test_actual_total_budget_failure_preserves_negative_diagnostics(self):
        failed = m.final_receipt(m.initial_receipt(SHA, RUN, TOOL, SOURCE))
        failed.update(error_category="reading_budget_exceeded", failure_stage="total_budget")
        failed["budget"]["elapsed_milliseconds"] = 45001
        failed["release"]["owned_tunnel_process_released"] = True
        actual = validate(failed, code=1, final=True)
        self.assertEqual(actual["budget"]["elapsed_milliseconds"], 45001)
        self.assertTrue(actual["release"]["owned_tunnel_process_released"])
        self.assertFalse(actual["complete"])
        for change in (lambda v: v.update(error_category="tool_failed"), lambda v: v.update(failure_stage="native_reader"), lambda v: v.update(complete=True), lambda v: v.update(eligible_fcv_readiness=True), lambda v: v.update(fcv_observation_complete=True)):
            bad = copy.deepcopy(failed)
            change(bad)
            with self.assertRaises(m.ProfileError):
                validate(bad, code=1, final=True)
        with self.assertRaises(m.ProfileError):
            validate(failed, code=0, final=True)
        failed.pop("failure_stage")
        with self.assertRaises(m.ProfileError):
            validate(failed, code=1)  # native budget gate remains unchanged

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

    def test_v2_material_is_strictly_na_and_native_v1_cannot_be_final(self):
        value = final_positive()
        validate(value, final=True)
        for key in m.UNOWNED_MATERIAL:
            for invalid in (True, False, 0, "not_owned"):
                bad = copy.deepcopy(value)
                bad["release"][key] = invalid
                with self.assertRaises(m.ProfileError):
                    validate(bad, final=True)
        value["protocol"] = "mongo_admin_metadata_v1"
        with self.assertRaises(m.ProfileError):
            validate(value, final=True)
        value = final_positive()
        with self.assertRaises(m.ProfileError):
            validate(value, final=False)



class TransportTests(unittest.TestCase):
    def test_input_rejects_non_metadata_or_rebinding(self):
        config = m.inputs(env_fixture())
        self.assertEqual((config["host"], config["account"], config["ssh_port"], config["remote_port"]), (m.SSH_HOST, "root", 22, 27017))
        for key, value in (("MONGO_PROFILE_OPERATION", "backup"), ("MONGO_PROFILE_DATABASE", "all"), ("MONGO_PROFILE_ARCHIVE_INPUT", "archive"), ("MONGO_PROFILE_EXPECTED_TARGET_HASH", "0" * 64), ("MONGO_PROFILE_CONFIRM_SHA", "f" * 40), ("MONGODB_DBNAME", "admin"), ("MONGODB_PORT", "27018"), ("MONGODB_PORT", "0")):
            env = env_fixture()
            env[key] = value
            with self.assertRaises(m.ProfileError):
                m.inputs(env)
        env = dict(env_fixture(), MONGO_BACKUP_HOST="other", MONGO_BACKUP_USERNAME="other", MONGO_BACKUP_SSH_KEY="private fixture")
        self.assertEqual(m.inputs(env), config)
        self.assertFalse(any(key.startswith("MONGO_BACKUP_") for key in m.INPUT_ENV))

    def trusted_home(self, directory):
        home = Path(directory)
        (home / ".ssh").mkdir()
        (home / ".ssh/known_hosts").write_text("owned trusted metadata fixture\n")
        identity = home / ".ssh/identity"
        identity.write_text("owned identity fixture; never loaded for authentication\n")
        identity.chmod(0o600)
        config = home / "owned-ssh-config"
        config.write_text("Host 003-ysj\n  HostName 81.70.102.15\n  User wrong\n  Port 2222\n  IdentityFile " + str(identity) + "\n")
        return home, config

    def offline_capture(self, config, calls):
        real_capture = m.capture_private
        def capture(args, **kw):
            calls.append(args)
            if "-G" in args:
                return real_capture([args[0], "-F", str(config), *args[1:]], **kw)
            self.assertEqual(args[0:3], ["/usr/bin/ssh-keygen", "-F", m.SSH_HOST])
            return 0, b"# found\nowned fixture host key\n"
        return capture

    def test_real_offline_ssh_g_binds_existing_alias_then_freezes_only_identity(self):
        with tempfile.TemporaryDirectory() as directory:
            home, config = self.trusted_home(directory)
            calls = []
            with mock.patch.object(m, "capture_private", side_effect=self.offline_capture(config, calls)), mock.patch.dict(os.environ, {}, clear=True):
                base, local, known, known_hash = m.trusted_ssh(home, lambda: 5)
            self.assertEqual(len(calls), 2)
            self.assertEqual(calls[0][-5:], ["-l", "root", "-p", "22", "003-ysj"])
            self.assertEqual(base[1:3], ["-F", "/dev/null"])
            self.assertEqual(base[-4:], ["-l", "root", "-p", "22"])
            self.assertIn(str(home / ".ssh/identity"), base)
            for option in ("StrictHostKeyChecking=yes", "ForwardAgent=no", "ForwardX11=no", "ControlPath=none", "IdentityAgent=none", "UpdateHostKeys=no", "AddKeysToAgent=no"):
                self.assertIn(option, base)
            self.assertNotIn("ClearAllForwardings=yes", base)
            self.assertEqual(known_hash, m.trusted_file(known, 4 << 20))
            self.assertNotIn("MONGODB_PASSWORD", local)
            self.assertEqual(list(home.glob("qs-mongo*")), [])

    def test_real_offline_final_connection_has_exactly_its_own_loopback_forward(self):
        with tempfile.TemporaryDirectory() as directory:
            home, config = self.trusted_home(directory)
            with mock.patch.object(m, "capture_private", side_effect=self.offline_capture(config, [])), mock.patch.dict(os.environ, {}, clear=True):
                base, local, _known, _hash = m.trusted_ssh(home, lambda: 5)
            # A later alias change cannot add a foreign route/forward because
            # the final connection has frozen identity refs and no config.
            config.write_text("Host *\n  HostName other.invalid\n  ProxyCommand /usr/bin/false\n  LocalForward 127.0.0.1:38001 127.0.0.1:27017\n")
            code, raw = m.capture_private([*base, "-G", "-N", "-L", "127.0.0.1:37018:127.0.0.1:27017", m.SSH_HOST], env=local)
            self.assertEqual(code, 0)
            fields = {}
            for line in raw.decode().splitlines():
                key, value = line.split(None, 1)
                fields.setdefault(key, []).append(value)
            self.assertEqual(fields["hostname"], [m.SSH_HOST])
            self.assertEqual(fields["user"], ["root"])
            self.assertEqual(fields["port"], ["22"])
            self.assertEqual([entry.replace("[", "").replace("]", "") for entry in fields["localforward"]], ["127.0.0.1:37018 127.0.0.1:27017"])
            self.assertNotIn("remoteforward", fields)
            self.assertNotIn("dynamicforward", fields)
            self.assertEqual(fields["clearallforwardings"], ["no"])

    def test_real_offline_ssh_g_rejects_endpoint_proxy_hostkeyalias_localcommand_and_old_forwards(self):
        with tempfile.TemporaryDirectory() as directory:
            home, config = self.trusted_home(directory)
            original = config.read_text()
            variants = [original.replace("HostName 81.70.102.15", "HostName other.invalid")]
            variants += [original + "  " + override + "\n" for override in (
                "ProxyCommand /usr/bin/false", "ProxyJump other.invalid", "HostKeyAlias other",
                "LocalCommand /usr/bin/false", "LocalForward 127.0.0.1:38001 127.0.0.1:27017",
                "RemoteForward 38002 127.0.0.1:27017", "DynamicForward 127.0.0.1:38003")]
            for variant in variants:
                with self.subTest(variant=variant.splitlines()[-1]):
                    config.write_text(variant)
                    calls = []
                    with mock.patch.object(m, "capture_private", side_effect=self.offline_capture(config, calls)), mock.patch.dict(os.environ, {}, clear=True), self.assertRaises(m.ProfileError):
                        m.trusted_ssh(home, lambda: 5)
                    self.assertEqual(len(calls), 1)

    def test_effective_route_requires_single_root_ip_port(self):
        valid = b"user root\nhostname 81.70.102.15\nport 22\nidentityfile /owned/id\n"
        m.validate_ssh(valid)
        for raw in (valid.replace(b"root", b"other"), valid.replace(b"port 22", b"port 23"), valid.replace(b"port 22\n", b""), valid + b"hostname other\n", valid + b"localforward 127.0.0.1:1\n"):
            with self.assertRaises(m.ProfileError):
                m.validate_ssh(raw)

    def test_missing_untrusted_or_symlink_known_hosts_stop_before_ssh(self):
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            with mock.patch.object(m, "capture_private") as captured, self.assertRaises(OSError):
                m.trusted_ssh(home, lambda: 5)
            captured.assert_not_called()
            home, _config = self.trusted_home(directory)
            known = home / ".ssh/known_hosts"
            known.chmod(0o666)
            with mock.patch.object(m, "capture_private") as captured, self.assertRaises(m.ProfileError):
                m.trusted_ssh(home, lambda: 5)
            captured.assert_not_called()
            known.chmod(0o600)
            target = home / "known-target"
            known.rename(target)
            known.symlink_to(target)
            with mock.patch.object(m, "capture_private") as captured, self.assertRaises(m.ProfileError):
                m.trusted_ssh(home, lambda: 5)
            captured.assert_not_called()

    def test_agent_is_optional_and_only_existing_owned_socket_is_used_without_forwarding(self):
        with tempfile.TemporaryDirectory() as directory:
            home, config = self.trusted_home(directory)
            agent = home / "agent.sock"
            agent.write_bytes(b"not a socket")
            with mock.patch.dict(os.environ, {"SSH_AUTH_SOCK": str(agent)}, clear=True), mock.patch.object(m, "capture_private") as captured, self.assertRaises(m.ProfileError):
                m.trusted_ssh(home, lambda: 5)
            captured.assert_not_called()
            agent.unlink()
            with socket.socket(socket.AF_UNIX) as listener:
                listener.bind(str(agent))
                with mock.patch.dict(os.environ, {"SSH_AUTH_SOCK": str(agent)}, clear=True), mock.patch.object(m, "capture_private", side_effect=self.offline_capture(config, [])):
                    base, local, _known, _hash = m.trusted_ssh(home, lambda: 5)
                self.assertEqual(local["SSH_AUTH_SOCK"], str(agent))
                self.assertIn("IdentityAgent=" + str(agent), base)
                self.assertIn("ForwardAgent=no", base)

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

    def test_tunnel_release_failure_cannot_complete(self):
        self.owned_flow("release_failure")

    def test_early_tunnel_exit_does_not_connect_database(self):
        self.owned_flow("early_exit")

    def test_trusted_ssh_timeout_records_pre_database_stage(self):
        self.owned_flow("trusted_timeout")

    def test_existing_host_lookup_timeout_records_pre_database_stage(self):
        self.owned_flow("host_lookup_timeout")

    def test_native_failure_keeps_v1_facts_but_only_negative_v2(self):
        self.owned_flow("reader_failure")

    def test_existing_trust_changed_during_read_invalidates_observation(self):
        self.owned_flow("trust_changed")

    def test_foreign_listener_is_not_touched_or_used_for_database(self):
        self.owned_flow("foreign_listener")

    def test_main_failure_outputs_only_sealed_v2_without_secret_or_stderr(self):
        env = env_fixture()
        output, errors = io.StringIO(), io.StringIO()
        with mock.patch.dict(os.environ, env, clear=True), mock.patch.object(m.sys, "argv", ["profile", "--binary", "/owned/binary"]), mock.patch.object(m, "execute", side_effect=OSError(env["MONGODB_PASSWORD"])), contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
            code = m.main()
        self.assertEqual(code, 1)
        self.assertEqual(errors.getvalue(), "")
        self.assertNotIn(env["MONGODB_PASSWORD"], output.getvalue())
        value = json.loads(m.transport().decode_armored_receipt(output.getvalue()))
        self.assertEqual(value["protocol"], "mongo_admin_metadata_transport_v2")
        self.assertFalse(value["complete"])
        self.assertFalse(value["eligible_fcv_readiness"])
        self.assertEqual(value["source_sha"], SHA)
        self.assertEqual(value["run_id"], RUN)
        self.assertEqual(value["expected_target_hash"], m.EXPECTED_TARGET)
        for key in m.UNOWNED_MATERIAL:
            self.assertIsNone(value["release"][key])

    def owned_flow(self, mode):
        # SSH/tunnel/native responses are owned local protocol fixtures. The
        # real existing known_hosts hash and offline SSH-G are exercised.
        real_capture, real_popen = m.capture_private, subprocess.Popen
        env, seen = env_fixture(), []

        class OwnedTunnel:
            pid = 2147483000
            ended = False

            def poll(self):
                return 1 if mode == "early_exit" else (0 if self.ended else None)

            def wait(self, timeout):
                if mode == "release_failure":
                    raise OSError("fixture-private-error")
                self.ended = True
                return 0

        tunnel = OwnedTunnel()

        def popen(args, **kw):
            if args[0] == "/usr/bin/ssh" and "-N" in args:
                self.assertNotIn("MONGODB_PASSWORD", kw["env"])
                self.assertEqual(args[-1], m.SSH_HOST)
                self.assertEqual(args[1:3], ["-F", "/dev/null"])
                self.assertEqual(args.count("-L"), 1)
                self.assertNotIn("ClearAllForwardings=yes", args)
                seen.append("tunnel")
                return tunnel
            return real_popen(args, **kw)

        def capture(args, **kw):
            if args[0] == "/usr/bin/ssh-keyscan":
                self.fail("legacy keyscan route must not be called")
            if args[0] == "/usr/bin/ssh-keygen":
                if mode == "host_lookup_timeout":
                    raise m.ProfileError("reading_budget_exceeded")
                return 0, b"# found\nowned fixture host key\n"
            if args[0] in ("/usr/sbin/lsof", "/usr/bin/lsof"):
                port = next(arg.split(":")[-1] for arg in args if arg.startswith("-iTCP@"))
                pid = tunnel.pid + 1 if mode == "foreign_listener" else tunnel.pid
                return 0, ("p" + str(pid) + "\nn127.0.0.1:" + port + "\nTST=LISTEN\n").encode()
            if args[0] == "/usr/bin/ssh" and "-G" in args:
                if mode == "trusted_timeout":
                    raise m.ProfileError("reading_budget_exceeded")
                return real_capture([args[0], "-F", str(config), *args[1:]], **kw)
            seen.append("database")
            if mode == "interrupted":
                raise m.ProfileError("interrupted")
            payload = json.loads(kw["payload"])
            self.assertEqual(payload["username"], env["MONGODB_USERNAME"])
            self.assertEqual(payload["password"], env["MONGODB_PASSWORD"])
            self.assertEqual(set(args), {str(binary)})
            self.assertNotIn("env", kw)  # Native always uses its scrubbed environment.
            value = positive()
            value["tool_sha256"] = payload["tool_sha256"]
            value["source_binding_hash"] = payload["source_binding_hash"]
            if mode == "reader_failure":
                value["fcv_observation_complete"] = False
                value["error_category"] = "fcv_permission_denied"
                value["feature_compatibility"] = {key: False if type(v) is bool else 0 for key, v in value["feature_compatibility"].items()}
                return 1, encoded(value)
            if mode == "trust_changed":
                with (home / ".ssh/known_hosts").open("a") as stream:
                    stream.write("changed fixture trust metadata\n")
            return 0, encoded(value)

        with tempfile.TemporaryDirectory() as local:
            home, config = self.trusted_home(local)
            binary = home / "owned-tool"
            binary.write_bytes(b"owned-protocol-tool")
            binary.chmod(0o700)
            known = home / ".ssh/known_hosts"
            trust_before = known.read_bytes()
            with mock.patch.object(m.Path, "home", return_value=home), mock.patch.dict(os.environ, {}, clear=True), mock.patch.object(m, "source_check"), mock.patch.object(m, "capture_private", side_effect=capture), mock.patch.object(m.subprocess, "Popen", side_effect=popen), mock.patch.object(m.socket, "create_connection", return_value=nullcontext()), mock.patch.object(m.os, "killpg") as killed:
                code, value = m.execute(str(binary), env)
            self.assertEqual(value["protocol"], "mongo_admin_metadata_transport_v2")
            for key in m.UNOWNED_MATERIAL:
                self.assertIsNone(value["release"][key])
            self.assertEqual(list(home.glob("qs-mongo*")), [])
            self.assertTrue(known.exists())
            if mode != "trust_changed":
                self.assertEqual(known.read_bytes(), trust_before)
            if mode == "success":
                self.assertEqual(code, 0)
                self.assertTrue(value["complete"])
                self.assertTrue(m.owned_released(value))
                self.assertEqual(value["failure_stage"], "none")
            else:
                self.assertEqual(code, 1)
                self.assertFalse(value["complete"])
                self.assertFalse(value["eligible_fcv_readiness"])
            if mode in ("trusted_timeout", "host_lookup_timeout"):
                self.assertNotIn("database", seen)
                self.assertNotIn("tunnel", seen)
                killed.assert_not_called()
                self.assertEqual(value["failure_stage"], "trusted_ssh")
                self.assertEqual(value["error_category"], "reading_budget_exceeded")
                self.assertEqual(value["budget"]["logical_queries"], 0)
            elif mode == "early_exit":
                self.assertNotIn("database", seen)
                self.assertEqual(value["failure_stage"], "tunnel_listener")
                killed.assert_not_called()
            else:
                killed.assert_called_once_with(tunnel.pid, m.signal.SIGTERM)
            if mode in ("interrupted", "reader_failure"):
                self.assertEqual(value["failure_stage"], "native_reader")
            if mode == "release_failure":
                self.assertEqual(value["error_category"], "owned_resource_release_failed")
                self.assertEqual(value["failure_stage"], "owned_resource_release")
                self.assertFalse(value["release"]["owned_tunnel_process_released"])
            if mode == "trust_changed":
                self.assertEqual(value["failure_stage"], "source_recheck")
            if mode == "foreign_listener":
                self.assertNotIn("database", seen)
                self.assertEqual(value["failure_stage"], "tunnel_ownership")



def sys_executable():
    import sys
    return sys.executable


if __name__ == "__main__":
    unittest.main()
