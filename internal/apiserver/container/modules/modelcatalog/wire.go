package modelcatalog

import (
	"github.com/FangcunMount/component-base/pkg/event"
	modelcatalogRuntime "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/runtime"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	cachetarget "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	modelcatalogcache "github.com/FangcunMount/qs-server/internal/apiserver/cache/modelcatalog"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
)

// WireInput 包含容器组合根的输入
type WireInput struct {
	MongoDB                *mongo.Database
	MongoLimiter           backpressure.Acquirer
	EventPublisher         event.EventPublisher
	RankRedisClient        redis.UniversalClient
	RankCacheBuilder       *keyspace.Builder
	CacheSignalNotifier    ScaleCacheSignalNotifier
	SurveyRuntimeInfra     *surveymod.SurveyRuntimeInfra
	QuestionnairePublisher quesApp.QuestionnaireLifecycleService
	QuestionnaireQuery     quesApp.QuestionnaireQueryService
	StaticRedisClient      redis.UniversalClient
	StaticCacheBuilder     *keyspace.Builder
	CachePolicies          sharedcache.PolicyProvider
	CacheObserver          *observability.ComponentObserver
}

// Wire 构建和启动模型目录模块
func Wire(in WireInput) (*Module, error) {
	return Bootstrap(BootstrapInput{
		HotRank:   buildHotRankDeps(in),
		Lifecycle: buildLifecycleDeps(in),
		Catalog:   buildCatalogDeps(in.MongoDB, in.MongoLimiter, in.QuestionnaireQuery, catalogCacheConfig(in)),
	})
}

// buildHotRankDeps 构建热门排名依赖
func buildHotRankDeps(in WireInput) HotRankDeps {
	return HotRankDeps{RedisClient: in.RankRedisClient, KeyBuilder: in.RankCacheBuilder}
}

// buildLifecycleDeps 构建生命周期依赖
func buildLifecycleDeps(in WireInput) LifecycleDeps {
	deps := LifecycleDeps{
		EventPublisher:         in.EventPublisher,
		QuestionnairePublisher: in.QuestionnairePublisher,
		CacheSignalNotifier:    in.CacheSignalNotifier,
	}
	if infra := in.SurveyRuntimeInfra; infra != nil {
		deps.QuestionnaireCatalog = quesApp.NewPublishedQuestionnaireCatalog(infra.QuestionnaireRepo)
	}
	return deps
}

// catalogCacheWireConfig 模型目录缓存配置
type catalogCacheWireConfig struct {
	Redis    redis.UniversalClient
	Builder  *keyspace.Builder
	Policies sharedcache.PolicyProvider
	Observer *observability.ComponentObserver
	Notifier TypologyCacheSignalNotifier
}

// catalogCacheConfig 构建模型目录缓存配置
func catalogCacheConfig(in WireInput) catalogCacheWireConfig {
	var notifier TypologyCacheSignalNotifier
	if n, ok := in.CacheSignalNotifier.(TypologyCacheSignalNotifier); ok {
		notifier = n
	}
	return catalogCacheWireConfig{
		Redis: in.StaticRedisClient, Builder: in.StaticCacheBuilder,
		Policies: in.CachePolicies, Observer: in.CacheObserver,
		Notifier: notifier,
	}
}

// buildCatalogDeps 构建模型目录依赖
func buildCatalogDeps(
	mongoDB *mongo.Database,
	mongoLimiter backpressure.Acquirer,
	questionnaireQuery quesApp.QuestionnaireQueryService,
	cacheCfg catalogCacheWireConfig,
) CatalogDeps {
	if mongoDB == nil {
		return CatalogDeps{}
	}
	mongoOpts := mongoBase.BaseRepositoryOptions{Limiter: mongoLimiter}
	v2Repo := mongomodelcatalog.NewRepository(mongoDB, mongoOpts)
	normRepo := mongomodelcatalog.NewNormRepository(mongoDB, mongoOpts)
	draftRepo := mongomodelcatalog.NewDraftRepository(mongoDB, mongoOpts)
	publishedRepo := port.PublishedSnapshotRepository(v2Repo)
	dualStore := v2Repo
	var publishedStore interface {
		port.PublishedModelReader
		port.PublishedModelLister
	} = dualStore
	var publishedWarmer cachetarget.PublishedModelWarmer
	var cacheInvalidator PublishedModelCacheInvalidator
	if cacheCfg.Redis != nil && cacheCfg.Builder != nil {
		cached := modelcatalogcache.NewCachedPublishedModelStore(dualStore, cacheCfg.Redis, cacheCfg.Builder, cacheCfg.Policies, cacheCfg.Observer)
		publishedStore = cached
		publishedWarmer = cached
		// Writes remain transaction-local. Mutable visibility caches are
		// invalidated by lifecycle effects only after the transaction commits.
		cacheInvalidator = cached
	}
	runtimeCatalog := rulesetInfra.NewRuntimePublishedCatalogWithStore(publishedStore)
	trustedCatalog := modelcatalogRuntime.NewTrustedRuntimeCatalog(runtimeCatalog, runtimeCatalog)
	return CatalogDeps{
		PublishedLister:     publishedStore,
		PublishedCatalog:    trustedCatalog,
		PublishedWarmer:     publishedWarmer,
		CacheInvalidator:    cacheInvalidator,
		ModelRepo:           draftRepo,
		PublishedRepo:       publishedRepo,
		NormRepo:            normRepo,
		QuestionnaireQuery:  questionnaireQuery,
		CacheSignalNotifier: cacheCfg.Notifier,
		Transactions:        modtx.NewMongoRunner(mongoDB),
	}
}
