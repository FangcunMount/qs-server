package container

import (
	"log/slog"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/worker/options"
	redis "github.com/redis/go-redis/v9"
)

// Container 主容器，负责管理所有组件
type Container struct {
	initialized bool
	opts        *options.Options
	logger      *slog.Logger
	redisCache  redis.UniversalClient
	redisStore  redis.UniversalClient

	// gRPC 客户端（由 GRPCClientRegistry 注入）
	answerSheetClient *grpcclient.AnswerSheetClient
	evaluationClient  *grpcclient.EvaluationClient

	// 处理器注册表
	handlerRegistry *handlers.TopicRegistry
}

// NewContainer 创建新的容器
func NewContainer(opts *options.Options, logger *slog.Logger, redisCache redis.UniversalClient, redisStore redis.UniversalClient) *Container {
	return &Container{
		opts:        opts,
		logger:      logger,
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

	log.Info("🔧 Initializing Worker Container...")

	// 1. 初始化处理器注册表
	c.initHandlerRegistry()

	c.initialized = true
	log.Info("✅ Worker Container initialized successfully")

	return nil
}

// initHandlerRegistry 初始化 Topic 处理器注册表
func (c *Container) initHandlerRegistry() {
	log.Info("🎯 Initializing topic handler registry...")

	c.handlerRegistry = handlers.NewTopicRegistry(c.logger)
	handlers.RegisterDefaultTopicHandlers(c.handlerRegistry, &handlers.TopicHandlerDeps{
		Logger:            c.logger,
		AnswerSheetClient: c.answerSheetClient,
		EvaluationClient:  c.evaluationClient,
	})

	log.Info("✅ Topic handler registry initialized")
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

// ==================== Setters (用于 GRPCClientRegistry 注入) ====================

// SetAnswerSheetClient 设置答卷客户端
func (c *Container) SetAnswerSheetClient(client *grpcclient.AnswerSheetClient) {
	c.answerSheetClient = client
}

// SetEvaluationClient 设置测评客户端
func (c *Container) SetEvaluationClient(client *grpcclient.EvaluationClient) {
	c.evaluationClient = client
}

// ==================== Getters ====================

// TopicRegistry 获取 Topic 处理器注册表
func (c *Container) TopicRegistry() *handlers.TopicRegistry {
	return c.handlerRegistry
}

// Logger 获取日志器
func (c *Container) Logger() *slog.Logger {
	return c.logger
}

// Options 获取配置
func (c *Container) Options() *options.Options {
	return c.opts
}

// RedisCache 获取缓存 Redis 客户端
func (c *Container) RedisCache() redis.UniversalClient {
	return c.redisCache
}

// RedisStore 获取存储 Redis 客户端
func (c *Container) RedisStore() redis.UniversalClient {
	return c.redisStore
}
