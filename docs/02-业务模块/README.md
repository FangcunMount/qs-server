# 业务模块（02）

**本文回答**：`qs-server` 的业务模块应该按什么顺序阅读，当前业务名称和代码包名如何对应，旧模块文档迁移后从哪里进入。

本文只做入口导航和事实源说明。模块边界、全链路、事件和术语分别由本目录下的 `00-04` 文档承载。

---

## 1. 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| 组织方式 | 按业务理解路径组织，不按代码包、接口清单或表结构组织 |
| 核心阅读顺序 | `10-survey -> 20-model-catalog -> 30-evaluation -> 40-interpretation` |
| 支撑阅读顺序 | `50-actor -> 60-plan -> 70-statistics` |
| 代码注册事实 | 以 [`internal/apiserver/container/modules/registry.go`](../../internal/apiserver/container/modules/registry.go) 的 `BusinessPackages` 为准 |
| 代码包名 | 当前注册包是 `survey / modelcatalog / evaluation / interpretation / actor / plan / statistics` |
| 模型目录边界 | `modelcatalog` 是唯一模型资产模块；collection 的 `typology-models` 仅是业务 BFF facade |
| 旧文档处理 | `_archive` 只是重建期迁移输入；所有模块完成并验证后统一删除，不作为现行文档依赖 |

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
| 20 | Assessment Model | 测评模型资产层 | [20-model-catalog/README.md](./20-model-catalog/README.md) |
| 30 | Evaluation | 测评执行层 | [30-evaluation/README.md](./30-evaluation/README.md) |
| 40 | Interpretation Model / Report | 解释模型与报告产出层 | [40-interpretation/README.md](./40-interpretation/README.md) |
| 50 | Actor | 业务参与者上下文 | [50-actor/README.md](./50-actor/README.md) |
| 60 | Plan | 测评计划与任务编排 | [60-plan/README.md](./60-plan/README.md) |
| 70 | Statistics | 读侧统计与行为投影 | [70-statistics/README.md](./70-statistics/README.md) |

---

## 4. 命名映射

| 文档业务名称 | 当前代码包名 / 注册名 | 说明 |
| ------------ | --------------------- | ---- |
| `survey` | `survey` | 问卷定义、题目结构、答卷提交和 `AnswerSheet` 事实 |
| `model-catalog` | `modelcatalog` | 统一测评模型资产层，四类 canonical identity 共享 `DefinitionV2` 主路径 |
| `evaluation` | `evaluation` | 一次测评执行、执行状态、计分和结果生成 |
| `interpretation` | `interpretation` | 解释模型、报告模型、builder、adapter、`InterpretReport` 输出 |
| `actor` | `actor` | 受试者、从业者、操作者、访问上下文 |
| `plan` | `plan` | 测评计划、周期任务、任务生命周期 |
| `statistics` | `statistics` | 读侧统计、行为投影、指标聚合 |

`interpretation` 的文档名和当前注册名一致。注册事实必须从 `registry.go` 校验，不得沿用归档文档中的 `report module` 旧称。

---

## 5. 模块文档统一写法

每个模块 README 只做模块边界声明和阅读地图，必须回答：

1. 这个模块负责什么。
2. 这个模块不负责什么。
3. 这个模块有哪些核心领域模型。
4. 这个模块参与哪些关键业务链路。
5. 它依赖哪些模块，又被哪些模块依赖。

深度文档按语义编号：

| 编号段 | 内容 | 必须讲清的问题 |
| ------ | ---- | ---------------- |
| `10-19` | 领域模型 | 聚合边界、实体、值对象、不变式、生命周期和领域事件 |
| `20-29` | 领域规格、服务与扩展协议 | 规则为什么存在、如何约束聚合、与 application service 的边界 |
| `30-69` | 关键路径 | 触发入口、transport、application、domain、repository/transaction/outbox、失败语义、幂等和测试 |
| `80-89` | 模块边界 | 上下游合作、允许与禁止的依赖、跨模块编排位置 |
| `90-99` | 代码索引与验证 | 分层落点、组合根、契约、配置和定向测试命令 |

领域模型文档不能只列结构体字段；关键路径文档不能只列 handler/service 名称。每个结论都要回链到当前源码、机器契约或测试。核心模块可拆多个模型与关键路径文档，不为形式统一压缩为单文件。

---

## 6. 当前重建状态

| 模块 | 状态 | 说明 |
| ---- | ---- | ---- |
| `10-survey` | 已按新规范重建 | 已收口为 1 个入口 + 5 篇 canonical 文档：领域模型、两项核心设计和两条关键链路 |
| `20-model-catalog` | 已按新规范重建 | 已收口为 1 个入口 + 6 篇 canonical 文档：领域模型、Definition/身份/存储设计和发布/运行时链路 |
| `30-evaluation` | 已按新规范重建 | 已收口为 1 个入口 + 6 篇 canonical 文档：领域模型、运行时/可靠提交/Outcome 存储设计和两条关键链路 |
| `40-interpretation` 至 `70-statistics` | 待逐模块审核 | 现有内容仍需对照最新源码，不以占位文件数量判定完成 |

每完成一个模块，就删除被其替代的 active 占位文档并修正引用；只在全部模块完成且通过链接检查后，才统一删除 `_archive`。

---

## 7. Verify

```bash
make docs-hygiene
git diff --check
```
