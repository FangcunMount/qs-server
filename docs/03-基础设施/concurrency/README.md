# Concurrency / Resilience

## 1. 结论

qs-server 没有一个包办一切的“高并发组件”。现行实现把压力拆成六类问题：到达速率、单实例在途请求、下游依赖容量、重复业务意图、长任务所有权和运行时治理。

最重要的边界是：

- Gate、RateLimit、Backpressure 保护容量，不保证业务正确性；
- SubmitGuard 与 worker duplicate-suppression lease 是建议性保护，不是持久化事实；
- 答卷提交正确性由 Mongo 唯一约束、内容指纹和 AnswerSheet + Outbox 事务保证；
- Redis 可以改善效率、跨实例协调与控制面收敛，但 Redis 故障不能让服务伪造成功；
- 当前答卷提交链没有进程内 SubmitQueue。

状态：`已实现`，本文档以 2026-07-23 当前代码与生产配置为事实源。

## 2. 保护链

```mermaid
flowchart LR
    Client["客户端"] --> HTTPGate["collection 单实例 Gate"]
    HTTPGate --> Rate["全局 + 用户 RateLimit"]
    Rate --> App["collection application"]
    App --> Guard["Redis SubmitGuard<br/>建议性租约"]
    Guard --> GRPCGate["collection gRPC inflight Gate"]
    GRPCGate --> APIRate["apiserver 本地 RateLimit"]
    APIRate --> BP["MySQL / Mongo / IAM Backpressure"]
    BP --> Store["数据库连接池与持久化约束"]
```

这张图描述机制层次，不表示每条路由都会经过所有节点。提交路由的真实入口顺序是 `Submit Gate -> RateLimit`；普通查询、catalog、report-status 和 wait-report 使用不同 Gate 策略。

## 3. 机制速查

| 机制 | 控制量 | 作用范围 | 饱和或故障语义 |
| --- | --- | --- | --- |
| RateLimit | 单位时间允许量、burst | collection 可用 Redis 跨实例预算；apiserver 为单实例预算 | 超额 429；collection 的 Redis 限流后端运行期故障会 degraded-open |
| Gate | 在途请求数 | 单个 collection 进程 | submit 等待 50ms 后 429；多数查询 503；wait-report 可降级为 200 pending |
| gRPC inflight Gate | collection 到 apiserver 的在途调用数 | 单个 collection 进程 | 等待受调用 context 和配置共同约束，超时向上传播 |
| Backpressure | 对某依赖的在途操作数 | 单个 apiserver 进程，同一依赖跨 repository 共享 | 槽位等待超时，通常向上传播为 unavailable/503 |
| SubmitGuard | 同一提交意图的建议性 lease ownership | Redis 可跨 collection 实例 | 当前不阻断争用者；争用或 Redis 故障仍进入 durable accept，由 Mongo 收敛 |
| LockLease | leader、长任务或重复消费所有权 | Redis 可跨实例 | 争用不执行；续租失败取消协作式 body；具体调用方决定 fail-open/fail-closed |
| 持久化约束 | 可被承认的业务事实 | Mongo / MySQL | 唯一键、指纹、事务决定正确性 |

## 4. 阅读顺序

1. [压力模型与责任边界](./10-压力模型与责任边界.md)：先建立限流、并发、背压和正确性的统一模型。
2. [入口准入：RateLimit 与 Gate](./20-入口准入-RateLimit与Gate.md)：理解 HTTP 入口的真实顺序与失败语义。
3. [可靠提交：SubmitGuard 与幂等](./30-可靠提交-SubmitGuard与幂等.md)：回答重复请求、409、202 与 Mongo 最终真相。
4. [下游背压与容量预算](./40-下游背压与容量预算.md)：学习 QPS、并发、连接池和实例数为什么不能直接相乘。
5. [LockLease 与长任务互斥](./50-LockLease与长任务互斥.md)：理解租约、续租、失租和 fencing 边界。
6. [运行时治理与故障恢复](./60-运行时治理与故障恢复.md)：区分数据面、控制面和操作审计。
7. [可观测性、压测与验收](./70-可观测性-压测与验收.md)：把配置基线变成可验证容量。

## 5. 事实源

| 问题 | 当前事实源 |
| --- | --- |
| collection HTTP 准入 | `internal/collection-server/transport/rest/router_concurrency.go`、`router.go` |
| Gate 与 collection resilience | `internal/collection-server/concurrency`、`internal/collection-server/resilience/subsystem` |
| RateLimit / Backpressure / LockLease | `internal/pkg/resilience` 与当前 `component-base` 适配层 |
| 可靠提交 | `internal/collection-server/application/answersheet`、`internal/apiserver/application/survey/answersheet` |
| Mongo 幂等 | `internal/apiserver/infra/mongo/answersheet` |
| 运行时治理 | `internal/apiserver/application/systemgovernance`、三个进程的 resilience subsystem |
| 生产基线 | `configs/collection-server.prod.yaml`、`configs/apiserver.prod.yaml`、`configs/worker.prod.yaml` |
| 压测入口 | `Makefile` 的 `perf-*` 目标、`scripts/perf` |

## 6. 当前限制与规划

| 状态 | 结论 |
| --- | --- |
| `已实现` | collection 在启动时有 Redis rate backend 就构造分布式 limiter；没有 backend 才构造本地 limiter。 |
| `已实现` | 分布式 limiter 的 Redis 运行期错误为 degraded-open，不会自动切换到本地保守阈值。 |
| `已实现` | resilience control 保留 queue controller 协议，但生产装配没有注册 queue，action registry 也未暴露 queue action。 |
| `规划改造` | Redis 限流故障后切换本地保守预算、连续失败与窗口失败率判定、半开探测、渐进恢复。 |
| `规划改造` | 基于依赖饱和信号的自适应并发或 Circuit Breaker。 |

规划项只是讨论入口；在代码、配置、指标与故障测试同时落地前，不得作为现行能力对外承诺。

## 7. 学习检查

读完本批文档后，应能独立回答：

1. 为什么同一用户、同一 `idempotency_key`、相同内容应返回同一结果，而不是 429 或 409？
2. 为什么 SubmitGuard 可以失效，而 Mongo 唯一键不能缺失？
3. 为什么“每实例 80 QPS，3 个实例就是 240 QPS”不是容量结论？
4. RateLimit、Gate、Backpressure、连接池分别限制什么量？
5. Redis 故障时 fail-open 与 fail-closed 各保护了什么、牺牲了什么？
