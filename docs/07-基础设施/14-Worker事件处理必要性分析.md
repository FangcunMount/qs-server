# Worker 事件处理必要性分析

## 1. 问题背景

当前系统存在**双重缓存失效机制**：

1. **Repository 层自动失效**：`CachedScaleRepository`、`CachedQuestionnaireRepository` 在 Create/Update/Remove 时自动失效缓存
2. **Worker 事件处理失效**：Worker 处理 `scale.published`、`questionnaire.published` 等事件时也失效缓存

**问题**：Worker 中的缓存失效是否还有必要？是否存在冗余？

## 2. 当前实现分析

### 2.1 Repository 层缓存失效

**量表缓存**（`scale_cache.go`）：

```go
// Create 创建量表（同时写入缓存）
func (r *CachedScaleRepository) Create(ctx context.Context, domain *scale.MedicalScale) error {
    if err := r.repo.Create(ctx, domain); err != nil {
        return err
    }
    // 创建成功后写入缓存
    if r.client != nil {
        if err := r.setCache(ctx, domain.GetCode().String(), domain); err != nil {
            // 缓存写入失败不影响创建
        }
    }
    return nil
}

// Update 更新量表（同时失效缓存）
func (r *CachedScaleRepository) Update(ctx context.Context, domain *scale.MedicalScale) error {
    if err := r.repo.Update(ctx, domain); err != nil {
        return err
    }
    // 更新成功后失效缓存
    if r.client != nil {
        r.deleteCache(ctx, domain.GetCode().String())
    }
    return nil
}
```

**问卷缓存**（`questionnaire_cache.go`）：

```go
// Update 更新问卷（同时失效缓存）
func (r *CachedQuestionnaireRepository) Update(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
    if err := r.repo.Update(ctx, qDomain); err != nil {
        return err
    }
    // 更新成功后失效缓存（删除所有版本的缓存）
    if r.client != nil {
        r.deleteCacheByCode(ctx, code)
    }
    return nil
}
```

### 2.2 Worker 事件处理

**量表事件**（`scale_handler.go`）：
- `handleScalePublished`：当前**不失效缓存**（注释说明采用 Lazy Loading）
- `handleScaleUnpublished`：失效缓存
- `handleScaleUpdated`：失效缓存
- `handleScaleArchived`：清除所有版本缓存

**问卷事件**（`questionnaire_handler.go`）：
- `handleQuestionnairePublished`：当前**不失效缓存**（注释说明采用 Lazy Loading）
- `handleQuestionnaireUnpublished`：失效缓存
- `handleQuestionnaireArchived`：清除所有版本缓存

## 3. 事件的其他用途

### 3.1 事件消费者（来自 `configs/events.yaml`）

**问卷发布事件**（`questionnaire.published`）：
- `collection-server`：可能需要更新自己的缓存或索引
- `search-service`：需要更新搜索索引

**量表发布事件**（`scale.published`）：
- `collection-server`：可能需要更新自己的缓存或索引
- `qs-worker`：处理缓存失效（但当前未实现）

**问卷/量表下架/归档事件**：
- `collection-server`：更新缓存或索引
- `search-service`：更新搜索索引
- `qs-worker`：失效缓存

### 3.2 事件的其他价值

1. **跨服务通知**：通知 collection-server、search-service 等外部服务
2. **异步解耦**：事件处理是异步的，不阻塞主流程
3. **防御性编程**：如果 Repository 层失效失败，Worker 可以作为兜底
4. **审计日志**：事件处理可以记录操作日志
5. **未来扩展**：可以添加其他业务逻辑（如通知、统计等）

## 4. 必要性分析

### 4.1 缓存失效的必要性

**当前情况**：
- ✅ Repository 层已自动失效缓存（同步）
- ✅ Worker 层也失效缓存（异步，作为兜底）

**分析**：

| 场景 | Repository 失效 | Worker 失效 | 必要性 |
|-----|---------------|------------|--------|
| **正常情况** | ✅ 已失效 | ⚠️ 冗余 | 不必要 |
| **Repository 失效失败** | ❌ 失败 | ✅ 兜底 | **必要** |
| **跨服务缓存** | ❌ 只失效 apiserver | ✅ 失效所有服务 | **必要** |
| **异步场景** | ✅ 同步失效 | ✅ 异步兜底 | 可选 |

### 4.2 事件本身的价值

**事件必须保留**，原因：

1. **跨服务通信**：collection-server、search-service 需要这些事件
2. **架构解耦**：事件驱动架构，服务间松耦合
3. **业务扩展**：未来可能需要添加其他业务逻辑

## 5. 优化建议

### 5.1 当前实现的问题

1. **冗余失效**：Repository 层已失效，Worker 再次失效是冗余的
2. **不一致**：Published 事件不失效，但 Unpublished/Archived 失效
3. **职责不清**：缓存失效应该在 Repository 层还是 Worker 层？

### 5.2 推荐方案

#### 方案 A：移除 Worker 中的缓存失效（推荐）

**理由**：
1. Repository 层已自动失效，Worker 失效是冗余的
2. 职责清晰：Repository 层负责数据访问和缓存，Worker 负责其他业务逻辑
3. 减少代码复杂度

**实施**：
- 移除 Worker 中的缓存失效逻辑
- 保留事件处理（用于通知其他服务）
- 如果 Repository 失效失败，可以通过监控告警发现

#### 方案 B：保留 Worker 作为兜底（保守）

**理由**：
1. 防御性编程：如果 Repository 失效失败，Worker 可以兜底
2. 跨服务场景：如果其他服务也有缓存，Worker 可以统一失效

**实施**：
- 保留 Worker 中的缓存失效逻辑
- 添加幂等性检查（避免重复失效）
- 添加日志记录（便于排查问题）

#### 方案 C：只保留关键事件的失效（折中）

**理由**：
- Published 事件：不失效（采用 Lazy Loading）
- Unpublished/Archived 事件：失效（确保下架/归档后立即失效）

**实施**：
- 保持当前实现
- 明确注释说明原因

## 6. 最终建议

### 6.1 短期（推荐）

**移除 Worker 中的缓存失效逻辑**，原因：

1. ✅ **职责清晰**：Repository 层负责缓存，Worker 负责其他业务逻辑
2. ✅ **减少冗余**：避免双重失效
3. ✅ **简化代码**：减少维护成本
4. ✅ **事件保留**：事件本身仍有价值（通知其他服务）

**保留的内容**：
- ✅ 事件发布（用于通知其他服务）
- ✅ 事件处理框架（用于未来扩展）
- ✅ 其他业务逻辑（如日志、统计等）

### 6.2 长期（可选）

**如果确实需要 Worker 失效缓存**，考虑：

1. **统一失效接口**：通过 gRPC 或 HTTP 调用 apiserver 的缓存失效接口
2. **幂等性保证**：确保重复失效不会出错
3. **监控告警**：监控 Repository 层失效失败的情况

## 7. 总结

### 7.1 事件本身的价值 ✅

**必须保留**：
- 跨服务通知（collection-server、search-service）
- 架构解耦（事件驱动）
- 业务扩展（未来可能添加其他逻辑）

### 7.2 Worker 缓存失效的必要性 ❌

**已移除**（2025-01-XX）：
- Repository 层已自动失效
- Worker 失效是冗余的
- 职责应该清晰分离

### 7.3 当前状态

1. ✅ **事件已保留**：事件本身有价值，必须保留
2. ✅ **Worker 缓存失效已移除**：简化代码，职责清晰
3. ⏳ **监控待添加**：监控 Repository 层失效失败的情况
4. ✅ **文档已更新**：明确说明缓存失效的职责归属

**实现状态**：Worker 中的缓存失效逻辑已移除，事件处理仅用于通知其他服务。

