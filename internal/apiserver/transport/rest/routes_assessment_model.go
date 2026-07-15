package rest

import (
	"net/http"

	codesHandler "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

func (r *Router) registerAssessmentModelProtectedRoutes(apiV1 *gin.RouterGroup) {
	if r.deps.AssessmentModel.Management == nil || r.deps.AssessmentModel.Definition == nil || r.deps.AssessmentModel.Query == nil || r.deps.AssessmentModel.Release == nil {
		return
	}
	handler := codesHandler.NewAssessmentModelHandler(r.deps.AssessmentModel.Management, r.deps.AssessmentModel.Definition, r.deps.AssessmentModel.Publication, r.deps.AssessmentModel.Query, r.deps.AssessmentModel.Assets)
	models := apiV1.Group("/assessment-models")
	{
		manage := models.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageAssessmentModels))
		definition := models.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityEditAssessmentModelDefinitions))
		publication := models.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityPublishAssessmentModels))
		read := models.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityReadAssessmentModels))
		registerRouteSpecs(manage, assessmentModelManageRoutes(handler))
		registerRouteSpecs(definition, assessmentModelDefinitionRoutes(handler))
		registerRouteSpecs(publication, assessmentModelPublicationRoutes(handler))
		registerRouteSpecs(read, assessmentModelReadRoutes(handler))
		releases := apiV1.Group("/assessment-releases", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityPublishAssessmentModels))
		releaseHandler := codesHandler.NewAssessmentReleaseHandler(r.deps.AssessmentModel.Release)
		registerRouteSpecs(releases, assessmentReleaseRoutes(releaseHandler))
	}
}

func assessmentModelManageRoutes(handler *codesHandler.AssessmentModelHandler) []routeSpec {
	return []routeSpec{
		{method: http.MethodPost, path: "", handlers: []gin.HandlerFunc{handler.Create}},
		{method: http.MethodPost, path: "/:code/restore-draft", handlers: []gin.HandlerFunc{handler.RestoreDraftFromPublished}},
		{method: http.MethodPut, path: "/:code/basic-info", handlers: []gin.HandlerFunc{handler.UpdateBasicInfo}},
		{method: http.MethodDelete, path: "/:code", handlers: []gin.HandlerFunc{handler.Delete}},
		{method: http.MethodPut, path: "/:code/questionnaire", handlers: []gin.HandlerFunc{handler.BindQuestionnaire}},
	}
}

func assessmentModelDefinitionRoutes(handler *codesHandler.AssessmentModelHandler) []routeSpec {
	return []routeSpec{
		{method: http.MethodPut, path: "/:code/definition", handlers: []gin.HandlerFunc{handler.UpdateDefinition}},
		{method: http.MethodGet, path: "/:code/definition", handlers: []gin.HandlerFunc{handler.GetDefinition}},
		{method: http.MethodPost, path: "/:code/codes/apply", handlers: []gin.HandlerFunc{handler.ApplyCodes}},
		{method: http.MethodPost, path: "/:code/validate", handlers: []gin.HandlerFunc{handler.Validate}},
		{method: http.MethodPost, path: "/:code/preview-report", handlers: []gin.HandlerFunc{handler.PreviewReport}},
		{method: http.MethodPost, path: "/:code/outcomes/:outcome_code/image", handlers: []gin.HandlerFunc{handler.UploadOutcomeImage}},
	}
}

func assessmentModelPublicationRoutes(*codesHandler.AssessmentModelHandler) []routeSpec { return nil }

func assessmentReleaseRoutes(handler *codesHandler.AssessmentReleaseHandler) []routeSpec {
	return []routeSpec{
		{method: http.MethodPost, path: "/:code/publish", handlers: []gin.HandlerFunc{handler.Publish}},
		{method: http.MethodPost, path: "/:code/archive", handlers: []gin.HandlerFunc{handler.Archive}},
	}
}

func assessmentModelReadRoutes(handler *codesHandler.AssessmentModelHandler) []routeSpec {
	return []routeSpec{
		{method: http.MethodGet, path: "/hot", handlers: []gin.HandlerFunc{handler.ListHot}},
		{method: http.MethodGet, path: "/published/:code", handlers: []gin.HandlerFunc{handler.GetPublished}},
		{method: http.MethodGet, path: "/published", handlers: []gin.HandlerFunc{handler.ListPublished}},
		{method: http.MethodGet, path: "/options", handlers: []gin.HandlerFunc{handler.Options}},
		{method: http.MethodGet, path: "/:code/questionnaire", handlers: []gin.HandlerFunc{handler.GetQuestionnaire}},
		{method: http.MethodGet, path: "/:code/qrcode", handlers: []gin.HandlerFunc{handler.GetQRCode}},
		{method: http.MethodGet, path: "/:code", handlers: []gin.HandlerFunc{handler.Get}},
		{method: http.MethodGet, path: "", handlers: []gin.HandlerFunc{handler.List}},
	}
}
