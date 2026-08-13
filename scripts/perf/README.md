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

120～280 不保存独立 profile。编排器固定链路探针为 1 QPS，其余流量按 300 配比使用最大余数法整数缩放，确保总量精确。VU 按到达率、典型耗时、超时和 headroom 计算，并为 HTTP 到达率场景预留 `startupBuffer` 个冷启动 VU；环境变量仍可显式覆盖。

## 三维指标契约

### 吞吐与处理能力

| 指标 | 定义 |
| --- | --- |
| `business_qps` | k6 业务 iteration 到达率；同时展示目标、实际、达成率和 dropped iterations |
| `http_rps` | 真实 HTTP 请求速率；包括 HTTP 报告轮询产生的请求 |
| `ws_sessions_per_second` | WebSocket 新建会话速率，不混入 HTTP RPS |
| `accepted_tps` | 每秒被可靠受理的答卷数，按总量和模型类型展示 |
| `completed_tps` | 服务端成功 Interpretation Run 增量除以阶段时长，按总量和模型类型展示 |
| `expected_completions` | assessment intake 在观测窗口内新建的 Assessment 数；排除独立问卷和幂等重放 |
| `no_assessment_required` | assessment intake 正常结束且无需创建 Assessment 的独立问卷受理量 |
| `final_completion_rate` | 服务端完成增量除以 `expected_completions`，不得使用全部受理量作为分母 |
| `request_amplification` | HTTP RPS ÷ business QPS |
| `polling_amplification` | 报告轮询请求数 ÷ 链路探针初始数，不计作重试 |

异步场景必须区分受理 TPS、应完成受理量与完成 TPS。缺少服务端完成证据或 assessment intake outcome 证据时，最终完成率是 `N/A`，不得用全部受理量或链路探针估算。

WebSocket 同时记录端到端 `report_ws_first_message_latency` 与握手后 `report_ws_subscribe_to_first_message_latency`。后者从客户端发送 subscribe 帧起计时，只用于定位首帧慢在公网握手还是服务端状态读取，不单独改变验收阈值。

按模型展示 subscribe-to-message 时延使用三个独立的诊断 Trend，避免 K6 对无阈值 tagged submetric 不导出而产生伪 `N/A`。

`dropped_iterations` 保留全局 `count==0` 硬门，同时为每个活动 scenario 生成相同的带 `scenario` 标签子阈值。任何一次丢迭代仍会让阶段失败，子阈值只负责指出发生在哪条流量链路。

### 时延与响应体验

每个活动关键操作和端到端探针在终端和 Markdown 主表统一输出：样本数、P50、P90、P95、P99。最大耗时和平均耗时继续保留在 `summary.json`，用于诊断而不挤占主结果表。

`experience` 体验线：

| 操作 | P95 | P99 |
| --- | ---: | ---: |
| 查询 | < 300ms | < 500ms |
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

P50/P90 是体验分布观察指标；P95/P99 是体验或保护门禁。最大耗时达到请求超时后，由超时门禁判定。

### 可靠性与正确性

统一以初始操作数作为分母：

- `success_rate = 成功业务结果 / 初始操作数`；
- `error_rate = 失败业务结果 / 初始操作数`，包含超时；
- `timeout_rate = 超时结果 / 初始操作数`，是错误率子集；
- `final_failure_rate = 重试后仍失败 / 初始操作数`。

报告同时输出成功、错误、超时的数量和比例。最终成功但经历服务端重试的事务仍计为成功，重试成本单列。

#### 分层重试

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

## 结果展示契约

终端阶段摘要和 `report.md` 使用相同的三个一级维度，不再把重试拆成第四个维度：

1. 吞吐与处理能力：目标/实际 QPS、达成率、dropped iterations、HTTP/WS 速率、受理/完成 TPS；
2. 时延与响应体验：各操作样本数与 P50/P90/P95/P99；
3. 可靠性与正确性：各操作成功率、错误率、超时率，以及按层级统计的重试率。

`report.md` 顶部先按三个维度给出跨阶段总览，随后按阶段展开同一结构。排队、服务端证据和原始阈值放在附录，不与三类业务结果并列。`summary.json` 继续保存完整机器证据和来源。

K6 原生尾部的完整指标全集不再重复打印到终端；`handleSummary()` 将其标准化后写入阶段的 `raw-k6-summary.json`。K6 运行进度仍可见，阶段结束后由 `perfctl` 先输出三维结果，再在底部追加精简的“原生运行诊断”。诊断先按 `QUERY / 查询`、`SUBMIT / 提交`、`SESSION / 会话`、`WEBSOCKET / 报告订阅`、`STATISTICS / 统计` 和 `ASYNC CHAIN / 异步链路` 展示每个活动接口的原始时延、样本速率、成功/错误/超时及可用的失败类型；随后展示 K6 WebSocket 内置指标、总运行时长/当前与峰值 VU/迭代/dropped 状态，以及按场景列出的 pre/max VU、持续时间、目标速率和 dropped。该区块是运行诊断附录，不新增第四个验收维度；`handleSummary()` 未提供的 interrupted iterations 明确显示为 `N/A`。

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

受控窗口必须显式设置 `PERF_ISOLATED_ENV=true`，但该变量只代表操作者声明，不能单独形成通过证据。collection-server 与 qs-apiserver 通过 `qs_perf_traffic_requests_total{origin="perf|other"}` 对业务请求做有界分类：携带 `X-Perf-Run-ID` 的 k6 请求属于 `perf`，其余业务请求属于 `other`；健康检查、readyz、metrics、ping、version 与 pprof 不计入。编排器比较阶段前后 `origin="other"` 增量，只有声明为隔离且增量为 0 才判定隔离通过；增量大于 0 判定失败，运行版本尚未暴露该指标则判定 `INCOMPLETE`。完成 TPS 使用 Interpretation 成功计数在两次 Prometheus 快照之间的增量，并以实际快照时间窗作为分母，不使用计划时长代替观测窗口。

WebSocket 报告场景不再随机争抢报告样本。所有活动 WS scenario 按 testee ID 稳定分片，每个 testee 只归属一个活动 scenario；scenario 内再按 `iterationInTest` 轮转，避免医疗、行为和人格报告并发复用同一 testee 而误触 `max_per_testee`。实际失败会话只增加一次 `report_status_failed`，并在原生诊断中按容量拒绝、限流、协议、传输、握手、缺少消息和服务端拒绝分类。

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
