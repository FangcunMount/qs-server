# Redis 三层设计与落地手册

**本文回答**：在 `qs-server` 当前代码基础上，Redis 的 **Cache 层、分布式锁层、治理层** 应该怎样理解、怎样继续演进，以及新建一个缓存或一把锁时，工程上应该按什么步骤落地。

## 30 秒结论

| 层次 | 解决的核心问题 | 当前实现状态 | 读者最该关心什么 |
| ---- | -------------- | ------------ | ---------------- |
| Cache 层 | 如何把 Redis 当读侧缓存安全地用起来 | `apiserver` 已经形成完整骨架 | 新增缓存时必须先定 family、policy、失效策略，再写实现 |
| 分布式锁层 | 如何把 Redis 当 lease lock 安全地用起来 | `LockSpec + Manager` 已经落地 | 业务代码不要自己硬编码锁名、TTL 和释放语义 |
| 治理层 | 如何统一 family 路由、状态、预热、后台治理动作 | runtime snapshot、manual warmup、readiness 已落地 | 预热和 family 健康不再是各模块各管各的 |

一句话：**Redis Foundation 已经沉到底层，`qs-server` 现在真正要维护的是 Cache / Lock / Governance 三层稳定职责。**

## 一、先把三层边界讲清楚

### 1. Foundation 不是这份手册的主角

本文默认这些前提已经成立：

- `component-base/pkg/database` 负责 Redis 连接与 named profile registry
- `internal/pkg/redisplane` 负责 family -> profile / namespace / fallback runtime 路由
- `internal/pkg/rediskey` 负责统一 key builder

因此，本文不再讨论“怎么连 Redis”，而是讨论“Redis 之上怎样形成稳定平台层”。

### 2. 三层不是三个目录，而是三类职责

| 层次 | 应负责什么 | 不应负责什么 |
| ---- | ---------- | ------------ |
| Cache 层 | 命中、回源、回填、失效、TTL、negative cache、singleflight、热点与预热 | 直接暴露原始 Redis client 给业务随手拼 key |
| 分布式锁层 | lease、ownership、TTL、contention、降级语义、统一 metrics | 把锁当业务真相或代替数据库约束 |
| 治理层 | family runtime、status snapshot、readiness、warmup orchestrator、后台治理命令 | 替业务决定“该不该建缓存”或直接操作业务领域对象 |

## 二、Cache 层应该怎样设计

### 1. Cache 层真正要解决的问题

Cache 层的中心从来不是“能不能 `GET/SET` Redis”，而是：

1. 这个读模型是否值得缓存。
2. 命中失败时怎样回源。
3. 写后 / 删后怎样失效。
4. 热点、抖动、批量过期时怎样避免把下游打穿。

所以 Cache 层的核心对象应是“缓存对象生命周期”，不是“一个万能 Redis wrapper”。

### 2. Cache 层的稳定对象

#### 值对象

| 对象 | 说明 |
| ---- | ---- |
| `CachePolicy` | TTL、NegativeTTL、Compression、Singleflight、JitterRatio 的组合 |
| `CachePolicyKey` | 对象级策略键，例如 `scale`、`questionnaire`、`assessment_detail` |
| `WarmupTarget` | 一个可治理、可执行、可计数的预热目标 |
| `CacheKey` | 已完成 namespace 和业务维度拼装的最终 key |

#### 实体

| 对象 | 说明 |
| ---- | ---- |
| `CacheEntry` | 一条具体缓存记录 |
| `VersionToken` | query/list 缓存使用的版本游标 |
| `HotsetItem` | 一个带 score 的热点候选 |
| `WarmupRun` | 一次预热执行结果 |

#### 聚合

| 聚合 | 说明 |
| ---- | ---- |
| `ObjectCacheAggregate` | 单对象缓存的完整边界 |
| `QueryCacheAggregate` | query/list 缓存的完整边界 |
| `WarmupAggregate` | 一次 warmup 计划的 target 集合与结果 |

### 3. Cache 层适合采用的模式

#### Decorator

对象仓储缓存优先走 decorator。现在 `scale / questionnaire / assessment / testee / plan` 这条线已经是这个模式。

#### Read-Through

命中失败后的回源、回填和降级必须统一，不应让每个 repository 再写一套。

#### Versioned Key

高基数 query/list 缓存优先：

- `version token`
- `versioned result key`

不要再回到 `DeletePattern` 和扫描删键。

#### Singleflight

热点 miss 时单实例合并回源，避免瞬时打穿下游。

#### Local Hot Cache

对极高频、短 TTL 的热点 query 可以在 Redis 前加一层很薄的本地热点缓存。

### 4. `qs-server` 当前 Cache 层已经做到什么

当前已经落地的部分：

1. family runtime 统一由 `redisplane` 提供。
2. 对象级策略统一收口到 `cachepolicy`。
3. `infra/cache` 已经收缩为纯实现层，不再兼管 runtime 路由与治理。
4. query/list 缓存已经走 `VersionTokenStore + VersionedQueryCache`。
5. hotset 与 warmup 已经统一到 `cachegovernance`。
6. manual warmup 已经标准化成 `POST /internal/v1/cache/governance/warmup-targets`。

对应代码：

- [internal/apiserver/infra/cache](../../internal/apiserver/infra/cache)
- [internal/apiserver/infra/cachepolicy](../../internal/apiserver/infra/cachepolicy)
- [internal/apiserver/application/cachegovernance](../../internal/apiserver/application/cachegovernance)

### 5. 当前 Cache 层还不够好的地方

#### 不足 1：新增缓存还没有模板化得足够彻底

现在已经能新增缓存，但“标准作业流”更多还停留在经验层，而不是被显式模板化成一眼可复制的实现骨架。

#### 不足 2：缓存准入规则还不够硬

当前团队已经有隐性共识，但还没有把“什么时候不该建缓存”写成正式准入表，容易凭直觉上缓存。

#### 不足 3：热榜和手工预热仍以缓存治理为主，尚未沉成更完整的运维工作流

接口已经有了，但 operating 后台能做的动作还比较集中在 warmup，没有扩展到更完整的 Redis 治理动作集合。

## 三、新建一个业务缓存，如何从 0 到 1 落地

### 第 1 步：先判断这个缓存值不值得建

先回答下面 5 个问题：

1. 是否是高频读。
2. 是否存在稳定 key。
3. 是否允许一定时间的陈旧。
4. 是否存在明确失效点。
5. miss 回源是否可控。

其中有两项以上答不上来，就先不要建缓存。

### 第 2 步：确定它属于哪个 family

当前 `qs-server` 常用 family 判断：

| family | 适合什么 |
| ------ | -------- |
| `static_meta` | 量表、问卷、已发布列表等静态或半静态对象 |
| `object_view` | 单对象快照 |
| `query_result` | 统计查询、列表查询、聚合结果 |
| `meta_hotset` | version token、hotset 元数据 |
| `sdk_token` | 第三方 SDK token / ticket |

### 第 3 步：在 `cachepolicy` 注册对象策略

必须先做两件事：

1. 新增 `CachePolicyKey`
2. 在 `FamilyFor` 中声明它属于哪个 family

硬约束：**没有 policy key 和 family 归属，就不应该开始写缓存实现。**

### 第 4 步：定义默认策略

最少要确定：

- TTL
- JitterRatio
- 是否允许 Negative Cache
- NegativeTTL
- 是否允许 Compression
- 是否启用 Singleflight

### 第 5 步：选择缓存形态

#### 对象缓存

优先：

- repository decorator
- read-through
- 写成功后按 key 失效

#### query/list 缓存

优先：

- version token
- versioned key
- 旧 key 依赖 TTL 自然过期

不要优先：

- `DeletePattern`
- `SCAN + DEL`
- 无版本的直接前缀清理

### 第 6 步：接入 `redisplane.Handle`

从 container/runtime 中拿 family 对应的：

- `Client`
- `Builder`

业务实现不要再自行解析：

- redis profile
- namespace suffix
- fallback 策略

### 第 7 步：实现命中、回源与回填

推荐固定结构：

1. 构造 key
2. 先读缓存
3. miss 时走 load
4. 成功后按策略回填
5. 缓存错误降级成 miss，而不是直接把主流程打断

### 第 8 步：定义失效流程

#### 对象缓存

优先：

- 写后删
- 发布后主动重建或 warmup

#### query/list 缓存

优先：

- bump version token
- 让旧 key 自然过期

### 第 9 步：处理 TTL 抖动与雪崩

#### TTL 抖动

必须在策略层配置 `JitterRatio`，避免同类 key 同时过期。

常见原则：

- 静态缓存：抖动可小
- 对象缓存：中等抖动
- query/list：抖动要更积极

#### 缓存雪崩

最少同时做三件事：

1. TTL jitter
2. singleflight
3. 对最热点 query 增加 local hot cache

### 第 10 步：考虑 Negative Cache

只给下面这类对象加：

- “不存在”本身是高频结果
- 空结果较稳定
- 误缓存空值的风险可接受

不适合 negative cache 的，就不要为了“统一”而硬上。

### 第 11 步：接入治理

新增缓存后至少要回答：

1. 它属于哪个 family。
2. 它是否允许 warmup。
3. 是否应该记录 hotset。
4. readiness / degraded 时怎样降级。

### 第 12 步：补测试

至少覆盖：

- hit / miss / error
- 写后失效
- negative cache
- version token 失效
- jitter 不为零
- 降级行为

## 四、缓存创建与删除流程怎么设计

### 1. 对象缓存

#### 创建流程

`读请求 -> miss -> 回源 -> set -> 后续命中`

#### 删除流程

`写成功 / 删除成功 -> invalidate 单 key -> 后续读 miss 再重建`

### 2. query/list 缓存

#### 创建流程

`先取 version -> 拼 versioned key -> miss 回源 -> 写 versioned key`

#### 删除流程

`业务写成功 -> bump version token -> 老 key 留给 TTL 自然淘汰`

核心原则：**query/list 的“删除”不应再等价于扫描删 Redis 键。**

## 五、当前预热体系做到什么程度

### 1. 当前已经是一个真实治理系统，不是零散脚本

当前 warmup 已统一支持：

- startup
- scale publish
- questionnaire publish
- statistics sync
- repair complete
- manual warmup

这意味着预热已经具备：

- 标准 trigger
- 统一 orchestrator
- family-aware 状态输出
- internal API 接口，以及可供后台接入的稳定治理命令

### 2. 当前能不能通过 operating 后台添加问卷、量表预热

可以。就 `qs-server` 当前代码而言，后端 internal API 已经稳定提供，后台可以直接按该契约接入：

- 后端 internal API：
  `POST /internal/v1/cache/governance/warmup-targets`
- 接入约定与页面说明：
  [docs/04-接口与运维/06-operating 缓存治理页接入.md](../04-接口与运维/06-operating%20缓存治理页接入.md)

当前支持的 target kind：

- `static.scale`
- `static.questionnaire`
- `static.scale_list`
- `query.stats_system`
- `query.stats_questionnaire`
- `query.stats_plan`

### 3. 当前预热体系还缺什么

仍有几个可以继续做的点：

1. 更丰富的治理命令，而不只是 warmup。
2. 更明确的“哪些 family 允许人工干预”的策略面。
3. 更完整的 warmup 结果保留与排障视图。

## 六、分布式锁层应该怎样设计

### 1. Lock 层要解决的核心问题

Lock 层关心的是：

- 某个关键区是否只能有一个执行者
- 某个后台任务是否需要 leader
- 某个提交是否要防重复处理
- 某次 release 是否仍由锁持有者执行

它关心的是：

- identity
- lease
- ownership
- TTL

不是 cache hit/miss 语义。

### 2. Lock 层的稳定对象

#### 值对象

| 对象 | 说明 |
| ---- | ---- |
| `LockIdentity` | 锁用途与业务 key 的组合 |
| `LeaseToken` | 锁的 ownership token |
| `LockTTL` | 锁租约 TTL |
| `LockName` | 监控与治理视角下的锁名称 |
| `LockSpec` | 一类锁的稳定规格：名称、说明、默认 TTL |

#### 实体

| 对象 | 说明 |
| ---- | ---- |
| `Lease` | 一次成功获取的租约 |
| `LockAttempt` | 一次加锁尝试，可能成功、争用或出错 |

### 3. Lock 层适合采用的模式

#### Lease Lock

当前 `SETNX + EX + compare-and-del` 的租约锁模型是合理的基础。

#### Guard Pattern

对于“防重复处理”，锁应该放在业务主流程前面做 guard，而不是散落在深层业务代码里。

#### Leader Election

对于 scheduler 一类任务，应把“谁能执行”视为 leader lease 问题。

### 4. `qs-server` 当前 Lock 层已经做到什么

已经完成：

1. 统一 `redislock.Manager`
2. 统一 `LockSpec`
3. `AcquireSpec / ReleaseSpec`
4. 内建锁规格已经覆盖 worker、apiserver、collection-server 的当前主要互斥场景

当前内建规格：

- `answersheet_processing`
- `plan_scheduler_leader`
- `statistics_sync_leader`
- `statistics_sync`
- `behavior_pending_reconcile`
- `collection_submit`

### 5. 当前 Lock 层还不够好的地方

1. **没有续租**：更长任务场景仍然不够强。
2. **没有 fencing token**：若未来保护外部副作用强顺序写，需要额外模型。
3. **治理可视化还偏轻**：family readiness 已有，但锁级操作面仍偏指标化。

## 七、如何从 0 到 1 新建一把分布式锁

### 第 1 步：先定义它保护什么

先说清楚：

- 保护的是“重复处理”还是“leader 选主”
- 锁冲突时是正常跳过，还是必须报错
- Redis 不可用时是 fail-open 还是 fail-close

### 第 2 步：定义业务 key

不要让业务方直接写最终 Redis key。只定义：

- 业务 key 基底
- identity 维度

最终 Redis key 由 lock 层统一拼装。

### 第 3 步：定义 `LockSpec`

最少给出：

- `Name`
- `Description`
- `DefaultTTL`

### 第 4 步：确定冲突语义

典型有三类：

1. **contention = 正常跳过**
2. **contention = 业务失败**
3. **Redis error = 降级继续**

必须写清楚，不要把语义藏在调用点分支里。

### 第 5 步：通过 `Manager.AcquireSpec` 接入

业务代码只调用：

- `AcquireSpec`
- `ReleaseSpec`

不要：

- 自己写锁名
- 自己给默认 TTL
- 自己直接调原始 Redis 命令

### 第 6 步：定义释放与降级策略

至少明确：

- release 必须校验 token ownership
- wrong token 不能误解锁
- Redis 不可用时是否继续业务路径

### 第 7 步：接入观测

至少要能回答：

- 成功率
- contention 频率
- degraded 次数
- 关键锁名分布

### 第 8 步：补测试

至少覆盖：

- acquire / release
- contention
- TTL 生效
- wrong token release
- degrade 语义

## 八、治理层应该怎样继续演进

### 当前已经完成的部分

1. family runtime 路由统一
2. family snapshot / readiness 统一
3. 三进程 `/readyz` 与 `/governance/redis` 对齐
4. `apiserver` manual warmup 已标准化

### 仍可继续演进的方向

1. 把更多 Redis 治理动作抽成标准 command，而不只 warmup。
2. 让 lock 也进入更完整的治理 query / command 面。
3. 补更强的 lock 生命周期能力，例如续租。

## 九、对当前实现的总评价

### 已经做对的事

1. Redis runtime 路由已经从业务代码里抽走。
2. Cache、Lock、Governance 三层边界已经基本可见。
3. `collection-server` 的 Redis 使用范围已经收得比较干净。
4. 手工预热和 LockSpec 已经从“设计想法”变成了真实实现。

### 还需要继续瘦身和重构的点

1. Cache 新增模板仍可继续标准化。
2. Lock 层的 `WithLease / Renew` 还可以继续补。
3. 治理层可以继续从“预热平台”演进成“Redis 操作面平台”。

## 十、建议阅读顺序

1. 先看 [12-Redis文档中心.md](./12-Redis文档中心.md) 建立四层阅读地图。
2. 再看 [06-Redis使用情况.md](./06-Redis使用情况.md) 拿到当前运行边界。
3. 然后读本文，看三层职责与接入流程。
4. 如果要盘点缓存对象，再看 [13-Redis缓存业务清单.md](./13-Redis缓存业务清单.md)。
5. 如果要接 operating，再看 [04-接口与运维/06-operating 缓存治理页接入.md](../04-接口与运维/06-operating%20缓存治理页接入.md)。
6. 如果要看演进背景，再回读 `07-10` 和历史专题稿。

## 一句话收尾

`qs-server` 现在不再缺“Redis 能力”，真正要维护的是：**让新增缓存和新增锁都沿着同一条受约束、可治理、可观测的路径进入系统。**

---

*写作约定见 [CONTRIBUTING-DOCS.md](../CONTRIBUTING-DOCS.md)。*
