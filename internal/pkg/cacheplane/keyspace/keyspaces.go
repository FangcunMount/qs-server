package keyspace

import (
	"fmt"
	"strconv"

	rediskit "github.com/FangcunMount/component-base/pkg/redis"
)

// CacheKeyspace builds cache payload and version-token keys.
type CacheKeyspace struct {
	keyspace rediskit.Keyspace
}

func NewCacheKeyspace(ns string) CacheKeyspace {
	return CacheKeyspace{keyspace: rediskit.NewKeyspace(ns)}
}

func (k CacheKeyspace) Scale(code string) string {
	return k.keyspace.Prefix("scale:" + code)
}

func (k CacheKeyspace) ScaleList() string {
	return k.keyspace.Prefix("scale:list:v1")
}

func (k CacheKeyspace) ScaleHotDaily(day string) string {
	return k.keyspace.Prefix("scale:hot:{rank}:daily:" + day)
}

func (k CacheKeyspace) ScaleHotWindow(token string) string {
	return k.keyspace.Prefix("scale:hot:{rank}:window:" + token)
}

func (k CacheKeyspace) ScaleHotProjected(eventID string) string {
	return k.keyspace.Prefix("scale:hot:{rank}:projected:" + eventID)
}

func (k CacheKeyspace) Questionnaire(code, version string) string {
	if version == "" {
		return k.keyspace.Prefix("questionnaire:" + code)
	}
	return k.keyspace.Prefix("questionnaire:" + code + ":" + version)
}

func (k CacheKeyspace) PublishedQuestionnaire(code string) string {
	return k.keyspace.Prefix("questionnaire:published:" + code)
}

func (k CacheKeyspace) AssessmentDetail(id uint64) string {
	return k.keyspace.Prefix(fmt.Sprintf("assessment:detail:%d", id))
}

func (k CacheKeyspace) AssessmentList(userID uint64, suffix string) string {
	key := fmt.Sprintf("assess:list:%d:v1", userID)
	if suffix != "" {
		key += suffix
	}
	return k.keyspace.Prefix(key)
}

func (k CacheKeyspace) QueryVersion(kind, scope string) string {
	return k.keyspace.Prefix("query:version:" + kind + ":" + scope)
}

func (k CacheKeyspace) VersionedQuery(kind, scope string, version uint64, hash string) string {
	key := fmt.Sprintf("query:%s:%s:v%d", kind, scope, version)
	if hash != "" {
		key += ":" + hash
	}
	return k.keyspace.Prefix(key)
}

func (k CacheKeyspace) AssessmentListVersion(userID uint64) string {
	return k.QueryVersion("assessment:list", strconv.FormatUint(userID, 10))
}

func (k CacheKeyspace) AssessmentListVersioned(userID, version uint64, hash string) string {
	return k.VersionedQuery("assessment:list", strconv.FormatUint(userID, 10), version, hash)
}

func (k CacheKeyspace) TesteeInfo(id uint64) string {
	return k.keyspace.Prefix(fmt.Sprintf("testee:info:%d", id))
}

func (k CacheKeyspace) PlanInfo(id uint64) string {
	return k.keyspace.Prefix(fmt.Sprintf("plan:info:%d", id))
}

func (k CacheKeyspace) StatsQuery(cacheKey string) string {
	return k.keyspace.Prefix("stats:query:" + cacheKey)
}

func (k CacheKeyspace) WeChatSDK(key string) string {
	return k.keyspace.Prefix("wechat:cache:" + key)
}

// GovernanceKeyspace builds cache governance keys.
type GovernanceKeyspace struct {
	keyspace rediskit.Keyspace
}

func NewGovernanceKeyspace(ns string) GovernanceKeyspace {
	return GovernanceKeyspace{keyspace: rediskit.NewKeyspace(ns)}
}

func (k GovernanceKeyspace) WarmupHotset(family, kind string) string {
	return k.keyspace.Prefix("warmup:hot:" + family + ":" + kind)
}

// LockKeyspace builds lock lease keys.
type LockKeyspace struct {
	keyspace rediskit.Keyspace
}

func NewLockKeyspace(ns string) LockKeyspace {
	return LockKeyspace{keyspace: rediskit.NewKeyspace(ns)}
}

func (k LockKeyspace) AnswerSheetProcessing(answerSheetID uint64) string {
	return k.keyspace.Prefix(fmt.Sprintf("answersheet:processing:%d", answerSheetID))
}

func (k LockKeyspace) Lock(raw string) string {
	return k.keyspace.Prefix(raw)
}

// OpsKeyspace builds short-lived operational keys.
type OpsKeyspace struct {
	keyspace rediskit.Keyspace
}

func NewOpsKeyspace(ns string) OpsKeyspace {
	return OpsKeyspace{keyspace: rediskit.NewKeyspace(ns)}
}

func (k OpsKeyspace) IdempotencyInflight(key string) string {
	return k.keyspace.Prefix("submit:idempotency:" + key + ":lock")
}

func (k OpsKeyspace) IdempotencyDone(key string) string {
	return k.keyspace.Prefix("submit:idempotency:" + key + ":done")
}
