# Observability：从信号到可证明的运行事实

Observability 层解决的不是“有没有日志和 Prometheus”，而是当一次请求变慢、一个事件不再推进或一个治理动作执行失败时，能否用多种独立证据还原发生了什么、影响了谁、事实停在哪一层，以及下一步操作是否安全。

## 1. 先看结论

- 当前系统有四类互补证据：结构化日志、Prometheus 指标、运行时状态快照、持久化业务/治理事实。任何一种都不能单独证明端到端正确。
- `request_id` 关联一次同步调用，`event_id` 关联一次异步消息，业务 ID 关联持久化事实，`idempotency_key` 关联业务意图；不能用一个字段替代其它字段。
- HTTP middleware 会接收调用方提供的 `X-Request-ID`，缺失时生成 UUID，并向响应和下游 gRPC 传播。当前没有格式/长度约束，因此它适合关联，不适合作为可信身份或指标 label。
- gRPC 日志 interceptor 会从 metadata 读取或生成 trace/request ID，但生成一个 `trace_id` 字符串不等于创建了 OpenTelemetry span。
- 当前可确认的 `enable-tracing` 装配只作用于 IAM SDK。代码中没有发现 QS 三个进程统一初始化 exporter、TracerProvider 和跨 HTTP/MQ 的 span pipeline，因此不能宣称已具备端到端分布式追踪。
- apiserver `/health` 是静态 liveness 响应；`/readyz` 只判断 Redis runtime snapshot，且 status service 缺失或报错时会回退为 `Ready=true`。它们不能证明 MySQL、MongoDB、IAM、MQ 或业务模块可用。
- collection-server 与 worker 的 liveness 同样较浅；其 readiness 主要表达 Redis family，collection 额外要求 resilience control 完成首次同步。
- System Governance 把本地/远程快照、Prometheus 时间窗和持久化 checkpoint/retry/dead-letter 证据汇总为诊断视图，但它是投影，不是业务正确性的事实源。
- 指标 label 必须来自有限枚举；用户 ID、Assessment ID、event ID、request ID、错误原文和 Redis key 只能进受控日志/审计，不能进入 Prometheus label。
- 当前 collection-server 的 `APILogger` 默认记录最多 16 KiB 的请求和响应体；字段名脱敏无法覆盖答案、报告等业务敏感内容。这是现存隐私风险，不是推荐实践。
- “没有指标”“指标为 0”“查询不到状态”是三种不同结论。Prometheus/远程组件不可用时，System Governance 会标记 `available=false` 和原因，不应伪装成零。

## 2. 四个证据平面

| 平面 | 回答的问题 | 代表实现 | 不能独立证明 |
| --- | --- | --- | --- |
| Logs | 某次操作经过了哪些分支，错误上下文是什么 | HTTP/gRPC middleware、worker/application 日志 | 总体比例、积压规模、持久事实已经提交 |
| Metrics | 错误率、延迟、并发、积压是否发生趋势变化 | Prometheus collectors、`/metrics` | 某一业务实体的完整历史和精确错误正文 |
| Runtime snapshot | 某进程此刻如何装配、Redis family/韧性能力是否降级 | `/readyz`、`/governance/redis`、`/governance/resilience` | DB/MQ 全链健康、过去发生过什么 |
| Durable evidence | 权威事实停在哪个状态、动作由谁授权、是否可重放 | Outbox、dead letter、retry hold、checkpoint、action audit | 当前进程资源占用与瞬时延迟 |

一次可靠诊断通常要把四者串起来：

```text
symptom / alert
      ↓
metrics: scope, rate, latency, backlog
      ↓
runtime snapshot: configured, degraded, synchronized
      ↓
logs: request/event branch and error category
      ↓
durable state: committed fact, claim, outbox, hold, dead letter
```

例如 `qs_event_consume_total{outcome="nack"}` 增长只能证明 consumer 正在失败；要判断事件会不会丢，还要检查 transport attempt、`event_delivery_dead_letter`、业务状态和人工 replay disposition。

## 3. 关联标识与传播

### 3.1 标识职责

| 标识 | 生命周期 | 主要用途 |
| --- | --- | --- |
| `request_id` | 一次 HTTP/gRPC 调用及其直接下游调用 | 同步日志关联、错误返回、治理动作请求幂等 |
| `trace_id` | 一条被显式传播的调用/执行链 | gRPC 日志、Evaluation/Interpretation Run 追踪 |
| `event_id` | 一份 Event envelope | publish、Outbox、MQ、consume、dead letter 关联 |
| `idempotency_key` | 一次业务意图的多次投递 | AnswerSheet 等业务幂等；不是链路追踪 ID |
| 业务 ID | AnswerSheet/Assessment/Run/Outcome/Report 生命周期 | 权威状态查询与跨阶段诊断 |
| `action_request_id` | 一次人工 retry/governance 意图 | 操作审计、冲突检测、重复请求结果回放 |

`request_id` 不能代替 `idempotency_key`：网络重试通常会产生另一个 request，而同一业务意图仍应返回第一次持久化结果。`event_id` 也不能代替业务 ID：同一业务实体可能经历多个 Event 和 retry event。

### 3.2 HTTP

`internal/pkg/middleware.RequestID`：

1. 优先采用传入的 `X-Request-ID`。
2. 缺失时生成 UUID v4。
3. 写入 Gin context、标准 `context.Context`、请求 header 和响应 header。
4. collection gRPC client 可从标准 context 取出并写入下游 `x-request-id` metadata。

当前 middleware 不验证入站值的格式、长度或字符集。边界代理应限制 header 大小；日志查询也不应把 request ID 当作已认证的调用方声明。

### 3.3 gRPC 与异步链路

- server/client logging interceptor 从 metadata 取 `x-request-id`、`x-trace-id`，缺失时生成本地 ID，并注入日志 trace context。
- collection-server 调用 apiserver 时传播 HTTP request ID。
- AnswerSheet durable submit 把 `request_id` 写入 AnswerSheet/Event payload；worker 再把它附到调用 apiserver 的 gRPC metadata。
- Event envelope 的 `event_id/type/topic` 是异步主关联键，consumer 日志应同时记录业务 ID 与 attempt/outcome。

这形成了可用的“日志相关性”，但还不是完整 distributed tracing。要声明 OpenTelemetry 已落地，至少需要确认各进程的 provider/exporter、HTTP/gRPC/MQ instrumentation、上下文格式、采样策略和 collector 后端，而不能只看配置中存在 `enable-tracing: true`。

## 4. 日志

### 4.1 当前实现

- 通用 HTTP server 全局安装 RequestID 和 Context middleware，再按 `server.middlewares` 名称安装其它 middleware。
- collection-server 显式安装基础 Logger 与 `APILogger`。
- `APILogger` 记录 request start/end、method、path、query、client IP、headers、status、duration 和可选 body，默认 body 上限为 16 KiB。
- gRPC client/server logger 记录 method、duration、status 与 trace/request ID。
- worker 同时存在 component-base log 与 `slog` 初始化路径；排障前应先确认具体 handler 使用哪套 logger。
- 生产配置通常把业务日志写 stdout 和滚动文件；实际采集、保留、索引和脱敏仍由部署环境负责。

### 4.2 当前配置与实现偏差

`configs/apiserver.prod.yaml` 配置了 `enhanced_logger`，但当前 middleware registry 只注册 `logger`、`apilogger` 等名称。Generic server 遇到未知名称只记录 warning 并跳过，因此不能仅根据配置文本断言 apiserver 已安装增强 access log。

collection-server 则明确安装 `APILogger`，其默认配置会记录请求和响应 body。脱敏只识别 password、secret、token、authorization 等字段名；AnswerSheet answers、报告正文、姓名等不是通用 secret key，可能被原样记录。正确整改方向是：

- 生产默认关闭 request/response body 日志。
- 若确需调试，按 route allowlist 与结构化字段显式采样，不记录完整 payload。
- 答案、报告、token、cookie、delegated subject、数据库/Redis key 和对象签名 URL禁止进入普通日志。
- 错误日志记录分类、阶段、稳定 ID 和可恢复性，不记录完整敏感对象。

### 4.3 最低结构化字段

| 场景 | 建议字段 |
| --- | --- |
| HTTP | component、request_id、method、route template、status、duration、error_category |
| gRPC | component、request_id/trace_id、full_method、code、duration、peer/service identity |
| Event | service、event_id、event_type、topic、attempt、settlement、business_id |
| DB/Cache | store/family、operation、result、duration、degraded/backpressure reason |
| Governance | org_id、actor_user_id、action_request_id、action_id、target、status |

日志中的 route 应优先使用模板，而不是含动态 ID 的 raw path；错误分类使用有限枚举，错误原文只作为受控字段。

## 5. Prometheus 指标

### 5.1 主要指标族

| 系统问题 | 当前代表指标 |
| --- | --- |
| Cache hit/write/latency/payload | `qs_cache_get_total`、`qs_cache_write_total`、`qs_cache_operation_duration_seconds`、`qs_cache_payload_bytes` |
| Cache warmup/hotset/version/signal | `qs_cache_warmup_duration_seconds`、`qs_cache_hotset_size`、`qs_query_cache_version_total`、`qs_cache_signal_*` |
| Redis runtime/lock | `qs_cache_family_available`、`qs_cache_family_degraded_total`、`qs_runtime_component_ready`、`qs_cache_lock_*` |
| Event publish/consume | `qs_event_publish_total`、`qs_event_consume_total`、`qs_event_consume_duration_seconds` |
| Outbox backlog | `qs_event_outbox_total`、`qs_event_outbox_backlog`、`qs_event_outbox_oldest_age_seconds` 及 by-type 版本 |
| Resilience | `qs_resilience_decision_total`、`qs_resilience_backpressure_inflight`、`qs_resilience_backpressure_wait_duration_seconds` |
| Collection gate | `collection_http_gate_wait_seconds`、`collection_grpc_inflight_wait_seconds`、`qs_collection_submit_gate_reject_total` |
| Reliable submit | `qs_collection_answersheet_submit_total`、`qs_collection_answersheet_submit_stage_duration_seconds`、`qs_apiserver_answersheet_durable_submit_total` |
| Report wait/status | `signaling_*`、`report_wait_*`、`wait_report_*`、`report_status_*` |
| Evaluation/Interpretation/Statistics | run/lease/recovery/reconcile、`qs_statistics_*` 等业务流程指标 |
| Governance audit fallback | `system_governance_audit_fallback_pending`、`system_governance_audit_fallback_total` |

系统还通过 Gin Prometheus 输出 HTTP 指标。最终查询前应以实际 `/metrics` 暴露名为准，不应仅凭 collector 变量名猜测 dashboard。

### 5.2 Label 基数

当前公共指标主要使用有限维度：

- cache：family、policy、operation、result。
- Event：service/source/relay、topic、event_type、outcome。
- resilience：component、kind/scope/resource/strategy、outcome。
- runtime：component、family、profile、reason。

新增指标必须保持相同原则。禁止作为 label：

- user/org/testee/answersheet/assessment/report ID。
- request/event/idempotency/action request ID。
- URL、raw path、Redis key、异常 message/stack。
- 任意模型名、问卷 code 或客户端输入，除非先证明值域有严格上限。

高基数信息放日志或持久化审计；metrics 只负责聚合趋势。Histogram bucket 也要根据 SLO 和真实延迟设计，不能用单个平均值掩盖尾延迟。

### 5.3 暴露面

- Generic API server 在 `server.metrics=true` 时注册 `/metrics`；apiserver 生产配置当前启用。
- worker 单独启动 metrics server，生产配置绑定 `0.0.0.0:9092`，同时开放 health/ready/governance。
- collection-server 复用通用 HTTP server 的 metrics 配置能力；具体是否开放以完成后的 runtime options 为准。

这些 endpoint 没有业务 JWT 保护。应由独立监听地址、网络策略、反向代理 ACL 或监控专网限制访问。

## 6. Health、Readiness 与 Governance

### 6.1 当前语义矩阵

| 进程/端点 | 当前判断 | 返回非 2xx 的条件 | 没有覆盖 |
| --- | --- | --- | --- |
| apiserver `/health` | 固定结构的 `healthy` | 无 | MySQL、Mongo、Redis、IAM、MQ、模块 |
| apiserver `/healthz` | Generic server 静态 `ok`（配置开启时） | 无 | 全部依赖 |
| apiserver `/readyz` | Redis runtime summary | summary `Ready=false` | DB、IAM、MQ；status service 异常还会 fallback ready |
| collection `/health` | 固定 `healthy` + Redis snapshot | 无 | apiserver/IAM 可达性、业务 handler |
| collection `/readyz` | Redis summary + resilience control 初次同步 | Redis not ready 或 control 未同步 | apiserver gRPC、IAM、完整业务路径 |
| worker `/healthz` | 固定 `healthy` + Redis snapshot | 无 | MQ consumer、apiserver gRPC、DB |
| worker `/readyz` | Redis runtime summary | summary `Ready=false` | MQ、consumer registration、gRPC、业务 handler |

`Container.HealthCheck()` 确实会 ping IAM、MySQL、Mongo、默认 Redis 并检查模块，但当前 apiserver `/health`、`/readyz` handler 没有调用它。文档和探针必须按实际 handler 语义解释，不能把一个存在但未接线的方法当成线上健康保证。

### 6.2 Liveness 与 readiness 的正确使用

- liveness 只应回答“进程是否需要重启”。外部依赖短暂失败通常不应触发重启风暴。
- readiness 回答“是否应继续接收这类流量”。如果只检查 Redis，就只能称为 Redis/runtime readiness，不能代表完整业务 readiness。
- startup/synchronization probe 用于首次策略/缓存/控制面同步，避免初始化未完成就接流量。
- 深度 dependency check 更适合受保护的诊断接口或 synthetic transaction，不宜全部塞进高频 liveness。

当前命名容易让调用者过度理解，尤其是 apiserver status service 失败时 `Ready=true`。在代码收敛前，部署探针和 runbook 必须显式记录这些限制。

### 6.3 Governance snapshot

三个进程可暴露 Redis/Resilience snapshot；apiserver 的受保护 System Governance API 还会汇总：

- apiserver 本地 Cache/Event/Resilience 状态。
- collection-server、worker 等配置的远程 component snapshot。
- Prometheus 时间窗证据，默认窗口为 5 分钟。
- Outbox、retry hold、dead letter、checkpoint 和候选操作。
- 已启用/规划中的治理 action。

Prometheus 查询默认 3 秒 timeout；远程 component fetch 默认 3 秒 timeout、响应上限 1 MiB。失败会保留 `available=false` 与原因，而不是生成虚假零值。

System Governance internal routes走组织管理员 capability，但各 component 的 `/governance/redis`、`/governance/resilience` 当前是公开 handler，依赖网络边界保护。聚合结果还可能是不同时间点的快照，因此用于诊断和操作决策，不作为事务真值。

## 7. 持久化审计与恢复证据

高风险治理 action 使用 `(org_id, request_id)` claim：

1. MySQL `system_governance_action_runs` 是 claim authority。
2. claim 前会先查询已配置的 Redis terminal fallback；fallback 查询失败或 MySQL claim 失败时 action 都不执行。
3. 同 request ID 已完成则回放原结果，仍在 running 则返回冲突。
4. action 执行后先在约 3 秒窗口内重试完成 MySQL audit。
5. MySQL terminal outcome 暂时无法写入时，才把脱敏后的 terminal replay 写入 Redis fallback；fallback 不保存 input 或 actor credential，且不设置 TTL。
6. recovery runner 每 30 秒最多回填 100 条到 MySQL，成功后删除 fallback。
7. MySQL 与 fallback 都无法保存 terminal outcome 时，调用方收到“结果无法持久化”的错误；此时 action 可能已经发生，必须依靠 request ID 和目标状态审计，不能盲目重试不同 key。

这条设计说明 Redis 在这里不是业务 action 的 claim authority，只是终态审计的持久 fallback。真正的人工恢复还要结合：

- Event Outbox `pending/failed/manual_required`。
- delivery dead letter 与 replay request。
- retry hold/retry disposition。
- Evaluation/Interpretation/Statistics checkpoint、lease、claim history。

日志消失不应导致这些事实消失；反过来，审计 row 存在也不证明下游副作用已经全部完成。

## 8. 典型诊断链

### 8.1 AnswerSheet 已返回 202，但 Assessment 未出现

1. 用 `answersheet_id`、`request_id` 查 durable submit 日志和指标阶段。
2. 查 Mongo AnswerSheet 与同事务 Outbox 是否存在；202 的正确性承诺止于 durable AnswerSheet + Outbox。
3. 看 `qs_event_outbox_backlog*`、publish outcome 与 oldest age。
4. 用 `event_id` 查 worker consume、attempt、NACK/hold/dead-letter。
5. 查 apiserver intake/Assessment 幂等事实与 Evaluation checkpoint。
6. 只有确认进入 `manual_required`/dead letter 且目标状态允许时，才走治理 replay。

### 8.2 Redis 故障后提交错误上升

1. 看 `/governance/redis` family/reason 和 `qs_cache_family_degraded_total`。
2. 看 resilience decision、gate wait/reject 与 DB backpressure in-flight/wait。
3. 区分 cache miss 增加、rate limiter degraded-open、SubmitGuard fallback 与数据库真实错误。
4. 查 Mongo duplicate/conflict 和 durable submit outcome，验证正确性是否仍由唯一键与 transaction 保持。
5. readiness 只说明 Redis runtime 判定；不能据此断言数据库已经过载。

### 8.3 System Governance action 返回 500

1. 固定原 `org_id + request_id`，不要换 key 立即重试。
2. 查 MySQL action run 是 running、success 还是 failed。
3. 看 audit fallback pending/operation 指标和 Redis fallback。
4. 直接检查目标状态，判断 action 是否已经生效。
5. 仅在原 request ID 可安全回放或人工确认后继续。

## 9. 当前缺口与演进方向

以下是目标态，不是当前已实现能力：

1. 为三个进程定义分层 probe：process liveness、startup synchronization、按流量类型划分的 readiness、受保护 deep check。
2. apiserver status service 失败时 readiness 应明确 unknown/degraded，而不是 fallback ready。
3. 修正 middleware registry/config 名称偏差，建立统一 access log schema。
4. 生产默认关闭 body 日志，并为答卷、报告、身份与治理输入建立隐私测试。
5. 若需要分布式追踪，统一初始化 OpenTelemetry，覆盖 HTTP、gRPC、MQ、Outbox relay，并定义 sampling/PII 规则。
6. 在仓库或部署配置中固化 SLI/SLO、recording rule、alert rule、dashboard 与 runbook；当前 collector 丰富不等于告警闭环已经存在。
7. 为关键 Event、Outbox、DB backpressure、report wait 和 action audit fallback 设置基于速率与持续时间的告警，避免对单点瞬时值告警。
8. 将 component governance/metrics/pprof 放到受限运维监听面。

## 10. 扩展与验收清单

新增一条基础设施或业务链路时：

1. 定义 request/event/business/action 四类关联 ID 如何传播。
2. 为成功、业务拒绝、依赖故障、降级、重试和终态失败定义有限 outcome。
3. 选择 Counter/Gauge/Histogram，并证明所有 label 值域有界。
4. 日志只记录稳定 ID、阶段、分类和耗时；审查 body、claim、token 与业务敏感字段。
5. 明确哪个数据库/集合/表是 durable evidence，如何查询、重放和恢复。
6. 决定依赖失败影响 liveness、readiness、degradation 还是只影响某个 feature。
7. 对 status source unavailable 单独建模，不把 unknown 写成 zero/healthy。
8. 为 metrics、health/status handler、审计幂等与恢复 runner 写测试。
9. 在真实进程上检查 `/metrics`、probe、日志字段和网络暴露，而不是只跑 unit test。

验证入口：

```bash
go test ./internal/pkg/middleware \
  ./internal/pkg/cache/observe \
  ./internal/pkg/eventing/observe \
  ./internal/pkg/redisruntime/observability \
  ./internal/pkg/resilience \
  ./internal/pkg/reportstatus

go test ./internal/apiserver/application/systemgovernance/... \
  ./internal/apiserver/infra/mysql/systemgovernance \
  ./internal/apiserver/infra/redis/systemgovernance \
  ./internal/apiserver/transport/rest \
  ./internal/collection-server/transport/rest/handler \
  ./internal/worker/observability
```

这些测试能验证 collector、状态投影与 handler 分支，但不能替代 Prometheus scrape、日志采集后端、真实网络 ACL、告警路由和故障演练。
