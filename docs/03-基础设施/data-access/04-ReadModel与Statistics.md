# Read Model 与 Statistics

**本文回答**：Statistics 与 journey projection 为什么属于读侧模型，如何与主业务聚合、行为投影、Query Cache 保持边界。

## 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| 解决问题 | 面向查询的统计聚合不能反向污染主业务聚合 |
| 核心设计 | read model repository + projector/reconcile + query cache |
| 当前边界 | 统计读模型可冗余字段，但不成为 Assessment/Plan/Testee 真值 |
| 相关文档 | 业务侧见 [statistics 深讲](../../02-业务模块/statistics/README.md)，Redis Query Cache 见 [redis/04](../redis/04-QueryCache与StaticList.md) |

## 主图

```mermaid
flowchart LR
    Events["Domain Events / Footprints"] --> Projector["Behavior Projector"]
    Projector --> ReadModel["Statistics Read Model"]
    ReadModel --> QueryRepo["Statistics Repository"]
    QueryRepo --> QueryCache["Query Cache"]
    QueryCache --> REST["REST Query"]
```

## 架构设计

Statistics read model 是查询优化层：

| 层 | 职责 |
| ---- | ---- |
| projector | 将事件/footprint 投影为查询结构 |
| repository | 提供读侧查询和聚合 |
| query cache | 缓存版本化查询结果 |
| REST handler | 只做请求参数与响应 DTO |

## 为什么这样设计

如果统计查询直接扫主业务表，复杂指标会把主写模型拖成报表模型；如果把统计结果回写业务聚合，又会让聚合不变量混入读侧需求。当前设计选择独立 read model，以换取查询性能和模型边界。

## 取舍与边界

- Read model 存在最终一致性窗口。
- Reconcile / pending 逻辑属于 behavior projection 专题，不写入主业务状态机。
- 统计口径变更必须同时更新 read model、cache、测试与文档，不允许只改 SQL。

## 代码锚点与测试锚点

| 能力 | 锚点 |
| ---- | ---- |
| Statistics MySQL repository | [repository.go](../../../internal/apiserver/infra/mysql/statistics/repository.go) |
| Journey read model | [read_model.go](../../../internal/apiserver/infra/mysql/statistics/readmodel/read_model.go) |
| Statistics business docs | [statistics/README.md](../../02-业务模块/statistics/README.md) |

## Verify

```bash
go test ./internal/apiserver/infra/mysql/statistics/... ./internal/apiserver/application/statistics/...
```
