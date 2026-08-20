# Select seeddata duplicate testees

只读识别 `daily_simulation` 重试产生的重复 Testee，并生成后续审计所需的显式 ID 清单。

## 判定边界

- 必须显式指定 clinician 和创建日期范围。
- 只处理指定 `source`，默认 `daily_simulation`。
- 重复键为 `IAM user + 创建日期 + org + 姓名 + 性别 + 生日 + source`。
- 每组优先保留下游完成度最高的一条：Outcome、evaluated Assessment、completed Task、submitted Assessment、Assessment、Intake、Enrollment 依次比较；完成度相同才按 `created_at, testee_id` 保留最早记录。
- Profile、ProfileLink、User 不完整，或 Testee/Profile 姓名、性别、生日不一致时 fail closed。
- 工具只执行只读事务，不修改数据库。

## 生成清单

```bash
go run ./scripts/oneoff/select_seeddata_duplicate_testees \
  --mysql-dsn "$QS_MYSQL_DSN" \
  --iam-schema iam \
  --clinician-id "$QS_CLINICIAN_ID" \
  --created-from "$QS_CREATED_FROM" \
  --created-through "$QS_CREATED_THROUGH" \
  --output-dir "$QS_AUDIT_OUTPUT_DIR" \
  --workers 2 \
  --progress-interval 5s \
  --timeout 30m
```

selector 按上海自然日拆分查询；重复键本身包含创建日期，因此日期分片不会拆散重复组。每个
分片使用独立的 `REPEATABLE READ` 只读事务，并发数默认 `2`、上限 `4`。查询从 clinician
关系索引缩小 Testee 范围，再以 `org_id + testee_id` 命中 Intake/Enrollment 联合索引。
进度输出会持续显示已完成日期、当前活动日期、累计重复行数和耗时；通过 `tee` 保存日志时
仍然是一行一条记录，不依赖交互式终端控制字符。生产首次执行建议保持 `--workers 2`，确认
数据库负载正常后才提高到 `4`。

输出文件权限为 `0600`，且不会覆盖已有文件：

- `manifest.csv`：目标 Testee/Profile/ProfileLink、对应 IAM User、保留行映射和下游完成度计数。
- `testee_ids.txt`：显式 Testee 范围；可供 `cleanup_perf_testee_data --testee-ids-file`
  只清理其中的临时测评，但不能用于删除重复 Testee。
- `profile_ids.txt`：IAM Profile 审计范围；当前 `cleanup_perf_testee_data` 不读取也不删除 IAM 数据。
- `summary.txt`：范围与行数摘要。

生成后必须先人工核对 `summary.txt`、`manifest.csv` 抽样，以及按日期汇总是否与事故查询一致。

## 与临时测评清理工具的边界

`cleanup_perf_testee_data` 现在只删除 `assessment.origin_type = 'adhoc'` 的临时测评及其派生数据，
并明确保留 Testee、IAM Profile、关系、计划入组和计划任务。因此它不能完成重复 Testee 清零。

如果仅需清理这些重复 Testee 产生的临时测评，可在停止相关写入并完成 MySQL/Mongo 数据库级
备份后先执行 dry-run：

```bash
go run ./scripts/oneoff/cleanup_perf_testee_data \
  --mysql-dsn "$QS_MYSQL_DSN" \
  --mongo-uri "$QS_MONGO_URI" \
  --mongo-db qs \
  --testee-ids-file "$QS_AUDIT_OUTPUT_DIR/testee_ids.txt" \
  --backup-suffix "$QS_BACKUP_SUFFIX"
```

机构业务 ID、日期窗口、输出目录与备份后缀都是本次变更输入，必须进入审批与
evidence，不在 sidecar 中预置某次生产参数。

核对临时 Assessment、Mongo 和统计范围后，使用完全相同的参数追加 `--apply`。删除后按工具
输出的日期窗口运行 `rebuild_statistics` repair/validate/publish。若需要删除重复 Testee/Profile，
必须另行使用具备独立 dry-run、备份、引用校验和回滚边界的专用维护工具；当前仓库没有将该
删除能力隐含在临时测评清理脚本中。
