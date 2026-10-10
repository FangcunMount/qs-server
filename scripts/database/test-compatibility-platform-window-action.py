#!/usr/bin/env python3
"""Offline schema/ordering and real local pipe tests; no GH/root/SSH proof."""
import ast
import copy
import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    value = importlib.util.module_from_spec(spec); spec.loader.exec_module(value)
    return value


ROOT = Path(__file__).resolve().parents[2]
A = module("platform_action", Path(__file__).with_name("compatibility-platform-window-action.py"))
W = module("platform_window", Path(__file__).with_name("compatibility-window-tool.py"))
P = module("platform_fence", Path(__file__).with_name("compatibility-platform-fence.py"))
T = module("platform_receipt", ROOT / "scripts/dbops/receipt-transport.py")
F = module("window_fixtures", Path(__file__).with_name("test-compatibility-window-tool.py"))
W = F.tool  # Keep exception classes identical to the reused fixture module.


class RunnerWindowTests(unittest.TestCase):
    def fixture(self, stage="apply"):
        fixture = F.WindowToolMetadata()
        approval, native = fixture.native(stage)
        approval.update(b_image_id="sha256:" + "9" * 64, b_program_sha256="a" * 64)
        # Synthetic protocol values are never installed as a live native proof.
        return approval, native

    def frame(self, approval, native):
        value = dict(format_version=1, kind="independent_window_tool_call_result", dispatcher_source_sha=approval["dispatcher_source_sha"],
            tool_source_sha=approval["tool_source_sha"], approved_template_sha256=approval["request_template_sha256"], derived_request_sha256=native["request_sha256"], native_result=native)
        raw = json.dumps(value, sort_keys=True, separators=(",", ":")).encode()
        return (chr(0xE100) + "".join(chr(0xE000 + v) for v in raw) + chr(0xE101) + "\n").encode()

    def classify(self, approval, native, code):
        return A.disposition(W, T, self.frame(approval, native), code, approval, "22-3")[0]

    def test_only_complete_apply_purge_or_bound_recovery_protocol_is_releasable(self):
        for stage in ("apply", "purge"):
            with self.subTest(stage=stage):
                a, n = self.fixture(stage); n.update(acceptance_complete=True, purge_complete=True)
                self.assertEqual(self.classify(a, n, 0), "native_completed")
        a, n = self.fixture("recover"); n.update(recovery_attempted=True, recovery_complete=True)
        self.assertEqual(self.classify(a, n, 0), "native_recovered")

    def test_recovery_complete_without_terminal_complete_never_releases_workflows(self):
        # Actual native Close can clear complete after recovery succeeds. The
        # old formatter normalizes child-exit/close errors, so both their raw
        # and generic public forms must be conservative. These are protocol
        # fixtures, not a same-process terminal-release producer or GH proof.
        for stage in ("apply", "verify", "recover", "purge"):
            for error in ("lifecycle_service_channel_child_exit_unknown",
                          "lifecycle_native_operation_failed", "none"):
                for include_recovery_error in (False, True):
                    with self.subTest(stage=stage, error=error,
                                      include_recovery_error=include_recovery_error):
                        a, n = self.fixture(stage)
                        n.update(complete=False, recovery_attempted=True,
                                 recovery_complete=True, acceptance_complete=False,
                                 error_category=error)
                        if include_recovery_error:
                            n["recovery_error_category"] = "none"
                        reason, validated = A.disposition(W, T, self.frame(a, n), 1, a, "22-3")
                        self.assertEqual(reason, "native_unknown_or_not_released")
                        self.assertFalse(validated["complete"])
                        self.assertTrue(validated["recovery_complete"])
                        if error == "lifecycle_service_channel_child_exit_unknown":
                            self.assertEqual(validated["error_category"], "lifecycle_native_operation_failed")

    def test_verification_acceptance_or_failed_purge_does_not_open_workflows(self):
        for stage, updates, code in (("verify", dict(acceptance_complete=True), 0),
            ("apply", dict(complete=False, acceptance_complete=True, error_category="lifecycle_native_operation_failed"), 1),
            ("recover", dict(complete=False, recovery_attempted=True, recovery_complete=False, error_category="lifecycle_native_operation_failed"), 1)):
            with self.subTest(stage=stage):
                a, n = self.fixture(stage); n.update(updates)
                self.assertEqual(self.classify(a, n, code), "native_unknown_or_not_released")

    def test_exact_compiled_preflight_shape_is_releasable_but_transport_failure_is_not(self):
        a, n = self.fixture()
        for key in W.BOOLS: n[key] = False
        n.update(error_category="lifecycle_actual_host_adapters_missing", original_source_sha="", manifest_sha256="", archive_sha256="", restore_elapsed_millis=0)
        self.assertEqual(self.classify(a, n, 1), "native_preflight_refused")
        for code in (0, 125, 255, -15, True):
            with self.subTest(code=code), self.assertRaises(A.Rejected): self.classify(a, n, code)
        for key in W.BOOLS:
            changed = dict(n); changed[key] = True
            with self.subTest(key=key), self.assertRaises((A.Rejected, W.Refused)): self.classify(a, changed, 1)

    def test_wrong_source_run_derived_hash_foreign_fields_or_ambiguous_frames_reject(self):
        a, n = self.fixture(); n.update(acceptance_complete=True, purge_complete=True)
        for key, value in (("source_sha", "f" * 40), ("run_id", "21-2"), ("target_hash", "c" * 64), ("drop_ready", True), ("credentials", {})):
            changed = dict(n); changed[key] = value
            with self.subTest(key=key), self.assertRaises(A.Rejected): self.classify(a, changed, 0)
        frame = self.frame(a, n)
        for raw in (b"{\"complete\":true}", frame + frame, frame[:-5]):
            with self.assertRaises(A.Rejected): A.disposition(W, T, raw, 0, a, "22-3")

    def test_effectful_scope_and_original_template_bind_both_directions(self):
        fixture = F.WindowToolMetadata(); r = fixture.request(); a = fixture.approval(r, "apply")
        a.update(b_image_id="sha256:" + "9" * 64, b_program_sha256="a" * 64)
        self.assertEqual(fixture.approve(a), a)
        self.assertEqual(W.decode(W.derive_request(W.canonical(r), a, "22-3"))["writer_control"], r["writer_control"])
        for field, value in (("original_source_sha", "f" * 40), ("dispatcher_source_sha", "f" * 40), ("operation_id", "23-1"), ("original_run_id", "23-1"), ("manifest_sha256", "f" * 64), ("runner_id", True), ("workflow_ids", [24,24]), ("drop_ready", True)):
            changed = copy.deepcopy(a); changed["workflow_scope"][field] = value
            with self.subTest(field=field), self.assertRaises(W.Refused): fixture.approve(changed)
        changed = copy.deepcopy(r); changed["writer_control"]["workflow_scope_sha256"] = "f" * 64
        a["request_template_sha256"] = W.digest(W.canonical(changed))
        with self.assertRaises(W.Refused): W.derive_request(W.canonical(changed), a, "22-3")

    def test_prepare_keeps_exact_fourteen_fields_and_rejects_scope_even_null(self):
        fixture = F.WindowToolMetadata(); r = fixture.request(); a = fixture.approval(r)
        self.assertEqual(len(a), 14); self.assertEqual(fixture.approve(a), a)
        for value in (None, {}, {"drop_ready": False}):
            changed = dict(a, workflow_scope=value)
            with self.assertRaises(W.Refused): fixture.approve(changed)

    def test_actual_local_child_last_write_and_exit_proves_both_pipe_eof_and_wait(self):
        code, raw = A.collect_owned([sys.executable, "-I", "-c", "import os; os.write(1,b'last reply\\n')"], timeout=5)
        self.assertEqual((code, raw), (0, b"last reply\n"))

    def test_actual_local_private_control_pipe_is_held_open_until_child_terminal(self):
        program = "import sys,select; print(sys.stdin.buffer.readline().decode().strip(),flush=True); assert not select.select([sys.stdin],[],[],.05)[0]"
        code, raw = A.collect_owned([sys.executable, "-I", "-c", program], packet=b"expected-binding\n", timeout=5)
        self.assertEqual((code, raw), (0, b"expected-binding\n"))

    def test_actual_timeout_closes_control_and_never_returns_a_terminal_success(self):
        program = "import sys; sys.stdin.buffer.readline(); sys.stdin.buffer.read()"
        with self.assertRaises(A.Rejected):
            A.collect_owned([sys.executable, "-I", "-c", program], packet=b"expected-binding\n", timeout=.05)

    def test_actual_full_input_pipe_cannot_escape_the_execution_timeout(self):
        # This is a local OS pipe, not a synthetic EOF or remote/root proof.
        with self.assertRaises(A.Rejected):
            A.collect_owned([sys.executable, "-I", "-c", "import time; time.sleep(.2)"], packet=b"x" * (1 << 20), timeout=.05)

    def test_exact_owned_ephemeral_identity_inode_cleanup_and_unknown_retention(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary).resolve(); directory.chmod(0o700)
            asset = A.IdentityAsset(directory / "ssh.key", b"synthetic-owned-identity")
            try:
                asset.remove(); self.assertFalse(asset.path.exists())
                self.assertEqual(os.fstat(asset.fd).st_nlink, 0)
            finally: asset.close()
            asset = A.IdentityAsset(directory / "ssh.key", b"synthetic-owned-identity")
            try:
                (directory / "ssh.key").write_bytes(b"changed-identity")
                with self.assertRaises(A.Rejected): asset.remove()
                self.assertTrue(asset.path.exists())
            finally: asset.close()

    def test_remote_bootstrap_is_closed_parseable_and_does_not_set_token_env(self):
        ast.parse(A.REMOTE)
        self.assertIn("os.O_EXCL", A.REMOTE)
        self.assertIn("control=sys.stdin.fileno()", A.REMOTE)
        self.assertIn("os.environ.clear()", A.REMOTE)
        for value in ("os.exec", "os.system", "recovery_complete", "GITHUB_TOKEN", "whole_writer_fence_proven", "rmtree"):
            self.assertNotIn(value, A.REMOTE)

    def test_runner_factory_actual_descriptor_platform_is_observation_only(self):
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary).resolve(); directory.chmod(0o700)
            fixture = F.WindowToolMetadata(); r = fixture.request(); a = fixture.approval(r, "apply"); s = a["workflow_scope"]
            with patch.dict(os.environ, {"GITHUB_RUN_ID":"900", "GITHUB_RUN_ATTEMPT":"1", "GITHUB_SHA":s["dispatcher_source_sha"], "GITHUB_REPOSITORY":P.REPO, "GITHUB_EVENT_NAME":"workflow_dispatch", "GITHUB_REF":"refs/heads/main"}, clear=True):
                owner = P.open_native_workflow_quarantine(s, P.digest(P.canonical(s)), directory, b"synthetic-never-sent", total_seconds=6840)
                try:
                    self.assertFalse(owner._installed)
                    self.assertFalse(owner._restored)
                finally: owner.close()

    def test_permissions_and_actual_owner_restore_are_only_in_dedicated_effect_job(self):
        workflow = (ROOT / ".github/workflows/compatibility-retirement.yml").read_text()
        ordinary, effect = workflow.split("  controlled-window-mutation:\n")
        self.assertNotIn("actions: write", ordinary)
        self.assertIn("inputs.prepare_mode != 'window-tool' || inputs.operation == 'prepare'", ordinary)
        self.assertIn("inputs.prepare_mode == 'window-tool' && inputs.operation != 'prepare'", effect)
        self.assertEqual(effect.count("actions: write"), 1)
        self.assertIn("persist-credentials: false", effect)
        self.assertNotIn("appleboy/ssh-action", effect)
        self.assertIn("RETIREMENT_PLATFORM_TOKEN: ${{ github.token }}", effect)
        self.assertIn("compatibility-platform-window-action.py", effect)

    def test_public_runner_receipt_is_armored_and_contains_no_raw_native_or_credentials(self):
        receipt = dict(protocol="runner_platform_window_owner_v1", error_category="platform_window_native_unknown", whole_writer_fence_proven=False, drop_ready=False)
        stream = io.StringIO()
        with contextlib.redirect_stdout(stream): A.emit(receipt, ("synthetic-private-token",))
        text = stream.getvalue()
        self.assertNotIn("synthetic-private-token", text)
        self.assertNotIn("platform_window_native_unknown", text)
        self.assertEqual(json.loads(T.decode_armored_receipt(text)), receipt)
        with self.assertRaises(T.ReceiptTransportError):
            # Each load owns its class; use the locally loaded transport below.
            T.encode_armored_receipt(dict(receipt, credentials={}), schema={"protocol":frozenset({"runner_platform_window_owner_v1"}),"error_category":frozenset({"platform_window_native_unknown"}),"whole_writer_fence_proven":"bool","drop_ready":"bool"})



class RootPasswordClosedTransport(unittest.TestCase):
    def test_actual_remote_rejects_malformed_password_before_staging(self):
        import json,subprocess,sys
        for password in (None,42,'fixture\npassword','fixture\rpassword','fixture\x00password','\u00e9'*2049):
            with self.subTest(kind=type(password).__name__):
                packet={'bindings':['apply','12-1','22-1','d'*40,'c'*64,'e'*64],'approval':'','approval_sha256':'a'*64,'package_sha256':'b'*64,'tool_directory':'/tmp/qs-independent-window-tool.invalid','credentials':{'MYSQL_PASSWORD':'fixture_db_secret'},'sudo_password':password}
                result=subprocess.run([sys.executable,'-I','-c',A.REMOTE],input=(json.dumps(packet)+'\n').encode(),capture_output=True,timeout=5,check=False)
                self.assertEqual(result.returncode,125);self.assertEqual(result.stdout,b'');self.assertEqual(result.stderr,b'')

    def test_remote_password_is_separate_from_credentials_and_not_in_root_packet(self):
        import ast
        ast.parse(A.REMOTE)
        self.assertIn("'credentials','sudo_password'",A.REMOTE)
        self.assertIn("if os.getuid()!=0 and password: os.environ['SUDO_PASSWORD']=password",A.REMOTE)
        source=Path(A.__file__).read_text()
        packet=source[source.index('        root_packet ='):source.index('        if len(root_packet)')]
        self.assertNotIn('sudo_password',packet);self.assertNotIn('SUDO_PASSWORD',packet)
        for name in ('native-window.intent.json','native-window.result.json'):
            call=next(n for n in ast.walk(ast.parse(source)) if isinstance(n,ast.Call) and any(isinstance(arg,ast.Constant) and arg.value==name for arg in n.args))
            self.assertNotIn('sudo_password',ast.unparse(call));self.assertNotIn('SUDO_PASSWORD',ast.unparse(call))

    def test_action_plumbs_only_original_a_secret_and_receipt_cannot_claim_s0(self):
        source=Path(__file__).resolve().parents[2]
        workflow=(source/'.github/workflows/compatibility-retirement.yml').read_text()
        self.assertEqual(workflow.count('SUDO_PASSWORD: ${{'),3)
        self.assertEqual(sum('SUDO_PASSWORD' in line for line in workflow.splitlines() if line.strip().startswith('envs:')),2)
        self.assertIn('secrets.SVRA_SUDO_PASSWORD',workflow)
        self.assertNotIn('secrets.SVRD_SUDO_PASSWORD',workflow)
        receipt = dict(protocol="runner_platform_window_owner_v1",
                       error_category="platform_window_native_unknown",
                       whole_writer_fence_proven=False, drop_ready=False)
        stream = io.StringIO()
        with contextlib.redirect_stdout(stream):
            A.emit(receipt, ("fixture-sudo-password",))
        self.assertEqual(json.loads(T.decode_armored_receipt(stream.getvalue())), receipt)
        # The platform receipt cannot acquire S0 physical-exit authority. The
        # separate S0 suite owns the actual fixed launcher and stdin tests.
        with patch.object(A, "load", return_value=T), self.assertRaises(T.ReceiptTransportError), \
             contextlib.redirect_stdout(io.StringIO()):
            A.emit(dict(receipt, s0_physical_exit_complete=True), ())

if __name__ == "__main__": unittest.main()
