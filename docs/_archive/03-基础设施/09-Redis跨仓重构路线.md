# 【历史路线稿】Redis 跨仓重构路线

> **状态说明**
> 本文保留为跨仓重构的历史路线稿，描述当时对 `component-base`、`qs-server`、`iam-contracts` 的演进判断，**不再作为现行真值层入口**。
> 现行阅读入口已收口到 [06-Redis使用情况.md](../../03-基础设施/06-Redis使用情况.md) 与 [11-Redis三层设计与落地手册.md](../../03-基础设施/11-Redis三层设计与落地手册.md)。
> 本文中的跨仓绝对路径和代码锚点按历史阶段保留，**不保证继续可跳转**。

**本文回答**：带着 [08-Redis分层重构设计](./08-Redis分层重构设计.md) 重新回看 `qs-server`、`component-base`、`iam-contracts` 三个仓库之后，哪些 Redis 能力应该上移到基础库，哪些应该留在业务仓，哪些应该按共同模式重构但不强行抽共库。

## 30 秒结论

先给结论，不展开：

| 仓库 | 当前角色 | 应该承担什么 | 不应该承担什么 |
| ---- | -------- | ------------ | -------------- |
| `component-base` | 低层 Redis 原语和连接管理 | **Foundation**：连接、profile、keyspace、lease、scan/delete、consume、jitter 等通用原语 | 业务缓存、warmup、hotset、query cache 治理 |
| `qs-server` | Redis 使用最复杂的业务仓 | **Cache 平台 + Governance**：对象缓存、query cache、hotset、warmup、status、family route；另有本仓 lock 平台适配 | 把业务缓存平台整体上移到 `component-base` |
| `iam-contracts` | Redis 业务适配器集合 | **按共同模式重构**：token store、OTP、微信 access token/cache；采用统一 keyspace/lock/adapter 模型 | 复制 `qs-server` 的缓存治理体系 |

一句话说清：  
**`component-base` 负责通用 Redis 原语，`qs-server` 负责缓存平台，`iam-contracts` 负责自己的业务适配器；三者共享模式，不共享所有代码。**

---

## 1. 三仓现状审计结论

## 1.1 `component-base`

当前已经具备一套比较清晰的低层 Redis 原语：

- 连接与配置：
  - [`pkg/database/redis.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/component-base/pkg/database/redis.go)
  - [`pkg/database/redis_profile_registry.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/component-base/pkg/database/redis_profile_registry.go)
- keyspace / namespace：
  - [`pkg/redis/keyspace.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/component-base/pkg/redis/keyspace.go)
- 删除与扫描：
  - [`pkg/redis/delete.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/component-base/pkg/redis/delete.go)
- 抖动：
  - [`pkg/redis/jitter.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/component-base/pkg/redis/jitter.go)
- lease lock：
  - [`pkg/redis/lock.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/component-base/pkg/redis/lock.go)
- 单 key 原子消费：
  - [`pkg/redis/consume.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/component-base/pkg/redis/consume.go)

这说明 `component-base` 已经站在 **Redis Foundation** 的位置上了。

### 当前不足

虽然方向对了，但还缺几项更稳定的平台抽象：

1. **缺少显式 runtime facade**  
   现在有 `RedisConnection`、`NamedRedisRegistry`，但没有更清晰的 `Resolver / Runtime / Binding` 模型。
2. **lease 原语偏函数式，缺少 richer model**  
   只有 `AcquireLease` / `ReleaseLease`，没有 `Lease` / `LockAttempt` / `RenewLease` 这类更完整的锁模型。
3. **缺少通用 typed redis adapter 原语**  
   目前只有底层 `Get/Set/Del` 级别的 client，没有轻量的 `JSONValueStore`、`BinaryStore` 这类基础适配器。
4. **缺少资源级 observability 语义**  
   有命令级 hook 生态，但没有 `resource/profile/capability` 这类更高层统一观测标签。

## 1.2 `qs-server`

`qs-server` 是三仓里 Redis 使用最复杂的一个：

- 连接/profile/key builder：
  - [`internal/pkg/options/redis_options.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/pkg/options/redis_options.go)
  - [`internal/pkg/rediskey/builder.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/pkg/rediskey/builder.go)
- cache runtime：
  - [`internal/apiserver/infra/cache/`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/infra/cache/)
  - [`internal/apiserver/infra/statistics/cache.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/infra/statistics/cache.go)
- cache governance：
  - [`internal/apiserver/application/cachegovernance/`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/application/cachegovernance/)
  - [`internal/apiserver/infra/cache/catalog.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/infra/cache/catalog.go)
  - [`internal/apiserver/infra/cache/hotset.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/infra/cache/hotset.go)
- lock：
  - [`internal/pkg/redislock/lock.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/pkg/redislock/lock.go)
  - worker 使用点：`answersheet_handler`、`plan_scheduler`

### 当前不足

1. **Foundation 与本仓装配没有彻底剥开**  
   `redis_options`、`rediskey`、family 解析、degraded route 分散在多个位置。
2. **cache runtime 与 cache governance 仍然相互缠绕**  
   `infra/cache` 里同时存在 runtime、policy、meta、governance 入口。
3. **`redislock` 是非常薄的 wrapper**  
   目前只是对 `component-base/pkg/redis/lock.go` 的再包一层，没有形成真正的本仓 lock 平台。
4. **本仓已经有 cache 平台雏形，但还未彻底目录化**  
   `ReadThrough`、`VersionedQueryCache`、`WarmupTarget`、`Coordinator` 都已经存在，但结构还不够稳定。

## 1.3 `iam-contracts`

`iam-contracts` 不是“没有 Redis”，而是已经形成了一组**业务适配器**：

- token store：
  - [`internal/apiserver/infra/redis/token-store.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/iam-contracts/internal/apiserver/infra/redis/token-store.go)
- OTP verifier / code store / send gate：
  - [`internal/apiserver/infra/redis/otp_verifier.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/iam-contracts/internal/apiserver/infra/redis/otp_verifier.go)
- 微信 app access token cache：
  - [`internal/apiserver/infra/redis/accesstoken_cache.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/iam-contracts/internal/apiserver/infra/redis/accesstoken_cache.go)
- wechat SDK cache adapter：
  - [`internal/apiserver/infra/redis/wechatsdk_cache.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/iam-contracts/internal/apiserver/infra/redis/wechatsdk_cache.go)
- Redis 初始化与 hook：
  - [`internal/apiserver/database.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/iam-contracts/internal/apiserver/database.go)

### 当前不足

1. **更像 adapter 集合，不是成体系的平台层**  
   各 adapter 自己拼 key、自己做 marshal/unmarshal、自己处理日志。
2. **没有统一 key builder / keyspace 入口**  
   大量 key 是硬编码字符串，如 `refresh_token:*`、`otp:*`、`idp:wechat:token:*`。
3. **锁能力只在单个 adapter 内部使用**  
   `TryLockRefresh` 直接调用 `AcquireLease`，但没有统一 lock 语义层。
4. **不需要复制 `qs-server` 的 cache governance**  
   IAM 当前 Redis 用法更接近 session/token/OTP/store，不是对象缓存平台。

---

## 2. 跨仓重构的基本原则

## 2.1 原则 1：只把“第二个仓库也会需要”的能力上移

应该上移到 `component-base` 的，必须满足：

- 与业务对象无关
- 至少两个仓库都会用到
- 上移后不会把基础库绑死在某个业务模型上

### 适合上移

- keyspace / namespace
- profile registry / runtime binding
- lease lock primitives
- atomic consume primitives
- scan/delete helpers
- TTL jitter
- 轻量 typed Redis store 基础抽象

### 不适合上移

- `CacheFamily`
- `WarmupTarget`
- `HotsetRecorder`
- `VersionedQueryCache`
- `CacheGovernance Coordinator`

这些都是 `qs-server` 特有的缓存平台设计，不应塞进基础库。

## 2.2 原则 2：共享“模式”，不强迫共享“整层代码”

`qs-server` 和 `iam-contracts` 可以共享：

- keyspace 模式
- lock 模式
- adapter 结构模式
- 统一错误与观测模式

但不需要共享：

- 同一个 cache 平台实现
- 同一个 warmup/governance 目录
- 同一个 query cache 体系

## 2.3 原则 3：`component-base` 保持“低层稳定”，业务仓保持“高层清晰”

重构目标不是把所有 Redis 代码都上移，而是形成下面的稳定关系：

- `component-base`：低层稳定原语
- `qs-server`：高层缓存平台
- `iam-contracts`：高层业务适配器

---

## 3. `component-base` 该如何重构

## 3.1 目标定位

`component-base` 应正式承担 **Redis Foundation**，并明确自己不做业务缓存平台。

## 3.2 具体重构建议

### A. 把 `NamedRedisRegistry` 提升成更明确的 runtime 模型

当前：

- `RedisConnection`
- `NamedRedisRegistry`

建议新增一个更稳定的 facade，例如：

```go
type RedisRuntime struct {
    Resolver RedisProfileResolver
    Default  RedisBinding
}

type RedisBinding struct {
    Name   string
    DB     int
    Client redis.UniversalClient
    State  RedisProfileState
}
```

#### 这样做的价值

- `qs-server` 和 `iam-contracts` 都能基于同一 runtime/binding 概念做本仓装配
- `ProfileStatus` / `fallback` / `reconnect` 语义更容易被上层治理面消费

### B. 把 lease lock 从“函数”提升到“模型 + 原语”

当前：

- [`AcquireLease`]( /Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/component-base/pkg/redis/lock.go )
- [`ReleaseLease`]( /Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/component-base/pkg/redis/lock.go )

建议新增：

```go
type Lease struct {
    Key       string
    Token     string
    TTL       time.Duration
    AcquiredAt time.Time
}

type LeaseAttempt struct {
    Lease    *Lease
    Acquired bool
}

func TryAcquireLease(...) (*LeaseAttempt, error)
func RenewLease(...) error
func ReleaseLeaseHandle(...) error
```

#### 这样做的价值

- `qs-server` 的 worker lock、IAM 的 `TryLockRefresh` 都能复用更稳定的模型
- 冲突与错误可以被更清晰地区分

### C. 增加轻量 typed store 原语，但不做业务 cache 平台

建议新增非常克制的基础抽象，例如：

```go
type JSONValueStore[T any] interface {
    Get(ctx context.Context, key string) (*T, error)
    Set(ctx context.Context, key string, value *T, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
}
```

#### 注意边界

这里的 typed store 只解决：

- 序列化
- 统一错误包装
- nil / not found 语义

不解决：

- read-through
- negative cache
- singleflight
- warmup
- version token

### D. 明确文档：`component-base/pkg/redis` 只做基础原语

建议补文档，明确说明：

- 可以放：`Keyspace`、`Lease`、`ConsumeIfExists`、`DeleteByPattern`、`JitterTTL`
- 不放：`CacheCatalog`、`WarmupTarget`、`QueryCache`

---

## 4. `qs-server` 该如何重构

## 4.1 目标定位

`qs-server` 应成为 **Redis Cache Platform + Governance** 的宿主仓，同时保留自己的 lock 平台适配层。

## 4.2 具体重构建议

### A. 先收敛本仓 Foundation 适配层

建议新增：

```text
internal/pkg/redisruntime/
```

负责：

- 从 `options.RedisOptions` 组装 runtime
- 解析 named profile
- 为 apiserver / worker 暴露统一 resolver

当前分散入口：

- [`internal/apiserver/database.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/database.go)
- [`internal/worker/database.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/worker/database.go)
- `server.go` 里的 family resolve

这些应逐步向 `redisruntime` 收敛。

### B. 把 `infra/cache` 显式拆成 runtime / object / query / meta

建议目标结构：

```text
internal/apiserver/infra/cache/
├── runtime/
├── object/
├── query/
└── meta/
```

#### 迁移建议

- `readthrough.go`、`redis_cache.go`、`singleflight.go`、`local_hot_cache.go`
  -> `runtime/`
- `scale_cache.go`、`questionnaire_cache.go`、`assessment_detail_cache.go`、`plan_cache.go`、`testee_cache.go`
  -> `object/`
- `versioned_query_cache.go`、`my_assessment_list_cache.go`、`statistics/cache.go`、`global_list_cache.go`
  -> `query/`
- `hotset.go`、`version_token_store.go`
  -> `meta/`

### C. `cachegovernance` 保持在 application，不上移

这是 `qs-server` 的平台治理面，应继续保留在：

- [`internal/apiserver/application/cachegovernance/`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/application/cachegovernance/)

但它应该逐步从具体缓存实现中退耦，更多依赖：

- `CacheCatalog`
- `WarmupPlanner`
- `WarmupExecutor`
- `StatusService`

而不是直接知道具体 repository 细节。

### D. `redislock` 要么删薄壳，要么升级成真正的平台层

当前 [`internal/pkg/redislock/lock.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/pkg/redislock/lock.go) 只是薄包装。

两个选择里必须选一个：

#### 方案 1：删掉薄壳，直接用 `component-base/pkg/redis`

适用于：

- 只需要最小 lease 原语
- 不打算在本仓发展 lock 平台

#### 方案 2：升级成 `internal/pkg/lock/redis`

适用于：

- 想在 `qs-server` 里统一 answersheet lock / leader lock / future lock metrics
- 想形成本仓 lock 服务语义层

结合当前设计稿，我更建议**方案 2**。

### E. 删除重复 builder 包装

当前 [`internal/apiserver/infra/cache/interface.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/infra/cache/interface.go) 里的 `CacheKeyBuilder` 基本是在重复包装 [`internal/pkg/rediskey/builder.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/pkg/rediskey/builder.go)。

建议：

- 统一收敛到 `rediskey.Builder`
- `CacheKeyBuilder` 只保留确实需要的 cache 特化行为；如果没有，就删除

这是低风险重构点。

---

## 5. `iam-contracts` 该如何重构

## 5.1 目标定位

`iam-contracts` 不应复制 `qs-server` 的缓存平台，而应形成：

- 清晰的 Redis adapter 层
- 统一的 keyspace / lock / logging 模式
- 更少重复的 `Get/Set/Del/Exists + marshal/unmarshal` 样板代码

## 5.2 具体重构建议

### A. 引入本仓 key builder / keyspace 约束

当前 IAM 大量 Redis key 是硬编码字符串：

- `refresh_token:{value}`
- `token_blacklist:{id}`
- `otp:{scene}:{phone}:{code}`
- `idp:wechat:token:{appID}`

建议在 IAM 本仓新增一个很薄的 keyspace 入口，例如：

```text
internal/apiserver/infra/redis/keys.go
```

统一封装：

- `RefreshTokenKey(tokenValue)`
- `TokenBlacklistKey(tokenID)`
- `OTPKey(scene, phone, code)`
- `OTPSendGateKey(scene, phone)`
- `WechatAccessTokenKey(appID)`
- `WechatAccessTokenLockKey(appID)`

这样做的价值不是“抽象更优雅”，而是：

- key schema 统一
- 将来做 namespace / profile / key migration 更容易
- 减少 adapter 各自拼字符串

### B. 抽一个本仓 shared adapter base

当前这些适配器都在重复做：

- `client.Get/Set/Del/Exists`
- `json.Marshal/Unmarshal`
- `redis.Nil` 判定
- `redisInfo/redisError`

建议在 IAM 本仓内形成一个**只服务 IAM 的 adapter base**，例如：

```text
internal/apiserver/infra/redis/base.go
```

里面可提供：

- JSON get/set helper
- nil/not-found helper
- keyspace helper
- common logging helper

#### 注意边界

这层不应上移到 `component-base`，因为：

- 它已经开始带 IAM 的 adapter 口味
- 日志规范和 key schema 都是 IAM 仓特有的

### C. 保留业务适配器，不强行平台化成 cache governance

下面这些适配器应继续保留在 IAM 里：

- `RedisStore`
- `OTPVerifierImpl`
- `accessTokenCache`
- `WechatSDKCache`

但它们应改成：

- 依赖 shared base/helper
- 统一 key builder
- 统一 not-found / logging / lock 语义

而不是各自一套小风格。

### D. 把 access token refresh lock 抽成统一 lock service

当前 [`accesstoken_cache.go`](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/iam-contracts/internal/apiserver/infra/redis/accesstoken_cache.go) 里的 `TryLockRefresh` 已经在使用 `AcquireLease`。

建议后续引入一个本仓级的 `LockService` 或 `LeaseService`，让：

- access token refresh
- future session / authz rebuild / suggest refresh

都共享同一套 lock 语义，而不是每个 adapter 各自直接碰 `AcquireLease`。

---

## 6. 哪些能力应该共用，哪些不该共用

## 6.1 应上移到 `component-base`

- `Lease` / `LeaseAttempt` / `RenewLease`
- `RedisRuntime` / `RedisBinding` / `ProfileResolver`
- `JSONValueStore` 或同等级的极轻量 typed store 基础能力
- 更统一的 `DeleteByPattern` / `ScanKeys` 选项和统计接口

## 6.2 应保留在 `qs-server`

- `CacheFamily`
- `CachePolicyKey`
- `ReadThroughService`
- `VersionedQueryService`
- `WarmupTarget`
- `HotsetRecorder`
- `CacheGovernance Coordinator`

## 6.3 应保留在 `iam-contracts`

- refresh token store
- token blacklist store
- OTP store / send gate
- wechat access token cache
- wechat SDK cache adapter

## 6.4 只共享模式，不共享代码

- key builder 设计
- adapter base 设计
- lock service 设计
- resource registry / governance 思维

---

## 7. 推荐重构顺序

## 阶段 1：先动 `component-base`

先补低层稳定原语：

1. `RedisRuntime` / `RedisBinding`
2. richer lease model
3. 轻量 typed store

原因：

- 这是两边都能复用的底座
- 不会碰业务缓存语义
- 风险最低

## 阶段 2：再收 `qs-server` 的本仓结构

按下面顺序：

1. `internal/pkg/redisruntime`
2. `internal/pkg/lock/redis`
3. `infra/cache` 拆 runtime/object/query/meta
4. `cachegovernance` 与 runtime 退耦

## 阶段 3：最后收 `iam-contracts`

按下面顺序：

1. 统一 key builder / keyspace
2. 抽 shared adapter base
3. 统一 lock service
4. 清理重复 `Get/Set/Marshal` 样板

这个顺序的好处是：

- 先稳定基础库
- 再稳定最复杂的业务仓
- 最后用 IAM 做“轻量适配器收口”

---

## 8. 最终判断

带着设计稿回看三仓之后，最重要的判断是：

1. **`component-base` 已经是 Redis Foundation 的宿主，但还没完全长成平台底座。**
2. **`qs-server` 不该把缓存平台整体上移；它应该继续做缓存平台本体。**
3. **`iam-contracts` 不该复制 `qs-server` 的治理体系；它应该按统一模式整理业务 Redis adapter。**

所以真正合理的跨仓重构不是“抽一个大 Redis 库”，而是：

- 在 `component-base` 抽稳底座
- 在 `qs-server` 把缓存平台做清
- 在 `iam-contracts` 把 adapter 模式做齐

这才符合三仓当前真实代码形态，也最不容易在重构中把边界搞乱。

---

*建议配套阅读：先看 [08-Redis分层重构设计](./08-Redis分层重构设计.md)，再读本文；前者给出层次设计，本文给出跨仓落地路径。*
