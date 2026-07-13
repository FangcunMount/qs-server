# Interpretation 模块

> 状态：已实现。本文只做模块边界和阅读地图，详细规则以下方 6 篇 canonical 文档为准。

## 1. 30 秒结论

Interpretation 把已可靠提交的 EvaluationOutcome 转换为可追溯、可授权查询的报告成品：

```text
EvaluationOutcome Record + frozen ReportInput
  -> InterpretationInput                 只读解释事实
  -> ReportGeneration                    幂等生成意图
  -> InterpretationRun                  一次可重试构建尝试
  -> Report Draft                        进程内内容
  -> InterpretReport                     不可变成品
  -> report_query_catalog                当前报告查询索引
```

Interpretation 不重新计分，不修改 Assessment、EvaluationRun 或 Outcome。报告生成失败只会改变 ReportGeneration / InterpretationRun，不能把已 `evaluated` 的 Assessment 改为 `failed`。

## 2. 模块边界

| Interpretation 负责 | Interpretation 不负责 |
| --- | --- |
| 把持久化 Outcome 和冻结 ReportInput 映射为解释输入 | 答卷校验、分数计算、等级判定 |
| ReportGeneration 幂等、Run lease、失败分类与重试 | Assessment / EvaluationRun 状态机 |
| 按完整机制键选择 Builder 和冻结模板版本 | ModelCatalog draft 编辑和 Published Model 发布 |
| 不可变 InterpretReport 及报告终态事件 | Statistics 聚合、Plan 调度、Actor 关系管理 |
| 受试者、临床人员、管理员和运维的报告查询用例 | IAM 身份、Assessment 归属和照护关系本身 |

必须区分：

- `EvaluationOutcome` 是评分事实；`InterpretReport` 是人可读解释成品。
- `ReportGeneration` 是聚合根；`InterpretReport` 是成功后才存在的不可变子成品。
- `Report Draft` 可用于预览或事务提交前；下游只能依赖已持久化成品。
- `report_query_catalog` 是读模型索引，不是第四个领域对象，也不保存报告正文。
- 客户端 `interpreted / completed` 是 Assessment 与报告存在性的组合投影，不是 Assessment 新状态。

## 3. 文档地图

| 顺序 | 文档 | 核心问题 |
| --- | --- | --- |
| 10 | [领域模型](./10-领域模型.md) | Generation、Run、Draft、Report 和查询投影为什么必须拆分 |
| 20 | [核心设计：冻结输入与报告渲染扩展](./20-核心设计-冻结输入与报告渲染扩展.md) | Outcome schema、ReportInput、机制键、Builder 和模板版本如何联合扩展 |
| 21 | [核心设计：生成幂等与可靠提交](./21-核心设计-生成幂等与可靠提交.md) | Generation key、Run lease、失败重试和 Mongo 事务保护什么 |
| 22 | [核心设计：报告数据存储与查询索引](./22-核心设计-报告数据存储与查询索引.md) | 5 个 Mongo collection、唯一约束、catalog 与历史兼容如何分工 |
| 30 | [关键链路：Outcome 驱动报告生成](./30-关键链路-Outcome驱动报告生成.md) | Worker 如何从 `evaluation.outcome.committed` 进入构建、提交与 ACK/NACK |
| 31 | [关键链路：多角色报告查询与状态投影](./31-关键链路-多角色报告查询与状态投影.md) | participant、clinician、admin、operations 如何授权并读取同一成品 |

扩展新报告机制先看 `20`；排查重复生成、长时运行或失败重试先看 `21`；排查 Mongo 数据、catalog 或历史报告先看 `22`；跟踪生产主链直接进入 `30` 或 `31`。

## 4. 事实源与验证

| 主题 | 事实源 |
| --- | --- |
| Domain | [`internal/apiserver/domain/interpretation`](../../../internal/apiserver/domain/interpretation/) |
| Application | [`internal/apiserver/application/interpretation`](../../../internal/apiserver/application/interpretation/) |
| Mongo | [`internal/apiserver/infra/mongo/interpretation`](../../../internal/apiserver/infra/mongo/interpretation/) |
| Query port | [`port/interpretationreadmodel`](../../../internal/apiserver/port/interpretationreadmodel/) |
| Composition root | [`container/modules/interpretation`](../../../internal/apiserver/container/modules/interpretation/) |
| Worker / gRPC | [`worker/handlers/assessment_evaluated_handler.go`](../../../internal/worker/handlers/assessment_evaluated_handler.go)、[`interpretation.proto`](../../../api/grpc/proto/interpretation/interpretation.proto) |
| Event catalog | [`configs/events.yaml`](../../../configs/events.yaml)、[`internal/pkg/eventing/catalog`](../../../internal/pkg/eventing/catalog/) |

```bash
go test ./internal/apiserver/domain/interpretation/...
go test ./internal/apiserver/application/interpretation/...
go test ./internal/apiserver/infra/mongo/interpretation ./internal/apiserver/port/interpretationreadmodel
go test ./internal/apiserver/container/modules/interpretation ./internal/apiserver/transport/grpc/service ./internal/worker/handlers
make docs-hygiene
```
