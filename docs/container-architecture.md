# 容器架构重构：从硬编码到动态注册

## 🎯 **重构目标**

解决原有 Container 设计中的硬编码问题，实现真正的**开放封闭原则**（对扩展开放，对修改封闭）。

## ❌ **原有问题**

### 硬编码依赖
```go
type Container struct {
    // 硬编码的特定仓储
    mysqlQuestionnaireRepo storage.QuestionnaireRepository
    userRepo               storage.UserRepository
    mongoDocumentRepo      storage.QuestionnaireDocumentRepository
    
    // 硬编码的特定服务
    questionnaireService   *services.QuestionnaireService
    userService           *services.UserService
    
    // 硬编码的特定处理器
    questionnaireHandler  *handlers.QuestionnaireHandler
}
```

### 扩展性问题
- 每次添加新业务模块（如 `scale`、`response`、`evaluation`）都需要：
  - 修改 Container 结构体
  - 修改初始化方法
  - 修改路由注册逻辑
- 违反开放封闭原则
- 代码耦合度高，难以维护

## ✅ **新架构设计**

### 核心概念

#### 1. **注册器模式（Registry Pattern）**
```go
type Container struct {
    // 基础设施依赖
    mysqlDB       *gorm.DB
    mongoClient   *mongo.Client
    mongoDatabase string
    
    // 组件注册表 - 核心创新
    components map[string]*ComponentDefinition
    
    // 路由配置器
    router *Router
}
```

#### 2. **组件定义（Component Definition）**
```go
type ComponentDefinition struct {
    Name     string           // 组件名称
    Type     ComponentType    // 组件类型
    Factory  ComponentFactory // 工厂函数
    Instance interface{}      // 单例实例（懒加载）
}

type ComponentFactory func(container *Container) (interface{}, error)
```

#### 3. **组件类型（Component Types）**
```go
const (
    RepositoryType ComponentType = "repository"  // 仓储层
    ServiceType    ComponentType = "service"     // 服务层
    HandlerType    ComponentType = "handler"     // 处理器层
)
```

### 核心特性

#### 🔄 **懒加载 + 单例模式**
- 组件只在第一次使用时创建
- 后续调用返回同一实例
- 提高启动速度，节省内存

#### 🏭 **工厂模式**
- 每个组件通过工厂函数创建
- 支持复杂的依赖注入逻辑
- 易于测试和模拟

#### 🔍 **类型安全**
- 编译时类型检查
- 运行时类型断言
- 错误处理机制

## 🚀 **使用方法**

### 1. 注册组件

```go
// 注册仓储
container.RegisterComponent("questionnaireRepo", RepositoryType, func(c *Container) (interface{}, error) {
    return mysqlAdapter.NewQuestionnaireRepository(c.mysqlDB, nil, ""), nil
})

// 注册服务
container.RegisterComponent("questionnaireService", ServiceType, func(c *Container) (interface{}, error) {
    repo, err := c.GetComponent("questionnaireRepo")
    if err != nil {
        return nil, err
    }
    return services.NewQuestionnaireService(repo.(storage.QuestionnaireRepository)), nil
})

// 注册处理器
container.RegisterComponent("questionnaireHandler", HandlerType, func(c *Container) (interface{}, error) {
    service, err := c.GetComponent("questionnaireService")
    if err != nil {
        return nil, err
    }
    return handlers.NewQuestionnaireHandler(service.(*services.QuestionnaireService)), nil
})
```

### 2. 获取组件

```go
// 获取单个组件
service, err := container.GetComponent("questionnaireService")
if err != nil {
    // 处理错误
}

// 获取某类型的所有组件
handlers, err := container.GetComponentsByType(HandlerType)

// 获取组件（失败时panic）
service := container.MustGetComponent("questionnaireService")
```

### 3. 动态路由注册

```go
// 路由器自动注册所有处理器
for name, handler := range handlers {
    router.registerHandlerRoutes(name, handler)
}
```

## 📈 **扩展新业务模块**

### 添加 Scale 模块

**步骤1：创建组件注册方法**
```go
func (c *Container) registerScaleComponents() {
    // 注册量表仓储
    c.RegisterComponent("scaleRepo", RepositoryType, func(container *Container) (interface{}, error) {
        return mysqlAdapter.NewScaleRepository(container.mysqlDB), nil
    })
    
    // 注册量表服务
    c.RegisterComponent("scaleService", ServiceType, func(container *Container) (interface{}, error) {
        repo, err := container.GetComponent("scaleRepo")
        if err != nil {
            return nil, err
        }
        return services.NewScaleService(repo.(storage.ScaleRepository)), nil
    })
    
    // 注册量表处理器
    c.RegisterComponent("scaleHandler", HandlerType, func(container *Container) (interface{}, error) {
        service, err := container.GetComponent("scaleService")
        if err != nil {
            return nil, err
        }
        return handlers.NewScaleHandler(service.(*services.ScaleService)), nil
    })
}
```

**步骤2：注册到核心组件**
```go
func (c *Container) registerCoreComponents() error {
    c.registerQuestionnaireComponents()
    c.registerUserComponents()
    c.registerScaleComponents()        // 👈 只需要添加这一行！
    return nil
}
```

**步骤3：添加路由注册**
```go
func (c *Container) registerHandlerRoutes(name string, handler interface{}) error {
    switch name {
    case "questionnaireHandler":
        return c.router.RegisterQuestionnaireRoutes(handler)
    case "scaleHandler":               // 👈 只需要添加这个case！
        return c.router.RegisterScaleRoutes(handler)
    default:
        return c.router.RegisterGenericRoutes(name, handler)
    }
}
```

**就这样！** 没有修改 Container 的核心逻辑，完全符合开放封闭原则。

## 🎨 **架构优势**

### 1. **可扩展性**
- 添加新模块无需修改核心代码
- 支持插件化开发
- 易于模块化管理

### 2. **可维护性**
- 组件职责清晰
- 依赖关系明确
- 易于调试和测试

### 3. **灵活性**
- 支持条件注册（如 MongoDB 可选）
- 支持不同的实现策略
- 易于配置驱动

### 4. **性能**
- 懒加载机制
- 单例模式避免重复创建
- 最小化内存占用

## 📊 **对比总结**

| 特性 | 原有架构 | 新架构 |
|------|----------|--------|
| 扩展性 | ❌ 需要修改核心代码 | ✅ 无需修改核心代码 |
| 可维护性 | ❌ 硬编码，难以维护 | ✅ 组件化，易于维护 |
| 开放封闭原则 | ❌ 违反 | ✅ 符合 |
| 测试友好性 | ❌ 难以模拟 | ✅ 易于模拟 |
| 启动性能 | ❌ 全量初始化 | ✅ 懒加载 |
| 内存使用 | ❌ 可能浪费 | ✅ 按需分配 |

## 🔮 **未来扩展**

### 1. 配置驱动
可以进一步扩展为配置驱动的组件注册：

```yaml
# components.yaml
components:
  - name: questionnaireRepo
    type: repository
    factory: mysql.NewQuestionnaireRepository
    
  - name: questionnaireService
    type: service
    factory: services.NewQuestionnaireService
    dependencies: [questionnaireRepo]
```

### 2. 注解驱动
使用 Go 的 struct tag 或者代码生成工具：

```go
type QuestionnaireService struct {
    repo storage.QuestionnaireRepository `inject:"questionnaireRepo"`
}
```

### 3. 多环境支持
```go
// 开发环境使用 MySQL
container.RegisterComponent("questionnaireRepo", RepositoryType, func(c *Container) (interface{}, error) {
    return mysqlAdapter.NewQuestionnaireRepository(c.mysqlDB), nil
})

// 测试环境使用 Memory
container.RegisterComponent("questionnaireRepo", RepositoryType, func(c *Container) (interface{}, error) {
    return memoryAdapter.NewQuestionnaireRepository(), nil
})
```

## 🎉 **总结**

通过引入**注册器模式**和**工厂模式**，我们成功解决了原有架构的硬编码问题：

1. **彻底解耦**：组件之间通过接口和工厂函数解耦
2. **高度可扩展**：添加新模块只需要注册，无需修改核心代码
3. **符合 SOLID 原则**：特别是开放封闭原则和依赖倒置原则
4. **易于测试**：每个组件都可以独立测试和模拟
5. **性能优化**：懒加载和单例模式提高性能

这是一个**企业级的依赖注入容器**设计，为项目的长期发展奠定了坚实的架构基础。 