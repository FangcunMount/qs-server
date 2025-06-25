package apiserver

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/api/http/handlers"
)

// Router 路由配置器
type Router struct {
	engine *gin.Engine

	// 处理器映射表
	handlers map[string]interface{}
}

// NewRouter 创建新的路由配置器
func NewRouter() *Router {
	return &Router{
		engine:   gin.New(),
		handlers: make(map[string]interface{}),
	}
}

// RegisterQuestionnaireRoutes 注册问卷处理器路由
func (r *Router) RegisterQuestionnaireRoutes(handler interface{}) error {
	questionnaireHandler, ok := handler.(*handlers.QuestionnaireHandler)
	if !ok {
		return fmt.Errorf("invalid questionnaire handler type")
	}

	r.handlers["questionnaire"] = questionnaireHandler

	// 初始化路由（如果尚未初始化）
	if r.engine.Routes() == nil || len(r.engine.Routes()) == 0 {
		r.installMiddleware()
		r.registerHealthRoutes()
	}

	// 注册问卷路由
	r.registerQuestionnaireHandlerRoutes(questionnaireHandler)

	return nil
}

// RegisterUserRoutes 注册用户处理器路由
func (r *Router) RegisterUserRoutes(handler interface{}) error {
	// TODO: 当用户处理器实现时，可以在这里添加逻辑
	r.handlers["user"] = handler
	return nil
}

// RegisterGenericRoutes 注册通用处理器路由
func (r *Router) RegisterGenericRoutes(name string, handler interface{}) error {
	r.handlers[name] = handler
	// TODO: 可以根据name和handler类型动态注册路由
	return fmt.Errorf("generic route registration not implemented for %s", name)
}

// installMiddleware 安装中间件
func (r *Router) installMiddleware() {
	r.engine.Use(gin.Logger())
	r.engine.Use(gin.Recovery())

	// TODO: 可以在这里添加更多中间件
	// r.engine.Use(cors.Default())
	// r.engine.Use(ratelimit.RateLimiter(...))
}

// registerHealthRoutes 注册健康检查路由
func (r *Router) registerHealthRoutes() {
	r.engine.GET("/health", r.healthCheck)
	r.engine.GET("/ping", r.ping)
}

// registerQuestionnaireHandlerRoutes 注册问卷处理器的具体路由
func (r *Router) registerQuestionnaireHandlerRoutes(handler *handlers.QuestionnaireHandler) {
	// API 版本组
	v1 := r.engine.Group("/api/v1")

	// 问卷路由组
	questionnaires := v1.Group("/questionnaires")
	{
		questionnaires.POST("", handler.CreateQuestionnaire)
		questionnaires.GET("", handler.GetQuestionnaire)
		questionnaires.GET("/list", handler.ListQuestionnaires)
		questionnaires.PUT("/:id", handler.UpdateQuestionnaire)
		questionnaires.POST("/:id/publish", handler.PublishQuestionnaire)
		questionnaires.DELETE("/:id", handler.DeleteQuestionnaire)
	}
}

// GetEngine 获取 Gin 引擎
func (r *Router) GetEngine() *gin.Engine {
	return r.engine
}

// GetHandlers 获取所有已注册的处理器
func (r *Router) GetHandlers() map[string]interface{} {
	return r.handlers
}

// healthCheck 健康检查处理函数
func (r *Router) healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":       "ok",
		"architecture": "hexagonal",
		"version":      "v1.0.0",
		"components": gin.H{
			"domain":      "questionnaire, user",
			"ports":       "storage",
			"adapters":    "mysql, mongodb, http",
			"application": "questionnaire_service, user_service",
		},
		"registered_handlers": r.getRegisteredHandlerNames(),
	})
}

// getRegisteredHandlerNames 获取已注册的处理器名称列表
func (r *Router) getRegisteredHandlerNames() []string {
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	return names
}

// ping 简单的连通性测试
func (r *Router) ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
}
