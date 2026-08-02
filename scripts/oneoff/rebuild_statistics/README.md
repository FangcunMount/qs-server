# Statistics 校验、修复与重建

该工具不直接操作 Fact 或统计结果表，而是按机构、按上海自然日小窗口调用唯一的
`POST /internal/v2/statistics/runs`。人工操作与夜间同步共享 Collector、Projection、幂等规则、
冲突检测和 `statistics_sync_run` 运行账本。

先执行只读校验。校验不仅要求 Run 成功，还要求三个 Collector 的 `inserted/conflict`
全部为零；缺少 Fact、Fact 冲突或响应缺少规范计数都会让工具非零退出：

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

最后只对最近完整上海自然日执行 `publish`，重建全局 Fulfillment/Snapshot，并切换缓存 Generation：

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

历史批次完成后可使用一次性编排模式。它会把指定范围拆成小窗口，严格按
`repair -> validate` 顺序执行；如果指定结束日早于执行时最新完整上海自然日，还会对中间日期
执行 catch-up，最后只 publish 最新完整日：

```bash
QS_STATISTICS_TOKEN='***' go run ./scripts/oneoff/rebuild_statistics \
  --base-url http://127.0.0.1:8081 \
  --org-ids 1 \
  --from 2025-01-01 \
  --to 2026-08-01 \
  --window-days 7 \
  --timeout 10m \
  --reason hist-20250101-20260801-v2 \
  --mode historical-backfill \
  --confirm
```

任何一步失败都会停止，错误会给出可恢复的窗口起始日。修复原因后保留原始 `--from/--to`，
增加错误中给出的日期即可跳过此前成功窗口：

```bash
go run ./scripts/oneoff/rebuild_statistics \
  --base-url http://127.0.0.1:8081 \
  --org-ids 1 \
  --from 2025-01-01 \
  --to 2026-08-01 \
  --resume-from 2025-03-05 \
  --window-days 7 \
  --timeout 10m \
  --reason hist-20250101-20260801-v2 \
  --mode historical-backfill \
  --confirm
```

`--resume-from` 是包含式上海业务日期，可以位于原始历史范围或自动 catch-up 范围内。
恢复窗口会重新执行一次幂等 repair，再执行严格 validate。不要改 `reason` 来绕过失败；
`reason` 只用于审计，不参与 Fact 幂等身份。

如果最终 publish 已经提交 MySQL 数据、但缓存发布失败，服务端会返回 `data_committed`。
工具会自动调用受保护的 `resume-cache` 接口，复用同一 Run，不重新采集或投影。

## 生产执行边界

- ServerA 上使用 `http://127.0.0.1:8081`，避免 Nginx 的同步请求超时。
- 当前生产 Statistics 锁租约为 30 分钟且未启用自动续租。单窗口必须明显短于 30 分钟；
  默认继续使用 7 天窗口和 10 分钟 HTTP 超时，不建议直接改回 31 天。
- `--timeout` 只调整客户端单请求超时，不延长服务端锁租约。
- 工具只补齐/校验 Fact，并原子替换窗口 Daily；不会删除“来源已经不存在”的孤岛 Fact。
  执行前必须完成业务源和三类 `statistics_*_fact` 的孤岛审计与清理。
- 不要与 nightly publish 或另一个人工 Statistics Run 并发执行。
- Token 只通过 `QS_STATISTICS_TOKEN` 环境变量注入，不写入脚本、命令历史或文档。

执行前检查未完成 Run：

```sql
SELECT id, run_mode, status, stage, window_start, window_end,
       error_code, error_message, started_at
FROM statistics_sync_run
WHERE org_id = 1
  AND status IN ('running', 'data_committed')
ORDER BY id DESC;
```

`data_committed` 必须先由工具自动续传或人工调用 `resume-cache` 完成；对仍然持有 Redis 锁的
`running` Run 不得启动新批次。

单次 Run 最多 31 天。工具不会打印 Token；任一机构或窗口失败都会立即停止。修复后可重跑
同一窗口，`fact_key` 保证幂等。`org_id` 通过受保护请求作用域传递，不进入请求体。
