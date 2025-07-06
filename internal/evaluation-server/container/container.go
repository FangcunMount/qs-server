package container

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/application/message"
	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/options"
	"github.com/yshujie/questionnaire-scale/pkg/log"
	"github.com/yshujie/questionnaire-scale/pkg/pubsub"
)

// Container 主容器，负责管理所有组件
type Container struct {
	// 基础设施层
	Subscriber *pubsub.RedisSubscriber

	// 应用层
	MessageHandler message.Handler

	// 配置
	grpcClientConfig   *options.GRPCClientOptions
	messageQueueConfig *options.MessageQueueOptions
	initialized        bool
}

// NewContainer 创建新的容器
func NewContainer(grpcClient *options.GRPCClientOptions, messageQueue *options.MessageQueueOptions) *Container {
	return &Container{
		grpcClientConfig:   grpcClient,
		messageQueueConfig: messageQueue,
		initialized:        false,
	}
}

// Initialize 初始化容器中的所有组件
func (c *Container) Initialize() error {
	if c.initialized {
		return nil
	}

	log.Info("🔧 Initializing Evaluation Server Container...")

	// 1. 初始化应用层
	if err := c.initializeApplication(); err != nil {
		return fmt.Errorf("failed to initialize application: %w", err)
	}

	// 2. 初始化基础设施层（Redis订阅者）
	if err := c.initializeInfrastructure(); err != nil {
		return fmt.Errorf("failed to initialize infrastructure: %w", err)
	}

	c.initialized = true
	log.Info("✅ Evaluation Server Container initialized successfully")

	return nil
}

// initializeApplication 初始化应用层
func (c *Container) initializeApplication() error {
	log.Info("   📋 Initializing application services...")

	// 创建消息处理器
	c.MessageHandler = message.NewHandler()

	log.Info("   ✅ Application services initialized")
	return nil
}

// initializeInfrastructure 初始化基础设施层
func (c *Container) initializeInfrastructure() error {
	log.Info("   📡 Initializing Redis subscriber...")

	// 创建 Redis 订阅者
	if c.messageQueueConfig.Type == "redis" {
		redisConfig := c.messageQueueConfig.ToRedisConfig()
		if redisConfig == nil {
			return fmt.Errorf("failed to convert message queue config to redis config")
		}

		c.Subscriber = pubsub.NewRedisSubscriber(redisConfig)

		// 连接 Redis
		ctx := context.Background()
		if err := c.Subscriber.Connect(ctx); err != nil {
			return fmt.Errorf("failed to connect to Redis: %w", err)
		}

		// 注册消息处理器
		c.Subscriber.RegisterHandler(c.messageQueueConfig.Topic, c.MessageHandler.GetMessageHandler())

		log.Info("   ✅ Redis subscriber initialized")
	} else {
		log.Warnf("   ⚠️  Unsupported message queue type: %s", c.messageQueueConfig.Type)
	}

	return nil
}

// StartSubscription 启动消息订阅
func (c *Container) StartSubscription(ctx context.Context) error {
	if !c.initialized {
		return fmt.Errorf("container not initialized")
	}

	if c.Subscriber == nil {
		return fmt.Errorf("subscriber not initialized")
	}

	log.Infof("🚀 Starting message subscription for topic: %s", c.messageQueueConfig.Topic)

	// 启动订阅（这是一个阻塞操作）
	return c.Subscriber.Subscribe(ctx, c.messageQueueConfig.Topic, c.MessageHandler.GetMessageHandler())
}

// StartMessageSubscriber 启动消息队列订阅者（保持兼容性）
func (c *Container) StartMessageSubscriber() error {
	ctx := context.Background()
	return c.StartSubscription(ctx)
}

// HealthCheck 检查容器健康状态
func (c *Container) HealthCheck(ctx context.Context) error {
	if !c.initialized {
		return fmt.Errorf("container not initialized")
	}

	// 检查 Redis 连接
	if c.Subscriber != nil {
		if err := c.Subscriber.HealthCheck(ctx); err != nil {
			return fmt.Errorf("redis subscriber health check failed: %w", err)
		}
	}

	return nil
}

// Cleanup 清理资源
func (c *Container) Cleanup() error {
	log.Info("🧹 Cleaning up container resources...")

	// 关闭 Redis 订阅者
	if c.Subscriber != nil {
		if err := c.Subscriber.Close(); err != nil {
			log.Errorf("Failed to close redis subscriber: %v", err)
		}
	}

	c.initialized = false
	log.Info("🏁 Container cleanup completed")

	return nil
}

// GetContainerInfo 获取容器信息
func (c *Container) GetContainerInfo() map[string]interface{} {
	return map[string]interface{}{
		"name":        "evaluation-server-container",
		"version":     "1.0.0",
		"initialized": c.initialized,
		"components": map[string]bool{
			"redis_subscriber": c.Subscriber != nil,
			"message_handler":  c.MessageHandler != nil,
		},
	}
}

// IsInitialized 检查容器是否已初始化
func (c *Container) IsInitialized() bool {
	return c.initialized
}
