package interpretation

import (
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/FangcunMount/component-base/pkg/errors"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	interpretationapp "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation"
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
	QueryService assessmentApp.ReportQueryService

	reader          evaluationreadmodel.ReportReader
	builderRegistry interpretationreporting.ReportBuilderRegistry
	durableSaver    interpretationreporting.ReportDurableSaver
	stateStore      interpretationapp.ReportStateStore
	generator       interpretationreporting.Generator
	outcomeService  interpretationapp.OutcomeReportService
	readyIndexer    *appEventing.PostCommitReadyIndexer
	readyIndex      *outboxready.Index
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
	reportRepo, err := mongoEval.NewReportRepositoryWithTopicResolver(deps.MongoDB, deps.TopicResolver, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report repository: %v", err)
	}
	module.stateStore = reportRepo
	module.reader = mongoEval.NewReportReadModel(deps.MongoDB, mongoOptions)
	module.QueryService = interpretationapp.NewReportQueryService(module.reader)

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
	module.durableSaver = interpretationreporting.NewTransactionalReportDurableSaver(mongoTxRunner, reportRepo, reportOutboxStore, module.readyIndexer)

	if len(deps.ModelDescriptors) > 0 {
		registry, err := buildReportBuilderRegistry(deps.ModelDescriptors)
		if err != nil {
			return nil, err
		}
		module.builderRegistry = registry
		generator, err := interpretationreporting.NewGenerator(registry)
		if err != nil {
			return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report generator: %v", err)
		}
		module.generator = generator
	}

	return module, nil
}

// BindOutcomeRepository completes the cross-storage Interpretation use case
// after Evaluation has installed its canonical outcome repository.
func (m *Module) BindOutcomeRepository(repo domainoutcome.Repository) error {
	if m == nil || repo == nil || m.stateStore == nil || m.generator == nil || m.durableSaver == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "interpretation outcome service dependencies are not configured")
	}
	m.outcomeService = interpretationapp.NewOutcomeReportService(repo, m.stateStore, m.generator, m.durableSaver)
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
