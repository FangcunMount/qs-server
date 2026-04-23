package rest

import (
	"net/http"

	codesHandler "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
	"github.com/gin-gonic/gin"
)

// registerQuestionnaireProtectedRoutes 注册问卷相关的受保护路由。
func (r *Router) registerQuestionnaireProtectedRoutes(apiV1 *gin.RouterGroup) {
	quesHandler := r.deps.Survey.QuestionnaireHandler
	if quesHandler == nil {
		return
	}

	questionnaires := apiV1.Group("/questionnaires")
	{
		manage := questionnaires.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityManageQuestionnaires))
		read := questionnaires.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityReadQuestionnaires))
		registerRouteSpecs(manage, questionnaireManageRoutes(quesHandler))
		registerRouteSpecs(read, questionnaireReadRoutes(quesHandler))
	}
}

// registerAnswersheetProtectedRoutes 注册答卷相关的受保护路由。
func (r *Router) registerAnswersheetProtectedRoutes(apiV1 *gin.RouterGroup) {
	answersheetHandler := r.deps.Survey.AnswerSheetHandler
	if answersheetHandler == nil {
		return
	}

	answersheets := apiV1.Group("/answersheets")
	{
		admin := answersheets.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityOrgAdmin))
		read := answersheets.Group("", restmiddleware.RequireCapabilityMiddleware(restmiddleware.CapabilityReadAnswersheets))

		admin.POST("/admin-submit", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.AdminSubmitGlobalQPS,
			r.rateCfg.AdminSubmitGlobalBurst,
			r.rateCfg.AdminSubmitUserQPS,
			r.rateCfg.AdminSubmitUserBurst,
			answersheetHandler.AdminSubmit,
		)...)
		read.GET("/:id", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			answersheetHandler.GetByID,
		)...)
		read.GET("", r.rateLimitedHandlers(
			r.rateCfg,
			r.rateCfg.QueryGlobalQPS,
			r.rateCfg.QueryGlobalBurst,
			r.rateCfg.QueryUserQPS,
			r.rateCfg.QueryUserBurst,
			answersheetHandler.List,
		)...)
	}
}

func questionnaireManageRoutes(handler *codesHandler.QuestionnaireHandler) []routeSpec {
	return []routeSpec{
		{method: http.MethodPost, path: "", handlers: []gin.HandlerFunc{handler.Create}},
		{method: http.MethodPut, path: "/:code/basic-info", handlers: []gin.HandlerFunc{handler.UpdateBasicInfo}},
		{method: http.MethodPost, path: "/:code/draft", handlers: []gin.HandlerFunc{handler.SaveDraft}},
		{method: http.MethodPost, path: "/:code/publish", handlers: []gin.HandlerFunc{handler.Publish}},
		{method: http.MethodPost, path: "/:code/unpublish", handlers: []gin.HandlerFunc{handler.Unpublish}},
		{method: http.MethodPost, path: "/:code/archive", handlers: []gin.HandlerFunc{handler.Archive}},
		{method: http.MethodDelete, path: "/:code", handlers: []gin.HandlerFunc{handler.Delete}},
		{method: http.MethodPost, path: "/:code/questions", handlers: []gin.HandlerFunc{handler.AddQuestion}},
		{method: http.MethodPut, path: "/:code/questions/:qcode", handlers: []gin.HandlerFunc{handler.UpdateQuestion}},
		{method: http.MethodDelete, path: "/:code/questions/:qcode", handlers: []gin.HandlerFunc{handler.RemoveQuestion}},
		{method: http.MethodPost, path: "/:code/questions/reorder", handlers: []gin.HandlerFunc{handler.ReorderQuestions}},
		{method: http.MethodPut, path: "/:code/questions/batch", handlers: []gin.HandlerFunc{handler.BatchUpdateQuestions}},
	}
}

func questionnaireReadRoutes(handler *codesHandler.QuestionnaireHandler) []routeSpec {
	return []routeSpec{
		{method: http.MethodGet, path: "", handlers: []gin.HandlerFunc{handler.List}},
		{method: http.MethodGet, path: "/:code", handlers: []gin.HandlerFunc{handler.GetByCode}},
		{method: http.MethodGet, path: "/published/:code", handlers: []gin.HandlerFunc{handler.GetPublishedByCode}},
		{method: http.MethodGet, path: "/published", handlers: []gin.HandlerFunc{handler.ListPublished}},
		{method: http.MethodGet, path: "/:code/qrcode", handlers: []gin.HandlerFunc{handler.GetQRCode}},
	}
}
