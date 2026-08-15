# Interpretation 独立运行账本迁移

该命令将以下 MongoDB 集合幂等迁移到 MySQL migration `000068` 创建的表：

- `interpretation_admission_failures`
- `interpretation_catalog_audit_checkpoints`
- `interpretation_attention_projections`

报告文档、Generation、Run、Report Catalog 和 Mongo Outbox 不在本次迁移范围内。

## 执行顺序

1. 进入短维护窗口，停止 `qs-apiserver` 和全部 `qs-worker`，避免 Mongo/MySQL 之间继续产生新写入。
2. 执行 `--apply --prepare-schema --confirm-services-stopped`；该模式先通过仓库内嵌 migrator 应用包括 `000068` 在内的全部待执行 MySQL migration，再搬迁数据。
3. 如果 schema 已由独立发布步骤升级，可先执行无参数只读预检，再省略 `--prepare-schema` 执行 apply。
4. 启动 `qs-apiserver`，验证准入失败查询与 Catalog Audit。
5. 逐个启动 `qs-worker`，验证 Attention Projection 重试指标。
6. 观察窗口结束前保留三个 Mongo 源集合，不要 drop。

建议通过环境变量传递凭据，避免写入 shell history：

```bash
export QS_MYSQL_DSN='***'
export QS_MONGO_URI='***'
export QS_MONGO_DB='qs'

go run ./scripts/oneoff/migrate_interpretation_runtime_ledgers

go run ./scripts/oneoff/migrate_interpretation_runtime_ledgers \
  --apply \
  --prepare-schema \
  --confirm-services-stopped
```

命令会逐条验证 MySQL 状态不落后于 Mongo；重复执行不会增加 Admission attempt，也不会覆盖更新的 MySQL 状态。
