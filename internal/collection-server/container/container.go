package container

import (
	"github.com/FangcunMount/iam-contracts/pkg/log"
)

// Container 主容器，负责管理所有组件
type Container struct {
	initialized bool
}

// NewContainer 创建新的容器
func NewContainer() *Container {
	return &Container{
		initialized: false,
	}
}

// Initialize 初始化容器中的所有组件
func (c *Container) Initialize() error {
	if c.initialized {
		return nil
	}

	log.Info("🔧 Initializing Collection Server Container...")

	// TODO: 在这里初始化各层组件
	// 1. 初始化基础设施层
	// 2. 初始化应用层
	// 3. 初始化接口层

	c.initialized = true
	log.Info("✅ Collection Server Container initialized successfully")

	return nil
}

// Cleanup 清理资源
func (c *Container) Cleanup() {
	log.Info("🧹 Cleaning up container resources...")

	// TODO: 清理各组件资源

	c.initialized = false
	log.Info("🏁 Container cleanup completed")
}

// IsInitialized 检查容器是否已初始化
func (c *Container) IsInitialized() bool {
	return c.initialized
}
