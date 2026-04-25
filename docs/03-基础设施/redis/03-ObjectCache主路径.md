# Object Cache 主路径

**本文回答**：object repository cache 的 read-through、entry codec/store、negative cache、compression、async writeback 和 delete invalidation 如何协作。

## 30 秒结论

| 组件 | 职责 |
| ---- | ---- |
| `Cached*Repository` | 保持 repository decorator 形态，负责 key 与业务回源 |
| `ReadThroughRunner` | 统一 hit/miss/load/writeback/singleflight 流程 |
| `ObjectCacheStore` | 统一 Redis entry get/set/delete/negative/exists |
| `CacheEntryCodec` | domain object 与 JSON payload 转换 |
| `cacheentry.PayloadStore` | Redis payload、compression、observability |

## Read-through 时序

```mermaid
sequenceDiagram
    participant App as Application
    participant Repo as CachedRepository
    participant RT as ReadThroughRunner
    participant Store as ObjectCacheStore
    participant Redis as Redis
    participant DB as SourceRepo

    App->>Repo: FindByID / FindByCode
    Repo->>RT: Read(options)
    RT->>Store: Get(key)
    Store->>Redis: GET
    alt positive hit
        Redis-->>Store: payload
        Store-->>RT: object
        RT-->>Repo: object
    else negative hit
        Redis-->>Store: empty payload
        Store-->>RT: nil,nil
        RT-->>Repo: nil
    else miss or Redis error
        RT->>DB: Load()
        DB-->>RT: object / not found
        RT-->>Repo: result first
        RT-->>Store: async Set or SetNegative
    end
```

## Entry codec/store 模型

```mermaid
classDiagram
    class CachedRepository {
      buildKey()
      load()
      invalidate()
    }
    class ReadThroughRunner {
      Read(options)
    }
    class ObjectCacheStore~T~ {
      Get(key)
      Set(key,value)
      SetNegative(key)
      Delete(key)
      Exists(key)
    }
    class CacheEntryCodec~T~ {
      Encode(value)
      Decode(raw)
    }
    class PayloadStore {
      Get(key)
      Set(key,raw,ttl)
      Delete(key)
      Exists(key)
    }
    CachedRepository --> ReadThroughRunner
    CachedRepository --> ObjectCacheStore
    ObjectCacheStore --> CacheEntryCodec
    ObjectCacheStore --> PayloadStore
```

## Async writeback

```mermaid
sequenceDiagram
    participant Caller
    participant RT as ReadThrough
    participant DB as SourceRepo
    participant Store

    Caller->>RT: Read miss
    RT->>DB: Load
    DB-->>RT: value
    RT-->>Caller: return value
    RT->>Store: async Set(value)
    Store-->>RT: best-effort result
```

## 当前 object cache 清单

| 对象 | 文件 | family |
| ---- | ---- | ------ |
| scale | [scale_cache.go](../../../internal/apiserver/infra/cache/scale_cache.go) | `static_meta` |
| questionnaire | [questionnaire_cache.go](../../../internal/apiserver/infra/cache/questionnaire_cache.go) | `static_meta` |
| assessment_detail | [assessment_detail_cache.go](../../../internal/apiserver/infra/cache/assessment_detail_cache.go) | `object_view` |
| testee | [testee_cache.go](../../../internal/apiserver/infra/cache/testee_cache.go) | `object_view` |
| plan | [plan_cache.go](../../../internal/apiserver/infra/cache/plan_cache.go) | `object_view` |

## 行为边界

- `redis.Nil` 是 miss。
- Redis error 对 read-through 主路径按 miss 降级。
- negative sentinel 是空 payload。
- nil cache/client 下，Get 返回 miss，Set/Delete no-op。
- delete invalidation 是 best-effort，但不应阻断主写流程。

## Verify

- [object_cache_store.go](../../../internal/apiserver/infra/cache/object_cache_store.go)
- [object_readthrough.go](../../../internal/apiserver/infra/cache/object_readthrough.go)
- [readthrough.go](../../../internal/apiserver/infra/cache/readthrough.go)
- [object_cache_contract_test.go](../../../internal/apiserver/infra/cache/object_cache_contract_test.go)
