package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsCollector 缓存指标收集器
// 用于收集缓存操作的统计信息
type MetricsCollector struct {
	mu sync.RWMutex

	// 操作计数
	hits   int64 // 命中次数
	misses int64 // 未命中次数
	errors int64 // 错误次数

	// 延迟统计
	totalLatency   int64 // 总延迟（纳秒）
	operationCount int64 // 操作总数

	// 内存使用（需要从 Redis 获取）
	memoryUsage int64
	keyCount    int64
}

// NewMetricsCollector 创建指标收集器
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{}
}

// RecordHit 记录命中
func (m *MetricsCollector) RecordHit(latency time.Duration) {
	atomic.AddInt64(&m.hits, 1)
	atomic.AddInt64(&m.totalLatency, int64(latency))
	atomic.AddInt64(&m.operationCount, 1)
}

// RecordMiss 记录未命中
func (m *MetricsCollector) RecordMiss(latency time.Duration) {
	atomic.AddInt64(&m.misses, 1)
	atomic.AddInt64(&m.totalLatency, int64(latency))
	atomic.AddInt64(&m.operationCount, 1)
}

// RecordError 记录错误
func (m *MetricsCollector) RecordError(latency time.Duration) {
	atomic.AddInt64(&m.errors, 1)
	atomic.AddInt64(&m.totalLatency, int64(latency))
	atomic.AddInt64(&m.operationCount, 1)
}

// RecordOperation 记录操作（用于 Set/Delete 等）
func (m *MetricsCollector) RecordOperation(latency time.Duration, err error) {
	atomic.AddInt64(&m.totalLatency, int64(latency))
	atomic.AddInt64(&m.operationCount, 1)
	if err != nil {
		atomic.AddInt64(&m.errors, 1)
	}
}

// GetMetrics 获取当前指标
func (m *MetricsCollector) GetMetrics(ctx context.Context, cache Cache) (*CacheMetrics, error) {
	hits := atomic.LoadInt64(&m.hits)
	misses := atomic.LoadInt64(&m.misses)
	errors := atomic.LoadInt64(&m.errors)
	totalLatency := atomic.LoadInt64(&m.totalLatency)
	operationCount := atomic.LoadInt64(&m.operationCount)

	// 计算命中率
	totalRequests := hits + misses
	var hitRate, missRate, errorRate float64
	var avgLatency float64

	if totalRequests > 0 {
		hitRate = float64(hits) / float64(totalRequests)
		missRate = float64(misses) / float64(totalRequests)
	}

	if operationCount > 0 {
		avgLatency = float64(totalLatency) / float64(operationCount) / 1e6 // 转换为毫秒
		errorRate = float64(errors) / float64(operationCount)
	}

	// 从 Redis 获取内存使用和键数量（如果支持）
	var memoryUsage, keyCount int64
	if redisCache, ok := cache.(*RedisCache); ok {
		// 尝试获取 Redis 信息（需要实现）
		// 这里先返回 0，后续可以通过 INFO 命令获取
		_ = redisCache
	}

	return &CacheMetrics{
		HitRate:     hitRate,
		MissRate:    missRate,
		AvgLatency:  avgLatency,
		ErrorRate:   errorRate,
		MemoryUsage: memoryUsage,
		KeyCount:    keyCount,
	}, nil
}

// Reset 重置指标
func (m *MetricsCollector) Reset() {
	atomic.StoreInt64(&m.hits, 0)
	atomic.StoreInt64(&m.misses, 0)
	atomic.StoreInt64(&m.errors, 0)
	atomic.StoreInt64(&m.totalLatency, 0)
	atomic.StoreInt64(&m.operationCount, 0)
}

// MetricsCache 带指标收集的缓存装饰器
type MetricsCache struct {
	cache   Cache
	metrics *MetricsCollector
}

// NewMetricsCache 创建带指标收集的缓存
func NewMetricsCache(cache Cache) *MetricsCache {
	return &MetricsCache{
		cache:   cache,
		metrics: NewMetricsCollector(),
	}
}

// Get 获取缓存值（带指标收集）
func (c *MetricsCache) Get(ctx context.Context, key string) ([]byte, error) {
	start := time.Now()
	data, err := c.cache.Get(ctx, key)
	latency := time.Since(start)

	if err == ErrCacheNotFound {
		c.metrics.RecordMiss(latency)
		return nil, err
	}
	if err != nil {
		c.metrics.RecordError(latency)
		return nil, err
	}

	c.metrics.RecordHit(latency)
	return data, nil
}

// Set 设置缓存值（带指标收集）
func (c *MetricsCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	start := time.Now()
	err := c.cache.Set(ctx, key, value, ttl)
	latency := time.Since(start)

	c.metrics.RecordOperation(latency, err)
	return err
}

// Delete 删除缓存（带指标收集）
func (c *MetricsCache) Delete(ctx context.Context, key string) error {
	start := time.Now()
	err := c.cache.Delete(ctx, key)
	latency := time.Since(start)

	c.metrics.RecordOperation(latency, err)
	return err
}

// Exists 检查键是否存在（带指标收集）
func (c *MetricsCache) Exists(ctx context.Context, key string) (bool, error) {
	start := time.Now()
	exists, err := c.cache.Exists(ctx, key)
	latency := time.Since(start)

	c.metrics.RecordOperation(latency, err)
	return exists, err
}

// MGet 批量获取（带指标收集）
func (c *MetricsCache) MGet(ctx context.Context, keys []string) (map[string][]byte, error) {
	start := time.Now()
	result, err := c.cache.MGet(ctx, keys)
	latency := time.Since(start)

	c.metrics.RecordOperation(latency, err)
	return result, err
}

// MSet 批量设置（带指标收集）
func (c *MetricsCache) MSet(ctx context.Context, items map[string][]byte, ttl time.Duration) error {
	start := time.Now()
	err := c.cache.MSet(ctx, items, ttl)
	latency := time.Since(start)

	c.metrics.RecordOperation(latency, err)
	return err
}

// DeletePattern 按模式删除（带指标收集）
func (c *MetricsCache) DeletePattern(ctx context.Context, pattern string) error {
	start := time.Now()
	err := c.cache.DeletePattern(ctx, pattern)
	latency := time.Since(start)

	c.metrics.RecordOperation(latency, err)
	return err
}

// Ping 健康检查（带指标收集）
func (c *MetricsCache) Ping(ctx context.Context) error {
	start := time.Now()
	err := c.cache.Ping(ctx)
	latency := time.Since(start)

	c.metrics.RecordOperation(latency, err)
	return err
}

// GetMetrics 获取指标
func (c *MetricsCache) GetMetrics(ctx context.Context) (*CacheMetrics, error) {
	return c.metrics.GetMetrics(ctx, c.cache)
}
