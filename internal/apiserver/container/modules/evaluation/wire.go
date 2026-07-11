package evaluation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	evalregistry "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry"
	modelcatalogRuntime "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/pkg/event"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// WireInput carries composition-root inputs for evaluation module installation.
type WireInput struct {
	MySQLDB                                     *gorm.DB
	MongoDB                                     *mongo.Database
	EventPublisher                              event.EventPublisher
	RedisClient                                 redis.UniversalClient
	CacheBuilder                                *keyspace.Builder
	QueryRedisClient                            redis.UniversalClient
	QueryCacheBuilder                           *keyspace.Builder
	MetaRedisClient                             redis.UniversalClient
	AssessmentPolicy                            cachepolicy.CachePolicy
	AssessmentListPolicy                        cachepolicy.CachePolicy
	DisableEvaluationCache                      bool
	Observer                                    *observability.ComponentObserver
	TopicResolver                               eventcatalog.TopicResolver
	MySQLLimiter                                backpressure.Acquirer
	MongoLimiter                                backpressure.Acquirer
	AssessmentOutboxRelayBatchSize              int
	AssessmentOutboxRelayPublishWorkers         int
	AssessmentOutboxRelayImmediateMaxConcurrent int
	TesteeAccessChecker                         assessment.TesteeAccessChecker
	OpsHandle                                   *cacheplane.Handle
	ReportStatusConfig                          reportstatus.Config
	SurveyRuntimeInfra                          *surveymod.SurveyRuntimeInfra
	PublishedModelCatalog                       rulesetport.Catalog
	StaticRedisClient                           redis.UniversalClient
	StaticCacheBuilder                          *keyspace.Builder
	PublishedModelPolicy                        cachepolicy.CachePolicy
	ModelDescriptors                            []evaldomain.ModelDescriptor
	TypologyRegistry                            evalregistry.TypologyRegistry
	RuntimeDescriptorRegistry                   *evalpipeline.RuntimeDescriptorRegistry
	ReportPorts                                 compose.ReportIntegrationPorts
}

// WireResult carries evaluation module and shared catalog side effects.
type WireResult struct {
	Module                *Module
	PublishedModelCatalog rulesetport.Catalog
}

// EnsurePublishedModelCatalog builds the shared published-model catalog used by evaluation and gRPC export.
func EnsurePublishedModelCatalog(in PublishedModelCatalogInput) (rulesetport.Catalog, error) {
	catalog := in.Existing
	if catalog == nil {
		if in.MongoDB == nil {
			return nil, fmt.Errorf("mongo database is nil")
		}
		mongoOpts := mongoBase.BaseRepositoryOptions{Limiter: in.MongoLimiter}
		created, err := rulesetInfra.NewRuntimePublishedCatalog(in.MongoDB, mongoOpts, rulesetInfra.PublishedModelCacheConfig{
			Redis:    in.StaticRedisClient,
			Builder:  in.StaticCacheBuilder,
			Policy:   in.PublishedModelPolicy,
			Observer: in.Observer,
		})
		if err != nil {
			return nil, err
		}
		catalog = created
	}
	if trusted, ok := catalog.(*modelcatalogRuntime.TrustedRuntimeCatalog); ok {
		return trusted, nil
	}
	reader, ok := catalog.(rulesetport.PublishedModelReader)
	if !ok {
		return nil, fmt.Errorf("runtime published model catalog must implement PublishedModelReader")
	}
	lister, ok := catalog.(rulesetport.PublishedModelLister)
	if !ok {
		return nil, fmt.Errorf("runtime published model catalog must implement PublishedModelLister")
	}
	return modelcatalogRuntime.NewTrustedRuntimeCatalog(reader, lister), nil
}

// PublishedModelCatalogInput collects dependencies for published-model catalog construction.
type PublishedModelCatalogInput struct {
	MongoDB              *mongo.Database
	MongoLimiter         backpressure.Acquirer
	Existing             rulesetport.Catalog
	StaticRedisClient    redis.UniversalClient
	StaticCacheBuilder   *keyspace.Builder
	PublishedModelPolicy cachepolicy.CachePolicy
	Observer             *observability.ComponentObserver
}

// Wire builds and bootstraps the evaluation module from composition inputs.
func Wire(in WireInput) (WireResult, error) {
	modelDescriptors := in.ModelDescriptors
	if len(modelDescriptors) == 0 {
		return WireResult{}, fmt.Errorf("model descriptors are required")
	}
	if in.TypologyRegistry.Len() == 0 {
		return WireResult{}, fmt.Errorf("typology registry is required")
	}

	catalog := in.PublishedModelCatalog
	var inputResolver evaluationinput.Resolver
	var scaleCatalog evaluationinput.ScaleCatalog
	if infra := in.SurveyRuntimeInfra; infra != nil {
		var err error
		if catalog == nil {
			catalog, err = EnsurePublishedModelCatalog(PublishedModelCatalogInput{
				MongoDB:              in.MongoDB,
				MongoLimiter:         in.MongoLimiter,
				StaticRedisClient:    in.StaticRedisClient,
				StaticCacheBuilder:   in.StaticCacheBuilder,
				PublishedModelPolicy: in.PublishedModelPolicy,
				Observer:             in.Observer,
			})
			if err != nil {
				return WireResult{}, err
			}
		}
		resolver, err := evaluationinputInfra.NewRepositoryResolver(
			infra.AnswerSheetRepo,
			infra.QuestionnaireRepo,
			catalog,
			modelDescriptors,
			mongomodelcatalog.NewNormRepository(in.MongoDB, mongoBase.BaseRepositoryOptions{Limiter: in.MongoLimiter}),
		)
		if err != nil {
			return WireResult{}, fmt.Errorf("evaluation input resolver: %w", err)
		}
		inputResolver = resolver
		scaleCatalog = resolver
	}

	redisClient := in.RedisClient
	queryRedisClient := in.QueryRedisClient
	if in.DisableEvaluationCache {
		redisClient = nil
		queryRedisClient = nil
	}

	var versionStore cachequery.VersionTokenStore
	if queryRedisClient != nil {
		versionStore = cachequery.NewRedisVersionTokenStoreWithKindAndObserver(
			in.MetaRedisClient,
			string(cachepolicy.PolicyAssessmentList),
			in.Observer,
		)
	}

	var publishedModelReader rulesetport.PublishedModelReader
	if reader, ok := catalog.(rulesetport.PublishedModelReader); ok {
		publishedModelReader = reader
	}

	module, err := Bootstrap(BootstrapInput{
		MySQLDB:                             in.MySQLDB,
		MongoDB:                             in.MongoDB,
		InputResolver:                       inputResolver,
		ScaleCatalog:                        scaleCatalog,
		EventPublisher:                      in.EventPublisher,
		RedisClient:                         redisClient,
		CacheBuilder:                        in.CacheBuilder,
		AssessmentPolicy:                    in.AssessmentPolicy,
		QueryRedisClient:                    queryRedisClient,
		QueryCacheBuilder:                   in.QueryCacheBuilder,
		AssessmentListPolicy:                in.AssessmentListPolicy,
		VersionStore:                        versionStore,
		Observer:                            in.Observer,
		TopicResolver:                       in.TopicResolver,
		MySQLLimiter:                        in.MySQLLimiter,
		MongoLimiter:                        in.MongoLimiter,
		AssessmentOutboxRelayBatchSize:      in.AssessmentOutboxRelayBatchSize,
		AssessmentOutboxRelayPublishWorkers: in.AssessmentOutboxRelayPublishWorkers,
		AssessmentOutboxRelayImmediateMaxConcurrent: in.AssessmentOutboxRelayImmediateMaxConcurrent,
		TesteeAccessChecker:                         in.TesteeAccessChecker,
		OpsHandle:                                   in.OpsHandle,
		ReportStatusConfig:                          in.ReportStatusConfig,
		ModelDescriptors:                            modelDescriptors,
		TypologyRegistry:                            in.TypologyRegistry,
		RuntimeDescriptorRegistry:                   in.RuntimeDescriptorRegistry,
		ReportQueryService:                          in.ReportPorts.QueryService,
		PublishedModelReader:                        publishedModelReader,
	})
	if err != nil {
		return WireResult{}, err
	}
	return WireResult{Module: module, PublishedModelCatalog: catalog}, nil
}
