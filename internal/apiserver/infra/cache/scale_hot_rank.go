package cache

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	redis "github.com/redis/go-redis/v9"
)

const defaultScaleHotRankRetention = 400 * 24 * time.Hour

var projectScaleHotSubmissionScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
	return 0
end
redis.call("ZINCRBY", KEYS[2], 1, ARGV[2])
redis.call("EXPIRE", KEYS[2], tonumber(ARGV[1]))
redis.call("SET", KEYS[1], "1", "EX", tonumber(ARGV[1]))
return 1
`)

// RedisScaleHotRankProjection 用 Redis ZSET 维护量表热度榜读模型。
// 计数按提交日期分桶，读取时合并最近 N 天，支持首页的滚动窗口热度。
type RedisScaleHotRankProjection struct {
	client    redis.UniversalClient
	keys      *keyspace.Builder
	retention time.Duration
	now       func() time.Time
}

var _ domainScale.ScaleHotRankProjection = (*RedisScaleHotRankProjection)(nil)
var _ domainScale.ScaleHotRankReadModel = (*RedisScaleHotRankProjection)(nil)

func NewRedisScaleHotRankProjection(client redis.UniversalClient, builder *keyspace.Builder) *RedisScaleHotRankProjection {
	if client == nil || builder == nil {
		return nil
	}
	return &RedisScaleHotRankProjection{
		client:    client,
		keys:      builder,
		retention: defaultScaleHotRankRetention,
		now:       time.Now,
	}
}

func (r *RedisScaleHotRankProjection) ProjectSubmission(ctx context.Context, fact domainScale.ScaleHotRankSubmissionFact) error {
	if r == nil || r.client == nil || r.keys == nil {
		return nil
	}
	eventID := strings.TrimSpace(fact.EventID)
	questionnaireCode := strings.TrimSpace(fact.QuestionnaireCode)
	if questionnaireCode == "" {
		return nil
	}
	if eventID == "" {
		return fmt.Errorf("scale hot rank projection event id is empty")
	}
	submittedAt := fact.SubmittedAt
	if submittedAt.IsZero() {
		submittedAt = r.now()
	}

	ttlSeconds := int64(r.retention.Seconds())
	if ttlSeconds <= 0 {
		ttlSeconds = int64(defaultScaleHotRankRetention.Seconds())
	}
	processedKey := r.keys.BuildScaleHotProjectedKey(eventID)
	dailyKey := r.keys.BuildScaleHotDailyKey(submittedAt.Local().Format("20060102"))
	return projectScaleHotSubmissionScript.Run(ctx, r.client, []string{processedKey, dailyKey}, ttlSeconds, questionnaireCode).Err()
}

func (r *RedisScaleHotRankProjection) Top(ctx context.Context, query domainScale.ScaleHotRankQuery) ([]domainScale.ScaleHotRankEntry, error) {
	windowDays := query.WindowDays
	limit := query.Limit
	if r == nil || r.client == nil || r.keys == nil || limit <= 0 || windowDays <= 0 {
		return []domainScale.ScaleHotRankEntry{}, nil
	}

	keys := r.windowKeys(windowDays)
	if len(keys) == 0 {
		return []domainScale.ScaleHotRankEntry{}, nil
	}
	if len(keys) == 1 {
		values, err := r.client.ZRevRangeWithScores(ctx, keys[0], 0, int64(limit-1)).Result()
		if err != nil {
			return nil, err
		}
		return hotRankItemsFromZ(values), nil
	}

	token := fmt.Sprintf("%s:%d", r.now().Local().Format("20060102"), windowDays)
	dest := r.keys.BuildScaleHotWindowKey(token)
	if err := r.client.ZUnionStore(ctx, dest, &redis.ZStore{
		Keys:      keys,
		Aggregate: "SUM",
	}).Err(); err != nil {
		return nil, err
	}
	_ = r.client.Expire(ctx, dest, time.Minute).Err()

	values, err := r.client.ZRevRangeWithScores(ctx, dest, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}
	return hotRankItemsFromZ(values), nil
}

func (r *RedisScaleHotRankProjection) windowKeys(windowDays int) []string {
	now := r.now().Local()
	keys := make([]string, 0, windowDays)
	for i := 0; i < windowDays; i++ {
		day := now.AddDate(0, 0, -i).Format("20060102")
		keys = append(keys, r.keys.BuildScaleHotDailyKey(day))
	}
	return keys
}

func hotRankItemsFromZ(values []redis.Z) []domainScale.ScaleHotRankEntry {
	items := make([]domainScale.ScaleHotRankEntry, 0, len(values))
	for _, value := range values {
		member, ok := value.Member.(string)
		if !ok || strings.TrimSpace(member) == "" {
			continue
		}
		items = append(items, domainScale.ScaleHotRankEntry{
			QuestionnaireCode: member,
			Score:             int64(math.Round(value.Score)),
		})
	}
	return items
}
