package evaluation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	typologyEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
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
	MySQLDB                        *gorm.DB
	MongoDB                        *mongo.Database
	EventPublisher                 event.EventPublisher
	RedisClient                    redis.UniversalClient
	CacheBuilder                   *keyspace.Builder
	QueryRedisClient               redis.UniversalClient
	QueryCacheBuilder              *keyspace.Builder
	MetaRedisClient                redis.UniversalClient
	AssessmentPolicy               cachepolicy.CachePolicy
	AssessmentListPolicy           cachepolicy.CachePolicy
	DisableEvaluationCache         bool
	Observer                       *observability.ComponentObserver
	TopicResolver                  eventcatalog.TopicResolver
	MySQLLimiter                   backpressure.Acquirer
	MongoLimiter                   backpressure.Acquirer
	AssessmentOutboxRelayBatchSize int
	TesteeAccessChecker            assessment.TesteeAccessChecker
	OpsHandle                      *cacheplane.Handle
	ReportStatusConfig             reportstatus.Config
	ScaleInfra                     *surveymod.ScaleInfra
	RuleSetCatalog                 rulesetport.RuleSetCatalog
	ModelDescriptors               []evaldomain.ModelDescriptor
	TypologyRegistry               typologyEvaluation.ModuleRegistry
	ReportPorts                    compose.ReportIntegrationPorts
}

// WireResult carries evaluation module and shared catalog side effects.
type WireResult struct {
	Module         *Module
	RuleSetCatalog rulesetport.RuleSetCatalog
}

// EnsureRuleSetCatalog builds the shared ruleset catalog used by evaluation and gRPC export.
func EnsureRuleSetCatalog(in RuleSetCatalogInput) (rulesetport.RuleSetCatalog, error) {
	if in.Existing != nil {
		return in.Existing, nil
	}
	if in.MongoDB == nil {
		return nil, fmt.Errorf("mongo database is nil")
	}
	mongoOpts := mongoBase.BaseRepositoryOptions{Limiter: in.MongoLimiter}
	var scaleSource rulesetInfra.ScaleBindingSource
	if in.ScaleInfra != nil && in.ScaleInfra.ScaleRepo != nil {
		scaleSource = evaluationinputInfra.NewRepositoryScaleBindingSource(in.ScaleInfra.ScaleRepo)
	}
	return rulesetInfra.NewCatalog(in.MongoDB, scaleSource, mongoOpts)
}

// RuleSetCatalogInput collects dependencies for ruleset catalog construction.
type RuleSetCatalogInput struct {
	MongoDB      *mongo.Database
	MongoLimiter backpressure.Acquirer
	ScaleInfra   *surveymod.ScaleInfra
	Existing     rulesetport.RuleSetCatalog
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

	catalog := in.RuleSetCatalog
	var inputResolver evaluationinput.Resolver
	var scaleCatalog evaluationinput.ScaleCatalog
	if infra := in.ScaleInfra; infra != nil {
		var err error
		if catalog == nil {
			catalog, err = EnsureRuleSetCatalog(RuleSetCatalogInput{
				MongoDB:      in.MongoDB,
				MongoLimiter: in.MongoLimiter,
				ScaleInfra:   infra,
			})
			if err != nil {
				return WireResult{}, err
			}
		}
		resolver, err := evaluationinputInfra.NewRepositoryResolver(
			infra.ScaleRepo,
			infra.AnswerSheetRepo,
			infra.QuestionnaireRepo,
			catalog,
			modelDescriptors,
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

	module, err := Bootstrap(BootstrapInput{
		MySQLDB:                        in.MySQLDB,
		MongoDB:                        in.MongoDB,
		InputResolver:                  inputResolver,
		ScaleCatalog:                   scaleCatalog,
		EventPublisher:                 in.EventPublisher,
		RedisClient:                    redisClient,
		CacheBuilder:                   in.CacheBuilder,
		AssessmentPolicy:               in.AssessmentPolicy,
		QueryRedisClient:               queryRedisClient,
		QueryCacheBuilder:              in.QueryCacheBuilder,
		AssessmentListPolicy:           in.AssessmentListPolicy,
		VersionStore:                   versionStore,
		Observer:                       in.Observer,
		TopicResolver:                  in.TopicResolver,
		MySQLLimiter:                   in.MySQLLimiter,
		MongoLimiter:                   in.MongoLimiter,
		AssessmentOutboxRelayBatchSize: in.AssessmentOutboxRelayBatchSize,
		TesteeAccessChecker:            in.TesteeAccessChecker,
		OpsHandle:                      in.OpsHandle,
		ReportStatusConfig:             in.ReportStatusConfig,
		ModelDescriptors:               modelDescriptors,
		TypologyRegistry:               in.TypologyRegistry,
		ReportReader:                   in.ReportPorts.Reader,
		ReportBuilderRegistry:          in.ReportPorts.BuilderRegistry,
		ReportDurableSaver:             in.ReportPorts.DurableSaver,
		PostCommitReadyIndexer:         in.ReportPorts.PostCommitReadyIndexer,
		OutboxReadyIndex:               in.ReportPorts.ReadyIndex,
	})
	if err != nil {
		return WireResult{}, err
	}
	return WireResult{Module: module, RuleSetCatalog: catalog}, nil
}
