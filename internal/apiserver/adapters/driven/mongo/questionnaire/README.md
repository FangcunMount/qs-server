# 问卷 MongoDB 层

这个包实现了问卷领域的 MongoDB 持久化层，遵循六边形架构的出站适配器模式。

## 架构设计

### 文件结构
```
mongo/questionnaire/
├── README.md           # 使用文档
├── document.go         # MongoDB 文档结构
├── mapper.go          # 领域模型与文档的映射器
└── repository.go      # MongoDB 存储库实现
```

### 核心组件

#### 1. QuestionnaireDocument (document.go)
MongoDB 文档结构，包含：
- `DomainID`: uint64 领域模型ID，便于与MySQL等其他存储的兼容性
- `Code`: 问卷编码
- `Title`: 问卷标题  
- `ImgUrl`: 问卷图片URL
- `Version`: 版本号
- `Status`: 状态
- 基础审计字段（创建时间、更新时间等）

#### 2. QuestionnaireMapper (mapper.go)
负责领域模型与MongoDB文档之间的转换：
- `ToDocument()`: 领域模型 → MongoDB文档
- `ToDomain()`: MongoDB文档 → 领域模型
- ID转换工具方法

#### 3. Repository (repository.go)
实现 `port.QuestionnaireRepository` 接口，提供：
- 基础CRUD操作
- 按编码查询
- 软删除支持
- 业务查询方法

## 使用示例

### 初始化存储库
```go
import (
    "go.mongodb.org/mongo-driver/mongo"
    mongoQuestionnaire "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/driven/mongo/questionnaire"
)

// 假设已经有了MongoDB数据库连接
var db *mongo.Database

// 创建问卷存储库
questionnaireRepo := mongoQuestionnaire.NewRepository(db)
```

### 保存问卷
```go
questionnaire := &questionnaire.Questionnaire{
    Code:    "SURVEY001",
    Title:   "客户满意度调查",
    Version: 1,
    Status:  1, // 活跃状态
}

err := questionnaireRepo.Save(ctx, questionnaire)
if err != nil {
    // 处理错误
}
```

### 查询问卷
```go
// 按ID查询
questionnaire, err := questionnaireRepo.FindByID(ctx, 12345)

// 按编码查询
questionnaire, err := questionnaireRepo.FindByCode(ctx, "SURVEY001")
```

### 更新问卷
```go
questionnaire.Title = "更新后的标题"
err := questionnaireRepo.Update(ctx, questionnaire)
```

### 删除问卷
```go
// 软删除
err := questionnaireRepo.Remove(ctx, 12345)

// 物理删除（如果需要）
err := questionnaireRepo.HardDelete(ctx, 12345)
```

## 设计特点

### 1. ID映射策略
- 使用 `DomainID` 字段存储领域模型的 uint64 ID
- MongoDB 的 ObjectID 作为数据库层的主键
- 支持与MySQL等其他存储的ID兼容

### 2. 软删除支持
- 通过 `deleted_at` 字段实现软删除
- 查询时自动排除已删除的文档
- 提供物理删除方法用于数据清理

### 3. 审计字段
- 自动设置创建时间、更新时间
- 支持创建者、更新者、删除者追踪
- 符合企业级应用的审计要求

### 4. 索引建议
为了获得最佳性能，建议在以下字段上创建索引：
```javascript
// 在MongoDB中创建索引
db.questionnaires.createIndex({ "domain_id": 1 }, { unique: true })
db.questionnaires.createIndex({ "code": 1 }, { unique: true })
db.questionnaires.createIndex({ "status": 1 })
db.questionnaires.createIndex({ "deleted_at": 1 })
```

## 注意事项

1. **事务支持**: MongoDB支持多文档事务，如需要可以在应用层添加事务管理
2. **并发控制**: 当前实现依赖MongoDB的原子操作，复杂场景可考虑乐观锁
3. **性能优化**: 根据查询模式调整索引策略
4. **数据迁移**: 提供了ID映射机制，便于从其他数据库迁移数据

## 扩展点

- 可以添加更多业务查询方法
- 支持分页查询
- 添加聚合查询支持
- 实现批量操作
- 添加缓存层集成 