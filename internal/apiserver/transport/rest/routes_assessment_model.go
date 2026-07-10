package rest

import (
	"net/http"

	codesHandler "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

func (r *Router) registerAssessmentModelProtectedRoutes(apiV1 *gin.RouterGroup) {
	if r.deps.AssessmentModel.Service == nil {
		return
	}
	handler := codesHandler.NewAssessmentModelHandler(r.deps.AssessmentModel.Service, r.deps.AssessmentModel.Management, r.deps.AssessmentModel.Publication)
	models := apiV1.Group("/assessment-models")
	{
		manage := models.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageAssessmentModels))
		read := models.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityReadAssessmentModels))
		registerRouteSpecs(manage, assessmentModelManageRoutes(handler))
		registerRouteSpecs(read, assessmentModelReadRoutes(handler))
	}
}

func assessmentModelManageRoutes(handler *codesHandler.AssessmentModelHandler) []routeSpec {
	return []routeSpec{
		{method: http.MethodPost, path: "", handlers: []gin.HandlerFunc{handler.Create}},
		{method: http.MethodPut, path: "/:code/basic-info", handlers: []gin.HandlerFunc{handler.UpdateBasicInfo}},
		{method: http.MethodDelete, path: "/:code", handlers: []gin.HandlerFunc{handler.Delete}},
		{method: http.MethodPost, path: "/:code/publish", handlers: []gin.HandlerFunc{handler.Publish}},
		{method: http.MethodPost, path: "/:code/unpublish", handlers: []gin.HandlerFunc{handler.Unpublish}},
		{method: http.MethodPost, path: "/:code/archive", handlers: []gin.HandlerFunc{handler.Archive}},
		{method: http.MethodPut, path: "/:code/questionnaire", handlers: []gin.HandlerFunc{handler.BindQuestionnaire}},
		{method: http.MethodPut, path: "/:code/definition", handlers: []gin.HandlerFunc{handler.UpdateDefinition}},
		{method: http.MethodPost, path: "/:code/codes/apply", handlers: []gin.HandlerFunc{handler.ApplyCodes}},
		{method: http.MethodPost, path: "/:code/validate", handlers: []gin.HandlerFunc{handler.Validate}},
		{method: http.MethodPost, path: "/:code/preview-report", handlers: []gin.HandlerFunc{handler.PreviewReport}},
	}
}

func assessmentModelReadRoutes(handler *codesHandler.AssessmentModelHandler) []routeSpec {
	return []routeSpec{
		{method: http.MethodGet, path: "/options", handlers: []gin.HandlerFunc{handler.Options}},
		{method: http.MethodGet, path: "/:code/questionnaire", handlers: []gin.HandlerFunc{handler.GetQuestionnaire}},
		{method: http.MethodGet, path: "/:code/definition", handlers: []gin.HandlerFunc{handler.GetDefinition}},
		{method: http.MethodGet, path: "/:code/qrcode", handlers: []gin.HandlerFunc{handler.GetQRCode}},
		{method: http.MethodGet, path: "/:code", handlers: []gin.HandlerFunc{handler.Get}},
		{method: http.MethodGet, path: "", handlers: []gin.HandlerFunc{handler.List}},
	}
}
