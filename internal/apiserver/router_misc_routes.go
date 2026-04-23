package apiserver

import (
	codesHandler "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
	"github.com/gin-gonic/gin"
)

// registerUserProtectedRoutes 注册用户相关的受保护路由。
// 用户管理已迁移到 IAM 服务，此方法保留以便未来扩展。
func (r *Router) registerUserProtectedRoutes(_ *gin.RouterGroup) {
	// 用户相关功能已迁移到 iam-contracts 项目
}

// registerCodesRoutes 注册 codes 申请路由。
func (r *Router) registerCodesRoutes(apiV1 *gin.RouterGroup) {
	if r.container == nil || r.container.CodesService == nil {
		return
	}

	handler := codesHandler.NewCodesHandler(r.container.CodesService)
	codes := apiV1.Group("/codes", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	codes.POST("/apply", handler.Apply)
}

// registerAdminRoutes 注册管理员路由。
func (r *Router) registerAdminRoutes(apiV1 *gin.RouterGroup) {
	admin := apiV1.Group("/admin")
	// admin.Use(r.requireAdminRole()) // 需要实现管理员权限检查中间件
	{
		admin.GET("/users", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			r.unsupportedFeature,
		)...)
		admin.GET("/statistics", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			r.unsupportedFeature,
		)...)
		admin.GET("/logs", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			r.unsupportedFeature,
		)...)
	}
}
