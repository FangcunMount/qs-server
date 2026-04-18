package rediskey

import (
	"fmt"
	"strconv"

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

// ComposeNamespace 组合根 namespace 与子 suffix。
func ComposeNamespace(root, child string) string {
	root = rediskit.NormalizeNamespace(root)
	child = rediskit.NormalizeNamespace(child)
	switch {
	case root == "":
		return child
	case child == "":
		return root
	default:
		return root + ":" + child
	}
}

// Builder 统一管理 Redis key 的构建规则。
type Builder struct {
	keyspace rediskit.Keyspace
}

// NewBuilder 创建统一 Redis key 构建器。
func NewBuilder() *Builder {
	return NewBuilderWithNamespace(namespace)
}

// NewBuilderWithNamespace 创建绑定到指定 namespace 的 Redis key 构建器。
func NewBuilderWithNamespace(ns string) *Builder {
	return &Builder{keyspace: rediskit.NewKeyspace(ns)}
}

func (b *Builder) BuildScaleKey(code string) string {
	return b.prefix("scale:" + code)
}

func (b *Builder) BuildScaleListKey() string {
	return b.prefix("scale:list:v1")
}

func (b *Builder) BuildQuestionnaireKey(code, version string) string {
	if version == "" {
		return b.prefix("questionnaire:" + code)
	}
	return b.prefix("questionnaire:" + code + ":" + version)
}

func (b *Builder) BuildPublishedQuestionnaireKey(code string) string {
	return b.prefix("questionnaire:published:" + code)
}

func (b *Builder) BuildAssessmentDetailKey(id uint64) string {
	return b.prefix(fmt.Sprintf("assessment:detail:%d", id))
}

func (b *Builder) BuildAssessmentListKey(userID uint64, suffix string) string {
	key := fmt.Sprintf("assess:list:%d:v1", userID)
	if suffix != "" {
		key += suffix
	}
	return b.prefix(key)
}

func (b *Builder) BuildQueryVersionKey(kind, scope string) string {
	return b.prefix("query:version:" + kind + ":" + scope)
}

func (b *Builder) BuildVersionedQueryKey(kind, scope string, version uint64, hash string) string {
	key := fmt.Sprintf("query:%s:%s:v%d", kind, scope, version)
	if hash != "" {
		key += ":" + hash
	}
	return b.prefix(key)
}

func (b *Builder) BuildAssessmentListVersionKey(userID uint64) string {
	return b.BuildQueryVersionKey("assessment:list", strconv.FormatUint(userID, 10))
}

func (b *Builder) BuildAssessmentListVersionedKey(userID, version uint64, hash string) string {
	return b.BuildVersionedQueryKey("assessment:list", strconv.FormatUint(userID, 10), version, hash)
}

func (b *Builder) BuildTesteeInfoKey(id uint64) string {
	return b.prefix(fmt.Sprintf("testee:info:%d", id))
}

func (b *Builder) BuildPlanInfoKey(id uint64) string {
	return b.prefix(fmt.Sprintf("plan:info:%d", id))
}

func (b *Builder) BuildStatsQueryKey(cacheKey string) string {
	return b.prefix("stats:query:" + cacheKey)
}

func (b *Builder) BuildWarmupHotsetKey(family, kind string) string {
	return b.prefix("warmup:hot:" + family + ":" + kind)
}

func (b *Builder) BuildAnswerSheetProcessingLockKey(answerSheetID uint64) string {
	return b.prefix(fmt.Sprintf("answersheet:processing:%d", answerSheetID))
}

func (b *Builder) BuildLockKey(lockKey string) string {
	return b.prefix(lockKey)
}

func (b *Builder) BuildWeChatCacheKey(key string) string {
	return b.prefix("wechat:cache:" + key)
}

func (b *Builder) prefix(key string) string {
	if b == nil {
		return AddNamespace(key)
	}
	return b.keyspace.Prefix(key)
}
