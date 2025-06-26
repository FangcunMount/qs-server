# 自动发现容器 (Auto-Discovery Container)

## 概述

自动发现容器是一个基于**约定优于配置**原则的依赖注入容器，它能够自动发现、注册和初始化应用中的所有组件，无需手动编写注册代码。

## ✨ 核心特性

### 1. **零配置自动注册**
- 组件在 `init()` 函数中自动注册
- 基于业务实体的目录约定自动发现
- 支持依赖关系自动解析

### 2. **依赖关系管理**
- 自动检测组件间的依赖关系
- 拓扑排序确保正确的初始化顺序
- 循环依赖检测和报错

### 3. **企业级特性**
- 懒加载：组件按需创建
- 单例模式：确保组件唯一性
- 优雅关闭：资源自动清理
- 健康检查：组件状态监控

## 🏗️ 架构设计

### 核心组件

```go
// 全局注册表
type GlobalRegistry struct {
    components map[string]*ComponentMeta
}

// 组件元数据
type ComponentMeta struct {
    Name          string               // 组件名称
    Type          ComponentType        // 组件类型 (repository/service/handler)
    Factory       AutoDiscoveryFactory // 工厂函数
    Dependencies  []string             // 依赖的组件名称
    InterfaceType reflect.Type         // 实现的接口类型
}

// 自动发现容器
type AutoDiscoveryContainer struct {
    repositories map[string]interface{}
    services     map[string]interface{}
    handlers     map[string]handlers.Handler
}
```

### 注册机制

```go
// 注册存储库组件
RegisterRepository("user", factoryFunc, interfaceType, dependencies...)

// 注册服务组件  
RegisterService("user", factoryFunc, interfaceType, dependencies...)

// 注册处理器组件
RegisterHandler("user", factoryFunc, dependencies...)
```

## 🚀 使用方法

### 1. 自动注册组件

在 `internal/apiserver/auto_register.go` 中定义组件注册：

```go
func init() {
    registerUserComponents()
    registerQuestionnaireComponents()
}

func registerUserComponents() {
    // 注册用户存储库
    RegisterRepository(
        "user",
        func(container *AutoDiscoveryContainer) (interface{}, error) {
            return mysqlUserAdapter.NewRepository(container.GetMySQLDB()), nil
        },
        reflect.TypeOf((*storage.UserRepository)(nil)).Elem(),
    )

    // 注册用户服务
    RegisterService(
        "user",
        func(container *AutoDiscoveryContainer) (interface{}, error) {
            repo, exists := container.GetRepository("user")
            if !exists {
                return nil, fmt.Errorf("user repository not found")
            }
            return services.NewUserService(repo.(storage.UserRepository)), nil
        },
        reflect.TypeOf((*services.UserService)(nil)).Elem(),
        "user", // 依赖用户存储库
    )

    // 注册用户处理器
    RegisterHandler(
        "user",
        func(container *AutoDiscoveryContainer) (interface{}, error) {
            service, exists := container.GetService("user")
            if !exists {
                return nil, fmt.Errorf("user service not found")
            }
            return user.NewHandler(service.(*services.UserService)), nil
        },
        "user", // 依赖用户服务
    )
}
```

### 2. 创建和初始化容器

```go
// 创建自动发现容器
container := NewAutoDiscoveryContainer(mysqlDB, mongoClient, mongoDatabase)

// 自动发现并初始化所有组件
if err := container.Initialize(); err != nil {
    log.Fatalf("Failed to initialize container: %v", err)
}

// 获取路由器
router := container.GetRouter()
```

### 3. 运行时输出

应用启动时会看到自动注册过程：

```
📝 Registered repository component: user (dependencies: [])
📝 Registered service component: user (dependencies: [user])
📝 Registered handler component: user (dependencies: [user])
📝 Registered repository component: questionnaire (dependencies: [])
📝 Registered service component: questionnaire (dependencies: [questionnaire])
📝 Registered handler component: questionnaire (dependencies: [questionnaire])

🚀 Starting automatic component discovery and registration...
📦 Discovered 2 repository components
  ✓ Initialized repository: user
  ✓ Initialized repository: questionnaire
🔧 Discovered 2 service components
  ✓ Initialized service: user
  ✓ Initialized service: questionnaire
🌐 Discovered 2 handler components
  ✓ Initialized handler: user
  ✓ Initialized handler: questionnaire
🔗 Auto-registering routes for: questionnaire
🔗 Auto-registering routes for: user
🔗 Route registration completed
✅ Automatic component discovery completed successfully!
```

## 📁 目录约定

自动发现机制基于以下目录约定：

```
internal/apiserver/
├── adapters/
│   ├── api/http/handlers/
│   │   ├── user/           # 用户处理器
│   │   └── questionnaire/  # 问卷处理器
│   └── storage/
│       ├── mysql/
│       │   ├── user/       # 用户MySQL适配器
│       │   └── questionnaire/
│       └── mongodb/
│           └── questionnaire/
├── application/
│   ├── questionnaire/      # 问卷应用服务
│   └── services/
├── domain/
│   ├── user/              # 用户领域
│   └── questionnaire/     # 问卷领域
└── ports/
    └── storage/           # 存储端口定义
```

## 🔄 依赖关系解析

容器自动解析组件间的依赖关系：

1. **存储库层** (无依赖)
   - `user` repository
   - `questionnaire` repository

2. **服务层** (依赖存储库)
   - `user` service → 依赖 `user` repository
   - `questionnaire` service → 依赖 `questionnaire` repository

3. **处理器层** (依赖服务)
   - `user` handler → 依赖 `user` service
   - `questionnaire` handler → 依赖 `questionnaire` service

## 🎯 扩展新业务实体

添加新的业务实体（如 `scale`）只需：

1. **创建目录结构**：
   ```
   adapters/storage/mysql/scale/
   adapters/api/http/handlers/scale/
   application/scale/
   domain/scale/
   ports/storage/scale/
   ```

2. **在 auto_register.go 中添加注册**：
   ```go
   func registerScaleComponents() {
       RegisterRepository("scale", scaleRepoFactory, scaleRepoInterface)
       RegisterService("scale", scaleServiceFactory, scaleServiceInterface, "scale")
       RegisterHandler("scale", scaleHandlerFactory, "scale")
   }
   ```

3. **在 init() 中调用**：
   ```go
   func init() {
       registerUserComponents()
       registerQuestionnaireComponents()
       registerScaleComponents()  // 新增
   }
   ```

## 🛡️ 错误处理

自动发现容器提供完善的错误处理：

- **循环依赖检测**：检测并报告循环依赖
- **组件未找到**：依赖的组件不存在时报错
- **接口不匹配**：组件未实现预期接口时报错
- **工厂函数错误**：组件创建失败时传播错误

## 🔍 调试和监控

### 组件注册表信息

```go
container.PrintRegistryInfo()
```

输出示例：
```
📋 Component Registry Summary:

repository Components:
  • user (key: repository:user, deps: [])
  • questionnaire (key: repository:questionnaire, deps: [])

service Components:
  • user (key: service:user, deps: [user])
  • questionnaire (key: service:questionnaire, deps: [questionnaire])

handler Components:
  • user (key: handler:user, deps: [user])
  • questionnaire (key: handler:questionnaire, deps: [questionnaire])
```

### 健康检查端点

访问 `/health` 端点可以查看容器状态：

```json
{
  "status": "healthy",
  "version": "1.0.0",
  "discovery": "auto",
  "repositories": ["questionnaire", "user"],
  "services": ["questionnaire", "user"],
  "handlers": ["questionnaire", "user"]
}
```

## 💡 最佳实践

1. **命名约定**：使用业务实体名称作为组件名
2. **依赖声明**：明确声明组件依赖关系
3. **错误处理**：工厂函数中进行充分的错误检查
4. **接口隔离**：每个组件实现单一职责的接口
5. **资源清理**：在 Cleanup 方法中释放资源

## 🚀 性能优势

- **启动时间**：组件按需创建，减少启动开销
- **内存使用**：单例模式避免重复创建
- **CPU效率**：依赖关系预解析，运行时无需计算
- **扩展性**：新组件零配置接入

这个自动发现容器实现了真正的**约定优于配置**，让开发者专注于业务逻辑而不是基础设施代码！ 