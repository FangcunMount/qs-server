package interpretation

import (
	"time"

	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/FangcunMount/component-base/pkg/errors"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	interpretationapp "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation"
	interpretationgeneration "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/generation"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	reportmaterialize "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/materialize"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventoutbox"
	mongoEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/redis/outboxready"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/outboxpriority"
)

// Module assembles report read/query, builder-registry, and durable write capabilities.
type Module struct {
	QueryService          interpretationapp.ReportQueryService
	LifecycleQueryService interpretationapp.LifecycleQueryService

	reader             evaluationreadmodel.ReportReader
	builderRegistry    interpretationreporting.ReportBuilderRegistry
	generationExecutor interpretationgeneration.Executor
	generationRepo     *mongoEval.GenerationRepository
	runRepo            *mongoEval.RunRepository
	artifactRepo       *mongoEval.ArtifactRepository
	outcomeService     interpretationapp.OutcomeReportService
	readyIndexer       *appEventing.PostCommitReadyIndexer
	readyIndex         *outboxready.Index
}

// Deps defines explicit constructor dependencies for the report module.
type Deps struct {
	MongoDB          *mongo.Database
	TopicResolver    eventcatalog.TopicResolver
	MongoLimiter     backpressure.Acquirer
	ModelDescriptors []evaldomain.ModelDescriptor
	OpsHandle        *cacheplane.Handle
}

// New assembles the report module.
func New(deps Deps) (*Module, error) {
	if deps.MongoDB == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "MongoDB database connection is nil or invalid")
	}

	module := &Module{}
	mongoOptions := mongoBase.BaseRepositoryOptions{Limiter: deps.MongoLimiter}
	module.reader = mongoEval.NewReportReadModel(deps.MongoDB, mongoOptions)
	module.QueryService = interpretationapp.NewReportQueryService(module.reader)
	generationRepo, err := mongoEval.NewGenerationRepository(deps.MongoDB, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report generation repository: %v", err)
	}
	module.generationRepo = generationRepo
	runRepo, err := mongoEval.NewRunRepository(deps.MongoDB, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize interpretation run repository: %v", err)
	}
	module.runRepo = runRepo
	artifactRepo, err := mongoEval.NewArtifactRepository(deps.MongoDB, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize interpretation artifact repository: %v", err)
	}
	module.artifactRepo = artifactRepo

	priorityOpts := []mongoEventOutbox.StoreOption{
		mongoEventOutbox.WithPriorityTiers(outboxpriority.ClaimOrder(nil, nil)),
	}
	if deps.MongoLimiter != nil {
		priorityOpts = append(priorityOpts, mongoEventOutbox.WithLimiter(deps.MongoLimiter))
	}
	reportOutboxStore, err := mongoEventOutbox.NewStoreWithTopicResolver(deps.MongoDB, deps.TopicResolver, priorityOpts...)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report outbox store: %v", err)
	}
	var opsClient redis.UniversalClient
	if deps.OpsHandle != nil {
		opsClient = deps.OpsHandle.Client
	}
	module.readyIndex = outboxready.NewIndex(opsClient, outboxready.StoreMongoDomainEvents)
	module.readyIndexer = appEventing.NewPostCommitReadyIndexer(module.readyIndex)
	mongoTxRunner := modtx.NewMongoRunner(deps.MongoDB)
	if len(deps.ModelDescriptors) > 0 {
		registry, err := buildReportBuilderRegistry(deps.ModelDescriptors)
		if err != nil {
			return nil, err
		}
		module.builderRegistry = registry
		starter, err := interpretationgeneration.NewStarter(mongoTxRunner, module.generationRepo, module.runRepo, module.artifactRepo, 5*time.Minute)
		if err != nil {
			return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report generation starter: %v", err)
		}
		committer, err := interpretationgeneration.NewInterpretationCommitter(mongoTxRunner, module.generationRepo, module.runRepo, module.artifactRepo, reportOutboxStore, module.readyIndexer)
		if err != nil {
			return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize interpretation committer: %v", err)
		}
		executor, err := interpretationgeneration.NewExecutor(starter, registry, committer)
		if err != nil {
			return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize interpretation generation executor: %v", err)
		}
		module.generationExecutor = executor
	}

	return module, nil
}

// BindOutcomeRepository completes the cross-storage Interpretation use case
// after Evaluation has installed its canonical outcome repository.
func (m *Module) BindOutcomeRepository(repo domainoutcome.Repository) error {
	if m == nil || repo == nil || m.generationExecutor == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "interpretation outcome service dependencies are not configured")
	}
	m.outcomeService = interpretationapp.NewOutcomeReportService(repo, m.generationExecutor)
	m.LifecycleQueryService = interpretationapp.NewLifecycleQueryService(repo, m.generationRepo, m.runRepo, m.artifactRepo)
	return nil
}

func (m *Module) OutcomeService() interpretationapp.OutcomeReportService {
	if m == nil {
		return nil
	}
	return m.outcomeService
}

func buildReportBuilderRegistry(descs []evaldomain.ModelDescriptor) (interpretationreporting.ReportBuilderRegistry, error) {
	builders, err := reportmaterialize.ReportBuilders(descs, domainreport.NewDefaultInterpretReportBuilder(nil))
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to build report builders: %v", err)
	}
	expanded := interpretationreporting.ExpandAudienceProfileBuilders(builders...)
	registry, err := interpretationreporting.NewReportBuilderRegistry(expanded...)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report builder registry: %v", err)
	}
	return registry, nil
}

// Cleanup releases module resources.
func (m *Module) Cleanup() error {
	return nil
}

// CheckHealth verifies module health.
func (m *Module) CheckHealth() error {
	return nil
}

// ModuleInfo returns module metadata.
func (m *Module) ModuleInfo() modules.ModuleInfo {
	return modules.ModuleInfo{
		Name:        string(Name),
		Version:     "1.0.0",
		Description: "解读报告模块",
	}
}
