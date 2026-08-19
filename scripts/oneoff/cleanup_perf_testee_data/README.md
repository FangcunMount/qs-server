# 按 Testee 清理临时测评数据

工具只接受显式 `--testee-ids` 或 `--testee-ids-file`，并且只把
`assessment.origin_type = 'adhoc'`（领域展示名“临时测评”）纳入清理范围。
默认执行 dry-run。

工具会保留以下数据：

- Testee、IAM Profile、登录身份和凭据；
- 医患关系、测评入口和接诊日志；
- 测评计划、计划入组、计划任务及其统计事实；
- 同一 Testee 的 `origin_type = 'plan'` 计划测评及其答卷、结果和报告。

若发现候选临时测评意外关联了计划任务，工具会直接中止，避免破坏计划数据。

先停止会继续写入目标 Testee 临时测评的进程并完成数据库备份，然后执行 dry-run：

```bash
go run ./scripts/oneoff/cleanup_perf_testee_data \
  --mysql-dsn '***' \
  --mongo-uri '***' \
  --mongo-db qs \
  --testee-ids-file /secure/path/testee-ids.txt \
  --backup-suffix temp_assessment_20260819
```

核对输出中的 `temporary_assessments`、Outcome、AnswerSheet、Report、Outbox 和
Statistics 范围后才允许 apply：

```bash
go run ./scripts/oneoff/cleanup_perf_testee_data \
  --mysql-dsn '***' \
  --mongo-uri '***' \
  --mongo-db qs \
  --testee-ids-file /secure/path/testee-ids.txt \
  --backup-suffix temp_assessment_20260819 \
  --apply
```

`--skip-backup` 仅用于已经完成外部备份的显式运维场景。脚本不再提供按
`testee_id` 扫描整个 Outbox payload、删除 Testee/IAM 数据或 `--mongo-only`
恢复模式，因为这些路径无法保证“只清理临时测评”的边界。Apply 会先删 Mongo
派生数据、再删 MySQL 数据；若 MySQL 阶段失败，仍可由保留的 MySQL Assessment
重新建立同一范围后重试。

内置 MySQL 备份表使用短前缀 `cbpt_`；工具在创建任何备份对象前，会根据 MySQL
64 字符标识符上限校验 `--backup-suffix`。备份插入会从
`information_schema.columns` 读取源表列，显式排除生成列；备份表通过
`CREATE TABLE ... LIKE` 保留生成列表达式，由 MySQL 根据普通列重新计算其值。
Mongo 备份集合使用 `cleanup_bak_temp_assessment_<collection>_<suffix>`。

删除后按输出的受影响日期使用 `rebuild_statistics` 执行普通 repair、validate 和 publish。
