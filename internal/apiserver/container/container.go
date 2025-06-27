package container

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	userModule "github.com/yshujie/questionnaire-scale/internal/apiserver/module/user"
)

// Container 主容器
// 组合所有业务模块和基础设施组件
type Container struct {
	// 基础设施
	mysqlDB     *gorm.DB
	mongoClient *mongo.Client
	mongoDB     string

	// 业务模块
	userModule *userModule.Module
	// questionnaireModule *questionnaireModule.Module  // 待实现

	// 容器状态
	initialized bool
}

// NewContainer 创建容器
func NewContainer(mysqlDB *gorm.DB, mongoClient *mongo.Client, mongoDB string) *Container {
	return &Container{
		mysqlDB:     mysqlDB,
		mongoClient: mongoClient,
		mongoDB:     mongoDB,
		initialized: false,
	}
}

// Initialize 初始化容器
func (c *Container) Initialize() error {
	if c.initialized {
		return nil
	}

	// 初始化用户模块
	if err := c.initUserModule(); err != nil {
		return fmt.Errorf("failed to initialize user module: %w", err)
	}

	// 初始化其他模块...
	// if err := c.initQuestionnaireModule(); err != nil {
	//     return fmt.Errorf("failed to initialize questionnaire module: %w", err)
	// }

	c.initialized = true
	fmt.Printf("🏗️  Container initialized with modules: user\n")

	return nil
}

// initUserModule 初始化用户模块
func (c *Container) initUserModule() error {
	c.userModule = userModule.NewModule(c.mysqlDB)
	fmt.Printf("📦 User module initialized\n")
	return nil
}

// GetUserModule 获取用户模块
func (c *Container) GetUserModule() *userModule.Module {
	return c.userModule
}

// GetUserHandler 获取用户处理器（便捷方法）
func (c *Container) GetUserHandler() interface{} {
	if c.userModule == nil {
		return nil
	}
	return c.userModule.GetHandler()
}

// GetUserService 获取用户服务（便捷方法）
func (c *Container) GetUserService() interface{} {
	if c.userModule == nil {
		return nil
	}
	return c.userModule.GetService()
}

// GetUserRepository 获取用户仓库（便捷方法）
func (c *Container) GetUserRepository() interface{} {
	if c.userModule == nil {
		return nil
	}
	return c.userModule.GetRepository()
}

// HealthCheck 健康检查
func (c *Container) HealthCheck(ctx context.Context) error {
	// 检查MySQL连接
	if c.mysqlDB != nil {
		sqlDB, err := c.mysqlDB.DB()
		if err != nil {
			return fmt.Errorf("failed to get mysql db: %w", err)
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			return fmt.Errorf("mysql ping failed: %w", err)
		}
	}

	// 检查MongoDB连接（如果有）
	if c.mongoClient != nil {
		if err := c.mongoClient.Ping(ctx, nil); err != nil {
			return fmt.Errorf("mongodb ping failed: %w", err)
		}
	}

	// 检查模块健康状态
	if err := c.checkModulesHealth(ctx); err != nil {
		return fmt.Errorf("modules health check failed: %w", err)
	}

	return nil
}

// checkModulesHealth 检查模块健康状态
func (c *Container) checkModulesHealth(ctx context.Context) error {
	// 检查用户模块健康状态
	if c.userModule != nil {
		// 这里可以添加模块特定的健康检查
		// 例如：检查模块依赖是否正常工作
		if c.userModule.GetRepository() == nil {
			return fmt.Errorf("user repository is nil")
		}
		if c.userModule.GetService() == nil {
			return fmt.Errorf("user service is nil")
		}
		if c.userModule.GetHandler() == nil {
			return fmt.Errorf("user handler is nil")
		}
	}

	return nil
}

// Cleanup 清理资源
func (c *Container) Cleanup() error {
	fmt.Printf("🧹 Cleaning up container resources...\n")

	// 清理用户模块
	if c.userModule != nil {
		if err := c.userModule.Cleanup(); err != nil {
			return fmt.Errorf("failed to cleanup user module: %w", err)
		}
		fmt.Printf("   ✅ User module cleaned up\n")
	}

	// 清理其他模块...
	// if c.questionnaireModule != nil {
	//     if err := c.questionnaireModule.Cleanup(); err != nil {
	//         return fmt.Errorf("failed to cleanup questionnaire module: %w", err)
	//     }
	//     fmt.Printf("   ✅ Questionnaire module cleaned up\n")
	// }

	c.initialized = false
	fmt.Printf("🏁 Container cleanup completed\n")

	return nil
}

// GetContainerInfo 获取容器信息
func (c *Container) GetContainerInfo() map[string]interface{} {
	modules := make(map[string]interface{})

	if c.userModule != nil {
		modules["user"] = c.userModule.ModuleInfo()
	}

	return map[string]interface{}{
		"name":         "apiserver-container",
		"version":      "2.0.0",
		"architecture": "hexagonal",
		"initialized":  c.initialized,
		"modules":      modules,
		"infrastructure": map[string]bool{
			"mysql":   c.mysqlDB != nil,
			"mongodb": c.mongoClient != nil,
		},
	}
}

// IsInitialized 检查容器是否已初始化
func (c *Container) IsInitialized() bool {
	return c.initialized
}

// GetLoadedModules 获取已加载的模块列表
func (c *Container) GetLoadedModules() []string {
	modules := make([]string, 0)

	if c.userModule != nil {
		modules = append(modules, "user")
	}

	// 如果有其他模块，继续添加
	// if c.questionnaireModule != nil {
	//     modules = append(modules, "questionnaire")
	// }

	return modules
}

// PrintContainerInfo 打印容器信息
func (c *Container) PrintContainerInfo() {
	info := c.GetContainerInfo()

	fmt.Printf("🏗️  Container Information:\n")
	fmt.Printf("   Name: %s\n", info["name"].(string))
	fmt.Printf("   Version: %s\n", info["version"].(string))
	fmt.Printf("   Architecture: %s\n", info["architecture"].(string))
	fmt.Printf("   Initialized: %v\n", info["initialized"].(bool))

	infra := info["infrastructure"].(map[string]bool)
	fmt.Printf("   Infrastructure:\n")
	if infra["mysql"] {
		fmt.Printf("     • MySQL: ✅\n")
	} else {
		fmt.Printf("     • MySQL: ❌\n")
	}
	if infra["mongodb"] {
		fmt.Printf("     • MongoDB: ✅\n")
	} else {
		fmt.Printf("     • MongoDB: ❌\n")
	}

	fmt.Printf("   Loaded Modules:\n")
	for _, module := range c.GetLoadedModules() {
		fmt.Printf("     • %s\n", module)
	}
}
