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
	codesapp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	interpretationclinician "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/clinician"
	interpretationoperations "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/operations"
	reportqueryjourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportquery"
	reportwaitjourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportwait"
	assessmentModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	answerSheetApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	questionnaireApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	systemgovApp "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	workbenchApp "github.com/FangcunMount/qs-server/internal/apiserver/application/workbench"
	cachemodel "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/ratelimit"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/gin-gonic/gin"
)

// Router 集中的路由管理器。
type Router struct {
	deps             Deps
	rateCfg          *options.RateLimitOptions
	rateLimitBudgets map[rateLimitBudget]routeLimiters
}

type rateLimitBudget = ratelimit.BudgetID

const (
	rateLimitBudgetQuery       rateLimitBudget = "query"
	rateLimitBudgetSubmit      rateLimitBudget = "submit"
	rateLimitBudgetAdminSubmit rateLimitBudget = "admin_submit"
	rateLimitBudgetWaitReport  rateLimitBudget = "wait_report"
)

type routeLimiters struct {
	global ratelimit.RateLimiter
	user   ratelimit.RateLimiter
}

type routeSpec struct {
	method   string
	path     string
	handlers []gin.HandlerFunc
}

type Deps struct {
	RateLimit   *options.RateLimitOptions
	RateBudgets ratelimit.RateBudgetProvider

	Survey          SurveyDeps
	AssessmentModel AssessmentModelDeps
	Actor           ActorDeps
	Evaluation      EvaluationDeps
	Interpretation  InterpretationDeps
	Plan            PlanDeps
	Statistics      StatisticsDeps
	Workbench       WorkbenchDeps

	CodesService             codesapp.CodesService
	QRCodeObjectStore        objectstorageport.PublicObjectStore
	QRCodeObjectKeyPrefix    string
	AssessmentAssetStore     objectstorageport.ObjectStore
	AssessmentAssetKeyPrefix string
	GovernanceStatusService  statisticsApp.GovernanceStatusReader
	EventStatusService       appEventing.StatusService
	SystemGovernanceFacade   systemgovApp.Facade
	Backpressure             []resilienceplane.BackpressureSnapshot
	Locks                    []resilienceplane.CapabilitySnapshot
	ResilienceSnapshot       func() resilienceplane.RuntimeSnapshot
	IAM                      IAMDeps
}

type SurveyDeps struct {
	QuestionnaireLifecycleService questionnaireApp.QuestionnaireLifecycleService
	QuestionnaireContentService   questionnaireApp.QuestionnaireContentService
	QuestionnaireQueryService     questionnaireApp.QuestionnaireQueryService
	QuestionnaireQRCodeService    questionnaireApp.QuestionnaireQRCodeQueryService
	AnswerSheetManagementService  answerSheetApp.AnswerSheetManagementService
	AnswerSheetSubmissionService  answerSheetApp.AnswerSheetSubmissionService
}

type AssessmentModelDeps struct {
	Management  assessmentModelApp.CatalogManagementService
	Definition  assessmentModelApp.DefinitionAuthoringService
	Publication assessmentModelApp.PublicationService
	Release     assessmentModelApp.AssessmentReleaseService
	Query       assessmentModelApp.CatalogQueryService
	NormTables  assessmentModelApp.NormTableService
	Assets      assessmentModelApp.OutcomeImageService
}

type ActorDeps struct {
	TesteeManagementService       testeeApp.TesteeManagementService
	TesteeQueryService            testeeApp.TesteeQueryService
	TesteeBackendQueryService     testeeApp.TesteeBackendQueryService
	TesteeAccessService           actorAccessApp.TesteeAccessService
	TesteeScaleAnalysisService    evaluationoperator.ScaleAnalysisService
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
	OperatorRecoveryService  evaluationoperator.RecoveryService
	OperatorExecutionService evaluationoperator.BatchExecutionService
	ProtectedQueryService    evaluationoperator.QueryService
}

type InterpretationDeps struct {
	ReportQueryJourney reportqueryjourney.Service
	ReportWaitJourney  reportwaitjourney.Service
	ClinicianService   interpretationclinician.Service
	OperationsService  interpretationoperations.Service
}

type PlanDeps struct {
	CommandService      planApp.PlanCommandService
	QueryService        planApp.PlanQueryService
	TesteeAccessService actorAccessApp.TesteeAccessService
}

type WorkbenchDeps struct {
	WorkbenchService workbenchApp.Service
}

type StatisticsDeps struct {
	Enabled bool

	ReadService                  statisticsApp.ReadService
	PeriodicStatsService         statisticsApp.PeriodicStatsService
	SyncService                  statisticsApp.StatisticsSyncService
	TesteeAccessService          statisticsApp.TesteeAccessValidator
	WarmupCoordinator            statisticsApp.WarmupCoordinator
	CacheGovernanceStatusService statisticsApp.GovernanceStatusReader
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

	budgets := make(map[rateLimitBudget]routeLimiters, 4)
	if deps.RateBudgets != nil {
		for _, id := range []rateLimitBudget{rateLimitBudgetQuery, rateLimitBudgetSubmit, rateLimitBudgetAdminSubmit, rateLimitBudgetWaitReport} {
			if budget, ok := deps.RateBudgets.Budget(id); ok {
				budgets[id] = routeLimiters{global: budget.Global, user: budget.User}
			}
		}
	}
	return &Router{
		deps:             deps,
		rateCfg:          rateCfg,
		rateLimitBudgets: budgets,
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

	fmt.Printf("🔗 Registered routes for: public, protected(api/v1, api/v2), internal(internal/v1)\n")
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
	budget rateLimitBudget,
	handler gin.HandlerFunc,
) []gin.HandlerFunc {
	if !r.rateCfg.Enabled {
		return []gin.HandlerFunc{handler}
	}
	limiters, ok := r.rateLimitBudgets[budget]
	if !ok {
		panic("unknown REST rate-limit budget: " + string(budget))
	}

	return []gin.HandlerFunc{
		middleware.LimitWithLimiter(limiters.global, nil, middleware.LimitOptions{
			Component: "apiserver",
			Scope:     "rest",
			Resource:  "global",
			Strategy:  "local",
		}),
		middleware.LimitWithLimiter(limiters.user, requestLimitKey, middleware.LimitOptions{
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
// @Summary 管理员接口（未实现）
// @Tags Admin
// @Produce json
// @Success 501 {object} map[string]interface{}
// @Router /api/v1/admin/users [get]
// @Router /api/v1/admin/statistics [get]
// @Router /api/v1/admin/logs [get]
func (r *Router) unsupportedFeature(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"code":    501,
		"message": "功能当前不支持",
		"path":    c.Request.URL.Path,
		"method":  c.Request.Method,
	})
}

// healthCheck 健康检查处理函数。
// @Summary 健康检查
// @Description 返回 apiserver 健康状态
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /health [get]
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
// @Summary 连通性测试
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /ping [get]
func (r *Router) ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
		"status":  "ok",
		"router":  "centralized",
		"auth":    "enabled",
	})
}

// readyCheck reports whether apiserver dependencies are ready.
// @Summary 就绪检查
// @Description 返回 apiserver 及其 Redis 依赖的就绪状态。
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /readyz [get]
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

// redisGovernance returns the Redis family governance snapshot.
// @Summary Redis 治理状态
// @Description 返回 apiserver Redis family 的运行状态。
// @Tags health
// @Produce json
// @Success 200 {object} cachemodel.RuntimeSnapshot
// @Router /governance/redis [get]
func (r *Router) redisGovernance(c *gin.Context) {
	c.JSON(http.StatusOK, r.runtimeSnapshot(c))
}

func (r *Router) runtimeSnapshot(c *gin.Context) cachemodel.RuntimeSnapshot {
	if r != nil && r.deps.GovernanceStatusService != nil {
		snapshot, err := r.deps.GovernanceStatusService.GetRuntime(c.Request.Context())
		if err == nil && snapshot != nil {
			return *snapshot
		}
	}
	return cachemodel.RuntimeSnapshot{
		GeneratedAt: time.Now(),
		Component:   "apiserver",
		Families:    []cachemodel.FamilyStatus{},
		Summary: cachemodel.RuntimeSummary{
			Ready: true,
		},
	}
}
