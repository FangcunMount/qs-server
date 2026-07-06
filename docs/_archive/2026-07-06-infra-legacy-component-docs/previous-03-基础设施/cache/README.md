# 缓存模块阅读地图

**本文回答**：qs-server 的缓存模块解决什么系统问题；L1 本地缓存、L2 Redis、预热、TTL、失效和降级如何共同治理读侧流量；现有 `redis/` 细文档应该如何阅读。

---

## 30 秒结论

缓存不是单纯的性能优化，而是 qs-server 的**读侧治理层**。

前台查询问卷、测评目录、模型资产、报告状态时，请求量远高于写入量。如果每次都直接访问 MongoDB / MySQL，会把简单读流量放大成数据库压力。因此系统采用 L1 本地缓存 + L2 Redis 缓存：

- L1 解决单进程内热点对象重复解析和重复 gRPC 调用问题。
- L2 解决多进程、多实例之间的共享缓存问题，降低 apiserver 回源 DB 压力。
- 缓存预热降低服务启动和模型发布后的冷启动抖动。
- TTL 分层避免所有缓存同一时间失效。
- 变更事件和 Redis signaling 驱动目录、模型、问卷缓存刷新。
- Redis 异常时允许回源，但必须配合 singleflight、backpressure、rate limit 或队列，避免缓存雪崩变成 DB 雪崩。

---

## 能力矩阵

| 能力 | 当前定位 | 典型对象 | 继续阅读 |
| ---- | -------- | -------- | -------- |
| L1 本地缓存 | 进程内短生命周期缓存，适合热点目录、模型元数据、低变化配置 | collection-server 问卷、量表、人格模型 REST DTO | [../redis/10-Catalog目录L1-L2缓存.md](../redis/10-Catalog目录L1-L2缓存.md) |
| L2 Redis 缓存 | 跨进程共享缓存，承接多实例读请求 | Published model、questionnaire、scale、query list | [../redis/02-Cache层总览.md](../redis/02-Cache层总览.md) |
| 缓存预热 | 启动或发布后提前加载热点对象，减少冷启动击穿 | WarmupTarget、cachegovernance coordinator | [../redis/05-Hotset与WarmupTarget模型.md](../redis/05-Hotset与WarmupTarget模型.md)、[../redis/07-缓存治理层.md](../redis/07-缓存治理层.md) |
| TTL 分层 | 按数据变化频率设置不同有效期 | 目录较长 TTL，报告状态较短 TTL，临时信令更短 | [../redis/03-ObjectCache主路径.md](../redis/03-ObjectCache主路径.md)、[../redis/10-Catalog目录L1-L2缓存.md](../redis/10-Catalog目录L1-L2缓存.md) |
| 失效机制 | 发布、下架、配置变更后主动失效或信令刷新 | questionnaire/scale/personality cache changed | [../redis/10-Catalog目录L1-L2缓存.md](../redis/10-Catalog目录L1-L2缓存.md)、`configs/signals.yaml` |
| 故障降级 | Redis 异常时回源或 degraded，但保留并发保护 | family status、fallback、degraded mode | [../redis/08-观测降级与排障.md](../redis/08-观测降级与排障.md) |

---

## 当前读侧链路

| 链路 | 缓存作用 | 风险 |
| ---- | -------- | ---- |
| 问卷 / 目录查询 | L1 命中省 collection -> apiserver gRPC；L2 命中省 apiserver -> Mongo | L1 未开启或信令失效会放大 gRPC 和 Mongo 压力 |
| 测评模型查询 | L2 缓存 published model / scale snapshot，供目录读和 submit 热路径复用 | 模型发布后需要失效与预热，否则会有冷启动抖动 |
| 报告状态查询 | Redis report_status 承接前端持续查询；非终态通过 `next_poll_after_ms` 退避 | 固定频率短轮询会随在线用户数和生成耗时线性放大 |
| 统计和列表读 | QueryCache / StaticList 缓解稳定列表和读模型重复查询 | 版本失效和参数 hash 必须控制粒度 |

---

## L1 + L2 分工

| 层 | 进程 | 存储 | 解决的问题 | 一致性边界 |
| -- | ---- | ---- | ---------- | ---------- |
| L1 | collection-server | 进程内 local TTL cache | 热点 REST DTO 重复解析、重复 gRPC 调用 | 由 TTL 和 Redis signaling 兜底，不能当强一致事实源 |
| L2 | qs-apiserver | Redis cache family | 多实例共享缓存，降低 Mongo/MySQL 回源 | 由 CachePolicy、version token、delete invalidation、信令和 TTL 控制 |

目录缓存的当前事实源是 [../redis/10-Catalog目录L1-L2缓存.md](../redis/10-Catalog目录L1-L2缓存.md)。

---

## 预热、失效与信令

`configs/signals.yaml` 把一次性信令定义为 `ephemeral_signal`：

```text
Redis Pub/Sub 一次性唤醒，非业务事实。
丢失可接受，订阅方通过 TTL、查询或下次变更兜底。
```

当前信令包括：

| 信令 | 作用 |
| ---- | ---- |
| `questionnaire_cache_changed` | 问卷缓存失效 / 预热唤醒 |
| `scale_cache_changed` | 量表缓存失效 / 预热唤醒 |
| `personality_model_cache_changed` | 人格模型缓存失效 / 预热唤醒 |
| `report_status_changed` | 报告状态变更后唤醒 wait-report |

注意：缓存信令和事件系统不是替代关系。缓存信令只负责读侧刷新和在线唤醒，可靠业务事实仍看数据库状态、Outbox 和领域事件。

---

## 降级原则

| 故障 | 允许行为 | 保护要求 |
| ---- | -------- | -------- |
| L1 miss | 回到 apiserver gRPC | singleflight 合并 miss，避免热点击穿 |
| L2 Redis 不可用 | 回源 Mongo/MySQL | backpressure / rate limit 控制回源并发 |
| 信令丢失 | 等 TTL 或下次查询修正 | TTL 不能过长到影响可接受陈旧窗口 |
| 预热失败 | 允许冷启动回源 | 观测 warmup failure，避免发布后集中 miss |
| report_status miss | 查询持久化状态或返回非终态退避 | 不让客户端紧循环查询 |

---

## 代码事实源

| 主题 | 路径 |
| ---- | ---- |
| collection L1 | `internal/collection-server/application/catalogl1`、`internal/collection-server/application/catalogcache` |
| apiserver L2 | `internal/apiserver/infra/cache`、`internal/apiserver/infra/cachequery`、`internal/apiserver/infra/cachepolicy` |
| cache plane | `internal/pkg/cacheplane` |
| cache governance | `internal/apiserver/application/cachegovernance`、`internal/pkg/cachegovernance` |
| cache signal | `internal/pkg/cachesignal`、`configs/signals.yaml` |
| report status | `configs/*report_status*`、`api/rest/collection.yaml` |

