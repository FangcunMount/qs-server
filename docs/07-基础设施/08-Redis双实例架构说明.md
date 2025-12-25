# Redis 双实例架构说明

> **最后更新**：2025-01-XX  
> **相关文档**：
> - `07-全局缓存架构设计.md` - 全局缓存架构
> - `09-缓存实现总结.md` - 缓存实现总结
> - `12-Redis存储空间评估.md` - 存储空间评估

## 1. 架构设计

系统使用**双 Redis 实例架构**，分别用于不同的用途：

```
┌─────────────────────────────────────────┐
│         qs-apiserver / worker           │
└──────────────┬──────────────┬───────────┘
               │              │
        ┌──────▼──────┐  ┌───▼──────┐
        │ redis-cache │  │redis-store│
        │   (6379)    │  │  (6380)  │
        └─────────────┘  └──────────┘
```

## 2. 用途区分

### 2.1 redis-cache（缓存实例）

**定位**：临时缓存，提升读取性能

**用途**：
- ✅ **缓存数据**：量表、问卷、测评详情等（Cache-Aside 模式）
- ✅ **统计查询结果**：问卷/受试者/计划统计（TTL=5分钟）
- ✅ **事件幂等性**：防止重复处理事件（TTL=7天）
- ✅ **会话数据**：用户会话（如果使用）
- ✅ **限流数据**：限流计数器等

**特点**：
- 数据可以丢失（可以从持久层恢复）
- 使用 TTL 自动过期
- 高读写频率
- 性能优先

**当前使用场景**：
```go
// 量表缓存
scaleCache := cache.NewCachedScaleRepository(repo, redisCache)

// 统计缓存
statisticsCache := statisticsCache.NewStatisticsCache(redisCache)

// 事件幂等性
cache.IsEventProcessed(ctx, eventID)
```

### 2.2 redis-store（存储实例）

**定位**：持久化存储、队列、发布订阅

**用途**：
- ✅ **持久化存储**：需要长期保存的数据（如 CodesService 计数器）
- ✅ **消息队列**：NSQ/RabbitMQ 的替代或补充
- ✅ **发布订阅**：事件发布订阅（如果使用 Redis Pub/Sub）
- ✅ **分布式锁**：分布式锁存储
- ✅ **临时存储**：需要持久化但不需要数据库的事务性数据

**特点**：
- 数据重要，不能丢失
- 通常不使用 TTL 或 TTL 很长
- 读写频率相对较低
- 可靠性优先

**当前使用场景**：
```go
// CodesService（代码申请服务）
codesService := codesapp.NewService(redisStore)
```

## 3. 数据放置决策

### 3.1 量表、问卷数据应该放在哪里？

**答案：redis-cache**

**理由**：
1. **缓存语义**：量表、问卷数据是"缓存"，不是"持久化存储"
2. **数据源**：主要持久化存储在 MongoDB，Redis 只是缓存层
3. **可恢复性**：缓存丢失可以从 MongoDB 恢复
4. **TTL 策略**：需要设置合理的 TTL（如 24 小时）
5. **架构清晰**：符合"缓存"和"存储"的职责分离

### 3.2 不应该放在 redis-store 的原因

1. **语义混淆**：redis-store 是"存储"，不是"缓存"
2. **职责不清**：如果放到 redis-store，就失去了缓存的语义
3. **数据冗余**：MongoDB 已经是持久化存储，不需要在 Redis 中再持久化
4. **资源浪费**：redis-store 应该用于其他用途（队列、发布订阅等）

### 3.3 数据放置决策表

| 数据类型 | redis-cache | redis-store | 理由 |
|---------|------------|-------------|------|
| **量表数据** | ✅ | ❌ | 缓存，可从 MongoDB 恢复 |
| **问卷数据** | ✅ | ❌ | 缓存，可从 MongoDB 恢复 |
| **统计查询结果** | ✅ | ❌ | 临时缓存，TTL=5分钟 |
| **事件幂等性** | ✅ | ❌ | 临时数据，TTL=7天 |
| **CodesService 计数器** | ❌ | ✅ | 持久化存储，不能丢失 |
| **消息队列** | ❌ | ✅ | 持久化存储 |
| **发布订阅** | ❌ | ✅ | 持久化存储 |
| **分布式锁** | ❌ | ✅ | 持久化存储 |

## 4. 配置示例

### 4.1 开发环境

```yaml
redis:
  # Cache Redis - 缓存实例
  cache:
    host: "127.0.0.1"
    port: 6379
    database: 0  # 缓存使用 DB 0
  
  # Store Redis - 存储实例
  store:
    host: "127.0.0.1"
    port: 6380
    database: 0  # 存储使用 DB 0
```

### 4.2 生产环境

```yaml
redis:
  cache:
    host: "redis-cache.example.com"
    port: 6379
    database: 0
  
  store:
    host: "redis-store.example.com"
    port: 6380
    database: 1  # 可以使用不同的 DB
```

## 5. 最佳实践

### 5.1 缓存数据（redis-cache）

```go
// ✅ 正确：使用 redis-cache
cachedRepo := cache.NewCachedScaleRepository(
    baseRepo,
    redisCache,  // 使用 cache 实例
)

// 设置合理的 TTL
ttl := 24 * time.Hour
```

### 5.2 持久化数据（redis-store）

```go
// ✅ 正确：使用 redis-store
codesService := codesapp.NewService(redisStore)

// 不使用 TTL 或使用很长的 TTL
// 数据需要持久化保存
```

### 5.3 避免的做法

```go
// ❌ 错误：将缓存数据放到 redis-store
cachedRepo := cache.NewCachedScaleRepository(
    baseRepo,
    redisStore,  // 错误：应该用 redis-cache
)

// ❌ 错误：将持久化数据放到 redis-cache
codesService := codesapp.NewService(redisCache)  // 错误：应该用 redis-store
```

## 6. 监控建议

### 6.1 redis-cache 监控指标

- 内存使用率（可能较高，因为缓存数据）
- 命中率（Hit Rate）
- 键数量
- TTL 分布

### 6.2 redis-store 监控指标

- 内存使用率（相对较低）
- 持久化状态（AOF/RDB）
- 队列长度（如果用于队列）
- 连接数

## 7. 总结

**核心原则**：
- **redis-cache**：临时缓存，可丢失，性能优先
- **redis-store**：持久化存储，不能丢失，可靠性优先

**量表、问卷数据**：
- ✅ 应该放在 **redis-cache**
- ❌ 不应该放在 **redis-store**

**原因**：
- 符合缓存语义
- 数据可从 MongoDB 恢复
- 架构职责清晰
- 资源利用合理

