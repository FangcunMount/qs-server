package statistics

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	redis "github.com/redis/go-redis/v9"
)

// GenerationPublisher atomically invalidates every Statistics query key for one
// organization after result data has committed. Query keys include the value
// returned by Generation, so publication never needs wildcard deletion.
type GenerationPublisher struct {
	client     redis.UniversalClient
	keyBuilder *keyspace.Builder
}

func NewGenerationPublisher(client redis.UniversalClient, keyBuilder *keyspace.Builder) *GenerationPublisher {
	if client != nil && keyBuilder == nil {
		panic("statistics query key builder is required")
	}
	return &GenerationPublisher{client: client, keyBuilder: keyBuilder}
}

func (p *GenerationPublisher) Publish(ctx context.Context, orgID int64, _ time.Time) (int64, error) {
	if p == nil || p.client == nil {
		return 0, fmt.Errorf("statistics generation cache is unavailable")
	}
	return p.client.Incr(ctx, p.keyBuilder.BuildStatisticsGenerationKey(orgID)).Result()
}

func (p *GenerationPublisher) Generation(ctx context.Context, orgID int64) (int64, error) {
	if p == nil || p.client == nil {
		return 0, fmt.Errorf("statistics generation cache is unavailable")
	}
	value, err := p.client.Get(ctx, p.keyBuilder.BuildStatisticsGenerationKey(orgID)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return value, err
}
