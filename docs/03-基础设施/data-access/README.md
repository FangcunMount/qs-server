# Data Access

Data Access 维护事实所有权、本地事务、repository/read-model port、Outbox 同事务、数据库约束与存储 backpressure。
Schema migration、dirty 和生产修复已提升为独立的[Migration 与恢复](../migration-recovery/README.md)主题。

## 阅读顺序

1. [存储所有权与事务边界](./10-存储所有权与事务边界.md)：`data/migration/transaction` 中的数据所有权与业务事务 canonical 文档。
2. [核心设计决策与替代方案](./15-核心设计决策与替代方案.md)：多存储、本地事务、Outbox、约束和查询模型的取舍。

## 不变量

- MySQL 与 MongoDB 分别拥有不同业务事实；Redis 只拥有明确标注的 operational state，不替代权威业务事实。
- MySQL UoW 和 Mongo session transaction 都只承诺单后端本地事务，系统没有跨库原子提交。
- 可靠事件与业务事实必须落在同一后端、同一事务的 Outbox；跨库后续动作靠 at-least-once、幂等和可恢复状态机收敛。
- unique constraint、claim/CAS 和 durable readback 裁决正确性；Redis guard/ready-index 只降低竞争与恢复成本。

## 验证边界

包级非缓存测试可以证明 adapter/事务契约；真实 MySQL/Mongo Replica Set、连接池压力、故障恢复与跨库延迟必须单列环境证据。迁移与 one-off 的验证入口见 [Migration 与恢复](../migration-recovery/README.md)。
