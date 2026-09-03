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
        self.assertEqual(len(services), 13)
        self.assertEqual(sum(len(rpcs) for rpcs in services.values()), 55)
        self.assertIn("GenerateReportFromAssessment", services["interpretation.InterpretationAutomationService"])
        self.assertIn("ExecutePromptEvaluationStep", services["interpretation.AIExplanationAutomationService"])
        self.assertIn("SyncAssessmentAttention", services["internalapi.InternalService"])

    def test_migration_inventory_is_paired_and_current(self) -> None:
        inventory, issues = check_docs_facts.migration_inventory()
        self.assertEqual(issues, [])
        self.assertEqual(inventory["mysql"], {"max_version": 70, "version_count": 70})
        self.assertEqual(inventory["mongodb"], {"max_version": 33, "version_count": 33})

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

    def test_normalize_markdown_collapses_blanks_and_fixes_heading_spacing(self) -> None:
        source = (
            "# Title\n"
            "\n"
            "\n"
            "Intro.\n"
            "## 1. Next\n"
            "body\n"
            "---\n"
            "## 2. After rule\n"
        )
        self.assertEqual(
            check_docs_hygiene.normalize_markdown(source),
            "# Title\n"
            "\n"
            "Intro.\n"
            "\n"
            "## 1. Next\n"
            "body\n"
            "\n"
            "---\n"
            "\n"
            "## 2. After rule\n",
        )

    def test_normalize_markdown_repairs_markup_table_gap_and_emphasis_heading(self) -> None:
        source = (
            "### Item\n"
            "\n"
            "flow A - > B\n"
            "\n"
            "**当前事实**\n"
            "\n"
            "broken * *bold** ok\n"
            "\n"
            "keep example `* *` and ` - > ` intact\n"
            "\n"
            "| a | b |\n"
            "| --- | --- |\n"
            "| 1 | 2 |\n"
            "Next paragraph.\n"
        )
        updated = check_docs_hygiene.normalize_markdown(source)
        self.assertEqual(
            updated,
            "### Item\n"
            "\n"
            "flow A -> B\n"
            "\n"
            "#### 当前事实\n"
            "\n"
            "broken **bold** ok\n"
            "\n"
            "keep example `* *` and ` - > ` intact\n"
            "\n"
            "| a | b |\n"
            "| --- | --- |\n"
            "| 1 | 2 |\n"
            "\n"
            "Next paragraph.\n",
        )
        self.assertEqual(
            check_docs_hygiene.content_fingerprint(source),
            check_docs_hygiene.content_fingerprint(updated),
        )

    def test_content_fingerprint_ignores_heading_shape_but_catches_deletes(self) -> None:
        left = "### Item\n\n**当前事实**\n\nkeep me\n"
        right = "### Item\n\n#### 当前事实\n\nkeep me\n"
        self.assertEqual(
            check_docs_hygiene.content_fingerprint(left),
            check_docs_hygiene.content_fingerprint(right),
        )
        self.assertNotEqual(
            check_docs_hygiene.content_fingerprint(left),
            check_docs_hygiene.content_fingerprint("### Item\n\n#### 当前事实\n"),
        )

    def test_compact_table_line_keeps_alignment_markers(self) -> None:
        self.assertEqual(
            check_docs_hygiene.compact_table_line("| ID      | :--- | ---: | :---: |"),
            "| ID | :--- | ---: | :---: |",
        )

    def test_check_spacing_reports_extra_blanks_and_heading_gaps(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "sample.md"
            path.write_text("# Title\n\n\nIntro.\n## Next\n", encoding="utf-8")
            issues = {(issue.kind, issue.line_no) for issue in check_docs_hygiene.check_spacing(path)}
            self.assertEqual(issues, {("extra-blank-lines", 3), ("heading-spacing", 5)})

    def test_check_spacing_reports_table_gap_and_emphasis_heading(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "sample.md"
            path.write_text(
                "### Item\n\n**当前事实**\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\nNext.\n",
                encoding="utf-8",
            )
            kinds = {issue.kind for issue in check_docs_hygiene.check_spacing(path)}
            self.assertEqual(kinds, {"emphasis-as-heading", "table-spacing"})

    def test_reflow_joins_wrap_broken_cjk_without_adding_space(self) -> None:
        source = (
            "Redis 故障、等待超时或关闭 coalescer 都进入“先 durable lookup，miss 再完整受理”的\n"
            "同一路径。因此后续仍走同一条回源校验。\n"
        )
        updated = check_docs_hygiene.reflow_markdown(source)
        self.assertIn("的同一路径。", updated)
        self.assertNotIn("的 同一路径", updated)
        self.assertEqual(
            check_docs_hygiene.content_fingerprint(source),
            check_docs_hygiene.content_fingerprint(updated),
        )

    def test_reflow_wraps_long_prose_and_keeps_fences_tables_comments(self) -> None:
        long = "这是一句用来验证折行的中文。" * 12
        source = (
            "# Title\n"
            "\n"
            f"{long}\n"
            "\n"
            "```text\n"
            "keep this fence line exactly as written even if it is quite long indeed\n"
            "```\n"
            "\n"
            "| a | b |\n"
            "| --- | --- |\n"
            "| 1 | 2 |\n"
            "\n"
            "<!-- docs-facts: checkout_sha=git:HEAD last_ci_sha=0123456789abcdef0123456789abcdef01234567 deployed_sha=unknown -->\n"
            "\n"
            "**读者**：前端  \n"
            "**服务**：collection-server\n"
        )
        updated = check_docs_hygiene.format_docs_markdown(source, reflow=True)
        prose_lines = [
            line
            for line in updated.splitlines()
            if line and not line.startswith(("#", "|", ">", "`", "<", "*"))
            and "keep this fence" not in line
        ]
        self.assertTrue(any(len(line) <= 120 for line in prose_lines))
        self.assertIn("keep this fence line exactly as written even if it is quite long indeed", updated)
        self.assertIn("<!-- docs-facts: checkout_sha=git:HEAD", updated)
        self.assertIn("**读者**：前端  ", updated)
        self.assertEqual(
            check_docs_hygiene.content_fingerprint(source),
            check_docs_hygiene.content_fingerprint(updated),
        )

    def test_reflow_keeps_facts_tokens_contiguous(self) -> None:
        cache = (
            "Negative cache 只适合“明确不存在且短期内重复查询”的对象读取。"
            "当前 questionnaire 与 testee adapter 显式声明 `CacheNegative: true`，"
            "仍需 effective Policy 同时启用才会写 sentinel；published-model 的 "
            "by-questionnaire、catalog-list 与 algorithms 三条 read-through 均显式声明 "
            "`CacheNegative: false`，Policy 打开也不会使它们写 negative sentinel。\n"
        )
        event = (
            "- `evaluation.failed`、`interpretation.report.generated` 与 "
            "`interpretation.report.failed` 的 report-status Redis 写入/Signal "
            "唤醒是 best-effort；Reporter 吞掉写入与通知错误，handler 可以最终 ACK。\n"
        )
        cache_updated = check_docs_hygiene.reflow_markdown(cache)
        event_updated = check_docs_hygiene.reflow_markdown(event)
        self.assertIn("published-model 的 by-questionnaire、catalog-list 与 algorithms", cache_updated)
        self.assertIn("report-status Redis 写入/Signal 唤醒是 best-effort", event_updated)
        self.assertEqual(
            check_docs_hygiene.content_fingerprint(cache),
            check_docs_hygiene.content_fingerprint(cache_updated),
        )
        self.assertEqual(
            check_docs_hygiene.content_fingerprint(event),
            check_docs_hygiene.content_fingerprint(event_updated),
        )


if __name__ == "__main__":
    unittest.main()
