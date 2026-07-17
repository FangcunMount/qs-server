# observability

observability 模块是 qs-server 的运行证据层，用于证明 cache、event、concurrency 是否真的保护了高并发测评链路。

## 1. 这个模块解决什么问题

它解决“系统慢在哪里、事件卡在哪里、缓存是否命中、队列是否积压、报告为什么没出来”的定位问题。

## 2. 它在 qs-server 中处于什么位置

observability 横跨 HTTP、gRPC、cache、Outbox、MQ、worker、DB 和 report 查询链路。

## 3. 整体架构是什么

日志负责还原单次请求；指标负责持续监控；链路追踪负责串起跨进程调用；业务埋点负责把 answer_sheet_id、assessment_id、report_id、event_id、outbox_id 串起来。

## 4. 关键链路有哪些

| 链路 | 文档 |
| --- | --- |
| 整体架构 | [01-可观测性整体架构.md](01-可观测性整体架构.md) |
| 日志 | [02-日志设计.md](02-日志设计.md) |
| 指标 | [03-指标设计.md](03-指标设计.md) |
| 链路追踪 | [04-链路追踪设计.md](04-链路追踪设计.md) |
| 业务埋点 | [05-业务埋点与测评链路观测.md](05-业务埋点与测评链路观测.md) |
| 告警定位 | [06-告警与故障定位.md](06-告警与故障定位.md) |

## 5. 为什么选择当前方案

基础设施不是只看进程存活。qs-server 的关键风险是异步链路和读侧放大，所以观测必须围绕缓存命中、队列边界、Outbox 积压、worker 延迟和报告等待。

## 6. 代码事实源

- [../../../internal/pkg/redisruntime/observability](../../../internal/pkg/redisruntime/observability)
- [../../../internal/pkg/eventing/observe](../../../internal/pkg/eventing/observe)
- [../../../internal/pkg/resilience](../../../internal/pkg/resilience)
- [../../../internal/apiserver/application/systemgovernance](../../../internal/apiserver/application/systemgovernance)
- [../../../internal/worker/observability](../../../internal/worker/observability)
