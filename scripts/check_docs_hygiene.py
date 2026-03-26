#!/usr/bin/env python3
"""
Check hygiene for current docs/ markdown files.

Default scope excludes docs/_archive because archive files are transitional
reference material and are expected to age out.

Checks:
1. Relative markdown links resolve to existing files/directories.
2. Markdown fragment links resolve to real headings.
3. If a file uses numbered H2 headings (`## 1. ...`), the sequence is strictly
   increasing without duplicates or skips.
"""
from __future__ import annotations

import argparse
import re
import sys
import unicodedata
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Iterable, List, Sequence
from urllib.parse import unquote


ROOT = Path(__file__).resolve().parent.parent
DOCS_ROOT = ROOT / "docs"
MARKDOWN_LINK_RE = re.compile(r"(?<!\!)\[[^\]]+\]\(([^)]+)\)")
NUMBERED_H2_RE = re.compile(r"^##\s+(\d+)\.\s+")
HEADING_RE = re.compile(r"^(#{1,6})\s+(.*?)\s*$")


@dataclass
class Issue:
    kind: str
    file_path: Path
    line_no: int
    detail: str


def iter_docs(include_archive: bool) -> Iterable[Path]:
    for path in sorted(DOCS_ROOT.rglob("*.md")):
        if not include_archive and "_archive" in path.parts:
            continue
        yield path


def strip_markdown(text: str) -> str:
    text = re.sub(r"`([^`]*)`", r"\1", text)
    text = re.sub(r"\[([^\]]+)\]\([^)]+\)", r"\1", text)
    text = re.sub(r"<[^>]+>", "", text)
    return text.strip()


def slugify_heading(text: str) -> str:
    text = strip_markdown(text).lower()
    chars: List[str] = []
    last_was_hyphen = False
    for ch in text:
        category = unicodedata.category(ch)
        if ch.isspace() or ch == "-":
            if chars and not last_was_hyphen:
                chars.append("-")
                last_was_hyphen = True
            continue
        if category.startswith(("L", "N")):
            chars.append(ch)
            last_was_hyphen = False
            continue
    return "".join(chars).strip("-")


def collect_heading_slugs(file_path: Path) -> Dict[str, int]:
    slugs: Dict[str, int] = {}
    slug_counts: Dict[str, int] = {}
    in_fence = False
    for line_no, line in enumerate(file_path.read_text(encoding="utf-8").splitlines(), 1):
        stripped = line.strip()
        if stripped.startswith("```") or stripped.startswith("~~~"):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        match = HEADING_RE.match(line)
        if not match:
            continue
        slug = slugify_heading(match.group(2))
        if not slug:
            continue
        count = slug_counts.get(slug, 0)
        slug_counts[slug] = count + 1
        final_slug = slug if count == 0 else f"{slug}-{count}"
        slugs[final_slug] = line_no
    return slugs


def split_target(raw_target: str) -> tuple[str, str]:
    target = raw_target.strip()
    if "#" in target:
        path_part, fragment = target.split("#", 1)
        return path_part, fragment
    return target, ""


def is_external_target(target: str) -> bool:
    return target.startswith(("http://", "https://", "mailto:"))


def resolve_target(from_file: Path, path_part: str) -> Path:
    decoded = unquote(path_part)
    if not decoded:
        return from_file
    if decoded.startswith("/"):
        return (ROOT / decoded.lstrip("/")).resolve()
    return (from_file.parent / decoded).resolve()


def check_links(file_path: Path, heading_cache: Dict[Path, Dict[str, int]]) -> List[Issue]:
    issues: List[Issue] = []
    lines = file_path.read_text(encoding="utf-8").splitlines()
    for line_no, line in enumerate(lines, 1):
        for match in MARKDOWN_LINK_RE.finditer(line):
            raw_target = match.group(1).strip()
            if not raw_target or raw_target.startswith("#"):
                resolved = file_path
                fragment = raw_target[1:] if raw_target.startswith("#") else ""
            else:
                if is_external_target(raw_target):
                    continue
                path_part, fragment = split_target(raw_target)
                resolved = resolve_target(file_path, path_part)
                if not resolved.exists():
                    issues.append(
                        Issue(
                            kind="dead-link",
                            file_path=file_path,
                            line_no=line_no,
                            detail=f"{raw_target} -> {resolved}",
                        )
                    )
                    continue
            if not fragment:
                continue
            if resolved.suffix.lower() != ".md":
                continue
            anchors = heading_cache.setdefault(resolved, collect_heading_slugs(resolved))
            decoded_fragment = unquote(fragment)
            if decoded_fragment not in anchors:
                issues.append(
                    Issue(
                        kind="missing-anchor",
                        file_path=file_path,
                        line_no=line_no,
                        detail=f"{raw_target} -> #{decoded_fragment}",
                    )
                )
    return issues


def check_numbered_h2(file_path: Path) -> List[Issue]:
    issues: List[Issue] = []
    lines = file_path.read_text(encoding="utf-8").splitlines()
    numbered: List[tuple[int, int, str]] = []
    for line_no, line in enumerate(lines, 1):
        match = NUMBERED_H2_RE.match(line)
        if match:
            numbered.append((line_no, int(match.group(1)), line.strip()))
    if len(numbered) < 2:
        return issues
    previous = numbered[0][1]
    for line_no, current, raw in numbered[1:]:
        expected = previous + 1
        if current != expected:
            issues.append(
                Issue(
                    kind="bad-h2-numbering",
                    file_path=file_path,
                    line_no=line_no,
                    detail=f"expected {expected}, got {current}: {raw}",
                )
            )
        previous = current
    return issues


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Check docs hygiene for current markdown files")
    parser.add_argument(
        "--include-archive",
        action="store_true",
        help="also check docs/_archive (disabled by default)",
    )
    args = parser.parse_args(argv)

    files = list(iter_docs(include_archive=args.include_archive))
    heading_cache: Dict[Path, Dict[str, int]] = {}
    issues: List[Issue] = []
    for file_path in files:
        issues.extend(check_links(file_path, heading_cache))
        issues.extend(check_numbered_h2(file_path))

    if issues:
        print(f"docs hygiene failed: {len(issues)} issue(s)")
        for issue in issues:
            rel = issue.file_path.relative_to(ROOT)
            print(f"[{issue.kind}] {rel}:{issue.line_no}: {issue.detail}")
        return 1

    scope = "including docs/_archive" if args.include_archive else "excluding docs/_archive"
    print(f"docs hygiene OK: scanned {len(files)} markdown files ({scope})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
