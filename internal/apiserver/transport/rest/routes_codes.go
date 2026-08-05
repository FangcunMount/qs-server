package rest

import (
	codesHandler "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

// registerCodesRoutes 注册 codes 申请路由。
func (r *Router) registerCodesRoutes(apiV1 *gin.RouterGroup) {
	if r.deps.CodesService == nil {
		return
	}

	handler := codesHandler.NewCodesHandler(r.deps.CodesService)
	codes := apiV1.Group("/codes", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
	codes.POST("/apply", handler.Apply)
}
