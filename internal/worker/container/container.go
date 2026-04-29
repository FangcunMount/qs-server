package container

import (
	"context"
	"log/slog"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	workernotifier "github.com/FangcunMount/qs-server/internal/worker/infra/notifier"
	workereventing "github.com/FangcunMount/qs-server/internal/worker/integration/eventing"
	"github.com/FangcunMount/qs-server/internal/worker/options"
	"github.com/FangcunMount/qs-server/internal/worker/port"
)

// Container 主容器，负责管理所有组件
type Container struct {
	initialized  bool
	opts         *options.Options
	logger       *slog.Logger
	lockManager  locklease.Manager
	lockBuilder  *keyspace.Builder
	eventCatalog *eventcatalog.Catalog

	// gRPC 客户端（由 GRPCClientRegistry 注入）
	answerSheetClient *grpcclient.AnswerSheetClient
	evaluationClient  *grpcclient.EvaluationClient
	internalClient    *grpcclient.InternalClient

	// 事件分发器
	eventDispatcher *workereventing.Dispatcher
}

// ClientBundle is the worker runtime client graph produced by the gRPC
// integration stage and consumed by the container composition root.
type ClientBundle struct {
	AnswerSheet *grpcclient.AnswerSheetClient
	Evaluation  *grpcclient.EvaluationClient
	Internal    *grpcclient.InternalClient
}

// NewContainer 创建新的容器
func NewContainer(opts *options.Options, logger *slog.Logger, lockHandle *cacheplane.Handle, lockManager locklease.Manager, eventCatalog *eventcatalog.Catalog) *Container {
	lockBuilder := keyspace.NewBuilder()
	if lockHandle != nil {
		lockBuilder = lockHandle.Builder
	}
	return &Container{
		opts:         opts,
		logger:       logger,
		lockManager:  lockManager,
		lockBuilder:  lockBuilder,
		eventCatalog: eventCatalog,
		initialized:  false,
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
	deps := &workereventing.HandlerDependencies{
		Logger:            c.logger,
		AnswerSheetClient: c.answerSheetClient,
		EvaluationClient:  c.evaluationClient,
		InternalClient:    c.internalClient,
		LockManager:       c.lockManager,
		LockKeyBuilder:    c.lockBuilder,
		Notifier:          c.buildNotifier(),
	}

	// 创建事件分发器
	c.eventDispatcher = workereventing.NewDispatcher(c.logger, deps, handlers.NewRegistry())

	if err := c.eventDispatcher.Initialize(c.eventCatalog); err != nil {
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

// InitializeRuntimeClients installs the runtime client bundle built by the
// integration stage. It replaces per-client setter wiring with one explicit
// composition edge.
func (c *Container) InitializeRuntimeClients(bundle ClientBundle) {
	if c == nil {
		return
	}
	c.answerSheetClient = bundle.AnswerSheet
	c.evaluationClient = bundle.Evaluation
	c.internalClient = bundle.Internal
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
	return c.eventDispatcher.DispatchEvent(ctx, eventType, payload)
}

// ResilienceSnapshot returns the worker's current resilience capability summary.
func (c *Container) ResilienceSnapshot() resilienceplane.RuntimeSnapshot {
	snapshot := resilienceplane.NewRuntimeSnapshot("worker", time.Now())
	lockConfigured := c != nil && c.lockManager != nil
	lockReason := ""
	if !lockConfigured {
		lockReason = "worker duplicate suppression lock manager unavailable"
	}
	snapshot.DuplicateSuppression = []resilienceplane.CapabilitySnapshot{{
		Name:       "answersheet_submitted",
		Kind:       resilienceplane.ProtectionDuplicateSuppression.String(),
		Strategy:   "redis_lock",
		Configured: lockConfigured,
		Degraded:   !lockConfigured,
		Reason:     lockReason,
	}}
	snapshot.Locks = []resilienceplane.CapabilitySnapshot{{
		Name:       "answersheet_processing",
		Kind:       resilienceplane.ProtectionLock.String(),
		Strategy:   "redis_lock",
		Configured: lockConfigured,
		Degraded:   !lockConfigured,
		Reason:     lockReason,
	}}
	return resilienceplane.FinalizeRuntimeSnapshot(snapshot)
}

// Logger 获取日志器
func (c *Container) Logger() *slog.Logger {
	return c.logger
}

// Options 获取配置
func (c *Container) Options() *options.Options {
	return c.opts
}
