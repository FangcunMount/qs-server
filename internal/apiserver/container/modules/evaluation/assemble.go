package evaluation

import (
	retrymodel "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	mysqlFailure "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluationfailure"
	mysqlSubmit "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluationsubmit"
	retrystore "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/retryschedule"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationtransaction"

	mysqlEventTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"

	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaluationintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	evaluationoutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomecommit "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/commit"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	evaluationscheduler "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scheduler"
	evaluationtestee "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	evaluationworker "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/worker"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	evaluationcache "github.com/FangcunMount/qs-server/internal/apiserver/cache/evaluation"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	mysqlEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationconsistency"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/workbenchreadmodel"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
)

// Module assembles evaluation application services.
type Module struct {
	IntakeService            evaluationintake.Service
	TesteeService            evaluationtestee.Service
	RuntimeStatusReader      *evaluationtestee.RuntimeStatusReader
	OperatorQuery            evaluationoperator.QueryService
	GovernedRetry            evaluationoperator.GovernedRetryService
	ScaleAnalysis            evaluationoperator.ScaleAnalysisService
	WorkerService            evaluationworker.Service
	OperatorExecutionService evaluationoperator.BatchExecutionService
	ConsistencyAuditService  evaluationscheduler.Service
	EvaluationLeaseRecoverer evaluationscheduler.LeaseRecoverer

	outcomeRepository         domainoutcome.Repository
	workbenchLatestRiskReader workbenchreadmodel.LatestRiskReader
}

// Deps defines explicit constructor dependencies for the evaluation module.
type Deps struct {
	MySQLDB                   *gorm.DB
	MongoDB                   *mongo.Database
	InputResolver             evaluationinput.Resolver
	RedisClient               redis.UniversalClient
	CacheBuilder              *keyspace.Builder
	CachePolicies             sharedcache.PolicyProvider
	Observer                  *observability.ComponentObserver
	MySQLLimiter              backpressure.Acquirer
	TesteeAccessChecker       evaluationoperator.AccessChecker
	ExecutionPaths            []modelcatalog.ExecutionPath
	RuntimeDescriptorRegistry *evalpipeline.RuntimeDescriptorRegistry
	PublishedModelReader      rulesetport.PublishedModelReader
	OutboxProfile             appEventing.ProfileBinding
	EventTransactionOwner     *mysqlEventTx.Committer
	TransactionRoutes         eventcatalog.TopicResolver
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

	module := &Module{outcomeRepository: infra.outcomeRepo}
	if err := module.wireEvaluationEngine(normalized, infra); err != nil {
		return nil, err
	}
	module.wireAssessmentApplications(normalized, infra)
	module.wireScheduler(infra)

	return module, nil
}

type evaluationInfra struct {
	transactionSubmitter  evaluationtransaction.Submitter
	transactionFailure    evaluationtransaction.FailureCommitter
	retrySchedules        *retrystore.Store
	assessmentRepo        assessment.Repository
	runRepo               evaluationrun.Repository
	outcomeRepo           domainoutcome.Repository
	scoreRepo             assessment.ScoreRepository
	assessmentReader      evaluationreadmodel.AssessmentReader
	latestRiskReader      workbenchreadmodel.LatestRiskReader
	scoreProjectionReader evaluationreadmodel.ScoreProjectionReader
	consistencyReader     evaluationconsistency.Reader
	assessmentOutboxStore appEventing.EventStager
	txRunner              apptransaction.Runner
	postCommit            appEventing.PostCommitDispatcher
	assessmentAccessCache evaluationtestee.AssessmentAccessCache
	assessmentDetailCache evaluationtestee.AssessmentDetailCache
}

func newEvaluationInfra(normalized Deps) (*evaluationInfra, error) {
	infra := &evaluationInfra{}
	mysqlOptions := mysql.BaseRepositoryOptions{Limiter: normalized.MySQLLimiter}
	baseAssessmentRepo := mysqlEval.NewAssessmentRepository(normalized.MySQLDB, mysqlOptions)
	if normalized.RedisClient != nil {
		caches := evaluationcache.NewAssessmentCaches(normalized.RedisClient, normalized.CacheBuilder, normalized.CachePolicies, normalized.Observer)
		infra.assessmentRepo = evaluationcache.NewInvalidatingAssessmentRepository(baseAssessmentRepo, caches)
		if capabilityEnabled(normalized.CachePolicies, cachepolicy.CapabilityEvaluationAssessmentAccess) {
			infra.assessmentAccessCache = caches
		}
		if capabilityEnabled(normalized.CachePolicies, cachepolicy.CapabilityEvaluationAssessmentDetail) {
			infra.assessmentDetailCache = caches
		}
	} else {
		infra.assessmentRepo = baseAssessmentRepo
	}

	assessmentReadModel := mysqlEval.NewAssessmentReadModel(normalized.MySQLDB, mysqlOptions)
	infra.assessmentReader = assessmentReadModel
	infra.latestRiskReader = assessmentReadModel
	infra.scoreRepo = mysqlEval.NewScoreRepository(normalized.MySQLDB, mysqlOptions)
	infra.scoreProjectionReader = mysqlEval.NewScoreProjectionReadModel(normalized.MySQLDB, mysqlOptions)
	infra.consistencyReader = mysqlEval.NewConsistencyReadModel(normalized.MySQLDB)
	infra.runRepo = mysqlEval.NewRunRepository(normalized.MySQLDB)
	infra.outcomeRepo = mysqlEval.NewOutcomeRepository(normalized.MySQLDB)
	infra.txRunner = modtx.NewMySQLRunner(normalized.MySQLDB)
	if normalized.EventTransactionOwner == nil {
		if normalized.OutboxProfile.Stager == nil || normalized.OutboxProfile.PostCommit == nil {
			return nil, errors.WithCode(code.ErrModuleInitializationFailed, "assessment MySQL event profile is required")
		}
		if _, ok := normalized.OutboxProfile.Stager.(outboxport.ReferencePreparer); !ok {
			return nil, errors.WithCode(code.ErrModuleInitializationFailed, "assessment MySQL event reference preparer is required")
		}
		infra.assessmentOutboxStore = normalized.OutboxProfile.Stager
		infra.postCommit = normalized.OutboxProfile.PostCommit

	}

	if owner := normalized.EventTransactionOwner; owner != nil {
		submitter, ok := infra.assessmentRepo.(assessment.PendingSubmissionRepository)
		if !ok {
			return nil, errors.WithCode(code.ErrModuleInitializationFailed, "original pending submission writer required")
		}
		var err error
		infra.retrySchedules, err = retrystore.New(normalized.MySQLDB, owner.Binding(), mysqlOptions)
		if err != nil {
			return nil, err
		}
		infra.transactionSubmitter, err = mysqlSubmit.New(normalized.MySQLDB, owner, submitter)
		if err != nil {
			return nil, err
		}
		infra.transactionFailure, err = mysqlFailure.New(normalized.MySQLDB, owner, normalized.TransactionRoutes, infra.assessmentRepo, infra.runRepo)
		if err != nil {
			return nil, err
		}
		infra.consistencyReader, err = mysqlEval.NewConsistencyReadModelWithTransactions(normalized.MySQLDB, owner.Binding())
		if err != nil {
			return nil, err
		}
	}

	return infra, nil
}

func (m *Module) wireEvaluationEngine(normalized Deps, infra *evaluationInfra) error {
	if normalized.InputResolver != nil {
		wiringDeps := WiringDeps{ScaleScorer: ruleengine.NewScaleFactorScorer()}
		if normalized.RuntimeDescriptorRegistry != nil {
			if err := evalruntime.AttachNativePipelines(normalized.RuntimeDescriptorRegistry, evalruntime.NativePipelineDeps{
				ScaleScorer:          evalruntime.MaterializeFactorScoringPipelineComponents(wiringDeps),
				FactorNorm:           evalruntime.MaterializeFactorNormPipelineComponents(wiringDeps),
				TaskPerformance:      evalruntime.MaterializeTaskPerformancePipelineComponents(wiringDeps),
				FactorClassification: evalruntime.MaterializeFactorClassificationPipelineComponents(wiringDeps),
			}); err != nil {
				return err
			}
		}
		scoreProjector := outcomescoring.NewAssessmentScoreProjector(infra.scoreRepo)
		evaluationCommitter := outcomecommit.NewCommitter(
			infra.txRunner,
			infra.assessmentRepo,
			infra.outcomeRepo,
			infra.runRepo,
			scoreProjector,
			infra.assessmentOutboxStore,
			infra.postCommit,
		)

		if owner := normalized.EventTransactionOwner; owner != nil {
			var err error
			evaluationCommitter, err = outcomecommit.NewTransactionalCommitter(owner, infra.assessmentRepo, infra.outcomeRepo, infra.runRepo, scoreProjector)
			if err != nil {
				return err
			}
		}
		engineOptions := []execute.EngineOption{
			execute.WithTransactionalOutbox(infra.txRunner, infra.assessmentOutboxStore),
			execute.WithPostCommitDispatcher(infra.postCommit),
			execute.WithRuntimeDescriptorRegistry(normalized.RuntimeDescriptorRegistry),
			execute.WithRunRepository(infra.runRepo),
			execute.WithEvaluationCommitter(evaluationCommitter),
		}
		if infra.transactionFailure != nil {
			engineOptions = append(engineOptions, execute.WithFailureTransactionCommitter(infra.transactionFailure))
		}
		engine := execute.NewEngine(
			infra.assessmentRepo,
			normalized.InputResolver,
			engineOptions...,
		)
		m.WorkerService = evaluationworker.NewService(engine, infra.assessmentRepo, infra.outcomeRepo, infra.runRepo)
		if reader, ok := infra.runRepo.(evaluationrun.ExpiredLeaseReader); ok {
			m.EvaluationLeaseRecoverer = evaluationscheduler.NewLeaseRecoverer(reader, m.WorkerService)
		}
		m.OperatorExecutionService = evaluationoperator.NewBatchExecutionService(infra.assessmentRepo, engine, normalized.TesteeAccessChecker)
	}
	return nil
}

func (m *Module) wireAssessmentApplications(normalized Deps, infra *evaluationInfra) {
	var modelValidator evaluationintake.EvaluationModelValidator
	if normalized.PublishedModelReader != nil {
		modelValidator = evaluationintake.NewPublishedEvaluationModelValidator(normalized.PublishedModelReader)
	}
	intakeOptions := []evaluationintake.Option{evaluationintake.WithPostCommitDispatcher(infra.postCommit)}
	if infra.transactionSubmitter != nil {
		intakeOptions = append(intakeOptions, evaluationintake.WithTransactionSubmitter(infra.transactionSubmitter))
	}
	m.IntakeService = evaluationintake.NewService(
		infra.assessmentRepo,
		modelValidator,
		infra.txRunner,
		infra.assessmentOutboxStore,
		intakeOptions...,
	)
	scoreFacts := evaluationoutcome.NewScoreFactReader(infra.outcomeRepo, infra.scoreProjectionReader)
	m.TesteeService = evaluationtestee.NewServiceWithCaches(infra.assessmentRepo, infra.assessmentReader, scoreFacts, infra.assessmentAccessCache, infra.assessmentDetailCache)
	m.RuntimeStatusReader = evaluationtestee.NewRuntimeStatusReader(m.TesteeService, infra.runRepo)
	m.OperatorQuery = evaluationoperator.NewQueryService(infra.assessmentRepo, infra.assessmentReader, normalized.TesteeAccessChecker, scoreFacts, infra.runRepo)
	var retryOptions []evaluationoperator.GovernedRetryOption
	if infra.retrySchedules != nil {
		retryOptions = append(retryOptions, evaluationoperator.WithRetrySchedules(infra.retrySchedules, retrymodel.Builder{Resolver: normalized.TransactionRoutes}))
	}
	m.GovernedRetry = evaluationoperator.NewGovernedRetryService(infra.assessmentRepo, infra.runRepo, infra.txRunner, infra.assessmentOutboxStore, normalized.TesteeAccessChecker, retryOptions...)
	m.ScaleAnalysis = evaluationoperator.NewScaleAnalysisService(m.OperatorQuery)
	m.workbenchLatestRiskReader = infra.latestRiskReader
}

func capabilityEnabled(provider sharedcache.PolicyProvider, capability sharedcache.Capability) bool {
	if provider == nil {
		return false
	}
	effective, ok := provider.Resolve(capability)
	return ok && effective.Enabled
}

func (m *Module) wireScheduler(infra *evaluationInfra) {
	m.ConsistencyAuditService = evaluationscheduler.NewService(infra.consistencyReader)
}

func normalizeDeps(deps Deps) (Deps, error) {
	if (deps.EventTransactionOwner == nil) != (deps.TransactionRoutes == nil) {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "transaction owner and explicit routes must be configured together")
	}
	if deps.EventTransactionOwner != nil && !deps.EventTransactionOwner.UsesPrimary(deps.MySQLDB) {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "evaluation transactions require the original host primary")
	}

	if deps.MySQLDB == nil {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "MySQL database connection is nil or invalid")
	}
	if deps.InputResolver != nil {
		if len(deps.ExecutionPaths) == 0 {
			return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "execution paths are required when input resolver is configured")
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
