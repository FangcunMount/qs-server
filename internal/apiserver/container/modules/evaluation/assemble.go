package evaluation

import (
	"context"
	"os"

	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	typologyEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
		evaluationResult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
		evaluationscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scoring"
		interpretationapp "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/internal/outboxruntime"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	assessmentCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	mysqlEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/redis/outboxready"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/waiter"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
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

	SubmissionService     assessmentApp.AssessmentSubmissionService
	ManagementService     assessmentApp.AssessmentManagementService
	ReportQueryService    assessmentApp.ReportQueryService
	ScoreQueryService     assessmentApp.ScoreQueryService
	WaitService           assessmentApp.AssessmentWaitService
	AccessQueryService    assessmentApp.AssessmentAccessQueryService
	ProtectedQueryService assessmentApp.AssessmentProtectedQueryService
	LatestRiskReader      evaluationreadmodel.LatestRiskReader
	AssessmentReader      evaluationreadmodel.AssessmentReader

	EvaluationService    execute.Service
	ReportStatusReporter *reportstatus.Reporter

	OutboxReadyIndex              *outboxready.Index
	AssessmentOutboxPendingLister outboxport.PendingEventRefLister
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
	TypologyRegistry                            typologyEvaluation.ModuleRegistry
	ReportReader                                evaluationreadmodel.ReportReader
	ReportBuilderRegistry                       evaluationResult.ReportBuilderRegistry
	ReportDurableSaver                          evaluationResult.ReportDurableSaver
	PublishedModelReader                        rulesetport.PublishedModelReader
	AsyncInterpretation                         bool
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
	}
	if err := module.wireEvaluationEngine(normalized, infra); err != nil {
		return nil, err
	}
	module.wireAssessmentApplications(normalized, infra)

	return module, nil
}

type evaluationInfra struct {
	assessmentRepo               assessment.Repository
	scoreRepo                    assessment.ScoreRepository
	assessmentReader             evaluationreadmodel.AssessmentReader
	latestRiskReader             evaluationreadmodel.LatestRiskReader
	scoreReader                  evaluationreadmodel.ScoreReader
	assessmentOutboxStore        *mysqlEventOutbox.Store
	txRunner                     apptransaction.Runner
	waiterRegistry               *waiter.WaiterRegistry
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
	infra.scoreReader = mysqlEval.NewScoreReadModel(normalized.MySQLDB, mysqlOptions)
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

	if normalized.InputResolver != nil {
		infra.waiterRegistry = waiter.NewWaiterRegistry(logger.L(context.Background()))
	}
	return infra, nil
}

func (m *Module) wireEvaluationEngine(normalized Deps, infra *evaluationInfra) error {
	var suggestionGenerator report.SuggestionGenerator

	if normalized.InputResolver != nil {
		reportStatusReporter, err := reportstatus.NewReporter(normalized.OpsHandle, normalized.ReportStatusConfig)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report status reporter: %v", err)
		}
		m.ReportStatusReporter = reportStatusReporter

		reportBuilder := report.NewDefaultInterpretReportBuilder(suggestionGenerator)
		wiringDeps := WiringDeps{
			ScaleReportBuilder: reportBuilder,
			ScaleScorer:        ruleengine.NewScaleFactorScorer(),
			TypologyRegistry:   normalized.TypologyRegistry,
		}
		descs := normalized.ModelDescriptors
		evaluators, err := MaterializeEvaluators(descs, wiringDeps)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to build evaluation evaluators: %v", err)
		}
		evaluatorRegistry, err := execute.NewEvaluatorRegistry(evaluators...)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize evaluation evaluator registry: %v", err)
		}
		scoreProjectors, err := evaluationResult.NewScoreProjectorRegistry(
			evaluationResult.NewScaleScoreProjector(infra.scoreRepo),
		)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize evaluation score projector registry: %v", err)
		}
		if normalized.ReportBuilderRegistry == nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "report builder registry is required when input resolver is configured")
		}
		resultWriter, err := evaluationResult.NewWriter(
			infra.assessmentRepo,
			scoreProjectors,
			normalized.ReportBuilderRegistry,
			normalized.ReportDurableSaver,
			evaluationResult.NewWaiterCompletionNotifier(infra.waiterRegistry),
			reportStatusReporter,
		)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize evaluation result writer: %v", err)
		}
		interpretationWriter, err := evaluationResult.NewInterpretationWriter(
			infra.assessmentRepo,
			scoreProjectors,
			normalized.ReportBuilderRegistry,
			normalized.ReportDurableSaver,
			evaluationResult.NewWaiterCompletionNotifier(infra.waiterRegistry),
			reportStatusReporter,
		)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize interpretation writer: %v", err)
		}
		scoringWriter := evaluationscoring.NewWriter(infra.assessmentRepo, scoreProjectors)
		interpretationService := interpretationapp.NewService(interpretationWriter)

		m.EvaluationService = execute.NewService(
			infra.assessmentRepo,
			normalized.InputResolver,
			resultWriter,
			execute.WithTransactionalOutbox(infra.txRunner, infra.assessmentOutboxStore),
			execute.WithPostCommitReadyIndexer(infra.postCommitReadyIndexer),
			execute.WithEvaluatorRegistry(evaluatorRegistry),
			execute.WithReportStatusReporter(reportStatusReporter),
			execute.WithScoringWriter(scoringWriter),
			execute.WithInterpretationService(interpretationService),
			execute.WithAsyncInterpretation(normalized.AsyncInterpretation),
		)
	}
	return nil
}

func (m *Module) wireAssessmentApplications(normalized Deps, infra *evaluationInfra) {
	creatorOpts := make([]assessment.AssessmentCreatorOption, 0, 1)
	if normalized.PublishedModelReader != nil {
		creatorOpts = append(creatorOpts, assessment.WithEvaluationModelValidator(
			assessmentApp.NewCompositeEvaluationModelValidator(
				assessmentApp.NewPersonalityEvaluationModelValidator(normalized.PublishedModelReader),
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
		m.SubmissionService = assessmentApp.NewSubmissionService(
			infra.assessmentRepo,
			infra.assessmentReader,
			assessmentCreator,
			infra.txRunner,
			infra.assessmentOutboxStore,
			listCache,
			assessmentApp.WithImmediateDispatcher(infra.assessmentImmediate),
		)
	} else {
		m.SubmissionService = assessmentApp.NewSubmissionService(
			infra.assessmentRepo,
			infra.assessmentReader,
			assessmentCreator,
			infra.txRunner,
			infra.assessmentOutboxStore,
			nil,
			assessmentApp.WithImmediateDispatcher(infra.assessmentImmediate),
		)
	}

	m.ManagementService = assessmentApp.NewManagementService(infra.assessmentRepo, infra.assessmentReader, infra.txRunner, infra.assessmentOutboxStore)
	if normalized.ReportReader != nil {
		m.ReportQueryService = assessmentApp.NewReportQueryService(normalized.ReportReader)
	}
	m.ScoreQueryService = assessmentApp.NewScoreQueryService(
		infra.scoreReader,
		infra.assessmentReader,
		normalized.ScaleCatalog,
	)

	m.WaitService = assessmentApp.NewWaitService(m.ManagementService, infra.waiterRegistry)
	m.AccessQueryService = assessmentApp.NewAssessmentAccessQueryService(
		m.ManagementService,
		normalized.TesteeAccessChecker,
	)
	m.ProtectedQueryService = assessmentApp.NewProtectedQueryService(
		m.ManagementService,
		m.ReportQueryService,
		m.ScoreQueryService,
		m.WaitService,
		m.AccessQueryService,
		infra.assessmentReader,
	)
	m.LatestRiskReader = infra.latestRiskReader
	m.AssessmentReader = infra.assessmentReader
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
		if deps.ReportBuilderRegistry == nil {
			return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "report builder registry is required when input resolver is configured")
		}
		if deps.ReportDurableSaver == nil {
			return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "report durable saver is required when input resolver is configured")
		}
	}
	if deps.ReportReader == nil {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "report reader is required")
	}
	if !deps.AsyncInterpretation && asyncInterpretationFromEnv() {
		deps.AsyncInterpretation = true
	}
	return deps, nil
}

func asyncInterpretationFromEnv() bool {
	switch os.Getenv("EVALUATION_ASYNC_INTERPRETATION") {
	case "1", "true", "TRUE", "yes", "YES":
		return true
	default:
		return false
	}
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
		Description: "评估模块（测评、得分、报告）",
	}
}
