# 当前 Redis 使用清单（qs-server）

> ⚠️ 2024 Q1 更新：redis-store 已移除，当前仅保留单实例 Redis（原 redis-cache）。本清单中关于 store 的条目保留为历史信息。

目的：梳理现有各服务对 Redis 的实际使用场景、键空间与实例归属，为后续（如有）重新引入多实例时提供基线。

## 实例与配置入口

- 配置：`redis.*`（单实例）。APIServer、Worker、Collection Server 三个服务均使用同一套 Redis 连接。
- 连接管理：`internal/apiserver/database.go`、`internal/worker/database.go`、`internal/collection-server/database.go` 会注册单实例 Redis 客户端；APIServer/Worker 支持按配置关闭部分缓存。
- 运维校验：`make check-redis` / `scripts/check-infra.sh redis` 仅探测单实例。

## APIServer 模块使用

- **领域缓存（全部走 redis-cache）**
  - 量表：`scale:{code}`，TTL 24h，Cache-Aside，命中 miss 后异步回填（`internal/apiserver/infra/cache/scale_cache.go`）。
  - 问卷：`questionnaire:{code}` 或 `questionnaire:{code}:{version}`，TTL 12h，Cache-Aside（`.../cache/questionnaire_cache.go`）。
  - 测评详情：`assessment:detail:{id}`，TTL 2h，Cache-Aside（`.../cache/assessment_detail_cache.go`）。
  - 测评状态：`assessment:status:{id}`，TTL 30m，Write-Through，更新/写库时同步写缓存（`.../cache/assessment_status_cache.go`）。
  - 受试者：`testee:info:{id}`，TTL 2h，Cache-Aside（`.../cache/testee_cache.go`）。
  - 计划：`plan:info:{id}`，TTL 2h，Cache-Aside（`.../cache/plan_cache.go`）。
  - 统一封装：底层使用 `RedisCache`（`.../cache/redis_cache.go`），支持 MGet/MSet、模式删除、健康检查。
  - TTL 可通过配置 `cache.ttl.*` 覆盖，启动时生效；`cache.ttl-jitter-ratio`（默认 0.1）用于给各 TTL 加抖动，避免同一时间批量过期。
- **统计预聚合与查询缓存（redis-cache）**
  - 事件幂等：`event:processed:{event_id}`，TTL 7d，用于统计事件处理幂等（Worker 写，APIServer 读校验），文件 `internal/apiserver/infra/statistics/cache.go` & `internal/worker/handlers/statistics_handler.go`。
  - 累计/窗口/日报：`stats:accum:{org}:{type}:{key}:{metric}`、`stats:window:{org}:{type}:{key}:{window}`、`stats:daily:{org}:{type}:{key}:{date}`；窗口/日报默认 TTL 90d，累计默认不过期（如需改可调 `DefaultAccumStatsTTL`）；Worker 在消费测评事件时写入，APIServer 统计服务读取、校验、落库（`internal/apiserver/application/statistics/*service.go`、`.../statistics/sync_service.go`）。
  - 分布统计：`stats:dist:{org}:{type}:{key}:{dimension}`，默认 TTL 90d；同上。
  - 查询结果缓存：`stats:query:{...}`，TTL 5m，供统计查询接口快速返回（`.../statistics/*_service.go` 调用 `SetQueryCache`）。
- **其他**
- 小程序二维码/AccessToken 缓存：`wechat:cache:{key}`，TTL 由 SDK 传入，使用 redis-cache 作为 `cache.Cache` 适配器（`internal/apiserver/infra/wechatapi/cache_adapter.go`）。
- CodesService：历史曾计划使用 redis-store 计数器，现实现直接调用本地 `meta.GenerateCodeWithPrefix`，未写 Redis。
- 预热与指标：`WarmupCache` 支持预热量表等缓存；`metrics.go` 统计缓存命中率等（均基于 redis-cache）。

## 统计键生命周期（新增）

- 默认 TTL：`stats:daily:*`/`stats:window:*`/`stats:dist:*` = 90 天；`stats:accum:*` 不过期（需长期累计时保留，可调整常量）。
- TTL 应用位置：`internal/apiserver/infra/statistics/cache.go` 在写入后统一 `Expire`，历史键也会被刷新 TTL。
- 历史无 TTL 键修复工具：`cmd/tools/redis-stats-ttl-fix`，示例：
  - `go run ./cmd/tools/redis-stats-ttl-fix --addr 127.0.0.1:6379 --pass xxx` 补齐 TTL。
  - `--dry-run` 只统计不写，`--ttl-*` 参数可调。
- 定时落库：APIServer 启动时默认开启统计同步定时任务（最终一致），配置 `statistics_sync.*` 控制启停和间隔；默认延迟 1 分钟启动，daily/accumulated 每 10 分钟，plan 每 30 分钟（见 `internal/apiserver/server.go`）。

## Worker 使用

- 统计事件处理（redis-cache）：
  - `statistics_assessment_submitted_handler` 与 `statistics_assessment_interpreted_handler` 将测评提交/解读事件写入上述 `stats:*` 与 `event:processed:*` 键（`internal/worker/handlers/statistics_handler.go`）。
  - 可通过配置关闭统计缓存（传入 nil 跳过），未使用 redis-store。

## Collection Server 使用

- 当前未实际读写 Redis。容器接收 cache/store 客户端但未在应用/接口层调用（`internal/collection-server/container/container.go`），后续若需限流、幂等可复用现有配置。

## 工具与脚本

- Seeder/工具的 Redis 连接封装：`cmd/tools/internal/common/common.go`（可用于本地数据生成）。
- 基础设施检查：`scripts/check-infra.sh` / `make check-redis` 同时校验 cache/store。

## 现状总结

- 实际生产流量全部落在 **redis-cache**（缓存、统计预聚合、微信 token、幂等标记）；**redis-store 仅在 CodesService 注入但代码未用到**。
- 统计键空间（`stats:*`、`event:processed:*`）无 TTL，长期增长需规划清理/落库策略。
- 双实例语义已在文档强调（`docs/07-基础设施/07-全局缓存架构设计.md` 等），但代码层尚未强制区分 store 与 cache 的职责。
