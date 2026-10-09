#!/usr/bin/env python3
"""Offline typed-diagnostic transport tests; no database or Docker execution."""

import contextlib
import hashlib
import importlib.util
import io
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location("mongo_target_tool", Path(__file__).with_name("compatibility-retirement.py"))
tool = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = tool
spec.loader.exec_module(tool)

TARGET = (b"QS_MONGO_TARGET_DIAGNOSTIC phase=find kind=server code=50 "
          b"run_ctx=active page_ctx=active pass=1 page=3 records=2000 source_bytes=456789 "
          b"page_elapsed_ms=2000 run_elapsed_ms=10000 mysql_elapsed_ms=6000")
INDEX = (b"QS_MONGO_INDEX_DIAGNOSTIC phase=list kind=server code=13 "
         b"namespace_sha256=" + b"a" * 64 + b" elapsed_ms=25")


class MongoTargetDiagnosticTransport(unittest.TestCase):
    def forwarded(self, raw):
        output = io.StringIO()
        with contextlib.redirect_stderr(output):
            tool._forward_mongo_target_diagnostic(raw)
        return output.getvalue()

    def through_private_fd(self, raw):
        output = io.StringIO()
        with tempfile.TemporaryFile() as fd, contextlib.redirect_stderr(output):
            fd.write(raw)
            fd.flush()
            tool._forward_inventory_diagnostic_fd(fd)
        return output.getvalue()

    def test_fixed_golden_and_primary_phases(self):
        for phase in (b"find", b"iterate", b"close"):
            line = TARGET.replace(b"phase=find", b"phase=" + phase)
            self.assertEqual(self.forwarded(line + b"\n"), line.decode("ascii") + "\n")

    def test_actual_timeout_kinds_and_context_states_remain_separate(self):
        for kind, code, run_state, page_state in (
                (b"context_deadline", b"0", b"deadline", b"deadline"),
                (b"context_deadline", b"0", b"active", b"deadline"),
                (b"network_timeout", b"0", b"active", b"active"),
                (b"context_cancelled", b"0", b"active", b"cancelled"),
                (b"other", b"0", b"active", b"active")):
            line = TARGET.replace(b"kind=server code=50", b"kind=" + kind + b" code=" + code)
            line = line.replace(b"run_ctx=active page_ctx=active", b"run_ctx=" + run_state + b" page_ctx=" + page_state)
            self.assertEqual(self.forwarded(line + b"\n"), line.decode("ascii") + "\n")

    def test_numeric_ranges_and_unknown_or_private_fields_are_rejected(self):
        replacements = (
            (b"code=50", b"code=2147483648"),
            (b"code=50", b"code=-2147483649"),
            (b"code=50", b"code=050"),
            (b"pass=1", b"pass=3"),
            (b"page=3", b"page=1002"),
            (b"records=2000", b"records=1000001"),
            (b"source_bytes=456789", b"source_bytes=2147483649"),
            (b"page_elapsed_ms=2000", b"page_elapsed_ms=10001"),
            (b"mysql_elapsed_ms=6000", b"mysql_elapsed_ms=10001"),
            (b"phase=find", b"phase=PRIVATE_URI"),
            (b"run_ctx=active", b"run_ctx=timeout_guessed"),
            (b"kind=server", b"kind=PRIVATE_ERROR"),
        )
        for old, new in replacements:
            self.assertEqual(self.forwarded(TARGET.replace(old, new) + b"\n"), "")
        self.assertEqual(self.forwarded(TARGET + b" uri=PRIVATE_URI\n"), "")
        self.assertEqual(self.forwarded(TARGET + b"\r\n"), "")
        self.assertEqual(self.forwarded(TARGET), "")

    def test_ambiguous_target_lines_and_index_markers_never_become_target(self):
        self.assertEqual(self.forwarded(TARGET + b"\n" + TARGET + b"\n"), "")
        self.assertEqual(self.forwarded(INDEX + b"\nPRIVATE_BSON_URI_TOKEN\n"), "")
        self.assertEqual(self.forwarded(b"X" * 8193 + b"\n" + TARGET + b"\n"), "")

    def test_index_and_target_contracts_are_forwarded_independently(self):
        raw = b"PRIVATE_BSON_URI_TOKEN\n" + INDEX + b"\n" + TARGET + b"\n"
        self.assertEqual(self.through_private_fd(raw), INDEX.decode("ascii") + "\n" + TARGET.decode("ascii") + "\n")
        self.assertEqual(self.through_private_fd(INDEX + b"\n" + INDEX + b"\n" + TARGET + b"\n"), TARGET.decode("ascii") + "\n")

    def test_large_stderr_keeps_only_complete_bounded_target_tail(self):
        raw = b"PRIVATE_UNKNOWN_" * 1000 + b"\n" + INDEX + b"\n" + TARGET + b"\n"
        self.assertEqual(self.through_private_fd(raw), TARGET.decode("ascii") + "\n")

    def test_tail_cutoff_cannot_turn_private_line_suffix_into_marker(self):
        suffix = TARGET + b"\n" + b"Z" * (8192 - len(TARGET) - 2) + b"\n"
        self.assertEqual(len(suffix), 8192)
        self.assertEqual(self.through_private_fd(b"X" * 9000 + b"q" + suffix), "")
        self.assertEqual(self.through_private_fd(b"X" * 9000 + b"\n" + suffix), TARGET.decode("ascii") + "\n")

    def test_capture_preserves_primary_stdout_bytes_and_hash(self):
        primary = b'{"complete":false,"drop_ready":false,"error_category":"inventory_incomplete"}\n'

        def child(command, **kwargs):
            os.write(kwargs["stderr"].fileno(), b"PRIVATE_URI\n" + TARGET + b"\n")
            return subprocess.CompletedProcess(command, 1, stdout=primary)

        output = io.StringIO()
        with mock.patch.object(tool.subprocess, "run", side_effect=child), contextlib.redirect_stderr(output):
            code, raw = tool.capture_fixed(["fixed-readonly-tool"], timeout=1530, mongo_index_diagnostics=True)
        self.assertEqual((code, raw), (1, primary))
        self.assertEqual(hashlib.sha256(raw).digest(), hashlib.sha256(primary).digest())
        self.assertEqual(output.getvalue(), TARGET.decode("ascii") + "\n")

    def test_real_child_stderr_transport_does_not_rewrite_stdout_or_exit(self):
        primary = b'{"error_category":"inventory_incomplete"}\n'
        program = "import os;os.write(2," + repr(b"PRIVATE_URI\n" + TARGET + b"\n") + ");os.write(1," + repr(primary) + ");raise SystemExit(1)"
        output = io.StringIO()
        with contextlib.redirect_stderr(output):
            code, raw = tool.capture_fixed([sys.executable, "-I", "-c", program], timeout=5, mongo_index_diagnostics=True)
        self.assertEqual((code, raw), (1, primary))
        self.assertEqual(output.getvalue(), TARGET.decode("ascii") + "\n")

    def test_noninventory_capture_still_discards_stderr(self):
        with mock.patch.object(tool.subprocess, "run", return_value=subprocess.CompletedProcess(["fixed-tool"], 0, stdout=b"fixed\n")) as child:
            self.assertEqual(tool.capture_fixed(["fixed-tool"], timeout=5), (0, b"fixed\n"))
        self.assertEqual(child.call_args.kwargs["stderr"], subprocess.DEVNULL)


if __name__ == "__main__":
    unittest.main()
