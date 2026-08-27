# MongoDB Migrations

本目录存放 MongoDB 迁移文件，使用 `golang-migrate` 进行版本管理。

本页只说明 Mongo migration 文件与索引契约。通用破坏性风险、dirty 状态处置、备份与恢复边界
以[数据库迁移](../../README.md)为准；仓库中的迁移定义不能证明任一环境已经执行，本页也不记录
会随部署变化的生产运行结果。

## 📁 目录结构

```text
mongodb/
├── 000001_init_collections.up.json    # 初始化集合和索引
├── 000001_init_collections.down.json  # 回滚初始化
└── README.md
```

## 📋 集合说明

| 集合名 | 描述 | 主要索引 |
| -------- | ------ | ---------- |
| `answersheets` | 答卷集合 | domain_id, questionnaire, filler |
| `questionnaires` | 问卷集合（head/snapshot） | role-based partial unique（见 000013） |
| `assessment_models` | 测评模型（head/snapshot） | role-based partial unique（见 000013） |
| `assessment_norms` | 常模表 | `idx_assessment_norms_table_version` unique |
| `report_generations` | 报告生成意图（v2） | outcome_id, report_type, template_version |
| `interpretation_runs` | 报告生成尝试（v2） | generation_id, attempt |
| `interpret_report_artifacts` | 成功报告成品（v2） | generation_id, assessment_id, testee_id |
| `report_query_catalog` | Assessment 级当前报告查询索引（v2） | assessment_id unique, org/testee sort indexes（见 000015） |
| `interpretation_report_templates` | Interpretation 报告模板发布资产（v2） | template_id+template_version unique（见 000017） |
| `ai_explanation_generations` | 手动 AI 解读生成意图 | semantic unique、规范 Input 快照、终态 TTL（见 000025/000031） |
| `ai_explanation_runs` | 单次 Provider 执行尝试 | generation+attempt unique、expired lease、终态 TTL（见 000025/000031） |
| `ai_explanation_artifacts` | 通过确定性校验的不可变 AI 解读内容 | generation unique、结构化 Content、TTL、Org/Testee 数据主体导出 keyset（见 000025/000031/000032） |
| `ai_explanation_profiles` | AI 解读发布策略 | profile_id+version unique、published selector、status/created keyset（见 000025/000033） |
| `ai_explanation_prompt_evaluations` | 合成 Prompt 评测、语义评分与人工复核的不可变证据 | domain_id unique、active release unique、profile/status、suite/prompt release、过期执行租约与 Org/created keyset 查询（见 000026/000027/000033） |
| `ai_explanation_prompt_evaluation_daily_budgets` | 机构级 Prompt 评测 UTC 日调用预留账本 | org+budget day unique、reservation run unique（见 000027） |
| `ai_explanation_participant_daily_budgets` | Participant 每次 Provider attempt 的 UTC 日调用预留账本 | org+budget day unique、reservation generation+attempt unique（见 000028/000030） |
| `ai_explanation_participant_active_capacity` | Participant Provider 执行的分布式活跃槽账本 | org unique、active generation/run unique（见 000029） |
| `interpretation_catalog_repair_plans` | Catalog 修复 dry-run 快照 | dry_run_id unique、expires_at TTL（见 000019） |

## AI explanation runtime（000025）

`000025_add_ai_explanation_runtime` 只创建四个生命周期集合和与 Repository 对齐的索引，不发布
任何 Profile，也不启用功能开关。Prompt/Profile/Provider Route 组合必须先完成真实模型评测与人工复核，
再通过独立的发布流程写入 `ai_explanation_profiles`；数据库结构存在不代表 AI 解读已可用。

## AI explanation Prompt evaluation evidence（000026）

`000026_add_ai_explanation_prompt_evaluations` 创建合成评测证据集合及与 Repository 对齐的三个索引，
并为 Profile 增加只约束 `status=published` 的 selector-slot partial unique index，避免并发发布形成同级歧义。
Mongo driver 在迁移到 26 前要求 `ai_explanation_profiles` 中不存在任何 pre-existing published Profile；
若发现手工或预发布数据会中止迁移，禁止猜测 slot key 或自动背书既有发布状态。
该迁移不调用 Provider、不写 approved evaluation、不发布 Profile，也不启用功能。Profile 发布审计通过
`published_evidence_run_id` 引用已经由应用 gate 判定为 approved 的完整评测运行；没有真实 35 次生成、
零调用 preflight、semantic rubric 和双角色人工复核时，不存在可用发布证据。

## AI explanation Prompt evaluation durable execution（000027）

`000027_add_ai_explanation_prompt_evaluation_execution` 为 operator-only Prompt 评测执行增加四类物理保护：

- `active_release_key` 的 partial unique index，保证同一冻结 release 在 `collecting|awaiting_review` 阶段最多只有一个 active run；终态保存会 unset 该键；
- `active_execution_org_key` 的 partial unique index，保证同一机构最多只有一个 `collecting` run；进入 `awaiting_review`、取消或终态时 unset 该键；
- `(status, execution.phase, execution.lease_expires_at, domain_id)` partial index，只覆盖 `collecting + prepared`，为自动唤醒过期 prepared checkpoint 提供有界查询；`dispatching` 不进入扫描索引；
- `ai_explanation_prompt_evaluation_daily_budgets` 按 `(org_id, budget_day)` 唯一，并对 `reservations.run_id` 建唯一 sparse index，防止同一 Run 跨预算桶重复预留。

Mongo driver 在迁移到 27 前要求不存在 pre-existing `collecting` 或 `awaiting_review` 评测。该功能在 27
之前没有受 active-release 唯一键保护，driver 因此拒绝猜测或自动改写这些未完成运行。迁移只增加预算集合与执行索引，
不会启动评测、调用 Provider、恢复任务或发布 Profile。应用在一个 Mongo 事务中完成日预算预留、Run 创建和首条 Outbox；默认每次预留 70、每机构 UTC 日上限 140，取消或失败不返还预留。应用已提供默认关闭的周期 scanner；只有显式启用对应配置后才会扫描，并且 scanner 只持久化唤醒事件，不直接调用 Provider。000031 之后账本按显式 lifecycle policy 从 UTC 日结束计算 `expires_at`；仓库不提供法务期限默认值。

## AI explanation participant capacity（000028）

`000028_add_ai_explanation_participant_capacity` 创建按 `(org_id, budget_day)` 唯一的 Participant 预算账本，
初始版本对 `reservations.generation_id` 建立唯一 sparse index。应用为首次语义 Generation 预留 1 次 Provider
调用，并在同一个 Mongo 事务内提交预算、Generation 与 requested Outbox；事务任一写入失败即整体回滚。
相同语义请求优先读取既有 Generation，并在并发争用后再次回读，因此精确复用不重复扣减，也不会因预算
已满而遮蔽已存在结果。机构、用户和 Assessment 三个 UTC 日上限在同一文档条件更新中同时裁决。

仓库默认值 `500/5/3` 只是功能关闭状态下的保守配置起点，不是生产容量或成本结论；上线前必须结合模型
价格、机构规模和灰度数据确认。该账本不退款，000031 之后按显式 policy 设置 TTL，也不等同于 token/金额计费账本；后续若允许新的
Provider retry attempt，必须先扩展预留模型，不能直接绕过这次首调用预算。`000030` 已将唯一预留身份升级为
`GenerationID + attempt`，所以治理批准的每次人工 retry 都会再预留 1 次调用；首次请求仍对应 attempt 1。

## AI explanation participant active capacity（000029）

`000029_add_ai_explanation_participant_active_capacity` 创建每机构一份、仅保留当前活跃项的 Provider
执行槽账本。应用在 `pending -> generating` 的 Mongo 事务中同时取得机构/用户/Assessment 三层槽、
创建 Run 并 CAS 保存 Generation；生成成功或失败时，又在对应终态事务内精确释放同一
`GenerationID + RunID` 槽。进程崩溃或 lease 恢复期间不提前释放，因此活跃槽代表仍需结算的外部执行，
而不是 HTTP 到达率或待处理请求数。

仓库默认上限为机构/用户/Assessment `10/2/1`，仅是功能关闭状态下的保守起点。Mongo driver 在迁移
到 29 前要求不存在 `generating + participant` Generation；否则既有 Run 没有可证明的槽记录，迁移会
fail closed。`pending` Generation 不受此限制，它会在实际开始 Run 时按新契约取得槽。账本不设 TTL，
不得用过期时间自动删除仍可能处于 Provider 调用或恢复窗口中的活跃项。

## AI explanation participant manual retry governance（000030）

`000030_add_ai_explanation_participant_retry_governance` 将 Participant UTC 日调用预算的唯一身份从
`generation_id` 改为 `reservation_id = ai-explanation:<generation_id>:attempt:<n>`，从而允许同一个失败
Generation 在人工授权后为下一次独立 Provider 调用再次预留额度。同时为 Run 上的
`retry_authorization.request_id` 与 `retry_authorization.event_id` 建立全局唯一稀疏索引，约束治理动作和
durable wake-up 都只能发生一次。

迁移到 30 前要求 Participant 日预算账本不存在既有 reservation。该功能分支尚未承载 Participant 流量，
driver 因此拒绝猜测旧 reservation 的 attempt 来源；若未来要迁移已有流量，必须先设计带审计证据的专用
backfill。down migration 会尝试恢复“一代一次”的旧唯一索引；只要已存在多 attempt reservation，它就会
因唯一键冲突而失败，不能把 down migration 当作数据回滚方案。

## AI explanation data lifecycle（000031）

`000031_add_ai_explanation_retention_ttl` 为 Generation、Run、Artifact、Prompt evaluation 和两类 UTC 日预算账本创建 `expires_at` 的 `expireAfterSeconds=0` 索引。migration 不写任何业务过期时间，也不猜测合规期限。

应用只有在 `ai_explanation.data_lifecycle` 同时配置 policy version 和三类正保留期后才允许启用功能。终态记录写入 `expires_at + retention_policy_version`，预算账本从 UTC 日结束计算；启用 policy 时 Repository 启动扫描会拒绝历史终态记录缺少 lifecycle 元数据，要求先做离线 backfill。

down migration 只删除本版本拥有的 TTL indexes，不恢复已经由 TTL 删除的数据，也不撤销 `expires_at` 字段。任何生产回滚必须先停止相关写入并按备份恢复策略处理，不能把 down migration 当作数据恢复。

## AI explanation subject export index（000032）

`000032_add_ai_explanation_subject_export_index` 在 `ai_explanation_artifacts` 上增加 `(source.association.org_id, source.association.testee_id, audience, generated_at desc, domain_id desc)` 索引，支持数据主体导出按 Org/Testee 与 Participant audience 精确过滤，并使用固定 snapshot 下的 `generated_at + artifact_id` keyset 稳定分页。migration 只建索引，不导出数据、不扩大授权，也不改写 Artifact。

应用导出路径只投影标准报告来源、Profile/Prompt/Route/校验器版本收据和最终正文，排除 Input、Prompt 渲染、Run、Outbox、Provider Invocation/request ID、token/延迟和预算账本。该索引存在只证明仓库查询契约已有物理支撑，不代表真实授权联调、大数据量或生产合规验收已完成。down migration 只删除该索引，不改写任何 Artifact。

## AI explanation governance catalog indexes（000033）

`000033_add_ai_explanation_governance_catalog_indexes` 为评测审核目录增加 `(requested_org_id, status?, created_at desc, domain_id desc)`，为 Profile 治理目录增加 `(status?, created_at desc, domain_id desc)`。这些索引只服务可信管理面的组织隔离和稳定 keyset 分页，不改变评测、审核或 Profile 状态，也不会自动发布任何配置。down migration 只删除本版本拥有的四个索引。

## Report catalog bounded audit（000021）

`000021_add_report_catalog_audit_checkpoint` 历史上创建 checkpoint 集合，并为 active artifact winner
扫描与已知机构 archive keyset 扫描增加专用索引。扫描索引仍由应用只读校验；任一索引缺失时审计
runner 保持 degraded 且不推进当前位于 MySQL `interpretation_catalog_audit_checkpoint` 的 checkpoint。

down migration 只删除本版本拥有的两个扫描索引和 checkpoint 集合。回滚应优先关闭
`report_catalog_audit.enable` 并保留 checkpoint，不应依赖 down migration。

## Interpretation runtime ledgers retirement（000023）

`000023_retire_interpretation_runtime_ledgers` 在停写、全量导入和逐批校验完成后，退役三个已经由
MySQL 接管的 MongoDB 运行账本：

- `interpretation_admission_failures` → `interpretation_admission_failure`；
- `interpretation_attention_projections` → `interpretation_attention_projection`；
- `interpretation_catalog_audit_checkpoints` → `interpretation_catalog_audit_checkpoint`。

Mongo driver 在执行版本 23 前强制确认三个源集合均为空；若某个版本 22 环境已在受控切换中删除源集合，
driver 只会临时创建空命名空间以使 `drop` 幂等。任一集合仍有文档都会中止启动迁移。down migration
只恢复空集合及历史索引，不恢复已迁移数据；生产回滚必须使用切换前备份和 MySQL 对应快照。

## Empty compatibility collections retirement（000022）

`000022_retire_empty_compatibility_collections` 退役两个要求在迁移前证明为空的 compatibility collection：
`answersheet_submit_idempotency` 与 `archived_reports`。Mongo driver 在执行本版本前强制确认：

- 两个集合文档数均为 0；
- `report_query_catalog` 中 `source_kind=archive` 的引用数为 0。

为保证全新数据库也能执行 MongoDB `drop` 命令，driver 会先幂等地确保这两个空命名空间存在，
再执行文档数和 Catalog 引用预检。

任一条件不满足都会中止启动迁移，禁止通过 drop collection 丢弃兼容数据。down migration
只恢复空集合和原索引，不恢复历史文档；生产回滚仍应以迁移前备份为准。

## Legacy collections retirement（000020）

`000020_retire_legacy_collections` 删除已退出运行时的 `published_assessment_models`、
`interpret_reports`、`evaluation_rule_sets` 和 `scales`。这些集合不属于上表中的现行集合，
运行时 IndexManager 和维护脚本不得重新创建它们。

down migration 只重建 MongoDB 19 的空集合与索引结构，不恢复文档。生产回退不得依赖 down
migration 恢复数据，必须连同 migration state 使用完整 MongoDB 备份恢复。

## Interpretation recovery indexes（000019）

`000019_add_interpretation_recovery_indexes` 历史上增加按 ReportID 查询 Attention 投影的索引、AdmissionFailure 运维稳定分页索引，并创建保存 7 天受控 Catalog repair dry-run 的 TTL 集合。前两类索引随版本 23 的运行账本退役而退出当前结构；repair plan 集合继续使用。down migration 只删除本版本增加的索引和 repair plan 集合。

## Statistics Collector indexes（000018）

`000018_add_statistics_collector_indexes` 为 canonical Statistics Collector 的 Mongo 来源建立机构级稳定扫描索引：

- `answersheets(org_id, filled_at, domain_id)`，仅覆盖 `deleted_at:null`；
- `interpret_report_artifacts(org_id, generated_at, domain_id)`；
- `interpretation_runs(org_id, status, finished_at, domain_id)`。

这些索引与 Collector 的 `[from,to)` 窗口、机构过滤和 `(event_time,domain_id)` 排序共同构成扫描契约；down migration 只删除本 migration 拥有的三个索引。

## Report templates（000017）

`000017_add_report_templates` 创建 `interpretation_report_templates` 集合与 release unique 索引。Repository 启动时会幂等 seed 并 publish `legacy-v1` 兼容模板（standard/mbti/sbti/bigfive）。

ModelCatalog 发布门禁已绑定 Interpretation `PublishedTemplateLookup`；模板版本未发布或 lookup 缺失时拒绝发布。

## Report query catalog（000015）

`000015_add_report_query_catalog` 是 `report_query_catalog` 集合与索引的标准部署入口：

1. 创建 `report_query_catalog` 集合
2. 建立与 `reportCatalogIndexModels()` 对齐的 7 个索引（含 `uk_report_catalog_assessment` unique）

Runtime `ReportCatalogProjector` 的 `CreateMany` 仅作防御性 reconcile，不替代 migration 契约。

启动时 `bootstrap` 会在 Mongo migration 后执行 `VerifyReportCatalogIndexes`（缺失 required index → 拒绝启动）。

## Unified schema（000013）

`000013_unified_modelcatalog_schema` 是 ModelCatalog unified schema 的标准部署入口：

1. `000013_*.up.json` 只创建 role-based partial unique indexes 与 `assessment_norms.table_version` unique index，不执行 `dropIndexes`；
2. migration 后，`IndexManager.ReconcileUnifiedModelCatalogIndexes` 幂等删除
   `assessment_models.idx_assessment_models_code` 与 `questionnaires.idx_code_version`，并对齐当前 canonical indexes；
3. `VerifyUnifiedModelCatalogIndexes` 要求 required indexes 全部存在、forbidden legacy indexes 全部不存在，否则拒绝启动。

JSON up 刻意不使用非幂等 `dropIndexes`，以避免已切换环境因 `IndexNotFound` 进入
dirty@13。若迁移已是 dirty，不得直接执行 `migrate force` 或手工改写 migration state；
按[数据库迁移](../../README.md)核对真实索引、备份与补偿方案后再经审批处置。

验证入口：

```bash
go test -count=1 ./internal/pkg/migration ./internal/pkg/mongodb
```

## 🔧 迁移文件格式

MongoDB 迁移文件使用 JSON 格式，包含 `db.runCommand` 操作数组：

```json
[
  {
    "createIndexes": "collection_name",
    "indexes": [
      {
        "key": { "field": 1 },
        "name": "idx_field",
        "unique": true
      }
    ]
  }
]
```

## 📖 常用命令

### 创建索引

```json
{
  "createIndexes": "answersheets",
  "indexes": [
    {
      "key": { "domain_id": 1 },
      "name": "idx_domain_id",
      "unique": true
    }
  ]
}
```

### 删除索引

```json
{
  "dropIndexes": "answersheets",
  "index": "idx_domain_id"
}
```

### 删除所有索引

```json
{
  "dropIndexes": "answersheets",
  "index": "*"
}
```

## ⚠️ 注意事项

1. MongoDB 会自动创建集合，无需显式 `create` 命令
2. 迁移主要用于管理索引和 Schema 验证规则
3. `_id` 索引由 MongoDB 自动创建和管理
4. 回滚脚本使用 `"index": "*"` 删除所有非 `_id` 索引
