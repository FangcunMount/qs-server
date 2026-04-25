package container

import (
	"context"
	"log/slog"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	"github.com/FangcunMount/qs-server/internal/worker/application"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	workernotifier "github.com/FangcunMount/qs-server/internal/worker/infra/notifier"
	"github.com/FangcunMount/qs-server/internal/worker/options"
	"github.com/FangcunMount/qs-server/internal/worker/port"
)

// Container 主容器，负责管理所有组件
type Container struct {
	initialized bool
	opts        *options.Options
	logger      *slog.Logger
	lockManager *redislock.Manager
	lockBuilder *rediskey.Builder

	// gRPC 客户端（由 GRPCClientRegistry 注入）
	answerSheetClient *grpcclient.AnswerSheetClient
	evaluationClient  *grpcclient.EvaluationClient
	internalClient    *grpcclient.InternalClient

	// 事件分发器
	eventDispatcher *application.EventDispatcher
}

// NewContainer 创建新的容器
func NewContainer(opts *options.Options, logger *slog.Logger, lockHandle *redisplane.Handle, lockManager *redislock.Manager) *Container {
	lockBuilder := rediskey.NewBuilder()
	if lockHandle != nil {
		lockBuilder = lockHandle.Builder
	}
	return &Container{
		opts:        opts,
		logger:      logger,
		lockManager: lockManager,
		lockBuilder: lockBuilder,
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
		LockManager:       c.lockManager,
		LockKeyBuilder:    c.lockBuilder,
		Notifier:          c.buildNotifier(),
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

func (c *Container) buildNotifier() port.TaskNotifier {
	if c.opts == nil || c.opts.Notification == nil {
		return nil
	}

	gatewayNotifier := workernotifier.NewGatewayNotifier(
		c.opts.Notification.GatewayURL,
		c.opts.Notification.GatewayToken,
		time.Duration(c.opts.Notification.TimeoutMs)*time.Millisecond,
	)
	if gatewayNotifier != nil {
		if c.opts.Notification.WebhookURL != "" && c.logger != nil {
			c.logger.Info("notification gateway configured; webhook adapter disabled",
				"gateway_url", c.opts.Notification.GatewayURL,
				"webhook_url", c.opts.Notification.WebhookURL,
			)
		}
		return gatewayNotifier
	}
	return workernotifier.NewWebhookNotifier(
		c.opts.Notification.WebhookURL,
		time.Duration(c.opts.Notification.TimeoutMs)*time.Millisecond,
		c.opts.Notification.SharedSecret,
	)
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
func (c *Container) GetTopicSubscriptions() []eventcatalog.TopicSubscription {
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
