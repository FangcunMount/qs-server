package cachequery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
)

const defaultAssessmentListCacheTTL = 10 * time.Minute

const defaultAssessmentListLocalMaxEntries = 512

// MyAssessmentListCache caches "my assessment list" queries.
// It uses version tokens and versioned keys so the hot path does not need pattern deletion.
type MyAssessmentListCache struct {
	query      *VersionedQueryCache
	keyBuilder *rediskey.Builder
}

func NewMyAssessmentListCacheWithBuilderAndPolicy(c cacheentry.Cache, versionStore VersionTokenStore, keyBuilder *rediskey.Builder, policy cachepolicy.CachePolicy) *MyAssessmentListCache {
	return NewMyAssessmentListCacheWithBuilderPolicyAndObserver(c, versionStore, keyBuilder, policy, nil)
}

func NewMyAssessmentListCacheWithBuilderPolicyAndObserver(c cacheentry.Cache, versionStore VersionTokenStore, keyBuilder *rediskey.Builder, policy cachepolicy.CachePolicy, observer FamilyObserver) *MyAssessmentListCache {
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
		query: NewVersionedQueryCacheWithObserver(
			c,
			versionStore,
			cachepolicy.PolicyAssessmentList,
			policy,
			policy.TTLOr(defaultAssessmentListCacheTTL),
			NewLocalHotCache[[]byte](30*time.Second, defaultAssessmentListLocalMaxEntries),
			observer,
		),
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
		return cacheentry.ErrCacheNotFound
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
