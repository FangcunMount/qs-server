# Statistics 校验、修复与重建

该工具不直接操作 Fact 或统计结果表，而是按机构、按上海自然日小窗口调用唯一的
`POST /internal/v2/statistics/runs`。人工操作与夜间同步共享 Collector、Projection、幂等规则、
冲突检测和 `statistics_sync_run` 运行账本。

先执行只读校验：

```bash
QS_STATISTICS_TOKEN='***' go run ./scripts/oneoff/rebuild_statistics \
  --base-url https://qs.example.com \
  --org-ids 1,2 \
  --from 2026-01-01 \
  --to 2026-01-07 \
  --window-days 7 \
  --reason rebuild_preflight \
  --validate-only
```

校验通过后，使用 `repair` 重建指定窗口的 Fact 和 Daily：

```bash
QS_STATISTICS_TOKEN='***' go run ./scripts/oneoff/rebuild_statistics \
  --base-url https://qs.example.com \
  --org-ids 1,2 \
  --from 2026-01-01 \
  --to 2026-01-07 \
  --window-days 7 \
  --reason approved_statistics_repair \
  --mode repair \
  --confirm
```

最后对最近完整自然日执行 `publish`，重建全局 Fulfillment/Snapshot，并切换缓存 Generation：

```bash
QS_STATISTICS_TOKEN='***' go run ./scripts/oneoff/rebuild_statistics \
  --base-url https://qs.example.com \
  --org-ids 1,2 \
  --from 2026-01-07 \
  --to 2026-01-07 \
  --reason approved_statistics_publish \
  --mode publish \
  --confirm
```

历史批次完成后可使用一次性编排模式。它会把范围拆成不超过 31 天的窗口，严格按
`repair -> validate` 顺序执行每个窗口，全部成功后只对结束日执行一次 `publish`：

```bash
QS_STATISTICS_TOKEN='***' go run ./scripts/oneoff/rebuild_statistics \
  --base-url https://qs.example.com \
  --org-ids 1 \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --window-days 31 \
  --reason hist-20250101-20260727-v1 \
  --mode historical-backfill \
  --confirm
```

该固定范围会生成 19 个 repair/validate 窗口，最终发布水位为 `2026-07-27`。
任何一步失败都会停止，后续窗口和最终 publish 不会执行。

单次 Run 最多 31 天。工具不会打印 Token；任一机构或窗口失败都会立即停止。修复后可重跑
同一窗口，`fact_key` 保证幂等。`org_id` 通过受保护请求作用域传递，不进入请求体。
