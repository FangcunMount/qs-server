#!/usr/bin/env python3
"""Check low-cost facts and boundaries for the active documentation tree.

This complements check_docs_hygiene.py. It deliberately checks only facts that
can be derived cheaply and deterministically from the repository; prose still
requires code review when behavior changes.
"""
from __future__ import annotations

import json
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
VERSION_LEDGER = DOCS / "00-总览/09-当前版本定档验收台账.md"
PRODUCTION_CONFIG = ROOT / "configs/apiserver.prod.yaml"
DEVELOPMENT_CONFIG = ROOT / "configs/apiserver.dev.yaml"
SCHEDULER_BOOTSTRAP = ROOT / "internal/apiserver/process/runtime_bootstrap.go"
PERF_PLAN_SOURCE = ROOT / "scripts/perf/perfctl/plan.go"
PERF_RUNNER_SOURCE = ROOT / "scripts/perf/perfctl/runner.go"
PERF_CONFIG = ROOT / "scripts/perf/qs-perf.config.example.json"
PERF_README = ROOT / "scripts/perf/README.md"
PERF_SOP = DOCS / "04-接口与运维/11-300QPS混合场景压测SOP.md"
RUNTIME_SCHEDULER_DOC = DOCS / "01-运行时/06-后台任务与调度.md"
OPS_SCHEDULER_DOC = DOCS / "04-接口与运维/07-调度任务.md"
MONGO_AUDIT_SOURCE = ROOT / "internal/apiserver/application/mongoconsistency/audit.go"
MONGO_AUDIT_METRICS_SOURCE = ROOT / "internal/apiserver/application/mongoconsistency/metrics.go"
APISERVER_OPTIONS_SOURCE = ROOT / "internal/apiserver/options/options.go"
MONGO_AUDIT_METRICS_DOC = DOCS / "04-接口与运维/08-健康检查与观测.md"
IR_CHECKLIST = DOCS / "02-业务模块/40-interpretation/90-设计问题与重构清单.md"
IR_R001_RECORD = DOCS / "02-业务模块/40-interpretation/91-IR-R001-AssessmentOwnership闭环记录.md"
SWAGGER_CONTRACTS = {
    "apiserver": ROOT / "internal/apiserver/docs/swagger.json",
    "collection": ROOT / "internal/collection-server/docs/swagger.json",
}

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

SCHEDULER_RUNNERS = (
    ("PlanRunner", "PlanScheduler", "plan_scheduler", "Plan"),
    ("StatisticsSyncRunner", "StatisticsSync", "statistics_sync", "StatisticsSync"),
    (
        "EvaluationConsistencyAuditRunner",
        "EvaluationConsistencyAudit",
        "evaluation_consistency_audit",
        "EvaluationConsistencyAudit",
    ),
    (
        "EvaluationLeaseRecoveryRunner",
        "EvaluationLeaseRecovery",
        "evaluation_lease_recovery",
        "EvaluationLeaseRecovery",
    ),
    (
        "InterpretationLeaseRecoveryRunner",
        "InterpretationLeaseRecovery",
        "interpretation_lease_recovery",
        "InterpretationLeaseRecovery",
    ),
    ("ReportCatalogAuditRunner", "ReportCatalogAudit", "report_catalog_audit", "ReportCatalogAudit"),
    ("MongoConsistencyAuditRunner", "MongoConsistencyAudit", "mongo_consistency_audit", "MongoConsistencyAudit"),
)

# These are safety budgets, not target sizes. The old 120-file global limit
# forced evidence-rich modules into oversized README files. Keep a wider
# repository guardrail and a per-module guardrail so docs can be split by
# responsibility without allowing one module to grow without review.
MAX_ACTIVE_MARKDOWN = 150
MAX_BUSINESS_MODULE_MARKDOWN = 18

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
    DEVELOPMENT_CONFIG,
    PRODUCTION_CONFIG,
    EVENTS,
    SIGNALS,
    SCHEDULER_BOOTSTRAP,
    PERF_PLAN_SOURCE,
    PERF_RUNNER_SOURCE,
    PERF_CONFIG,
    PERF_README,
    PERF_SOP,
    MONGO_AUDIT_SOURCE,
    MONGO_AUDIT_METRICS_SOURCE,
    APISERVER_OPTIONS_SOURCE,
}

STALE_PATTERNS = {
    "legacy assessment.submitted event": re.compile(r"(?<![\w.])assessment\.submitted(?![\w.])"),
    "legacy assessment.evaluated event": re.compile(r"(?<![\w.])assessment\.evaluated(?![\w.])"),
    "legacy assessment.interpreted event": re.compile(r"(?<![\w.])assessment\.interpreted(?![\w.])"),
    "unqualified report.generated event": re.compile(r"(?<!interpretation\.)(?<![\w.])report\.generated(?![\w.])"),
}

STALE_TAXONOMY_PATTERNS = {
    "retired docs/05-专题分析 taxonomy": re.compile(r"docs/05-专题分析(?:/|`|\b)"),
}

INLINE_CODE_RE = re.compile(r"(?<!`)`([^`\n]+)`(?!`)")
SOURCE_PATH_PREFIXES = (
    ".github/",
    "api/",
    "cmd/",
    "configs/",
    "deployments/",
    "docs/",
    "internal/",
    "pkg/",
    "scripts/",
)
SOURCE_PATH_FILES = {"Dockerfile", "Makefile", "go.mod", "go.sum"}
IGNORED_MARKDOWN_DIR_NAMES = {".git", "node_modules", "vendor", "tmp"}

# High-risk sidecar and design facts that previously drifted away from the
# current configuration/migration/storage implementation. These are narrow
# ratchets: if the implementation intentionally changes, update the source,
# documentation and this contract in the same change.
CURRENT_FACT_SNIPPETS = {
    ROOT / "configs/env/README.md": {
        "required": (
            "`Makefile` 不会自动读取本目录下的 `.env` 文件",
            "`QS_APISERVER_`",
            "`COLLECTION_SERVER_`",
            "`QS_WORKER_`",
        ),
        "forbidden": (
            "Makefile 已配置自动加载环境变量",
            "go get github.com/joho/godotenv",
            "make config-check",
        ),
    },
    ROOT / "internal/pkg/migration/README.md": {
        "required": (
            "迁移版本机制只保证同一版本不会被正常重复执行，不保证迁移无损",
            "应用内 `Migrator` 当前没有 `Rollback()` 方法",
            "dirty 状态会阻断继续迁移",
        ),
        "forbidden": (
            "NewMigratorWithDriver",
            "migrator.Rollback()",
            "迁移不会覆盖数据",
            "不会删除或覆盖现有数据",
        ),
    },
    DOCS / "02-业务模块/20-model-catalog/26-核心设计-数据存储与一致性.md": {
        "required": (
            "| Questionnaire head CAS | 已实现 |",
            "| 标准 migration 对齐 unified schema | 已实现 |",
            "| 服务启动关键索引验证 | 已实现 |",
            "| Norm table_version unique index | 已实现 |",
            "| active pair 与 runtime 周期一致性审计 | 已实现/生产关闭 |",
        ),
        "forbidden": (
            "Questionnaire 并发编辑当前是 last-write-wins",
            "| Questionnaire head CAS | 未实现 |",
            "| 标准 migration 对齐 unified schema | **未完成** |",
            "| active pair 周期一致性审计 | 未实现 |",
            "生产 dirty@13 恢复（当前阻塞）",
        ),
    },
    VERSION_LEDGER: {
        "required": (
            "| 当前 checkout SHA |",
            "| 最后通过 CI 的 SHA |",
            "| 实际部署 SHA |",
            "| 生产证据时间 |",
            "| Mongo consistency 生产证据 |",
        ),
        "forbidden": (
            "apiserver 174 paths",
            "collection 45 paths",
        ),
    },
    DOCS / "01-运行时/06-后台任务与调度.md": {
        "required": (
            "`answersheet_outbox`",
            "`outbox_answersheet`",
            "`generation_run`",
            "`generated_terminal`",
            "`retry_outbox`",
            "`model_release`",
            "`published_model_runtime`",
            "`mongo_consistency_audit_checkpoint`",
            "checkpoint CAS conflict",
        ),
        "forbidden": (),
    },
    DOCS / "03-基础设施/data-access/README.md": {
        "required": (
            "Mongo 业务事实只读",
            "`CheckpointStore` 是唯一写端口",
            "`Save(expectedRevision, next)`",
        ),
        "forbidden": (),
    },
    DOCS / "04-接口与运维/07-调度任务.md": {
        "required": (
            "退出码 2",
            "MySQL migration 000069",
            "Mongo migration 000024",
            "Redis leader lock",
        ),
        "forbidden": (),
    },
    DOCS / "04-接口与运维/08-健康检查与观测.md": {
        "required": (
            "`qs_mongo_consistency_audit_enabled`",
            "`qs_mongo_consistency_audit_ready`",
            "`qs_mongo_consistency_audit_last_success_timestamp_seconds`",
            "`qs_mongo_consistency_audit_drift{severity,kind}`",
            "`qs_mongo_consistency_audit_errors_total{stage}`",
            "`qs_mongo_consistency_audit_checkpoint_cas_conflicts_total`",
            "`qs_mongo_consistency_audit_batches_total{phase,outcome}`",
            "`qs_mongo_consistency_audit_batch_duration_seconds{phase,outcome}`",
            "`qs_mongo_consistency_audit_scanned_total{phase}`",
        ),
        "forbidden": (),
    },
    IR_R001_RECORD: {
        "required": (
            "状态：**已关闭**",
            "`report_status_assessment_ownership_total{result}`",
            "`internal/collection-server/application/reportwait/service.go`",
            "`api/grpc/proto/evaluation/evaluation.proto`",
            "`configs/grpc-acl.prod.yaml`",
        ),
        "forbidden": (
            "状态：已实现待验收",
            "## 剩余验收",
        ),
    },
    PERF_SOP: {
        "required": (
            "make perf-run PLAN=ceiling-120",
            "### Ceiling-120",
            "`capacity_110`",
            "`capacity_120`",
            "不会进入 200/240/280/300",
        ),
        "forbidden": (),
    },
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


def maintained_markdown() -> Iterable[Path]:
    for path in sorted(ROOT.rglob("*.md")):
        relative = path.relative_to(ROOT)
        if any(part in IGNORED_MARKDOWN_DIR_NAMES for part in relative.parts):
            continue
        if ARCHIVE in path.parents:
            continue
        yield path


def source_path_candidates(text: str) -> Iterable[tuple[str, int]]:
    """Yield repository-relative source paths written as inline code.

    Only source-like prefixes are considered. Runtime artifacts such as
    tmp/perf/runs/<run-id>/ and identifiers such as report_status are outside
    this ratchet by design.
    """

    for match in INLINE_CODE_RE.finditer(text):
        for raw_candidate in re.split(r"[\s、，；;]+", match.group(1).strip()):
            candidate = raw_candidate.strip("()[]\"'。，；：")
            if not candidate:
                continue
            if candidate not in SOURCE_PATH_FILES and not candidate.startswith(SOURCE_PATH_PREFIXES):
                continue
            candidate = re.sub(r"#.*$", "", candidate)
            candidate = re.sub(r":\d+(?::\d+)?$", "", candidate)
            candidate = re.sub(r"\.(?:go|md|yaml|yml|json|sh|js|ts):[A-Za-z_][A-Za-z0-9_.]*$", lambda m: m.group(0).split(":", 1)[0], candidate)
            yield candidate.rstrip("/"), match.start()


def source_path_exists(candidate: str) -> bool:
    if not candidate:
        return False
    if candidate.endswith("/..."):
        candidate = candidate[:-4]
    placeholder = re.search(r"<[^>]+>", candidate)
    if placeholder:
        static_parent = candidate[: placeholder.start()].rstrip("/")
        return bool(static_parent) and (ROOT / static_parent).exists()
    brace = re.search(r"\{([^{}]+)\}", candidate)
    if brace:
        variants = [
            candidate[: brace.start()] + value + candidate[brace.end() :]
            for value in brace.group(1).split(",")
        ]
        return all(source_path_exists(variant) for variant in variants)
    if "|" in candidate:
        prefix, alternatives = candidate.rsplit("/", 1) if "/" in candidate else ("", candidate)
        variants = [f"{prefix}/{value}" if prefix else value for value in alternatives.split("|")]
        return all(source_path_exists(variant) for variant in variants)
    if any(char in candidate for char in "*?["):
        return any(ROOT.glob(candidate))
    if (ROOT / candidate).exists():
        return True
    symbol_base = re.sub(r"(?:\.[A-Z][A-Za-z0-9_]*)+$", "", candidate)
    return symbol_base != candidate and (ROOT / symbol_base).exists()


def yaml_block_values(text: str, section: str) -> dict[str, str]:
    block = re.search(
        rf"(?m)^{re.escape(section)}:\s*(?:#.*)?\n(?P<body>(?:^[ \t]+.*(?:\n|$))*)",
        text,
    )
    if not block:
        return {}
    values: dict[str, str] = {}
    for line in block.group("body").splitlines():
        setting = re.match(r"^\s+([a-z_]+):\s*(.*?)\s*(?:#.*)?$", line)
        if setting:
            values[setting.group(1)] = setting.group(2).strip()
    return values


def scheduler_registrations(text: str) -> list[tuple[str, str]]:
    return re.findall(
        r"runtimescheduler\.New([A-Za-z]+Runner)\(\s*cfg\.([A-Za-z]+)",
        text,
        flags=re.DOTALL,
    )


def mongo_audit_phases(text: str) -> list[str]:
    constant_values = dict(
        re.findall(r'\b(Phase[A-Za-z]+)\s+Phase\s*=\s*"([a-z_]+)"', text)
    )
    block = re.search(
        r"var AuditPhases = \[\]Phase\{(?P<body>.*?)\n\}",
        text,
        flags=re.DOTALL,
    )
    if not block:
        return []
    constants = re.findall(r"\b(Phase[A-Za-z]+)\b", block.group("body"))
    return [constant_values[name] for name in constants if name in constant_values]


def interface_methods(text: str, interface_name: str) -> set[str]:
    block = re.search(
        rf"type {re.escape(interface_name)} interface \{{(?P<body>.*?)\n\}}",
        text,
        flags=re.DOTALL,
    )
    if not block:
        return set()
    return set(re.findall(r"(?m)^\s*([A-Z][A-Za-z0-9_]*)\(", block.group("body")))


def performance_plan_names(plan_text: str, runner_text: str) -> list[str]:
    plans = re.findall(r'case "([a-z0-9-]+)":', plan_text)
    if 'opts.Plan == "diagnose"' in runner_text:
        plans.append("diagnose")
    return plans


def performance_phase_specs(text: str) -> list[tuple[str, str, int, str, bool]]:
    specs: list[tuple[str, str, int, str, bool]] = []
    for match in re.finditer(r"\{(?P<body>\s*ID:\s*\"[^\"]+\"[^}]*)\}", text):
        body = match.group("body")
        phase_id = re.search(r'\bID:\s*"([^"]+)"', body)
        profile = re.search(r'\bProfile:\s*"([^"]+)"', body)
        target = re.search(r"\bTargetQPS:\s*(\d+)", body)
        duration = re.search(r'\bDuration:\s*"([^"]+)"', body)
        if not all((phase_id, profile, target, duration)):
            continue
        specs.append(
            (
                phase_id.group(1),
                profile.group(1),
                int(target.group(1)),
                duration.group(1),
                "Dynamic: true" in body,
            )
        )
    return specs


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
        module_dir = DOCS / "02-业务模块" / directory
        readme = module_dir / "README.md"
        if not readme.exists():
            issues.append(Issue("missing-module-readme", f"{package}: {readme.relative_to(ROOT)}"))
        module_files = list(module_dir.rglob("*.md")) if module_dir.exists() else []
        if len(module_files) > MAX_BUSINESS_MODULE_MARKDOWN:
            issues.append(
                Issue(
                    "business-module-doc-tree-too-large",
                    f"{package}: {len(module_files)} files; limit is {MAX_BUSINESS_MODULE_MARKDOWN}",
                )
            )

    event_text = EVENTS.read_text(encoding="utf-8")
    configured_events = set(re.findall(r"^  ([a-z0-9_.]+):\s*$", event_text, flags=re.MULTILINE))
    for event_name in sorted(REQUIRED_EVENTS - configured_events):
        issues.append(Issue("missing-event-contract", event_name))

    signal_text = SIGNALS.read_text(encoding="utf-8")
    configured_signals = set(re.findall(r"^  ([a-z0-9_]+):\s*$", signal_text, flags=re.MULTILINE))
    for signal_name in sorted(REQUIRED_SIGNALS - configured_signals):
        issues.append(Issue("missing-signal-contract", signal_name))

    files = list(active_markdown())
    if len(files) > MAX_ACTIVE_MARKDOWN:
        issues.append(
            Issue(
                "active-doc-tree-too-large",
                f"{len(files)} files; limit is {MAX_ACTIVE_MARKDOWN}",
            )
        )

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
        for label, pattern in STALE_TAXONOMY_PATTERNS.items():
            match = pattern.search(text)
            if match:
                issues.append(
                    Issue(
                        "stale-doc-taxonomy",
                        f"{path.relative_to(ROOT)}:{line_for_offset(text, match.start())}: {label}",
                    )
                )

    for path in maintained_markdown():
        text = path.read_text(encoding="utf-8")
        for candidate, offset in source_path_candidates(text):
            if source_path_exists(candidate):
                continue
            issues.append(
                Issue(
                    "missing-backtick-source-path",
                    f"{path.relative_to(ROOT)}:{line_for_offset(text, offset)}: {candidate}",
                )
            )

    for path, contract in CURRENT_FACT_SNIPPETS.items():
        if not path.exists():
            issues.append(Issue("missing-current-fact-doc", str(path.relative_to(ROOT))))
            continue
        text = path.read_text(encoding="utf-8")
        for snippet in contract["required"]:
            if snippet not in text:
                issues.append(
                    Issue(
                        "missing-current-fact",
                        f"{path.relative_to(ROOT)}: {snippet}",
                    )
                )
        for snippet in contract["forbidden"]:
            if snippet in text:
                issues.append(
                    Issue(
                        "stale-current-fact",
                        f"{path.relative_to(ROOT)}: {snippet}",
                    )
                )

    # Scheduler inventory is derived from the composition root, then checked
    # against both versioned configs and the runtime/operations inventories.
    if SCHEDULER_BOOTSTRAP.exists():
        scheduler_source = SCHEDULER_BOOTSTRAP.read_text(encoding="utf-8")
        actual_registrations = scheduler_registrations(scheduler_source)
        expected_registrations = [(constructor, option) for constructor, option, _, _ in SCHEDULER_RUNNERS]
        if actual_registrations != expected_registrations:
            issues.append(
                Issue(
                    "scheduler-composition-drift",
                    f"source={actual_registrations}, expected={expected_registrations}",
                )
            )
        runtime_scheduler_text = RUNTIME_SCHEDULER_DOC.read_text(encoding="utf-8")
        ops_scheduler_text = OPS_SCHEDULER_DOC.read_text(encoding="utf-8")
        for constructor, _, _, ops_label in SCHEDULER_RUNNERS:
            if constructor not in runtime_scheduler_text:
                issues.append(Issue("scheduler-runtime-doc-drift", constructor))
            if f"| {ops_label} |" not in ops_scheduler_text:
                issues.append(Issue("scheduler-ops-doc-drift", ops_label))

        for config_path in (DEVELOPMENT_CONFIG, PRODUCTION_CONFIG):
            config_text = config_path.read_text(encoding="utf-8")
            top_level_sections = set(re.findall(r"(?m)^([a-z_][a-z0-9_]*):\s*(?:#.*)?$", config_text))
            for _, _, config_key, _ in SCHEDULER_RUNNERS:
                if config_key not in top_level_sections:
                    issues.append(
                        Issue(
                            "scheduler-config-inventory-drift",
                            f"{config_path.relative_to(ROOT)}: {config_key}",
                        )
                    )

        production_text = PRODUCTION_CONFIG.read_text(encoding="utf-8")
        for _, _, config_key, ops_label in SCHEDULER_RUNNERS:
            enabled = yaml_block_values(production_text, config_key).get("enable")
            if enabled not in {"true", "false"}:
                issues.append(
                    Issue(
                        "scheduler-production-enable-missing",
                        f"{config_key}: {enabled!r}",
                    )
                )
                continue
            intent = "启用" if enabled == "true" else "关闭"
            row = re.compile(
                rf"(?m)^\|\s*{re.escape(ops_label)}\s*\|[^\n]*\|\s*(?:\*\*)?{intent}(?:\*\*)?\s*\|$"
            )
            if not row.search(ops_scheduler_text):
                issues.append(
                    Issue(
                        "scheduler-production-status-doc-drift",
                        f"{ops_label}: expected {intent} from {config_key}.enable={enabled}",
                    )
                )
    else:
        issues.append(Issue("missing-scheduler-bootstrap", str(SCHEDULER_BOOTSTRAP.relative_to(ROOT))))

    # IR-R001 is closed in the active checklist. Its dedicated companion must
    # remain a closure record rather than silently reverting to an open plan.
    if IR_CHECKLIST.exists():
        ir_text = IR_CHECKLIST.read_text(encoding="utf-8")
        if not re.search(
            r"(?m)^\| IR-R001 \| P0 \|[^\n]*\| 已发布 \| 已关闭 \|",
            ir_text,
        ):
            issues.append(Issue("ir-r001-status-drift", "checklist must remain 已发布 / 已关闭"))
    retired_ir_plan = DOCS / "02-业务模块/40-interpretation/91-IR-R001-AssessmentOwnership实施计划.md"
    if retired_ir_plan.exists():
        issues.append(Issue("retired-ir-r001-plan-returned", str(retired_ir_plan.relative_to(ROOT))))

    # Mongo audit phases, read-only ports, config defaults and metrics are all
    # source-derived so adding a phase or metric requires the runbooks to move
    # in the same change.
    if MONGO_AUDIT_SOURCE.exists():
        mongo_source = MONGO_AUDIT_SOURCE.read_text(encoding="utf-8")
        phases = mongo_audit_phases(mongo_source)
        if len(phases) != 7:
            issues.append(Issue("mongo-audit-phase-count-drift", f"source phases={phases}"))
        for doc_path in (RUNTIME_SCHEDULER_DOC, OPS_SCHEDULER_DOC):
            doc_text = doc_path.read_text(encoding="utf-8")
            for phase in phases:
                if f"`{phase}`" not in doc_text:
                    issues.append(
                        Issue(
                            "mongo-audit-phase-doc-drift",
                            f"{doc_path.relative_to(ROOT)}: {phase}",
                        )
                    )
        scanner_methods = interface_methods(mongo_source, "Scanner")
        if scanner_methods != {"UpperBound", "ScanBatch"}:
            issues.append(Issue("mongo-audit-scanner-port-drift", str(sorted(scanner_methods))))
        checkpoint_methods = interface_methods(mongo_source, "CheckpointStore")
        if checkpoint_methods != {"Load", "Save"}:
            issues.append(Issue("mongo-audit-checkpoint-port-drift", str(sorted(checkpoint_methods))))
    else:
        issues.append(Issue("missing-mongo-audit-source", str(MONGO_AUDIT_SOURCE.relative_to(ROOT))))

    if MONGO_AUDIT_METRICS_SOURCE.exists():
        metrics_source = MONGO_AUDIT_METRICS_SOURCE.read_text(encoding="utf-8")
        metric_names = set(re.findall(r'\bName:\s*"([a-z0-9_]+)"', metrics_source))
        metrics_doc_text = MONGO_AUDIT_METRICS_DOC.read_text(encoding="utf-8")
        for name in sorted(metric_names):
            full_name = f"qs_mongo_consistency_audit_{name}"
            if full_name not in metrics_doc_text:
                issues.append(Issue("mongo-audit-metric-doc-drift", full_name))

    for config_path in (DEVELOPMENT_CONFIG, PRODUCTION_CONFIG):
        enabled = yaml_block_values(config_path.read_text(encoding="utf-8"), "mongo_consistency_audit").get("enable")
        if enabled != "false":
            issues.append(
                Issue(
                    "mongo-audit-default-enable-drift",
                    f"{config_path.relative_to(ROOT)}: expected false, got {enabled!r}",
                )
            )
    options_text = APISERVER_OPTIONS_SOURCE.read_text(encoding="utf-8")
    mongo_default = re.search(
        r"func NewMongoConsistencyAuditOptions\(\).*?return &MongoConsistencyAuditOptions\{(?P<body>.*?)\n\s*\}",
        options_text,
        flags=re.DOTALL,
    )
    if not mongo_default or "Enable: false" not in mongo_default.group("body"):
        issues.append(Issue("mongo-audit-option-default-drift", "NewMongoConsistencyAuditOptions.Enable must be false"))

    # Perf plans are executable contracts. Freeze the approved plan registry,
    # formal profiles and the bounded ceiling-120 phase chain in docs-facts.
    if PERF_PLAN_SOURCE.exists() and PERF_RUNNER_SOURCE.exists() and PERF_CONFIG.exists():
        plan_text = PERF_PLAN_SOURCE.read_text(encoding="utf-8")
        runner_text = PERF_RUNNER_SOURCE.read_text(encoding="utf-8")
        plan_names = performance_plan_names(plan_text, runner_text)
        expected_plans = ["quick", "baseline", "ceiling-120", "admission", "diagnose"]
        if plan_names != expected_plans:
            issues.append(Issue("perf-plan-registry-drift", f"source={plan_names}, expected={expected_plans}"))
        sop_text = PERF_SOP.read_text(encoding="utf-8")
        perf_readme_text = PERF_README.read_text(encoding="utf-8")
        makefile_text = (ROOT / "Makefile").read_text(encoding="utf-8")
        for plan_name in plan_names:
            command = f"make perf-run PLAN={plan_name}"
            for doc_path, doc_text in ((PERF_SOP, sop_text), (PERF_README, perf_readme_text)):
                if command not in doc_text:
                    issues.append(
                        Issue(
                            "perf-plan-doc-drift",
                            f"{doc_path.relative_to(ROOT)}: {command}",
                        )
                    )
            if plan_name not in makefile_text:
                issues.append(Issue("perf-plan-makefile-drift", plan_name))

        config_profiles = set(
            (json.loads(PERF_CONFIG.read_text(encoding="utf-8")).get("qpsProfiles") or {}).keys()
        )
        formal_profiles = {profile for _, profile, _, _, dynamic in performance_phase_specs(plan_text) if not dynamic}
        if config_profiles != formal_profiles:
            issues.append(
                Issue(
                    "perf-formal-profile-drift",
                    f"config={sorted(config_profiles)}, source={sorted(formal_profiles)}",
                )
            )

        ceiling_match = re.search(
            r'case "ceiling-120":(?P<body>.*?)case "admission":',
            plan_text,
            flags=re.DOTALL,
        )
        ceiling_specs = performance_phase_specs(ceiling_match.group("body")) if ceiling_match else []
        expected_ceiling = [
            ("capacity_110", "capacity_110", 110, "2m", True),
            ("capacity_120", "capacity_120", 120, "2m", True),
        ]
        if ceiling_specs != expected_ceiling:
            issues.append(
                Issue(
                    "perf-ceiling-120-source-drift",
                    f"source={ceiling_specs}, expected={expected_ceiling}",
                )
            )
        for phase_id, _, target, duration, _ in ceiling_specs:
            for token in (f"`{phase_id}`", f"{target} QPS", f"`{duration}`"):
                if token not in sop_text:
                    issues.append(Issue("perf-ceiling-120-sop-drift", token))
        if "make perf-run PLAN=ceiling-120 DRY_RUN=1" not in sop_text:
            issues.append(Issue("perf-ceiling-120-sop-drift", "missing ceiling-120 dry-run command"))

    if VERSION_LEDGER.exists():
        ledger_text = VERSION_LEDGER.read_text(encoding="utf-8")
        for service, swagger_path in SWAGGER_CONTRACTS.items():
            if not swagger_path.exists():
                issues.append(Issue("missing-swagger-contract", str(swagger_path.relative_to(ROOT))))
                continue
            swagger = json.loads(swagger_path.read_text(encoding="utf-8"))
            paths = swagger.get("paths", {}) or {}
            operation_count = sum(
                1
                for methods in paths.values()
                for method in methods
                if method.lower() in {"get", "post", "put", "delete", "patch", "options", "head"}
            )
            unique_path_count = len(paths)
            expected = re.compile(
                rf"\|\s*{re.escape(service)} REST\s*\|[^\n]*"
                rf"\*\*{operation_count} method-path operations\*\*[^\n]*"
                rf"\*\*{unique_path_count} unique paths\*\*"
            )
            if not expected.search(ledger_text):
                issues.append(
                    Issue(
                        "version-ledger-api-count-drift",
                        f"{service}: expected {operation_count} operations / {unique_path_count} unique paths "
                        f"from {swagger_path.relative_to(ROOT)}",
                    )
                )

    ops_scheduler = DOCS / "04-接口与运维/07-调度任务.md"
    if PRODUCTION_CONFIG.exists() and ops_scheduler.exists():
        config_text = PRODUCTION_CONFIG.read_text(encoding="utf-8")
        block = re.search(
            r"(?m)^mongo_consistency_audit:\s*\n(?P<body>(?:^[ \t]+.*(?:\n|$))*)",
            config_text,
        )
        config_values = {}
        if block:
            for line in block.group("body").splitlines():
                setting = re.match(r"^\s+([a-z_]+):\s*(.*?)\s*(?:#.*)?$", line)
                if setting:
                    config_values[setting.group(1)] = setting.group(2).strip()
        if "enable" not in config_values:
            issues.append(Issue("missing-mongo-audit-production-config", "mongo_consistency_audit.enable"))
        else:
            ops_text = ops_scheduler.read_text(encoding="utf-8")
            for key, value in config_values.items():
                expected_setting = f"  {key}: {value}"
                if expected_setting not in ops_text:
                    issues.append(
                        Issue(
                            "mongo-audit-production-config-doc-drift",
                            f"expected {expected_setting.strip()!r} from {PRODUCTION_CONFIG.relative_to(ROOT)}",
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
