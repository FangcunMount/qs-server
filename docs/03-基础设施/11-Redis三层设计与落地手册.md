# Redis 三层设计与落地手册

## 这份文档回答什么
这份文档重新整理 `qs-server` 中 Redis 的三层设计：

- Redis Cache 层
- 分布式锁层
- 缓存治理层

同时回答四类工程问题：

1. 这三层分别应该有哪些稳定的领域对象、领域服务、应用服务和设计模式。
2. `qs-server` 当前实现已经做到什么程度，还缺什么。
3. 新建一个业务缓存，应该如何从 0 到 1 落地，包括 TTL 抖动、缓存雪崩防护、创建与失效流程。
4. 新建一个分布式锁，应该如何从 0 到 1 落地，以及在项目里怎样接入。

本文默认前提：

- `component-base/pkg/redis` 仍然是唯一 Redis Foundation。
- `internal/pkg/redisplane` 是唯一 Redis runtime 路由来源。
- `internal/pkg/cacheobservability` 是唯一 family 级治理状态与指标来源。
- `internal/apiserver/infra/cachepolicy` 是唯一对象级缓存策略来源。

## 30 秒结论
如果只看结论，先记住下面这张表：

| 层次 | 核心问题 | 稳定对象 | 典型模式 | 当前结论 |
| ---- | -------- | -------- | -------- | -------- |
| Cache 层 | 如何把 Redis 当读侧缓存安全地用起来 | `CachePolicy`、`CacheEntry`、`VersionToken`、`WarmupTarget` | decorator、read-through、versioned key、singleflight、local hot cache | apiserver 已经基本成形，但还缺“新增缓存的标准作业流”和更统一的手工预热入口 |
| 分布式锁层 | 如何把 Redis 当租约锁安全地用起来 | `LockIdentity`、`Lease`、`LockSpec` | lease lock、ownership token、leader election、幂等闸门 | 已有 `redislock.Manager`，但还偏薄，缺少续租、锁规格注册、治理可视化 |
| 治理层 | 如何统一做 family 路由、状态、预热、降级、后台治理 | `Family`、`Handle`、`RuntimeSnapshot`、`WarmupRun` | registry、runtime catalog、orchestrator、snapshot | family 路由与状态已经统一，但“运行时人工预热”和“锁治理”还没有真正平台化 |

更具体地说：

- **Cache 层** 现在最像一个完整子系统，尤其是 `apiserver`。
- **Lock 层** 现在可用，但还没有形成“标准化锁建模手册”。
- **治理层** 现在已经统一了 family 状态和 readiness，但“后台驱动的治理动作”还不够系统化。

## 一、先把边界讲清楚

### 1.1 Redis 基础层不是这次讨论重点，但必须作为前提
Redis 的连接、profile、namespace、key builder、基础命令，不应该被 Cache / Lock / Governance 三层反复实现。这个底座已经由下面几处承担：

- `component-base/pkg/redis`
- `component-base/pkg/database`
- `internal/pkg/rediskey`
- `internal/pkg/redisplane`

因此，本文不再讨论“如何连 Redis”，而是讨论“Redis 之上怎么形成三层稳定模型”。

### 1.2 三层不是三个包，而是三类稳定职责
这三层真正的边界不是目录名，而是职责边界：

- **Cache 层**：把 Redis 当缓存。
- **Lock 层**：把 Redis 当租约锁。
- **Governance 层**：统一管理 family 路由、状态、预热、观测和后台治理动作。

它们依赖同一个 Foundation，但不应该互相吞掉职责。

## 二、Cache 层应该如何设计

### 2.1 Cache 层要解决的不是“能不能读写 Redis”，而是“如何安全命中”
Cache 层的核心问题始终只有四个：

1. 这个对象是否值得缓存。
2. 命中失败时如何回源。
3. 写后和删后如何失效。
4. 发生抖动、热点或批量过期时如何避免把数据库打穿。

因此，Cache 层的设计中心不应该是“Redis client”，而应该是“缓存对象生命周期”。

### 2.2 Cache 层的领域对象
这里的“领域对象”指 Redis 技术子域里的稳定对象，不是业务领域对象。

#### 值对象
| 对象 | 说明 |
| ---- | ---- |
| `CachePolicy` | TTL、NegativeTTL、Compression、Singleflight、JitterRatio 的组合 |
| `CacheKey` | 已经完成 namespace 与业务维度拼装后的最终缓存键 |
| `CachePolicyKey` | 对象级缓存策略键，例如 `scale`、`questionnaire`、`assessment_detail` |
| `WarmupTarget` | 一个可治理、可排序、可执行的预热目标 |

#### 实体
| 对象 | 说明 |
| ---- | ---- |
| `CacheEntry` | 某个具体缓存记录，包含 key、payload、TTL、写入时间 |
| `VersionToken` | 某个 query/list 缓存的版本游标 |
| `HotsetItem` | 一个带 score 的热点目标 |
| `WarmupRun` | 一次预热执行的结果快照 |

#### 聚合
| 聚合 | 说明 |
| ---- | ---- |
| `ObjectCacheAggregate` | 一个具体对象缓存的完整边界，包括 key、policy、load、invalidate 规则 |
| `QueryCacheAggregate` | 一个 query/list 缓存的完整边界，包括 version token 与结果键 |
| `WarmupAggregate` | 一次 warmup 编排要执行的 target 集合及其结果 |

### 2.3 Cache 层的领域服务
| 服务 | 职责 |
| ---- | ---- |
| `PolicyCatalog` | 对象级策略选择与 family 归属映射 |
| `ReadThroughService` | 统一处理命中、回源、回填、singleflight、降级 |
| `VersionTokenService` | 维护 query/list version token |
| `HotsetRecorder` | 记录运行时热点 |
| `WarmupOrchestrator` | 执行 startup/publish/repair/statistics-sync 触发的预热动作 |

### 2.4 Cache 层的应用服务
Cache 层不直接暴露给终端用户，它通过业务应用服务被消费。当前比较典型的应用服务有：

- `questionnaire.QueryService`
- `scale.QueryService`
- `statistics.SystemStatisticsService`
- `statistics.QuestionnaireStatisticsService`
- `statistics.PlanStatisticsService`

这些应用服务做两件事：

1. 面向业务提供读接口。
2. 在合适的位置记录 hotset 或触发 query warmup。

也就是说，**业务应用服务负责“什么时候需要缓存”，Cache 层负责“缓存如何安全运作”**。

### 2.5 Cache 层适合采用的设计模式
#### 1. Decorator
对象仓储缓存最适合 decorator。当前 `scale/questionnaire/assessment_detail/testee/plan` 都是这种思路。

它的优点是：

- 不改业务仓储接口。
- 缓存与真实仓储职责清晰。
- 易于做 A/B 开关与回退。

#### 2. Read-Through
缓存命中失败时由统一逻辑回源，是比“每个仓储自己写一遍 get/set/fallback”更稳定的方式。

#### 3. Versioned Key
列表和统计查询不适合走 `DeletePattern`，更适合走：

- `version token`
- `versioned result key`

这样可以避免批量删除和高基数模式扫描。

#### 4. Singleflight
热点 miss 时单实例内合并回源，避免同一个 key 瞬时重复打库。

#### 5. Local Hot Cache
对于高频短周期热点结果，可以在 Redis 之前加很薄的一层本地热点缓存，减轻 Redis 压力。

### 2.6 `qs-server` 当前 Cache 层做到什么程度
当前 `apiserver` 的 Cache 层已经比较完整，优点是：

1. 有明确的对象级策略 `cachepolicy.CachePolicy`。
2. 有统一的 `ReadThrough`。
3. query/list 缓存已经走 `VersionTokenStore + VersionedQueryCache`。
4. 有 local hot cache。
5. 有运行时热榜 `Hotset`。
6. 预热已经能覆盖：
   - 启动期静态预热
   - 启动期 query 预热
   - 问卷/量表发布后的静态预热
   - statistics sync 后的 query 预热
   - repair complete 后的 query 回补

### 2.7 当前 Cache 层的不足
虽然骨架已经成形，但还不够“平台化”：

#### 不足 1：新建缓存的标准作业流还没有沉淀成手册
现在代码层已经能支持新增缓存，但没有一份明确的 0 到 1 清单。结果是后来者仍然容易：

- 直接在业务服务里拼 key
- 忘记接 policy
- 忘记接 family
- 忘记处理 negative cache、singleflight、invalidate

#### 不足 2：对象缓存与 query 缓存的接入方法还没有模板化
当前经验散落在多个文件里，对熟悉代码的人是清楚的，对后来者则仍然需要“读很多实现才能模仿”。

#### 不足 3：手工预热入口还不够体系化
目前预热入口主要来自：

- `startup`
- `publish`
- `statistics_sync`
- `repair_complete`

这已经说明预热不是零散脚本，而是一个真实系统。但它还缺少“后台显式下发预热任务”的通用入口。

#### 不足 4：缺少“缓存是否值得建”的准入规则
当前系统更像“能建就建”，但没有形成清晰的准入问题集，例如：

- 命中率是否足够高？
- 是否有稳定 key？
- 失效是否可控？
- 是否需要 negative cache？
- 是否需要 version token？

这导致新增缓存容易凭直觉，不容易被架构约束。

## 三、分布式锁层应该如何设计

### 3.1 分布式锁层要解决的核心问题
Lock 层不是“缓存的一个特例”，而是另一类完全不同的资源语义。

它要解决的是：

- 某个关键区是否只能有一个实例执行。
- 某个任务是否需要 leader。
- 某个提交是否要防重复处理。
- 某个租约是否仍由当前持有者拥有。

因此，Lock 层的中心应该是：

- `Identity`
- `Lease`
- `Ownership`
- `TTL`

而不是缓存的 hit/miss/negative/warmup 语义。

### 3.2 Lock 层的领域对象
#### 值对象
| 对象 | 说明 |
| ---- | ---- |
| `LockIdentity` | 锁用途和业务维度组合，例如 `answersheet_processing + assessment_id` |
| `LeaseToken` | Redis 返回的 ownership token |
| `LockTTL` | 锁租约 TTL |
| `LockName` | 监控与治理视角下的锁名称 |

#### 实体
| 对象 | 说明 |
| ---- | ---- |
| `Lease` | 一次成功获取的租约 |
| `LockAttempt` | 一次加锁尝试，结果可能是成功、争用或错误 |

#### 聚合
| 聚合 | 说明 |
| ---- | ---- |
| `LockResource` | 某一类锁的完整定义，包括 identity、ttl、冲突行为、降级策略 |
| `LeaderLease` | 一类专门用于 leader election 的锁资源 |

### 3.3 Lock 层的领域服务
| 服务 | 职责 |
| ---- | ---- |
| `LockManager` | 统一执行 acquire/release，并输出锁级观测 |
| `LeaderElectionService` | 用于计划调度、后台任务选主 |
| `IdempotencyGuardService` | 用于请求去重和提交保护 |
| `LeaseOwnershipService` | 用于校验 release 时 token ownership |

### 3.4 Lock 层适合采用的设计模式
#### 1. Lease Lock
当前使用 Redis TTL + token ownership 的租约锁是对的，因为它满足了：

- 自动过期
- 可跨实例
- 释放时能校验 token

#### 2. Guard Pattern
对于“提交一次只能处理一次”的问题，最适合把锁放在 guard 前面，而不是把幂等判断写进业务主流程深处。

#### 3. Leader Election
对于 scheduler、batch worker 这类场景，应该把“谁能执行”建模成 leader lease，而不是散落在 cron / goroutine 逻辑里。

### 3.5 `qs-server` 当前 Lock 层做到什么程度
当前已经有：

- `internal/pkg/redislock.Manager`
- `worker` 的 answersheet processing gate
- `worker` 的 plan scheduler leader election
- `collection-server` 的 submit guard
- `apiserver` 的 statistics sync 锁

说明锁已经不是“零散工具函数”，而是有共享入口了。

### 3.6 当前 Lock 层的不足
#### 不足 1：`Manager` 还偏薄
当前 `redislock.Manager` 只提供 `Acquire/Release`，还没有形成完整的锁规格建模。

建议未来补出 `LockSpec`：

- 默认 TTL
- identity 规则
- contention 语义
- 出错时是 fail-close 还是 fail-open
- 是否允许降级跳过

#### 不足 2：还残留兼容 helper
当前还保留了 `AcquireClient/ReleaseClient` 兼容函数。这说明“业务代码不要直接碰原始 Redis lock primitive”这条规则还没有彻底收死。

#### 不足 3：没有续租模型
现在锁全靠 TTL 自然失效，对于短任务足够，但对于未来更长的后台任务，缺少：

- `Renew`
- `KeepAlive`
- 超时观测

#### 不足 4：没有 fencing token
如果将来某些锁保护的是外部副作用或者长链路写操作，仅靠普通 lease token 还不够，可能需要 fencing token。现在项目里还没到这一步，但这是后续要提前思考的演进方向。

#### 不足 5：治理可视化不足
现在锁有 metrics，但没有像 cache governance 那样，形成“可查询当前 lock family 状态、失败趋势、关键锁名”的操作视图。

## 四、治理层应该如何设计

### 4.1 治理层要解决的核心问题
治理层不是 Redis 命令层，也不是缓存实现层。

它关心的是：

- family 如何路由到哪个 profile
- 当前 family 是否可用、是否降级
- readiness 如何判断
- warmup 如何编排
- hotset 如何被读取和消费
- 后台运营如何发起治理动作

因此，治理层的中心对象不是“缓存值”，而是：

- `Family`
- `Handle`
- `Snapshot`
- `WarmupRun`
- `GovernanceCommand`

### 4.2 治理层的领域对象
#### 值对象
| 对象 | 说明 |
| ---- | ---- |
| `Family` | 逻辑 Redis workload，例如 `static_meta`、`query_result`、`lock_lease` |
| `Route` | family 对应的 profile、namespace suffix、fallback 策略 |
| `RuntimeSummary` | 当前组件 family 的健康汇总 |
| `GovernanceCommand` | 一次治理动作的输入，例如“预热这些 target” |

#### 实体
| 对象 | 说明 |
| ---- | ---- |
| `Handle` | family 在本进程解析后的运行时视图 |
| `FamilyStatus` | 当前 family 的治理状态 |
| `WarmupRun` | 一次 warmup 执行记录 |

#### 聚合
| 聚合 | 说明 |
| ---- | ---- |
| `RedisRuntime` | 当前进程对全部 family 的运行时视图 |
| `GovernanceSnapshot` | 对外暴露的治理快照 |
| `WarmupPlan` | 一次预热任务的 target 边界 |

### 4.3 治理层的领域服务
| 服务 | 职责 |
| ---- | ---- |
| `RuntimeResolver` | 解析 family -> handle |
| `FamilyStatusRegistry` | 注册、更新、汇总 family 状态 |
| `WarmupCoordinator` | 编排 startup / publish / repair / manual warmup |
| `GovernanceQueryService` | 对外输出 snapshot / hotset / readiness |
| `GovernanceCommandService` | 对外接收 repair / manual warmup / refresh 指令 |

### 4.4 治理层适合采用的设计模式
#### 1. Registry
family 状态天然适合 registry 模式，当前 `FamilyStatusRegistry` 就是这个思路。

#### 2. Snapshot
治理接口对外输出不应该暴露内部对象图，而应该统一输出 snapshot。

#### 3. Orchestrator
Warmup 明显是 orchestration 问题，不是某个 repository 的附属行为。

#### 4. Command/Query Separation
治理层很适合 CQRS 式分离：

- 查询：`/governance/redis`、`/cache/governance/status`
- 命令：`repair-complete`、未来的 `manual-warmup`

### 4.5 `qs-server` 当前治理层做到什么程度
已经完成的部分：

1. `redisplane` 已经统一 family 路由。
2. `cacheobservability` 已经统一 family 状态、metrics、readiness。
3. `worker` 和 `collection-server` 已经暴露了统一治理快照。
4. `apiserver` 已经有 `cache governance status`、`hotset`、`repair-complete`。
5. `WarmupCoordinator` 已经能处理多种触发器。

### 4.6 当前治理层的不足
#### 不足 1：目前还是“预置触发器治理”，不是“通用治理命令平台”
当前 warmup 的触发器都是写死的：

- startup
- publish
- statistics_sync
- repair_complete

这说明治理已经是系统化的，但仍然偏“内建触发”，而不是“后台可发起任意 warmup 命令”。

#### 不足 2：manual warmup 没有标准命令接口
如果运营后台要做：

- 预热某个问卷
- 预热某个量表
- 预热某个 org 的统计查询

目前没有一个统一的 `manual-warmup` API。

#### 不足 3：锁治理没有纳入统一 command/query 面
现在 family readiness 能看到 `lock_lease` 是否可用，但“哪些锁正在频繁争用、哪些锁错误率高”还没有进入治理命令与治理页面模型。

## 五、现在回答“新建一个业务缓存如何从 0 到 1 落地”

下面给出推荐标准流程。

### 第 1 步：先判定这是不是一个合格的缓存候选
必须先回答五个问题：

1. 是否是读侧热点。
2. 是否有稳定 key。
3. 是否有可接受的陈旧窗口。
4. 是否能定义清晰的失效点。
5. 命中失败后回源是否可控。

如果这五个问题里有两个以上答不上来，就不应该先上缓存。

### 第 2 步：判断它属于哪一类缓存
常见只分三类：

1. **静态/半静态对象缓存**
   例如量表、问卷。
2. **对象视图缓存**
   例如 testee、assessment detail、plan info。
3. **query/list 缓存**
   例如统计查询、用户列表、我的测评列表。

这一步会决定它属于哪个 `redisplane.Family`。

### 第 3 步：在 `cachepolicy` 中增加对象策略键
需要做两件事：

1. 新增 `CachePolicyKey`
2. 在 `FamilyFor(key)` 中声明它属于哪个 family

这是新增缓存的第一条硬约束：**对象策略和 family 归属必须先注册，再写实现。**

### 第 4 步：定义默认策略
需要至少明确：

- TTL
- NegativeTTL
- 是否允许 NegativeCache
- 是否允许 Compression
- 是否允许 Singleflight
- JitterRatio

如果这些值拿不准，说明缓存设计还不完整。

### 第 5 步：确定缓存形态
#### 对象缓存
优先采用：

- repository decorator
- read-through

#### query/list 缓存
优先采用：

- version token
- versioned result key
- optional local hot cache

不要优先使用：

- `DeletePattern`
- 直接 scan 全删
- 把业务过滤条件硬拼成一个不可控大 key 集合

### 第 6 步：接入 `redisplane.Handle`
从 container 中拿到该 family 的：

- `Client`
- `Builder`

业务缓存实现自身不要再解析 profile、namespace suffix。

### 第 7 步：实现命中、回源、回填
推荐固定结构：

1. 先构 key
2. `Get`
3. miss 时走 `Load`
4. 成功后异步或同步 `Set`
5. 所有回填与失效路径都走同一策略对象

### 第 8 步：设计失效流程
这是最重要的步骤之一。

#### 对象缓存
优先使用：

- 写后删
- 发布后预热
- 保存后失效

#### query/list 缓存
优先使用：

- bump version token
- 旧 key 依赖 TTL 自然淘汰

### 第 9 步：处理 TTL 抖动和雪崩
#### TTL 抖动
必须在策略里配置 `JitterRatio`，避免同类 key 同时过期。

推荐原则：

- 静态缓存：抖动可小一些
- 对象视图缓存：中等抖动
- query/list 缓存：抖动更重要

#### 缓存雪崩
至少做三件事：

1. `JitterTTL`
2. `Singleflight`
3. 对最热点 query 增加 local hot cache

如果某个缓存的热点程度很高，但这三件事一个都没做，这个缓存就不应上线。

### 第 10 步：考虑 Negative Cache
不是所有对象都要 negative cache，但要明确判断：

- “不存在”是否也是热点查询结果
- 不存在状态是否稳定
- 是否会出现刚创建立即查询的时序问题

如果“不存在”是高频而稳定的，就应该给 `NegativeTTL`。

### 第 11 步：接入治理
至少要接这三项：

1. `cachepolicy` 注册
2. `cacheobservability` 埋点
3. 如有必要，接入 `hotset` 或 warmup

### 第 12 步：补测试
至少要有：

- key 正确性
- read-through 命中/回源
- invalidate 正确性
- version token 行为
- TTL/negative/singleflight 行为

## 六、创建与删除流程怎么设计

### 6.1 对象缓存
#### 创建流程
1. 回源拿到对象
2. 按 `CachePolicy` 序列化/压缩
3. `Set(key, payload, jitteredTTL)`

#### 删除流程
1. 业务写操作成功
2. 删除对象 key
3. 若是发布类对象，可额外触发 warmup

### 6.2 query/list 缓存
#### 创建流程
1. 读取 version token
2. 构建 versioned result key
3. miss 时回源
4. set versioned result key

#### 删除流程
1. bump version token
2. 旧 key 依赖 TTL 过期

这就是为什么 `DeletePattern` 不应该再作为主路径能力出现。

## 七、缓存预热现在做到什么程度

### 7.1 当前实现已经不是“零散预热”
现在的预热已经具备体系化特征，主要体现在 `WarmupCoordinator`：

- `startup` 预热
- `publish` 预热
- `statistics_sync` 后置预热
- `repair_complete` 回补预热
- `hotset` 热点选择

所以结论不是“预热还没成体系”，而是：

**预热已经有体系，但还没成为一套完整的运行时治理命令系统。**

### 7.2 当前能不能通过 operating 后台添加问卷、量表预热
严格说，**还不能以一个通用治理命令的方式做到**。

现在已有的最接近能力是：

- 问卷发布后自动触发 `HandleQuestionnairePublished`
- 量表发布后自动触发 `HandleScalePublished`
- repair complete 可带 org/questionnaire/plan 维度做 query 回补

但缺少一个通用的后台接口，例如：

- `POST /internal/v1/cache/governance/warmup-targets`

请求体形如：

```json
{
  "trigger": "manual",
  "targets": [
    {"kind": "static.questionnaire", "scope": "questionnaire:Q-001"},
    {"kind": "static.scale", "scope": "scale:S-001"},
    {"kind": "query.stats_system", "scope": "org:1"}
  ]
}
```

### 7.3 我对运行时人工预热的建议
建议新增一个统一的治理命令入口，而不是继续增加特例接口。

#### 建议新增
1. `GovernanceCommandService`
2. `ManualWarmupRequest`
3. `ManualWarmupValidator`
4. `ManualWarmupHandler`

#### 建议接口
- `POST /internal/v1/cache/governance/warmup-targets`
- `GET /internal/v1/cache/governance/warmup-runs`

#### 为什么这样设计
因为运行时人工预热本质上是治理命令，不是业务接口，不应该塞进问卷或量表业务 API。

## 八、如何从 0 到 1 新建一个分布式锁

### 第 1 步：先定义锁保护的是什么
锁不是为了“保险起见加一下”，而是为了保护一个一致性边界。

先明确：

- 保护的是哪段关键区。
- 锁粒度是全局、org、plan、assessment、submit 还是 entry。
- 发生争用时，是跳过、重试还是报错。

### 第 2 步：定义 `LockIdentity`
最少要有：

- `Name`
- `Key`

建议规则：

- `Name` 表达锁用途
- `Key` 表达业务维度

例如：

- `plan_scheduler_leader + org:1`
- `answersheet_processing + assessment:123`
- `collection_submit + submit:abc`

### 第 3 步：定义锁 TTL
TTL 不能靠拍脑袋。

必须考虑：

- 正常执行耗时
- 最坏重试时间
- 提前过期导致并发重入的风险
- TTL 太长导致故障恢复慢的问题

经验上：

- 短临界区：TTL 短一些
- leader 锁：TTL 要覆盖一个调度周期，但又不能长到阻碍故障切换

### 第 4 步：定义冲突语义
争用失败后，业务该怎么处理必须写清楚：

- `skip`
- `retry`
- `fail`

如果不写清楚，锁就会变成隐藏控制流。

### 第 5 步：通过 `redislock.Manager` 接入
业务代码不要直接拿 Redis client 自己写 `SET NX EX`。

正确做法是：

1. 从 container 注入 lock manager
2. 构建 `Identity`
3. `Acquire`
4. 成功后进入关键区
5. `defer Release`

### 第 6 步：设计失败与降级策略
必须提前决定：

- Redis lock 不可用时，是 fail-open 还是 fail-close
- contention 是否记为正常业务分支
- acquire error 是否中断主流程

不同业务场景答案不同：

- scheduler leader：通常 fail-close 更合理
- 某些幂等保护：有时 fail-close
- 某些非关键后台任务：可以考虑 fail-open 或 skip

### 第 7 步：接入观测
至少需要：

- acquire 成功
- acquire contention
- acquire error
- release 成功
- release error

并且锁名要稳定，否则监控不可用。

### 第 8 步：补测试
至少覆盖：

- 正常 acquire/release
- contention
- TTL expiry
- wrong token release
- Redis 不可用时的行为

## 九、我对“锁从 0 到 1”在项目内的进一步建议

### 9.1 增加 `LockSpec`
建议不要每个调用点都自己决定 lock ttl 和 identity 规则，而是逐步补成：

```go
type LockSpec struct {
    Name        string
    DefaultTTL  time.Duration
    FailMode    string
    Description string
}
```

然后由业务代码只传业务维度，由锁层补齐统一规格。

### 9.2 增加 `WithLease`
可以加一个 helper：

```go
func (m *Manager) WithLease(ctx context.Context, identity Identity, ttl time.Duration, fn func(context.Context) error) (acquired bool, err error)
```

这样可以减少业务侧重复写：

- acquire
- defer release
- 错误埋点

### 9.3 未来需要考虑续租
如果未来出现超过 TTL 的后台长任务，`Manager` 需要扩展：

- `Renew`
- `KeepAlive`

否则锁模型只适用于短临界区。

## 十、对 `qs-server` 当前实现的总评价

### 10.1 已经做对的事
1. 已经把 Redis family runtime 路由收口到 `redisplane`。
2. 已经把 family status / readiness 收口到 `cacheobservability`。
3. 已经把 apiserver 的对象级缓存策略收口到 `cachepolicy`。
4. 已经把 `infra/cache` 瘦身为实现层。
5. 已经把锁从零散 helper 提升为共享 `redislock.Manager`。
6. 已经把 warmup/hotset 做成真实的治理编排，而不是散落脚本。

### 10.2 还不够好的地方
1. 缺少“新增缓存/新增锁”的标准作业手册。
2. 缺少手工治理命令平台，特别是 manual warmup。
3. 锁层还没有 `LockSpec`、`WithLease`、续租和更完整的治理快照。
4. cache governance 还偏 warmup 导向，没有形成完整的运行时命令系统。
5. 目前的设计更多是“代码已经具备能力”，还没有完全升级成“团队内可复制的方法论”。

## 十一、建议的下一步演进顺序

建议按下面顺序继续推进：

1. **先补文档与模板**
   把“新增缓存”和“新增锁”的标准流程做成模板。
2. **再补 manual warmup 命令接口**
   让 operating 后台可以显式发起问卷、量表、统计 query 的预热。
3. **再补 LockSpec**
   把锁从“能用”升级成“可治理、可复用”。
4. **最后再考虑续租与更强锁语义**
   只有当出现长任务或更强一致性需求时，再引入续租或 fencing token。

## 十二、一句话收尾
`qs-server` 的 Redis 体系现在已经从“分散使用 Redis”走到了“具备平台雏形”，但还差最后一步：把已经写在代码里的能力，整理成团队可以稳定复制的三层设计与落地手册。
