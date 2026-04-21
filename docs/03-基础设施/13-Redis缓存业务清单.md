# Redis 缓存业务清单

**本文回答**：`qs-server` 当前到底缓存了哪些业务对象、查询结果和治理元数据，它们分别属于哪个 Redis family、采用什么缓存模式、怎样失效、是否支持 warmup，以及哪些读路径当前明确**没有**进入 Redis 读缓存。

## 30 秒结论

| 主题 | 当前结论 |
| ---- | -------- |
| 完整业务缓存在哪 | 只有 `apiserver` 有完整 Redis Cache 层；`worker` 不做读缓存，`collection-server` 只做操作性 Redis |
| 当前主要业务缓存 | `scale`、`questionnaire`、`scale_list`、`assessment_detail`、`assessment_list`、`testee`、`plan`、`stats_query` |
| 关键模式 | 对象缓存优先 `decorator + read-through + 写后失效`；列表/查询缓存优先 `version token + versioned key` |
| 当前治理元数据 | `meta_hotset` 负责 `version token + hotset + warmup metadata`，它不是直接面向业务读者的领域缓存 |
| 当前支持 manual warmup 的对象 | `static.scale`、`static.questionnaire`、`static.scale_list`、`query.stats_system`、`query.stats_questionnaire`、`query.stats_plan` |
| 当前明确不做的事 | `collection-server` 不做领域读缓存；大多数列表查询不做 Redis 读缓存；不再依赖 `DeletePattern` 做 query 失效 |

## 范围说明

本文只整理 **Redis cache usage**，不覆盖：

- 分布式锁：见 [06-Redis使用情况.md](./06-Redis使用情况.md) 和 [11-Redis三层设计与落地手册.md](./11-Redis三层设计与落地手册.md)
- `collection-server` 的限流、幂等、in-flight guard：它们属于操作性 Redis，不属于领域读缓存
- `_archive` 中已经淘汰的缓存形态

## 总表：当前 Redis 缓存对象与查询

| 业务模块 / 能力 | 缓存对象 | Family | 模式 | 失效方式 | Warmup |
| ---- | ---- | ---- | ---- | ---- | ---- |
| Scale | 单量表 `scale` | `static_meta` | `repository decorator + read-through` | Create 写入；Update / Remove 删单 key | `static.scale` |
| Scale | 已发布量表列表 `scale_list` | `static_meta` | 全量重建 + Redis 列表缓存 + 本地热点缓存 | 重建覆盖；列表为空时删 key | `static.scale_list` |
| Survey / Questionnaire | 问卷主记录、已发布快照、指定版本快照 `questionnaire` | `static_meta` | `repository decorator + read-through + negative cache` | 按 code 家族删 key | `static.questionnaire` |
| Evaluation | 测评详情 `assessment_detail` | `object_view` | `repository decorator + read-through` | Save / Delete 删单 key | 暂无 |
| Evaluation | “我的测评列表” `assessment_list` | `query_result` + `meta_hotset` | `version token + versioned key + 本地热点缓存` | bump version token | 暂无直接 manual warmup |
| Actor | 受试者详情 `testee` | `object_view` | `repository decorator + read-through + negative cache` | Save / Update / Delete 删单 key | 暂无 |
| Plan | 计划详情 `plan` | `object_view` | `repository decorator + read-through` | Save 删单 key | 暂无 |
| Statistics | 统计查询结果 `stats_query` | `query_result` | query result cache | TTL 自然过期 + warmup 重建 | `query.stats_*` |
| Governance | version token / hotset | `meta_hotset` | 元数据缓存 / 热点排行 | bump / trim / TTL | 由治理层自己消费 |
| SDK | 微信 SDK token/ticket | `sdk_token` | SDK adapter cache | SDK TTL 到期 / Delete | 暂无 |

## 一、`static_meta`：静态或半静态对象缓存

### 1. `scale`：单量表缓存

代码入口：
- [internal/apiserver/infra/cache/scale_cache.go](../../internal/apiserver/infra/cache/scale_cache.go)
- [internal/apiserver/container/assembler/scale.go](../../internal/apiserver/container/assembler/scale.go)

当前行为：

- 缓存对象：按 `code` 读取的量表详情
- 模式：`repository decorator + read-through`
- 回源：`repo.FindByCode`
- 回填：命中 miss 后异步回填
- 失效：`Update / Remove` 删除单 key
- 例外：`FindByQuestionnaireCode`、`FindSummaryList` 明确不走 Redis 读缓存

适合理解为：

- 这是**静态读模型缓存**
- 不是量表列表缓存
- 不是按多条件筛选的查询缓存

### 2. `scale_list`：已发布量表列表缓存

代码入口：
- [internal/apiserver/application/scale/global_list_cache.go](../../internal/apiserver/application/scale/global_list_cache.go)

当前行为：

- 缓存对象：已发布量表的全局摘要列表
- 模式：**全量重建**后写入 Redis，再叠加一层节点内短 TTL 本地热点缓存
- 失效：不是逐项删 key，而是通过 `Rebuild` 覆盖重建
- 空列表处理：直接删 Redis key
- 读取：按页切片，但底层 Redis 中是一份整列表快照

它和 `scale` 的差别是：

- `scale` 缓存单对象
- `scale_list` 缓存一个已经聚合好的发布列表快照

### 3. `questionnaire`：问卷及发布快照缓存

代码入口：
- [internal/apiserver/infra/cache/questionnaire_cache.go](../../internal/apiserver/infra/cache/questionnaire_cache.go)
- [internal/apiserver/container/assembler/survey.go](../../internal/apiserver/container/assembler/survey.go)

当前行为：

- 缓存对象不止一类：
  - 当前 head
  - active published snapshot
  - 指定 `code + version` 的发布快照
- 模式：`repository decorator + read-through`
- negative cache：已开启
- 失效：围绕 `code` 家族删 key
  - `Update`
  - `SetActivePublishedVersion`
  - `ClearActivePublishedVersion`
  - `Remove / HardDelete / HardDeleteFamily`

这意味着问卷缓存不是“单 key 对单对象”的最小模型，而是一个**以问卷 code 为聚合根的缓存家族**。

## 二、`object_view`：单对象视图缓存

### 1. `assessment_detail`

代码入口：
- [internal/apiserver/infra/cache/assessment_detail_cache.go](../../internal/apiserver/infra/cache/assessment_detail_cache.go)
- [internal/apiserver/container/assembler/evaluation.go](../../internal/apiserver/container/assembler/evaluation.go)

当前行为：

- 缓存对象：按 `assessment ID` 查询的测评详情
- 模式：`repository decorator + read-through`
- 失效：`Save / SaveWithEvents / SaveWithAdditionalEvents / Delete` 都会删单 key
- 不缓存的读路径：按 testee、按 plan、按 org、分页列表查询都透传底层 repo

### 2. `testee`

代码入口：
- [internal/apiserver/infra/cache/testee_cache.go](../../internal/apiserver/infra/cache/testee_cache.go)
- [internal/apiserver/container/assembler/actor.go](../../internal/apiserver/container/assembler/actor.go)

当前行为：

- 缓存对象：按 `testee ID` 查询的受试者详情
- 模式：`repository decorator + read-through + negative cache`
- 失效：`Save / Update / Delete` 删单 key
- 不缓存的读路径：
  - `FindByProfile`
  - `ListByOrg`
  - `ListByTags`
  - `ListKeyFocus`
  - `Count*`

也就是说，当前 `testee` 只缓存最核心的**单对象详情视图**。

### 3. `plan`

代码入口：
- [internal/apiserver/infra/cache/plan_cache.go](../../internal/apiserver/infra/cache/plan_cache.go)
- [internal/apiserver/container/assembler/plan.go](../../internal/apiserver/container/assembler/plan.go)

当前行为：

- 缓存对象：按 `plan ID` 查询的计划详情
- 模式：`repository decorator + read-through`
- 失效：`Save` 删单 key
- 不缓存的读路径：
  - `FindByScaleCode`
  - `FindActivePlans`
  - `FindByTesteeID`
  - `FindList`

## 三、`query_result + meta_hotset`：查询与列表缓存

### 1. `assessment_list`：我的测评列表

代码入口：
- [internal/apiserver/infra/cache/my_assessment_list_cache.go](../../internal/apiserver/infra/cache/my_assessment_list_cache.go)
- [internal/apiserver/infra/cache/versioned_query_cache.go](../../internal/apiserver/infra/cache/versioned_query_cache.go)
- [internal/apiserver/infra/cache/version_token_store.go](../../internal/apiserver/infra/cache/version_token_store.go)

当前行为：

- 缓存对象：用户维度的“我的测评列表”查询结果
- 维度：`userID + status + scaleCode + riskLevel + date range + page/pageSize`
- 模式：
  - `version token`
  - `versioned data key`
  - 节点内短 TTL 本地热点缓存
- 失效：**不删旧 key**，而是 bump version token
- 结果：旧 key 交给 TTL 自然过期，主路径不回到 `DeletePattern`

这是当前 `qs-server` 查询缓存最有代表性的实现。

### 2. `stats_query`：统计查询结果缓存

代码入口：
- [internal/apiserver/infra/statistics/cache.go](../../internal/apiserver/infra/statistics/cache.go)
- [internal/apiserver/container/container.go](../../internal/apiserver/container/container.go)

当前行为：

- 缓存对象：统计查询结果
- family：`query_result`
- 模式：query result cache，带压缩和 TTL 抖动
- 治理联动：
  - `query.stats_system`
  - `query.stats_questionnaire`
  - `query.stats_plan`
  这三类 warmup target 都会走到它

和 `assessment_list` 的差别是：

- `assessment_list` 是业务查询列表缓存，失效以 version token 为核心
- `stats_query` 更偏聚合查询缓存，更多依赖 TTL 和 warmup 重建

## 四、`meta_hotset`：缓存元数据与治理型缓存

### 1. version token

代码入口：
- [internal/apiserver/infra/cache/version_token_store.go](../../internal/apiserver/infra/cache/version_token_store.go)

当前角色：

- 为 query/list cache 提供版本游标
- 当前主要服务于 `assessment_list`
- 从“业务上看”它不是直接对外暴露的缓存对象，但没有它，`versioned query cache` 就不成立

### 2. hotset / warmup metadata

代码入口：
- [internal/apiserver/infra/cache/hotset.go](../../internal/apiserver/infra/cache/hotset.go)
- [internal/apiserver/application/cachegovernance/coordinator.go](../../internal/apiserver/application/cachegovernance/coordinator.go)

当前角色：

- 记录热点 target
- 支撑 startup / publish / statistics sync / repair / manual warmup
- 提供治理页 hotset 预览与 warmup 候选

它属于 Redis cache usage，但它服务的是**治理面**，不是直接服务某个业务接口的领域对象读取。

## 五、`sdk_token`：第三方 SDK 缓存

代码入口：
- [internal/apiserver/infra/wechatapi/cache_adapter.go](../../internal/apiserver/infra/wechatapi/cache_adapter.go)
- [internal/apiserver/container/container.go](../../internal/apiserver/container/container.go)

当前行为：

- 缓存对象：微信 SDK 使用的 access token / ticket 等字符串值
- 模式：把 Redis 适配成 `silenceper/wechat` 的 `cache.Cache`
- family：`sdk_token`
- 降级：如果 `sdk_token` family 不可用，则自动回退到 SDK 内存缓存

它不是领域缓存，但它确实是 `qs-server` 当前 Redis Cache 层的一部分。

## 六、当前明确没有进入 Redis 读缓存的路径

这部分同样重要，因为它决定了边界。

### 1. `collection-server` 的领域读路径

当前没有进入 Redis 读缓存。它只使用：

- `ops_runtime`
- `lock_lease`

代码入口：
- [internal/collection-server/infra/redisops](../../internal/collection-server/infra/redisops)
- [internal/pkg/redisplane/ratelimiter.go](../../internal/pkg/redisplane/ratelimiter.go)

### 2. `worker`

当前没有对象缓存或查询缓存，主要消费：

- `lock_lease`

### 3. 大多数列表查询

当前明确未缓存或不直接缓存的路径包括：

- `scale` 的 `FindSummaryList`
- `plan` 的 `FindList`
- `testee` 的各类列表和统计
- `assessment` 的大多数按条件列表

这说明当前 Redis cache 仍然是**有选择地缓存高价值读模型**，而不是“所有查询都上一层 Redis”。

## 七、怎么用这份清单

如果你现在要做三件事，这份清单的使用方式分别是：

1. **排查一个业务接口有没有缓存**
   - 先按业务模块找条目，再回到对应 `infra/cache/*.go`
2. **新增一个缓存**
   - 先确认这个读模型更像本清单中的哪一类，再回读 [11-Redis三层设计与落地手册.md](./11-Redis三层设计与落地手册.md)
3. **解释 Redis 容量和命中面**
   - 先分清它属于 `static_meta / object_view / query_result / meta_hotset / sdk_token` 中的哪一类，再去看对应 family 的 runtime 和治理状态

---

*写作约定见 [CONTRIBUTING-DOCS.md](../CONTRIBUTING-DOCS.md)。*
