# MySQL 访问模式

## 1. 解决什么问题

MySQL 访问模式解决关系型数据、状态流转、列表查询、统计读模型和后台管理查询问题。

## 2. 所在位置

MySQL 主要承接 Assessment、Plan、Task、Staff、ScreeningProject、Statistics 等关系型数据。

## 3. 设计目标

索引贴合查询；事务边界小；状态变更清晰；读模型服务高频列表和统计。

## 4. 正常流程

应用服务通过 repository 写入关系数据；列表和统计查询使用读模型或聚合表；慢查询通过索引和查询形态优化。

## 5. 异常流程

事务失败整体回滚；死锁或锁等待超时需要重试或缩短事务；慢查询不能通过加缓存掩盖写模型问题。

## 6. 观测指标

query latency、slow query、lock wait、rows examined、connection pool usage、transaction duration。

## 7. 代码事实源

- [../../../internal/apiserver/infra/mysql](../../../internal/apiserver/infra/mysql)
- [../../../internal/pkg/database/mysql](../../../internal/pkg/database/mysql)
- [../../../internal/pkg/migration/migrations/mysql](../../../internal/pkg/migration/migrations/mysql)
