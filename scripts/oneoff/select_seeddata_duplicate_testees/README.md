# Select seeddata duplicate testees

只读识别 `daily_simulation` 重试产生的重复 Testee，并生成后续清理所需的显式 ID 清单。

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
  --clinician-id 614995509882401326 \
  --created-from 2026-08-05 \
  --created-through 2026-08-12 \
  --output-dir /secure/path/seeddata-duplicates-20260812 \
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
- `testee_ids.txt`：供 `cleanup_perf_testee_data --testee-ids-file` 使用。
- `profile_ids.txt`：供 `cleanup_perf_testee_data --profile-ids-file` 使用。
- `summary.txt`：范围与行数摘要。

生成后必须先人工核对 `summary.txt`、`manifest.csv` 抽样，以及按日期汇总是否与事故查询一致，再进入清理工具 dry-run。

## 清理顺序

runner 以及其他会写目标 Testee 的进程保持停止，且 MySQL/Mongo 已完成数据库级备份后，先执行 dry-run：

```bash
go run ./scripts/oneoff/cleanup_perf_testee_data \
  --mysql-dsn "$QS_MYSQL_DSN" \
  --iam-mysql-dsn "$IAM_MYSQL_DSN" \
  --mongo-uri "$QS_MONGO_URI" \
  --mongo-db qs \
  --testee-ids-file /secure/path/seeddata-duplicates-20260812/testee_ids.txt \
  --profile-ids-file /secure/path/seeddata-duplicates-20260812/profile_ids.txt \
  --scan-event-payloads \
  --mysql-outbox-scan-batch-size 500 \
  --iam-delete-batch-size 500 \
  --backup-suffix seeddata_dup_20260812
```

`--scan-event-payloads` 以目标 Testee 最早创建时间为下界、以启动扫描时的 Outbox 最大 ID
为上界，按主键游标分批读取；每条 payload 只解码一次，遇到非法 JSON 会 fail closed。
它不再执行“每个 Testee ID 对 Outbox 全表做一次 REGEXP”的查询。

核对 QS、Mongo、IAM 各表计数和样例后，使用完全相同的参数追加 `--apply`。工具先备份，
再依次删除 QS/MySQL、Mongo、IAM；进入 IAM 前会重新确认 QS 中已无目标或非目标 Testee
引用这些 Profile，随后按 `--iam-delete-batch-size` 分批事务删除 ProfileLink 和 Profile。
删除后按工具输出的日期窗口运行 `rebuild_statistics` repair/validate/publish，并重新执行本 selector；
输出 `no duplicate rows matched the explicit scope` 才表示本范围已清零。
