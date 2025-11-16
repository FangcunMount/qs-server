package container

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/evaluation-server/application/message"
	grpcclient "github.com/FangcunMount/qs-server/internal/evaluation-server/infrastructure/grpc"
	"github.com/FangcunMount/qs-server/internal/evaluation-server/options"
	"github.com/FangcunMount/qs-server/pkg/pubsub"
)

// Container 容器
type Container struct {
	// gRPC 客户端
	QuestionnaireClient   *grpcclient.QuestionnaireClient
	AnswerSheetClient     *grpcclient.AnswerSheetClient
	MedicalScaleClient    *grpcclient.MedicalScaleClient
	InterpretReportClient *grpcclient.InterpretReportClient

	// gRPC 客户端工厂
	grpcClientFactory *grpcclient.ClientFactory

	// 基础设施层
	Subscriber pubsub.Subscriber

	// 应用层
	MessageHandler message.Handler

	// 配置
	grpcClientConfig   *options.GRPCClientOptions
	messageQueueConfig *options.MessageQueueOptions
	concurrencyConfig  *options.ConcurrencyOptions
	pubsubConfig       *pubsub.Config
	initialized        bool
}

// NewContainer 创建新的容器
func NewContainer(grpcClient *options.GRPCClientOptions, messageQueue *options.MessageQueueOptions, concurrency *options.ConcurrencyOptions, pubsubConfig *pubsub.Config) *Container {
	return &Container{
		grpcClientConfig:   grpcClient,
		messageQueueConfig: messageQueue,
		concurrencyConfig:  concurrency,
		pubsubConfig:       pubsubConfig,
		initialized:        false,
	}
}

// Initialize 初始化容器中的所有组件
func (c *Container) Initialize() error {
	if c.initialized {
		return nil
	}

	log.Info("🔧 Initializing Evaluation Server Container...")

	// 1. 初始化 gRPC 客户端
	if err := c.initializeGRPCClients(); err != nil {
		return fmt.Errorf("failed to initialize gRPC clients: %w", err)
	}

	// 2. 初始化应用层
	if err := c.initializeApplication(); err != nil {
		return fmt.Errorf("failed to initialize application: %w", err)
	}

	// 3. 初始化基础设施层（Watermill订阅者）
	if err := c.initializeInfrastructure(); err != nil {
		return fmt.Errorf("failed to initialize infrastructure: %w", err)
	}

	c.initialized = true
	log.Info("✅ Evaluation Server Container initialized successfully")

	return nil
}

// initializeGRPCClients 初始化 gRPC 客户端
func (c *Container) initializeGRPCClients() error {
	log.Info("   🔌 Initializing gRPC clients...")

	// 创建 gRPC 客户端工厂
	factory, err := grpcclient.NewClientFactory(c.grpcClientConfig.Endpoint)
	if err != nil {
		return fmt.Errorf("failed to create gRPC client factory: %w", err)
	}
	c.grpcClientFactory = factory

	// 创建 gRPC 客户端
	c.QuestionnaireClient = grpcclient.NewQuestionnaireClient(factory)
	c.AnswerSheetClient = grpcclient.NewAnswerSheetClient(factory)
	c.MedicalScaleClient = grpcclient.NewMedicalScaleClient(factory)
	c.InterpretReportClient = grpcclient.NewInterpretReportClient(factory)

	log.Info("   ✅ gRPC clients initialized")
	return nil
}

// initializeApplication 初始化应用层
func (c *Container) initializeApplication() error {
	log.Info("   📋 Initializing application services...")

	// 创建消息处理器，使用并发版本（从配置获取最大并发数）
	c.MessageHandler = message.NewHandlerWithConcurrency(
		c.AnswerSheetClient,
		c.QuestionnaireClient,
		c.MedicalScaleClient,
		c.InterpretReportClient,
		c.concurrencyConfig.MaxConcurrency, // 从配置获取最大并发数
	)

	log.Infof("   ✅ Application services initialized (with concurrent processing, max concurrency: %d)", c.concurrencyConfig.MaxConcurrency)
	return nil
}

// initializeInfrastructure 初始化基础设施层
func (c *Container) initializeInfrastructure() error {
	log.Info("   📡 Initializing Watermill subscriber...")

	// 创建订阅者
	subscriber, err := pubsub.NewSubscriber(c.pubsubConfig)
	if err != nil {
		return fmt.Errorf("failed to create subscriber: %w", err)
	}
	c.Subscriber = subscriber

	// 订阅消息
	if err := c.Subscriber.Subscribe(context.Background(), c.messageQueueConfig.Topic, c.MessageHandler.GetMessageHandler()); err != nil {
		return fmt.Errorf("failed to subscribe to topic %s: %w", c.messageQueueConfig.Topic, err)
	}

	log.Info("   ✅ Subscriber initialized")
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

	// 启动订阅者（这是一个阻塞操作）
	return c.Subscriber.Run(ctx)
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

	// 检查 Watermill 订阅者
	if c.Subscriber != nil {
		if err := c.Subscriber.HealthCheck(ctx); err != nil {
			return fmt.Errorf("watermill subscriber health check failed: %w", err)
		}
	}

	return nil
}

// Cleanup 清理容器资源
func (c *Container) Cleanup() error {
	log.Info("🧹 Cleaning up container resources...")

	// 关闭 Watermill 订阅者
	if c.Subscriber != nil {
		if err := c.Subscriber.Close(); err != nil {
			log.Errorf("Failed to close watermill subscriber: %v", err)
		}
	}

	// 关闭 gRPC 连接
	if c.grpcClientFactory != nil {
		if err := c.grpcClientFactory.Close(); err != nil {
			return fmt.Errorf("关闭 gRPC 连接失败: %v", err)
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
			"watermill_subscriber": c.Subscriber != nil,
			"message_handler":      c.MessageHandler != nil,
			"grpc_clients":         c.grpcClientFactory != nil,
		},
	}
}

// IsInitialized 检查容器是否已初始化
func (c *Container) IsInitialized() bool {
	return c.initialized
}
