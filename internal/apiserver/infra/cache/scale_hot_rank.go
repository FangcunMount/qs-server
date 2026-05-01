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

// RedisScaleHotRank 用 Redis ZSET 维护量表热度榜。
// 计数按提交日期分桶，读取时合并最近 N 天，支持首页的滚动窗口热度。
type RedisScaleHotRank struct {
	client    redis.UniversalClient
	keys      *keyspace.Builder
	retention time.Duration
	now       func() time.Time
}

var _ domainScale.HotRankRecorder = (*RedisScaleHotRank)(nil)
var _ domainScale.HotRankReader = (*RedisScaleHotRank)(nil)

func NewRedisScaleHotRank(client redis.UniversalClient, builder *keyspace.Builder) *RedisScaleHotRank {
	if client == nil || builder == nil {
		return nil
	}
	return &RedisScaleHotRank{
		client:    client,
		keys:      builder,
		retention: defaultScaleHotRankRetention,
		now:       time.Now,
	}
}

func (r *RedisScaleHotRank) RecordSubmission(ctx context.Context, questionnaireCode string, submittedAt time.Time) error {
	if r == nil || r.client == nil || r.keys == nil {
		return nil
	}
	questionnaireCode = strings.TrimSpace(questionnaireCode)
	if questionnaireCode == "" {
		return nil
	}
	if submittedAt.IsZero() {
		submittedAt = r.now()
	}

	key := r.keys.BuildScaleHotDailyKey(submittedAt.Local().Format("20060102"))
	pipe := r.client.Pipeline()
	pipe.ZIncrBy(ctx, key, 1, questionnaireCode)
	pipe.Expire(ctx, key, r.retention)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *RedisScaleHotRank) TopSubmissions(ctx context.Context, windowDays, limit int) ([]domainScale.HotRankItem, error) {
	if r == nil || r.client == nil || r.keys == nil || limit <= 0 || windowDays <= 0 {
		return []domainScale.HotRankItem{}, nil
	}

	keys := r.windowKeys(windowDays)
	if len(keys) == 0 {
		return []domainScale.HotRankItem{}, nil
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

func (r *RedisScaleHotRank) windowKeys(windowDays int) []string {
	now := r.now().Local()
	keys := make([]string, 0, windowDays)
	for i := 0; i < windowDays; i++ {
		day := now.AddDate(0, 0, -i).Format("20060102")
		keys = append(keys, r.keys.BuildScaleHotDailyKey(day))
	}
	return keys
}

func hotRankItemsFromZ(values []redis.Z) []domainScale.HotRankItem {
	items := make([]domainScale.HotRankItem, 0, len(values))
	for _, value := range values {
		member, ok := value.Member.(string)
		if !ok || strings.TrimSpace(member) == "" {
			continue
		}
		items = append(items, domainScale.HotRankItem{
			QuestionnaireCode: member,
			Score:             int64(math.Round(value.Score)),
		})
	}
	return items
}
