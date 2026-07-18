# Data Access

## 1. 结论

MySQL、MongoDB 和 Redis 承担不同持久化/缓存责任；repository、read model、UoW 与 Outbox 必须保持显式边界。

## 2. 责任

- domain repository：聚合持久化接口；
- read-model port：面向查询的投影接口；
- infra adapter：具体数据库实现；
- UoW：组织同一业务事务中的写入；
- Outbox：在业务事务内登记可靠事件意图；
- Redis：缓存、信令、限流或租约，不自动成为主事实源。

## 3. 证据

从目标模块 `port`/repository interface 追到 `infra/mysql`、`infra/mongo`、cache adapter 与 container wiring；migration 只证明 schema 演进，不证明表可安全删除。

## 4. 验证

运行数据访问、UoW/outbox 和 retired-table architecture tests，并覆盖 repository 的并发、幂等和失败路径。
