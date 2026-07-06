# Observability 阅读地图

**本文回答**：`observability/` 子目录这一组文档应该如何阅读；qs-server 的可观测性负责什么、不负责什么；Metrics、Healthz/Pprof、Logging/Audit、GovernanceEndpoint 与排障 SOP 分别应该去哪里看。

---

## 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| 模块定位 | `observability/` 是 qs-server 的**全局可观测性规范入口**，收口 metrics、healthz、pprof、logging、audit、governance endpoint 与排障路径 |
| 当前入口 | GenericAPIServer 默认开启 healthz、metrics、profiling；`/healthz` 返回 ok，metrics 由 gin-prometheus 挂载，pprof 注册在 `/debug/pprof` |
| 指标事实 | Redis/cache/lock/warmup 指标在 `cachegovernance/observability`；Resilience 指标在 `resilienceplane`；其它模块指标应遵守同一低基数规则 |
| 日志事实 | 当前大量代码使用 component-base logger/log；文档重点约束字段语义、敏感信息和 correlation |
| 审计边界 | Audit 面向安全/管理/服务间访问，不等同普通业务日志 |
| 治理端点 | Governance endpoint 默认只读；manual warmup / repair complete 这类 action 必须有明确 SOP、权限和审计 |
| 不负责 | 不替代 Redis/Event/Resilience/Security 各自深讲的观测细节；不定义业务指标口径 |
| 推荐读法 | 先读整体架构，再读 metrics、healthz/pprof、logging/audit，最后读 governance endpoint 与排障 SOP |

一句话概括：

> **observability 负责统一“怎么看系统”，而不是替每个基础设施模块重复写一遍内部排障细节。**

---

## 1. 本目录文档地图

```text
observability/
├── README.md
├── 00-整体架构.md
├── 01-Metrics指标体系.md
├── 02-Healthz与Pprof.md
├── 03-Logging与Audit.md
└── 04-GovernanceEndpoint与排障SOP.md
```

| 顺序 | 文档 | 先回答什么 |
| ---- | ---- | ---------- |
| 1 | [00-整体架构.md](./00-整体架构.md) | 三进程可观测入口总图，metrics/healthz/pprof/log/audit/governance 如何协作 |
| 2 | [01-Metrics指标体系.md](./01-Metrics指标体系.md) | Prometheus 指标命名、低基数 label、Redis/Resilience 指标入口 |
| 3 | [02-Healthz与Pprof.md](./02-Healthz与Pprof.md) | `/healthz`、readiness 语义、pprof 适用场景和风险 |
| 4 | [03-Logging与Audit.md](./03-Logging与Audit.md) | 结构化日志字段、敏感信息、审计日志边界 |
| 5 | [04-GovernanceEndpoint与排障SOP.md](./04-GovernanceEndpoint与排障SOP.md) | governance endpoint 只读边界、常见故障的观测入口、新增观测能力 SOP |

---

## 2. 与其它模块的边界

| 模块 | 自己负责 | Observability 负责 |
| ---- | -------- | ------------------ |
| `event/` | 事件目录、outbox、worker Ack/Nack、事件排障细节 | 统一指标/日志/治理入口规范 |
| `redis/` | family status、cache hit/miss、lock degraded、warmup | metrics 低基数原则、状态端点边界 |
| `resilience/` | outcome、queue depth、backpressure、degraded-open | outcome 指标读取方式和告警入口 |
| `security/` | Principal、AuthzSnapshot、CapabilityDecision | audit、安全日志、permission denied 排障入口 |
| `runtime/` | stage、container、lifecycle、shutdown | 启动/关闭日志规范、healthz/pprof |
| `integrations/` | WeChat/OSS/Notification adapter 错误 | 外部调用日志脱敏和观测边界 |

---

## 3. 推荐阅读路径

### 3.1 第一次理解 observability

```text
00-整体架构
  -> 01-Metrics指标体系
  -> 02-Healthz与Pprof
  -> 03-Logging与Audit
```

### 3.2 要新增指标

```text
01-Metrics指标体系
  -> 04-GovernanceEndpoint与排障SOP
```

重点看：

- 指标类型。
- label 低基数。
- 现有 namespace。
- 是否已有模块内指标。
- 是否需要 dashboard / alert。

### 3.3 要排查线上问题

```text
04-GovernanceEndpoint与排障SOP
  -> 对应模块深讲文档
```

例如：

- submit 429：进入 resilience。
- Redis degraded：进入 redis。
- permission denied：进入 security。
- event backlog：进入 event。
- CPU high：进入 pprof。

---

## 4. 当前主要观测入口

| 入口 | 说明 |
| ---- | ---- |
| `/healthz` | GenericAPIServer health endpoint，返回 status ok |
| `/metrics` | gin-prometheus 自动挂载的 Prometheus metrics endpoint |
| `/debug/pprof/*` | pprof 性能剖析入口 |
| cache governance status | Redis runtime、family、warmup、hotset 状态 |
| resilience status | RateLimit、SubmitQueue、Backpressure、Lock/Idempotency 状态 |
| logs | component-base 结构化日志 |
| audit | gRPC AuditInterceptor 和后续安全/管理操作审计 |

---

## 5. 核心原则

1. 指标 label 必须低基数。
2. 日志可以包含定位字段，但敏感信息必须脱敏。
3. Audit 和普通日志分开理解。
4. Healthz 不能做重操作。
5. Pprof 默认只用于排障，生产访问必须受控。
6. Governance endpoint 默认只读。
7. 新增观测能力必须能说明：看什么、为什么看、怎么排障、是否告警。

---

## 6. Verify

```bash
go test ./internal/pkg/server
go test ./internal/pkg/cachegovernance/observability
go test ./internal/pkg/resilienceplane
```

如果修改文档：

```bash
make docs-hygiene
git diff --check
```

---

## 7. 下一跳

| 目标 | 文档 |
| ---- | ---- |
| 整体架构 | [00-整体架构.md](./00-整体架构.md) |
| Metrics | [01-Metrics指标体系.md](./01-Metrics指标体系.md) |
| Healthz / Pprof | [02-Healthz与Pprof.md](./02-Healthz与Pprof.md) |
| Logging / Audit | [03-Logging与Audit.md](./03-Logging与Audit.md) |
| Governance / 排障 SOP | [04-GovernanceEndpoint与排障SOP.md](./04-GovernanceEndpoint与排障SOP.md) |
