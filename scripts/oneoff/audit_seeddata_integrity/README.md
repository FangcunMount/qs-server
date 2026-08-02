# Seeddata Report 孤岛一次性审计与清理

这是为本次历史回填故障准备的一次性工具。它对明确的 `org_id + batch_id + 日期范围` 检查
stage 所属的 Report 链，并删除“stage 明确归属本批次、且 MySQL Assessment 和 Outcome 均已消失”的
Mongo 孤岛。默认只读；完成本次治理后应删除工具，不把它维护成长期产品能力。

它不是全库数据质量平台，`valid=true` 只表示下面列出的最低检查通过，不证明 IAM、Testee 或所有
业务字段完整。

## 审计范围

- 指定日期范围内 `seed_backfill_stage` 的资源类型、资源 ID 和场景时间顺序；
- Entry resolve/intake 日志是否存在；
- Plan Enrollment、Task 及直接父记录是否存在；
- Mongo AnswerSheet 文档是否存在；
- Assessment、EvaluationOutcome、Report Artifact、Generation、Run、Catalog 的 Report 链；
- Assessment/Outcome 已从 MySQL 消失、但 Mongo Report 链仍存在的精确删除候选。

工具不会把“AnswerSheet 没有 Assessment”直接判定为可删除数据，因为非 Assessment 问卷允许只保存
AnswerSheet。资源 ID、Org/Testee 关系或时间不一致也只报告，不自动删除。只有同时满足以下条件的
Report 才进入自动删除计划：

1. `seed_backfill_stage` 明确证明 Report 属于指定批次和日期范围；
2. Mongo Artifact 的完整身份与 stage 一致；
3. 对应 MySQL Assessment 和 EvaluationOutcome 均已不存在；只缺一个时仅报告。

## 1. 停止写入并准备连接

只读审计期间停止对应 historical runner。执行 apply 前必须暂停 historical runner、QS Worker 和
Statistics scheduler，并先完成独立的 MySQL/Mongo 全量备份。

```bash
export QS_MYSQL_DSN='user:password@tcp(mysql:3306)/qs?parseTime=true&loc=Asia%2FShanghai'
export QS_MONGO_URI='mongodb://user:password@mongo:27017/?replicaSet=rs0&authSource=admin'
export QS_MONGO_DB='qs'
```

脚本要求 MySQL migration 至少为 63、Mongo migration 至少为 19，且 migration 不能 dirty。审计报告
会保存 MySQL/Mongo deployment identity；apply 必须连接同一个 deployment。

连接账户需要读取 QS MySQL/Mongo。执行 apply 时还需要：

- Mongo 原集合读写及创建备份 collection 的权限；
- MySQL `statistics_assessment_fact`、`domain_event_outbox` 的读写权限；
- MySQL 创建 `seed_orphan_stats_bak_<suffix>` 和 `seed_orphan_outbox_bak_<suffix>` 的权限。

## 2. 只读审计

审计结果使用原子写并保存为 `0600`：

```bash
go run ./scripts/oneoff/audit_seeddata_integrity \
  --org-id 1 \
  --batch-id hist-20250101-20260727-v2 \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --output /secure/path/hist-20250101-20260727-v2.integrity-audit.json
```

发现问题时命令会非零退出，但 JSON 报告仍会写入。先检查摘要和存储身份：

```bash
jq '{
  valid,
  problem_count,
  warning_count,
  report_stages,
  answersheet_stages,
  delete_candidates: (.deletion_candidates | length),
  storage,
  checks: [.checks[] | select(.problems > 0)]
}' /secure/path/hist-20250101-20260727-v2.integrity-audit.json
```

人工查看每一个删除候选：

```bash
jq '.deletion_candidates' \
  /secure/path/hist-20250101-20260727-v2.integrity-audit.json
```

## 3. 单次备份并精确删除

只有人工审核过上述报告后才能执行：

```bash
go run ./scripts/oneoff/audit_seeddata_integrity \
  --org-id 1 \
  --batch-id hist-20250101-20260727-v2 \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --audit-report /secure/path/hist-20250101-20260727-v2.integrity-audit.json \
  --backup-suffix hist_v2_20260802 \
  --confirm DELETE_SEEDDATA_ORPHANS \
  --confirm-services-stopped \
  --apply \
  --output /secure/path/hist-20250101-20260727-v2.integrity-apply.json
```

apply 会在删除前重新检查：

- audit report 的日期 scope 和 SHA-256 plan hash；
- MySQL/Mongo deployment identity 和 migration；
- stage 是否仍由同一个 batch/scenario 拥有；
- MySQL 父记录是否仍然缺失；
- Mongo `_id`、Report/Assessment/Outcome/Generation/Run ID 是否仍与报告一致。

备份名称：

```text
<原集合>__seed_orphan_backup_<backup-suffix>
seed_orphan_stats_bak_<backup-suffix>
seed_orphan_outbox_bak_<backup-suffix>
```

`backup-suffix` 必须是本次 apply 从未使用过的新值。脚本发现同名备份表或 collection 会拒绝执行，且
不提供自动恢复或重复 apply：如果中途失败，保持所有服务停止，先根据备份人工恢复，再重新审计。
不要直接换一个 suffix 继续删除。

## 4. 删除后的门禁

重新运行只读审计，并使用新的输出文件：

```bash
go run ./scripts/oneoff/audit_seeddata_integrity \
  --org-id 1 \
  --batch-id hist-20250101-20260727-v2 \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --output /secure/path/hist-20250101-20260727-v2.integrity-after-cleanup.json
```

工具不会自动删除 `seed_backfill_stage` 或 attempt。删除孤岛后：

- 如果批次仍要继续：单独审核并重置受影响 scenario 的 stage，再由 seeddata `--resume` 重建；
- 如果批次已经废弃：保留 stage 作为审计证据，待整批退役时由历史 rollback 工具最后清理；
- 不要在这个工具里混入 Testee、IAM 用户或医护人员清理。

保留 stage 的批次重新审计后仍可能出现 `report_stage_artifact_missing`。这是账本如实指出完成事实后来被
清理，不代表 Mongo 删除失败。应核对 Mongo、Statistics、MySQL Outbox 的备份和删除数量，并在整批
退役流程最后处理 stage/attempt。

随后只重跑受影响的最小 Statistics repair/validate 窗口，确认通过后再继续全日期编排。

## 5. 限制与退役

- 该工具不证明 IAM 场景总数、AnswerSheet 内容、Plan 业务字段或 Statistics 最终结果；这些仍由
  seeddata verify、Statistics validate 和人工抽样负责。
- 自动清理只覆盖能够证明为指定批次、指定日期范围，且 MySQL 父链缺失的 Report 孤岛。
- MySQL Assessment/Outcome/Plan 自身错配只报告，不自动删除。
- 备份至少保留 90 天，并与 audit/apply JSON 一起保存。
- 本轮治理验收后删除脚本，不继续扩展兼容性、恢复编排或通用数据治理能力。
