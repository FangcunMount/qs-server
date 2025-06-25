# 存储系统使用指南

## 📖 概述

问卷收集&量表测评系统采用分层存储架构，支持三种数据库的协同工作：

- **MySQL** (store层) - 结构化数据持久存储
- **Redis** (storage层) - 缓存和分析数据存储  
- **MongoDB** (document层) - 文档和日志存储

## 🏗️ 存储架构

### 1. 重构后的分层设计

```
┌─────────────────────────────────────────┐
│            API Handler Layer            │  <- 路由处理函数
├─────────────────────────────────────────┤
│         StorageManager Layer            │  <- 业务存储抽象
├─────────────────────────────────────────┤
│        DatabaseManager Layer            │  <- 底层连接管理
├─────────────┬─────────────┬─────────────┤
│    MySQL    │    Redis    │   MongoDB   │  <- 实际数据库
└─────────────┴─────────────┴─────────────┘
```

**重构解决的问题**:
- ✅ 消除了 DatabaseManager 和 StorageManager 的职责重叠
- ✅ 建立了清晰的依赖关系：StorageManager 依赖 DatabaseManager
- ✅ 避免了重复的数据库连接管理

### 2. 目录结构

```
internal/apiserver/
├── database.go         # DatabaseManager - 底层连接管理
├── storage_manager.go  # StorageManager - 业务存储抽象
├── store/              # 持久化存储层 (MySQL)
│   ├── store.go        # Factory接口
│   ├── user.go         # UserStore接口
│   └── mysql/          # MySQL实现
├── storage/            # 缓存存储层 (Redis)  
│   ├── store.go        # AnalyticsStorage接口
│   └── redis/          # Redis实现
└── document/           # 文档存储层 (MongoDB)
    ├── store.go        # DocumentStorage接口
    └── mongodb/        # MongoDB实现
```

### 3. 重构后的职责分离

#### DatabaseManager (底层连接管理)
- ✅ 管理 MySQL、Redis、MongoDB 的原始连接
- ✅ 使用 `pkg/database` 抽象层
- ✅ 提供 `GetMySQLDB()`, `GetRedisClient()`, `GetMongoSession()`
- ✅ 统一的连接池和健康检查

#### StorageManager (业务存储抽象)  
- ✅ 管理业务层存储接口
- ✅ 依赖 DatabaseManager 提供的连接
- ✅ 提供 `GetStore()`, `GetAnalyticsStorage()`, `GetDocumentStorage()`
- ✅ 业务层初始化和抽象

## 🚀 使用方法

### 1. 在路由处理函数中获取存储

```go
func someHandler(c *gin.Context) {
    // 通过参数传递的storageManager获取各种存储
    
    // 获取MySQL store (业务数据)
    store := storageManager.GetStore()
    if store != nil {
        userStore := store.Users()
        // 进行用户CRUD操作
    }
    
    // 获取Redis analytics (缓存分析)
    analytics := storageManager.GetAnalyticsStorage()
    if analytics != nil {
        // 进行缓存操作
    }
    
    // 获取MongoDB document (文档日志)
    docStorage := storageManager.GetDocumentStorage()
    if docStorage != nil {
        // 进行文档操作
    }
}
```

### 2. MySQL使用示例 (结构化业务数据)

```go
func createUser(c *gin.Context) {
    store := storageManager.GetStore()
    if store == nil {
        c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Store not available"})
        return
    }
    
    // 获取用户存储接口
    userStore := store.Users()
    
    // 创建用户
    user := &v1.User{
        ObjectMeta: metav1.ObjectMeta{
            Name: "john_doe",
        },
        Nickname: "John",
        Email:    "john@example.com",
        Password: "hashedpassword",
    }
    
    ctx := c.Request.Context()
    err := userStore.Create(ctx, user, metav1.CreateOptions{})
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusCreated, gin.H{"user": user})
}

func getUser(c *gin.Context) {
    store := storageManager.GetStore()
    userStore := store.Users()
    
    username := c.Param("username")
    ctx := c.Request.Context()
    
    user, err := userStore.Get(ctx, username, metav1.GetOptions{})
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"user": user})
}
```

### 3. Redis使用示例 (缓存和分析)

```go
func cacheUserSession(c *gin.Context) {
    analytics := storageManager.GetAnalyticsStorage()
    if analytics == nil {
        c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Analytics storage not available"})
        return
    }
    
    userID := c.Param("userID")
    sessionData := `{"user_id":"` + userID + `","login_time":"2024-01-01T00:00:00Z"}`
    
    // 设置会话缓存，TTL为1小时
    err := analytics.SetKey("session:"+userID, sessionData, 3600)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "message": "Session cached successfully",
        "key":     "session:" + userID,
        "ttl":     3600,
    })
}

func getAnalyticsData(c *gin.Context) {
    analytics := storageManager.GetAnalyticsStorage()
    
    // 获取并删除分析数据集合
    data := analytics.GetAndDeleteSet("user-actions")
    
    c.JSON(http.StatusOK, gin.H{
        "analytics_data": data,
        "count":         len(data),
    })
}
```

### 4. MongoDB使用示例 (文档和日志)

```go
func logUserActivity(c *gin.Context) {
    docStorage := storageManager.GetDocumentStorage()
    if docStorage == nil {
        c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Document storage not available"})
        return
    }
    
    // 创建活动日志文档
    activityLog := map[string]interface{}{
        "type":       "user_activity",
        "user_id":    c.Param("userID"),
        "action":     c.PostForm("action"),
        "timestamp":  time.Now(),
        "ip_address": c.ClientIP(),
        "user_agent": c.GetHeader("User-Agent"),
        "details": map[string]interface{}{
            "page":     c.PostForm("page"),
            "duration": c.PostForm("duration"),
        },
    }
    
    ctx := c.Request.Context()
    err := docStorage.Insert(ctx, "activity_logs", activityLog)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"message": "Activity logged successfully"})
}

func getActivityLogs(c *gin.Context) {
    docStorage := storageManager.GetDocumentStorage()
    userID := c.Param("userID")
    
    // 查询用户活动日志
    filter := map[string]interface{}{"user_id": userID}
    
    ctx := c.Request.Context()
    logs, err := docStorage.Find(ctx, "activity_logs", filter)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "user_id": userID,
        "logs":    logs,
        "count":   len(logs),
    })
}
```

### 5. 综合使用示例 (三种数据库协同)

```go
func submitQuestionnaire(c *gin.Context) {
    var request struct {
        QuestionnaireID string                 `json:"questionnaire_id"`
        UserID         string                 `json:"user_id"`
        Answers        map[string]interface{} `json:"answers"`
    }
    
    if err := c.ShouldBindJSON(&request); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    ctx := c.Request.Context()
    
    // 1. MySQL: 保存问卷答案 (结构化数据)
    store := storageManager.GetStore()
    if store != nil {
        // responseStore := store.Responses()
        // 保存问卷回答到MySQL
        log.Infof("Saving questionnaire response to MySQL for user %s", request.UserID)
    }
    
    // 2. Redis: 缓存用户最新答题状态 (快速访问)
    analytics := storageManager.GetAnalyticsStorage()
    if analytics != nil {
        cacheKey := "user_latest_response:" + request.UserID
        cacheData := fmt.Sprintf(`{"questionnaire_id":"%s","submitted_at":"%s"}`, 
            request.QuestionnaireID, time.Now().Format(time.RFC3339))
        
        analytics.SetKey(cacheKey, cacheData, 86400) // 缓存24小时
        log.Infof("Cached latest response status for user %s", request.UserID)
    }
    
    // 3. MongoDB: 记录详细的提交日志 (审计追踪)
    docStorage := storageManager.GetDocumentStorage()
    if docStorage != nil {
        submissionLog := map[string]interface{}{
            "type":             "questionnaire_submission",
            "questionnaire_id": request.QuestionnaireID,
            "user_id":          request.UserID,
            "answers":          request.Answers,
            "timestamp":        time.Now(),
            "ip_address":       c.ClientIP(),
            "user_agent":       c.GetHeader("User-Agent"),
            "session_id":       c.GetHeader("Session-ID"),
        }
        
        docStorage.Insert(ctx, "submission_logs", submissionLog)
        log.Infof("Logged questionnaire submission for user %s", request.UserID)
    }
    
    c.JSON(http.StatusOK, gin.H{
        "message": "Questionnaire submitted successfully",
        "operations": gin.H{
            "mysql_saved":    true,
            "redis_cached":   true,
            "mongodb_logged": true,
        },
    })
}
```

## 📊 使用场景对比

| 使用场景 | MySQL (store) | Redis (storage) | MongoDB (document) |
|---------|---------------|-----------------|-------------------|
| 用户注册登录 | ✅ 用户基本信息 | ✅ 登录状态缓存 | ✅ 登录行为日志 |
| 问卷管理 | ✅ 问卷结构数据 | ✅ 热门问卷缓存 | ✅ 问卷变更历史 |
| 答卷提交 | ✅ 答案数据 | ✅ 答题进度缓存 | ✅ 提交操作日志 |
| 数据分析 | ✅ 统计汇总 | ✅ 实时计数器 | ✅ 原始分析数据 |
| 系统监控 | ✅ 错误统计 | ✅ 性能指标 | ✅ 详细错误日志 |

## 🔧 配置管理

### 配置文件示例 (configs/qs-apiserver.yaml)

```yaml
# MySQL - 结构化业务数据
mysql:
  host: "127.0.0.1:3306"
  username: "root"
  password: "password"
  database: "questionnaire_scale"
  max-idle-connections: 10
  max-open-connections: 100
  max-connection-life-time: "1h"
  log-level: 1

# Redis - 缓存和分析数据  
redis:
  host: "127.0.0.1"
  port: 6379
  password: ""
  database: 0
  max-idle: 50
  max-active: 100
  timeout: 5
  enable-cluster: false

# MongoDB - 文档和日志数据
mongodb:
  url: "mongodb://127.0.0.1:27017/questionnaire_scale"
  use-ssl: false
```

### 可选配置

如果某种数据库不需要使用，可以将关键配置留空：

```yaml
# 不使用Redis
redis:
  host: ""

# 不使用MongoDB  
mongodb:
  url: ""
```

## 🔍 健康检查和测试

### API接口

```bash
# 存储健康检查
GET /api/v1/storage-test

# MySQL使用示例
POST /api/v1/mysql-example

# Redis使用示例  
POST /api/v1/redis-example

# MongoDB使用示例
POST /api/v1/mongodb-example

# 综合使用示例
POST /api/v1/comprehensive-example
```

### 测试命令

```bash
# 检查存储连接状态
curl http://localhost:8080/api/v1/storage-test

# 测试MongoDB文档插入
curl -X POST http://localhost:8080/api/v1/mongodb-example

# 测试综合使用
curl -X POST http://localhost:8080/api/v1/comprehensive-example \
  -H "Content-Type: application/json" \
  -d '{"username":"test_user","email":"test@example.com"}'
```

## 📚 最佳实践

### 1. 数据一致性

```go
// 使用事务确保MySQL数据一致性
func createUserWithProfile(ctx context.Context, user *User, profile *Profile) error {
    store := storageManager.GetStore()
    db := store.GetDB() // 假设有这个方法
    
    return db.Transaction(func(tx *gorm.DB) error {
        if err := tx.Create(user).Error; err != nil {
            return err
        }
        profile.UserID = user.ID
        return tx.Create(profile).Error
    })
}
```

### 2. 缓存策略

```go
// 先查缓存，再查数据库
func getUserWithCache(userID string) (*User, error) {
    // 1. 先查Redis缓存
    analytics := storageManager.GetAnalyticsStorage()
    if analytics != nil {
        // 尝试从缓存获取
    }
    
    // 2. 缓存未命中，查MySQL
    store := storageManager.GetStore()
    user, err := store.Users().Get(ctx, userID, metav1.GetOptions{})
    if err != nil {
        return nil, err
    }
    
    // 3. 更新缓存
    if analytics != nil {
        userData, _ := json.Marshal(user)
        analytics.SetKey("user:"+userID, string(userData), 3600)
    }
    
    return user, nil
}
```

### 3. 错误处理

```go
func robustOperation(c *gin.Context) {
    // 主业务逻辑使用MySQL (必须成功)
    store := storageManager.GetStore()
    if store == nil {
        c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Database unavailable"})
        return
    }
    
    // 执行核心操作
    result, err := performCoreOperation(store)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    // 可选操作1: Redis缓存 (失败不影响主流程)
    if analytics := storageManager.GetAnalyticsStorage(); analytics != nil {
        if err := cacheResult(analytics, result); err != nil {
            log.Warnf("Failed to cache result: %v", err)
        }
    }
    
    // 可选操作2: MongoDB日志 (失败不影响主流程)  
    if docStorage := storageManager.GetDocumentStorage(); docStorage != nil {
        if err := logOperation(docStorage, result); err != nil {
            log.Warnf("Failed to log operation: %v", err)
        }
    }
    
    c.JSON(http.StatusOK, result)
}
```

### 4. 性能优化

```go
// 并发操作多个存储
func parallelStorageOperations(data interface{}) error {
    var wg sync.WaitGroup
    var mu sync.Mutex
    var errors []error
    
    // 并发写入Redis和MongoDB
    wg.Add(2)
    
    // Redis操作
    go func() {
        defer wg.Done()
        if analytics := storageManager.GetAnalyticsStorage(); analytics != nil {
            if err := analytics.SetKey("key", "data", 3600); err != nil {
                mu.Lock()
                errors = append(errors, err)
                mu.Unlock()
            }
        }
    }()
    
    // MongoDB操作
    go func() {
        defer wg.Done()
        if docStorage := storageManager.GetDocumentStorage(); docStorage != nil {
            if err := docStorage.Insert(context.Background(), "collection", data); err != nil {
                mu.Lock()
                errors = append(errors, err)
                mu.Unlock()
            }
        }
    }()
    
    wg.Wait()
    
    if len(errors) > 0 {
        return fmt.Errorf("storage operations failed: %v", errors)
    }
    return nil
}
```

## 🔧 故障排除

### 常见问题

1. **连接失败**: 检查配置文件和网络连接
2. **权限错误**: 确认数据库用户权限
3. **性能问题**: 调整连接池大小
4. **数据一致性**: 使用事务和重试机制

### 监控指标

- 连接池使用率
- 响应时间
- 错误率
- 缓存命中率

通过这套存储系统，你可以根据不同的业务需求选择合适的存储方案，实现高性能、高可用的问卷收集&量表测评系统！ 