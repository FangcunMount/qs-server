package rest

import (
	"net/http"

	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	codesHandler "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	"github.com/gin-gonic/gin"
)

func (r *Router) registerAssessmentModelProtectedRoutes(apiV1 *gin.RouterGroup) {
	if r.deps.AssessmentModel.Management == nil || r.deps.AssessmentModel.Definition == nil || r.deps.AssessmentModel.Query == nil || r.deps.AssessmentModel.Release == nil {
		return
	}
	handler := codesHandler.NewAssessmentModelHandler(r.deps.AssessmentModel.Management, r.deps.AssessmentModel.Definition, r.deps.AssessmentModel.Query, r.deps.AssessmentModel.Assets)
	models := apiV1.Group("/assessment-models")
	{
		registerRouteSpecs(models, assessmentModelRoutes(handler))
		releaseHandler := codesHandler.NewAssessmentReleaseHandler(r.deps.AssessmentModel.Release, r.deps.AssessmentModel.Query)
		releases := apiV1.Group("/assessment-releases")
		registerRouteSpecs(releases, assessmentReleaseRoutes(releaseHandler))
	}
}

func assessmentModelRoutes(handler *codesHandler.AssessmentModelHandler) []routeSpec {
	return []routeSpec{
		{method: http.MethodPost, path: "", handlers: withPermission(authzapp.AssessmentModelResource, "create", handler.Create)},
		{method: http.MethodPut, path: "/:code/basic-info", handlers: withPermission(authzapp.AssessmentModelResource, "update", handler.UpdateBasicInfo)},
		{method: http.MethodDelete, path: "/:code", handlers: withPermission(authzapp.AssessmentModelResource, "delete", handler.Delete)},
		{method: http.MethodPut, path: "/:code/questionnaire", handlers: withPermission(authzapp.AssessmentModelResource, "update", handler.BindQuestionnaire)},
		{method: http.MethodPut, path: "/:code/definition", handlers: withPermission(authzapp.AssessmentModelResource, "update", handler.UpdateDefinition)},
		{method: http.MethodGet, path: "/:code/definition", handlers: withPermission(authzapp.AssessmentModelResource, "read", handler.GetDefinition)},
		{method: http.MethodPost, path: "/:code/codes/apply", handlers: withPermission(authzapp.AssessmentModelResource, "update", handler.ApplyCodes)},
		{method: http.MethodPost, path: "/:code/validate", handlers: withPermission(authzapp.AssessmentModelResource, "read", handler.Validate)},
		{method: http.MethodPost, path: "/:code/preview-report", handlers: withPermission(authzapp.AssessmentModelResource, "read", handler.PreviewReport)},
		{method: http.MethodPost, path: "/:code/outcomes/:outcome_code/image", handlers: withPermission(authzapp.AssessmentModelResource, "update", handler.UploadOutcomeImage)},
		{method: http.MethodGet, path: "/hot", handlers: withPermission(authzapp.AssessmentModelResource, "list", handler.ListHot)},
		{method: http.MethodGet, path: "/published/:code", handlers: withPermission(authzapp.AssessmentModelResource, "read", handler.GetPublished)},
		{method: http.MethodGet, path: "/published", handlers: withPermission(authzapp.AssessmentModelResource, "list", handler.ListPublished)},
		{method: http.MethodGet, path: "/options", handlers: withPermission(authzapp.AssessmentModelResource, "list", handler.Options)},
		{method: http.MethodGet, path: "/:code/questionnaire", handlers: withPermission(authzapp.AssessmentModelResource, "read", handler.GetQuestionnaire)},
		{method: http.MethodGet, path: "/:code/qrcode", handlers: withPermission(authzapp.AssessmentModelResource, "read", handler.GetQRCode)},
		{method: http.MethodGet, path: "/:code", handlers: withPermission(authzapp.AssessmentModelResource, "read", handler.Get)},
		{method: http.MethodGet, path: "", handlers: withPermission(authzapp.AssessmentModelResource, "list", handler.List)},
	}
}

func assessmentReleaseRoutes(handler *codesHandler.AssessmentReleaseHandler) []routeSpec {
	return []routeSpec{
		{method: http.MethodPost, path: "/:code/publish", handlers: withPermission(authzapp.AssessmentModelResource, "publish", handler.Publish)},
		{method: http.MethodPost, path: "/:code/unpublish", handlers: withPermission(authzapp.AssessmentModelResource, "unpublish", handler.Unpublish)},
		{method: http.MethodPost, path: "/:code/archive", handlers: withPermission(authzapp.AssessmentModelResource, "archive", handler.Archive)},
		{method: http.MethodGet, path: "/:code/versions", handlers: withPermission(authzapp.AssessmentModelResource, "read", handler.Versions)},
	}
}
