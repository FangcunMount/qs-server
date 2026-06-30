package report

import (
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/FangcunMount/component-base/pkg/errors"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	typologyEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	evaluationResult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	ammod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/assessmentmodel"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/evaluation"
	mongoEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventoutbox"
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
	builderRegistry evaluationResult.ReportBuilderRegistry
	durableSaver    evaluationResult.ReportDurableSaver
	readyIndexer    *appEventing.PostCommitReadyIndexer
	readyIndex      *outboxready.Index
}

// Deps defines explicit constructor dependencies for the report module.
type Deps struct {
	MongoDB          *mongo.Database
	TopicResolver    eventcatalog.TopicResolver
	MongoLimiter     backpressure.Acquirer
	ModelDescriptors []evaldomain.ModelDescriptor
	TypologyRegistry typologyEvaluation.ModuleRegistry
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
	module.reader = mongoEval.NewReportReadModel(deps.MongoDB, mongoOptions)
	module.QueryService = assessmentApp.NewReportQueryService(module.reader)

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
	module.readyIndex = outboxready.NewIndex(opsClient)
	module.readyIndexer = appEventing.NewPostCommitReadyIndexer(module.readyIndex)
	mongoTxRunner := modtx.NewMongoRunner(deps.MongoDB)
	module.durableSaver = evaluationResult.NewTransactionalReportDurableSaver(mongoTxRunner, reportRepo, reportOutboxStore, module.readyIndexer)

	if len(deps.ModelDescriptors) > 0 {
		if deps.TypologyRegistry.Len() == 0 {
			return nil, errors.WithCode(code.ErrModuleInitializationFailed, "typology registry is required when model descriptors are configured")
		}
		registry, err := buildReportBuilderRegistry(deps.ModelDescriptors, deps.TypologyRegistry)
		if err != nil {
			return nil, err
		}
		module.builderRegistry = registry
	}

	return module, nil
}

// Reader exposes the report read model port.
func (m *Module) Reader() evaluationreadmodel.ReportReader {
	if m == nil {
		return nil
	}
	return m.reader
}

// BuilderRegistry exposes the evaluation report builder registry.
func (m *Module) BuilderRegistry() evaluationResult.ReportBuilderRegistry {
	if m == nil {
		return nil
	}
	return m.builderRegistry
}

// DurableSaver exposes the transactional report write port.
func (m *Module) DurableSaver() evaluationResult.ReportDurableSaver {
	if m == nil {
		return nil
	}
	return m.durableSaver
}

// PostCommitReadyIndexer exposes the shared post-commit outbox ready indexer.
func (m *Module) PostCommitReadyIndexer() *appEventing.PostCommitReadyIndexer {
	if m == nil {
		return nil
	}
	return m.readyIndexer
}

// ReadyIndex exposes the shared outbox ready index used by evaluation outbox relay.
func (m *Module) ReadyIndex() *outboxready.Index {
	if m == nil {
		return nil
	}
	return m.readyIndex
}

func buildReportBuilderRegistry(descs []evaldomain.ModelDescriptor, typologyRegistry typologyEvaluation.ModuleRegistry) (evaluationResult.ReportBuilderRegistry, error) {
	wiringDeps := ammod.ReportWiringDeps{
		ScaleReportBuilder: domainreport.NewDefaultInterpretReportBuilder(nil),
		TypologyRegistry:   typologyRegistry,
	}
	builders, err := ammod.MaterializeReportBuilders(descs, wiringDeps)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to build report builders: %v", err)
	}
	registry, err := evaluationResult.NewReportBuilderRegistry(builders...)
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
