package cache

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
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
	HitRate     float64 `json:"hit_rate"`     // 命中率（0-1）
	MissRate    float64 `json:"miss_rate"`    // 未命中率（0-1）
	AvgLatency  float64 `json:"avg_latency"`  // 平均延迟（ms）
	ErrorRate   float64 `json:"error_rate"`   // 错误率（0-1）
	MemoryUsage int64   `json:"memory_usage"` // 内存使用（bytes）
	KeyCount    int64   `json:"key_count"`    // 键数量
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
type CacheKeyBuilder struct {
	builder *rediskey.Builder
}

// NewCacheKeyBuilderWithNamespace 创建绑定显式 namespace 的缓存键构建器。
func NewCacheKeyBuilderWithNamespace(namespace string) *CacheKeyBuilder {
	return &CacheKeyBuilder{builder: rediskey.NewBuilderWithNamespace(namespace)}
}

func (b *CacheKeyBuilder) redisBuilder() *rediskey.Builder {
	if b == nil || b.builder == nil {
		panic("cache key builder is required")
	}
	return b.builder
}

// BuildScaleKey 构建量表缓存键
func (b *CacheKeyBuilder) BuildScaleKey(code string) string {
	return b.redisBuilder().BuildScaleKey(code)
}

// BuildScaleListKey 构建量表全局列表缓存键
func (b *CacheKeyBuilder) BuildScaleListKey() string {
	return b.redisBuilder().BuildScaleListKey()
}

// BuildQuestionnaireKey 构建问卷缓存键
func (b *CacheKeyBuilder) BuildQuestionnaireKey(code, version string) string {
	return b.redisBuilder().BuildQuestionnaireKey(code, version)
}

// BuildPublishedQuestionnaireKey 构建当前已发布问卷缓存键
func (b *CacheKeyBuilder) BuildPublishedQuestionnaireKey(code string) string {
	return b.redisBuilder().BuildPublishedQuestionnaireKey(code)
}

// BuildAssessmentDetailKey 构建测评详情缓存键
func (b *CacheKeyBuilder) BuildAssessmentDetailKey(id uint64) string {
	return b.redisBuilder().BuildAssessmentDetailKey(id)
}

// BuildAssessmentListKey 构建“我的测评列表”缓存键
// suffix 可用于携带筛选条件哈希，格式示例：":abc123"
func (b *CacheKeyBuilder) BuildAssessmentListKey(userID uint64, suffix string) string {
	return b.redisBuilder().BuildAssessmentListKey(userID, suffix)
}

// BuildQueryVersionKey 构建 query version token 键。
func (b *CacheKeyBuilder) BuildQueryVersionKey(kind, scope string) string {
	return b.redisBuilder().BuildQueryVersionKey(kind, scope)
}

// BuildVersionedQueryKey 构建带 version token 的 query 结果键。
func (b *CacheKeyBuilder) BuildVersionedQueryKey(kind, scope string, version uint64, hash string) string {
	return b.redisBuilder().BuildVersionedQueryKey(kind, scope, version, hash)
}

// BuildAssessmentListVersionKey 构建“我的测评列表”version token 键。
func (b *CacheKeyBuilder) BuildAssessmentListVersionKey(userID uint64) string {
	return b.redisBuilder().BuildAssessmentListVersionKey(userID)
}

// BuildAssessmentListVersionedKey 构建带 version token 的“我的测评列表”结果键。
func (b *CacheKeyBuilder) BuildAssessmentListVersionedKey(userID, version uint64, hash string) string {
	return b.redisBuilder().BuildAssessmentListVersionedKey(userID, version, hash)
}

// BuildTesteeInfoKey 构建受试者信息缓存键
func (b *CacheKeyBuilder) BuildTesteeInfoKey(id uint64) string {
	return b.redisBuilder().BuildTesteeInfoKey(id)
}

// BuildPlanInfoKey 构建计划信息缓存键
func (b *CacheKeyBuilder) BuildPlanInfoKey(id uint64) string {
	return b.redisBuilder().BuildPlanInfoKey(id)
}

// BuildStatsQueryKey 构建统计查询缓存键
func (b *CacheKeyBuilder) BuildStatsQueryKey(statType, key string) string {
	return b.redisBuilder().BuildStatsQueryKey(statType + ":" + key)
}
