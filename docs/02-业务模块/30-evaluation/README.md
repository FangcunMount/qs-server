# Evaluation 模块

> 状态：已实现。本文只做模块边界和阅读地图，详细规则以下方 6 篇 canonical 文档为准。

## 1. 30 秒结论

Evaluation 把已提交答卷和已发布模型执行为可追溯的评分事实：

```text
AnswerSheet + Published Model
  -> Assessment            一次测评业务实例
  -> EvaluationRun         一次可 claim 的执行尝试
  -> Execution             进程内计算结果
  -> EvaluationOutcome     不可变持久化事实
  -> score projection + evaluation.outcome.committed
```

`Assessment=evaluated` 表示评分事实已可靠提交，不表示报告已生成。Interpretation 只通过 Outcome ID 和只读事实契约生成报告，不回写 Evaluation 状态。

## 2. 模块边界

| Evaluation 负责 | Evaluation 不负责 |
| --- | --- |
| Assessment 生命周期与业务引用 | Questionnaire、Question 和 AnswerSheet 内容 |
| EvaluationRun 尝试、claim、lease、失败与重试 | AssessmentModel 草稿、Definition 编辑和发布 |
| 执行输入解析、运行时路由和计算机制 | 报告模板、文案、Report 和 InterpretationRun |
| canonical Outcome、Assessment 摘要与 score 查询投影 | Plan 调度、Statistics 聚合和工作台组合读模型 |
| `evaluation.requested / evaluation.outcome.committed / evaluation.failed` 可靠事件 | 客户端 `completed / interpreted` 组合进度 |

必须区分：

- AnswerSheet 是 Survey 的作答事实，Assessment 是 Evaluation 的测评实例。
- Execution 是尚未提交的进程内结果，EvaluationOutcome Record 才是下游可依赖的事实。
- `assessment_score` 是 Outcome 的查询投影，不是所有算法的统一事实模型。
- 报告失败不得把已 `evaluated` 的 Assessment 改为 `failed`。

## 3. 文档地图

| 顺序 | 文档 | 核心问题 |
| --- | --- | --- |
| 10 | [领域模型](./10-领域模型.md) | Assessment、EvaluationRun、Execution 和 Outcome 为什么必须拆分 |
| 20 | [核心设计：执行身份与运行时扩展](./20-核心设计-执行身份与运行时扩展.md) | InputProvider、ModelRoute、Descriptor 和四类计算机制如何联合扩展 |
| 21 | [核心设计：状态、幂等与可靠提交](./21-核心设计-状态幂等与可靠提交.md) | 双状态机、Run claim/lease、失败重试和 MySQL 事务保护什么 |
| 22 | [核心设计：评估事实与数据存储](./22-核心设计-评估事实与数据存储.md) | schema v2 Outcome、冻结 ReportInput、投影和四类主表如何分工 |
| 30 | [关键链路：答卷入站与测评请求](./30-关键链路-答卷入站与测评请求.md) | answersheet.submitted 如何幂等创建 Assessment 并发出 evaluation.requested |
| 31 | [关键链路：Worker 执行与报告驱动](./31-关键链路-Worker执行与报告驱动.md) | Worker 如何解析输入、执行算法、提交 Outcome 并驱动 Interpretation |

扩展新模型或算法时先看 `20`；排查重复执行、长时运行或失败重试时看 `21`；排查数据或报告输入时看 `22`；跟踪生产主链时直接进入 `30` 或 `31`。

## 4. 事实源与验证

| 主题 | 事实源 |
| --- | --- |
| Domain | [`internal/apiserver/domain/evaluation`](../../../internal/apiserver/domain/evaluation/) |
| Application / runtime | [`internal/apiserver/application/evaluation`](../../../internal/apiserver/application/evaluation/) |
| Input / fact ports | [`port/evaluationinput`](../../../internal/apiserver/port/evaluationinput/)、[`port/evaluationfact`](../../../internal/apiserver/port/evaluationfact/) |
| MySQL | [`infra/mysql/evaluation`](../../../internal/apiserver/infra/mysql/evaluation/)、[`infra/mysql/checkpoint`](../../../internal/apiserver/infra/mysql/checkpoint/) |
| Composition root | [`container/modules/evaluation`](../../../internal/apiserver/container/modules/evaluation/) |
| Event / transport | [`configs/events.yaml`](../../../configs/events.yaml)、[`evaluation.proto`](../../../api/grpc/proto/evaluation/evaluation.proto) |

```bash
go test ./internal/apiserver/domain/evaluation/...
go test ./internal/apiserver/application/evaluation/...
go test ./internal/apiserver/infra/mysql/evaluation ./internal/apiserver/infra/mysql/checkpoint
go test ./internal/apiserver/container/modules/evaluation/...
make docs-hygiene
```
