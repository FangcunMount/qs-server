# Soft-delete Assessment retained-alias rows

破坏性作业：将 `evaluation_model_algorithm ∈ {mbti,sbti,bigfive,behavioral_rating_default}` 的 Assessment **软删**（`deleted_at`），并 **物理删除** 同算法的 `evaluation_outcome`（该表无 soft-delete）。

默认 dry-run。`--apply` 必须带确认串。

## 用法

```bash
# 1) 备份 MySQL 后 dry-run
go run ./scripts/oneoff/soft_delete_assessment_retained_aliases/ \
  --mysql-dsn "$MYSQL_DSN"

# 2) 可先限流
go run ./scripts/oneoff/soft_delete_assessment_retained_aliases/ \
  --mysql-dsn "$MYSQL_DSN" \
  --apply --confirm=DELETE_RETAINED_ALIAS_ASSESSMENTS --limit=50

# 3) 全量
go run ./scripts/oneoff/soft_delete_assessment_retained_aliases/ \
  --mysql-dsn "$MYSQL_DSN" \
  --apply --confirm=DELETE_RETAINED_ALIAS_ASSESSMENTS

# 4) 门禁：dual_identity_gate 应清零 alias；full_gate 仍可能因 empty algorithm FAIL
go run ./scripts/oneoff/observe_identity_retirement_gate/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs --mysql-dsn "$MYSQL_DSN"
```

## 注意

- 不处理 `scale|空` 等 empty-algorithm 行（约 4k）。
- 软删后相关答卷/报告/统计可能成为孤儿，需按现有 orphan cleanup 流程另清。
- dual_identity_gate=PASS（可加 `--metrics-ok`）后，才可按 dual-identity checklist 删代码。
