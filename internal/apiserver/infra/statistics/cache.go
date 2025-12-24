package statistics

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	redis "github.com/redis/go-redis/v9"
)

// StatisticsCache 统计缓存（Redis操作封装）
type StatisticsCache struct {
	client redis.UniversalClient
}

// NewStatisticsCache 创建统计缓存
func NewStatisticsCache(client redis.UniversalClient) *StatisticsCache {
	return &StatisticsCache{
		client: client,
	}
}

// ==================== 每日统计 ====================

// IncrementDailyCount 递增每日计数
func (c *StatisticsCache) IncrementDailyCount(
	ctx context.Context,
	orgID int64,
	statType statistics.StatisticType,
	statKey string,
	date time.Time,
	metric string, // "submission" or "completion"
) error {
	key := fmt.Sprintf("stats:daily:%d:%s:%s:%s", orgID, statType, statKey, date.Format("2006-01-02"))
	field := fmt.Sprintf("%s_count", metric)
	return c.client.HIncrBy(ctx, key, field, 1).Err()
}

// GetDailyCount 获取每日计数
func (c *StatisticsCache) GetDailyCount(
	ctx context.Context,
	orgID int64,
	statType statistics.StatisticType,
	statKey string,
	date time.Time,
) (submissionCount, completionCount int64, err error) {
	key := fmt.Sprintf("stats:daily:%d:%s:%s:%s", orgID, statType, statKey, date.Format("2006-01-02"))

	values, err := c.client.HMGet(ctx, key, "submission_count", "completion_count").Result()
	if err != nil {
		return 0, 0, err
	}

	if values[0] != nil {
		if count, ok := values[0].(string); ok {
			parsed, _ := strconv.ParseInt(count, 10, 64)
			submissionCount = parsed
		}
	}
	if values[1] != nil {
		if count, ok := values[1].(string); ok {
			parsed, _ := strconv.ParseInt(count, 10, 64)
			completionCount = parsed
		}
	}

	return submissionCount, completionCount, nil
}

// ==================== 滑动窗口统计 ====================

// IncrementWindowCount 递增滑动窗口计数
func (c *StatisticsCache) IncrementWindowCount(
	ctx context.Context,
	orgID int64,
	statType statistics.StatisticType,
	statKey string,
	window string, // "last7d", "last15d", "last30d"
) error {
	key := fmt.Sprintf("stats:window:%d:%s:%s:%s", orgID, statType, statKey, window)
	return c.client.Incr(ctx, key).Err()
}

// GetWindowCount 获取滑动窗口计数
func (c *StatisticsCache) GetWindowCount(
	ctx context.Context,
	orgID int64,
	statType statistics.StatisticType,
	statKey string,
	window string,
) (int64, error) {
	key := fmt.Sprintf("stats:window:%d:%s:%s:%s", orgID, statType, statKey, window)
	result := c.client.Get(ctx, key)
	if result.Err() == redis.Nil {
		return 0, nil
	}
	return result.Int64()
}

// ==================== 累计统计 ====================

// IncrementAccumCount 递增累计计数
func (c *StatisticsCache) IncrementAccumCount(
	ctx context.Context,
	orgID int64,
	statType statistics.StatisticType,
	statKey string,
	metric string, // "total_submissions", "total_completions"
) error {
	key := fmt.Sprintf("stats:accum:%d:%s:%s:%s", orgID, statType, statKey, metric)
	return c.client.Incr(ctx, key).Err()
}

// GetAccumCount 获取累计计数
func (c *StatisticsCache) GetAccumCount(
	ctx context.Context,
	orgID int64,
	statType statistics.StatisticType,
	statKey string,
	metric string,
) (int64, error) {
	key := fmt.Sprintf("stats:accum:%d:%s:%s:%s", orgID, statType, statKey, metric)
	result := c.client.Get(ctx, key)
	if result.Err() == redis.Nil {
		return 0, nil
	}
	return result.Int64()
}

// ==================== 分布统计 ====================

// IncrementDistribution 递增分布统计
func (c *StatisticsCache) IncrementDistribution(
	ctx context.Context,
	orgID int64,
	statType statistics.StatisticType,
	statKey string,
	dimension string, // "origin", "risk", "status"
	value string, // 分布值
) error {
	key := fmt.Sprintf("stats:dist:%d:%s:%s:%s", orgID, statType, statKey, dimension)
	return c.client.HIncrBy(ctx, key, value, 1).Err()
}

// GetDistribution 获取分布统计
func (c *StatisticsCache) GetDistribution(
	ctx context.Context,
	orgID int64,
	statType statistics.StatisticType,
	statKey string,
	dimension string,
) (map[string]int64, error) {
	key := fmt.Sprintf("stats:dist:%d:%s:%s:%s", orgID, statType, statKey, dimension)
	result := c.client.HGetAll(ctx, key)
	if result.Err() != nil {
		return nil, result.Err()
	}

	values := result.Val()
	distribution := make(map[string]int64)
	for k, v := range values {
		count, _ := strconv.ParseInt(v, 10, 64)
		distribution[k] = count
	}

	return distribution, nil
}

// ==================== 幂等性检查 ====================

// IsEventProcessed 检查事件是否已处理
func (c *StatisticsCache) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	key := fmt.Sprintf("event:processed:%s", eventID)
	result := c.client.Exists(ctx, key)
	if result.Err() != nil {
		return false, result.Err()
	}
	return result.Val() > 0, nil
}

// MarkEventProcessed 标记事件已处理
func (c *StatisticsCache) MarkEventProcessed(ctx context.Context, eventID string, ttl time.Duration) error {
	key := fmt.Sprintf("event:processed:%s", eventID)
	return c.client.Set(ctx, key, "1", ttl).Err()
}

// ==================== 查询结果缓存 ====================

// GetQueryCache 获取查询结果缓存
func (c *StatisticsCache) GetQueryCache(ctx context.Context, cacheKey string) (string, error) {
	key := fmt.Sprintf("stats:query:%s", cacheKey)
	result := c.client.Get(ctx, key)
	if result.Err() == redis.Nil {
		return "", nil
	}
	return result.Val(), result.Err()
}

// SetQueryCache 设置查询结果缓存
func (c *StatisticsCache) SetQueryCache(ctx context.Context, cacheKey string, value string, ttl time.Duration) error {
	key := fmt.Sprintf("stats:query:%s", cacheKey)
	return c.client.Set(ctx, key, value, ttl).Err()
}

// ==================== 扫描所有每日统计键 ====================

// ScanDailyKeys 扫描所有每日统计键
func (c *StatisticsCache) ScanDailyKeys(ctx context.Context, orgID int64, statType statistics.StatisticType) ([]string, error) {
	pattern := fmt.Sprintf("stats:daily:%d:%s:*", orgID, statType)
	var keys []string
	var cursor uint64

	for {
		var err error
		var batch []string
		batch, cursor, err = c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, batch...)
		if cursor == 0 {
			break
		}
	}

	return keys, nil
}
