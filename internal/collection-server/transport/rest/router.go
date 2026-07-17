package rest

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	auth "github.com/FangcunMount/iam/v2/pkg/sdk/auth/verifier"
	"github.com/FangcunMount/qs-server/internal/collection-server/concurrency"
	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	collectionmiddleware "github.com/FangcunMount/qs-server/internal/collection-server/transport/rest/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/httpauth"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/ratelimit"
	"github.com/gin-gonic/gin"
)

// Router 集中的路由管理器
type Router struct {
	container *container.Container
}

// NewRouter 创建路由管理器
func NewRouter(c *container.Container) *Router {
	return &Router{
		container: c,
	}
}

// RegisterRoutes 注册所有路由
func (r *Router) RegisterRoutes(engine *gin.Engine) {
	// 设置全局中间件
	r.setupGlobalMiddleware(engine)

	// OpenAPI 契约（OAS 3.1）与 UI
	engine.Static("/api/rest", "./api/rest")
	engine.Static("/swagger-ui", "./web/swagger-ui/swagger-ui-dist")
	engine.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/swagger-ui/")
	})

	// 注册公开路由
	r.registerPublicRoutes(engine)

	// 注册业务路由
	r.registerBusinessRoutes(engine)
}

// setupGlobalMiddleware 设置全局中间件
func (r *Router) setupGlobalMiddleware(engine *gin.Engine) {
	// Recovery 中间件
	engine.Use(gin.Recovery())

	// RequestID 中间件
	engine.Use(pkgmiddleware.RequestID())

	// 基础日志中间件
	engine.Use(pkgmiddleware.Logger())

	// API详细日志中间件
	engine.Use(pkgmiddleware.APILogger())

	// 其他中间件
	engine.Use(pkgmiddleware.NoCache)
	engine.Use(pkgmiddleware.Options)
}

// registerPublicRoutes 注册公开路由
func (r *Router) registerPublicRoutes(engine *gin.Engine) {
	healthHandler := r.container.HealthHandler()

	// 健康检查路由
	engine.GET("/health", healthHandler.Health)
	engine.GET("/readyz", healthHandler.Ready)
	engine.GET("/governance/redis", healthHandler.RedisFamilies)
	engine.GET("/governance/resilience", healthHandler.Resilience)
	engine.GET("/ping", healthHandler.Ping)

	// 公开的API路由
	publicAPI := engine.Group("/api/v1/public")
	{
		publicAPI.GET("/info", healthHandler.Info)
	}
}

// registerBusinessRoutes 注册业务路由
func (r *Router) registerBusinessRoutes(engine *gin.Engine) {
	api := engine.Group("/api/v1")

	// 应用 IAM JWT 认证中间件（如果启用，使用 SDK TokenVerifier 本地验签）
	r.applyIAMAuth(api, isPublicCatalogReadOnly)

	// 问卷相关路由
	r.registerQuestionnaireRoutes(api)

	// 答卷相关路由
	r.registerAnswerSheetRoutes(api)

	// 测评相关路由
	r.registerEvaluationRoutes(api)

	r.registerAssessmentModelCatalogRoutes(api)

	// 类型学模型相关路由
	r.registerTypologyModelRoutes(api)

	// 类型学测评相关路由
	r.registerTypologyAssessmentRoutes(api)
	r.registerTypologyAssessmentSessionRoutes(api)

	// 行为能力测评相关路由
	r.registerBehaviorAssessmentRoutes(api)

	// 受试者相关路由
	r.registerTesteeRoutes(api)

	// WebSocket 报告事件推送
	r.registerReportEventsRoutes(api)
}

func (r *Router) applyIAMAuth(api *gin.RouterGroup, skip func(*gin.Context) bool) {
	if r.container.IAMModule == nil || !r.container.IAMModule.IsEnabled() {
		fmt.Printf("⚠️  Warning: IAM authentication is disabled, routes are unprotected!\n")
		return
	}

	tokenVerifier := r.container.IAMModule.SDKTokenVerifier()
	if tokenVerifier == nil {
		fmt.Printf("⚠️  Warning: TokenVerifier not available, JWT authentication disabled!\n")
		return
	}

	api.Use(withAuthSkip(skip, pkgmiddleware.JWTAuthMiddlewareWithOptions(tokenVerifier, r.iamVerifyOptions())))
	// collection 的 org 由 testee/业务层决定，不在 HTTP 入口解析 OrgScope；仅校验 IAM 身份与授权域。
	api.Use(withAuthSkip(skip, collectionmiddleware.UserIdentityMiddleware()))
	api.Use(withAuthSkip(skip, httpauth.RequireTenantDomainMiddleware()))
	if loader := r.container.IAMModule.AuthzSnapshotLoader(); loader != nil {
		// 授权快照只负责权限视图，不替代 JWT 的权威在线校验。
		api.Use(withAuthSkip(skip, httpauth.AuthzSnapshotMiddleware(loader)))
	} else {
		fmt.Printf("⚠️  Warning: IAM AuthzSnapshotLoader unavailable for collection-server (need gRPC)\n")
	}
	fmt.Printf("🔐 JWT + IAM authz snapshot middleware enabled for /api/v1 (%s)\n", r.iamVerificationMode())
}

func (r *Router) iamVerifyOptions() *auth.VerifyOptions {
	if r == nil || r.container == nil || r.container.IAMModule == nil || r.container.IAMModule.Client() == nil {
		return &auth.VerifyOptions{IncludeMetadata: true}
	}
	cfg := r.container.IAMModule.Client().Config()
	if cfg == nil || cfg.JWT == nil {
		return &auth.VerifyOptions{IncludeMetadata: true}
	}
	return &auth.VerifyOptions{
		ForceRemote:     cfg.JWT.ForceRemoteVerification,
		IncludeMetadata: true,
	}
}

func (r *Router) iamVerificationMode() string {
	opts := r.iamVerifyOptions()
	if opts.ForceRemote {
		return "authoritative remote verification"
	}
	return "local JWKS verification"
}

// withAuthSkip
func withAuthSkip(skip func(*gin.Context) bool, next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if skip != nil && skip(c) {
			c.Next()
			return
		}
		next(c)
	}
}

// isPublicCatalogReadOnly returns the unauthenticated catalogue read routes.
func isPublicCatalogReadOnly(c *gin.Context) bool {
	if c.Request.Method != http.MethodGet {
		return false
	}

	// path 白名单
	whitelist := []string{
		"/api/v1/assessment-models",
		"/api/v1/assessment-models/hot",
		"/api/v1/assessment-models/options",
		"/api/v1/typology-models",
		"/api/v1/typology-models/categories",
	}

	return slices.Contains(whitelist, strings.TrimRight(c.Request.URL.Path, "/"))
}

func (r *Router) registerAssessmentModelCatalogRoutes(api *gin.RouterGroup) {
	modelHandler := r.container.AssessmentModelCatalogHandler()
	models := api.Group("/assessment-models")
	{
		models.GET("/hot", r.catalogHandlers(modelHandler.ListHot)...)
		models.GET("/options", r.catalogHandlers(modelHandler.Options)...)
		models.GET("", r.catalogHandlers(modelHandler.List)...)
		models.GET("/:code", r.catalogHandlers(modelHandler.Get)...)
	}
}

func requestLimitKey(c *gin.Context) string {
	userID := pkgmiddleware.GetUserID(c)
	if userID != "" {
		return "user:" + userID
	}
	return "ip:" + c.ClientIP()
}

func rateLimitedHandlers(
	provider ratelimit.RateBudgetProvider,
	_ ratelimit.Backend,
	scope string,
	rateCfg *options.RateLimitOptions,
	_ float64,
	_ int,
	_ float64,
	_ int,
	handler gin.HandlerFunc,
) []gin.HandlerFunc {
	if rateCfg == nil || !rateCfg.Enabled {
		return []gin.HandlerFunc{handler}
	}
	budgetID := ratelimit.BudgetID(strings.ReplaceAll(scope, "-", "_"))
	if provider != nil {
		if budget, ok := provider.Budget(budgetID); ok {
			return []gin.HandlerFunc{
				distributedLimit(budget.Global, "limit:"+scope+":global", nil),
				distributedLimit(budget.User, "limit:"+scope+":user", requestLimitKey),
				handler,
			}
		}
	}
	return []gin.HandlerFunc{func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"message": "rate limit budget unavailable"})
	}}
}

func waitConcurrencyHandlers(
	gate *concurrency.Gate,
	waitCfg *options.WaitReportOptions,
	handlers ...gin.HandlerFunc,
) []gin.HandlerFunc {
	if gate == nil {
		return handlers
	}
	if waitCfg == nil {
		waitCfg = options.NewWaitReportOptions()
	}
	retryAfter := waitCfg.DegradeRetryAfterSeconds
	if waitCfg.DegradeImmediateEnabled {
		mw := gate.TryMiddleware(func(c *gin.Context) {
			WriteDegradedWaitReport(c, retryAfter)
		})
		return append([]gin.HandlerFunc{mw}, handlers...)
	}
	return append([]gin.HandlerFunc{gate.BlockingMiddleware()}, handlers...)
}

func distributedLimit(
	limiter ratelimit.RateLimiter,
	scope string,
	keyFn func(*gin.Context) string,
) gin.HandlerFunc {
	return distributedLimitWithOptions(limiter, scope, keyFn, pkgmiddleware.LimitOptions{})
}

func distributedLimitWithOptions(
	limiter ratelimit.RateLimiter,
	scope string,
	keyFn func(*gin.Context) string,
	opts pkgmiddleware.LimitOptions,
) gin.HandlerFunc {
	if limiter == nil {
		return pkgmiddleware.LimitDegradedOpen(opts)
	}
	return pkgmiddleware.LimitWithLimiter(limiter, func(c *gin.Context) string {
		key := scope
		if keyFn != nil {
			suffix := keyFn(c)
			if suffix != "" {
				key += ":" + suffix
			}
		}
		return key
	}, opts)
}

func ensureRateLimitOptions(rateCfg *options.RateLimitOptions) *options.RateLimitOptions {
	if rateCfg == nil {
		return options.NewRateLimitOptions()
	}
	return rateCfg
}

// registerQuestionnaireRoutes 注册问卷相关路由
func (r *Router) registerQuestionnaireRoutes(api *gin.RouterGroup) {
	questionnaireHandler := r.container.QuestionnaireHandler()

	questionnaires := api.Group("/questionnaires")
	{
		questionnaires.GET("", r.catalogHandlers(questionnaireHandler.List)...)
		questionnaires.GET("/:code", r.catalogHandlers(questionnaireHandler.Get)...)
	}
}

// registerAnswerSheetRoutes 注册答卷相关路由
func (r *Router) registerAnswerSheetRoutes(api *gin.RouterGroup) {
	answerSheetHandler := r.container.AnswerSheetHandler()
	rateCfg := ensureRateLimitOptions(r.container.RateLimitOptions())

	answersheets := api.Group("/answersheets")
	{
		answersheets.POST("", r.rateLimitedSubmitHandlers(
			r.container.RateLimitBackend(),
			"submit",
			rateCfg,
			rateCfg.SubmitGlobalQPS,
			rateCfg.SubmitGlobalBurst,
			rateCfg.SubmitUserQPS,
			rateCfg.SubmitUserBurst,
			answerSheetHandler.Submit,
		)...)
		answersheets.GET("/submit-status", r.rateLimitedQueryHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			answerSheetHandler.SubmitStatus,
		)...)
		answersheets.GET("/:id", r.rateLimitedQueryHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			answerSheetHandler.Get,
		)...)
	}
}

// registerEvaluationRoutes 注册测评相关路由
func (r *Router) registerEvaluationRoutes(api *gin.RouterGroup) {
	evaluationHandler := r.container.EvaluationHandler()
	rateCfg := ensureRateLimitOptions(r.container.RateLimitOptions())
	var profileLinks *iam.ProfileLinkService
	if r.container.IAMModule != nil {
		profileLinks = r.container.IAMModule.ProfileLinkService()
	}
	reportIdentity := collectionmiddleware.TesteeProfileLinkMiddleware(r.container.TesteeService(), profileLinks, "testee_id")

	assessments := api.Group("/assessments")
	{
		// 医学量表测评列表（放在 :id 前面避免路由冲突）
		assessments.GET("", r.rateLimitedQueryHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			evaluationHandler.ListAssessments,
		)...)
		// 因子趋势（放在 :id 前面避免路由冲突）
		assessments.GET("/trend", r.rateLimitedQueryHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			evaluationHandler.GetFactorTrend,
		)...)
		// 高风险因子
		assessments.GET("/:id/factors/high-risk", r.rateLimitedQueryHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			evaluationHandler.GetHighRiskFactors,
		)...)
		// 测评得分
		assessments.GET("/:id/scores", r.rateLimitedQueryHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			evaluationHandler.GetAssessmentScores,
		)...)
		// 医学量表报告（总分、因子解读和建议）
		assessments.GET("/:id/report", append([]gin.HandlerFunc{reportIdentity}, r.rateLimitedQueryHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			evaluationHandler.GetAssessmentReport,
		)...)...)
		// 测评趋势摘要
		assessments.GET("/:id/trend-summary", r.rateLimitedQueryHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			evaluationHandler.GetAssessmentTrendSummary,
		)...)
		// 长轮询等待报告生成
		assessments.GET("/:id/report-status", append([]gin.HandlerFunc{reportIdentity}, r.rateLimitedReportStatusHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			evaluationHandler.GetReportStatus,
		)...)...)
		assessments.GET("/:id/wait-report", append([]gin.HandlerFunc{reportIdentity}, r.waitReportHandlers(
			rateLimitedHandlers(
				r.container.RateBudgetProvider(),
				r.container.RateLimitBackend(),
				"wait-report",
				rateCfg,
				rateCfg.WaitReportGlobalQPS,
				rateCfg.WaitReportGlobalBurst,
				rateCfg.WaitReportUserQPS,
				rateCfg.WaitReportUserBurst,
				evaluationHandler.WaitReport,
			)...,
		)...)...)
	}
}

// registerTypologyModelRoutes 注册类型学模型相关路由
func (r *Router) registerTypologyModelRoutes(api *gin.RouterGroup) {
	handler := r.container.TypologyModelHandler()
	rateCfg := ensureRateLimitOptions(r.container.RateLimitOptions())

	models := api.Group("/typology-models")
	{
		models.GET("/categories", r.rateLimitedCatalogHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			handler.GetCategories,
		)...)
		models.GET("", r.rateLimitedCatalogHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			handler.List,
		)...)
		models.GET("/:code", r.rateLimitedCatalogHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			handler.Get,
		)...)
	}
}

// registerTypologyAssessmentRoutes 注册类型学测评相关路由
func (r *Router) registerTypologyAssessmentRoutes(api *gin.RouterGroup) {
	handler := r.container.TypologyAssessmentHandler()
	rateCfg := ensureRateLimitOptions(r.container.RateLimitOptions())
	var profileLinks *iam.ProfileLinkService
	if r.container.IAMModule != nil {
		profileLinks = r.container.IAMModule.ProfileLinkService()
	}
	reportIdentity := collectionmiddleware.TesteeProfileLinkMiddleware(r.container.TesteeService(), profileLinks, "testee_id")

	assessments := api.Group("/typology-assessments")
	{
		assessments.GET("", r.rateLimitedQueryHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			handler.List,
		)...)
		assessments.GET("/:id/report-status", append([]gin.HandlerFunc{reportIdentity}, r.rateLimitedReportStatusHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			handler.GetReportStatus,
		)...)...)
		assessments.GET("/:id/wait-report", append([]gin.HandlerFunc{reportIdentity}, r.waitReportHandlers(
			rateLimitedHandlers(
				r.container.RateBudgetProvider(),
				r.container.RateLimitBackend(),
				"wait-report",
				rateCfg,
				rateCfg.WaitReportGlobalQPS,
				rateCfg.WaitReportGlobalBurst,
				rateCfg.WaitReportUserQPS,
				rateCfg.WaitReportUserBurst,
				handler.WaitReport,
			)...,
		)...)...)
		assessments.GET("/:id/report", append([]gin.HandlerFunc{reportIdentity}, r.rateLimitedQueryHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			handler.GetReport,
		)...)...)
		assessments.GET("/:id", r.rateLimitedQueryHandlers(
			r.container.RateLimitBackend(),
			"query",
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			handler.Get,
		)...)
	}
}

// registerBehaviorAssessmentRoutes registers the product-level behavior ability facade.
func (r *Router) registerBehaviorAssessmentRoutes(api *gin.RouterGroup) {
	handler := r.container.BehaviorAssessmentHandler()
	rateCfg := ensureRateLimitOptions(r.container.RateLimitOptions())
	var profileLinks *iam.ProfileLinkService
	if r.container.IAMModule != nil {
		profileLinks = r.container.IAMModule.ProfileLinkService()
	}
	reportIdentity := collectionmiddleware.TesteeProfileLinkMiddleware(r.container.TesteeService(), profileLinks, "testee_id")

	assessments := api.Group("/behavior-assessments")
	{
		assessments.GET("", r.rateLimitedQueryHandlers(r.container.RateLimitBackend(), "query", rateCfg, rateCfg.QueryGlobalQPS, rateCfg.QueryGlobalBurst, rateCfg.QueryUserQPS, rateCfg.QueryUserBurst, handler.List)...)
		assessments.GET("/:id/report-status", append([]gin.HandlerFunc{reportIdentity}, r.rateLimitedReportStatusHandlers(r.container.RateLimitBackend(), "query", rateCfg, rateCfg.QueryGlobalQPS, rateCfg.QueryGlobalBurst, rateCfg.QueryUserQPS, rateCfg.QueryUserBurst, handler.GetReportStatus)...)...)
		assessments.GET("/:id/wait-report", append([]gin.HandlerFunc{reportIdentity}, r.waitReportHandlers(rateLimitedHandlers(r.container.RateBudgetProvider(), r.container.RateLimitBackend(), "wait-report", rateCfg, rateCfg.WaitReportGlobalQPS, rateCfg.WaitReportGlobalBurst, rateCfg.WaitReportUserQPS, rateCfg.WaitReportUserBurst, handler.WaitReport)...)...)...)
		assessments.GET("/:id/report", append([]gin.HandlerFunc{reportIdentity}, r.rateLimitedQueryHandlers(r.container.RateLimitBackend(), "query", rateCfg, rateCfg.QueryGlobalQPS, rateCfg.QueryGlobalBurst, rateCfg.QueryUserQPS, rateCfg.QueryUserBurst, handler.GetReport)...)...)
		assessments.GET("/:id", r.rateLimitedQueryHandlers(r.container.RateLimitBackend(), "query", rateCfg, rateCfg.QueryGlobalQPS, rateCfg.QueryGlobalBurst, rateCfg.QueryUserQPS, rateCfg.QueryUserBurst, handler.Get)...)
	}
}

func (r *Router) registerTypologyAssessmentSessionRoutes(api *gin.RouterGroup) {
	handler := r.container.TypologyAssessmentSessionHandler()
	rateCfg := ensureRateLimitOptions(r.container.RateLimitOptions())

	api.POST("/typology-assessment-sessions", r.rateLimitedSubmitHandlers(
		r.container.RateLimitBackend(),
		"query",
		rateCfg,
		rateCfg.QueryGlobalQPS,
		rateCfg.QueryGlobalBurst,
		rateCfg.QueryUserQPS,
		rateCfg.QueryUserBurst,
		handler.Start,
	)...)
}

// registerTesteeRoutes 注册受试者相关路由
func (r *Router) registerTesteeRoutes(api *gin.RouterGroup) {
	testeeHandler := r.container.TesteeHandler()

	testees := api.Group("/testees")
	{
		// 检查受试者是否存在（放在 :id 前面避免路由冲突）
		testees.GET("/exists", r.queryHandlers(testeeHandler.Exists)...)
		// 创建受试者
		testees.POST("", r.submitHandlers(testeeHandler.Create)...)
		// 查询受试者列表
		testees.GET("", r.queryHandlers(testeeHandler.List)...)
		// 获取受试者详情
		testees.GET("/:id", r.queryHandlers(testeeHandler.Get)...)
		// 获取受试者照护上下文
		testees.GET("/:id/care-context", r.queryHandlers(testeeHandler.GetCareContext)...)
		// 更新受试者信息
		testees.PUT("/:id", r.submitHandlers(testeeHandler.Update)...)
	}
}

func (r *Router) registerReportEventsRoutes(api *gin.RouterGroup) {
	handler := r.container.ReportEventsHandler()
	if handler == nil || !handler.Enabled() {
		return
	}
	path := strings.TrimPrefix(handler.Path(), "/api/v1")
	if path == "" || path == handler.Path() {
		path = "/report-events"
	}
	api.GET(path, handler.ServeHTTP)
}
