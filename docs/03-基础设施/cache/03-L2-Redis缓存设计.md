# L2 Redis 缓存设计

## 1. 解决什么问题

L2 Redis 解决多实例之间缓存不能共享的问题。只靠 L1 时，每个实例都可能在冷启动、热点访问、目录变更后穿透 DB；Redis 提供跨进程共享缓存和临时状态承载。

## 2. 所在位置

L2 位于应用服务与 MongoDB / MySQL 之间，主要由 qs-apiserver 的 cache store、query cache、cache entry 和 collection-server 的 report status adapter 使用。

## 3. 设计目标

跨实例共享热点数据；减少 DB 回源；统一 keyspace 和 TTL 策略；支持命中、回源、失效、预热的观测；Redis 异常时可受控降级。

## 4. 整体流程

L1 未命中后读取 Redis；Redis 命中则返回并回填 L1；Redis 未命中则通过 loader 回源 DB，成功后写入 Redis 并设置 TTL。

## 5. 核心数据结构

L2 key 需要包含 family、业务 ID、版本或查询维度；value 采用可演进序列化；TTL 由数据类型和变化频率决定；query cache 还需要 version token 支持主动失效。

## 6. 正常流程

目录、模型、问卷结构和列表查询按 keyspace 读写 Redis。缓存治理模块可枚举 family 状态、执行 warmup / repair，并暴露命中与异常指标。

## 7. 异常流程

Redis 超时、连接错误或反序列化失败时，服务不得无限重试。可受控回源 DB，失败则返回错误；错误值不能污染缓存。

## 8. 幂等 / 降级 / 背压

写缓存是幂等覆盖；回源要受 singleflight 和超时保护；Redis 故障期间要限制 DB 回源并观察 DB QPS；热点 key 可用 jitter 和短期旧值降低同步失效风险。

## 9. 可选方案

只用 DB 不可承接高频读；只用 L1 不适合多实例；把报告正文只放 Redis 会丢失事实源边界。

## 10. 当前方案取舍

Redis 只做 L2 缓存和临时状态，不取代 MongoDB / MySQL。报告状态可以放 Redis 加速查询，但报告正文和最终事实仍回到持久化存储。

## 11. 观测指标

Redis hit/miss、Redis latency、error count、serialization error、fallback DB count、keyspace warmup result、version token invalidation count。

## 12. 代码事实源

- [../../../internal/apiserver/cache](../../../internal/apiserver/cache)
- [../../../internal/pkg/cache/query](../../../internal/pkg/cache/query)
- [../../../internal/pkg/cache/redis](../../../internal/pkg/cache/redis)
- [../../../internal/apiserver/cache/catalog](../../../internal/apiserver/cache/catalog)
- [../../../internal/pkg/reportstatus](../../../internal/pkg/reportstatus)
