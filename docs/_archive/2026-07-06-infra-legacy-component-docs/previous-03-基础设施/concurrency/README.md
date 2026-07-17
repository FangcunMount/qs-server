# 高并发保护阅读地图

**本文回答**：qs-server 的高并发保护模块解决什么系统问题；入口限流、SubmitQueue、SubmitGuard、下游背压和 report 查询治理如何配合；现有 `resilience/` 细文档应该如何阅读。

---

## 30 秒结论

高并发保护不等于“加限流”。qs-server 的高并发重点分成三段：

1. **入口保护**：保护 collection-server 和 qs-apiserver，避免突发流量直接打穿业务服务。
2. **提交削峰与重复抑制**：答卷提交会触发 AnswerSheet 写入、Assessment 创建、Outbox 写入和 worker 异步执行，必须比普通查询更严格。
3. **查询治理与下游背压**：报告生成是异步的，前端会持续查询状态；如果不治理，report 查询会随在线用户数和报告生成耗时线性放大。

---

## 能力矩阵

| 能力 | 解决的问题 | 当前机制 | 继续阅读 |
| ---- | ---------- | -------- | -------- |
| 入口限流 | 单用户、脚本或整体入口洪峰 | HTTP limiter，超限返回 429 / Retry-After | [../resilience/01-RateLimit入口限流.md](../resilience/01-RateLimit入口限流.md) |
| SubmitQueue | 把瞬时答卷提交洪峰变成有界队列 | collection-server memory channel + worker pool，返回 202 + request_id | [../resilience/02-SubmitQueue提交削峰.md](../resilience/02-SubmitQueue提交削峰.md) |
| SubmitGuard | 防重复点击、客户端重试、网络抖动造成重复提交 | done marker + in-flight Redis lease | [../resilience/04-LockLease幂等与重复抑制.md](../resilience/04-LockLease幂等与重复抑制.md) |
| 下游背压 | worker、MQ、DB、gRPC 下游能力有限 | bounded semaphore、MaxInFlight、timeout、degraded outcome | [../resilience/03-Backpressure下游背压.md](../resilience/03-Backpressure下游背压.md) |
| Worker 重复抑制 | MQ 重投或并发消费导致重复副作用 | answersheet duplicate suppression + handler 幂等 | [../event/03-Worker消费与AckNack.md](../event/03-Worker消费与AckNack.md) |
| Report 查询治理 | 异步报告生成后的查询风暴 | report-status 短轮询、wait-report 长轮询、WebSocket / SSE 方向 | [../../04-接口与运维/12-小程序报告等待接入指南.md](../../04-接口与运维/12-小程序报告等待接入指南.md) |
| 压测容量 | 保护是否真的有效 | P95、错误率、report success、outbox 排水、worker 消费 | [../../04-接口与运维/11-300QPS混合场景压测SOP.md](../../04-接口与运维/11-300QPS混合场景压测SOP.md) |

---

## 入口保护

入口限流保护的是 collection-server 和 qs-apiserver 的入口，而不是业务正确性。

| 维度 | 说明 |
| ---- | ---- |
| 用户级限流 | 防止单个用户或脚本高频刷接口 |
| 接口级限流 | 不同接口不同阈值；提交接口比普通查询更敏感 |
| Nginx / 应用双层保护 | Nginx 挡连接和入口洪峰，应用层按业务语义限流 |
| 快速失败 | 超过阈值直接返回，避免所有请求进入后端排队 |

---

## SubmitQueue：提交削峰

答卷提交不是普通查询。一次提交会触发：

```text
答案校验
AnswerSheet 写入
Assessment 创建
Outbox 事件写入
worker 异步执行
报告状态推进
```

SubmitQueue 的职责是把“瞬时提交洪峰”变成 collection-server 内部的有界队列：

```text
POST answersheets
  -> 快速入队
  -> 返回 202 + request_id
  -> 后台 worker pool 调 apiserver
  -> 前端查 submit status / 后续 assessment
```

边界必须清楚：

- SubmitQueue 不是 MQ。
- SubmitQueue 不是 durable queue。
- SubmitQueue 不保证进程退出 drain。
- 业务事实可靠性仍由 apiserver durable submit、DB 约束和 Outbox 兜底。

---

## SubmitGuard：重复提交抑制

重复提交抑制不是简单限流，它解决的是短期业务幂等问题。

同一个 testee、questionnaire、assessment task 在短时间内重复提交时，系统应该识别为同一次业务动作，而不是生成多份答卷、多次测评、多份报告。

当前 SubmitGuard 由两部分组成：

| 组件 | 作用 |
| ---- | ---- |
| done marker | 已完成时复用 answerSheetID |
| in-flight lock | 处理中时返回进行中或抑制重复处理 |

最终正确性仍不能只靠 Redis lease；AnswerSheet durable submit、Assessment 状态机、唯一约束和事件幂等仍是兜底。

---

## 下游背压

背压负责在 worker、MQ、数据库、gRPC 下游处理能力不足时，让上游降速，而不是无限堆积。

| 下游 | 背压手段 |
| ---- | -------- |
| qs-apiserver | max-inflight、超时、提交队列 |
| MQ | 消费并发控制、积压监控、失败重试 |
| worker | worker pool、消费速率、Nack / retry、duplicate suppression |
| DB | 降低回源、缓存过滤、批量处理、bounded in-flight |
| report 查询 | `next_poll_after_ms`、长轮询 timeout、Redis 状态过滤、WS 连接上限 |

---

## Report 查询模式

报告生成是异步的，前端一定会查询状态；如果所有客户端固定间隔短轮询，请求量会随着在线用户数和报告生成耗时线性放大。

当前后端文档中同时存在三种感知模式：

| 模式 | 定位 | 优点 | 风险 | 当前阅读入口 |
| ---- | ---- | ---- | ---- | ------------ |
| 短轮询 `report-status` | 基础兼容路径 | 简单、稳定、兼容性好 | 请求量大，必须遵守 `next_poll_after_ms` | [小程序报告等待接入指南](../../04-接口与运维/12-小程序报告等待接入指南.md) |
| 长轮询 `wait-report` | legacy 兼容路径；解释一次性信令和连接治理边界 | 减少无效请求，有结果可立即返回 | 占用连接，需要 timeout 和独立并发池 | [小程序报告等待接入指南](../../04-接口与运维/12-小程序报告等待接入指南.md) |
| WebSocket / SSE | 后续或可选增强路径；当前 WS 特性开关可关闭 | 实时性最好，查询压力最低 | 连接管理、限流、运维成本更高 | `report_events.enabled`、`api/rest/collection.yaml` |

推荐接入策略以接口文档为准：优先 WS，失败则按 `next_poll_after_ms` 短轮询；`wait-report` 保留给旧版兼容，不应被新客户端紧循环调用。

---

## 压测和观测指标

| 目标 | 关注指标 |
| ---- | -------- |
| 入口是否被打穿 | HTTP failed rate、429、Retry-After、接口 p95 |
| SubmitQueue 是否过载 | queue depth、queue_full、queue_failed、status TTL |
| SubmitGuard 是否生效 | idempotency_hit、lock_contention、degraded_open |
| 下游是否积压 | backpressure_timeout、DB in-flight、gRPC timeout |
| Outbox 是否排水 | pending / publishing / failed、oldest age |
| Worker 是否消费正常 | MaxInFlight、handler duration、Ack/Nack、duplicate_skipped |
| Report 查询是否放大 | report_status_success_rate、wait-report timeout、WS 101、`next_poll_after_ms` 遵守情况 |

压测入口见 [11-300QPS混合场景压测SOP.md](../../04-接口与运维/11-300QPS混合场景压测SOP.md)。

---

## 代码事实源

| 主题 | 路径 |
| ---- | ---- |
| resilience model / metrics | `internal/pkg/resilience` |
| RateLimit | `internal/pkg/resilience/ratelimit`、collection/apiserver middleware |
| SubmitQueue | `internal/collection-server/application/answersheet/submit_queue.go` |
| SubmitGuard | `internal/collection-server/infra/redisops`、`internal/pkg/resilience/locklease` |
| Backpressure | `internal/pkg/resilience/backpressure`、`internal/pkg/resilience` |
| Worker duplicate suppression | `internal/worker/handlers`、`internal/pkg/resilience/locklease` |
| report status / wait-report | `api/rest/collection.yaml`、`internal/collection-server/application/reportwait` |
