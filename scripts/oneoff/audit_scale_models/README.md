# Scale 数据完整性检查

`audit_scale_models` 是只读检查器，审计当前 canonical Scale 数据：

- `assessment_models` 中未删除的 `kind=scale` 草稿；
- `published_assessment_models` 中未删除、已发布的 `model_kind=scale` 快照；
- 草稿/快照绑定的精确问卷发布版本；
- `DefinitionV2` 的结构、题目与选项引用；
- 发布 payload 是否等于该 `DefinitionV2` 的 Scale 投影；
- 已发布快照与仍处于 published 状态的草稿之间的问卷绑定和版本一致性。

历史 `scales` 集合是退役兼容数据，**不**作为当前正确性的判定来源。

```bash
# 所有当前 Scale
go run ./scripts/oneoff/audit_scale_models/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs

# 只检查指定模型，适用于发布前排障
go run ./scripts/oneoff/audit_scale_models/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs \
  --codes 'PHQ9,GAD7'

# 机器可读输出
go run ./scripts/oneoff/audit_scale_models/ \
  --mongo-uri "$MONGO_URI" --json
```

无异常返回 `0`；发现数据一致性异常返回 `2`；连接或查询失败返回 `1`。脚本不写入 MongoDB。
