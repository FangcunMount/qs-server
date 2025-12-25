# 缓存位置决策：apiserver vs collection-server

## 1. 架构概览

### 1.1 服务职责

| 服务 | 职责 | 数据访问 | 接口类型 |
|-----|------|---------|---------|
| **qs-apiserver** | 核心业务服务 | 直接访问 MySQL/MongoDB | gRPC（内部）+ REST（管理端） |
| **collection-server** | BFF 层（小程序端） | 通过 gRPC 调用 apiserver | REST（小程序） |

### 1.2 数据流向

```
小程序 → collection-server (REST) → apiserver (gRPC) → 数据库
```

## 2. 两种缓存方案对比

### 2.1 方案 A：缓存放在 apiserver 层

**架构**：
```
小程序 → collection-server → apiserver (缓存) → 数据库
```

**优点**：

1. ✅ **单一数据源**：缓存与数据源在同一层，数据一致性更容易保证
2. ✅ **缓存共享**：所有调用方（collection-server、管理端、其他服务）共享同一份缓存
3. ✅ **失效简单**：数据更新时，只需在 apiserver 层失效缓存
4. ✅ **职责清晰**：apiserver 负责数据访问和缓存，collection-server 只负责协议转换
5. ✅ **已实现**：当前量表、问卷缓存已在此层实现

**缺点**：

1. ⚠️ **gRPC 开销**：即使有缓存，collection-server 仍需通过 gRPC 调用 apiserver
2. ⚠️ **网络延迟**：collection-server 到 apiserver 的网络延迟无法避免

### 2.2 方案 B：缓存放在 collection-server 层

**架构**：
```
小程序 → collection-server (缓存) → apiserver (gRPC) → 数据库
```

**优点**：

1. ✅ **减少网络调用**：缓存命中时，无需调用 apiserver
2. ✅ **降低 apiserver 负载**：减少 gRPC 请求量
3. ✅ **更快的响应**：小程序端响应更快（减少一次网络跳转）

**缺点**：

1. ❌ **缓存不一致风险**：apiserver 更新数据时，collection-server 的缓存可能过期
2. ❌ **缓存失效复杂**：需要跨服务通知（事件/消息队列）或依赖 TTL
3. ❌ **缓存重复**：多个 collection-server 实例需要各自维护缓存
4. ❌ **职责混乱**：collection-server 作为 BFF 层，不应该承担数据缓存职责
5. ❌ **维护成本高**：需要额外的缓存失效机制

### 2.3 方案 C：两级缓存（推荐）

**架构**：
```
小程序 → collection-server (L1 本地缓存) → apiserver (L2 Redis 缓存) → 数据库
```

**优点**：

1. ✅ **最佳性能**：L1 本地缓存最快，L2 Redis 缓存共享
2. ✅ **降级策略**：L1 未命中 → L2 → 数据库
3. ✅ **减少网络调用**：L1 命中时无需调用 apiserver

**缺点**：

1. ⚠️ **实现复杂**：需要维护两级缓存的一致性
2. ⚠️ **内存占用**：L1 本地缓存占用内存

## 3. 决策建议

### 3.1 推荐方案：**方案 A（apiserver 层缓存）**

**理由**：

1. **架构清晰**：
   - apiserver 是数据源，缓存应该靠近数据源
   - collection-server 是 BFF 层，职责是协议转换，不应该承担数据缓存

2. **数据一致性**：
   - 缓存与数据源在同一层，失效机制简单可靠
   - 避免跨服务缓存不一致问题

3. **已实现**：
   - 当前量表、问卷缓存已在 apiserver 层实现
   - 保持架构一致性

4. **性能可接受**：
   - gRPC 调用延迟通常 < 10ms（同机房）
   - Redis 缓存命中时，apiserver 响应 < 5ms
   - 总延迟 < 15ms，对小程序端可接受

### 3.2 可选优化：**方案 C（两级缓存）**

**适用场景**：

- 小程序端访问量极高（> 10k QPS）
- 对响应时间要求极高（< 10ms）
- 有足够的开发资源维护两级缓存

**实现建议**：

- L1：collection-server 本地内存缓存（TTL=5分钟）
- L2：apiserver Redis 缓存（TTL=12-24小时）
- 失效：通过事件通知 L1 缓存失效

## 4. 当前实现状态

### 4.1 已实现（apiserver 层）

✅ **量表缓存**：

- 位置：`internal/apiserver/infra/cache/scale_cache.go`
- 策略：Cache-Aside，TTL=24h
- 装饰器模式：`CachedScaleRepository`

✅ **问卷缓存**：

- 位置：`internal/apiserver/infra/cache/questionnaire_cache.go`
- 策略：Cache-Aside，TTL=12h
- 装饰器模式：`CachedQuestionnaireRepository`

### 4.2 collection-server 层

⚠️ **当前状态**：

- 注释中提到"可选：缓存热点数据"
- 但未实现缓存
- 直接调用 apiserver 的 gRPC 服务

## 5. 实施建议

### 5.1 短期（当前）

**保持现状**：缓存放在 apiserver 层

**原因**：

- 架构清晰，职责明确
- 已实现，稳定运行
- 性能可接受

### 5.2 中期（如有性能瓶颈）

**监控指标**：

- collection-server → apiserver 的 gRPC 延迟
- apiserver 缓存命中率
- 小程序端响应时间

**优化方向**：

- 如果 gRPC 延迟 > 20ms：考虑优化网络或部署
- 如果缓存命中率 < 80%：优化缓存策略
- 如果响应时间 > 100ms：考虑两级缓存

### 5.3 长期（高并发场景）

**考虑方案 C（两级缓存）**：

**实现步骤**：

1. 在 collection-server 添加本地缓存（如 `groupcache` 或 `bigcache`）
2. 实现缓存失效机制（通过事件或 TTL）
3. 监控两级缓存的命中率和一致性

## 6. 总结

| 方案 | 推荐度 | 适用场景 |
|-----|-------|---------|
| **方案 A：apiserver 层** | ⭐⭐⭐⭐⭐ | 当前推荐，架构清晰，已实现 |
| **方案 B：collection-server 层** | ⭐⭐ | 不推荐，缓存一致性风险高 |
| **方案 C：两级缓存** | ⭐⭐⭐⭐ | 高并发场景可选，实现复杂 |

**最终决策**：**保持缓存放在 apiserver 层**，这是当前最佳方案。

