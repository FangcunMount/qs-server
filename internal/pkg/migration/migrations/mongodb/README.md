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
|--------|------|----------|
| `answersheets` | 答卷集合 | domain_id, questionnaire, filler |
| `questionnaires` | 问卷集合 | domain_id, code+version, status |
| `scales` | 量表集合 | domain_id, code, questionnaire |
| `interpret_reports` | 解读报告集合 | domain_id, testee_id, scale_code |

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

