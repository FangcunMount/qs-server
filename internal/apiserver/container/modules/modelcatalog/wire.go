package modelcatalog

import (
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/pkg/event"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
)

// WireInput carries composition-root inputs for assessment-model installation.
type WireInput struct {
	MongoDB                *mongo.Database
	MongoLimiter           backpressure.Acquirer
	EventPublisher         event.EventPublisher
	RankRedisClient        redis.UniversalClient
	RankCacheBuilder       *keyspace.Builder
	CacheSignalNotifier    ScaleCacheSignalNotifier
	ScaleInfra             *surveymod.ScaleInfra
	QuestionnairePublisher quesApp.QuestionnaireLifecycleService
	QuestionnaireQuery     quesApp.QuestionnaireQueryService
	StaticRedisClient      redis.UniversalClient
	StaticCacheBuilder     *keyspace.Builder
	PublishedModelPolicy   cachepolicy.CachePolicy
	CacheObserver          *observability.ComponentObserver
}

// Wire builds and bootstraps the assessment-model module from composition inputs.
func Wire(in WireInput) (*Module, error) {
	return Bootstrap(BootstrapInput{
		HotRank:   buildHotRankDeps(in),
		Lifecycle: buildLifecycleDeps(in),
		Catalog:   buildCatalogDeps(in.MongoDB, in.MongoLimiter, in.QuestionnaireQuery, catalogCacheConfig(in)),
	})
}

func buildHotRankDeps(in WireInput) HotRankDeps {
	return HotRankDeps{RedisClient: in.RankRedisClient, KeyBuilder: in.RankCacheBuilder}
}

func buildLifecycleDeps(in WireInput) LifecycleDeps {
	deps := LifecycleDeps{
		EventPublisher:         in.EventPublisher,
		QuestionnairePublisher: in.QuestionnairePublisher,
		CacheSignalNotifier:    in.CacheSignalNotifier,
	}
	if infra := in.ScaleInfra; infra != nil {
		deps.QuestionnaireCatalog = quesApp.NewPublishedQuestionnaireCatalog(infra.QuestionnaireRepo)
	}
	return deps
}

type catalogCacheWireConfig struct {
	rulesetInfra.PublishedModelCacheConfig
	Notifier TypologyCacheSignalNotifier
}

func catalogCacheConfig(in WireInput) catalogCacheWireConfig {
	var notifier TypologyCacheSignalNotifier
	if n, ok := in.CacheSignalNotifier.(TypologyCacheSignalNotifier); ok {
		notifier = n
	}
	return catalogCacheWireConfig{
		PublishedModelCacheConfig: rulesetInfra.PublishedModelCacheConfig{
			Redis:    in.StaticRedisClient,
			Builder:  in.StaticCacheBuilder,
			Policy:   in.PublishedModelPolicy,
			Observer: in.CacheObserver,
		},
		Notifier: notifier,
	}
}

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
	publishedRepo := port.PublishedModelRepository(mongomodelcatalog.NewPublishedModelRepoAdapter(v2Repo))
	dualStore := modelcatalog.NewPublishedStore(v2Repo)
	publishedLister := port.PublishedModelLister(dualStore)
	if cacheCfg.Redis != nil && cacheCfg.Builder != nil {
		cached := cache.NewCachedPublishedModelStore(dualStore, cacheCfg.Redis, cacheCfg.Builder, cacheCfg.Policy, cacheCfg.Observer)
		publishedLister = cached
		publishedRepo = cache.NewInvalidatingPublishedModelRepository(publishedRepo, cached)
	}
	return CatalogDeps{
		PublishedLister:     publishedLister,
		ModelRepo:           draftRepo,
		PublishedRepo:       publishedRepo,
		NormRepo:            normRepo,
		QuestionnaireQuery:  questionnaireQuery,
		CacheSignalNotifier: cacheCfg.Notifier,
	}
}
