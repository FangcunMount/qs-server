package container

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/collection-server/application/validation"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/infrastructure/grpc"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/interface/restful/handler"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/options"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Container 主容器，负责管理所有组件
type Container struct {
	// 基础设施层
	QuestionnaireClient grpc.QuestionnaireClient
	AnswersheetClient   grpc.AnswersheetClient

	// 应用层
	ValidationService validation.Service

	// 接口层
	QuestionnaireHandler handler.QuestionnaireHandler
	AnswersheetHandler   handler.AnswersheetHandler

	// 配置
	grpcClientConfig *options.GRPCClientOptions
	initialized      bool
}

// NewContainer 创建新的容器
func NewContainer(grpcClientConfig *options.GRPCClientOptions) *Container {
	return &Container{
		grpcClientConfig: grpcClientConfig,
		initialized:      false,
	}
}

// Initialize 初始化容器中的所有组件
func (c *Container) Initialize() error {
	if c.initialized {
		return nil
	}

	log.Info("🔧 Initializing Collection Server Container...")

	// 1. 初始化基础设施层（GRPC 客户端）
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
	return nil
}

// initializeApplication 初始化应用层
func (c *Container) initializeApplication() error {
	log.Info("   📋 Initializing application services...")

	// 创建校验服务
	c.ValidationService = validation.NewService()

	log.Info("   ✅ Application services initialized")
	return nil
}

// initializeInterface 初始化接口层
func (c *Container) initializeInterface() error {
	log.Info("   🌐 Initializing interface handlers...")

	// 创建处理器
	c.QuestionnaireHandler = handler.NewQuestionnaireHandler(
		c.QuestionnaireClient,
		c.ValidationService,
	)

	c.AnswersheetHandler = handler.NewAnswersheetHandler(
		c.AnswersheetClient,
		c.ValidationService,
	)

	log.Info("   ✅ Interface handlers initialized")
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
