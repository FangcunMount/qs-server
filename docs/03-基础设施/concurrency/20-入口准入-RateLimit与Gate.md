# 入口准入：RateLimit 与 Gate

## 1. 结论

RateLimit 与 Gate 都会拒绝请求，但控制的不是同一个量：

- RateLimit 控制“每秒允许多少新请求”，吸收持续高流量和 burst；
- Gate 控制“当前同时处理多少请求”，吸收慢请求和依赖延迟造成的并发堆积。

当前 collection 提交路由先取得 Submit Gate，再执行全局和用户 RateLimit。这是代码真实顺序，不是理论上唯一正确的顺序。

本章的推理起点不是“限流器放前还是放后”，而是先回答两件事：Redis limiter 本身需不需要并发保护，以及被 rate 拒绝的请求短暂占用 Gate 的代价是否已经成为主要瓶颈。没有这两类证据，交换中间件顺序只是移动排队位置。

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

三种候选顺序的权衡如下：

| 方案 | 收益 | 新风险 | 适用前提 |
| --- | --- | --- | --- |
| RateLimit → Gate | 超额请求更早离开，Gate 留给获准请求 | Redis/limiter 变慢时，入口调用 limiter 的并发没有 Gate 保护 | limiter 已有独立本地并发/超时保护 |
| Gate → RateLimit（当前） | 先限制本进程占用，Redis 变慢也不会无限放大调用 | 最终被 429 的请求也短暂占 Gate | 优先保证本实例故障边界可控 |
| 本地粗限流 → 分布式限流 → Gate | Redis 前还能挡住极端 burst | 双重预算会误拒绝，生效 QPS 和指标更难解释 | 有明确攻击/突发模型且能共同校准两套预算 |

重新排序的触发条件应是：Gate 等待主要由“必然会被 RateLimit 拒绝”的请求造成，且 Redis limiter 的延迟和故障已有独立保护。平均 QPS 上升不是充分条件。

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

query、wait-report、report-events 的 `newBudget` 会选择按原配置运行的本地 limiter。submit 另有显式保守 fallback：

- global 为每实例 30 QPS / burst 45；
- user 为每实例每用户 10 QPS / burst 15；
- 可通过 `rate_limit.submit_degraded_local.enabled=false` 禁用这层本地 fallback。

### 5.2 启动时有 backend，运行期 Redis 出错

分布式 limiter 返回 `degraded_open` 时：

- submit 再检查同一组每实例本地 global/user 预算；本地饱和返回 429 与 `Retry-After`；
- 其他 budget 继续 degraded-open；
- Redis 下一次调用恢复正常后，submit 立即重新采用分布式决策。

当前没有连续失败窗口或跨请求恢复状态。fallback 只对单次明确的 `degraded_open` 决策生效。

### 5.3 为什么选择 fail-open

RateLimit 是容量保护，不是业务事实。Redis 故障时一律 fail-closed 会把原本可以由 DB 承载的小流量也拒绝掉。当前实现选择保可用性，把风险继续交给 Gate、gRPC inflight、apiserver RateLimit、Backpressure 和数据库约束。

这个选择不是无成本：如果故障时流量很大，后端保护层会承受更多压力。因此需要监控 `degraded_open`、Gate 拒绝、Backpressure timeout 和 DB 饱和信号。

还要区分“limiter 后端运行期失败”和“路由根本拿不到已装配 budget”：

- 已取得 distributed limiter，但 Redis `Allow` 出错：degraded-open；
- `rate_limit.enabled=true`，但 `RateBudgetProvider` 没有对应 budget：路由直接返回 503；
- limiter 对象为 nil：HTTP middleware 走 degraded-open。

前者是显式故障策略，第二种是 composition invariant 破坏，不能混成同一个 Redis 故障。

### 5.4 为什么 submit 不是完全 fail-open

submit 会进入跨服务校验和 Mongo transaction，成本显著高于普通缓存命中。Redis 故障若让每个实例都按 300 QPS 分布式基线无条件放行，扩容会把集群放大量直接推给 gRPC/Mongo。因此当前只在 primary 明确返回 `degraded_open` 时，追加每实例 30/10 QPS 的保守本地预算。

这不是一条通用常数：实例数变化会改变聚合上界；query、wait-report、report-events 尚未复制该策略，也是因为它们的成本与可降级结果不同。应先取得路径成本、cache hit、下游拐点和故障流量证据，再决定每个 budget 的 fallback。

当前恢复契约只有单次判断：下一次 Redis 调用正常，就重新采用分布式预算；没有连续失败窗口或有状态恢复过程。文档只描述这一现行语义，不把候选算法列为当前能力。

## 6. 仓库版本化配置意图

以下数字来自仓库 `collection-server.prod.yaml`，只是版本化部署意图；不证明目标环境采用了同一 effective config，也不是容量证明。

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

## 8. Redis 故障本地保护

状态：`submit 已实现`。

当前路径是：

```text
Redis 健康
  -> 使用分布式高阈值
单次 Redis degraded-open
  -> 检查本实例 30/10 QPS 保守预算
Redis 下一次调用恢复
  -> 直接回到分布式预算
```

有状态恢复策略不属于当前实现。实例数变化会改变本地 fallback 的理论聚合上限，扩缩容时必须同步复核配置。

## 9. 验证入口

- 路由策略：`internal/collection-server/transport/rest/router_concurrency_test.go`
- fail-open：`internal/collection-server/transport/rest/rate_limit_test.go`
- budget 版本与保守切换：`internal/pkg/resilience/ratelimit/budget_test.go`
- 配置校验：`internal/collection-server/options/options_test.go`
- 压测：`make perf-run PLAN=baseline`、`make perf-run PLAN=admission`；故障与幂等专项使用 `PLAN=diagnose CASE=...`

## 10. 验证问题

假设一个 collection 实例的 submit Gate 已经占满 96 个槽位，但 RateLimit token 仍然充足：

1. 新请求为什么还是会在 50ms 后得到 429？
2. 这能否证明 RateLimit 配得太高？
3. 你需要同时观察哪些延迟和下游信号，才能判断是 Mongo 慢、gRPC Gate 满，还是 application 前置校验变慢？
