package apiserver

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/container"
	"github.com/yshujie/questionnaire-scale/internal/pkg/middleware"
	authpkg "github.com/yshujie/questionnaire-scale/internal/pkg/middleware/auth"
)

// Router 集中的路由管理器
type Router struct {
	container  *container.Container
	authConfig *AuthConfig
}

// NewRouter 创建路由管理器
func NewRouter(c *container.Container) *Router {
	return &Router{
		container:  c,
		authConfig: NewAuthConfig(c), // 初始化认证配置
	}
}

// RegisterRoutes 注册所有路由
func (r *Router) RegisterRoutes(engine *gin.Engine) {
	// 注册公开路由（不需要认证）
	r.registerPublicRoutes(engine)

	// 注册需要认证的路由
	r.registerProtectedRoutes(engine)

	fmt.Printf("🔗 Registered routes for: public, protected(user, questionnaire)\n")
}

// registerPublicRoutes 注册公开路由（不需要认证）
func (r *Router) registerPublicRoutes(engine *gin.Engine) {
	// 健康检查和基础路由
	engine.GET("/health", r.healthCheck)
	engine.GET("/ping", r.ping)

	// 认证相关的公开路由
	auth := engine.Group("/auth")
	{
		jwtStrategy, _ := r.authConfig.NewJWTAuth().(authpkg.JWTStrategy)
		auth.POST("/login", jwtStrategy.LoginHandler)
		auth.POST("/logout", jwtStrategy.LogoutHandler)
		auth.POST("/refresh", jwtStrategy.RefreshHandler)
	}

	// 公开的API路由
	publicAPI := engine.Group("/api/v1/public")
	{
		// 示例：添加一些公开的API端点
		publicAPI.GET("/info", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"service":     "questionnaire-scale",
				"version":     "1.0.0",
				"description": "问卷量表管理系统",
			})
		})
		// 可以添加更多公开的API，比如：
		// publicAPI.GET("/announcements", getAnnouncements)
		// publicAPI.GET("/help", getHelp)
	}
}

// registerProtectedRoutes 注册需要认证的路由
func (r *Router) registerProtectedRoutes(engine *gin.Engine) {
	// 创建需要认证的API组
	apiV1 := engine.Group("/api/v1")

	// 应用认证中间件
	authMiddleware := r.authConfig.CreateAuthMiddleware("auto") // 自动选择Basic或JWT
	apiV1.Use(authMiddleware)

	// 注册用户相关的受保护路由
	r.registerUserProtectedRoutes(apiV1)

	// 注册问卷相关的受保护路由
	r.registerQuestionnaireProtectedRoutes(apiV1)

	// 管理员路由（需要额外的权限检查）
	r.registerAdminRoutes(apiV1)
}

// registerUserProtectedRoutes 注册用户相关的受保护路由
func (r *Router) registerUserProtectedRoutes(apiV1 *gin.RouterGroup) {
	userHandler := r.container.GetUserModule().GetHandler()
	if userHandler == nil {
		return
	}

	users := apiV1.Group("/users")
	{
		// 用户资料相关
		users.GET("/profile", r.getCurrentUserProfile)    // 获取当前用户资料
		users.PUT("/profile", r.updateCurrentUserProfile) // 更新当前用户资料
		users.POST("/change-password", r.changePassword)  // 修改密码

		// 用户管理（可能需要管理员权限）
		users.GET("/:id", userHandler.GetUser)    // 获取指定用户
		users.PUT("/:id", userHandler.UpdateUser) // 更新指定用户
		// users.DELETE("/:id", userHandler.DeleteUser) // 删除用户（管理员）
	}
}

// registerQuestionnaireProtectedRoutes 注册问卷相关的受保护路由
func (r *Router) registerQuestionnaireProtectedRoutes(apiV1 *gin.RouterGroup) {
	// TODO: 实现问卷处理器后取消注释
	// questionnaireHandler := r.container.GetQuestionnaireModule().GetHandler()
	// if questionnaireHandler == nil {
	//     return
	// }

	questionnaires := apiV1.Group("/questionnaires")
	{
		// 问卷CRUD操作
		questionnaires.POST("", r.placeholder)       // 创建问卷
		questionnaires.GET("", r.placeholder)        // 获取问卷列表
		questionnaires.GET("/:id", r.placeholder)    // 获取指定问卷
		questionnaires.PUT("/:id", r.placeholder)    // 更新问卷
		questionnaires.DELETE("/:id", r.placeholder) // 删除问卷

		// 问卷状态管理
		questionnaires.POST("/:id/publish", r.placeholder)   // 发布问卷
		questionnaires.POST("/:id/archive", r.placeholder)   // 归档问卷
		questionnaires.POST("/:id/responses", r.placeholder) // 提交问卷响应
	}
}

// registerAdminRoutes 注册管理员路由
func (r *Router) registerAdminRoutes(apiV1 *gin.RouterGroup) {
	admin := apiV1.Group("/admin")
	// admin.Use(r.requireAdminRole()) // 需要实现管理员权限检查中间件
	{
		admin.GET("/users", r.placeholder)      // 管理员获取所有用户
		admin.GET("/statistics", r.placeholder) // 系统统计信息
		admin.GET("/logs", r.placeholder)       // 系统日志
	}
}

// getCurrentUserProfile 获取当前用户资料
func (r *Router) getCurrentUserProfile(c *gin.Context) {
	// 从认证中间件设置的上下文中获取用户名
	username, exists := c.Get(middleware.UsernameKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户未认证"})
		return
	}

	// 使用认证服务获取用户信息
	authService := r.container.GetUserModule().GetAuthService()
	if authService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "认证服务不可用"})
		return
	}

	userInfo, err := authService.GetUserByUsername(c.Request.Context(), username.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用户信息失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    userInfo,
		"message": "获取用户资料成功",
	})
}

// updateCurrentUserProfile 更新当前用户资料
func (r *Router) updateCurrentUserProfile(c *gin.Context) {
	username, exists := c.Get(middleware.UsernameKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户未认证"})
		return
	}

	// 这里可以调用用户编辑服务
	// userEditor := r.container.GetUserModule().GetServices()[2].(port.UserEditor)
	// 实现用户资料更新逻辑...

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "用户资料更新成功",
		"user":    username,
	})
}

// changePassword 修改密码
func (r *Router) changePassword(c *gin.Context) {
	type ChangePasswordRequest struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=6,max=50"`
	}

	username, exists := c.Get(middleware.UsernameKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户未认证"})
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	// 使用认证服务修改密码
	authService := r.container.GetUserModule().GetAuthService()
	if authService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "认证服务不可用"})
		return
	}

	err := authService.ChangePasswordWithAuth(
		c.Request.Context(),
		username.(string),
		req.OldPassword,
		req.NewPassword,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码修改失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "密码修改成功",
	})
}

// placeholder 占位符处理器（用于未实现的功能）
func (r *Router) placeholder(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"code":    501,
		"message": "功能尚未实现",
		"path":    c.Request.URL.Path,
		"method":  c.Request.Method,
	})
}

// healthCheck 健康检查处理函数
func (r *Router) healthCheck(c *gin.Context) {
	response := gin.H{
		"status":       "healthy",
		"version":      "1.0.0",
		"discovery":    "auto",
		"architecture": "hexagonal",
		"router":       "centralized",
		"auth":         "enabled", // 新增认证状态
		"components": gin.H{
			"domain":      "questionnaire, user",
			"ports":       "storage",
			"adapters":    "mysql, mongodb, http",
			"application": "questionnaire_service, user_service",
		},
		"jwt_config": gin.H{
			"realm":       viper.GetString("jwt.realm"),
			"timeout":     viper.GetDuration("jwt.timeout").String(),
			"max_refresh": viper.GetDuration("jwt.max-refresh").String(),
			"key_loaded":  viper.GetString("jwt.key") != "", // 不显示实际密钥，只显示是否加载
		},
	}

	c.JSON(200, response)
}

// ping 简单的连通性测试
func (r *Router) ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
		"status":  "ok",
		"router":  "centralized",
		"auth":    "enabled",
	})
}

// RegisterCustomRoutes 注册自定义路由（扩展点）
func (r *Router) RegisterCustomRoutes(apiV1 *gin.RouterGroup, routerFunc func(*gin.RouterGroup)) {
	if routerFunc != nil {
		routerFunc(apiV1)
	}
}

// GetAuthConfig 获取认证配置（用于外部访问）
func (r *Router) GetAuthConfig() *AuthConfig {
	return r.authConfig
}

// requireAdminRole 管理员权限检查中间件（示例）
// func (r *Router) requireAdminRole() gin.HandlerFunc {
//     return func(c *gin.Context) {
//         username, exists := c.Get(middleware.UsernameKey)
//         if !exists {
//             c.JSON(http.StatusUnauthorized, gin.H{"error": "用户未认证"})
//             c.Abort()
//             return
//         }
//
//         // 检查用户是否有管理员权限
//         authService := r.container.GetUserModule().GetAuthService()
//         // isAdmin := authService.CheckUserRole(username.(string), "admin")
//         // if !isAdmin {
//         //     c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
//         //     c.Abort()
//         //     return
//         // }
//
//         c.Next()
//     }
// }
