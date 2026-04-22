package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
)

const defaultAssessmentListCacheTTL = 10 * time.Minute

const defaultAssessmentListLocalMaxEntries = 512

// MyAssessmentListCache “我的测评列表”缓存。
// 采用 version token + versioned key 失效，避免主路径使用 DeletePattern。
type MyAssessmentListCache struct {
	query      *VersionedQueryCache
	keyBuilder *rediskey.Builder
}

// NewMyAssessmentListCacheWithBuilderAndPolicy 创建带显式 builder/policy 的“我的测评列表”缓存。
func NewMyAssessmentListCacheWithBuilderAndPolicy(c Cache, versionStore VersionTokenStore, keyBuilder *rediskey.Builder, policy cachepolicy.CachePolicy) *MyAssessmentListCache {
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
		query: NewVersionedQueryCache(
			c,
			versionStore,
			cachepolicy.PolicyAssessmentList,
			policy,
			policy.TTLOr(defaultAssessmentListCacheTTL),
			NewLocalHotCache[[]byte](30*time.Second, defaultAssessmentListLocalMaxEntries),
		),
		keyBuilder: keyBuilder,
	}
}

// Get 读取缓存并解码到 dest（指针）
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
		return ErrCacheNotFound
	}
	versionKey := c.buildVersionKey(userID)
	return c.query.Get(ctx, versionKey, func(version uint64) string {
		return c.buildDataKey(userID, version, page, pageSize, status, scaleCode, riskLevel, dateFrom, dateTo)
	}, dest)
}

// Set 写入缓存（value 将被 JSON 序列化）
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

// Invalidate bump version token；旧 key 依赖 TTL 和 versioned key 自然过期。
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
