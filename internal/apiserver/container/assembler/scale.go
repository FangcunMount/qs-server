package assembler

import (
	"go.mongodb.org/mongo-driver/mongo"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	scaleInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ScaleModule Scale 模块（量表子域）
// 按照 DDD 限界上下文组织
type ScaleModule struct {
	// repository 层
	Repo scale.Repository

	// service 层 - 按行为者组织
	LifecycleService scaleApp.ScaleLifecycleService
	FactorService    scaleApp.ScaleFactorService
	QueryService     scaleApp.ScaleQueryService
	CategoryService  scaleApp.ScaleCategoryService
	ListCache        scalelistcache.PublishedListCache
	Reader           scalereadmodel.ScaleReader

	// 事件发布器（由容器统一注入）
	eventPublisher event.EventPublisher
}

// ScaleModuleDeps 定义 Scale 模块的显式构造依赖。
type ScaleModuleDeps struct {
	MongoDB           *mongo.Database
	EventPublisher    event.EventPublisher
	QuestionnaireRepo domainQuestionnaire.Repository
	RedisClient       redis.UniversalClient
	CacheBuilder      *keyspace.Builder
	RankRedisClient   redis.UniversalClient
	RankCacheBuilder  *keyspace.Builder
	IdentityService   *iam.IdentityService
	ScalePolicy       cachepolicy.CachePolicy
	ScaleListPolicy   cachepolicy.CachePolicy
	HotsetRecorder    cachetarget.HotsetRecorder
	Observer          *observability.ComponentObserver
	MongoLimiter      backpressure.Acquirer
}

// NewScaleModule 创建 Scale 模块。
func NewScaleModule(deps ScaleModuleDeps) (*ScaleModule, error) {
	normalized, err := normalizeScaleModuleDeps(deps)
	if err != nil {
		return nil, err
	}

	module := &ScaleModule{}
	module.eventPublisher = normalized.EventPublisher

	// 初始化 repository 层（基础实现）
	baseRepo := scaleInfra.NewRepository(normalized.MongoDB, mongoBase.BaseRepositoryOptions{Limiter: normalized.MongoLimiter})
	module.Reader = scaleInfra.NewScaleReadModel(baseRepo)
	// 如果提供了 Redis 客户端，使用缓存装饰器
	if normalized.RedisClient != nil {
		module.Repo = scaleCache.NewCachedScaleRepositoryWithBuilderPolicyAndObserver(baseRepo, normalized.RedisClient, normalized.CacheBuilder, normalized.ScalePolicy, normalized.Observer)
	} else {
		module.Repo = baseRepo
	}

	// 初始化量表全局列表缓存
	var listCache scalelistcache.PublishedListCache
	if normalized.RedisClient != nil {
		listCache = cachequery.NewPublishedScaleListCacheWithPolicyAndKeyBuilder(
			cacheentry.NewRedisCache(normalized.RedisClient),
			module.Reader,
			normalized.IdentityService,
			normalized.CacheBuilder,
			normalized.ScaleListPolicy,
		)
	}
	module.ListCache = listCache

	// 初始化 service 层（依赖 repository，使用模块统一的事件发布器）
	module.LifecycleService = scaleApp.NewLifecycleService(module.Repo, quesApp.NewPublishedQuestionnaireCatalog(normalized.QuestionnaireRepo), module.eventPublisher, listCache)
	module.FactorService = scaleApp.NewFactorService(module.Repo, listCache, module.eventPublisher)
	hotRankReader := scaleCache.NewRedisScaleHotRankProjection(normalized.RankRedisClient, normalized.RankCacheBuilder)
	module.QueryService = scaleApp.NewQueryService(module.Repo, module.Reader, normalized.IdentityService, listCache, normalized.HotsetRecorder, hotRankReader)
	module.CategoryService = scaleApp.NewCategoryService()

	return module, nil
}

func normalizeScaleModuleDeps(deps ScaleModuleDeps) (ScaleModuleDeps, error) {
	if deps.MongoDB == nil {
		return ScaleModuleDeps{}, errors.WithCode(code.ErrModuleInitializationFailed, "database connection is nil")
	}
	if deps.EventPublisher == nil {
		deps.EventPublisher = event.NewNopEventPublisher()
	}
	return deps, nil
}

// Cleanup 清理模块资源
func (m *ScaleModule) Cleanup() error {
	return nil
}

// CheckHealth 检查模块健康状态
func (m *ScaleModule) CheckHealth() error {
	return nil
}

// ModuleInfo 返回模块信息
func (m *ScaleModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "scale",
		Version:     "2.0.0",
		Description: "量表管理模块（重构版）",
	}
}
