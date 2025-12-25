package cache

import (
	"context"
	"fmt"
	"time"
)

// Cache 统一缓存接口
// 提供基础的缓存操作能力
type Cache interface {
	// Get 获取缓存值
	Get(ctx context.Context, key string) ([]byte, error)

	// Set 设置缓存值
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete 删除缓存
	Delete(ctx context.Context, key string) error

	// Exists 检查键是否存在
	Exists(ctx context.Context, key string) (bool, error)

	// MGet 批量获取
	MGet(ctx context.Context, keys []string) (map[string][]byte, error)

	// MSet 批量设置
	MSet(ctx context.Context, items map[string][]byte, ttl time.Duration) error

	// DeletePattern 按模式删除（谨慎使用，性能较低）
	DeletePattern(ctx context.Context, pattern string) error

	// Ping 健康检查
	Ping(ctx context.Context) error
}

// TypedCache 类型化缓存接口
// 提供类型安全的缓存操作，自动处理序列化/反序列化
type TypedCache[T any] interface {
	// Get 获取缓存值
	Get(ctx context.Context, key string) (*T, error)

	// Set 设置缓存值
	Set(ctx context.Context, key string, value *T, ttl time.Duration) error

	// Delete 删除缓存
	Delete(ctx context.Context, key string) error

	// Exists 检查键是否存在
	Exists(ctx context.Context, key string) (bool, error)
}

// CacheMetrics 缓存指标
type CacheMetrics struct {
	HitRate     float64 `json:"hit_rate"`      // 命中率（0-1）
	MissRate    float64 `json:"miss_rate"`     // 未命中率（0-1）
	AvgLatency  float64 `json:"avg_latency"`   // 平均延迟（ms）
	ErrorRate   float64 `json:"error_rate"`    // 错误率（0-1）
	MemoryUsage int64   `json:"memory_usage"`  // 内存使用（bytes）
	KeyCount    int64   `json:"key_count"`     // 键数量
}

// CacheManager 缓存管理器
// 提供缓存管理和监控能力
type CacheManager interface {
	// GetStats 获取缓存统计信息
	GetStats(ctx context.Context) (*CacheMetrics, error)

	// ClearPattern 清空指定模式的缓存
	ClearPattern(ctx context.Context, pattern string) (int, error)

	// Warmup 预热指定键的缓存
	Warmup(ctx context.Context, keys []string) error

	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) error
}

// CacheKeyBuilder 缓存键构建器
// 统一管理缓存键的构建规则
type CacheKeyBuilder struct{}

// NewCacheKeyBuilder 创建缓存键构建器
func NewCacheKeyBuilder() *CacheKeyBuilder {
	return &CacheKeyBuilder{}
}

// BuildScaleKey 构建量表缓存键
func (b *CacheKeyBuilder) BuildScaleKey(code string) string {
	return "scale:" + code
}

// BuildQuestionnaireKey 构建问卷缓存键
func (b *CacheKeyBuilder) BuildQuestionnaireKey(code, version string) string {
	return "questionnaire:" + code + ":" + version
}

// BuildAssessmentStatusKey 构建测评状态缓存键
func (b *CacheKeyBuilder) BuildAssessmentStatusKey(id uint64) string {
	return fmt.Sprintf("assessment:status:%d", id)
}

// BuildAssessmentDetailKey 构建测评详情缓存键
func (b *CacheKeyBuilder) BuildAssessmentDetailKey(id uint64) string {
	return fmt.Sprintf("assessment:detail:%d", id)
}

// BuildTesteeInfoKey 构建受试者信息缓存键
func (b *CacheKeyBuilder) BuildTesteeInfoKey(id uint64) string {
	return fmt.Sprintf("testee:info:%d", id)
}

// BuildPlanInfoKey 构建计划信息缓存键
func (b *CacheKeyBuilder) BuildPlanInfoKey(id uint64) string {
	return fmt.Sprintf("plan:info:%d", id)
}

// BuildStatsQueryKey 构建统计查询缓存键
func (b *CacheKeyBuilder) BuildStatsQueryKey(statType, key string) string {
	return "stats:query:" + statType + ":" + key
}

// BuildEventProcessedKey 构建事件幂等性缓存键
func (b *CacheKeyBuilder) BuildEventProcessedKey(eventID string) string {
	return "event:processed:" + eventID
}

