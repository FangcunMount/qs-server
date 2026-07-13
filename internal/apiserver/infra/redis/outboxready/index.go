package outboxready

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/redis/go-redis/v9"
)

const keyPrefix = "outbox:ready"

// Ready-index store namespaces isolate relay claim paths across Mongo/MySQL outboxes.
const (
	StoreMongoDomainEvents     = "mongo-domain-events"
	StoreAssessmentMySQLOutbox = "assessment-mysql-outbox"
)

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
	client         redis.UniversalClient
	store          string
	priorityBucket func(string) string
	buckets        []string
}

// NewIndexWithRegistry binds ready-index routing to the same immutable policy
// used by the owning outbox profile.
func NewIndexWithRegistry(client redis.UniversalClient, store string, registry *eventcatalog.EffectiveRegistry) *Index {
	if registry == nil {
		return nil
	}
	return newIndex(client, store, registry.PriorityBucket, registry.ReadyIndexBuckets())
}

func newIndex(client redis.UniversalClient, store string, priorityBucket func(string) string, buckets []string) *Index {
	if client == nil || store == "" {
		return nil
	}
	return &Index{client: client, store: store, priorityBucket: priorityBucket, buckets: append([]string(nil), buckets...)}
}

func (i *Index) bucketKey(bucket string) string {
	return fmt.Sprintf("%s:%s:%s", keyPrefix, i.store, bucket)
}

func (i *Index) Enqueue(ctx context.Context, eventType, eventID string, nextAttemptAt, createdAt time.Time) error {
	if i == nil || i.client == nil || eventID == "" {
		return nil
	}
	bucket := i.priorityBucket(eventType)
	key := i.bucketKey(bucket)
	score := ReadyScore(nextAttemptAt, createdAt)
	return i.client.ZAdd(ctx, key, redis.Z{Score: score, Member: eventID}).Err()
}

func (i *Index) Remove(ctx context.Context, eventType, eventID string) error {
	if i == nil || i.client == nil || eventID == "" {
		return nil
	}
	key := i.bucketKey(i.priorityBucket(eventType))
	return i.client.ZRem(ctx, key, eventID).Err()
}

// RemoveByEventID drops an event ID from every ready-index bucket.
func (i *Index) RemoveByEventID(ctx context.Context, eventID string) error {
	if i == nil || i.client == nil || eventID == "" {
		return nil
	}
	for _, bucket := range i.buckets {
		key := i.bucketKey(bucket)
		if err := i.client.ZRem(ctx, key, eventID).Err(); err != nil {
			return err
		}
	}
	return nil
}

// MaxClaimScore is the upper bound for claiming events due at or before now.
func MaxClaimScore(now time.Time) float64 {
	return float64(now.UnixMilli())*scoreTieFactor + scoreTieMask
}

func (i *Index) ClaimDueIDs(ctx context.Context, bucket string, limit int, now time.Time) ([]string, error) {
	if i == nil || i.client == nil || limit <= 0 {
		return nil, nil
	}
	key := i.bucketKey(bucket)
	max := strconv.FormatFloat(MaxClaimScore(now), 'f', -1, 64)
	result, err := claimDueIDsScript.Run(ctx, i.client, []string{key}, max, limit).StringSlice()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}
