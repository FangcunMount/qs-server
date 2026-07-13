package evaluation

import (
	"fmt"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"

	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	evaluationcache "github.com/FangcunMount/qs-server/internal/apiserver/cache/evaluation"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongomodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/workbenchreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	querycache "github.com/FangcunMount/qs-server/internal/pkg/cache/query"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/pkg/event"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// WireInput carries composition-root inputs for evaluation module installation.
type WireInput struct {
	MySQLDB                   *gorm.DB
	MongoDB                   *mongo.Database
	EventPublisher            event.EventPublisher
	RedisClient               redis.UniversalClient
	CacheBuilder              *keyspace.Builder
	QueryRedisClient          redis.UniversalClient
	QueryCacheBuilder         *keyspace.Builder
	MetaRedisClient           redis.UniversalClient
	AssessmentPolicy          cachepolicy.CachePolicy
	AssessmentListPolicy      cachepolicy.CachePolicy
	Observer                  *observability.ComponentObserver
	MySQLLimiter              backpressure.Acquirer
	MongoLimiter              backpressure.Acquirer
	TesteeAccessChecker       evaluationoperator.AccessChecker
	SurveyRuntimeInfra        *surveymod.SurveyRuntimeInfra
	PublishedModelCatalog     rulesetport.Catalog
	RuntimeDescriptorRegistry *evalpipeline.RuntimeDescriptorRegistry
	OutboxProfile             appEventing.ProfileBinding
}

// WireResult carries evaluation module and shared catalog side effects.
type WireResult struct {
	Module                    *Module
	WorkbenchLatestRiskReader workbenchreadmodel.LatestRiskReader
}

// Wire builds and bootstraps the evaluation module from composition inputs.
func Wire(in WireInput) (WireResult, error) {
	if in.PublishedModelCatalog == nil {
		return WireResult{}, fmt.Errorf("modelcatalog published model catalog is required")
	}
	executionPaths, err := evalruntime.ExecutionPathsFromRegistry(in.RuntimeDescriptorRegistry)
	if err != nil {
		return WireResult{}, fmt.Errorf("evaluation runtime registry: %w", err)
	}
	executionPaths = evalruntime.FilterExecutablePaths(executionPaths)

	catalog := in.PublishedModelCatalog
	var inputResolver evaluationinput.Resolver
	var scaleCatalog evaluationinput.ScaleCatalog
	if infra := in.SurveyRuntimeInfra; infra != nil {
		resolver, err := evaluationinputInfra.NewRepositoryResolver(
			infra.AnswerSheetRepo,
			infra.QuestionnaireRepo,
			catalog,
			executionPaths,
			mongomodelcatalog.NewNormRepository(in.MongoDB, mongoBase.BaseRepositoryOptions{Limiter: in.MongoLimiter}),
		)
		if err != nil {
			return WireResult{}, fmt.Errorf("evaluation input resolver: %w", err)
		}
		inputResolver = resolver
		scaleCatalog = resolver
	}

	var versionStore querycache.VersionTokenStore
	if in.QueryRedisClient != nil {
		versionStore = evaluationcache.NewVersionTokenStore(in.MetaRedisClient, in.Observer)
	}

	var publishedModelReader rulesetport.PublishedModelReader
	if reader, ok := catalog.(rulesetport.PublishedModelReader); ok {
		publishedModelReader = reader
	}

	module, err := Bootstrap(BootstrapInput{
		MySQLDB:                   in.MySQLDB,
		InputResolver:             inputResolver,
		ScaleCatalog:              scaleCatalog,
		EventPublisher:            in.EventPublisher,
		RedisClient:               in.RedisClient,
		CacheBuilder:              in.CacheBuilder,
		AssessmentPolicy:          in.AssessmentPolicy,
		QueryRedisClient:          in.QueryRedisClient,
		QueryCacheBuilder:         in.QueryCacheBuilder,
		AssessmentListPolicy:      in.AssessmentListPolicy,
		VersionStore:              versionStore,
		Observer:                  in.Observer,
		MySQLLimiter:              in.MySQLLimiter,
		TesteeAccessChecker:       in.TesteeAccessChecker,
		ExecutionPaths:            executionPaths,
		RuntimeDescriptorRegistry: in.RuntimeDescriptorRegistry,
		PublishedModelReader:      publishedModelReader,
		OutboxProfile:             in.OutboxProfile,
	})
	if err != nil {
		return WireResult{}, err
	}
	return WireResult{Module: module, WorkbenchLatestRiskReader: module.workbenchLatestRiskReader}, nil
}
