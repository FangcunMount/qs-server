# Historical Statistics verification

该工具在回填前保存 Statistics 事实表基线，并在 repair 后同时执行两种检查：

1. 通过 `seed_backfill_stage` 中的精确资源 ID，逐日匹配 access、plan、assessment 三类事实；
2. 比较“当前总数 - 回填前基线”是否至少覆盖本批次增量。生产期间的正常流量会显示为
   `unattributed_delta`，不会被错误归入历史批次。

构建：

```bash
go build -o tmp/bin/verify-historical-statistics ./scripts/oneoff/verify_historical_statistics
```

回填前采集基线：

```bash
export QS_MYSQL_DSN='<user>:<password>@tcp(<host>:3306)/<db>?parseTime=true&loc=Asia%2FShanghai'
tmp/bin/verify-historical-statistics \
  --mode capture-baseline \
  --org-id <org-id> \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --output tmp/historical-statistics-baseline.json
```

19 个 repair/validate 窗口和最终 publish 成功后验证：

```bash
tmp/bin/verify-historical-statistics \
  --mode verify \
  --org-id <org-id> \
  --batch-id hist-20250101-20260727-v1 \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --baseline tmp/historical-statistics-baseline.json \
  --output tmp/historical-statistics-verification.json
```

`complete=false` 或非零退出表示存在缺失事实、事实日期错误或总增量不足，必须停止发布验收并保留输出用于排查。
