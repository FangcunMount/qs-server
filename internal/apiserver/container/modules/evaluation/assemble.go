package evaluation

import (
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/event"
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
	evaluationcache "github.com/FangcunMount/qs-server/internal/apiserver/cache/evaluation"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	mysqlEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/workbenchreadmodel"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	querycache "github.com/FangcunMount/qs-server/internal/pkg/cache/query"
	redisstore "github.com/FangcunMount/qs-server/internal/pkg/cache/redis"
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
	OperatorQuery            evaluationoperator.QueryService
	GovernedRetry            evaluationoperator.GovernedRetryService
	ScaleAnalysis            evaluationoperator.ScaleAnalysisService
	WorkerService            evaluationworker.Service
	OperatorExecutionService evaluationoperator.BatchExecutionService
	SchedulerService         evaluationscheduler.Service
	LeaseRecoverer           evaluationscheduler.LeaseRecoverer

	outcomeRepository         domainoutcome.Repository
	workbenchLatestRiskReader workbenchreadmodel.LatestRiskReader
}

// Deps defines explicit constructor dependencies for the evaluation module.
type Deps struct {
	MySQLDB                    *gorm.DB
	InputResolver              evaluationinput.Resolver
	ScaleCatalog               evaluationinput.ScaleCatalog
	EventPublisher             event.EventPublisher
	RedisClient                redis.UniversalClient
	CacheBuilder               *keyspace.Builder
	CachePolicies              sharedcache.PolicyProvider
	QueryRedisClient           redis.UniversalClient
	QueryCacheBuilder          *keyspace.Builder
	VersionStore               querycache.VersionTokenStore
	Observer                   *observability.ComponentObserver
	MySQLLimiter               backpressure.Acquirer
	TesteeAccessChecker        evaluationoperator.AccessChecker
	ExecutionPaths             []modelcatalog.ExecutionPath
	RuntimeDescriptorRegistry  *evalpipeline.RuntimeDescriptorRegistry
	PublishedModelReader       rulesetport.PublishedModelReader
	ActivePublishedModelReader rulesetport.ActivePublishedModelReader
	OutboxProfile              appEventing.ProfileBinding
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
	assessmentRepo           assessment.Repository
	runRepo                  evaluationrun.Repository
	outcomeRepo              domainoutcome.Repository
	scoreRepo                assessment.ScoreRepository
	assessmentReader         evaluationreadmodel.AssessmentReader
	submittedCandidateReader evaluationscheduler.SubmittedCandidateReader
	latestRiskReader         workbenchreadmodel.LatestRiskReader
	scoreProjectionReader    evaluationreadmodel.ScoreProjectionReader
	assessmentOutboxStore    appEventing.EventStager
	txRunner                 apptransaction.Runner
	postCommit               appEventing.PostCommitDispatcher
}

func newEvaluationInfra(normalized Deps) (*evaluationInfra, error) {
	infra := &evaluationInfra{}
	mysqlOptions := mysql.BaseRepositoryOptions{Limiter: normalized.MySQLLimiter}
	baseAssessmentRepo := mysqlEval.NewAssessmentRepository(normalized.MySQLDB, mysqlOptions)
	if normalized.RedisClient != nil {
		infra.assessmentRepo = evaluationcache.NewCachedAssessmentRepositoryWithBuilderProviderAndObserver(baseAssessmentRepo, normalized.RedisClient, normalized.CacheBuilder, normalized.CachePolicies, normalized.Observer)
	} else {
		infra.assessmentRepo = baseAssessmentRepo
	}

	assessmentReadModel := mysqlEval.NewAssessmentReadModel(normalized.MySQLDB, mysqlOptions)
	infra.assessmentReader = assessmentReadModel
	infra.submittedCandidateReader = assessmentReadModel
	infra.latestRiskReader = assessmentReadModel
	infra.scoreRepo = mysqlEval.NewScoreRepository(normalized.MySQLDB, mysqlOptions)
	infra.scoreProjectionReader = mysqlEval.NewScoreProjectionReadModel(normalized.MySQLDB, mysqlOptions)
	infra.runRepo = mysqlEval.NewRunRepository(normalized.MySQLDB)
	infra.outcomeRepo = mysqlEval.NewOutcomeRepository(normalized.MySQLDB)
	infra.txRunner = modtx.NewMySQLRunner(normalized.MySQLDB)
	if normalized.OutboxProfile.Stager == nil || normalized.OutboxProfile.PostCommit == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "assessment MySQL event profile is required")
	}
	infra.assessmentOutboxStore = normalized.OutboxProfile.Stager
	infra.postCommit = normalized.OutboxProfile.PostCommit

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
		engine := execute.NewEngine(
			infra.assessmentRepo,
			normalized.InputResolver,
			execute.WithTransactionalOutbox(infra.txRunner, infra.assessmentOutboxStore),
			execute.WithPostCommitDispatcher(infra.postCommit),
			execute.WithRuntimeDescriptorRegistry(normalized.RuntimeDescriptorRegistry),
			execute.WithRunRepository(infra.runRepo),
			execute.WithEvaluationCommitter(evaluationCommitter),
		)
		m.WorkerService = evaluationworker.NewService(engine, infra.assessmentRepo, infra.outcomeRepo, infra.runRepo)
		if reader, ok := infra.runRepo.(evaluationrun.ExpiredLeaseReader); ok {
			m.LeaseRecoverer = evaluationscheduler.NewLeaseRecoverer(reader, m.WorkerService)
		}
		m.OperatorExecutionService = evaluationoperator.NewBatchExecutionService(infra.assessmentRepo, engine, normalized.TesteeAccessChecker)
	}
	return nil
}

func (m *Module) wireAssessmentApplications(normalized Deps, infra *evaluationInfra) {
	var modelValidator evaluationintake.EvaluationModelValidator
	if normalized.ActivePublishedModelReader != nil || normalized.PublishedModelReader != nil {
		modelValidator = evaluationintake.NewCompositeEvaluationModelValidator(
			evaluationintake.NewPublishedEvaluationModelValidator(
				normalized.ActivePublishedModelReader,
				normalized.PublishedModelReader,
			),
		)
	}
	if normalized.QueryRedisClient != nil && normalized.VersionStore != nil {
		listCache := evaluationcache.NewMyAssessmentListCacheWithBuilderProviderAndObserver(
			redisstore.NewStore(normalized.QueryRedisClient),
			normalized.VersionStore,
			normalized.QueryCacheBuilder,
			normalized.CachePolicies,
			normalized.Observer,
		)
		m.IntakeService = evaluationintake.NewService(
			infra.assessmentRepo,
			modelValidator,
			infra.txRunner,
			infra.assessmentOutboxStore,
			listCache,
			evaluationintake.WithPostCommitDispatcher(infra.postCommit),
		)
	} else {
		m.IntakeService = evaluationintake.NewService(
			infra.assessmentRepo,
			modelValidator,
			infra.txRunner,
			infra.assessmentOutboxStore,
			nil,
			evaluationintake.WithPostCommitDispatcher(infra.postCommit),
		)
	}
	scoreFacts := evaluationoutcome.NewScoreFactReader(infra.outcomeRepo, infra.scoreProjectionReader)
	m.TesteeService = evaluationtestee.NewService(infra.assessmentRepo, infra.assessmentReader, scoreFacts)
	m.OperatorQuery = evaluationoperator.NewQueryService(infra.assessmentRepo, infra.assessmentReader, normalized.TesteeAccessChecker, scoreFacts, infra.runRepo)
	m.GovernedRetry = evaluationoperator.NewGovernedRetryService(infra.assessmentRepo, infra.runRepo, infra.txRunner, infra.assessmentOutboxStore, normalized.TesteeAccessChecker)
	m.ScaleAnalysis = evaluationoperator.NewScaleAnalysisService(m.OperatorQuery)
	m.workbenchLatestRiskReader = infra.latestRiskReader
}

func (m *Module) wireScheduler(infra *evaluationInfra) {
	m.SchedulerService = evaluationscheduler.NewServiceWithRuns(infra.assessmentRepo, infra.outcomeRepo, infra.submittedCandidateReader, infra.runRepo)
}

func normalizeDeps(deps Deps) (Deps, error) {
	if deps.MySQLDB == nil {
		return Deps{}, errors.WithCode(code.ErrModuleInitializationFailed, "MySQL database connection is nil or invalid")
	}
	if deps.EventPublisher == nil {
		deps.EventPublisher = event.NewNopEventPublisher()
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
