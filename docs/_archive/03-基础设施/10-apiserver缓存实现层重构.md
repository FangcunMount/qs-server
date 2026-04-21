# 【阶段完成记录】apiserver 缓存实现层重构

> **状态说明**
> 本文记录 `infra/cache` 收缩为纯实现层时的阶段性重构结果，属于**完成记录**，不再承担 Redis 现行真值层入口职责。
> 当前实现请读 [06-Redis使用情况.md](../../03-基础设施/06-Redis使用情况.md)，新增缓存与锁的接入方法请读 [11-Redis三层设计与落地手册.md](../../03-基础设施/11-Redis三层设计与落地手册.md)。
> 本文中的代码锚点按阶段完成时刻保留，**不保证继续与当前仓库路径完全一致**。

## 结论
`internal/apiserver/infra/cache` 现在只负责 `apiserver` 自身的缓存实现，不再承担 Redis 路由、family 治理、全局 TTL/压缩兼容层、旧式预热服务等职责。

重构后的固定边界如下：

- `component-base/pkg/redis`
  只负责 Redis Foundation，包括连接、底层工具与通用 Redis 能力。
- `internal/pkg/redisplane`
  是唯一的 Redis runtime 路由与 family 治理入口，负责 family 到 profile/namespace/builder 的解析。
- `internal/pkg/cacheobservability`
  是唯一的 family 级状态、指标、readiness 快照来源。
- `internal/apiserver/infra/cachepolicy`
  是唯一的对象级缓存策略来源，负责对象策略键、family 归属、family 默认策略合并。
- `internal/apiserver/infra/cache`
  只保留 apiserver 具体缓存实现，包括 repository decorator、read-through、version token、versioned query、本地热点缓存和 hotset。
- `internal/apiserver/application/cachegovernance`
  负责预热、热榜、治理状态组装；它消费 `redisplane + cacheobservability + infra/cache(hotset)`，不再依赖 `infra/cache` 内部自带 catalog。

## 这次移除了什么
以下旧层已经整体移除，不再保留兼容 facade：

- `CacheCatalog / CacheFamily / PolicyFamily`
- `CacheKeyBuilder`
- `TTLOptions / ApplyTTLOptions / ApplyCompressionFlag`
- `CacheManager / CacheMetrics / TypedCache`
- `WarmupService`
- `DeletePattern`、`MGet`、`MSet`、`Ping` 这类不在主路径上的缓存抽象能力

这些能力移除后，`infra/cache` 不再是“既做缓存，又做治理，又做路由”的混合层。

## 新的依赖方向
### 1. 运行时路由
`apiserver/server.go` 与 `container/container.go` 只通过 `redisplane.Handle` 获取：

- `Client`
- `Builder`
- `Namespace`
- `AllowWarmup`

`infra/cache` 目录内的实现不再自己推导 Redis profile、namespace suffix，也不再维护第二套 family 枚举。

### 2. 对象级策略
所有对象缓存策略统一走 `cachepolicy.PolicyCatalog`：

- `CachePolicyKey -> CachePolicy`
- `CachePolicyKey -> redisplane.Family`

对象缓存只关心自己的策略，不再关心 Redis runtime 路由。

### 3. 实现层最小接口
`infra/cache/store.go` 中的 `Cache` 只保留主路径真实需要的方法：

- `Get`
- `Set`
- `Delete`
- `Exists`

这意味着：

- “我的测评列表”继续通过 `version token + versioned key` 失效
- 不再依赖 `DeletePattern`
- 不再把模式删除作为生产主路径能力暴露出去

## `infra/cache` 现在还保留什么
保留的都是 apiserver 必须存在的缓存实现：

- repository decorator
  - `scale_cache.go`
  - `questionnaire_cache.go`
  - `assessment_detail_cache.go`
  - `testee_cache.go`
  - `plan_cache.go`
- query/list cache
  - `version_token_store.go`
  - `versioned_query_cache.go`
  - `my_assessment_list_cache.go`
- 通用实现
  - `redis_cache.go`
  - `readthrough.go`
  - `singleflight.go`
  - `local_hot_cache.go`
- 热点与预热支撑
  - `hotset.go`

## `cachegovernance` 现在怎么工作
`cachegovernance` 不再接收 `CacheCatalog`。

它现在依赖：

- `redisplane` 提供 family runtime 信息
- `cacheobservability` 提供 family 状态快照
- `hotset` 提供 query/static 热点查询与写入

因此，预热是否允许、family 是否可用、readiness 是否降级，这些判断现在都来源于共享治理面，而不是 apiserver 私有的一套目录对象。

## 迁移后的编码约束
后续如果继续新增或修改缓存代码，需要遵守下面的约束：

1. 不要在 `infra/cache` 中新增 Redis profile、namespace、fallback 解析逻辑。
2. 不要在 `infra/cache` 中新增第二套 family 枚举。
3. 对象缓存策略只能从 `cachepolicy` 读取。
4. family 级状态只能从 `cacheobservability` 读取。
5. 预热许可、builder、namespace 只能从 `redisplane.Handle` 读取。
6. 不要重新引入 `DeletePattern` 作为主路径失效方式。

## 对开发者的直接影响
当你要新增一个 apiserver 缓存对象时，推荐顺序如下：

1. 在 `cachepolicy` 中增加对象策略键，并指定它属于哪个 `redisplane.Family`。
2. 在 `container` 中通过对应 family 的 `Handle` 取 `Client + Builder`。
3. 在 `infra/cache` 中实现具体 decorator 或 read-through。
4. 如需纳入预热或热榜，再在 `cachegovernance` 中显式接入。

这样可以保证缓存实现、运行时路由、治理状态三件事始终分层，而不是重新缠在一起。
