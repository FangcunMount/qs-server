# Redis 缓存业务清单

**本文回答**：`qs-server` 当前哪些业务对象、查询结果、治理元数据和 SDK token 进入 Redis cache；详细实现机制回链到 Redis 深讲文档。

## 30 秒结论

| 主题 | 当前结论 |
| ---- | -------- |
| 完整业务缓存 | 只在 `apiserver` |
| object cache | `scale`、`questionnaire`、`assessment_detail`、`testee`、`plan` |
| query/static-list | `assessment_list`、`stats_query`、`scale_list` |
| governance metadata | version token、hotset、warmup metadata |
| SDK cache | 微信 SDK token/ticket |
| 深讲入口 | object 看 [redis/03](./redis/03-ObjectCache主路径.md)，query/list 看 [redis/04](./redis/04-QueryCache与StaticList.md)，hotset 看 [redis/05](./redis/05-Hotset与WarmupTarget模型.md) |

## 总表

| 能力 | 缓存对象 | Family | 模式 | 失效 / 刷新 | Warmup |
| ---- | -------- | ------ | ---- | ----------- | ------ |
| Scale | 单量表 | `static_meta` | object decorator + read-through | Create 写入；Update/Remove 删除 | `static.scale` |
| Scale | 已发布量表列表 | `static_meta` | static-list rebuilder | Rebuild 覆盖；空列表删除 | `static.scale_list` |
| Questionnaire | head / published / versioned snapshot | `static_meta` | object decorator + negative cache | code 家族删除 | `static.questionnaire` |
| Evaluation | assessment detail | `object_view` | object decorator | Save/Delete 删除 | 暂无 |
| Evaluation | 我的测评列表 | `query_result` + `meta_hotset` | version token + versioned key | bump version token | 暂无直接 manual warmup |
| Actor | testee | `object_view` | object decorator + negative cache | Save/Update/Delete 删除 | 暂无 |
| Plan | plan | `object_view` | object decorator | Save 删除 | 暂无 |
| Statistics | 统计查询结果 | `query_result` | query cache | TTL + warmup 重建 | `query.stats_*` |
| Governance | version token / hotset | `meta_hotset` | metadata / ZSet | bump / trim / TTL | 治理层内部消费 |
| SDK | 微信 token/ticket | `sdk_token` | SDK adapter | SDK TTL / delete | 暂无 |

## 代码锚点

| 类型 | 文件 |
| ---- | ---- |
| object cache | [infra/cache](../../internal/apiserver/infra/cache) |
| query cache | [infra/cachequery](../../internal/apiserver/infra/cachequery) |
| static-list | [application/scale/global_list_cache.go](../../internal/apiserver/application/scale/global_list_cache.go) |
| statistics cache | [infra/statistics/cache.go](../../internal/apiserver/infra/statistics/cache.go) |
| hotset | [infra/cachehotset/store.go](../../internal/apiserver/infra/cachehotset/store.go) |
| warmup target | [cachetarget/target.go](../../internal/apiserver/cachetarget/target.go) |
| SDK adapter | [infra/wechatapi/cache_adapter.go](../../internal/apiserver/infra/wechatapi/cache_adapter.go) |

## 明确没有 Redis 读缓存的路径

- collection-server 领域查询。
- worker 事件处理。
- 未显式接入 `cachequery` 或 static-list rebuilder 的普通列表查询。
- scheduler leader lock 和 submit guard，它们属于 lock/ops runtime，不属于读缓存。

## 回链

- object cache 细节：[redis/03-ObjectCache主路径.md](./redis/03-ObjectCache主路径.md)
- query/static-list 细节：[redis/04-QueryCache与StaticList.md](./redis/04-QueryCache与StaticList.md)
- hotset/warmup target 细节：[redis/05-Hotset与WarmupTarget模型.md](./redis/05-Hotset与WarmupTarget模型.md)
- 新增缓存 SOP：[redis/09-新增Redis能力SOP.md](./redis/09-新增Redis能力SOP.md)

---

*写作约定见 [CONTRIBUTING-DOCS.md](../CONTRIBUTING-DOCS.md)。*
