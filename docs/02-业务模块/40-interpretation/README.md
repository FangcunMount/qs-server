# Interpretation Model / Report

**本文回答**：Interpretation Model 如何把 Evaluation 的结构化结果转换成用户可理解的 `InterpretReport`。当前代码实现仍位于 `interpretation` module。

---

## 1. 这个模块负责什么

Interpretation Model / Report 负责“测评结果如何被解释成报告”：

- 解释模型和报告模型。
- `InterpretReport` 聚合。
- Report Builder Registry。
- Score-based adapter。
- Personality adapter。
- 解释文案、建议、风险提示。
- 报告聚合与持久化。

---

## 2. 这个模块不负责什么

- 不提交答卷。
- 不执行测评状态机。
- 不发布模型资产。
- 不调度周期任务。
- 不维护统计读模型。

一句话：**InterpretationModel 是规则资产，InterpretReport 是报告实例。**

---

## 3. 核心领域模型

| 模型 | 含义 | 深讲 |
| ---- | ---- | ---- |
| `InterpretationModel` | 解释规则资产 | [03-解释模型设计.md](./03-解释模型设计.md) |
| `ReportTemplate` | 报告结构和 section 模板 | [05-解释适配器与模板机制.md](./05-解释适配器与模板机制.md) |
| `InterpretReport` | 最终报告实例 | [02-领域模型.md](./02-领域模型.md) |
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

---

## 5. 上下游依赖

| 方向 | 模块 | 关系 |
| ---- | ---- | ---- |
| 上游 | `evaluation` | 提供结构化执行结果 |
| 上游 | `model-catalog` | 提供模型身份、快照和解释绑定 |
| 下游 | `statistics` | 消费 `report.generated` 和报告行为投影 |
| 下游 | 查询端 | 读取报告状态和内容 |

代码事实入口：

- [`internal/apiserver/domain/interpretation`](../../../internal/apiserver/domain/interpretation/)
- [`internal/apiserver/container/modules/interpretation`](../../../internal/apiserver/container/modules/interpretation/)
