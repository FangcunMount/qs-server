package container

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/module"
	authModule "github.com/yshujie/questionnaire-scale/internal/apiserver/module/auth"
	userModule "github.com/yshujie/questionnaire-scale/internal/apiserver/module/user"
)

// modulePool 模块池
var modulePool = make(map[string]module.Module)

// Container 主容器
// 组合所有业务模块和基础设施组件
type Container struct {
	// 基础设施
	mysqlDB     *gorm.DB
	mongoClient *mongo.Client
	mongoDB     string

	// 业务模块
	AuthModule *authModule.Module
	UserModule *userModule.Module

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

	// 初始化认证模块
	if err := c.initAuthModule(); err != nil {
		return fmt.Errorf("failed to initialize auth module: %w", err)
	}

	c.initialized = true
	fmt.Printf("🏗️  Container initialized with modules: user\n")

	return nil
}

// initUserModule 初始化用户模块
func (c *Container) initUserModule() error {
	userModule := userModule.NewModule()
	if err := userModule.Initialize(c.mysqlDB); err != nil {
		return fmt.Errorf("failed to initialize user module: %w", err)
	}

	c.UserModule = userModule
	modulePool["user"] = userModule

	fmt.Printf("📦 User module initialized\n")
	return nil
}

// initAuthModule 初始化认证模块
func (c *Container) initAuthModule() error {
	authModule := authModule.NewModule()
	if err := authModule.Initialize(c.mysqlDB); err != nil {
		return fmt.Errorf("failed to initialize auth module: %w", err)
	}

	c.AuthModule = authModule
	modulePool["auth"] = authModule

	fmt.Printf("📦 Auth module initialized\n")
	return nil
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
	for _, module := range modulePool {
		if err := module.CheckHealth(); err != nil {
			return fmt.Errorf("module health check failed: %w", err)
		}
	}
	return nil
}

// Cleanup 清理资源
func (c *Container) Cleanup() error {
	fmt.Printf("🧹 Cleaning up container resources...\n")

	for _, module := range modulePool {
		if err := module.Cleanup(); err != nil {
			return fmt.Errorf("failed to cleanup module: %w", err)
		}
		fmt.Printf("   ✅ %s module cleaned up\n", module.ModuleInfo().Name)
	}

	c.initialized = false
	fmt.Printf("🏁 Container cleanup completed\n")

	return nil
}

// GetContainerInfo 获取容器信息
func (c *Container) GetContainerInfo() map[string]interface{} {
	modules := make(map[string]interface{})
	for _, module := range modulePool {
		modules[module.ModuleInfo().Name] = module.ModuleInfo()
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

	for _, module := range modulePool {
		modules = append(modules, module.ModuleInfo().Name)
	}

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
