# QPS 容量档位与资源配置建议

**本文回答**：如果希望 `qs-server` 分别承接 QPS 100、200、300、500、700、900、1000，应调整哪些配置，分别需要怎样的主机硬件与 Docker 容器资源。本文给的是容量规划基线，不替代真实生产压测。

---

## 30 秒结论

| 目标 QPS | 推荐部署形态 | 应用侧容器 CPU / 内存合计 | 数据层建议 | 结论 |
| ---- | ---- | ---- | ---- | ---- |
| 100 | 单机单实例 | 约 1.5 vCPU / 1.5GiB | 单机 MySQL + Mongo + Redis 可承接 | 可用小规格，重点保护 DB |
| 200 | 单机单实例 | 约 2 vCPU / 2GiB | 当前 prod 配置接近该档 | 当前生产配置是保守 200 QPS 基线 |
| 300 | 单机单实例上限附近 | 约 3 vCPU / 3GiB | DB/Mongo 需要独立或明显余量 | 建议开始拆数据层 |
| 500 | 至少应用双实例 | 约 5 vCPU / 5GiB | MySQL/Mongo/Redis 建议独立节点 | 单机 Compose 可跑，但不建议单点承诺 |
| 700 | 应用多实例 | 约 7 vCPU / 7GiB | 数据层必须独立，Redis 不应与应用混部 | 进入横向扩容档 |
| 900 | 应用多实例 + LB | 约 9 vCPU / 9GiB | 数据层需要按慢查询和连接数专项压测 | 不能只调限流数字 |
| 1000 | 应用多实例 + LB | 约 10-12 vCPU / 10-12GiB | MySQL/Mongo/Redis/IAM 都要有独立容量预算 | 需要正式压测验收 |

最重要的原则：

1. `rate_limit.*_global_qps` 控制入口可接受速率；它不能提升真实处理能力。
2. `submit_queue` 只削平答卷提交尖峰；它不是跨实例持久队列。
3. `backpressure.*.max_inflight` 控制下游并发占用；它应小于等于数据库连接池和下游承载能力。
4. QPS 500 以上不建议靠单个 Compose 文件单实例硬撑，应按 collection / apiserver / worker 横向扩容，并将 MySQL、MongoDB、Redis、NSQ、IAM 独立部署。

---

## 当前配置基线

当前生产配置已经是一个“保守 200 QPS 左右”的基线：

| 位置 | 当前关键值 | 含义 |
| ---- | ---- | ---- |
| `configs/collection-server.prod.yaml` | `rate_limit.submit_global_qps=250`、`query_global_qps=250` | collection 入口保护 |
| `configs/collection-server.prod.yaml` | `grpc_client.max_inflight=80` | collection 到 apiserver 的并发 gRPC 上限 |
| `configs/collection-server.prod.yaml` | `submit_queue.queue_size=500`、`worker_count=8` | 答卷提交短时削峰 |
| `configs/collection-server.prod.yaml` | `concurrency.max-concurrency=40` | collection 内部校验并发，上限校验为 100 |
| `configs/apiserver.prod.yaml` | `rate_limit.*_global_qps=200` | apiserver REST 入口限流 |
| `configs/apiserver.prod.yaml` | `backpressure.mysql.max_inflight=80`、`mongo=100`、`iam=40` | 下游并发保护 |
| `configs/apiserver.prod.yaml` | `mysql.max-open-connections=80` | MySQL 连接池上限 |
| `configs/worker.prod.yaml` | `worker.concurrency=16`、`mysql.max-open-connections=30` | 后台消费并发 |
| `build/docker/docker-compose.prod.yml` | apiserver `1.0 CPU / 1024MiB`，collection `0.5 CPU / 512MiB`，worker `0.5 CPU / 256MiB` | 当前容器资源 |

这组配置适合在较小机器上保护下游，不适合直接通过把限流改到 1000 来承诺 1000 QPS。

---

## QPS 档位配置表

下表给的是单套 QS 应用集群的目标值。`burst` 建议取 QPS 的 1.5 倍；如果有明显秒级脉冲，可取 2 倍，但必须同时扩大队列和下游容量。

### collection-server

| 目标 QPS | `submit_global_qps` | `query_global_qps` | `wait_report_global_qps` | `burst` 建议 | `grpc_client.max_inflight` | `submit_queue.queue_size` | `submit_queue.worker_count` | `concurrency.max-concurrency` |
| ---- | ---- | ---- | ---- | ---- | ---- | ---- | ---- | ---- |
| 100 | 100 | 100 | 60 | 150 | 50 | 300 | 4 | 20 |
| 200 | 200-250 | 200-250 | 120 | 300-400 | 80 | 500 | 8 | 40 |
| 300 | 300 | 300 | 160 | 450 | 120 | 800 | 12 | 60 |
| 500 | 500 | 500 | 250 | 750 | 200 | 1200 | 20 | 80 |
| 700 | 700 | 700 | 350 | 1050 | 280 | 1600 | 28 | 100 |
| 900 | 900 | 900 | 450 | 1350 | 360 | 2200 | 36 | 100 |
| 1000 | 1000 | 1000 | 500 | 1500 | 400 | 2500 | 40 | 100 |

注意：

- `concurrency.max-concurrency` 当前代码校验不能超过 100；QPS 700 以上应靠多实例分摊，而不是继续加这个值。
- 如果压测使用单个用户 token，`*_user_qps` 也要临时提高到目标 QPS，否则会被单用户限流挡住；生产通常不应把用户级限流放得和全局一样高。
- `submit_queue.worker_count` 只影响答卷提交路径，不提高普通查询能力。

### apiserver

| 目标 QPS | `rate_limit.*_global_qps` | `rate_limit.*_global_burst` | MySQL `max-open-connections` | `backpressure.mysql.max_inflight` | `backpressure.mongo.max_inflight` | `backpressure.iam.max_inflight` | Redis `max-active` |
| ---- | ---- | ---- | ---- | ---- | ---- | ---- | ---- |
| 100 | 100 | 150 | 60 | 60 | 80 | 30 | 100 |
| 200 | 200 | 300 | 80 | 80 | 100 | 40 | 200 |
| 300 | 300 | 450 | 120 | 120 | 150 | 60 | 300 |
| 500 | 500 | 750 | 200 | 200 | 240 | 100 | 500 |
| 700 | 700 | 1050 | 280 | 280 | 340 | 140 | 700 |
| 900 | 900 | 1350 | 360 | 360 | 430 | 180 | 900 |
| 1000 | 1000 | 1500 | 400 | 400 | 480 | 200 | 1000 |

调参关系：

- `mysql.max-open-connections` 不应低于 `backpressure.mysql.max_inflight`，否则背压放行后仍会卡在连接池。
- `backpressure.mysql.max_inflight` 不应高于 MySQL 实例可稳定承接的并发事务数。MySQL CPU 打满、锁等待升高或慢查询增多时，应降低该值，而不是继续提高。
- Mongo 与 IAM 同理，先看 p95/p99 和 timeout，再决定是否扩大槽位。
- Redis `max-active` 是连接池预算，不等于 Redis QPS。Redis CPU、慢命令和网络 RTT 才是最终约束。

### worker

| 目标 QPS | `worker.concurrency` | Worker MySQL `max-open-connections` | Worker Redis `max-active` | 说明 |
| ---- | ---- | ---- | ---- | ---- |
| 100 | 8 | 20 | 50 | 后台消费保守即可 |
| 200 | 16 | 30 | 100 | 当前 prod 接近该档 |
| 300 | 24 | 50 | 150 | 需要确认 NSQ 积压不增长 |
| 500 | 40 | 80 | 250 | 建议 worker 独立资源池 |
| 700 | 56 | 120 | 350 | 多 worker 实例优先于单实例高并发 |
| 900 | 72 | 160 | 450 | 关注 evaluation lifecycle 积压 |
| 1000 | 80 | 180 | 500 | 需要按事件类型拆 channel/实例时再扩 |

Worker 并发要和 apiserver MySQL 背压一起看。后台任务如果也回调 apiserver 或占用 MySQL，不应让 worker 并发明显高于 apiserver 的下游保护能力。

---

## Docker 容器资源分配建议

### 单实例资源表

| 目标 QPS | qs-apiserver | qs-collection-server | qs-worker | 适用范围 |
| ---- | ---- | ---- | ---- | ---- |
| 100 | `cpus: "0.75"` / `mem_limit: "768m"` / `GOMEMLIMIT=512MiB` | `0.5` / `512m` / `320MiB` | `0.25` / `256m` / `160MiB` | 小流量、数据层同机可接受 |
| 200 | `1.0` / `1024m` / `640MiB` | `0.5` / `512m` / `320MiB` | `0.5` / `256m` / `192MiB` | 当前基线 |
| 300 | `1.5` / `1536m` / `1GiB` | `1.0` / `1024m` / `700MiB` | `0.5` / `512m` / `320MiB` | 单机应用上限附近 |
| 500 | `2.5` / `2560m` / `1800MiB` | `1.5` / `1536m` / `1GiB` | `1.0` / `1024m` / `700MiB` | 建议至少拆为 2 套实例 |
| 700 | `3.5` / `3584m` / `2500MiB` | `2.0` / `2048m` / `1500MiB` | `1.5` / `1536m` / `1GiB` | 应用多实例 |
| 900 | `4.5` / `4608m` / `3300MiB` | `2.5` / `2560m` / `1800MiB` | `2.0` / `2048m` / `1500MiB` | 应用多实例 + LB |
| 1000 | `5.0` / `5120m` / `3600MiB` | `3.0` / `3072m` / `2200MiB` | `2.0` / `2048m` / `1500MiB` | 正式容量档 |

`GOMEMLIMIT` 建议设置为容器 `mem_limit` 的 65%-75%。如果内存中缓存、响应对象或 goroutine 堆栈明显增长，优先加内存，不要把 `GOMEMLIMIT` 贴近 `mem_limit`。

### 推荐横向拆分

| 目标 QPS | 推荐实例数 | 单实例目标 |
| ---- | ---- | ---- |
| 100 | collection 1、apiserver 1、worker 1 | 单实例承接全量 |
| 200 | collection 1、apiserver 1、worker 1 | 单实例承接全量 |
| 300 | collection 1、apiserver 1、worker 1-2 | 单实例承接全量，worker 可拆 |
| 500 | collection 2、apiserver 2、worker 2 | 每套约 250 QPS |
| 700 | collection 3、apiserver 3、worker 2-3 | 每套约 230-250 QPS |
| 900 | collection 4、apiserver 4、worker 3-4 | 每套约 225-250 QPS |
| 1000 | collection 4、apiserver 4、worker 4 | 每套约 250 QPS |

Compose 文件当前写了固定 `container_name`，不适合直接 `docker compose up --scale`。要做横向扩容，应改成无固定 `container_name` 的服务模板，或者迁移到 Swarm / Kubernetes，通过 LB 把流量分摊到多个 collection-server；collection 再通过服务发现或 LB 访问多个 apiserver。

---

## 主机硬件建议

### 只跑 QS 三个应用容器

| 目标 QPS | 应用节点建议 |
| ---- | ---- |
| 100 | 2 vCPU / 4GiB |
| 200 | 4 vCPU / 8GiB |
| 300 | 4-6 vCPU / 12GiB |
| 500 | 2 台 4 vCPU / 8GiB，或 1 台 8 vCPU / 16GiB |
| 700 | 3 台 4 vCPU / 8GiB，或 2 台 8 vCPU / 16GiB |
| 900 | 4 台 4 vCPU / 8GiB，或 2-3 台 8 vCPU / 16GiB |
| 1000 | 4 台 4 vCPU / 8GiB 起，推荐 3 台 8 vCPU / 16GiB |

### 应用与数据层同机

只建议用于 100-200 QPS 或测试环境：

| 目标 QPS | 最低整机建议 | 更稳妥建议 |
| ---- | ---- | ---- |
| 100 | 4 vCPU / 8GiB / SSD | 4 vCPU / 16GiB |
| 200 | 4 vCPU / 16GiB / SSD | 8 vCPU / 16GiB |
| 300 | 8 vCPU / 24GiB / SSD | 8 vCPU / 32GiB，并开始拆数据层 |
| 500+ | 不建议同机 | 应用、MySQL、Mongo、Redis、NSQ 分离 |

### 独立数据层建议

| 目标 QPS | MySQL | MongoDB | Redis | NSQ / MQ |
| ---- | ---- | ---- | ---- | ---- |
| 100 | 2 vCPU / 4GiB | 2 vCPU / 4GiB | 1 vCPU / 1GiB | 1 vCPU / 1GiB |
| 200 | 4 vCPU / 8GiB | 4 vCPU / 8GiB | 1-2 vCPU / 2GiB | 1-2 vCPU / 2GiB |
| 300 | 4-8 vCPU / 16GiB | 4-8 vCPU / 16GiB | 2 vCPU / 4GiB | 2 vCPU / 4GiB |
| 500 | 8 vCPU / 32GiB | 8 vCPU / 32GiB | 2-4 vCPU / 4-8GiB | 2-4 vCPU / 4-8GiB |
| 700 | 12-16 vCPU / 48GiB | 12-16 vCPU / 48GiB | 4 vCPU / 8GiB | 4 vCPU / 8GiB |
| 900 | 16 vCPU / 64GiB | 16 vCPU / 64GiB | 4-8 vCPU / 8-16GiB | 4-8 vCPU / 8-16GiB |
| 1000 | 16-24 vCPU / 64GiB+ | 16-24 vCPU / 64GiB+ | 8 vCPU / 16GiB | 8 vCPU / 16GiB |

这张表假设读侧缓存命中率正常、慢查询已经治理、磁盘是 SSD/NVMe。若存在大统计查询、低缓存命中、批量导入或热点组织集中访问，应按实际压测上调。

---

## 按档位改配置的示例

### 200 QPS

保持当前生产配置基本可用，只需要确认以下值一致：

```yaml
# configs/apiserver.prod.yaml
rate_limit:
  submit_global_qps: 200
  submit_global_burst: 300
  query_global_qps: 200
  query_global_burst: 300

backpressure:
  mysql:
    max_inflight: 80
  mongo:
    max_inflight: 100
  iam:
    max_inflight: 40

mysql:
  max-open-connections: 80
```

```yaml
# configs/collection-server.prod.yaml
grpc_client:
  max_inflight: 80

submit_queue:
  queue_size: 500
  worker_count: 8

concurrency:
  max-concurrency: 40
```

### 500 QPS

500 QPS 推荐拆为 2 套应用实例，每套按 250 QPS 配置；如果临时单实例承接，需要同时提高容器资源和下游连接池。

```yaml
# configs/apiserver.prod.yaml
rate_limit:
  submit_global_qps: 500
  submit_global_burst: 750
  query_global_qps: 500
  query_global_burst: 750
  wait_report_global_qps: 250
  wait_report_global_burst: 375

backpressure:
  mysql:
    max_inflight: 200
  mongo:
    max_inflight: 240
  iam:
    max_inflight: 100

mysql:
  max-open-connections: 200

redis:
  max-active: 500
```

```yaml
# configs/collection-server.prod.yaml
rate_limit:
  submit_global_qps: 500
  submit_global_burst: 750
  query_global_qps: 500
  query_global_burst: 750
  wait_report_global_qps: 250
  wait_report_global_burst: 375

grpc_client:
  max_inflight: 200

submit_queue:
  queue_size: 1200
  worker_count: 20

concurrency:
  max-concurrency: 80
```

### 1000 QPS

1000 QPS 不建议单实例配置。推荐 4 套应用实例，每套按 250 QPS 左右配置，然后通过 LB 聚合到 1000 QPS。单实例硬拉到 1000 会让 collection 本地队列、apiserver 背压、MySQL 连接池、Mongo 连接池和 IAM 调用同时变成故障放大器。

每套实例建议：

```yaml
# 每套 collection-server
rate_limit:
  submit_global_qps: 250
  submit_global_burst: 400
  query_global_qps: 250
  query_global_burst: 400
  wait_report_global_qps: 125
  wait_report_global_burst: 200

grpc_client:
  max_inflight: 100

submit_queue:
  queue_size: 700
  worker_count: 10

concurrency:
  max-concurrency: 50
```

```yaml
# 每套 apiserver
rate_limit:
  submit_global_qps: 250
  submit_global_burst: 400
  query_global_qps: 250
  query_global_burst: 400
  wait_report_global_qps: 125
  wait_report_global_burst: 200

backpressure:
  mysql:
    max_inflight: 100
  mongo:
    max_inflight: 120
  iam:
    max_inflight: 50

mysql:
  max-open-connections: 100

redis:
  max-active: 250
```

集群总量约为：MySQL 背压 400、Mongo 背压 480、IAM 背压 200、collection 到 apiserver gRPC in-flight 400，与上面的 1000 QPS 总表一致。

---

## 验收指标

每个档位上线前至少做 10-20 分钟稳态压测，再做 1-3 分钟 1.5 倍 burst 压测。通过标准建议：

| 指标 | 目标 |
| ---- | ---- |
| HTTP 错误率 | `< 1%`，非预期 5xx 必须为 0 或可解释 |
| p95 延迟 | 普通查询 `< 500ms`，提交链路 `< 1000ms` |
| p99 延迟 | 普通查询 `< 1500ms`，提交链路 `< 3000ms` |
| 429 | 只应在超过目标 QPS 或 burst 时出现 |
| `backpressure_timeout` | 稳态不应持续出现 |
| SubmitQueue depth | burst 后应快速回落，不能持续增长 |
| MySQL / Mongo 慢查询 | 不应随 QPS 档位线性恶化 |
| NSQ depth | worker 消费后应回落，不应持续积压 |
| Go 内存 | RSS 应低于 `mem_limit`，GC 不应导致 p99 明显抖动 |

压测入口可参考：

```bash
RPS=200 DURATION=10m VUS=100 MAX_VUS=500 BASE_URL=http://127.0.0.1:8082 k6 run scripts/perf/k6-collection.js
```

提交链路压测需要提供真实 token 和答卷 payload；如果只压公开查询接口，不能代表答卷提交容量。

---

## 调参顺序

1. 先确定目标 QPS 和请求混合比例：公开查询、登录态查询、答卷提交、wait-report 各占多少。
2. 调 collection 入口 `rate_limit`，让入口目标和业务目标一致。
3. 调 collection `grpc_client.max_inflight`、`submit_queue` 和 `concurrency`，保证不会在 BFF 层过早排队。
4. 调 apiserver `rate_limit` 和 `backpressure`，让下游并发上限和数据库连接池一致。
5. 调 MySQL/Mongo/Redis/NSQ 资源，确认慢查询、连接池等待、Redis 慢命令和 MQ depth 不持续增长。
6. 调 Docker `cpus`、`mem_limit` 和 `GOMEMLIMIT`，确认 CPU 有 30% 以上余量、内存没有贴线。
7. QPS 500 以上优先横向扩容，再微调单实例参数。

---

## 常见错误

- 只把 `rate_limit.*_global_qps` 改到 1000，但不改连接池、背压、容器 CPU 和数据层资源。
- 把 `submit_queue.queue_size` 改得很大，以为等于吞吐提升；这只会把延迟和内存风险藏起来。
- 把 `backpressure.mysql.max_inflight` 设得高于 MySQL 可承载并发，导致 DB 锁等待和 p99 延迟恶化。
- QPS 700 以上仍用单个 collection-server，因为 `SubmitQueue` 不跨实例，单实例重启会丢本地队列状态。
- 压测时只打缓存命中的读接口，然后用结果承诺答卷提交 QPS。

---

## 相关文档

- [部署与端口](./03-部署与端口.md)
- [缓存与限流](../03-基础设施/03-缓存与限流.md)
- [Resilience Plane 文档中心](../03-基础设施/resilience/README.md)
- [SubmitQueue 提交削峰](../03-基础设施/resilience/02-SubmitQueue提交削峰.md)
- [Backpressure 下游背压](../03-基础设施/resilience/03-Backpressure下游背压.md)

---

*写作约定见 [CONTRIBUTING-DOCS.md](../CONTRIBUTING-DOCS.md)。*
