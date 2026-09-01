package rest

import (
	"net/http"

	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	handler "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	"github.com/gin-gonic/gin"
)

func (r *Router) registerNormTableProtectedRoutes(apiV1 *gin.RouterGroup) {
	if r.deps.AssessmentModel.NormTables == nil {
		return
	}
	normHandler := handler.NewNormTableHandler(r.deps.AssessmentModel.NormTables)
	normTables := apiV1.Group("/norm-tables")
	registerRouteSpecs(normTables, []routeSpec{
		{method: http.MethodGet, path: "", handlers: withPermission(authzapp.NormTableResource, "list", normHandler.List)},
		{method: http.MethodGet, path: "/:version", handlers: withPermission(authzapp.NormTableResource, "read", normHandler.Get)},
		{method: http.MethodPost, path: "", handlers: withPermission(authzapp.NormTableResource, "import", normHandler.Import)},
	})
}
