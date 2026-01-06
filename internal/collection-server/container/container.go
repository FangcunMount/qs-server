package container

import (
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/testee"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/iam"
	"github.com/FangcunMount/qs-server/internal/collection-server/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	redis "github.com/redis/go-redis/v9"
)

// Container 主容器，负责管理所有组件
type Container struct {
	initialized bool
	opts        *options.Options
	redisCache  redis.UniversalClient
	redisStore  redis.UniversalClient

	// IAM 模块
	IAMModule *IAMModule

	// gRPC 客户端（由 GRPCClientRegistry 注入）
	answerSheetClient   *grpcclient.AnswerSheetClient
	questionnaireClient *grpcclient.QuestionnaireClient
	evaluationClient    *grpcclient.EvaluationClient
	actorClient         *grpcclient.ActorClient
	scaleClient         *grpcclient.ScaleClient

	// 应用层服务
	submissionService         *answersheet.SubmissionService
	questionnaireQueryService *questionnaire.QueryService
	evaluationQueryService    *evaluation.QueryService
	scaleQueryService         *scale.QueryService
	testeeService             *testee.Service

	// 接口层处理器
	answerSheetHandler   *handler.AnswerSheetHandler
	questionnaireHandler *handler.QuestionnaireHandler
	evaluationHandler    *handler.EvaluationHandler
	scaleHandler         *handler.ScaleHandler
	testeeHandler        *handler.TesteeHandler
	healthHandler        *handler.HealthHandler
}

// NewContainer 创建新的容器
func NewContainer(opts *options.Options, redisCache redis.UniversalClient, redisStore redis.UniversalClient) *Container {
	return &Container{
		opts:        opts,
		redisCache:  redisCache,
		redisStore:  redisStore,
		initialized: false,
	}
}

// Initialize 初始化容器中的所有组件
func (c *Container) Initialize() error {
	if c.initialized {
		return nil
	}

	log.Info("🔧 Initializing Collection Server Container...")

	// 1. 初始化应用层
	c.initApplicationServices()

	// 2. 初始化接口层
	c.initHandlers()

	c.initialized = true
	log.Info("✅ Collection Server Container initialized successfully")

	return nil
}

// initApplicationServices 初始化应用层服务
func (c *Container) initApplicationServices() {
	log.Info("🎯 Initializing application services...")

	// 获取 GuardianshipService（如果 IAM 启用）
	var guardianshipService *iam.GuardianshipService
	if c.IAMModule != nil && c.IAMModule.IsEnabled() {
		guardianshipService = c.IAMModule.GuardianshipService()
	}

	c.submissionService = answersheet.NewSubmissionService(c.answerSheetClient, c.actorClient, guardianshipService)
	c.questionnaireQueryService = questionnaire.NewQueryService(c.questionnaireClient)
	c.evaluationQueryService = evaluation.NewQueryService(c.evaluationClient, c.scaleClient)
	c.scaleQueryService = scale.NewQueryService(c.scaleClient)
	c.testeeService = testee.NewService(c.actorClient, guardianshipService)

	log.Info("✅ Application services initialized")
}

// initHandlers 初始化接口层处理器
func (c *Container) initHandlers() {
	log.Info("🌐 Initializing REST handlers...")

	// 获取 GuardianshipService（如果 IAM 启用）
	var guardianshipService *iam.GuardianshipService
	if c.IAMModule != nil && c.IAMModule.IsEnabled() {
		guardianshipService = c.IAMModule.GuardianshipService()
	}

	c.answerSheetHandler = handler.NewAnswerSheetHandler(c.submissionService)
	c.questionnaireHandler = handler.NewQuestionnaireHandler(c.questionnaireQueryService)
	c.evaluationHandler = handler.NewEvaluationHandler(c.evaluationQueryService)
	c.scaleHandler = handler.NewScaleHandler(c.scaleQueryService)
	c.testeeHandler = handler.NewTesteeHandler(c.testeeService, guardianshipService)
	c.healthHandler = handler.NewHealthHandler("collection-server", "2.0.0")

	log.Info("✅ REST handlers initialized")
}

// Cleanup 清理资源
func (c *Container) Cleanup() {
	log.Info("🧹 Cleaning up container resources...")

	c.initialized = false
	log.Info("🏁 Container cleanup completed")
}

// IsInitialized 检查容器是否已初始化
func (c *Container) IsInitialized() bool {
	return c.initialized
}

// ==================== Getters ====================

// AnswerSheetHandler 获取答卷处理器
func (c *Container) AnswerSheetHandler() *handler.AnswerSheetHandler {
	return c.answerSheetHandler
}

// QuestionnaireHandler 获取问卷处理器
func (c *Container) QuestionnaireHandler() *handler.QuestionnaireHandler {
	return c.questionnaireHandler
}

// HealthHandler 获取健康检查处理器
func (c *Container) HealthHandler() *handler.HealthHandler {
	return c.healthHandler
}

// EvaluationHandler 获取测评处理器
func (c *Container) EvaluationHandler() *handler.EvaluationHandler {
	return c.evaluationHandler
}

// TesteeHandler 获取受试者处理器
func (c *Container) TesteeHandler() *handler.TesteeHandler {
	return c.testeeHandler
}

// ScaleHandler 获取量表处理器
func (c *Container) ScaleHandler() *handler.ScaleHandler {
	return c.scaleHandler
}

// RateLimitOptions 获取限流配置
func (c *Container) RateLimitOptions() *options.RateLimitOptions {
	return c.opts.RateLimit
}

// ==================== Setters (用于 GRPCClientRegistry 注入) ====================

// SetAnswerSheetClient 设置答卷客户端
func (c *Container) SetAnswerSheetClient(client *grpcclient.AnswerSheetClient) {
	c.answerSheetClient = client
}

// SetQuestionnaireClient 设置问卷客户端
func (c *Container) SetQuestionnaireClient(client *grpcclient.QuestionnaireClient) {
	c.questionnaireClient = client
}

// SetEvaluationClient 设置测评客户端
func (c *Container) SetEvaluationClient(client *grpcclient.EvaluationClient) {
	c.evaluationClient = client
}

// SetActorClient 设置 Actor 客户端
func (c *Container) SetActorClient(client *grpcclient.ActorClient) {
	c.actorClient = client
}

// SetScaleClient 设置量表客户端
func (c *Container) SetScaleClient(client *grpcclient.ScaleClient) {
	c.scaleClient = client
}

// ActorClient 获取 Actor 客户端
func (c *Container) ActorClient() *grpcclient.ActorClient {
	return c.actorClient
}
