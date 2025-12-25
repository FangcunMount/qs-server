package cache

import (
	"context"
	"fmt"
)

// CacheManagerImpl 缓存管理器实现
type CacheManagerImpl struct {
	cache   Cache
	metrics *MetricsCollector
}

// NewCacheManager 创建缓存管理器
func NewCacheManager(cache Cache) CacheManager {
	metrics := NewMetricsCollector()
	if metricsCache, ok := cache.(*MetricsCache); ok {
		metrics = metricsCache.metrics
	}

	return &CacheManagerImpl{
		cache:   cache,
		metrics: metrics,
	}
}

// GetStats 获取缓存统计信息
func (m *CacheManagerImpl) GetStats(ctx context.Context) (*CacheMetrics, error) {
	return m.metrics.GetMetrics(ctx, m.cache)
}

// ClearPattern 清空指定模式的缓存
func (m *CacheManagerImpl) ClearPattern(ctx context.Context, pattern string) (int, error) {
	// 使用 DeletePattern 删除
	if err := m.cache.DeletePattern(ctx, pattern); err != nil {
		return 0, fmt.Errorf("failed to clear pattern: %w", err)
	}

	// 注意：DeletePattern 不返回删除数量，这里返回 0
	// 如果需要精确计数，需要先 Scan 再 Delete
	return 0, nil
}

// Warmup 预热指定键的缓存
func (m *CacheManagerImpl) Warmup(ctx context.Context, keys []string) error {
	// 这是一个通用接口，具体预热逻辑由调用方实现
	// 这里只做接口占位
	return fmt.Errorf("warmup not implemented for generic cache manager")
}

// HealthCheck 健康检查
func (m *CacheManagerImpl) HealthCheck(ctx context.Context) error {
	return m.cache.Ping(ctx)
}
