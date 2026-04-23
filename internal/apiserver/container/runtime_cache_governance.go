package container

import (
	"context"
	"fmt"
)

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
