package modelcatalog

import (
	scaleLifecycle "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior/scale/lifecycle"
	appTypologyModel "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	mongoruleset "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/ruleset"
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
	IdentityService        *iam.IdentityService
	HotsetRecorder         cachetarget.HotsetRecorder
	CacheSignalNotifier    scaleLifecycle.CacheSignalNotifier
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
	surveyPorts := SurveyBootstrapPorts{}
	if in.QuestionnairePublisher != nil {
		surveyPorts.QuestionnairePublisher = in.QuestionnairePublisher
	}
	if infra := in.ScaleInfra; infra != nil {
		surveyPorts.QuestionnaireCatalog = quesApp.NewPublishedQuestionnaireCatalog(infra.QuestionnaireRepo)
	}
	return Bootstrap(BootstrapInput{
		Scale:    buildScaleDeps(in),
		Typology: buildTypologyDeps(in.MongoDB, in.MongoLimiter, in.QuestionnaireQuery, typologyCacheConfig(in)),
		Survey:   surveyPorts,
	})
}

func buildScaleDeps(in WireInput) ScaleDeps {
	deps := ScaleDeps{
		EventPublisher:      in.EventPublisher,
		RankRedisClient:     in.RankRedisClient,
		RankCacheBuilder:    in.RankCacheBuilder,
		IdentityService:     in.IdentityService,
		HotsetRecorder:      in.HotsetRecorder,
		CacheSignalNotifier: in.CacheSignalNotifier,
	}
	if infra := in.ScaleInfra; infra != nil {
		deps.Repo = infra.ScaleRepo
		deps.Reader = infra.ScaleReader
		deps.ListCache = infra.ScaleListCache
		deps.HotListCache = infra.ScaleHotListCache
		deps.QuestionnaireCatalog = quesApp.NewPublishedQuestionnaireCatalog(infra.QuestionnaireRepo)
	}
	if in.MongoDB != nil {
		mongoOpts := mongoBase.BaseRepositoryOptions{Limiter: in.MongoLimiter}
		v2Repo := mongomodelcatalog.NewRepository(in.MongoDB, mongoOpts)
		deps.RuleSetPublisher = rulesetInfra.NewScaleRuleSetPublisher(v2Repo)
	}
	return deps
}

type typologyCacheWireConfig struct {
	rulesetInfra.PublishedModelCacheConfig
	Notifier appTypologyModel.CacheSignalNotifier
}

func typologyCacheConfig(in WireInput) typologyCacheWireConfig {
	var notifier appTypologyModel.CacheSignalNotifier
	if n, ok := in.CacheSignalNotifier.(appTypologyModel.CacheSignalNotifier); ok {
		notifier = n
	}
	return typologyCacheWireConfig{
		PublishedModelCacheConfig: rulesetInfra.PublishedModelCacheConfig{
			Redis:    in.StaticRedisClient,
			Builder:  in.StaticCacheBuilder,
			Policy:   in.PublishedModelPolicy,
			Observer: in.CacheObserver,
		},
		Notifier: notifier,
	}
}

func buildTypologyDeps(
	mongoDB *mongo.Database,
	mongoLimiter backpressure.Acquirer,
	questionnaireQuery quesApp.QuestionnaireQueryService,
	cacheCfg typologyCacheWireConfig,
) TypologyDeps {
	if mongoDB == nil {
		return TypologyDeps{}
	}
	mongoOpts := mongoBase.BaseRepositoryOptions{Limiter: mongoLimiter}
	v2Repo := mongomodelcatalog.NewRepository(mongoDB, mongoOpts)
	draftRepo := mongomodelcatalog.NewDraftRepository(mongoDB, mongoOpts)
	publishedRepo := port.PublishedModelRepository(mongomodelcatalog.NewPublishedModelRepoAdapter(v2Repo))
	legacyRepo := mongoruleset.NewRepository(mongoDB, mongoOpts)
	dualStore := modelcatalog.NewDualStore(v2Repo, legacyRepo)
	publishedLister := port.PublishedModelLister(dualStore)
	algorithmLister := port.PublishedAlgorithmLister(dualStore)
	if cacheCfg.Redis != nil && cacheCfg.Builder != nil {
		cached := cache.NewCachedPublishedModelStore(dualStore, cacheCfg.Redis, cacheCfg.Builder, cacheCfg.Policy, cacheCfg.Observer)
		publishedLister = cached
		algorithmLister = cached
		publishedRepo = cache.NewInvalidatingPublishedModelRepository(publishedRepo, cached)
	}
	return TypologyDeps{
		PublishedLister:          publishedLister,
		PublishedAlgorithmLister: algorithmLister,
		ModelRepo:                draftRepo,
		PublishedRepo:            publishedRepo,
		QuestionnaireQuery:       questionnaireQuery,
		CacheSignalNotifier:      cacheCfg.Notifier,
	}
}
