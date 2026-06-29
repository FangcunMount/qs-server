package outboxready

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/outboxpriority"
	"github.com/redis/go-redis/v9"
)

const keyPrefix = "outbox:ready"

// Atomically ZRANGEBYSCORE due members then ZREM them so concurrent relays do not
// read the same IDs. Mongo outbox remains the truth source; Reconciler backfills
// rows that were popped but not durably claimed.
var claimDueIDsScript = redis.NewScript(`
local key = KEYS[1]
local max_score = ARGV[1]
local limit = tonumber(ARGV[2])
if limit <= 0 then
	return {}
end
local members = redis.call('ZRANGEBYSCORE', key, '-inf', max_score, 'LIMIT', 0, limit)
if #members == 0 then
	return {}
end
redis.call('ZREM', key, unpack(members))
return members
`)

// Index is a best-effort Redis ZSet scheduler for outbox pending events.
type Index struct {
	client redis.UniversalClient
}

func NewIndex(client redis.UniversalClient) *Index {
	if client == nil {
		return nil
	}
	return &Index{client: client}
}

func (i *Index) Enqueue(ctx context.Context, eventType, eventID string, nextAttemptAt time.Time) error {
	if i == nil || i.client == nil || eventID == "" {
		return nil
	}
	bucket := outboxpriority.Bucket(eventType)
	key := fmt.Sprintf("%s:%s", keyPrefix, bucket)
	score := float64(nextAttemptAt.UnixMilli())
	return i.client.ZAdd(ctx, key, redis.Z{Score: score, Member: eventID}).Err()
}

func (i *Index) Remove(ctx context.Context, eventType, eventID string) error {
	if i == nil || i.client == nil || eventID == "" {
		return nil
	}
	key := fmt.Sprintf("%s:%s", keyPrefix, outboxpriority.Bucket(eventType))
	return i.client.ZRem(ctx, key, eventID).Err()
}

// RemoveByEventID drops an event ID from every ready-index bucket.
func (i *Index) RemoveByEventID(ctx context.Context, eventID string) error {
	if i == nil || i.client == nil || eventID == "" {
		return nil
	}
	for _, bucket := range outboxpriority.ReadyIndexBuckets {
		key := fmt.Sprintf("%s:%s", keyPrefix, bucket)
		if err := i.client.ZRem(ctx, key, eventID).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (i *Index) ClaimDueIDs(ctx context.Context, bucket string, limit int, now time.Time) ([]string, error) {
	if i == nil || i.client == nil || limit <= 0 {
		return nil, nil
	}
	key := fmt.Sprintf("%s:%s", keyPrefix, bucket)
	max := strconv.FormatInt(now.UnixMilli(), 10)
	result, err := claimDueIDsScript.Run(ctx, i.client, []string{key}, max, limit).StringSlice()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (i *Index) Reconcile(ctx context.Context, bucket string, eventIDs []string, nextAttemptAt time.Time) error {
	if i == nil || i.client == nil || len(eventIDs) == 0 {
		return nil
	}
	key := fmt.Sprintf("%s:%s", keyPrefix, bucket)
	score := float64(nextAttemptAt.UnixMilli())
	members := make([]redis.Z, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		members = append(members, redis.Z{Score: score, Member: eventID})
	}
	return i.client.ZAdd(ctx, key, members...).Err()
}
