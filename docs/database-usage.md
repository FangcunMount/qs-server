# 数据库使用指南

## 概述

问卷收集&量表测评系统支持同时连接MySQL、Redis、MongoDB三种数据库。每种数据库都有其特定的用途：

- **MySQL**: 主要用于存储结构化数据（问卷、用户、答卷等）
- **Redis**: 用于缓存、会话存储、计数器等
- **MongoDB**: 用于存储非结构化数据（日志、统计数据等）

## 配置

### 1. 配置文件设置

在 `configs/qs-apiserver.yaml` 中配置数据库连接信息：

```yaml
# MySQL 数据库配置
mysql:
  host: "127.0.0.1:3306"
  username: "root"
  password: "your_password"
  database: "questionnaire_scale"
  max-idle-connections: 10
  max-open-connections: 100
  max-connection-life-time: "1h"
  log-level: 1

# Redis 数据库配置
redis:
  host: "127.0.0.1"
  port: 6379
  password: ""
  database: 0
  max-idle: 50
  max-active: 100
  timeout: 5
  enable-cluster: false

# MongoDB 数据库配置
mongodb:
  url: "mongodb://127.0.0.1:27017/questionnaire_scale"
  use-ssl: false
```

### 2. 可选配置

如果某个数据库不需要使用，可以将其配置留空：

```yaml
# 不使用Redis，留空host配置
redis:
  host: ""

# 不使用MongoDB，留空url配置
mongodb:
  url: ""
```

## 在代码中使用数据库

### 1. 获取数据库连接

在路由处理函数中，通过 `DatabaseManager` 获取数据库连接：

```go
func someHandler(c *gin.Context) {
    // 获取数据库管理器 (需要通过依赖注入或全局变量)
    dbManager := getDBManager() // 这里需要根据你的架构实现
    
    // 获取MySQL连接
    mysqlDB, err := dbManager.GetMySQLDB()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    // 获取Redis连接
    redisClient, err := dbManager.GetRedisClient()
    if err != nil {
        // Redis可能未配置，可以选择忽略或返回错误
        log.Warnf("Redis not available: %v", err)
    }
    
    // 获取MongoDB连接
    mongoSession, err := dbManager.GetMongoSession()
    if err != nil {
        // MongoDB可能未配置，可以选择忽略或返回错误
        log.Warnf("MongoDB not available: %v", err)
    }
    
    // 使用数据库连接进行操作
    // ...
}
```

### 2. MySQL 使用示例

```go
func createUser(c *gin.Context) {
    db, err := dbManager.GetMySQLDB()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not available"})
        return
    }
    
    // 使用GORM进行数据库操作
    user := &User{
        Username: "test_user",
        Email:    "test@example.com",
    }
    
    if err := db.Create(user).Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, user)
}
```

### 3. Redis 使用示例

```go
func cacheData(c *gin.Context) {
    redisClient, err := dbManager.GetRedisClient()
    if err != nil {
        // 如果Redis不可用，可以选择跳过缓存或返回错误
        log.Warnf("Redis not available: %v", err)
        c.JSON(http.StatusOK, gin.H{"message": "Data processed without cache"})
        return
    }
    
    // 设置缓存
    err = redisClient.Set("user:123", "user_data", time.Hour).Err()
    if err != nil {
        log.Errorf("Redis set error: %v", err)
    }
    
    // 获取缓存
    val, err := redisClient.Get("user:123").Result()
    if err != nil {
        log.Errorf("Redis get error: %v", err)
    }
    
    c.JSON(http.StatusOK, gin.H{"cached_data": val})
}
```

### 4. MongoDB 使用示例

```go
func logActivity(c *gin.Context) {
    mongoSession, err := dbManager.GetMongoSession()
    if err != nil {
        // MongoDB可能未配置，可以选择忽略日志
        log.Warnf("MongoDB not available: %v", err)
        c.JSON(http.StatusOK, gin.H{"message": "Activity logged to file"})
        return
    }
    
    // 使用MongoDB存储日志
    session := mongoSession.Copy()
    defer session.Close()
    
    collection := session.DB("questionnaire_scale").C("activity_logs")
    
    logEntry := map[string]interface{}{
        "user_id":   "123",
        "activity":  "login",
        "timestamp": time.Now(),
        "ip":        c.ClientIP(),
    }
    
    if err := collection.Insert(logEntry); err != nil {
        log.Errorf("MongoDB insert error: %v", err)
    }
    
    c.JSON(http.StatusOK, gin.H{"message": "Activity logged"})
}
```

## 健康检查

系统提供了数据库健康检查接口：

```bash
# 检查所有数据库连接状态
curl http://localhost:8080/health/db

# 测试数据库连接
curl http://localhost:8080/api/v1/db-test
```

## 最佳实践

### 1. 错误处理

```go
func robustDatabaseOperation(c *gin.Context) {
    // 主数据库操作（MySQL）
    db, err := dbManager.GetMySQLDB()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Database unavailable"})
        return
    }
    
    // 执行核心业务逻辑
    result, err := performCoreOperation(db)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    // 可选的缓存操作（Redis）
    if redisClient, err := dbManager.GetRedisClient(); err == nil {
        cacheResult(redisClient, result)
    }
    
    // 可选的日志记录（MongoDB）
    if mongoSession, err := dbManager.GetMongoSession(); err == nil {
        logOperation(mongoSession, "operation_completed", result)
    }
    
    c.JSON(http.StatusOK, result)
}
```

### 2. 事务处理

```go
func transactionExample(c *gin.Context) {
    db, err := dbManager.GetMySQLDB()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Database unavailable"})
        return
    }
    
    // 开启事务
    tx := db.Begin()
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
        }
    }()
    
    // 执行数据库操作
    if err := tx.Create(&User{Username: "user1"}).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    if err := tx.Create(&Profile{UserID: 1, Name: "User 1"}).Error; err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    // 提交事务
    if err := tx.Commit().Error; err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"message": "Transaction completed"})
}
```

### 3. 连接池管理

数据库连接池由配置文件管理，建议根据实际负载调整：

```yaml
mysql:
  max-idle-connections: 10    # 根据并发量调整
  max-open-connections: 100   # 根据数据库服务器配置调整
  max-connection-life-time: "1h"  # 连接重用时间

redis:
  max-idle: 50      # Redis空闲连接数
  max-active: 100   # Redis最大连接数
  timeout: 5        # 连接超时时间
```

## 故障排除

### 常见问题

1. **数据库连接失败**
   - 检查配置文件中的连接参数
   - 确保数据库服务正在运行
   - 检查网络连接和防火墙设置

2. **连接池耗尽**
   - 调整 `max-open-connections` 参数
   - 检查是否有连接泄漏（未正确关闭连接）

3. **Redis连接问题**
   - 确保Redis服务运行正常
   - 检查Redis配置文件中的 `bind` 和 `protected-mode` 设置

### 日志查看

```bash
# 查看数据库相关日志
tail -f /data/logs/qs/qs-apiserver.log | grep -i "database\|mysql\|redis\|mongodb"
```

## 扩展功能

### 1. 添加新的数据库类型
### 2. 实现数据库连接监控
### 3. 添加数据库迁移功能
### 4. 实现读写分离 