package evaluationcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/internal/adapterkit"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	cacheobserve "github.com/FangcunMount/qs-server/internal/pkg/cache/observe"
	querycache "github.com/FangcunMount/qs-server/internal/pkg/cache/query"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	redis "github.com/redis/go-redis/v9"
)

const defaultAssessmentListLocalMaxEntries = 512

func NewVersionTokenStore(client redis.UniversalClient, health cacheobserve.FamilyObserver) querycache.VersionTokenStore {
	return adapterkit.NewVersionTokenStore(client, cachepolicy.CapabilityEvaluationAssessmentList, health)
}

// MyAssessmentListCache caches "my assessment list" queries.
// It uses version tokens and versioned keys so the hot path does not need pattern deletion.
type MyAssessmentListCache struct {
	query      *querycache.Versioned
	keyBuilder *keyspace.Builder
}

func NewMyAssessmentListCacheWithBuilderAndProvider(c sharedcache.Store, versionStore querycache.VersionTokenStore, keyBuilder *keyspace.Builder, policies sharedcache.PolicyProvider) *MyAssessmentListCache {
	return NewMyAssessmentListCacheWithBuilderProviderAndObserver(c, versionStore, keyBuilder, policies, nil)
}

func NewMyAssessmentListCacheWithBuilderProviderAndObserver(c sharedcache.Store, versionStore querycache.VersionTokenStore, keyBuilder *keyspace.Builder, policies sharedcache.PolicyProvider, observer cacheobserve.FamilyObserver) *MyAssessmentListCache {
	if c == nil {
		return nil
	}
	if versionStore == nil {
		panic("version token store is required")
	}
	if keyBuilder == nil {
		panic("cache key builder is required")
	}
	return &MyAssessmentListCache{
		query: querycache.NewVersioned(querycache.VersionedOptions{
			Store:      c,
			Version:    versionStore,
			Capability: sharedcache.Capability(cachepolicy.CapabilityEvaluationAssessmentList),
			Policies:   policies,
			Memory:     querycache.NewLocalHotCache[[]byte](30*time.Second, defaultAssessmentListLocalMaxEntries),
			Observer:   adapterkit.NewCapabilityObserver(cachepolicy.CapabilityEvaluationAssessmentList, observer),
		}),
		keyBuilder: keyBuilder,
	}
}

func (c *MyAssessmentListCache) Get(
	ctx context.Context,
	userID uint64,
	page, pageSize int,
	status string,
	scaleCode string,
	riskLevel string,
	dateFrom string,
	dateTo string,
	dest interface{},
) error {
	if c == nil || c.query == nil {
		return sharedcache.ErrMiss
	}
	versionKey := c.buildVersionKey(userID)
	return c.query.Get(ctx, versionKey, func(version uint64) string {
		return c.buildDataKey(userID, version, page, pageSize, status, scaleCode, riskLevel, dateFrom, dateTo)
	}, dest)
}

func (c *MyAssessmentListCache) Set(
	ctx context.Context,
	userID uint64,
	page, pageSize int,
	status string,
	scaleCode string,
	riskLevel string,
	dateFrom string,
	dateTo string,
	value interface{},
) {
	if c == nil || c.query == nil {
		return
	}
	versionKey := c.buildVersionKey(userID)
	c.query.Set(ctx, versionKey, func(version uint64) string {
		return c.buildDataKey(userID, version, page, pageSize, status, scaleCode, riskLevel, dateFrom, dateTo)
	}, value)
}

func (c *MyAssessmentListCache) Invalidate(ctx context.Context, userID uint64) error {
	if c == nil || c.query == nil {
		return nil
	}
	return c.query.Invalidate(ctx, c.buildVersionKey(userID))
}

func (c *MyAssessmentListCache) buildDataKey(
	userID uint64,
	version uint64,
	page, pageSize int,
	status string,
	scaleCode string,
	riskLevel string,
	dateFrom string,
	dateTo string,
) string {
	raw := fmt.Sprintf(
		"status=%s&scale_code=%s&risk_level=%s&date_from=%s&date_to=%s&page=%d&page_size=%d",
		status,
		scaleCode,
		riskLevel,
		dateFrom,
		dateTo,
		page,
		pageSize,
	)
	hash := sha256.Sum256([]byte(raw))
	return c.keyBuilder.BuildAssessmentListVersionedKey(userID, version, hex.EncodeToString(hash[:])[:8])
}

func (c *MyAssessmentListCache) buildVersionKey(userID uint64) string {
	return c.keyBuilder.BuildAssessmentListVersionKey(userID)
}
