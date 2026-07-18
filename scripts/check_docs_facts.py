#!/usr/bin/env python3
"""Check low-cost facts and boundaries for the active documentation tree.

This complements check_docs_hygiene.py. It deliberately checks only facts that
can be derived cheaply and deterministically from the repository; prose still
requires code review when behavior changes.
"""
from __future__ import annotations

import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable


ROOT = Path(__file__).resolve().parent.parent
DOCS = ROOT / "docs"
ARCHIVE = DOCS / "_archive"
REGISTRY = ROOT / "internal/apiserver/container/modules/registry.go"
EVENTS = ROOT / "configs/events.yaml"
SIGNALS = ROOT / "configs/signals.yaml"

EXPECTED_TOP_LEVEL_DIRS = {
    "00-总览",
    "01-运行时",
    "02-业务模块",
    "03-基础设施",
    "04-接口与运维",
    "05-决策记录",
    "06-宣讲",
    "_archive",
}

BUSINESS_DOC_DIRS = {
    "survey": "10-survey",
    "modelcatalog": "20-model-catalog",
    "evaluation": "30-evaluation",
    "interpretation": "40-interpretation",
    "actor": "50-actor",
    "plan": "60-plan",
    "statistics": "70-statistics",
}

REQUIRED_EVENTS = {
    "answersheet.submitted",
    "evaluation.requested",
    "evaluation.outcome.committed",
    "evaluation.failed",
    "interpretation.report.generated",
    "interpretation.report.failed",
}

REQUIRED_SIGNALS = {
    "report_status_changed",
    "questionnaire_cache_changed",
}

REQUIRED_CONTRACTS = {
    ROOT / "api/rest/apiserver.yaml",
    ROOT / "api/rest/collection.yaml",
    ROOT / "api/grpc/proto",
    EVENTS,
    SIGNALS,
}

STALE_PATTERNS = {
    "legacy assessment.submitted event": re.compile(r"(?<![\w.])assessment\.submitted(?![\w.])"),
    "legacy assessment.evaluated event": re.compile(r"(?<![\w.])assessment\.evaluated(?![\w.])"),
    "legacy assessment.interpreted event": re.compile(r"(?<![\w.])assessment\.interpreted(?![\w.])"),
    "unqualified report.generated event": re.compile(r"(?<!interpretation\.)(?<![\w.])report\.generated(?![\w.])"),
}

ARCHIVE_LINK_RE = re.compile(r"\[[^\]]+\]\(([^)]*_archive[^)]*)\)")
ARCHIVE_LINK_ALLOWLIST = {DOCS / "README.md"}


@dataclass(frozen=True)
class Issue:
    kind: str
    detail: str


def active_markdown() -> Iterable[Path]:
    for path in sorted(DOCS.rglob("*.md")):
        if ARCHIVE not in path.parents:
            yield path


def parse_business_packages(registry_text: str) -> list[str]:
    constant_values = {
        name: value
        for name, value in re.findall(
            r"(Package[A-Za-z]+)\s+PackageName\s*=\s*\"([^\"]+)\"",
            registry_text,
        )
    }
    block_match = re.search(
        r"var BusinessPackages = \[\]PackageName\{(?P<body>.*?)\n\}",
        registry_text,
        flags=re.DOTALL,
    )
    if not block_match:
        return []
    constants = re.findall(r"\b(Package[A-Za-z]+)\b", block_match.group("body"))
    return [constant_values[name] for name in constants if name in constant_values]


def line_for_offset(text: str, offset: int) -> int:
    return text.count("\n", 0, offset) + 1


def main() -> int:
    issues: list[Issue] = []

    top_level_dirs = {path.name for path in DOCS.iterdir() if path.is_dir()}
    unexpected_dirs = sorted(top_level_dirs - EXPECTED_TOP_LEVEL_DIRS)
    missing_dirs = sorted(EXPECTED_TOP_LEVEL_DIRS - top_level_dirs)
    if unexpected_dirs:
        issues.append(Issue("unexpected-doc-root", ", ".join(unexpected_dirs)))
    if missing_dirs:
        issues.append(Issue("missing-doc-root", ", ".join(missing_dirs)))

    for contract in sorted(REQUIRED_CONTRACTS):
        if not contract.exists():
            issues.append(Issue("missing-contract", str(contract.relative_to(ROOT))))

    registry_text = REGISTRY.read_text(encoding="utf-8")
    business_packages = parse_business_packages(registry_text)
    if business_packages != list(BUSINESS_DOC_DIRS):
        issues.append(
            Issue(
                "business-module-drift",
                f"registry={business_packages}, docs={list(BUSINESS_DOC_DIRS)}",
            )
        )
    for package, directory in BUSINESS_DOC_DIRS.items():
        readme = DOCS / "02-业务模块" / directory / "README.md"
        if not readme.exists():
            issues.append(Issue("missing-module-readme", f"{package}: {readme.relative_to(ROOT)}"))

    event_text = EVENTS.read_text(encoding="utf-8")
    configured_events = set(re.findall(r"^  ([a-z0-9_.]+):\s*$", event_text, flags=re.MULTILINE))
    for event_name in sorted(REQUIRED_EVENTS - configured_events):
        issues.append(Issue("missing-event-contract", event_name))

    signal_text = SIGNALS.read_text(encoding="utf-8")
    configured_signals = set(re.findall(r"^  ([a-z0-9_]+):\s*$", signal_text, flags=re.MULTILINE))
    for signal_name in sorted(REQUIRED_SIGNALS - configured_signals):
        issues.append(Issue("missing-signal-contract", signal_name))

    files = list(active_markdown())
    if len(files) > 120:
        issues.append(Issue("active-doc-tree-too-large", f"{len(files)} files; limit is 120"))

    for path in files:
        text = path.read_text(encoding="utf-8")
        if path not in ARCHIVE_LINK_ALLOWLIST:
            match = ARCHIVE_LINK_RE.search(text)
            if match:
                issues.append(
                    Issue(
                        "active-doc-links-archive",
                        f"{path.relative_to(ROOT)}:{line_for_offset(text, match.start())}: {match.group(1)}",
                    )
                )
        for label, pattern in STALE_PATTERNS.items():
            match = pattern.search(text)
            if match:
                issues.append(
                    Issue(
                        "stale-contract-name",
                        f"{path.relative_to(ROOT)}:{line_for_offset(text, match.start())}: {label}",
                    )
                )

    retired_design_doc = DOCS / "系统设计文档.md"
    if retired_design_doc.exists():
        issues.append(Issue("retired-active-doc", str(retired_design_doc.relative_to(ROOT))))

    if issues:
        print(f"docs facts failed: {len(issues)} issue(s)")
        for issue in issues:
            print(f"[{issue.kind}] {issue.detail}")
        return 1

    print(
        "docs facts OK: "
        f"{len(files)} active markdown files, "
        f"{len(business_packages)} business modules, "
        f"{len(REQUIRED_EVENTS)} core events"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
