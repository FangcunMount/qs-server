package collection

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/container"
	"github.com/yshujie/questionnaire-scale/pkg/log"
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
	// 注册公开路由
	r.registerPublicRoutes(engine)

	// 注册API路由
	r.registerAPIRoutes(engine)

	log.Info("🔗 Registered routes for: public, questionnaire, answersheet")
}

// registerPublicRoutes 注册公开路由
func (r *Router) registerPublicRoutes(engine *gin.Engine) {
	// 健康检查和基础路由
	engine.GET("/health", r.healthCheck)
	engine.GET("/ping", r.ping)

	// 公开的API路由
	publicAPI := engine.Group("/api/v1/public")
	{
		publicAPI.GET("/info", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"service":     "collection-server",
				"version":     "1.0.0",
				"description": "问卷收集服务",
			})
		})
	}
}

// registerAPIRoutes 注册API路由
func (r *Router) registerAPIRoutes(engine *gin.Engine) {
	// 创建API组
	apiV1 := engine.Group("/api/v1")

	// 注册问卷相关路由
	r.registerQuestionnaireRoutes(apiV1)

	// 注册答卷相关路由
	r.registerAnswersheetRoutes(apiV1)
}

// registerQuestionnaireRoutes 注册问卷相关路由
func (r *Router) registerQuestionnaireRoutes(apiV1 *gin.RouterGroup) {
	if r.container.QuestionnaireHandler == nil {
		return
	}

	questionnaires := apiV1.Group("/questionnaires")
	{
		questionnaires.GET("", r.container.QuestionnaireHandler.List)             // 获取问卷列表
		questionnaires.GET("/:code", r.container.QuestionnaireHandler.Get)        // 获取问卷详情
		questionnaires.GET("/:code/raw", r.container.QuestionnaireHandler.GetRaw) // 获取原始问卷
	}
}

// registerAnswersheetRoutes 注册答卷相关路由
func (r *Router) registerAnswersheetRoutes(apiV1 *gin.RouterGroup) {
	if r.container.AnswersheetHandler == nil {
		return
	}

	answersheets := apiV1.Group("/answersheets")
	{
		answersheets.POST("", r.container.AnswersheetHandler.Submit) // 提交答卷
		answersheets.GET("/:id", r.container.AnswersheetHandler.Get) // 获取答卷详情
		answersheets.GET("", r.container.AnswersheetHandler.List)    // 获取答卷列表
	}
}

// healthCheck 健康检查处理函数
func (r *Router) healthCheck(c *gin.Context) {
	response := gin.H{
		"status":       "healthy",
		"version":      "1.0.0",
		"service":      "collection-server",
		"architecture": "clean",
		"components": gin.H{
			"grpc_clients": "questionnaire, answersheet",
			"validation":   "enabled",
			"handlers":     "questionnaire, answersheet",
		},
	}

	c.JSON(200, response)
}

// ping 简单的连通性测试
func (r *Router) ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
		"status":  "ok",
		"service": "collection-server",
	})
}
