from __future__ import annotations

import tempfile
import unittest
from pathlib import Path
from unittest import mock

from scripts import check_docs_facts, check_docs_hygiene


class DocsFactsHelpersTest(unittest.TestCase):
    def test_yaml_section_keys_reads_every_event_and_signal(self) -> None:
        events = check_docs_facts.yaml_section_keys(
            check_docs_facts.EVENTS.read_text(encoding="utf-8"),
            "events",
            r"[a-z0-9_.]+",
        )
        signals = check_docs_facts.yaml_section_keys(
            check_docs_facts.SIGNALS.read_text(encoding="utf-8"),
            "signals",
            r"[a-z0-9_]+",
        )
        self.assertEqual(events, check_docs_facts.EXPECTED_EVENTS)
        self.assertEqual(signals, check_docs_facts.EXPECTED_SIGNALS)

    def test_grpc_inventory_includes_multiline_and_deprecated_rpcs(self) -> None:
        services, proto_file_count = check_docs_facts.grpc_inventory()
        self.assertEqual(proto_file_count, 7)
        self.assertEqual(len(services), 11)
        self.assertEqual(sum(len(rpcs) for rpcs in services.values()), 49)
        self.assertIn("GenerateReportFromAssessment", services["interpretation.InterpretationAutomationService"])
        self.assertIn("SyncAssessmentAttention", services["internalapi.InternalService"])

    def test_migration_inventory_is_paired_and_current(self) -> None:
        inventory, issues = check_docs_facts.migration_inventory()
        self.assertEqual(issues, [])
        self.assertEqual(inventory["mysql"], {"max_version": 70, "version_count": 70})
        self.assertEqual(inventory["mongodb"], {"max_version": 24, "version_count": 24})

    def test_ledger_metadata_uses_named_fields(self) -> None:
        text = (
            "<!-- docs-facts: checkout_sha=git:HEAD "
            "last_ci_sha=0123456789abcdef0123456789abcdef01234567 "
            "deployed_sha=unknown -->"
        )
        self.assertEqual(
            check_docs_facts.parse_ledger_metadata(text),
            {
                "checkout_sha": "git:HEAD",
                "last_ci_sha": "0123456789abcdef0123456789abcdef01234567",
                "deployed_sha": "unknown",
            },
        )


class DocsHygieneScopeTest(unittest.TestCase):
    def test_facts_inventory_excludes_tmp_markdown(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "docs").mkdir()
            (root / "docs/current.md").write_text("# Current\n", encoding="utf-8")
            (root / "tmp/run").mkdir(parents=True)
            (root / "tmp/run/stale.md").write_text("# Stale\n", encoding="utf-8")
            with mock.patch.object(check_docs_facts, "ROOT", root):
                files = [path.relative_to(root).as_posix() for path in check_docs_facts.maintained_markdown()]
            self.assertEqual(files, ["docs/current.md"])

    def test_iter_docs_excludes_tmp_markdown(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "docs").mkdir()
            (root / "docs/current.md").write_text("# Current\n", encoding="utf-8")
            (root / "tmp/run").mkdir(parents=True)
            (root / "tmp/run/stale.md").write_text("[missing](nope.md)\n", encoding="utf-8")
            with mock.patch.object(check_docs_hygiene, "ROOT", root), mock.patch.object(
                check_docs_hygiene,
                "DOCS_ROOT",
                root / "docs",
            ):
                files = [path.relative_to(root).as_posix() for path in check_docs_hygiene.iter_docs(False)]
            self.assertEqual(files, ["docs/current.md"])


if __name__ == "__main__":
    unittest.main()
