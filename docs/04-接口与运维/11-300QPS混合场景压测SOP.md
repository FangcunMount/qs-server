# 300 QPS 混合场景压测 SOP

**本文回答**：如何用 k6 / ghz 对 qs-server 三进程架构做 300 QPS 混合压测，覆盖 collection-server 前台入口、qs-apiserver 权威业务状态和 qs-worker 异步解析链路，并记录 HTTP、gRPC、异步链路和资源指标。

---

## 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| 压测入口 | k6 压 HTTP 混合场景；ghz 压 gRPC 等价链路 |
| 三进程覆盖 | collection 负责前台查询、提交、状态查询；apiserver 负责统计、持久化、事件 outbox；worker 负责 MQ 消费和 internal gRPC 回调 |
| 默认配比 | 问卷/测评配置查询 120 QPS，答卷提交 60 QPS，报告状态查询 90 QPS，统计查询 30 QPS |
| 异步观测 | worker `/metrics` 的 `qs_event_consume_*`，apiserver/worker event outbox 指标，NSQ depth |
| 结果口径 | 具体 P95、P99、错误率、worker 消费速率和积压清空耗时必须以实测结果填写 |

---

## 1. 场景配比

| 场景 | QPS | 说明 |
| ---- | ---: | ---- |
| 问卷/测评配置查询 | 120 | 模拟进入问卷页和加载配置 |
| 答卷提交 | 60 | 写链路，触发 SubmitQueue、apiserver durable submit、Outbox、MQ、worker |
| 报告状态查询 | 90 | 模拟提交后前端轮询 `wait-report` |
| 统计查询 | 30 | 模拟后台运营和筛查进度查询 |
| 合计 | 300 | 混合读写压力 |

---

## 2. 前置数据

`seeddata-runner` 当前线上配置可以直接作为压测数据口径：

| 字段 | 值 |
| ---- | ---- |
| `COLLECTION_BASE_URL` | `https://collect.fangcunmount.cn` |
| `APISERVER_BASE_URL` | `https://qs.fangcunmount.cn` |
| `IAM_BASE_URL` | `https://iam.fangcunmount.cn` |
| `ORG_ID` | `1` |
| `SCALE_CODES` | `3adyDE,zOO4eG,WFIRSP,bJFKi3,mbdoeV,tuixuu,sJFa2R,tssl35` |
| `PLAN_IDS` | `614333603412718126,614187067651404334` |
| `TESTEE_SOURCE` | `daily_simulation` |

压测前至少准备：

| 数据 | 用途 | k6/ghz 环境变量 |
| ---- | ---- | ---- |
| 用户 token | collection 和 apiserver 认证 | `TOKEN`，或分别传 `COLLECTION_TOKEN` / `APISERVER_TOKEN` |
| 多用户 token | 避免单用户限流影响吞吐测试 | `COLLECTION_TOKENS` / `APISERVER_TOKENS`，英文逗号分隔 |
| 多用户 token 文件 | 避免命令行过长和 token 泄漏 | `TOKENS_FILE`，必要时再拆 `COLLECTION_TOKENS_FILE` / `APISERVER_TOKENS_FILE` |
| k6 配置文件 | 集中维护压测参数 | `PERF_CONFIG_FILE` |
| 量表 code | 自动发现问卷并生成答案 | `SCALE_CODES` |
| 可访问 testee_id | 答卷提交和报告状态查询 | `TESTEE_IDS`，或 `AUTO_DISCOVER_SEEDDATA=true` |
| 已存在 assessment_id | 报告状态查询样本 | `ASSESSMENT_IDS` / `REPORT_SAMPLES_FILE`，或 `AUTO_DISCOVER_SEEDDATA=true` |
| 答案 payload | 可选，覆盖自动答案生成 | `ANSWERS_JSON` 或 `ANSWERS_FILE` |
| gRPC mTLS 证书 | ghz 直连 apiserver | `GRPC_CACERT` / `GRPC_CERT` / `GRPC_KEY` |

如果本机没有现成 token，可以用 IAM 凭据换取。`seeddata-runner/configs/seeddata.yaml` 里用户名和密码为空，线上通常由 systemd `Environment` 或 `EnvironmentFile` 注入；先从服务器确认 `IAM_USERNAME` / `IAM_PASSWORD`，不要提交到仓库：

```bash
export IAM_USERNAME='...'
export IAM_PASSWORD='...'
export TOKEN="$(./scripts/perf/fetch-iam-token.sh)"
test -n "$TOKEN"
```

k6 会在 `DISCOVER_ANSWERS=true` 时按 `SCALE_CODES` 拉取 `/api/v1/scales/{code}` 和 `/api/v1/questionnaires/{code}`，并按 seeddata-runner 的口径生成 `Radio` / `Checkbox` / `Text` / `Textarea` / `Number` 答案。`Section` 题会跳过。

推荐使用配置文件集中维护 k6 参数，环境变量仍然优先于配置文件：

```bash
mkdir -p tmp/perf
cp scripts/perf/qs-perf.config.example.json tmp/perf/qs-perf.config.json
```

如果已经有旧版 `tmp/perf/qs-perf.config.json`，不要直接覆盖其中的真实 URL/token 文件配置；只需要把示例配置里的 `qpsProfile` 和 `qpsProfiles` 合并进去。

然后编辑 `tmp/perf/qs-perf.config.json`。常用字段：

| 字段 | 说明 |
| ---- | ---- |
| `collectionBaseUrl` / `apiserverBaseUrl` | 线上或压测环境入口 |
| `tokensFile` | 公共批量 token 输出文件 |
| `collectionTokensFile` / `apiserverTokensFile` | 可选，collection/apiserver 权限不同才拆开 |
| `qpsProfile` | 默认压测档位，不传 `QPS_PROFILE` 时使用 |
| `qpsProfiles.*.qps` | 各档位的四类场景 QPS |
| `qpsProfiles.*.duration` | 各档位的压测持续时间 |
| `qpsProfiles.*.vusers` | 各档位的 k6 VU 预分配和上限 |
| `scaleCodes` / `planIds` / `testeeSource` | seeddata 数据口径 |
| `autoDiscoverSeeddata` / `discoverAnswers` | 是否自动找 testee/assessment 和生成答案 |
| `paths.*` | 接口路径覆盖 |

文件类字段支持绝对路径；如果写相对路径，会优先按配置文件所在目录解析。例如配置文件在 `tmp/perf/qs-perf.config.json`，`tokensFile: "tokens.json"` 会读取 `tmp/perf/tokens.json`。

内置档位：

| `QPS_PROFILE` | 总 QPS | 配比 | 用途 |
| ---- | ---: | ---- | ---- |
| `smoke_4` | 4 | 1 / 1 / 1 / 1 | 全链路连通性 smoke |
| `pretest_60` | 60 | 24 / 12 / 18 / 6 | 预压测，默认档位 |
| `pretest_120` | 120 | 48 / 24 / 36 / 12 | 中间档，观察限流和资源曲线 |
| `mixed_300` | 300 | 120 / 60 / 90 / 30 | 正式 300 QPS 混合压测 |
| `mixed_300_probe` | 300 + 0.2 | 300 QPS + 异步链路探针 | 采样 `report_generated_latency` |

使用配置文件和指定档位运行：

```bash
k6 run -e PERF_CONFIG_FILE="$(pwd)/tmp/perf/qs-perf.config.json" \
  -e QPS_PROFILE=pretest_60 \
  --summary-export tmp/perf/300qps/k6-summary.json \
  scripts/perf/k6-mixed-300qps.js
```

优先级：命令行环境变量 > `QPS_PROFILE` 档位配置 > 根配置 > 脚本默认值。因此临时调整仍可直接传 `-e QUERY_RPS=10 -e DURATION=1m`。

注意限流口径：collection 默认单用户限流为 `submit=5 QPS`、`query=10 QPS`、`wait-report=2 QPS`，默认全局 `wait-report=80 QPS`。如果正式场景要打 `wait-report=90 QPS`，需要满足至少一项：

- 压测环境临时调高 `rate_limit.wait_report_global_qps` 到大于 90，例如 120。
- 准备足够多的 `COLLECTION_TOKENS`，按默认 `wait-report=2 QPS/user` 计算，90 QPS 至少 45 个前台测试用户 token；考虑抖动建议 60 个以上。
- 如果本轮只验证服务端非限流吞吐，可以临时降低 `REPORT_RPS` 到 80 以下，并记录结论口径。

多 token 传法：

```bash
export COLLECTION_TOKENS='token1,token2,token3'
export APISERVER_TOKENS='admin-token1,admin-token2'
```

更推荐使用本地凭据文件批量换 token。凭据文件不要提交到 Git，可参考 `scripts/perf/iam-users.example.json`，复制到 `tmp/perf/`：

```bash
cp scripts/perf/iam-users.example.json tmp/perf/iam-users.json
```

编辑 `tmp/perf/iam-users.json`。如果当前测试账号同时能访问 collection 和 apiserver，把用户都放在 `collection_users` 即可，生成一个公共 `tokens.json`：

```json
{
  "collection_users": [
    { "username": "collection-user-1", "password": "...", "tenant_id": 1 }
  ],
  "apiserver_users": []
}
```

然后换 token 到临时文件：

```bash
IAM_USERS_FILE=tmp/perf/iam-users.json \
IAM_USERS_GROUP=collection_users \
TOKENS_OUTPUT_FILE=tmp/perf/tokens.json \
./scripts/perf/fetch-iam-tokens.sh
```

`collection_users` 默认按 seeddata-runner 的 daily simulation 登录口径省略 `tenant_id`；如果要先验证 1 个账号，可以加 `IAM_USERS_LIMIT=1`：

```bash
IAM_USERS_FILE=tmp/perf/iam-users.json \
IAM_USERS_GROUP=collection_users \
IAM_USERS_LIMIT=1 \
TOKENS_OUTPUT_FILE=tmp/perf/token-smoke.json \
./scripts/perf/fetch-iam-tokens.sh
```

如果 collection 和 apiserver 需要不同权限账号，再分别使用 `collection_users` / `apiserver_users` 生成 `collection-tokens.json` / `apiserver-tokens.json`，并在配置中使用 `collectionTokensFile` / `apiserverTokensFile`。

如果 smoke 在 `setup()` 阶段出现 `setup_discovery_failed` + `http_403_total`，并报 `No testee IDs found`，通常说明公共 `tokens.json` 里的 collection 用户没有 apiserver 自动发现权限。此时生成 apiserver 专用 token：

```bash
IAM_USERS_FILE=tmp/perf/iam-users.json \
IAM_USERS_GROUP=apiserver_users \
TOKENS_OUTPUT_FILE=tmp/perf/apiserver-tokens.json \
./scripts/perf/fetch-iam-tokens.sh
```

并在 `tmp/perf/qs-perf.config.json` 增加：

```json
"apiserverTokensFile": "apiserver-tokens.json"
```

k6 使用 token 文件：

```bash
export TOKENS_FILE=tmp/perf/tokens.json
```

每次运行 k6 前先做 token preflight。它只输出 token 数量、剩余 TTL 和 HTTP 状态码，不打印 token 明文：

```bash
PERF_CONFIG_FILE=tmp/perf/qs-perf.config.json \
./scripts/perf/check-token-preflight.sh
```

只有 `collection scale ...: 200`、`apiserver testees: 200`，且 `min_ttl_seconds` 大于本轮压测持续时间时，再开始 k6。IAM access token TTL 较短，正式 `mixed_300` 是 10 分钟，必须刚生成完 token 就跑。

`ANSWERS_FILE` 可以是单个对象或数组：

```json
[
  {
    "questionnaire_code": "kTC43z",
    "questionnaire_version": "3.0.1",
    "testee_id": "601002327771460142",
    "answers": [
      { "question_code": "1o8TK1yK", "question_type": "Radio", "value": "g1B0fi9d" }
    ]
  }
]
```

`REPORT_SAMPLES_FILE` 格式：

```json
[
  { "assessment_id": "601002327771460143", "testee_id": "601002327771460142" }
]
```

---

## 3. k6 HTTP 混合压测

只读连通性 smoke，不需要 token，只打 collection 公开量表接口：

```bash
COLLECTION_BASE_URL=https://collect.fangcunmount.cn \
QUERY_RPS=1 \
SUBMIT_RPS=0 \
REPORT_RPS=0 \
STATS_RPS=0 \
DURATION=5s \
QUESTIONNAIRE_QUERY_PATHS='/api/v1/scales?page=1&page_size=20&status=published,/api/v1/scales/categories,/api/v1/scales/hot?limit=5' \
k6 run scripts/perf/k6-mixed-300qps.js
```

小流量全链路 smoke，使用 seeddata 数据自动发现 testee/assessment 并自动生成答案：

```bash
TOKEN="$TOKEN" \
COLLECTION_BASE_URL=https://collect.fangcunmount.cn \
APISERVER_BASE_URL=https://qs.fangcunmount.cn \
ORG_ID=1 \
SCALE_CODES=3adyDE,zOO4eG,WFIRSP,bJFKi3,mbdoeV,tuixuu,sJFa2R,tssl35 \
PLAN_IDS=614333603412718126,614187067651404334 \
TESTEE_SOURCE=daily_simulation \
AUTO_DISCOVER_SEEDDATA=true \
QUERY_RPS=1 \
SUBMIT_RPS=1 \
REPORT_RPS=1 \
STATS_RPS=1 \
DURATION=30s \
k6 run --summary-export tmp/perf/k6-smoke-summary.json scripts/perf/k6-mixed-300qps.js
```

如果已经使用 token 文件，小流量 smoke 可以改为：

```bash
TOKENS_FILE=tmp/perf/tokens.json \
COLLECTION_BASE_URL=https://collect.fangcunmount.cn \
APISERVER_BASE_URL=https://qs.fangcunmount.cn \
ORG_ID=1 \
SCALE_CODES=3adyDE,zOO4eG,WFIRSP,bJFKi3,mbdoeV,tuixuu,sJFa2R,tssl35 \
PLAN_IDS=614333603412718126,614187067651404334 \
TESTEE_SOURCE=daily_simulation \
AUTO_DISCOVER_SEEDDATA=true \
QUERY_RPS=1 \
SUBMIT_RPS=1 \
REPORT_RPS=1 \
STATS_RPS=1 \
DURATION=30s \
k6 run --summary-export tmp/perf/k6-smoke-summary.json scripts/perf/k6-mixed-300qps.js
```

正式 300 QPS：

```bash
mkdir -p tmp/perf/300qps

OUT_DIR=tmp/perf/300qps ./scripts/perf/snapshot-observability.sh before

TOKEN="$TOKEN" \
COLLECTION_BASE_URL=https://collect.fangcunmount.cn \
APISERVER_BASE_URL=https://qs.fangcunmount.cn \
ORG_ID=1 \
SCALE_CODES=3adyDE,zOO4eG,WFIRSP,bJFKi3,mbdoeV,tuixuu,sJFa2R,tssl35 \
PLAN_IDS=614333603412718126,614187067651404334 \
TESTEE_SOURCE=daily_simulation \
AUTO_DISCOVER_SEEDDATA=true \
QUERY_RPS=120 \
SUBMIT_RPS=60 \
REPORT_RPS=90 \
STATS_RPS=30 \
DURATION=10m \
k6 run --summary-export tmp/perf/300qps/k6-summary.json scripts/perf/k6-mixed-300qps.js

OUT_DIR=tmp/perf/300qps ./scripts/perf/snapshot-observability.sh after
```

使用 token 文件时，正式 300 QPS 命令改为：

```bash
TOKENS_FILE=tmp/perf/tokens.json \
COLLECTION_BASE_URL=https://collect.fangcunmount.cn \
APISERVER_BASE_URL=https://qs.fangcunmount.cn \
ORG_ID=1 \
SCALE_CODES=3adyDE,zOO4eG,WFIRSP,bJFKi3,mbdoeV,tuixuu,sJFa2R,tssl35 \
PLAN_IDS=614333603412718126,614187067651404334 \
TESTEE_SOURCE=daily_simulation \
AUTO_DISCOVER_SEEDDATA=true \
QUERY_RPS=120 \
SUBMIT_RPS=60 \
REPORT_RPS=90 \
STATS_RPS=30 \
DURATION=10m \
k6 run --summary-export tmp/perf/300qps/k6-summary.json scripts/perf/k6-mixed-300qps.js
```

如果已经把这些值写入 `tmp/perf/qs-perf.config.json`，命令可以简化为：

```bash
k6 run -e PERF_CONFIG_FILE="$(pwd)/tmp/perf/qs-perf.config.json" \
  -e QPS_PROFILE=mixed_300 \
  --summary-export tmp/perf/300qps/k6-summary.json \
  scripts/perf/k6-mixed-300qps.js
```

如果预压阶段出现失败，优先查看 k6 输出中的这些计数：

- `http_429_total`：限流，先检查 token 数量和服务端限流配置。
- `http_401_total` / `http_403_total`：token 失效或权限不匹配。
- `http_5xx_total`：服务端错误，再看服务日志、慢 SQL、worker/MQ 积压。
- `questionnaire_query_failed` / `answer_submit_failed` / `report_status_failed` / `statistics_failed`：定位失败集中在哪类场景。

可选严格阈值：

```bash
STRICT_THRESHOLDS=true k6 run scripts/perf/k6-mixed-300qps.js
```

说明：

- `report_status_query` 默认打 collection `/api/v1/assessments/{assessment_id}/wait-report`。
- 如果样本 assessment 不是终态，`wait-report` 会长轮询到 `REPORT_TIMEOUT`，P95 会反映等待时间，不应直接当作接口慢查询。
- `CHAIN_PROBE_RPS` 默认 0。需要采样 `report.generated` 端到端延迟时，可另起小流量探针，例如 `CHAIN_PROBE_RPS=0.2`，它会提交答卷、轮询 submit-status、查 assessment、等待报告终态，并输出 `report_generated_latency`。

---

## 4. ghz gRPC 压测

`scripts/perf/ghz-qs-grpc.sh` 支持以下 case：

| CASE | 链路 | RPC |
| ---- | ---- | ---- |
| `collection-submit` | collection -> apiserver submit 等价链路 | `answersheet.AnswerSheetService.SaveAnswerSheet` |
| `worker-score` | worker -> apiserver internal gRPC | `internalapi.InternalService.CalculateAnswerSheetScore` |
| `worker-create-assessment` | worker -> apiserver internal gRPC | `internalapi.InternalService.CreateAssessmentFromAnswerSheet` |
| `worker-evaluate` | worker -> apiserver internal gRPC | `internalapi.InternalService.EvaluateAssessment` |
| `worker-attention` | worker -> apiserver internal gRPC | `internalapi.InternalService.SyncAssessmentAttention` |

collection submit 等价压测。默认 dev/prod gRPC 配置是 TLS/mTLS，按实际证书路径传 `GRPC_CACERT`、`GRPC_CERT`、`GRPC_KEY`；只有临时将 apiserver gRPC 改成 plaintext 时，才使用 `GRPC_PLAINTEXT=true`。

```bash
CASE=collection-submit \
GRPC_TARGET=127.0.0.1:9090 \
RPS=60 \
DURATION=300s \
CONCURRENCY=60 \
QUESTIONNAIRE_CODE=kTC43z \
QUESTIONNAIRE_VERSION=3.0.1 \
TESTEE_ID=601002327771460142 \
WRITER_ID=601002327771460142 \
ORG_ID=1 \
ANSWERS_JSON='[{"question_code":"1o8TK1yK","question_type":"Radio","value":"g1B0fi9d"}]' \
GRPC_CACERT=/data/infra/ssl/grpc/ca/ca-chain.crt \
GRPC_CERT=/data/infra/ssl/grpc/server/qs-collection-server.crt \
GRPC_KEY=/data/infra/ssl/grpc/server/qs-collection-server.key \
GRPC_CNAME=qs-apiserver \
FORMAT=json \
OUTPUT=tmp/perf/ghz-collection-submit.json \
scripts/perf/ghz-qs-grpc.sh
```

worker internal gRPC 示例：

```bash
CASE=worker-evaluate \
GRPC_TARGET=127.0.0.1:9090 \
RPS=60 \
DURATION=300s \
CONCURRENCY=60 \
ASSESSMENT_ID=601002327771460143 \
GRPC_CACERT=/data/infra/ssl/grpc/ca/ca-chain.crt \
GRPC_CERT=/data/infra/ssl/grpc/server/qs-worker.crt \
GRPC_KEY=/data/infra/ssl/grpc/server/qs-worker.key \
GRPC_CNAME=qs-apiserver \
FORMAT=json \
OUTPUT=tmp/perf/ghz-worker-evaluate.json \
scripts/perf/ghz-qs-grpc.sh
```

plaintext 环境才传：

```bash
GRPC_PLAINTEXT=true
```

需要 metadata 时传 `GHZ_METADATA_JSON`：

```bash
GHZ_METADATA_JSON='{"authorization":"Bearer xxx"}'
```

---

## 5. 观测采样

压测前后各采一次：

```bash
OUT_DIR=tmp/perf/300qps ./scripts/perf/snapshot-observability.sh before
k6 run --summary-export tmp/perf/300qps/k6-summary.json scripts/perf/k6-mixed-300qps.js
OUT_DIR=tmp/perf/300qps ./scripts/perf/snapshot-observability.sh after
```

关键指标：

| 指标 | 来源 |
| ---- | ---- |
| HTTP QPS / latency / failed | k6 summary、服务 `/metrics` |
| collection SubmitQueue depth/status | collection `/governance/resilience` 和 `qs_resilience_queue_*` |
| Outbox backlog | `qs_event_outbox_backlog`、`qs_event_outbox_oldest_age_seconds` |
| NSQ topic/channel depth | `http://<nsqd>:4151/stats?format=json` |
| worker ack/nack | worker `/metrics` 的 `qs_event_consume_total` |
| worker 消费耗时 | worker `/metrics` 的 `qs_event_consume_duration_seconds` |
| gRPC p95/p99/error rate | ghz JSON 输出；当前仓库未发现独立的 collection/worker gRPC latency Prometheus 指标 |
| report.generated 延迟 | k6 `report_generated_latency` 小流量链路探针，或按事件/outbox/报告时间戳离线计算 |

资源指标建议同步采：

- `docker stats --no-stream` 或 Prometheus/container metrics。
- MySQL CPU、连接数、慢 SQL。
- MongoDB 慢查询和连接池。
- Redis `INFO stats`、`INFO commandstats`、hit rate。

---

## 6. 结果模板

### qs-server 300 QPS 混合场景压测结果

### 1. 测试环境

- 机器配置：
- qs-apiserver 副本数：
- collection-server 副本数：
- qs-worker 副本数：
- MySQL：
- MongoDB：
- Redis：
- NSQ：
- Git Commit：

### 2. 压测场景

- 总压力：300 QPS
- 持续时间：
- 问卷查询：120 QPS
- 答卷提交：60 QPS
- 报告状态查询：90 QPS
- 统计查询：30 QPS

### 3. HTTP 结果

| 接口 | QPS | P95 | P99 | 错误率 |
| --- | ---: | ---: | ---: | ---: |
| 问卷查询 | | | | |
| 答卷提交 | | | | |
| 报告状态查询 | | | | |
| 统计查询 | | | | |
| 总体 | 300 | | | |

### 4. gRPC 结果

| 链路 | RPS | P95 | P99 | 错误率 |
| --- | ---: | ---: | ---: | ---: |
| collection -> apiserver submit | | | | |
| worker -> apiserver internal | | | | |

### 5. 异步链路结果

| 指标 | 结果 |
| --- | ---: |
| NSQ 最大积压 | |
| worker 平均消费速率 | |
| worker 峰值消费速率 | |
| 积压清空耗时 | |
| report.generated P95 | |
| retry / nack 数量 | |

### 6. 资源使用

| 组件 | CPU 峰值 | 内存峰值 | 备注 |
| --- | ---: | ---: | --- |
| collection-server | | | |
| qs-apiserver | | | |
| qs-worker | | | |
| MySQL | | | |
| MongoDB | | | |
| Redis | | | |

### 7. 结论

- 是否达到 300 QPS：
- 主要瓶颈：
- 已优化项：
- 后续优化项：

---

## 7. Verify

```bash
k6 inspect scripts/perf/k6-mixed-300qps.js
bash -n scripts/perf/ghz-qs-grpc.sh
bash -n scripts/perf/snapshot-observability.sh
```
