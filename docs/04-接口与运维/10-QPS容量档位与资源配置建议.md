# QPS 容量档位与资源配置建议

**本文回答**：当前生产硬件可以稳定承接多少 QPS，100 QPS 对应多少 Qlume 同时测评用户，110/120 QPS 正式压测分别证明了什么，以及未来提升到 200、300、500 QPS 时应如何规划应用、数据层和硬件资源。本文不再推演 500 QPS 以上档位；未来档位只提供规划和压测起点，不替代真实验收。

---

## 30 秒结论

### 当前容量结论

| 问题 | 结论 | 证据状态 |
| ---- | ---- | ---- |
| 生产长期按多少运行 | **100 业务 QPS** | **运行建议**：相对 110 QPS 已验证边界保留约 10% 余量 |
| 当前硬件已通过多少 | **110 QPS** | **已验证 PASS**：实际 110.07 QPS，Dropped=0，负载窗口完成率 100% |
| 从哪里开始不再稳定 | **120 QPS** | **已验证 FAIL**：Dropped=5，两类 Submit P95 超过 500ms |
| 100 QPS 对应多少同时测评用户 | 平均测评 5～10 分钟时约 **3,000～5,000 人** | **规划推算**：建议对外保守值 3,000，完成用户旅程压测前不写成 SLA |
| 未来容量规划到哪里 | **200 / 300 / 500 QPS** | **规划改造**：尚未验收；500 QPS 是本文规划上限 |

一句话口径：

> 当前生产硬件建议按 **100 QPS** 长期运行；**110 QPS** 是已验证通过边界，**120 QPS** 是保护上限。在流量配比与 K6 基本一致、用户进入和交卷足够均匀时，100 QPS 可规划为约 **3,000～5,000 名用户同时测评**；该人数是业务换算，不是已通过的并发用户 SLA。

### 前台硬件起点

| 目标 | 成本型正常态档 | 生产 N+1 档 | 状态 |
| ---: | -------------- | --------- | ---- |
| 100 QPS | **1 × 4C/8G**：Nginx + apiserver ×1 + collection ×2 | 暂不规划；当前是单故障域 | 100 为运行建议，110 为已验证边界 |
| 200 QPS | **1 × 8C/16G**：Nginx + apiserver ×1 + collection ×2 | **2 × 8C/16G** 对称应用节点 | 未验收 |
| 300 QPS | **2 × 8C/16G**：每节点 Nginx + apiserver ×1 + collection ×1，前置云 SLB | **3 × 8C/16G** 对称应用节点 | 未验收 |
| 500 QPS | **3 × 8C/16G**：每节点 Nginx + apiserver ×1 + collection ×1，前置云 SLB | **4 × 8C/16G** 对称应用节点 | 未验收，本文上限 |

成本型正常态档只承诺全部节点健康时的目标容量；节点故障后允许降级，不能标记为高可用或 N+1。300/500 QPS 不使用“第一台 Nginx 代理全部远端实例”的单入口方案，而由云 SLB 将流量分配给每台应用节点上的 Nginx。

---

## 阅读地图与证据口径

| 想回答的问题 | 阅读位置 |
| ------------ | -------- |
| 当前真实容量到底是多少 | 第 1 节：110/120 QPS 正式证据 |
| 100 QPS 可以支持多少人同时测评 | 第 2 节：业务容量换算 |
| 当前哪些配置是保护上限 | 第 3 节：现行部署与保护基线 |
| 200、300、500 QPS 要买什么资源 | 第 4 节：未来硬件设计 |
| 数据库、MQ 和多实例怎么扩 | 第 5 节：横向扩容与数据层 |
| 怎么正式验收或复验 | 第 6 节：压测验收与执行 |
| 应该先调哪个参数 | 第 7 节：调参和运维顺序 |
| 旧的 280/300 QPS 结果为什么撤销 | 第 8 节：历史记录 |

本文使用以下状态：

| 状态 | 含义 |
| ---- | ---- |
| **已验证** | 已取得当前发布版本、隔离流量、阈值和服务端观测证据 |
| **运行建议** | 从已验证边界向下留出稳态余量，不是独立的新实验 |
| **规划推算** | 用当前数据线性换算，只能用于容量、预算和压测起点 |
| **规划改造** | 未来部署方向，不代表当前已实现或已可承诺 |
| **历史撤销** | 保留为配置和问题演进记录，不再作为容量事实 |

核心原则：

1. `rate_limit.*_global_qps` 只控制入口速率，不能提高真实处理能力。
2. 答卷提交不再经过进程内队列；Submit Gate 只限制在途数，`202` 必须来自可靠提交。
3. `backpressure.*.max_inflight` 必须匹配数据库连接池和下游承载能力。
4. 当前硬件不再通过调大并发或限流数字追求 120 QPS 以上；先增加物理 CPU 或拆分服务，再重新定标。
5. 500 QPS 必须横向扩容；所有未来档位都必须由隔离压测确认。
6. 300/500 QPS 使用云 SLB + 对称应用节点；成本型档允许故障降级，N+1 档才要求单节点故障后维持目标 QPS。

---

## 1. 当前容量事实：110 QPS 通过，120 QPS 到达保护上限

### 1.1 正式运行证据

| 项目 | 值 |
| ---- | -- |
| 日期 | 2026-08-14 |
| 执行入口 | `make perf-run PLAN=ceiling-120` |
| 正式运行目录 | `/opt/qs-perf/runs/20260814-222422.217926702-unknown` |
| 压测工具链提交 | `6ea0a69f` |
| CI | `31807645494` 成功 |
| Production Deploy | `31808469843` 成功 |
| 前台主机 | serverA 4C/8G；Nginx + apiserver ×1 + collection ×2 同机 |
| 异步消费 | worker ×3 + NSQ |
| 阶段 | `capacity_110` → 有界恢复门 → `capacity_120` |

### 1.2 吞吐、时延与正确性

| 维度 | `capacity_110` | `capacity_120` |
| ---- | -------------- | -------------- |
| 阶段 verdict | **PASS** | **FAIL** |
| 目标 / 实际 QPS | 110.00 / **110.07** | 120.00 / **120.03** |
| Dropped iterations | **0** | **5** |
| 负载窗口完成率 | **100.00%** | **100.00%** |
| medical Submit P95 | **431.37ms** | **543.26ms** |
| personality Submit P95 | **460.72ms** | **537.48ms** |
| Submit P95 门禁 | `<500ms` | `<500ms` |
| HTTP / 业务操作成功率 | **100% / 100%** | **100% / 100%** |
| 错误 / 超时 / 客户端与服务端重试 | **0 / 0 / 0** | **0 / 0 / 0** |
| serverA CPU 平均 / 峰值 | **83.66% / 93.19%** | **92.00% / 96.16%** |
| 流量隔离与观测完整性 | 通过 | 通过 |
| 6 个组件进程连续性 | 通过 | 通过 |

两个阶段的 HTTP 与业务操作成功率均为 100%，错误率、超时率和客户端/服务端重试率均为 0。进入 120 QPS 前的恢复门在第 2 次检查通过，最终 Outbox 与压测 NSQ topic 均恢复为 0。

### 1.3 容量判定

110 QPS 同时满足到达率、Dropped、操作时延、成功率、负载窗口完成率、流量隔离、进程连续性和恢复门，因此可记录为当前硬件的**已验证通过边界**。但 serverA CPU 平均已达 83.66%，不适合长期满载运行，因此生产稳态目标定为 100 QPS。

120 QPS 虽然抵达 120.03 QPS，且业务正确性仍为 100%，但已经同时出现：

- 5 个业务迭代未能按计划发出；
- medical/personality Submit P95 分别为 543.26ms/537.48ms，均超过 `<500ms` 门禁；
- serverA CPU 平均 92.00%、峰值 96.16%，已接近共享 4 核主机的计算上限。

该窗口 I/O wait 接近 0，Outbox 和 NSQ 最终恢复为 0，证据更符合**serverA 共享 CPU 饱和**，而不是 MongoDB 积压或跨云网络抖动。因此 120 QPS 必须记录为**保护上限**，不能因为实际到达率和正确性达标就改写成容量通过。

---

## 2. 100 QPS 的业务容量：约 3,000～5,000 名同时测评用户

### 2.1 QPS 是业务操作到达率，不是用户数

K6 中的 QPS 以 business iteration 计数，不等于活跃用户数，也不一定等于原始 HTTP RPS。一个全链路探针会发起提交、readiness 轮询和报告等多个 HTTP 请求；WebSocket 也同时包含握手、订阅和状态帧。

动态容量档以 `admission_300` 为标准混合比例，保留 1 QPS 全链路探针，其余流量由 `scripts/perf/perfctl/plan.go` 中的 `scaledWorkload` 用最大余数法整数缩放。100 QPS 档的业务操作配比为：

| 类别 | 包含的 K6 场景 | QPS | 占比 |
| ---- | ----------------- | --: | ---: |
| 模型与问卷查询 | medical model、personality model、medical/personality questionnaire | 48 | 48% |
| 答卷提交 | medical submit 6 + personality submit 2 | 8 | 8% |
| 报告 WebSocket | medical 23 + behavior 3 + personality 7 | 33 | 33% |
| 统计查询 | statistics | 10 | 10% |
| 全链路探针 | submit → Assessment → report | 1 | 1% |
| **合计** |  | **100** | **100%** |

### 2.2 需要分开四种“并发”

| 口径 | 含义 | 能否用 100 QPS 直接回答 |
| ---- | ---- | -------------------------- |
| 同时在线 | 小程序已打开，但可能长时间没有请求 | **不能**；空闲用户几乎不消耗 qs-server QPS |
| 同时答题 | 用户已获取问卷，正在小程序本地选择答案 | **可估算**；需要平均测评时长 |
| 同一秒交卷 | 大量用户同时提交整份答卷 | **不等于同时答题人数**；当前稳态约 9.1 份/秒 |
| 同时等报告 | 已交卷用户保持 WebSocket 或短轮询 | **需单独看** WS 连接数、报告时延和轮询放大率 |

### 2.3 从可靠受理 TPS 换算同时测评人数

110 QPS 正式阶段的可靠受理 TPS 为 10.01。在流量比例、缓存命中率、数据量、响应体大小和依赖时延基本不变的前提下：

```text
100 QPS 可靠受理 TPS
= 100 × 10.01 ÷ 110
≈ 9.1 份答卷/秒
```

小程序在用户答题期间主要维护本地答案，交卷时才将整个 `answers` 数组一次性提交。因此可按在途数据的基本关系换算：

```text
同时测评人数
≈ 每秒交卷人数 × 平均测评时长（秒）
≈ 9.1 × 平均测评时长（秒）
```

| 平均测评时长 | 换算 | 预计同时测评人数 |
| --------------: | ----: | -------------------: |
| 3 分钟 | 9.1 × 180 | 约 **1,640 人** |
| 5 分钟 | 9.1 × 300 | 约 **2,730 人** |
| 8 分钟 | 9.1 × 480 | 约 **4,370 人** |
| 10 分钟 | 9.1 × 600 | 约 **5,460 人** |
| 15 分钟 | 9.1 × 900 | 约 **8,190 人** |

上表假定用户持续进入、持续交卷，不存在明显的整点、活动结束或课堂统一交卷尖峰。“9.1 份/秒”表示稳态受理率，不表示同一毫秒的突发提交已经单独验收。

### 2.4 小程序旅程与业务口径

```text
进入测评
  → 获取模型/问卷或创建 personality session
  → 用户在小程序本地答题 5～10 分钟
  → POST /answersheets 一次性提交答卷
  → assessment-readiness
  → WebSocket report-events，失败则 report-status 短轮询
  → 终态后获取报告
```

用户在本地阅读题目、思考和选择答案时，并不会每秒向 qs-server 发起请求。因此系统可以同时存在数千个答题中的用户，但任一秒只有其中一小部分在查询、交卷或等待报告。现行契约见 [小程序接入文档](./15-小程序接入文档.md) 与 [小程序报告等待接入指南](./12-小程序报告等待接入指南.md)。

| 使用场景 | 建议表达 |
| -------- | -------- |
| 内部容量规划 | 当前 100 QPS 稳态档对应约 **3,000～5,000 名同时测评用户** |
| 产品、商务或运营沟通 | 建议先使用 **3,000 名并发测评用户** 的保守规划值 |
| 对外 SLA 或招投标承诺 | 必须先完成 3,000 用户完整旅程验收；未验收前不写“已验证支持” |
| 仅打开小程序、没有操作的在线用户 | 不使用 QPS 换算；需根据会话、长连接、心跳和 IAM token 刷新单独估算 |

### 2.5 适用条件和风险

只有在以下条件基本成立时，才可使用 3,000～5,000 人的估算：

- 医学、人格、查询、提交、报告和统计配比与本次 K6 基本一致；
- 平均测评时长为 5～10 分钟，用户进入和交卷足够均匀；
- 缓存命中率、数据规模、响应体大小和依赖时延没有明显恶化；
- WebSocket 是报告等待主路径，不与 HTTP 短轮询同时双跑；
- 没有无法隔离的大量后台统计、运营或其他业务流量。

以下因素会使实际可承载人数下降：

- 课堂、企业活动或考试在同一分钟集中交卷；
- WebSocket 批量断线，用户同时降级为 HTTP 短轮询；
- 网络抖动导致小程序重试、重连和重复请求；
- IAM 登录、首页或其他服务流量与 qs-server 同时到达高峰；
- 缓存冷启动、GC、MongoDB 索引/压缩、Prometheus compaction 或同机进程抢占 CPU；
- 报告生成时间增加，使 WebSocket 在线数和轮询放大率升高。

现行 K6 混合压测把查询、提交、WebSocket 和统计拆成独立到达率场景，没有模拟一个虚拟用户持续经历“进入→思考→交卷→等报告”的 5～10 分钟完整旅程。因此，100/110/120 QPS 是当前容量结论，3,000～5,000 名用户是业务换算。旅程验收方案见第 6.4 节。

---

## 3. 现行部署与保护基线

### 3.1 当前四机拓扑

| 主机 | 当前规格 | 主要职责 |
| ---- | -------- | -------- |
| serverA | 4C/8G | Nginx + apiserver ×1 + collection ×2 |
| serverB | 2C/2G | IAM |
| serverC | 4C/4G | MongoDB 单节点副本集 |
| serverD | 4C/4G | worker ×3 + NSQ + operating + Prometheus + Grafana |
| 云托管 | 规格不在本仓库声明 | MySQL + Redis |

serverA 是当前可计算的前台参照单元：100 QPS 为建议稳态，110 QPS 为通过边界。apiserver 4 CPU/4GiB、collection 每副本 2 CPU/1.5GiB 的容器 quota 共享 4 个物理核，不能相加成 8 核。worker ×3 提供异步冗余，不参与大多数前台查询和可靠受理响应；NSQ depth 为 0 时，继续增加 worker 不会提高前台 QPS。

serverC 只描述当前部署事实。下一次硬件升级时，MongoDB 迁移到与应用同地域、同 VPC 的阿里云 MongoDB 独享型云盘版副本集；完成保留期验证后，serverC 退出 Mongo 主链路。

### 3.2 当前保护参数

以下数字是入口、在途、连接池或并发保护上限，不是可以相加的容量数字：

| 位置 | 关键值 | 含义 |
| ---- | ------ | ---- |
| collection rate_limit | submit/query global QPS 300；**report_events global 120** | 入口保护，不是处理能力 |
| collection submit degraded local | 每实例 global 30/45；user 10/15 | Redis rate backend 故障时的保守预算 |
| collection grpc_client | max_inflight **420** | 2026-07 历史调参值，不代表当前容量 |
| collection submit | accept timeout **2000ms**，gate wait **50ms** | 可靠受理时限与有界等待 |
| collection questionnaire_cache | enabled，TTL 180s，max_entries 256 | 已发布问卷 REST DTO 进程内 L1 |
| collection scale_cache | enabled，TTL 180s，max_entries 256 | 量表目录 REST DTO 进程内 L1 |
| collection typology_cache | enabled，TTL 180s，max_entries 256 | 人格模型目录 REST DTO 进程内 L1 |
| collection concurrency | catalog **512** + query **280** + submit **96** | 单副本分池上限；普通槽位最长等待 4000ms，catalog 800ms |
| collection wait_report | max_http_concurrency **400**，degrade_immediate_enabled | wait-report 独立池；槽位满立即 pending |
| collection report_events | enabled **true**；max_connections 2000 | WebSocket 报告推送主路径 |
| collection redis pool | max-active 256 | collection 侧 Redis 活跃连接 |
| apiserver rate_limit | submit/query/wait-report global QPS 300，admin submit 360 | 后台 REST 入口保护 |
| apiserver backpressure | mysql **150**，mongo **48**，iam **100** | 在途上限；timeout 分别为 5000/1500/4000ms |
| apiserver mysql pool | max open **150** | MySQL 连接池 |
| apiserver mongo pool | max **64**，max connecting **8** | mongo inflight 保留 25% 连接余量 |
| worker concurrency | **48** | 后台消费并发 |

目录缓存分层见 [Cache 架构与责任边界](../03-基础设施/cache/10-架构与责任边界.md)。配置中的 300、420、512 等数字均为保护上限或历史调参值，不得作为 QPS 相加或外推。

---

## 4. 200～500 QPS 未来硬件设计

### 4.1 推算模型

本节固定采用以下部署前提：应用、worker/NSQ 等 ECS 位于**阿里云同地域、同可用区、同 VPC**，MongoDB、MySQL、Redis 等云数据库位于阿里云同地域并通过 VPC 内网访问。只在**请求混合比例、缓存命中率、数据量、响应体大小和依赖时延与本次 `ceiling-120` 基本一致**时使用以下模型：

- 110 QPS：serverA 平均忙碌 CPU 为 `4 × 83.66% = 3.35` 核，约 `0.0304 core/QPS`；
- 120 QPS：serverA 平均忙碌 CPU 为 `4 × 92.00% = 3.68` 核，约 `0.0307 core/QPS`；
- 规划时向上取 `0.031 core/QPS`；
- 成本型正常态档按全部节点健康时的 CPU 水位设计，允许节点故障后降低可承载 QPS；
- 生产 N+1 档的正常窗口 CPU 目标不高于 70%，单节点故障窗口不高于 85%；
- 相同混合比例下暂按 `Submit TPS ≈ QPS × 0.091` 估算异步和 Mongo 压力。

```text
前台忙碌核数       = 目标 QPS × 0.031
70% CPU 前台 vCPU = 前台忙碌核数 ÷ 0.70
预计 Submit TPS    = 目标 QPS × 10.01 ÷ 110
```

| 目标 QPS | 预计忙碌核 | 成本档前台规格 | 正常 CPU | 单节点故障后剩余 vCPU | 故障时理论 CPU | 预计 Submit TPS | 推算置信度 |
| -------- | -----------: | ---------------- | --------: | ---------------------: | -------------: | ---------------: | ---------- |
| 100 | 3.1 | 1 × 4C/8G | 77.5% | 0 | 不可服务 | 9.1 | 高：100 为当前运行建议 |
| 200 | 6.2 | 1 × 8C/16G | 77.5% | 0 | 不可服务 | 18.2 | 中：约为实测边界 1.8 倍，必须复测 |
| 300 | 9.3 | 2 × 8C/16G | 58.1% | 8 | 116.3% | 27.3 | 中低：需要跨主机部署与 SLB 验收 |
| 500 | 15.5 | 3 × 8C/16G | 64.6% | 16 | 96.9% | 45.5 | 低：本文规划上限，只用于硬件预算 |

“前台 vCPU”包含 Nginx、apiserver、collection、内核网络和容器网络开销，不包含 IAM、worker、Mongo、MySQL、Redis、NSQ 和 Prometheus。CPU 线性模型只说明前台正常态起点，不能替代各服务容器 throttling、数据库连接池或下游容量证据。

成本档节点故障后的降级上界按同一模型估算：300 QPS 剩余 8 核时，70% 稳态约为 181 QPS、85% 保护水位约为 219 QPS；500 QPS 剩余 16 核时，70% 稳态约为 361 QPS、85% 保护水位约为 439 QPS。云 SLB 可以保住入口，但不能凭空补回丢失节点的计算能力。

### 4.2 成本型正常态档：故障可降级，非 N+1

该档位优先复用统一的 8C/16G 应用节点，不为单节点故障预留完整目标容量。它适合预算受限、允许维护窗口、短时中断或故障降级的部署，不得标记为高可用或 SLA 档：

| 角色 | 200 QPS 成本档 | 300 QPS 成本档 | 500 QPS 成本档 |
| ---- | ---------------- | ---------------- | ---------------- |
| 前台计算 | **1 × 8C/16G**：Nginx + apiserver ×1 + collection ×2 | **2 × 8C/16G**：每节点 Nginx + apiserver ×1 + collection ×1 | **3 × 8C/16G**：每节点 Nginx + apiserver ×1 + collection ×1 |
| 公网入口 | 单节点 Nginx | 云 SLB → 2 个应用节点 Nginx | 云 SLB → 3 个应用节点 Nginx |
| IAM | 1 × 2C/4G | 1 × 2C/4G | 1 × 4C/8G |
| worker + NSQ | 1 × 8C/16G + 500GB SSD | 1 × 8C/16G + 500GB SSD | 1 × 12C/24G + 1TB SSD |
| Prometheus/Grafana | 1 × 2C/4G + 300GB SSD | 1 × 2C/4G + 300GB SSD | 1 × 4C/8G + 500GB SSD |
| 阿里云 MongoDB | **2C/8G** 独享型云盘副本集 + 500GB ESSD AutoPL | **4C/16G** 独享型云盘副本集 + 500GB ESSD AutoPL | **4C/16G** 独享型云盘副本集 + 1TB ESSD AutoPL |
| 云 MySQL | 2C/4G 高可用版 | 4C/8G 高可用版 | 4C/16G 高可用版 |
| 云 Redis | 2C/4G 高可用版 | 2C/4G 高可用版 | 4C/8G 高可用版 |
| K6 压测机 | 独立 8C/16G | 独立 8C/16G | 独立 8C/16G |
| 网络 | 阿里云同地域、同可用区、同 VPC，≥1Gbps | 阿里云同地域、同可用区、同 VPC，≥1Gbps | 阿里云同地域、同可用区、同 VPC，≥2Gbps |

worker 与 NSQ 继续部署在独立机器上，避免占用前台 CPU；但 `nsqd + nsqlookupd + nsqadmin` 是三个组件，不等于三个高可用 NSQ 节点。该机器仍是异步层单故障域，必须用 Submit TPS、worker CPU、NSQ depth、Outbox oldest age 和磁盘 IOPS 单独验收。

### 4.3 生产档：N+1 冗余起点

| 角色 | 200 QPS 推荐起点 | 300 QPS 推荐起点 | 500 QPS 推荐起点 |
| ---- | ---------------- | ---------------- | ---------------- |
| 公网入口 | 云 SLB → 全部应用节点 Nginx | 云 SLB → 全部应用节点 Nginx | 云 SLB → 全部应用节点 Nginx |
| 前台应用 | **2 × 8C/16G**；每节点 Nginx + apiserver ×1 + collection ×1 | **3 × 8C/16G**；每节点 Nginx + apiserver ×1 + collection ×1 | **4 × 8C/16G**；每节点 Nginx + apiserver ×1 + collection ×1 |
| IAM | 2 × 2C/4G | 2 × 4C/8G | 2 × 4C/8G |
| worker | 2 × 4C/8G | 2 × 8C/16G | 2 × 8C/16G |
| NSQ | 4C/8G 主节点 + 4C/8G 热备 | 2 × 4C/8G；自动切换完成前按主备运行 | 2 × 4C/8G + 3 lookupd |
| 阿里云 MongoDB | 4C/16G 副本集 + 500GB ESSD AutoPL，多可用区 | 8C/32G 副本集 + 1TB ESSD AutoPL，多可用区 | 8C/32G 副本集 + 1TB ESSD AutoPL，多可用区 |
| 云 MySQL | 4C/8G 高可用版 | 8C/16G 高可用版 | 8C/32G 高可用版 |
| 云 Redis | 2C/4G 高可用版 | 4C/8G 高可用版 | 4C/8G 高可用版 |
| 观测节点 | 1 × 4C/8G + 500GB SSD | 1 × 4C/8G + 500GB SSD | 1 × 8C/16G + 1TB SSD |
| K6 压测机 | 独立 8C/16G | 独立 8C/16G | 独立 8C/16G |
| 网络 | 阿里云同地域、同可用区、同 VPC，≥1Gbps | 阿里云同地域、同可用区、同 VPC，≥1Gbps | 阿里云同地域、同可用区、同 VPC，≥2Gbps |

200 QPS 的两个 8 核节点失去一个后，剩余节点预计 CPU 约 77.5%；300 QPS 的三个 8 核节点失去一个后，剩余 16 核预计 CPU 约 58.1%；500 QPS 的四个 8 核节点失去一个后，剩余 24 核预计 CPU 约 64.6%。这些数字说明硬件数量满足实例级 N+1 的计算起点，仍必须通过真实摘除节点的故障窗口压测。

所有 ECS 位于同一可用区，因此这里的 N+1 只覆盖容器、进程、ECS 或单宿主机故障，**不覆盖整个可用区故障**。云数据库位于同地域；只有实际购买并验收跨可用区高可用规格时，数据层才能覆盖可用区级故障。

阿里云 ECS 的计算型/高主频计算型规格包含 8C/16G、12C/24G、16C/32G、24C/48G 等档位，成本型正常态档优先选固定算力、非突发型实例；实际可购规格以[阿里云 ECS 计算型实例规格](https://help.aliyun.com/zh/ecs/user-guide/compute-optimized-instance-families/)为准。阿里云 MongoDB 独享型云盘副本集提供 2C/8G、4C/16G、8C/32G 等规格并支持 ESSD AutoPL，地域、连接数、IOPS 与吞吐上限以[副本集实例规格表](https://help.aliyun.com/zh/mongodb/product-overview/replica-set-instance-types)为准。

### 4.4 对称应用节点的容器资源起点

| 节点形态 | 服务组成 | CPU 起点 | 内存起点 | 说明 |
| -------- | -------- | -------- | -------- | ---- |
| 200 QPS 单节点 | Nginx + apiserver ×1 + collection ×2 | apiserver 4C；collection 合计约 3C；Nginx/系统约 1C | apiserver 6GiB；collection 合计 3～4GiB；其余留给 Nginx/系统 | 两个 collection 保留进程级冗余，但整机仍是单故障域 |
| 300/500 QPS 对称节点 | Nginx + apiserver ×1 + collection ×1 | apiserver 4C；collection 2C；Nginx/系统至少 2C | apiserver 6GiB；collection 2GiB；Nginx 1GiB；系统与 page cache 保留约 7GiB | 每台结构一致，便于 SLB 健康检查、扩容和摘除 |
| worker + NSQ 节点 | worker ×2～3 + NSQ 组件 | worker quota 合计不超过物理核的 60%～70% | 给 nsqd 队列、页缓存和操作系统保留余量 | 以积压和恢复时间定标，不按前台 QPS 等比增加并发 |

上述是首轮压测起点，不要求把 CPU quota 机械设置为恰好相等。Go 服务的 `GOMEMLIMIT` 建议为容器内存的 65%～75%；同机容器 quota 之和即使超过物理核数，也必须以宿主机 CPU、容器 throttling、P95/P99 和 Dropped 判断，不能把 quota 相加当作机器容量。

---

## 5. 横向扩容与数据层边界

### 5.1 实例数与故障域

100/110/120 三行是当前部署事实与压测结论；200 QPS 以上是首轮硬件设计，必须在跨物理节点部署后重新压测：

| 目标 QPS | 档位 | 8C/16G 应用节点 | Nginx | apiserver | collection | 异步层 | 状态 |
| -------- | ---- | ----------------: | ----: | --------: | ---------: | ------ | ---- |
| 100 | 当前 | 0；使用 1 × 4C/8G | 1 | 1 | 2 | worker ×3 + NSQ 独立机 | 当前建议稳态 |
| 110 | 当前 | 0；使用 1 × 4C/8G | 1 | 1 | 2 | worker ×3 + NSQ 独立机 | 已验证通过边界 |
| 120 | 当前 | 0；使用 1 × 4C/8G | 1 | 1 | 2 | worker ×3 + NSQ 独立机 | 当前保护上限，FAIL |
| 200 | 成本型正常态 | 1 | 1 | 1 | 2 | worker + NSQ 独立机 | 单机故障即中断 |
| 200 | 生产 N+1 | 2 | 2 | 2 | 2 | worker/NSQ 另做冗余 | 单节点故障后维持目标的规划起点 |
| 300 | 成本型正常态 | 2 | 2 | 2 | 2 | worker + NSQ 独立机 | 故障后降级至约 181～219 QPS |
| 300 | 生产 N+1 | 3 | 3 | 3 | 3 | worker/NSQ 另做冗余 | 单节点故障后维持目标的规划起点 |
| 500 | 成本型正常态 | 3 | 3 | 3 | 3 | worker + NSQ 独立机 | 故障后降级至约 361～439 QPS |
| 500 | 生产 N+1 | 4 | 4 | 4 | 4 | worker/NSQ 另做冗余 | 单节点故障后维持目标的规划起点 |

当前 collection Compose 已移除固定 `container_name` 与宿主机端口映射，由 CD 使用固定 project `qs-collection`、短 service key `server` 执行 `docker compose up --scale`，生成 `qs-collection-server-N` 容器名。Nginx `collect-api` upstream 使用 Docker resolver 动态解析显式别名 `qs-collection-server`，按默认轮询分流，不配置 `ip_hash`、固定权重或主备。该模式只提供同机进程级冗余。

300/500 QPS 需要由阿里云 SLB 把公网流量分配到全部应用节点的 Nginx；每台 Nginx 优先路由本机 apiserver/collection 组合。当前 CD、Compose 和 Nginx 配置尚未完成这套对称多主机部署，实施时必须补齐节点注册、私网健康检查、TLS/CORS、WebSocket、保护性 503 策略和摘除恢复验证。不能只在第一台保留 Nginx，再把它当作全部远端实例的唯一入口。

### 5.2 数据层、MQ 和观测

本次 `ceiling-120` 只证明压测结束后 Outbox 与 NSQ 可以恢复为 0，没有提供 Mongo、MySQL、Redis、NSQ 和 worker 主机的完整 CPU、IOPS 与 working set 曲线。因此第 4 节的数据层规格是采购起点，仍需要以下验收：

| 组件 | 当前边界 | 扩容前必须完成 |
| ---- | -------- | -------------- |
| MongoDB | 当前 infra 为 serverC 单节点副本集；未来为阿里云多可用区副本集 | 200 QPS 生产化前完成云数据库迁移；使用 VPC 内网连接串、`replicaSet`、`directConnection=false` 和 TLS，验证事务 P95、复制延迟、IOPS、oplog 窗口与主节点切换 |
| MySQL | 云 RDS；所有 apiserver/worker/IAM 共享连接预算 | 汇总所有实例 max-open，给管理连接和故障恢复保留余量；以慢查询与 IOPS 决定升配 |
| Redis | 500 QPS 规划仍使用云高可用版，不以分片为前置条件 | 验证连接池、分布式限流、lease、锁和主从故障切换；只有证据要求分片时再验收 cluster 契约 |
| NSQ producer | publisher 直接连接单个 `nsq-addr`；consumer 通过 lookupd 发现 nsqd | 多 nsqd 不自动等于 producer 高可用；必须增加多端点/四层 LB/故障切换，并验证重连期间 Outbox 不丢失且最终归零 |
| Prometheus | 当前与 worker、NSQ、operating 同机 | 200 QPS 起迁到独立观测节点，避免 TSDB compaction 与业务争抢磁盘和 CPU |

实例增加后，连接池必须按集群合计。以 500 QPS 成本档的 3 个 apiserver 为例，如果原样复制当前单实例上限，仅 apiserver 就会形成 MySQL 450、MongoDB 192、Redis max-active 1152 的理论上限，尚未包含 collection、worker 和 IAM。购买或调整云数据库前必须先建立集群连接预算，并给管理、健康检查、故障切换和连接风暴保留余量。

本文所有未来档位都按阿里云同地域 VPC 规划：ECS 位于同一可用区，云数据库位于同地域并使用 VPC 内网连接；跨云链路只保留灾备或非主链路。网络带宽只是采购下限，还必须观察 PPS、重传率、overlay 开销和东西向流量。同可用区可以降低主链路时延，但不提供可用区级容灾。

Mongo 迁移不能以“新实例可连接”作为完成标准。切换前应完成数据全量/增量校验和索引核对；切换窗口停止旧实例写入后比较关键集合计数与业务抽样；切换后验证 Submit、Outbox、报告生成、备份恢复和主节点故障切换。serverC 至少保留一个约定回滚观察期，确认无回切需求后再退役其 Mongo 数据与网络入口。

---

## 6. 压测验收与执行

### 6.1 统一验收门禁

| 指标 | 目标 |
| ---- | ---- |
| 目标 QPS 达成率 | 满足 profile 门，并且 Dropped iterations = 0 |
| HTTP 5xx | 非预期 0 |
| 操作成功率 | > 99% |
| 错误率、超时率 | 满足 profile 门；关键提交超时 = 0 |
| 普通查询 P95 | < 500ms |
| 可靠提交 P95 / P99 | < 500ms / < 1000ms |
| 429 | 只在超过目标 QPS/burst 时出现 |
| backpressure_timeout | 稳态不应持续出现 |
| 负载窗口最终完成率 | 100% |
| 202 后 AnswerSheet + Outbox 可查率 | 100% |
| 重复幂等意图产生新 AnswerSheet | 0 |
| Outbox / MQ depth | 阶段增量满足门禁，恢复门后回到干净基线 |
| 压测流量隔离 | `origin="other"` 增量 = 0 |
| 组件进程连续性 | 阶段前后进程身份不变 |
| DB 慢查询 | 不随 QPS 线性恶化 |
| RSS | 低于 mem_limit，有 GC 余量 |

120 QPS 虽然成功率和最终完成率均为 100%，但 Dropped=5，且两类 Submit P95 超门，因此整体必须判定为 FAIL。单独通过正确性或 P99 门不能覆盖负载与 P95 失败。

### 6.2 当前 QPS 边界复验

```bash
make perf-preflight
make perf-run PLAN=ceiling-120
```

`ceiling-120` 固定执行 `capacity_110` → 恢复门 → `capacity_120`，不会继续进入 200 QPS。110 PASS、120 FAIL 是当前预期的保护边界证据，因此命令最终非零退出不表示压测工具故障；应检查 `report.md` 与 `summary.json` 的分阶段 verdict。

只检查编排和配置，不发起公网负载：

```bash
make perf-run PLAN=ceiling-120 DRY_RUN=1
```

完成硬件扩容或拓扑调整后，才重新探索容量：

```bash
make perf-run PLAN=baseline
make perf-run PLAN=admission
```

当前 `admission` 的最高阶段是 300 QPS。500 QPS 不能通过修改 `RPS` 环境变量绕过正式编排；部署目标硬件后，应先为压测工具增加独立的 500 QPS 阶段、阶段前后恢复门，并分别执行成本型正常态窗口或生产档 N+1 故障窗口。

2026-07 的历史分步 profile 不再作为现行执行入口；只保留第 8 节的过程记录。

### 6.3 多节点拓扑验收

300/500 QPS 在容量验收前必须先通过多节点拓扑门，避免把“总 CPU 足够”误写成“多机流量已经生效”：

| 阶段 | 必须验证 | 通过条件 |
| ---- | -------- | -------- |
| 部署前检查 | SLB backend、每台 Nginx、apiserver、collection、证书与版本 | 实例数和目标档一致，全部 readiness 通过，部署 SHA 一致 |
| 正常态窗口 | SLB 分流、Nginx `$upstream_addr`、每节点请求量、CPU/throttling、连接池 | 所有节点收到业务流量；没有单节点长期热点；满足第 6.1 节全部门禁 |
| 成本档摘除 | 主动摘除一台应用节点 | SLB 在健康检查窗口内停止分流；入口不中断；只承诺在 300→181～219、500→361～439 QPS 的降级范围内通过 |
| N+1 摘除 | 主动摘除一台应用节点并保持原目标 QPS | 200/300/500 QPS 原目标不变，Dropped=0，时延、正确性、恢复门全部通过 |
| 恢复窗口 | 重新加入节点 | readiness 通过后再接流；没有连接风暴、Outbox/NSQ 异常增长或 WebSocket 大规模重连失败 |

成本型正常态档和生产 N+1 档必须分别记录 verdict。成本档在节点故障后不能维持原目标 QPS，是设计边界而不是压测工具故障；N+1 档如果摘除节点后降速或超门，则不得标记为 N+1 通过。

### 6.4 并发用户旅程验收

建议新增独立 journey profile，不改变当前 QPS admission 的含义：

```text
登录/恢复会话
  → 查询模型与问卷
  → 按分布模拟 5～10 分钟答题时间
  → 可靠提交答卷
  → assessment-readiness
  → WebSocket 订阅，断线时降级 report-status
  → 报告终态与正文读取
```

| 阶段 | 同时测评用户 | 用途 |
| ---- | -----------------: | ---- |
| journey-2000 | 2,000 | 验证压测机和数据准备，建立低档基线 |
| journey-3000 | 3,000 | 验收建议对外保守值 |
| journey-5000 | 5,000 | 验证内部规划上界，失败不影响 3,000 档的独立结论 |

旅程档位必须同时验证：全链路成功率；错误/超时/重试/重连率；关键操作 P50/P90/P95/P99；实际业务 QPS；受理与完成 TPS；Outbox/NSQ 稳态门；流量隔离与进程连续性；serverA CPU、WebSocket 连接数、数据库连接池和 worker/NSQ 同窗口证据。

停止规则：

- 2,000 用户档失败：不进入 3,000，先区分压测机 VU 资源与服务端瓶颈；
- 3,000 用户档失败：不对外承诺 3,000，保留上一个通过档；
- 5,000 用户档失败：只否定 5,000 上界，不覆盖 3,000 档的独立结论；
- 无法归因的生产流量、进程重启或观测缺失：记录为 `INCOMPLETE`，不写成容量通过。

### 6.5 Redis RateLimit 降级专项验收

Redis RateLimit 降级预算必须在隔离灰度环境单独验收：

```bash
make perf-init
# 编辑 tmp/perf/iam-users.json 后刷新 token
make perf-tokens
make perf-preflight

PERF_ISOLATED_ENV=true \
REDIS_FAILURE_CONFIRMED=true \
TESTEE_IDS='<at-least-six-token-aligned-testee-ids>' \
make perf-run PLAN=diagnose CASE=collection-runtime-degraded-low
```

脚本默认读取 `SNAP-VI` 问卷并按题型生成 answers；low、global、user 分别使用前 2、6、1 组 collection token 与 `TESTEE_IDS`。只有需要完全控制测试意图时才提供 `SUBMIT_CASES_JSON`。

过载验收分别使用 `CASE=collection-runtime-degraded-global` 与 `CASE=collection-runtime-degraded-user`。脚本以 15 秒 warmup 排除初始 burst，再在默认 60 秒 steady 窗口验证双实例 global 成功准入不超过 63 QPS、单 writer 成功准入不超过 21 QPS。脚本只验证已注入的故障，不负责停止 Redis。

恢复 Redis 后设置 `REDIS_RECOVERY_CONFIRMED=true` 执行 `CASE=collection-runtime-recovery`，验证两个 readiness 均为 200、本地 fallback 停止增长且 Redis 分布式策略恢复。

观测入口：

```bash
curl -s http://127.0.0.1:<port>/metrics
curl -s http://127.0.0.1:<port>/governance/resilience
curl -s http://127.0.0.1:<port>/governance/redis
```

---

## 7. 调参和运维顺序

### 7.1 应用参数不按 QPS 等比放大

| 参数 | 集群级原则 | 单实例原则 |
| ---- | ---------- | ---------- |
| collection / apiserver global rate limit | 按目标业务 QPS 设置一次，burst 通常从目标的 1.2～1.5 倍开始压测 | 使用 Redis 分布式限流时，不得再乘实例数 |
| collection query / catalog concurrency | 由实例数、CPU 与 P95 共同定标 | 槽位满时保持有界等待，不用无界排队换到达率 |
| collection Submit Gate | 由可靠受理 TPS、Mongo 事务时延与 apiserver 实例数决定 | 不得高于下游可用事务槽位 |
| apiserver Mongo backpressure | 集群总在途必须给 Mongo 连接池和健康检查留余量 | 延续 `max_inflight <= 75% × mongo pool`，当前为 48/64 |
| apiserver MySQL pool | 所有 apiserver 连接池之和必须低于 RDS `max_connections` 预算 | 不按全局 QPS 把每个实例的 pool 同时放大 |
| worker concurrency | 根据 worker CPU、事件处理 P95、NSQ depth 与 Outbox age 调整 | 优先增加实例，再小步提升单实例并发 |

因此，不再维护“每个 QPS 档位直接对应一组 pool/backpressure 数字”的配置表。硬件或拓扑变化后，应保持当前保护边界作为起点，通过 admission 逐档调整。

### 7.2 调参次序

1. 确定请求混合比例。
2. 调整 collection rate limit。
3. 调整 collection gRPC max_inflight 和 Submit Gate，不得引入进程内排队。
4. 调整 apiserver rate limit 和 backpressure。
5. 调整 DB/Mongo/Redis/MQ 资源。
6. 调整容器 CPU/memory/GOMEMLIMIT。
7. 当前硬件到 110 QPS 后停止继续调大并发；需要更高容量时先增加物理资源或拆分服务。
8. 300/500 QPS 档使用云 SLB + 对称应用节点，优先跨物理节点横向扩容。
9. 按隔离、负载、时延、正确性、稳态和恢复门完整压测验收。

改动 `configs/*.prod.yaml` 后必须重启 `qs-apiserver` 和 `qs-collection-server`；压测前确保网络稳定，并由统一编排器自动生成阶段 VU。

### 7.3 常见错误

- 只把 QPS 数字调大；
- 通过无界等待掩盖下游慢；
- backpressure 高于 DB 承载；
- worker 并发高于 apiserver 处理能力；
- 只压缓存命中接口就承诺提交 QPS；
- 把 CPU 线性推算直接当作 Mongo、MySQL、Redis、NSQ 的容量结论；
- 部署多个 nsqd 就宣称 producer 已经自动故障切换；
- 只增加容器 CPU quota，却没有增加宿主机物理 CPU；
- 300/500 QPS 只保留一台 Nginx，导致其他应用节点无法形成独立入口；
- 把同可用区的实例级 N+1 写成可用区级容灾。

---

## 8. 历史调参记录：保留过程，撤销容量结论

### 8.1 2026-07 serverA 4C/8G 历史参数

以下数字用于解释配置演进，不是当前执行入口或容量承诺：

| 位置 | 历史值 | 当时目的 |
| ---- | ------ | -------- |
| collection `concurrency.max-query-concurrency` | **460** | 读路径槽位，catalog 满槽 Try 503 |
| collection `rate_limit.report_events_global_qps` | **120** | WS subscribe 96/s 留余量 |
| collection `grpc_client.max_inflight` | **420** | 对齐 apiserver gRPC 承载 |
| collection `grpc_client.inflight_wait_ms` | **4000** | 减少 2s 快速失败 |
| collection `concurrency.max_submit_concurrency` | **96** | 可靠提交初始准入边界 |
| collection `submit.gate_wait_ms` / `accept_timeout_ms` | **50 / 2000** | 有界等待与总受理超时 |
| apiserver `backpressure.mongo.max_inflight` | **120** | submit + outbox 历史攻关值，当前为 48 |
| apiserver `backpressure.mysql.max_inflight` | **150** | 对齐 mysql pool |
| apiserver backpressure `timeout_ms` | **4000～5000** | 应用内排队，避免 K6 30s 雪崩 |
| K6 历史手工 VU | report max **380**，全场景 max **<700** | 当前已改为按到达率、典型时延、超时和 headroom 自动计算 |

### 8.2 历史输出和撤销原因

| 历史 Profile | failed | 当前判定 |
| -------------- | -----: | -------- |
| `mixed_280_models` | 0.20% | 历史工具判为边际通过；未证明窗口内业务完成能力 |
| `mixed_300_http_query` | 0.01% | 历史工具判为通过；只是读 + WS 子集，无 probe |
| `mixed_300` 全量（×2） | 8.75%～10.60% | **未过**：catalog 503 + chain_probe 128～137 |

这些运行主要证明入口能按目标到达率发起请求，并且系统在事后有机会排空。由于当时的阶段 verdict 未硬性比较快照窗口完成率、干净 backlog 基线与 Outbox/NSQ 增量，“更多请求进入后慢慢处理”仍可能被写成通过。

因此，原先“4C/8G 可验收约 280/s”以及“8C/16G 已验收 300/s”的容量承诺一并撤销。当前有效结论只能写为 100 QPS 建议稳态、110 QPS 已验证边界、120 QPS 保护上限；升级后的新承诺只能来自现行 admission 的逐档稳态门。

---

## 9. 更新规则与证据入口

### 9.1 必须重新定标的变化

出现以下任一变化时，不再直接使用本文容量换算：

- serverA 硬件、服务拆分或实例数变化；
- MongoDB 迁移到云数据库或数据层主要规格变化；
- K6 流量比例、报告等待策略或小程序请求链发生明显变化；
- 真实用户的平均测评时长不在 5～10 分钟范围；
- 活动流量变为集中开始、集中交卷；
- 容量告警、P95/P99、Dropped、Outbox/NSQ 或 CPU 显示新的拐点。

### 9.2 证据索引

| 内容 | 位置 |
| ---- | ---- |
| 压测执行流程、门禁和结果解读 | [300 QPS 混合场景压测 SOP](./11-300QPS混合场景压测SOP.md) |
| 小程序 session、提交、readiness 和报告契约 | [小程序接入文档](./15-小程序接入文档.md) |
| WebSocket 与短轮询边界 | [小程序报告等待接入指南](./12-小程序报告等待接入指南.md) |
| 目录缓存责任与 L1/L2 边界 | [Cache 架构与责任边界](../03-基础设施/cache/10-架构与责任边界.md) |
| K6 基准流量配比 | `scripts/perf/qs-perf.config.example.json` |
| 阶段与流量缩放 | `scripts/perf/perfctl/plan.go` |
| 吞吐、时延和正确性报告 | `scripts/perf/perfctl/report.go` |
