# Cache Capability Registry

## 结论

Cache 已形成三个稳定入口：共享机制在 `internal/pkg/cache`，apiserver 的 capability/adapter/governance 在 `internal/apiserver/cache`，collection-server 的 L1 与生命周期在 `internal/collection-server/cache`。Redis family/profile/namespace 属于 `internal/pkg/redisruntime`，不再以 cache package 承载 lock、rank、signal 等 workload。

进程内 Effective Registry 使用路径派生的稳定 ID，例如 `cache.capabilities.catalog.questionnaire` 对应 `catalog.questionnaire`。Registry 返回 layer、family、最终 Policy、配置来源与 schema version。

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

| Capability ID | Owner | Layer | Family | Policy 来源 | Loader / 事实源 | 失效与预热 |
| --- | --- | --- | --- | --- | --- | --- |
| `catalog.questionnaire` | collection-server | L1 | `local` | `cache.capabilities.catalog.questionnaire` | apiserver questionnaire gRPC | signal 精确/前缀删除，TTL 兜底 |
| `catalog.typology` | collection-server | L1 | `local` | `cache.capabilities.catalog.typology` | assessment-model catalog gRPC | signal 驱逐；启动预热 list/categories |
| `catalog.scale` | apiserver | L2 | `static_meta` | apiserver effective catalog | Mongo published model | publish/manual warmup；现有成功判定保持 |
| `catalog.questionnaire` | apiserver | L2 | `static_meta` | apiserver effective catalog | Mongo questionnaire | 写后删除；startup/publish/manual warmup |
| `catalog.published_model` | apiserver | L2 | `static_meta` | apiserver effective catalog | Mongo published model | upsert 后失效 list/algorithm keys |
| `assessment.detail` | apiserver | L2 | `object_view` | apiserver effective catalog | MySQL assessment | Save/Delete 后按 ID 删除 |
| `assessment.list` | apiserver | L1+L2 | `query_result` + `meta_hotset` | apiserver effective catalog | assessment read model | version token bump |
| `actor.testee` | apiserver | L2 | `object_view` | apiserver effective catalog | MySQL testee | 写后删除；negative sentinel |
| `plan.detail` | apiserver | L2 | `object_view` | apiserver effective catalog | MySQL plan | Save 后删除 |
| `statistics.query` | apiserver | L2 | `query_result` + `meta_hotset` | apiserver effective catalog | statistics read model | version token / service invalidation；startup/repair/manual warmup |
| `report_status` | 三进程 | operational Redis state | `ops_runtime` | `cache.capabilities.report_status` | report workflow | 单调状态覆盖、TTL、signal 唤醒 |

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

- apiserver 使用 `defaults/capabilities/governance`；
- collection-server 使用 `capabilities.catalog` 与 `capabilities.report_status`；
- worker 只保留真实消费者 `capabilities.report_status`；
- `scale_cache`、`wait_report.status_ttl_seconds`、`wait_report.pubsub_*` 和 worker 无消费者开关已删除；
- 未知 cache capability 或字段由 `pkg/app` raw-settings validator 在启动时拒绝；
- Redis route 仍只由 `redis_runtime` 管理。

## 行为合同

本轮重构保持以下外部行为：

- Redis key 字节、namespace suffix 与 versioned-key 结构；
- payload JSON/gzip、negative sentinel 与 miss/error 语义；
- TTL、jitter、FIFO、clone、prefix delete 与 singleflight；
- Prometheus metric name 与 label schema；
- Cache-Aside 留在 application query service 或 infrastructure decorator；domain 不依赖 cache。

## 新增 capability 的登记要求

新增能力必须同时声明 owner、layer、family、Policy、key/codec、loader、失效、预热、观测、配置 source 与 contract test。没有 runtime consumer 的配置不得进入 production YAML。

## 验证入口

```bash
go test ./internal/pkg/cache/... ./internal/pkg/redisruntime/...
go test ./internal/apiserver/cache/... ./internal/collection-server/cache
go test ./internal/pkg/configcontract ./internal/pkg/architecture
```
