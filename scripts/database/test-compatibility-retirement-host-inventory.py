import ast
import base64
import importlib.util
import json
import copy
from types import SimpleNamespace
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock

SOURCE = Path(__file__).with_name("compatibility-retirement-host-inventory.py")
spec = importlib.util.spec_from_file_location("owned_inventory", SOURCE)
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
CID = "c" * 64


class Fixture:
    def __init__(self, root):
        self.root = Path(root).resolve()
        self.put("/etc/passwd", b"deploy:x:1001:1001:PUBLIC_COMMENT:/home/deploy:/bin/bash\nroot:x:0:0:root:/root:/bin/bash\n")
        self.put("/etc/group", b"deploy:x:1001:deploy\ndocker:x:998:deploy\n")
        self.put("/etc/ssh/sshd_config", b"Include sshd_config.d/*.conf\n# PRIVATE_CONFIG_SENTINEL\nPasswordAuthentication no\n")
        self.put("/etc/ssh/sshd_config.d/20.conf", b"Match User deploy\n PermitTTY no\n")
        self.put("/etc/ssh/sshd_config.d/10.conf", b"PubkeyAuthentication yes\n")
        wire = (len(b"ssh-ed25519").to_bytes(4, "big") + b"ssh-ed25519" + (32).to_bytes(4, "big") + b"a" * 32)
        self.put("/home/deploy/.ssh/authorized_keys", b'restrict,command="/fixed PRIVATE_COMMAND_SENTINEL" ssh-ed25519 ' + base64.b64encode(wire) + b" COMMENT_SENTINEL\n")
        self.put("/proc/sys/kernel/random/boot_id", b"73d1485a-8bc0-430c-9928-7b70f76164c2\n")
        self.proc(123, "/usr/sbin/sshd")
        for kind in ("mnt", "pid", "user"):
            self.link("/proc/123/ns/" + kind, kind + ":[12345]")
        for path in ("/etc/cron.d", "/etc/cron.hourly", "/etc/cron.daily", "/etc/cron.weekly", "/etc/cron.monthly",
                     "/var/spool/cron", "/var/spool/cron/crontabs", "/data/qs", "/var/run"):
            self.physical(path).mkdir(parents=True, exist_ok=True)
        self.put("/etc/crontab", b"PASSWORD=NEVER_READ_SECRET_ENV\n* * * * * root docker run old-writer\n")
        self.put("/etc/cron.d/qs", b"TOKEN=NEVER_READ_SECRET_ENV\n")
        self.put("/var/run/docker.sock", b"socket_metadata_fixture")
        self.request = {"protocol": m.PROTOCOL, "source_sha": "a" * 40, "operation_id": "123-1", "run_id": "456-1", "host_role": "server_a",
                        "matches": [{"user": "deploy", "host": "runner", "addr": "127.0.0.1", "laddr": "127.0.0.1", "lport": "22"}]}
        self.identity = {"uid": 1001, "euid": 1001, "groups": [1001, 998], "pid": 123, "os": ["Linux", "PRIVATE_HOST", "6.1", "version", "aarch64"]}
        self.settings = {k: "none" for k in m.SSH_PROPERTIES}
        self.settings.update(authorizedkeysfile=".ssh/authorized_keys", pubkeyauthentication="yes", strictmodes="yes", forcecommand="none", acceptenv="LANG LC_*")
        self.inspect = {"id": CID, "image": "sha256:" + "d" * 64, "status": "exited", "running": False, "started": "2026-10-09T00:00:00Z",
                        "restarts": 0, "privileged": False, "readonly": False, "user": "PRIVATE_USER", "project": "PRIVATE_PROJECT", "service": "PRIVATE_SERVICE",
                        "mounts": [{"source": "/data/qs", "target": "/app/configs", "rw": True, "type": "bind"}]}

    def physical(self, path):
        return self.root / path.lstrip("/")

    def put(self, path, raw):
        p = self.physical(path)
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_bytes(raw)
        p.chmod(0o600)
        return p

    def link(self, path, target):
        p = self.physical(path)
        p.parent.mkdir(parents=True, exist_ok=True)
        p.symlink_to(target)

    def proc(self, pid, exe):
        self.put(f"/proc/{pid}/stat", (str(pid) + " (sshd) S " + " ".join(["0"] * 18 + ["10000"] + ["0"] * 5)).encode())
        self.put(f"/proc/{pid}/status", b"Uid:\t0\t0\t0\t0\n")
        self.put(f"/proc/{pid}/cgroup", b"0::/system.slice/sshd.service\n")
        self.put(f"/proc/{pid}/cmdline", b"/usr/sbin/sshd\x00-D\x00")
        self.link(f"/proc/{pid}/exe", exe)


class FakeRunner:
    def __init__(self, fixture):
        self.f, self.calls, self.hook = fixture, [], None

    def run(self, kind, arg=None):
        argv = m._argv(kind, arg)
        if self.hook:
            self.hook(kind, arg)
        f = self.f
        values = {"sudo_list": b"PRIVATE_SUDO_SENTINEL", "docker_list": (CID + "\n").encode(),
                  "sessions": b"1 1001 deploy seat0 tty1\n", "units": b"qs.service loaded active running PRIVATE_DESCRIPTION\n",
                  "unit_files": b"qs.service enabled enabled\n", "timers": b"PRIVATE_TIMER_SENTINEL\n"}
        if kind == "sshd":
            raw = ("\n".join(k + " " + v for k, v in f.settings.items()) + "\n").encode()
        elif kind == "docker_inspect":
            raw = m.canonical(f.inspect)
        elif kind == "unit":
            raw = b"Id=qs.service\nActiveState=active\nSubState=running\nMainPID=1\nFragmentPath=/etc/systemd/system/qs.service\nDropInPaths=\nUser=deploy\nGroup=deploy\nWorkingDirectory=/data/qs\n"
        elif kind == "session":
            raw = b"Id=1\nUser=1001\nLeader=123\nRemote=yes\nType=tty\nState=active\nActive=yes\nService=sshd\n"
        else:
            raw = values[kind]
        self.calls.append({"kind": kind, "argv_sha256": m.sha(m.canonical(argv)), "raw_sha256": m.sha(raw), "bytes": len(raw)})
        return raw


class InventoryTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="qs-host-inventory-unit-")
        self.addCleanup(self.tmp.cleanup)
        self.f = Fixture(self.tmp.name)

    def collect(self, runner=None, files=None):
        raw = m.canonical(self.f.request)
        return m._collect(m.request(raw, m.sha(raw)), m.sha(raw), files or m._Files(str(self.f.root)), runner or FakeRunner(self.f), self.f.identity)

    def rejected(self, category, fn):
        with self.assertRaises(m.Unknown) as e:
            fn()
        self.assertEqual(str(e.exception), category)

    def test_complete_local_fixture_is_still_unknown_not_authority(self):
        v = self.collect()
        self.assertEqual(len(v["observations"]["accounts"]["local_subjects"]), 2)
        self.assertEqual(v["observations"]["processes"]["observed"], 1)
        self.assertEqual(len(v["observations"]["docker"]["containers"]), 1)
        self.assertTrue(all(x["unchanged"] for x in v["end_rechecks"]))
        self.assertTrue(set(m.ALWAYS_UNKNOWN).issubset(v["unknown"]))
        self.assertTrue(all(x is False for x in v["capabilities"].values()))
        raw = m.canonical(v)
        for sentinel in (b"PRIVATE_", b"NEVER_READ_SECRET_ENV", b"deploy", b"/home/deploy", b"/data/qs"):
            self.assertNotIn(sentinel, raw)

    def test_unknown_permission_is_not_zero_rows_or_success(self):
        runner = FakeRunner(self.f)
        def denied(kind, arg):
            if kind == "docker_list":
                raise m.Unknown("command_denied_or_failed")
        runner.hook = denied
        v = self.collect(runner)
        self.assertIsNone(v["observations"]["docker"])
        self.assertIn("docker_visibility:command_denied_or_failed", v["unknown"])

    def test_untrusted_complete_flag_and_duplicate_keys_refused(self):
        for v in (dict(self.f.request, complete=True), dict(self.f.request, scope="production")):
            raw = m.canonical(v)
            self.rejected("inventory_request_rejected", lambda: m.request(raw, m.sha(raw)))
        raw = b'{"protocol":1,"protocol":2}\n'
        self.rejected("inventory_request_rejected", lambda: m.request(raw, m.sha(raw)))

    def test_independent_hash_source_and_context_schema_required(self):
        raw = m.canonical(self.f.request)
        self.rejected("inventory_request_rejected", lambda: m.request(raw, "0" * 64))
        for value in (True, "a" * 39, "$(credential)"):
            self.f.request["source_sha"] = value
            raw = m.canonical(self.f.request)
            self.rejected("inventory_request_rejected", lambda: m.request(raw, m.sha(raw)))

    def test_command_set_cannot_accept_shell_sudo_python_or_free_prefix(self):
        for kind, arg in (("shell", "id"), ("sudo_python", "/tmp/x"), ("docker_inspect", "$(TOKEN)"), ("unit", "--Environment=TOKEN"), ("session", ";id")):
            self.rejected("command_not_in_closed_read_set", lambda: m._argv(kind, arg))
        for kind in m.COMMANDS:
            argv = m._argv(kind)
            self.assertNotIn("sh", argv)
            self.assertNotIn("-S", argv)
        self.assertEqual(m._argv("sudo_list"), ["/usr/bin/sudo", "-n", "-l"])
        self.assertNotIn("Env", m.DOCKER_FORMAT)
        self.assertNotIn("exec", m._argv("docker_inspect", CID))

    def test_all_includes_real_ordered_eof_and_bound_hash(self):
        v = self.collect()
        includes = [x for x in v["observations"]["ssh"]["include_records"] if "raw_sha256" in x]
        self.assertEqual([x["raw_sha256"] for x in includes], [m.sha(self.f.physical(x).read_bytes()) for x in
                         ("/etc/ssh/sshd_config", "/etc/ssh/sshd_config.d/10.conf", "/etc/ssh/sshd_config.d/20.conf")])

    def test_include_cycle_symlink_and_unseen_nested_glob_are_unknown(self):
        cases = ((b"Include sshd_config\n", "ssh_include_cycle_or_depth_unknown"),
                 (b"Include */*.conf\n", "ssh_include_path_unsupported"))
        for raw, reason in cases:
            self.f.put("/etc/ssh/sshd_config", raw)
            self.assertIn("ssh_config_visibility:" + reason, self.collect()["unknown"])
        self.f.put("/etc/ssh/sshd_config", b"Include sshd_config.d/*.conf\n")
        p = self.f.physical("/etc/ssh/sshd_config.d/10.conf")
        p.unlink()
        p.symlink_to(self.f.physical("/etc/group"))
        self.assertIn("ssh_config_visibility:file_link_or_type_unsupported", self.collect()["unknown"])

    def test_dynamic_authorized_keys_provider_and_unenumerated_match_remain_unknown(self):
        self.f.settings["authorizedkeyscommand"] = "/private/dynamic PROVIDER_SENTINEL"
        v = self.collect()
        self.assertIn("dynamic_ssh_key_or_principal_provider_unknown", v["unknown"])
        self.assertIn("all_match_contexts_not_enumerated", v["unknown"])
        self.assertNotIn(b"PROVIDER_SENTINEL", m.canonical(v))

    def test_header_only_or_trailing_wire_not_counted_as_authorized_key(self):
        wire = len(b"ssh-ed25519").to_bytes(4, "big") + b"ssh-ed25519"
        self.f.put("/home/deploy/.ssh/authorized_keys", b"ssh-ed25519 " + base64.b64encode(wire) + b"\n")
        v = self.collect()
        self.assertIn("ssh_authorized_key_visibility:ssh_authorized_key_schema_unknown", v["unknown"])
        self.assertEqual(v["observations"]["ssh"]["matches"][0]["key_files"], [])
        good = wire + (32).to_bytes(4, "big") + b"a" * 32
        self.rejected("ssh_authorized_key_schema_unknown", lambda: m._public_wire(good + b"extra", "ssh-ed25519"))

    def test_cli_override_and_missing_daemon_not_inferred_from_default_config(self):
        self.f.put("/proc/123/cmdline", b"/usr/sbin/sshd\x00-D\x00-o\x00PasswordAuthentication=yes\x00")
        self.assertIn("ssh_daemon_cli_override_unproven", self.collect()["unknown"])
        self.f.physical("/proc/123/exe").unlink()
        self.f.link("/proc/123/exe", "/usr/bin/other")
        self.assertIn("ssh_actual_daemon_config_unknown", self.collect()["unknown"])

    def test_hidden_proc_is_unknown_not_writer_free_empty_set(self):
        p = self.f.physical("/proc/123/status")
        p.unlink()
        v = self.collect()
        self.assertIn("process_visibility:file_missing", v["unknown"])
        self.assertEqual(v["observations"]["processes"]["observed"], 0)
        self.assertFalse(v["capabilities"]["writer_fence_proven"])

    def test_stopped_extra_container_is_included_not_hidden(self):
        row = self.collect()["observations"]["docker"]["containers"][0]
        self.assertFalse(row["running"])
        self.assertEqual(row["id"], CID)
        self.assertEqual(row["image_config_digest"], self.f.inspect["image"])

    def test_projection_extra_env_or_bad_bool_cannot_emit_raw_data(self):
        for field in ("Env", "running", "privileged", "readonly"):
            old = dict(self.f.inspect)
            self.f.inspect[field] = "PRIVATE_CREDENTIAL_SENTINEL"
            v = self.collect()
            self.assertIsNone(v["observations"]["docker"])
            self.assertNotIn(b"PRIVATE_CREDENTIAL", m.canonical(v))
            self.f.inspect = old

    def test_container_changed_at_end_is_unknown(self):
        runner, count = FakeRunner(self.f), [0]
        def changed(kind, arg):
            if kind == "docker_inspect":
                count[0] += 1
                if count[0] == 2:
                    self.f.inspect["restarts"] += 1
        runner.hook = changed
        v = self.collect(runner)
        self.assertIn("docker_end_recheck:end_recheck_unproven", v["unknown"])

    def test_process_start_change_at_end_is_unknown(self):
        runner, count = FakeRunner(self.f), [0]
        def changed(kind, arg):
            if kind == "sudo_list":
                self.f.put("/proc/123/stat", b"123 (sshd) S " + b"0 " * 18 + b"20000 " + b"0 " * 5)
                count[0] += 1
        runner.hook = changed
        v = self.collect(runner)
        self.assertEqual(count[0], 1)
        self.assertIn("process_end_recheck:end_recheck_unproven", v["unknown"])

    def test_sshd_same_pid_but_original_config_argv_changes_is_unknown(self):
        runner = FakeRunner(self.f)
        def changed(kind, arg):
            if kind == "sudo_list":
                self.f.put("/proc/123/cmdline", b"/usr/sbin/sshd\x00-D\x00-f\x00/etc/ssh/other_config\x00")
        runner.hook = changed
        v = self.collect(runner)
        self.assertIn("process_end_recheck:end_recheck_unproven", v["unknown"])

    def test_account_and_sudo_schema_errors_have_no_raw_error(self):
        self.f.put("/etc/passwd", b"PRIVATE_CREDENTIAL_SENTINEL\n")
        v = self.collect()
        self.assertIsNone(v["observations"]["accounts"])
        self.assertIn("accounts_visibility:account_schema_unknown", v["unknown"])
        self.assertNotIn(b"PRIVATE_CREDENTIAL", m.canonical(v))

    def test_actual_private_subprocess_has_no_inherited_secret_environment(self):
        with mock.patch.dict(os.environ, {"PRIVATE_SECRET_ENV": "SENTINEL_SECRET"}):
            raw = m._capture([sys.executable, "-B", "-c", "import os;print('PRIVATE_SECRET_ENV' in os.environ)"], 2)
        self.assertEqual(raw, b"False\n")

    def test_current_paths_are_metadata_not_generic_secret_body_reads(self):
        files = m._Files(str(self.f.root))
        self.f.put("/data/qs/provider.env", b"DO_NOT_READ_SECRET\n")
        self.f.inspect["mounts"][0]["source"] = "/data/qs/provider.env"
        v = self.collect(files=files)
        self.assertNotIn("/data/qs/provider.env", files.witnesses)
        self.assertIn("/data/qs/provider.env", files.metadata_witnesses)
        self.assertNotIn(b"DO_NOT_READ_SECRET", m.canonical(v))

    def test_added_include_at_end_cannot_be_ignored_by_same_effective_output(self):
        runner = FakeRunner(self.f)
        def added(kind, arg):
            if kind == "sudo_list":
                self.f.put("/etc/ssh/sshd_config.d/30.conf", b"# only comment\n")
        runner.hook = added
        v = self.collect(runner)
        self.assertIn("ssh_config_visibility:directory_changed_during_read", v["unknown"])

    def test_cron_and_environment_body_never_opened(self):
        files = m._Files(str(self.f.root))
        original, observed = files.read, []
        def read(path, *args, **kwargs):
            observed.append(path)
            if path.startswith(("/etc/cron", "/var/spool/cron")) or path.endswith(("/environ", "/shadow")):
                self.fail("forbidden content read")
            return original(path, *args, **kwargs)
        files.read = read
        v = self.collect(files=files)
        self.assertIn("cron_contents_and_indirect_scripts_not_read", v["unknown"])
        self.assertTrue(v["observations"]["cron"])
        self.assertFalse(any("environ" in m._argv(k).__repr__() for k in m.COMMANDS))

    def test_misconfigured_sshd_cannot_redirect_reads_to_shadow_or_private_keys(self):
        self.f.put("/etc/shadow", b"PRIVATE_CREDENTIAL_SENTINEL")
        self.f.put("/home/deploy/.ssh/id_ed25519", b"PRIVATE_KEY_SENTINEL")
        self.f.put("/proc/123/cmdline", b"/usr/sbin/sshd\x00-D\x00-f\x00/etc/shadow\x00")
        files = m._Files(str(self.f.root))
        v = self.collect(files=files)
        self.assertIn("ssh_config_visibility:ssh_include_path_unsupported", v["unknown"])
        self.assertNotIn("/etc/shadow", files.witnesses)
        self.f.settings["authorizedkeysfile"] = "/home/deploy/.ssh/id_ed25519"
        self.f.put("/proc/123/cmdline", b"/usr/sbin/sshd\x00-D\x00")
        v = self.collect()
        self.assertIn("ssh_public_key_file_name_unsupported", v["unknown"])
        self.assertNotIn(b"PRIVATE_KEY", m.canonical(v))
        for path in ("/etc/shadow", "/home/deploy/.ssh/id_ed25519", "/proc/123/environ", "/data/config.env", "/etc/ssh/ssh_host_ed25519_key"):
            self.rejected("file_content_forbidden", lambda: files.read(path))

    def test_public_unknown_cli_argument_does_not_echo_credential(self):
        proc = subprocess.run([sys.executable, "-B", str(SOURCE), "--PRIVATE_CREDENTIAL_SENTINEL"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=2)
        self.assertEqual(proc.returncode, 1)
        self.assertEqual(proc.stderr, b"")
        self.assertEqual(json.loads(proc.stdout)["category"], "inventory_input_rejected")
        self.assertNotIn(b"PRIVATE_CREDENTIAL", proc.stdout)

    def test_readonly_sudo_and_docker_are_not_writer_denial(self):
        v = self.collect()
        self.assertFalse(v["observations"]["sudo_list"]["actual_command_denial_proven"])
        self.assertIn("historical_refs_reruns_queues_approvals_not_fenced", v["unknown"])

    def test_real_hardlink_fifo_and_symlink_approval_rejected(self):
        files = m._Files(str(self.f.root))
        p = self.f.put("/input.json", b"public request\n")
        os.link(p, self.f.physical("/linked"))
        self.rejected("file_link_or_type_unsupported", lambda: files.read("/input.json"))
        self.f.physical("/linked").unlink()
        p.unlink()
        os.mkfifo(p)
        self.rejected("file_link_or_type_unsupported", lambda: files.read("/input.json"))
        p.unlink()
        p.symlink_to(self.f.physical("/etc/group"))
        self.rejected("file_link_or_type_unsupported", lambda: files.read("/input.json"))

    def test_actual_same_bytes_new_inode_recheck_refused(self):
        files = m._Files(str(self.f.root))
        p = self.f.physical("/etc/group")
        files.read("/etc/group")
        with p.open("rb") as held:
            old, raw = os.fstat(held.fileno()), held.read()
            p.unlink()
            self.f.put("/etc/group", raw)
            self.assertFalse(os.path.samestat(old, p.stat()))
            self.rejected("file_changed_during_read", files.recheck)

    def test_stream_binary_hash_does_not_keep_bytes_or_use_config_cap(self):
        raw = b"x" * (m.FILE_CAP + 1024)
        self.f.put("/binary", raw)
        files = m._Files(str(self.f.root))
        data, proof = files.read("/binary", binary=True)
        self.assertIsNone(data)
        self.assertEqual(proof["raw_sha256"], m.sha(raw))
        self.assertEqual(files.bytes, 0)
        self.assertEqual(files.binary_bytes, len(raw))
        files.recheck()

    def test_file_budget_and_deadline_cannot_be_self_approved(self):
        files = m._Files(str(self.f.root))
        files.bytes = m.TOTAL_BYTES
        self.rejected("file_budget_exceeded", lambda: files.read("/etc/group"))
        files = m._Files(str(self.f.root))
        files.deadline = time.monotonic() - 1
        self.rejected("inventory_deadline_exceeded", lambda: files.read("/etc/group"))

    def test_actual_bounded_subprocess_timeout_output_and_error_privacy(self):
        self.assertEqual(m._capture([sys.executable, "-B", "-c", "print('readonly')"], 2), b"readonly\n")
        self.rejected("command_timeout", lambda: m._capture([sys.executable, "-B", "-c", "import time;time.sleep(5)"], .05))
        self.rejected("command_denied_or_failed", lambda: m._capture([sys.executable, "-B", "-c", "import sys;sys.stderr.write('PRIVATE_CREDENTIAL_SENTINEL');sys.exit(7)"], 2))
        self.rejected("command_output_budget_exceeded", lambda: m._capture([sys.executable, "-B", "-c", "import sys;sys.stdout.write('x'*3000000)"], 2))
        self.rejected("command_denied_or_failed", lambda: m._capture([sys.executable, "-B", "-c", "import sys;sys.stderr.write('PRIVATE_STDERR');print('apparently complete')"], 2))

    def test_unprotected_executable_cannot_enter_capture(self):
        class UnprotectedFiles:
            def metadata(self, path):
                return {"is_link": False, "uid": 1001, "mode": 0o755}
        with mock.patch.object(m, "_capture", side_effect=AssertionError("must not execute")):
            self.rejected("command_executable_unprotected", lambda: m._Runner(UnprotectedFiles()).run("sudo_list"))

    def test_property_projection_cannot_include_environment_or_duplicate_keys(self):
        for raw in (b"Id=qs.service\nEnvironment=PRIVATE_SECRET\n", b"Id=1\nId=2\n"):
            self.rejected("property_projection_schema_unknown", lambda: m._properties(raw, {"Id"}))

    def test_public_cli_fifo_times_out_safely_not_indefinite_open(self):
        p = self.f.physical("/request_fifo")
        os.mkfifo(p)
        proc = subprocess.run([sys.executable, "-B", str(SOURCE), "--request-file", str(p), "--request-sha256", "a" * 64],
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=2)
        self.assertEqual(proc.returncode, 1)
        self.assertEqual(proc.stderr, b"")
        self.assertEqual(json.loads(proc.stdout)["category"], "file_link_or_type_unsupported")

    def test_closed_error_cannot_relay_untrusted_category(self):
        self.assertEqual(str(m.Unknown("PRIVATE_CREDENTIAL_SENTINEL")), "inventory_fixed_failure")
        self.assertTrue(all(n.args[0].value in m.ERROR_CATEGORIES for n in ast.walk(ast.parse(SOURCE.read_bytes()))
                            if isinstance(n, ast.Call) and isinstance(n.func, ast.Name) and n.func.id == "reject" and n.args and isinstance(n.args[0], ast.Constant)))

    def test_rejections_do_not_leak_descriptors(self):
        directory = Path("/proc/self/fd") if Path("/proc/self/fd").exists() else Path("/dev/fd")
        before = len(os.listdir(directory))
        files = m._Files(str(self.f.root))
        for unused in range(10):
            self.rejected("file_missing", lambda: files.read("/absent"))
            files.read("/etc/group")
        self.assertEqual(len(os.listdir(directory)), before)

    def test_root_anchor_named_inode_mismatch_refuses_before_file_body(self):
        files = m._Files(str(self.f.root))
        original = os.stat
        def changed(path, *args, **kwargs):
            s = original(path, *args, **kwargs)
            if path == "/":
                values = list(s)
                values[1] += 1
                return os.stat_result(values)
            return s
        with mock.patch.object(m.os, "stat", changed):
            self.rejected("file_changed_during_read", lambda: files.read("/etc/group"))
        self.assertEqual(files.bytes, 0)


class ObservationRunner(FakeRunner):
    def __init__(self, fixture, usedns="no"):
        super().__init__(fixture)
        self.usedns = usedns

    def run(self, kind, arg=None):
        if kind != "sshd_global":
            return super().run(kind, arg)
        argv = m._argv(kind, arg)
        if self.hook:
            self.hook(kind, arg)
        raw = ("usedns " + self.usedns + "\n").encode()
        self.calls.append({"kind": kind, "argv_sha256": m.sha(m.canonical(argv)), "raw_sha256": m.sha(raw), "bytes": len(raw)})
        return raw


class ObservationTests(unittest.TestCase):
    setUp = InventoryTests.setUp
    rejected = InventoryTests.rejected
    def session(self, connection="192.0.2.10 45001 192.0.2.20 22"):
        v = {"uid": 1001, "euid": 1001, "username": "deploy", "ssh_connection": connection,
             "origin": "ordinary_ssh_session_environment"}
        v["identity_sha256"] = m.sha(m.canonical({k:v[k] for k in ("uid", "euid", "username")}))
        v["connection_sha256"] = m.sha(connection.encode())
        return v

    def v2(self, session=None):
        v = {k:x for k,x in self.f.request.items() if k != "matches"}
        return dict(v, protocol=m.OBSERVATION_PROTOCOL, context_mode=m.CONTEXT_MODE, session=session or self.session())

    def fixture2(self):
        self.f.proc(124, "/usr/bin/python3")
        self.f.put("/proc/124/stat", ("124 (python) S 123 " + " ".join(["0"] * 17 + ["10001"] + ["0"] * 5)).encode())
        self.f.put("/proc/124/status", b"Uid:\t1001\t1001\t1001\t1001\n")
        for kind in ("mnt", "pid", "user"):
            self.f.link("/proc/124/ns/" + kind, kind + ":[12345]")
        self.f.identity["pid"] = 124
        self.f.request = self.v2()

    def files(self, protected=True):
        files = m._Files(str(self.f.root));original = files.read
        def read(path, *args, **kwargs):
            raw, proof = original(path, *args, **kwargs)
            if path.startswith("/etc/ssh/"):
                proof["root_mode_protected"] = protected
                proof["ancestor_root_mode_protected"] = protected
            return raw, proof
        files.read = read
        return files

    def collect2(self, usedns="no", protected=True):
        raw = m.canonical(self.f.request)
        runner = ObservationRunner(self.f, usedns)
        result = m._collect(m.request(raw, m.sha(raw)),m.sha(raw),self.files(protected),runner,self.f.identity)
        return result, runner

    def test_v2_actual_ipv4_ipv6_identity_parser_and_no_user_env(self):
        for connection in ("192.0.2.10 45001 192.0.2.20 22", "2001:db8::1 1234 2001:db8::2 2222", "::ffff:192.0.2.1 1 ::1 65535"):
            with self.subTest(ip=connection.split()[0]):
                v=self.session(connection);self.assertEqual(len(m.validate_session(v)),4)
                with mock.patch.object(m.os,"getuid",return_value=1001),mock.patch.object(m.os,"geteuid",return_value=1001),mock.patch.object(m.pwd,"getpwuid",return_value=SimpleNamespace(pw_uid=1001,pw_name="deploy")),mock.patch.dict(m.os.environ,{"SSH_CONNECTION":connection,"USER":"FORGED_USER","LOGNAME":"FORGED"},clear=True):
                    self.assertEqual(m.observe_current_session(),v)
        for connection in ("", "192.0.2.1 22", "192.0.2.1  22 192.0.2.2 22", "example.org 22 192.0.2.2 22", "::1%eth0 22 ::1 22", "192.0.2.1 0 192.0.2.2 22", "192.0.2.1 22 192.0.2.2 65536", "PRIVATE_SECRET 22 ::1 22", "192.0.2.1\n22 192.0.2.2 22"):
            with self.subTest(case=len(connection)):
                self.rejected("inventory_request_rejected",lambda:m.validate_session(self.session(connection)))

    def test_v2_rejects_false_uid_nss_missing_hash_and_complete(self):
        for k,v in (("uid",True),("origin","authenticated"),("connection_sha256","a"*64),("complete",True),("username","PRIVATE_SECRET;cmd")):
            bad=self.session();bad[k]=v
            self.rejected("inventory_request_rejected",lambda:m.validate_session(bad))
        with mock.patch.object(m.pwd,"getpwuid",side_effect=KeyError("PRIVATE_SECRET")):
            self.rejected("inventory_request_rejected",m.observe_current_session)

    def test_v2_real_cli_error_protocol_and_missing_session_remain_closed(self):
        v=self.v2();raw=m.canonical(v);path=self.f.put('/v2-request.json',raw)
        result=subprocess.run([sys.executable,'-B',str(SOURCE),'--request-file',str(path),'--request-sha256',m.sha(raw)],stdout=subprocess.PIPE,stderr=subprocess.PIPE,env={'PATH':'/usr/bin:/bin'},timeout=3)
        self.assertEqual(result.returncode,1);self.assertEqual(result.stderr,b'')
        value=json.loads(result.stdout);self.assertEqual(value['protocol'],m.OBSERVATION_PROTOCOL)
        self.assertIn(value['category'],('linux_host_required','inventory_request_rejected'))
        self.assertFalse(any(value['capabilities'].values()));self.assertNotIn('192.0.2',result.stdout.decode())
        self.assertNotIn('deploy',result.stdout.decode())

    def test_v2_request_exact_and_v1_never_falls_back(self):
        v=self.v2();raw=m.canonical(v);self.assertEqual(m.request(raw,m.sha(raw)),v)
        for change in ({"matches":[]},{"context_mode":"all_matches"},{"complete":True},{"protocol":m.PROTOCOL}):
            bad=dict(v,**change);raw=m.canonical(bad)
            self.rejected("inventory_request_rejected",lambda:m.request(raw,m.sha(raw)))
        old=dict(self.f.request,matches=[]);raw=m.canonical(old)
        self.rejected("inventory_request_rejected",lambda:m.request(raw,m.sha(raw)))

    def test_v2_usedns_no_requires_actual_ancestor_protected_config_then_partial(self):
        self.fixture2();v,runner=self.collect2()
        so=v["session_observation"]
        self.assertEqual(v["protocol"],m.OBSERVATION_PROTOCOL);self.assertEqual(so["host_status"],"numeric_peer_from_usedns_no")
        self.assertEqual(so["usedns"],"no");self.assertEqual(so["match_context_count"],1)
        expected={"user":"deploy","host":"192.0.2.10","addr":"192.0.2.10","laddr":"192.0.2.20","lport":"22"}
        self.assertEqual(so["derived_matches_sha256"],m.sha(m.canonical([expected])))
        self.assertTrue(so["partial"]);self.assertFalse(so["origin_proven"]);self.assertFalse(any(v["capabilities"].values()))
        self.assertIn("session_environment_origin_unproven",v["unknown"]);self.assertIn("ssh_daemon_loaded_configuration_unproven",v["unknown"])
        self.assertTrue(all(x["unchanged"] for x in v["end_rechecks"]))
        public=m.canonical(v)
        for hidden in (b"192.0.2",b"deploy",b"PRIVATE_"):self.assertNotIn(hidden,public)
        self.assertTrue(any(x["kind"]=="sshd_global" for x in runner.calls))

    def test_v2_usedns_yes_missing_or_denied_host_query_does_not_guess_match(self):
        self.fixture2()
        for mode in ("yes","unknown"):
            v,runner=self.collect2(mode)
            self.assertEqual(v["session_observation"]["host_status"],"host_unobserved")
            self.assertEqual(v["observations"]["ssh"]["matches"],[])
            self.assertFalse(any(x["kind"]=="sshd" for x in runner.calls))
            self.assertIsNotNone(v["observations"]["docker"]);self.assertIsNotNone(v["observations"]["systemd"])
        runner=ObservationRunner(self.f)
        runner.hook=lambda kind,arg: m.reject("command_denied_or_failed") if kind=="sshd_global" else None
        raw=m.canonical(self.f.request);v=m._collect(self.f.request,m.sha(raw),self.files(),runner,self.f.identity)
        self.assertEqual(v["observations"]["ssh"]["matches"],[]);self.assertIn("session_context_visibility:command_denied_or_failed",v["unknown"])

    def test_v2_foreign_daemon_cli_override_or_unprotected_config_is_unknown(self):
        self.fixture2()
        v,runner=self.collect2(protected=False);self.assertEqual(v["session_observation"]["host_status"],"host_unobserved")
        self.assertFalse(any(x["kind"]=="sshd_global" for x in runner.calls))
        self.f.put("/proc/123/cmdline",b"/usr/sbin/sshd\x00-D\x00-o\x00UseDNS=no\x00")
        v,runner=self.collect2();self.assertEqual(v["observations"]["ssh"]["matches"],[])
        self.assertFalse(any(x["kind"]=="sshd_global" for x in runner.calls))
        self.f.put("/proc/124/stat",("124 (python) S 0 "+" ".join(["0"]*17+["10001"]+["0"]*5)).encode())
        v,runner=self.collect2();self.assertIn("ssh_session_daemon_binding_unknown",v["unknown"])

    def test_v2_current_connection_or_uid_change_is_rejected_at_public_entry(self):
        self.fixture2();raw=m.canonical(self.f.request);runner=ObservationRunner(self.f)
        first=self.f.request["session"];changed=self.session("192.0.2.11 45001 192.0.2.20 22")
        for observations in ([changed],[first,changed]):
            with mock.patch.object(m.sys,"platform","linux"),mock.patch.object(m,"observe_current_session",side_effect=observations),mock.patch.object(m,"_Files",return_value=self.files()),mock.patch.object(m,"_Runner",return_value=runner),mock.patch.object(m.os,"getuid",return_value=1001),mock.patch.object(m.os,"geteuid",return_value=1001),mock.patch.object(m.os,"getpid",return_value=124):
                self.rejected("inventory_request_rejected",lambda:m.inventory(raw,m.sha(raw)))

    def test_v2_public_inventory_consumes_request_not_private_bypass(self):
        self.fixture2();raw=m.canonical(self.f.request);runner=ObservationRunner(self.f)
        with mock.patch.object(m.sys,"platform","linux"),mock.patch.object(m,"observe_current_session",return_value=self.f.request["session"]),mock.patch.object(m,"_Files",return_value=self.files()),mock.patch.object(m,"_Runner",return_value=runner),mock.patch.object(m.os,"getuid",return_value=1001),mock.patch.object(m.os,"geteuid",return_value=1001),mock.patch.object(m.os,"getpid",return_value=124):
            v=m.inventory(raw,m.sha(raw))
        self.assertTrue(v["session_observation"]["identity_connection_rechecked"]);self.assertTrue(v["session_observation"]["partial"])

    def test_v2_daemon_config_change_during_lifecycle_fails_end_check(self):
        self.fixture2();runner=ObservationRunner(self.f);seen=0
        def hook(kind,arg):
            nonlocal seen
            if kind=="sshd_global":
                seen+=1
                if seen==2:runner.usedns="yes"
        runner.hook=hook;raw=m.canonical(self.f.request)
        v=m._collect(self.f.request,m.sha(raw),self.files(),runner,self.f.identity)
        self.assertIn("ssh_end_recheck:end_recheck_unproven",v["unknown"])
        self.assertFalse(next(x["unchanged"] for x in v["end_rechecks"] if x["kind"]=="ssh_end_recheck"))

if __name__ == "__main__":
    unittest.main()
