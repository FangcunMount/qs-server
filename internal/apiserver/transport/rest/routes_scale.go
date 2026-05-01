package rest

import (
	"net/http"

	codesHandler "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

// registerScaleProtectedRoutes 注册量表相关的受保护路由。
func (r *Router) registerScaleProtectedRoutes(apiV1 *gin.RouterGroup) {
	deps := r.deps.Scale
	if deps.LifecycleService == nil || deps.FactorService == nil || deps.QueryService == nil || deps.CategoryService == nil {
		return
	}
	scaleHandler := codesHandler.NewScaleHandler(
		deps.LifecycleService,
		deps.FactorService,
		deps.QueryService,
		deps.CategoryService,
		deps.QRCodeService,
	)

	scales := apiV1.Group("/scales")
	{
		manage := scales.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageScales))
		read := scales.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityReadScales))
		registerRouteSpecs(manage, scaleManageRoutes(scaleHandler))
		registerRouteSpecs(read, scaleReadRoutes(scaleHandler))
	}
}

func scaleManageRoutes(handler *codesHandler.ScaleHandler) []routeSpec {
	return []routeSpec{
		{method: http.MethodPost, path: "", handlers: []gin.HandlerFunc{handler.Create}},
		{method: http.MethodPut, path: "/:code/basic-info", handlers: []gin.HandlerFunc{handler.UpdateBasicInfo}},
		{method: http.MethodPut, path: "/:code/questionnaire", handlers: []gin.HandlerFunc{handler.UpdateQuestionnaire}},
		{method: http.MethodPost, path: "/:code/publish", handlers: []gin.HandlerFunc{handler.Publish}},
		{method: http.MethodPost, path: "/:code/unpublish", handlers: []gin.HandlerFunc{handler.Unpublish}},
		{method: http.MethodPost, path: "/:code/archive", handlers: []gin.HandlerFunc{handler.Archive}},
		{method: http.MethodDelete, path: "/:code", handlers: []gin.HandlerFunc{handler.Delete}},
		{method: http.MethodPut, path: "/:code/factors/batch", handlers: []gin.HandlerFunc{handler.BatchUpdateFactors}},
		{method: http.MethodPut, path: "/:code/interpret-rules", handlers: []gin.HandlerFunc{handler.ReplaceInterpretRules}},
	}
}

func scaleReadRoutes(handler *codesHandler.ScaleHandler) []routeSpec {
	return []routeSpec{
		{method: http.MethodGet, path: "/categories", handlers: []gin.HandlerFunc{handler.GetCategories}},
		{method: http.MethodGet, path: "/by-questionnaire", handlers: []gin.HandlerFunc{handler.GetByQuestionnaireCode}},
		{method: http.MethodGet, path: "/published/:code", handlers: []gin.HandlerFunc{handler.GetPublishedByCode}},
		{method: http.MethodGet, path: "/published", handlers: []gin.HandlerFunc{handler.ListPublished}},
		{method: http.MethodGet, path: "/:code/factors", handlers: []gin.HandlerFunc{handler.GetFactors}},
		{method: http.MethodGet, path: "/:code/qrcode", handlers: []gin.HandlerFunc{handler.GetQRCode}},
		{method: http.MethodGet, path: "/:code", handlers: []gin.HandlerFunc{handler.GetByCode}},
		{method: http.MethodGet, path: "", handlers: []gin.HandlerFunc{handler.List}},
	}
}
