# 03-基础设施

qs-server 的基础设施文档围绕高并发测评链路阅读，不再按 Redis、MQ、限流等组件平均展开。核心问题只有三个：

1. 高频读请求如何不打穿 MongoDB / MySQL。
2. 答卷提交后的异步测评、报告生成和状态通知如何可靠推进。
3. 突发提交、重复提交、下游积压和报告查询风暴如何被治理。

因此本目录的主线是：

| 主线 | 定位 | 优先阅读 |
| --- | --- | --- |
| cache | 读侧治理层，承接目录、模型、报告状态等高频查询 | [cache/README.md](cache/README.md) |
| event | 异步一致性治理层，承接领域事件、Outbox、MQ、一次性信令 | [event/README.md](event/README.md) |
| concurrency | 高并发保护层，承接入口限流、提交削峰、重复提交抑制、背压和 report 查询治理 | [concurrency/README.md](concurrency/README.md) |

observability、data-access、security、runtime 是支撑能力，服务于前三条主线，不抢阅读入口。

## 1. 怎么读

先读 [00-基础设施总览.md](00-基础设施总览.md) 建立系统位置，再读 [01-基础设施能力地图.md](01-基础设施能力地图.md) 看能力域边界，随后读 [02-基础设施设计原则.md](02-基础设施设计原则.md) 理解工程取舍，最后用 [03-核心链路全景.md](03-核心链路全景.md) 把答卷提交和报告查询两条链路串起来。

如果只关心面试或架构讲解，优先读 `cache / event / concurrency` 三个 README，再读 7 篇重点链路：

| 文档 | 价值 |
| --- | --- |
| [cache/01-缓存模块整体架构.md](cache/01-缓存模块整体架构.md) | 说明缓存是读侧治理，不是 Redis 封装 |
| [event/03-Outbox可靠出站链路.md](event/03-Outbox可靠出站链路.md) | 说明业务写入和事件发布的一致性边界 |
| [event/05-一次性信令链路.md](event/05-一次性信令链路.md) | 说明 Redis Pub/Sub 只做临时唤醒 |
| [concurrency/03-SubmitQueue提交削峰链路.md](concurrency/03-SubmitQueue提交削峰链路.md) | 说明提交洪峰如何变成有界处理能力 |
| [concurrency/04-SubmitGuard重复提交抑制链路.md](concurrency/04-SubmitGuard重复提交抑制链路.md) | 说明重复点击和客户端重试如何按业务幂等处理 |
| [concurrency/07-Report长轮询查询链路.md](concurrency/07-Report长轮询查询链路.md) | 说明 report 查询风暴如何被 wait + signal 治理 |
| [concurrency/09-容量边界与压测验证.md](concurrency/09-容量边界与压测验证.md) | 说明高并发保护如何用指标验证 |

## 2. 当前事实源

本目录只把现行源码、配置、OpenAPI 和运维文档作为事实源。旧组件目录已经归档到 [../_archive/2026-07-06-infra-legacy-component-docs/](../_archive/2026-07-06-infra-legacy-component-docs/)，只作历史参考，不参与 active truth layer。

关键事实源包括：

| 类型 | 事实源 |
| --- | --- |
| 事件契约 | [../../configs/events.yaml](../../configs/events.yaml)、[../../configs/signals.yaml](../../configs/signals.yaml) |
| 接口契约 | [../../api/rest/apiserver.yaml](../../api/rest/apiserver.yaml)、[../../api/rest/collection.yaml](../../api/rest/collection.yaml) |
| 运维验证 | [../04-接口与运维/11-300QPS混合场景压测SOP.md](../04-接口与运维/11-300QPS混合场景压测SOP.md)、[../04-接口与运维/12-小程序报告等待接入指南.md](../04-接口与运维/12-小程序报告等待接入指南.md) |

## 3. 维护规则

新增基础设施文档时先选择能力域，再选择链路文档。不要新增组件名目录；确实需要讲 Redis、MQ、DB、IAM、Nginx 时，必须放在对应能力域中解释它解决的系统问题。

关键链路文档统一包含：解决什么问题、所在位置、设计目标、整体流程、核心数据结构、正常流程、异常流程、幂等 / 降级 / 背压、可选方案、当前方案取舍、观测指标、代码事实源。
