package rest

import (
	"net/http"

	handler "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	middleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

func (r *Router) registerNormTableProtectedRoutes(apiV1 *gin.RouterGroup) {
	if r.deps.AssessmentModel.NormTables == nil {
		return
	}
	normHandler := handler.NewNormTableHandler(r.deps.AssessmentModel.NormTables)
	read := apiV1.Group("/norm-tables", middleware.RequireCapabilityMiddleware(middleware.CapabilityReadNormTables))
	manage := apiV1.Group("/norm-tables", middleware.RequireCapabilityMiddleware(middleware.CapabilityManageNormTables))
	registerRouteSpecs(read, []routeSpec{
		{method: http.MethodGet, path: "", handlers: []gin.HandlerFunc{normHandler.List}},
		{method: http.MethodGet, path: "/:version", handlers: []gin.HandlerFunc{normHandler.Get}},
	})
	registerRouteSpecs(manage, []routeSpec{{method: http.MethodPost, path: "", handlers: []gin.HandlerFunc{normHandler.Import}}})
}
