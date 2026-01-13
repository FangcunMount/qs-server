package container

import (
	"context"
	"log/slog"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	"github.com/FangcunMount/qs-server/internal/worker/application"
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

	// gRPC 客户端（由 GRPCClientRegistry 注入）
	answerSheetClient *grpcclient.AnswerSheetClient
	evaluationClient  *grpcclient.EvaluationClient
	internalClient    *grpcclient.InternalClient

	// 事件分发器
	eventDispatcher *application.EventDispatcher
}

// NewContainer 创建新的容器
func NewContainer(opts *options.Options, logger *slog.Logger, redisCache redis.UniversalClient) *Container {
	return &Container{
		opts:        opts,
		logger:      logger,
		redisCache:  redisCache,
		initialized: false,
	}
}

// Initialize 初始化容器中的所有组件
func (c *Container) Initialize() error {
	if c.initialized {
		return nil
	}

	log.Info("🔧 Initializing Worker Container...")

	// 初始化事件分发器
	if err := c.initEventDispatcher(); err != nil {
		return err
	}

	c.initialized = true
	log.Info("✅ Worker Container initialized successfully")

	return nil
}

// initEventDispatcher 初始化事件分发器
func (c *Container) initEventDispatcher() error {
	log.Info("🎯 Initializing event dispatcher...")

	// 构建处理器依赖
	deps := &application.HandlerDependencies{
		Logger:            c.logger,
		AnswerSheetClient: c.answerSheetClient,
		EvaluationClient:  c.evaluationClient,
		InternalClient:    c.internalClient,
		RedisCache:        c.redisCache,
	}

	// 创建事件分发器
	c.eventDispatcher = application.NewEventDispatcher(c.logger, deps)

	// 确定配置路径
	configPath := "configs/events.yaml"
	if c.opts.Worker != nil && c.opts.Worker.EventConfigPath != "" {
		configPath = c.opts.Worker.EventConfigPath
	}

	// 初始化
	if err := c.eventDispatcher.Initialize(configPath); err != nil {
		return err
	}

	// 打印订阅信息
	c.eventDispatcher.PrintSubscriptionInfo()

	log.Info("✅ Event dispatcher initialized")
	return nil
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

// SetInternalClient 设置内部服务客户端
func (c *Container) SetInternalClient(client *grpcclient.InternalClient) {
	c.internalClient = client
}

// ==================== Getters ====================

// GetTopicSubscriptions 获取需要订阅的 Topic 列表
func (c *Container) GetTopicSubscriptions() []eventconfig.TopicSubscription {
	if c.eventDispatcher == nil {
		return nil
	}
	return c.eventDispatcher.GetTopicSubscriptions()
}

// DispatchEvent 分发事件到对应的处理器
func (c *Container) DispatchEvent(ctx context.Context, eventType string, payload []byte) error {
	if c.eventDispatcher == nil {
		return nil
	}
	return c.eventDispatcher.Dispatch(ctx, eventType, payload)
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
