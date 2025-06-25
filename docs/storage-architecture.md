# 存储架构设计文档

## 🎯 重构后的分层架构

经过重构，我们解决了 `DatabaseManager` 和 `StorageManager` 职责重叠的问题，建立了清晰的分层架构：

```
┌─────────────────────────────────────────┐
│           API Handler Layer             │  <- 路由处理函数
├─────────────────────────────────────────┤
│         StorageManager Layer            │  <- 业务存储抽象
├─────────────────────────────────────────┤
│        DatabaseManager Layer            │  <- 底层连接管理
├─────────────┬─────────────┬─────────────┤
│    MySQL    │    Redis    │   MongoDB   │  <- 实际数据库
└─────────────┴─────────────┴─────────────┘
```

## 🔧 职责分离

### 1. DatabaseManager (底层连接管理)
**职责**: 管理所有数据库的原始连接

```go
type DatabaseManager struct {
    registry *database.Registry
    config   *config.Config
}
```

**功能**:
- ✅ 初始化 MySQL、Redis、MongoDB 连接
- ✅ 使用 `pkg/database` 抽象层
- ✅ 提供原始客户端: `GetMySQLDB()`, `GetRedisClient()`, `GetMongoSession()`
- ✅ 统一的连接池管理和健康检查
- ✅ 底层连接生命周期管理

### 2. StorageManager (业务存储抽象)
**职责**: 管理业务层的存储接口

```go
type StorageManager struct {
    config       *config.Config
    dbManager    *DatabaseManager    // 👈 依赖DatabaseManager
    storeFactory store.Factory       // MySQL业务层
    analyticsStorage storage.AnalyticsStorage // Redis业务层
    documentFactory document.Factory // MongoDB业务层
}
```

**功能**:
- ✅ 使用 `internal/apiserver` 业务存储抽象
- ✅ 提供业务接口: `GetStore()`, `GetAnalyticsStorage()`, `GetDocumentStorage()`
- ✅ 依赖 DatabaseManager 提供的连接
- ✅ 业务层存储初始化和管理

## 🏗️ 重构前后对比

### 重构前的问题 ❌
```go
// 重叠的职责
DatabaseManager:
├── MySQL连接管理 (重复)
├── Redis连接管理 (重复)  
├── MongoDB连接管理 (重复)
└── 配置管理 (重复)

StorageManager:
├── MySQL连接管理 (重复)
├── Redis连接管理 (重复)
├── MongoDB连接管理 (重复)
└── 配置管理 (重复)
```

### 重构后的架构 ✅
```go
// 清晰的职责分离
DatabaseManager:
├── 底层连接管理 (唯一)
├── 连接池管理 (唯一)
├── 健康检查 (唯一)
└── 连接生命周期 (唯一)

StorageManager:
├── 业务存储抽象 (唯一)
├── 存储接口管理 (唯一)
├── 依赖DatabaseManager (委托)
└── 业务层初始化 (唯一)
```

## 🚀 使用方式

### 1. 初始化顺序
```go
// 1. 创建底层连接管理器
dbManager := NewDatabaseManager(cfg)
if err := dbManager.Initialize(); err != nil {
    log.Fatal(err)
}

// 2. 创建业务存储管理器 (依赖DatabaseManager)
storageManager := NewStorageManager(cfg, dbManager)
if err := storageManager.Initialize(); err != nil {
    log.Fatal(err)
}
```

### 2. 在API处理函数中使用
```go
func someHandler(c *gin.Context) {
    // 通过StorageManager获取业务存储接口
    
    // MySQL - 结构化业务数据
    store := storageManager.GetStore()
    if store != nil {
        userStore := store.Users()
        // 执行业务操作
    }
    
    // Redis - 缓存和分析
    analytics := storageManager.GetAnalyticsStorage()
    if analytics != nil {
        // 执行缓存操作
    }
    
    // MongoDB - 文档和日志
    docStorage := storageManager.GetDocumentStorage()
    if docStorage != nil {
        // 执行文档操作
    }
}
```

### 3. 底层数据库访问 (如果需要)
```go
func lowLevelDatabaseOperation(c *gin.Context) {
    // 通过DatabaseManager直接访问底层连接 (不推荐在业务代码中使用)
    mysqlDB, err := dbManager.GetMySQLDB()
    if err != nil {
        // 处理错误
    }
    // 直接使用GORM操作
}
```

## 🔍 健康检查

### 分层健康检查
```go
// StorageManager健康检查 (推荐)
status := storageManager.HealthCheck()
// 返回: {
//   "database_manager": "healthy",
//   "mysql_store": "connected",
//   "redis_analytics": "connected", 
//   "mongodb_documents": "connected"
// }

// DatabaseManager健康检查 (底层)
err := dbManager.HealthCheck()
// 检查所有底层连接是否可用
```

## 📊 架构优势

### ✅ 职责清晰
- **DatabaseManager**: 专注底层连接管理
- **StorageManager**: 专注业务存储抽象
- 避免了职责重叠和代码重复

### ✅ 依赖关系明确
- StorageManager 依赖 DatabaseManager
- 单向依赖，避免循环依赖
- 易于测试和模拟

### ✅ 扩展性好
- 新增数据库类型只需在 DatabaseManager 中实现
- 新增业务存储只需在 StorageManager 中实现
- 两层可以独立演进

### ✅ 易于维护
- 底层连接问题在 DatabaseManager 中解决
- 业务逻辑问题在 StorageManager 中解决
- 问题定位更加精确

## 🔧 配置管理

### 统一配置
```yaml
# configs/qs-apiserver.yaml
mysql:
  host: "127.0.0.1:3306"
  username: "root"
  password: "password"
  database: "questionnaire_scale"

redis:
  host: "127.0.0.1"
  port: 6379
  database: 0

mongodb:
  url: "mongodb://127.0.0.1:27017/questionnaire_scale"
```

### 配置使用
- **DatabaseManager**: 使用配置创建底层连接
- **StorageManager**: 使用相同配置创建业务层抽象，但复用底层连接

## 🧪 测试策略

### 单元测试
```go
// 测试DatabaseManager
func TestDatabaseManager(t *testing.T) {
    cfg := &config.Config{...}
    dm := NewDatabaseManager(cfg)
    
    err := dm.Initialize()
    assert.NoError(t, err)
    
    // 测试连接获取
    db, err := dm.GetMySQLDB()
    assert.NoError(t, err)
    assert.NotNil(t, db)
}

// 测试StorageManager
func TestStorageManager(t *testing.T) {
    // 创建模拟的DatabaseManager
    mockDBManager := &MockDatabaseManager{...}
    
    sm := NewStorageManager(cfg, mockDBManager)
    err := sm.Initialize()
    assert.NoError(t, err)
    
    // 测试业务接口
    store := sm.GetStore()
    assert.NotNil(t, store)
}
```

### 集成测试
```go
func TestFullStorageStack(t *testing.T) {
    // 测试完整的存储栈
    cfg := loadTestConfig()
    
    dbManager := NewDatabaseManager(cfg)
    err := dbManager.Initialize()
    require.NoError(t, err)
    defer dbManager.Close()
    
    storageManager := NewStorageManager(cfg, dbManager)
    err = storageManager.Initialize()
    require.NoError(t, err)
    defer storageManager.Close()
    
    // 测试端到端操作
    testEndToEndOperations(t, storageManager)
}
```

## 📈 性能考虑

### 连接复用
- DatabaseManager 管理连接池，避免重复创建连接
- StorageManager 复用底层连接，提升性能

### 资源管理
- 统一的连接生命周期管理
- 优雅的关闭流程
- 避免资源泄漏

## 🔮 未来扩展

### 新增数据库类型
1. 在 `pkg/database/databases/` 中实现新的数据库驱动
2. 在 `DatabaseManager` 中添加初始化逻辑
3. 在 `StorageManager` 中添加业务抽象层

### 新增存储模式
1. 在 `internal/apiserver/` 下创建新的存储抽象
2. 在 `StorageManager` 中集成新的存储模式
3. 复用 `DatabaseManager` 提供的底层连接

通过这种架构设计，我们实现了职责清晰、易于扩展、便于维护的存储系统！ 