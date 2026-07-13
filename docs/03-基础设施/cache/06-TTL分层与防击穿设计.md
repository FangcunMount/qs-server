# TTL 分层与防击穿设计

## 1. 解决什么问题

不同数据变化频率不同，不能使用同一 TTL。统一过期会导致缓存雪崩；热点 key 失效会导致缓存击穿；不存在的数据频繁查询会导致缓存穿透。

## 2. 所在位置

TTL 和防击穿位于 L1/L2 cache store、cache policy、read-through loader 和 report status 查询链路中。

## 3. 设计目标

按数据类型设置 TTL；给 TTL 加 jitter；热点回源用 singleflight；不存在数据可短 TTL 空值缓存；降级时限制 DB 回源。

## 4. 整体流程

读请求 miss 后进入 loader；loader 先合并并发，再回源；写入缓存时按 family 选择 TTL 并加随机抖动。

## 5. 核心数据结构

| 数据类型 | 变化频率 | TTL 策略 |
| --- | --- | --- |
| 测评目录 | 低 | 长 TTL + 主动失效 |
| 问卷结构 | 中低 | 中长 TTL + 版本号 |
| 模型快照 | 低 | 长 TTL + 不变快照 |
| 报告状态 | 高 | 短 TTL |
| 提交防重 key | 临时 | 极短 TTL |

## 6. 正常流程

热点数据在 TTL 内命中；过期后由一个 loader 回源并刷新缓存；其它并发请求等待或共享结果。

## 7. 异常流程

回源失败不写缓存；空值缓存只使用短 TTL；DB 或 Redis 慢时限制并发并暴露错误指标。

## 8. 幂等 / 降级 / 背压

缓存刷新是覆盖式幂等；TTL jitter 避免同一批 key 同时过期；singleflight 降低击穿；空值缓存降低穿透；降级回源必须有上限。

## 9. 可选方案

固定 TTL 简单但容易雪崩；不过期缓存延迟低但失效不可控；强制实时回源会打穿 DB。

## 10. 当前方案取舍

采用分层 TTL、jitter、singleflight 和短 TTL 空值缓存组合，控制一致性窗口和回源压力。

## 11. 观测指标

TTL expiration burst、singleflight shared ratio、empty-cache hit、DB fallback count、hot key miss rate、cache stampede reject count。

## 12. 代码事实源

- [../../../internal/pkg/cache/policy.go](../../../internal/pkg/cache/policy.go)
- [../../../internal/pkg/loadguard](../../../internal/pkg/loadguard)
- [../../../internal/apiserver/cache/catalog](../../../internal/apiserver/cache/catalog)
- [../../../internal/collection-server/options/catalog_l1_cache.go](../../../internal/collection-server/options/catalog_l1_cache.go)
