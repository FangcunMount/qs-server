# 数据协调重构：从存储层到应用服务层

## 重构背景

在之前的架构中，我们在存储层（`adapters/storage/composite/`）实现了组合多个数据源（MySQL + MongoDB）的逻辑。这种设计违反了六边形架构的分层原则和单一职责原则。

## 问题分析

### 🚨 原有架构问题

#### 1. **存储层职责越界**
```go
// ❌ 存储层在做应用服务层的工作
func (r *questionnaireCompositeRepository) Save(ctx context.Context, q *questionnaire.Questionnaire) error {
    // 1. 保存基础信息到 MySQL
    if err := r.mysqlRepo.Save(ctx, q); err != nil {
        return fmt.Errorf("failed to save questionnaire to MySQL: %w", err)
    }

    // 2. 保存文档结构到 MongoDB
    if err := r.documentRepo.SaveDocument(ctx, q); err != nil {
        // 如果 MongoDB 失败，尝试回滚 MySQL（简单实现）
        _ = r.mysqlRepo.Remove(ctx, q.ID())  // ❌ 事务逻辑
        return fmt.Errorf("failed to save questionnaire document to MongoDB: %w", err)
    }
}
```

#### 2. **违反单一职责原则**
- **存储层应该**：专注于与特定数据源的CRUD操作
- **实际在做**：协调多个数据源、事务管理、数据合并

#### 3. **架构层次混乱**
- 存储层包含业务编排逻辑
- 事务管理散布在基础设施层
- 数据一致性逻辑缺乏统一管理

## 重构方案

### ✅ 新的架构设计

#### 1. **存储层职责清晰**
```go
// ✅ 存储层专注于单一数据源
type mysqlQuestionnaireRepository struct {
    // 只负责MySQL操作
}

type mongoQuestionnaireRepository struct {
    // 只负责MongoDB操作
}
```

#### 2. **应用服务层数据协调**
```go
// ✅ 应用层负责多数据源协调
type DataCoordinator struct {
    mysqlRepo    storage.QuestionnaireRepository
    documentRepo storage.QuestionnaireDocumentRepository
}

// 应用层处理业务规则和事务逻辑
func (c *DataCoordinator) SaveQuestionnaire(ctx context.Context, q *questionnaire.Questionnaire) error {
    // 业务规则：先保存基础信息，再保存文档结构
    
    // 1. 保存基础信息到 MySQL
    if err := c.mysqlRepo.Save(ctx, q); err != nil {
        return fmt.Errorf("failed to save questionnaire basic info: %w", err)
    }

    // 2. 保存文档结构到 MongoDB
    if err := c.documentRepo.SaveDocument(ctx, q); err != nil {
        // 应用层处理事务一致性：回滚MySQL操作
        if rollbackErr := c.mysqlRepo.Remove(ctx, q.ID()); rollbackErr != nil {
            return fmt.Errorf("failed to save document and rollback failed: original=%w, rollback=%w", err, rollbackErr)
        }
        return fmt.Errorf("failed to save questionnaire document: %w", err)
    }

    return nil
}
```

## 架构对比

### 重构前
```
┌─────────────────────────────────────────────┐
│                应用服务层                      │
│  ┌─────────────────────────────────────────┐ │
│  │        QuestionnaireService            │ │
│  └─────────────────────────────────────────┘ │
└─────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────┐
│                 存储层                       │
│  ┌─────────────────────────────────────────┐ │
│  │    CompositeRepository                 │ │ ❌ 职责越界
│  │  ┌─────────────┐  ┌─────────────────┐  │ │
│  │  │ MySQLRepo   │  │ MongoRepo       │  │ │
│  │  └─────────────┘  └─────────────────┘  │ │
│  └─────────────────────────────────────────┘ │
└─────────────────────────────────────────────┘
```

### 重构后
```
┌─────────────────────────────────────────────┐
│                应用服务层                      │
│  ┌─────────────────────────────────────────┐ │
│  │        QuestionnaireService            │ │ ✅ 业务协调
│  │                                       │ │
│  │  ┌─────────────────────────────────────┐ │ │
│  │  │       DataCoordinator              │ │ │
│  │  └─────────────────────────────────────┘ │ │
│  └─────────────────────────────────────────┘ │
└─────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────┐
│                 存储层                       │
│  ┌─────────────┐        ┌─────────────────┐  │
│  │ MySQLRepo   │        │ MongoRepo       │  │ ✅ 职责单一
│  └─────────────┘        └─────────────────┘  │
└─────────────────────────────────────────────┘
```

## 重构实现

### 1. **DataCoordinator 核心功能**

#### 数据保存协调
- 先保存MySQL基础信息
- 再保存MongoDB文档结构
- 失败时自动回滚

#### 数据查询合并
- 从MySQL获取基础信息
- 从MongoDB获取文档结构
- 应用层合并数据

#### 数据一致性检查
- 检查两个数据源的数据一致性
- 提供数据修复功能

### 2. **问卷服务增强**

#### 双构造函数设计
```go
// 多数据源模式
func NewService(
    mysqlRepo storage.QuestionnaireRepository,
    mongoRepo storage.QuestionnaireDocumentRepository,
) *Service

// 单数据源模式（向后兼容）
func NewServiceWithSingleRepo(questionnaireRepo storage.QuestionnaireRepository) *Service
```

#### 新增数据一致性方法
- `CheckQuestionnaireDataConsistency()` - 检查数据一致性
- `RepairQuestionnaireData()` - 修复数据不一致
- `GetCompleteQuestionnaire()` - 获取完整问卷（包含文档）

### 3. **文件结构变化**

#### 删除文件
- `internal/apiserver/adapters/storage/composite/questionnaire.go` (162行)

#### 新增文件
- `internal/apiserver/application/questionnaire/coordinator.go` (189行)

#### 修改文件
- `internal/apiserver/application/questionnaire/service.go` (增强315行 → 398行)

## 架构优势

### 1. **清晰的职责分离**
- **存储层**：专注单一数据源CRUD
- **应用层**：负责业务协调和事务管理
- **领域层**：保持纯粹的业务规则

### 2. **更好的可测试性**
- DataCoordinator可以独立测试
- 存储层适配器更容易模拟
- 业务逻辑与基础设施解耦

### 3. **增强的扩展性**
- 易于添加新的数据源
- 支持不同的数据协调策略
- 灵活的事务管理机制

### 4. **数据一致性保障**
- 统一的数据一致性检查
- 自动化数据修复机制
- 清晰的错误处理和回滚逻辑

## 使用示例

### 多数据源模式
```go
// 创建服务
mysqlRepo := mysql.NewQuestionnaireRepository(db)
mongoRepo := mongodb.NewQuestionnaireRepository(mongoClient)
service := questionnaire.NewService(mysqlRepo, mongoRepo)

// 使用数据一致性功能
consistency, err := service.CheckQuestionnaireDataConsistency(ctx, "questionnaire-id")
if consistency["status"] != "consistent" {
    err = service.RepairQuestionnaireData(ctx, "questionnaire-id")
}

// 获取完整问卷
completeQ, err := service.GetCompleteQuestionnaire(ctx, "questionnaire-id")
```

### 单数据源模式（向后兼容）
```go
// 创建服务
repo := mysql.NewQuestionnaireRepository(db)
service := questionnaire.NewServiceWithSingleRepo(repo)

// 正常使用，无数据协调功能
q, err := service.GetQuestionnaire(ctx, query)
```

## 总结

这次重构将数据协调逻辑从存储层移到了应用服务层，**实现了更符合六边形架构原则的设计**：

1. **存储层职责单一**：每个适配器专注于单一数据源
2. **应用层业务协调**：DataCoordinator处理跨数据源的业务逻辑
3. **清晰的架构边界**：各层职责明确，易于维护和扩展
4. **企业级特性**：数据一致性保障、事务管理、错误处理

这是一个**从技术债务到架构优雅**的典型重构案例，体现了六边形架构和DDD设计的核心价值。 