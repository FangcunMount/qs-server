# 业务模块（02）

**本文回答**：`qs-server` 的业务模块应该按什么顺序阅读，当前业务名称和代码包名如何对应，旧模块文档迁移后从哪里进入。

本文只做入口导航和事实源说明。模块边界、全链路、事件和术语分别由本目录下的 `00-04` 文档承载。

---

## 1. 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| 组织方式 | 按业务理解路径组织，不按代码包、接口清单或表结构组织 |
| 核心阅读顺序 | `10-survey -> 20-assessment-model -> 30-evaluation -> 40-interpretation-model` |
| 支撑阅读顺序 | `50-actor -> 60-plan -> 70-statistics` |
| 代码注册事实 | 以 [`internal/apiserver/container/modules/registry.go`](../../internal/apiserver/container/modules/registry.go) 的 `BusinessPackages` 为准 |
| 代码包名 | 当前注册包是 `survey / assessmentmodel / evaluation / report / actor / plan / statistics` |
| 兼容注册名 | `assessmentmodel` 兼容注册 `scale`、`personalitymodel`；二者不再作为独立核心模块维护 |
| 旧文档处理 | 旧未编号目录已归档到 [`docs/_archive/2026-07-06-business-module-redesign`](../_archive/2026-07-06-business-module-redesign/) |

一句话概括：**qs-server 的测评业务由作答事实、模型资产、测评执行、解释报告四层核心链路组成，actor、plan、statistics 是围绕这条链路的支撑模块。**

---

## 2. 为什么不按代码包组织

业务模块文档的核心读者不是来查 API 列表或表结构的，而是要理解：

1. 这个模块解决什么业务问题。
2. 这个模块有哪些核心领域对象。
3. 这些对象之间是什么关系。
4. 这些对象在关键业务链路里如何协作。
5. 这个模块和其他模块的边界在哪里。

因此本目录采用领域理解路径：

```text
Survey
  -> Assessment Model
  -> Evaluation
  -> Interpretation Model / Report
  -> Actor
  -> Plan
  -> Statistics
```

代码包名仍是事实源，但不再决定文档阅读顺序。

---

## 3. 当前入口

| 阅读目标 | 入口 |
| -------- | ---- |
| 建立业务模块总图 | [00-业务模块总览.md](./00-业务模块总览.md) |
| 查模块职责、上下游和禁止反向依赖 | [01-模块边界与依赖关系.md](./01-模块边界与依赖关系.md) |
| 串起作答、执行、报告和统计 | [02-核心业务全链路.md](./02-核心业务全链路.md) |
| 查当前事件契约与投影方向 | [03-领域事件总览.md](./03-领域事件总览.md) |
| 统一术语和反例边界 | [04-术语表.md](./04-术语表.md) |

模块入口：

| 顺序 | 模块 | 当前定位 | 入口 |
| ---- | ---- | -------- | ---- |
| 10 | Survey | 作答事实层 | [10-survey/README.md](./10-survey/README.md) |
| 20 | Assessment Model | 测评模型资产层 | [20-assessment-model/README.md](./20-assessment-model/README.md) |
| 30 | Evaluation | 测评执行层 | [30-evaluation/README.md](./30-evaluation/README.md) |
| 40 | Interpretation Model / Report | 解释模型与报告产出层 | [40-interpretation-model/README.md](./40-interpretation-model/README.md) |
| 50 | Actor | 业务参与者上下文 | [50-actor/README.md](./50-actor/README.md) |
| 60 | Plan | 测评计划与任务编排 | [60-plan/README.md](./60-plan/README.md) |
| 70 | Statistics | 读侧统计与行为投影 | [70-statistics/README.md](./70-statistics/README.md) |

---

## 4. 命名映射

| 文档业务名称 | 当前代码包名 / 注册名 | 说明 |
| ------------ | --------------------- | ---- |
| `survey` | `survey` | 问卷定义、题目结构、答卷提交和 `AnswerSheet` 事实 |
| `assessment-model` | `assessmentmodel`，兼容注册 `scale/personalitymodel` | 统一测评模型资产层，旧 Scale 和 Personality Model 归入这一层 |
| `evaluation` | `evaluation` | 一次测评执行、执行状态、计分和结果生成 |
| `interpretation-model` | `report` | 解释模型、报告模型、builder、adapter、`InterpretReport` 输出 |
| `actor` | `actor` | 受试者、从业者、操作者、访问上下文 |
| `plan` | `plan` | 测评计划、周期任务、任务生命周期 |
| `statistics` | `statistics` | 读侧统计、行为投影、指标聚合 |

文档中的 `interpretation-model` 对应当前代码中的 `report module`。后续如果代码包重命名，应先调整容器注册、装配、测试和兼容策略，再同步文档。

---

## 5. 模块文档统一写法

每个模块 README 必须回答：

1. 这个模块负责什么。
2. 这个模块不负责什么。
3. 这个模块有哪些核心领域模型。
4. 这个模块参与哪些关键业务链路。
5. 它依赖哪些模块，又被哪些模块依赖。

每个模块的 `02-领域模型.md` 必须包含：

1. 模块核心概念。
2. 领域模型图。
3. 聚合根与实体。
4. 值对象。
5. 领域服务。
6. 领域事件。
7. 模型边界与反例。

模块文档可以引用接口、事件、存储和代码路径，但不能让这些实现细节替代领域模型和业务链路。

---

## 6. Verify

```bash
make docs-hygiene
git diff --check
```
