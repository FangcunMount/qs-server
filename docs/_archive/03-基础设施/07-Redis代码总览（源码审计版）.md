# 【历史审计稿】Redis 代码总览（源码审计版）

> **状态说明**
> 本文保留为历史审计基线，记录 Redis 分层重构前后的源码观察过程，**不再作为现行真值层入口**。
> 当前实现请先读 [06-Redis使用情况.md](../../03-基础设施/06-Redis使用情况.md)，设计与接入手册请读 [11-Redis三层设计与落地手册.md](../../03-基础设施/11-Redis三层设计与落地手册.md)。
> 本文中的代码锚点按历史阶段保留，**不保证继续与当前仓库路径完全一致**。

**本文回答**：`qs-server` 当前源码里 Redis 到底接在什么地方、谁在使用、key 如何生成、TTL 如何控制、哪些是运行时真实路径、哪些只是保留配置或旧文档残留。

## 30 秒结论

| 维度 | 当前源码结论 |
| ---- | ------------ |
| Redis 主使用方 | **`qs-apiserver`**，承担静态对象缓存、对象视图缓存、query cache、meta/hotset、微信 SDK 缓存 |
| worker 当前 Redis 主用途 | **只有 lock/lease**：`answersheet:processing:*` 和 `qs:plan-scheduler:*` |
| collection-server | **当前运行时不初始化 Redis**；只保留配置兼容 |
| 配置模型 | **默认 Redis + named profiles**；family 路由到不同 profile / DB |
| key 规则 | 统一由 [`internal/pkg/rediskey/builder.go`](../../internal/pkg/rediskey/builder.go) 生成 |
| TTL 语义 | **有限 TTL + jitter + 事件失效**；不是永不过期 |
| query/list 失效策略 | 逐步从 `SCAN + DEL` 收敛到 **version token + versioned key** |
| 重要现实 | 仓库里已有 Redis 文档，但部分内容已比源码老；本文以当前代码审计结果为准 |

## 三进程分工

| 进程 | 当前 Redis 角色 | 代码入口 |
| ---- | --------------- | -------- |
| `qs-apiserver` | 缓存主进程：静态缓存、对象缓存、query cache、meta hotset、微信 SDK 缓存 | [`internal/apiserver/server.go`](../../internal/apiserver/server.go), [`internal/apiserver/container/container.go`](../../internal/apiserver/container/container.go) |
| `qs-worker` | lock/lease 客户端；用于答卷处理闸门和 plan scheduler 选主 | [`internal/worker/server.go`](../../internal/worker/server.go), [`internal/worker/handlers/answersheet_handler.go`](../../internal/worker/handlers/answersheet_handler.go), [`internal/worker/plan_scheduler.go`](../../internal/worker/plan_scheduler.go) |
| `collection-server` | 当前运行时不连接 Redis | [`internal/collection-server/server.go`](../../internal/collection-server/server.go) |

## 配置与连接层

### 1. 基础 Redis 配置

- 通用选项定义在 [`internal/pkg/options/redis_options.go`](../../internal/pkg/options/redis_options.go)
- `apiserver` 和 `worker` 都支持：
  - `redis.*`：默认 Redis
  - `redis_profiles.*`：命名 profile，通常只覆盖 DB

生产默认配置见 [`configs/apiserver.prod.yaml`](../../configs/apiserver.prod.yaml)：

| profile | 默认 DB | 用途 |
| ------- | ------- | ---- |
| 默认 redis | `1` | 基础默认连接 |
| `static_cache` | `2` | 量表/问卷静态对象 |
| `object_cache` | `3` | assessment/testee/plan 对象视图 |
| `query_cache` | `4` | query result，如统计查询、我的测评列表 |
| `sdk_cache` | `5` | 微信 SDK token/cache |
| `lock_cache` | `6` | lock/lease |
| `meta_cache` | `7` | hotset / version token |

### 2. 数据库管理器

- `apiserver` Redis 初始化在 [`internal/apiserver/database.go`](../../internal/apiserver/database.go)
- `worker` Redis 初始化在 [`internal/worker/database.go`](../../internal/worker/database.go)
- 两边都使用 `component-base` 的 `NamedRedisRegistry`
- profile 解析策略：
  - profile 存在且可用：使用 named profile
  - profile 未配置：回退默认 Redis
  - profile 已配置但不可用：返回错误或进入 degraded

## 命名空间与 key 生成

### 1. 统一 key builder

统一入口是 [`internal/pkg/rediskey/builder.go`](../../internal/pkg/rediskey/builder.go)。

关键方法：

- `BuildScaleKey(code)` -> `scale:{code}`
- `BuildScaleListKey()` -> `scale:list:v1`
- `BuildQuestionnaireKey(code, version)` -> `questionnaire:{code}` / `questionnaire:{code}:{version}`
- `BuildPublishedQuestionnaireKey(code)` -> `questionnaire:published:{code}`
- `BuildAssessmentDetailKey(id)` -> `assessment:detail:{id}`
- `BuildAssessmentListVersionKey(userID)` -> `query:version:assessment:list:{userID}`
- `BuildAssessmentListVersionedKey(...)` -> `query:assessment:list:{userID}:v{n}:{hash}`
- `BuildTesteeInfoKey(id)` -> `testee:info:{id}`
- `BuildPlanInfoKey(id)` -> `plan:info:{id}`
- `BuildStatsQueryKey(cacheKey)` -> `stats:query:{cacheKey}`
- `BuildWarmupHotsetKey(family, kind)` -> `warmup:hot:{family}:{kind}`
- `BuildAnswerSheetProcessingLockKey(id)` -> `answersheet:processing:{id}`
- `BuildLockKey(lockKey)` -> 原样拼进 namespace
- `BuildWeChatCacheKey(key)` -> `wechat:cache:{key}`

### 2. namespace 组合

- `cache.namespace` 是根 namespace
- 每个 family 再叠加自己的 `namespace_suffix`
- 组合逻辑在 [`internal/pkg/rediskey/builder.go`](../../internal/pkg/rediskey/builder.go) 和 [`internal/apiserver/infra/cache/catalog.go`](../../internal/apiserver/infra/cache/catalog.go)

最终常见前缀形态是：

- `prod:cache:static:*`
- `prod:cache:object:*`
- `prod:cache:query:*`
- `prod:cache:meta:*`
- `prod:cache:sdk:*`
- `prod:cache:lock:*`

## apiserver：缓存 family / profile 路由

family 定义在 [`internal/apiserver/infra/cache/catalog.go`](../../internal/apiserver/infra/cache/catalog.go)：

| family | 默认 profile | namespace suffix | 用途 |
| ------ | ------------- | ---------------- | ---- |
| `static_meta` | `static_cache` | `cache:static` | 量表/问卷静态对象与列表 |
| `object_view` | `object_cache` | `cache:object` | assessment/testee/plan 单对象 |
| `query_result` | `query_cache` | `cache:query` | 统计 query cache、我的测评列表 |
| `meta_hotset` | `meta_cache` | `cache:meta` | hotset ZSet、version token |
| `sdk_token` | `sdk_cache` | `cache:sdk` | 微信 SDK 缓存 |
| `lock_lease` | `lock_cache` | `cache:lock` | lock/lease |

运行时 family 解析逻辑在 [`internal/apiserver/server.go`](../../internal/apiserver/server.go) `resolveRedisFamilyClient(...)`：

- profile 为空：走默认 Redis
- profile 缺失：fallback 默认 Redis
- profile 不可用：degraded，family 返回 `nil`
- 可用：走 named profile

## apiserver：缓存对象总表

### 1. static family

#### 量表对象缓存

- 文件：[`internal/apiserver/infra/cache/scale_cache.go`](../../internal/apiserver/infra/cache/scale_cache.go)
- key：`scale:{code}`
- 默认生产 TTL：`2h`
- 读路径：`FindByCode` 走 `ReadThrough`
- 写/失效：
  - `Create` 直接写缓存
  - `Update` / `Remove` 删除缓存
- 不缓存：
  - `FindSummaryList`
  - `FindByQuestionnaireCode`

#### 问卷对象缓存

- 文件：[`internal/apiserver/infra/cache/questionnaire_cache.go`](../../internal/apiserver/infra/cache/questionnaire_cache.go)
- key：
  - `questionnaire:{code}`
  - `questionnaire:published:{code}`
  - `questionnaire:{code}:{version}`
- 默认生产 TTL：`2h`
- 支持 **negative cache**
- 失效策略：
  - `Update` / `Remove` / `HardDelete*` / active version 切换时按 code 家族删 key
- 预热只覆盖：
  - head/work key
  - published key
  - 不会全量预热所有历史版本 key

#### 已发布量表列表缓存

- 文件：[`internal/apiserver/application/scale/global_list_cache.go`](../../internal/apiserver/application/scale/global_list_cache.go)
- key：`scale:list:v1`
- Redis TTL：`10m`
- 节点内额外有一层 `LocalHotCache`，TTL `30s`
- 写法：
  - `Rebuild()` 整体重建
  - `GetPage()` 从 Redis 读整表后按页切片

### 2. object family

#### 测评详情缓存

- 文件：[`internal/apiserver/infra/cache/assessment_detail_cache.go`](../../internal/apiserver/infra/cache/assessment_detail_cache.go)
- key：`assessment:detail:{id}`
- 生产 TTL：`1h`
- `FindByID` 读穿透
- `Save*` / `Delete` 时失效

#### 受试者缓存

- 文件：[`internal/apiserver/infra/cache/testee_cache.go`](../../internal/apiserver/infra/cache/testee_cache.go)
- key：`testee:info:{id}`
- 生产 TTL：`30m`
- 支持 **negative cache**
- 只缓存单条 `FindByID`
- 批量 `FindByIDs`、各种列表查询不缓存

#### 计划缓存

- 文件：[`internal/apiserver/infra/cache/plan_cache.go`](../../internal/apiserver/infra/cache/plan_cache.go)
- key：`plan:info:{id}`
- 生产 TTL：`12h`
- 只缓存 `FindByID`
- 其他列表/按 scale 查找透传数据库

### 3. query family

#### 统计查询缓存

- 文件：[`internal/apiserver/infra/statistics/cache.go`](../../internal/apiserver/infra/statistics/cache.go)
- key：`stats:query:{cacheKey}`
- TTL：来自 `cache.query.ttl`，生产默认 `5m`
- 当前只承担 **查询结果缓存**
- Redis 读错误会降级成 miss，不阻塞主流程

#### 我的测评列表缓存

- 文件：[`internal/apiserver/infra/cache/my_assessment_list_cache.go`](../../internal/apiserver/infra/cache/my_assessment_list_cache.go)
- 关键点：
  - 不再用主路径 `DeletePattern`
  - 使用 **version token + versioned key**
- 相关组件：
  - [`version_token_store.go`](../../internal/apiserver/infra/cache/version_token_store.go)
  - [`versioned_query_cache.go`](../../internal/apiserver/infra/cache/versioned_query_cache.go)
- key 结构：
  - version key：`query:version:assessment:list:{userID}`
  - data key：`query:assessment:list:{userID}:v{n}:{hash}`
- 失效：
  - `Invalidate()` 只 bump version token
  - 旧数据依赖 TTL 自然过期

### 4. meta family

#### hotset / warmup 元数据

- 文件：[`internal/apiserver/infra/cache/hotset.go`](../../internal/apiserver/infra/cache/hotset.go)
- 存储结构：Redis `ZSet`
- key：`warmup:hot:{family}:{kind}`
- 用途：
  - 记录热点静态对象 / 热点 query 目标
  - 启动后或 repair/publish 后治理预热

#### query version token

- 文件：[`internal/apiserver/infra/cache/version_token_store.go`](../../internal/apiserver/infra/cache/version_token_store.go)
- 本质：
  - `Current` -> `GET`
  - `Bump` -> `INCR`

### 5. sdk family

#### 微信 SDK 缓存

- 文件：[`internal/apiserver/infra/wechatapi/cache_adapter.go`](../../internal/apiserver/infra/wechatapi/cache_adapter.go)
- key：`wechat:cache:{sdkKey}`
- 用途：给微信 SDK 适配 `cache.Cache`
- 降级：
  - Redis client 为 `nil` 时退回 `cache.NewMemory()`

## worker：当前真实 Redis 使用

### 1. answersheet 处理闸门

- 文件：[`internal/worker/handlers/answersheet_handler.go`](../../internal/worker/handlers/answersheet_handler.go)
- key：`answersheet:processing:{answerSheetID}`
- TTL：`5m`
- 实现：[`internal/pkg/redislock/lock.go`](../../internal/pkg/redislock/lock.go)
- 语义：
  - 外层 best-effort lease lock
  - Redis 不可用时允许 degraded 继续执行

### 2. plan scheduler leader lock

- 文件：[`internal/worker/plan_scheduler.go`](../../internal/worker/plan_scheduler.go)
- key：默认 `qs:plan-scheduler:leader`
- TTL：默认 `50s`
- 用途：多 worker 只允许一个实例推进 plan task 调度
- Redis 不可用时：
  - scheduler 直接不启动

## collection-server：当前边界

[`internal/collection-server/server.go`](../../internal/collection-server/server.go) 当前不初始化 Redis client。  
也就是说，`collection-server` 只保留 `redis` 配置面，**但运行时不再使用 Redis**。

## 通用抽象层

### 1. Cache 接口

- 文件：[`internal/apiserver/infra/cache/interface.go`](../../internal/apiserver/infra/cache/interface.go)
- 抽象：
  - `Cache`
  - `TypedCache`
  - `CacheManager`
  - `CacheKeyBuilder`

### 2. 通用 Redis 封装

- 文件：[`internal/apiserver/infra/cache/redis_cache.go`](../../internal/apiserver/infra/cache/redis_cache.go)
- 提供：
  - `Get/Set/Delete/Exists`
  - `MGet/MSet`
  - `DeletePattern`
  - `Ping`

注意：

- `DeletePattern` 底层还是 `SCAN + DEL`
- 当前它已经不是主推荐失效路径
- query/list 新路径优先使用 version token

### 3. 统一读穿透

- 文件：[`internal/apiserver/infra/cache/readthrough.go`](../../internal/apiserver/infra/cache/readthrough.go)
- 统一了：
  - cache get
  - source load
  - optional singleflight
  - positive cache write-back
  - negative cache write-back

### 4. 统一策略

- 文件：
  - [`internal/apiserver/infra/cachepolicy/policy.go`](../../internal/apiserver/infra/cachepolicy/policy.go)
  - [`internal/apiserver/infra/cache/catalog.go`](../../internal/apiserver/infra/cache/catalog.go)
- 能力：
  - TTL
  - negative TTL
  - compression
  - singleflight
  - jitter ratio
  - family policy + object policy merge

## 当前源码与旧文档的差异提醒

这次代码审计里，至少有两类信息需要以源码为准：

1. **对象缓存 TTL 已不是旧文档里的 24h / 12h**
- 当前生产默认：
  - `scale = 2h`
  - `questionnaire = 2h`
  - `assessment_detail = 1h`
  - `assessment_list = 10m`
  - `testee = 30m`
  - `plan = 12h`

2. **统计侧 Redis 已显著收缩**
- 当前源码里明确存在的是 `stats:query:*`
- 在 `internal/worker` 当前代码审计中，未找到 `event:processed:*`、`stats:daily:*` 的运行时写入实现
- `SyncDailyStatistics` 当前使用的是 **MySQL `GET_LOCK`**，不是 Redis lock，见 [`internal/apiserver/application/statistics/sync_service.go`](../../internal/apiserver/application/statistics/sync_service.go)

如果你后续要继续核这块，建议先把旧文档里的统计 Redis 描述当成“历史设计”，再逐项和代码比对。

## 建议的阅读顺序

1. [`internal/pkg/options/redis_options.go`](../../internal/pkg/options/redis_options.go)
2. [`configs/apiserver.prod.yaml`](../../configs/apiserver.prod.yaml)
3. [`internal/pkg/rediskey/builder.go`](../../internal/pkg/rediskey/builder.go)
4. [`internal/apiserver/infra/cache/catalog.go`](../../internal/apiserver/infra/cache/catalog.go)
5. [`internal/apiserver/server.go`](../../internal/apiserver/server.go) `resolveRedisFamilyClient`
6. [`internal/apiserver/infra/cache/readthrough.go`](../../internal/apiserver/infra/cache/readthrough.go)
7. 各对象缓存：
   - [`scale_cache.go`](../../internal/apiserver/infra/cache/scale_cache.go)
   - [`questionnaire_cache.go`](../../internal/apiserver/infra/cache/questionnaire_cache.go)
   - [`assessment_detail_cache.go`](../../internal/apiserver/infra/cache/assessment_detail_cache.go)
   - [`testee_cache.go`](../../internal/apiserver/infra/cache/testee_cache.go)
   - [`plan_cache.go`](../../internal/apiserver/infra/cache/plan_cache.go)
8. query/meta：
   - [`global_list_cache.go`](../../internal/apiserver/application/scale/global_list_cache.go)
   - [`my_assessment_list_cache.go`](../../internal/apiserver/infra/cache/my_assessment_list_cache.go)
   - [`versioned_query_cache.go`](../../internal/apiserver/infra/cache/versioned_query_cache.go)
   - [`version_token_store.go`](../../internal/apiserver/infra/cache/version_token_store.go)
   - [`hotset.go`](../../internal/apiserver/infra/cache/hotset.go)
9. worker 锁：
   - [`internal/pkg/redislock/lock.go`](../../internal/pkg/redislock/lock.go)
   - [`internal/worker/handlers/answersheet_handler.go`](../../internal/worker/handlers/answersheet_handler.go)
   - [`internal/worker/plan_scheduler.go`](../../internal/worker/plan_scheduler.go)

这条顺序基本就是从“配置 -> key -> 路由 -> 通用框架 -> 具体缓存 -> 锁”的完整 Redis 心智模型。
