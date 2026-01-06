package collection

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	"github.com/FangcunMount/qs-server/internal/collection-server/interface/restful/middleware"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
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
	r.applyIAMAuth(api, isPublicScaleReadOnly)

	// 问卷相关路由
	r.registerQuestionnaireRoutes(api)

	// 答卷相关路由
	r.registerAnswerSheetRoutes(api)

	// 测评相关路由
	r.registerEvaluationRoutes(api)

	// 量表相关路由
	r.registerScaleRoutes(api)

	// 受试者相关路由
	r.registerTesteeRoutes(api)
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

	api.Use(withAuthSkip(skip, pkgmiddleware.JWTAuthMiddleware(tokenVerifier)))
	api.Use(withAuthSkip(skip, middleware.UserIdentityMiddleware()))
	fmt.Printf("🔐 JWT authentication middleware enabled for /api/v1 (local JWKS verification)\n")
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

// isPublicScaleReadOnly 是否开放接口
func isPublicScaleReadOnly(c *gin.Context) bool {
	if c.Request.Method != http.MethodGet {
		return false
	}

	// path 白名单
	whitelist := []string{
		"/api/v1/scales",
		"/api/v1/scales/categories",
	}

	return slices.Contains(whitelist, strings.TrimRight(c.Request.URL.Path, "/"))
}

func requestLimitKey(c *gin.Context) string {
	userID := pkgmiddleware.GetUserID(c)
	if userID != "" {
		return "user:" + userID
	}
	return "ip:" + c.ClientIP()
}

func rateLimitedHandlers(
	rateCfg *options.RateLimitOptions,
	globalQPS float64,
	globalBurst int,
	userQPS float64,
	userBurst int,
	handler gin.HandlerFunc,
) []gin.HandlerFunc {
	if rateCfg == nil || !rateCfg.Enabled {
		return []gin.HandlerFunc{handler}
	}

	return []gin.HandlerFunc{
		pkgmiddleware.Limit(globalQPS, globalBurst),
		pkgmiddleware.LimitByKey(userQPS, userBurst, requestLimitKey),
		handler,
	}
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
		questionnaires.GET("", questionnaireHandler.List)
		questionnaires.GET("/:code", questionnaireHandler.Get)
	}
}

// registerAnswerSheetRoutes 注册答卷相关路由
func (r *Router) registerAnswerSheetRoutes(api *gin.RouterGroup) {
	answerSheetHandler := r.container.AnswerSheetHandler()
	evaluationHandler := r.container.EvaluationHandler()
	rateCfg := ensureRateLimitOptions(r.container.RateLimitOptions())

	answersheets := api.Group("/answersheets")
	{
		answersheets.POST("", rateLimitedHandlers(
			rateCfg,
			rateCfg.SubmitGlobalQPS,
			rateCfg.SubmitGlobalBurst,
			rateCfg.SubmitUserQPS,
			rateCfg.SubmitUserBurst,
			answerSheetHandler.Submit,
		)...)
		answersheets.GET("/:id", rateLimitedHandlers(
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			answerSheetHandler.Get,
		)...)
		// 通过答卷ID获取测评详情
		answersheets.GET("/:id/assessment", rateLimitedHandlers(
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			evaluationHandler.GetMyAssessmentByAnswerSheetID,
		)...)
	}
}

// registerEvaluationRoutes 注册测评相关路由
func (r *Router) registerEvaluationRoutes(api *gin.RouterGroup) {
	evaluationHandler := r.container.EvaluationHandler()
	rateCfg := ensureRateLimitOptions(r.container.RateLimitOptions())

	assessments := api.Group("/assessments")
	{
		// 测评列表
		assessments.GET("", rateLimitedHandlers(
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			evaluationHandler.ListMyAssessments,
		)...)
		// 因子趋势（放在 :id 前面避免路由冲突）
		assessments.GET("/trend", rateLimitedHandlers(
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			evaluationHandler.GetFactorTrend,
		)...)
		// 高风险因子
		assessments.GET("/high-risk", rateLimitedHandlers(
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			evaluationHandler.GetHighRiskFactors,
		)...)
		// 测评详情
		assessments.GET("/:id", rateLimitedHandlers(
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			evaluationHandler.GetMyAssessment,
		)...)
		// 测评得分
		assessments.GET("/:id/scores", rateLimitedHandlers(
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			evaluationHandler.GetAssessmentScores,
		)...)
		// 测评报告
		assessments.GET("/:id/report", rateLimitedHandlers(
			rateCfg,
			rateCfg.QueryGlobalQPS,
			rateCfg.QueryGlobalBurst,
			rateCfg.QueryUserQPS,
			rateCfg.QueryUserBurst,
			evaluationHandler.GetAssessmentReport,
		)...)
		// 长轮询等待报告生成
		assessments.GET("/:id/wait-report", rateLimitedHandlers(
			rateCfg,
			rateCfg.WaitReportGlobalQPS,
			rateCfg.WaitReportGlobalBurst,
			rateCfg.WaitReportUserQPS,
			rateCfg.WaitReportUserBurst,
			evaluationHandler.WaitReport,
		)...)
	}
}

// registerScaleRoutes 注册量表相关路由
func (r *Router) registerScaleRoutes(api *gin.RouterGroup) {
	scaleHandler := r.container.ScaleHandler()

	scales := api.Group("/scales")
	{
		// 获取量表分类列表（放在 :code 前面避免路由冲突）
		scales.GET("/categories", scaleHandler.GetCategories)
		// 获取量表列表
		scales.GET("", scaleHandler.List)
		// 获取量表详情
		scales.GET("/:code", scaleHandler.Get)
	}
}

// registerTesteeRoutes 注册受试者相关路由
func (r *Router) registerTesteeRoutes(api *gin.RouterGroup) {
	testeeHandler := r.container.TesteeHandler()

	testees := api.Group("/testees")
	{
		// 检查受试者是否存在（放在 :id 前面避免路由冲突）
		testees.GET("/exists", testeeHandler.Exists)
		// 创建受试者
		testees.POST("", testeeHandler.Create)
		// 查询受试者列表
		testees.GET("", testeeHandler.List)
		// 获取受试者详情
		testees.GET("/:id", testeeHandler.Get)
		// 更新受试者信息
		testees.PUT("/:id", testeeHandler.Update)
	}
}
