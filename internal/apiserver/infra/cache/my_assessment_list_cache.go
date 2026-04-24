package cache

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
)

type MyAssessmentListCache = cachequery.MyAssessmentListCache

func NewMyAssessmentListCacheWithBuilderAndPolicy(c Cache, versionStore VersionTokenStore, keyBuilder *rediskey.Builder, policy cachepolicy.CachePolicy) *MyAssessmentListCache {
	return cachequery.NewMyAssessmentListCacheWithBuilderAndPolicy(c, versionStore, keyBuilder, policy)
}

func NewMyAssessmentListCacheWithBuilderPolicyAndObserver(c Cache, versionStore VersionTokenStore, keyBuilder *rediskey.Builder, policy cachepolicy.CachePolicy, observer *Observer) *MyAssessmentListCache {
	return cachequery.NewMyAssessmentListCacheWithBuilderPolicyAndObserver(c, versionStore, keyBuilder, policy, observer)
}
