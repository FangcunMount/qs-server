package statistics

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	redis "github.com/redis/go-redis/v9"
)

const (
	// 统计类键生命周期
	DefaultDailyStatsTTL     = 90 * 24 * time.Hour
	eventProcessedWindowDays = 7
	eventProcessedBucketTTL  = 8 * 24 * time.Hour
)

var tryMarkEventProcessedScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
	return 0
end

for i = 2, #KEYS do
	if redis.call("SISMEMBER", KEYS[i], ARGV[1]) == 1 then
		return 0
	end
end

local added = redis.call("SADD", KEYS[2], ARGV[1])
if added == 1 then
	redis.call("EXPIRE", KEYS[2], tonumber(ARGV[2]))
	return 1
end

return 0
`)

// normalizeDate 将日期统一到本地时区的 00:00:00，避免跨日边界差异
func normalizeDate(t time.Time) time.Time {
	d := t.In(time.Local)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
}

// StatisticsCache 统计缓存（Redis操作封装）
type StatisticsCache struct {
	client redis.UniversalClient
	keys   *rediskey.Builder
}

// NewStatisticsCache 创建统计缓存
func NewStatisticsCache(client redis.UniversalClient) *StatisticsCache {
	return &StatisticsCache{
		client: client,
		keys:   rediskey.NewBuilder(),
	}
}

func (c *StatisticsCache) TryMarkEventProcessed(ctx context.Context, eventID string, now time.Time) (bool, error) {
	result, err := tryMarkEventProcessedScript.Run(
		ctx,
		c.client,
		c.eventProcessedKeys(eventID, now),
		eventID,
		int(eventProcessedBucketTTL/time.Second),
	).Int()
	if err != nil {
		return false, fmt.Errorf("try mark event processed: %w", err)
	}
	return result == 1, nil
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
	date = normalizeDate(date)
	key := c.keys.BuildStatsDailyKey(orgID, string(statType), statKey, date.Format("2006-01-02"))
	field := fmt.Sprintf("%s_count", metric)
	if err := c.client.HIncrBy(ctx, key, field, 1).Err(); err != nil {
		return err
	}
	return c.ensureTTL(ctx, key, DefaultDailyStatsTTL)
}

// GetDailyCount 获取每日计数
func (c *StatisticsCache) GetDailyCount(
	ctx context.Context,
	orgID int64,
	statType statistics.StatisticType,
	statKey string,
	date time.Time,
) (submissionCount, completionCount int64, err error) {
	date = normalizeDate(date)
	key := c.keys.BuildStatsDailyKey(orgID, string(statType), statKey, date.Format("2006-01-02"))

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

// ==================== 幂等性检查 ====================

// IsEventProcessed 检查事件是否已处理
func (c *StatisticsCache) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	key := c.keys.BuildEventProcessedKey(eventID)
	result := c.client.Exists(ctx, key)
	if result.Err() != nil {
		return false, result.Err()
	}
	return result.Val() > 0, nil
}

// MarkEventProcessed 标记事件已处理
func (c *StatisticsCache) MarkEventProcessed(ctx context.Context, eventID string, ttl time.Duration) error {
	key := c.keys.BuildEventProcessedKey(eventID)
	return c.client.Set(ctx, key, "1", ttl).Err()
}

// ==================== 查询结果缓存 ====================

// GetQueryCache 获取查询结果缓存
func (c *StatisticsCache) GetQueryCache(ctx context.Context, cacheKey string) (string, error) {
	key := c.keys.BuildStatsQueryKey(cacheKey)
	result := c.client.Get(ctx, key)
	if result.Err() == redis.Nil {
		return "", nil
	}
	return result.Val(), result.Err()
}

// SetQueryCache 设置查询结果缓存
func (c *StatisticsCache) SetQueryCache(ctx context.Context, cacheKey string, value string, ttl time.Duration) error {
	key := c.keys.BuildStatsQueryKey(cacheKey)
	return c.client.Set(ctx, key, value, ttl).Err()
}

// ==================== 扫描所有每日统计键 ====================

// ScanDailyKeys 扫描所有每日统计键
func (c *StatisticsCache) ScanDailyKeys(ctx context.Context, orgID int64, statType statistics.StatisticType) ([]string, error) {
	pattern := c.keys.BuildStatsDailyPattern(orgID, string(statType))
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

// ensureTTL 统一设置 TTL，避免历史键无限增长
func (c *StatisticsCache) ensureTTL(ctx context.Context, key string, ttl time.Duration) error {
	if c.client == nil || ttl <= 0 {
		return nil
	}
	// 直接刷新 TTL（滑动窗口），不区分新老键，确保历史无 TTL 键被覆盖
	return c.client.Expire(ctx, key, ttl).Err()
}

func (c *StatisticsCache) eventProcessedKeys(eventID string, now time.Time) []string {
	keys := make([]string, 0, 1+eventProcessedWindowDays)
	keys = append(keys, c.keys.BuildEventProcessedKey(eventID))
	for dayOffset := 0; dayOffset < eventProcessedWindowDays; dayOffset++ {
		dateKey := normalizeDate(now).AddDate(0, 0, -dayOffset).Format("2006-01-02")
		keys = append(keys, c.keys.BuildEventProcessedBucketKey(dateKey))
	}
	return keys
}
