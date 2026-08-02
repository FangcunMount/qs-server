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

## 长时间运行时自动刷新 IAM Token

只设置 `QS_STATISTICS_TOKEN` 的静态模式保持兼容，适合能够在 Token 到期前结束的短任务。
跨数小时的历史重建应改用 IAM 密码登录模式。密码不能放在命令行、环境变量或仓库配置中，
先在 ServerA 创建仅当前执行用户可读的普通文件：

```bash
sudo install -m 0600 -o root -g root /dev/null \
  /secure/path/qs-statistics-iam-password
sudoedit /secure/path/qs-statistics-iam-password
```

文件内容只写 IAM 密码本身。工具拒绝符号链接、目录以及授予 group/other 权限的文件。
然后配置 IAM 登录参数：

```bash
export QS_STATISTICS_IAM_LOGIN_URL='https://iam.fangcunmount.cn/api/v2/authn/login'
export QS_STATISTICS_IAM_USERNAME='system@fangcunmount.com'
export QS_STATISTICS_IAM_PASSWORD_FILE='/secure/path/qs-statistics-iam-password'

# system 账号通常不需要显式 tenant_id；需要时只能设置数字 ID。
unset QS_STATISTICS_IAM_TENANT_ID

# 可选；默认在 JWT 到期前 2 分钟刷新。
export QS_STATISTICS_IAM_REFRESH_SKEW='2m'
```

此时可以不再设置静态 Token：

```bash
unset QS_STATISTICS_TOKEN

go run ./scripts/oneoff/rebuild_statistics \
  --base-url http://127.0.0.1:8081 \
  --org-ids 1 \
  --from 2025-01-01 \
  --to 2026-08-01 \
  --resume-from 2025-01-01 \
  --window-days 7 \
  --timeout 10m \
  --reason hist-20250101-20260801-v2 \
  --mode historical-backfill \
  --confirm
```

工具启动时通过 IAM 获取 Token，每次请求前检查 JWT `exp`，在到期窗口内自动重新登录；
如果 apiserver 返回一次 401，还会强制重新登录并原请求重试一次。403 表示权限/机构作用域问题，
不会通过刷新掩盖。每次登录都会重新读取密码文件，因此密码轮换后不必把密码暴露给进程环境。
日志只打印安全的刷新原因和过期时间，不打印 Token、用户名或密码。

也可以同时保留 `QS_STATISTICS_TOKEN` 和上述 IAM 参数：有效期足够的现有 Token 会先使用，
接近到期或收到 401 后才通过 IAM 刷新。CLI 对应参数为 `--iam-login-url`、`--iam-username`、
`--iam-password-file`、`--iam-tenant-id` 和 `--iam-refresh-skew`，显式 CLI 参数优先于环境变量。

已经启动的旧版本进程不会动态获得自动刷新能力。它若因 401 停止，使用错误中给出的
`--resume-from` 日期和相同 `reason` 在新版本上继续；此前成功的窗口不需要重做。

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
- 短任务的静态 Token 只通过 `QS_STATISTICS_TOKEN` 注入；长任务使用上述权限为 `0600` 的
  IAM 密码文件。两种密钥都不得写入脚本、命令历史、日志或仓库文档。

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
