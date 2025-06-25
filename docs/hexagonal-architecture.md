# 🏗️ 六边形架构重构文档

## 概述

项目已成功重构为六边形架构（又称端口和适配器架构），实现了业务逻辑与技术实现的完全解耦。

## 🏛️ 架构图

```
                    🌐 HTTP API
                         |
                    ┌─────────┐
                    │ Router  │ ◄── 路由配置器
                    │ Config  │
                    └─────┬───┘
                          │
                    ┌─────┴────┐
                    │ HTTP     │ ◄── HTTP适配器
                    │ Handlers │
                    └─────┬────┘
                          │
              ┌───────────┴───────────┐
              │   Application Layer   │ ◄── 应用服务层
              │  (Use Cases/Services) │
              └─────────┬─────────────┘
                        │
              ┌─────────┴─────────┐
              │   Domain Layer    │ ◄── 核心领域
              │ (Business Logic)  │
              └─────────┬─────────────┘
                        │
                ┌───────┴───────┐
                │     Ports     │ ◄── 端口（接口）
                └───────┬───────┘
                        │
            ┌───────────┴─────────────┐
            │  Storage Adapters       │ ◄── 存储适配器
            │ (MySQL + MongoDB)       │
            └─────────────────────────┘
```

## 📁 目录结构

```
internal/apiserver/
├── domain/                    # 🔵 核心领域（业务逻辑）
│   ├── questionnaire/
│   │   ├── questionnaire.go   # 聚合根
│   │   └── errors.go          # 领域错误
│   └── user/
│       └── user.go            # 用户聚合根
├── ports/                     # 🔌 端口（接口契约）
│   └── storage/
│       ├── questionnaire.go   # QuestionnaireRepository 接口
│       └── user.go            # UserRepository 接口
├── adapters/                  # 🔧 适配器（具体实现）
│   ├── storage/
│   │   └── mysql/
│   │       ├── questionnaire.go  # MySQL+MongoDB 混合适配器
│   │       └── user.go           # MySQL 适配器
│   └── api/
│       └── http/
│           └── handlers/
│               └── questionnaire_handler.go  # HTTP 处理器
├── application/               # 📋 应用服务层
│   └── services/
│       ├── questionnaire_service.go  # 问卷应用服务
│       └── user_service.go          # 用户应用服务
├── container.go              # 🔗 依赖注入容器
├── router.go                 # 🛣️ 路由配置器
└── server.go                 # 🚀 服务器入口
```

## 🔄 数据流

### 1. HTTP 请求流程
```
HTTP Request → Router → Handler → Application Service → Domain Objects → Port Interface → Adapter → Database
```

### 2. 依赖方向
```
外层 → 内层（依赖倒置原则）
Router → HTTP Handlers → Application Services → Domain Objects
Storage Adapters → Port Interfaces
```

## 🧩 核心组件

### 🔗 Container（依赖注入容器）
```go
// 职责：组装和管理所有组件的生命周期
type Container struct {
    // 外部依赖
    mysqlDB       *gorm.DB
    mongoSession  *mgo.Session
    
    // 内部组件
    questionnaireRepo    storage.QuestionnaireRepository
    questionnaireService *services.QuestionnaireService
    questionnaireHandler *handlers.QuestionnaireHandler
    router              *Router
}
```

### 🛣️ Router（路由配置器）
```go
// 职责：专门负责路由配置和中间件管理
type Router struct {
    engine               *gin.Engine
    questionnaireHandler *handlers.QuestionnaireHandler
}
```

### 🔵 Domain Layer（领域层）
- **问卷聚合根** (`questionnaire.Questionnaire`)
  - 封装问卷业务规则
  - 提供业务操作方法（创建、发布、归档等）
- **用户聚合根** (`user.User`)
  - 封装用户业务规则
  - 提供用户操作方法（激活、封禁等）

### 🔌 Ports（端口层）
- **存储端口** (`storage.QuestionnaireRepository`, `storage.UserRepository`)
  - 定义数据访问契约
  - 不依赖具体技术实现

### 🔧 Adapters（适配器层）
- **存储适配器**
  - MySQL + MongoDB 混合存储（问卷）
  - MySQL 存储（用户）
- **HTTP 适配器**
  - REST API 处理器
  - 请求/响应转换

### 📋 Application Layer（应用层）
- **应用服务** (`QuestionnaireService`, `UserService`)
  - 协调领域对象和端口
  - 实现具体的用例场景

## 🔗 依赖注入和路由配置

### 新的初始化流程

```go
// 1. 创建容器
container := NewContainer(mysqlDB, mongoSession, mongoDatabase)

// 2. 初始化所有组件（按依赖顺序）
container.Initialize() // 内部按顺序初始化：
                      // → Adapters
                      // → Application Services  
                      // → HTTP Handlers
                      // → Router

// 3. 获取配置好的路由引擎
router := container.GetRouter()
```

### 职责分离对比

| 组件 | 旧职责 | 新职责 | 优势 |
|------|--------|--------|------|
| **Container** | 依赖注入 + 路由配置 | 纯依赖注入管理 | 单一职责，更清晰 |
| **Router** | 无（散落在container中） | 专门路由配置 | 路由逻辑集中，易维护 |

## 🎯 架构优势

### 1. **更清晰的职责分离**
- **Container**: 专注于组件生命周期管理
- **Router**: 专注于路由和中间件配置
- **Handler**: 专注于HTTP请求处理
- **Service**: 专注于业务用例实现

### 2. **更高的可维护性**
- 路由配置独立，便于管理和扩展
- 添加新的中间件只需修改 Router
- 添加新的业务模块遵循固定模式

### 3. **更好的可测试性**
- 每个组件都可以独立测试
- Router 可以独立测试路由配置
- Container 可以独立测试依赖注入

### 4. **更强的扩展性**
- 新增业务模块：Handler → Service → Domain → Port → Adapter
- 新增中间件：直接在 Router 中配置
- 新增路由组：在 Router 中添加新的注册方法

## 🚀 API 接口

### 问卷管理
- `POST /api/v1/questionnaires` - 创建问卷
- `GET /api/v1/questionnaires` - 获取问卷详情
- `GET /api/v1/questionnaires/list` - 获取问卷列表
- `PUT /api/v1/questionnaires/{id}` - 更新问卷
- `POST /api/v1/questionnaires/{id}/publish` - 发布问卷
- `DELETE /api/v1/questionnaires/{id}` - 删除问卷

### 健康检查
- `GET /health` - 架构状态检查
- `GET /ping` - 简单连通性测试

## 🔧 技术栈

- **Web框架**: Gin
- **数据库**: MySQL (主要存储) + MongoDB (文档存储)
- **缓存**: Redis (可选)
- **ORM**: GORM
- **依赖注入**: 自定义容器
- **路由管理**: 独立路由配置器

## 📝 添加新功能的标准流程

### 1. 添加新的业务模块（以"量表"为例）

```go
// 1. 创建领域对象
// domain/scale/scale.go

// 2. 创建端口接口
// ports/storage/scale.go

// 3. 创建适配器实现
// adapters/storage/mysql/scale.go

// 4. 创建应用服务
// application/services/scale_service.go

// 5. 创建HTTP处理器
// adapters/api/http/handlers/scale_handler.go

// 6. 在Container中注册组件
func (c *Container) initializeHandlers() error {
    c.scaleHandler = handlers.NewScaleHandler(c.scaleService)
    return nil
}

// 7. 在Router中注册路由
func (r *Router) registerAPIRoutes() {
    v1 := r.engine.Group("/api/v1")
    r.registerScaleRoutes(v1)  // 新增
}

func (r *Router) registerScaleRoutes(rg *gin.RouterGroup) {
    scales := rg.Group("/scales")
    {
        scales.POST("", r.scaleHandler.CreateScale)
        scales.GET("/:id", r.scaleHandler.GetScale)
        // ... 其他路由
    }
}
```

### 2. 添加新的中间件

```go
// 在 Router 的 installMiddleware 方法中添加
func (r *Router) installMiddleware() {
    r.engine.Use(gin.Logger())
    r.engine.Use(gin.Recovery())
    r.engine.Use(cors.Default())        // 新增CORS
    r.engine.Use(ratelimit.New())       // 新增限流
    r.engine.Use(auth.Middleware())     // 新增认证
}
```

## ⚡ 快速开始

1. **启动服务**
```bash
make run
```

2. **健康检查**
```bash
curl http://localhost:8080/health
curl http://localhost:8080/ping
```

3. **创建问卷**
```bash
curl -X POST http://localhost:8080/api/v1/questionnaires \
  -H "Content-Type: application/json" \
  -d '{
    "code": "survey001",
    "title": "客户满意度调查",
    "description": "评估客户对我们服务的满意度",
    "created_by": "admin"
  }'
```

## 🎉 总结

通过职责分离重构，项目架构更加清晰：

- 🔗 **Container** 专注于依赖注入和组件管理
- 🛣️ **Router** 专注于路由配置和中间件管理
- 📋 **Handler** 专注于HTTP请求处理
- 🔵 **Service** 专注于业务用例实现
- 🎯 **Domain** 专注于业务规则和逻辑

这种分离使得每个组件都有明确的职责，代码更易维护、测试和扩展！ 🎊 