# qs-server 文档中心

> 本文是 `docs/` 的根入口。
>
> 它只负责三件事：说明文档分层、给出稳定阅读入口、明确事实源优先级。
>
> 它不替代各目录 README，也不重复维护每个子模块的完整细节。

---

## 1. 30 秒结论

| 维度 | 结论 |
| --- | --- |
| 文档目标 | 先让读者知道从哪里进入，再进入具体目录 |
| 真值层 | `00-05` 是现行真值层，`06-宣讲` 是讲解层；历史材料只作参考，不作为当前事实源 |
| 事实优先级 | 源码与机器契约优先于 prose 文档 |
| 业务模块入口 | 统一从 [02-业务模块/README.md](./02-业务模块/README.md) 进入 |
| 当前业务模块 | 4 个核心模块：`survey / model-catalog / evaluation / report`；3 个支撑模块：`actor / plan / statistics` |
| 代码映射 | 当前注册包是 `survey / modelcatalog / evaluation / report / actor / plan / statistics` |
| 执行主线 | Survey 提供答卷事实，Assessment Model 提供发布模型资产，Evaluation 执行测评并产出结果，Interpretation Model / Report 输出最终解释报告 |
| 兼容说明 | `scale/personalitymodel` 是 `modelcatalog` 的旧注册名或具体模型资产入口，不再作为独立核心模块 |

---

## 2. 当前文档主线

qs-server 当前文档的主线是：

```text
00-总览        先理解系统地图、代码边界和核心链路
01-运行时      再理解 qs-apiserver / collection-server / qs-worker 如何协作
02-业务模块    再按编号路径理解 4 个核心模块 + 3 个支撑模块
03-基础设施    再理解缓存、事件、高并发保护，以及数据访问、安全、外部集成等支撑能力
04-接口与运维  再查看 REST / gRPC / 部署 / 运维入口
05-专题分析    最后理解关键设计判断和系统级权衡
06-宣讲        用于对外讲解、技术分享、面试表达
```

业务链路的核心表达是：

```text
Survey
    管问卷定义、题目结构、答卷提交和 AnswerSheet 事实

Assessment Model
    管测评模型资产：Kind / Snapshot / Binding / Payload
    Scale / MBTI / BigFive 等都是模型资产或执行插件

Evaluation
    管一次测评执行：Assessment / Outcome / Retry / Events
    （EvaluationRun 为演进方向，当前未实现）

Interpretation Model / Report
    管解释模型、报告 builder、adapter、InterpretReport 聚合与持久化

Actor / Plan / Statistics
    管参与者上下文、计划任务编排和读侧统计投影
```

---

## 3. 事实来源优先级

阅读或维护文档时，默认按下面的优先级判断真值：

```text
1. 源码与运行时行为
   internal/、cmd/、pkg/

2. 机器契约与配置
   api/rest/、api/grpc/gen/、configs/events.yaml、configs/*.yaml

3. docs/00-05 现行正文

4. docs/06-宣讲

5. 历史材料或 archive
```

如果 prose 文档与代码、契约冲突，以代码和契约为准。文档可以解释设计意图，但不能反向覆盖源码事实。

当前业务模块注册事实以 [`internal/apiserver/container/modules/registry.go`](../internal/apiserver/container/modules/registry.go) 为准。

---

## 4. 根目录地图

| 目录 | 解决什么问题 | 什么时候进入 |
| --- | --- | --- |
| [00-总览](./00-总览/) | 系统地图、代码边界、主链路、本地开发入口 | 第一次读仓库时 |
| [01-运行时](./01-运行时/) | 三进程职责、调用方向、运行时协作 | 需要确认谁调谁、怎么跑时 |
| [02-业务模块](./02-业务模块/) | 业务模块职责、模型、链路、状态机和事实源 | 需要看领域设计和业务实现时 |
| [03-基础设施](./03-基础设施/) | 缓存、事件、高并发保护，以及数据访问、安全、外部集成、运行时等支撑能力 | 需要看机制、配置和代码挂载点时 |
| [04-接口与运维](./04-接口与运维/) | REST / gRPC 契约、端口部署、调度任务、事故复盘 | 需要看机器契约和运维入口时 |
| [05-专题分析](./05-专题分析/) | 设计判断、边界拆分、异步评估、保护层 | 需要看系统级设计权衡时 |
| [06-宣讲](./06-宣讲/) | 对外讲解、技术分享、答辩材料 | 需要把项目讲清楚时 |

---

## 5. 推荐阅读入口

### 5.1 第一次读仓库

推荐顺序：

```text
00-总览/README.md
00-总览/01-系统地图.md
00-总览/02-代码组织与边界.md
00-总览/03-核心业务链路.md
01-运行时/README.md
02-业务模块/README.md
```

目标是先建立：

```text
系统有哪些进程；
核心链路如何流转；
代码按什么边界组织；
业务模块之间如何协作；
哪些目录是事实源。
```

### 5.2 需要理解业务模块

从这里进入：

```text
02-业务模块/README.md
```

建议顺序：

```text
10-survey
    先理解问卷定义和答卷提交事实

20-model-catalog
    再理解 Kind / Snapshot / Binding / Payload 抽象

30-evaluation
    再理解 Assessment / EvaluationRun / Result / Retry / Events

40-interpretation
    最后理解 report module 如何产出 InterpretReport

50-actor / 60-plan / 70-statistics
    按参与者、计划编排、读侧统计问题进入
```

旧 `scale/` 目录已经退出现行阅读路径。医学量表细节从 `20-model-catalog` 进入，历史材料只在 `docs/_archive/` 中保留。

### 5.3 需要看接口与运维

需要看接口契约：

```text
04-接口与运维/
api/rest/apiserver.yaml
api/rest/collection.yaml
api/grpc/gen
```

需要看运行部署：

```text
01-运行时/README.md
04-接口与运维/
cmd/qs-apiserver/apiserver.go
cmd/collection-server/main.go
cmd/qs-worker/main.go
```

需要看事件配置：

```text
configs/events.yaml
03-基础设施/event/README.md
```

---

## 6. 核心业务模块入口

| 模块 | 当前定位 | 入口 |
| ---- | -------- | ---- |
| Survey | 作答事实层，负责问卷定义、题目结构、答卷提交和 `AnswerSheet` 事实沉淀 | [02-业务模块/10-survey/README.md](./02-业务模块/10-survey/README.md) |
| Assessment Model | 测评模型资产层，负责统一管理医学量表、人格模型等模型资产 | [02-业务模块/20-model-catalog/README.md](./02-业务模块/20-model-catalog/README.md) |
| Evaluation | 测评执行层，负责将 `AnswerSheet` 与 Assessment Model 结合，完成一次 `Assessment` 执行并生成结果 | [02-业务模块/30-evaluation/README.md](./02-业务模块/30-evaluation/README.md) |
| Interpretation Model / Report | 解释模型层，负责报告构建、解释适配、`InterpretReport` 聚合与持久化；当前代码实现仍位于 `interpretation` module | [02-业务模块/40-interpretation/README.md](./02-业务模块/40-interpretation/README.md) |

支撑模块入口：

| 模块 | 当前定位 | 入口 |
| ---- | -------- | ---- |
| Actor | 参与者与访问上下文 | [02-业务模块/50-actor/README.md](./02-业务模块/50-actor/README.md) |
| Plan | 测评计划与周期任务编排 | [02-业务模块/60-plan/README.md](./02-业务模块/60-plan/README.md) |
| Statistics | 读侧统计、行为投影与指标聚合；文档名与代码包名保持 `statistics` | [02-业务模块/70-statistics/README.md](./02-业务模块/70-statistics/README.md) |

---

## 7. 基础设施入口

| 能力 | 入口 |
| ---- | ---- |
| 基础设施总览 | [03-基础设施/README.md](./03-基础设施/README.md)、[03-基础设施/00-基础设施总览.md](./03-基础设施/00-基础设施总览.md) |
| 能力地图 | [03-基础设施/01-基础设施能力地图.md](./03-基础设施/01-基础设施能力地图.md) |
| 缓存模块 | [03-基础设施/cache/README.md](./03-基础设施/cache/README.md) |
| 事件模块 | [03-基础设施/event/README.md](./03-基础设施/event/README.md)、`configs/events.yaml`、`configs/signals.yaml` |
| 高并发保护模块 | [03-基础设施/concurrency/README.md](./03-基础设施/concurrency/README.md) |
| Observability 支撑层 | [03-基础设施/observability/README.md](./03-基础设施/observability/README.md) |
| Data Access 支撑层 | [03-基础设施/data-access/README.md](./03-基础设施/data-access/README.md) |
| Security 支撑层 | [03-基础设施/security/README.md](./03-基础设施/security/README.md) |
| Runtime 支撑层 | [03-基础设施/runtime/README.md](./03-基础设施/runtime/README.md) |

---

## 8. 宣讲层入口

需要准备技术分享、面试表达或对外介绍时，从这里进入：

```text
06-宣讲/README.md
06-宣讲/09-30分钟技术分享脚本.md
06-宣讲/10-架构图素材索引.md
06-宣讲/11-面试追问证据索引.md
```

宣讲层回答“怎么把项目讲清楚”，不承担机器契约和实现真值。

---

## 9. 历史材料政策

历史材料统一放在 `docs/_archive/`，只能作为信息源或迁移参考，不能直接视为现行事实。

使用规则：

```text
现行文档默认不依赖历史材料；
从历史材料回迁内容前，必须重新核对源码和契约；
docs/_archive 默认排除在 active truth layer 和 hygiene 规则之外。
```

---

## 10. 维护入口

文档写作约定：

```text
CONTRIBUTING-DOCS.md
```

提交前校验：

```bash
make docs-hygiene
git diff --check
```

涉及接口契约时再执行：

```bash
make docs-verify
```

---

## 11. 最终原则

维护 qs-server 文档时，始终坚持三条原则：

```text
第一，源码和机器契约优先于 prose 文档；
第二，根 README 只做入口和导航，不重复维护二级目录细节；
第三，业务模块文档必须服务于当前代码事实和下一阶段演进，而不是停留在历史设计稿。
```

当前业务模块文档的核心演进方向是：

```text
Survey 保持作答事实层；
Assessment Model 收敛模型资产和发布快照；
Scale / MBTI / SBTI 作为模型资产与执行插件存在；
Evaluation 保持通用测评执行层；
Interpretation Model / Report 保持最终解释报告聚合。
```
