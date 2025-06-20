# 数据库连接注册器

## 概述

`pkg/database` 包提供了一个基于注册器模式的数据库连接管理器，支持按需注册不同类型的数据库连接。这种设计提供了更大的灵活性和可扩展性。

## 设计理念

### 🎯 **注册器模式的优势**

1. **按需注册**: 只注册需要的数据库类型，避免不必要的连接
2. **灵活配置**: 可以根据配置或环境变量决定使用哪些数据库
3. **易于扩展**: 新增数据库类型只需要实现 `Connection` 接口
4. **条件使用**: 组件可以检查数据库是否可用，进行条件性操作

### 🏗️ **架构特点**

1. **接口抽象**: 通过 `Connection` 接口统一不同数据库的操作
2. **注册机制**: 使用注册器管理所有数据库连接
3. **类型安全**: 通过 `DatabaseType` 枚举确保类型安全
4. **线程安全**: 注册器内部使用读写锁保证并发安全

## 核心组件

### 1. **Connection 接口**

```go
type Connection interface {
    Type() DatabaseType        // 返回数据库类型
    Connect() error           // 建立连接
    Close() error            // 关闭连接
    HealthCheck(ctx context.Context) error // 健康检查
    GetClient() interface{}  // 获取原始客户端
}
```

### 2. **Registry 注册器**

```go
type Registry struct {
    connections map[DatabaseType]Connection
    configs     map[DatabaseType]interface{}
    initialized bool
}
```

### 3. **DatabaseType 枚举**

```go
type DatabaseType string

const (
    MySQL   DatabaseType = "mysql"
    Redis   DatabaseType = "redis"
    MongoDB DatabaseType = "mongodb"
    Etcd    DatabaseType = "etcd"
)
```

## 使用方式

### 1. **基本使用流程**

```go
// 1. 创建注册器
registry := database.NewRegistry()

// 2. 注册需要的数据库
mysqlConfig := &database.MySQLConfig{...}
mysqlConn := database.NewMySQLConnection(mysqlConfig)
registry.Register(database.MySQL, mysqlConfig, mysqlConn)

redisConfig := &database.RedisConfig{...}
redisConn := database.NewRedisConnection(redisConfig)
registry.Register(database.Redis, redisConfig, redisConn)

// 3. 初始化所有连接
registry.Init()

// 4. 在组件中使用
mysqlClient, _ := registry.GetClient(database.MySQL)
redisClient, _ := registry.GetClient(database.Redis)

// 5. 优雅关闭
defer registry.Close()
```

### 2. **条件性注册**

```go
registry := database.NewRegistry()

// 必需数据库
registry.Register(database.MySQL, mysqlConfig, mysqlConn)
registry.Register(database.Redis, redisConfig, redisConn)

// 可选数据库（根据配置决定）
if shouldUseMongoDB() {
    registry.Register(database.MongoDB, mongoConfig, mongoConn)
}

// 初始化
registry.Init()
```

### 3. **组件中的使用**

```go
type MyComponent struct {
    registry *database.Registry
}

func (c *MyComponent) DoWork() error {
    // 检查 MySQL 是否可用
    if c.registry.IsRegistered(database.MySQL) {
        client, err := c.registry.GetClient(database.MySQL)
        if err == nil {
            if db, ok := client.(*gorm.DB); ok {
                // 使用 MySQL
                db.Create(&User{...})
            }
        }
    }

    // 检查 Redis 是否可用
    if c.registry.IsRegistered(database.Redis) {
        client, err := c.registry.GetClient(database.Redis)
        if err == nil {
            if redisClient, ok := client.(redis.UniversalClient); ok {
                // 使用 Redis
                redisClient.Set("key", "value", time.Hour)
            }
        }
    }

    return nil
}
```

## 配置说明

### MySQL 配置

```go
type MySQLConfig struct {
    Host                  string        // 数据库主机地址
    Username              string        // 用户名
    Password              string        // 密码
    Database              string        // 数据库名
    MaxIdleConnections    int           // 最大空闲连接数
    MaxOpenConnections    int           // 最大打开连接数
    MaxConnectionLifeTime time.Duration // 连接最大存活时间
    LogLevel              int           // 日志级别
}
```

### Redis 配置

```go
type RedisConfig struct {
    Host                  string   // Redis 主机地址
    Port                  int      // Redis 端口
    Addrs                 []string // Redis 地址列表（集群模式）
    Password              string   // Redis 密码
    Database              int      // Redis 数据库编号
    MaxIdle               int      // 最大空闲连接数
    MaxActive             int      // 最大活跃连接数
    Timeout               int      // 连接超时时间
    EnableCluster         bool     // 是否启用集群模式
    UseSSL                bool     // 是否使用 SSL
    SSLInsecureSkipVerify bool     // 是否跳过 SSL 验证
}
```

### MongoDB 配置

```go
type MongoConfig struct {
    URL                      string // MongoDB 连接 URL
    UseSSL                   bool   // 是否使用 SSL
    SSLInsecureSkipVerify    bool   // 是否跳过 SSL 验证
    SSLAllowInvalidHostnames bool   // 是否允许无效主机名
    SSLCAFile                string // SSL CA 证书文件
    SSLPEMKeyfile            string // SSL PEM 密钥文件
}
```

## 高级功能

### 1. **健康检查**

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := registry.HealthCheck(ctx); err != nil {
    log.Printf("Health check failed: %v", err)
}
```

### 2. **查看已注册的数据库**

```go
registered := registry.ListRegistered()
log.Printf("Registered databases: %v", registered)

// 检查特定数据库是否已注册
if registry.IsRegistered(database.MySQL) {
    log.Println("MySQL is registered")
}
```

### 3. **扩展新的数据库类型**

```go
// 1. 定义新的数据库类型
const (
    PostgreSQL DatabaseType = "postgresql"
)

// 2. 实现 Connection 接口
type PostgreSQLConnection struct {
    config *PostgreSQLConfig
    client *gorm.DB
}

func (p *PostgreSQLConnection) Type() DatabaseType {
    return PostgreSQL
}

func (p *PostgreSQLConnection) Connect() error {
    // 实现连接逻辑
    return nil
}

// ... 实现其他接口方法

// 3. 注册使用
postgresConfig := &PostgreSQLConfig{...}
postgresConn := NewPostgreSQLConnection(postgresConfig)
registry.Register(PostgreSQL, postgresConfig, postgresConn)
```

## 最佳实践

### 1. **注册时机**

- 在应用程序启动时注册所有需要的数据库
- 在初始化之前完成所有注册操作
- 注册后立即进行初始化

### 2. **错误处理**

- 始终检查注册和初始化的错误
- 在组件中检查数据库是否可用
- 实现适当的降级策略

### 3. **资源管理**

- 使用 defer 确保注册器正确关闭
- 监控数据库连接的健康状态
- 实现连接池的合理配置

### 4. **配置管理**

- 使用环境变量或配置文件管理数据库配置
- 支持不同环境的配置切换
- 实现配置验证

## 与原有架构的对比

### **原有架构的问题**

1. **硬编码依赖**: 每个组件都硬编码了数据库连接逻辑
2. **代码重复**: 相同的连接逻辑在多个地方重复
3. **配置分散**: 数据库配置分散在各个组件中
4. **扩展困难**: 新增数据库类型需要修改多个组件

### **注册器模式的优势**

1. **解耦**: 组件与具体的数据库实现解耦
2. **复用**: 统一的连接管理，避免重复代码
3. **灵活**: 按需注册，支持条件性使用
4. **可扩展**: 新增数据库类型只需要实现接口

## 迁移指南

### 1. **从原有架构迁移**

```go
// 原有代码
func GetMySQLFactoryOr(opts *genericoptions.MySQLOptions) (store.Factory, error) {
    options := &db.Options{...}
    dbIns, err = db.New(options)
    return &datastore{dbIns}, nil
}

// 新代码
registry := database.GetManager()
mysqlClient, err := registry.GetClient(database.MySQL)
if err != nil {
    return nil, err
}
```

### 2. **配置迁移**

```yaml
# 原有配置
mysql:
  host: 127.0.0.1:3306
  username: iam
  password: iam59!z$

# 新配置
database:
  mysql:
    host: 127.0.0.1:3306
    username: iam
    password: iam59!z$
  redis:
    host: 127.0.0.1
    port: 6379
    password: iam59!z$
```

## 注意事项

1. **线程安全**: 注册器是线程安全的，但客户端使用需要自行保证
2. **初始化顺序**: 必须先注册再初始化
3. **资源清理**: 确保在应用关闭时正确关闭注册器
4. **类型断言**: 使用 `GetClient()` 后需要进行类型断言

## 未来改进

1. **连接池监控**: 添加连接池使用情况的监控指标
2. **自动重连**: 实现数据库连接断开时的自动重连机制
3. **配置热更新**: 支持运行时更新数据库配置
4. **多租户支持**: 支持多租户环境下的数据库连接管理
5. **插件化**: 支持通过插件方式扩展新的数据库类型
