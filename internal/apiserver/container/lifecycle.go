package container

import (
	"context"
	"fmt"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
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
func (c *Container) Cleanup() error { return c.CleanupContext(context.Background()) }

// CleanupContext owns exactly one real cleanup coordinator. A caller timeout
// does not cancel its drain or release the database/publisher still in use.
func (c *Container) CleanupContext(ctx context.Context) error {
	if c == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.cleanupStarted.Store(true)
	c.cleanupOnce.Do(func() {
		c.cleanupDone = make(chan struct{})
		if c.eventSubsystem != nil {
			c.eventSubsystem.StopAdmission()
		}
		go func() {
			c.cleanupErr = c.cleanupResources(context.Background())
			close(c.cleanupDone)
		}()
	})
	result := func() error { return c.cleanupErr }
	select {
	case <-c.cleanupDone:
		return result()
	default:
	}
	select {
	case <-c.cleanupDone:
		return result()
	case <-ctx.Done():
		select {
		case <-c.cleanupDone:
			return result()
		default:
		}
		return &eventsubsystem.ShutdownIncomplete{PendingClosures: 1, Cause: ctx.Err()}
	}
}

func (c *Container) cleanupModules(ctx context.Context) error {
	for _, module := range c.loadedModules() {
		var err error
		if contextual, ok := module.(interface{ CleanupContext(context.Context) error }); ok && c.transactions != nil {
			err = contextual.CleanupContext(ctx)
		} else {
			err = module.Cleanup()
		}
		if err != nil {
			return fmt.Errorf("failed to cleanup module: %w", err)
		}
		c.printf("   ✅ %s module cleaned up\n", module.ModuleInfo().Name)
	}
	return nil
}

func (c *Container) cleanupResources(ctx context.Context) error {
	c.printf("🧹 Cleaning up container resources...\n")
	if c.resilienceCancel != nil {
		c.resilienceCancel()
		c.resilienceCancel = nil
	}
	if c.actionAuditCancel != nil {
		c.actionAuditCancel()
		c.actionAuditCancel = nil
	}
	// Transaction consumers may still finalize original business writes. Drain
	// them while owner Checkers and the original databases remain alive.
	transactionMode := c.transactions != nil
	if transactionMode {
		if err := c.cleanupModules(ctx); err != nil {
			return err
		}
	}
	if c.eventSubsystem != nil {
		if err := c.eventSubsystem.CloseContext(ctx); err != nil {
			return fmt.Errorf("failed to close event subsystem: %w", err)
		}
	}
	if c.cache != nil {
		if err := c.cache.Close(); err != nil {
			return fmt.Errorf("failed to close cache subsystem: %w", err)
		}
	}

	// 清理 IAM 模块
	if c.IAMModule != nil {
		if err := c.IAMModule.Close(); err != nil {
			return fmt.Errorf("failed to cleanup IAM module: %w", err)
		}
		c.printf("   ✅ IAM module cleaned up\n")
	}

	if !transactionMode {
		if err := c.cleanupModules(ctx); err != nil {
			return err
		}
	}

	c.initialized = false
	c.printf("🏁 Container cleanup completed\n")

	return nil
}

func (c *Container) StartEventSubsystem(ctx context.Context) error {
	if c == nil || c.eventSubsystem == nil {
		return nil
	}
	if c.cleanupStarted.Load() {
		return fmt.Errorf("container shutdown already requested")
	}
	return c.eventSubsystem.Start(ctx)
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

// WarmupCache 预热缓存（异步执行，不阻塞）
func (c *Container) WarmupCache(ctx context.Context) error {
	if !c.initialized {
		return fmt.Errorf("container not initialized")
	}
	if coordinator := c.WarmupCoordinator(); coordinator != nil {
		if err := coordinator.WarmStartup(ctx); err != nil {
			return fmt.Errorf("cache governance startup warmup failed: %w", err)
		}
		return nil
	}
	return fmt.Errorf("cache governance warmup coordinator is unavailable")
}

// StopEventAdmission is fast and leaves Checkers running through host drain.
func (c *Container) StopEventAdmission() {
	if c != nil && c.eventSubsystem != nil {
		c.eventSubsystem.StopAdmission()
	}
}
