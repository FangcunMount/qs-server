# Evaluation

**本文回答**：Evaluation 如何作为测评执行层，把 `AnswerSheet` 和模型快照结合，完成一次测评执行并生成结构化结果。

---

## 1. 这个模块负责什么

Evaluation 负责“一次测评如何执行”：

- `Assessment`：一次具体测评行为。
- `EvaluationRun`：一次执行尝试和运行状态。
- `EvaluationResult`：结构化执行结果。
- 计分、因子计算、等级判定。
- canonical EvaluationOutcome 与 `assessment_score` 等评估事实。
- 执行状态机、失败、重试、幂等。
- `assessment.submitted`、`assessment.evaluated`、`assessment.failed` 等执行事件。

---

## 2. 这个模块不负责什么

- 不定义问卷和题目结构。
- 不维护模型资产草稿和发布。
- 不维护最终报告模板和解释文案。
- 不调度周期任务。
- 不维护统计读模型。

一句话：**Evaluation 只生成可信、可持久化的结构化测评结果，不生成最终解释报告，也不以报告是否成功来改写评估事实。**

已确认的目标边界：`Assessment` 归属 Evaluation，成功终态是 `evaluated`。面向客户端的 `interpreted / completed` 是 Assessment 与 Report 的组合投影，不是 Assessment 聚合状态。当前代码仍保留 `StatusInterpreted` 兼容路径，属于待迁移实现。

`EvaluationRun.succeeded` 表示 EvaluationOutcome 已可靠提交，不包含 Report 生成；生产目标态不保留 Evaluation 内联生成 Report 的同步模式。

---

## 3. 核心领域模型

| 模型 | 含义 | 深讲 |
| ---- | ---- | ---- |
| `Assessment` | 一次测评执行实例 | [02-领域模型.md](./02-领域模型.md) |
| `EvaluationRun` | 执行尝试和状态 | [05-状态机与失败重试.md](./05-状态机与失败重试.md) |
| `EvaluationResult` | 总分、等级、因子分和原始结果 | [04-计分与因子计算链路.md](./04-计分与因子计算链路.md) |
| `EvaluationError` | 失败原因和重试语义 | [05-状态机与失败重试.md](./05-状态机与失败重试.md) |

---

## 4. 关键业务链路

| 链路 | 文档 |
| ---- | ---- |
| 消费答卷事件并创建测评 | [03-测评执行链路.md](./03-测评执行链路.md) |
| 计分、因子、等级判定 | [04-计分与因子计算链路.md](./04-计分与因子计算链路.md) |
| Assessment 与 EvaluationRun 双状态机 | [05-状态机与失败重试.md](./05-状态机与失败重试.md) |
| 幂等和一致性 | [06-幂等与一致性设计.md](./06-幂等与一致性设计.md) |
| 接口、事件和存储 | [07-接口事件与存储.md](./07-接口事件与存储.md) |

---

## 5. 上下游依赖

| 方向 | 模块 | 关系 |
| ---- | ---- | ---- |
| 上游 | `survey` | 提供 `AnswerSheet` |
| 上游 | `model-catalog` | 提供模型快照和执行 payload |
| 下游 | `interpretation` | 消费结构化结果生成报告 |
| 下游 | `statistics` | 消费执行事件和服务过程投影 |

代码事实入口：

- [`internal/apiserver/domain/evaluation`](../../../internal/apiserver/domain/evaluation/)
- [`internal/apiserver/container/modules/evaluation`](../../../internal/apiserver/container/modules/evaluation/)
