package container

import (
	"context"
	"fmt"

	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
)

// HealthCheck 健康检查
func (c *Container) HealthCheck(ctx context.Context) error {
	// 检查 IAM 连接
	if c.IAMModule != nil && c.IAMModule.IsEnabled() {
		if err := c.IAMModule.HealthCheck(ctx); err != nil {
			return fmt.Errorf("IAM health check failed: %w", err)
		}
	}

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
	if c.mongoDB != nil {
		if err := c.mongoDB.Client().Ping(ctx, nil); err != nil {
			return fmt.Errorf("mongodb ping failed: %w", err)
		}
	}

	// 检查 Redis 连接
	if c.redisCache != nil {
		if err := c.redisCache.Ping(ctx).Err(); err != nil {
			return fmt.Errorf("redis cache ping failed: %w", err)
		}
	}

	// 检查模块健康状态
	if err := c.checkModulesHealth(ctx); err != nil {
		return fmt.Errorf("modules health check failed: %w", err)
	}

	return nil
}

// checkModulesHealth 检查模块健康状态
func (c *Container) checkModulesHealth(_ context.Context) error {
	for _, module := range c.loadedModules() {
		if err := module.CheckHealth(); err != nil {
			return fmt.Errorf("module health check failed: %w", err)
		}
	}
	return nil
}

// Cleanup 清理资源
func (c *Container) Cleanup() error {
	c.printf("🧹 Cleaning up container resources...\n")

	// 清理 IAM 模块
	if c.IAMModule != nil {
		if err := c.IAMModule.Close(); err != nil {
			return fmt.Errorf("failed to cleanup IAM module: %w", err)
		}
		c.printf("   ✅ IAM module cleaned up\n")
	}

	for _, module := range c.loadedModules() {
		if err := module.Cleanup(); err != nil {
			return fmt.Errorf("failed to cleanup module: %w", err)
		}
		c.printf("   ✅ %s module cleaned up\n", module.ModuleInfo().Name)
	}

	c.initialized = false
	c.printf("🏁 Container cleanup completed\n")

	return nil
}

// GetContainerInfo 获取容器信息
func (c *Container) GetContainerInfo() map[string]interface{} {
	modules := make(map[string]interface{})
	for _, module := range c.loadedModules() {
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
			"mongodb": c.mongoDB != nil,
			"redis":   c.redisCache != nil,
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

	for _, module := range c.loadedModules() {
		modules = append(modules, module.ModuleInfo().Name)
	}

	return modules
}

func (c *Container) HotsetInspector() scaleCache.HotsetInspector {
	if c == nil {
		return nil
	}
	return c.hotsetInspector
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
