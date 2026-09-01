package rest

import (
	"net/http"

	authzapp "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	codesHandler "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	"github.com/gin-gonic/gin"
)

// registerQuestionnaireProtectedRoutes 注册问卷相关的受保护路由。
func (r *Router) registerQuestionnaireProtectedRoutes(apiV1 *gin.RouterGroup) {
	deps := r.deps.Survey
	if deps.QuestionnaireLifecycleService == nil || deps.QuestionnaireContentService == nil || deps.QuestionnaireQueryService == nil {
		return
	}
	quesHandler := codesHandler.NewQuestionnaireHandler(
		deps.QuestionnaireLifecycleService,
		deps.QuestionnaireContentService,
		deps.QuestionnaireQueryService,
		deps.QuestionnaireQRCodeService,
	)

	questionnaires := apiV1.Group("/questionnaires")
	{
		registerRouteSpecs(questionnaires, questionnaireRoutes(quesHandler))
	}
}

// registerAnswersheetProtectedRoutes 注册答卷相关的受保护路由。
func (r *Router) registerAnswersheetProtectedRoutes(apiV1 *gin.RouterGroup) {
	deps := r.deps.Survey
	if deps.AnswerSheetManagementService == nil || deps.AnswerSheetSubmissionService == nil {
		return
	}
	answersheetHandler := codesHandler.NewAnswerSheetHandler(
		deps.AnswerSheetManagementService,
		deps.AnswerSheetSubmissionService,
	)

	answersheets := apiV1.Group("/answersheets")
	{
		answersheets.POST("/admin-submit", withPermission(authzapp.AnswerSheetResource, "admin_submit", r.rateLimitedHandlers(rateLimitBudgetAdminSubmit, answersheetHandler.AdminSubmit)...)...)
		answersheets.GET("/:id", withPermission(authzapp.AnswerSheetResource, "read", r.rateLimitedHandlers(rateLimitBudgetQuery, answersheetHandler.GetByID)...)...)
		answersheets.GET("", withPermission(authzapp.AnswerSheetResource, "list", r.rateLimitedHandlers(rateLimitBudgetQuery, answersheetHandler.List)...)...)
	}
}

func questionnaireRoutes(handler *codesHandler.QuestionnaireHandler) []routeSpec {
	return []routeSpec{
		{method: http.MethodPost, path: "", handlers: withPermission(authzapp.QuestionnaireResource, "create", handler.Create)},
		{method: http.MethodPut, path: "/:code/basic-info", handlers: withPermission(authzapp.QuestionnaireResource, "update", handler.UpdateBasicInfo)},
		{method: http.MethodPost, path: "/:code/draft", handlers: withPermission(authzapp.QuestionnaireResource, "update", handler.SaveDraft)},
		// Standalone questionnaire management remains available until every
		// legacy survey is migrated to an assessment release. Questionnaire-bound
		// assessment editors no longer call these routes.
		{method: http.MethodPost, path: "/:code/publish", handlers: withPermission(authzapp.QuestionnaireResource, "publish", handler.Publish)},
		{method: http.MethodPost, path: "/:code/unpublish", handlers: withPermission(authzapp.QuestionnaireResource, "unpublish", handler.Unpublish)},
		{method: http.MethodPost, path: "/:code/archive", handlers: withPermission(authzapp.QuestionnaireResource, "archive", handler.Archive)},
		{method: http.MethodDelete, path: "/:code", handlers: withPermission(authzapp.QuestionnaireResource, "delete", handler.Delete)},
		{method: http.MethodPost, path: "/:code/questions", handlers: withPermission(authzapp.QuestionnaireResource, "update", handler.AddQuestion)},
		{method: http.MethodPut, path: "/:code/questions/:qcode", handlers: withPermission(authzapp.QuestionnaireResource, "update", handler.UpdateQuestion)},
		{method: http.MethodDelete, path: "/:code/questions/:qcode", handlers: withPermission(authzapp.QuestionnaireResource, "update", handler.RemoveQuestion)},
		{method: http.MethodPost, path: "/:code/questions/reorder", handlers: withPermission(authzapp.QuestionnaireResource, "update", handler.ReorderQuestions)},
		{method: http.MethodPut, path: "/:code/questions/batch", handlers: withPermission(authzapp.QuestionnaireResource, "update", handler.BatchUpdateQuestions)},
		{method: http.MethodGet, path: "", handlers: withPermission(authzapp.QuestionnaireResource, "list", handler.List)},
		{method: http.MethodGet, path: "/:code/versions", handlers: withPermission(authzapp.QuestionnaireResource, "read", handler.ListVersions)},
		{method: http.MethodGet, path: "/:code", handlers: withPermission(authzapp.QuestionnaireResource, "read", handler.GetByCode)},
		{method: http.MethodGet, path: "/published/:code", handlers: withPermission(authzapp.QuestionnaireResource, "read", handler.GetPublishedByCode)},
		{method: http.MethodGet, path: "/published", handlers: withPermission(authzapp.QuestionnaireResource, "list", handler.ListPublished)},
		{method: http.MethodGet, path: "/:code/qrcode", handlers: withPermission(authzapp.QuestionnaireResource, "read", handler.GetQRCode)},
	}
}
