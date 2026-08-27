#!/usr/bin/env python3
"""Check low-cost facts and boundaries for the active documentation tree.

This complements check_docs_hygiene.py. It deliberately checks only facts that
can be derived cheaply and deterministically from the repository; prose still
requires code review when behavior changes.
"""
from __future__ import annotations

import json
import re
import shlex
import subprocess
import sys
from dataclasses import dataclass
from datetime import date, datetime
from functools import lru_cache
from pathlib import Path
from typing import Iterable


ROOT = Path(__file__).resolve().parent.parent
DOCS = ROOT / "docs"
ARCHIVE = DOCS / "_archive"
REGISTRY = ROOT / "internal/apiserver/container/modules/registry.go"
EVENTS = ROOT / "configs/events.yaml"
SIGNALS = ROOT / "configs/signals.yaml"
VERSION_LEDGER = DOCS / "00-总览/09-当前版本定档验收台账.md"
CLOSURE_MANIFEST = DOCS / "document-closure.json"
INFRA_PRODUCTION_EVIDENCE = DOCS / "infrastructure-production-evidence.json"
CURRENT_PRODUCTION_MAX_VALIDITY_DAYS = 30
PROTO_ROOT = ROOT / "api/grpc/proto"
MIGRATION_ROOT = ROOT / "internal/pkg/migration/migrations"
MIGRATION_README = ROOT / "internal/pkg/migration/README.md"
PRODUCTION_CONFIG = ROOT / "configs/apiserver.prod.yaml"
DEVELOPMENT_CONFIG = ROOT / "configs/apiserver.dev.yaml"
SCHEDULER_BOOTSTRAP = ROOT / "internal/apiserver/process/runtime_bootstrap.go"
PERF_PLAN_SOURCE = ROOT / "scripts/perf/perfctl/plan.go"
PERF_RUNNER_SOURCE = ROOT / "scripts/perf/perfctl/runner.go"
PERF_CONFIG = ROOT / "scripts/perf/qs-perf.config.example.json"
PERF_README = ROOT / "scripts/perf/README.md"
PERF_SOP = DOCS / "04-接口与运维/11-300QPS混合场景压测SOP.md"
BRIEFING_DOC = DOCS / "06-宣讲/01-系统技术简报.md"
RUNTIME_SCHEDULER_DOC = DOCS / "01-运行时/06-后台任务与调度.md"
OPS_SCHEDULER_DOC = DOCS / "04-接口与运维/07-调度任务.md"
MONGO_AUDIT_SOURCE = ROOT / "internal/apiserver/application/mongoconsistency/audit.go"
MONGO_AUDIT_METRICS_SOURCE = ROOT / "internal/apiserver/application/mongoconsistency/metrics.go"
APISERVER_OPTIONS_SOURCE = ROOT / "internal/apiserver/options/options.go"
MONGO_AUDIT_METRICS_DOC = DOCS / "04-接口与运维/08-健康检查与观测.md"
IR_CHECKLIST = DOCS / "02-业务模块/40-interpretation/90-设计问题与重构清单.md"
APISERVER_RUNTIME_DOC = DOCS / "01-运行时/01-qs-apiserver启动与组合根.md"
GRPC_RUNTIME_DOC = DOCS / "01-运行时/04-进程间调用与gRPC.md"
INFRA_OVERVIEW_DOC = DOCS / "03-基础设施/00-基础设施总览.md"
INFRA_RUNTIME_DOC = DOCS / "03-基础设施/runtime/README.md"
INFRA_LIFECYCLE_DOC = DOCS / "03-基础设施/runtime/10-进程生命周期、启动与关闭.md"
INFRA_SECURITY_DOC = DOCS / "03-基础设施/security/README.md"
INFRA_SECURITY_CANONICAL_DOC = DOCS / "03-基础设施/security/10-身份、服务与资源授权.md"
INFRA_OBSERVABILITY_DOC = DOCS / "03-基础设施/observability/README.md"
INFRA_OBSERVABILITY_DETAIL_DOC = DOCS / "03-基础设施/observability/10-日志、指标与关联标识.md"
INFRA_PROBE_DOC = DOCS / "03-基础设施/observability/20-健康探针、治理与持久证据.md"
INFRA_OBSERVABILITY_DECISIONS_DOC = DOCS / "03-基础设施/observability/15-核心设计决策与替代方案.md"
INFRA_CONFIG_DOC = DOCS / "03-基础设施/config-deployment/10-配置、Secret与外部文件.md"
INFRA_DEPLOYMENT_DOC = DOCS / "03-基础设施/config-deployment/20-镜像、CD与网络拓扑.md"
LOCKLEASE_DOC = DOCS / "03-基础设施/concurrency/50-LockLease与长任务互斥.md"
SIGNAL_DOC = DOCS / "03-基础设施/event/60-Signal一次性信令.md"
CACHE_MODEL_DOC = DOCS / "03-基础设施/cache/15-从事实到派生读取-一致性与容量模型.md"
CACHE_REGISTRY_DOC = DOCS / "03-基础设施/cache/20-Capability-Registry与配置.md"
CACHE_KERNEL_DOC = DOCS / "03-基础设施/cache/30-缓存内核与读写链路.md"
CACHE_README = DOCS / "03-基础设施/cache/README.md"
CACHE_CONSISTENCY_DOC = DOCS / "03-基础设施/cache/40-一致性失效与降级.md"
CACHE_OBSERVABILITY_DOC = DOCS / "03-基础设施/cache/60-可观测性与运营页面.md"
CACHE_ACCEPTANCE_DOC = DOCS / "03-基础设施/cache/70-扩展与验收.md"
EVENT_STATE_DOC = DOCS / "03-基础设施/event/15-从事实到结算-失败窗口与状态机.md"
EVENT_CONTRACT_DOC = DOCS / "03-基础设施/event/20-事件契约与演进.md"
EVENT_OUTBOX_DOC = DOCS / "03-基础设施/event/30-Outbox可靠出站链路.md"
EVENT_MQ_DOC = DOCS / "03-基础设施/event/40-MQ发布消费与结算.md"
EVENT_OBSERVABILITY_DOC = DOCS / "03-基础设施/event/50-可观测性与故障恢复.md"
EVENT_README = DOCS / "03-基础设施/event/README.md"
CONCURRENCY_BACKPRESSURE_DOC = DOCS / "03-基础设施/concurrency/40-下游背压与容量预算.md"
CONCURRENCY_GOVERNANCE_DOC = DOCS / "03-基础设施/concurrency/60-运行时治理与故障恢复.md"
CONCURRENCY_ACCEPTANCE_DOC = DOCS / "03-基础设施/concurrency/70-可观测性-压测与验收.md"
CONCURRENCY_README = DOCS / "03-基础设施/concurrency/README.md"
GRPC_SIDECAR_DOC = ROOT / "internal/pkg/grpc/README.md"
LOCKLEASE_CATALOG_SOURCE = ROOT / "internal/pkg/resilience/locklease/catalog.go"
GRPC_SERVER_SOURCE = ROOT / "internal/pkg/grpc/server.go"
GRPC_CONFIG_SOURCE = ROOT / "internal/pkg/grpc/config.go"
GRPC_OPTIONS_SOURCE = ROOT / "internal/pkg/options/grpc.go"
APISERVER_GRPC_BOOTSTRAP_SOURCE = ROOT / "internal/apiserver/process/transport_bootstrap.go"
GRPC_ACL_CONFIG = ROOT / "configs/grpc-acl.prod.yaml"
WORKER_PRODUCTION_CONFIG = ROOT / "configs/worker.prod.yaml"
COLLECTION_HEALTH_SOURCE = ROOT / "internal/collection-server/transport/rest/handler/health_handler.go"
COLLECTION_ROUTER_SOURCE = ROOT / "internal/collection-server/transport/rest/router.go"
WORKER_PROBE_SOURCE = ROOT / "internal/worker/observability/metrics_server.go"
APISERVER_ROUTER_SOURCE = ROOT / "internal/apiserver/transport/rest/router.go"
GENERIC_SERVER_SOURCE = ROOT / "internal/pkg/server/genericapiserver.go"
PROCESS_SOURCES = {
    "apiserver": (
        ROOT / "internal/apiserver/process/runner.go",
        ROOT / "internal/apiserver/process/lifecycle.go",
    ),
    "collection-server": (
        ROOT / "internal/collection-server/process/runner.go",
        ROOT / "internal/collection-server/process/lifecycle.go",
    ),
    "worker": (
        ROOT / "internal/worker/process/runner.go",
        ROOT / "internal/worker/process/lifecycle.go",
    ),
}
DOCKERFILES = {
    "apiserver": ROOT / "build/docker/Dockerfile.qs-apiserver",
    "collection-server": ROOT / "build/docker/Dockerfile.collection-server",
    "worker": ROOT / "build/docker/Dockerfile.qs-worker",
}
PRODUCTION_COMPOSE = ROOT / "build/docker/docker-compose.prod.yml"
PACKAGE_SCRIPT = ROOT / "scripts/cd/prepare-package.sh"
DEPLOY_SCRIPT = ROOT / "scripts/cd/remote-deploy.sh"
IMAGE_METADATA_SCRIPT = ROOT / "scripts/cd/image-metadata.sh"
WORKER_READINESS_SCRIPT = ROOT / "scripts/cd/wait-worker-readiness.sh"
ONEOFF_README = ROOT / "scripts/oneoff/README.md"
ONEOFF_CLEANUP_SOURCE = ROOT / "scripts/oneoff/cleanup_orphaned_assessment_documents/main.go"
INFRA_MIGRATION_RECOVERY_DOC = DOCS / "03-基础设施/migration-recovery/20-One-off、恢复与生产证据.md"
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
    (
        "AIExplanationPromptEvaluationLeaseRecoveryRunner",
        "AIExplanationPromptEvaluationLeaseRecovery",
        "ai_explanation_prompt_evaluation_lease_recovery",
        "AIExplanationPromptEvaluationLeaseRecovery",
    ),
    (
        "AIExplanationParticipantLeaseRecoveryRunner",
        "AIExplanationParticipantLeaseRecovery",
        "ai_explanation_participant_lease_recovery",
        "AIExplanationParticipantLeaseRecovery",
    ),
    ("ReportCatalogAuditRunner", "ReportCatalogAudit", "report_catalog_audit", "ReportCatalogAudit"),
    ("MongoConsistencyAuditRunner", "MongoConsistencyAudit", "mongo_consistency_audit", "MongoConsistencyAudit"),
)

EXPECTED_SCHEDULER_REGISTRATIONS = tuple((runner, option) for runner, option, _, _ in SCHEDULER_RUNNERS)
EXPECTED_SCHEDULER_PRODUCTION_ENABLE = {
    config_key: "false"
    if config_key
    in {
        "mongo_consistency_audit",
        "ai_explanation_prompt_evaluation_lease_recovery",
        "ai_explanation_participant_lease_recovery",
    }
    else "true"
    for _, _, config_key, _ in SCHEDULER_RUNNERS
}

EXPECTED_LOCKLEASE_INVENTORY = (
    ("worker", "answersheet_processing", "duplicate_suppression", "5m", "auto"),
    ("apiserver", "plan_scheduler_leader", "leader", "50s", "auto"),
    ("apiserver", "statistics_sync_leader", "leader", "30m", "auto"),
    ("apiserver", "statistics_sync", "task_lock", "30m", "auto"),
    ("apiserver", "evaluation_consistency_audit", "leader", "30s", "auto"),
    ("apiserver", "evaluation_lease_recovery", "leader", "30s", "auto"),
    ("apiserver", "interpretation_lease_recovery", "leader", "30s", "auto"),
    ("apiserver", "ai_explanation_prompt_evaluation_lease_recovery", "leader", "30s", "auto"),
    ("apiserver", "ai_explanation_participant_lease_recovery", "leader", "30s", "auto"),
    ("apiserver", "report_catalog_audit", "leader", "30s", "auto"),
    ("apiserver", "mongo_consistency_audit", "leader", "30s", "auto"),
    ("worker", "attention_projection_reconcile", "leader", "30m", "auto"),
    ("apiserver", "authz_role_projection_reconcile", "leader", "15m", "auto"),
    ("collection-server", "collection_submit", "duplicate_suppression", "5m", "auto"),
)

EXPECTED_SIGNAL_TOPOLOGY = {
    "assessment_model_cache_changed": {
        "delivery": "ephemeral_signal",
        "transport": "redis_pubsub",
        "publishers": ["apiserver"],
        "subscribers": ["collection-server"],
    },
    "questionnaire_cache_changed": {
        "delivery": "ephemeral_signal",
        "transport": "redis_pubsub",
        "publishers": ["apiserver"],
        "subscribers": ["apiserver", "collection-server"],
    },
    "report_status_changed": {
        "delivery": "ephemeral_signal",
        "transport": "redis_pubsub",
        "publishers": ["apiserver", "worker"],
        "subscribers": ["collection-server"],
    },
    "scale_cache_changed": {
        "delivery": "ephemeral_signal",
        "transport": "redis_pubsub",
        "publishers": ["apiserver"],
        "subscribers": ["apiserver"],
    },
    "typology_model_cache_changed": {
        "delivery": "ephemeral_signal",
        "transport": "redis_pubsub",
        "publishers": ["apiserver"],
        "subscribers": ["apiserver", "collection-server"],
    },
}

EXPECTED_PROCESS_STAGES = {
    "apiserver": [
        "prepare resources",
        "initialize container",
        "initialize integrations",
        "initialize transports",
        "start background runtimes",
        "register shutdown callback",
    ],
    "collection-server": [
        "prepare resources",
        "initialize container",
        "initialize integrations",
        "initialize transports",
        "register shutdown callback",
    ],
    "worker": [
        "prepare resources",
        "initialize container",
        "initialize integrations",
        "initialize runtime",
        "register shutdown callback",
    ],
}

# Review targets require an explicit closure-manifest exception when exceeded.
# Hard ceilings remain fail-closed so the exception mechanism cannot allow
# unbounded growth or recreate oversized catch-all documents.
TARGET_ACTIVE_MARKDOWN = 150
HARD_MAX_ACTIVE_MARKDOWN = 165
TARGET_BUSINESS_MODULE_MARKDOWN = 18
HARD_MAX_BUSINESS_MODULE_MARKDOWN = 22

EXPECTED_EVENTS = {
    "questionnaire.changed",
    "assessment_model.changed",
    "answersheet.submitted",
    "evaluation.requested",
    "evaluation.retry.requested",
    "evaluation.outcome.committed",
    "evaluation.failed",
    "interpretation.report.generated",
    "interpretation.report.failed",
    "interpretation.retry.requested",
    "interpretation.ai_explanation.requested",
    "interpretation.ai_explanation.retry.requested",
    "interpretation.ai_explanation.lease_recovery.requested",
    "interpretation.ai_explanation.generated",
    "interpretation.ai_explanation.failed",
    "interpretation.ai_explanation.prompt_evaluation.step_requested",
    "task.opened",
    "task.completed",
    "task.expired",
    "task.canceled",
}

EXPECTED_SIGNALS = {
    "report_status_changed",
    "questionnaire_cache_changed",
    "assessment_model_cache_changed",
    "scale_cache_changed",
    "typology_model_cache_changed",
}

EXPECTED_MIGRATION_MAX = {"mysql": 70, "mongodb": 32}
EXPECTED_DOC_STATUS = {"aligned", "drifted", "needs_review", "planned", "archive_candidate"}
EXPECTED_OWNERS = {
    "overview",
    "runtime",
    "survey",
    "modelcatalog",
    "evaluation",
    "interpretation",
    "actor",
    "plan",
    "statistics",
    "infra",
    "interfaces",
    "decisions",
    "briefing",
    "governance",
    "repository",
    "ci",
    "tooling",
    "vendor",
}
EXPECTED_PRODUCTION_LEVELS = {"not_applicable", "none", "historical", "current"}
EXPECTED_REVIEW_STATES = {"unreviewed", "reviewing", "ready", "conditional", "blocked", "not_applicable"}
EXPECTED_SIGNOFF_STATES = {"unsigned", "signed"}
MODULE_SIGNOFF_DIMENSIONS = ["runtime", "contract", "data", "security", "test", "operations", "production"]
INFRASTRUCTURE_TOPICS = {
    "runtime_lifecycle": "docs/03-基础设施/runtime/10-进程生命周期、启动与关闭.md",
    "config_secrets": "docs/03-基础设施/config-deployment/10-配置、Secret与外部文件.md",
    "deployment_cd_network": "docs/03-基础设施/config-deployment/20-镜像、CD与网络拓扑.md",
    "data_migration_transaction": "docs/03-基础设施/data-access/10-存储所有权与事务边界.md",
    "event_outbox_mq": "docs/03-基础设施/event/README.md",
    "cache_redis_signal": "docs/03-基础设施/cache/README.md",
    "concurrency_resilience": "docs/03-基础设施/concurrency/README.md",
    "security_acl_resource_ownership": "docs/03-基础设施/security/10-身份、服务与资源授权.md",
    "observability_probes": "docs/03-基础设施/observability/20-健康探针、治理与持久证据.md",
    "oneoff_recovery_production_evidence": (
        "docs/03-基础设施/migration-recovery/20-One-off、恢复与生产证据.md"
    ),
}
INFRASTRUCTURE_REQUIRED_SOURCE_SCOPES = {
    "runtime_lifecycle": {
        "apiserver_entry": ("cmd/qs-apiserver/apiserver.go",),
        "collection_entry": ("cmd/collection-server/main.go",),
        "worker_entry": ("cmd/qs-worker/main.go",),
        "apiserver_process": ("internal/apiserver/process/runner.go",),
        "collection_process": ("internal/collection-server/process/runner.go",),
        "worker_process": ("internal/worker/process/runner.go",),
        "composition_container": ("internal/apiserver/container/root.go",),
        "processruntime_dependency": ("go.mod",),
        "apiserver_config": ("configs/apiserver.prod.yaml",),
        "collection_config": ("configs/collection-server.prod.yaml",),
        "worker_config": ("configs/worker.prod.yaml",),
    },
    "config_secrets": {
        "apiserver_config": ("configs/apiserver.prod.yaml",),
        "collection_config": ("configs/collection-server.prod.yaml",),
        "worker_config": ("configs/worker.prod.yaml",),
        "event_catalog": ("configs/events.yaml",),
        "signal_catalog": ("configs/signals.yaml",),
        "cache_catalog": ("configs/cache/apiserver.prod.yaml",),
        "grpc_acl": ("configs/grpc-acl.prod.yaml",),
        "config_contract": ("internal/pkg/configcontract/config_contract_test.go",),
        "external_file_packaging": ("scripts/cd/prepare-package.sh",),
    },
    "deployment_cd_network": {
        "delivery_workflow": (".github/workflows/cd.yml",),
        "apiserver_image": ("build/docker/Dockerfile.qs-apiserver",),
        "collection_image": ("build/docker/Dockerfile.collection-server",),
        "worker_image": ("build/docker/Dockerfile.qs-worker",),
        "compose_topology": ("build/docker/docker-compose.prod.yml",),
        "package_script": ("scripts/cd/prepare-package.sh",),
        "deploy_script": ("scripts/cd/remote-deploy.sh",),
    },
    "data_migration_transaction": {
        "mysql_repository": ("internal/apiserver/infra/mysql/actor/testee_repository.go",),
        "mongo_repository": ("internal/apiserver/infra/mongo/answersheet/durable_submit.go",),
        "migration_runtime": ("internal/pkg/migration/migrate.go",),
        "transaction_boundary": ("internal/apiserver/container/internal/transaction/runner.go",),
        "outbox_atomicity": ("internal/pkg/architecture/uow_outbox_ratchet_test.go",),
    },
    "event_outbox_mq": {
        "event_catalog": ("configs/events.yaml",),
        "signal_catalog": ("configs/signals.yaml",),
        "messaging_dependency": ("go.mod",),
        "event_runtime": ("internal/pkg/eventing/runtime/publisher.go",),
        "event_contract": ("internal/pkg/eventing/catalog/catalog.go",),
        "dead_letter_transport": ("internal/pkg/eventing/transport/dead_letter.go",),
        "apiserver_event_subsystem": ("internal/apiserver/eventing/subsystem/subsystem.go",),
        "worker_messaging_runtime": ("internal/worker/integration/messaging/runtime.go",),
        "worker_business_handler": ("internal/worker/handlers/assessment_handler.go",),
        "report_status_reporter": ("internal/pkg/reportstatus/reporter.go",),
        "report_status_cache": ("internal/pkg/reportstatus/cache.go",),
        "delivery_replay": ("internal/apiserver/application/systemgovernance/delivery_replay.go",),
        "mysql_outbox_store": ("internal/apiserver/infra/mysql/eventoutbox/store.go",),
        "mongo_outbox_store": ("internal/apiserver/infra/mongo/eventoutbox/store.go",),
        "outbox_atomicity": ("internal/pkg/architecture/uow_outbox_ratchet_test.go",),
    },
    "cache_redis_signal": {
        "signal_catalog": ("configs/signals.yaml",),
        "apiserver_config": ("configs/apiserver.prod.yaml",),
        "collection_config": ("configs/collection-server.prod.yaml",),
        "apiserver_prod_policy": ("configs/cache/apiserver.prod.yaml",),
        "collection_prod_policy": ("configs/cache/collection-server.prod.yaml",),
        "cache_policy": ("internal/apiserver/cache/catalog/policy.go",),
        "apiserver_cache_subsystem": ("internal/apiserver/cache/subsystem/subsystem.go",),
        "collection_cache_subsystem": ("internal/collection-server/cache/subsystem.go",),
        "collection_l1_cache": ("internal/collection-server/application/modelcatalog/readthrough.go",),
        "published_model_l1": ("internal/apiserver/cache/modelcatalog/published_model_l1.go",),
        "cache_metric_reader": ("internal/apiserver/application/systemgovernance/metric_evidence_reader.go",),
        "cache_core": ("internal/pkg/cache/query/versioned.go",),
        "signal_contract": ("internal/pkg/signalcatalog/types.go",),
        "redis_runtime": ("internal/pkg/redisruntime/runtime.go",),
        "config_contract": ("internal/pkg/configcontract/config_contract_test.go",),
    },
    "concurrency_resilience": {
        "resilience_model": ("internal/pkg/resilience/model.go",),
        "admission_strategy": ("internal/pkg/resilience/admission/strategy.go",),
        "backpressure_limiter": ("internal/pkg/resilience/backpressure/limiter.go",),
        "locklease_catalog": ("internal/pkg/resilience/locklease/catalog.go",),
        "locklease_runtime": ("internal/pkg/resilience/locklease/subsystem/subsystem.go",),
        "collection_concurrency_gate": ("internal/collection-server/concurrency/gate.go",),
        "collection_route_gates": ("internal/collection-server/transport/rest/router_concurrency.go",),
        "reliable_submit": ("internal/collection-server/application/answersheet/submission_service.go",),
        "submit_coalescer": ("internal/collection-server/infra/redisops/submit_coalescer.go",),
        "mongo_durable_submit": ("internal/apiserver/infra/mongo/answersheet/durable_submit.go",),
        "mysql_repository_limiter": ("internal/pkg/database/mysql/base.go",),
        "mongo_repository_limiter": ("internal/apiserver/infra/mongo/base.go",),
        "mysql_actor_raw_handle": ("internal/apiserver/infra/mysql/actor/read_model.go",),
        "mongo_answersheet_readmodel_raw_handle": (
            "internal/apiserver/infra/mongo/answersheet/readmodel.go",
        ),
        "mongo_answersheet_repository_raw_handle": (
            "internal/apiserver/infra/mongo/answersheet/repo.go",
        ),
        "governance_action_executor": ("internal/apiserver/application/systemgovernance/action_executor.go",),
        "governance_tune_rate_limit": ("internal/apiserver/resilience/subsystem/governance.go",),
        "governance_release_lock": ("internal/apiserver/resilience/subsystem/command_agent.go",),
        "scheduler_lock_caller": ("internal/apiserver/runtime/scheduler/leader_lock.go",),
        "transaction_boundary": ("internal/apiserver/container/internal/transaction/runner.go",),
        "apiserver_config": ("configs/apiserver.prod.yaml",),
        "collection_config": ("configs/collection-server.prod.yaml",),
        "worker_config": ("configs/worker.prod.yaml",),
    },
    "security_acl_resource_ownership": {
        "http_identity": ("internal/pkg/httpauth/identity.go",),
        "apiserver_rest_middleware": ("internal/apiserver/transport/rest/middleware/iam_middleware.go",),
        "collection_rest_middleware": (
            "internal/collection-server/transport/rest/middleware/iam_middleware.go",
        ),
        "grpc_acl_runtime": ("internal/pkg/grpc/server.go",),
        "grpc_acl_config": ("configs/grpc-acl.prod.yaml",),
        "apiserver_config": ("configs/apiserver.prod.yaml",),
        "collection_config": ("configs/collection-server.prod.yaml",),
        "worker_config": ("configs/worker.prod.yaml",),
        "resource_ownership": ("internal/collection-server/application/testeeaccess/authorizer.go",),
    },
    "observability_probes": {
        "apiserver_handler": ("internal/apiserver/transport/rest/router.go",),
        "collection_handler": ("internal/collection-server/transport/rest/handler/health_handler.go",),
        "worker_handler": ("internal/worker/observability/metrics_server.go",),
        "generic_server": ("internal/pkg/server/genericapiserver.go",),
        "system_governance": ("internal/apiserver/application/systemgovernance/facade.go",),
        "redis_observability": ("internal/pkg/redisruntime/observability/observer.go",),
        "event_observability": ("internal/pkg/eventing/observe/metrics.go",),
        "lease_observability": ("internal/pkg/resilience/locklease/observability/metrics.go",),
        "apiserver_config": ("configs/apiserver.prod.yaml",),
        "collection_config": ("configs/collection-server.prod.yaml",),
        "worker_config": ("configs/worker.prod.yaml",),
    },
    "oneoff_recovery_production_evidence": {
        "cleanup_tool": ("scripts/oneoff/cleanup_orphaned_assessment_documents/main.go",),
        "recovery_tool": ("scripts/oneoff/rebuild_statistics/main.go",),
        "migration_runtime": ("internal/pkg/migration/migrate.go",),
        "production_ledger": ("docs/infrastructure-production-evidence.json",),
    },
}
SHA_RE = re.compile(r"^[0-9a-f]{40}$")
DATE_RE = re.compile(r"^\d{4}-\d{2}-\d{2}$")
SHA256_RE = re.compile(r"^sha256:[0-9a-f]{64}$")
LEDGER_METADATA_RE = re.compile(r"<!--\s*docs-facts:\s*(?P<body>.*?)\s*-->")

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
    BRIEFING_DOC,
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
    DOCS / "03-基础设施/data-access/10-存储所有权与事务边界.md": {
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
            "Mongo migration 000027",
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


def git_output(*args: str) -> str:
    completed = subprocess.run(
        ["git", *args],
        cwd=ROOT,
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return completed.stdout.strip()


def current_git_head() -> str:
    return git_output("rev-parse", "HEAD")


def current_source_baseline() -> str:
    # Documentation, scripts and workflow-only commits must not invalidate the
    # reviewed source baseline. The baseline moves with product contracts,
    # composition roots and deploy/runtime packaging that infrastructure docs
    # are expected to explain; Markdown-only changes remain excluded.
    return git_output(
        "log",
        "-1",
        "--format=%H",
        "--",
        "cmd",
        "internal",
        "pkg",
        "api",
        "configs",
        "scripts/oneoff",
        ".github/workflows",
        "build/docker",
        "scripts/cd",
        "Makefile",
        ":(exclude,glob)**/*.md",
    )


@lru_cache(maxsize=None)
def is_git_ancestor(commit: str, head: str) -> bool:
    if not SHA_RE.fullmatch(commit) or not SHA_RE.fullmatch(head):
        return False
    completed = subprocess.run(
        ["git", "merge-base", "--is-ancestor", commit, head],
        cwd=ROOT,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    return completed.returncode == 0


def valid_iso_date(value: object) -> bool:
    if not isinstance(value, str) or not DATE_RE.fullmatch(value):
        return False
    try:
        date.fromisoformat(value)
    except ValueError:
        return False
    return True


def yaml_section_keys(text: str, section: str, key_pattern: str) -> set[str]:
    keys: set[str] = set()
    in_section = False
    key_re = re.compile(rf"^  ({key_pattern}):\s*$")
    for line in text.splitlines():
        if not in_section:
            if re.fullmatch(rf"{re.escape(section)}:\s*(?:#.*)?", line):
                in_section = True
            continue
        if line and not line[0].isspace() and not line.lstrip().startswith("#"):
            break
        match = key_re.fullmatch(line)
        if match:
            keys.add(match.group(1))
    return keys


def migration_versions(directory: Path, suffix: str) -> set[int]:
    versions: set[int] = set()
    pattern = re.compile(rf"^(\d{{6}})_.+\.{re.escape(suffix)}$")
    for path in sorted(directory.iterdir()):
        if not path.is_file():
            continue
        match = pattern.fullmatch(path.name)
        if match:
            versions.add(int(match.group(1)))
    return versions


def migration_inventory() -> tuple[dict[str, dict[str, int]], list[Issue]]:
    inventory: dict[str, dict[str, int]] = {}
    issues: list[Issue] = []
    for backend, suffix in (("mysql", "sql"), ("mongodb", "json")):
        directory = MIGRATION_ROOT / backend
        up_versions = migration_versions(directory, f"up.{suffix}")
        down_versions = migration_versions(directory, f"down.{suffix}")
        if up_versions != down_versions:
            issues.append(
                Issue(
                    "migration-pair-drift",
                    f"{backend}: up-only={sorted(up_versions - down_versions)}, down-only={sorted(down_versions - up_versions)}",
                )
            )
        maximum = max(up_versions, default=0)
        expected_sequence = set(range(1, maximum + 1))
        if up_versions != expected_sequence:
            issues.append(
                Issue(
                    "migration-version-gap",
                    f"{backend}: missing={sorted(expected_sequence - up_versions)} max={maximum}",
                )
            )
        expected_max = EXPECTED_MIGRATION_MAX[backend]
        if maximum != expected_max:
            issues.append(Issue("migration-max-drift", f"{backend}: source={maximum}, expected={expected_max}"))
        inventory[backend] = {"max_version": maximum, "version_count": len(up_versions)}
    return inventory, issues


def grpc_inventory() -> tuple[dict[str, list[str]], int]:
    services: dict[str, list[str]] = {}
    proto_files = sorted(PROTO_ROOT.rglob("*.proto"))
    for path in proto_files:
        text = path.read_text(encoding="utf-8")
        package_match = re.search(r"(?m)^package\s+([A-Za-z_][A-Za-z0-9_.]*)\s*;", text)
        if not package_match:
            continue
        package = package_match.group(1)
        current_service: str | None = None
        brace_depth = 0
        for line in text.splitlines():
            if current_service is None:
                service_match = re.match(r"^service\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{", line)
                if not service_match:
                    continue
                current_service = f"{package}.{service_match.group(1)}"
                if current_service in services:
                    raise ValueError(f"duplicate proto service {current_service}")
                services[current_service] = []
                brace_depth = line.count("{") - line.count("}")
                continue
            rpc_match = re.search(r"\brpc\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(", line)
            if rpc_match:
                services[current_service].append(rpc_match.group(1))
            brace_depth += line.count("{") - line.count("}")
            if brace_depth <= 0:
                current_service = None
                brace_depth = 0
    return dict(sorted(services.items())), len(proto_files)


def machine_contract_inventory(
    migrations: dict[str, dict[str, int]],
    events: set[str],
    signals: set[str],
    grpc_services: dict[str, list[str]],
    proto_file_count: int,
) -> dict[str, object]:
    return {
        "migrations": migrations,
        "events": {
            "source": "configs/events.yaml",
            "count": len(events),
            "names": sorted(events),
        },
        "signals": {
            "source": "configs/signals.yaml",
            "count": len(signals),
            "names": sorted(signals),
        },
        "grpc": {
            "source": "api/grpc/proto",
            "proto_file_count": proto_file_count,
            "service_count": len(grpc_services),
            "rpc_count": sum(len(rpcs) for rpcs in grpc_services.values()),
            "services": grpc_services,
        },
    }


def parse_ledger_metadata(text: str) -> dict[str, str]:
    matches = LEDGER_METADATA_RE.findall(text)
    if len(matches) != 1:
        return {}
    values: dict[str, str] = {}
    for token in matches[0].split():
        if "=" not in token:
            continue
        key, value = token.split("=", 1)
        values[key] = value
    return values


def valid_repo_pattern(pattern: str) -> bool:
    if not pattern or pattern.startswith("/") or "\\" in pattern:
        return False
    return ".." not in Path(pattern).parts


def ignored_repo_path(path: str) -> bool:
    normalized = path.removeprefix("./")
    return any(part in IGNORED_MARKDOWN_DIR_NAMES for part in Path(normalized).parts)


def glob_segment_regex(segment: str) -> str:
    """Translate one Git-style path segment without allowing `*` across `/`."""

    result: list[str] = []
    index = 0
    while index < len(segment):
        char = segment[index]
        if char == "*":
            result.append("[^/]*")
        elif char == "?":
            result.append("[^/]")
        elif char == "[":
            end = segment.find("]", index + 1)
            if end < 0:
                result.append(r"\[")
            else:
                content = segment[index + 1 : end]
                negate = content.startswith(("!", "^"))
                if negate:
                    content = content[1:]
                content = content.replace("\\", r"\\").replace("]", r"\]")
                result.append(f"[{'^' if negate else ''}{content}]")
                index = end
        else:
            result.append(re.escape(char))
        index += 1
    return "".join(result)


@lru_cache(maxsize=None)
def pathspec_regex(pattern: str) -> re.Pattern[str] | None:
    """Compile the repository pathspec used by both existence and freshness.

    `**/` intentionally matches zero or more directory levels. This is the
    behavior expected from Git glob pathspecs and avoids the historical bug
    where `internal/pkg/grpc/**/*.go` missed `internal/pkg/grpc/server.go`.
    """

    if not valid_repo_pattern(pattern):
        return None
    parts = pattern.rstrip("/").split("/")
    regex_parts: list[str] = []
    for index, part in enumerate(parts):
        if part == "**":
            if index == len(parts) - 1:
                regex_parts.append(".*")
            else:
                regex_parts.append("(?:[^/]+/)*")
            continue
        regex_parts.append(glob_segment_regex(part))
        if index < len(parts) - 1:
            regex_parts.append("/")
    return re.compile("^" + "".join(regex_parts) + "$")


def path_matches_scope(path: str, scope: str) -> bool:
    normalized_path = path.removeprefix("./")
    if ignored_repo_path(normalized_path):
        return False
    if scope == "active_markdown":
        return (
            normalized_path.startswith("docs/")
            and normalized_path.endswith(".md")
            and not normalized_path.startswith("docs/_archive/")
        )
    matcher = pathspec_regex(scope)
    if matcher is None:
        return False
    if matcher.fullmatch(normalized_path):
        return True
    # A literal directory fact source owns its descendants for freshness.
    if not any(char in scope for char in "*?["):
        return normalized_path.startswith(scope.rstrip("/") + "/")
    return False


@lru_cache(maxsize=None)
def cached_manifest_source_matches(root_value: str, pattern: str) -> tuple[Path, ...]:
    root = Path(root_value)
    if not valid_repo_pattern(pattern):
        return ()
    if not any(char in pattern for char in "*?["):
        path = root / pattern
        return (path,) if not ignored_repo_path(pattern) and path.exists() else ()
    matcher = pathspec_regex(pattern)
    if matcher is None:
        return ()
    return tuple(sorted(
        path
        for path in root.rglob("*")
        if not ignored_repo_path(path.relative_to(root).as_posix())
        and matcher.fullmatch(path.relative_to(root).as_posix())
    ))


def manifest_source_matches(pattern: str) -> list[Path]:
    return list(cached_manifest_source_matches(str(ROOT), pattern))


def changed_paths_since(commit: str, cache: dict[str, set[str]]) -> set[str]:
    if commit not in cache:
        outputs = (
            git_output("diff", "--name-only", f"{commit}..HEAD", "--"),
            git_output("diff", "--name-only", "--"),
            git_output("diff", "--name-only", "--cached", "--"),
            git_output("ls-files", "--others", "--exclude-standard"),
        )
        cache[commit] = {
            line
            for output in outputs
            for line in output.splitlines()
            if line and not ignored_repo_path(line)
        }
    return cache[commit]


def validate_machine_contracts(manifest: dict[str, object], actual: dict[str, object]) -> list[Issue]:
    declared = manifest.get("machine_contracts")
    if declared != actual:
        return [Issue("machine-contract-manifest-drift", f"declared={declared!r}, source={actual!r}")]
    return []


def validate_gap(gap: object, context: str) -> list[Issue]:
    if not isinstance(gap, dict):
        return [Issue("closure-gap-schema", f"{context}: expected object")]
    missing = [key for key in ("id", "kind", "severity", "summary", "exit_criteria") if not gap.get(key)]
    if missing:
        return [Issue("closure-gap-schema", f"{context}: missing={missing}")]
    return []


@lru_cache(maxsize=None)
def make_targets() -> set[str]:
    makefile = ROOT / "Makefile"
    if not makefile.exists():
        return set()
    return set(
        re.findall(
            r"(?m)^([A-Za-z0-9_.-]+)\s*:(?!=)",
            makefile.read_text(encoding="utf-8"),
        )
    )


def command_is_verifiable(command: object) -> bool:
    """Validate a reproducible repository command without executing manifest text.

    This gate proves only that a command is repository-scoped and replayable;
    it deliberately never executes ledger-controlled text. A declared
    ``result=passed`` therefore remains recorded evidence, not a test replay.
    CI must run the relevant command independently when it is an acceptance
    prerequisite.
    """

    if not isinstance(command, str) or not command.strip() or "\n" in command:
        return False
    if re.search(r"[|&;<>`$()]", command):
        return False
    try:
        tokens = shlex.split(command)
    except ValueError:
        return False
    if not tokens:
        return False
    if tokens[0] == "make":
        targets = [token for token in tokens[1:] if not token.startswith("-") and "=" not in token]
        return bool(targets) and all(target in make_targets() for target in targets)
    if len(tokens) >= 2 and tokens[:2] == ["go", "test"]:
        packages = [token for token in tokens[2:] if not token.startswith("-")]
        if not packages:
            return False
        for package in packages:
            if not package.startswith("./"):
                return False
            base = package.removeprefix("./").removesuffix("/...").rstrip("/")
            if base and not (ROOT / base).exists():
                return False
        return True
    if tokens[0] in {"python", "python3"} and len(tokens) >= 2:
        if tokens[1] == "-m":
            return len(tokens) >= 3 and tokens[2] == "unittest"
        script = tokens[1]
        return script.startswith("scripts/") and script.endswith(".py") and (ROOT / script).is_file()
    if tokens[0] in {"bash", "sh"} and len(tokens) >= 2:
        script = tokens[1]
        return script.startswith("scripts/") and script.endswith(".sh") and (ROOT / script).is_file()
    return False


def command_is_non_cached_test(command: object) -> bool:
    """Require full, cache-bypassing Go suites for infrastructure test evidence."""
    if not command_is_verifiable(command) or not isinstance(command, str):
        return False
    try:
        tokens = shlex.split(command)
    except ValueError:
        return False
    if tokens[:2] != ["go", "test"]:
        # Repository test scripts and Make targets are validated by
        # command_is_verifiable; Go's test cache semantics do not apply.
        return True
    count_flags: list[str] = []
    for token in tokens[2:]:
        if token in {"-run", "-list", "-skip", "-exec", "-c", "-args", "--"} or token.startswith(
            ("-run=", "-list=", "-skip=", "-exec=", "-args=")
        ):
            return False
        if token == "-count" or token.startswith("-count="):
            count_flags.append(token)
    return count_flags == ["-count=1"]


def source_selector_matches(path_pattern: object, selector: object) -> bool:
    if not isinstance(path_pattern, str) or not isinstance(selector, str) or not selector.strip():
        return False
    for path in manifest_source_matches(path_pattern):
        if not path.is_file():
            continue
        try:
            if selector in path.read_text(encoding="utf-8"):
                return True
        except (OSError, UnicodeDecodeError):
            continue
    return False


def source_selector_is_trivial(selector: object) -> bool:
    """Reject selectors that prove only file presence, not the claimed behavior."""
    if not isinstance(selector, str):
        return True
    value = selector.strip()
    return any(
        pattern.fullmatch(value)
        for pattern in (
            re.compile(r"#![^\n]+"),
            re.compile(r"package\s+[A-Za-z_][A-Za-z0-9_]*"),
            re.compile(r"[A-Za-z_][A-Za-z0-9_.-]*:\s*"),
            re.compile(r'["\'][A-Za-z_][A-Za-z0-9_.-]*["\']\s*:\s*'),
        )
    )


def validate_evidence_item(item: object, context: str, head: str) -> list[Issue]:
    if not isinstance(item, dict):
        return [Issue("closure-evidence-schema", f"{context}: expected object")]
    kind = item.get("kind")
    common = {"kind", "result", "source_sha", "verified_on"}
    if kind == "command":
        required = common | {"command"}
        expected_result = "passed"
    elif kind == "source_selector":
        required = common | {"path", "selector"}
        expected_result = "matched"
    else:
        return [
            Issue(
                "closure-evidence-kind",
                f"{context}: expected command/source_selector, got {kind!r}",
            )
        ]
    if set(item) != required:
        return [
            Issue(
                "closure-evidence-schema",
                f"{context}: fields={sorted(item)}, expected={sorted(required)}",
            )
        ]
    issues: list[Issue] = []
    if item.get("result") != expected_result:
        issues.append(
            Issue(
                "closure-evidence-result",
                f"{context}: expected {expected_result!r}, got {item.get('result')!r}",
            )
        )
    source_sha = item.get("source_sha")
    if not isinstance(source_sha, str) or not SHA_RE.fullmatch(source_sha) or not is_git_ancestor(source_sha, head):
        issues.append(Issue("closure-evidence-sha", f"{context}: invalid/non-ancestor source_sha"))
    if not valid_iso_date(item.get("verified_on")):
        issues.append(Issue("closure-evidence-date", f"{context}: invalid verified_on"))
    if kind == "command" and not command_is_verifiable(item.get("command")):
        issues.append(Issue("closure-evidence-command", f"{context}: unverifiable command"))
    if kind == "source_selector" and not source_selector_matches(item.get("path"), item.get("selector")):
        issues.append(Issue("closure-evidence-selector", f"{context}: selector did not match source"))
    return issues


def parse_rfc3339(value: object) -> datetime | None:
    if not isinstance(value, str) or "T" not in value:
        return None
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None
    return parsed if parsed.tzinfo is not None else None


def valid_rfc3339(value: object) -> bool:
    return parse_rfc3339(value) is not None


def markdown_anchor_exists(path: Path, anchor: str) -> bool:
    """Resolve GitHub-style heading anchors, including duplicate suffixes."""
    try:
        text = path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return False
    if re.search(rf'<a\s+[^>]*(?:id|name)=["\']{re.escape(anchor)}["\']', text, re.IGNORECASE):
        return True
    seen: dict[str, int] = {}
    for line in text.splitlines():
        match = re.match(r"^\s{0,3}#{1,6}\s+(.+?)\s*#*\s*$", line)
        if match is None:
            continue
        heading = re.sub(r"<[^>]+>", "", match.group(1)).replace("`", "").strip().lower()
        base = "".join(character for character in heading if character.isalnum() or character in " _-")
        base = re.sub(r"\s+", "-", base.strip())
        if not base:
            continue
        duplicate_index = seen.get(base, 0)
        seen[base] = duplicate_index + 1
        candidate = base if duplicate_index == 0 else f"{base}-{duplicate_index}"
        if candidate == anchor:
            return True
    return False


def repository_record_anchor_exists(path: Path, anchor: str) -> bool:
    if path.suffix.lower() == ".md":
        return markdown_anchor_exists(path, anchor)
    line_match = re.fullmatch(r"L([1-9][0-9]*)(?:-L([1-9][0-9]*))?", anchor)
    if line_match is None:
        return False
    try:
        line_count = sum(1 for _ in path.open(encoding="utf-8"))
    except (OSError, UnicodeDecodeError):
        return False
    start = int(line_match.group(1))
    end = int(line_match.group(2) or start)
    return start <= end <= line_count


def validate_exact_gap(gap: object, context: str) -> list[Issue]:
    expected_keys = {"id", "kind", "severity", "summary", "exit_criteria"}
    if not isinstance(gap, dict) or set(gap) != expected_keys:
        fields = sorted(gap) if isinstance(gap, dict) else type(gap).__name__
        return [Issue("infrastructure-gap-schema", f"{context}: fields={fields}")]
    missing = [key for key in expected_keys if not isinstance(gap[key], str) or not gap[key].strip()]
    if missing:
        return [Issue("infrastructure-gap-schema", f"{context}: empty={sorted(missing)}")]
    return []


def production_evidence_ref_valid(kind: object, ref: object) -> bool:
    if not isinstance(kind, str) or not isinstance(ref, str) or not ref.strip():
        return False
    if kind == "github_run":
        return bool(
            re.fullmatch(
                r"https://github\.com/FangcunMount/qs-server/actions/runs/[1-9][0-9]*",
                ref,
            )
        )
    if kind == "repository_record":
        path_value, separator, anchor = ref.partition("#")
        if separator and not anchor:
            return False
        path = ROOT / path_value
        if not (
            valid_repo_pattern(path_value)
            and not any(char in path_value for char in "*?[")
            and not ignored_repo_path(path_value)
            and not path_value.startswith("docs/_archive/")
            and path.is_file()
        ):
            return False
        return not separator or repository_record_anchor_exists(path, anchor)
    return False


def validate_environment_test(
    item: object,
    context: str,
    head: str,
    topic_gap_ids: set[str],
) -> list[Issue]:
    if not isinstance(item, dict) or item.get("kind") != "environment_test":
        return [Issue("infrastructure-environment-test-schema", f"{context}: expected environment_test object")]
    status = item.get("status")
    if status == "not_run":
        expected_keys = {"kind", "environment", "planned_command", "status", "gap_id"}
        if set(item) != expected_keys:
            return [
                Issue(
                    "infrastructure-environment-test-schema",
                    f"{context}: fields={sorted(item)}, expected={sorted(expected_keys)}",
                )
            ]
        issues: list[Issue] = []
        if not isinstance(item.get("environment"), str) or not item["environment"].strip():
            issues.append(Issue("infrastructure-environment-test-environment", context))
        if not isinstance(item.get("planned_command"), str) or not item["planned_command"].strip() or "\n" in item["planned_command"]:
            issues.append(Issue("infrastructure-environment-test-command", context))
        if item.get("gap_id") not in topic_gap_ids:
            issues.append(Issue("infrastructure-environment-test-gap", f"{context}: {item.get('gap_id')!r}"))
        return issues

    required_keys = {"kind", "environment", "command", "status", "result", "source_sha", "verified_on"}
    allowed_keys = required_keys | {"evidence"}
    fields = set(item)
    if (
        status not in {"passed", "failed"}
        or (status == "passed" and fields != allowed_keys)
        or (status == "failed" and fields not in {required_keys, allowed_keys})
    ):
        return [
            Issue(
                "infrastructure-environment-test-schema",
                f"{context}: invalid status/fields={sorted(item)}",
            )
        ]
    issues = []
    if not isinstance(item.get("environment"), str) or not item["environment"].strip():
        issues.append(Issue("infrastructure-environment-test-environment", context))
    command = item.get("command")
    if not isinstance(command, str) or not command.strip() or "\n" in command:
        issues.append(Issue("infrastructure-environment-test-command", context))
    if item.get("result") != status:
        issues.append(
            Issue(
                "infrastructure-environment-test-result",
                f"{context}: status={status!r}, result={item.get('result')!r}",
            )
        )
    source_sha = item.get("source_sha")
    if not isinstance(source_sha, str) or not SHA_RE.fullmatch(source_sha) or not is_git_ancestor(source_sha, head):
        issues.append(Issue("infrastructure-environment-test-sha", context))
    if not valid_iso_date(item.get("verified_on")):
        issues.append(Issue("infrastructure-environment-test-date", context))
    if "evidence" in item:
        evidence = item["evidence"]
        if not isinstance(evidence, list) or not evidence:
            issues.append(Issue("infrastructure-environment-test-evidence", f"{context}: non-empty ref list required"))
        else:
            for evidence_index, ref in enumerate(evidence):
                if (
                    not isinstance(ref, dict)
                    or set(ref) != {"kind", "ref"}
                    or not production_evidence_ref_valid(ref.get("kind"), ref.get("ref"))
                ):
                    issues.append(
                        Issue(
                            "infrastructure-environment-test-evidence",
                            f"{context}[{evidence_index}]",
                        )
                    )
    return issues


def validate_infrastructure_production_evidence(
    head: str,
) -> tuple[dict[str, dict[str, object]], list[Issue]]:
    if not INFRA_PRODUCTION_EVIDENCE.exists():
        return {}, [
            Issue(
                "infrastructure-production-ledger-missing",
                str(INFRA_PRODUCTION_EVIDENCE.relative_to(ROOT)),
            )
        ]
    try:
        raw = json.loads(INFRA_PRODUCTION_EVIDENCE.read_text(encoding="utf-8"))
    except (json.JSONDecodeError, OSError) as error:
        return {}, [Issue("infrastructure-production-ledger-parse", str(error))]
    if not isinstance(raw, dict):
        return {}, [Issue("infrastructure-production-ledger-schema", "top-level value must be an object")]

    issues: list[Issue] = []
    if set(raw) != {"schema_version", "policy", "entries"} or raw.get("schema_version") != 1:
        issues.append(Issue("infrastructure-production-ledger-schema", f"top-level fields={sorted(raw)}"))
    expected_policy = {
        "current_evidence_requires_exact_deployed_sha": True,
        "current_evidence_max_validity_days": CURRENT_PRODUCTION_MAX_VALIDITY_DAYS,
        "expires_on_scope": "current-state eligibility, not record retention",
        "effective_config_unknown_codes": ["unknown_not_recorded"],
    }
    if raw.get("policy") != expected_policy:
        issues.append(
            Issue(
                "infrastructure-production-ledger-policy",
                f"declared={raw.get('policy')!r}, expected={expected_policy!r}",
            )
        )
    raw_entries = raw.get("entries")
    if not isinstance(raw_entries, list):
        return {}, issues + [Issue("infrastructure-production-ledger-schema", "entries must be a list")]

    expected_entry_keys = {
        "id",
        "status",
        "observed_at",
        "environment",
        "deployed_sha",
        "source_baseline_sha",
        "effective_config_hash",
        "effective_config_hash_limitation",
        "command",
        "result",
        "owner",
        "topics",
        "expires_on",
        "supersedes",
        "evidence",
        "limitations",
    }
    entries: dict[str, dict[str, object]] = {}
    for index, entry in enumerate(raw_entries):
        context = f"entries[{index}]"
        if not isinstance(entry, dict) or set(entry) != expected_entry_keys:
            fields = sorted(entry) if isinstance(entry, dict) else type(entry).__name__
            issues.append(Issue("infrastructure-production-entry-schema", f"{context}: fields={fields}"))
            continue
        entry_id = entry.get("id")
        if not isinstance(entry_id, str) or not entry_id.strip():
            issues.append(Issue("infrastructure-production-entry-id", context))
            continue
        if entry_id in entries:
            issues.append(Issue("infrastructure-production-entry-duplicate", entry_id))
            continue
        entries[entry_id] = entry

        status = entry.get("status")
        if status not in {"historical", "current", "superseded"}:
            issues.append(Issue("infrastructure-production-entry-status", f"{entry_id}: {status!r}"))
        observed_at = parse_rfc3339(entry.get("observed_at"))
        if observed_at is None:
            issues.append(Issue("infrastructure-production-entry-observed-at", entry_id))
        elif observed_at > datetime.now(observed_at.tzinfo):
            issues.append(Issue("infrastructure-production-entry-observed-at-future", entry_id))
        if not isinstance(entry.get("environment"), str) or not str(entry["environment"]).strip():
            issues.append(Issue("infrastructure-production-entry-environment", entry_id))
        for field in ("deployed_sha", "source_baseline_sha"):
            sha = entry.get(field)
            if not isinstance(sha, str) or not SHA_RE.fullmatch(sha) or not is_git_ancestor(sha, head):
                issues.append(Issue("infrastructure-production-entry-sha", f"{entry_id}.{field}: {sha!r}"))

        effective_hash = entry.get("effective_config_hash")
        hash_limitation = entry.get("effective_config_hash_limitation")
        has_hash = (
            isinstance(effective_hash, str)
            and bool(SHA256_RE.fullmatch(effective_hash))
            and effective_hash != "sha256:" + "0" * 64
        )
        has_limitation = (
            isinstance(hash_limitation, dict)
            and set(hash_limitation) == {"code", "detail"}
            and hash_limitation.get("code") == "unknown_not_recorded"
            and isinstance(hash_limitation.get("detail"), str)
            and bool(hash_limitation["detail"].strip())
        )
        if has_hash == has_limitation:
            issues.append(
                Issue(
                    "infrastructure-production-effective-config",
                    f"{entry_id}: exactly one hash or limitation is required",
                )
            )
        if effective_hash is not None and not has_hash:
            issues.append(Issue("infrastructure-production-effective-config", f"{entry_id}: invalid hash"))
        if hash_limitation is not None and not has_limitation:
            issues.append(Issue("infrastructure-production-effective-config", f"{entry_id}: invalid limitation"))

        # This is a recorded production command, not a command executed by this
        # offline gate. Immutable provenance and structured results carry the
        # trust boundary for current evidence.
        if not isinstance(entry.get("command"), str) or not str(entry["command"]).strip():
            issues.append(Issue("infrastructure-production-entry-command", entry_id))
        result = entry.get("result")
        if not isinstance(result, dict) or set(result) != {"status", "summary", "measurements"}:
            issues.append(Issue("infrastructure-production-entry-result", entry_id))
        else:
            if result.get("status") not in {"passed", "failed", "findings"}:
                issues.append(Issue("infrastructure-production-entry-result", f"{entry_id}: invalid status"))
            if not isinstance(result.get("summary"), str) or not result["summary"].strip():
                issues.append(Issue("infrastructure-production-entry-result", f"{entry_id}: empty summary"))
            if not isinstance(result.get("measurements"), dict):
                issues.append(Issue("infrastructure-production-entry-result", f"{entry_id}: measurements must be object"))
        if not isinstance(entry.get("owner"), str) or not str(entry["owner"]).strip():
            issues.append(Issue("infrastructure-production-entry-owner", entry_id))
        entry_topics = entry.get("topics")
        if (
            not isinstance(entry_topics, list)
            or not entry_topics
            or not all(isinstance(value, str) and value in INFRASTRUCTURE_TOPICS for value in entry_topics)
            or len(entry_topics) != len(set(entry_topics))
        ):
            issues.append(Issue("infrastructure-production-entry-topics", entry_id))
        if not valid_iso_date(entry.get("expires_on")):
            issues.append(Issue("infrastructure-production-entry-expires", entry_id))
        supersedes = entry.get("supersedes")
        if not isinstance(supersedes, list) or not all(isinstance(value, str) and value for value in supersedes):
            issues.append(Issue("infrastructure-production-entry-supersedes", entry_id))
        elif len(supersedes) != len(set(supersedes)) or entry_id in supersedes:
            issues.append(Issue("infrastructure-production-entry-supersedes", f"{entry_id}: duplicate/self reference"))
        evidence = entry.get("evidence")
        if not isinstance(evidence, list) or not evidence:
            issues.append(Issue("infrastructure-production-entry-evidence", f"{entry_id}: non-empty list required"))
        else:
            for evidence_index, item in enumerate(evidence):
                if not isinstance(item, dict) or set(item) != {"kind", "ref"} or not production_evidence_ref_valid(
                    item.get("kind"), item.get("ref")
                ):
                    issues.append(
                        Issue(
                            "infrastructure-production-entry-evidence",
                            f"{entry_id}[{evidence_index}]",
                        )
                    )
        limitations = entry.get("limitations")
        if (
            not isinstance(limitations, list)
            or not limitations
            or not all(isinstance(value, str) and value.strip() for value in limitations)
        ):
            issues.append(Issue("infrastructure-production-entry-limitations", entry_id))
        if status == "current":
            if not has_hash or hash_limitation is not None:
                issues.append(
                    Issue(
                        "infrastructure-production-entry-current-effective-config",
                        f"{entry_id}: current evidence requires an exact effective_config_hash",
                    )
                )
            deployed_sha = entry.get("deployed_sha")
            baseline_sha = entry.get("source_baseline_sha")
            if (
                not isinstance(deployed_sha, str)
                or not SHA_RE.fullmatch(deployed_sha)
                or not isinstance(baseline_sha, str)
                or not SHA_RE.fullmatch(baseline_sha)
                or not is_git_ancestor(baseline_sha, deployed_sha)
            ):
                issues.append(
                    Issue(
                        "infrastructure-production-entry-current-baseline-binding",
                        f"{entry_id}: source baseline must be an ancestor of the exact deployed SHA",
                    )
                )
            if deployed_sha != head:
                issues.append(
                    Issue(
                        "infrastructure-production-entry-current-checkout-binding",
                        f"{entry_id}: current evidence deployed_sha must equal checkout HEAD",
                    )
                )
            if valid_iso_date(entry.get("expires_on")):
                expires_on = date.fromisoformat(str(entry["expires_on"]))
                if expires_on < date.today():
                    issues.append(Issue("infrastructure-production-entry-expired-current", entry_id))
                if observed_at is not None:
                    validity_days = (expires_on - observed_at.date()).days
                    if not 0 <= validity_days <= CURRENT_PRODUCTION_MAX_VALIDITY_DAYS:
                        issues.append(
                            Issue(
                                "infrastructure-production-entry-validity-window",
                                f"{entry_id}: {validity_days} days, max={CURRENT_PRODUCTION_MAX_VALIDITY_DAYS}",
                            )
                        )
            if not isinstance(result, dict) or result.get("status") != "passed":
                issues.append(Issue("infrastructure-production-entry-current-result", entry_id))
            elif not isinstance(result.get("measurements"), dict) or not result["measurements"]:
                issues.append(Issue("infrastructure-production-entry-current-measurements", entry_id))
            if not isinstance(evidence, list) or not any(
                isinstance(item, dict)
                and item.get("kind") == "github_run"
                and production_evidence_ref_valid(item.get("kind"), item.get("ref"))
                for item in evidence
            ):
                issues.append(Issue("infrastructure-production-entry-current-immutable-evidence", entry_id))

    for entry_id, entry in entries.items():
        supersedes = entry.get("supersedes")
        if isinstance(supersedes, list):
            missing = sorted(set(supersedes) - entries.keys())
            if missing:
                issues.append(Issue("infrastructure-production-entry-supersedes", f"{entry_id}: missing={missing}"))
    return entries, issues


def current_production_entry_eligible(
    entry: dict[str, object],
    head: str,
    manifest_source_baseline: object,
) -> bool:
    deployed_sha = entry.get("deployed_sha")
    entry_baseline = entry.get("source_baseline_sha")
    observed_at = parse_rfc3339(entry.get("observed_at"))
    expires_on = date.fromisoformat(str(entry["expires_on"])) if valid_iso_date(entry.get("expires_on")) else None
    result = entry.get("result")
    evidence = entry.get("evidence")
    return (
        entry.get("status") == "current"
        and isinstance(result, dict)
        and result.get("status") == "passed"
        and isinstance(result.get("measurements"), dict)
        and bool(result["measurements"])
        and observed_at is not None
        and observed_at <= datetime.now(observed_at.tzinfo)
        and expires_on is not None
        and expires_on >= date.today()
        and 0 <= (expires_on - observed_at.date()).days <= CURRENT_PRODUCTION_MAX_VALIDITY_DAYS
        and isinstance(entry.get("effective_config_hash"), str)
        and bool(SHA256_RE.fullmatch(str(entry["effective_config_hash"])))
        and entry.get("effective_config_hash") != "sha256:" + "0" * 64
        and entry.get("effective_config_hash_limitation") is None
        and isinstance(manifest_source_baseline, str)
        and SHA_RE.fullmatch(manifest_source_baseline) is not None
        and entry_baseline == manifest_source_baseline
        and isinstance(deployed_sha, str)
        and SHA_RE.fullmatch(deployed_sha) is not None
        and deployed_sha == head
        and is_git_ancestor(manifest_source_baseline, deployed_sha)
        and is_git_ancestor(deployed_sha, head)
        and isinstance(evidence, list)
        and any(
            isinstance(item, dict)
            and item.get("kind") == "github_run"
            and production_evidence_ref_valid(item.get("kind"), item.get("ref"))
            for item in evidence
        )
    )


def validate_infrastructure_signoff(
    manifest: dict[str, object],
    primary_paths: set[str],
    head: str,
) -> list[Issue]:
    signoff = manifest.get("infrastructure_signoff")
    if not isinstance(signoff, dict) or set(signoff) != {"dimensions", "topics"}:
        return [Issue("infrastructure-signoff-schema", "expected exact dimensions/topics object")]
    issues: list[Issue] = []
    if signoff.get("dimensions") != MODULE_SIGNOFF_DIMENSIONS:
        issues.append(Issue("infrastructure-signoff-dimensions", f"declared={signoff.get('dimensions')!r}"))
    manifest_baseline = manifest.get("source_baseline")
    manifest_source_baseline = manifest_baseline.get("sha") if isinstance(manifest_baseline, dict) else None
    production_entries, production_issues = validate_infrastructure_production_evidence(head)
    issues.extend(production_issues)
    raw_topics = signoff.get("topics")
    if not isinstance(raw_topics, list):
        return issues + [Issue("infrastructure-signoff-schema", "topics must be a list")]

    expected_topic_keys = {
        "topic",
        "canonical_doc",
        "fact_sources",
        "non_cached_tests",
        "environment_tests",
        "production_evidence",
        "gaps",
        "dimensions",
    }
    topics: dict[str, dict[str, object]] = {}
    canonical_docs: set[str] = set()
    changed_cache: dict[str, set[str]] = {}
    for index, raw_topic in enumerate(raw_topics):
        context = f"infrastructure_signoff.topics[{index}]"
        if not isinstance(raw_topic, dict) or set(raw_topic) != expected_topic_keys:
            fields = sorted(raw_topic) if isinstance(raw_topic, dict) else type(raw_topic).__name__
            issues.append(Issue("infrastructure-topic-schema", f"{context}: fields={fields}"))
            continue
        topic = raw_topic.get("topic")
        if not isinstance(topic, str) or topic not in INFRASTRUCTURE_TOPICS:
            issues.append(Issue("infrastructure-topic-id", f"{context}: {topic!r}"))
            continue
        if topic in topics:
            issues.append(Issue("infrastructure-topic-duplicate", topic))
            continue
        topics[topic] = raw_topic

        canonical_doc = raw_topic.get("canonical_doc")
        expected_doc = INFRASTRUCTURE_TOPICS[topic]
        if canonical_doc != expected_doc:
            issues.append(
                Issue(
                    "infrastructure-topic-canonical-doc",
                    f"{topic}: declared={canonical_doc!r}, expected={expected_doc!r}",
                )
            )
        if not isinstance(canonical_doc, str) or canonical_doc not in primary_paths or not (ROOT / canonical_doc).is_file():
            issues.append(Issue("infrastructure-topic-canonical-doc", f"{topic}: not an active primary document"))
        elif canonical_doc in canonical_docs:
            issues.append(Issue("infrastructure-topic-canonical-duplicate", canonical_doc))
        else:
            canonical_docs.add(canonical_doc)

        fact_sources = raw_topic.get("fact_sources")
        source_patterns: list[str] = []
        if not isinstance(fact_sources, list) or not fact_sources:
            issues.append(Issue("infrastructure-topic-fact-sources", f"{topic}: non-empty list required"))
        else:
            seen_sources: set[str] = set()
            for source_index, source in enumerate(fact_sources):
                if (
                    not isinstance(source, dict)
                    or set(source) != {"kind", "path"}
                    or source.get("kind") != "source"
                    or not isinstance(source.get("path"), str)
                ):
                    issues.append(Issue("infrastructure-topic-fact-source-schema", f"{topic}[{source_index}]"))
                    continue
                pattern = source["path"]
                if pattern in seen_sources:
                    issues.append(Issue("infrastructure-topic-fact-source-duplicate", f"{topic}: {pattern}"))
                seen_sources.add(pattern)
                if not manifest_source_matches(pattern):
                    issues.append(Issue("infrastructure-topic-fact-source-missing", f"{topic}: {pattern}"))
                else:
                    source_patterns.append(pattern)
        required_scopes = INFRASTRUCTURE_REQUIRED_SOURCE_SCOPES.get(topic, {})
        for scope_name, anchors in required_scopes.items():
            missing_anchors = [anchor for anchor in anchors if not (ROOT / anchor).exists()]
            if missing_anchors:
                issues.append(
                    Issue(
                        "infrastructure-required-source-anchor-missing",
                        f"{topic}.{scope_name}: {missing_anchors}",
                    )
                )
                continue
            if not any(
                path_matches_scope(anchor, pattern)
                for anchor in anchors
                for pattern in source_patterns
            ):
                issues.append(
                    Issue(
                        "infrastructure-topic-required-source-scope",
                        f"{topic}.{scope_name}: anchors={list(anchors)}",
                    )
                )

        topic_gaps = raw_topic.get("gaps")
        topic_gap_ids: set[str] = set()
        if not isinstance(topic_gaps, list):
            issues.append(Issue("infrastructure-topic-gaps", f"{topic}: expected list"))
        else:
            for gap_index, gap in enumerate(topic_gaps):
                issues.extend(validate_exact_gap(gap, f"{topic}.gaps[{gap_index}]"))
                if isinstance(gap, dict) and isinstance(gap.get("id"), str):
                    if gap["id"] in topic_gap_ids:
                        issues.append(Issue("infrastructure-topic-gap-duplicate", f"{topic}: {gap['id']}"))
                    topic_gap_ids.add(gap["id"])

        non_cached_tests = raw_topic.get("non_cached_tests")
        if not isinstance(non_cached_tests, list) or not non_cached_tests:
            issues.append(Issue("infrastructure-topic-non-cached-tests", f"{topic}: non-empty list required"))
            non_cached_tests = []
        for evidence_index, item in enumerate(non_cached_tests):
            if not isinstance(item, dict) or item.get("kind") != "command":
                issues.append(Issue("infrastructure-topic-non-cached-test-kind", f"{topic}[{evidence_index}]"))
            issues.extend(validate_evidence_item(item, f"{topic}.non_cached_tests[{evidence_index}]", head))
            if isinstance(item, dict) and not command_is_non_cached_test(item.get("command")):
                issues.append(
                    Issue(
                        "infrastructure-topic-non-cached-test-command",
                        f"{topic}[{evidence_index}]: full Go suites require -count=1 and forbid -run",
                    )
                )

        environment_tests = raw_topic.get("environment_tests")
        if not isinstance(environment_tests, list) or not environment_tests:
            issues.append(Issue("infrastructure-topic-environment-tests", f"{topic}: non-empty list required"))
            environment_tests = []
        for evidence_index, item in enumerate(environment_tests):
            issues.extend(
                validate_environment_test(
                    item,
                    f"{topic}.environment_tests[{evidence_index}]",
                    head,
                    topic_gap_ids,
                )
            )

        production_refs = raw_topic.get("production_evidence")
        resolved_production: list[dict[str, object]] = []
        if not isinstance(production_refs, list) or not all(isinstance(value, str) and value for value in production_refs):
            issues.append(Issue("infrastructure-topic-production-evidence", f"{topic}: string id list required"))
            production_refs = []
        elif len(production_refs) != len(set(production_refs)):
            issues.append(Issue("infrastructure-topic-production-evidence", f"{topic}: duplicate ids"))
        for evidence_id in production_refs:
            entry = production_entries.get(evidence_id)
            if entry is None:
                issues.append(Issue("infrastructure-topic-production-evidence-missing", f"{topic}: {evidence_id}"))
            elif topic not in entry.get("topics", []):
                issues.append(
                    Issue(
                        "infrastructure-topic-production-evidence-unrelated",
                        f"{topic}: {evidence_id}",
                    )
                )
            else:
                resolved_production.append(entry)

        dimensions = raw_topic.get("dimensions")
        if not isinstance(dimensions, dict) or set(dimensions) != set(MODULE_SIGNOFF_DIMENSIONS):
            issues.append(Issue("infrastructure-topic-dimension-inventory", topic))
            continue
        for dimension in MODULE_SIGNOFF_DIMENSIONS:
            value = dimensions[dimension]
            dimension_context = f"{topic}.{dimension}"
            expected_dimension_keys = {"review_state", "rationale", "evidence", "gaps", "signoff"}
            if not isinstance(value, dict) or set(value) != expected_dimension_keys:
                fields = sorted(value) if isinstance(value, dict) else type(value).__name__
                issues.append(Issue("infrastructure-dimension-schema", f"{dimension_context}: fields={fields}"))
                continue
            review_state = value.get("review_state")
            if review_state not in EXPECTED_REVIEW_STATES:
                issues.append(Issue("infrastructure-dimension-review-state", f"{dimension_context}: {review_state!r}"))
            rationale = value.get("rationale")
            if rationale is not None and (not isinstance(rationale, str) or not rationale.strip()):
                issues.append(Issue("infrastructure-dimension-rationale", dimension_context))
            if review_state == "not_applicable" and not isinstance(rationale, str):
                issues.append(Issue("infrastructure-dimension-rationale", f"{dimension_context}: required"))

            dimension_evidence = value.get("evidence")
            if not isinstance(dimension_evidence, list):
                issues.append(Issue("infrastructure-dimension-evidence", f"{dimension_context}: expected list"))
                dimension_evidence = []
            for evidence_index, item in enumerate(dimension_evidence):
                issues.extend(validate_evidence_item(item, f"{dimension_context}.evidence[{evidence_index}]", head))
            dimension_gaps = value.get("gaps")
            if not isinstance(dimension_gaps, list):
                issues.append(Issue("infrastructure-dimension-gaps", f"{dimension_context}: expected list"))
                dimension_gaps = []
            for gap_index, gap in enumerate(dimension_gaps):
                issues.extend(validate_exact_gap(gap, f"{dimension_context}.gaps[{gap_index}]"))
            if review_state == "blocked" and not dimension_gaps:
                issues.append(Issue("infrastructure-dimension-blocked-without-gap", dimension_context))
            if review_state == "ready" and not dimension_evidence:
                issues.append(Issue("infrastructure-dimension-ready-without-evidence", dimension_context))

            dimension_signoff = value.get("signoff")
            expected_signoff_keys = {"state", "by", "at", "source_sha"}
            if not isinstance(dimension_signoff, dict) or set(dimension_signoff) != expected_signoff_keys:
                issues.append(Issue("infrastructure-dimension-signoff-schema", dimension_context))
                continue
            state = dimension_signoff.get("state")
            if state == "unsigned":
                if dimension_signoff != {"state": "unsigned", "by": None, "at": None, "source_sha": None}:
                    issues.append(Issue("infrastructure-dimension-unsigned-metadata", dimension_context))
            elif state == "signed":
                signed_sha = dimension_signoff.get("source_sha")
                if review_state != "ready" or not dimension_evidence:
                    issues.append(Issue("infrastructure-dimension-signed-without-ready-evidence", dimension_context))
                if not isinstance(dimension_signoff.get("by"), str) or not dimension_signoff["by"].strip():
                    issues.append(Issue("infrastructure-dimension-signoff-by", dimension_context))
                if not valid_iso_date(dimension_signoff.get("at")):
                    issues.append(Issue("infrastructure-dimension-signoff-date", dimension_context))
                if not isinstance(signed_sha, str) or not SHA_RE.fullmatch(signed_sha) or not is_git_ancestor(signed_sha, head):
                    issues.append(Issue("infrastructure-dimension-signoff-sha", dimension_context))
            else:
                issues.append(Issue("infrastructure-dimension-signoff-state", f"{dimension_context}: {state!r}"))

            upgraded = review_state == "ready" or state == "signed"
            if upgraded:
                for evidence_index, item in enumerate(dimension_evidence):
                    if (
                        isinstance(item, dict)
                        and item.get("kind") == "source_selector"
                        and source_selector_is_trivial(item.get("selector"))
                    ):
                        issues.append(
                            Issue(
                                "infrastructure-dimension-trivial-selector",
                                f"{dimension_context}.evidence[{evidence_index}]",
                            )
                        )
            if upgraded and source_patterns:
                evidence_shas = {
                    item.get("source_sha")
                    for item in dimension_evidence
                    if isinstance(item, dict)
                    and isinstance(item.get("source_sha"), str)
                    and SHA_RE.fullmatch(str(item["source_sha"]))
                    and is_git_ancestor(str(item["source_sha"]), head)
                }
                if state == "signed" and isinstance(dimension_signoff.get("source_sha"), str):
                    signed_sha = str(dimension_signoff["source_sha"])
                    if SHA_RE.fullmatch(signed_sha) and is_git_ancestor(signed_sha, head):
                        evidence_shas.add(signed_sha)
                for evidence_sha in sorted(evidence_shas):
                    changed = changed_paths_since(evidence_sha, changed_cache)
                    stale = sorted(
                        changed_path
                        for changed_path in changed
                        if any(path_matches_scope(changed_path, pattern) for pattern in source_patterns)
                    )
                    if stale:
                        issues.append(
                            Issue(
                                "infrastructure-topic-fact-source-stale",
                                f"{dimension_context}: evidence={evidence_sha}, changed={stale[:10]}",
                            )
                        )
            if dimension == "test" and upgraded and not non_cached_tests:
                issues.append(Issue("infrastructure-test-upgrade-without-non-cached-test", topic))
            passed_environment_tests = [
                item
                for item in environment_tests
                if isinstance(item, dict) and item.get("status") == "passed" and item.get("result") == "passed"
            ]
            if dimension == "operations" and upgraded and not passed_environment_tests:
                issues.append(Issue("infrastructure-operations-upgrade-without-environment-test", topic))
            if dimension == "production" and upgraded:
                eligible = [
                    entry
                    for entry in resolved_production
                    if current_production_entry_eligible(entry, head, manifest_source_baseline)
                ]
                if not eligible:
                    issues.append(Issue("infrastructure-production-upgrade-without-current-evidence", topic))
            if dimension == "production" and not production_refs:
                has_production_gap = any(
                    isinstance(gap, dict) and gap.get("kind") == "production"
                    for gap in dimension_gaps
                )
                if review_state == "ready" or state != "unsigned" or not has_production_gap:
                    issues.append(
                        Issue(
                            "infrastructure-production-empty-evidence-without-gap",
                            f"{topic}: empty evidence requires non-ready, unsigned production dimension with production gap",
                        )
                    )

    if set(topics) != set(INFRASTRUCTURE_TOPICS):
        issues.append(
            Issue(
                "infrastructure-topic-inventory",
                f"declared={sorted(topics)}, expected={sorted(INFRASTRUCTURE_TOPICS)}",
            )
        )
    return issues


def validate_budget_settings(manifest: dict[str, object]) -> tuple[list[dict[str, object]], list[Issue]]:
    issues: list[Issue] = []
    budgets = manifest.get("budgets")
    if not isinstance(budgets, dict):
        return [], [Issue("closure-budget-schema", "missing budgets object")]
    expected = {
        "active_markdown": {"target": TARGET_ACTIVE_MARKDOWN, "hard_ceiling": HARD_MAX_ACTIVE_MARKDOWN},
        "business_module_markdown": {
            "target": TARGET_BUSINESS_MODULE_MARKDOWN,
            "hard_ceiling": HARD_MAX_BUSINESS_MODULE_MARKDOWN,
        },
    }
    for key, value in expected.items():
        if budgets.get(key) != value:
            issues.append(Issue("closure-budget-drift", f"{key}: declared={budgets.get(key)!r}, expected={value!r}"))
    exceptions = budgets.get("exceptions", [])
    if not isinstance(exceptions, list):
        return [], issues + [Issue("closure-budget-schema", "budgets.exceptions must be a list")]
    valid: list[dict[str, object]] = []
    for index, exception in enumerate(exceptions):
        if not isinstance(exception, dict):
            issues.append(Issue("closure-budget-exception-schema", f"index={index}: expected object"))
            continue
        missing = [key for key in ("scope", "reason", "owner", "review_date") if not exception.get(key)]
        if missing:
            issues.append(Issue("closure-budget-exception-schema", f"index={index}: missing={missing}"))
            continue
        if exception["owner"] not in EXPECTED_OWNERS:
            issues.append(Issue("closure-budget-exception-owner", f"index={index}: {exception['owner']!r}"))
            continue
        if not valid_iso_date(exception["review_date"]):
            issues.append(Issue("closure-budget-exception-date", f"index={index}: {exception['review_date']!r}"))
            continue
        valid.append(exception)
    return valid, issues


def budget_exception_covers(exceptions: list[dict[str, object]], paths: list[str], total_scope: bool = False) -> bool:
    for exception in exceptions:
        scope = str(exception["scope"])
        if total_scope and scope == "active_markdown":
            return True
        if paths and all(path_matches_scope(path, scope) for path in paths):
            return True
    return False


def validate_closure_manifest(
    manifest: dict[str, object],
    primary_files: list[Path],
    maintained_files: list[Path],
    head: str,
    source_baseline: str,
    actual_contracts: dict[str, object],
) -> tuple[list[Issue], list[dict[str, object]]]:
    issues: list[Issue] = []
    if manifest.get("schema_version") != 1:
        issues.append(Issue("closure-schema-version", f"expected 1, got {manifest.get('schema_version')!r}"))
    if manifest.get("checkout_ref") != "git:HEAD":
        issues.append(Issue("closure-checkout-ref", "checkout_ref must be symbolic git:HEAD"))

    expected_status_policy = {
        "doc_status": sorted(EXPECTED_DOC_STATUS),
        "review_state": sorted(EXPECTED_REVIEW_STATES),
        "signoff_state": sorted(EXPECTED_SIGNOFF_STATES),
        "production_level": sorted(EXPECTED_PRODUCTION_LEVELS),
    }
    if manifest.get("status_policy") != expected_status_policy:
        issues.append(
            Issue(
                "closure-status-policy",
                f"declared={manifest.get('status_policy')!r}, expected={expected_status_policy!r}",
            )
        )

    baseline = manifest.get("source_baseline")
    if not isinstance(baseline, dict):
        issues.append(Issue("closure-source-baseline", "missing source_baseline object"))
    else:
        if baseline.get("sha") != source_baseline:
            issues.append(
                Issue(
                    "closure-source-baseline-drift",
                    f"declared={baseline.get('sha')!r}, source={source_baseline}",
                )
            )
        if not valid_iso_date(baseline.get("verified_on")):
            issues.append(Issue("closure-source-baseline-date", f"invalid={baseline.get('verified_on')!r}"))

    coverage = manifest.get("coverage")
    coverage_counts: dict[str, int] = {}
    expected_coverage_policy = {
        "primary": {
            "include": ["docs/**/*.md"],
            "exclude": ["docs/_archive/**/*.md"],
            "budgeted": True,
        },
        "sidecars": {
            "include": ["**/*.md"],
            "exclude": ["docs/**/*.md", "docs/_archive/**/*.md", "tmp/**/*.md"],
            "budgeted": False,
        },
    }
    if not isinstance(coverage, dict) or coverage.get("mode") != "exactly_one":
        issues.append(Issue("closure-coverage-policy", "coverage.mode must be exactly_one"))
    else:
        for class_name, expected_policy in expected_coverage_policy.items():
            declared_policy = coverage.get(class_name)
            if not isinstance(declared_policy, dict):
                issues.append(Issue("closure-coverage-policy", f"coverage.{class_name} must be an object"))
                continue
            for key, expected_value in expected_policy.items():
                if declared_policy.get(key) != expected_value:
                    issues.append(
                        Issue(
                            "closure-coverage-policy",
                            f"coverage.{class_name}.{key}: declared={declared_policy.get(key)!r}, expected={expected_value!r}",
                        )
                    )
            declared_count = declared_policy.get("expected_count")
            if type(declared_count) is not int or declared_count < 0:
                issues.append(
                    Issue(
                        "closure-coverage-count",
                        f"coverage.{class_name}.expected_count must be a non-negative integer",
                    )
                )
            else:
                coverage_counts[class_name] = declared_count

    exceptions, budget_issues = validate_budget_settings(manifest)
    issues.extend(budget_issues)
    issues.extend(validate_machine_contracts(manifest, actual_contracts))

    raw_documents = manifest.get("documents")
    if not isinstance(raw_documents, list):
        return issues + [Issue("closure-document-schema", "documents must be a list")], exceptions
    primary_paths = {path.relative_to(ROOT).as_posix() for path in primary_files}
    maintained_paths = {path.relative_to(ROOT).as_posix() for path in maintained_files}
    sidecar_paths = maintained_paths - primary_paths
    if coverage_counts.get("primary") != len(primary_paths):
        issues.append(
            Issue(
                "closure-primary-count",
                f"source={len(primary_paths)}, declared={coverage_counts.get('primary')!r}",
            )
        )
    if coverage_counts.get("sidecars") != len(sidecar_paths):
        issues.append(
            Issue(
                "closure-sidecar-count",
                f"source={len(sidecar_paths)}, declared={coverage_counts.get('sidecars')!r}",
            )
        )
    declared: dict[str, dict[str, object]] = {}
    for index, raw in enumerate(raw_documents):
        if not isinstance(raw, dict) or not isinstance(raw.get("path"), str):
            issues.append(Issue("closure-document-schema", f"index={index}: missing path"))
            continue
        path = raw["path"]
        if path in declared:
            issues.append(Issue("closure-document-duplicate", path))
            continue
        declared[path] = raw
    for path in sorted(maintained_paths - declared.keys()):
        issues.append(Issue("closure-document-uncovered", path))
    for path in sorted(declared.keys() - maintained_paths):
        issues.append(Issue("closure-document-out-of-scope", path))

    changed_cache: dict[str, set[str]] = {}
    for path in sorted(maintained_paths & declared.keys()):
        entry = declared[path]
        expected_class = "primary" if path in primary_paths else "sidecar"
        if entry.get("class") != expected_class:
            issues.append(
                Issue(
                    "closure-document-class",
                    f"{path}: declared={entry.get('class')!r}, expected={expected_class}",
                )
            )
        owner = entry.get("owner")
        status = entry.get("doc_status")
        if owner not in EXPECTED_OWNERS:
            issues.append(Issue("closure-document-owner", f"{path}: {owner!r}"))
        if status not in EXPECTED_DOC_STATUS:
            issues.append(Issue("closure-document-status", f"{path}: {status!r}"))

        verified = entry.get("verified")
        verified_sha: str | None = None
        if not isinstance(verified, dict):
            issues.append(Issue("closure-document-verified", f"{path}: expected object"))
        else:
            raw_sha = verified.get("source_sha")
            raw_date = verified.get("verified_on")
            if raw_sha is None and raw_date is None:
                if status == "aligned":
                    issues.append(Issue("closure-document-unverified-aligned", path))
            elif not isinstance(raw_sha, str) or not SHA_RE.fullmatch(raw_sha) or not valid_iso_date(raw_date):
                issues.append(Issue("closure-document-verified", f"{path}: invalid source_sha/verified_on"))
            elif not is_git_ancestor(raw_sha, head):
                issues.append(Issue("closure-document-source-sha", f"{path}: {raw_sha} is not an ancestor of HEAD"))
            else:
                verified_sha = raw_sha

        fact_sources = entry.get("fact_sources")
        if not isinstance(fact_sources, list):
            issues.append(Issue("closure-document-fact-sources", f"{path}: expected list"))
            fact_sources = []
        if status == "aligned" and not fact_sources:
            issues.append(Issue("closure-document-aligned-without-sources", path))
        source_patterns: list[str] = []
        for source_index, source in enumerate(fact_sources):
            if not isinstance(source, dict) or not source.get("kind") or not isinstance(source.get("path"), str):
                issues.append(Issue("closure-document-fact-source-schema", f"{path}: index={source_index}"))
                continue
            pattern = source["path"]
            if not manifest_source_matches(pattern):
                issues.append(Issue("closure-document-fact-source-missing", f"{path}: {pattern}"))
                continue
            source_patterns.append(pattern)

        verification = entry.get("verification")
        if not isinstance(verification, list):
            issues.append(Issue("closure-document-verification", f"{path}: expected list"))
            verification = []
        if status == "aligned" and not verification:
            issues.append(Issue("closure-document-aligned-without-verification", path))
        for evidence_index, evidence in enumerate(verification):
            issues.extend(validate_evidence_item(evidence, f"{path}.verification[{evidence_index}]", head))

        production = entry.get("production_evidence")
        if not isinstance(production, dict) or production.get("level") not in EXPECTED_PRODUCTION_LEVELS:
            issues.append(Issue("closure-production-evidence", f"{path}: invalid level"))
        else:
            refs = production.get("refs")
            limitations = production.get("limitations")
            if not isinstance(refs, list) or not isinstance(limitations, list):
                issues.append(Issue("closure-production-evidence", f"{path}: refs/limitations must be lists"))
            if production.get("level") == "current":
                deployed_sha = production.get("deployed_sha")
                if not refs or not isinstance(deployed_sha, str) or not SHA_RE.fullmatch(deployed_sha):
                    issues.append(Issue("closure-current-production-evidence", f"{path}: refs/deployed_sha required"))
                if not valid_iso_date(production.get("observed_at")):
                    issues.append(Issue("closure-current-production-evidence", f"{path}: observed_at must be YYYY-MM-DD"))

        gaps = entry.get("gaps")
        if not isinstance(gaps, list):
            issues.append(Issue("closure-document-gaps", f"{path}: expected list"))
            gaps = []
        if status in {"drifted", "needs_review"} and not gaps:
            issues.append(Issue("closure-document-status-without-gap", path))
        for gap_index, gap in enumerate(gaps):
            issues.extend(validate_gap(gap, f"{path}.gaps[{gap_index}]"))

        if status == "aligned" and verified_sha and source_patterns:
            changed = changed_paths_since(verified_sha, changed_cache)
            stale = sorted(
                changed_path
                for changed_path in changed
                if any(path_matches_scope(changed_path, pattern) for pattern in source_patterns)
            )
            if stale:
                issues.append(
                    Issue(
                        "closure-aligned-source-changed",
                        f"{path}: verified={verified_sha}, changed={stale[:10]}",
                    )
                )

    signoff = manifest.get("module_signoff")
    if not isinstance(signoff, dict):
        issues.append(Issue("closure-module-signoff", "missing module_signoff object"))
    else:
        if signoff.get("dimensions") != MODULE_SIGNOFF_DIMENSIONS:
            issues.append(Issue("closure-module-dimensions", f"declared={signoff.get('dimensions')!r}"))
        rows = signoff.get("rows")
        if not isinstance(rows, list):
            issues.append(Issue("closure-module-signoff", "rows must be a list"))
        else:
            by_module: dict[str, dict[str, object]] = {}
            for row in rows:
                if not isinstance(row, dict) or not isinstance(row.get("module"), str):
                    issues.append(Issue("closure-module-row", f"invalid={row!r}"))
                    continue
                module = row["module"]
                if module in by_module:
                    issues.append(Issue("closure-module-duplicate", module))
                    continue
                by_module[module] = row
            if set(by_module) != set(BUSINESS_DOC_DIRS):
                issues.append(
                    Issue(
                        "closure-module-inventory",
                        f"declared={sorted(by_module)}, expected={sorted(BUSINESS_DOC_DIRS)}",
                    )
                )
            for module, row in by_module.items():
                dimensions = row.get("dimensions")
                if not isinstance(dimensions, dict) or set(dimensions) != set(MODULE_SIGNOFF_DIMENSIONS):
                    issues.append(Issue("closure-module-dimension-inventory", module))
                    continue
                for dimension in MODULE_SIGNOFF_DIMENSIONS:
                    value = dimensions[dimension]
                    context = f"{module}.{dimension}"
                    if not isinstance(value, dict):
                        issues.append(Issue("closure-module-dimension-schema", context))
                        continue
                    review_state = value.get("review_state")
                    if review_state not in EXPECTED_REVIEW_STATES:
                        issues.append(Issue("closure-module-review-state", f"{context}: {review_state!r}"))
                    evidence = value.get("evidence")
                    gaps = value.get("gaps")
                    if not isinstance(evidence, list) or not isinstance(gaps, list):
                        issues.append(Issue("closure-module-evidence-schema", context))
                        continue
                    for evidence_index, item in enumerate(evidence):
                        issues.extend(validate_evidence_item(item, f"{context}.evidence[{evidence_index}]", head))
                    for gap_index, gap in enumerate(gaps):
                        issues.extend(validate_gap(gap, f"{context}.gaps[{gap_index}]"))
                    if review_state == "blocked" and not gaps:
                        issues.append(Issue("closure-module-blocked-without-gap", context))
                    if review_state == "not_applicable" and not value.get("rationale"):
                        issues.append(Issue("closure-module-not-applicable-without-rationale", context))
                    dimension_signoff = value.get("signoff")
                    if not isinstance(dimension_signoff, dict) or dimension_signoff.get("state") not in EXPECTED_SIGNOFF_STATES:
                        issues.append(Issue("closure-module-signoff-schema", context))
                        continue
                    if dimension_signoff["state"] == "signed":
                        signed_sha = dimension_signoff.get("source_sha")
                        if not evidence or not dimension_signoff.get("by") or not valid_iso_date(dimension_signoff.get("at")):
                            issues.append(Issue("closure-module-signed-without-evidence", context))
                        if not isinstance(signed_sha, str) or not SHA_RE.fullmatch(signed_sha) or not is_git_ancestor(signed_sha, head):
                            issues.append(Issue("closure-module-signoff-sha", f"{context}: {signed_sha!r}"))

    issues.extend(validate_infrastructure_signoff(manifest, primary_paths, head))
    return issues, exceptions


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
    generated_contracts = {
        "configs/env/config.prod.env": (PACKAGE_SCRIPT, "configs/env/config.prod.env"),
    }
    if candidate in generated_contracts:
        generator, selector = generated_contracts[candidate]
        return generator.is_file() and selector in generator.read_text(encoding="utf-8")
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
        return bool(manifest_source_matches(candidate))
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


def camel_to_snake(value: str) -> str:
    first_pass = re.sub(r"(.)([A-Z][a-z]+)", r"\1_\2", value)
    return re.sub(r"([a-z0-9])([A-Z])", r"\1_\2", first_pass).lower()


def yaml_scalar_inventory(text: str) -> tuple[dict[tuple[str, ...], str], set[tuple[str, ...]]]:
    """Read simple YAML mappings and report ambiguous duplicate keys.

    The checked configuration fragments only need mapping/scalar semantics, so
    the gate stays dependency-free. Mapping keys are recorded as well as scalar
    keys: a repeated section must fail closed instead of silently replacing the
    earlier section as the old helper did.
    """

    values: dict[tuple[str, ...], str] = {}
    duplicates: set[tuple[str, ...]] = set()
    seen: set[tuple[str, ...]] = set()
    stack: list[tuple[int, str]] = []
    sequence_indexes: dict[tuple[tuple[str, ...], int], int] = {}
    for raw_line in text.splitlines():
        if not raw_line.strip() or raw_line.lstrip().startswith("#"):
            continue
        sequence_match = re.match(
            r"^(?P<indent> *)-\s+(?P<key>[A-Za-z0-9_.-]+):(?P<value>.*)$",
            raw_line,
        )
        if sequence_match:
            indent = len(sequence_match.group("indent"))
            while stack and stack[-1][0] >= indent:
                stack.pop()
            parent = tuple(item[1] for item in stack)
            counter_key = (parent, indent)
            sequence_indexes[counter_key] = sequence_indexes.get(counter_key, -1) + 1
            stack.append((indent, f"[{sequence_indexes[counter_key]}]"))
            match = sequence_match
            key_indent = indent + 1
        else:
            if raw_line.lstrip().startswith("- "):
                continue
            match = re.match(r"^(?P<indent> *)(?P<key>[A-Za-z0-9_.-]+):(?P<value>.*)$", raw_line)
            key_indent = len(match.group("indent")) if match else 0
        if not match:
            continue
        if not sequence_match:
            while stack and stack[-1][0] >= key_indent:
                stack.pop()
        key = match.group("key")
        path = tuple(item[1] for item in stack) + (key,)
        if path in seen:
            duplicates.add(path)
        seen.add(path)
        value = re.sub(r"\s+#.*$", "", match.group("value")).strip()
        if not value:
            stack.append((key_indent, key))
            continue
        if len(value) >= 2 and value[0] == value[-1] and value[0] in {'"', "'"}:
            value = value[1:-1]
        values[path] = value
    return values, duplicates


def yaml_scalar_values(text: str) -> dict[tuple[str, ...], str]:
    """Return scalar values for callers whose validator handles duplicates."""

    return yaml_scalar_inventory(text)[0]


def yaml_duplicate_issues(path: Path, text: str) -> list[Issue]:
    _, duplicates = yaml_scalar_inventory(text)
    return [
        Issue(
            "yaml-duplicate-key",
            f"{path.relative_to(ROOT)}: {'.'.join(key_path)}",
        )
        for key_path in sorted(duplicates)
    ]


def normalize_markdown_cell(value: str) -> str:
    value = value.strip()
    while len(value) >= 2 and value.startswith("**") and value.endswith("**"):
        value = value[2:-2].strip()
    return value.replace("`", "").strip()


def markdown_table(text: str, headers: tuple[str, ...]) -> list[list[str]]:
    lines = text.splitlines()
    for index, line in enumerate(lines[:-1]):
        if not line.lstrip().startswith("|"):
            continue
        cells = [normalize_markdown_cell(cell) for cell in line.strip().strip("|").split("|")]
        if tuple(cells) != headers:
            continue
        separator = [cell.strip() for cell in lines[index + 1].strip().strip("|").split("|")]
        if len(separator) != len(headers) or not all(re.fullmatch(r":?-{3,}:?", cell) for cell in separator):
            return []
        rows: list[list[str]] = []
        for row_line in lines[index + 2 :]:
            if not row_line.lstrip().startswith("|"):
                break
            row = [normalize_markdown_cell(cell) for cell in row_line.strip().strip("|").split("|")]
            if len(row) != len(headers):
                break
            rows.append(row)
        return rows
    return []


def signal_topology(text: str) -> dict[str, dict[str, object]]:
    scalar = yaml_scalar_values(text)
    names = yaml_section_keys(text, "signals", r"[a-z0-9_]+")
    subscribers: dict[str, list[str]] = {name: [] for name in names}
    current: str | None = None
    in_subscribers = False
    in_signals = False
    for line in text.splitlines():
        if line == "signals:":
            in_signals = True
            continue
        if not in_signals:
            continue
        if line and not line[0].isspace() and not line.lstrip().startswith("#"):
            break
        name_match = re.fullmatch(r"  ([a-z0-9_]+):\s*", line)
        if name_match:
            current = name_match.group(1)
            in_subscribers = False
            continue
        if current is None:
            continue
        if re.fullmatch(r"    subscribers:\s*", line):
            in_subscribers = True
            continue
        subscriber = re.fullmatch(r"      -\s*([A-Za-z0-9_-]+)\s*", line)
        if in_subscribers and subscriber:
            subscribers[current].append(subscriber.group(1))
            continue
        if line.startswith("    ") and not line.startswith("      -"):
            in_subscribers = False

    result: dict[str, dict[str, object]] = {}
    for name in names:
        publisher = scalar.get(("signals", name, "publisher"), "")
        result[name] = {
            "delivery": scalar.get(("signals", name, "delivery"), ""),
            "transport": scalar.get(("signals", name, "transport"), ""),
            "publishers": sorted(part.strip() for part in publisher.split(",") if part.strip()),
            "subscribers": sorted(subscribers[name]),
        }
    return dict(sorted(result.items()))


def signal_doc_topology(text: str) -> dict[str, dict[str, object]]:
    rows = markdown_table(
        text,
        ("Signal", "Delivery", "Transport", "Publishers", "Subscribers", "作用"),
    )
    result: dict[str, dict[str, object]] = {}
    for row in rows:
        publishers = sorted(part.strip() for part in re.split(r"[,\u3001]", row[3]) if part.strip())
        subscribers = sorted(part.strip() for part in re.split(r"[,\u3001]", row[4]) if part.strip())
        result[row[0]] = {
            "delivery": row[1],
            "transport": row[2],
            "publishers": publishers,
            "subscribers": subscribers,
        }
    return dict(sorted(result.items()))


def go_duration_token(expression: str) -> str:
    match = re.fullmatch(r"(?:(\d+)\s*\*\s*)?time\.(Second|Minute|Hour)", expression.strip())
    if not match:
        return expression.strip()
    amount = match.group(1) or "1"
    suffix = {"Second": "s", "Minute": "m", "Hour": "h"}[match.group(2)]
    return amount + suffix


def locklease_inventory(text: str) -> list[tuple[str, str, str, str, str]]:
    workload_ids = dict(
        re.findall(r'\b(Workload[A-Za-z0-9]+)\s+WorkloadID\s*=\s*"([a-z0-9_]+)"', text)
    )
    kinds = dict(re.findall(r'\b(Kind[A-Za-z0-9]+)\s+Kind\s*=\s*"([a-z_]+)"', text))
    renewal_modes = dict(
        re.findall(r'\b(RenewalMode[A-Za-z0-9]+)\s+RenewalMode\s*=\s*"([a-z_]+)"', text)
    )
    capabilities = re.findall(
        r'\{(Workload[A-Za-z0-9]+),\s*"([a-z-]+)",\s*(Kind[A-Za-z0-9]+),'
        r'\s*Spec\{.*?DefaultTTL:\s*([^},]+)\},\s*(RenewalMode[A-Za-z0-9]+)\}',
        text,
    )
    result: list[tuple[str, str, str, str, str]] = []
    for workload, component, kind, ttl, renewal_mode in capabilities:
        if workload not in workload_ids or kind not in kinds or renewal_mode not in renewal_modes:
            continue
        result.append(
            (
                component,
                workload_ids[workload],
                kinds[kind],
                go_duration_token(ttl),
                renewal_modes[renewal_mode],
            )
        )
    return result


def locklease_doc_inventory(text: str) -> list[tuple[str, str, str, str]]:
    rows = markdown_table(text, ("component", "workload", "kind", "默认 TTL", "用途"))
    return [(row[0], row[1], row[2].replace(" ", "_"), row[3]) for row in rows]


CHINESE_COUNTS = {
    "一": 1,
    "两": 2,
    "二": 2,
    "三": 3,
    "四": 4,
    "五": 5,
    "六": 6,
    "七": 7,
    "八": 8,
    "九": 9,
    "十": 10,
    "十一": 11,
    "十二": 12,
    "十三": 13,
}


def parsed_count(value: str) -> int | None:
    if value.isdigit():
        return int(value)
    return CHINESE_COUNTS.get(value)


def explicit_inventory_counts(text: str, noun: str) -> list[int]:
    values = re.findall(rf"([0-9一二三四五六七八九十两]+)\s*个\s+(?:apiserver\s+)?(?:Scheduler\s+)?{noun}", text, re.I)
    return [count for value in values if (count := parsed_count(value)) is not None]


def section_between(text: str, start: str, end: str | None) -> str:
    start_index = text.find(start)
    if start_index < 0:
        return ""
    end_index = text.find(end, start_index + len(start)) if end else -1
    return text[start_index : end_index if end_index >= 0 else None]


def ordered_tokens(text: str, tokens: list[str]) -> bool:
    cursor = 0
    for token in tokens:
        index = text.find(token, cursor)
        if index < 0:
            return False
        cursor = index + len(token)
    return True


def process_stage_names(text: str) -> list[str]:
    block = re.search(
        r"stages:\s*\[\]processruntime\.Stage\[prepareState\]\{(?P<body>.*?)\n\s*\},",
        text,
        flags=re.DOTALL,
    )
    if not block:
        return []
    stage_types = re.findall(r"\b([a-z][A-Za-z0-9]*Stage)\{", block.group("body"))
    names = dict(
        re.findall(
            r'func \(([a-z][A-Za-z0-9]*Stage)\) Name\(\) string \{ return "([^"]+)" \}',
            text,
        )
    )
    return [names[stage] for stage in stage_types if stage in names]


def go_function_body(text: str, function_name: str) -> str:
    match = re.search(rf"func\s+(?:\([^)]*\)\s+)?{re.escape(function_name)}\([^)]*\)[^{{]*\{{", text)
    if not match:
        return ""
    start = match.end() - 1
    depth = 0
    for index in range(start, len(text)):
        if text[index] == "{":
            depth += 1
        elif text[index] == "}":
            depth -= 1
            if depth == 0:
                return text[start + 1 : index]
    return ""


def scheduler_contract_issues() -> list[Issue]:
    source = SCHEDULER_BOOTSTRAP.read_text(encoding="utf-8")
    registrations = scheduler_registrations(source)
    issues: list[Issue] = []
    if registrations != list(EXPECTED_SCHEDULER_REGISTRATIONS):
        issues.append(
            Issue(
                "scheduler-source-inventory-drift",
                f"composition root={registrations}, expected={list(EXPECTED_SCHEDULER_REGISTRATIONS)}",
            )
        )
    constructors = [constructor for constructor, _ in registrations]
    labels = [constructor.removesuffix("Runner") for constructor in constructors]

    apiserver_runtime = APISERVER_RUNTIME_DOC.read_text(encoding="utf-8")
    documented_constructors = set(re.findall(r"`([A-Za-z]+Runner)`", apiserver_runtime))
    if documented_constructors != set(constructors):
        issues.append(
            Issue(
                "scheduler-apiserver-runtime-doc-drift",
                f"documented={sorted(documented_constructors)}, source={sorted(constructors)}",
            )
        )
    for doc_path in (APISERVER_RUNTIME_DOC, RUNTIME_SCHEDULER_DOC, INFRA_RUNTIME_DOC, OPS_SCHEDULER_DOC):
        text = doc_path.read_text(encoding="utf-8")
        wrong_counts = [count for count in explicit_inventory_counts(text, "(?:Runner|runner)") if count != len(registrations)]
        if wrong_counts:
            issues.append(
                Issue(
                    "scheduler-doc-count-drift",
                    f"{doc_path.relative_to(ROOT)}: counts={wrong_counts}, source={len(registrations)}",
                )
            )
    background_text = RUNTIME_SCHEDULER_DOC.read_text(encoding="utf-8")
    for constructor in constructors:
        if constructor not in background_text:
            issues.append(Issue("scheduler-runtime-doc-drift", constructor))
    ops_text = OPS_SCHEDULER_DOC.read_text(encoding="utf-8")
    ops_rows = markdown_table(ops_text, ("Runner", "责任", "持久进度/互斥", "当前生产意图"))
    ops_labels = [row[0] for row in ops_rows]
    if ops_labels != labels:
        issues.append(Issue("scheduler-ops-doc-drift", f"documented={ops_labels}, source={labels}"))

    config_sections: dict[Path, set[str]] = {}
    for config_path in (DEVELOPMENT_CONFIG, PRODUCTION_CONFIG):
        text = config_path.read_text(encoding="utf-8")
        issues.extend(yaml_duplicate_issues(config_path, text))
        config_sections[config_path] = set(re.findall(r"(?m)^([a-z_][a-z0-9_]*):\s*(?:#.*)?$", text))
    production_text = PRODUCTION_CONFIG.read_text(encoding="utf-8")
    ops_by_label = {row[0]: row for row in ops_rows}
    for constructor, option in registrations:
        config_key = camel_to_snake(option)
        for config_path, sections in config_sections.items():
            if config_key not in sections:
                issues.append(Issue("scheduler-config-inventory-drift", f"{config_path.relative_to(ROOT)}: {config_key}"))
        enabled = yaml_block_values(production_text, config_key).get("enable")
        if enabled not in {"true", "false"}:
            issues.append(Issue("scheduler-production-enable-missing", f"{config_key}: {enabled!r}"))
            continue
        expected_enabled = EXPECTED_SCHEDULER_PRODUCTION_ENABLE[config_key]
        if enabled != expected_enabled:
            issues.append(
                Issue(
                    "scheduler-production-enable-drift",
                    f"{config_key}: source={enabled}, expected={expected_enabled}",
                )
            )
        label = constructor.removesuffix("Runner")
        expected_intent = "启用" if enabled == "true" else "关闭"
        if label not in ops_by_label or ops_by_label[label][3] != expected_intent:
            issues.append(
                Issue(
                    "scheduler-production-status-doc-drift",
                    f"{label}: expected {expected_intent} from {config_key}.enable={enabled}",
                )
            )
    return issues


def locklease_contract_issues() -> list[Issue]:
    source_inventory = locklease_inventory(LOCKLEASE_CATALOG_SOURCE.read_text(encoding="utf-8"))
    documented_inventory = locklease_doc_inventory(LOCKLEASE_DOC.read_text(encoding="utf-8"))
    issues: list[Issue] = []
    if source_inventory != list(EXPECTED_LOCKLEASE_INVENTORY):
        issues.append(
            Issue(
                "locklease-source-inventory-drift",
                f"source={source_inventory}, expected={list(EXPECTED_LOCKLEASE_INVENTORY)}",
            )
        )
    documented_projection = [entry[:4] for entry in source_inventory]
    if documented_inventory != documented_projection:
        issues.append(
            Issue(
                "locklease-doc-inventory-drift",
                f"documented={documented_inventory}, source={documented_projection}",
            )
        )
    text = LOCKLEASE_DOC.read_text(encoding="utf-8")
    wrong_counts = [count for count in explicit_inventory_counts(text, "workload") if count != len(source_inventory)]
    if wrong_counts:
        issues.append(Issue("locklease-doc-count-drift", f"counts={wrong_counts}, source={len(source_inventory)}"))
    if not re.search(r"renewal mode[^。\n]*`?auto`?", text, flags=re.I):
        issues.append(Issue("locklease-renewal-doc-drift", "catalog renewal mode auto is not documented"))
    return issues


def signal_contract_issues() -> list[Issue]:
    source_text = SIGNALS.read_text(encoding="utf-8")
    source = signal_topology(source_text)
    documented = signal_doc_topology(SIGNAL_DOC.read_text(encoding="utf-8"))
    issues = yaml_duplicate_issues(SIGNALS, source_text)
    if source != EXPECTED_SIGNAL_TOPOLOGY:
        issues.append(Issue("signal-topology-source-drift", f"source={source}, expected={EXPECTED_SIGNAL_TOPOLOGY}"))
    if documented != source:
        issues.append(Issue("signal-topology-doc-drift", f"documented={documented}, source={source}"))
    return issues


def acl_contract_issues() -> list[Issue]:
    issues: list[Issue] = []
    production_text = PRODUCTION_CONFIG.read_text(encoding="utf-8")
    issues.extend(yaml_duplicate_issues(PRODUCTION_CONFIG, production_text))
    production_values = yaml_scalar_values(production_text)
    actual = {
        "enabled": production_values.get(("grpc", "acl", "enabled")),
        "config-file": production_values.get(("grpc", "acl", "config-file")),
        "default-policy": production_values.get(("grpc", "acl", "default-policy")),
    }
    expected = {
        "enabled": "true",
        "config-file": "configs/grpc-acl.prod.yaml",
        "default-policy": "deny",
    }
    if actual != expected:
        issues.append(Issue("grpc-acl-production-config-drift", f"source={actual}, expected={expected}"))
    acl_text = GRPC_ACL_CONFIG.read_text(encoding="utf-8")
    issues.extend(yaml_duplicate_issues(GRPC_ACL_CONFIG, acl_text))
    acl_values = yaml_scalar_values(acl_text)
    if acl_values.get(("default_policy",)) != "deny":
        issues.append(Issue("grpc-acl-file-policy-drift", "configs/grpc-acl.prod.yaml must remain default-deny"))

    server_source = GRPC_SERVER_SOURCE.read_text(encoding="utf-8")
    fail_closed_tokens = [
        "if config.ACL.Enabled",
        "loadACLConfig(config.ACL.ConfigFile, config.ACL.DefaultPolicy)",
        'return nil, fmt.Errorf("initialize gRPC ACL from %q: %w"',
        'defaultPolicy != "deny"',
    ]
    if not ordered_tokens(server_source, fail_closed_tokens):
        issues.append(Issue("grpc-acl-load-fail-closed-drift", "server constructor no longer proves strict ACL loading"))

    default_deny_pattern = (
        r"(?:default[-_ ]?deny|default[_ -]?policy[^。\n]*deny|默认策略[^。\n]*deny|"
        r"策略[^。\n]*deny|deny[^。\n]{0,16}默认策略)"
    )
    fail_closed_pattern = r"(?:阻断启动|阻止[^。\n]*构造|构造失败|启动失败|不得启动|fail-closed)"
    production_docs = {
        GRPC_RUNTIME_DOC: (
            r"configs/grpc-acl\.prod\.yaml",
            default_deny_pattern,
            r"(?:开启|启用|enabled)",
            fail_closed_pattern,
        ),
        INFRA_LIFECYCLE_DOC: (default_deny_pattern, r"(?:开启|启用|enabled)", fail_closed_pattern),
        INFRA_SECURITY_CANONICAL_DOC: (
            r"configs/grpc-acl\.prod\.yaml",
            default_deny_pattern,
            r"(?:开启|启用|enabled)",
            fail_closed_pattern,
        ),
        INFRA_CONFIG_DOC: (
            r"configs/grpc-acl\.prod\.yaml",
            default_deny_pattern,
            r"(?:开启|启用|enabled)",
            fail_closed_pattern,
        ),
    }
    for path, required_patterns in production_docs.items():
        text = path.read_text(encoding="utf-8")
        for pattern in required_patterns:
            if not re.search(pattern, text, flags=re.I):
                issues.append(Issue("grpc-acl-doc-drift", f"{path.relative_to(ROOT)}: missing /{pattern}/"))

    sidecar_text = GRPC_SIDECAR_DOC.read_text(encoding="utf-8")
    for pattern in (
        r"configs/grpc-acl\.prod\.yaml",
        r"default[-_ ]?deny|deny-by-default|策略[^。\n]*deny",
        r"(?:构造失败|启动失败|fail-closed)",
    ):
        if not re.search(pattern, sidecar_text, flags=re.I):
            issues.append(Issue("grpc-acl-doc-drift", f"{GRPC_SIDECAR_DOC.relative_to(ROOT)}: missing /{pattern}/"))

    runtime_text = INFRA_LIFECYCLE_DOC.read_text(encoding="utf-8")
    if re.search(r"ACL[^。\n]*(?:读取|加载)失败[^。\n]*(?:退回|fallback)", runtime_text, flags=re.I):
        issues.append(Issue("grpc-acl-runtime-fallback-doc-drift", str(INFRA_LIFECYCLE_DOC.relative_to(ROOT))))
    if not (
        re.search(r"ACL[^。\n]*(?:失败|缺失|非法)[^。\n]*(?:阻断|退出|构造失败|fail-closed)", runtime_text, flags=re.I)
        or ("构造失败" in runtime_text and "不会回退" in runtime_text)
    ):
        issues.append(Issue("grpc-acl-runtime-fail-closed-doc-missing", str(INFRA_LIFECYCLE_DOC.relative_to(ROOT))))
    return issues


def worker_log_contract_issues() -> list[Issue]:
    config_text = WORKER_PRODUCTION_CONFIG.read_text(encoding="utf-8")
    issues = yaml_duplicate_issues(WORKER_PRODUCTION_CONFIG, config_text)
    values = yaml_scalar_values(config_text)
    actual = {
        "disable-caller": values.get(("log", "disable-caller")),
        "disable-stacktrace": values.get(("log", "disable-stacktrace")),
        "level": values.get(("log", "level")),
        "format": values.get(("log", "format")),
        "enable-color": values.get(("log", "enable-color")),
        "development": values.get(("log", "development")),
    }
    expected = {
        "disable-caller": "true",
        "disable-stacktrace": "true",
        "level": "warn",
        "format": "json",
        "enable-color": "false",
        "development": "false",
    }
    if actual != expected:
        issues.append(Issue("worker-production-log-source-drift", f"source={actual}, expected={expected}"))

    runtime_text = INFRA_LIFECYCLE_DOC.read_text(encoding="utf-8")
    required_profile = (
        r"warn",
        r"json",
        r"(?:development\s*[:=]\s*false|development[^。\n]*(?:关闭|禁用))",
        r"(?:(?:enable-)?color\s*[:=]\s*false|(?:color|颜色)[^。\n]*(?:关闭|禁用)|禁色)",
    )
    for pattern in required_profile:
        if not re.search(pattern, runtime_text, flags=re.I):
            issues.append(Issue("worker-production-log-doc-missing", f"{INFRA_LIFECYCLE_DOC.relative_to(ROOT)}: /{pattern}/"))

    projected_docs = {
        INFRA_OVERVIEW_DOC: (r"warn", r"json"),
        INFRA_LIFECYCLE_DOC: required_profile,
        INFRA_SECURITY_CANONICAL_DOC: required_profile,
        INFRA_OBSERVABILITY_DECISIONS_DOC: required_profile,
    }
    for path, patterns in projected_docs.items():
        text = path.read_text(encoding="utf-8")
        for pattern in patterns:
            if not re.search(pattern, text, flags=re.I):
                issues.append(Issue("worker-production-log-doc-missing", f"{path.relative_to(ROOT)}: /{pattern}/"))
        for line_number, line in enumerate(text.splitlines(), start=1):
            if re.search(
                r"(?:production|生产)[^。\n]{0,60}(?:配置(?:仍|又)?(?:是|为)|使用|级别(?:是|为))"
                r"[^。\n]{0,30}\b(?:debug|console)\b",
                line,
                flags=re.I,
            ):
                issues.append(
                    Issue(
                        "worker-production-log-doc-drift",
                        f"{path.relative_to(ROOT)}:{line_number}: stale debug/console assertion",
                    )
                )
    return issues


def process_lifecycle_contract_issues() -> list[Issue]:
    issues: list[Issue] = []
    runtime_text = INFRA_LIFECYCLE_DOC.read_text(encoding="utf-8")
    sections = {
        "apiserver": section_between(runtime_text, "## 5. apiserver", "## 6. collection-server"),
        "collection-server": section_between(runtime_text, "## 6. collection-server", "## 7. worker"),
        "worker": section_between(runtime_text, "## 7. worker", "## 8. "),
    }
    for process, (runner_path, _) in PROCESS_SOURCES.items():
        source_stages = process_stage_names(runner_path.read_text(encoding="utf-8"))
        if not source_stages:
            issues.append(Issue("process-stage-source-parse", process))
            continue
        expected_stages = EXPECTED_PROCESS_STAGES[process]
        if source_stages != expected_stages:
            issues.append(
                Issue(
                    "process-stage-source-drift",
                    f"{process}: source={source_stages}, expected={expected_stages}",
                )
            )
        if not ordered_tokens(sections[process], source_stages):
            issues.append(
                Issue(
                    "process-stage-doc-drift",
                    f"{process}: expected ordered stages={source_stages}",
                )
            )

    shutdown_sections = {
        "apiserver": section_between(runtime_text, "### 10.1 apiserver", "### 10.2 collection-server"),
        "collection-server": section_between(runtime_text, "### 10.2 collection-server", "### 10.3 worker"),
        "worker": section_between(runtime_text, "### 10.3 worker", "## 11. "),
    }
    lifecycle_source_tokens = {
        "apiserver": [
            "runPrepareRunShutdownHooks",
            "containerCleanup",
            "stopAuthzSync",
            "closeDatabase",
            "closeHTTP",
            "closeGRPC",
        ],
        "collection-server": [
            "closeHTTP",
            'AddShutdownHook("close grpc clients"',
            'AddShutdownHook("close database"',
            'AddShutdownHook("stop authz sync"',
            'AddShutdownHook("close iam"',
            'AddShutdownHook("cleanup container"',
        ],
        "worker": [
            "holdReplayer.Stop()",
            "subscriber.Stop()",
            "subscriber.Close()",
            "publisher.Close()",
            "deadLetterRecorder.Close()",
            "holdStore.Close()",
            'AddShutdownHook("stop subscriber"',
            'AddShutdownHook("close grpc manager"',
            'AddShutdownHook("close database"',
            'AddShutdownHook("shutdown metrics"',
            'AddShutdownHook("cleanup container"',
        ],
    }
    doc_tokens = {
        "apiserver": ["runtime shutdown hooks", "Container.Cleanup", "authz subscriber", "DatabaseManager", "HTTP", "gRPC"],
        "collection-server": ["HTTP shutdown", "gRPC manager", "DatabaseManager", "authz sync", "IAM", "cleanup container"],
        "worker": [
            "hold replayer",
            "subscriber",
            "publisher",
            "dead-letter",
            "hold store",
            "gRPC manager",
            "DatabaseManager",
            "5 秒",
            "metrics server",
            "cleanup container",
        ],
    }
    for process, (_, lifecycle_path) in PROCESS_SOURCES.items():
        lifecycle_text = lifecycle_path.read_text(encoding="utf-8")
        if process == "apiserver":
            source_contract = (
                go_function_body(lifecycle_text, "registerShutdownCallback")
                + go_function_body(lifecycle_text, "runProcessLifecycleDeps")
            )
        elif process == "collection-server":
            source_contract = go_function_body(lifecycle_text, "runCollectionLifecycle")
        else:
            source_contract = (
                go_function_body(lifecycle_text, "buildLifecycleDeps")
                + go_function_body(lifecycle_text, "runWorkerLifecycle")
            )
        if not ordered_tokens(source_contract, lifecycle_source_tokens[process]):
            issues.append(Issue("process-shutdown-source-drift", f"{process}: order selectors changed"))
        if not ordered_tokens(shutdown_sections[process], doc_tokens[process]):
            issues.append(Issue("process-shutdown-doc-drift", f"{process}: expected ordered shutdown={doc_tokens[process]}"))
    runtime_bootstrap = SCHEDULER_BOOTSTRAP.read_text(encoding="utf-8")
    if 'AddShutdownHook("stop schedulers"' not in runtime_bootstrap:
        issues.append(Issue("process-shutdown-source-drift", "apiserver scheduler shutdown hook missing"))
    return issues


def probe_contract_issues() -> list[Issue]:
    issues = grpc_healthz_port_source_issues()
    source_requirements = {
        APISERVER_ROUTER_SOURCE: (
            'func (r *Router) healthCheck',
            '"status":       "healthy"',
            'func (r *Router) readyCheck',
            'if !snapshot.Summary.Ready',
            'if err == nil && snapshot != nil',
            'Ready: true',
        ),
        GENERIC_SERVER_SOURCE: ('s.GET("/healthz"',),
        COLLECTION_ROUTER_SOURCE: (
            'engine.GET("/health"',
            'engine.GET("/readyz"',
            'engine.GET("/serve-readyz"',
        ),
        COLLECTION_HEALTH_SOURCE: (
            'if !snapshot.Summary.Ready',
            'if !controlSynchronized',
            'serveReady := controlSynchronized',
        ),
        WORKER_PROBE_SOURCE: (
            'mux.HandleFunc("/healthz"',
            '"status":    "healthy"',
            'mux.HandleFunc("/readyz"',
            'if !snapshot.Summary.Ready',
        ),
    }
    for path, tokens in source_requirements.items():
        text = path.read_text(encoding="utf-8")
        for token in tokens:
            if token not in text:
                issues.append(Issue("probe-source-contract-drift", f"{path.relative_to(ROOT)}: {token}"))

    image_probe_expectations = {
        "apiserver": "http://localhost:8080/healthz",
        "collection-server": "http://localhost:8080/health",
    }
    for service, token in image_probe_expectations.items():
        text = DOCKERFILES[service].read_text(encoding="utf-8")
        if "HEALTHCHECK" not in text or token not in text:
            issues.append(Issue("probe-image-contract-drift", f"{service}: expected {token}"))
    if "HEALTHCHECK" in DOCKERFILES["worker"].read_text(encoding="utf-8"):
        issues.append(Issue("probe-image-contract-drift", "worker image must not claim a Docker HEALTHCHECK"))

    metadata_text = IMAGE_METADATA_SCRIPT.read_text(encoding="utf-8")
    if metadata_text.count('HEALTH_PATH="${HEALTH_PATH:-/health}"') != 2:
        issues.append(Issue("probe-deploy-contract-drift", "apiserver/collection CD health path must remain /health"))
    deploy_text = DEPLOY_SCRIPT.read_text(encoding="utf-8")
    collection_probe_tokens = [
        "/serve-readyz",
        "HTTP/[0-9.]+[[:space:]]+404",
        "falling back to /readyz",
        "/readyz",
        "return 1",
    ]
    collection_probe_body = section_between(
        deploy_text,
        "collection_serving_ready() {",
        "\n}\n\ndeploy_collection()",
    )
    if not ordered_tokens(collection_probe_body, collection_probe_tokens):
        issues.append(
            Issue(
                "probe-deploy-contract-drift",
                "collection must probe /serve-readyz and fall back to /readyz only after 404",
            )
        )
    worker_readiness = WORKER_READINESS_SCRIPT.read_text(encoding="utf-8")
    if 'WORKER_READY_URL="${WORKER_READY_URL:-http://127.0.0.1:9092/readyz}"' not in worker_readiness:
        issues.append(Issue("probe-deploy-contract-drift", "worker readiness URL must remain /readyz"))
    if "grep -Fq '\"status\":\"ready\"'" not in worker_readiness:
        issues.append(Issue("probe-deploy-contract-drift", "worker readiness must verify the ready response body"))

    rows = markdown_table(
        INFRA_PROBE_DOC.read_text(encoding="utf-8"),
        ("进程/端点", "当前判断", "返回非 2xx 的条件", "没有覆盖"),
    )
    by_endpoint = {row[0]: row for row in rows}
    expected_tokens = {
        "apiserver /health": ((1, "固定"), (2, "无"), (3, "MySQL")),
        "apiserver /healthz": ((1, "静态"), (2, "无"), (3, "全部依赖")),
        "apiserver /readyz": ((1, "Redis"), (2, "Ready=false"), (3, "fallback ready")),
        "collection /health": ((1, "固定"), (2, "无"), (3, "apiserver")),
        "collection /readyz": ((1, "Redis"), (1, "初次同步"), (2, "control 未同步")),
        "collection /serve-readyz": ((1, "初次同步"), (2, "control 未同步"), (3, "Redis degraded")),
        "worker /healthz": ((1, "固定"), (2, "无"), (3, "MQ consumer")),
        "worker /readyz": ((1, "Redis"), (2, "Ready=false"), (3, "MQ")),
    }
    if set(by_endpoint) != set(expected_tokens):
        issues.append(Issue("probe-doc-inventory-drift", f"documented={sorted(by_endpoint)}, expected={sorted(expected_tokens)}"))
    for endpoint, checks in expected_tokens.items():
        row = by_endpoint.get(endpoint)
        if row is None:
            continue
        for column, token in checks:
            if token not in row[column]:
                issues.append(Issue("probe-doc-semantics-drift", f"{endpoint}: column={column} missing {token!r}"))
    issues.extend(
        grpc_healthz_port_doc_issues(
            INFRA_DEPLOYMENT_DOC.read_text(encoding="utf-8"),
        )
    )
    return issues


def grpc_healthz_port_source_issues() -> list[Issue]:
    """Freeze the current 9091 compatibility-field boundary.

    GRPCOptions still parses healthz-port and computes a legacy HealthzAddr,
    but the apiserver composition root copies neither value into the active
    internal/pkg/grpc Config. That server creates exactly one listener from
    BindAddress/BindPort, so 9091 is not a separately probeable endpoint.
    """

    issues: list[Issue] = []
    production_values = yaml_scalar_values(PRODUCTION_CONFIG.read_text(encoding="utf-8"))
    if production_values.get(("grpc", "healthz-port")) != "9091":
        issues.append(Issue("grpc-healthz-port-config-drift", "grpc.healthz-port must remain the documented 9091 field"))

    options_text = GRPC_OPTIONS_SOURCE.read_text(encoding="utf-8")
    for token in ("HealthzPort", 'mapstructure:"healthz-port"', "c.HealthzAddr"):
        if token not in options_text:
            issues.append(Issue("grpc-healthz-port-option-drift", f"missing compatibility selector {token!r}"))

    bootstrap_body = go_function_body(
        APISERVER_GRPC_BOOTSTRAP_SOURCE.read_text(encoding="utf-8"),
        "applyGRPCOptions",
    )
    if not bootstrap_body or "HealthzPort" in bootstrap_body or "HealthzAddr" in bootstrap_body:
        issues.append(
            Issue(
                "grpc-healthz-port-wiring-drift",
                "applyGRPCOptions must continue to omit the unused healthz-port compatibility field",
            )
        )

    runtime_config = GRPC_CONFIG_SOURCE.read_text(encoding="utf-8")
    server_text = GRPC_SERVER_SOURCE.read_text(encoding="utf-8")
    run_body = go_function_body(server_text, "Run")
    if "HealthzPort" in runtime_config or "HealthzAddr" in runtime_config:
        issues.append(Issue("grpc-healthz-port-listener-drift", "internal/pkg/grpc Config gained a healthz listener field"))
    if (
        not run_body
        or run_body.count("net.Listen(") != 1
        or "s.config.BindPort" not in run_body
        or "Healthz" in run_body
        or "9091" in run_body
    ):
        issues.append(
            Issue(
                "grpc-healthz-port-listener-drift",
                "internal/pkg/grpc.Run must retain its single BindPort listener and no 9091/Healthz listener",
            )
        )
    return issues


def grpc_healthz_port_doc_issues(text: str) -> list[Issue]:
    issues: list[Issue] = []
    no_listener = re.search(
        r"9091[^\n]*(?:(?:当前[^\n]*)?(?:未创建|未实现|没有创建|没有|无)[^\n]*(?:独立[^\n]*)?(?:listener|监听器)|"
        r"(?:独立[^\n]*)?(?:listener|监听器)[^\n]*(?:未创建|未实现|没有创建|没有|无))",
        text,
        flags=re.I,
    )
    not_probe_target = re.search(
        r"(?:9091[^\n]*(?:不得|不应|无需|不能)[^\n]*(?:探针|探测|可达|验收)|"
        r"(?:不得|不应|无需|不能)[^\n]*(?:探针|探测|可达|验收)[^\n]*9091)",
        text,
        flags=re.I,
    )
    if not no_listener:
        issues.append(
            Issue(
                "grpc-healthz-port-doc-listener-drift",
                f"{INFRA_DEPLOYMENT_DOC.relative_to(ROOT)}: 9091 must be described as having no independent listener",
            )
        )
    if not not_probe_target:
        issues.append(
            Issue(
                "grpc-healthz-port-doc-probe-drift",
                f"{INFRA_DEPLOYMENT_DOC.relative_to(ROOT)}: 9091 must not be a reachability/probe target",
            )
        )
    stale_patterns = (
        r"9091\s+gRPC health",
        r"9091[^\n]*是否对监控可达需",
        r"9091[^\n]*不可从非授权公网访问",
        r"9091[^\n]*可达边界主要依赖部署网络",
    )
    for pattern in stale_patterns:
        if re.search(pattern, text, flags=re.I):
            issues.append(
                Issue(
                    "grpc-healthz-port-doc-current-listener-claim",
                    f"{INFRA_DEPLOYMENT_DOC.relative_to(ROOT)}: stale /{pattern}/",
                )
            )
    return issues


def markdown_go_test_packages(text: str) -> list[str]:
    """Extract repository package operands from documented Go test commands."""

    packages: list[str] = []
    lines = text.splitlines()
    index = 0
    while index < len(lines):
        line = lines[index]
        start = line.find("go test")
        if start < 0:
            index += 1
            continue
        command_parts = [line[start:]]
        while command_parts[-1].rstrip().endswith("\\") and index + 1 < len(lines):
            index += 1
            command_parts.append(lines[index].strip())
        command = " ".join(part.rstrip().rstrip("\\").strip(" `") for part in command_parts)
        packages.extend(re.findall(r"(?<![A-Za-z0-9_])\./[A-Za-z0-9_./*-]+", command))
        index += 1
    return packages


def infrastructure_non_cached_test_path_issues() -> list[Issue]:
    issues: list[Issue] = []
    infrastructure_root = DOCS / "03-基础设施"
    packages_by_doc: dict[Path, list[str]] = {}
    for path in sorted(infrastructure_root.rglob("*.md")):
        packages = markdown_go_test_packages(path.read_text(encoding="utf-8"))
        packages_by_doc[path] = packages
        for package in packages:
            if not source_path_exists(package):
                issues.append(
                    Issue(
                        "infra-non-cached-test-package-missing",
                        f"{path.relative_to(ROOT)}: {package}",
                    )
                )

    expected_testeeaccess = "./internal/collection-server/application/testeeaccess"
    retired_testeeaccess = "./internal/pkg/testeeaccess"
    for path in (INFRA_SECURITY_DOC, INFRA_SECURITY_CANONICAL_DOC):
        packages = packages_by_doc.get(path, [])
        if retired_testeeaccess in packages or retired_testeeaccess in path.read_text(encoding="utf-8"):
            issues.append(Issue("infra-security-retired-test-package", f"{path.relative_to(ROOT)}: {retired_testeeaccess}"))
        if expected_testeeaccess not in packages:
            issues.append(
                Issue(
                    "infra-security-test-package-missing",
                    f"{path.relative_to(ROOT)}: expected {expected_testeeaccess}",
                )
            )
    return issues


def docker_copy_pairs(text: str) -> set[tuple[str, str]]:
    pairs: set[tuple[str, str]] = set()
    for line in text.splitlines():
        match = re.match(r"^COPY(?:\s+--[^ ]+)*\s+([^ ]+)\s+([^ ]+)\s*$", line)
        if match:
            pairs.add((match.group(1), match.group(2)))
    return pairs


def docker_config_contract_issues() -> list[Issue]:
    required_copies = {
        "apiserver": {
            ("configs/apiserver.dev.yaml", "/app/configs/apiserver.dev.yaml"),
            ("configs/apiserver.prod.yaml", "/app/configs/apiserver.prod.yaml"),
            ("configs/cache", "/app/configs/cache"),
            ("configs/grpc-acl.prod.yaml", "/app/configs/grpc-acl.prod.yaml"),
            ("configs/events.yaml", "/app/configs/events.yaml"),
        },
        "collection-server": {
            ("configs/collection-server.dev.yaml", "/app/configs/collection-server.dev.yaml"),
            ("configs/collection-server.prod.yaml", "/app/configs/collection-server.prod.yaml"),
            ("configs/cache", "/app/configs/cache"),
            ("configs/events.yaml", "/app/configs/events.yaml"),
        },
        "worker": {
            ("configs/worker.dev.yaml", "/app/configs/worker.dev.yaml"),
            ("configs/worker.prod.yaml", "/app/configs/worker.prod.yaml"),
            ("configs/events.yaml", "/app/configs/events.yaml"),
        },
    }
    issues: list[Issue] = []
    for service, dockerfile in DOCKERFILES.items():
        actual = docker_copy_pairs(dockerfile.read_text(encoding="utf-8"))
        missing = sorted(required_copies[service] - actual)
        if missing:
            issues.append(Issue("docker-required-config-copy-drift", f"{service}: missing={missing}"))
    worker_dockerfile = DOCKERFILES["worker"].read_text(encoding="utf-8")
    if 'CMD ["--config=/app/configs/worker.prod.yaml"]' not in worker_dockerfile:
        issues.append(Issue("docker-required-config-command-drift", "worker image default config must remain production"))

    compose = PRODUCTION_COMPOSE.read_text(encoding="utf-8")
    required_compose_tokens = (
        "/opt/qs-server/qs-apiserver/configs/env/config.prod.env",
        "/opt/qs-server/qs-apiserver/configs:/app/configs",
        '--config=/app/configs/apiserver.prod.yaml',
        "/opt/qs-server/qs-collection-server/configs/env/config.prod.env",
        "/opt/qs-server/qs-collection-server/configs:/app/configs",
        '--config=/app/configs/collection-server.prod.yaml',
        "/opt/qs-server/qs-worker/configs/env/config.prod.env",
        "/opt/qs-server/qs-worker/configs:/app/configs",
        '--config=/app/configs/worker.prod.yaml',
    )
    for token in required_compose_tokens:
        if token not in compose:
            issues.append(Issue("docker-required-config-mount-drift", token))

    package_text = PACKAGE_SCRIPT.read_text(encoding="utf-8")
    if 'cp -r configs "$PACKAGE_DIR/"' not in package_text:
        issues.append(Issue("docker-required-config-package-drift", "deployment package must copy the complete configs tree"))
    deploy_text = DEPLOY_SCRIPT.read_text(encoding="utf-8")
    if 'rsync -a "$DEPLOY_TMP/configs/" "/opt/qs-server/${CONTAINER_NAME}/configs/"' not in deploy_text:
        issues.append(Issue("docker-required-config-package-drift", "remote deploy must synchronize the complete configs tree"))

    config_doc = INFRA_CONFIG_DOC.read_text(encoding="utf-8")
    for token in (
        "configs/events.yaml",
        "configs/signals.yaml",
        "configs/cache/apiserver.prod.yaml",
        "configs/cache/collection-server.prod.yaml",
        "configs/grpc-acl.prod.yaml",
    ):
        if token not in config_doc:
            issues.append(Issue("docker-required-config-doc-drift", f"{INFRA_CONFIG_DOC.relative_to(ROOT)}: {token}"))
    deployment_doc = INFRA_DEPLOYMENT_DOC.read_text(encoding="utf-8")
    for token in (
        "/opt/qs-server/<service>/configs",
        "/app/configs",
        "镜像内的默认文件不替代本次部署包",
    ):
        if token not in deployment_doc:
            issues.append(Issue("docker-required-config-doc-drift", f"{INFRA_DEPLOYMENT_DOC.relative_to(ROOT)}: {token}"))
    return issues


def oneoff_safety_contract_issues(text: str | None = None) -> list[Issue]:
    if text is None:
        text = ONEOFF_README.read_text(encoding="utf-8")
    required_patterns = (
        r"cleanup_orphaned_assessment_documents[^\n]*blocked\s*/\s*audit-only",
        r"cleanup_orphaned_assessment_documents[^。\n]*必须保持\s*blocked",
        r"不得使用\s*`--apply`",
        r"`--hard-delete`",
        r"`--skip-backup`",
        r"只能输出\s*dry-run\s*审计结果",
    )
    issues = [
        Issue("oneoff-orphan-cleanup-safety-doc-drift", f"{ONEOFF_README.relative_to(ROOT)}: missing /{pattern}/")
        for pattern in required_patterns
        if not re.search(pattern, text, flags=re.I)
    ]
    source_text = ONEOFF_CLEANUP_SOURCE.read_text(encoding="utf-8")
    write_flags = set(re.findall(r'flag\.BoolVar\([^,]+,\s*"([a-z-]+)"', source_text))
    expected_write_flags = {"apply", "hard-delete", "skip-backup"}
    if not expected_write_flags.issubset(write_flags):
        issues.append(
            Issue(
                "oneoff-orphan-cleanup-binary-capability-drift",
                f"source flags={sorted(write_flags)}, expected residual flags={sorted(expected_write_flags)}",
            )
        )
        return issues

    for pattern in (
        r"代码中存在这些开关[^。\n]*不代表当前运维授权",
        r"`--apply`",
        r"`--hard-delete`",
        r"`--skip-backup`",
    ):
        if not re.search(pattern, text, flags=re.I):
            issues.append(
                Issue(
                    "oneoff-orphan-cleanup-residual-capability-undisclosed",
                    f"{ONEOFF_README.relative_to(ROOT)}: missing /{pattern}/",
                )
            )

    canonical = INFRA_MIGRATION_RECOVERY_DOC.read_text(encoding="utf-8")
    for pattern in (
        r"不是二进制\s*fail-closed",
        r"源码仍解析并执行[^。\n]*`--apply`[^。\n]*`--hard-delete`[^。\n]*`--skip-backup`",
        r"不能[^。\n]*解读为[^。\n]*技术上禁用",
    ):
        if not re.search(pattern, canonical, flags=re.I):
            issues.append(
                Issue(
                    "oneoff-orphan-cleanup-residual-capability-undisclosed",
                    f"{INFRA_MIGRATION_RECOVERY_DOC.relative_to(ROOT)}: missing /{pattern}/",
                )
            )
    return issues


def priority_infrastructure_doc_contract_issues() -> list[Issue]:
    """Ratchet the highest-risk cache, event and concurrency semantics.

    These checks intentionally stay narrow. They do not certify every sentence;
    they prevent the concrete responsibility-chain drifts found during the
    source audit from returning behind an otherwise green structural gate.
    """

    issues: list[Issue] = []

    required_tokens = {
        CACHE_MODEL_DOC: (
            "application 显式调用 typed cache port",
            "Repository wrapper 的职责不是缓存读取",
            "不是 validation 已强制",
            "questionnaire 可在 adapter 与 effective Policy 同时允许时写 negative",
            "published-model 当前不写 negative sentinel",
            "published-model 的完整 L1+L2 读取不具有这个原子边界",
            "L2 使用旧快照而 L1 TTL 使用新快照",
            "published-model L1 仍使用启动时 jitter",
        ),
        CACHE_REGISTRY_DOC: (
            "published-model L1 是当前明确例外",
            "`TTLJitterRatio` 在 L1 构造时固定",
            "不是一个跨 L1/L2 的单快照操作",
            "每次单独 `Resolve/All/Snapshot` 调用只看到完整旧版本或完整新版本",
            "不为跨多次 Resolve 的组合操作锁定同一快照",
            "当前也没有 reload 后验证 L1 jitter 的契约测试",
        ),
        CACHE_KERNEL_DOC: (
            "questionnaire 与 testee adapter 显式声明 `CacheNegative: true`",
            "published-model 的 by-questionnaire、catalog-list 与 algorithms",
            "`CacheNegative: false`",
            "Policy 打开也不会使它们写 negative sentinel",
            "jitter ratio 仍固定在构造 Options",
            "published-model 外层 `cacheEnabled()`、L2 read-through 与 L1 Set 的 `TTLProvider` 会多次 Resolve",
            "reload 后仍使用启动时 jitter",
        ),
        CACHE_README: (
            "普通 object/query read-through 每次只从 `PolicyProvider` 解析一次 Policy",
            "published-model L1+L2 当前会在外层 enabled、L2 read-through 和 L1 Set 多次 Resolve",
            "L1 jitter 保留启动时副本",
            "status 的 effective jitter 不证明 L1 已热生效该值",
        ),
        CACHE_CONSISTENCY_DOC: (
            "仓库 production policy 的版本化意图",
            "effective runtime",
            "exact deployed SHA",
        ),
        CACHE_OBSERVABILITY_DOC: (
            "qs_apiserver_l1_cache_requests_total",
            "qs_apiserver_l1_cache_entries",
            "qs_apiserver_l1_cache_max_entries",
            "qs_apiserver_l1_cache_evictions_total",
            "四条独立 PromQL",
            "没有被 `MetricEvidenceReader`",
        ),
        CACHE_ACCEPTANCE_DOC: (
            "canonical capability catalog v3",
            "apiserver published-model L1 当前仅导出原始 Prometheus 指标",
        ),
        EVENT_OUTBOX_DOC: (
            "publish success + mark failed",
            "MarkEvent(s)Published",
            "Outbox 行仍保持 `publishing`",
            "stale-claim recovery",
            "不会立即改写为 retry 状态",
        ),
        EVENT_CONTRACT_DOC: (
            "当前共享代码缺口：report-status best-effort 写入不参与 settlement",
            "正常 worker composition",
            "`reportstatus.NewReporter` 始终返回非 nil reporter 和 nil error",
            "`interpretation.report.generated` 调用 `SetCompleted`",
            "`interpretation.report.failed` 的 manual/terminal 分支",
            "Redis status 写入失败只记录 `report_status_set_failed_total`",
            "signal notify 失败也只记录 `signaling_notify_failed_total`",
            "错误都不返回给 handler",
            "合法 payload 都可能最终 ACK",
        ),
        EVENT_OBSERVABILITY_DOC: (
            "qs-operating-system 的页面路由、重定向与渲染行为是跨仓事实",
            "不在本文档的 exact-SHA closure 内",
            "qs-operating-system 的独立 exact SHA",
        ),
        EVENT_README: (
            "`evaluation.failed`、`interpretation.report.generated` 与 `interpretation.report.failed`",
            "report-status Redis 写入/Signal 唤醒是 best-effort",
            "handler 可以最终 ACK",
            "ACK 不证明 report-status 投影或唤醒已成功",
        ),
        SIGNAL_DOC: (
            "测试内硬编码的 expected topology",
            "不会扫描 composition root",
            "本轮已人工反查",
        ),
        LOCKLEASE_DOC: (
            "DefaultTTL、caller override 与 snapshot",
            "plan scheduler 声明 `lock_ttl: 2m`",
            "不是 active run 的 effective TTL",
        ),
        CONCURRENCY_BACKPRESSURE_DOC: (
            "composition root 已注入 limiter”不等于该 repository 的所有方法都受控",
            "MySQL `BaseRepository.WithContext()` / `DB()`",
            "Mongo `BaseRepository.Collection()` / `DB()`",
            "Actor read model",
            "AnswerSheet read model",
            "limiter 指标只表示“显式 acquire 的受控操作”",
            "不能证明所有 repository 方法都经过 acquire",
        ),
        CONCURRENCY_GOVERNANCE_DOC: (
            "state/command、目标实例实际 effect 与 terminal audit 不在同一事务中",
            "error、partial 或 timeout，不足以证明“没有任何效果”",
            "`resilience.tune_rate_limit` 和 `resilience.release_lock`",
            "`retry.manual_actions_enabled=true`",
            "当前没有专用的 `resilience control operation` Prometheus counter",
        ),
        CONCURRENCY_ACCEPTANCE_DOC: (
            "snapshot 不能证明 active run 使用哪一个 TTL",
            "当前没有专用的统一 control-operation Prometheus counter",
            "Backpressure 指标只覆盖显式调用 limiter acquire 的方法",
            "MySQL Actor read model",
            "Mongo AnswerSheet read model/repository",
            "不是全部 MySQL/Mongo 并发",
        ),
        CONCURRENCY_README: (
            "limiter 注入不等于方法级全覆盖",
            "MySQL Actor read model",
            "Mongo AnswerSheet read model/repository",
            "现有 Backpressure 指标只是已接入方法的子集",
        ),
    }
    for path, tokens in required_tokens.items():
        text = path.read_text(encoding="utf-8")
        for token in tokens:
            if token not in text:
                issues.append(
                    Issue(
                        "priority-infrastructure-doc-drift",
                        f"{path.relative_to(ROOT)}: missing {token!r}",
                    )
                )

    cache_text = "\n".join(
        path.read_text(encoding="utf-8")
        for path in sorted((DOCS / "03-基础设施/cache").glob("*.md"))
    )
    if "Registry v2" in cache_text:
        issues.append(Issue("priority-cache-retired-registry-version", "cache docs contain Registry v2"))

    event_handoff_orders = {
        EVENT_STATE_DOC: (
            "业务 channel 达到 MaxAttempts",
            "发布 cb.failed.<hash> handoff topic",
            "finish 原消息",
            "独立 handoff consumer",
            "MySQL recorder",
            "event_delivery_dead_letter(manual_required)",
        ),
        EVENT_MQ_DOC: (
            "业务 channel 达到 MaxAttempts",
            "发布 cb.failed.<hash> handoff topic",
            "finish 原消息",
            "独立 handoff consumer",
            "MySQL FailedMessageHandler / recorder",
            "event_delivery_dead_letter(manual_required)",
        ),
    }
    for path, event_handoff_order in event_handoff_orders.items():
        text = path.read_text(encoding="utf-8")
        if not ordered_tokens(text, event_handoff_order):
            issues.append(
                Issue(
                    "priority-event-terminal-handoff-drift",
                    f"{path.relative_to(ROOT)}: expected ordered terminal handoff",
                )
            )
        for token in ("handoff publish", "requeue", "recorder 成功后"):
            if token not in text:
                issues.append(
                    Issue(
                        "priority-event-terminal-handoff-drift",
                        f"{path.relative_to(ROOT)}: missing {token!r}",
                    )
                )

    go_mod = (ROOT / "go.mod").read_text(encoding="utf-8")
    if not re.search(r"^\s*github\.com/FangcunMount/component-base\s+v0\.6\.9\s*$", go_mod, flags=re.M):
        issues.append(
            Issue(
                "priority-event-provider-dependency-drift",
                "component-base version changed; re-audit terminal handoff before updating the docs",
            )
        )

    production_values = yaml_scalar_values(PRODUCTION_CONFIG.read_text(encoding="utf-8"))
    expected_backpressure = {
        ("backpressure", "mysql", "max_inflight"): "150",
        ("backpressure", "mysql", "timeout_ms"): "5000",
        ("backpressure", "mongo", "max_inflight"): "48",
        ("backpressure", "mongo", "timeout_ms"): "1500",
        ("backpressure", "iam", "max_inflight"): "100",
        ("backpressure", "iam", "timeout_ms"): "4000",
    }
    for key, expected in expected_backpressure.items():
        if production_values.get(key) != expected:
            issues.append(
                Issue(
                    "priority-backpressure-config-drift",
                    f"{'.'.join(key)}={production_values.get(key)!r}, expected={expected!r}",
                )
            )
    backpressure_doc = CONCURRENCY_BACKPRESSURE_DOC.read_text(encoding="utf-8")
    for row in (
        "| MySQL | 150 | 5000ms |",
        "| Mongo | 48 | 1500ms |",
        "| IAM | 100 | 4000ms |",
        "不代表目标环境 effective config",
    ):
        if row not in backpressure_doc:
            issues.append(
                Issue(
                    "priority-backpressure-doc-drift",
                    f"{CONCURRENCY_BACKPRESSURE_DOC.relative_to(ROOT)}: missing {row!r}",
                )
            )

    concurrency_text = "\n".join(
        path.read_text(encoding="utf-8")
        for path in sorted((DOCS / "03-基础设施/concurrency").glob("*.md"))
    )
    if "resilience_control_operation_total" in concurrency_text:
        issues.append(
            Issue(
                "priority-concurrency-phantom-metric",
                "concurrency docs reference nonexistent resilience_control_operation_total",
            )
        )
    governance_text = CONCURRENCY_GOVERNANCE_DOC.read_text(encoding="utf-8")
    for ordered in (
        ("MySQL claim", "Redis `CompareAndSwap`", "本地 budget 执行 `Apply`", "terminal audit"),
        ("MySQL claim", "发布带 expiry 的 Redis command", "取消 active leader body", "terminal audit"),
    ):
        if not ordered_tokens(governance_text, ordered):
            issues.append(
                Issue(
                    "priority-concurrency-governance-order-drift",
                    f"{CONCURRENCY_GOVERNANCE_DOC.relative_to(ROOT)}: expected ordered stages={ordered}",
                )
            )
    return issues


def infrastructure_contract_issues() -> list[Issue]:
    issues: list[Issue] = []
    for validator in (
        scheduler_contract_issues,
        locklease_contract_issues,
        signal_contract_issues,
        acl_contract_issues,
        worker_log_contract_issues,
        process_lifecycle_contract_issues,
        probe_contract_issues,
        docker_config_contract_issues,
        infrastructure_non_cached_test_path_issues,
        oneoff_safety_contract_issues,
        priority_infrastructure_doc_contract_issues,
    ):
        issues.extend(validator())
    return issues


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


def briefing_case_sections(text: str) -> list[str]:
    return re.findall(
        r"(?ms)^## \d+\. 案例卡[一二三四五]：.*?(?=^## \d+\.|\Z)",
        text,
    )


def main() -> int:
    issues: list[Issue] = []

    try:
        head = current_git_head()
        source_baseline = current_source_baseline()
    except subprocess.CalledProcessError as error:
        detail = error.stderr.strip() if error.stderr else str(error)
        print(f"docs facts failed: git metadata unavailable: {detail}")
        return 1

    migration_contracts, migration_issues = migration_inventory()
    issues.extend(migration_issues)
    event_text = EVENTS.read_text(encoding="utf-8")
    issues.extend(yaml_duplicate_issues(EVENTS, event_text))
    configured_events = yaml_section_keys(event_text, "events", r"[a-z0-9_.]+")
    signal_text = SIGNALS.read_text(encoding="utf-8")
    configured_signals = yaml_section_keys(signal_text, "signals", r"[a-z0-9_]+")
    if configured_events != EXPECTED_EVENTS:
        issues.append(
            Issue(
                "event-contract-inventory-drift",
                f"missing={sorted(EXPECTED_EVENTS - configured_events)}, extra={sorted(configured_events - EXPECTED_EVENTS)}",
            )
        )
    if configured_signals != EXPECTED_SIGNALS:
        issues.append(
            Issue(
                "signal-contract-inventory-drift",
                f"missing={sorted(EXPECTED_SIGNALS - configured_signals)}, extra={sorted(configured_signals - EXPECTED_SIGNALS)}",
            )
        )
    try:
        grpc_services, proto_file_count = grpc_inventory()
    except ValueError as error:
        issues.append(Issue("grpc-contract-inventory", str(error)))
        grpc_services, proto_file_count = {}, 0
    actual_contracts = machine_contract_inventory(
        migration_contracts,
        configured_events,
        configured_signals,
        grpc_services,
        proto_file_count,
    )

    files = list(active_markdown())
    maintained_files = list(maintained_markdown())
    closure_exceptions: list[dict[str, object]] = []
    if CLOSURE_MANIFEST.exists():
        try:
            manifest = json.loads(CLOSURE_MANIFEST.read_text(encoding="utf-8"))
        except (json.JSONDecodeError, OSError) as error:
            issues.append(Issue("closure-manifest-parse", str(error)))
        else:
            if not isinstance(manifest, dict):
                issues.append(Issue("closure-manifest-schema", "top-level value must be an object"))
            else:
                closure_issues, closure_exceptions = validate_closure_manifest(
                    manifest,
                    files,
                    maintained_files,
                    head,
                    source_baseline,
                    actual_contracts,
                )
                issues.extend(closure_issues)
    else:
        issues.append(Issue("missing-closure-manifest", str(CLOSURE_MANIFEST.relative_to(ROOT))))

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
        module_paths = [path.relative_to(ROOT).as_posix() for path in module_files]
        if len(module_files) > HARD_MAX_BUSINESS_MODULE_MARKDOWN:
            issues.append(
                Issue(
                    "business-module-doc-tree-hard-ceiling",
                    f"{package}: {len(module_files)} files; hard ceiling is {HARD_MAX_BUSINESS_MODULE_MARKDOWN}",
                )
            )
        elif len(module_files) > TARGET_BUSINESS_MODULE_MARKDOWN and not budget_exception_covers(
            closure_exceptions,
            module_paths,
        ):
            issues.append(
                Issue(
                    "business-module-doc-tree-review-target",
                    f"{package}: {len(module_files)} files exceeds target {TARGET_BUSINESS_MODULE_MARKDOWN} without budget exception",
                )
            )

    active_paths = [path.relative_to(ROOT).as_posix() for path in files]
    if len(files) > HARD_MAX_ACTIVE_MARKDOWN:
        issues.append(
            Issue(
                "active-doc-tree-hard-ceiling",
                f"{len(files)} files; hard ceiling is {HARD_MAX_ACTIVE_MARKDOWN}",
            )
        )
    elif len(files) > TARGET_ACTIVE_MARKDOWN and not budget_exception_covers(
        closure_exceptions,
        active_paths,
        total_scope=True,
    ):
        issues.append(
            Issue(
                "active-doc-tree-review-target",
                f"{len(files)} files exceeds target {TARGET_ACTIVE_MARKDOWN} without budget exception",
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

    if MIGRATION_README.exists():
        migration_readme_text = MIGRATION_README.read_text(encoding="utf-8")
        documented_max = re.search(
            r"当前目录末端版本为 MySQL `(\d+)`、MongoDB `(\d+)`",
            migration_readme_text,
        )
        expected_mysql = migration_contracts["mysql"]["max_version"]
        expected_mongodb = migration_contracts["mongodb"]["max_version"]
        if not documented_max:
            issues.append(Issue("migration-readme-max-missing", str(MIGRATION_README.relative_to(ROOT))))
        elif (int(documented_max.group(1)), int(documented_max.group(2))) != (expected_mysql, expected_mongodb):
            issues.append(
                Issue(
                    "migration-readme-max-drift",
                    f"documented={documented_max.groups()}, source=({expected_mysql}, {expected_mongodb})",
                )
            )

    # The briefing layer is intentionally derived, but its decision cards must
    # remain traceable and honest. Freeze the three reading routes, five-card
    # inventory, field order and explicit personal-evidence placeholders.
    if BRIEFING_DOC.exists():
        briefing_text = BRIEFING_DOC.read_text(encoding="utf-8")
        for duration in ("3 分钟", "10 分钟", "30 分钟"):
            if f"| {duration} |" not in briefing_text:
                issues.append(Issue("briefing-reading-route-drift", duration))
        cases = briefing_case_sections(briefing_text)
        if len(cases) != 5:
            issues.append(Issue("briefing-case-count-drift", f"expected 5, got {len(cases)}"))
        required_fields = (
            "问题",
            "约束",
            "备选方案",
            "决策",
            "代码链",
            "失败窗口",
            "验证",
            "仍存边界",
        )
        for index, case in enumerate(cases, start=1):
            offsets = [case.find(f"### {field}") for field in required_fields]
            if any(offset < 0 for offset in offsets) or offsets != sorted(offsets):
                issues.append(
                    Issue(
                        "briefing-case-schema-drift",
                        f"case {index}: required order={' -> '.join(required_fields)}",
                    )
                )
            if case.count("### 本人补充区（必须本人填写）") != 1:
                issues.append(Issue("briefing-personal-evidence-drift", f"case {index}: supplement block"))
            for field in ("个人角色", "实际贡献", "协作冲突/分歧", "个人量化结果"):
                if f"| {field} | `[待本人补充" not in case:
                    issues.append(
                        Issue(
                            "briefing-personal-evidence-drift",
                            f"case {index}: {field}",
                        )
                    )

    # Source-derived infrastructure inventories and high-risk runtime/config
    # contracts share one ratchet so a prose-only fix cannot bypass them.
    issues.extend(infrastructure_contract_issues())

    # IR-R001 is closed in the active checklist. Its dedicated companion must
    # remain a closure record rather than silently reverting to an open plan.
    if IR_CHECKLIST.exists():
        ir_text = IR_CHECKLIST.read_text(encoding="utf-8")
        if not re.search(
            r"(?m)^\|\s*IR-R001\s*\|\s*P0\s*\|[^\n]*\|\s*已发布\s*\|\s*已关闭\s*\|",
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
        ledger_metadata = parse_ledger_metadata(ledger_text)
        expected_ledger_fields = {"checkout_sha", "last_ci_sha", "deployed_sha"}
        if set(ledger_metadata) != expected_ledger_fields:
            issues.append(
                Issue(
                    "version-ledger-metadata",
                    f"fields={sorted(ledger_metadata)}, expected={sorted(expected_ledger_fields)}",
                )
            )
        else:
            if ledger_metadata["checkout_sha"] != "git:HEAD":
                issues.append(Issue("version-ledger-checkout-ref", "checkout_sha must be symbolic git:HEAD"))
            for field in ("last_ci_sha", "deployed_sha"):
                value = ledger_metadata[field]
                if value != "unknown" and not SHA_RE.fullmatch(value):
                    issues.append(Issue("version-ledger-evidence-sha", f"{field}={value!r}"))
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
        f"{len(configured_events)} events, "
        f"{len(configured_signals)} signals, "
        f"{sum(len(rpcs) for rpcs in grpc_services.values())} gRPC RPCs, "
        f"migrations mysql={migration_contracts['mysql']['max_version']}/mongodb={migration_contracts['mongodb']['max_version']}, "
        f"checkout={head[:12]}, source_baseline={source_baseline[:12]}"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
