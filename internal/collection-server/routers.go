package collection

import (
	"fmt"
	"net/http"

	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	"github.com/FangcunMount/qs-server/internal/collection-server/interface/restful/middleware"
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

	// CORS 中间件
	engine.Use(pkgmiddleware.Cors())

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
	if r.container.IAMModule != nil && r.container.IAMModule.IsEnabled() {
		tokenVerifier := r.container.IAMModule.SDKTokenVerifier()
		if tokenVerifier != nil {
			api.Use(pkgmiddleware.JWTAuthMiddleware(tokenVerifier))
			// 添加用户身份解析中间件：将 JWT claims 中的 UserID 转换为 uint64
			api.Use(middleware.UserIdentityMiddleware())
			fmt.Printf("🔐 JWT authentication middleware enabled for /api/v1 (local JWKS verification)\n")
		} else {
			fmt.Printf("⚠️  Warning: TokenVerifier not available, JWT authentication disabled!\n")
		}
	} else {
		fmt.Printf("⚠️  Warning: IAM authentication is disabled, routes are unprotected!\n")
	}

	// 问卷相关路由
	r.registerQuestionnaireRoutes(api)

	// 答卷相关路由
	r.registerAnswerSheetRoutes(api)

	// 测评相关路由
	r.registerEvaluationRoutes(api)

	// 受试者相关路由
	r.registerTesteeRoutes(api)
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

	answersheets := api.Group("/answersheets")
	{
		answersheets.POST("", answerSheetHandler.Submit)
		answersheets.GET("/:id", answerSheetHandler.Get)
	}
}

// registerEvaluationRoutes 注册测评相关路由
func (r *Router) registerEvaluationRoutes(api *gin.RouterGroup) {
	evaluationHandler := r.container.EvaluationHandler()

	assessments := api.Group("/assessments")
	{
		// 测评列表
		assessments.GET("", evaluationHandler.ListMyAssessments)
		// 因子趋势（放在 :id 前面避免路由冲突）
		assessments.GET("/trend", evaluationHandler.GetFactorTrend)
		// 高风险因子
		assessments.GET("/high-risk", evaluationHandler.GetHighRiskFactors)
		// 测评详情
		assessments.GET("/:id", evaluationHandler.GetMyAssessment)
		// 测评得分
		assessments.GET("/:id/scores", evaluationHandler.GetAssessmentScores)
		// 测评报告
		assessments.GET("/:id/report", evaluationHandler.GetAssessmentReport)
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
