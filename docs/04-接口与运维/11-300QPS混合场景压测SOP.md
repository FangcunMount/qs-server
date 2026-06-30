# 300 QPS 混合场景压测 SOP

**本文回答**：如何在当前生产拓扑下，用 k6 对 qs-server 做三类压测（前台读写、异步链路 SLA、后台排水观测），分档升压（smoke → pretest → mixed_300 / mixed_300_models），如何准备 token/数据、如何观测、以及常见故障如何快速定位。

---

## 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| 压测入口 | k6 打公网域名 `collect.fangcunmount.cn` + `qs.fangcunmount.cn` |
| **脚本入口** | `scripts/perf/k6/mixed.js`（`k6-mixed-300qps.js` 为兼容 shim） |
| **Makefile 入口** | `make help` → **K6 压测** 段；`make perf-init` → `perf-tokens` → `perf-preflight` → `perf-smoke` … |
| 三类目标 | **前台读写**（model query/submit/wait-report）；**异步 SLA**（`async_chain_probe_*`）；**后台排水**（`perf-outbox120` + DB snapshot） |
| 推荐升档 | `smoke_4` → `pretest_60` → `pretest_120` → `mixed_140_submit24`（或 `mixed_140`）→ `mixed_160`…`mixed_280` → `mixed_300` / `mixed_300_models`，每档通过再升 |
| 配置入口 | `tmp/perf/qs-perf.config.json`（`make perf-init` 从 example 初始化，不覆盖已有文件） |
| 压测前必做 | 换新鲜 token + `check-token-preflight.sh` + 确认 Nginx 对压测 IP 已放宽 `limit_conn` |
| 验收 | K6 HTTP + chain probe SLA **且**（outbox/scanner 场景）压测前后 DB snapshot 不堆积；**优化后** outbox 可在压测结束后数分钟内消化，与 HTTP 429 可并存 |

---

## 1. 生产拓扑与压测链路

```text
压测机(k6)
  → collect.fangcunmount.cn ─┐
  → qs.fangcunmount.cn     ─┤→ serverA: Nginx
                             │     ├─ qs-collection-server:8080（同机 Docker 网）
                             │     └─ qs-apiserver:8080/9090
                             │
serverD: qs-worker ──gRPC──→ qs-apiserver:9090（异步 MQ 链路，一般不直接导致 wait-report 502）
serverB: iam-apiserver（IAM gRPC，overlay IP 如 172.20.0.28）
```

A/B 通过 Swarm overlay **`infra-network`** 互通；`iam-apiserver` 须由 **Docker DNS** 解析到 overlay 地址。**禁止** `extra_hosts` 把 `iam-apiserver` 指到 serverB 宿主机 Tailscale IP——serverB 宿主机 **9090 常被 mihomo 占用**，会导致 IAM mTLS 报 `first record does not look like a TLS handshake`。

| 节点 | 规格（2026-06） | 组件 | 压测相关性 |
| ---- | --------------- | ---- | ---------- |
| serverA | 8C/16G | nginx、qs-apiserver（5C/8G）、qs-collection-server（2C/4G） | **HTTP 同步链路主战场** |
| serverB | 2C/2G | IAM | JWKS / gRPC；非 HTTP 压测主瓶颈 |
| serverD | 4C/4G | qs-worker | 异步消费；`pretest_*` 阶段通常不是 HTTP 5xx 主因 |

压测从**公网**打入，client IP 为压测机公网地址（曾用 `61.49.247.106`）。Nginx 在 serverA 统一入口，**应用容器日志无 5xx 但 k6 有 5xx 时，优先查 Nginx access/error**。

---

## 2. 场景配比与档位

### 2.1 mixed_300 目标配比（多 model，产品真实 submit）

`mixed_300` / `mixed_300_probe` 使用**拆分场景**（profile 含 `medicalQuery` 等字段时自动启用）。**submit 按单机稳态上限 24/s 设定**（2026-06 压测：`mixed_140_submit24` 全绿）；原 60/s 为过度预留，已下调。

| 场景 | QPS | 接口 |
| ---- | ---: | ---- |
| 医学量表目录查询 | 80 | `GET /api/v1/scales*` |
| 人格模型目录查询 | 40 | `GET /api/v1/personality-models*` |
| 问卷详情查询 | 13 + 13 | `GET /api/v1/questionnaires/{code}` |
| 答卷提交（medical 80% + personality 20%） | **24** | `POST /api/v1/answersheets` |
| 报告 wait（热样本） | 100 | medical/personality `wait-report` |
| 统计查询 | 29 | apiserver `GET /api/v1/statistics/*` |
| 异步链路探针 | 1 | session → submit → … → wait-report |
| **合计** | **~300** | 读/report 为主，submit 与生产峰值对齐 |

**压测三类目标**：① 前台读写（query/submit/report 热样本）② 异步 SLA（`async_chain_probe_*`）③ 后台排水（`outbox_120` + snapshot DB）。

### 2.2 内置 `QPS_PROFILE`

| 档位 | 总 QPS | 配比（query/submit/report/stats） | 时长 | 用途 |
| ---- | ---: | -------------------------------- | ---- | ---- |
| `smoke_4` | 4 | 1/1/1/1 | 30s | 连通性、setup 自动发现 |
| `pretest_60` | 60 | 25/10/19/6 | 3m | **第一档正式预压** |
| `pretest_120` | 120 | 51/19/38/12 | 5m | 观察限流与资源曲线 |
| `pretest_120_submit_only` | 19 | 0/19/0/0 | 5m | 隔离 submit |
| `pretest_120_balanced` | 92 | 34/19/26/13 | 5m | 混合降读压验收档 |
| `mixed_140` | 140 | 58/24/44/14 | 5m | 120→300 细粒度升档 |
| `mixed_140_submit24` | 136 | 59/19/44/14 | 5m | 读/report 升档，submit 隔离 19/s |
| `mixed_160` | 160 | 68/24/52/16 | 5m | 同上（submit 封顶 24/s） |
| `mixed_180` | 180 | 80/24/58/18 | 5m | 同上 |
| `mixed_200` | 200 | 92/24/64/20 | 5m | 接近 prod 保守基线 |
| `mixed_240` | 240 | 112/24/80/24 | 8m | 同上 |
| `mixed_280` | 280 | 132/24/96/28 | 8m | 加压读/report |
| `mixed_300` | ~300 | 146 query + **24 submit** + 100 report + 29 stats + 1 probe | 10m | **验收档** |
| `mixed_300_probe` | 同上 | 同上 | 10m | 与 mixed_300 等效 |
| `mixed_300_models` | ~291 | 医学+人格拆分，submit 合计 **20/s** | 10m | 产品真实流量混合 |
| `outbox_120` | ~235 | submit/report 各 **96** + 低 query | 10m | **专测 outbox 排水** |
| `personality_60` | ~60 | 人格 session/submit/wait | 5m | 人格链路专项 |
| `capacity_no_scanner` | ~271 | 同 mixed_300 | 10m | 主链路容量（scanner 可关） |
| `capacity_with_scanner` | ~271 | 同 mixed_300 | 10m | 开启 `behavior_journey_scan` 后跑 |

旧档 `pretest_*` / `mixed_140`…仍用 `qps.query` 单桶。**submit 自 2026-06 统一下调**：`pretest` 约 ×0.8，`mixed_140` 及以上封顶 **24/s**，少掉的 QPS 补到 query/report。

### 2.3 K6 脚本目录

```
scripts/perf/k6/
  mixed.js              # 入口（Makefile PERF_K6_SCRIPT）
  lib/config.js         # 环境变量、profile、路径、token
  lib/metrics.js        # 指标、阈值、scenario 注册
  lib/http.js           # HTTP 封装
  lib/data.js           # discovery、答卷构造、scenarioData
  lib/util.js           # 工具函数
  lib/options.js        # 按 profile 注册 scenarios
  scenarios/            # model-query / submit / report / statistics / chain-probe
scripts/perf/k6-mixed-300qps.js   # 兼容 shim → re-export mixed.js
```

**原则**：上一档 `http_req_failed < 1%`、无明显 503/502 尖刺后再升档。`pretest_60` 曾出现 p90 ~5s 但 0 失败，可接受；`pretest_120` 在跨机 collection 时大量 502/30s 超时，迁回 serverA 后应复测。

**2026-06-30 优化后复测（outbox limiter + immediate 并发 + published model 缓存 + 连接池预算）**：

| 档位 | HTTP 结论 | outbox 排水 | 备注 |
| ---- | --------- | ----------- | ---- |
| `pretest_60` | **通过**：submit 100%，p95≈184ms | 脚本结束前后基本消化完 | 稳态 submit≈12/s |
| `pretest_120_balanced` | **通过**：100%，0×429 | — | 92QPS 混合（32/24/24/12） |
| `pretest_120` | **通过**：100%，0×429，submit p95≈192ms | 待观测 | 调优后全量 120QPS 混合已过线 |
| `mixed_140` | **未通过**：worker32 90.4%/806×429；worker40 **97.83%**/182×429 | — | 有效 ~25.5/s；待 worker48 复测 |
| `mixed_140_submit24` | **通过**：100%，0×429，submit p95≈222ms | — | 56/42/14 读压下 submit 24/s 稳 |

`pretest_120` 未过线时优先看 `http_429_total`（collection `submit queue full`）与 `questionnaire_query_timeout`，不要误判为 outbox 堆积。`mixed_140` 仅 submit 超容量时，先跑 `mixed_140_submit24` 分离读压与 submit 上限。

**wait-report VU sizing**：`wait-report` 是长轮询接口，`report` 场景的 VU 不宜远高于 `report_rps * timeout`。已验证 `pretest_120` 使用 `report=36 QPS`、`REPORT_VUS=220`、`REPORT_MAX_VUS=500` 可 0 失败完成；`mixed_300` 先使用 `report=90 QPS`、`REPORT_VUS=600`、`REPORT_MAX_VUS=900`。过大的 report VU（如 `700/1800` 或 `900/2200`）会制造大量空闲连接，k6 可能在请求进入 Nginx access 前出现 EOF。

---

## 3. 一次性准备

### 3.1 配置文件

```bash
mkdir -p tmp/perf
cp scripts/perf/qs-perf.config.example.json tmp/perf/qs-perf.config.json
```

已有 `tmp/perf/qs-perf.config.json` 时**不要覆盖**其中的 URL/token 路径；只合并示例里新增的 `qpsProfiles` 字段。

常用字段见 `scripts/perf/qs-perf.config.example.json`：`collectionBaseUrl`、`apiserverBaseUrl`、`tokensFile`、`qpsProfile`、`scaleCodes`、`planIds`、`autoDiscoverSeeddata` 等。相对路径按配置文件所在目录解析。

### 3.2 seeddata 数据口径

与线上一致（`seeddata-runner`）：

| 字段 | 值 |
| ---- | ---- |
| `COLLECTION_BASE_URL` | `https://collect.fangcunmount.cn` |
| `APISERVER_BASE_URL` | `https://qs.fangcunmount.cn` |
| `ORG_ID` | `1` |
| `SCALE_CODES` | `3adyDE,…,tssl35`（医学量表） |
| `PERSONALITY_MODEL_CODES` | `MBTI_OEJTS,SBTI_FUN` |
| `modelMix` | `{ medical: 0.8, personality: 0.2 }`（legacy submit 桶） |
| `PLAN_IDS` | `614333603412718126,614187067651404334` |
| `TESTEE_SOURCE` | `daily_simulation` |

### 3.3 多用户 token（必做）

collection 默认单用户限流：`submit=5`、`query=10`、`wait-report=2` QPS。要打 `report=90` QPS，至少 **45 个 collection token**（建议 **60+**）。

```bash
make perf-init
# 编辑 tmp/perf/iam-users.json，勿提交 Git
make perf-tokens
```

或分步：`make perf-tokens-collection` / `make perf-tokens-apiserver`。

`qs-perf.config.example.json` 已默认 `tokensFile` + `apiserverTokensFile`。**不要**用 `cp example` 覆盖已有 `tmp/perf/qs-perf.config.json`（会丢掉自定义 URL/token 路径）。

preflight 在未配置 `apiserverTokensFile` 时，会用 `tokensFile` 探测 apiserver，此时 `apiserver testees: 403` **是预期现象**，不代表 collection token 坏了。

### 3.4 Token preflight（每次 k6 前）

IAM access token TTL 短；**换完 token 立刻跑**，不要隔几小时再用同一文件。

```bash
make perf-preflight
```

通过标准：

- `expired=0`
- collection catalog 探测均为 **2xx**（scales 列表/categories/hot/详情、personality-models 列表/categories/详情；questionnaire 在能解析到 `questionnaire_code` 时探测）
- `apiserver testees: 200`（或你配置的 limit）
- `min_ttl_seconds` **大于本轮压测时长**（`mixed_300` 为 10 分钟，建议 TTL > 15 分钟）

`smoke_4` profile 已将 `questionnaire_query` 收窄为 preflight 同款稳定路径（scale 详情 + personality-models 列表），避免 smoke 随机打到未预检的 catalog 接口。

首次 `pretest_60` 若 `expired=99/101`，会得到约 **7% 401**，与网关无关，刷新 token 即可。

---

## 4. 压测前检查清单

| # | 检查项 | 命令/方法 |
| - | ------ | --------- |
| 1 | token 新鲜且 preflight 通过 | `check-token-preflight.sh` |
| 2 | Nginx 对压测 IP 放宽连接数 | 见 [§7 Nginx 专项](#7-nginx-专项) |
| 3 | collection 在 serverA 运行 | `docker ps` 见 `qs-collection-server`；Nginx upstream `qs-collection-server:8080` |
| 4 | 压测机公网 IP 已知 | 用于 Nginx `http-conn-limit.conf` 白名单 |
| 5 | （可选）metrics 隧道 | Mac 上 SSH 隧道采 snapshot，见 [§6](#6-观测采样) |

---

## 5. 执行命令

### 5.1 Makefile（推荐）

```bash
make perf-init              # 首次：初始化 tmp/perf，不覆盖已有配置
# 编辑 tmp/perf/iam-users.json

make perf-tokens            # collection + apiserver 两套 token
make perf-preflight         # 预检（apiserver testees 须 200）

make perf-smoke             # smoke_4
make perf-pretest60         # pretest_60，summary → tmp/perf/pretest60/
make perf-pretest120        # pretest_120
make perf-pretest120-submit-only  # 仅 submit=19QPS 隔离复测
make perf-pretest120-balanced     # 混合降读压 34/19/26/13
make perf-mixed140          # mixed_140
make perf-mixed140-submit24 # mixed_140 读压 + submit=19
make perf-mixed160          # mixed_160
make perf-mixed180          # mixed_180
make perf-mixed200          # mixed_200
make perf-mixed240          # mixed_240
make perf-mixed280          # mixed_280
make perf-mixed300          # mixed_300 + 前后 snapshot
make perf-mixed300probe     # mixed_300_probe（与 mixed_300 等效）
make perf-mixed300-models   # 医学+人格拆分 submit/report
make perf-outbox120         # outbox 排水专测 + snapshot
make perf-personality60     # 人格 session 链路
make perf-mixed300-scanner  # 需先开启 behavior_journey_scan

make perf-k6 QPS_PROFILE=mixed_300_models
make perf-verify            # bash 语法 + k6 inspect（入口 + shim）
```

完整列表：`make help`（**K6 压测** 段）。

### 5.2 标准升档流程（原始 k6 命令）

```bash
# 0. make perf-tokens && make perf-preflight

make perf-smoke
make perf-pretest60
make perf-pretest120
make perf-mixed300
```

等价手写：

```bash
k6 run -e PERF_CONFIG_FILE="$(pwd)/tmp/perf/qs-perf.config.json" \
  -e QPS_PROFILE=smoke_4 scripts/perf/k6/mixed.js
```

优先级：**命令行 `-e` > `QPS_PROFILE` 档位 > 根配置 > 脚本默认**。

### 5.3 只读 smoke（无需 token）

```bash
COLLECTION_BASE_URL=https://collect.fangcunmount.cn \
QUERY_RPS=1 SUBMIT_RPS=0 REPORT_RPS=0 STATS_RPS=0 DURATION=5s \
QUESTIONNAIRE_QUERY_PATHS='/api/v1/scales?page=1&page_size=20&status=published' \
k6 run scripts/perf/k6/mixed.js
```

### 5.4 严格阈值（可选）

```bash
STRICT_THRESHOLDS=true k6 run -e PERF_CONFIG_FILE=tmp/perf/qs-perf.config.json \
  -e QPS_PROFILE=pretest_60 scripts/perf/k6/mixed.js
```

---

## 6. 观测采样

### 6.1 脚本 snapshot

```bash
OUT_DIR=tmp/perf/pretest60 ./scripts/perf/snapshot-observability.sh before
# ... k6 ...
OUT_DIR=tmp/perf/pretest60 ./scripts/perf/snapshot-observability.sh after
```

默认 URL 走本机隧道端口；在 **Mac** 上先开 SSH 隧道（压测期间保持）：

```bash
# serverA — apiserver metrics
ssh -N -L 18082:127.0.0.1:8081 root@<serverA-tailscale-ip>

# serverA — collection metrics（collection 迁回 A 后）
ssh -N -L 18083:127.0.0.1:8082 root@<serverA-tailscale-ip>
```

```bash
export COLLECTION_METRICS_URL=http://127.0.0.1:18083/metrics
export COLLECTION_RESILIENCE_URL=http://127.0.0.1:18083/governance/resilience
export APISERVER_METRICS_URL=http://127.0.0.1:18082/metrics
```

worker（serverD）、NSQ 隧道为可选项；snapshot 失败写 `.err`，**不阻断** k6。

**DB 快照（可选，压测前后对比 outbox/scanner）**：

```bash
export MONGO_URI='mongodb://user:pass@host:27017/db?authSource=admin'
export MYSQL_CLI_ARGS='-h127.0.0.1 -uuser -ppass dbname'
export PERF_ORG_ID=1
OUT_DIR=tmp/perf/300qps ./scripts/perf/snapshot-observability.sh after
```

生成文件包括：`mongo-outbox.json`、`mysql-outbox.txt`、`analytics-scan-watermarks.txt`、`assessment-episode-status.txt`、`behavior-footprint-events.txt`、`statistics-journey-daily.txt`。

**outbox 是否追上（优化后判据）**：压测期间 k6 可能仍有 429/超时，但**压测结束后 3 分钟内** `domain_event_outbox` 的 `pending/failed/publishing` 应回落到接近 0。若仍长时间堆积，再查 relay worker、Mongo 慢查与连接池；若已快速消化而 HTTP 仍失败，主因通常在 collection 入口限流或读压饱和。

### 6.2 关键指标

| 指标 | 来源 |
| ---- | ---- |
| HTTP 失败率 / 延迟 | k6 summary、`http_5xx_total` 等自定义 counter |
| SubmitQueue | collection `/governance/resilience` |
| Outbox / MQ | apiserver event status、worker metrics、NSQ stats |
| 容器资源 | serverA 上 `docker stats qs-apiserver qs-collection-server nginx` |
| Nginx 5xx | `/data/logs/nginx/access.log` 按时段统计 `$9` |

300 QPS 混合压测中，如果 NSQ depth 为 0 但 report 长时间不生成，必须先查 `mongo-domain-events`。事件可能还卡在 Mongo outbox，尚未进入 NSQ：

```javascript
db.domain_event_outbox.aggregate([
  {$match:{status:{$in:["pending","failed","publishing"]}}},
  {$group:{_id:{status:"$status",event_type:"$event_type",topic:"$topic_name"}, n:{$sum:1}, oldest:{$min:"$created_at"}, newest:{$max:"$created_at"}}},
  {$sort:{n:-1}},
  {$limit:50}
])
```

### 6.3 k6 summary 解读

失败分类优先看自定义 counter：

| counter | 含义 |
| ------- | ---- |
| `http_401_total` | token 过期或无效 → 先换 token |
| `http_403_total` | 权限不足 → 检查 token 分组 |
| `http_4xx_total` | 所有 4xx 汇总；再看 401/403/429 细分 |
| `http_429_total` | 应用层限流 → 加 token 或调 `rate_limit` |
| `http_5xx_total` | 服务端或 **Nginx 503/502** → 按 §8 分流 |
| `http_transport_error_total` | 没拿到 HTTP 响应，例如客户端 30s 超时 |
| `http_timeout_total` | `request timeout` 汇总；注意不是 `wait-report?timeout=5` |

| `chain_probe_failed` | 异步探针失败次数（应接近 0） |
| `medical_report_generated_latency` / `personality_report_generated_latency` | 端到端报告 SLA |
| `submit_to_assessment_latency` / `assessment_to_report_latency` | 异步分段延迟 |

混合压测脚本会额外输出按场景拆分的失败 counter：

| counter 形态 | 含义 |
| ------------ | ---- |
| `report_status_5xx` / `answer_submit_5xx` / `questionnaire_query_5xx` / `statistics_5xx` | 哪类接口返回了 5xx |
| `report_status_transport_error` / `answer_submit_transport_error` / `questionnaire_query_transport_error` / `statistics_transport_error` | 哪类接口没有拿到 HTTP 响应 |
| `report_status_timeout` / `answer_submit_timeout` / `questionnaire_query_timeout` / `statistics_timeout` | 哪类接口打满了 k6 `HTTP_TIMEOUT` |

新版 k6 `--summary-export` JSON schema 可能变化；若 `jq '.metrics.*.values.count'` 为 null：

```bash
jq 'keys' tmp/perf/pretest60/k6-summary.json
jq '.metrics.http_req_failed' tmp/perf/pretest60/k6-summary.json
```

---

## 7. Nginx 专项

Nginx **仅在 serverA**。仓库配置：

- `configs/nginx/http-conn-limit.conf` — 压测 IP 白名单 + 默认 20 连接/IP
- `configs/nginx/conf.d/collect.fangcunmount.cn.conf` — upstream `keepalive 512`
- `configs/nginx/conf.d/qs.fangcunmount.cn.conf` — 同上

宿主机路径（示例）：

| 路径 | 说明 |
| ---- | ---- |
| `/opt/infra/components/nginx/nginx.conf` | 主配置 |
| `/data/apps/nginx-configs` | `conf.d/apps` |
| `/opt/infra/components/nginx/conf.d` | `conf.d` |

### 7.1 压测前确认 limit_conn

```bash
docker exec nginx grep -E 'limit_conn|conn_limit' /etc/nginx/nginx.conf /etc/nginx/conf.d/ 2>/dev/null
```

`http-conn-limit.conf` 示例：压测机 IP → 5000 连接，其他 IP → 20。

### 7.2 修改后必须 restart

bind mount 场景下 **`nginx -s reload` 可能未同步宿主机改动**；改完宿主机文件后：

```bash
docker exec nginx nginx -t && docker restart nginx
docker exec nginx grep limit_conn /etc/nginx/nginx.conf
```

若宿主机与容器 `md5sum` 不一致，说明未生效，必须 restart。

### 7.3 503 特征（limit_conn）

access log：`503` 且耗时 **0.000s**；error log：`limiting connections by zone "conn_limit_per_ip"`。此时 **collection/apiserver 容器日志通常无对应 5xx**。

---

## 8. 常见故障与根因（实测）

| 现象 | 根因 | 处理 |
| ---- | ---- | ---- |
| ~7% 失败，k6 无 429，collection 日志全 200 | token 过期 → **401**（计入 failed） | 换 token + preflight |
| k6 `http_5xx`，collection **0 条 5xx**，nginx access `503` 0.000s | **Nginx limit_conn** | 白名单压测 IP 或临时调高；`docker restart nginx` |
| `pretest_120` 大量 502，`upstream prematurely closed` | 跨机 upstream（历史：collection 在 serverB） | collection 迁 serverA；调大 upstream `keepalive` |
| `wait-report` k6 EOF，但 Nginx access 只有成功数、error log 为空 | k6 report VU 过大导致入口前连接复用异常 | 下调 `REPORT_VUS/REPORT_MAX_VUS`；`pretest_120` 用 `220/500`，`mixed_300` 先用 `600/900` |
| NSQ depth 为 0，但 report 不生成且 Mongo outbox pending 激增 | 事件卡在 `mongo-domain-events`，还没发布到 NSQ | 查 `outbox_relay.mongo.interval/batch_size/publish_workers`、主链路事件 pending/publishing 与 Mongo 慢查询 |
| pretest120 collection 大量 429、`submit queue full`，apiserver Mongo `idle connections: 0` | immediate 无上限 goroutine + `publish_workers=128` 与业务读抢 Mongo 连接池 | 降 `publish_workers`（48）、设 `immediate_max_concurrent`（16）；`backpressure.mongo.max_inflight`≤80；outbox Store 走 limiter；`published_assessment_models` 加 Redis 缓存；immediate 仅 `answersheet.submitted` |
| 优化后 outbox 3min 内消化，但 pretest120 submit≈96%、`http_429`≈250、questionnaire 读超时 | collection submit 队列背压 + 48/s 问卷读压饱和（apiserver 侧 Mongo 争用已缓解） | 部署 collection `worker_count=32, queue_size=1600`；跑 `pretest_120_balanced`（32/24/24/12）；隔离 submit 见 `pretest_120_submit_only` |
| `mixed_140` submit≈90%→98%、仍 182×429（worker40） | submit 28/s 超混合稳态吞吐，apiserver 变慢拉高单次耗时 | 上调 `worker_count=48, queue_size=2400` 复测；仍不足则查 apiserver backpressure |
| 502 无 503，upstream 超时 | apiserver/collection 过载或 gRPC 背压 | 看 `docker stats`、backpressure metrics |
| k6 30s 超时增多 | 下游排队或 wait-report 长轮询 | 区分场景；非终态 assessment 会拉高 report P95 |
| `setup_discovery_failed` + 403 | token 无 apiserver 权限 | 单独 `apiserver_users` token |
| worker/NSQ snapshot 失败 | 未建隧道 | 可忽略，不影响 HTTP 压测结论 |

**排障顺序**：k6 counter 分类 → Nginx access（是否 503/0.000s）→ 应用 access log（`middleware/logger.go` 状态码分布）→ `docker stats` → gRPC/backpressure。

### 8.1 混合场景失败分流

先看场景级 counter，不要只看总 `http_req_failed`：

| 现象 | 下一步 |
| ---- | ------ |
| `*_5xx` 高，`*_timeout` 低 | 查 Nginx access/error 与应用 access，确认是网关 502/503 还是应用 5xx |
| `*_timeout` 高，P95 接近 `HTTP_TIMEOUT` | 查应用是否排队到 30s：SubmitQueue、gRPC max inflight、apiserver backpressure、DB 慢查询 |
| `report_status_timeout` 高于其他场景 | 区分正常长轮询等待和下游阻塞；`wait-report?timeout=5` 应约 5s 内返回 pending，不应拖到 30s |
| `answer_submit_5xx` 或 `answer_submit_timeout` 高 | 优先查 collection SubmitQueue 与 collection → apiserver gRPC；再查 apiserver durable submit、Mongo/MySQL/NSQ |
| 四类场景同时 timeout | 优先查 serverA 入口和 apiserver 进程整体饱和，而不是单个接口逻辑 |

### 8.2 在 serverA 统计应用 HTTP 状态码

```bash
docker logs qs-collection-server --since 10m 2>&1 \
  | grep 'middleware/logger.go' \
  | grep -oE ' [0-9]{3} - ' | sort | uniq -c | sort -rn
```

勿用 `grep timeout` 粗筛——query 参数 `timeout=5` 会误匹配。

### 8.3 在 serverA 统计 Nginx 5xx

```bash
awk '{print $9}' /data/logs/nginx/access.log | sort | uniq -c | sort -rn | head -20
awk '$9 ~ /^5/' /data/logs/nginx/access.log | tail -20
```

---

## 9. ghz gRPC 压测（可选）

HTTP 混合压测通过后，可用 `scripts/perf/ghz-qs-grpc.sh` 单独压 gRPC 等价链路（collection submit、worker internal 等）。生产 gRPC 为 mTLS，需挂载 `/data/infra/ssl/grpc` 证书；仅 dev plaintext 时设 `GRPC_PLAINTEXT=true`。

详见脚本内 `CASE` 说明；非 `mixed_300` 验收必选项。

---

## 10. 结果记录模板

### 10.1 环境

| 项 | 填写 |
| -- | ---- |
| 日期 / Git SHA | |
| serverA 规格 | 8C/16G，apiserver 5C/8G，collection 2C/4G |
| serverD worker 副本 | |
| 压测机公网 IP | |
| Nginx conn_limit 压测 IP 配额 | |
| token 数量（collection / apiserver） | |
| `QPS_PROFILE` | |

### 10.2 HTTP 结果

| 档位 | 总 QPS | http_req_failed | http_5xx | http_401 | http_429 | P95 | 结论 |
| ---- | ---: | ---: | ---: | ---: | ---: | ---: | ---- |
| pretest_60 | 60 | 0% | 0 | 0 | 0 | submit p95≈184ms | **通过**（2026-06-30） |
| pretest_120_balanced | 92 | 0% | 0 | 0 | 0 | submit p95≈250ms | **通过**（2026-06-30） |
| pretest_120 | 120 | 0% | 0 | 0 | 0 | submit p95≈192ms | **通过**（2026-06-30，worker32+apiserver优化后） |
| mixed_140 | 140 | 0.44% | 0 | 0 | 182 | submit p95≈1.11s | **未通过**（2026-06-30，worker40，有效~25.5/s） |
| mixed_140_submit24 | 136 | 0% | 0 | 0 | 0 | submit p95≈222ms | **通过**（2026-06-30） |
| mixed_160 | 160 | | | | | | |
| mixed_180 | 180 | | | | | | |
| mixed_200 | 200 | | | | | | |
| mixed_240 | 240 | | | | | | |
| mixed_280 | 280 | | | | | | |
| mixed_300 | 300 | | | | | | |

### 10.4 验收标准（HTTP + 后台）

| 维度 | 通过条件 |
| ---- | -------- |
| K6 | `http_req_failed < 1%`；submit 202 > 99%；wait-report 200 > 99%；`chain_probe_failed` ≈ 0 |
| 延迟 | medical report p95 < 60s；personality report p95 < 90s（probe 开启时） |
| Outbox | Mongo/MySQL pending 不持续增长；`failed` = 0 或可解释 |
| Scanner | 若启用 `behavior_journey_scan`：`analytics_scan_watermarks` 推进；`statistics_journey_daily` 更新 |

---

## 11. Verify

```bash
make perf-verify
```

---

## 相关文档

- 容量与资源配置：`10-QPS容量档位与资源配置建议.md`
- 部署拓扑：`../../.github/workflows/README.md`（serverA/B/D）
- Nginx 配置：`configs/nginx/http-conn-limit.conf`、`configs/nginx/conf.d/collect.fangcunmount.cn.conf`
