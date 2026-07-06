# Repository 与 DAO 取舍

## 1. 解决什么问题

Repository 与 DAO 取舍解决领域层是否暴露数据库细节、基础设施代码如何组织的问题。

## 2. 所在位置

Repository / port 位于应用服务和 infra 实现之间；DAO / store 位于 infra 内部，贴近数据库表或集合。

## 3. 设计目标

领域服务依赖业务语义接口；infra 内部保留高效查询；不要让领域层依赖 SQL / Mongo query 细节。

## 4. 正常流程

应用服务调用 repository；repository 组合 DAO / store 完成持久化；复杂读模型可以暴露独立 query port。

## 5. 异常流程

如果 repository 变成万能接口，需要按业务用例拆分；如果 DAO 泄漏到领域层，需要收回到 infra。

## 6. 观测指标

repository method latency、DAO error、query shape count、read model fallback、slow query。

## 7. 代码事实源

- [../../../internal/apiserver/port](../../../internal/apiserver/port)
- [../../../internal/apiserver/infra/mongo](../../../internal/apiserver/infra/mongo)
- [../../../internal/apiserver/infra/mysql](../../../internal/apiserver/infra/mysql)
