# 入口准入：RateLimit 与 Gate

## 1. 结论

RateLimit 与 Gate 都会拒绝请求，但控制的不是同一个量：

- RateLimit 控制“每秒允许多少新请求”，吸收持续高流量和 burst；
- Gate 控制“当前同时处理多少请求”，吸收慢请求和依赖延迟造成的并发堆积。

当前 collection 提交路由先取得 Submit Gate，再执行全局和用户 RateLimit。这是代码真实顺序，不是理论上唯一正确的顺序。

## 2. 提交入口真实顺序

`rateLimitedSubmitHandlers` 先构造 rate handlers，再交给 `submitHandlers` 包裹，所以 Gin handler 链是：

```text
Submit Gate
  -> Global RateLimit
  -> User RateLimit
  -> AnswerSheet handler
  -> SubmissionService.AcceptDurably
```

这一顺序的含义：

- 优点：连 RateLimit 后端调用都受本地并发 Gate 保护，突发时先限制当前实例占用；
- 代价：最终会被 RateLimit 拒绝的请求，也会短暂占用 Gate 槽位；
- 结论：不能只看中间件名字猜顺序，必须看组合函数如何 append。

如果未来调整顺序，需要用 Redis 故障、热点用户、突发请求和提交延迟四类场景重新验收，而不是只比较平均 QPS。

## 3. Route-to-Gate 矩阵

| 路由类别 | Gate 策略 | 当前饱和结果 |
| --- | --- | --- |
| catalog L1 hit | 绕过 catalog Gate | 直接读 L1 |
| catalog L1 miss | 最多等待 `catalog_max_wait_ms` | 503 |
| report-status 短查询 | Try query Gate，不等待 | 503 |
| 普通 query | 最多等待 `max_wait_ms` | 503 |
| submit | 最多等待 `submit.gate_wait_ms` | 429，`Retry-After: 1` |
| wait-report | 独立 Gate；启用 immediate degrade 时 Try | 200 pending，带 `Retry-After` |
| collection -> apiserver gRPC | 等待 gRPC inflight Gate | 超时/取消向上传播 |

Gate 实现在 `internal/collection-server/concurrency/gate.go`，底层是进程内信号量。每个 collection 实例都有自己的计数器。

## 4. RateLimit 的两层预算

每个 budget 同时包含：

1. global limiter：约束该 budget 的总体到达速率；
2. user limiter：按用户 key 约束热点用户。

请求必须依次通过两层预算。global 防止总流量压垮系统，user 防止单用户耗尽公共容量。

当前 budget：

| 进程 | budget |
| --- | --- |
| collection-server | `query`、`submit`、`wait_report`、`report_events` |
| apiserver | `query`、`submit`、`admin_submit`、`wait_report` |

collection 在装配时有 Redis backend 就构造分布式 limiter；没有 backend 时才构造本地 token bucket。apiserver 当前始终使用本地 token bucket。

## 5. Redis 故障的真实行为

### 5.1 启动时没有 backend

collection `newBudget` 会选择本地 limiter：

- global 使用进程内 token bucket；
- user 使用按 key 的进程内 token bucket；
- 动态切换 policy 时有 1 秒 conservative transition，旧、新 limiter 中更保守的结果可以继续生效。

### 5.2 启动时有 backend，运行期 Redis 出错

分布式 limiter 返回 `degraded_open` 并允许请求通过。当前不会：

- 自动改用本地 30 QPS；
- 根据连续失败次数打开 Circuit Breaker；
- 半开探测 Redis；
- 逐步恢复到高阈值。

这解释了为什么“Redis 正常高阈值，Redis 故障自动切 30 QPS”目前只能写成规划，不能写成系统现状。

### 5.3 为什么选择 fail-open

RateLimit 是容量保护，不是业务事实。Redis 故障时一律 fail-closed 会把原本可以由 DB 承载的小流量也拒绝掉。当前实现选择保可用性，把风险继续交给 Gate、gRPC inflight、apiserver RateLimit、Backpressure 和数据库约束。

这个选择不是无成本：如果故障时流量很大，后端保护层会承受更多压力。因此需要监控 `degraded_open`、Gate 拒绝、Backpressure timeout 和 DB 饱和信号。

还要区分“limiter 后端运行期失败”和“路由根本拿不到已装配 budget”：

- 已取得 distributed limiter，但 Redis `Allow` 出错：degraded-open；
- `rate_limit.enabled=true`，但 `RateBudgetProvider` 没有对应 budget：路由直接返回 503；
- limiter 对象为 nil：HTTP middleware 走 degraded-open。

前者是显式故障策略，第二种是 composition invariant 破坏，不能混成同一个 Redis 故障。

## 6. 当前生产基线

以下数字来自生产配置，是初始预算，不是容量证明。

### 6.1 collection RateLimit

| budget | global QPS / burst | user QPS / burst |
| --- | ---: | ---: |
| submit | 300 / 450 | 120 / 180 |
| query | 300 / 450 | 120 / 180 |
| wait-report | 200 / 300 | 60 / 120 |
| report-events | 120 / 180 | 20 / 40 |

### 6.2 collection Gate

| Gate | 单实例容量 | 等待策略 |
| --- | ---: | --- |
| catalog | 512 | L1 miss 最多 800ms |
| query | 280 | 最多 4000ms |
| submit | 96 | 最多 50ms |
| wait-report | 400 | 满时立即返回 pending |
| gRPC downstream | 420 | 配置等待 4000ms，但更早的请求 deadline 优先 |

`submit.accept_timeout_ms=2000` 是整个可靠受理的外层 deadline。即使某个内层配置写着 4000ms 或 5000ms，也不能突破更早到期的 request context。

## 7. 为什么实例数不能直接乘 QPS

“每实例 80 QPS，3 个实例就是 240 QPS”至少隐含了五个未经证明的假设：

1. limiter 确实是本地的；Redis 全局 limiter 不应按实例相乘；
2. 负载均衡完全均匀，没有长连接、热点用户或 hash 偏斜；
3. 每实例可安全处理 80 QPS，而不是仅仅允许 80 QPS 进入；
4. 请求成本相同，且没有重试、后台任务和内部放大；
5. 下游数据库、连接池和网络能承受聚合后的依赖操作量。

实例减少时，本地预算的理论总准入会下降；但系统是否更危险取决于客户端退避。如果客户端收到 429 后立即重试，重试流量会放大。服务端必须提供 `Retry-After`，客户端还应使用指数退避、抖动和最大尝试次数。

## 8. 规划中的 Redis 故障本地保护

状态：`规划改造`。

讨论中的方向是：

```text
Redis 健康
  -> 使用分布式高阈值
连续失败 + 窗口失败率越界
  -> 进入本地保守预算
恢复窗口满足更严格条件
  -> 半开、小步抬升、持续观测
稳定
  -> 回到分布式预算
```

落地前必须补齐：

- 单实例安全预算的推导与压测证据；
- 状态机、最短驻留时间和抖动控制；
- Redis 超时与业务拒绝的独立指标；
- 实例数变化时的预算来源；
- 恢复时的渐进放量；
- 故障注入测试。

## 9. 验证入口

- 路由策略：`internal/collection-server/transport/rest/router_concurrency_test.go`
- fail-open：`internal/collection-server/transport/rest/rate_limit_test.go`
- budget 版本与保守切换：`internal/pkg/resilience/ratelimit/budget_test.go`
- 配置校验：`internal/collection-server/options/options_test.go`
- 压测：`make perf-reliable-submit24`、`make perf-reliable-submit48-burst`、`make perf-reliable-submit96-boundary`

## 10. 学习问题

假设一个 collection 实例的 submit Gate 已经占满 96 个槽位，但 RateLimit token 仍然充足：

1. 新请求为什么还是会在 50ms 后得到 429？
2. 这能否证明 RateLimit 配得太高？
3. 你需要同时观察哪些延迟和下游信号，才能判断是 Mongo 慢、gRPC Gate 满，还是 application 前置校验变慢？
