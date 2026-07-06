# Assessment Model

**本文回答**：Assessment Model 如何作为统一测评模型资产层，吸收旧 `scale/personalitymodel`，为 Evaluation 和报告层提供可发布、可追溯的模型快照。

---

## 1. 这个模块负责什么

Assessment Model 负责“用什么模型规则解释答卷”：

- `AssessmentKind`：测评模型类型。
- `AssessmentModel`：模型资产定义。
- `AssessmentModelSnapshot`：发布后冻结的模型快照。
- `ModelBinding`：模型与问卷、入口或解释资产的绑定。
- `ModelPayload`：医学量表、人格模型、行为能力测评等模型资产 payload。
- 旧 `scale/personalitymodel` 的兼容注册和资产归并。

---

## 2. 这个模块不负责什么

- 不保存用户答卷事实。
- 不执行一次测评状态机。
- 不生成最终报告实例。
- 不调度周期任务。
- 不维护统计读模型。

---

## 3. 核心领域模型

| 模型 | 含义 | 深讲 |
| ---- | ---- | ---- |
| `AssessmentModel` | 模型资产聚合 | [02-领域模型.md](./02-领域模型.md) |
| `AssessmentKind` | 模型分类和执行识别 | [03-测评模型创建链路.md](./03-测评模型创建链路.md) |
| `AssessmentModelSnapshot` | 发布态冻结快照 | [04-模型发布与快照链路.md](./04-模型发布与快照链路.md) |
| `ModelBinding` | 模型和问卷 / 解释模型的绑定 | [05-模型绑定与适配机制.md](./05-模型绑定与适配机制.md) |

---

## 4. 关键业务链路

| 链路 | 文档 |
| ---- | ---- |
| 创建模型资产、配置 Kind 和 Payload | [03-测评模型创建链路.md](./03-测评模型创建链路.md) |
| 发布版本、生成快照、冻结执行输入 | [04-模型发布与快照链路.md](./04-模型发布与快照链路.md) |
| 绑定问卷、执行器和解释模型 | [05-模型绑定与适配机制.md](./05-模型绑定与适配机制.md) |
| 处理模型版本和旧能力兼容 | [06-模型版本与兼容策略.md](./06-模型版本与兼容策略.md) |
| 新增测评模型 | [07-扩展新测评模型SOP.md](./07-扩展新测评模型SOP.md) |

---

## 5. 上下游依赖

| 方向 | 模块 | 关系 |
| ---- | ---- | ---- |
| 上游 | 管理后台 / 模型配置 | 创建、维护和发布模型资产 |
| 下游 | `evaluation` | 加载快照并执行测评 |
| 下游 | `interpretation` | 使用模型身份和解释绑定生成报告 |

---

## 6. 端到端支持矩阵（以源码为准）

| Kind / 模型族 | 发布 | Input Provider | Evaluator | Report Builder | 说明 |
| ------------- | ---- | -------------- | --------- | -------------- | ---- |
| `scale` | 是 | 是 | 是 | 是 | 医学量表主链路 |
| `personality/typology`（MBTI/SBTI/BigFive/配置化） | 是 | 是 | 是 | 是 | configured runtime 主路径 |
| `behavior_ability`（domain: `behavioral_rating`） | 是 | 否（scale legacy binding） | 否（scale legacy binding） | 否 | API 名 behavior_ability；走医学量表执行链，非独立 behavioral_rating runtime |
| `behavioral_rating`（预留） | 否 | 否 | 否 | 否 | 未来独立行为评分 runtime，当前未启用 |
| `cognitive` | 部分（enum） | 否 | 否 | 否 | 预留，UI 禁用 |
| `custom` | 部分（enum） | 否 | 否 | 否 | 预留，UI 禁用 |

legacy `adapter/{mbti,sbti,bigfive}` 仅服务表征测试，非生产执行路径。

代码事实入口：

- [`internal/apiserver/domain/modelcatalog`](../../../internal/apiserver/domain/modelcatalog/)
- [`internal/apiserver/container/modules/modelcatalog`](../../../internal/apiserver/container/modules/modelcatalog/)
