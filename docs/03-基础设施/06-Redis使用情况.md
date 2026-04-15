# Redis 使用情况

**本文回答**：`qs-server` 当前到底把 Redis 用在了哪些地方，分别是谁在写、谁在读、TTL 和 key 形态是什么，以及哪些属于主服务运行时、哪些只是工具或本地联调用途。

## 30 秒结论

先看一屏结论：

| 维度 | 结论 |
| ---- | ---- |
| 运行时主用途 | **对象缓存 / 列表缓存、统计查询缓存、统计 daily 中转、事件幂等、分布式互斥锁、微信 SDK token 缓存** |
| 主要使用方 | **`qs-apiserver`** 负责大部分缓存；**`qs-worker`** 负责统计幂等、问卷 daily 预聚合、answersheet 锁、plan scheduler 锁 |
| 已清理的旧用法 | 运行时代码里已不再写 `stats:window:*`、`stats:accum:*`、`stats:dist:*`；问卷列表缓存、测评状态缓存、`CodesService` 的假 Redis 依赖、`collection-server` 运行时 Redis 装配都已移除 |
| 命名空间 | Redis key 统一由 [`internal/pkg/rediskey`](../../internal/pkg/rediskey/) 生成；`cache.namespace` 会同时作用于缓存、统计、锁和微信 SDK 缓存 |
| 设计边界 | 锁是**单 Redis lease lock**，属于 best-effort 分布式锁；不是强一致协调系统 |
| 部署现实 | `apiserver` 默认可用统计 Redis；`worker` 默认 `cache.disable_statistics_cache=true`，所以统计预聚合和幂等键只有在显式开启时才会产生 |

## Redis 在三进程里的位置

| 进程 | 当前 Redis 角色 |
| ---- | --------------- |
| `qs-apiserver` | 通用缓存、列表缓存、统计查询结果缓存、微信 SDK 缓存 |
| `qs-worker` | 统计事件幂等、问卷 daily 预聚合、`answersheet` 处理闸门、plan scheduler 选主锁 |
| `collection-server` | **当前运行时不再连接 / 使用 Redis**；仅保留配置项兼容 |

代码锚点：

- `apiserver` 注入 Redis 到各模块见 [internal/apiserver/container/container.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/container/container.go)
- `worker` Redis 初始化与按配置禁用统计 Redis 见 [internal/worker/server.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/worker/server.go)
- `collection-server` 当前不再初始化 Redis，见 [internal/collection-server/server.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/collection-server/server.go)

## 运行时缓存

### 通用规则

- 这组缓存主要实现于 [internal/apiserver/infra/cache](../../internal/apiserver/infra/cache)
- Redis key 统一由 [`internal/pkg/rediskey`](../../internal/pkg/rediskey/) 生成；`infra/cache/namespace.go` 现在只是对统一 namespace 入口的薄封装
- TTL 支持全局覆盖与抖动，见 [ttl_config.go](../../internal/apiserver/infra/cache/ttl_config.go)
- 问卷、量表、测评详情等对象缓存都使用 repository 装饰器方式接入
- 列表缓存额外带一层进程内短 TTL 内存缓存，减少热点 Redis `GET` 和 JSON 解码

### 缓存清单

| 缓存 | Key / Pattern | 谁在写 | 谁在读 | TTL |
| ---- | ------------- | ------ | ------ | --- |
| 量表单条缓存 | `scale:{code}` | `Create` 写入；`FindByCode` miss 回填；`Update` / `Remove` 失效 | 所有通过 `ScaleRepo.FindByCode` 取量表的链路 | `24h` |
| 问卷工作版本缓存 | `questionnaire:{code}` | `Create` 写入；`FindByCode` miss 回填；`Update` / `Remove` / `HardDelete*` 失效 | 后台详情、编辑流按 `code` 读当前工作版本 | `12h` |
| 问卷当前已发布缓存 | `questionnaire:published:{code}` | `CreatePublishedSnapshot(active=true)` 写入；`FindPublishedByCode` miss 回填；发布激活切换时失效 | 面向提交和公开读取的当前已发布问卷 | `12h` |
| 问卷精确版本缓存 | `questionnaire:{code}:{version}` | 发布快照写入；`FindByCodeVersion` miss 回填；按 `code` 家族失效 | 答卷、评估、历史回放按精确版本读取 | `12h`；空值 negative cache `5m` |
| 测评详情缓存 | `assessment:detail:{id}` | `FindByID` miss 回填；`Save` / `Delete` 失效 | 测评详情、评估链路按 ID 查询 | `2h` |
| 受试者缓存 | `testee:info:{id}` | `FindByID` miss 回填；`Save` / `Update` / `Delete` 失效 | 访问校验、受试者查询、关系管理 | `2h` |
| 计划缓存 | `plan:info:{id}` | `FindByID` miss 回填；`Save` 失效 | 计划查询、计划相关命令服务 | `2h` |
| 已发布量表列表缓存 | `scale:list:v1` | 量表 / 因子变更后 `Rebuild`；查询 miss 也可触发重建 | 已发布量表列表查询 | Redis `10m` + 本地内存 `30s` |
| 我的测评列表缓存 | `assess:list:{user}:v1:{hash}` | 列表查询 miss 回填；创建 / 提交后按用户前缀失效 | “我的测评列表”查询 | Redis `10m` + 本地内存 `30s` |

主要代码：

- [internal/apiserver/infra/cache/scale_cache.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/infra/cache/scale_cache.go)
- [internal/apiserver/infra/cache/questionnaire_cache.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/infra/cache/questionnaire_cache.go)
- [internal/apiserver/infra/cache/assessment_detail_cache.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/infra/cache/assessment_detail_cache.go)
- [internal/apiserver/infra/cache/testee_cache.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/infra/cache/testee_cache.go)
- [internal/apiserver/infra/cache/plan_cache.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/infra/cache/plan_cache.go)
- [internal/apiserver/application/scale/global_list_cache.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/application/scale/global_list_cache.go)
- [internal/apiserver/infra/cache/my_assessment_list_cache.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/infra/cache/my_assessment_list_cache.go)

### 预热

`apiserver` 启动后会异步预热已发布量表与问卷；如果打开 `cache.statistics_warmup`，还会预热统计查询结果缓存。

- 入口见 [internal/apiserver/container/container.go](../../internal/apiserver/container/container.go) `WarmupCache`
- 实现见 [internal/apiserver/infra/cache/warmup.go](../../internal/apiserver/infra/cache/warmup.go)

## 统计侧 Redis

统计侧 Redis 现在已经收口，不再承担一整套复杂预聚合模型；运行时代码里只保留 3 类 key family。

| Key family | 谁在写 | 谁在读 | TTL | 用途 |
| ---------- | ------ | ------ | --- | ---- |
| `stats:query:{cacheKey}` | `system/questionnaire/testee/plan` 统计服务在 miss 后回填 | 同一批统计查询服务先读缓存，再回源 MySQL / 原始表 | `5m` | 统计查询结果缓存 |
| `event:processed:{eventID}` | worker 统计 handler 处理成功后写入 | 同一 handler 在处理前检查 | `7d` | 统计事件幂等 |
| `stats:daily:{org}:questionnaire:{code}:{date}` | worker 统计 handler 对问卷提交数 / 完成数做 `HINCRBY` | `SyncDailyStatistics`、`SyncAccumulatedStatistics`、`ValidateConsistency` | `90d` | 问卷 daily 中转站 |

主要代码：

- 写 / 读封装在 [internal/apiserver/infra/statistics/cache.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/infra/statistics/cache.go)
- worker 写入在 [internal/worker/handlers/statistics_handler.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/worker/handlers/statistics_handler.go)
- 同步 / 校验在 [internal/apiserver/application/statistics/sync_service.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/application/statistics/sync_service.go) 和 [internal/apiserver/application/statistics/validator_service.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/application/statistics/validator_service.go)

### 当前边界

- 运行时代码里已不再写 `stats:window:*`、`stats:accum:*`、`stats:dist:*`
- `system`、`plan`、`testee` 统计现在主要依赖 **`stats:query:*` + MySQL / 原始表回源**
- `questionnaire` 是唯一还保留 Redis daily 中转链的统计类型
- `worker` 默认 `cache.disable_statistics_cache=true`，见 [internal/worker/options/options.go](../../internal/worker/options/options.go)；如果不显式开启，`event:processed:*` 和 `stats:daily:*` 实际不会产生
- `apiserver` 统计模块在 Redis 不可用时会降级，只保留无 Redis 的查询路径，见 [internal/apiserver/container/assembler/statistics.go](../../internal/apiserver/container/assembler/statistics.go)

## 锁

项目里当前真正的 Redis 锁只有 2 把，底层都复用 [internal/pkg/redislock/lock.go](../../internal/pkg/redislock/lock.go)：

- 获取锁：`SETNX key token EX ttl`
- 释放锁：Lua compare-and-del

它们属于**单 Redis 的 lease lock**，可以视作 best-effort 分布式锁，但不是强一致协调系统：没有续租、没有 fencing token、没有 quorum。

| 锁 | Key | 谁在用 | TTL | 作用 |
| -- | --- | ------ | --- | ---- |
| answersheet 处理闸门 | `answersheet:processing:{answerSheetID}` | worker `answersheet_submitted_handler` | `5m` | 抑制重复计分和重复创建测评 |
| plan scheduler 选主锁 | `qs:plan-scheduler:leader`（可配置） | worker 内建 plan scheduler | 默认 `50s` | 多 worker 只允许一个实例推进 plan task 调度 |

### `answersheet` 锁的当前语义

`answersheet` 这把锁已经被重构成“外层处理闸门 + 下游幂等兜底”：

- `locked`：拿到锁后执行“计分 + 创建测评”
- `duplicate_skip`：锁已被占用则直接确认消息成功，不再做重复工作
- `degraded`：Redis 不可用或加锁失败时继续处理，由下游幂等兜底

实现见 [internal/worker/handlers/answersheet_handler.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/worker/handlers/answersheet_handler.go)。

### `plan scheduler` 锁的当前语义

- worker 开启 `plan_scheduler.enable=true` 后，每次 tick 先抢 leader 锁
- 没抢到锁时直接跳过本轮 tick
- Redis 不可用时，scheduler 直接不启动
- 由于没有续租，当前更适合轻量级的 HA 选主，而不是强一致任务协调

实现见 [internal/worker/plan_scheduler.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/worker/plan_scheduler.go)；`apiserver` 本地 scheduler 已废弃，见 [internal/apiserver/plan_scheduler.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/plan_scheduler.go)。

## 第三方 SDK 缓存

微信 SDK 使用 Redis 保存 token / ticket 等缓存：

| Key 前缀 | 谁在写 / 读 | 降级策略 |
| -------- | ----------- | -------- |
| `wechat:cache:{sdkKey}` | 微信 SDK 通过 `RedisCacheAdapter` 间接读写 | Redis client 为 `nil` 时退回内存缓存 |

代码见 [internal/apiserver/infra/wechatapi/cache_adapter.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/internal/apiserver/infra/wechatapi/cache_adapter.go)。

## 工具与非主服务运行时用途

这几处也会连接 Redis，但不属于线上主服务的常规运行时路径：

| 路径 | 用途 |
| ---- | ---- |
| [cmd/tools/redis-stats-ttl-fix](../../cmd/tools/redis-stats-ttl-fix) | 扫描 `stats:daily:*` 并补 TTL |
| [cmd/tools/seeddata/plan_seed_gateway.go](../../cmd/tools/seeddata/plan_seed_gateway.go) | `seeddata` 的本地 plan 链路会连接本地 Redis，复用计划相关运行时能力 |
| [cmd/tools/internal/common/common.go](../../cmd/tools/internal/common/common.go) | CLI / seeddata 打开 Redis 连接的公共 helper |

## 当前边界与注意事项

1. **统计 Redis 是否生效取决于部署**：`worker` 默认关闭统计 Redis，所以很多环境里只有 `stats:query:*` 在工作。
2. **锁不是强一致协调系统**：当前锁实现适合“抑制重复工作 / 选主”，不适合需要 fencing 或长时间持有的任务。
3. **namespace 需要跨进程一致配置**：`apiserver` 和 `worker` 现在都支持 `cache.namespace`；如果两边配置不同，会落到不同前缀下。
4. **`collection-server` 只剩 Redis 配置兼容**：运行时已不再初始化 Redis client；保留配置主要是为了不破坏外部配置面。
5. **文档以当前代码为准**：若与旧专题文档或旧统计文档不一致，以本页和源码为准。

## 代码索引

- 通用缓存接口与 key builder：
  [internal/apiserver/infra/cache/interface.go](../../internal/apiserver/infra/cache/interface.go)
- 通用 Redis cache 封装：
  [internal/apiserver/infra/cache/redis_cache.go](../../internal/apiserver/infra/cache/redis_cache.go)
- 统计 Redis 封装：
  [internal/apiserver/infra/statistics/cache.go](../../internal/apiserver/infra/statistics/cache.go)
- 锁实现：
  [internal/pkg/redislock/lock.go](../../internal/pkg/redislock/lock.go)

---

*写作约定见 [CONTRIBUTING-DOCS.md](../CONTRIBUTING-DOCS.md)。*
