# K6 混合场景 Admission 压测 SOP

## 30 秒结论

正式压测只有一个入口：

```bash
make perf-run PLAN=quick
make perf-run PLAN=baseline
make perf-run PLAN=admission
make perf-run PLAN=diagnose CASE=<专项场景>
```

`admission` 自动执行 smoke、60、120、200、240、280、恢复证据门、300 和最终排空验收。操作者不再逐档拼命令；任何硬门禁失败立即停止，证据不足时标记 `INCOMPLETE` 并禁止进入 300。

本地静态检查或 dry-run 不能称为 300 QPS 验收成功。正式结论必须引用本次 run ID、Git SHA、`summary.json`、`report.md` 和 `evidence.json`。

## 一、运行前准备

### 1. 工具与配置

- k6 版本不低于 v1.5.0；
- 安装 Go、jq、curl；
- 压测只在允许写入测试数据的受控非生产环境执行；
- `tmp/perf/qs-perf.config.json` 中 URL、token 文件、组织与模型配置指向本次环境；
- collection、worker 的聚合 readiness 与 Prometheus federation 指标可从编排机访问；
- apiserver 的 `/readyz` 与 `/metrics` 可从编排机访问；
- NSQD `/stats?format=json` 可访问；
- 环境中没有无法区分的并发业务流量，否则完成 TPS 证据不具备准入效力。

初始化并刷新 token：

```bash
make perf-init
make perf-tokens
make perf-preflight
make perf-verify
```

`perf-init` 保留本地 URL 与凭据，但会把主线 profiles 同步为 `smoke_4`、`experience_60`、`admission_300`。旧 profile 和旧 Make 命令不再受支持。

### 2. 观测地址

本机默认地址如下；生产环境必须使用聚合端点，不能把多副本服务的普通
Nginx 轮询 `/metrics`、`/readyz` 当成全体副本证据：

```bash
export COLLECTION_METRICS_URL=https://collect.fangcunmount.cn/perf/metrics
export COLLECTION_READYZ_URL=https://collect.fangcunmount.cn/perf/readyz
export APISERVER_METRICS_URL=https://qs.fangcunmount.cn/metrics
export APISERVER_READYZ_URL=https://qs.fangcunmount.cn/readyz
export WORKER_METRICS_URL=https://worker.fangcunmount.cn/metrics
export WORKER_READYZ_URL=https://worker.fangcunmount.cn/readyz
export NSQD_STATS_URL=https://nsqd.fangcunmount.cn/stats
export EXPECTED_COLLECTION_REPLICAS=2
export EXPECTED_WORKER_REPLICAS=3
export PERF_ISOLATED_ENV=true
# 共享 NSQD 时建议限定本次链路涉及的 topic
# export PERF_NSQ_TOPICS='<topic-a>,<topic-b>'
# 可选诊断，不参与正式证据完整性门禁
# export NSQLOOKUPD_NODES_URL=https://nsqd.fangcunmount.cn/nodes
```

`/perf/metrics` 与 worker `/metrics` 由 Prometheus federation 返回所有目标并保留
`job`、`instance` 标签；对应 ready 端点返回同时满足 Prometheus `up` 和
`qs_runtime_component_ready=1` 的实例数量。perfctl 会将数量与两个
`EXPECTED_*_REPLICAS` 变量比较，并拒绝没有 `instance` 标签的普通直连指标。
worker、NSQD 域名须解析到 serverA Nginx；NSQ 公网入口只允许读取
`/stats`、`/ping`、`/nodes`，不得开放 topic/channel 管理接口。

只有确认窗口内的完成量可归因于本次压测时，才设置 `PERF_ISOLATED_ENV=true`；未设置或显式设为其他值都会把证据标记为 `INCOMPLETE`。完成 TPS 使用阶段前后 Interpretation Prometheus 计数增量，并以这两次指标快照的实际时间窗为分母，不以配置中的计划时长代替服务端观测窗口。

NSQD depth 按 channel 待消费工作求和；topic 没有 channel 时才使用 topic depth，避免重复累计。共享 NSQD 必须通过 `PERF_NSQ_TOPICS` 限定可解释的 topic 范围，配置后没有匹配 topic 会视为证据缺失。

恢复门默认最多等待 5 分钟、每 10 秒检查一次。确有环境依据时可设置：

```bash
export PERF_RECOVERY_TIMEOUT=8m
export PERF_RECOVERY_POLL=15s
```

不要为了让结果变绿而随意扩大恢复窗口；变更必须写入跑次说明。

### 3. 只检查编排计划

```bash
make perf-run PLAN=admission DRY_RUN=1
```

dry-run 会生成本次 effective config 并列出阶段，不发压测流量。

## 二、选择执行计划

### Quick

```bash
make perf-run PLAN=quick
```

用于日常连通性检查。它不替代体验基线或容量验收。

### Baseline

```bash
make perf-run PLAN=baseline
```

依次运行 smoke 与 60 QPS/5 分钟体验档。发布前性能回归、依赖变更或正式 admission 前应先获得有效 baseline。

### Admission

```bash
make perf-run PLAN=admission
```

固定阶段：

| 顺序 | 阶段 | 时长 | 说明 |
| ---: | --- | ---: | --- |
| 1 | smoke 4 QPS | 30s | 连通性 |
| 2 | experience 60 QPS | 5min | 用户体验线 |
| 3 | capacity 120 QPS | 2min | 渐进容量 |
| 4 | capacity 200 QPS | 3min | 渐进容量 |
| 5 | capacity 240 QPS | 4min | 渐进容量 |
| 6 | capacity 280 QPS | 3min | 300 前证据阶段 |
| 7 | 恢复门 | 最多 5min | 健康、Outbox、NSQ 必须回落 |
| 8 | admission 300 QPS | 10min | 正式保护线 |
| 9 | 最终恢复门 | 最多 5min | 排空与残留验收 |

120～280 由 300 配比动态缩放，链路探针始终是 1 QPS，总 QPS 精确等于阶段目标。

### Diagnose

```bash
make perf-run PLAN=diagnose CASE=submit-coalescing-healthy
```

缺失或非法 CASE 时，命令会列出注册专项并终止。专项一次只运行一个，不得拿专项结果替代 admission。

## 三、三维验收顺序

### 1. 先确认负载真实达到

先看：

- business QPS 目标、实际与达成率；
- `dropped_iterations` 是否为 0；
- 实际持续时间是否足够；
- HTTP RPS 与 business QPS 的请求放大是否合理；
- WS sessions/s 是否符合报告流量结构。

目标 300 QPS 但实际只有 250 QPS，即使错误率和延迟很好，也不能判为 300 QPS 通过。

### 2. 吞吐与处理能力

同步请求关注 business QPS 和 HTTP RPS。异步答卷链路必须同时关注：

- 受理 TPS：答卷是否可靠进入系统；
- 完成 TPS：成功 Interpretation Run 是否持续跟上；
- 最终完成率：目标窗口内完成量占受理量的比例；
- Outbox backlog、最老待处理年龄与 NSQ depth 是否持续增长；
- 压测停止后积压是否回到基线。

受理 TPS 高但完成 TPS 低，只能说明入口能接收，不能说明系统完成了业务交付。

### 3. 时延与响应体验

每个活动操作查看样本数、P50、P95、P99、最大耗时和平均耗时：

- P50：典型用户体验；
- P95：大多数用户体验；
- P99：尾部慢请求与高压风险；
- 最大耗时：极端异常样本；
- 排队等待：Outbox/异步链路饱和信号。

60 QPS 使用 `experience` 体验目标；120～300 使用 `protection` 高压保护线。P50 正常而 P99 明显劣化时，优先检查排队、慢 SQL、连接池与下游抖动。

### 4. 可靠性与正确性

每个活动关键操作必须同时查看样本数、成功/错误/超时数量和比例：

- 成功率 `> 99%`；
- 错误率 `< 1%`，超时包含在错误内；
- 全局 HTTP 超时率 `< 0.1%`；
- 答卷提交超时数为 0；
- 端到端链路探针超时数为 0；
- 最终失败率与幂等专项结果符合场景目标。

重试按 client、business、outbox、hold、transport 分层展示，不计算含义不清的“总重试率”。重试率高但最终成功，说明系统靠重试恢复，仍需要定位瞬态故障；重试率与最终失败率同时升高，说明恢复机制无效。

## 四、Verdict 与停止规则

| Verdict | 处理方式 |
| --- | --- |
| `PASS` | 当前阶段通过，可以继续 |
| `FAIL` | 硬门禁越界，立即停止后续升档 |
| `INCOMPLETE` | 服务端或恢复证据不完整；不得进入 300 |
| `ERROR` | 配置、脚本或 k6 运行异常；本次跑次无效 |

常见 `INCOMPLETE` 原因：

- readyz 或 metrics 端点不可达；
- Worker 指标缺失；
- `qs_retry_layer_attempt_total` 未部署到所有相关进程；
- Interpretation 完成计数无法取得阶段前后增量；
- NSQD 证据缺失；
- Outbox backlog 或最老年龄无法解析；
- 环境混有无法隔离的业务流量，完成 TPS 无法归因。

重试率本版不做硬门禁。至少积累三次相同环境、相同计划的有效 admission 结果后，再单独评审阈值。

进入 300 时会再次核对 smoke、60、120、200、240、280 六个前置阶段；任一阶段不是 `PASS` 或没有实际执行，即使 280 后的即时恢复快照已经健康，也不得执行 300。

## 五、报告与归档

命令结束时控制台输出最终 verdict 和报告路径。运行目录：

```text
tmp/perf/runs/<run-id>/
├── effective-config.json
├── report.md
├── summary.json
├── raw-k6-summary.json
├── evidence.json
├── smoke/
├── experience_60/
├── capacity_120/
├── capacity_200/
├── capacity_240/
├── capacity_280/
├── pre-admission-300/
├── admission_300/
└── final-recovery/
```

评审时至少记录：

- run ID、Git SHA、环境与机器规格；
- 最终 verdict 及所有 reason；
- 各阶段目标/实际 QPS、HTTP RPS、双 TPS；
- 各关键操作 P50/P95/P99/max；
- 成功率、错误率、超时率、最终失败率；
- 分层重试率；
- 280 后和 300 后的恢复耗时、Outbox 与 NSQ 残留。

无证据值必须保留 `N/A`。禁止手工改成 0 或从别的跑次拼接成“通过”。

## 六、常见结论模式

| 现象 | 结论方向 |
| --- | --- |
| QPS 达标，P99 变高 | 排队或接近饱和 |
| QPS 达标，错误率上升 | 吞吐以失败为代价，不算扛住 |
| 受理 TPS 高，完成 TPS 低 | Worker、下游或报告生成跟不上 |
| 提交快，Outbox/NSQ 持续积压 | 入口健康，后续交付链路堵塞 |
| 重试率高，最终成功率仍高 | 瞬态不稳定，靠重试恢复 |
| 重试率高，最终失败率也高 | 重试未恢复问题 |
| P50 正常，P99 很差 | 少数用户尾延迟严重 |
| dropped iterations > 0 | VU 或系统能力不足，目标负载未真实达到 |
| k6 失败但应用无对应错误 | 检查 Nginx、网络、token 与压测端资源 |

## 七、最终结论模板

> 在 `<环境/Git SHA>` 上执行 `<run-id>` admission。系统实际达到 `<QPS>`，dropped iterations 为 `<数量>`；HTTP RPS 为 `<值>`，受理/完成 TPS 为 `<值>/<值>`。各关键操作 P50/P95/P99/max `<是否达标>`，成功率、错误率和超时率 `<是否达标>`。压测结束后 Outbox/NSQ 于 `<时间>` 内回落至基线，分层重试 `<摘要>`。最终 verdict 为 `<PASS/FAIL/INCOMPLETE/ERROR>`。

只有吞吐、时延、正确性和恢复证据同时满足，才能写“系统在目标负载下稳定完成业务交付”。
