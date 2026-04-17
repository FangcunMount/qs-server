package rediskey

import (
	"fmt"

	rediskit "github.com/FangcunMount/component-base/pkg/redis"
)

var namespace string

// ApplyNamespace 设置全局 Redis key 命名空间（可选）。
// 传入空字符串表示不使用命名空间。
func ApplyNamespace(ns string) {
	namespace = rediskit.NormalizeNamespace(ns)
}

// AddNamespace 在 key 前增加命名空间（如果设置了）。
func AddNamespace(key string) string {
	return rediskit.NewKeyspace(namespace).Prefix(key)
}

// Builder 统一管理 Redis key 的构建规则。
type Builder struct{}

// NewBuilder 创建统一 Redis key 构建器。
func NewBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) BuildScaleKey(code string) string {
	return AddNamespace("scale:" + code)
}

func (b *Builder) BuildScaleListKey() string {
	return AddNamespace("scale:list:v1")
}

func (b *Builder) BuildQuestionnaireKey(code, version string) string {
	if version == "" {
		return AddNamespace("questionnaire:" + code)
	}
	return AddNamespace("questionnaire:" + code + ":" + version)
}

func (b *Builder) BuildPublishedQuestionnaireKey(code string) string {
	return AddNamespace("questionnaire:published:" + code)
}

func (b *Builder) BuildAssessmentDetailKey(id uint64) string {
	return AddNamespace(fmt.Sprintf("assessment:detail:%d", id))
}

func (b *Builder) BuildAssessmentListKey(userID uint64, suffix string) string {
	key := fmt.Sprintf("assess:list:%d:v1", userID)
	if suffix != "" {
		key += suffix
	}
	return AddNamespace(key)
}

func (b *Builder) BuildTesteeInfoKey(id uint64) string {
	return AddNamespace(fmt.Sprintf("testee:info:%d", id))
}

func (b *Builder) BuildPlanInfoKey(id uint64) string {
	return AddNamespace(fmt.Sprintf("plan:info:%d", id))
}

func (b *Builder) BuildStatsDailyKey(orgID int64, statType, statKey, date string) string {
	return AddNamespace(fmt.Sprintf("stats:daily:%d:%s:%s:%s", orgID, statType, statKey, date))
}

func (b *Builder) BuildStatsDailyPattern(orgID int64, statType string) string {
	return AddNamespace(fmt.Sprintf("stats:daily:%d:%s:*", orgID, statType))
}

func (b *Builder) BuildStatsQueryKey(cacheKey string) string {
	return AddNamespace("stats:query:" + cacheKey)
}

func (b *Builder) BuildEventProcessedKey(eventID string) string {
	return AddNamespace("event:processed:" + eventID)
}

func (b *Builder) BuildEventProcessedBucketKey(date string) string {
	return AddNamespace("event:processed:bucket:" + date)
}

func (b *Builder) BuildAnswerSheetProcessingLockKey(answerSheetID uint64) string {
	return AddNamespace(fmt.Sprintf("answersheet:processing:%d", answerSheetID))
}

func (b *Builder) BuildLockKey(lockKey string) string {
	return AddNamespace(lockKey)
}

func (b *Builder) BuildWeChatCacheKey(key string) string {
	return AddNamespace("wechat:cache:" + key)
}
