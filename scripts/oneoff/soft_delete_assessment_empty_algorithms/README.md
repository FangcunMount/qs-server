# Soft-delete Assessment empty-algorithm rows

破坏性作业：将 `evaluation_model_algorithm IS NULL OR ''` 的 Assessment **软删**（`deleted_at`），并 **物理删除** 同条件的 `evaluation_outcome`（该表无 soft-delete）。

用于清掉 MC-R018 `full_gate` 的 `assessment_empty_algorithm` 库存。默认 dry-run。`--apply` 必须带确认串。

## 用法

```bash
# 1) 备份 MySQL 后 dry-run
go run ./scripts/oneoff/soft_delete_assessment_empty_algorithms/ \
  --mysql-dsn "$MYSQL_DSN"

# 2) 可先限流
go run ./scripts/oneoff/soft_delete_assessment_empty_algorithms/ \
  --mysql-dsn "$MYSQL_DSN" \
  --apply --confirm=DELETE_EMPTY_ALGORITHM_ASSESSMENTS --limit=50

# 3) 全量
go run ./scripts/oneoff/soft_delete_assessment_empty_algorithms/ \
  --mysql-dsn "$MYSQL_DSN" \
  --apply --confirm=DELETE_EMPTY_ALGORITHM_ASSESSMENTS

# 4) 门禁：assessment_empty_algorithm 应清零；full_gate 可能仍 WARN（缺 metrics 背书）
go run ./scripts/oneoff/observe_identity_retirement_gate/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs --mysql-dsn "$MYSQL_DSN"
```

## 注意

- 不处理 retained-alias 行（见 `soft_delete_assessment_retained_aliases`）。
- 软删后相关答卷/报告/统计可能成为孤儿，需按现有 orphan cleanup 流程另清。
- 空 Algorithm runtime invent 已删；本作业只清库存。
