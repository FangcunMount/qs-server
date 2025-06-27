# Container 接口架构设计文档

## 🎯 设计目标

为六边形架构项目提供统一的、可扩展的、企业级的依赖注入容器接口，实现：
- **统一契约**：所有容器实现都遵循相同的接口规范
- **高度抽象**：上层代码只依赖接口，不依赖具体实现
- **易于扩展**：支持多种容器实现（SimpleContainer、AutoDiscoveryContainer等）
- **企业级特性**：健康检查、指标收集、事件监听、构建器模式

## 🏗️ 架构概览

### 核心接口层次

```
Container 接口 (统一契约)
    ├── SimpleContainer (配置驱动实现)
    ├── AutoDiscoveryContainer (自动发现实现) 
    └── MockContainer (测试实现)
```

### 设计模式应用

- **接口隔离原则** (ISP)：Container接口专注于依赖注入职责
- **依赖倒置原则** (DIP)：Router依赖Container接口而非具体实现
- **构建器模式**：ContainerBuilder提供流畅的构建体验
- **工厂模式**：ComponentFactory用于创建组件实例
- **单例模式**：组件在容器中保持单例

## 📋 Container 接口定义

### 生命周期管理
```go
type Container interface {
    // 初始化容器中的所有组件
    Initialize() error
    // 清理容器资源
    Cleanup()
    // 检查容器健康状态
    HealthCheck(ctx context.Context) error
}
```

### 分层组件访问

#### 数据库层
```go
// 获取MySQL数据库连接
GetMySQLDB() *gorm.DB
// 获取MongoDB客户端和数据库
GetMongoClient() *mongo.Client
GetMongoDatabase() *mongo.Database
GetMongoDatabaseName() string
```

#### 存储库层 (Repository)
```go
// 获取用户存储库
GetUserRepository() storage.UserRepository
// 获取问卷存储库
GetQuestionnaireRepository() storage.QuestionnaireRepository
```

#### 应用服务层 (Application Service)
```go
// 获取用户编辑器和查询器
GetUserEditor() *userApp.UserEditor
GetUserQuery() *userApp.UserQuery
// 获取问卷编辑器和查询器
GetQuestionnaireEditor() *questionnaireApp.QuestionnaireEditor
GetQuestionnaireQuery() *questionnaireApp.QuestionnaireQuery
```

#### HTTP处理器层 (Handler)
```go
// 获取用户处理器
GetUserHandler() handlers.Handler
// 获取问卷处理器
GetQuestionnaireHandler() handlers.Handler
```

#### Web层
```go
// 获取配置好的路由器
GetRouter() *gin.Engine
```

## 🔧 企业级特性

### 1. 容器配置 (ContainerConfig)

```go
type ContainerConfig struct {
    // 数据库配置
    MySQLDB           *gorm.DB
    MongoClient       *mongo.Client  
    MongoDatabaseName string

    // 行为配置
    EnableLazyLoading bool // 懒加载
    EnableHealthCheck bool // 健康检查
    EnableMetrics     bool // 指标收集

    // 扩展配置
    CustomComponents map[string]ComponentFactory
}
```

### 2. 组件元数据 (ComponentMeta)

```go
type ComponentMeta struct {
    Name         string            // 组件名称
    Type         ComponentType     // 组件类型
    Dependencies []string          // 依赖关系
    Instance     interface{}       // 实例缓存
    Factory      ComponentFactory  // 工厂函数
    Loaded       bool              // 加载状态
    LoadOrder    int               // 加载顺序
    Metadata     map[string]string // 扩展元数据
}
```

### 3. 容器统计 (ContainerStats)

```go
type ContainerStats struct {
    TotalComponents     int                     // 总组件数
    LoadedComponents    int                     // 已加载组件数
    ComponentsByType    map[ComponentType]int   // 按类型统计
    LoadingTime         int64                   // 初始化耗时
    MemoryUsage         int64                   // 内存使用
    ComponentLoadOrder  []string                // 加载顺序
    DependencyGraph     map[string][]string     // 依赖关系图
    HealthCheckResults  map[string]bool         // 健康检查结果
}
```

### 4. 事件系统

```go
type ContainerEvent string

const (
    ComponentRegistered  ContainerEvent = "component_registered"
    ComponentLoaded      ContainerEvent = "component_loaded"
    ComponentFailed      ContainerEvent = "component_failed"
    ContainerInitialized ContainerEvent = "container_initialized"
    ContainerShutdown    ContainerEvent = "container_shutdown"
)

type ContainerEventListener func(event ContainerEvent, componentName string, err error)
```

## 🚀 使用方法

### 1. 直接创建容器
```go
// 使用现有的SimpleContainer
container := NewSimpleContainer(mysqlDB, mongoClient, mongoDB)
```

### 2. 构建器模式
```go
container, err := NewContainerBuilder().
    WithMySQLDB(mysqlDB).
    WithMongoDB(mongoClient, mongoDB).
    WithLazyLoading(true).
    WithHealthCheck(true).
    WithMetrics(true).
    WithEventListener(eventListener).
    WithCustomComponent("custom", customFactory).
    Build()
```

### 3. 容器使用
```go
// 初始化
err := container.Initialize()

// 健康检查
err = container.HealthCheck(ctx)

// 获取组件
userRepo := container.GetUserRepository()
router := container.GetRouter()

// 诊断信息
components := container.GetLoadedComponents()
container.PrintContainerInfo()

// 清理
container.Cleanup()
```

## 🎨 架构优势

### 1. **抽象与解耦**
- Router不再依赖具体的SimpleContainer，而是依赖Container接口
- 上层代码与具体容器实现解耦
- 易于替换和扩展不同的容器实现

### 2. **可测试性**
- 可以创建MockContainer用于单元测试
- 接口隔离使得组件更容易被模拟
- 支持测试专用的容器构建器

### 3. **企业级特性**
- 健康检查：实时监控组件状态
- 指标收集：性能监控和诊断
- 事件系统：组件生命周期监听
- 错误处理：统一的错误类型和处理

### 4. **扩展性**
- 支持自定义组件工厂
- 支持多种容器实现策略
- 支持插件化架构

## 🔮 未来扩展

### 1. 配置文件驱动
```yaml
# container.yaml
containers:
  default:
    mysql:
      host: localhost
      port: 3306
    components:
      - name: user-repository
        type: repository
        factory: mysql.NewUserRepository
```

### 2. 依赖注入注解
```go
type UserService struct {
    repo storage.UserRepository `inject:"user-repository"`
}
```

### 3. 容器集群支持
```go
type ContainerCluster interface {
    Container
    AddNode(Container) error
    RemoveNode(string) error
    LoadBalance() Container
}
```

### 4. 中间件和拦截器
```go
type ContainerMiddleware interface {
    BeforeInitialize(Container) error
    AfterInitialize(Container) error
    BeforeCleanup(Container) error
}
```

## 📊 对比分析

| 特性 | 原有设计 | 新Container接口 |
|------|----------|-----------------|
| 抽象程度 | ❌ 具体实现耦合 | ✅ 高度抽象 |
| 可扩展性 | ❌ 硬编码依赖 | ✅ 接口驱动 |
| 可测试性 | ❌ 难以模拟 | ✅ 易于模拟 |
| 企业级特性 | ❌ 基础功能 | ✅ 企业级完整 |
| 健康检查 | ❌ 无 | ✅ 完整支持 |
| 事件系统 | ❌ 无 | ✅ 完整支持 |
| 构建器模式 | ❌ 无 | ✅ 流畅API |
| 错误处理 | ❌ 基础 | ✅ 统一规范 |

## 🏆 总结

新的Container接口设计实现了：

1. **统一抽象**：为所有容器实现提供统一契约
2. **企业级特性**：健康检查、指标收集、事件监听
3. **高度可扩展**：支持多种实现策略和自定义组件
4. **易于测试**：接口隔离使得测试更容易
5. **构建器模式**：提供流畅的配置体验
6. **完整的生命周期管理**：从初始化到清理的全生命周期支持

这是一个**面向未来的架构设计**，为项目的长期发展提供了坚实的基础架构支撑。 