# K6 压测与 Admission 设计

## 结论

K6 主线只有一个入口和四种计划：

```bash
make perf-run PLAN=quick
make perf-run PLAN=baseline
make perf-run PLAN=admission
make perf-run PLAN=diagnose CASE=<专项场景>
```

旧的逐档 Make 命令和重复 QPS profiles 已删除。`quick`、`baseline`、`admission` 由 Go 编排器分阶段调用 k6；每个阶段单独判定，硬门禁失败立即停止。

压测结论必须同时回答三个问题：

1. 系统每秒受理并完成多少工作；
2. 典型用户与尾部用户等待多久；
3. 压力下结果是否正确、稳定且能够恢复。

只写“达到 300 QPS”不是完整验收结论。

## 计划与阶段

| PLAN | 阶段 | 用途 |
| --- | --- | --- |
| `quick` | smoke 4 QPS × 30s | 配置、鉴权与全链路连通性 |
| `baseline` | smoke + experience 60 QPS × 5min | 正常负载体验基线 |
| `admission` | smoke → 60 → 120 → 200 → 240 → 280 → 恢复门 → 300 → 最终恢复门 | 正式容量准入 |
| `diagnose` | 一个注册专项 | 故障、降级、幂等或 gRPC 专项 |

Admission 的容量阶段持续时间固定为：

| 阶段 | 目标 | 时长 |
| --- | ---: | ---: |
| `capacity_120` | 120 QPS | 2min |
| `capacity_200` | 200 QPS | 3min |
| `capacity_240` | 240 QPS | 4min |
| `capacity_280` | 280 QPS | 3min |
| `admission_300` | 300 QPS | 10min |

300 QPS 的正式配比是：医疗模型查询 80、人格模型查询 40、两类问卷各 13、医疗/人格提交 19/5、医疗/行为/人格报告 70/10/20、Statistics 29、链路探针 1。

120～280 不保存独立 profile。编排器固定链路探针为 1 QPS，其余流量按 300 配比使用最大余数法整数缩放，确保总量精确。VU 按到达率、典型耗时、超时和 headroom 计算；环境变量仍可显式覆盖。

## 三维指标契约

### 吞吐与处理能力

| 指标 | 定义 |
| --- | --- |
| `business_qps` | k6 业务 iteration 到达率；同时展示目标、实际、达成率和 dropped iterations |
| `http_rps` | 真实 HTTP 请求速率；包括 HTTP 报告轮询产生的请求 |
| `ws_sessions_per_second` | WebSocket 新建会话速率，不混入 HTTP RPS |
| `accepted_tps` | 每秒被可靠受理的答卷数，按总量和模型类型展示 |
| `completed_tps` | 服务端成功 Interpretation Run 增量除以阶段时长，按总量和模型类型展示 |
| `final_completion_rate` | 服务端完成增量除以受理量 |
| `request_amplification` | HTTP RPS ÷ business QPS |
| `polling_amplification` | 报告轮询请求数 ÷ 链路探针初始数，不计作重试 |

异步场景必须区分受理 TPS 与完成 TPS。缺少服务端完成证据时，完成 TPS 是 `N/A`，不得用链路探针估算。

### 时延与响应体验

每个活动关键操作和端到端探针统一输出：样本数、P50、P95、P99、最大耗时和平均耗时。

`experience` 体验线：

| 操作 | P95 | P99 |
| --- | ---: | ---: |
| 查询 | < 200ms | < 500ms |
| 答卷受理 | < 300ms | < 800ms |
| WS 握手与首帧 | < 300ms | < 800ms |
| Statistics | < 700ms | < 1.5s |

`protection` 保护线：

| 操作 | P95 | P99 |
| --- | ---: | ---: |
| 查询 | < 500ms | < 1.2s |
| 答卷受理 | < 500ms | < 1s |
| WS 握手与首帧 | < 500ms | < 1.2s |
| Statistics | < 1s | < 2s |

P50、平均值和最大耗时当前是观察指标；P95/P99 是体验或保护门禁。最大耗时达到请求超时后，由超时门禁判定。

### 可靠性与正确性

统一以初始操作数作为分母：

- `success_rate = 成功业务结果 / 初始操作数`；
- `error_rate = 失败业务结果 / 初始操作数`，包含超时；
- `timeout_rate = 超时结果 / 初始操作数`，是错误率子集；
- `final_failure_rate = 重试后仍失败 / 初始操作数`。

报告同时输出成功、错误、超时的数量和比例。最终成功但经历服务端重试的事务仍计为成功，重试成本单列。

## 分层重试

服务端增量暴露：

```text
qs_retry_layer_attempt_total{
  layer,
  component,
  attempt_class,
  origin,
  outcome
}
```

`layer` 只允许 `business | outbox | hold | transport`；`attempt_class` 只允许 `initial | retry`。业务 origin 使用 `initial | automatic | manual | force | lease_recovery`，其他层固定为 `na`。

报告分别展示 k6 客户端、业务、Outbox、retry-hold 和 MQ transport 重试。`retry_rate = retry attempts / initial attempts`，允许超过 100%；初始尝试为 0 时显示 `N/A`。本版本只记录基线，不设置重试硬阈值。

## 报告契约

每次运行写入 `tmp/perf/runs/<run-id>/`：

| 文件 | 用途 |
| --- | --- |
| `report.md` | 评审用的人类可读报告 |
| `summary.json` | `qs-perf-report/v1` 机器契约 |
| `raw-k6-summary.json` | 各阶段未经归一化的 k6 汇总 |
| `evidence.json` | readyz、Prometheus、NSQD 与恢复证据 |
| `<phase>/...` | 每个阶段自己的 raw、summary、report 和 evidence |

所有标准化数值包含单位、样本数和来源。没有证据使用 `null`/`N/A`，不伪装成 0。

Verdict 与退出码：

| Verdict | 退出码 | 含义 |
| --- | ---: | --- |
| `PASS` | 0 | 负载、时延、正确性与证据门通过 |
| `FAIL` | 2 | 业务硬门禁失败 |
| `INCOMPLETE` | 3 | Prometheus、readyz、Worker、Outbox 或 NSQD 等证据不足 |
| `ERROR` | 4 | 配置、脚本、k6 或编排异常 |

上表是 `perfctl` 的退出码；GNU Make 对失败 recipe 统一返回自己的非零状态（通常为 2）。CI 通过 `summary.json.verdict` 区分失败类型，不依赖 Make 转发子进程的精确数值。

## Admission 硬门禁

- 每个活动关键操作成功率 `> 99%`；
- 每个活动关键操作错误率 `< 1%`；
- 全局 HTTP 超时率 `< 0.1%`；
- 答卷提交和端到端链路探针超时数为 0；
- `dropped_iterations == 0`；
- 实际 business QPS 至少达到目标的 99%；
- 对应 tier 的 P95/P99 全部通过；
- 进入 300 前，smoke、60、120、200、240、280 必须全部为 `PASS`，且 collection、apiserver、worker、Outbox 和 NSQ 恢复证据通过；
- 300 后必须完成最终排空与恢复验收。

服务端证据缺失会产生 `INCOMPLETE`。在无法隔离并发业务流量的环境中，完成 TPS 与最终完成率不可作为正式准入证据。

受控窗口必须显式设置 `PERF_ISOLATED_ENV=true`；未声明或声明为非隔离环境时，报告会保留观测值但将服务端证据标记为 `INCOMPLETE`。完成 TPS 使用 Interpretation 成功计数在两次 Prometheus 快照之间的增量，并以实际快照时间窗作为分母，不使用计划时长代替观测窗口。

NSQD 报告优先按 channel depth 统计待消费工作；没有 channel 时才使用 topic depth，避免 topic 与 channel 重复累计。共享 NSQD 可设置 `PERF_NSQ_TOPICS=topic-a,topic-b`，将恢复证据限定到本次压测相关 topic；配置后没有匹配 topic 会标记为证据缺失。

## 专项注册表

非法或缺失 CASE 会输出当前可选项。常用示例：

```bash
make perf-run PLAN=diagnose CASE=submit-coalescing-healthy
make perf-run PLAN=diagnose CASE=submit-redis-degraded-low
make perf-run PLAN=diagnose CASE=collection-runtime-recovery
make perf-run PLAN=diagnose CASE=grpc
```

诊断专项一次只运行一个，不进入正式升档链。

## 本地验证边界

```bash
make perf-verify
make perf-run PLAN=admission DRY_RUN=1
```

`perf-verify` 只证明 Go 编排与报告单测通过、脚本可解析、三个正式 profile 及阈值契约一致。`DRY_RUN=1` 只展示阶段和生成有效配置。两者都不证明真实环境已通过 60 或 300 QPS；正式结论必须来自受控非生产窗口的完整 run 目录。
