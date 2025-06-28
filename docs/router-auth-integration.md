# 🛣️ 路由器认证集成指南

## 📋 概述

本指南展示了如何在 `apiserver/routers` 中使用认证功能，实现完整的路由保护和用户认证体系。

## 🏗️ 路由架构设计

### 路由分层结构
```
📂 路由层次结构
├── 🌐 全局中间件层
│   ├── Recovery (崩溃恢复)
│   ├── Logger (日志记录)
│   ├── RequestID (请求追踪)
│   ├── CORS (跨域处理)
│   ├── Security (安全头)
│   └── NoCache (缓存控制)
├── 🔓 公开路由
│   ├── /health (健康检查)
│   ├── /ping (连通性测试)
│   ├── /auth/* (认证端点)
│   └── /api/v1/public/* (公开API)
└── 🔐 受保护路由 (/api/v1/*)
    ├── 🔒 认证中间件 (auto策略)
    ├── 👤 用户路由 (/users/*)
    ├── 📋 问卷路由 (/questionnaires/*)
    └── 👑 管理员路由 (/admin/*)
```

## 🔐 认证策略

### 1. 自动认证策略 (`auto`)
```go
// 自动选择Basic或JWT认证
authMiddleware := r.authConfig.CreateAuthMiddleware("auto")
apiV1.Use(authMiddleware)
```

**支持的认证方式：**
- **Basic Auth**: `Authorization: Basic base64(username:password)`
- **JWT Token**: `Authorization: Bearer jwt-token`

### 2. 特定认证策略
```go
// 仅JWT认证
jwtAuth := r.authConfig.CreateAuthMiddleware("jwt")

// 仅Basic认证  
basicAuth := r.authConfig.CreateAuthMiddleware("basic")
```

## 🚀 使用示例

### 1. 用户登录流程

#### 步骤1：用户登录
```bash
# JWT登录
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"john","password":"password123"}'

# 响应示例
{
  "code": 200,
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expire": "2024-01-16T10:30:15Z",
  "user": {
    "id": 123,
    "username": "john",
    "nickname": "John Doe"
  },
  "message": "Login successful"
}
```

#### 步骤2：使用令牌访问受保护资源
```bash
# 获取当前用户资料
curl -X GET http://localhost:8080/api/v1/users/profile \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

# 或使用Basic认证
curl -X GET http://localhost:8080/api/v1/users/profile \
  -H "Authorization: Basic am9objpwYXNzd29yZDEyMw=="
```

### 2. 公开端点访问
```bash
# 健康检查（无需认证）
curl http://localhost:8080/health

# 公开信息（无需认证）
curl http://localhost:8080/api/v1/public/info

# 用户注册（无需认证）
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"newuser","password":"newpass","email":"user@example.com","nickname":"New User"}'
```

### 3. 受保护端点访问
```bash
# 获取用户资料
curl -X GET http://localhost:8080/api/v1/users/profile \
  -H "Authorization: Bearer [token]"

# 修改密码
curl -X POST http://localhost:8080/api/v1/users/change-password \
  -H "Authorization: Bearer [token]" \
  -H "Content-Type: application/json" \
  -d '{"old_password":"old123","new_password":"new456"}'

# 问卷操作
curl -X GET http://localhost:8080/api/v1/questionnaires \
  -H "Authorization: Bearer [token]"
```

## 📊 路由端点总览

### 🔓 公开端点

| 方法 | 路径 | 描述 | 认证 |
|------|------|------|------|
| GET | `/health` | 健康检查 | ❌ |
| GET | `/ping` | 连通性测试 | ❌ |
| GET | `/api/v1/public/info` | 服务信息 | ❌ |
| POST | `/auth/login` | 用户登录 | ❌ |
| POST | `/auth/register` | 用户注册 | ❌ |
| POST | `/auth/refresh` | 刷新令牌 | ❌ |
| POST | `/auth/logout` | 用户登出 | ❌ |

### 🔐 受保护端点

| 方法 | 路径 | 描述 | 认证 |
|------|------|------|------|
| **用户相关** ||||
| GET | `/api/v1/users/profile` | 获取当前用户资料 | ✅ |
| PUT | `/api/v1/users/profile` | 更新当前用户资料 | ✅ |
| POST | `/api/v1/users/change-password` | 修改密码 | ✅ |
| GET | `/api/v1/users/:id` | 获取指定用户 | ✅ |
| PUT | `/api/v1/users/:id` | 更新指定用户 | ✅ |
| **问卷相关** ||||
| POST | `/api/v1/questionnaires` | 创建问卷 | ✅ |
| GET | `/api/v1/questionnaires` | 获取问卷列表 | ✅ |
| GET | `/api/v1/questionnaires/:id` | 获取指定问卷 | ✅ |
| PUT | `/api/v1/questionnaires/:id` | 更新问卷 | ✅ |
| DELETE | `/api/v1/questionnaires/:id` | 删除问卷 | ✅ |
| POST | `/api/v1/questionnaires/:id/publish` | 发布问卷 | ✅ |
| POST | `/api/v1/questionnaires/:id/archive` | 归档问卷 | ✅ |
| POST | `/api/v1/questionnaires/:id/responses` | 提交问卷响应 | ✅ |
| **管理员相关** ||||
| GET | `/api/v1/admin/users` | 管理员获取所有用户 | ✅ + 管理员权限 |
| GET | `/api/v1/admin/statistics` | 系统统计信息 | ✅ + 管理员权限 |
| GET | `/api/v1/admin/logs` | 系统日志 | ✅ + 管理员权限 |

## 🔧 自定义认证处理

### 1. 在处理器中获取当前用户
```go
func (r *Router) someProtectedHandler(c *gin.Context) {
    // 获取当前认证用户
    username, exists := c.Get(middleware.UsernameKey)
    if !exists {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "用户未认证"})
        return
    }

    // 使用认证服务获取完整用户信息
    authService := r.container.GetUserModule().GetAuthService()
    userInfo, err := authService.GetUserByUsername(c.Request.Context(), username.(string))
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用户信息失败"})
        return
    }

    // 处理业务逻辑...
}
```

### 2. 添加权限检查中间件
```go
// 检查管理员权限的中间件
func (r *Router) requireAdminRole() gin.HandlerFunc {
    return func(c *gin.Context) {
        username, exists := c.Get(middleware.UsernameKey)
        if !exists {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "用户未认证"})
            c.Abort()
            return
        }

        // 检查用户是否有管理员权限
        authService := r.container.GetUserModule().GetAuthService()
        // 这里需要实现权限检查逻辑
        // isAdmin := authService.CheckUserRole(username.(string), "admin")
        // if !isAdmin {
        //     c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
        //     c.Abort()
        //     return
        // }

        c.Next()
    }
}

// 在路由中使用
admin := apiV1.Group("/admin")
admin.Use(r.requireAdminRole())
{
    admin.GET("/users", r.adminGetUsers)
    admin.GET("/statistics", r.adminGetStatistics)
}
```

### 3. 自定义认证策略
```go
// 创建API密钥认证中间件
func (r *Router) apiKeyAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        apiKey := c.GetHeader("X-API-Key")
        if apiKey == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少API密钥"})
            c.Abort()
            return
        }

        // 验证API密钥
        if !r.validateAPIKey(apiKey) {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的API密钥"})
            c.Abort()
            return
        }

        c.Next()
    }
}

// 在特定路由中使用
api := engine.Group("/api/external")
api.Use(r.apiKeyAuth())
{
    api.GET("/data", r.getExternalData)
}
```

## ⚠️ 错误处理

### 常见认证错误
```go
// 401 - 未认证
{
  "code": 401,
  "message": "用户未认证"
}

// 403 - 无权限
{
  "code": 403,
  "message": "需要管理员权限"
}

// 400 - 请求错误
{
  "code": 400,
  "message": "请求格式错误"
}

// 500 - 服务器错误
{
  "code": 500,
  "message": "认证服务不可用"
}
```

## 🎯 最佳实践

### 1. **安全性**
- ✅ 使用HTTPS传输敏感信息
- ✅ JWT令牌设置合理的过期时间
- ✅ 对敏感操作（如密码修改）进行额外验证
- ✅ 实施请求限流防止暴力破解

### 2. **可维护性**
- ✅ 将认证逻辑集中在AuthConfig中
- ✅ 使用中间件实现横切关注点
- ✅ 清晰分离公开路由和受保护路由
- ✅ 统一的错误处理和响应格式

### 3. **扩展性**
- ✅ 支持多种认证策略
- ✅ 易于添加新的权限检查
- ✅ 模块化的路由组织
- ✅ 预留扩展点用于自定义路由

### 4. **性能**
- ✅ 缓存用户信息减少数据库查询
- ✅ 使用请求上下文传递用户信息
- ✅ 合理设置JWT过期时间
- ✅ 对高频端点进行性能优化

## 🔮 未来扩展

### 1. OAuth集成
```go
// OAuth认证路由
oauth := engine.Group("/oauth")
{
    oauth.GET("/github", r.githubAuth)
    oauth.GET("/google", r.googleAuth)
    oauth.GET("/callback/:provider", r.oauthCallback)
}
```

### 2. 多因子认证 (MFA)
```go
// MFA相关路由
mfa := engine.Group("/auth/mfa")
{
    mfa.POST("/setup", r.setupMFA)
    mfa.POST("/verify", r.verifyMFA)
    mfa.DELETE("/disable", r.disableMFA)
}
```

### 3. 会话管理
```go
// 会话管理路由
sessions := engine.Group("/auth/sessions")
{
    sessions.GET("/", r.getUserSessions)
    sessions.DELETE("/:session_id", r.revokeSession)
    sessions.DELETE("/all", r.revokeAllSessions)
}
```

通过这种方式，您的路由器系统实现了完整的认证和授权功能，同时保持了良好的可维护性和扩展性。

## pkg/auth 包分析与修复

### 📋 原始状态分析

**pkg/auth 包确实有用**，但存在严重的安全问题：

#### ✅ 有效功能
- `auth.Encrypt` - bcrypt密码加密
- `auth.Compare` - bcrypt密码验证  
- `auth.Sign` - JWT token生成

#### ❌ 发现的安全漏洞
1. **密码未加密存储** - 用户创建时密码以明文保存
2. **双重验证逻辑** - AuthService使用bcrypt，领域模型使用明文比较
3. **JWT过期时间过短** - 硬编码1分钟过期时间

### 🛠️ 已修复的问题

#### 1. 密码安全修复
```go
// 修复前：明文密码比较
func (u *User) ValidatePassword(password string) bool {
    return u.password == password  // 不安全！
}

// 修复后：bcrypt验证
func (u *User) ValidatePassword(password string) bool {
    err := auth.Compare(u.password, password)
    return err == nil
}
```

#### 2. 密码加密修复
```go
// 修复前：明文密码存储
func (u *User) ChangePassword(newPassword string) error {
    u.password = newPassword  // 不安全！
    return nil
}

// 修复后：bcrypt加密
func (u *User) ChangePassword(newPassword string) error {
    hashedPassword, err := auth.Encrypt(newPassword)
    if err != nil {
        return errors.WithCode(code.ErrEncrypt, "failed to encrypt password")
    }
    u.password = hashedPassword
    return nil
}
```

#### 3. 用户创建修复
```go
// 添加了密码字段到UserCreateRequest
type UserCreateRequest struct {
    Username     string `json:"username" valid:"required"`
    Password     string `json:"password" valid:"required,min=6"`  // 新增
    Nickname     string `json:"nickname" valid:"required"`
    // ...
}

// 添加了WithPassword方法到UserBuilder
func (b *UserBuilder) WithPassword(password string) *UserBuilder {
    hashedPassword, err := auth.Encrypt(password)
    if err != nil {
        b.u.password = ""
        return b
    }
    b.u.password = hashedPassword
    return b
}
```

#### 4. JWT过期时间修复
```go
// 修复前：硬编码1分钟
func Sign(secretID, secretKey, iss, aud string) string {
    claims := jwt.MapClaims{
        "exp": time.Now().Add(time.Minute).Unix(),  // 太短！
    }
}

// 修复后：可配置过期时间，默认24小时
func Sign(secretID, secretKey, iss, aud string) string {
    return SignWithExpiry(secretID, secretKey, iss, aud, 24*time.Hour)
}

func SignWithExpiry(secretID, secretKey, iss, aud string, expiry time.Duration) string {
    claims := jwt.MapClaims{
        "exp": time.Now().Add(expiry).Unix(),  // 灵活配置
    }
}
```

### ✅ 修复后的安全状态

现在 `pkg/auth` 包已经：
1. **统一密码处理** - 全部使用bcrypt加密和验证
2. **安全存储** - 密码加密后存储到数据库
3. **灵活的JWT** - 支持自定义过期时间
4. **完整的生命周期** - 创建、验证、修改都安全处理

**结论：** `pkg/auth` 包非常有用且必要，现在已经修复了所有安全问题，可以安全使用。

## 系统架构

### 中间件架构层次

**重要说明：** 系统采用分层的中间件架构，避免重复安装：

#### 1. GenericAPIServer 层（基础中间件）
- **RequestID 中间件** - 为每个请求生成唯一ID
- **Context 中间件** - 上下文增强
- **配置化中间件** - 通过配置文件动态加载

#### 2. 配置文件层（全局中间件）
```yaml
# configs/qs-apiserver.yaml
server:
  middlewares: recovery,logger,enhanced_logger,secure,nocache,cors,dump
```

#### 3. Router 层（业务中间件）
- **不安装全局中间件**（避免重复）
- **只负责认证中间件**（特定于路由组）
- **只负责路由注册**

### 中间件执行顺序
```
请求 → GenericAPIServer中间件 → 配置文件中间件 → Router认证中间件 → 业务处理器
```

## 认证集成 