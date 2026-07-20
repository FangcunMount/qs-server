# 目录人格 kind 迁移

该工具默认只审计 Mongo 中已经退役的 `personality` 目录值。它只会迁移满足以下条件的记录：

- `sub_kind` / `model_sub_kind` 为 `typology`；
- 算法为 `personality_typology`、`bigfive`、`mbti` 或 `sbti`；
- 产品通道为空、`personality` 或 `typology`。

它在 `assessment_models` 中分别处理 `head` 与 `published_snapshot` 两类记录；不满足条件的行会列为 `SKIP`，不会被自动修改。

```bash
go run ./scripts/oneoff/migrate_personality_kind_values --mongo-uri "$MONGO_URI" --mongo-db qs
go run ./scripts/oneoff/migrate_personality_kind_values --mongo-uri "$MONGO_URI" --mongo-db qs --apply
```

应用完成后，重启 collection-server 或发布对应的 `TypologyModelCacheChangedSignal`，再验证 `POST /api/v1/typology-assessment-sessions`。

MySQL Assessment/Outcome 上的 `evaluation_model_kind=personality` 见 [`normalize_assessment_personality_kind`](../normalize_assessment_personality_kind/)。
