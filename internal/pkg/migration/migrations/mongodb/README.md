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
| `interpretation_catalog_repair_plans` | Catalog 修复 dry-run 快照 | dry_run_id unique、expires_at TTL（见 000019） |

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
