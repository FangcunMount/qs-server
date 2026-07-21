package statisticsv2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	redis "github.com/redis/go-redis/v9"
)

type l1Entry struct {
	payload    []byte
	generation int64
	expiresAt  time.Time
}

// QueryCache combines generation-aware Redis data with a bounded process-local
// last-known-good value. L1 is intentionally small and time-bound; it exists to
// provide an explainable stale response during Redis incidents.
type QueryCache struct {
	client        redis.UniversalClient
	gen           *GenerationPublisher
	ttl, staleTTL time.Duration
	mu            sync.Mutex
	l1            map[string]l1Entry
	now           func() time.Time
}

func NewQueryCache(client redis.UniversalClient) *QueryCache {
	return &QueryCache{client: client, gen: NewGenerationPublisher(client), ttl: 26 * time.Hour, staleTTL: 72 * time.Hour, l1: map[string]l1Entry{}, now: time.Now}
}

func logicalL1Key(orgID int64, logical string) string { return fmt.Sprintf("%d:%s", orgID, logical) }
func redisDataKey(orgID, generation int64, logical string) string {
	sum := sha256.Sum256([]byte(logical))
	return fmt.Sprintf("query:data:statistics:v2:org:%d:g:%d:%s", orgID, generation, hex.EncodeToString(sum[:]))
}

func (c *QueryCache) Get(ctx context.Context, orgID int64, logical string, out any) (bool, bool) {
	if c == nil {
		return false, false
	}
	generation, generationErr := c.gen.Generation(ctx, orgID)
	if generationErr == nil {
		payload, err := c.client.Get(ctx, redisDataKey(orgID, generation, logical)).Bytes()
		if err == nil && json.Unmarshal(payload, out) == nil {
			c.putL1(orgID, logical, payload, generation)
			return true, false
		}
		if err != nil && err != redis.Nil {
			return c.getL1(orgID, logical, out, true)
		}
		if hit, _ := c.getL1Generation(orgID, logical, generation, out); hit {
			return true, false
		}
		return false, false
	}
	return c.getL1(orgID, logical, out, true)
}

func (c *QueryCache) Set(ctx context.Context, orgID int64, logical string, value any) {
	if c == nil {
		return
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return
	}
	generation, err := c.gen.Generation(ctx, orgID)
	if err == nil {
		_ = c.client.Set(ctx, redisDataKey(orgID, generation, logical), payload, c.ttl).Err()
	}
	c.putL1(orgID, logical, payload, generation)
}

func (c *QueryCache) putL1(orgID int64, logical string, payload []byte, generation int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.l1) >= 2048 {
		for key, value := range c.l1 {
			if c.now().After(value.expiresAt) {
				delete(c.l1, key)
			}
		}
		if len(c.l1) >= 2048 {
			for key := range c.l1 {
				delete(c.l1, key)
				break
			}
		}
	}
	c.l1[logicalL1Key(orgID, logical)] = l1Entry{payload: append([]byte(nil), payload...), generation: generation, expiresAt: c.now().Add(c.staleTTL)}
}

func (c *QueryCache) getL1(orgID int64, logical string, out any, stale bool) (bool, bool) {
	c.mu.Lock()
	entry, ok := c.l1[logicalL1Key(orgID, logical)]
	if ok && c.now().After(entry.expiresAt) {
		delete(c.l1, logicalL1Key(orgID, logical))
		ok = false
	}
	c.mu.Unlock()
	if !ok || json.Unmarshal(entry.payload, out) != nil {
		return false, false
	}
	return true, stale
}

func (c *QueryCache) getL1Generation(orgID int64, logical string, generation int64, out any) (bool, bool) {
	c.mu.Lock()
	entry, ok := c.l1[logicalL1Key(orgID, logical)]
	c.mu.Unlock()
	if !ok || entry.generation != generation || c.now().After(entry.expiresAt) || json.Unmarshal(entry.payload, out) != nil {
		return false, false
	}
	return true, false
}
