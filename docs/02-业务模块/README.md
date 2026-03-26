# 业务模块（02）

本目录按**限界上下文**拆分说明 `qs-apiserver` 内六大领域模块：职责、模型、核心设计与代码锚点。各篇结构对齐 [CONTRIBUTING-DOCS.md](../CONTRIBUTING-DOCS.md) 中的**业务模块推荐结构**（文首约定、`30 秒`、边界、模型与服务、核心设计、边界与索引）。

**前提**：模块均运行在 **qs-apiserver** 进程内（非独立微服务）；与 **collection-server** / **qs-worker** 的分工见 [00-总览/01-系统地图.md](../00-总览/01-系统地图.md)、[01-运行时/](../01-运行时/)。

---

## 文档列表与推荐阅读顺序

| 顺序 | 文档 | 模块 | 一句话 |
| ---- | ---- | ---- | ------ |
| 1 | [01-survey](./01-survey.md) | Survey | 问卷/答卷采集、校验与生命周期事件 |
| 2 | [02-scale](./02-scale.md) | Scale | 量表、因子与计分/解读规则 |
| 3 | [03-evaluation](./03-evaluation.md) | Evaluation | 测评状态机、评估引擎、报告与 `assessment.*` / `report.*` |
| 4 | [05-actor](./05-actor.md) | Actor | 受试者等参与者与相关能力 |
| 5 | [04-plan](./04-plan.md) | Plan | 测评计划与任务生命周期 |
| 6 | [06-statistics](./06-statistics.md) | Statistics | 统计聚合与同步 |

先读 **survey → scale → evaluation** 可最快对齐「采集 / 规则 / 产出」三界；**actor**、**plan**、**statistics** 可按业务关心顺序选读。

---

## 与周边文档的关系

| 方向 | 文档 |
| ---- | ---- |
| 总览与主链路 | [00-总览/03-核心业务链路.md](../00-总览/03-核心业务链路.md)、[05-专题分析/](../05-专题分析/) |
| 事件与契约 | [configs/events.yaml](../../configs/events.yaml)、[03-基础设施/01-事件系统.md](../03-基础设施/01-事件系统.md) |
| REST / gRPC | [api/rest/](../../api/rest/)、[internal/apiserver/interface/grpc/proto/](../../internal/apiserver/interface/grpc/proto/) |
| 装配入口 | [internal/apiserver/container/](../../internal/apiserver/container/)、各 [assembler/](../../internal/apiserver/container/assembler/) |

维护模块文档时以**源码与上述契约为准**；变更领域行为后应同步核对本篇与 `events.yaml` / OpenAPI / proto。
