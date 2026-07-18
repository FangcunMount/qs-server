# Plan

**本文回答**：Plan 如何负责测评计划、周期任务和任务生命周期编排，并与 Survey、Evaluation 协作。

---

## 1. 这个模块负责什么

Plan 负责“什么时候、对谁、开放什么测评任务”：

- `AssessmentPlan`：测评计划。
- `AssessmentTask`：计划拆出的应测任务。
- `PlanScheduleRule`：周期和触发规则。
- 任务开放、完成、过期、取消。
- 任务入口、通知和跨模块引用。

---

## 2. 这个模块不负责什么

- 不定义问卷题目。
- 不执行计分。
- 不生成报告。
- 不维护统计读模型。

一句话：**Plan 只负责任务生命周期，Evaluation 负责真正的测评执行。**

---

## 3. 核心领域模型

| 模型 | 含义 | 深讲 |
| ---- | ---- | ---- |
| `AssessmentPlan` | 测评计划 | [02-领域模型.md](./02-领域模型.md) |
| `AssessmentTask` | 应测任务 | [05-任务状态机.md](./05-任务状态机.md) |
| `PlanScheduleRule` | 周期规则 | [04-任务生成与周期调度链路.md](./04-任务生成与周期调度链路.md) |
| `TaskExecutionRef` | 与答卷、测评、报告的引用 | [06-计划与测评执行协作.md](./06-计划与测评执行协作.md) |

---

## 4. 关键业务链路

| 链路 | 文档 |
| ---- | ---- |
| 创建测评计划 | [03-测评计划创建链路.md](./03-测评计划创建链路.md) |
| 生成任务和周期调度 | [04-任务生成与周期调度链路.md](./04-任务生成与周期调度链路.md) |
| 任务状态机 | [05-任务状态机.md](./05-任务状态机.md) |
| 与 Survey / Evaluation 协作 | [06-计划与测评执行协作.md](./06-计划与测评执行协作.md) |
| 接口、事件和存储 | [07-接口事件与存储.md](./07-接口事件与存储.md) |

---

## 5. 上下游依赖

| 方向 | 模块 | 关系 |
| ---- | ---- | ---- |
| 上游 | `actor` | 提供受试者和操作者上下文 |
| 下游 | `survey` | 任务入口引导答卷提交 |
| 下游 | `evaluation` | 任务完成后关联测评执行 |
| 下游 | `statistics` | 消费任务事件形成计划指标 |

代码事实入口：

- [`internal/apiserver/domain/plan`](../../../internal/apiserver/domain/plan/)
- [`internal/apiserver/container/modules/plan`](../../../internal/apiserver/container/modules/plan/)
