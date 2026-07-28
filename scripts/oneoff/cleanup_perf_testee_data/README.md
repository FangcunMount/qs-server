# 压测与历史回填数据清理

工具默认只执行 dry-run。普通压测仍可使用显式 `--testee-ids`；历史回填必须用批次账本和
runner manifest 共同确定范围，不能按 Org 或模糊日期删除。

先停止 seeddata-runner、worker、Plan scheduler、Statistics scheduler，并完成数据库备份，
然后执行 dry-run：

```bash
go run ./scripts/oneoff/cleanup_perf_testee_data \
  --mysql-dsn '***' \
  --mongo-uri '***' \
  --mongo-db qs \
  --seed-batch-id hist-20250101-20260727-v1 \
  --seed-manifest /secure/path/hist-20250101-20260727-v1.manifest.json \
  --dry-run-receipt /secure/path/hist-20250101-20260727-v1.rollback-receipt.json \
  --backup-suffix hist_20250101_20260727_v1
```

核对输出的 Testee、Assessment、Outcome、Plan Enrollment/Task、AnswerSheet、Report、
Outbox 和 Statistics 范围后才允许 apply：

```bash
go run ./scripts/oneoff/cleanup_perf_testee_data \
  --mysql-dsn '***' \
  --mongo-uri '***' \
  --mongo-db qs \
  --seed-batch-id hist-20250101-20260727-v1 \
  --seed-manifest /secure/path/hist-20250101-20260727-v1.manifest.json \
  --dry-run-receipt /secure/path/hist-20250101-20260727-v1.rollback-receipt.json \
  --statistics-base-url https://qs.example.internal \
  --confirm-services-stopped \
  --apply
```

`QS_STATISTICS_TOKEN` 通过环境变量注入。receipt v2 仅保存 operation ID、Org、batch、manifest/scope
哈希和生成时间，文件权限为 `0600`；v1 receipt 不能 apply，必须重新 dry-run。dry-run 会先把
MySQL 实际主键、Mongo 实际 `_id` 和 Statistics 业务日期写入持久化 operation/resource 账本。

批次模式强制读取 `seed_backfill_stage`，并要求 manifest 覆盖尚未进入 intake 的
`create_testee`/`resolve_entry` 场景；只有明确记录为本批次新建的 Testee 才会物理删除。
Statistics Fact 按本批次资源精确删除，Daily/Snapshot 按受影响日期重建；
`statistics_sync_run` 作为运维审计历史始终保留。
内建备份不能用 `--skip-backup` 绕过生产变更流程，历史批次也禁止 `--mongo-only`。apply
按照 `backed_up → mongo_deleted → mysql_deleted → statistics_repaired → ledgers_deleted → completed`
单调推进；同一 receipt 重试会从未完成 phase 恢复。任一跨存储步骤失败时 operation/resource
和服务端完成账本都会保留。脚本在删除业务事实后自动 repair/validate/publish Statistics，确认
完成后再恢复 scheduler 和 worker。operation、resource、phase attempt、receipt 与备份至少保留
90 天。
