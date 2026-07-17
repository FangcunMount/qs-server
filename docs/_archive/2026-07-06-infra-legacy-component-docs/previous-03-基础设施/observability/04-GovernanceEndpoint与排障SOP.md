# GovernanceEndpoint 与排障 SOP

**本文回答**：qs-server 中 governance endpoint 应该如何定义边界；哪些 endpoint 只能只读，哪些 action 必须受控；遇到 submit 429、queue full、Redis degraded、permission denied、event backlog、CPU high、外部通知失败时，应该从哪些观测入口进入；新增观测能力时需要什么门禁。

---

## 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| Governance 默认 | 只读状态入口，不是任意运维操作面 |
| 只读状态 | Redis family status、cache warmup status、hotset、resilience snapshot、event backlog summary |
| 受控 action | manual warmup、repair complete、retry/replay、release lock、clear queue 等必须独立设计权限、审计和 SOP |
| 排障主线 | 现象 → Metrics/Status/Logs/Pprof → 对应基础设施模块深讲 |
| 新增观测能力 | 必须明确指标类型、label、敏感信息、状态语义、告警和 tests |
| 解释模型观测 | 指标可按低基数 `model_type` 区分 Scale / MBTI / BigFive；Governance endpoint 可 drill down 到具体模型缓存、warmup、队列和执行状态 |
| 禁止 | 在 status endpoint 中顺手做 repair、delete、release、replay、clear |

一句话概括：

> **Governance endpoint 让状态可见；真正改变系统状态的操作必须单独成为受控治理动作。**

解释模型抽象化后，Governance endpoint 需要能区分不同模型的运行状态，例如 `model_type=scale`、`model_type=mbti`、`model_type=bigfive`。Prometheus 指标只使用低基数标签，例如 `model_type`、`provider`、`phase`；具体 `model_code`、`model_version`、`assessment_id`、`report_id` 等排障细节应进入日志、trace 或 governance drill-down，不应进入 metrics label。

---

## 1. Governance Endpoint 边界

### 1.1 允许的只读入口

- Redis runtime status。
- Redis family status。
- cache warmup snapshot。
- interpretation model cache status。
- interpretation model warmup status。
- interpretation execution snapshot。
- hotset top。
- resilience runtime snapshot。
- queue depth/status。
- interpretation queue depth/status by model_type。
- backpressure status。
- event backlog summary。
- health/degraded reason。
- version/build info。

### 1.2 受控 action

以下不能混在普通 status endpoint：

- manual cache warmup。
- repair complete warmup。
- retry outbox。
- replay event。
- clear queue。
- release lock。
- delete cache key。
- reset version token。
- drain queue。
- data repair。

如果必须提供，要求：

1. 内部/管理员权限。
2. 审计日志。
3. 参数校验。
4. dry-run 或明确确认。
5. 限制作用域。
6. 文档 SOP。
7. tests。

---

## 2. 排障入口总表

| 现象 | 第一入口 | 深入模块 |
| ---- | -------- | -------- |
| HTTP 429 | `qs_resilience_decision_total{outcome="rate_limited"}` | resilience |
| submit queue full | queue depth/status + `queue_full` outcome | resilience |
| submit processing 太久 | queue processing + logs + apiserver gRPC latency | resilience/runtime |
| Redis degraded | `qs_cache_family_available` / family status | redis |
| cache miss 激增 | `qs_cache_get_total` | redis |
| lock contention | lock acquire metrics + logs | redis/resilience |
| permission denied | AuthzSnapshot / capability logs | security |
| event backlog | outbox/worker metrics + event logs | event |
| MBTI 解释执行慢 | `qs_interpretation_execution_duration_seconds{model_type="mbti"}` + execution logs | evaluation/observability |
| 解释模型失败率高 | `qs_interpretation_failure_total{model_type,phase,reason}` | evaluation/observability |
| 解释模型列表缓存旧 | governance model cache status + warmup snapshot | redis/observability |
| MBTI 队列积压 | interpretation queue depth/status by `model_type` | event/resilience/observability |
| CPU high | pprof profile | observability/runtime |
| goroutine leak | pprof goroutine | observability/runtime |
| memory 增长 | pprof heap | observability/runtime |
| WeChat 通知失败 | notification logs + WeChat adapter logs | integrations |
| ready=false | runtime/status snapshot | redis/resilience/runtime |

---

## 3. 常见 Runbook

### 3.1 submit 返回 429

先判断来源：

1. RateLimit：看 `rate_limited`。
2. SubmitQueue：看 `queue_full`。
3. gRPC ResourceExhausted：看 SubmitGuard contention。

进入：

- `resilience/01-RateLimit入口限流.md`
- `resilience/02-SubmitQueue提交削峰.md`
- `resilience/04-LockLease幂等与重复抑制.md`

### 3.2 Redis degraded

看：

```promql
qs_cache_family_available == 0
increase(qs_cache_family_degraded_total[10m])
```

再查 governance family status：

- component。
- family。
- profile。
- namespace。
- last_error。
- consecutive_failures。

进入：

- `redis/08-观测降级与排障.md`

### 3.3 Event 没被消费

看：

- event publish logs。
- outbox backlog。
- worker Ack/Nack。
- MQ provider status。
- handler registry。

进入：

- `event/03-Worker消费与AckNack.md`
- `event/05-观测与排障.md`

### 3.4 Permission denied

看：

- Principal。
- OrgScope。
- AuthzSnapshot 是否加载。
- CapabilityDecision outcome。
- IAM resource/action。

进入：

- `security/02-AuthzSnapshot与CapabilityDecision.md`

### 3.5 CPU high

步骤：

1. 确认 `/debug/pprof` 是否可访问。
2. 采集 30 秒 CPU profile。
3. 同时看 top endpoints 和 goroutine。
4. 如果 CPU 在 JSON/Mongo/WeChat/模板解析，进入对应模块。

注意：pprof 不要公网暴露。

### 3.6 MBTI 解释执行慢

先看指标：

```promql
histogram_quantile(
  0.95,
  sum by (le, phase, result) (
    rate(qs_interpretation_execution_duration_seconds_bucket{model_type="mbti"}[5m])
  )
)
```

再看失败分布：

```promql
sum by (model_type, phase, reason) (
  increase(qs_interpretation_failure_total{model_type="mbti"}[10m])
)
```

然后进入 governance drill-down：

```text
model_type=mbti
provider=mbti
phase=load_context / evaluate / build_report
cache target=static.mbti_model_list / static.interpretation_model
queue status=pending / inflight / failed
```

排查路径：

1. `load_context` 慢：看 Redis Context cache、模型 repository、Mongo 查询。
2. `evaluate` 慢：看 Provider 执行日志和规则复杂度。
3. `build_report` 慢：看 report template、object storage、render pipeline。
4. 队列积压：看 worker concurrency、event backlog、resilience backpressure。
5. 失败率高：看 `reason` 是否集中在 `provider_not_found`、`context_load_failed`、`rule_invalid`。

注意：不要把 `model_code`、`model_version`、`assessment_id` 放进 Prometheus label。具体模型和单次 Assessment 排障走日志、trace 或 governance drill-down。

### 3.7 解释模型列表缓存旧

先看 governance endpoint：

```text
static.interpretation_model_list
static.mbti_model_list
static.interpretation_model
```

重点字段：

```text
model_type
provider
target_kind
status
last_warmup_result
last_warmup_at
last_error_code
```

排查路径：

1. `report.changed` 或具体模型发布/归档事件是否出站。
2. list cache `Rebuild` 是否执行。
3. Redis `static_meta` family 是否 available。
4. repository / read model 是否能查到 published model summaries。
5. LocalHotCache 是否 reset。
6. 是否把完整 TypeProfile / DimensionRule 放进 StaticList，导致 payload 过大。

### 3.8 MBTI 队列积压

先看 queue status：

```text
model_type=mbti
pending_count
inflight_count
failed_count
last_success_at
last_failure_at
```

再看：

```promql
sum by (stage, result) (
  increase(qs_evaluation_lifecycle_total[10m])
)
```

如果 `assessment.completed` 正常增长，但 `interpretation.completed` 不增长，说明卡在解释执行阶段。

如果 `interpretation.completed` 正常增长，但 `report.generated` 不增长，说明卡在报告生成阶段。

---

## 4. 新增观测能力 SOP

### 4.1 新增 metrics

必须：

1. 选择 Counter/Gauge/Histogram。
2. 指标名带 `qs_`。
3. label 低基数。
4. 明确单位。
5. 补 tests。
6. 更新文档。
7. 更新 dashboard/alert。
8. 如果是解释模型指标，优先使用低基数 `model_type`、`provider`、`phase`。
9. 不要把 `model_code`、`model_version`、`assessment_id`、`report_id` 放进 label。
10. 如果需要具体模型 drill-down，设计 governance endpoint 或日志字段，不要污染 metrics label。

### 4.2 新增 health/readiness check

必须：

1. 明确 liveness 还是 readiness。
2. 不做重操作。
3. 使用 cached snapshot。
4. 明确 degraded 是否影响 ready。
5. 补 tests/docs。

### 4.3 新增 log field

必须：

1. 字段名稳定。
2. 不含敏感信息。
3. 高基数字段谨慎。
4. request correlation 清晰。
5. 文档同步。

### 4.4 新增 audit event

必须：

1. 明确 actor。
2. operation。
3. resource。
4. scope。
5. result/decision。
6. reason。
7. 脱敏策略。
8. 保存策略。

### 4.5 新增 governance endpoint

必须：

1. 默认只读。
2. action 和 status 分离。
3. 参数范围受控。
4. 鉴权。
5. 审计。
6. tests。
7. SOP。
8. 如果涉及解释模型，状态必须能区分 `model_type` 和 `provider`。
9. 如果涉及缓存，状态必须能区分 `target_kind`，例如 `static.mbti_model_list`、`static.interpretation_model_list`。
10. 如果涉及队列，状态必须能区分 `pending_count`、`inflight_count`、`failed_count`。
11. drill-down 可以包含 `model_code` / `model_version`，但 summary 不能变成高基数指标。

### 4.6 新增解释模型观测入口

新增 MBTI / BigFive 等解释模型后，建议至少提供三类观测入口。

#### Metrics

```text
qs_interpretation_execution_total{model_type,provider,phase,result}
qs_interpretation_execution_duration_seconds{model_type,provider,phase,result}
qs_interpretation_failure_total{model_type,provider,phase,reason}
qs_evaluation_lifecycle_total{stage,result}
```

示例：

```text
qs_interpretation_execution_duration_seconds{model_type="mbti",provider="mbti",phase="evaluate",result="success"}
```

#### Governance endpoint

```text
/static.interpretation_model_list/status
/static.mbti_model_list/status
/interpretation/execution/status?model_type=mbti
/interpretation/queue/status?model_type=mbti
```

具体路径以实际 router 为准。

#### Logs / Trace

日志和 trace 可以包含：

```text
model_type
model_code
model_version
assessment_id
answer_sheet_id
report_id
provider
phase
reason
```

但这些高基数字段不应进入 Prometheus label。

---

## 5. 反模式

| 反模式 | 后果 |
| ------ | ---- |
| status endpoint 顺手清缓存 | 运维误操作 |
| governance endpoint 释放任意 lock | 并发安全破坏 |
| metrics label 放 userID | 高基数爆炸 |
| metrics label 放 model_code/model_version | 模型版本一多就高基数爆炸 |
| metrics label 放 assessment_id/report_id | 每次测评产生新时序，Prometheus 压力暴涨 |
| 用 MBTI type_code 做 metrics label | 类型分布应进 Statistics ReadModel，不进 Prometheus label |
| healthz 实时 ping 所有下游 | 健康检查放大故障 |
| pprof 公网开放 | 安全风险 |
| audit 记录完整敏感 payload | 合规风险 |
| 日志打印 token | 严重泄漏 |
| 观测没有 tests | 文档和代码漂移 |

---

## 6. 合并前检查清单

| 检查项 | 是否完成 |
| ------ | -------- |
| 指标 label 低基数 | ☐ |
| 如为解释模型指标，已确认只使用 `model_type` / `provider` / `phase` 等低基数 label | ☐ |
| 如需具体模型排障，已设计 governance drill-down 或日志字段 | ☐ |
| Governance endpoint 能区分不同模型的缓存和队列状态 | ☐ |
| 没有敏感信息 | ☐ |
| 状态 endpoint 只读 | ☐ |
| action endpoint 有鉴权和审计 | ☐ |
| health check 不做重操作 | ☐ |
| pprof 访问风险已说明 | ☐ |
| logs 字段规范 | ☐ |
| docs 更新 | ☐ |
| tests 更新 | ☐ |

---

## 7. Verify

```bash
go test ./internal/pkg/server
go test ./internal/pkg/cachegovernance/observability
go test ./internal/pkg/resilience
go test ./internal/apiserver/application/evaluation/...
go test ./internal/apiserver/application/cachegovernance
```

如果修改文档：

```bash
make docs-hygiene
git diff --check
```
