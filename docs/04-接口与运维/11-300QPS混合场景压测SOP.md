# 300 QPS 混合场景压测 SOP

---

## 本文回答

这份 SOP 回答四个问题：

1. `qs-server` 混合压测应该按什么顺序升档。
2. 每个 profile 的 QPS、report mode、时长和 Make 入口是什么。
3. 4C/8G 与 8C/16G 当前分别能承诺到什么水位。
4. 失败时先看哪些指标，如何区分 token、Nginx、capacity、outbox、report mode 和 VU sizing 问题。

本文只覆盖 `scripts/perf/k6/mixed.js` 这条混合压测链路。数据库 schema、NSQ/worker 消费模型、真实生产扩容方案不在本文内展开。

---

## 30 秒结论


| 维度                                | 当前结论                                                                                          |
| --------------------------------- | --------------------------------------------------------------------------------------------- |
| 标准入口                              | `make perf-init` -> `make perf-tokens` -> `make perf-preflight` -> `make perf-smoke` -> 按档位升压 |
| 脚本事实源                             | `scripts/perf/k6/mixed.js` + `scripts/perf/qs-perf.config.example.json`                       |
| 本地配置                              | `tmp/perf/qs-perf.config.json`；`make perf-init` 不覆盖已有文件                                       |
| Report 主路径                        | 默认 `websocket`，走 `/api/v1/report-events`；HTTP 短轮询只用于 `special_report_short_poll` 专项           |
| 4C/8G**：`mixed_280_models` **边际通过 | 约 280/s 三域混合可作为边际水位；曾见 0.20% failed，主要是 catalog Try 503                                       |
| `mixed_300_http_query` **通过       | 约 295/s 读 + WS，无 `chain_probe`，可作为 4C/8G Step2 验收                                             |
| `mixed_300` 全量 **未过               | 4C/8G 下全量 300 + `chain_probe` 可复现 8.75%～10.60% failed                                         |
| 8C/16G 口径                         | `mixed_300` **8C/16G 全量已通过；4C/8G 未承诺**                                                        |
| VU 策略                             | 按 `rps * p95 * 1.05` 控制 max；宁可 dropped iterations，也不要 VU 螺旋                                   |
| outbox 验收                         | 压测结束后 3 分钟内 pending / publishing / failed 回落到近 0；否则单独诊断 outbox 排水                             |


推荐升档路径：

```text
smoke_4
  -> pretest_60
  -> pretest_120
  -> mixed_140 -> mixed_160 -> mixed_180 -> mixed_200 -> mixed_220
  -> mixed_240_models
  -> mixed_280_models
  -> mixed_300_http -> mixed_300_http_query
  -> mixed_300
```

执行原则：

- 上一档全绿再升档。
- `mixed_280_models` 及以上建议冷却至少 30 分钟后单独跑。
- `mixed_300` 前必须执行 `make perf-sync-vusers`，避免旧长轮询 VU 配置造成无效跑次。
- 4C/8G 验收口径按“280 边际 + Step2 通过，全量 300 未承诺”执行。

---



## 一、压测目标与通过标准



### 1.1 本次混合压测看三类能力


| 能力   | 覆盖场景                                                             | 主要指标                                                  |
| ---- | ---------------------------------------------------------------- | ----------------------------------------------------- |
| 前台读写 | catalog/query、answersheet submit、report status/events、statistics | `http_req_failed`、场景 p95、submit 202、report success    |
| 异步链路 | `chain_probe` 从提交到报告可用                                           | `chain_probe_failed`、submit-to-report SLA             |
| 后台排水 | Mongo/MySQL outbox、NSQ、worker、report 生成                          | outbox pending/publishing/failed、oldest age、worker 消费 |




### 1.2 验收线


| 类别          | 通过条件                                                    |
| ----------- | ------------------------------------------------------- |
| HTTP 全局     | `http_req_failed < 1%`                                  |
| checks      | `checks > 99%`                                          |
| submit      | `answer_submit_success_rate > 99%`，状态码 202              |
| report      | `report_status_success_rate > 99%`；WS 101 连接成功也计入该 rate |
| chain probe | `chain_probe_failed < 3`，高档目标为 0                        |
| query 延迟    | 攻关档 query p95 目标 < 500ms；边际档可结合 failed 与尖刺判断            |
| outbox      | 压测结束 3 分钟内 pending/publishing/failed 回落到近 0             |


注意：

- HTTP 429 是限流或提交保护信号，不等价于 outbox 堆积。
- catalog 503 是 query semaphore Try reject，不是 rate limit 429。
- k6 5xx 但应用日志无 5xx 时，优先查 Nginx。
- report 自动发现会按模型身份拆分 `medical`、`personality`、`behavior`；behavior 报告走 `/behavior-assessments`，WebSocket 订阅使用 `kind=behavior`。
- `qps.stats` 在机构总览 `GET /statistics/overview` 与内容批量统计 `POST /statistics/contents/batch` 之间分配；后者按 `questionnaire/scale + code` 构造请求。

---



## 二、运行前准备



### 2.1 基础命令

```bash
make perf-init
make perf-tokens
make perf-preflight
make perf-smoke
```

`perf-preflight` 必须满足：

- token count 足够，`expired=0`。
- `min_ttl_seconds` 大于本轮压测时长。
- collection catalog、personality、questionnaire，以及 apiserver testees/statistics preflight 均为 200。
- `report_events.enabled=true`。



### 2.2 同步本地 profile

```bash
make perf-sync-profiles
make perf-sync-vusers
make perf-verify
```

`perf-sync-profiles` 会保留本地 token/URL，并移除已经退休的 `/statistics/system`、`/statistics/questionnaires/:code` 配置。

使用规则：

- `perf-sync-profiles` 只补缺失 profile，不覆盖已有 qps/vusers/reportMode。
- `perf-sync-vusers` 会覆盖 vusers + reportMode；WebSocket report 切换后、跑 `mixed_300` 前必须执行。
- `perf-verify` 会用 `k6 inspect` 检查关键 profile 和 scenario。



### 2.3 环境清单


| 检查项        | 要求                                                    |
| ---------- | ----------------------------------------------------- |
| 压测入口       | 公网打到 `collect.fangcunmount.cn` / `qs.fangcunmount.cn` |
| Nginx      | 压测 IP 已放宽 `limit_conn`；配置后重启 Nginx，不只 reload          |
| collection | 与 Nginx 同机部署时优先确认 CPU、连接数、goroutine、队列                |
| apiserver  | 确认 outbox relay 配置已部署，Mongo/MySQL 连接池无异常耗尽            |
| worker/NSQ | worker 运行中；NSQ topic/channel 无异常堆积                    |
| 本地 config  | `tmp/perf/qs-perf.config.json` 与 example 结构一致         |


---



## 三、Profile 速查

配置事实源：`scripts/perf/qs-perf.config.example.json`。


| Profile                        | 时长  | Report mode | QPS 配比                                                                                                                      | Make                                    |
| ------------------------------ | --- | ----------- | --------------------------------------------------------------------------------------------------------------------------- | --------------------------------------- |
| `smoke_4`                      | 30s | 默认          | query 1 / submit 1 / report 1 / stats 1                                                                                     | `make perf-smoke`                       |
| `pretest_60`                   | 3m  | 默认          | query 25 / submit 10 / report 19 / stats 6                                                                                  | `make perf-pretest60`                   |
| `pretest_120`                  | 5m  | 默认          | query 51 / submit 19 / report 38 / stats 12                                                                                 | `make perf-pretest120`                  |
| `mixed_140`                    | 5m  | 默认          | query 58 / submit 24 / report 44 / stats 14                                                                                 | `make perf-mixed140`                    |
| `mixed_160`                    | 5m  | 默认          | query 68 / submit 24 / report 52 / stats 16                                                                                 | `make perf-mixed160`                    |
| `mixed_180`                    | 5m  | 默认          | query 80 / submit 24 / report 58 / stats 18                                                                                 | `make perf-mixed180`                    |
| `mixed_200`                    | 5m  | 默认          | query 92 / submit 24 / report 64 / stats 20                                                                                 | `make perf-mixed200`                    |
| `mixed_220`                    | 5m  | 默认          | query 102 / submit 24 / report 72 / stats 22                                                                                | `make perf-mixed220`                    |
| `mixed_240_models`             | 8m  | websocket   | medical 54 / personality 27 / questionnaire 19 / submit 24 / report 88 / stats 28                                           | `make perf-mixed240-models`             |
| `mixed_280_models`             | 8m  | websocket   | medical 71 / personality 36 / questionnaire 25 / submit 24 / report 96 / stats 28                                           | `make perf-mixed280-models`             |
| `mixed_300_http`               | 10m | websocket   | 同 280 query / submit 24 / report 96 / stats 29 / no probe                                                                   | `make perf-mixed300-http`               |
| `mixed_300_http_query`         | 10m | websocket   | medical 80 / personality 40 / questionnaire 13 / personality questionnaire 13 / submit 24 / report 96 / stats 29 / no probe | `make perf-mixed300-http-query`         |
| `mixed_300`                    | 10m | websocket   | medical 80 / personality 40 / questionnaire 13 / personality questionnaire 13 / submit 24 / report 100 / stats 29 / probe 1 | `make perf-mixed300`                    |
| `mixed_300_http_query_nostats` | 10m | websocket   | Step2 去 stats 对照                                                                                                            | `make perf-mixed300-http-query-nostats` |
| `stats_isolate_29`             | 10m | 默认          | stats 29 only                                                                                                               | `make perf-stats-isolate29`             |
| `outbox_120`                   | 10m | websocket   | submit 96 / report 96 / stats 10 / probe 1 / 少量 query                                                                       | `make perf-outbox120`                   |
| `personality_60`               | 5m  | websocket   | personality query/session/submit/wait-report 专项                                                                             | `make perf-personality60`               |


专项不进入常规升档链：


| 专项                          | 用途                      | Make                                  |
| --------------------------- | ----------------------- | ------------------------------------- |
| `special_report_short_poll` | HTTP report-status 降级路径 | `make perf-special-report-short-poll` |
| `special_report_long_poll`  | wait-report 长轮询兼容验证     | `make perf-special-report-long-poll`  |
| `outbox_120`                | outbox 排水专项             | `make perf-outbox120`                 |
| `personality_60`            | 人格链路专项                  | `make perf-personality60`             |


---



## 四、标准执行流程



### 4.1 L0：连通性

```bash
make perf-smoke
```

通过后再继续。失败时先修 token、base URL、preflight、Nginx 和 IAM。

### 4.2 L1：预压

```bash
make perf-pretest60
make perf-pretest120
```

如果 `pretest_120` 失败：

- submit 429：跑 `make perf-pretest120-submit-only`。
- query timeout：跑 `make perf-diag-query120`。
- report 异常：跑 `make perf-diag-report120` 或 `make perf-special-report-short-poll`。



### 4.3 L2：140～220 升档

```bash
make perf-mixed140
make perf-mixed160
make perf-mixed180
make perf-mixed200
make perf-mixed220
```

该区间 submit 已封顶 24/s。若 submit 429 明显，先不要继续升档。

### 4.4 L3：240～280 三域验收

```bash
make perf-mixed240-models
make perf-mixed280-models
```

要求：

- 使用三域 query profile，不再用 legacy 单桶问卷压测作为升档依据。
- report mode 为 `websocket`。
- `mixed_280_models` 及以上建议冷却至少 30 分钟后单独跑。



### 4.5 L4：300 攻关

推荐顺序：

```bash
make perf-sync-vusers
make perf-mixed300-http
make perf-mixed300-http-query
make perf-stats-isolate29
make perf-mixed300-http-query-nostats
make perf-mixed300
```

判断方式：


| 步骤                             | 目的                                   | 通过后说明                     |
| ------------------------------ | ------------------------------------ | ------------------------- |
| `mixed_300_http`               | 用 280 query + 10m 验证 report/stats 基线 | 280 基准稳定                  |
| `mixed_300_http_query`         | 拉满 146/s 读 + 96/s WS，无 probe         | 4C/8G Step2 可过            |
| `stats_isolate_29`             | 隔离 statistics 29/s                   | 排除 stats 击穿               |
| `mixed_300_http_query_nostats` | Step2 去 stats 对照                     | 判断 stats 对 Step2 的影响      |
| `mixed_300`                    | 全量 300 + probe                       | 只有 8C/16G 历史可承诺；4C/8G 未承诺 |


---



## 五、观测与结果归档



### 5.1 k6 结果优先级

先看这些指标：

```text
http_req_failed
checks
answer_submit_success_rate
report_status_success_rate
chain_probe_failed
http_401_total / http_403_total / http_429_total / http_5xx_total
各场景 p95 / max / dropped_iterations / vus_max
```

推荐判读顺序：

1. `http_401_total`：token 过期或无效。
2. `http_403_total`：鉴权/权限/租户问题。
3. `http_429_total`：rate limit、submit queue、保护策略。
4. `http_5xx_total`：Nginx 或应用异常。
5. timeout / EOF：排队、VU 螺旋、上游连接或下游耗尽。
6. `chain_probe_failed`：异步 SLA 不达标，单独查 outbox/worker/report 生成。



### 5.2 snapshot

高档位建议压测前后都抓 snapshot：

```bash
OUT_DIR=tmp/perf/300qps ./scripts/perf/snapshot-observability.sh before
# run k6
OUT_DIR=tmp/perf/300qps ./scripts/perf/snapshot-observability.sh after
```



### 5.3 Mongo outbox 聚合

```javascript
db.domain_event_outbox.aggregate([
  {$match:{status:{$in:["pending","publishing","failed"]}}},
  {$group:{
    _id:{status:"$status", event_type:"$event_type"},
    n:{$sum:1},
    oldest:{$min:"$created_at"},
    oldest_update:{$min:"$updated_at"}
  }},
  {$sort:{"_id.status":1,n:-1}}
])
```

判读：

- `pending` 持续上涨：relay claim 不够快或 ready index/DB 查询成为瓶颈。
- `publishing` 长时间不降：publish 或 mark published 慢。
- `failed` 增长：查 publish/mark 失败日志。
- 压测后 3～5 分钟才清空：主链路可用，但后台排水还有余量风险。



### 5.4 Nginx 与应用日志

Nginx 判断：

- `503` 且 request time `0.000`：通常是 Nginx `limit_conn`。
- `499`：客户端超时主动断开，通常是 k6 timeout 后断连。
- 某一类 path/code 集中 5xx：先看脚本数据池或接口契约，不直接归因容量。

应用判断：

- collection/apiserver/worker CPU 都低但 outbox 堆积：优先查 outbox relay、DB 查询、mark published、ready index。
- NSQ/worker 都低但 Mongo outbox 高：通常瓶颈在 outbox claim/publish/mark，而不是 worker 消费。

---



## 六、Report Mode 规则


| Mode         | 使用场景                    | 路径                      | 是否进入升档 |
| ------------ | ----------------------- | ----------------------- | ------ |
| `websocket`  | 生产主路径、常规压测              | `/api/v1/report-events` | 是      |
| `short_poll` | HTTP report-status 降级专项 | `/report-status`        | 否      |
| `long_poll`  | wait-report 兼容专项        | `/wait-report`          | 否      |


注意：

- `qps.report` 表示用户侧查报告频率，不是裸 HTTP RPS。
- WebSocket 101 成功会写入 `report_status_success_rate=true`。
- missing sample、非 101、decode/error 会写失败样本或 failed counter。
- 下游饱和时不要通过提高 report max VU 硬冲，否则会放大排队和 503。

---



## 七、常见故障处理


| 现象                                          | 优先判断                       | 处理                                                                       |
| ------------------------------------------- | -------------------------- | ------------------------------------------------------------------------ |
| 约 7% failed，应用日志全 200                       | token 过期或无效                | `make perf-tokens && make perf-preflight`，确认 `expired=0`                 |
| k6 503，应用无 5xx，Nginx request time 0.000     | Nginx `limit_conn`         | 放宽压测 IP，重启 Nginx                                                         |
| report WS 失败 / EOF                          | report-events 未开启或代理异常     | 查 `report_events.enabled=true`、Nginx WS 代理、`paths.reportEvents`          |
| `mixed_300` failed 约 45%，`vus_max` 接近 2000  | 未同步旧 VU 配置                 | 作废跑次，执行 `make perf-sync-vusers` 后重跑                                      |
| catalog 503，非 429                           | query semaphore Try reject | 查 `max-query-concurrency`、query p95、dropped iterations；不要按 rate limit 处理 |
| submit 429                                  | submit 队列/保护策略             | 降 submit 到 24/s，跑 submit-only 诊断                                         |
| query 30s timeout                           | 下游排队或 VU 螺旋                | 查 L1 命中、apiserver gRPC、Mongo/MySQL、VU max                                |
| stats timeout                               | statistics 读模型或缓存击穿        | 跑 `stats_isolate_29` 和 `mixed_300_http_query_nostats`                    |
| outbox oldest age > 3min                    | 后台排水不足                     | 跑 `perf-outbox120`，查 relay publish/mark 日志                               |
| personality code 导致 questionnaire detail 失败 | 脚本混用不同契约                   | 通用 questionnaire 查询只放医学问卷；人格详情必须带 session version                        |
| chain probe 失败但 HTTP 主路径正常                  | 异步 SLA 不达标                 | 查 assessment/report outbox、worker、report status 生成链                      |


---



## 八、当前容量结论



### 8.1 4C/8G


| 档位                     | 结论   | 证据摘要                                                           |
| ---------------------- | ---- | -------------------------------------------------------------- |
| `pretest_120`          | 通过   | 0% failed                                                      |
| `mixed_200`            | 通过   | 0% failed，http p95 约 90ms                                      |
| `mixed_240_models`     | 通过   | 0% failed，三域 query p95 约 66/76/73ms                            |
| `mixed_280_models`     | 边际通过 | 0.20% failed，catalog 503 为主                                    |
| `mixed_300_http_query` | 通过   | 0.01% failed；146/s 读 + 96/s WS，无 probe                         |
| `mixed_300`            | 未通过  | 8.75%～10.60% failed；catalog 503 + `chain_probe_failed` 128～137 |


结论：

- 4C/8G 可作为 `mixed_240_models` 稳态水位。
- 4C/8G 可按 `mixed_280_models` 边际水位谨慎验收。
- 4C/8G 可以用 `mixed_300_http_query` 证明读 + WS Step2 能过。
- 4C/8G 不承诺全量 `mixed_300`，除非进一步扩容或降低 `chain_probe`/query 压力。



### 8.2 8C/16G

历史结论：

- `mixed_280_models` 通过。
- `mixed_300` 全量通过，HTTP 0% failed，submit/report 100%，`chain_probe_failed=0`。

8C/16G 结果可以作为全量 300 的历史能力证据；新版本上线后仍需按本文升档重新验收。

---



## 九、历史压测记录摘要


| 阶段                 | 主要问题                                          | 处理结果                                                        |
| ------------------ | --------------------------------------------- | ----------------------------------------------------------- |
| token/preflight 初期 | token 过期导致约 7% failed                         | 增加 `perf-tokens` + `perf-preflight` 前置                      |
| 跨机部署初期             | collection 在 serverB 时出现 502 / upstream close | collection 迁回 serverA，Nginx keepalive 调整                    |
| Nginx 限连接          | k6 503 但应用无 5xx                               | 压测 IP 白名单，重启 Nginx                                          |
| Mongo/outbox 初期    | relay worker 过高、immediate 无上限、Mongo 连接池耗尽     | 限制 publish workers / immediate 并发，增加 cached published model |
| submit 标定          | submit 28/s 导致大量 429                          | 混合档 submit 固定为 24/s                                         |
| catalog L1 前       | 220 以上读压 timeout                              | 增加 questionnaire/scale/personality L1                       |
| legacy 单桶 query    | 不能代表三域 catalog 容量                             | 改用 `mixed_240_models` / `mixed_280_models`                  |
| 长轮询 report         | VU 触顶、probe 雪崩                                | 常规切 WebSocket；长轮询只保留专项                                      |
| 4C/8G 缩容后          | 280 边际，300 全量不过                               | 当前验收口径改为 280 边际 + Step2 通过                                  |


详细历史指标归档：


| 档位                      | 环境           | 结论   | 关键指标                     |
| ----------------------- | ------------ | ---- | ------------------------ |
| `pretest_60`            | 8C/16G       | 通过   | 0% failed                |
| `pretest_120`           | 8C/16G 调优后   | 通过   | submit p95 约 192ms       |
| `mixed_140` submit 28/s | 8C/16G       | 未通过  | 806×429                  |
| `mixed_140_submit24`    | 8C/16G       | 通过   | submit p95 约 222ms       |
| `mixed_220` 无 L1        | 8C/16G       | 未通过  | query p95 约 30s          |
| `mixed_220` L1 后        | 8C/16G       | 通过   | query p95 约 172ms        |
| `mixed_240_models`      | 8C/16G       | 通过   | 三域全绿                     |
| `mixed_280_models`      | 8C/16G       | 通过   | http p95 约 114ms         |
| `mixed_300_http_query`  | 8C/16G 线 B 后 | 通过   | checks 100%              |
| `mixed_300`             | 8C/16G       | 通过   | http 0%，probe 0          |
| `mixed_240_models`      | 4C/8G        | 通过   | 0% failed                |
| `mixed_280_models`      | 4C/8G        | 边际通过 | 0.20% failed，catalog 503 |
| `mixed_300_http_query`  | 4C/8G        | 通过   | 0.01% failed             |
| `mixed_300`             | 4C/8G        | 未过   | 8.75%～10.60% failed      |


---



## 十、新跑次记录模板

压测后按下面格式追加，不要只贴 k6 summary。


| 日期         | Profile          | 环境    | 结论       | failed | 主要 p95            | outbox 3min | 主要问题 | 处理  |
| ---------- | ---------------- | ----- | -------- | ------ | ----------------- | ----------- | ---- | --- |
| YYYY-MM-DD | mixed_280_models | 4C/8G | 通过/边际/未过 | 0.00%  | http/query/report | 是/否         | —    | —   |


建议同时保存：

- k6 summary export：`tmp/perf/<profile>/k6-summary.json`
- before/after snapshot：`tmp/perf/<profile>/snapshot-*`
- Nginx 分组统计：按 status + path + code 聚合
- outbox Mongo aggregate
- apiserver / collection / worker 关键错误日志

---
