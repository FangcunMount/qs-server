# 一次性历史回填运行手册

适用批次：`hist-20250101-20260727-v1`，业务日期范围为上海时区
`2025-01-01..2026-07-27`（含首尾，共 573 天）。历史时间只写业务事实；鉴权、IAM 审计、
锁、Lease、重试和 Outbox 调度继续使用实际系统时间。

## 1. 构建与只读预检

分别在 qs-server 与 seeddata-runner 的待发布 revision 上执行完整测试并保存 revision：

```bash
go test ./...
git rev-parse HEAD
```

确认 migration `000059_add_seed_backfill_stage` 与
`000060_add_task_business_created_at` 已成功应用；冻结本批次使用的 Plan、Questionnaire、
Model 精确版本。回填期间不得发布或修改这些版本。

在任何历史写入前采集 Statistics 基线：

```bash
export QS_MYSQL_DSN='<user>:<password>@tcp(<host>:3306)/<db>?parseTime=true&loc=Asia%2FShanghai'
go run ./scripts/oneoff/verify_historical_statistics \
  --mode capture-baseline \
  --org-id <org-id> \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --output /secure/path/hist-20250101-20260727-v1.baseline.json
```

## 2. 开启受保护历史能力

apiserver 与 collection-server 必须使用相同的环境密钥，并将历史上下文限制到目标机构和日期：

```bash
export QS_HISTORICAL_CONTEXT_SECRET='<random-one-time-secret>'
```

apiserver 配置：

```yaml
historical_seed:
  enabled: true
  pause_plan_scheduler: true
  allowed_org_ids: [<org-id>]
  earliest_date: "2025-01-01"
  latest_date: "2026-07-27"
  timezone: Asia/Shanghai
  freshness: 5m
  secret_env: QS_HISTORICAL_CONTEXT_SECRET
```

collection-server 使用相同范围和密钥环境变量。若还有独立 Plan scheduler，必须同时停止；
普通 seeddata daemon 也必须停止。Evaluation/Interpretation worker 与 Outbox relay 保持运行，
否则场景无法到达 Report generated 终态。

先以 staging 一月窗口和生产 3 天 pilot 验证，核对 MySQL、MongoDB、Outcome、Report 与
`seed_backfill_stage` 后，再执行全量。

## 3. 执行与恢复

在 seeddata-runner 仓库执行：

```bash
export IAM_MOCK_CONSUMER_SHARED_SECRET='<secret>'
export QS_HISTORICAL_CONTEXT_SECRET='<same-one-time-secret>'
go run ./cmd/seeddata historical-backfill \
  --config configs/seeddata.yaml \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --batch-id hist-20250101-20260727-v1
```

任一天存在终态失败时命令会停止，不能进入下一天。修复原因后必须使用同一批次、配置和冻结版本：

```bash
go run ./cmd/seeddata historical-backfill \
  --config configs/seeddata.yaml \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --batch-id hist-20250101-20260727-v1 \
  --resume
```

不得更换幂等键绕过 payload conflict。每日检查 checkpoint，并定期执行
`historical-verify --batch-id hist-20250101-20260727-v1`；需要 Assessment 的场景只有
Report generated 才算完成。

## 4. Statistics 修复、发布与验收

runner 全量完成并通过 `historical-verify` 后，在 qs-server 仓库执行 19 个窗口的统一编排：

```bash
export QS_STATISTICS_TOKEN='<operator-token>'
go run ./scripts/oneoff/rebuild_statistics \
  --base-url https://<qs-host> \
  --org-ids <org-id> \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --window-days 31 \
  --reason hist-20250101-20260727-v1 \
  --mode historical-backfill \
  --confirm
```

该命令对每个窗口按 `repair -> validate` 执行；任一步失败即停止，全部窗口成功后才以
`2026-07-27` 执行最终 publish。随后进行精确事实与基线增量对账：

```bash
go run ./scripts/oneoff/verify_historical_statistics \
  --mode verify \
  --org-id <org-id> \
  --batch-id hist-20250101-20260727-v1 \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --baseline /secure/path/hist-20250101-20260727-v1.baseline.json \
  --output /secure/path/hist-20250101-20260727-v1.verification.json
```

最后运行 perf preflight/smoke；必须能自动发现 Report 样本。保存 manifest、stage ledger、
基线、验证输出和备份至少 90 天。

## 5. 关闭与失败处理

验收完成后关闭 apiserver/collection-server 的 `historical_seed.enabled`，恢复 Plan scheduler，
轮换并移除 `QS_HISTORICAL_CONTEXT_SECRET`。

若需回滚，先停止 runner、worker、Plan/Statistics scheduler 和 Outbox relay并完成 MySQL/Mongo
同时间点备份，再使用 `cleanup_perf_testee_data` 的批次模式 dry-run。只有核对 manifest 与
`seed_backfill_stage` 精确资源清单后才允许 `--apply`；不得按整个 Org 或模糊日期删除。
清理后重新执行 Statistics repair/validate/publish，确认一致后再恢复服务。
