# data-access

data-access 模块是 qs-server 的数据访问支撑层，用于约束 MongoDB、MySQL、Repository、事务边界和读写模型。

## 1. 这个模块解决什么问题

它解决“哪些数据放 Mongo、哪些数据放 MySQL、跨库一致性如何处理、读模型如何避免拖累写模型”的问题。

## 2. 它在 qs-server 中处于什么位置

data-access 位于领域服务和数据库之间，为 Survey、Evaluation、Report、Plan、Statistics 等模块提供持久化端口和基础设施适配。

## 3. 整体架构是什么

领域服务依赖 port / repository 接口；infra/mongo 和 infra/mysql 实现访问；跨库事实推进依赖 Outbox、状态机和补偿，而不是分布式强事务。

## 4. 关键链路有哪些

| 链路 | 文档 |
| --- | --- |
| 整体架构 | [01-数据访问整体架构.md](01-数据访问整体架构.md) |
| Mongo 访问 | [02-Mongo访问模式.md](02-Mongo访问模式.md) |
| MySQL 访问 | [03-MySQL访问模式.md](03-MySQL访问模式.md) |
| 读写模型 | [04-读写模型分离.md](04-读写模型分离.md) |
| 事务边界 | [05-事务边界与一致性.md](05-事务边界与一致性.md) |
| Repository 取舍 | [06-Repository与DAO取舍.md](06-Repository与DAO取舍.md) |

## 5. 为什么选择当前方案

Mongo 适合 AnswerSheet、Questionnaire、InterpretReport 等文档型数据；MySQL 适合 Assessment、Plan、Task、Staff、ScreeningProject 等关系型数据。跨库一致性通过 Outbox 和状态补偿处理。

## 6. 代码事实源

- [../../../internal/apiserver/infra/mongo](../../../internal/apiserver/infra/mongo)
- [../../../internal/apiserver/infra/mysql](../../../internal/apiserver/infra/mysql)
- [../../../internal/apiserver/port](../../../internal/apiserver/port)
- [../../../internal/apiserver/container/internal/transaction](../../../internal/apiserver/container/internal/transaction)
