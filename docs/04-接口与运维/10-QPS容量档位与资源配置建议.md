# QPS 容量定标与资源规划指南

本文用于回答“怎样为一个确定的环境定标容量、怎样把业务用户换算成请求负载、怎样决定扩容或调参”。它不保存当前生产容量、主机/副本数量或某次压测结果；这些时点证据统一进入[基础设施生产证据台账](../00-总览/10-基础设施生产证据台账.md)。

部署与网络事实见[配置与部署](../03-基础设施/config-deployment/README.md)，并发保护边界见 [Concurrency](../03-基础设施/concurrency/README.md)，正式压测步骤见 [300QPS 混合场景压测 SOP](./11-300QPS混合场景压测SOP.md)。

## 1. 30 秒执行入口

```bash
make perf-verify
make perf-run PLAN=quick DRY_RUN=1
make perf-preflight
make perf-run PLAN=quick
make perf-run PLAN=baseline
```

已有可信低档证据、只复验 110/120 两阶段时：

```bash
make perf-run PLAN=ceiling-120 DRY_RUN=1
make perf-run PLAN=ceiling-120
```

探索新环境的容量拐点时：

```bash
make perf-run PLAN=admission DRY_RUN=1
make perf-run PLAN=admission
```

任何命令的 PASS/FAIL/INCOMPLETE 只属于本次 run、SHA、effective config 和环境。不要把旧 run 的结论复制为“当前容量”。

## 2. 先定义容量问题

### 2.1 QPS 不等于用户数

至少分开四种量：

| 量 | 单位 | 用途 |
| --- | --- | --- |
| 业务到达率 | operation/s | 定义混合负载与入口限流 |
| HTTP/gRPC/MQ 放大 | request 或 message/s | 计算一次业务操作带来的下游压力 |
| 在途并发 | in-flight | 定标 Gate、Backpressure、pool 与 WebSocket |
| 同时在线/测评用户 | user | 产品容量；必须由用户旅程和停留时间换算 |

同一业务 QPS 在查询、提交、WebSocket、统计占比变化后，对 CPU、Mongo、MySQL、Redis 和 MQ 的压力会完全不同。容量结论必须附请求混合比例。

### 2.2 从旅程换算用户

先从真实或目标旅程取得：

- 平均/分位测评时长；
- 每个用户的模型/问卷查询次数；
- 提交次数与重试概率；
- WebSocket 在线时间、断线与轮询回退率；
- 报告和统计查询次数；
- 集中开始/集中交卷的 burst 分布。

基础换算：

```text
平均业务 QPS = 同时活跃用户 × 每用户每秒业务操作数
在途请求     ≈ 到达率 × 请求时延
在线连接数   ≈ 新建连接率 × 平均连接时长
Submit TPS   = 同时测评用户 ÷ 平均测评秒数 × 平均提交次数
```

结果只是负载输入。完成对应 journey profile 之前，不得把换算人数写成 SLA。

## 3. 建立本次容量工作表

每次定标先填写：

| 输入 | 必填内容 |
| --- | --- |
| 版本 | source/deployed SHA、镜像 digest |
| 配置 | 脱敏 effective-config hash、配置族 |
| 拓扑 | 实例、故障域、入口、依赖与网络 RTT；保存在本次证据，不写成长期文档 |
| 数据 | 数据量、索引、缓存冷热、响应体大小 |
| 流量 | profile、请求混合、burst、测试数据与隔离方式 |
| 资源 | CPU/memory limit、连接池、磁盘/IOPS、网络 |
| 门禁 | QPS、Dropped、错误/超时、P95/P99、正确性、积压与恢复 |
| 观测 | 逐实例指标、日志、Outbox/MQ/DB/Redis 与主机资源 |
| 停止条件 | 最大档位、错误阈值、积压阈值、恢复超时 |

缺 source SHA、effective config、流量隔离或服务端观测时，verdict 只能是 `INCOMPLETE`。

## 4. 资源预算方法

### 4.1 入口、在途与连接池不能相加

- RateLimit 控制单位时间到达，不能提高真实处理能力；
- Gate/Backpressure 控制在途，必须用目标到达率与实测时延推导；
- DB/Redis/MQ pool 是共享依赖预算，所有实例的上限要按集群汇总；
- SubmitCoalescer/lease 降低重复工作，不替代 transaction、unique、claim 或 CAS；
- worker 并发按事件处理 P95、积压增长/恢复和下游预算定标，不按前台 QPS等比放大。

### 4.2 CPU 和内存

只用同环境、同请求混合、未饱和档位的观测计算单位负载资源：

```text
单位 QPS 忙碌核 = 观测忙碌 CPU 核 ÷ 实际业务 QPS
目标忙碌核      = 单位 QPS 忙碌核 × 目标 QPS × 风险系数
内存预算        = steady RSS + cache/connection/VU 增量 + GC/故障余量
```

CPU 线性估算只是下一档起点。出现 throttling、GC、lock contention、缓存 miss、下游延迟或响应体变化时必须重新测量，不能继续线性外推。

### 4.3 异步与数据层

每档都要记录：

- AnswerSheet 受理/完成 TPS 与 transaction P95/P99；
- durable Outbox 新增、oldest age、publish/retry/dead-letter；
- MQ depth、消费速率、失败结算和排空时间；
- Mongo/MySQL pool 使用、慢查询、复制/事务时延；
- Redis 连接、RateLimit/lease/Signal 降级；
- Report/Statistics 的最终完成率和恢复窗口。

“压测结束后最终排空”不等于稳态通过；负载窗口内持续积压、靠停流量追平时必须判失败或不完整。

## 5. 选择统一 profile

| 计划 | 命令 | 用途 |
| --- | --- | --- |
| quick | `make perf-run PLAN=quick` | 最小连通、凭证和证据链检查 |
| baseline | `make perf-run PLAN=baseline` | 建立低档、可重复基线 |
| ceiling-120 | `make perf-run PLAN=ceiling-120` | 已有低档证据时复验 110/120 保护链 |
| admission | `make perf-run PLAN=admission` | 逐档探索当前工具支持范围内的容量拐点 |
| diagnose | `make perf-run PLAN=diagnose CASE=<case>` | 单一失败机制或治理能力专项 |

`ceiling-120` 固定执行 `capacity_110` → 恢复门 → `capacity_120`，不会进入 200/240/280/300。两阶段 verdict 由本次证据决定，不预设某档 PASS/FAIL。

`admission` 当前最高阶段为 300 QPS。不得通过临时修改 `RPS` 绕过正式 phase、恢复门或证据合同；新增更高档位时先扩展 perfctl 计划和验证测试。

## 6. 统一验收门

至少同时检查：

| 维度 | 通过要求 |
| --- | --- |
| 到达 | 实际业务 QPS 达到 profile 下限，Dropped 满足门禁 |
| 时延 | 各业务操作 P50/P90/P95/P99 满足当前 profile |
| 错误 | 5xx、业务失败、timeout、429 和分层重试可解释且不过门 |
| 正确性 | 202 后 durable AnswerSheet + Outbox 可查；幂等与最终业务事实正确 |
| 稳态 | 负载窗口完成率、Outbox/MQ 增量、DB/Redis/worker 指标不过门 |
| 恢复 | 停流量后在规定窗口回到干净基线 |
| 隔离 | 非压测流量增量为 0 或已精确扣除 |
| 连续性 | 实例未非预期重启，逐实例观测完整 |
| 证据 | run ID、SHA、effective config、summary/report/evidence 文件齐全 |

任一硬门失败立即停止后续档位。观测缺失、生产流量无法归因或实例重启原因不明时记 `INCOMPLETE`，不能挑选成功率等单一指标宣布通过。

## 7. 多节点与 N+1 验收

扩容后先验证流量确实到达所有目标实例，再谈总容量：

1. 保存 SLB/Nginx effective config hash、backend 与逐实例 SHA/digest；
2. 正常窗口核对每实例请求量、CPU/throttling、连接池和时延分布；
3. 经批准摘除一个故障域，确认入口在健康检查窗口内停止分流；
4. 成本档明确允许的降级 QPS；N+1 档必须在保持原目标 QPS 时仍通过所有门；
5. 重新加入实例只在 readiness 通过后接流量，并检查连接风暴、WebSocket 重连和 Outbox/MQ；
6. 实例数和宿主拓扑只写本次证据，不回写成本文常量。

同宿主多容器只提供进程级冗余；同可用区多主机也不等于可用区容灾。故障声明必须与实际验证的故障域一致。

## 8. 用户旅程验收

独立 journey profile 应覆盖：

```text
登录/恢复会话
  -> 查询模型与问卷
  -> 按目标分布等待/答题
  -> 可靠提交
  -> Assessment readiness
  -> WebSocket 等待，断线后回退事实查询
  -> 报告终态与读取
```

从低用户档开始逐级增加。每档独立记录成功率、重试/重连、P95/P99、实际 QPS、Submit TPS、在线连接、最终完成率、Outbox/MQ、逐实例资源和恢复。某档失败只否定该档及更高档，不覆盖已独立通过的低档。

## 9. 调参顺序

1. 固定请求混合、测试数据、隔离和依赖环境；
2. 先查 CPU/GC/网络和下游时延，确认真实瓶颈；
3. 小步调整入口 RateLimit 与 burst；
4. 用到达率 × 时延调整 Gate、gRPC inflight 和 Backpressure；
5. 汇总所有实例后调整 DB/Mongo/Redis/MQ pool；
6. 按积压与恢复调整 worker；
7. 再调整容器 CPU/memory/`GOMEMLIMIT` 或增加物理故障域；
8. 回到 baseline，再逐档 admission，不从失败档直接继续加参数。

禁止通过无界等待、把 Backpressure 调高于下游预算、只增加容器 quota、只压缓存命中接口或只看最终排空来“提高容量”。

## 10. 结果与更新

每次 run 保留 `summary.json`、`report.md`、`evidence.json`、完整命令、退出码、source/deployed SHA、effective config、拓扑 hash、观察窗口、owner 和限制。当前生产建议与历史撤销结论只从[基础设施生产证据台账](../00-总览/10-基础设施生产证据台账.md)读取。

硬件/故障域、服务拆分、配置族、数据库/MQ、流量混合、用户旅程、数据规模或依赖时延变化后，旧结果仍保留审计价值，但必须失效并重新定标。
