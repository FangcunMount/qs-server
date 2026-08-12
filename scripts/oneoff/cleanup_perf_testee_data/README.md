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

内置 MySQL 备份表使用短前缀 `cbpt_`；工具在创建任何备份对象前，会根据 MySQL 64 字符
标识符上限校验 `--backup-suffix`，避免并发备份启动后才因表名过长留下部分备份。
备份插入会从 `information_schema.columns` 读取源表列，显式排除生成列；备份表通过
`CREATE TABLE ... LIKE` 保留生成列表达式，由 MySQL 根据普通列重新计算其值。

IAM 删除发生在 QS/MySQL 和 Mongo 成功之后。若中途已删 QS 但尚未删 IAM，可用同一份
Testee/Profile 清单、同一备份后缀，加 `--mongo-only --allow-missing-testees` 恢复执行。

删除后按输出的受影响日期使用 `rebuild_statistics` 执行普通 repair、validate 和 publish。
