# Interpretation 旧集合退役

## 结论

`interpret_reports` 不属于 Interpretation 终局存储。只有 catalog 对账通过、运行时代码已无旧集合引用后，才能备份并 drop。备份保留 30 天；`archived_reports` 不删除。

## 前置 Gate

```bash
go run ./scripts/oneoff/backfill_interpretation_report_catalog \
  --mongo-uri "$MONGO_URI" --mongo-db "$MONGO_DB" \
  --mysql-dsn "$MYSQL_DSN" --verify-only
```

要求 `count_mismatch`、`missing_assessment`、`missing_org`、`missing_testee`、`missing_archive`、`wrong_priority`、`dangling_source` 全为 0。部署移除 legacy 索引和生产引用的版本，并完成一个正常观察窗口后再继续。

## 备份与校验

```bash
mkdir -p /var/backups/qs/interpretation

mongodump \
  --uri "$MONGO_URI" \
  --db "$MONGO_DB" \
  --collection interpret_reports \
  --archive=/var/backups/qs/interpretation/interpret_reports_$(date +%Y%m%dT%H%M%S).archive.gz \
  --gzip

sha256sum /var/backups/qs/interpretation/interpret_reports_*.archive.gz
stat --printf='%n %s bytes\n' /var/backups/qs/interpretation/interpret_reports_*.archive.gz
```

备份前另存以下 Mongo 输出，与文件大小、SHA-256 一起写入退役记录：

```javascript
db.interpret_reports.countDocuments({})
db.interpret_reports.getIndexes()
db.interpret_reports.aggregate([{ $sample: { size: 20 } }])
```

将备份恢复到隔离数据库，复核总量、索引和样本后才能 drop。

```bash
mongorestore \
  --uri "$RESTORE_MONGO_URI" \
  --archive=/var/backups/qs/interpretation/interpret_reports_<timestamp>.archive.gz \
  --gzip \
  --nsFrom="$MONGO_DB.interpret_reports" \
  --nsTo="qs_restore.interpret_reports"
```

恢复库的 `countDocuments({})` 必须与备份前记录相等，随机样本必须可读取，索引清单必须完整。

## Drop 与验收

```javascript
db.interpret_reports.drop()
```

随后重新执行 catalog verify，并完成管理员/参与者/临床报告查询、Worker 新报告生成、operations 生命周期查询 smoke test。观察期回滚固定为恢复备份并回滚上一发布版本，不新增 legacy feature flag。

备份自 drop 日起保留 30 天；期间对账持续为 0 且无回滚后删除备份。
