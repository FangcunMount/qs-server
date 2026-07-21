# Statistics V2 历史回填

该工具不直接操作 Fact 或统计结果表，而是按机构、按上海自然日小窗口调用统一的
`POST /internal/v2/statistics/runs`。因此，人工回填与夜间同步共享 Collector、Projection、
幂等规则、冲突检测和 `statistics_sync_run` 运行账本。

先执行只读校验：

```bash
QS_STATISTICS_V2_TOKEN='***' go run ./scripts/oneoff/backfill_statistics_v2 \
  --base-url https://qs.example.com \
  --org-ids 1,2 \
  --from 2025-01-01 \
  --to 2025-12-31 \
  --window-days 7 \
  --reason history_preflight \
  --validate-only
```

确认校验结果后才允许写入：

```bash
QS_STATISTICS_V2_TOKEN='***' go run ./scripts/oneoff/backfill_statistics_v2 \
  --base-url https://qs.example.com \
  --org-ids 1,2 \
  --from 2025-01-01 \
  --to 2025-12-31 \
  --window-days 7 \
  --reason approved_history_backfill \
  --mode repair \
  --confirm
```

单次 Run 最多 31 天。工具不会打印 Token，遇到任一机构或窗口失败时立即停止；修复后重跑
同一窗口即可依靠 `fact_key` 完成幂等补偿。`org_id` 通过受保护请求作用域传递，不进入请求体。

`repair` 是历史回填的默认模式：它写入 Fact 并重建指定窗口的 Daily，但不会移动机构的
`as_of_date` 或切换缓存。全部历史窗口修复并完成对账后，再对最近 7 个完整自然日显式执行
一次 `--mode publish --confirm`，用于重建全局 Fulfillment/Snapshot 并发布新的缓存 Generation。

从 migration `000055` 起，Repair 提交后、最终 Publish 完成前，V2 热请求仍可命中上一代缓存，
但冷请求会返回 `statistics_publication_in_progress`，不会读取尚未正式发布的 Daily。操作窗口必须
连续完成 Repair、对账和最终 Publish，不能把机构长期留在“结果已修复但未发布”状态。脚本输出的
`cache_generation/cache_published_at` 用于核对最终 Publish 实际切换的缓存代际。
