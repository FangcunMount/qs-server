package evaluation

import (
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	consistencyApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/consistency"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	outcomecommit "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/commit"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	evalregistry "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry"
	runqueryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runquery"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/internal/outboxruntime"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	assessmentCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	mysqlEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/redis/outboxready"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/outboxpriority"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// Module assembles evaluation application services.
type Module struct {
	AssessmentOutboxRelay        appEventing.OutboxRelay
	AssessmentOutboxStatusReader appEventing.NamedOutboxStatusReader

	IntakeService           assessmentApp.AnswerSheetAssessmentIntakeService
	TesteeQueryService      assessmentApp.TesteeAssessmentQueryService
	OperatorQueryService    assessmentApp.AssessmentOperatorQueryService
	OperatorRecoveryService assessmentApp.AssessmentOperatorRecoveryService
	WorkerResultReader      assessmentApp.AssessmentResultReader
	ScoreQueryService       assessmentApp.ScoreQueryService
	AccessQueryService      assessmentApp.AssessmentAccessQueryService
	ProtectedQueryService   assessmentApp.AssessmentProtectedQueryService
	RunQueryService         runqueryApp.Service
	LatestRiskReader        evaluationreadmodel.LatestRiskReader
	AssessmentReader        evaluationreadmodel.AssessmentReader

	WorkerExecutionService      execute.WorkerExecutionService
	OperatorExecutionService    execute.OperatorExecutionService
	ReportStatusReporter        *reportstatus.Reporter
	ConsistencyReconcileService consistencyApp.Service

	OutboxReadyIndex              *outboxready.Index
	AssessmentOutboxPendingLister outboxport.PendingEventRefLister
	outcomeRepository             domainoutcome.Repository
}

// Deps defines explicit constructor dependencies for the evaluation module.
type Deps struct {
	MySQLDB                                     *gorm.DB
	MongoDB                                     *mongo.Database
	InputResolver                               evaluationinput.Resolver
	ScaleCatalog                                evaluationinput.ScaleCatalog
	EventPublisher                              event.EventPublisher
	RedisClient                                 redis.UniversalClient
	CacheBuilder                                *keyspace.Builder
	AssessmentPolicy                            cachepolicy.CachePolicy
	QueryRedisClient                            redis.UniversalClient
	QueryCacheBuilder                           *keyspace.Builder
	AssessmentListPolicy                        cachepolicy.CachePolicy
	VersionStore                                cachequery.VersionTokenStore
	Observer                                    *observability.ComponentObserver
	TopicResolver                               eventcatalog.TopicResolver
	MySQLLimiter                                backpressure.Acquirer
	MongoLimiter                                backpressure.Acquirer
	AssessmentOutboxRelayBatchSize              int
	AssessmentOutboxRelayPublishWorkers         int
	AssessmentOutboxRelayImmediateMaxConcurrent int
	TesteeAccessChecker                         assessmentApp.TesteeAccessChecker
	OpsHandle                                   *cacheplane.Handle
	ReportStatusConfig                          reportstatus.Config
	ModelDescriptors                            []evaldomain.ModelDescriptor
	TypologyRegistry                            evalregistry.TypologyRegistry
	RuntimeDescriptorRegistry                   *evalpipeline.RuntimeDescriptorRegistry
	PublishedModelReader                        rulesetport.PublishedModelReader
}

// New assembles the evaluation module.
func New(deps Deps) (*Module, error) {
	normalized, err := normalizeDeps(deps)
	if err != nil {
		return nil, err
	}
	infra, err := newEvaluationInfra(normalized)
	if err != nil {
		return nil, err
	}

	module := &Module{
		AssessmentOutboxRelay:         infra.assessmentOutboxRelay,
		AssessmentOutboxStatusReader:  infra.assessmentOutboxStatusReader,
		OutboxReadyIndex:              infra.assessmentReadyIndex,
		AssessmentOutboxPendingLister: infra.assessmentOutboxStore,
		outcomeRepository:             infra.outcomeRepo,
	}
	if err := module.wireEvaluationEngine(normalized, infra); err != nil {
		return nil, err
	}
	module.wireAssessmentApplications(normalized, infra)
	module.wireConsistencyReconcile(normalized, infra)

	return module, nil
}

type evaluationInfra struct {
	assessmentRepo               assessment.Repository
	runRepo                      evaluationrun.Repository
	outcomeRepo                  domainoutcome.Repository
	scoreRepo                    assessment.ScoreRepository
	assessmentReader             evaluationreadmodel.AssessmentReader
	latestRiskReader             evaluationreadmodel.LatestRiskReader
	scoreProjectionReader        evaluationreadmodel.ScoreProjectionReader
	assessmentOutboxStore        *mysqlEventOutbox.Store
	txRunner                     apptransaction.Runner
	assessmentOutboxRelay        appEventing.OutboxRelay
	assessmentOutboxStatusReader appEventing.NamedOutboxStatusReader
	assessmentImmediate          *appEventing.ImmediateDispatcher
	assessmentReadyIndex         *outboxready.Index
	postCommitReadyIndexer       *appEventing.PostCommitReadyIndexer
}

func newEvaluationInfra(normalized Deps) (*evaluationInfra, error) {
	infra := &evaluationInfra{}
	mysqlOptions := mysql.BaseRepositoryOptions{Limiter: normalized.MySQLLimiter}
	baseAssessmentRepo := mysqlEval.NewAssessmentRepositoryWithTopicResolver(normalized.MySQLDB, normalized.TopicResolver, mysqlOptions)
	if normalized.RedisClient != nil {
		infra.assessmentRepo = assessmentCache.NewCachedAssessmentRepositoryWithBuilderPolicyAndObserver(baseAssessmentRepo, normalized.RedisClient, normalized.CacheBuilder, normalized.AssessmentPolicy, normalized.Observer)
	} else {
		infra.assessmentRepo = baseAssessmentRepo
	}

	assessmentReadModel := mysqlEval.NewAssessmentReadModel(normalized.MySQLDB, mysqlOptions)
	infra.assessmentReader = assessmentReadModel
	infra.latestRiskReader = assessmentReadModel
	infra.scoreRepo = mysqlEval.NewScoreRepository(normalized.MySQLDB, mysqlOptions)
	infra.scoreProjectionReader = mysqlEval.NewScoreProjectionReadModel(normalized.MySQLDB, mysqlOptions)
	infra.runRepo = mysqlEval.NewRunRepository(normalized.MySQLDB)
	infra.outcomeRepo = mysqlEval.NewOutcomeRepository(normalized.MySQLDB)
	infra.txRunner = modtx.NewMySQLRunner(normalized.MySQLDB)
	var opsClient redis.UniversalClient
	if normalized.OpsHandle != nil {
		opsClient = normalized.OpsHandle.Client
	}
	assessmentReadyIndex := outboxready.NewIndex(opsClient, outboxready.StoreAssessmentMySQLOutbox)
	infra.assessmentReadyIndex = assessmentReadyIndex
	mysqlPriorityOpts := []mysqlEventOutbox.StoreOption{mysqlEventOutbox.WithPriorityTiers(outboxpriority.ClaimOrder(nil, nil))}
	assessmentOutboxStore := mysqlEventOutbox.NewStoreWithTopicResolver(normalized.MySQLDB, normalized.TopicResolver, mysqlPriorityOpts...)
	infra.assessmentOutboxStore = assessmentOutboxStore
	outboxRuntime := outboxruntime.Build(outboxruntime.Spec{
		Name:                    "assessment-mysql-outbox",
		Store:                   assessmentOutboxStore,
		Publisher:               normalized.EventPublisher,
		ReadyIndex:              assessmentReadyIndex,
		BatchSize:               normalized.AssessmentOutboxRelayBatchSize,
		PublishWorkers:          normalized.AssessmentOutboxRelayPublishWorkers,
		ImmediateMaxConcurrent:  normalized.AssessmentOutboxRelayImmediateMaxConcurrent,
		ImmediateEnabled:        true,
		RequireDurablePublisher: true,
	})
	infra.postCommitReadyIndexer = outboxRuntime.PostCommitReadyIndexer
	infra.assessmentImmediate = outboxRuntime.Immediate
	infra.assessmentOutboxRelay = outboxRuntime.Relay
	infra.assessmentOutboxStatusReader = appEventing.NamedOutboxStatusReader{
		Name:   "assessment-mysql-outbox",
		Reader: assessmentOutboxStore,
	}

	return infra, nil
}

func (m *Module) wireEvaluationEngine(normalized Deps, infra *evaluationInfra) error {
	if normalized.InputResolver != nil {
		reportStatusReporter, err := reportstatus.NewReporter(normalized.OpsHandle, normalized.ReportStatusConfig)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report status reporter: %v", err)
		}
		m.ReportStatusReporter = reportStatusReporter

		wiringDeps := WiringDeps{
			ScaleScorer:      ruleengine.NewScaleFactorScorer(),
			TypologyRegistry: normalized.TypologyRegistry,
		}
		familyEvaluators, err := MaterializeFamilyEvaluators(wiringDeps)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to build family evaluators: %v", err)
		}
		if normalized.RuntimeDescriptorRegistry != nil {
			evalruntime.AttachNativePipelines(normalized.RuntimeDescriptorRegistry, evalruntime.NativePipelineDeps{
				ScaleScorer:          evalruntime.MaterializeFactorScoringPipelineComponents(wiringDeps),
				FactorNorm:           evalruntime.MaterializeFactorNormPipelineComponents(wiringDeps),
				TaskPerformance:      evalruntime.MaterializeTaskPerformancePipelineComponents(wiringDeps),
				FactorClassification: evalruntime.MaterializeFactorClassificationPipelineComponents(wiringDeps),
			})
		}
		evaluatorRegistry, err := execute.NewEvaluatorRegistry()
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize evaluation evaluator registry: %v", err)
		}
		scoreProjector := outcomescoring.NewAssessmentScoreProjector(infra.scoreRepo)
		evaluationCommitter := outcomecommit.NewCommitter(
			infra.txRunner,
			infra.assessmentRepo,
			infra.outcomeRepo,
			infra.runRepo,
			scoreProjector,
			infra.assessmentOutboxStore,
			infra.postCommitReadyIndexer,
		)
		engine := execute.NewEngine(
			infra.assessmentRepo,
			normalized.InputResolver,
			execute.WithTransactionalOutbox(infra.txRunner, infra.assessmentOutboxStore),
			execute.WithPostCommitReadyIndexer(infra.postCommitReadyIndexer),
			execute.WithEvaluatorRegistry(evaluatorRegistry),
			execute.WithRuntimeDescriptorRegistry(normalized.RuntimeDescriptorRegistry),
			execute.WithFamilyEvaluators(familyEvaluators),
			execute.WithRunRepository(infra.runRepo),
			execute.WithReportStatusReporter(reportStatusReporter),
			execute.WithEvaluationCommitter(evaluationCommitter),
		)
		m.WorkerExecutionService = engine
		m.OperatorExecutionService = engine
	}
	return nil
}

func (m *Module) wireAssessmentApplications(normalized Deps, infra *evaluationInfra) {
	creatorOpts := make([]assessment.AssessmentCreatorOption, 0, 1)
	if normalized.PublishedModelReader != nil {
		creatorOpts = append(creatorOpts, assessment.WithEvaluationModelValidator(
			assessmentApp.NewCompositeEvaluationModelValidator(
				assessmentApp.NewTypologyEvaluationModelValidator(normalized.PublishedModelReader),
			),
		))
	}
	assessmentCreator := assessment.NewDefaultAssessmentCreator(creatorOpts...)
	if normalized.QueryRedisClient != nil && normalized.VersionStore != nil {
		listCache := cachequery.NewMyAssessmentListCacheWithBuilderPolicyAndObserver(
			cacheentry.NewRedisCache(normalized.QueryRedisClient),
			normalized.VersionStore,
			normalized.QueryCacheBuilder,
			normalized.AssessmentListPolicy,
			normalized.Observer,
		)
		m.IntakeService = assessmentApp.NewAnswerSheetAssessmentIntakeService(
			infra.assessmentRepo,
			assessmentCreator,
			infra.txRunner,
			infra.assessmentOutboxStore,
			listCache,
			assessmentApp.WithIntakeImmediateDispatcher(infra.assessmentImmediate),
		)
		m.TesteeQueryService = assessmentApp.NewTesteeAssessmentQueryService(infra.assessmentRepo, infra.assessmentReader, listCache)
	} else {
		m.IntakeService = assessmentApp.NewAnswerSheetAssessmentIntakeService(
			infra.assessmentRepo,
			assessmentCreator,
			infra.txRunner,
			infra.assessmentOutboxStore,
			nil,
			assessmentApp.WithIntakeImmediateDispatcher(infra.assessmentImmediate),
		)
		m.TesteeQueryService = assessmentApp.NewTesteeAssessmentQueryService(infra.assessmentRepo, infra.assessmentReader, nil)
	}
	m.OperatorQueryService = assessmentApp.NewAssessmentOperatorQueryService(infra.assessmentRepo, infra.assessmentReader)
	m.OperatorRecoveryService = assessmentApp.NewAssessmentOperatorRecoveryService(infra.assessmentRepo, infra.txRunner, infra.assessmentOutboxStore)
	m.WorkerResultReader = m.OperatorQueryService
	m.ScoreQueryService = assessmentApp.NewScoreQueryService(
		infra.outcomeRepo,
		infra.scoreProjectionReader,
		infra.assessmentReader,
		normalized.ScaleCatalog,
	)

	m.AccessQueryService = assessmentApp.NewAssessmentAccessQueryService(
		m.OperatorQueryService,
		normalized.TesteeAccessChecker,
	)
	m.RunQueryService = runqueryApp.NewService(infra.runRepo)
	m.ProtectedQueryService = assessmentApp.NewProtectedQueryService(
		m.OperatorQueryService,
		m.ScoreQueryService,
		m.AccessQueryService,
		infra.assessmentReader,
		m.RunQueryService,
	)
	m.LatestRiskReader = infra.latestRiskReader
	m.AssessmentReader = infra.assessmentReader
}

func (m *Module) wireConsistencyReconcile(normalized Deps, infra *evaluationInfra) {
	reconciler := consistencyApp.NewReconciler(
		infra.assessmentRepo,
		consistencyApp.OutcomeExistenceChecker{
			Repository: infra.outcomeRepo,
		},
		infra.outcomeRepo,
		infra.assessmentRepo,
	)
	m.ConsistencyReconcileService = consistencyApp.NewReconcileService(reconciler, infra.assessmentReader)
}

func normalizeDeps(deps Deps) (Deps, error) {
	if deps.MySQLDB == nil {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "MySQL database connection is nil or invalid")
	}
	if deps.MongoDB == nil {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "MongoDB database connection is nil or invalid")
	}
	if deps.EventPublisher == nil {
		deps.EventPublisher = event.NewNopEventPublisher()
	}
	if deps.InputResolver != nil {
		if len(deps.ModelDescriptors) == 0 {
			return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "model descriptors are required when input resolver is configured")
		}
		if deps.TypologyRegistry.Len() == 0 {
			return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "typology registry is required when input resolver is configured")
		}
	}
	return deps, nil
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
		Description: "测评评分模块（Assessment、EvaluationRun、Outcome 与 score 投影）",
	}
}
