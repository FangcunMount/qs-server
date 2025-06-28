# 🔐 认证集成指南：在Auth中实现用户查询

## 📋 问题背景

在原有的认证实现中，`authenticator()` 函数使用了旧的 `store.Client().Users().Get()` API：

```go
// ❌ 旧的实现方式
user, err := store.Client().Users().Get(c, login.Username, metav1.GetOptions{})
```

但项目已经采用了六边形架构和领域驱动设计，需要集成现有的用户查询服务。

## 🏗️ 解决方案架构

### 1. 认证服务层次

```
🔐 认证中间件层
    ├── AuthConfig (认证配置器)
    ├── AuthService (认证服务)
    └── 认证策略 (Basic/JWT/Auto)

📋 应用服务层
    ├── UserQueryer (用户查询)
    ├── PasswordChanger (密码验证)
    └── UserActivator (状态管理)

🗄️ 领域层
    ├── User 聚合根
    └── UserRepository 端口

🔧 适配器层
    └── MySQL UserRepository 实现
```

### 2. 数据流

```
HTTP请求 → Auth中间件 → AuthService → UserRepository → MySQL数据库
          ↓
      认证结果 ← 用户信息 ← 领域对象 ← 数据库查询
```

## 🚀 实现步骤

### 第一步：创建认证服务

在 `internal/apiserver/application/user/auth_service.go` 中创建：

```go
type AuthService struct {
    userRepo            port.UserRepository
    passwordChanger     port.PasswordChanger  
    userQueryer         port.UserQueryer
    userActivator       port.UserActivator
}

// 核心认证方法
func (a *AuthService) Authenticate(ctx context.Context, req AuthenticateRequest) (*AuthenticateResponse, error) {
    // 1. 查找用户
    userEntity, err := a.userRepo.FindByUsername(ctx, req.Username)
    
    // 2. 检查用户状态
    if !userEntity.IsActive() { ... }
    
    // 3. 验证密码
    if !a.validatePassword(userEntity.Password(), req.Password) { ... }
    
    // 4. 返回用户信息
    return &AuthenticateResponse{User: userResponse}, nil
}
```

### 第二步：集成到用户模块

在 `internal/apiserver/module/user/module.go` 中：

```go
type Module struct {
    // 现有服务
    userRepository      port.UserRepository
    userCreator         port.UserCreator
    userQueryer         port.UserQueryer
    
    // 新增认证服务
    userAuthService     *userApp.AuthService  // 👈 新增
}

func NewModule(db *gorm.DB) *Module {
    // ... 现有初始化
    
    // 新增认证服务初始化
    userAuthService := userApp.NewAuthService(
        userRepository, 
        userPasswordChanger, 
        userQueryer, 
        userActivator,
    )
    
    return &Module{
        // ... 现有字段
        userAuthService: userAuthService,  // 👈 新增
    }
}

// 新增获取方法
func (m *Module) GetAuthService() *userApp.AuthService {
    return m.userAuthService
}
```

### 第三步：创建新的认证配置

在 `internal/apiserver/auth_new.go` 中：

```go
type AuthConfig struct {
    container   *container.Container
    authService *user.AuthService
}

func NewAuthConfig(container *container.Container) *AuthConfig {
    authService := container.GetUserModule().GetAuthService()
    return &AuthConfig{
        container:   container,
        authService: authService,
    }
}

// 创建认证器 - 使用AuthService
func (cfg *AuthConfig) createAuthenticator() func(c *gin.Context) (interface{}, error) {
    return func(c *gin.Context) (interface{}, error) {
        // 解析登录信息
        login, err := cfg.parseLogin(c)
        if err != nil {
            return "", jwt.ErrFailedAuthentication
        }

        // ✅ 使用新的认证服务
        authReq := user.AuthenticateRequest{
            Username: login.Username,
            Password: login.Password,
        }

        authResp, err := cfg.authService.Authenticate(ctx, authReq)
        if err != nil {
            return "", jwt.ErrFailedAuthentication
        }

        return authResp.User, nil
    }
}
```

## 📊 新旧对比

| 方面 | 旧实现 | 新实现 |
|------|--------|--------|
| **数据访问** | `store.Client().Users().Get()` | `authService.Authenticate()` |
| **架构风格** | ❌ 直接依赖存储层 | ✅ 六边形架构 |
| **业务逻辑** | ❌ 散落在认证中间件中 | ✅ 集中在领域服务中 |
| **可测试性** | ❌ 难以模拟store | ✅ 易于注入mock服务 |
| **状态检查** | ❌ 基础检查 | ✅ 完整的状态验证 |
| **密码验证** | ❌ 简单比较 | ✅ 加密后比较 |
| **错误处理** | ❌ 基础错误 | ✅ 结构化错误码 |

## 🔧 使用示例

### 1. 在路由中使用认证

```go
func setupAuthenticatedRoutes(container *container.Container) *gin.Engine {
    router := gin.New()
    
    // 创建认证配置
    authConfig := NewAuthConfig(container)
    
    // 应用认证中间件
    authMiddleware := authConfig.CreateAuthMiddleware("auto")
    
    // 保护的路由组
    protected := router.Group("/api/v1")
    protected.Use(authMiddleware)
    {
        protected.GET("/users/profile", getUserProfile)
        protected.PUT("/users/profile", updateUserProfile)
        protected.POST("/users/change-password", changePassword)
    }
    
    return router
}
```

### 2. 在处理器中获取当前用户

```go
func getUserProfile(c *gin.Context) {
    // 从认证中间件设置的上下文中获取用户名
    username, exists := c.Get(middleware.UsernameKey)
    if !exists {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
        return
    }
    
    // 使用用户查询服务获取完整信息
    userService := container.GetUserModule().GetAuthService()
    userInfo, err := userService.GetUserByUsername(c, username.(string))
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user info"})
        return
    }
    
    c.JSON(http.StatusOK, userInfo)
}
```

### 3. 不同的认证策略

```go
// Basic认证（用户名密码）
basicAuth := authConfig.CreateAuthMiddleware("basic")

// JWT认证（令牌）
jwtAuth := authConfig.CreateAuthMiddleware("jwt")

// 自动认证（根据请求头自动选择）
autoAuth := authConfig.CreateAuthMiddleware("auto")
```

## 🔍 核心方法详解

### AuthService.Authenticate()

```go
func (a *AuthService) Authenticate(ctx context.Context, req AuthenticateRequest) (*AuthenticateResponse, error) {
    // 1️⃣ 用户查询 - 使用六边形架构的Repository
    userEntity, err := a.userRepo.FindByUsername(ctx, req.Username)
    
    // 2️⃣ 状态检查 - 使用领域对象的业务方法
    if userEntity.IsBlocked() {
        return nil, errors.WithCode(code.ErrUserBlocked, "user is blocked")
    }
    
    // 3️⃣ 密码验证 - 使用加密后的密码比较
    if !a.validatePassword(userEntity.Password(), req.Password) {
        return nil, errors.WithCode(code.ErrPasswordIncorrect, "invalid password")
    }
    
    // 4️⃣ 构造响应 - 转换为应用层DTO
    return &AuthenticateResponse{User: userResponse}, nil
}
```

### AuthService.ValidatePasswordOnly()

```go
// 用于Basic认证的简化验证
func (a *AuthService) ValidatePasswordOnly(ctx context.Context, username, password string) (*port.UserResponse, error) {
    return a.passwordChanger.ValidatePassword(ctx, username, password)
}
```

### AuthService.GetUserByUsername()

```go
// 用于JWT认证的用户信息获取
func (a *AuthService) GetUserByUsername(ctx context.Context, username string) (*port.UserResponse, error) {
    userEntity, err := a.userRepo.FindByUsername(ctx, username)
    // ... 转换为UserResponse
}
```

## 🎯 集成优势

### 1. **架构一致性**
- ✅ 遵循六边形架构原则
- ✅ 认证逻辑与业务逻辑解耦
- ✅ 依赖注入和控制反转

### 2. **业务完整性** 
- ✅ 完整的用户状态检查
- ✅ 加密密码验证
- ✅ 统一的错误处理

### 3. **可维护性**
- ✅ 认证逻辑集中管理
- ✅ 易于扩展和修改
- ✅ 单元测试友好

### 4. **安全性**
- ✅ 密码加密存储和验证
- ✅ 用户状态实时检查
- ✅ 结构化的认证流程

## 🧪 测试示例

```go
func TestAuthService_Authenticate(t *testing.T) {
    // 模拟依赖
    mockRepo := &MockUserRepository{}
    mockPasswordChanger := &MockPasswordChanger{}
    mockQueryer := &MockUserQueryer{}
    mockActivator := &MockUserActivator{}
    
    // 创建认证服务
    authService := NewAuthService(mockRepo, mockPasswordChanger, mockQueryer, mockActivator)
    
    // 测试认证
    req := AuthenticateRequest{
        Username: "testuser",
        Password: "testpass",
    }
    
    resp, err := authService.Authenticate(context.Background(), req)
    
    assert.NoError(t, err)
    assert.Equal(t, "testuser", resp.User.Username)
}
```

## 🔮 下一步扩展

### 1. 添加权限管理
```go
type AuthService struct {
    // ... 现有字段
    permissionService port.PermissionService  // 新增权限服务
}

func (a *AuthService) CheckPermission(userID uint64, resource, action string) bool {
    return a.permissionService.HasPermission(userID, resource, action)
}
```

### 2. 添加多因子认证
```go
func (a *AuthService) AuthenticateWithMFA(ctx context.Context, req MFARequest) (*AuthenticateResponse, error) {
    // 1. 基础认证
    // 2. MFA验证
    // 3. 返回认证结果
}
```

### 3. 添加OAuth集成
```go
func (a *AuthService) AuthenticateWithOAuth(ctx context.Context, provider string, token string) (*AuthenticateResponse, error) {
    // OAuth认证逻辑
}
```

通过这种方式，您的认证系统完全集成了现有的六边形架构，实现了真正的用户查询和认证一体化。 