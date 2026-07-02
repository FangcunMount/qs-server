# QPS 容量档位与资源配置建议

**本文回答**：如果希望 qs-server 承接 QPS 100、200、300、500、700、900、1000，应调整哪些入口限流、SubmitQueue、Backpressure、连接池、worker 并发、容器资源和主机资源。本文给容量规划基线，不替代真实压测。

---

## 30 秒结论

| 目标 QPS | 推荐部署形态 | 结论 |
| -------- | ------------ | ---- |
| 100 | 单机单实例 | 小规格可承接，重点保护 DB |
| 200 | 单机单实例 | **通过**（`mixed_200`；**4C/8G** 2026-07-02） |
| 220 | 单机单实例 | **通过**（`mixed_220`；**4C/8G** 2026-07-02，http p95≈79ms） |
| 240 | 单机单实例 | **通过**（`mixed_240_models` 三域 **4C/8G** 2026-07-02，http p95≈100ms） |
| 280 | 单机单实例 | **通过**（8C/16G）；**4C/8G 榨干档调优后复测**（见 §2.4） |
| 300 | 单机单实例 | **通过**（`mixed_300`；**8C/16G** 验收）；4C/8G 须 §2.4 + 分步攻关 |
| 500 | 至少应用双实例 | 不建议单点承诺 |
| 700 | 应用多实例 | Redis/DB/MQ/IAM 应独立 |
| 900 | 应用多实例 + LB | 不能只调限流数字 |
| 1000 | 应用多实例 + LB | 必须正式压测验收 |

**单机实测（2026-07）**：8C/16G 下 `mixed_280_models` / `mixed_300` 全绿；4C/8G 下 **`mixed_200`～`mixed_240_models` 全绿**，280/300 见 §2.4。详见 [SOP §3.8](./11-300QPS混合场景压测SOP.md#38-轮次七servera-缩容复测2026-07-024c8g)。

核心原则：

1. `rate_limit.*_global_qps` 只控制入口速率，不能提高真实处理能力。
2. `submit_queue` 只削峰，不是跨实例持久队列。
3. `backpressure.*.max_inflight` 应匹配数据库连接池和下游承载能力。
4. QPS 500 以上优先横向扩容。
5. 容量档位必须用压测确认。

---

## 1. 当前配置基线

当前生产配置已按 `mixed_300` 目标和 serverA 8C/16G 单 apiserver 架构调整：

| 位置 | 关键值 | 含义 |
| ---- | ------ | ---- |
| collection rate_limit | submit/query global QPS 300，wait-report global QPS 200 | 入口保护（压测配比见 k6 profile，与此无关） |
| collection grpc_client | max_inflight **420** | 4C/8G 榨干档 |
| collection submit_queue | queue_size **2800**，worker_count **56** | 提交削峰 |
| collection questionnaire_cache | enabled，TTL 180s，max_entries 256 | 已发布问卷 REST DTO 进程内 L1（跳过 gRPC） |
| collection scale_cache | enabled，TTL 180s，max_entries 256 | 量表目录 REST DTO 进程内 L1 |
| collection personality_cache | enabled，TTL 180s，max_entries 256 | 人格模型目录 REST DTO 进程内 L1 |

目录缓存分层说明见 [Catalog L1+L2 缓存](../03-基础设施/redis/10-Catalog目录L1-L2缓存.md)。
| collection concurrency | query **400** + submit **96**（catalog/report-status Try 503） | 4C/8G 榨干档；读写池隔离 |
| collection wait_report | max_http_concurrency **400**，degrade_immediate_enabled | wait-report 独立池；槽位满立即 pending |
| collection report_events | enabled **false**（灰度）；max_connections 2000 | WebSocket 报告推送（方案 E） |
| collection redis pool | max-active 256 | collection 侧 Redis 活跃连接 |
| apiserver rate_limit | submit/query/wait-report global QPS 300，admin submit global QPS 360 | 后台 REST 入口 |
| apiserver backpressure | mysql **150**，mongo **120**，iam **100** | 4C/8G 榨干档；timeout 4～5s |
| apiserver mysql pool | max open **150** | DB 连接池 |
| worker concurrency | 48 | 后台消费并发 |

---

## 2. QPS 档位表

### 2.1 collection-server

| 目标 QPS | global QPS | burst | grpc max_inflight | queue_size | worker_count |
| -------- | ---------- | ----- | ----------------- | ---------- | ------------ |
| 100 | 100 | 150 | 50 | 300 | 4 |
| 200 | 200-250 | 300-400 | 80 | 500 | 8 |
| 300 | 300 | 450 | 120 | 800 | 12 |
| 500 | 500 | 750 | 200 | 1200 | 20 |
| 700 | 700 | 1050 | 280 | 1600 | 28 |
| 900 | 900 | 1350 | 360 | 2200 | 36 |
| 1000 | 1000 | 1500 | 400 | 2500 | 40 |

注意：单实例 `concurrency.max-concurrency` 不应无限提高，QPS 700+ 应靠多实例。

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

当前 **serverA 4C/8G** 部署：`qs-apiserver` 4 CPU / 4GiB，`qs-collection-server` 3 CPU / 3GiB（见 `build/docker/docker-compose.prod.yml`）。
8C/16G 历史验收：`qs-apiserver` 5 CPU / 8GiB，`qs-collection-server` 2 CPU / 4GiB。
submit 稳态由 `submit_queue` worker 与 apiserver 同步处理能力共同约束。

### 2.4 serverA 4C/8G 榨干档（2026-07）

针对 `mixed_280_models` / `mixed_300` 攻关，在 **不升配机器** 前提下对齐 inflight 与 k6 VU：

| 位置 | 关键值 | 说明 |
| ---- | ------ | ---- |
| collection `concurrency.max-query-concurrency` | **400** | 读路径槽位（catalog/report-status 满槽 Try 503） |
| collection `concurrency.max-submit-concurrency` | **96** | 写路径槽位（与读池隔离） |
| collection `grpc_client.max_inflight` | **420** | 对齐 apiserver gRPC 承载 |
| collection `grpc_client.inflight_wait_ms` | **4000** | 减少 2s 快速失败 |
| collection `submit_queue.worker_count` | **56** | 对齐 24/s submit |
| apiserver `backpressure.mongo.max_inflight` | **120** | submit+outbox 主瓶颈（原 80） |
| apiserver `backpressure.mysql.max_inflight` | **150** | 对齐 mysql pool |
| apiserver backpressure `timeout_ms` | **4000～5000** | 应用内排队，避免 k6 30s 雪崩 |
| k6 `mixed_280_models` VU max | submit **400** 等 | 见 `qs-perf.config.example.json` |

**部署**：改 `configs/*.prod.yaml` 后 **重启** `qs-apiserver` + `qs-collection-server`；本地 `make perf-sync-vusers` 同步 k6 VU；压测前冷却 ≥30min、网络稳定。

**验收顺序**：`mixed_280_models` 全绿 → `perf-mixed300-http` → `perf-mixed300-http-query` → `perf-mixed300`。

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

注意：当前 Compose 如果使用固定 `container_name`，不适合直接 `docker compose up --scale`。横向扩容应使用服务发现/LB/K8s/Swarm 或调整 Compose 模板。

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
| 提交链路 p95 | < 1000ms |
| p99 | 可解释，不能持续恶化 |
| 429 | 只在超过目标 QPS/burst 出现 |
| backpressure_timeout | 稳态不应持续出现 |
| SubmitQueue depth | burst 后应回落 |
| MQ depth | 不持续增长 |
| DB 慢查询 | 不随 QPS 线性恶化 |
| RSS | 低于 mem_limit，有 GC 余量 |

---

## 7. 调参顺序

1. 确定请求混合比例。
2. 调 collection rate limit。
3. 调 collection grpc max_inflight 和 submit_queue。
4. 调 apiserver rate limit 和 backpressure。
5. 调 DB/Mongo/Redis/MQ 资源。
6. 调容器 CPU/memory/GOMEMLIMIT。
7. QPS 500+ 优先横向扩容。
8. 压测验收。

---

## 8. 常见错误

- 只把 QPS 数字调大。
- queue_size 过大掩盖下游慢。
- backpressure 高于 DB 承载。
- worker 并发高于 apiserver 处理能力。
- 只压缓存命中接口就承诺提交 QPS。
- QPS 700+ 仍用单实例硬撑。

---

## 9. Verify

压测示例：

```bash
RPS=200 DURATION=10m VUS=100 MAX_VUS=500 BASE_URL=http://127.0.0.1:8082 k6 run scripts/perf/k6-collection.js
```

观测：

```bash
curl -s http://127.0.0.1:<port>/metrics
curl -s http://127.0.0.1:<port>/governance/resilience
curl -s http://127.0.0.1:<port>/governance/redis
```
