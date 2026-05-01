package rest

import (
	"fmt"
	"net/http"
	"time"

	auth "github.com/FangcunMount/iam/v2/pkg/sdk/auth/verifier"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	assessmentEntryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/assessmententry"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	operatorapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	codesapp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	answerSheetApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	questionnaireApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/gin-gonic/gin"
)

// Router 集中的路由管理器。
type Router struct {
	deps    Deps
	rateCfg *options.RateLimitOptions
}

type routeSpec struct {
	method   string
	path     string
	handlers []gin.HandlerFunc
}

type Deps struct {
	RateLimit *options.RateLimitOptions

	Survey     SurveyDeps
	Scale      ScaleDeps
	Actor      ActorDeps
	Evaluation EvaluationDeps
	Plan       PlanDeps
	Statistics StatisticsDeps

	CodesService            codesapp.CodesService
	QRCodeObjectStore       objectstorageport.PublicObjectStore
	QRCodeObjectKeyPrefix   string
	GovernanceStatusService cachegov.StatusService
	EventStatusService      appEventing.StatusService
	Backpressure            []resilienceplane.BackpressureSnapshot
	IAM                     IAMDeps
}

type SurveyDeps struct {
	QuestionnaireLifecycleService questionnaireApp.QuestionnaireLifecycleService
	QuestionnaireContentService   questionnaireApp.QuestionnaireContentService
	QuestionnaireQueryService     questionnaireApp.QuestionnaireQueryService
	QuestionnaireQRCodeService    questionnaireApp.QuestionnaireQRCodeQueryService
	AnswerSheetManagementService  answerSheetApp.AnswerSheetManagementService
	AnswerSheetSubmissionService  answerSheetApp.AnswerSheetSubmissionService
}

type ScaleDeps struct {
	LifecycleService scaleApp.ScaleLifecycleService
	FactorService    scaleApp.ScaleFactorService
	QueryService     scaleApp.ScaleQueryService
	CategoryService  scaleApp.ScaleCategoryService
	QRCodeService    scaleApp.ScaleQRCodeQueryService
}

type ActorDeps struct {
	TesteeManagementService       testeeApp.TesteeManagementService
	TesteeQueryService            testeeApp.TesteeQueryService
	TesteeBackendQueryService     testeeApp.TesteeBackendQueryService
	TesteeAccessService           actorAccessApp.TesteeAccessService
	TesteeScaleAnalysisService    testeeApp.ScaleAnalysisQueryService
	OperatorLifecycleService      operatorapp.OperatorLifecycleService
	OperatorAuthorizationService  operatorapp.OperatorAuthorizationService
	OperatorQueryService          operatorapp.OperatorQueryService
	ClinicianLifecycleService     clinicianApp.ClinicianLifecycleService
	ClinicianQueryService         clinicianApp.ClinicianQueryService
	ClinicianRelationshipService  clinicianApp.ClinicianRelationshipService
	AssessmentEntryService        assessmentEntryApp.AssessmentEntryService
	QRCodeService                 qrcodeApp.QRCodeService
	ActiveOperatorChecker         operatorapp.ActiveOperatorChecker
	OperatorRoleProjectionUpdater operatorapp.OperatorRoleProjectionUpdater
}

type EvaluationDeps struct {
	ManagementService   assessmentApp.AssessmentManagementService
	ReportQueryService  assessmentApp.ReportQueryService
	ScoreQueryService   assessmentApp.ScoreQueryService
	EvaluationService   engine.Service
	WaitService         assessmentApp.AssessmentWaitService
	TesteeAccessService actorAccessApp.TesteeAccessService
}

type PlanDeps struct {
	Handler *handler.PlanHandler
}

type StatisticsDeps struct {
	Handler *handler.StatisticsHandler
}

type IAMDeps struct {
	Enabled                 bool
	TokenVerifier           *auth.TokenVerifier
	ForceRemoteVerification bool
	SnapshotLoader          *iaminfra.AuthzSnapshotLoader
}

// NewRouter 创建路由管理器。
func NewRouter(deps Deps) *Router {
	rateCfg := deps.RateLimit
	if rateCfg == nil {
		rateCfg = options.NewRateLimitOptions()
	}

	return &Router{
		deps:    deps,
		rateCfg: rateCfg,
	}
}

// RegisterRoutes 注册所有路由。
func (r *Router) RegisterRoutes(engine *gin.Engine) {
	engine.Static("/api/rest", "./api/rest")
	engine.Static("/swagger-ui", "./web/swagger-ui/swagger-ui-dist")
	engine.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/swagger-ui/")
	})

	r.registerPublicRoutes(engine)
	r.registerProtectedRoutes(engine)
	r.registerInternalRoutes(engine)

	fmt.Printf("🔗 Registered routes for: public, protected(api/v1), internal(internal/v1)\n")
}

func registerRouteSpecs(group *gin.RouterGroup, routes []routeSpec) {
	for _, route := range routes {
		switch route.method {
		case http.MethodGet:
			group.GET(route.path, route.handlers...)
		case http.MethodPost:
			group.POST(route.path, route.handlers...)
		case http.MethodPut:
			group.PUT(route.path, route.handlers...)
		case http.MethodDelete:
			group.DELETE(route.path, route.handlers...)
		}
	}
}

func (r *Router) rateLimitedHandlers(
	rateCfg *options.RateLimitOptions,
	globalQPS float64,
	globalBurst int,
	userQPS float64,
	userBurst int,
	handler gin.HandlerFunc,
) []gin.HandlerFunc {
	if !rateCfg.Enabled {
		return []gin.HandlerFunc{handler}
	}

	return []gin.HandlerFunc{
		middleware.LimitWithOptions(globalQPS, globalBurst, middleware.LimitOptions{
			Component: "apiserver",
			Scope:     "rest",
			Resource:  "global",
			Strategy:  "local",
		}),
		middleware.LimitByKeyWithOptions(userQPS, userBurst, requestLimitKey, middleware.LimitOptions{
			Component: "apiserver",
			Scope:     "rest",
			Resource:  "user",
			Strategy:  "local_key",
		}),
		handler,
	}
}

func requestLimitKey(c *gin.Context) string {
	userID := middleware.GetUserID(c)
	if userID != "" {
		return "user:" + userID
	}
	return "ip:" + c.ClientIP()
}

// unsupportedFeature 明确标识当前保留但未支持的入口。
func (r *Router) unsupportedFeature(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"code":    501,
		"message": "功能当前不支持",
		"path":    c.Request.URL.Path,
		"method":  c.Request.Method,
	})
}

// healthCheck 健康检查处理函数。
func (r *Router) healthCheck(c *gin.Context) {
	response := gin.H{
		"status":       "healthy",
		"version":      "1.0.0",
		"discovery":    "auto",
		"architecture": "hexagonal",
		"router":       "centralized",
		"auth":         "delegated",
		"components": gin.H{
			"domain":      "questionnaire",
			"ports":       "storage",
			"adapters":    "mysql, mongodb, http",
			"application": "questionnaire_service",
		},
	}

	c.JSON(200, response)
}

// ping 简单的连通性测试。
func (r *Router) ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
		"status":  "ok",
		"router":  "centralized",
		"auth":    "enabled",
	})
}

func (r *Router) readyCheck(c *gin.Context) {
	snapshot := r.runtimeSnapshot(c)
	statusCode := http.StatusOK
	statusText := "ready"
	if !snapshot.Summary.Ready {
		statusCode = http.StatusServiceUnavailable
		statusText = "degraded"
	}
	c.JSON(statusCode, gin.H{
		"status":    statusText,
		"component": "apiserver",
		"redis":     snapshot,
	})
}

func (r *Router) redisGovernance(c *gin.Context) {
	c.JSON(http.StatusOK, r.runtimeSnapshot(c))
}

func (r *Router) runtimeSnapshot(c *gin.Context) observability.RuntimeSnapshot {
	if r != nil && r.deps.GovernanceStatusService != nil {
		snapshot, err := r.deps.GovernanceStatusService.GetRuntime(c.Request.Context())
		if err == nil && snapshot != nil {
			return *snapshot
		}
	}
	return observability.RuntimeSnapshot{
		GeneratedAt: time.Now(),
		Component:   "apiserver",
		Families:    []observability.FamilyStatus{},
		Summary: observability.RuntimeSummary{
			Ready: true,
		},
	}
}
