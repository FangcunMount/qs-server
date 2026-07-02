# 300 QPS 混合场景压测 SOP

---

## 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| 入口 | `make perf-init` → `perf-tokens` → `perf-preflight` → `perf-smoke` → 按 **L0～L4** 升档；详见 §K6 分档命令；脚本 `scripts/perf/k6/mixed.js` |
| 配置 | `tmp/perf/qs-perf.config.json`（`make perf-init` 不覆盖已有文件） |
| 三类目标 | ① 前台读写 ② 异步 SLA（`chain_probe`）③ 后台排水（`outbox_120` + DB snapshot） |
| 当前水位 | **4C/8G** 至 **`mixed_240_models` 全绿**；**`mixed_280_models` 边际**（0.01% failed、~41s 尖刺，非 7min 雪崩）；300 见 8C/16G 历史 |
| Report | 常规默认 **`short_poll`**；长轮询仅专项 `special_report_long_poll` |
| 验收 | `http_req_failed < 1%` + chain probe SLA；压测结束 **≤3min** outbox 回落至近 0 |
| 必做 | 新鲜 token + `perf-preflight` + Nginx 压测 IP 放宽 `limit_conn` |

**推荐路径**：

```text
smoke_4 → pretest_60 → pretest_120 → mixed_140…220 → mixed_240_models → mixed_280_models → mixed_300
```

攻关档（可选分步验证）：`perf-mixed300-http`（Step1）→ `perf-mixed300-http-query`（Step2）→ `perf-mixed300`（全量）。

档间冷却 **≥30min**（攻关档 `mixed_280_models` 及以上建议 **≥30min**）。上一档全绿再升档。

部署 outbox 调优后验收：`make perf-mixed280-models`（确认 `reportMode=short_poll`、`vusers.report.max≈320`）；4C/8G 目标 **checks 100%**、无 timeout 尖刺；边际过线需**单独重跑**后再升 300。

---

## 压测设计

### 目标与验收

| 维度 | 通过条件 |
| ---- | -------- |
| HTTP | `http_req_failed < 1%`；submit 202 > 99%；report 200 > 99% |
| 异步 | `chain_probe_failed` ≈ 0；report p95 < 60s（medical）/ 90s（personality） |
| 读延迟 | 攻关档 query p95 < 500ms |
| Outbox | 压测结束 **≤3min** `domain_event_outbox` pending/failed/publishing 回落至近 0 |

HTTP 429 与 outbox 堆积可并存，勿混为一谈。

### 拓扑

```text
压测机(k6) → collect.fangcunmount.cn / qs.fangcunmount.cn → serverA(Nginx)
  ├─ qs-collection-server:8080
  └─ qs-apiserver:8080/9090
serverD: qs-worker ──gRPC──→ apiserver
serverB: iam-apiserver（overlay infra-network）
```

serverA：**当前 4C/8G**（2026-07-02 由 8C/16G 缩容）。280/300 验收在 **8C/16G** 完成；200/220 已在现规格复测通过。

压测从公网打入；**k6 有 5xx 但应用无 5xx → 先查 Nginx**。

### 准备与配置

```bash
make perf-init && make perf-tokens && make perf-preflight
make perf-sync-profiles    # 只补缺失 profile，不覆盖已有 qps/vusers/reportMode
make perf-sync-vusers      # 用 example 覆盖本地各档 vusers（4C/8G 收紧后必跑）
```

- **token**：`report≥90/s` 需 **60+** collection token（单用户 wait-report 限 2/s）
- **preflight**：`expired=0`；catalog 2xx；`min_ttl` > 压测时长
- **profile 陷阱**：本地配置须手改 `reportMode=short_poll`、`paths.reportStatus`；跑 Step1 前确认 setup 日志 `medical_model_query=71/s`

```bash
jq '.reportMode="short_poll"
  | .paths.reportStatus="/api/v1/assessments/{assessment_id}/report-status?testee_id={testee_id}"
  | .paths.personalityReportStatus="/api/v1/personality-assessments/{assessment_id}/report-status?testee_id={testee_id}"' \
  tmp/perf/qs-perf.config.json > tmp/perf/qs-perf.config.json.tmp \
  && mv tmp/perf/qs-perf.config.json.tmp tmp/perf/qs-perf.config.json
```

**检查清单**：token 新鲜 · Nginx 白名单 · collection 在 serverA · 压测 IP 已知

### `mixed_300` 与 280 档增量

在 **`mixed_280_models` 已通过** 基础上，`mixed_300` 仅做有限增量（submit 仍 **24/s**）：

| 维度 | mixed_280_models | mixed_300 | Δ |
| ---- | ---------------- | --------- | - |
| 医学 query | 71/s | 80/s | +9 |
| 人格 query | 36/s | 40/s | +4 |
| 问卷 query | 25/s | 13+13/s | +1 |
| report（short_poll） | 96/s | 100/s | +4 |
| stats | 28/s | 29/s | +1 |
| chain probe | 0 | 1/s | +1 |
| **读合计** | 132/s | 146/s | **+14** |

验收须 **`reportMode=short_poll`**、拆分 query VU（与 280 同结构）、apiserver **线 B**（stats 防击穿）已部署。

### Profile 与命令

| 档位 | 总 QPS | 配比（query/submit/report/stats） | 时长 | Make |
| ---- | ---: | -------------------------------- | ---- | ---- |
| `smoke_4` | 4 | 1/1/1/1 | 30s | `perf-smoke` |
| `pretest_60` | 60 | 25/10/19/6 | 3m | `perf-pretest60` |
| `pretest_120` | 120 | 51/19/38/12 | 5m | `perf-pretest120` |
| `mixed_140`～`mixed_220` | 140～220 | submit **24** 封顶 | 5m | `perf-mixed140`…`perf-mixed220` |
| `mixed_240_models` | 240 | 54+27+19 /24/88/28 | 8m | `perf-mixed240-models` |
| `mixed_280_models` | 280 | 71+36+25 /24/96/28 | 8m | `perf-mixed280-models` |
| `mixed_300_http` | ~281 | 71+36+25 /24/96/29 | 10m | `perf-mixed300-http`（Step1） |
| `mixed_300_http_query` | ~295 | 146 /24/96/29 | 10m | `perf-mixed300-http-query`（Step2） |
| `mixed_300` | ~300 | 146+24+100+29+probe | 10m | `perf-mixed300`（**全量验收，已通过**） |
| `mixed_300_http_query_nostats` | ~266 | 146 /24/96/0 | 10m | `perf-mixed300-http-query-nostats` |
| `stats_isolate_29` | 29 | 仅 stats | 10m | `perf-stats-isolate29` |

**专项（不进升档）**：`special_report_long_poll`（`perf-special-report-long-poll`）、`outbox_120`、`personality_60`、`mixed_280_models_ws`。完整列表：`make help` → K6 压测。

### K6 分档命令

**通用前置**（每轮压测前）：

```bash
make perf-init && make perf-tokens && make perf-preflight
```

**裸跑 k6 模板**（`make perf-*` 等价，便于自定义 `SUMMARY_EXPORT`）：

```bash
export PERF_CONFIG="$(pwd)/tmp/perf/qs-perf.config.json"
export PERF_ROOT="$(pwd)"
export K6_SCRIPT="scripts/perf/k6/mixed.js"

k6 run \
  -e PERF_CONFIG_FILE="$PERF_CONFIG" \
  -e PERF_ROOT_DIR="$PERF_ROOT" \
  -e QPS_PROFILE=<profile名> \
  --summary-export tmp/perf/<输出目录>/k6-summary.json \
  "$K6_SCRIPT"
```

**L0 连通（30s）**

```bash
make perf-smoke
# 等价：QPS_PROFILE=smoke_4 make perf-k6
```

**L1 预压（3～5min）**

```bash
make perf-pretest60              # pretest_60，3min
make perf-pretest120             # pretest_120，5min
make perf-pretest120-balanced    # 降读压混合，排查 submit 争用
make perf-pretest120-submit-only # 仅 submit，隔离读压
```

**L2 升档 mixed_140～220（5min，submit 封顶 24/s）**

```bash
make perf-mixed140
make perf-mixed160
make perf-mixed180
make perf-mixed200
make perf-mixed220
# 排查 submit 429：make perf-mixed140-submit24
```

**L3 高水位 mixed_240～280（8min，须三域拆分 + short_poll）**

```bash
make perf-mixed240-models    # 三域 L1 验收（54/27/19/s），升档必跑
make perf-mixed280-models    # 280 攻关档；4C/8G 建议冷却 ≥30min 后单独跑

# legacy 单桶问卷（仅对照，不作升档依据）：
# make perf-mixed240 / make perf-mixed280
```

**L4 300 攻关与全量验收（10min）**

前置：`mixed_280_models` 全绿 + apiserver 线 B 已部署 + 本地 `reportMode=short_poll`。

```bash
# 分步排查（推荐顺序）
make perf-mixed300-http              # Step1：同 280 读压，无 probe
make perf-mixed300-http-query        # Step2：满配 query 146/s
make perf-stats-isolate29            # stats 隔离（线 A）
make perf-mixed300-http-query-nostats # Step2 去 stats 对照

# 全量验收（含 chain_probe + 前后 outbox snapshot）
make perf-mixed300
```

**专项 / 诊断（不进常规升档链）**

```bash
make perf-special-report-long-poll   # 长轮询 report（生产已弃用）
make perf-mixed280-models-ws         # WebSocket report-events
make perf-outbox120                  # outbox 排水 + snapshot
make perf-personality60              # 人格专项
make perf-diag-query120              # 仅 query 48/s
make perf-diag-submit120             # 仅 submit 24/s
make perf-diag-report120             # 仅 report 36/s
```

**观测 snapshot**（`mixed_300` / `outbox_120` 等含 snapshot 的档位可手动补跑）：

```bash
OUT_DIR=tmp/perf/300qps ./scripts/perf/snapshot-observability.sh before
# … k6 压测 …
OUT_DIR=tmp/perf/300qps ./scripts/perf/snapshot-observability.sh after
```

### Report 模式

常规默认 **`short_poll`**（`/report-status`，96/s 时 max VU≈**320**）。`websocket` 走 `/report-events`；`long_poll` 仅专项 profile。

`qps.report` = 用户侧查报告频率，**不是** Nginx 裸 HTTP RPS。下游饱和时勿盲目加 VU。

### 观测

```bash
OUT_DIR=tmp/perf/300qps ./scripts/perf/snapshot-observability.sh before   # k6 后 after
```

k6 优先看：`http_401`（换 token）→ `http_429`（加 token）→ `http_5xx`（Nginx/应用）→ `*_timeout`（排队）。异步看 `chain_probe_failed`、`submit_to_assessment_latency`。

**stats 攻关**：Step2 边际通过时，用 `nostats` / `stats_isolate_29` 隔离；apiserver 部署 B1–B4（singleflight、启动预热、`stale_on_timeout`）后重跑 Step2。详见 `configs/apiserver.prod.yaml` `cache.statistics_*`。

**常见故障**：

| 现象 | 处理 |
| ---- | ---- |
| ~7% 失败，应用全 200 | token 过期 → preflight |
| nginx 503 0.000s，应用无 5xx | `limit_conn` 白名单 → `docker restart nginx` |
| report VU 触顶 / EOF | 确认 `short_poll`、max VU≈320 |
| mixed_220 无 L1 读超时 | 部署 Catalog L1，勿加 VU |
| mixed_140 大量 429 | submit 降至 24/s |
| mixed_280 边际过（0.01% + ~41s 尖刺） | 4C/8G 容量触顶 / 残余负载；冷却 ≥30min **单独**重跑；查 outbox pending |
| mixed_280 ~428s 全场景雪崩 | 连档升压 + 冷却不足（§3.8）；修正 report VU≈320、`short_poll` |

脚本：`scripts/perf/k6/mixed.js`；配置示例：`scripts/perf/qs-perf.config.example.json`。Catalog 缓存见 [10-Catalog目录L1-L2缓存.md](../03-基础设施/redis/10-Catalog目录L1-L2缓存.md)。

---

## 多档压测记录

> 截至 2026-07-02。新跑次用 §3.10 模板追加。

### 3.1 水位总览

| 档位 | 结论 | 关键数据 |
| ---- | ---- | -------- |
| pretest_60～mixed_240_models（8C/16G 历史） | **通过** | 见 §3.2–3.7 |
| mixed_300_http Step1 | **通过** | 同 280 读压 10min；query p95 284/256/359ms |
| mixed_300_http_query Step2 | **通过**（线 B 后） | checks 100%；stats 0×timeout |
| **`pretest_120`～`mixed_240_models`**（4C/8G，outbox 调优后） | **通过** | 见 §3.8.1；0% failed |
| **`mixed_280_models`**（4C/8G，outbox 调优后） | **边际** | 0.01% failed；~41s 20×timeout；见 §3.8.1 |
| **`mixed_280_models`**（4C/8G，缩容首日连跑） | **边际未过** | checks 99.84%；~428s 雪崩；见 §3.8 |
| `mixed_280_models` / `mixed_300` | **通过**（8C/16G） | 4C/8G 280 待单独重测全绿或扩容 |

---

### 3.2 轮次一：预压与基础设施（2026-06）

| 档位/场景 | 结论 | 遇到的问题 | 解决方案 |
| --------- | ---- | ---------- | -------- |
| 首次 `pretest_60` | 约 7% 失败 | k6 无 429，collection 日志全 200；实为 **token 过期 401** 计入 failed | 压测前 `make perf-tokens` + `perf-preflight`；确认 `expired=0`、`min_ttl` > 压测时长 |
| `pretest_60` 稳态 | **通过** | p90 偶发 ~5s 但 0 失败 | 可接受；实测 apiserver submit 稳态约 **12/s** |
| `pretest_120`（跨机） | **未通过** | collection 在 serverB 时大量 **502**、`upstream prematurely closed` | collection **迁回 serverA**；Nginx upstream `keepalive` 调大 |
| k6 5xx，应用无 5xx | 误判为应用故障 | Nginx **`limit_conn`**：access 503 耗时 **0.000s** | 压测 IP 加入 `http-conn-limit.conf` 白名单（5000 连接）；改配置后 **`docker restart nginx`**（勿只 reload） |
| IAM mTLS 握手失败 | 压测/setup 403 | `extra_hosts` 把 `iam-apiserver` 指到 serverB 宿主机 Tailscale；9090 被 mihomo 占用 | 仅用 Docker overlay **DNS** 解析 `iam-apiserver` |

---

### 3.3 轮次二：Mongo 连接池与 outbox（2026-06-30）

**背景优化项**（本轮前已部署）：outbox `MongoLimiter`、`immediate` 并发上限、`published_assessment_models` Redis 缓存、连接池预算。

| 档位 | 结论 | 遇到的问题 | 解决方案 |
| ---- | ---- | ---------- | -------- |
| `pretest_60` | **通过** submit p95≈184ms | — | outbox 压测结束 **≤3min** 消化；稳态 submit≈12/s |
| `pretest_120`（混合 48/24/36/12） | **未过阈值** submit 96.2%，248×429 | apiserver Mongo：**idle connections: 0**、`MarkEventPublished` 超时、publishing 堆积最老 10min；`published_assessment_models` 27–29s 超时 → gRPC 30s 失败 → collection **submit queue full** | 见下行分项 |
| ↑ 根因链 | — | relay **128 worker** + immediate 无上限 goroutine + 业务读 **共用 Mongo 100 连接池**，outbox 不占 backpressure 名额 | — |
| ↑ 代码/配置 | — | — | `publish_workers` 128→**48**；`immediate_max_concurrent` **16**；immediate 仅 `answersheet.submitted`；`backpressure.mongo.max_inflight` 170→**80**；`eventoutbox.Store` 接入 `MongoLimiter` |
| ↑ 缓存 | — | submit 热路径每次打 Mongo 查 published model | 部署 **`CachedPublishedModelStore`**（Redis，TTL 2h） |
| ↑ 混合读压 | questionnaire **209×timeout**，~86s 起 EOF | **非缓存失效**（固定 8 个 code 应全命中）；collection **2C** 上 48/s 读 + 24/s submit + 36/s report 争用，连缓存命中也排队到 30s | 新增 **`pretest_120_submit_only`**（仅 submit 24/s）隔离；**`pretest_120_balanced`**（32/24/24/12）降读压 |
| `pretest_120_submit_only` | submit **99.05%**，429 仅 68 | 证实混合读压放大 submit 失败；纯 24/s 仍略超队列容量 | collection `submit_queue`：**worker 24→32**，**queue 1200→1600**；混合场景先降读压再过线 |
| `pretest_120_balanced` | **通过** 0×429 | — | 降 questionnaire 48→32 后混合可过线 |
| `pretest_120`（worker 调优后） | **通过** submit p95≈192ms | — | outbox ≤3min 消化 |
| `mixed_140`（submit **28**/s） | **未通过** 806×429，submit 90%～98% | submit 超单机混合稳态吞吐；worker32 时 90.4% 失败 | submit 压测值降至 **24/s**；`submit_queue` worker 调至 **40**；新增 **`mixed_140_submit24`** 验证 |
| `mixed_140_submit24` | **通过** submit p95≈222ms | — | 确认瓶颈在 submit 配比，非读压 |

**infra 建议（未改代码）**：Mongo 容器内存 3GiB→**4–6GiB**（WiredTiger cache 84% + 高 BLOCK I/O 时连接占用变长）。

---

### 3.4 轮次三：submit 重标定与问卷 L1（2026-07-01）

**背景**：submit 按真实流量重标定，`mixed_140+` **封顶 24/s**；部署 collection **`questionnaire_cache`** L1（TTL 180s）。

| 档位 | 结论 | 遇到的问题 | 解决方案 |
| ---- | ---- | ---------- | -------- |
| `mixed_140`～`mixed_200` | **通过** | `mixed_160` submit p95 偶发 ~781ms 尖刺 | 可接受；继续升档 |
| `mixed_220`（102/s 读，**无 L1**） | **未通过** http 0.56%～5.61%，query p95 8.8s～30s | 102/s 读压超 collection→apiserver **gRPC 路径**容量；每请求打 apiserver 无本地 REST 缓存 | 部署 **questionnaire L1**；**勿靠加 VU**（VU690 仍失败） |
| `mixed_220`（L1 后） | **通过** query p95≈172ms | — | max VU≈248；gRPC 压力骤降 |
| `mixed_240`（112/s，VU 730） | **边际** 122×query timeout | 读压偏高 + VU 不足边缘 | 可作为参考，不作为升档依据 |
| `mixed_240`（112/s，VU **1200**） | **未通过** http 4.64%，4411×timeout | **堆 VU 加剧下游过载**，非解法 | 下游饱和时 **禁止盲目加 VU**；应降 QPS 或加缓存/扩容 |
| `mixed_240`（legacy 100/s 单桶） | **通过** query p95≈58–66ms | 仅 `questionnaire_query`，**未验量表/人格 L1** | 三域验收须换 **`mixed_240_models`**（54/27/19/s） |
| `mixed_200`（L1 后复测） | **通过** query p95≈322ms | — | — |

---

### 3.5 轮次四：三域 Catalog L1 与 280 档（2026-07-01）

**背景**：`questionnaire_cache` + **`scale_cache`** + **`personality_cache`**（TTL 180s）；压测须 **拆分 query 场景**。

| 档位 | 结论 | 遇到的问题 | 解决方案 |
| ---- | ---- | ---------- | -------- |
| `mixed_240_models` | **通过** http p95≈154ms | — | 三域 54/27/19/s 全绿；max VU≈749 |
| `mixed_280`（legacy **132/s 单桶**） | HTTP 0 失败但 **读 p95≈1.36s 未过线** | 132/s 全打 `questionnaire_query`，单场景读压探顶 | **280 档必须用拆分 profile**；legacy 单桶不能代表容量 |
| `mixed_280_models`（71/36/25/s） | **通过** http p95≈114ms | `mixed_280_models` **连跑**可在 ~20–68s 雪崩 | 档间冷却 **≥10min**；残余负载 + k6 VU 螺旋，勿否定已通过结论 |
| Report 模式 | — | 生产已切 WebSocket + report-status；压测仍走长轮询会 **误打 wait-report**、VU 按长连接 sizing | 常规 profile 默认 **`short_poll`**；全局 `paths.reportStatus` 改 `/report-status`；长轮询仅 **`special_report_long_poll`** 专项 |

---

### 3.6 轮次五：300 档攻关（2026-07-01，历史）

| 档位/步骤 | 结论 | 遇到的问题 | 解决方案 |
| --------- | ---- | ---------- | -------- |
| `mixed_300` 全量（**长轮询** report 100/s） | **未通过** | ~101s report **VU 900 触顶**；`chain_probe_failed`=315 | 改 **`short_poll`**（max VU≈380）；分步攻关 |
| `mixed_300_http` 初版（**146/s** query） | **中断** ~68s 雪崩 | 一次拉满读压 + 10min | 拆 **Step1**（71/36/25）+ **Step2**（80/40/13/13） |
| Step1 | **通过** | — | 同 280 读压；max VU≈390 |
| Step2（线 B 前） | **边际通过** | 109×`statistics/system` timeout | **线 A** 隔离 + **线 B** B1–B4 |

---

### 3.7 轮次六：300 全量验收（2026-07，以 280 为基准）

**前置条件**（与 `mixed_280_models` 对齐）：三域 L1 · `short_poll` · report max VU **380**（非 900）· apiserver 线 B 已部署 · 档间冷却 ≥10min。

**相对 280 的增量验证**：

| 对比项 | mixed_280_models | mixed_300 | 结论 |
| ------ | ---------------- | --------- | ---- |
| 读压 | 132/s | 146/s（+14） | Step2 已验证 collection 可扛 |
| report | 96/s short_poll | 100/s short_poll | +4/s，VU 380≈100×3.5×1.1 |
| stats | 28/s | 29/s | 线 B 后 0×30s timeout |
| probe | — | 1/s | short_poll 下 VU 未触顶，probe 不再雪崩 |
| submit | 24/s | 24/s | 不变 |

| 档位 | 结论 | 关键指标 | 说明 |
| ---- | ---- | -------- | ---- |
| `mixed_300_http_query`（线 B 后复测） | **通过** | checks **100%**；stats/system **0×timeout**；三域 query p95 **380/310/420ms** | 相对 Step2（592/467/796ms）stats 干扰消除 |
| **`mixed_300` 全量** | **通过** | checks **100%**；http **0%**；submit/report **100%**；`chain_probe_failed` **0**；http p95≈**128ms**；max VU≈**430** | 在 280 同构 profile 上 +4 report +1 probe；outbox ≤3min |

**相对 280 延迟对比**（同机、同 L1、short_poll）：

| 指标 | mixed_280_models | mixed_300 | 增幅 |
| ---- | ---------------- | --------- | ---- |
| http p95 | 114ms | 128ms | +12% |
| 医学 query p95 | 112ms | 380ms | +14/s 读压主增量 |
| 人格 query p95 | 106ms | 310ms | |
| 问卷 query p95 | 127ms | 420ms | |
| report max VU | 320（96/s） | 380（100/s） | 未触顶 |

**结论**：300 档瓶颈已在轮次五消除（长轮询 VU、stats 击穿）；全量在 280 基准 + 有限增量下 **单机单实例可承诺 ~300 QPS 混合**（submit 24/s）。

---

### 3.8 轮次七：serverA 缩容复测（2026-07-02，4C/8G）

**背景**：serverA **8C/16G → 4C/8G**。L1 + `short_poll` + 线 B 已部署。同日连跑：`mixed_200` → `mixed_220` → `mixed_240`（legacy）→ **`mixed_240_models`**（档间约 8～15min）。

| 档位 | 时长 | 结论 | 关键指标（p95） | 对比 8C/16G 历史 |
| ---- | ---- | ---- | --------------- | ---------------- |
| **`mixed_200`**（92/24/64/20） | 5m | **通过** | http **97ms**；query **68ms** | 优于 query≈322ms |
| **`mixed_220`**（102/24/72/22） | 5m | **通过** | http **79ms**；query **66ms** | 优于 query≈172ms |
| **`mixed_240`**（100/24/88/28，legacy） | 8m | **通过** | http **85ms**；query **72ms** | 单桶问卷 |
| **`mixed_240_models`**（54/27/19 +24/88/28） | 8m | **通过** | http **100ms**；三域 **75/118/86ms** | 8C/16G http≈154ms |
| **`mixed_280_models`**（71/36/25 +24/96/28） | 8m | **边际未过** | k6 阈值全绿；checks **99.84%**、failed **0.15%**（212）；http p95 **571ms**；三域 p95 **921/413/1.72s** | 8C/16G http≈**114ms** |

**`mixed_280_models` 问题与解法**：

| 现象 | 根因 | 处理 |
| ---- | ---- | ---- |
| ~**428s**（第 7min）起全场景 30s timeout/EOF 雪崩 | 连跑 200→220→240→240_models 后仅 **~15min** 即打 280（+40/s 读、+8 report）；**4C/8G 残余负载 + 容量触顶** | 档间冷却 **≥30min** 后**单独**重跑；勿同日连升 |
| 182 医学 / 67 问卷 / 21 人格 / 27 stats timeout | 下游排队至 30s；非 L1 miss（med p50 仍 ~35ms） | 与上同；确认非 stats 击穿（线 B 已部署） |
| setup 显示 report `maxVUs` **560–860** | 本地 `qs-perf.config.json` 仍为长轮询时代 VU；`perf-sync-profiles` 不覆盖 | `jq` 改 `vusers.report` max≈**320**、`reportMode=short_poll` |
| k6 阈值过、SOP 读 p95 未过 | 尾段拉高超 500ms 线 | 不以本次作 4C/8G 280 承诺；通过后再升 `mixed_300` |

**4C/8G 结论（缩容首日）**：可承诺 **≤240 三域混合**；**280 单机边际不足**（连跑 + 冷却不足），需冷却重测 / 修正 report VU。

---

### 3.8.1 轮次七续：outbox 排水调优后复测（2026-07-02，4C/8G）

**背景**（同日部署）：`outbox_relay.assessment` interval **500ms**、batch **200**、workers **24**、`immediate_max_concurrent` **16**；`assessment.submitted` 加入 immediate 旁路；ReadyIndex 按 store 隔离（`mongo-domain-events` / `assessment-mysql-outbox`）；score 编码 `created_at` 实现同 due 时刻 FIFO。重启 apiserver 后 reconciler ~30s 回补旧 pending。

**跑次链**：`pretest_120` → `mixed_200` → `mixed_240_models`（档间 1～10min）→ 冷却 **34min** → `mixed_280_models`。

| 档位 | 时长 | 结论 | 关键指标（p95） | 备注 |
| ---- | ---- | ---- | --------------- | ---- |
| **`pretest_120`** | 5m | **通过** | http **101ms**；query **160ms**；submit **66ms** | 0% failed |
| **`mixed_200`** | 5m | **通过** | http **90ms**；query **88ms**；submit **81ms** | 0% failed；较调优前 13×timeout **已恢复** |
| **`mixed_240_models`** | 8m | **通过** | http **76ms**；三域 **66/76/73ms** | 0% failed；L1 + 三域拆分生效 |
| **`mixed_280_models`** | 8m | **边际** | http **98ms**；checks **99.98%**；failed **0.01%**（20） | ~**41s** 突发 20×30s timeout；`dropped_iterations=465` |

**`mixed_280_models` 与 §3.8 对比**：

| 维度 | §3.8（连跑 200→280） | §3.8.1（outbox 调优 + 34min 冷却） |
| ---- | -------------------- | ---------------------------------- |
| failed | 0.15%（212） | **0.01%**（20） |
| 超时形态 | ~428s 起全场景雪崩 | ~41s **尖刺**，稳态 p95 <100ms |
| http p95 | 571ms | **98ms** |

**4C/8G 结论（调优后）**：可承诺 **≤240 三域混合**；280/300 须应用 §2.4 榨干档 + 单独重跑验收。

### 3.8.2 轮次七续：4C/8G 榨干档配置（2026-07-02）

**变更**（`configs/*.prod.yaml` + `qs-perf.config.example.json`）：mongo inflight 80→**120**；collection HTTP/grpc 并发 400/360→**480/420**；背压 wait 2s→**4～5s**；k6 VU max 收紧（submit 1180→**400**）。

**部署**：重启 apiserver + collection；`make perf-sync-vusers`；冷却 ≥30min 后单独跑 `mixed_280_models`。

---

### 3.9 明细表（归档）

| 档位 | QPS | failed | 5xx | 401 | 429 | P95 | 结论 |
| ---- | ---: | ---: | ---: | ---: | ---: | ---: | ---- |
| pretest_60 | 60 | 0% | 0 | 0 | 0 | submit≈184ms | 通过 |
| pretest_120 | 120 | 0% | 0 | 0 | 0 | submit≈192ms | 通过 |
| pretest_120_balanced | 92 | 0% | 0 | 0 | 0 | submit≈250ms | 通过 |
| mixed_140（submit28） | 140 | 0.44%～1.92% | 0 | 0 | 806 | submit≈1.11s | 未通过 |
| mixed_140_submit24 | 136 | 0% | 0 | 0 | 0 | submit≈222ms | 通过 |
| mixed_140 | 140 | 0% | 0 | 0 | 0 | submit≈270ms | 通过 |
| mixed_160 | 160 | 0% | 0 | 0 | 0 | submit≈781ms | 通过 |
| mixed_180 | 180 | 0% | 0 | 0 | 0 | submit≈202ms | 通过 |
| mixed_200（8C/16G，L1 后） | 200 | 0% | 0 | 0 | 0 | query≈322ms | 通过 |
| **mixed_200（4C/8G，缩容首日）** | 200 | 0% | 0 | 0 | 0 | http≈97ms | 通过（2026-07-02） |
| **mixed_200（4C/8G，outbox 调优后）** | 200 | **0%** | **0** | **0** | **0** | http≈**90ms** | **通过**（2026-07-02） |
| mixed_220（无 L1） | 220 | 5.61% | 0 | 0 | 0 | query≈30s | 未通过 |
| mixed_220（8C/16G，L1 后） | 220 | 0% | 0 | 0 | 0 | query≈172ms | 通过 |
| **mixed_220（4C/8G）** | 220 | **0%** | **0** | **0** | **0** | http≈**79ms** | **通过**（2026-07-02） |
| **mixed_240（4C/8G，legacy）** | 240 | **0%** | **0** | **0** | **0** | http≈**85ms**；8min | **通过**（2026-07-02） |
| mixed_240_models（8C/16G） | 240 | 0% | 0 | 0 | 0 | http≈154ms | 通过（三域） |
| **mixed_240_models（4C/8G，缩容首日）** | 240 | **0%** | **0** | **0** | **0** | http≈**100ms**；三域 75/118/86ms | **通过**（2026-07-02） |
| **mixed_240_models（4C/8G，outbox 调优后）** | 240 | **0%** | **0** | **0** | **0** | http≈**76ms**；三域 66/76/73ms | **通过**（2026-07-02） |
| mixed_280_models（8C/16G） | 280 | 0% | 0 | 0 | 0 | http≈114ms | 通过 |
| **mixed_280_models（4C/8G，缩容首日连跑）** | 280 | **0.15%** | 0 | 0 | 0 | http≈571ms；7min 雪崩 | **边际未过**（2026-07-02） |
| **mixed_280_models（4C/8G，outbox 调优后）** | 280 | **0.01%** | 0 | 0 | 0 | http≈**98ms**；~41s 尖刺 | **边际**（2026-07-02） |
| mixed_300（长轮询，历史） | ~300 | 0.02% | — | 0 | 0 | http≈10.6s；probe=315 | 未通过 |
| mixed_300_http Step1 | ~281 | 0% | 0 | 0 | 0 | query 284/256/359ms | 通过 |
| mixed_300_http_query Step2（B 前） | ~295 | 0.06% | 0 | 0 | 0 | stats timeout×109 | 边际通过 |
| mixed_300_http_query Step2（B 后） | ~295 | 0% | 0 | 0 | 0 | query 380/310/420ms | 通过 |
| **mixed_300** | ~300 | **0%** | **0** | **0** | **0** | http≈128ms；probe=0 | **通过** |

---

### 3.10 新跑次模板

| 日期 | Profile | 结论 | 遇到的问题 | 解决方案 | failed | 场景 p95 | outbox 3min |
| ---- | ------- | ---- | ---------- | -------- | ---: | -------- | ----------- |
| 2026-07-02 | mixed_200 | 通过（4C/8G） | — | 缩容后复测 | 0% | http 97ms | — |
| 2026-07-02 | mixed_220 | 通过（4C/8G） | — | 缩容后复测 | 0% | http 79ms | — |
| 2026-07-02 | mixed_240 | 通过（4C/8G，legacy） | — | 缩容复测 | 0% | http 85ms | — |
| 2026-07-02 | mixed_240_models | 通过（4C/8G） | — | 缩容复测 | 0% | http 100ms | — |
| 2026-07-02 | mixed_280_models | 边际未过（4C/8G，缩容首日） | ~428s 超时雪崩；连跑间隔短 | 冷却≥30min 重跑；修正 report VU≈320 | 0.15% | http 571ms | — |
| 2026-07-02 | pretest_120 | 通过（4C/8G，outbox 调优后） | — | assessment relay + ReadyIndex 隔离 | 0% | http 101ms | — |
| 2026-07-02 | mixed_200 | 通过（4C/8G，outbox 调优后） | 调优前曾有 13×timeout | 同上 | 0% | http 90ms | — |
| 2026-07-02 | mixed_240_models | 通过（4C/8G，outbox 调优后） | — | 三域 L1 验收 | 0% | http 76ms；三域 66/76/73ms | — |
| 2026-07-02 | mixed_280_models | 边际（4C/8G，outbox 调优后） | ~41s 20×timeout；dropped=465 | 单独重跑；勿连档升 300 | 0.01% | http 98ms | — |

**相关**：[10-QPS容量档位与资源配置建议.md](./10-QPS容量档位与资源配置建议.md) · [12-小程序报告等待接入指南.md](./12-小程序报告等待接入指南.md)
