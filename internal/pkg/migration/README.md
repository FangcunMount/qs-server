# 数据库迁移

> 状态：当前有效
>
> 适用范围：`internal/pkg/migration` 的 MySQL 与 MongoDB schema migration
>
> 事实来源：`migrate.go`、`driver.go`、`internal/apiserver/bootstrap/database.go` 与嵌入式迁移文件

## 结论

qs-server 在 apiserver 启动阶段按配置执行 MySQL 与 MongoDB 向上迁移。迁移文件嵌入二进制，不依赖运行目录挂载 SQL 或 JSON 文件。

迁移版本机制只保证同一版本不会被正常重复执行，不保证迁移无损。仓库中存在 `DROP`、结构替换、数据清理和遗留对象退役迁移；任何生产迁移都必须按迁移内容评估风险、备份和验收。

当前实现的运行时入口只暴露 `Run()` 向上迁移：

- MySQL：`NewMigrator(db, config)`；
- MongoDB：`NewMongoMigrator(client, config)`；
- dirty 状态会阻断继续迁移；
- 当前目录末端版本为 MySQL `70`、MongoDB `32`。生产实际版本以数据库只读查询和[当前版本定档验收台账](../../../docs/00-总览/09-当前版本定档验收台账.md)为准；仓库目录版本不能单独证明生产已执行到该版本。

## 目录与职责

```text
internal/pkg/migration/
├── migrate.go
├── driver.go
├── mysql_driver.go
├── mongodb_driver.go
└── migrations/
    ├── mysql/
    │   ├── 000001_*.up.sql
    │   └── 000001_*.down.sql
    └── mongodb/
        ├── 000001_*.up.json
        └── 000001_*.down.json
```

MySQL 和 MongoDB 各自维护独立的 `schema_migrations` 状态，版本号只在同一种后端内连续，不要求两个后端同步。

## 启动时序

apiserver 的数据库 bootstrap 执行顺序为：

1. 建立数据库连接；
2. `migration.enabled=true` 时执行 MySQL migration；
3. 配置了 MongoDB 时执行 MongoDB migration；
4. MongoDB migration 完成后协调并验证 ModelCatalog 与报告目录索引；
5. 任一步失败都会中止启动，不应带着部分完成状态继续提供服务。

正常开发启动：

```bash
make run-apiserver
```

或显式运行二进制：

```bash
./bin/qs-apiserver --config=configs/apiserver.dev.yaml
```

当前仓库没有独立的 `make migrate-up`、`make migrate-down` 或 migration CLI。不要在文档、脚本或操作手册中假设这些命令存在。

## 配置

apiserver YAML 中的配置为：

```yaml
migration:
  enabled: true
  autoseed: false
  database: qs
```

默认值由应用 options 提供：`enabled=true`、`autoseed=false`。环境变量覆盖必须使用 apiserver 前缀，例如：

```bash
QS_APISERVER_MIGRATION_ENABLED=false make run-apiserver
```

无前缀的 `MIGRATION_ENABLED` 不会按当前配置加载规则覆盖该字段。关闭迁移只用于明确的开发或运维场景，不能作为绕过 dirty 状态或失败迁移的恢复手段。

## 新增迁移

### MySQL

在 `migrations/mysql/` 下增加同版本号的一对文件：

```text
000070_change_name.up.sql
000070_change_name.down.sql
```

要求：

- 版本号使用当前 MySQL 最大版本的下一个值；
- `up` 只承担一个清晰的 schema 或数据治理目标；
- `down` 明确可恢复到什么程度；若数据不可逆，不能声称可以完整回滚；
- DDL、DML、索引和约束变更都要评估锁表、耗时与线上数据分布；
- 对关键迁移增加空库建库、历史版本升级或专用集成测试。

### MongoDB

在 `migrations/mongodb/` 下增加同版本号的一对 JSON 文件：

```text
000033_change_name.up.json
000033_change_name.down.json
```

文件内容是操作数组，例如：

```json
[
  {
    "createIndexes": "example_collection",
    "indexes": [
      {
        "key": { "org_id": 1, "created_at": -1 },
        "name": "idx_example_org_created"
      }
    ]
  }
]
```

支持的操作和约束以 `mongodb_driver.go` 及 [MongoDB migration 说明](migrations/mongodb/README.md)为准，不要凭借 Mongo Shell 语法推断 JSON driver 一定支持。

## 风险分级

| 类型 | 例子 | 最低验收要求 |
|---|---|---|
| 增量变更 | 新表、新集合、新索引、可空字段 | 空库与升级路径测试、性能影响检查 |
| 数据改写 | backfill、状态归一化、唯一键收敛 | 只读盘点、影响行数、幂等性、失败恢复方案 |
| 破坏性退役 | `DROP TABLE`、`drop` 集合、删除字段或历史数据 | 调用与查询依赖清零、备份、恢复演练、明确授权、执行后盘点 |

破坏性迁移至少需要：

1. 扫描当前代码、脚本、定时任务、接口与跨仓消费者；
2. 对目标对象做生产只读盘点，确认行数、集合数、索引和近期读写；
3. 明确备份位置、保留期和恢复验证方式；
4. 在维护窗口执行，并记录迁移前后版本、耗时和影响量；
5. 运行应用健康检查、关键业务烟测与数据完整性查询；
6. 将结果回填到版本验收台账。

迁移文件存在 `down` 不代表数据可恢复。很多退役迁移的 `down` 只能重建空结构，无法复原已删除的数据。

## dirty 状态处理

当 `schema_migrations` 标记为 dirty 时，`Run()` 会返回错误并中止启动。处理顺序：

1. 保留现场，确认后端、版本、失败日志和已执行的语句或操作；
2. 检查目标 schema 与数据处于“未执行、部分执行、已执行但未结算”的哪一种状态；
3. 根据该版本的 `up/down` 和备份制定恢复或补偿方案；
4. 在测试环境复现并验证方案；
5. 经明确审批后，才可以修正版本状态或使用外部 migration 工具的 force 能力；
6. 重新启动并完成 schema、数据和业务验收。

不要把直接修改 `schema_migrations.dirty` 作为首选操作。仅清除 dirty 标记可能导致版本记录与真实 schema 永久分叉。

## 回滚边界

应用内 `Migrator` 当前没有 `Rollback()` 方法，正常启动路径也不执行 down migration。`.down.sql` 与 `.down.json` 用于迁移契约、受控测试和经审批的外部恢复流程。

生产回滚优先级通常是：

1. 停止继续放大影响；
2. 回退应用版本或关闭受影响入口；
3. 根据迁移性质选择前向修复、受控 down 或备份恢复；
4. 重新验证版本状态和业务数据。

对于已经删除的数据，应用回退和 down migration 都不能替代备份恢复。

## 验证

提交 migration 变更前至少运行：

```bash
go test ./internal/pkg/migration
git diff --check
make docs-hygiene
```

涉及真实 MySQL/MongoDB 的集成测试需要明确授权的测试实例。测试跳过、空库通过或本地编译成功，都不能单独证明生产迁移成功。

生产验收应记录：

- MySQL 与 MongoDB 的版本和 dirty 状态；
- 迁移执行耗时与错误日志；
- 目标表、集合、索引和数据量；
- 三个进程的实际构建 SHA、健康状态与关键链路烟测；
- 备份和恢复证据。

## 相关文档

- [当前版本定档验收台账](../../../docs/00-总览/09-当前版本定档验收台账.md)
- [本地开发与配置约定](../../../docs/00-总览/06-本地开发与配置约定.md)
- [配置与环境变量](../../../docs/04-接口与运维/05-配置与环境变量.md)
- [部署与端口](../../../docs/04-接口与运维/06-部署与端口.md)
