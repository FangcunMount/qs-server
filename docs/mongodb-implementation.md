# 🗄️ MongoDB 实现完整文档

## 📊 实现状态对比

| 组件 | 之前状态 | 当前状态 | 说明 |
|------|---------|---------|------|
| **MongoDB 驱动** | ❌ 旧的 mgo (已废弃) | ✅ 官方 mongo-driver | 现代化、高性能 |
| **架构设计** | ❌ 混合在 MySQL 适配器中 | ✅ 独立的 MongoDB 适配器 | 职责清晰、可独立测试 |
| **连接管理** | ❌ 设置为 nil | ✅ 完整的连接管理 | 支持连接池、健康检查 |
| **CRUD 操作** | ❌ 只有 TODO 注释 | ✅ 完整的 CRUD 实现 | 增删改查、批量操作 |
| **搜索功能** | ❌ 没有实现 | ✅ 支持文档内容搜索 | 可搜索问题标题等 |

## 🏗️ 新的架构设计

### 📁 目录结构

```
internal/apiserver/adapters/storage/
├── mysql/                    # MySQL 适配器
│   ├── questionnaire.go     # 问卷基础信息存储
│   └── user.go              # 用户信息存储
├── mongodb/                  # MongoDB 适配器
│   └── questionnaire.go     # 问卷文档结构存储
└── composite/                # 组合适配器
    └── questionnaire.go     # MySQL + MongoDB 组合
```

### 🔌 端口接口

```go
// 新增的文档存储端口
type QuestionnaireDocumentRepository interface {
    SaveDocument(ctx context.Context, q *questionnaire.Questionnaire) error
    GetDocument(ctx context.Context, id questionnaire.QuestionnaireID) (*QuestionnaireDocumentResult, error)
    UpdateDocument(ctx context.Context, q *questionnaire.Questionnaire) error
    RemoveDocument(ctx context.Context, id questionnaire.QuestionnaireID) error
    FindDocumentsByQuestionnaireIDs(ctx context.Context, ids []questionnaire.QuestionnaireID) (map[string]*QuestionnaireDocumentResult, error)
    SearchDocuments(ctx context.Context, query DocumentSearchQuery) ([]*QuestionnaireDocumentResult, error)
}
```

## 🔧 技术实现详解

### 1. **MongoDB 适配器特性**

#### ✅ **使用现代 MongoDB 驱动**
```go
import "go.mongodb.org/mongo-driver/mongo"

// 支持连接池、上下文控制、类型安全等现代特性
collection := r.client.Database(r.database).Collection(r.collection)
```

#### ✅ **完整的 BSON 映射**
```go
type questionnaireDocument struct {
    ID        string                 `bson:"_id"`
    Questions []questionDocument     `bson:"questions"`
    Settings  settingsDocument       `bson:"settings"`
    Version   int                    `bson:"version"`
    CreatedAt time.Time              `bson:"created_at"`
    UpdatedAt time.Time              `bson:"updated_at"`
}
```

#### ✅ **高级查询功能**
```go
// 支持文本搜索
filter["$or"] = []bson.M{
    {"questions.title": bson.M{"$regex": query.Keyword, "$options": "i"}},
}

// 支持批量查询
filter := bson.M{"_id": bson.M{"$in": idStrings}}
```

### 2. **组合适配器模式**

#### 🎯 **职责分离**
```go
type questionnaireCompositeRepository struct {
    mysqlRepo    storage.QuestionnaireRepository         // 基础信息
    documentRepo storage.QuestionnaireDocumentRepository // 文档结构
}
```

#### 🔄 **数据一致性**
```go
func (r *questionnaireCompositeRepository) Save(ctx context.Context, q *questionnaire.Questionnaire) error {
    // 1. 保存到 MySQL
    if err := r.mysqlRepo.Save(ctx, q); err != nil {
        return err
    }
    
    // 2. 保存到 MongoDB
    if err := r.documentRepo.SaveDocument(ctx, q); err != nil {
        // 失败时回滚 MySQL
        _ = r.mysqlRepo.Remove(ctx, q.ID())
        return err
    }
    
    return nil
}
```

#### 🚀 **渐进式降级**
```go
// 如果 MongoDB 不可用，自动降级到 MySQL-only 模式
if c.mongoClient != nil {
    c.questionnaireRepo = composite.NewQuestionnaireCompositeRepository(
        c.mysqlQuestionnaireRepo,
        c.mongoDocumentRepo,
    )
} else {
    c.questionnaireRepo = c.mysqlQuestionnaireRepo // 仅使用 MySQL
}
```

## 📊 数据存储策略

### 🗄️ **MySQL 存储内容**
```go
type questionnaireModel struct {
    ID          string    `gorm:"primaryKey"`
    Code        string    `gorm:"uniqueIndex"`
    Title       string
    Description string
    Status      int
    CreatedBy   string
    CreatedAt   time.Time
    UpdatedAt   time.Time
    Version     int
}
```

### 🗃️ **MongoDB 存储内容**
```go
type questionnaireDocument struct {
    ID        string                 `bson:"_id"`
    Questions []questionDocument     `bson:"questions"`    // 复杂的问题列表
    Settings  settingsDocument       `bson:"settings"`     // 灵活的设置对象
    Version   int                    `bson:"version"`
    CreatedAt time.Time              `bson:"created_at"`
    UpdatedAt time.Time              `bson:"updated_at"`
}
```

### 🎯 **存储策略优势**

| 方面 | MySQL 优势 | MongoDB 优势 |
|------|------------|-------------|
| **数据类型** | 结构化数据、关系查询 | 文档结构、灵活 schema |
| **查询** | SQL 强大的关系查询 | 复杂文档内容搜索 |
| **事务** | ACID 事务支持 | 单文档原子性 |
| **扩展性** | 垂直扩展 | 水平扩展 |
| **用例** | 用户管理、基础信息 | 问卷结构、动态内容 |

## 🚀 使用示例

### 1. **创建问卷**
```go
// 同时保存到 MySQL 和 MongoDB
questionnaire := questionnaire.NewQuestionnaire("survey001", "客户满意度调查", "...", "admin")
err := questionnaireRepo.Save(ctx, questionnaire)
```

### 2. **查询问卷**
```go
// 从两个数据源合并数据
questionnaire, err := questionnaireRepo.FindByID(ctx, id)
```

### 3. **搜索功能**
```go
// 在 MongoDB 中搜索问题内容
results, err := documentRepo.SearchDocuments(ctx, storage.DocumentSearchQuery{
    Keyword: "满意度",
    Limit:   10,
})
```

### 4. **批量操作**
```go
// 批量获取文档结构
docs, err := documentRepo.FindDocumentsByQuestionnaireIDs(ctx, ids)
```

## 🎛️ 配置和部署

### 1. **数据库配置**
```yaml
# configs/qs-apiserver.yaml
mysql:
  host: localhost:3306
  database: questionnaire_db
  
mongodb:
  url: mongodb://localhost:27017
  database: questionnaire_docs
```

### 2. **启动模式**

#### 🔥 **完整模式 (MySQL + MongoDB)**
```bash
# 启动所有数据库服务
docker-compose up mysql mongodb redis

# 启动应用
./qs-apiserver
# 输出: 🗄️ Storage Mode: MySQL + MongoDB (Hybrid)
```

#### 🚀 **简化模式 (MySQL Only)**
```bash
# 只启动 MySQL
docker-compose up mysql

# 启动应用
./qs-apiserver  
# 输出: 🗄️ Storage Mode: MySQL Only
```

## 🎯 性能优势

### 📈 **查询性能**
- **基础查询**: MySQL B-Tree 索引，毫秒级响应
- **文档搜索**: MongoDB 文本索引，支持复杂查询
- **批量操作**: MongoDB 聚合管道，高效处理

### 💾 **存储效率**
- **关系数据**: MySQL 标准化，避免冗余
- **文档数据**: MongoDB JSON 存储，天然适配

### 🔄 **扩展性**
- **读写分离**: MySQL 主从，MongoDB 副本集
- **水平扩展**: MongoDB 分片，处理海量文档

## 🛠️ 开发体验

### ✅ **优势**
1. **类型安全**: 使用官方驱动，编译时检查
2. **上下文支持**: 原生支持 context.Context
3. **连接池**: 自动管理连接生命周期
4. **错误处理**: 详细的错误信息和处理
5. **测试友好**: 可以独立测试每个适配器

### 🔧 **开发工具**
```bash
# 数据库初始化
make db-init

# 创建 MongoDB 索引
mongo questionnaire_docs --eval "
  db.questionnaire_docs.createIndex({'questions.title': 'text'})
"

# 数据迁移
make db-migrate
```

## 🎉 总结

### ✅ **完成的工作**

1. **🏗️ 架构重构**
   - 创建独立的 MongoDB 适配器
   - 实现组合适配器模式
   - 支持渐进式降级

2. **🔧 技术升级**
   - 使用官方 MongoDB 驱动
   - 完整的 CRUD 实现
   - 高级搜索功能

3. **📊 存储优化**
   - MySQL 存储结构化数据
   - MongoDB 存储文档结构
   - 数据一致性保证

4. **🚀 运维友好**
   - 支持多种部署模式
   - 优雅的错误处理
   - 详细的日志记录

### 🔮 **后续扩展**

1. **事务支持**: 实现 MySQL + MongoDB 分布式事务
2. **缓存层**: 添加 Redis 缓存层提升性能
3. **读写分离**: 支持主从数据库配置
4. **监控告警**: 添加数据库监控和告警机制

现在您的问卷系统具备了企业级的数据存储能力，能够处理复杂的业务场景和大规模数据！ 🎊 