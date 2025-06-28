// 认证集成使用示例
// 这个文件展示了如何在项目中使用新的认证系统

package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/container"
	// 实际项目中的导入路径
	// "github.com/yshujie/questionnaire-scale/internal/apiserver/auth_new"
)

// 示例：在现有项目中集成新认证系统
func main() {
	// 1. 初始化数据库连接（示例）
	var db *gorm.DB // 实际项目中从配置初始化

	// 2. 初始化容器
	container := container.NewContainer(db, nil, "")
	if err := container.Initialize(); err != nil {
		log.Fatalf("Failed to initialize container: %v", err)
	}

	// 3. 创建认证配置
	// authConfig := NewAuthConfig(container)

	// 4. 创建路由器
	router := gin.New()

	// 5. 设置不需要认证的路由
	router.POST("/auth/login", handleLogin)
	router.POST("/auth/register", handleRegister)
	router.GET("/health", handleHealth)

	// 6. 设置需要认证的路由组
	// setupProtectedRoutes(router, authConfig)

	// 7. 启动服务器
	fmt.Println("🚀 服务器启动在 :8080")
	router.Run(":8080")
}

// 登录处理器
func handleLogin(c *gin.Context) {
	type LoginRequest struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// 在实际项目中，这里会使用AuthService
	// authService := container.GetUserModule().GetAuthService()
	// authReq := user.AuthenticateRequest{
	//     Username: req.Username,
	//     Password: req.Password,
	// }
	//
	// authResp, err := authService.Authenticate(c.Request.Context(), authReq)
	// if err != nil {
	//     c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication failed"})
	//     return
	// }

	// 示例响应
	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"token":   "example-jwt-token",
		"expire":  time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"user": gin.H{
			"id":       123,
			"username": req.Username,
			"nickname": "示例用户",
		},
	})
}

// 注册处理器
func handleRegister(c *gin.Context) {
	type RegisterRequest struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Nickname string `json:"nickname" binding:"required"`
	}

	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// 在实际项目中，这里会使用UserCreator
	// userCreator := container.GetUserModule().GetServices()[0].(port.UserCreator)
	// createReq := port.UserCreateRequest{
	//     Username: req.Username,
	//     Nickname: req.Nickname,
	//     Email:    req.Email,
	//     Phone:    "", // 可选
	// }
	//
	// userResp, err := userCreator.CreateUser(c.Request.Context(), createReq)
	// if err != nil {
	//     c.JSON(http.StatusConflict, gin.H{"error": "User creation failed"})
	//     return
	// }

	c.JSON(http.StatusCreated, gin.H{
		"message": "User created successfully",
		"user": gin.H{
			"id":       456,
			"username": req.Username,
			"nickname": req.Nickname,
			"email":    req.Email,
		},
	})
}

// 健康检查
func handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// 设置受保护的路由组
func setupProtectedRoutes(router *gin.Engine, authConfig interface{}) {
	// 在实际项目中的用法：
	// protected := router.Group("/api/v1")
	// protected.Use(authConfig.CreateAuthMiddleware("auto"))
	// {
	//     protected.GET("/users/profile", getUserProfile)
	//     protected.PUT("/users/profile", updateUserProfile)
	//     protected.POST("/users/change-password", changePassword)
	//     protected.GET("/users/:id", getUserById)
	// }

	protected := router.Group("/api/v1")
	protected.Use(mockAuthMiddleware()) // 示例中间件
	{
		protected.GET("/users/profile", getUserProfile)
		protected.PUT("/users/profile", updateUserProfile)
		protected.POST("/users/change-password", changePassword)
	}
}

// 示例认证中间件（实际项目中会使用 authConfig.CreateAuthMiddleware）
func mockAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查Authorization头
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing Authorization header"})
			c.Abort()
			return
		}

		// 简单的令牌验证（实际项目中会使用JWT验证）
		if authHeader != "Bearer example-jwt-token" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// 设置用户上下文（实际项目中会从JWT claims中获取）
		c.Set("username", "testuser")
		c.Set("user_id", 123)

		c.Next()
	}
}

// 获取用户资料
func getUserProfile(c *gin.Context) {
	// 从认证中间件设置的上下文中获取用户信息
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	userID, _ := c.Get("user_id")

	// 在实际项目中，这里会使用AuthService查询用户信息
	// authService := container.GetUserModule().GetAuthService()
	// userInfo, err := authService.GetUserByUsername(c.Request.Context(), username.(string))
	// if err != nil {
	//     c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user info"})
	//     return
	// }

	// 示例响应
	c.JSON(http.StatusOK, gin.H{
		"id":           userID,
		"username":     username,
		"nickname":     "示例用户",
		"email":        "user@example.com",
		"phone":        "13800138000",
		"avatar":       "https://example.com/avatar.jpg",
		"introduction": "这是一个示例用户",
		"status":       "active",
		"created_at":   "2024-01-01T00:00:00Z",
		"updated_at":   "2024-01-01T00:00:00Z",
	})
}

// 更新用户资料
func updateUserProfile(c *gin.Context) {
	type UpdateRequest struct {
		Nickname     string `json:"nickname"`
		Email        string `json:"email"`
		Phone        string `json:"phone"`
		Avatar       string `json:"avatar"`
		Introduction string `json:"introduction"`
	}

	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	username, _ := c.Get("username")
	userID, _ := c.Get("user_id")

	// 在实际项目中，这里会使用UserEditor
	// userEditor := container.GetUserModule().GetServices()[2].(port.UserEditor)
	// updateReq := port.UserBasicInfoRequest{
	//     ID:           userID.(uint64),
	//     Nickname:     req.Nickname,
	//     Email:        req.Email,
	//     Phone:        req.Phone,
	//     Avatar:       req.Avatar,
	//     Introduction: req.Introduction,
	// }
	//
	// userResp, err := userEditor.UpdateBasicInfo(c.Request.Context(), updateReq)
	// if err != nil {
	//     c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
	//     return
	// }

	c.JSON(http.StatusOK, gin.H{
		"message": "Profile updated successfully",
		"user": gin.H{
			"id":           userID,
			"username":     username,
			"nickname":     req.Nickname,
			"email":        req.Email,
			"phone":        req.Phone,
			"avatar":       req.Avatar,
			"introduction": req.Introduction,
		},
	})
}

// 修改密码
func changePassword(c *gin.Context) {
	type ChangePasswordRequest struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6"`
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	username, _ := c.Get("username")
	userID, _ := c.Get("user_id")

	// 在实际项目中，这里会使用AuthService的密码修改功能
	// authService := container.GetUserModule().GetAuthService()
	// err := authService.ChangePasswordWithAuth(
	//     c.Request.Context(),
	//     username.(string),
	//     req.OldPassword,
	//     req.NewPassword,
	// )
	// if err != nil {
	//     c.JSON(http.StatusBadRequest, gin.H{"error": "Password change failed"})
	//     return
	// }

	fmt.Printf("User %d (%s) changed password\n", userID, username)

	c.JSON(http.StatusOK, gin.H{
		"message": "Password changed successfully",
	})
}

/*
🔧 使用方法：

1. 将此文件保存为 main.go
2. 运行：go run main.go
3. 测试不同的端点：

// 注册新用户
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"john","password":"password123","email":"john@example.com","nickname":"John Doe"}'

// 用户登录
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"john","password":"password123"}'

// 获取用户资料（需要认证）
curl -X GET http://localhost:8080/api/v1/users/profile \
  -H "Authorization: Bearer example-jwt-token"

// 更新用户资料（需要认证）
curl -X PUT http://localhost:8080/api/v1/users/profile \
  -H "Authorization: Bearer example-jwt-token" \
  -H "Content-Type: application/json" \
  -d '{"nickname":"John Smith","email":"john.smith@example.com"}'

// 修改密码（需要认证）
curl -X POST http://localhost:8080/api/v1/users/change-password \
  -H "Authorization: Bearer example-jwt-token" \
  -H "Content-Type: application/json" \
  -d '{"old_password":"password123","new_password":"newpassword456"}'

🎯 实际项目集成步骤：

1. 在你的 server.go 或 main.go 中：
   - 导入 AuthConfig
   - 初始化 Container
   - 创建认证配置：authConfig := NewAuthConfig(container)
   - 使用认证中间件：protected.Use(authConfig.CreateAuthMiddleware("auto"))

2. 在路由处理器中：
   - 使用 c.Get("username") 获取当前用户
   - 使用 container.GetUserModule().GetAuthService() 获取认证服务
   - 调用相应的用户查询、更新等方法

3. 配置JWT密钥：
   - 在配置文件中设置 jwt.key, jwt.timeout 等参数
   - 确保密钥的安全性

4. 错误处理：
   - 使用项目统一的错误码和错误处理机制
   - 返回结构化的错误响应
*/
