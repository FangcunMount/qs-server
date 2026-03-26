# qs-server one-pager

**本文回答**：这篇文档负责把 `qs-server` 压成一页可口述的结构化概览，先帮助读者在一屏内抓住系统目标、运行时形态、主链路、领域边界、存储策略和当前最值得主动说明的风险。

## 30 秒结论

如果只看一屏，先看下面这张表：

| 维度 | 结论 |
| ---- | ---- |
| 系统本质 | 围绕问卷 / 量表 / 测评结果构建的后端系统，不是单纯答案存储服务 |
| 运行时形态 | `collection-server` + `qs-apiserver` + `qs-worker` 三进程协作 |
| 主业务中心 | `qs-apiserver` 持有领域模型、主状态、gRPC 和事件发布 |
| 主链路骨架 | 提交答卷同步受理，评估和报告异步推进 |
| 最该记住的边界 | `survey` 管采集，`scale` 管规则，`evaluation` 管测评状态和结果 |
| 最该主动补的一句 | 当前最需要如实说明的风险是“答卷持久业务幂等不足”和“写库后发事件无 outbox” |

## 一页结论

| 维度 | 结论 | 证据 |
| ---- | ---- | ---- |
| 系统目标 | 把问卷/量表场景中的答卷提交，稳定推进到测评结果、风险等级、报告与下游标签 | [../README.md](../README.md)、[../02-业务模块/03-evaluation.md](../02-业务模块/03-evaluation.md) |
| 运行时形态 | 三个核心进程协作：`collection-server`、`qs-apiserver`、`qs-worker` | [../README.md](../README.md)、[../01-运行时](../01-运行时/) |
| 主业务中心 | `qs-apiserver` 承担领域模型、主 REST、gRPC、事件发布与评估引擎；另外两个进程分别做 BFF 和异步驱动器 | [../README.md](../README.md)、[06-关键决策卡.md](./06-关键决策卡.md) |
| 主链路 | `POST /api/v1/answersheets -> SaveAnswerSheet -> answersheet.submitted -> CreateAssessmentFromAnswerSheet -> assessment.submitted -> EvaluateAssessment -> report.generated` | [03-主链路 1：提交答卷.md](./03-主链路%201：提交答卷.md)、[04-主链路 2：异步评估流水线.md](./04-主链路%202：异步评估流水线.md) |
| 核心边界 | `survey` 负责问卷/答卷，`scale` 负责量表规则，`evaluation` 负责测评状态、得分、报告 | [05-DDD 领域地图与模块协作.md](./05-DDD%20领域地图与模块协作.md) |
| 存储策略 | `Assessment` / `AssessmentScore` 走 MySQL；`AnswerSheet` / `InterpretReport` 走 MongoDB；缓存与锁用 Redis | [../02-业务模块/03-evaluation.md](../02-业务模块/03-evaluation.md)、[../03-基础设施](../03-基础设施/) |
| 工程难点 | 同步提交要短，异步评估要稳；既要解释 DDD 拆分，又要解释并发、幂等、重试、背压 | [07-工程治理与证据.md](./07-工程治理与证据.md) |
| 当前最值得主动讲的风险 | 答卷写库后发事件还没有 outbox；答卷提交本身没有基于业务键的持久业务幂等 | [03-主链路 1：提交答卷.md](./03-主链路%201：提交答卷.md) |

## 一页讲稿

`qs-server` 不是一个只保存问卷答案的系统，而是一个围绕测评业务做边界拆分和异步编排的后端项目。用户先通过 `collection-server` 提交答卷，`qs-apiserver` 保存答卷并发出 `answersheet.submitted`，`qs-worker` 再调用 internal gRPC 先算答卷分，再创建并提交测评，随后消费 `assessment.submitted` 触发评估引擎，最终生成分数、风险和报告。

项目的技术价值主要体现在三点。第一，`survey / scale / evaluation` 的边界清楚，答卷、量表规则和测评生命周期各自有独立模型。第二，同步入口和异步评估被明确拆开，请求线程不背整条重计算链。第三，仓库里已经有一套工程治理证据，包括 SubmitQueue、worker 并发控制、文档卫生校验和契约对比脚本，但也保留了几个应该诚实讲出的风险点。

## 宣讲时先抓哪三个点

1. `apiserver` 是主业务中心，`collection` 和 `worker` 都围绕它协作。
2. 主链路由两个事件把同步提交和异步评估解耦。
3. 幂等和一致性不是一句“MQ 保证”就结束，当前实现强弱分层很明显。

## 回链入口

- 项目定位：[01-项目定位与受众画像.md](./01-项目定位与受众画像.md)
- 主链路一：[03-主链路 1：提交答卷.md](./03-主链路%201：提交答卷.md)
- 主链路二：[04-主链路 2：异步评估流水线.md](./04-主链路%202：异步评估流水线.md)
- 决策卡：[06-关键决策卡.md](./06-关键决策卡.md)
