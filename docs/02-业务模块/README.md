# 业务模块

本层按领域理解路径组织，不按接口、表或历史包名组织。

## 1. 阅读顺序

| 顺序 | 模块 | 核心问题 |
| --- | --- | --- |
| 10 | [Survey](./10-survey/README.md) | 用户提交了什么事实 |
| 20 | [ModelCatalog](./20-model-catalog/README.md) | 用什么发布模型解释与执行 |
| 30 | [Evaluation](./30-evaluation/README.md) | 一次测评如何执行并提交 Outcome |
| 40 | [Interpretation](./40-interpretation/README.md) | Outcome 如何变成可查询报告 |
| 50 | [Actor](./50-actor/README.md) | 谁参与、谁能访问 |
| 60 | [Plan](./60-plan/README.md) | 何时为谁安排测评任务 |
| 70 | [Statistics](./70-statistics/README.md) | 如何从事实构建读侧统计 |

## 2. 当前边界

业务模块清单以 [`registry.go`](../../internal/apiserver/container/modules/registry.go) 为准。`platform` 和 `iam` 是组合/集成包，不作为业务主线文档维护。

## 3. 现行深度

Survey、ModelCatalog、Evaluation、Interpretation、Actor 已保留独立的领域模型、核心设计、关键路径和设计问题文档。Plan 正按“领域模型—核心设计—关键链路—重构清单”拆分，README 与领域模型已经完成；Statistics 仍先保留可维护的模块入口。后续深拆必须以当前代码复核为前提，不能把归档模板直接搬回。

## 4. 跨模块原则

- 上游通过显式 port、事件或只读快照协作，不共享可变聚合。
- 草稿模型不得进入运行时；Evaluation 读取已发布快照。
- Evaluation 提交 Outcome 后不再推进报告状态；Interpretation 独立拥有报告生命周期。
- Statistics 是读侧投影，不是主业务事实源。
