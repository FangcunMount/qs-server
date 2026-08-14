# QPS 容量档位与资源配置建议

**本文回答**：如果希望 qs-server 承接 QPS 100、200、300、500、700、900、1000，应调整哪些入口限流、Submit Gate、Backpressure、连接池、worker 并发、容器资源和主机资源。本文给容量规划基线，不替代真实压测。

---

## 30 秒结论

| 目标 QPS | 推荐部署形态 | 结论 |
| -------- | ------------ | ---- |
| 100 | 单机单实例 | 当前 4C/8G 候选档；必须由新版 `admission` 重新验收 |
| 200 | 单机单实例 | 当前拓扑未验收，不承诺 |
| 220 | 单机单实例 | 当前拓扑未验收，不承诺 |
| 240 | 单机单实例 | 当前拓扑未验收，不承诺 |
| 280 | 单机单实例 | 2026-07 旧测试只证明到达率与可恢复性，容量结论已撤销 |
| 300 | 单机单实例 | 当前 4C/8G 拓扑未验收，不承诺；8C/16G 旧结果也需新门禁复测 |
| 500 | 至少应用双实例 | 不建议单点承诺 |
| 700 | 应用多实例 | Redis/DB/MQ/IAM 应独立 |
| 900 | 应用多实例 + LB | 不能只调限流数字 |
| 1000 | 应用多实例 + LB | 必须正式压测验收 |

**当前有效结论（2026-08-14）**：2026-07 的 4C/8G 与 8C/16G 结果没有把负载窗口完成率及阶段前后 Outbox/NSQ 增量作为阶段硬门，并允许压测后继续排空，因此只能保留为历史调参记录，不能继续写成稳态容量。当前版本必须重新执行 `make perf-run PLAN=baseline` 与 `make perf-run PLAN=admission`。

同日 `ff729be2` 的新版混合链路运行中，4C/8G serverA 在 110 QPS 通过当时的负载/时延/正确性门，120 QPS 出现 4 个 dropped iterations、Submit P95 约 920ms、serverA CPU 接近耗尽。该结果证明容量拐点已落在 110～120 之间，但它早于 backlog 窗口硬门，110 只能作为当前调优基线，不是最终容量承诺。

核心原则：

1. `rate_limit.*_global_qps` 只控制入口速率，不能提高真实处理能力。
2. 答卷提交不再经过进程内队列；Submit Gate 只限制在途数，`202` 必须来自可靠提交。
3. `backpressure.*.max_inflight` 应匹配数据库连接池和下游承载能力。
4. QPS 500 以上优先横向扩容。
5. 容量档位必须用压测确认。

---

## 1. 当前配置基线

当前生产配置曾按历史 300 QPS 目标和 serverA 8C/16G 单 apiserver 架构调整；当前验收事实源改为 `admission_300`：

| 位置 | 关键值 | 含义 |
| ---- | ------ | ---- |
| collection rate_limit | submit/query global QPS 300；**report_events global 120** | 入口保护（压测配比见 k6 profile） |
| collection submit degraded local | 每实例 global 30/45；user 10/15 | Redis rate backend 故障时的保守容量保护 |
| collection grpc_client | max_inflight **420** | 2026-07 历史调参值，不代表当前容量 |
| collection submit | accept timeout **2000ms**，gate wait **50ms** | 可靠受理时限和有界等待 |
| collection questionnaire_cache | enabled，TTL 180s，max_entries 256 | 已发布问卷 REST DTO 进程内 L1（跳过 gRPC） |
| collection scale_cache | enabled，TTL 180s，max_entries 256 | 量表目录 REST DTO 进程内 L1 |
| collection typology_cache | enabled，TTL 180s，max_entries 256 | 人格模型目录 REST DTO 进程内 L1 |

目录缓存分层说明见 [Cache 架构与责任边界](../03-基础设施/cache/10-架构与责任边界.md)。
| collection concurrency | query **460** + submit **96**（catalog Try 503） | 2026-07 历史调参值；当前必须以新 admission 复核 |
| collection wait_report | max_http_concurrency **400**，degrade_immediate_enabled | wait-report 独立池；槽位满立即 pending |
| collection report_events | enabled **false**（灰度）；max_connections 2000 | WebSocket 报告推送（方案 E） |
| collection redis pool | max-active 256 | collection 侧 Redis 活跃连接 |
| apiserver rate_limit | submit/query/wait-report global QPS 300，admin submit global QPS 360 | 后台 REST 入口 |
| apiserver backpressure | mysql **150**，mongo **120**，iam **100** | 保护上限，不是 QPS 或稳态容量；timeout 4～5s |
| apiserver mysql pool | max open **150** | DB 连接池 |
| worker concurrency | 48 | 后台消费并发 |

---

## 2. QPS 档位表

### 2.1 collection-server

| 目标 QPS | global QPS | burst | grpc max_inflight | submit max concurrency | gate wait |
| -------- | ---------- | ----- | ----------------- | ---------------------- | --------- |
| 100 | 100 | 150 | 50 | 48 | 50ms |
| 200 | 200-250 | 300-400 | 80 | 64 | 50ms |
| 300 | 300 | 450 | 120 | 96 | 50ms |
| 500 | 500 | 750 | 200 | 压测定标 | 50ms |
| 700 | 700 | 1050 | 280 | 压测定标 | 50ms |
| 900 | 900 | 1350 | 360 | 压测定标 | 50ms |
| 1000 | 1000 | 1500 | 400 | 压测定标 | 50ms |

注意：单实例 `concurrency.max-query-concurrency` 不应无限提高，QPS 700+ 应靠多实例。

### 2.2 apiserver

| 目标 QPS | rate limit global | mysql pool | mysql backpressure | mongo backpressure | iam backpressure |
| -------- | ----------------- | ---------- | ------------------ | ------------------ | ---------------- |
| 100 | 100 | 60 | 60 | 80 | 30 |
| 200 | 200 | 80 | 80 | 100 | 40 |
| 300 | 300 | 120 | 120 | 150 | 60 |
| 500 | 500 | 200 | 200 | 240 | 100 |
| 700 | 700 | 280 | 280 | 340 | 140 |
| 900 | 900 | 360 | 360 | 430 | 180 |
| 1000 | 1000 | 400 | 400 | 480 | 200 |

### 2.3 worker

| 目标 QPS | worker concurrency | MySQL pool | 说明 |
| -------- | ------------------ | ---------- | ---- |
| 100 | 8 | 20 | 保守 |
| 200 | 16 | 30 | 当前基线 |
| 300 | 24 | 50 | 看 MQ depth |
| 500 | 40 | 80 | 建议独立资源池 |
| 700 | 56 | 120 | 多实例优先 |
| 900 | 72 | 160 | 关注 event backlog |
| 1000 | 80 | 180 | 需按事件类型拆分 |

---

## 3. 容器资源建议

| 目标 QPS | apiserver | collection | worker |
| -------- | --------- | ---------- | ------ |
| 100 | 0.75 CPU / 768MiB | 0.5 CPU / 512MiB | 0.25 CPU / 256MiB |
| 200 | 1 CPU / 1GiB | 0.5 CPU / 512MiB | 0.5 CPU / 256MiB |
| 300 | 1.5 CPU / 1.5GiB | 1 CPU / 1GiB | 0.5 CPU / 512MiB |
| 500 | 2.5 CPU / 2.5GiB | 1.5 CPU / 1.5GiB | 1 CPU / 1GiB |
| 700 | 3.5 CPU / 3.5GiB | 2 CPU / 2GiB | 1.5 CPU / 1.5GiB |
| 900 | 4.5 CPU / 4.5GiB | 2.5 CPU / 2.5GiB | 2 CPU / 2GiB |
| 1000 | 5 CPU / 5GiB | 3 CPU / 3GiB | 2 CPU / 2GiB |

`GOMEMLIMIT` 建议设置为容器内存的 65%-75%。

当前 **serverA 4C/8G** 部署：`qs-apiserver` 4 CPU / 4GiB，`qs-collection-server` 默认 2 副本，每副本 2 CPU / 1.5GiB，collection 总配额为 4 CPU / 3GiB（见 `build/docker/docker-compose.prod.yml`）。CPU quota 允许同机服务按实际空闲时间共享物理 CPU，不代表同时保有 8 个物理核。
8C/16G 历史验收：`qs-apiserver` 5 CPU / 8GiB，`qs-collection-server` 2 CPU / 4GiB。
submit 稳态由 Submit Gate、gRPC inflight、Mongo 事务与 Outbox Stage 能力共同约束。

当前 serverA 的两个 collection 副本能够被 Nginx 实际分流，并提供进程故障冗余与各自的目录 L1；但它们与单个 apiserver、Nginx 和 Tailscale 共享同一台 4 核主机，不是物理横向扩容。三个 worker 副本能够竞争消费 NSQ 并提供异步层冗余，但不参与大多数前台查询和可靠受理响应；当 NSQ depth 为 0 时，增加 worker 不会提高前台 QPS。端到端容量必须以单 apiserver 和共享 serverA CPU 这一最窄层为准，不能把 `collection ×2`、`worker ×3` 的副本数相加成系统容量。

### 2.4 serverA 4C/8G 历史调参记录（2026-07，容量结论已撤销）

以下是 2026-07 旧 profiles 的历史攻关记录，用于解释配置来源，不是当前执行入口：

| 位置 | 关键值 | 说明 |
| ---- | ------ | ---- |
| collection `concurrency.max-query-concurrency` | **460** | 读路径槽位（catalog 满槽 Try 503） |
| collection `rate_limit.report_events_global_qps` | **120** | WS subscribe 96/s 留余量 |
| collection `grpc_client.max_inflight` | **420** | 对齐 apiserver gRPC 承载 |
| collection `grpc_client.inflight_wait_ms` | **4000** | 减少 2s 快速失败 |
| collection `concurrency.max_submit_concurrency` | **96** | 可靠提交初始准入边界 |
| collection `submit.gate_wait_ms` / `accept_timeout_ms` | **50 / 2000** | 有界等待与总受理超时 |
| apiserver `backpressure.mongo.max_inflight` | **120** | submit+outbox 主瓶颈（原 80） |
| apiserver `backpressure.mysql.max_inflight` | **150** | 对齐 mysql pool |
| apiserver backpressure `timeout_ms` | **4000～5000** | 应用内排队，避免 k6 30s 雪崩 |
| k6 历史手工 VU | report max **380**，全场景 max **<700** | 当前已替换为按到达率、典型耗时、超时和 headroom 自动计算 |

**部署**：改 `configs/*.prod.yaml` 后 **重启** `qs-apiserver` + `qs-collection-server`；压测前确保网络稳定，并由统一编排器自动生成阶段 VU。

**当前验收顺序**：`make perf-run PLAN=baseline` → `make perf-run PLAN=admission`。历史分步 profile 不再可执行。

**历史工具输出**（2026-07-02 晚，WS + 分池 + VU 收紧；不得作为当前容量验收）：

| Profile | failed | 判定 |
| ------- | ---: | ---- |
| `mixed_280_models` | 0.20% | 历史工具判为边际通过；未证明窗口内业务完成能力 |
| `mixed_300_http_query` | 0.01% | 历史工具判为通过；只是读 + WS 子集，无 probe |
| `mixed_300` 全量（×2） | 8.75%～10.60% | **未过**（catalog 503 + chain_probe 128–137） |

这些运行主要证明入口能按目标到达率发起请求，并且系统在事后有机会排空。由于阶段 verdict 未硬性比较快照窗口完成率、干净 backlog 基线与 Outbox/NSQ 增量，“更多请求进入后慢慢处理”仍可能被写成通过。因此原先“4C/8G 可验收约 280/s”以及“8C/16G 已验收 300/s”的容量承诺一并撤销；新的承诺只能来自现行 admission 的逐档稳态门。

---

## 4. 横向扩容建议

| 目标 QPS | 推荐实例数 |
| -------- | ---------- |
| 100 | collection 1、apiserver 1、worker 1 |
| 200 | collection 1、apiserver 1、worker 1 |
| 300 | collection 1、apiserver 1、worker 1-2 |
| 500 | collection 2、apiserver 2、worker 2 |
| 700 | collection 3、apiserver 3、worker 2-3 |
| 900 | collection 4、apiserver 4、worker 3-4 |
| 1000 | collection 4、apiserver 4、worker 4 |

当前 collection Compose 已移除固定 `container_name` 与宿主机端口映射，由 CD 使用固定 project `qs-collection`、短 service key `server` 执行 `docker compose up --scale`，生成 `qs-collection-server-N` 容器名。Nginx `collect-api` upstream 使用 Docker resolver 动态解析显式别名 `qs-collection-server`，按默认轮询分流，不配置 `ip_hash`、固定权重或主备。该模式只提供同机进程级冗余；跨宿主机高可用仍应使用 LB/K8s/Swarm。

---

## 5. 数据层建议

| 目标 QPS | 数据层建议 |
| -------- | ---------- |
| 100 | 单机 MySQL/Mongo/Redis 可承接 |
| 200 | 数据层最好有独立资源余量 |
| 300 | 建议开始拆数据层 |
| 500 | MySQL/Mongo/Redis/MQ 独立部署 |
| 700+ | 数据层专项压测 |
| 1000 | MySQL/Mongo/Redis/IAM 都要独立容量预算 |

---

## 6. 压测验收指标

| 指标 | 目标 |
| ---- | ---- |
| HTTP 5xx | 非预期 0 |
| 错误率 | < 1% |
| 普通查询 p95 | < 500ms |
| 可靠提交 24/s p95 | < 500ms |
| 可靠提交 24/s p99 | < 1000ms |
| 429 | 只在超过目标 QPS/burst 出现 |
| backpressure_timeout | 稳态不应持续出现 |
| 202 后 AnswerSheet + Outbox 可查率 | 100% |
| 重复幂等意图产生新 AnswerSheet | 0 |
| MQ depth | 不持续增长 |
| DB 慢查询 | 不随 QPS 线性恶化 |
| RSS | 低于 mem_limit，有 GC 余量 |

---

## 7. 调参顺序

1. 确定请求混合比例。
2. 调 collection rate limit。
3. 调 collection grpc max_inflight 和 Submit Gate，不得引入进程内排队。
4. 调 apiserver rate limit 和 backpressure。
5. 调 DB/Mongo/Redis/MQ 资源。
6. 调容器 CPU/memory/GOMEMLIMIT。
7. QPS 500+ 优先横向扩容。
8. 压测验收。

---

## 8. 常见错误

- 只把 QPS 数字调大。
- 通过无界等待掩盖下游慢。
- backpressure 高于 DB 承载。
- worker 并发高于 apiserver 处理能力。
- 只压缓存命中接口就承诺提交 QPS。
- QPS 700+ 仍用单实例硬撑。

---

## 9. Verify

压测示例：

```bash
RPS=200 DURATION=10m VUS=100 MAX_VUS=500 BASE_URL=https://collect.fangcunmount.cn k6 run scripts/perf/k6-collection.js
```

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

脚本默认读取 `SNAP-VI` 问卷并按题型生成 answers；low、global、user 分别使用
前 2、6、1 组 collection token 与 `TESTEE_IDS`。只有需要完全控制测试意图时才
提供 `SUBMIT_CASES_JSON` 覆盖自动生成结果。

过载验收分别使用 `PLAN=diagnose CASE=collection-runtime-degraded-global` 与
`PLAN=diagnose CASE=collection-runtime-degraded-user`。脚本以 15 秒 warmup 排除初始 burst，
再在默认 60 秒 steady 窗口自动验证双实例 global 成功准入不超过 63 QPS、单
writer 成功准入不超过 21 QPS。脚本只验证已经注入的故障，不负责停止 Redis；
验收完成后恢复 Redis，并设置 `REDIS_RECOVERY_CONFIRMED=true` 执行
`make perf-run PLAN=diagnose CASE=collection-runtime-recovery`，验证两个 readiness 均为 200、本地 fallback
停止增长且 Redis 分布式策略恢复。

观测：

```bash
curl -s http://127.0.0.1:<port>/metrics
curl -s http://127.0.0.1:<port>/governance/resilience
curl -s http://127.0.0.1:<port>/governance/redis
```
