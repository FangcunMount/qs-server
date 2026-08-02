# 压测 Testee 数据清理

工具只接受显式 `--testee-ids` 或 `--testee-ids-file`，默认执行 dry-run。

先停止会继续写入目标 Testee 的进程并完成数据库备份，然后执行 dry-run：

```bash
go run ./scripts/oneoff/cleanup_perf_testee_data \
  --mysql-dsn '***' \
  --mongo-uri '***' \
  --mongo-db qs \
  --testee-ids-file /secure/path/perf-testee-ids.txt \
  --backup-suffix perf_20260802
```

核对输出的 Testee、Assessment、Outcome、Plan Enrollment/Task、AnswerSheet、Report、Outbox
和 Statistics 范围后才允许 apply：

```bash
go run ./scripts/oneoff/cleanup_perf_testee_data \
  --mysql-dsn '***' \
  --mongo-uri '***' \
  --mongo-db qs \
  --testee-ids-file /secure/path/perf-testee-ids.txt \
  --backup-suffix perf_20260802 \
  --apply
```

`--skip-backup` 和 `--mongo-only` 仅用于已经完成外部备份或恢复中断清理的显式运维场景。
删除后按输出的受影响日期使用 `rebuild_statistics` 执行普通 repair、validate 和 publish。
