package keyspace

import rediskit "github.com/FangcunMount/component-base/pkg/redis"

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
	return NewCacheKeyspace(b.namespace()).Scale(code)
}

func (b *Builder) BuildScaleListKey() string {
	return NewCacheKeyspace(b.namespace()).ScaleList()
}

func (b *Builder) BuildScaleHotDailyKey(day string) string {
	return NewCacheKeyspace(b.namespace()).ScaleHotDaily(day)
}

func (b *Builder) BuildScaleHotWindowKey(token string) string {
	return NewCacheKeyspace(b.namespace()).ScaleHotWindow(token)
}

func (b *Builder) BuildScaleHotProjectedKey(eventID string) string {
	return NewCacheKeyspace(b.namespace()).ScaleHotProjected(eventID)
}

func (b *Builder) BuildQuestionnaireKey(code, version string) string {
	return NewCacheKeyspace(b.namespace()).Questionnaire(code, version)
}

func (b *Builder) BuildPublishedQuestionnaireKey(code string) string {
	return NewCacheKeyspace(b.namespace()).PublishedQuestionnaire(code)
}

func (b *Builder) BuildAssessmentDetailKey(id uint64) string {
	return NewCacheKeyspace(b.namespace()).AssessmentDetail(id)
}

func (b *Builder) BuildAssessmentListKey(userID uint64, suffix string) string {
	return NewCacheKeyspace(b.namespace()).AssessmentList(userID, suffix)
}

func (b *Builder) BuildQueryVersionKey(kind, scope string) string {
	return NewCacheKeyspace(b.namespace()).QueryVersion(kind, scope)
}

func (b *Builder) BuildVersionedQueryKey(kind, scope string, version uint64, hash string) string {
	return NewCacheKeyspace(b.namespace()).VersionedQuery(kind, scope, version, hash)
}

func (b *Builder) BuildAssessmentListVersionKey(userID uint64) string {
	return NewCacheKeyspace(b.namespace()).AssessmentListVersion(userID)
}

func (b *Builder) BuildAssessmentListVersionedKey(userID, version uint64, hash string) string {
	return NewCacheKeyspace(b.namespace()).AssessmentListVersioned(userID, version, hash)
}

func (b *Builder) BuildTesteeInfoKey(id uint64) string {
	return NewCacheKeyspace(b.namespace()).TesteeInfo(id)
}

func (b *Builder) BuildPlanInfoKey(id uint64) string {
	return NewCacheKeyspace(b.namespace()).PlanInfo(id)
}

func (b *Builder) BuildStatsQueryKey(cacheKey string) string {
	return NewCacheKeyspace(b.namespace()).StatsQuery(cacheKey)
}

func (b *Builder) BuildWarmupHotsetKey(family, kind string) string {
	return NewGovernanceKeyspace(b.namespace()).WarmupHotset(family, kind)
}

func (b *Builder) BuildAnswerSheetProcessingLockKey(answerSheetID uint64) string {
	return NewLockKeyspace(b.namespace()).AnswerSheetProcessing(answerSheetID)
}

func (b *Builder) BuildLockKey(lockKey string) string {
	return NewLockKeyspace(b.namespace()).Lock(lockKey)
}

func (b *Builder) BuildWeChatCacheKey(key string) string {
	return NewCacheKeyspace(b.namespace()).WeChatSDK(key)
}

// func (b *Builder) prefix(key string) string {
// 	if b == nil {
// 		return AddNamespace(key)
// 	}
// 	return b.keyspace.Prefix(key)
// }

func (b *Builder) namespace() string {
	if b == nil {
		return namespace
	}
	return b.keyspace.Namespace().String()
}

// Namespace returns the namespace bound to this builder.
func (b *Builder) Namespace() string {
	return b.namespace()
}
