# 配置驱动的容器架构设计

## 🎯 设计理念

**数据与代码分离，配置驱动组件加载**

这是一个优雅的中间方案：
- ❌ 避免完全手动的重复代码
- ❌ 避免过度复杂的自动化系统
- ✅ 实现配置驱动的"半自动化"
- ✅ 保持代码的简洁性和可维护性

## 🏗️ 四组件架构

### 架构图
```
📦 数据库组件组 (3个)
   ├── mysql-db
   ├── mongo-client  
   └── mongo-database

📦 存储库组件组 (2个)
   ├── user-repository
   └── questionnaire-repository

📦 应用服务组件组 (4个)
   ├── user-editor
   ├── user-query
   ├── questionnaire-editor
   └── questionnaire-query

📦 HTTP处理器组件组 (2个)
   ├── user-handler
   └── questionnaire-handler
```

### 加载顺序
依赖关系自动解析，按组顺序加载：
1. 数据库组件 → 2. 存储库组件 → 3. 应用服务组件 → 4. HTTP处理器组件

## 🧩 核心设计模式

### 1. 配置结构分离

```go
// ComponentConfig 组件配置（数据）
type ComponentConfig struct {
    Name         string                                       // 组件名称
    Dependencies []string                                     // 依赖关系
    Factory      func(*SimpleContainer) (interface{}, error) // 工厂函数
}

// ComponentGroupConfig 组件组配置（数据）
type ComponentGroupConfig struct {
    Name       string            // 组件组名称
    Components []ComponentConfig // 组件列表
}
```

### 2. 统一加载逻辑（代码）

```go
// 统一的组件组加载器
func (c *SimpleContainer) loadComponentGroup(group ComponentGroupConfig) error {
    for _, component := range group.Components {
        // 1. 检查依赖
        if err := c.checkDependencies(component); err != nil {
            return err
        }
        
        // 2. 创建实例
        instance, err := component.Factory(c)
        if err != nil {
            return err
        }
        
        // 3. 存储实例
        c.componentInstances[component.Name] = instance
    }
    return nil
}
```

## 📋 组件配置示例

### 数据库组件组
```go
var DatabaseComponentGroup = ComponentGroupConfig{
    Name: "数据库组件",
    Components: []ComponentConfig{
        {
            Name:         "mysql-db",
            Dependencies: []string{},
            Factory: func(c *SimpleContainer) (interface{}, error) {
                return c.mysqlDB, nil
            },
        },
        {
            Name:         "mongo-database", 
            Dependencies: []string{"mongo-client"},
            Factory: func(c *SimpleContainer) (interface{}, error) {
                return c.mongoClient.Database(c.mongoDatabaseName), nil
            },
        },
    },
}
```

### 存储库组件组
```go
var RepositoryComponentGroup = ComponentGroupConfig{
    Name: "存储库组件",
    Components: []ComponentConfig{
        {
            Name:         "user-repository",
            Dependencies: []string{"mysql-db"},
            Factory: func(c *SimpleContainer) (interface{}, error) {
                return mysqlUserAdapter.NewRepository(c.mysqlDB), nil
            },
        },
    },
}
```

### 应用服务组件组
```go
var ApplicationServiceComponentGroup = ComponentGroupConfig{
    Name: "应用服务组件",
    Components: []ComponentConfig{
        {
            Name:         "user-editor",
            Dependencies: []string{"user-repository"},
            Factory: func(c *SimpleContainer) (interface{}, error) {
                userRepo := c.componentInstances["user-repository"].(storage.UserRepository)
                return userApp.NewUserEditor(userRepo), nil
            },
        },
    },
}
```

## 🚀 运行时效果

### 启动输出
```bash
🚀 开始配置驱动的组件初始化...

📦 加载 数据库组件...
  ✓ mysql-db
  ✓ mongo-client
  ✓ mongo-database

📦 加载 存储库组件...
  ✓ user-repository
  ✓ questionnaire-repository

📦 加载 应用服务组件...
  ✓ user-editor
  ✓ user-query
  ✓ questionnaire-editor
  ✓ questionnaire-query

📦 加载 HTTP处理器组件...
  ✓ user-handler
  ✓ questionnaire-handler

📊 组件加载摘要:
  1. 数据库组件 ✓
  2. 存储库组件 ✓
  3. 应用服务组件 ✓
  4. HTTP处理器组件 ✓
  总计: 11 个组件成功加载

✅ 配置驱动的组件初始化完成
```

### 容器摘要
```bash
📊 容器组件摘要:
  数据库组件: 3 个
  存储库组件: 2 个
  应用服务组件: 4 个
  HTTP处理器组件: 2 个
  总计: 11 个组件
```

## 🔍 架构优势

### 1. 数据与代码分离
- **配置数据**：组件定义、依赖关系
- **通用代码**：统一的加载逻辑、依赖检查
- **结果**：添加新组件只需修改配置，无需重复代码

### 2. 半自动化加载
- ✅ 自动依赖检查和解析
- ✅ 统一的加载和错误处理
- ✅ 避免了复杂的反射和元数据系统
- ✅ 保持了代码的透明性和可调试性

### 3. 易于扩展
```go
// 添加新的Scale组件只需增加配置
var ScaleComponentGroup = ComponentGroupConfig{
    Name: "量表组件",
    Components: []ComponentConfig{
        {
            Name:         "scale-repository",
            Dependencies: []string{"mysql-db"},
            Factory: func(c *SimpleContainer) (interface{}, error) {
                return mysqlScaleAdapter.NewRepository(c.mysqlDB), nil
            },
        },
        {
            Name:         "scale-service",
            Dependencies: []string{"scale-repository"},
            Factory: func(c *SimpleContainer) (interface{}, error) {
                repo := c.componentInstances["scale-repository"].(storage.ScaleRepository)
                return scaleApp.NewScaleService(repo), nil
            },
        },
    },
}

// 然后添加到组件组列表
var ComponentGroups = []ComponentGroupConfig{
    DatabaseComponentGroup,
    RepositoryComponentGroup,
    ApplicationServiceComponentGroup,
    ScaleComponentGroup,          // 👈 新增
    HttpHandlerComponentGroup,
}
```

### 4. 配置驱动的灵活性
- **条件加载**：可以根据配置启用/禁用组件
- **环境适配**：不同环境可以有不同的组件配置
- **测试友好**：可以轻松替换组件用于测试

## 📊 重构对比

| 指标 | 完全手动 | 过度自动化 | **配置驱动** |
|------|----------|------------|------------|
| 代码复杂度 | 简单但重复 | 过度复杂 | **适中** |
| 扩展性 | 需重复代码 | 过度抽象 | **配置即可** |
| 可维护性 | 重复维护 | 难以调试 | **清晰透明** |
| 学习成本 | 低 | 高 | **中等** |
| 性能 | 最优 | 反射开销 | **接近最优** |

## 🎖️ 最佳实践

### 1. 组件命名约定
- 数据库组件：`xxx-db`, `xxx-client`
- 存储库组件：`xxx-repository`
- 应用服务组件：`xxx-editor`, `xxx-query`
- HTTP处理器组件：`xxx-handler`

### 2. 依赖关系设计
- 单向依赖：下层 → 上层
- 明确声明：显式列出所有依赖
- 最小依赖：只依赖直接需要的组件

### 3. 错误处理
- 依赖检查：启动时验证所有依赖
- 类型安全：运行时类型断言
- 详细日志：清晰的加载过程输出

## 🏆 总结

**配置驱动的 SimpleContainer** 实现了数据与代码的完美分离：

- **数据层**：组件定义和依赖关系配置
- **代码层**：统一的加载逻辑和管理机制
- **结果**：既避免了重复代码，又保持了简洁性

这是一个真正实用的架构设计，兼顾了**简洁性**、**可扩展性**和**可维护性**！🚀 