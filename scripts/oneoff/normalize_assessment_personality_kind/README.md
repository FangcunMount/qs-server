# Assessment personality → typology 身份规范化

将 MySQL `assessment` / `evaluation_outcome` 上退役的 `model_kind=personality` 改写为 canonical `typology`，并补齐 `sub_kind=typology`。

**不改** `model_algorithm`（`mbti`/`sbti`/`bigfive` 仍靠 dual-identity 回放）。

## 准入

| kind | algorithm | 结果 |
| --- | --- | --- |
| `personality` | `mbti`/`sbti`/`bigfive`/`personality_typology`/空 | 改写 |
| 其它 | 任意 | 跳过 |

## 用法

```bash
# dry-run
go run ./scripts/oneoff/normalize_assessment_personality_kind/ \
  --mysql-dsn "$MYSQL_DSN"

# 确认后再写入（可先 --limit=100）
go run ./scripts/oneoff/normalize_assessment_personality_kind/ \
  --mysql-dsn "$MYSQL_DSN" --apply

# 写后再跑门禁，personality|* 桶应消失（mbti 等 alias 仍在）
go run ./scripts/oneoff/observe_identity_retirement_gate/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs --mysql-dsn "$MYSQL_DSN"
```

Mongo 目录侧 `personality` kind 见 `migrate_personality_kind_values/`。
