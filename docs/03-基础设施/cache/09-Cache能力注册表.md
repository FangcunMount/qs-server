# Cache Capability Registry

**结论**：qs-server 当前没有一个统一实现所有缓存的 `cache` 包；缓存能力由 Redis workload runtime、L1/L2 cache kernel、业务缓存适配器、配置和 governance 共同组成。本表是现行能力的事实入口，用于回答“缓存在哪里、由谁负责、如何配置、如何失效、能否预热”，不把 Redis lock、rank 或 signaling 误写成普通缓存。

## 1. Registry 边界

本表按以下规则登记：

- 只有能从当前 composition root、constructor 或 registry 追到运行路径的能力才标记为 `active`。
- `L1` 表示进程内缓存；`L2` 表示 Redis 共享缓存。跨进程的 `collection L1 -> apiserver L2` 会拆成两条能力登记。
- `family` 是 Redis workload 路由类别，不等同于业务缓存类型。
- TTL 表示启动时从默认值或配置解析得到的策略；当前不支持运行期热更新 TTL。
- Redis 中的 lock、hot rank、signal、hotset metadata 会在相邻 workload 表登记，但不算作通用 cache data plane。

状态说明：

| 状态 | 含义 |
| --- | --- |
| `active` | 当前生产装配路径存在，主要行为有代码或测试证据 |
| `partial` | 能力入口存在，但某些层级、预热或失效闭环未被当前代码证明 |
| `isolated` | 当前有效，但未接入主 cache policy/governance 体系 |
| `candidate` | 存在配置、策略或信令定义，但没有对应生产消费者 |

## 2. 系统分层与入口

| 层 | 主要职责 | 当前入口 |
| --- | --- | --- |
| Redis workload runtime | family、profile、namespace、fallback、availability | [`internal/pkg/cacheplane`](../../../internal/pkg/cacheplane) |
| Cache kernel | L1 TTL、L2 entry、read-through、singleflight、negative cache、version token | [`internal/pkg/localttlcache`](../../../internal/pkg/localttlcache)、[`internal/apiserver/infra/cacheentry`](../../../internal/apiserver/infra/cacheentry)、[`internal/apiserver/infra/cache`](../../../internal/apiserver/infra/cache)、[`internal/apiserver/infra/cachequery`](../../../internal/apiserver/infra/cachequery) |
| Business adapter | key、codec、loader、写后失效、业务 DTO clone | 各业务模块和 infra cache decorator |
| Config governance | defaults、capability override、effective policy、source/version | 当前分散在 process Options；目标见 [`10-Cache终局设计.md`](10-Cache终局设计.md) |
| Governance | warmup target、hotset、signal、status、manual action | [`internal/apiserver/cachebootstrap`](../../../internal/apiserver/cachebootstrap)、[`internal/apiserver/application/cachegovernance`](../../../internal/apiserver/application/cachegovernance) |

apiserver 的组合入口是 [`cachebootstrap.Subsystem`](../../../internal/apiserver/cachebootstrap/subsystem.go)；collection-server 的 L1 组合入口是 [`catalog_registry.go`](../../../internal/collection-server/container/catalog_registry.go) 和 [`catalog_cache_runtime.go`](../../../internal/collection-server/container/catalog_cache_runtime.go)。

## 3. Active Cache Capability Registry

| 状态 | Capability / owner | 层级 | Family / policy | TTL | Loader / 事实源 | 失效与一致性 | 预热 | 观测与代码事实源 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `active` | `collection.questionnaire.published_detail` / collection-server | L1 | 无 Redis family；无 apiserver policy | `questionnaire_cache.ttl_seconds`，默认 180s；支持 jitter | gRPC `GetQuestionnaire` -> apiserver | `questionnaire_cache_changed` 精确/前缀驱逐；信令丢失由 TTL 兜底 | collection 启动时不主动预热 | `collection_l1_cache_hits_total/misses_total`；[`application/questionnaire`](../../../internal/collection-server/application/questionnaire)、[`application/catalogl1`](../../../internal/collection-server/application/catalogl1) |
| `active` | `collection.typology.catalog` detail/list/categories / collection-server | L1 | 无 Redis family；无 apiserver policy | `typology_cache.ttl_seconds`，默认 180s；支持 jitter | gRPC assessment-model catalog -> apiserver | `typology_model_cache_changed` 驱逐 detail/list/categories；TTL 兜底 | 启动异步预热第一页 list 和 categories；30s 总超时 | `collection_l1_cache_hits_total/misses_total`；[`application/typologymodel`](../../../internal/collection-server/application/typologymodel)、[`application/catalogcache/warmup.go`](../../../internal/collection-server/application/catalogcache/warmup.go) |
| `active` | `apiserver.questionnaire` head/published/version / apiserver | L2 | `static_meta` / `questionnaire` | 正向默认 12h；negative 默认 5m；全局/族 jitter | Mongo questionnaire repository | 写成功后删除 head/published/version keys；negative sentinel 防穿透；Redis 不可用时不安装 decorator | startup、publish signal、manual warmup；填充 head 和 published | `qs_cache_get_total`、write/duration/payload/family status；[`questionnaire_cache.go`](../../../internal/apiserver/infra/cache/questionnaire_cache.go) |
| `partial` | `apiserver.published_model.catalog` list/algorithms / apiserver | L2 | `static_meta` / `published_model` | 默认 24h；支持 jitter | Mongo dual published-model store | upsert 后删除 algorithms 和有限集合的 list keys；其它组合依赖 TTL | governance 定义 scale/typology target，但当前回调调用未缓存的 `FindPublishedModelByCode`，不能证明填充 list/algorithms cache | 通用 cache 指标；[`published_model_cache.go`](../../../internal/apiserver/infra/cache/published_model_cache.go)、[`container/runtime_cache.go`](../../../internal/apiserver/container/runtime_cache.go) |
| `active` | `apiserver.assessment.detail` / apiserver | L2 | `object_view` / `assessment_detail` | 默认 2h；支持 jitter | MySQL assessment repository | Save/Delete 后按 assessment ID 删除；miss 使用 policy-scoped singleflight | 无 | 通用 cache 指标；[`assessment_detail_cache.go`](../../../internal/apiserver/infra/cache/assessment_detail_cache.go) |
| `active` | `apiserver.testee.detail` / apiserver | L2 | `object_view` / `testee` | 正向默认 30m；negative 默认 5m；支持 jitter | MySQL testee repository | Save/Update/Delete 后按 testee ID 删除；异步写回正向/negative value | 无 | 通用 cache 指标；[`testee_cache.go`](../../../internal/apiserver/infra/cache/testee_cache.go) |
| `active` | `apiserver.plan.detail` / apiserver | L2 | `object_view` / `plan` | 默认 2h；支持 jitter | MySQL plan repository | Save 后按 plan ID 删除；miss 使用 singleflight | 无 | 通用 cache 指标；[`plan_cache.go`](../../../internal/apiserver/infra/cache/plan_cache.go) |
| `active` | `apiserver.assessment.list` / apiserver | L1 + L2 | data=`query_result` / `assessment_list`；version token=`meta_hotset` | L1 固定 30s；L2 默认 10m | assessment list read model | user 维度 version token `INCR`，新版本 key 替代 pattern delete | 无 | 通用 query cache/version/payload 指标；[`my_assessment_list_cache.go`](../../../internal/apiserver/infra/cachequery/my_assessment_list_cache.go) |
| `active` | `apiserver.statistics.query` system/overview/questionnaire/plan / apiserver | L2 | data=`query_result` / `stats_query`；version token=`meta_hotset` 或 static token | query policy TTL 优先，默认 5m；仅 policy TTL 为 0 时使用 service 传入 TTL | MySQL statistics read model / service | version token 或写侧 service invalidation；查询失败不写缓存 | startup seed、statistics sync、repair、manual；支持四类 query target | 通用 query cache/version 指标 + warmup run/item 指标；[`infra/statistics/cache.go`](../../../internal/apiserver/infra/statistics/cache.go)、[`application/cachegovernance`](../../../internal/apiserver/application/cachegovernance) |
| `active` | `apiserver.wechat.sdk_token` / apiserver | L2，Redis 不可用时退化为 SDK memory cache | `sdk_token` / 不进入对象 policy catalog | 由微信 SDK 每次 `Set` 传入 token timeout | 微信 SDK/API | SDK Delete 或 TTL；Redis 错误不阻断 SDK cache 接口 | 无 | family success/failure；[`infra/wechatapi/cache_adapter.go`](../../../internal/apiserver/infra/wechatapi/cache_adapter.go) |
| `active` | `report.status` / apiserver、collection-server、worker | Redis operational state，不是 DB read-through cache | `ops_runtime` / 不进入对象 policy catalog | 默认 48h；reporter 使用 `report_status.ttl_seconds`，collection DB fallback write 使用 `wait_report.status_ttl_seconds`，当前存在双 Source of Truth | report workflow 写入的 status snapshot | 状态优先级单调覆盖；TTL 清理；signal 用于唤醒读方 | 无 | `report_status` 专用 hit/miss/set 指标；[`internal/pkg/reportstatus`](../../../internal/pkg/reportstatus) |
| `isolated` | `iam.user`、`iam.profile_link` / apiserver、collection-server | IAM client 内部 L1 | 不进入 cacheplane / policy catalog | 默认 user=5m、profile link=10m；可配置 max size | IAM gRPC client | 主要依赖 TTL；由 IAM client 库管理 | 无 | 未接入主 cache governance；[`apiserver IAM bootstrap`](../../../internal/apiserver/container/modules/iam/bootstrap.go)、[`collection IAM module`](../../../internal/collection-server/container/iam_module.go) |
| `isolated` | `iam.authz_snapshot` / apiserver、collection-server | 进程内 L1 | 不进入 cacheplane / policy catalog | 默认 30s | IAM authorization snapshot reader | TTL；authz version sync 可触发本地失效 | 无 | 未接入主 cache metrics；[`internal/pkg/iamauth/snapshot_loader.go`](../../../internal/pkg/iamauth/snapshot_loader.go) |

## 4. Redis Family Registry

`cacheplane.Family` 表示 Redis workload，而不是“缓存对象 family”。当前九个 family 的职责如下：

| Family | 当前角色 | 是否属于通用 cache data plane | 主要消费者 |
| --- | --- | --- | --- |
| `default` | 未显式路由时的 fallback | 否 | runtime fallback |
| `static_meta` | 低变化静态对象/目录 L2 | 是 | questionnaire、published model |
| `object_view` | 按 ID 读取的业务对象 L2 | 是 | assessment、testee、plan |
| `query_result` | 查询结果和列表 L2 | 是 | assessment list、statistics |
| `meta_hotset` | query version token、warmup hotset metadata | 支撑 cache，但不是业务 payload cache | cachequery、cachehotset |
| `business_rank` | Redis ZSET 业务热度读模型 | 否 | scale hot rank projection |
| `sdk_token` | 第三方 SDK credential cache | 是，但属于 integration adapter | WeChat SDK |
| `lock_lease` | 分布式 lease/leader lock | 否 | schedulers、worker、submit protection |
| `ops_runtime` | report status、信令等临时运行态 | 部分；不是通用 read-through cache | reportstatus、cachesignal |

family 路由、profile、namespace、fallback 和 `allow_warmup` 的事实源是 [`cacheplane/catalog.go`](../../../internal/pkg/cacheplane/catalog.go) 与各进程的 `redis_runtime.families` 配置。

## 5. Cache Config Registry

配置状态按“声明、解析、装配、消费”四段链路判定。YAML 能解析不等于配置真正影响运行时。

| 状态 | 配置入口 | 当前消费者 / effective 行为 | 终局归属 |
| --- | --- | --- | --- |
| `active` | apiserver `redis` / `redis_profiles` / `redis_runtime` | Redis connection、family/profile/namespace runtime | `redisruntime`，不进入 cache policy |
| `partial` | apiserver `cache.static/object/query` | family default 会被 questionnaire/testee/stats-query 等对象 policy 覆盖 | `cache.defaults` 只提供可继承默认值；最终行为进入 `cache.capabilities` |
| `active` | apiserver `cache.ttl.*` | questionnaire、published model、assessment、testee、plan 等对象 policy | 迁移到对应 `cache.capabilities.<id>.ttl` |
| `active` | apiserver `cache.statistics_*` / `cache.warmup` | statistics read guard、startup seed、governance/hotset | `cache.governance` 与 capability warmup policy |
| `candidate` | apiserver `cache.meta/sdk/lock` | meta 在 process 装配被丢弃；sdk/lock family policy 无对象 Policy Key 消费 | 删除；SDK TTL 归 adapter，lock 只归 `redis_runtime` |
| `active` | collection `questionnaire_cache` / `typology_cache` | catalog registry 构造 L1、coalescer、signal watcher | `cache.capabilities.<id>.l1` |
| `candidate` | collection `scale_cache` | 仅 options/flags/validation，catalog registry 无 scale spec | 删除或 planned；建立真实 owner 前不能 enabled |
| `partial` | collection `wait_report.status_ttl_seconds` + shared `report_status.ttl_seconds` | 两条写路径使用不同配置源，当前值碰巧一致 | 合并为跨进程 `report.status` capability TTL |
| `candidate` | collection `wait_report.pubsub_channel` | 解析/校验存在，runtime 未读取；真实信令来自 `signaling.redis` | 删除 |
| `isolated` | `iam.jwks.cache-ttl` / `iam.user-cache` / `iam.profile-link-cache` | IAM integration 内部缓存 | 保留在 IAM，登记 capability/observe |

终局的结构、Effective Cache Config Registry、兼容迁移和冲突规则见 [`10-Cache终局设计.md` 的配置治理章节](10-Cache终局设计.md#7-配置治理)。

## 6. Warmup Capability Registry

| Warmup kind | Family | Trigger | 当前 executor | 当前判定 |
| --- | --- | --- | --- | --- |
| `static.questionnaire` | `static_meta` | startup、publish、manual | `CachedQuestionnaireRepository.WarmupCache` | `active`：明确填充 head/published L2 |
| `static.scale` | `static_meta` | startup、publish、manual | `FindPublishedModelByCode(KindScale)` | `partial`：当前 published-model decorator 对该方法直接透传，未证明写入现有 L2 |
| `static.typology_model` | `static_meta` | publish、manual | `FindPublishedModelByCode(KindTypology)` | `partial`：同上；startup static planner 当前也不枚举 typology code |
| `query.stats_overview` | `query_result` | startup seed、statistics sync、repair、manual | statistics overview service | `active` |
| `query.stats_system` | `query_result` | startup seed、statistics sync、repair、manual | system statistics service | `active` |
| `query.stats_questionnaire` | `query_result` | seed、repair、manual | questionnaire statistics service | `active` |
| `query.stats_plan` | `query_result` | seed、repair、manual | plan statistics service | `active` |
| collection typology list/categories | 无 Redis family | collection startup | typology query service | `active`，但独立于 apiserver governance |

## 7. Candidate 与未闭环项

| 项目 | 当前证据 | 需要决策 |
| --- | --- | --- |
| collection `scale_cache` | options、flags、validation、dev/prod YAML 仍存在；当前 `catalogSpecs` 只有 questionnaire 和 typology | 删除遗留配置，或补回明确的 scale L1 owner；不能继续保持“配置有效但无消费者” |
| `PolicyScale` | policy key、family mapping、TTL 配置存在；生产代码没有消费者 | 合并到 `PolicyPublishedModel`，或为独立 scale cache 建立真实 adapter |
| scale L1 signal | `scale_cache_changed` 仍发布；collection 没有 scale watcher | 明确信令只服务 apiserver warmup，或恢复 collection consumer |
| scale/typology warmup | target、signal、manual API 存在；executor 调用未缓存的 detail read | 改为预热真实 cache key，或降低状态描述，避免把成功调用等同于 cache 已填充 |
| TTL “动态设置” | family/object 配置和 per-write TTL 都存在；policy/L1 store 在启动时解析并持有 | 对外统一称“分层、可配置 TTL”；如需热更新，应单独设计 policy snapshot/version 生命周期 |
| 跨进程 L1+L2 | questionnaire/typology 的 L1 在 collection，L2/事实源在 apiserver | 文档和观测必须按跨进程链路表达，不能假设同一 cache client 内串联 |

## 8. 新增或修改 Cache Capability 的登记要求

每次新增缓存能力，必须在同一变更中确认：

1. owner process 和业务 owner；
2. L1、L2 或 L1+L2，以及是否跨进程；
3. Redis family、profile、namespace 和 policy key；
4. key builder 与 payload codec；
5. 正向 TTL、negative TTL、jitter 和是否允许 per-write override；
6. loader、singleflight、超时与降级回源边界；
7. 写后失效、version token、signal 和 TTL 兜底；
8. startup/publish/repair/manual warmup 是否真实填充目标 key；
9. hit/miss/error/latency/payload/degraded/warmup 观测；
10. contract、concurrency、invalidation 和 architecture tests。

配置同时要求：

11. YAML/flag/env 字段对应的 typed config 与 runtime consumer；
12. default、override、source 和 effective value；
13. legacy/deprecated 字段的删除版本与冲突策略；
14. 跨进程 capability 的 key/payload/TTL 一致性合同。

## 9. 验证入口

Registry 变更至少运行：

```bash
make docs-hygiene
git diff --check
```

缓存实现发生变化时，再按触达范围运行：

```bash
go test ./internal/pkg/cacheplane/...
go test ./internal/pkg/cachegovernance/...
go test ./internal/apiserver/cachebootstrap
go test ./internal/apiserver/infra/cache ./internal/apiserver/infra/cacheentry
go test ./internal/apiserver/infra/cachequery ./internal/apiserver/infra/cachepolicy
go test ./internal/apiserver/application/cachegovernance
go test ./internal/collection-server/application/catalogl1
go test ./internal/collection-server/application/catalogcache
```
