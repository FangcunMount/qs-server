# Metrics 指标体系

**本文回答**：qs-server 的 Prometheus 指标如何组织；当前 Redis/cache/lock/warmup 和 Resilience 指标有哪些；指标命名与 label 应遵守什么低基数原则；新增指标时应该如何选择 counter/gauge/histogram。

---

## 30 秒结论

| 指标类型 | 适用场景 |
| -------- | -------- |
| Counter | 事件次数、决策次数、错误次数，只增不减 |
| Gauge | 当前状态，例如 queue depth、in-flight、family available |
| Histogram | 延迟、耗时、payload size、wait duration |
| Summary | 当前不建议新增，优先 Histogram |

| 当前指标族 | 来源 |
| ---------- | ---- |
| `qs_cache_*` | Redis/cache/lock/warmup observability |
| `qs_query_cache_version_total` | QueryCache version token |
| `qs_runtime_component_ready` | Redis runtime readiness |
| `qs_resilience_*` | Resilience decision、queue、backpressure |
| `gin_*` | gin-prometheus HTTP metrics |

一句话概括：

> **Metrics 负责趋势和告警；业务定位细节进日志，不进 label。**

---

## 1. 指标命名原则

推荐格式：

```text
qs_<plane>_<thing>_<unit_or_total>
```

示例：

```text
qs_cache_get_total
qs_cache_operation_duration_seconds
qs_resilience_decision_total
qs_resilience_queue_depth
```

命名要求：

1. 前缀统一：`qs_`。
2. plane 清晰：cache / resilience / runtime / event / business。
3. 单位明确：seconds / bytes / total。
4. counter 以 `_total` 结尾。
5. histogram 明确单位。
6. 不用业务对象 ID 命名指标。

---

## 2. Label 低基数原则

允许作为 label：

- component。
- family。
- profile。
- policy。
- op。
- result。
- kind。
- trigger。
- resource。
- strategy。
- outcome。
- status。

禁止作为 label：

- userID。
- requestID。
- taskID。
- assessmentID。
- answerSheetID。
- openID。
- token。
- appSecret。
- raw cache key。
- raw lock key。
- raw URL。
- raw error。
- arbitrary query param。

### 2.1 为什么禁止高基数

高基数 label 会导致：

- Prometheus 内存暴涨。
- 查询变慢。
- Dashboard 卡顿。
- 告警不可维护。
- 成本上升。

---

## 3. Redis / Cache 指标

### 3.1 Cache get

```text
qs_cache_get_total{family,policy,result}
```

用途：

- hit/miss/error 趋势。
- cache 命中率。
- Redis get error。

### 3.2 Cache write

```text
qs_cache_write_total{family,policy,op,result}
```

用途：

- set/delete/invalidate 是否失败。
- 写缓存错误趋势。

### 3.3 Operation duration

```text
qs_cache_operation_duration_seconds{family,policy,op}
```

用途：

- cache get/set/source_load/version_current/version_bump 等耗时。

### 3.4 Payload size

```text
qs_cache_payload_bytes{family,policy,stage}
```

用途：

- payload raw/compressed size。
- 判断 compression 是否有效。
- 排查大对象缓存。

### 3.5 Family availability

```text
qs_cache_family_available{component,family,profile}
```

Gauge：

- 1 = available。
- 0 = unavailable。

### 3.6 Family degraded

```text
qs_cache_family_degraded_total{component,family,profile,reason}
```

用于观察 Redis family degraded transition。

---

## 4. Warmup / Hotset / QueryVersion / Lock 指标

### 4.1 Warmup duration

```text
qs_cache_warmup_duration_seconds{trigger,result}
```

trigger 示例：

- startup。
- publish。
- statistics_sync。
- repair。
- manual。

### 4.2 Hotset size

```text
qs_cache_hotset_size{family,kind}
```

用于观察 hotset ZSet 规模。

### 4.3 Query version

```text
qs_query_cache_version_total{kind,op,result}
```

op：

- current。
- bump。

### 4.4 Lock acquire/release/degraded

```text
qs_cache_lock_acquire_total{name,result}
qs_cache_lock_release_total{name,result}
qs_cache_lock_degraded_total{name,reason}
```

name 是 lock spec 名称，不应是 raw lock key。

---

## 5. Resilience 指标

### 5.1 Decision

```text
qs_resilience_decision_total{
  component,
  kind,
  scope,
  resource,
  strategy,
  outcome
}
```

这是 Resilience 最重要指标。

适用：

- rate_limit allowed / rate_limited / degraded_open。
- queue accepted/full/done/failed。
- backpressure acquired/timeout/released。
- lock contention。
- duplicate skipped。
- idempotency hit。

### 5.2 Queue depth

```text
qs_resilience_queue_depth{component,scope,resource,strategy}
```

用于 SubmitQueue 当前深度。

### 5.3 Queue status

```text
qs_resilience_queue_status_total{component,scope,status}
```

虽然名字是 `_total`，当前是 GaugeVec，用于当前 status count。后续如果重命名，应考虑兼容。

### 5.4 Backpressure in-flight

```text
qs_resilience_backpressure_inflight{component,scope,resource,strategy}
```

### 5.5 Backpressure wait duration

```text
qs_resilience_backpressure_wait_duration_seconds{component,scope,resource,strategy,outcome}
```

注意：这是等待槽位耗时，不是下游执行耗时。

---

## 6. HTTP metrics

GenericAPIServer 在 EnableMetrics=true 时：

```go
ginprometheus.NewPrometheus("gin").Use(s.Engine)
```

这会注册 Gin HTTP metrics。

注意：

- HTTP metrics 只描述 HTTP 层。
- 不替代业务 metrics。
- 不替代 resilience outcome。
- 不替代 event/outbox metrics。

---

## 7. 常用 PromQL

### 7.1 Cache hit/miss

```promql
sum by (family, policy, result) (
  increase(qs_cache_get_total[5m])
)
```

### 7.2 Redis family unavailable

```promql
qs_cache_family_available == 0
```

### 7.3 Resilience decisions

```promql
sum by (component, kind, scope, resource, strategy, outcome) (
  increase(qs_resilience_decision_total[5m])
)
```

### 7.4 Queue depth

```promql
qs_resilience_queue_depth
```

### 7.5 Backpressure timeout

```promql
sum by (scope, resource, strategy) (
  increase(qs_resilience_decision_total{kind="backpressure",outcome="backpressure_timeout"}[5m])
)
```

### 7.6 Lock degraded

```promql
sum by (name, reason) (
  increase(qs_cache_lock_degraded_total[10m])
)
```

---

## 8. 新增指标 SOP

### 8.1 选择类型

| 需求 | 类型 |
| ---- | ---- |
| 某事件发生次数 | Counter |
| 当前队列长度/状态 | Gauge |
| 一次操作耗时 | Histogram |
| payload size | Histogram |
| 当前可用性 | Gauge |
| 错误分布 | Counter + bounded reason |

### 8.2 必须说明

新增指标必须说明：

1. 指标名。
2. 类型。
3. labels。
4. 是否低基数。
5. 单位。
6. 采集点。
7. 告警建议。
8. 测试覆盖。

### 8.3 禁止

- label 包含业务 ID。
- label 包含 raw error。
- label 包含 URL path 任意参数。
- 用 Gauge 记录累计次数。
- 用 Counter 记录当前状态。
- 无单位的 duration/size 指标。

---

## 9. Verify

```bash
go test ./internal/pkg/cachegovernance/observability
go test ./internal/pkg/resilienceplane
```
