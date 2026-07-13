# Cache Capability Registry

## 结论

Cache 已形成三个稳定入口：共享机制在 `internal/pkg/cache`，apiserver 的 capability/adapter/governance 在 `internal/apiserver/cache`，collection-server 的 L1 与生命周期在 `internal/collection-server/cache`。Redis family/profile/namespace 属于 `internal/pkg/redisruntime`，不再以 cache package 承载 lock、rank、signal 等 workload。

apiserver 的 capability ID 使用业务模块前缀，例如 `cache.capabilities.survey.questionnaire` 对应 `survey.questionnaire`。collection-server 的既有 L1 ID 保持 `catalog.questionnaire/catalog.typology`。Registry v2 是进程内唯一 Policy 事实源，返回 owner、kind、layer、family、四层 Policy、最终 Policy、配置来源、snapshot/catalog 版本和兼容 metric label。apiserver 可通过受保护的 system-governance 动作原子 reload 七个普通 capability；collection-server 保持静态 snapshot。

## 代码入口

| 层 | 当前事实源 |
| --- | --- |
| 共享合同与 registry | [`internal/pkg/cache`](../../../internal/pkg/cache) |
| L1 TTL/read-through | [`internal/pkg/cache/local`](../../../internal/pkg/cache/local) |
| Redis payload | [`internal/pkg/cache/redis`](../../../internal/pkg/cache/redis) |
| Object/query kernel | [`internal/pkg/cache/object`](../../../internal/pkg/cache/object)、[`internal/pkg/cache/query`](../../../internal/pkg/cache/query) |
| Cache metrics | [`internal/pkg/cache/observe`](../../../internal/pkg/cache/observe) |
| apiserver capability | [`internal/apiserver/cache`](../../../internal/apiserver/cache) |
| collection capability | [`internal/collection-server/cache`](../../../internal/collection-server/cache) |
| Redis runtime/status | [`internal/pkg/redisruntime`](../../../internal/pkg/redisruntime) |

## Active capability

| Capability ID | Owner | Kind | Layer | Family | Policy 来源 | Legacy metric label | Loader / 事实源 | 失效与预热 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `catalog.questionnaire` | collection-server | cache | L1 | `local` | `cache.capabilities.catalog.questionnaire` | `catalog.questionnaire` | apiserver questionnaire gRPC | signal 精确/前缀删除，TTL 兜底 |
| `catalog.typology` | collection-server | cache | L1 | `local` | `cache.capabilities.catalog.typology` | `catalog.typology` | assessment-model catalog gRPC | signal 驱逐；启动预热 list/categories |
| `survey.questionnaire` | survey | cache | L2 | `static_meta` | `cache.capabilities.survey.questionnaire` | `questionnaire` | Mongo questionnaire | 写后删除；startup/publish/manual warmup |
| `modelcatalog.published_model` | modelcatalog | cache | L2 | `static_meta` | `cache.capabilities.modelcatalog.published_model` | `published_model` | Mongo published model | latest-by-code 真填充；upsert 后精确失效 latest/list/algorithm keys |
| `evaluation.assessment_detail` | evaluation | cache | L2 | `object_view` | `cache.capabilities.evaluation.assessment_detail` | `assessment_detail` | MySQL assessment | Save/Delete 后按 ID 删除 |
| `evaluation.assessment_list` | evaluation | cache | L1+L2 | `query_result` + `meta_hotset` | `cache.capabilities.evaluation.assessment_list` | `assessment_list` | assessment read model | version token bump |
| `actor.testee` | actor | cache | L2 | `object_view` | `cache.capabilities.actor.testee` | `testee` | MySQL testee | 写后删除；negative sentinel |
| `plan.detail` | plan | cache | L2 | `object_view` | `cache.capabilities.plan.detail` | `plan` | MySQL plan | Save 后删除 |
| `statistics.query` | statistics | cache | L2 | `query_result` + `meta_hotset` | `cache.capabilities.statistics.query` | `stats_query` | statistics read model | version token / service invalidation；startup/repair/manual warmup |
| `report_status` | interpretation workflow | operational_state | runtime | `ops_runtime` | `cache.capabilities.report_status` | 保持现状 | report workflow | 单调状态覆盖、TTL、signal 唤醒 |

IAM user/profile-link/JWKS 与 WeChat SDK cache 保持 integration owner，不进入共享 business capability catalog。

## Redis workload registry

| Family | 角色 | 普通 cache payload |
| --- | --- | --- |
| `static_meta` | 静态目录/object L2 | 是 |
| `object_view` | ID-based object L2 | 是 |
| `query_result` | versioned query L2 | 是 |
| `meta_hotset` | version token / hotset metadata | 否，支撑 cache |
| `business_rank` | 业务 ZSET read model | 否 |
| `sdk_token` | integration credential | integration 私有 |
| `lock_lease` | 分布式 lease | 否 |
| `ops_runtime` | report status / signaling | 否，运行态 |

## 配置合同

三进程统一使用嵌套 schema：

```yaml
cache:
  defaults:       # apiserver 可继承技术默认值
  capabilities:   # 稳定 capability 配置；map key 不含点
  governance:     # warmup/hotset/read guard
```

- apiserver 使用 `defaults/capabilities/governance`，普通 capability 按 `survey/modelcatalog/evaluation/actor/plan/statistics` 分组；
- collection-server 使用 `capabilities.catalog` 与 `capabilities.report_status`；
- worker 只保留真实消费者 `capabilities.report_status`；
- `scale_cache`、`wait_report.status_ttl_seconds`、`wait_report.pubsub_*` 和 worker 无消费者开关已删除；
- 未知 cache capability 或字段由 `pkg/app` raw-settings validator 在启动时拒绝；
- Redis route 仍只由 `redis_runtime` 管理。

普通 apiserver capability 可配置 `enabled/ttl/negative_ttl/ttl_jitter_ratio/compress/singleflight/negative`。继承顺序固定为 capability override → family defaults → global defaults → `Spec.Defaults`；最终 effective Policy 不保留 `inherit`。`report_status` 只使用 operational-state 合同字段。

运行期 reload 使用：

```http
POST /internal/v1/system-governance/actions/cache.reload_policy/runs
```

该动作仅允许修改七个普通 capability 的 `ttl/negative_ttl/ttl_jitter_ratio/compress/singleflight/negative` 以及 global/family policy defaults，要求 `qs:admin`、`confirm=true` 和 `expected_version`。`enabled/family/layer/governance/report_status` 均为启动期静态合同；非法 candidate、读取失败或 version conflict 不改变当前 snapshot。成功 reload 只影响后续操作和新写入 entry，既有 Redis expiry 不追溯修改。

## 行为合同

本轮重构保持以下外部行为：

- Redis key 字节、namespace suffix 与 versioned-key 结构；
- payload JSON/gzip、negative sentinel 与 miss/error 语义；
- TTL、jitter、FIFO、clone、prefix delete 与 singleflight；
- Prometheus metric name 与 label schema；
- Cache-Aside 留在 application query service 或 infrastructure decorator；domain 不依赖 cache。

`modelcatalog.published_model` 新增的 entry 是此前不存在的新合同：

```text
<static namespace>:assessment_model:published:latest:<kind>:<lowercase-code>
```

scale/typology warmup 同步执行 source load、Redis Set、Exists 与 read-back；只有 entry 已可见才记录 `ok`。capability disabled 或 Redis unavailable 记录 `skipped`。

## 新增 capability 的登记要求

新增能力必须同时声明 owner、layer、family、Policy、key/codec、loader、失效、预热、观测、配置 source 与 contract test。没有 runtime consumer 的配置不得进入 production YAML。

## 验证入口

```bash
go test ./internal/pkg/cache/... ./internal/pkg/redisruntime/...
go test ./internal/apiserver/cache/... ./internal/collection-server/cache
go test ./internal/pkg/configcontract ./internal/pkg/architecture
```
