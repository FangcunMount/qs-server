# Interpretation Model / Report

**本文回答**：Interpretation Model 如何把 Evaluation 的结构化结果转换成用户可理解的 `InterpretReport`。当前代码实现仍位于 `interpretation` module。

---

## 1. 这个模块负责什么

Interpretation Model / Report 负责“测评结果如何被解释成报告”：

- 解释模型和报告模型。
- `ReportGeneration`、`InterpretationRun` 与 `InterpretReport` 三对象模型。
- Report Builder Registry。
- Score-based adapter。
- Personality adapter。
- 解释文案、建议、风险提示。
- 报告聚合与持久化。
- 独立的报告生成状态、失败与重试。

---

## 2. 这个模块不负责什么

- 不提交答卷。
- 不执行测评状态机。
- 不修改或保存 Evaluation 的 Assessment 聚合。
- 不发布模型资产。
- 不调度周期任务。
- 不维护统计读模型。

一句话：**Interpretation 消费已经成立的 EvaluationOutcome 并生成 Report；报告成败不能改写评估事实。**

已落地的边界：Assessment 保持 `evaluated`，Report 独立进入 `generated / failed`；客户端 `completed` 或兼容 `interpreted` 由跨模块读模型派生。Interpretation 不再持有 Assessment Repository，`Assessment.ApplyOutcome` 与旧 reporting Writer 已删除。

---

## 3. 核心领域模型

| 模型 | 含义 | 深讲 |
| ---- | ---- | ---- |
| `InterpretationModel` | 解释规则资产 | [03-解释模型设计.md](./03-解释模型设计.md) |
| `ReportTemplate` | 报告结构和 section 模板 | [05-解释适配器与模板机制.md](./05-解释适配器与模板机制.md) |
| `ReportGeneration` / `InterpretationRun` / `InterpretReport` | 生成意图、执行尝试、最终成品 | [02-领域模型.md](./02-领域模型.md) |
| `ReportBuilder` | 报告构建适配能力 | [05-解释适配器与模板机制.md](./05-解释适配器与模板机制.md) |

---

## 4. 关键业务链路

| 链路 | 文档 |
| ---- | ---- |
| 设计解释规则、建议和模板 | [03-解释模型设计.md](./03-解释模型设计.md) |
| 消费执行结果并生成报告 | [04-报告生成链路.md](./04-报告生成链路.md) |
| Builder、adapter、template 扩展 | [05-解释适配器与模板机制.md](./05-解释适配器与模板机制.md) |
| 报告版本和可追溯性 | [06-报告版本与可追溯性.md](./06-报告版本与可追溯性.md) |
| 新增报告模型 | [07-扩展新报告模型SOP.md](./07-扩展新报告模型SOP.md) |
| 设计终局与重构判据 | [08-设计终局.md](./08-设计终局.md) |

---

## 5. 上下游依赖

| 方向 | 模块 | 关系 |
| ---- | ---- | ---- |
| 上游 | `evaluation` | 提供结构化执行结果 |
| 上游 | `model-catalog` | 提供模型身份、快照和解释绑定 |
| 下游 | `statistics` | 消费 `interpretation.report.generated` 和报告行为投影 |
| 下游 | 查询端 | 读取报告状态和内容 |

代码事实入口：

- [`internal/apiserver/domain/interpretation`](../../../internal/apiserver/domain/interpretation/)
- [`internal/apiserver/container/modules/interpretation`](../../../internal/apiserver/container/modules/interpretation/)
