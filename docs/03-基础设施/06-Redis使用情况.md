# Redis 当前实现与治理总览

**本文回答**：`qs-server` 当前到底怎样使用 Redis，三进程分别依赖哪些 Redis family，运行时暴露了哪些治理接口，以及应该从哪几篇文档进入 Redis 体系。

## 30 秒结论

| 维度 | 当前结论 |
| ---- | -------- |
| 真值层入口 | **先读 [12-Redis文档中心](./12-Redis文档中心.md)** 建立四层地图；再读本文看当前实现与运行边界；最后按需进入 [11-Redis三层设计与落地手册](./11-Redis三层设计与落地手册.md) 与 [13-Redis缓存业务清单](./13-Redis缓存业务清单.md) |
| Foundation | Foundation 已沉到 `component-base/pkg/redis` + `component-base/pkg/database`；`qs-server` 不再自己实现 runtime / keyspace / typed store / lease 原语 |
| Cache 层 | **只在 `apiserver` 完整存在**：`static_meta / object_view / query_result / meta_hotset / sdk_token` |
| Lock 层 | **三进程共享 `lock_lease`**；锁入口统一收口到 [`internal/pkg/redislock`](../../internal/pkg/redislock/) 的 `Manager + LockSpec` |
| Collection 侧 Redis | `collection-server` 已重新接入 Redis，但仅用于 **`ops_runtime + lock_lease`**，不承担领域读缓存，也不承担 durable queue |
| Governance | 三进程都已经暴露 `/readyz` 与 `/governance/redis`；`apiserver` 额外暴露 `/cache/governance/status`、`/cache/governance/hotset`、`/cache/governance/warmup-targets`、`/cache/governance/repair-complete` |
| 历史文档处理 | Redis 历史设计稿与阶段记录已迁入 [`docs/_archive`](../_archive/README.md)，不再作为现行真值层入口 |

## 先读哪几篇

Redis 相关文档现在收口成一个中心页和三篇现行真值文：

1. **总入口 / 阅读地图**：[12-Redis文档中心](./12-Redis文档中心.md)
2. **当前实现与运行边界**：本文
3. **三层设计、建模与接入手册**：[11-Redis三层设计与落地手册](./11-Redis三层设计与落地手册.md)
4. **业务缓存清单**：[13-Redis缓存业务清单](./13-Redis缓存业务清单.md)

如果要接 operating 或排查治理接口，再读：

- [04-接口与运维/06-operating 缓存治理页接入.md](../04-接口与运维/06-operating%20缓存治理页接入.md)

如果只是想看历史设计演进，再读：

- [07-Redis代码总览（源码审计版）](../_archive/03-基础设施/07-Redis代码总览（源码审计版）.md)
- [08-Redis分层重构设计](../_archive/03-基础设施/08-Redis分层重构设计.md)
- [09-Redis跨仓重构路线](../_archive/03-基础设施/09-Redis跨仓重构路线.md)
- [10-apiserver缓存实现层重构](../_archive/03-基础设施/10-apiserver缓存实现层重构.md)
- [05-缓存体系设计：从零散缓存到统一缓存平台](../_archive/05-专题分析/05-缓存体系设计：从零散缓存到统一缓存平台.md)

这些历史文档仍可提供术语和演进背景，但**现状以本文和代码为准**。

## Redis 在三进程里的位置

| 进程 | 当前 Redis 角色 | 说明 |
| ---- | --------------- | ---- |
| `qs-apiserver` | Cache 主进程 + Governance 主进程 + Statistics / SDK / Lock 消费方 | 承担对象缓存、查询缓存、hotset、手工预热、statistics sync 锁、微信 SDK 缓存 |
| `qs-worker` | Lock 消费方 | 当前只运行 `lock_lease` family；主要承担 answersheet 处理闸门等 worker 侧互斥 |
| `collection-server` | Operational Redis 消费方 | 只接 `ops_runtime + lock_lease`，用于限流、提交幂等与 in-flight guard；不做领域读缓存 |

代码锚点：

- `apiserver` 启动与 runtime 注入：
  [internal/apiserver/server.go](../../internal/apiserver/server.go)
  [internal/apiserver/container/container.go](../../internal/apiserver/container/container.go)
- `worker` 启动与 lock manager：
  [internal/worker/server.go](../../internal/worker/server.go)
- `collection-server` runtime 装配：
  [internal/collection-server/server.go](../../internal/collection-server/server.go)

## 当前 family 模型

逻辑 family 统一定义在：
[internal/pkg/redisplane/catalog.go](../../internal/pkg/redisplane/catalog.go)

| Family | 主要进程 | 典型用途 |
| ------ | -------- | -------- |
| `static_meta` | `apiserver` | 量表、问卷、已发布量表列表等静态或半静态缓存 |
| `object_view` | `apiserver` | `assessment detail`、`testee info`、`plan info` 等单对象视图缓存 |
| `query_result` | `apiserver` | 统计查询缓存、版本化 query/list 缓存 |
| `meta_hotset` | `apiserver` | hotset 排行、version token、warmup 元数据 |
| `sdk_token` | `apiserver` | 微信 SDK token / ticket 等第三方 SDK 缓存 |
| `lock_lease` | `apiserver` / `worker` / `collection-server` | 共享 lease lock family |
| `ops_runtime` | `collection-server` | 限流、提交幂等、in-flight guard 等操作性 Redis |

### family 路由配置

当前 family 路由统一使用：

- `redis`
- `redis_profiles`
- `redis_runtime`

配置与校验入口：

- apiserver：
  [internal/apiserver/options/options.go](../../internal/apiserver/options/options.go)
- worker：
  [internal/worker/options/options.go](../../internal/worker/options/options.go)
- collection-server：
  [internal/collection-server/options/options.go](../../internal/collection-server/options/options.go)

生产配置样例：

- [configs/apiserver.prod.yaml](../../configs/apiserver.prod.yaml)
- [configs/worker.prod.yaml](../../configs/worker.prod.yaml)
- [configs/collection-server.prod.yaml](../../configs/collection-server.prod.yaml)

## Cache 层：当前实现边界

### 1. `apiserver` 是唯一完整 Cache 层消费者

当前 `apiserver` 中仍然属于现行 Cache 层的实现，主要落在：

- [internal/apiserver/infra/cache](../../internal/apiserver/infra/cache)
- [internal/apiserver/infra/cachepolicy](../../internal/apiserver/infra/cachepolicy)
- [internal/apiserver/application/cachegovernance](../../internal/apiserver/application/cachegovernance)

其中：

- `infra/cache`：只保留具体缓存实现、read-through、versioned query、hotset、本地热点缓存、repository decorator。
- `cachepolicy`：对象级缓存策略唯一入口。
- `cachegovernance`：预热、状态聚合、治理命令与查询。

### 2. 当前对象与查询缓存

当前主缓存对象包括：

- 量表与问卷缓存：
  [scale_cache.go](../../internal/apiserver/infra/cache/scale_cache.go)
  [questionnaire_cache.go](../../internal/apiserver/infra/cache/questionnaire_cache.go)
- 单对象缓存：
  [assessment_detail_cache.go](../../internal/apiserver/infra/cache/assessment_detail_cache.go)
  [testee_cache.go](../../internal/apiserver/infra/cache/testee_cache.go)
  [plan_cache.go](../../internal/apiserver/infra/cache/plan_cache.go)
- 列表与 query 缓存：
  [global_list_cache.go](../../internal/apiserver/application/scale/global_list_cache.go)
  [my_assessment_list_cache.go](../../internal/apiserver/infra/cache/my_assessment_list_cache.go)
  [internal/apiserver/infra/statistics/cache.go](../../internal/apiserver/infra/statistics/cache.go)

当前原则：

- 对象缓存优先 `decorator + read-through + 写后失效`
- query/list 缓存优先 `version token + versioned key`
- 热点 query 和列表允许再叠一层短 TTL 本地热点缓存
- Redis key 统一走 [`internal/pkg/rediskey`](../../internal/pkg/rediskey/)

### 3. `collection-server` 当前不做领域读缓存

虽然 `collection-server` 已重新接入 Redis，但它只在：

- 分布式限流
- 提交幂等 / 重复抑制
- in-flight guard

这些操作性场景使用 Redis，不承担：

- 领域对象读缓存
- query/list 读缓存
- durable queue / Redis Streams 队列

相关实现：

- [internal/collection-server/infra/redisops](../../internal/collection-server/infra/redisops)

## Warmup 与治理接口

### 当前 warmup 触发器

`apiserver` 当前已经不是“只有启动时手写几段预热”的模式，而是统一由 `WarmupCoordinator` 编排：

- startup
- scale publish
- questionnaire publish
- statistics sync
- repair complete
- manual warmup

入口与实现：

- [internal/apiserver/container/container.go](../../internal/apiserver/container/container.go)
- [internal/apiserver/application/cachegovernance/coordinator.go](../../internal/apiserver/application/cachegovernance/coordinator.go)

### 当前 manual warmup

标准治理命令已经存在：

- `POST /internal/v1/cache/governance/warmup-targets`

支持的 target kind 当前为：

- `static.scale`
- `static.questionnaire`
- `static.scale_list`
- `query.stats_system`
- `query.stats_questionnaire`
- `query.stats_plan`

接口与模型：

- [internal/apiserver/interface/restful/handler/statistics.go](../../internal/apiserver/interface/restful/handler/statistics.go)
- [internal/apiserver/application/cachegovernance/manual_warmup.go](../../internal/apiserver/application/cachegovernance/manual_warmup.go)

### 当前治理查询面

`apiserver`：

- `GET /readyz`
- `GET /governance/redis`
- `GET /internal/v1/cache/governance/status`
- `GET /internal/v1/cache/governance/hotset`
- `POST /internal/v1/cache/governance/repair-complete`
- `POST /internal/v1/cache/governance/warmup-targets`

`worker`：

- `GET /readyz`
- `GET /governance/redis`
- `/metrics`

`collection-server`：

- `GET /readyz`
- `GET /governance/redis`

代码锚点：

- apiserver：
  [internal/apiserver/routers.go](../../internal/apiserver/routers.go)
- worker：
  [internal/worker/metrics_server.go](../../internal/worker/metrics_server.go)
- collection-server：
  [internal/collection-server/routers.go](../../internal/collection-server/routers.go)

## Lock 层：当前实现边界

### 当前 LockSpec

共享锁规格已经收口到：
[internal/pkg/redislock/spec.go](../../internal/pkg/redislock/spec.go)

当前内建锁规格：

| Spec | 默认 TTL | 主要使用方 | 说明 |
| ---- | -------- | ---------- | ---- |
| `answersheet_processing` | `5m` | `worker` | 抑制重复处理答卷 |
| `plan_scheduler_leader` | `50s` | `apiserver` | 调度器选主 |
| `statistics_sync_leader` | `30m` | `apiserver` | statistics sync 调度器多实例选主 |
| `statistics_sync` | `30m` | `apiserver` | nightly statistics sync 互斥 |
| `behavior_pending_reconcile` | `30s` | `apiserver` | behavior pending reconcile 多实例串行化执行 |
| `collection_submit` | `5m` | `collection-server` | 提交幂等与 in-flight guard |

调用入口统一为：

- `AcquireSpec`
- `ReleaseSpec`

代码锚点：

- [internal/pkg/redislock/lock.go](../../internal/pkg/redislock/lock.go)
- [internal/worker/handlers/answersheet_handler.go](../../internal/worker/handlers/answersheet_handler.go)
- [internal/apiserver/runtime/scheduler/plan_scheduler.go](../../internal/apiserver/runtime/scheduler/plan_scheduler.go)
- [internal/apiserver/runtime/scheduler/statistics_sync.go](../../internal/apiserver/runtime/scheduler/statistics_sync.go)
- [internal/apiserver/application/statistics/sync_service.go](../../internal/apiserver/application/statistics/sync_service.go)
- [internal/apiserver/runtime/scheduler/behavior_pending_reconcile.go](../../internal/apiserver/runtime/scheduler/behavior_pending_reconcile.go)
- [internal/collection-server/infra/redisops/submit_guard.go](../../internal/collection-server/infra/redisops/submit_guard.go)

### 当前锁边界

当前锁仍然是：

- 单 Redis lease lock
- token ownership release
- 无续租
- 无 fencing token

也就是说，它们适合：

- 重复工作抑制
- leader election
- 轻量互斥保护

不适合：

- 长时间持锁的大任务
- 需要 fencing token 的强顺序写场景
- 用 Redis 锁代替业务真相和数据库约束

## 当前边界与注意事项

1. **现行真值层已经收口**：Redis 现状看本文，建模与“从 0 到 1”接入看 [11-Redis三层设计与落地手册](./11-Redis三层设计与落地手册.md)。
2. **`collection-server` 的 Redis 角色已经变化**：它不再是“完全不用 Redis”，而是仅消费 `ops_runtime + lock_lease`。
3. **`manual warmup` 与 `LockSpec` 都已落地**：再阅读旧设计稿时，必须注意其中关于“尚未实现”的描述已经失效。
4. **旧的 `infra/cache` 路由 / catalog 文档已过时**：当前 family 路由以 `redisplane` 为准，对象策略以 `cachepolicy` 为准。
5. **历史设计稿仍可提供背景**：但如果旧文和本文冲突，以本文和代码为准。

## 代码索引

- Redis runtime：
  [internal/pkg/redisplane](../../internal/pkg/redisplane)
- Redis key builder：
  [internal/pkg/rediskey](../../internal/pkg/rediskey)
- Lock 层：
  [internal/pkg/redislock](../../internal/pkg/redislock)
- Governance 指标与快照：
  [internal/pkg/cacheobservability](../../internal/pkg/cacheobservability)
- apiserver Cache 层：
  [internal/apiserver/infra/cache](../../internal/apiserver/infra/cache)
  [internal/apiserver/infra/cachepolicy](../../internal/apiserver/infra/cachepolicy)
  [internal/apiserver/application/cachegovernance](../../internal/apiserver/application/cachegovernance)
- collection-server Redis ops：
  [internal/collection-server/infra/redisops](../../internal/collection-server/infra/redisops)

---

*写作约定见 [CONTRIBUTING-DOCS.md](../CONTRIBUTING-DOCS.md)。*
