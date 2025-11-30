package collection

import (
	"github.com/FangcunMount/qs-server/internal/collection-server/container"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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

	// Swagger 文档
	engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

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
	// TODO: 添加认证中间件
	// 目前暂时不加认证，后续可以添加 JWT 或其他认证方式
	api := engine.Group("/api/v1")

	// 问卷相关路由
	r.registerQuestionnaireRoutes(api)

	// 答卷相关路由
	r.registerAnswerSheetRoutes(api)

	// 测评相关路由
	r.registerEvaluationRoutes(api)
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
