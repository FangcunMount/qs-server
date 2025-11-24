package container

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/iam-contracts/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/validation"
	"github.com/FangcunMount/qs-server/internal/collection-server/infrastructure/auth"
	"github.com/FangcunMount/qs-server/internal/collection-server/infrastructure/grpc"
	"github.com/FangcunMount/qs-server/internal/collection-server/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
	"github.com/FangcunMount/qs-server/pkg/pubsub"
)

// Container 主容器，负责管理所有组件
type Container struct {
	// 基础设施层
	QuestionnaireClient grpc.QuestionnaireClient
	AnswersheetClient   grpc.AnswersheetClient
	Publisher           pubsub.Publisher
	JWTManager          *auth.JWTManager

	// 应用层
	ValidationService           validation.Service
	ValidationServiceConcurrent validation.ServiceConcurrent
	AnswersheetService          answersheet.Service
	QuestionnaireService        questionnaire.Service

	// 接口层
	QuestionnaireHandler handler.QuestionnaireHandler
	AnswersheetHandler   handler.AnswersheetHandler

	// 配置
	grpcClientConfig  *options.GRPCClientOptions
	pubsubConfig      *pubsub.Config
	concurrencyConfig *options.ConcurrencyOptions
	jwtConfig         *options.JWTOptions
	initialized       bool
}

// NewContainer 创建新的容器
func NewContainer(
	grpcClientConfig *options.GRPCClientOptions,
	pubsubConfig *pubsub.Config,
	concurrencyConfig *options.ConcurrencyOptions,
	jwtConfig *options.JWTOptions,
) *Container {
	return &Container{
		grpcClientConfig:  grpcClientConfig,
		pubsubConfig:      pubsubConfig,
		concurrencyConfig: concurrencyConfig,
		jwtConfig:         jwtConfig,
		initialized:       false,
	}
}

// Initialize 初始化容器中的所有组件
func (c *Container) Initialize() error {
	if c.initialized {
		return nil
	}

	log.Info("🔧 Initializing Collection Server Container...")

	// 1. 初始化基础设施层（GRPC 客户端和Watermill发布者）
	if err := c.initializeInfrastructure(); err != nil {
		return fmt.Errorf("failed to initialize infrastructure: %w", err)
	}

	// 2. 初始化应用层
	if err := c.initializeApplication(); err != nil {
		return fmt.Errorf("failed to initialize application: %w", err)
	}

	// 3. 初始化接口层
	if err := c.initializeInterface(); err != nil {
		return fmt.Errorf("failed to initialize interface: %w", err)
	}

	c.initialized = true
	log.Info("✅ Collection Server Container initialized successfully")

	return nil
}

// initializeInfrastructure 初始化基础设施层
func (c *Container) initializeInfrastructure() error {
	log.Info("   🔌 Initializing GRPC clients...")

	// 创建 GRPC 客户端
	questionnaireClient, err := grpc.NewQuestionnaireClient(c.grpcClientConfig)
	if err != nil {
		return fmt.Errorf("failed to create questionnaire client: %w", err)
	}
	c.QuestionnaireClient = questionnaireClient

	answersheetClient, err := grpc.NewAnswersheetClient(c.grpcClientConfig)
	if err != nil {
		return fmt.Errorf("failed to create answersheet client: %w", err)
	}
	c.AnswersheetClient = answersheetClient

	log.Info("   ✅ GRPC clients initialized")

	// 创建 JWT 管理器
	log.Info("   🔐 Initializing JWT manager...")
	c.JWTManager = auth.NewJWTManager(
		c.jwtConfig.SecretKey,
		time.Duration(c.jwtConfig.TokenDuration)*time.Hour,
	)
	log.Info("   ✅ JWT manager initialized")

	// 创建发布者
	log.Info("   📡 Initializing publisher...")
	publisher, err := pubsub.NewPublisher(c.pubsubConfig)
	if err != nil {
		return fmt.Errorf("failed to create publisher: %w", err)
	}
	c.Publisher = publisher

	log.Info("   ✅ Publisher initialized")
	return nil
}

// initializeApplication 初始化应用层
func (c *Container) initializeApplication() error {
	log.Info("   📋 Initializing application services...")

	// 创建问卷验证器（直接使用 gRPC client）
	questionnaireValidator := validation.NewQuestionnaireValidator(c.QuestionnaireClient)

	// 创建验证规则工厂
	ruleFactory := validation.NewDefaultValidationRuleFactory()

	// 创建答案验证器（并发版本）
	answerValidatorConcurrent := validation.NewAnswerValidatorConcurrent(ruleFactory, c.concurrencyConfig.MaxConcurrency)

	// 创建并发校验服务
	concurrentService := validation.NewServiceConcurrent(questionnaireValidator, answerValidatorConcurrent)

	// 使用适配器让并发服务实现原有Service接口
	c.ValidationService = validation.NewServiceAdapter(concurrentService)

	// 保存并发服务引用（用于直接访问并发功能）
	c.ValidationServiceConcurrent = concurrentService

	// 先创建问卷应用服务（答卷服务依赖它）
	c.QuestionnaireService = questionnaire.NewService(c.QuestionnaireClient)

	// 再创建答卷应用服务
	c.AnswersheetService = answersheet.NewService(c.AnswersheetClient, c.Publisher, c.QuestionnaireService)

	log.Infof("   ✅ Application services initialized (using concurrent validation, max concurrency: %d)", c.concurrencyConfig.MaxConcurrency)
	return nil
}

// initializeInterface 初始化接口层
func (c *Container) initializeInterface() error {
	log.Info("   🌐 Initializing interface handlers...")

	// 创建处理器（使用应用服务）
	c.QuestionnaireHandler = handler.NewQuestionnaireHandler(
		c.QuestionnaireService, // 使用问卷应用服务
		c.QuestionnaireClient,  // 保留gRPC客户端用于List操作
	)

	c.AnswersheetHandler = handler.NewAnswersheetHandler(
		c.AnswersheetService, // 使用答卷应用服务
		c.AnswersheetClient,  // 保留gRPC客户端用于查询操作
	)

	log.Info("   ✅ Interface handlers initialized (using concurrent validation via adapter)")
	return nil
}

// HealthCheck 检查容器健康状态
func (c *Container) HealthCheck(ctx context.Context) error {
	if !c.initialized {
		return fmt.Errorf("container not initialized")
	}

	// 检查 GRPC 客户端连接
	if err := c.QuestionnaireClient.HealthCheck(ctx); err != nil {
		return fmt.Errorf("questionnaire client health check failed: %w", err)
	}

	if err := c.AnswersheetClient.HealthCheck(ctx); err != nil {
		return fmt.Errorf("answersheet client health check failed: %w", err)
	}

	// Watermill 发布者不需要额外的健康检查
	log.Info("   ✅ All components healthy")

	return nil
}

// Cleanup 清理资源
func (c *Container) Cleanup() error {
	log.Info("🧹 Cleaning up container resources...")

	// 关闭 GRPC 客户端连接
	if c.QuestionnaireClient != nil {
		if err := c.QuestionnaireClient.Close(); err != nil {
			log.Errorf("Failed to close questionnaire client: %v", err)
		}
	}

	if c.AnswersheetClient != nil {
		if err := c.AnswersheetClient.Close(); err != nil {
			log.Errorf("Failed to close answersheet client: %v", err)
		}
	}

	// 关闭 Watermill 发布者
	if c.Publisher != nil {
		if err := c.Publisher.Close(); err != nil {
			log.Errorf("Failed to close watermill publisher: %v", err)
		}
	}

	c.initialized = false
	log.Info("🏁 Container cleanup completed")

	return nil
}

// GetContainerInfo 获取容器信息
func (c *Container) GetContainerInfo() map[string]interface{} {
	return map[string]interface{}{
		"name":        "collection-server-container",
		"version":     "1.0.0",
		"initialized": c.initialized,
		"components": map[string]bool{
			"questionnaire_client":  c.QuestionnaireClient != nil,
			"answersheet_client":    c.AnswersheetClient != nil,
			"watermill_publisher":   c.Publisher != nil,
			"validation_service":    c.ValidationService != nil,
			"questionnaire_handler": c.QuestionnaireHandler != nil,
			"answersheet_handler":   c.AnswersheetHandler != nil,
		},
	}
}

// IsInitialized 检查容器是否已初始化
func (c *Container) IsInitialized() bool {
	return c.initialized
}

// GetPublisher 获取发布者
func (c *Container) GetPublisher() pubsub.Publisher {
	return c.Publisher
}
