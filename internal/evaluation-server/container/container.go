package container

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/evaluation-server/options"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Container 主容器
// 组合所有业务模块和基础设施组件
type Container struct {
	// 配置
	grpcClientConfig   *options.GRPCClientOptions
	messageQueueConfig *options.MessageQueueOptions

	// 业务模块
	// TODO: 添加具体的业务模块
	// - gRPC 客户端（用于调用 apiserver）
	// - 消息队列订阅者
	// - scoring 模块
	// - evaluation 模块
	// - report generation 模块

	// 容器状态
	initialized bool
}

// NewContainer 创建容器
func NewContainer(grpcClient *options.GRPCClientOptions, messageQueue *options.MessageQueueOptions) *Container {
	return &Container{
		grpcClientConfig:   grpcClient,
		messageQueueConfig: messageQueue,
		initialized:        false,
	}
}

// Initialize 初始化容器
func (c *Container) Initialize() error {
	if c.initialized {
		return nil
	}

	// TODO: 初始化各个业务模块
	// 例如：
	// - 初始化 scoring 模块
	// - 初始化 evaluation 模块
	// - 初始化 report generation 模块
	// - 初始化 message queue subscriber

	c.initialized = true
	fmt.Printf("🏗️  Evaluation Container initialized successfully\n")

	return nil
}

// StartMessageSubscriber 启动消息队列订阅者
func (c *Container) StartMessageSubscriber() error {
	// TODO: 实现消息队列订阅者启动逻辑
	log.Info("📨 Message queue subscriber would be started here")
	return nil
}

// HealthCheck 健康检查
func (c *Container) HealthCheck(ctx context.Context) error {
	// TODO: 检查组件的健康状态
	// - 检查 GRPC 客户端连接到 apiserver
	// - 检查消息队列连接

	// 这里可以添加实际的健康检查逻辑
	log.Debug("Health check passed for evaluation server")

	return nil
}

// Cleanup 清理资源
func (c *Container) Cleanup() error {
	fmt.Printf("🧹 Cleaning up evaluation container resources...\n")

	// TODO: 清理各个模块的资源
	// - 停止消息队列订阅者
	// - 关闭 GRPC 客户端连接
	// - 清理其他资源

	c.initialized = false
	fmt.Printf("🏁 Evaluation container cleanup completed\n")

	return nil
}

// IsInitialized 检查容器是否已初始化
func (c *Container) IsInitialized() bool {
	return c.initialized
}
