#!/usr/bin/env python3
"""
Check hygiene for maintained repository markdown files.

Default scope covers repository markdown, including root and colocated README
files, and excludes docs/_archive because archive files are historical
reference material and are expected to age out.

Checks:
1. Relative markdown links resolve to existing files/directories.
2. Markdown fragment links resolve to real headings.
3. If a file uses numbered H2 headings (`## 1. ...`), the sequence is strictly
   increasing without duplicates or skips.
4. Active docs do not use consecutive blank lines, and headings / `---` rules
   are preceded by a blank line when they follow body text.
5. Tables are followed by a blank line before the next body block.
6. Standalone emphasis is not used as a heading; wrap-broken `* *` / ` - > `
   markup is rejected (and repaired by --fix).

`--fix` only repairs spacing/markup/heading shape. `--reflow` also wraps prose
to 120 characters. Both refuse to write when the content fingerprint would change.
"""
from __future__ import annotations

import argparse
import re
import sys
import unicodedata
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Iterable, List, Sequence, Tuple
from urllib.parse import unquote


ROOT = Path(__file__).resolve().parent.parent
DOCS_ROOT = ROOT / "docs"
# Runtime and generated artifacts are not maintained documentation. In
# particular tmp/ may contain copied Markdown evidence with intentionally stale
# links; scanning it makes the gate depend on local or prior CI residue.
IGNORED_DIR_NAMES = {".git", ".pytest_cache", "__pycache__", "node_modules", "tmp", "vendor"}
MARKDOWN_LINK_RE = re.compile(r"(?<!\!)\[[^\]]+\]\(([^)]+)\)")
NUMBERED_H2_RE = re.compile(r"^##\s+(\d+)\.\s+")
HEADING_RE = re.compile(r"^(#{1,6})\s+(.*?)\s*$")
HEADING_LINE_RE = re.compile(r"^#{1,6}\s+")
FENCE_RE = re.compile(r"^(```|~~~)")
TABLE_SEP_CELL_RE = re.compile(r"^:?-{3,}:?$")
EMPHASIS_HEADING_RE = re.compile(r"^\*\*([^*]+)\*\*\s*$")
COMPOUND_EMPHASIS_HEADING_RE = re.compile(r"^\*\*([^*]+)\*\*[：:]\s*\*\*([^*]+)\*\*\s*$")
BROKEN_ARROW = " - > "
BROKEN_BOLD = "* *"
REFLOW_WIDTH = 120
LINK_ATOM_RE = re.compile(r"!\[[^\]]*\]\([^)]+\)|\[[^\]]+\]\([^)]+\)|\[[^\]]+\]\[[^\]]*\]")
CODE_ATOM_RE = re.compile(r"`[^`]+`")
BOLD_ATOM_RE = re.compile(r"\*\*[^*]+\*\*")
URL_ATOM_RE = re.compile(r"https?://[^\s)>\]]+")
ASCII_ATOM_RE = re.compile(r"[A-Za-z0-9][A-Za-z0-9_./:+-]*")
HASH_REF_ATOM_RE = re.compile(r"#\d+")
WS_ATOM_RE = re.compile(r"[ \t]+")
LIST_LINE_RE = re.compile(r"^(\s*)([-*+] |\d+\. )(.*)$")
BLOCKQUOTE_LINE_RE = re.compile(r"^(\s*)>(\s?)(.*)$")
SENTENCE_END = set("。！？")
BREAK_AFTER = set("，。；：！？,;:）)]}>")
NO_START = set("，。；：、！？,;:）)]}/.")
CJK_WRAP_PUNCT = set("，。；：！？")
CLAUSE_END = set("。；！？")
CJK_GLUE_FOLLOW = set("《「『（(“\"'")
REFLOW_OVERFLOW = 24


@dataclass
class Issue:
    kind: str
    file_path: Path
    line_no: int
    detail: str


def iter_docs(include_archive: bool) -> Iterable[Path]:
    for path in sorted(ROOT.rglob("*.md")):
        relative = path.relative_to(ROOT)
        if any(part in IGNORED_DIR_NAMES for part in relative.parts):
            continue
        if not include_archive and DOCS_ROOT / "_archive" in path.parents:
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


def is_fence_line(line: str) -> bool:
    return bool(FENCE_RE.match(line.strip()))


def is_table_row(line: str) -> bool:
    return line.lstrip().startswith("|")


def check_spacing(file_path: Path) -> List[Issue]:
    issues: List[Issue] = []
    lines = file_path.read_text(encoding="utf-8").splitlines()
    in_fence = False
    blank_run = 0
    line_no = 1
    while line_no <= len(lines):
        line = lines[line_no - 1]
        if is_fence_line(line):
            in_fence = not in_fence
            blank_run = 0
            line_no += 1
            continue
        if in_fence:
            blank_run = 0
            line_no += 1
            continue
        if line.strip() == "":
            blank_run += 1
            if blank_run == 2:
                issues.append(
                    Issue(
                        kind="extra-blank-lines",
                        file_path=file_path,
                        line_no=line_no,
                        detail="consecutive blank lines outside fenced code",
                    )
                )
            line_no += 1
            continue
        prev = lines[line_no - 2] if line_no > 1 else ""
        if HEADING_LINE_RE.match(line) and line_no > 1 and prev.strip() != "" and not HEADING_LINE_RE.match(prev):
            issues.append(
                Issue(
                    kind="heading-spacing",
                    file_path=file_path,
                    line_no=line_no,
                    detail="heading should be preceded by a blank line",
                )
            )
        if line.strip() == "---" and line_no > 1 and prev.strip() != "":
            issues.append(
                Issue(
                    kind="hr-spacing",
                    file_path=file_path,
                    line_no=line_no,
                    detail="horizontal rule should be preceded by a blank line",
                )
            )
        if line_has_broken_markup(line, BROKEN_ARROW):
            issues.append(
                Issue(
                    kind="broken-arrow",
                    file_path=file_path,
                    line_no=line_no,
                    detail="wrap-broken arrow ` - > ` should be ` -> `",
                )
            )
        if line_has_broken_markup(line, BROKEN_BOLD):
            issues.append(
                Issue(
                    kind="broken-bold",
                    file_path=file_path,
                    line_no=line_no,
                    detail="wrap-broken bold `* *` should be `**`",
                )
            )
        stripped = line.strip()
        if not stripped.startswith("|") and (
            EMPHASIS_HEADING_RE.match(stripped) or COMPOUND_EMPHASIS_HEADING_RE.match(stripped)
        ):
            issues.append(
                Issue(
                    kind="emphasis-as-heading",
                    file_path=file_path,
                    line_no=line_no,
                    detail="standalone emphasis should be a real heading",
                )
            )
        if is_table_row(line):
            table_end = line_no
            while table_end < len(lines) and is_table_row(lines[table_end]):
                table_end += 1
            if table_end < len(lines):
                nxt = lines[table_end]
                if nxt.strip() != "" and not is_fence_line(nxt):
                    issues.append(
                        Issue(
                            kind="table-spacing",
                            file_path=file_path,
                            line_no=table_end + 1,
                            detail="table should be followed by a blank line",
                        )
                    )
            blank_run = 0
            line_no = table_end + 1
            continue
        blank_run = 0
        line_no += 1
    return issues


def compact_table_line(line: str) -> str:
    if not line.startswith("|"):
        return line
    raw_cells = line.split("|")
    inner = raw_cells[1:-1] if line.rstrip().endswith("|") else raw_cells[1:]
    compacted: List[str] = []
    for cell in inner:
        stripped = cell.strip()
        sep = stripped.replace(" ", "")
        if TABLE_SEP_CELL_RE.fullmatch(sep):
            left = sep.startswith(":")
            right = sep.endswith(":")
            if left and right:
                compacted.append(":---:")
            elif left:
                compacted.append(":---")
            elif right:
                compacted.append("---:")
            else:
                compacted.append("---")
        else:
            compacted.append(stripped)
    return "| " + " | ".join(compacted) + " |"


def iter_non_code_spans(line: str) -> Iterable[Tuple[int, int]]:
    """Yield [start, end) ranges outside inline `code` spans."""
    start = 0
    i = 0
    while i < len(line):
        if line[i] == "`":
            if start < i:
                yield start, i
            close = line.find("`", i + 1)
            if close < 0:
                return
            i = close + 1
            start = i
            continue
        i += 1
    if start < len(line):
        yield start, len(line)


def line_has_broken_markup(line: str, token: str) -> bool:
    for start, end in iter_non_code_spans(line):
        if token in line[start:end]:
            return True
    return False


def repair_markup_line(line: str) -> str:
    """Repair wrap-broken markup outside fenced/inline code. Does not reflow prose."""
    if BROKEN_ARROW not in line and BROKEN_BOLD not in line:
        return line
    pieces: List[str] = []
    last = 0
    for start, end in iter_non_code_spans(line):
        pieces.append(line[last:start])
        chunk = line[start:end].replace(BROKEN_ARROW, " -> ").replace(BROKEN_BOLD, "**")
        pieces.append(chunk)
        last = end
    pieces.append(line[last:])
    return "".join(pieces)


def convert_emphasis_heading_line(line: str, last_heading_level: int) -> Tuple[str | None, int]:
    stripped = line.strip()
    heading = HEADING_RE.match(line)
    if heading:
        return None, len(heading.group(1))
    compound = COMPOUND_EMPHASIS_HEADING_RE.match(stripped)
    if compound:
        level = min(last_heading_level + 1, 6)
        return f"{'#' * level} {compound.group(1).strip()}：{compound.group(2).strip()}", last_heading_level
    emph = EMPHASIS_HEADING_RE.match(stripped)
    if emph:
        inner = emph.group(1).strip()
        if inner and "*" not in inner:
            level = min(last_heading_level + 1, 6)
            return f"{'#' * level} {inner}", last_heading_level
    return None, last_heading_level


def content_fingerprint(text: str) -> str:
    """Compare information content while ignoring spacing and heading/emphasis shape."""
    pieces: List[str] = []
    in_fence = False
    for line in text.splitlines():
        if is_fence_line(line):
            in_fence = not in_fence
            pieces.append(line.strip())
            continue
        if in_fence:
            pieces.append(line)
            continue
        line = repair_markup_line(line)
        line = re.sub(r"^#{1,6}\s+", "", line)
        line = re.sub(r"^>\s?", "", line)
        converted, _ = convert_emphasis_heading_line(line, 1)
        if converted is not None:
            line = re.sub(r"^#{1,6}\s+", "", converted)
        line = re.sub(r"\*\*([^*]+)\*\*", r"\1", line)
        line = line.replace("`", "")
        pieces.append(line)
    compact = re.sub(r"\s+", "", "".join(pieces))
    return compact.replace("- >", "->")


def ensure_blank_after_tables(lines: Sequence[str]) -> List[str]:
    out: List[str] = []
    in_fence = False
    i = 0
    while i < len(lines):
        line = lines[i]
        if is_fence_line(line):
            in_fence = not in_fence
            out.append(line)
            i += 1
            continue
        if in_fence:
            out.append(line)
            i += 1
            continue
        if is_table_row(line):
            out.append(line)
            i += 1
            while i < len(lines) and is_table_row(lines[i]):
                out.append(lines[i])
                i += 1
            if i < len(lines) and lines[i].strip() != "" and not is_fence_line(lines[i]):
                out.append("")
            continue
        out.append(line)
        i += 1
    return out


def is_hr_line(line: str) -> bool:
    return line.strip() in {"---", "***", "___", "* * *"}


def is_passthrough_line(line: str) -> bool:
    stripped = line.lstrip()
    return stripped.startswith("<!--") or line.startswith("    ")


def has_hard_break(line: str) -> bool:
    return bool(line) and line.endswith("  ") and line.strip() != ""


def is_cjk(ch: str) -> bool:
    return "\u4e00" <= ch <= "\u9fff"


def extract_wrap_atoms(text: str) -> List[str]:
    atoms: List[str] = []
    i = 0
    n = len(text)
    while i < n:
        if text.startswith("->", i):
            atoms.append("->")
            i += 2
            continue
        matched = HASH_REF_ATOM_RE.match(text, i)
        if matched:
            atoms.append(matched.group(0))
            i = matched.end()
            continue
        for pattern in (LINK_ATOM_RE, CODE_ATOM_RE, BOLD_ATOM_RE, URL_ATOM_RE):
            matched = pattern.match(text, i)
            if matched:
                atoms.append(matched.group(0))
                i = matched.end()
                break
        else:
            matched = ASCII_ATOM_RE.match(text, i)
            if matched:
                atoms.append(matched.group(0))
                i = matched.end()
                continue
            matched = WS_ATOM_RE.match(text, i)
            if matched:
                atoms.append(matched.group(0))
                i = matched.end()
                continue
            atoms.append(text[i])
            i += 1
    return atoms


def smart_join(parts: Sequence[str]) -> str:
    out = ""
    for part in parts:
        piece = re.sub(r"\s+", " ", part.strip())
        if not piece:
            continue
        if not out:
            out = piece
            continue
        prev, nxt = out[-1], piece[0]
        unmatched_code = out.count("`") % 2 == 1
        if unmatched_code or prev in "/（([" or nxt in "/），。；：、！？,;:）)]":
            out += piece
        elif prev in "，。；：、！？":
            out += piece
        elif is_cjk(prev) and (is_cjk(nxt) or nxt in CJK_GLUE_FOLLOW):
            out += piece
        else:
            out += " " + piece
    return out


def wrap_text(text: str, first_prefix: str, next_prefix: str, width: int = REFLOW_WIDTH) -> List[str]:
    text = re.sub(r"\s+", " ", text).strip()
    if not text:
        return [first_prefix.rstrip()] if first_prefix.strip() else [""]
    atoms = extract_wrap_atoms(text)
    lines: List[str] = []
    prefix = first_prefix
    current = prefix
    last_break = len(prefix)

    def flush(upto: int | None = None) -> None:
        nonlocal current, last_break, prefix
        chunk = current if upto is None else current[:upto]
        chunk = chunk.rstrip()
        if chunk == prefix.rstrip() and prefix.strip() in {">", "> "}:
            lines.append(prefix.rstrip())
        elif chunk:
            lines.append(chunk)
        prefix = next_prefix
        current = prefix
        last_break = len(prefix)

    i = 0
    while i < len(atoms):
        atom = atoms[i]
        if current == prefix and atom.isspace():
            i += 1
            continue
        trial = current + atom
        if len(trial) <= width or (current == prefix and not atom.isspace()):
            current = trial
            can_break = atom.isspace() or atom[-1] in BREAK_AFTER
            nxt = atoms[i + 1] if i + 1 < len(atoms) else ""
            if nxt.startswith("#") or (not nxt.isspace() and nxt[:1] in NO_START):
                can_break = False
            if can_break:
                last_break = len(current)
            i += 1
            if current == prefix:
                last_break = len(prefix)
            continue
        if atom[:1] in "/（([" or current.rstrip()[-1:] in "/（([":
            current = trial
            i += 1
            continue
        lookahead = current
        j = i
        clause_at = 0
        while j < len(atoms) and len(lookahead) <= width + REFLOW_OVERFLOW:
            lookahead += atoms[j]
            if atoms[j][-1:] in CLAUSE_END:
                clause_at = len(lookahead)
                break
            j += 1
        if clause_at and clause_at <= width + REFLOW_OVERFLOW:
            while i < len(atoms) and len(current) < clause_at:
                current += atoms[i]
                i += 1
            last_break = len(current)
            continue
        break_at = last_break
        min_fill = max(len(prefix) + 40, int(width * 0.55))
        cjk_pos = 0
        for idx, ch in enumerate(current):
            if idx >= len(prefix) and ch in CJK_WRAP_PUNCT:
                cjk_pos = idx + 1
        if cjk_pos >= min_fill:
            break_at = cjk_pos
        if break_at > len(prefix):
            rest = current[break_at:]
            flush(break_at)
            current = prefix + rest.lstrip()
            last_break = len(prefix)
            if current[-1:] in BREAK_AFTER or current.endswith(" "):
                last_break = len(current)
            continue
        if current == prefix:
            current = trial
            i += 1
            flush()
            continue
        flush()
    if current.strip() or (current == prefix and first_prefix.strip() in {">"}):
        tail = current.rstrip()
        if tail:
            lines.append(tail)
    return lines or [first_prefix.rstrip() if first_prefix.strip() else ""]


def reflow_markdown(text: str) -> str:
    """Wrap prose to REFLOW_WIDTH. Fences, tables, headings, HTML comments stay verbatim."""
    ends_nl = text.endswith("\n")
    raw_lines = ensure_blank_after_tables(text.splitlines())
    out: List[str] = []
    i = 0
    in_code = False
    n = len(raw_lines)

    def flush_para(buf: Sequence[str], first_prefix: str, next_prefix: str) -> None:
        joined = smart_join(buf)
        if not joined and not first_prefix.strip():
            return
        out.extend(wrap_text(joined, first_prefix, next_prefix))

    while i < n:
        line = raw_lines[i]
        if is_fence_line(line):
            in_code = not in_code
            out.append(line)
            i += 1
            continue
        if in_code:
            out.append(line)
            i += 1
            continue
        if line.strip() == "":
            out.append("")
            i += 1
            continue
        if HEADING_LINE_RE.match(line) or line.startswith("#") or is_hr_line(line) or is_passthrough_line(line):
            out.append(line)
            i += 1
            continue
        if has_hard_break(line):
            flush_para([line.rstrip()], "", "")
            if out:
                out[-1] = out[-1].rstrip() + "  "
            i += 1
            continue
        if is_table_row(line):
            out.append(line)
            i += 1
            continue
        quote = BLOCKQUOTE_LINE_RE.match(line)
        if quote and not line.lstrip().startswith(">>"):
            while i < n:
                matched = BLOCKQUOTE_LINE_RE.match(raw_lines[i])
                if not matched:
                    break
                body = matched.group(3)
                if body.strip() == "":
                    out.append(">")
                    i += 1
                    continue
                parts = [body]
                i += 1
                while i < n:
                    nested = BLOCKQUOTE_LINE_RE.match(raw_lines[i])
                    if not nested:
                        break
                    body2 = nested.group(3)
                    if body2.strip() == "":
                        break
                    prev = parts[-1].rstrip()
                    wrap_cont = prev[-1:] in "，、；：/" or (
                        len(prev) >= 80 and prev[-1:] not in SENTENCE_END
                    )
                    if not wrap_cont:
                        break
                    parts.append(body2)
                    i += 1
                flush_para(parts, "> ", "> ")
            continue
        listed = LIST_LINE_RE.match(line)
        if listed:
            while i < n:
                matched = LIST_LINE_RE.match(raw_lines[i])
                if not matched:
                    break
                indent, marker, body = matched.group(1), matched.group(2), matched.group(3)
                item_parts = [body]
                i += 1
                hang = indent + " " * len(marker)
                while i < n:
                    nxt = raw_lines[i]
                    if nxt.strip() == "":
                        break
                    if HEADING_LINE_RE.match(nxt) or is_fence_line(nxt) or is_table_row(nxt) or is_hr_line(nxt):
                        break
                    if LIST_LINE_RE.match(nxt) or BLOCKQUOTE_LINE_RE.match(nxt):
                        break
                    if nxt.startswith(hang) or (indent and nxt.startswith(indent + " ")):
                        item_parts.append(nxt.strip())
                        i += 1
                        continue
                    break
                flush_para(item_parts, indent + marker, hang)
            continue
        parts = [line.strip()]
        i += 1
        while i < n:
            nxt = raw_lines[i]
            if nxt.strip() == "" or has_hard_break(nxt) or is_passthrough_line(nxt) or is_hr_line(nxt):
                break
            if HEADING_LINE_RE.match(nxt) or is_fence_line(nxt) or is_table_row(nxt):
                break
            if LIST_LINE_RE.match(nxt) or BLOCKQUOTE_LINE_RE.match(nxt):
                break
            if nxt.startswith("    "):
                break
            parts.append(nxt.strip())
            i += 1
        flush_para(parts, "", "")

    cleaned: List[str] = []
    blank = 0
    for line in out:
        if line.strip() == "":
            blank += 1
            if blank <= 1:
                cleaned.append("")
            continue
        blank = 0
        cleaned.append(line)
    while cleaned and cleaned[-1] == "":
        cleaned.pop()
    result = "\n".join(cleaned)
    if ends_nl:
        result += "\n"
    return result


def format_docs_markdown(text: str, *, compact_tables: bool = False, reflow: bool = False) -> str:
    updated = normalize_markdown(text, compact_tables=compact_tables)
    if not reflow:
        return updated
    updated = reflow_markdown(updated)
    return normalize_markdown(updated, compact_tables=compact_tables)


def normalize_markdown(text: str, *, compact_tables: bool = False) -> str:
    lines = text.splitlines()
    in_fence = False
    last_heading_level = 1
    repaired: List[str] = []
    for line in lines:
        if is_fence_line(line):
            in_fence = not in_fence
            repaired.append(line)
            continue
        if in_fence:
            repaired.append(line)
            continue
        line = repair_markup_line(line)
        converted, last_heading_level = convert_emphasis_heading_line(line, last_heading_level)
        if converted is not None:
            repaired.append(converted)
            continue
        repaired.append(line)
    lines = ensure_blank_after_tables(repaired)

    in_fence = False
    normalized: List[str] = []
    for line in lines:
        if is_fence_line(line):
            in_fence = not in_fence
            normalized.append(line)
            continue
        if in_fence:
            normalized.append(line)
            continue
        if compact_tables and line.startswith("|"):
            line = compact_table_line(line)
        if line.strip() == "":
            if normalized and normalized[-1] == "":
                continue
            normalized.append("")
            continue
        if normalized and normalized[-1] != "":
            prev = normalized[-1]
            if HEADING_LINE_RE.match(line) and not HEADING_LINE_RE.match(prev):
                normalized.append("")
            elif line.strip() == "---":
                normalized.append("")
        normalized.append(line)
    while normalized and normalized[0] == "":
        normalized.pop(0)
    while normalized and normalized[-1] == "":
        normalized.pop()
    return ("\n".join(normalized) + "\n") if normalized else ""


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
    parser = argparse.ArgumentParser(description="Check hygiene for maintained repository markdown files")
    parser.add_argument(
        "--include-archive",
        action="store_true",
        help="also check docs/_archive (disabled by default)",
    )
    parser.add_argument(
        "--fix",
        action="store_true",
        help=(
            "normalize docs/ spacing, table gaps, wrap-broken markup, and "
            "emphasis-as-heading; refuses writes that change content fingerprint"
        ),
    )
    parser.add_argument(
        "--reflow",
        action="store_true",
        help="also wrap docs/ prose to 120 characters; requires fingerprint to stay unchanged",
    )
    args = parser.parse_args(argv)

    files = list(iter_docs(include_archive=args.include_archive))
    heading_cache: Dict[Path, Dict[str, int]] = {}
    issues: List[Issue] = []
    if args.fix or args.reflow:
        for file_path in files:
            if DOCS_ROOT not in file_path.parents and file_path.parent != DOCS_ROOT:
                continue
            original = file_path.read_text(encoding="utf-8")
            compact_tables = file_path.name == "90-设计问题与重构清单.md" and "40-interpretation" in file_path.parts
            updated = format_docs_markdown(
                original,
                compact_tables=compact_tables,
                reflow=args.reflow,
            )
            if updated == original:
                continue
            if content_fingerprint(original) != content_fingerprint(updated):
                issues.append(
                    Issue(
                        kind="content-fingerprint",
                        file_path=file_path,
                        line_no=1,
                        detail="write refused because content fingerprint would change",
                    )
                )
                continue
            file_path.write_text(updated, encoding="utf-8")
    for file_path in files:
        issues.extend(check_links(file_path, heading_cache))
        issues.extend(check_numbered_h2(file_path))
        if DOCS_ROOT in file_path.parents or file_path.parent == DOCS_ROOT:
            issues.extend(check_spacing(file_path))

    if issues:
        print(f"docs hygiene failed: {len(issues)} issue(s)")
        for issue in issues:
            rel = issue.file_path.relative_to(ROOT)
            print(f"[{issue.kind}] {rel}:{issue.line_no}: {issue.detail}")
        return 1

    scope = "including docs/_archive" if args.include_archive else "repository markdown excluding docs/_archive"
    print(f"docs hygiene OK: scanned {len(files)} markdown files ({scope})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
