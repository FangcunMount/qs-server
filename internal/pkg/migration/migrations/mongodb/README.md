# MongoDB Migrations

本目录存放 MongoDB 迁移文件，使用 `golang-migrate` 进行版本管理。

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
| `scales` | 量表集合 | domain_id, code, questionnaire |
| `interpret_reports` | 解读报告集合 | domain_id, testee_id, scale_code |
| `report_generations` | 报告生成意图（v2） | outcome_id, report_type, template_version |
| `interpretation_runs` | 报告生成尝试（v2） | generation_id, attempt |
| `interpret_report_artifacts` | 成功报告成品（v2） | generation_id, assessment_id, testee_id |

## Unified schema（000013）

`000013_unified_modelcatalog_schema` 是 ModelCatalog unified schema 的标准部署入口：

1. 删除冲突旧索引：`assessment_models.idx_assessment_models_code`、`questionnaires.idx_code_version`
2. 建立与 cutover 脚本同构的 role-based partial unique indexes
3. 创建 `assessment_norms` 与 `table_version` unique index

**已做过 one-off cutover 的环境**：若旧冲突索引已不存在，`dropIndexes` 可能报 IndexNotFound。请先确认 `RequiredUnifiedIndexNames` 已齐全，再用 `migrate force 13` 标记版本，或手动补齐缺失索引后重跑。

启动时 `bootstrap` 会在 Mongo migration 后执行 `VerifyUnifiedModelCatalogIndexes`（缺失 required / 仍存在 forbidden legacy → 拒绝启动）。

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
