# 压测 Testee 数据清理

工具只接受显式 `--testee-ids` 或 `--testee-ids-file`，默认执行 dry-run。

先停止会继续写入目标 Testee 的进程并完成数据库备份，然后执行 dry-run：

```bash
go run ./scripts/oneoff/cleanup_perf_testee_data \
  --mysql-dsn '***' \
  --iam-mysql-dsn '***' \
  --mongo-uri '***' \
  --mongo-db qs \
  --testee-ids-file /secure/path/perf-testee-ids.txt \
  --profile-ids-file /secure/path/perf-profile-ids.txt \
  --iam-delete-batch-size 500 \
  --backup-suffix perf_20260802
```

核对输出的 Testee、Assessment、Outcome、Plan Enrollment/Task、AnswerSheet、Report、Outbox
和 Statistics 范围后才允许 apply：

```bash
go run ./scripts/oneoff/cleanup_perf_testee_data \
  --mysql-dsn '***' \
  --iam-mysql-dsn '***' \
  --mongo-uri '***' \
  --mongo-db qs \
  --testee-ids-file /secure/path/perf-testee-ids.txt \
  --profile-ids-file /secure/path/perf-profile-ids.txt \
  --iam-delete-batch-size 500 \
  --backup-suffix perf_20260802 \
  --apply
```

`--skip-backup` 和 `--mongo-only` 仅用于已经完成外部备份或恢复中断清理的显式运维场景。
配置 `--iam-mysql-dsn` 时必须同时提供显式 Profile ID 清单。工具会验证 Testee/Profile
一一对应、目标 Profile 没有被非目标 Testee 引用；QS/Mongo 删除后、IAM 删除前会再次执行
零引用检查，再分批事务化删除 IAM `profile_links` 和 `profiles`。不会删除 IAM User、登录身份或凭据。

`--scan-event-payloads` 是显式的补充扫描：从目标 Testee 最早创建时间开始，按 Outbox 主键
游标和 `--mysql-outbox-scan-batch-size` 分批读取，每条 JSON payload 最多解码一次；非法 JSON
会中止操作。默认关闭，不会执行无界的 Testee-ID × Outbox `REGEXP` 联接。

IAM 删除发生在 QS/MySQL 和 Mongo 成功之后。若中途已删 QS 但尚未删 IAM，可用同一份
Testee/Profile 清单、同一备份后缀，加 `--mongo-only --allow-missing-testees` 恢复执行。

删除后按输出的受影响日期使用 `rebuild_statistics` 执行普通 repair、validate 和 publish。
