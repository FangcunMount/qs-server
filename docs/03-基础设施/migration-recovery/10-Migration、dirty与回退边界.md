# Migration、dirty 与回退边界

## 1. 当前结论

qs-server 在 apiserver 启动资源阶段执行 MySQL 与 MongoDB 的嵌入式 up migration。两个后端分别维护版本/dirty 状态；任一启用迁移失败都会阻断继续启动。仓库没有应用级 rollback API，也没有通用 `make migrate-up/down/force` 命令。

本页与 [Data Access 的事务边界](../data-access/10-存储所有权与事务边界.md)共同构成 `data/migration/transaction` 主题：本页负责 schema 生命周期，Data Access 负责业务写入事务。

## 2. 启动顺序与事实源

1. `internal/apiserver/bootstrap/database.go` 建立数据库连接并做依赖检查；
2. `internal/pkg/migration/migrate.go` 根据 `migration.enabled` 运行迁移；
3. MySQL/Mongo driver 从 `internal/pkg/migration/migrations/` 的嵌入文件创建实例；
4. Mongo migration 之后执行所需的索引协调/校验；
5. 迁移或前置条件失败时，资源阶段失败，后续容器和 transport 不应启动。

当前迁移数量必须由目录和 checker 计算，不能写死在 prose。新增版本必须同后端 up/down 成对、版本唯一，并纳入契约测试。

## 3. dirty 状态处置

```text
发现 dirty
  -> 停止继续部署和重启风暴
  -> 冻结数据库、应用 SHA、migration 版本和错误日志
  -> 对照具体 up 文件核验已执行到哪一步
  -> 在同类测试环境重放并评估数据影响
  -> 选择完成迁移、从备份恢复或经审批修正版本状态
  -> 重跑 schema/索引/业务不变量检查
```

禁止把直接改 `schema_migrations.dirty` 或 `migrate force N` 当作常规步骤。`force` 只改变版本记账，不能补做缺失操作或恢复已删除数据。MongoDB 13 当前迁移不包含 `dropIndexes`，不得保留“force 13 绕过 dropIndexes”的旧指引。

## 4. 回退边界

- 应用正常启动路径只执行 up，不自动执行 down。
- down 文件用于契约测试和受控恢复设计；是否可执行必须逐版本审计。
- 删除表、集合、索引或重写数据后，down 可能只恢复结构，不能恢复数据。
- 旧镜像能否回滚取决于它是否兼容已经落地的新 schema、事件和配置。
- 备份恢复必须同时考虑业务数据与 migration state，不能只恢复其中一边。

## 5. 验证层级

| 层级 | 入口 | 状态口径 |
| --- | --- | --- |
| 静态契约 | `go test -count=1 ./internal/pkg/migration` | 当前源码/文件对、破坏性 token 和特定 schema 约束；skip 的 integration 不算通过 |
| MySQL 环境 | 配置真实隔离 DSN 运行 integration | 建库、0→latest、特定版本转换与 down 结构行为 |
| Mongo 环境 | Replica Set integration tag/环境 | 事务、索引、0→latest 与前置条件 |
| 部署前快照 | 版本、dirty、schema/index、备份可恢复性 | 本次目标环境是否具备迁移前提 |
| 部署后核验 | exact SHA + migration version + 业务查询 | 本次迁移与程序组合是否成立 |

## 6. 未闭合 gap

1. 本地包测试不能代替真实 MySQL 和 Mongo Replica Set 的 0→latest 验证。
2. 不是所有 down migration 都具备数据可逆性，缺少逐版本恢复等级台账。
3. 当前 checkout 没有绑定 exact SHA 的目标环境 version/dirty/schema 快照。
4. 应用启动自动迁移扩大了启动故障域；多副本并发、长迁移和锁等待必须在真实环境评估。
