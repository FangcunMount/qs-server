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
| `qs_interpretation_*` | 解释模型执行、失败、耗时、Provider 维度 |
| `qs_governance_*` | Governance endpoint / warmup / cache target / model list 状态 |
| `gin_*` | gin-prometheus HTTP metrics |

一句话概括：

> **Metrics 负责趋势和告警；业务定位细节进日志，不进 label。**

解释模型抽象化后，指标应能按 `model_type` 区分 Scale、MBTI、BigFive 等模型，但不能把 `model_code`、`assessment_id`、`answer_sheet_id`、`report_id` 等高基数字段放入 label。模型级排障细节应进入日志、trace 或治理接口详情。

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
qs_interpretation_execution_total
qs_interpretation_execution_duration_seconds
qs_governance_target_status
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
- model_type。
- provider。
- phase。
- target_kind。

禁止作为 label：

- userID。
- requestID。
- taskID。
- assessmentID。
- answerSheetID。
- modelCode。
- modelVersion。
- reportID。
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

## 6. Interpretation / Evaluation 指标

解释模型抽象化后，Evaluation 不应只暴露“量表评估”指标，而应按通用解释模型链路暴露指标。

### 6.1 Interpretation execution total

```text
qs_interpretation_execution_total{model_type,provider,phase,result}
```

label 说明：

| Label | 示例 | 说明 |
| ----- | ---- | ---- |
| `model_type` | `scale` / `mbti` / `bigfive` | 低基数模型类型 |
| `provider` | `scale` / `mbti` | Provider 名称，必须低基数 |
| `phase` | `load_context` / `evaluate` / `build_report` | 执行阶段 |
| `result` | `success` / `failed` / `skipped` | 执行结果 |

用途：

- 观察不同解释模型执行量。
- 观察 MBTI / Scale / BigFive 的失败比例。
- 判断失败集中在 context load、evaluate 还是 report build。

### 6.2 Interpretation execution duration

```text
qs_interpretation_execution_duration_seconds{model_type,provider,phase,result}
```

示例：

```text
qs_interpretation_execution_duration_seconds{model_type="mbti",provider="mbti",phase="evaluate",result="success"}
```

用途：

- 观察解释模型执行耗时。
- 比较 Scale 与 MBTI 等不同 Provider 的耗时差异。
- 排查某个阶段慢，例如 Context cache miss 导致 `load_context` 慢。

### 6.3 Interpretation failure total

```text
qs_interpretation_failure_total{model_type,provider,phase,reason}
```

`reason` 必须是有界枚举，例如：

```text
provider_not_found
context_load_failed
questionnaire_mismatch
rule_invalid
evaluate_failed
report_build_failed
```

不要把原始错误文本放进 `reason`。

### 6.4 Evaluation lifecycle total

```text
qs_evaluation_lifecycle_total{stage,result}
```

stage 示例：

```text
assessment_created
assessment_completed
interpretation_completed
interpretation_failed
report_generated
```

用途：

- 观察从答卷提交到报告生成的阶段漏斗。
- 配合 event / outbox 指标判断卡点。
- 不绑定具体解释模型。

### 6.5 禁止的解释模型 label

禁止：

```text
model_code
model_version
assessment_id
answer_sheet_id
report_id
user_id
type_code
```

说明：

- `model_type=mbti` 可以作为 label。
- `model_code=MBTI_STANDARD` 不建议作为 metrics label。
- MBTI 的 TypeCode 分布属于 Statistics ReadModel，不适合放进 Prometheus label。

---

## 7. HTTP metrics

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

## 8. 常用 PromQL

### 8.1 Cache hit/miss

```promql
sum by (family, policy, result) (
  increase(qs_cache_get_total[5m])
)
```

### 8.2 Redis family unavailable

```promql
qs_cache_family_available == 0
```

### 8.3 Resilience decisions

```promql
sum by (component, kind, scope, resource, strategy, outcome) (
  increase(qs_resilience_decision_total[5m])
)
```

### 8.4 Queue depth

```promql
qs_resilience_queue_depth
```

### 8.5 Backpressure timeout

```promql
sum by (scope, resource, strategy) (
  increase(qs_resilience_decision_total{kind="backpressure",outcome="backpressure_timeout"}[5m])
)
```

### 8.6 Lock degraded

```promql
sum by (name, reason) (
  increase(qs_cache_lock_degraded_total[10m])
)
```

### 8.7 MBTI interpretation duration

```promql
histogram_quantile(
  0.95,
  sum by (le, phase, result) (
    rate(qs_interpretation_execution_duration_seconds_bucket{model_type="mbti"}[5m])
  )
)
```

### 8.8 Interpretation failures by model type

```promql
sum by (model_type, phase, reason) (
  increase(qs_interpretation_failure_total[10m])
)
```

### 8.9 Evaluation lifecycle funnel

```promql
sum by (stage, result) (
  increase(qs_evaluation_lifecycle_total[10m])
)
```

---

## 9. Governance endpoint 与模型维度

Governance endpoint 不是 Prometheus 指标，但它必须能辅助区分不同模型的缓存、warmup 和队列状态。

建议治理状态包含：

```text
family
profile
target_kind
model_type
provider
status
last_run_at
last_error_code
```

### 9.1 缓存状态

解释模型缓存状态应能区分：

```text
static.interpretation_model_list
static.mbti_model_list
static.interpretation_model
query.interpretation_model_distribution
query.mbti_type_distribution
```

建议展示字段：

```text
target_kind
model_type
status
last_warmup_result
last_warmup_at
last_error_code
```

不要在 governance summary 中直接展开全部 `model_code` / `model_version` 明细。

如果需要查看某个具体模型的排障详情，应走 drill-down endpoint、日志或 trace，而不是 Prometheus label。

### 9.2 队列和执行状态

对于解释模型相关队列或执行状态，Governance endpoint 应区分：

```text
model_type=scale
model_type=mbti
model_type=bigfive
```

可展示：

```text
pending_count
inflight_count
failed_count
last_success_at
last_failure_at
```

但不要使用 `assessment_id`、`answer_sheet_id`、`report_id` 作为聚合维度。

---

## 10. 新增指标 SOP

### 10.1 选择类型

| 需求 | 类型 |
| ---- | ---- |
| 某事件发生次数 | Counter |
| 当前队列长度/状态 | Gauge |
| 一次操作耗时 | Histogram |
| payload size | Histogram |
| 当前可用性 | Gauge |
| 错误分布 | Counter + bounded reason |

### 10.2 必须说明

新增指标必须说明：

1. 指标名。
2. 类型。
3. labels。
4. 是否低基数。
5. 单位。
6. 采集点。
7. 告警建议。
8. 测试覆盖。
9. 如果涉及解释模型，说明是否使用 `model_type` / `provider` / `phase`，以及为什么这些 label 是低基数。
10. 如果涉及 Governance endpoint，说明 metrics 与 endpoint drill-down 的边界。

### 10.3 禁止

- label 包含业务 ID。
- label 包含 raw error。
- label 包含 URL path 任意参数。
- 用 Gauge 记录累计次数。
- 用 Counter 记录当前状态。
- 无单位的 duration/size 指标。
- label 包含 model_code / model_version。
- label 包含 MBTI type_code。
- 用 Prometheus label 承载模型详情排障。
- 用 Governance endpoint 的 drill-down 字段反向污染 metrics label。

---

## 11. 解释模型指标检查清单

新增解释模型指标前，逐项检查：

| 检查项 | 是否完成 |
| ------ | -------- |
| 是否能用 `model_type` 表达模型维度 | ☐ |
| 是否避免了 `model_code` / `model_version` label | ☐ |
| 是否区分 Provider 和 phase | ☐ |
| 失败 reason 是否是有界枚举 | ☐ |
| MBTI TypeCode 是否进入 Statistics ReadModel，而不是 Prometheus label | ☐ |
| Governance endpoint 是否能 drill down 到具体模型 | ☐ |
| PromQL 示例是否不依赖高基数 label | ☐ |

---

## 12. Verify

```bash
go test ./internal/pkg/cachegovernance/observability
go test ./internal/pkg/resilience
go test ./internal/apiserver/application/evaluation/...
go test ./internal/apiserver/application/cachegovernance
```
