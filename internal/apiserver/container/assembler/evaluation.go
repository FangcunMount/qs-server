package assembler

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine/pipeline"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	assessmentCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	interpretengineInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/interpretengine"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/evaluation"
	mongoEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventoutbox"
	mysqlEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventoutbox"
	ruleengineInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/waiter"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	interpretenginePort "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretengine"
	ruleenginePort "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// EvaluationModule 评估模块（测评、得分、报告）
// 整合 evaluation 子域的所有功能
type EvaluationModule struct {
	AssessmentOutboxRelay        appEventing.OutboxRelay
	AssessmentOutboxStatusReader appEventing.NamedOutboxStatusReader

	// ==================== Assessment 应用服务 ====================
	// 按行为者组织的测评服务

	// 提交服务 - 服务于答题者 (Testee)
	SubmissionService assessmentApp.AssessmentSubmissionService

	// 管理服务 - 服务于管理员 (Staff/Admin)
	ManagementService assessmentApp.AssessmentManagementService

	// 报告查询服务 - 服务于报告查询者
	ReportQueryService assessmentApp.ReportQueryService

	// 得分查询服务 - 服务于数据分析
	ScoreQueryService assessmentApp.ScoreQueryService

	// 等待报告服务 - 服务于 REST 长轮询
	WaitService assessmentApp.AssessmentWaitService

	// 访问控制查询服务 - 服务于 REST 查询访问收口
	AccessQueryService assessmentApp.AssessmentAccessQueryService

	// 受保护查询服务 - 服务于 REST 查询访问与查询编排收口
	ProtectedQueryService assessmentApp.AssessmentProtectedQueryService

	// ==================== 评估引擎 ====================

	// 评估引擎服务 - 服务于评估引擎 (qs-worker)
	EvaluationService engine.Service
}

// EvaluationModuleDeps 定义 Evaluation 模块的显式构造依赖。
type EvaluationModuleDeps struct {
	MySQLDB              *gorm.DB
	MongoDB              *mongo.Database
	InputResolver        evaluationinput.Resolver
	ScaleCatalog         evaluationinput.ScaleCatalog
	EventPublisher       event.EventPublisher
	RedisClient          redis.UniversalClient
	CacheBuilder         *keyspace.Builder
	AssessmentPolicy     cachepolicy.CachePolicy
	QueryRedisClient     redis.UniversalClient
	QueryCacheBuilder    *keyspace.Builder
	AssessmentListPolicy cachepolicy.CachePolicy
	VersionStore         cachequery.VersionTokenStore
	Observer             *observability.ComponentObserver
	TopicResolver        eventcatalog.TopicResolver
	MySQLLimiter         backpressure.Acquirer
	MongoLimiter         backpressure.Acquirer
	TesteeAccessChecker  assessmentApp.TesteeAccessChecker
}

// NewEvaluationModule 创建评估模块。
func NewEvaluationModule(deps EvaluationModuleDeps) (*EvaluationModule, error) {
	normalized, err := normalizeEvaluationModuleDeps(deps)
	if err != nil {
		return nil, err
	}
	infra, err := newEvaluationInfra(normalized)
	if err != nil {
		return nil, err
	}

	module := &EvaluationModule{
		AssessmentOutboxRelay:        infra.assessmentOutboxRelay,
		AssessmentOutboxStatusReader: infra.assessmentOutboxStatusReader,
	}
	module.wireEvaluationEngine(normalized, infra)
	module.wireAssessmentApplications(normalized, infra)

	return module, nil
}

type evaluationInfra struct {
	assessmentRepo               assessment.Repository
	scoreRepo                    assessment.ScoreRepository
	assessmentReader             evaluationreadmodel.AssessmentReader
	scoreReader                  evaluationreadmodel.ScoreReader
	reportReader                 evaluationreadmodel.ReportReader
	assessmentOutboxStore        *mysqlEventOutbox.Store
	reportDurableSaver           pipeline.ReportDurableSaver
	txRunner                     apptransaction.Runner
	waiterRegistry               *waiter.WaiterRegistry
	assessmentOutboxRelay        appEventing.OutboxRelay
	assessmentOutboxStatusReader appEventing.NamedOutboxStatusReader
}

func newEvaluationInfra(normalized EvaluationModuleDeps) (*evaluationInfra, error) {
	infra := &evaluationInfra{}
	mysqlOptions := mysql.BaseRepositoryOptions{Limiter: normalized.MySQLLimiter}
	mongoOptions := mongoBase.BaseRepositoryOptions{Limiter: normalized.MongoLimiter}
	baseAssessmentRepo := mysqlEval.NewAssessmentRepositoryWithTopicResolver(normalized.MySQLDB, normalized.TopicResolver, mysqlOptions)
	if normalized.RedisClient != nil {
		infra.assessmentRepo = assessmentCache.NewCachedAssessmentRepositoryWithBuilderPolicyAndObserver(baseAssessmentRepo, normalized.RedisClient, normalized.CacheBuilder, normalized.AssessmentPolicy, normalized.Observer)
	} else {
		infra.assessmentRepo = baseAssessmentRepo
	}

	infra.assessmentReader = mysqlEval.NewAssessmentReadModel(normalized.MySQLDB, mysqlOptions)
	infra.scoreRepo = mysqlEval.NewScoreRepository(normalized.MySQLDB, mysqlOptions)
	infra.scoreReader = mysqlEval.NewScoreReadModel(normalized.MySQLDB, mysqlOptions)
	reportRepo, err := mongoEval.NewReportRepositoryWithTopicResolver(normalized.MongoDB, normalized.TopicResolver, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report repository: %v", err)
	}
	infra.reportReader = mongoEval.NewReportReadModel(normalized.MongoDB, mongoOptions)
	infra.txRunner = newMySQLTransactionRunner(normalized.MySQLDB)
	mongoTxRunner := newMongoTransactionRunner(normalized.MongoDB)
	assessmentOutboxStore := mysqlEventOutbox.NewStoreWithTopicResolver(normalized.MySQLDB, normalized.TopicResolver)
	reportOutboxStore, err := mongoEventOutbox.NewStoreWithTopicResolver(normalized.MongoDB, normalized.TopicResolver)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report outbox store: %v", err)
	}
	infra.reportDurableSaver = pipeline.NewTransactionalReportDurableSaver(mongoTxRunner, reportRepo, reportOutboxStore)
	infra.assessmentOutboxStore = assessmentOutboxStore
	infra.assessmentOutboxRelay = appEventing.NewDurableOutboxRelay("assessment-mysql-outbox", assessmentOutboxStore, normalized.EventPublisher)
	infra.assessmentOutboxStatusReader = appEventing.NamedOutboxStatusReader{
		Name:   "assessment-mysql-outbox",
		Reader: assessmentOutboxStore,
	}

	if normalized.InputResolver != nil {
		infra.waiterRegistry = waiter.NewWaiterRegistry(logger.L(context.Background()))
	}
	return infra, nil
}

func (m *EvaluationModule) wireEvaluationEngine(
	normalized EvaluationModuleDeps,
	infra *evaluationInfra,
) {
	var suggestionGenerator report.SuggestionGenerator

	if normalized.InputResolver != nil {
		reportBuilder := report.NewDefaultReportBuilder(suggestionGenerator)

		pipelineRunner := newEvaluationPipelineRunner(evaluationPipelineDeps{
			AssessmentRepo:  infra.assessmentRepo,
			ScoreRepo:       infra.scoreRepo,
			ReportBuilder:   reportBuilder,
			ReportSaver:     infra.reportDurableSaver,
			WaiterRegistry:  infra.waiterRegistry,
			FactorScorer:    ruleengineInfra.NewScaleFactorScorer(),
			Interpreter:     interpretengineInfra.NewInterpreter(),
			DefaultProvider: interpretengineInfra.NewDefaultProvider(),
		})

		m.EvaluationService = engine.NewService(
			infra.assessmentRepo,
			normalized.InputResolver,
			pipelineRunner,
			engine.WithTransactionalOutbox(infra.txRunner, infra.assessmentOutboxStore),
		)
	}
}

func (m *EvaluationModule) wireAssessmentApplications(
	normalized EvaluationModuleDeps,
	infra *evaluationInfra,
) {
	assessmentCreator := assessment.NewDefaultAssessmentCreator()
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
		)
	} else {
		m.SubmissionService = assessmentApp.NewSubmissionService(
			infra.assessmentRepo,
			infra.assessmentReader,
			assessmentCreator,
			infra.txRunner,
			infra.assessmentOutboxStore,
			nil,
		)
	}

	m.ManagementService = assessmentApp.NewManagementService(infra.assessmentRepo, infra.assessmentReader, infra.txRunner, infra.assessmentOutboxStore)
	m.ReportQueryService = assessmentApp.NewReportQueryService(infra.reportReader)
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
	)
}

type evaluationPipelineDeps struct {
	AssessmentRepo  assessment.Repository
	ScoreRepo       assessment.ScoreRepository
	ReportBuilder   report.ReportBuilder
	ReportSaver     pipeline.ReportDurableSaver
	WaiterRegistry  *waiter.WaiterRegistry
	FactorScorer    ruleenginePort.ScaleFactorScorer
	Interpreter     interpretenginePort.Interpreter
	DefaultProvider interpretenginePort.DefaultProvider
}

func newEvaluationPipelineRunner(deps evaluationPipelineDeps) *pipeline.Chain {
	chain := pipeline.NewChain()
	chain.AddHandler(pipeline.NewValidationHandler())
	chain.AddHandler(pipeline.NewFactorScoreHandler(deps.FactorScorer))
	chain.AddHandler(pipeline.NewRiskLevelHandler(
		pipeline.NewRiskClassifier(),
		pipeline.NewAssessmentScoreWriter(deps.ScoreRepo),
	))
	chain.AddHandler(pipeline.NewInterpretationHandler(
		pipeline.NewInterpretationGenerator(deps.Interpreter, deps.DefaultProvider),
		pipeline.NewInterpretationFinalizer(
			pipeline.NewAssessmentResultWriter(deps.AssessmentRepo),
			pipeline.NewInterpretReportWriter(deps.ReportBuilder, deps.ReportSaver),
		),
	))
	if deps.WaiterRegistry != nil {
		chain.AddHandler(pipeline.NewWaiterNotifyHandler(
			pipeline.NewWaiterCompletionNotifier(deps.WaiterRegistry),
		))
	}
	return chain
}

func normalizeEvaluationModuleDeps(deps EvaluationModuleDeps) (EvaluationModuleDeps, error) {
	if deps.MySQLDB == nil {
		return EvaluationModuleDeps{}, errors.WithCode(code.ErrModuleInitializationFailed, "MySQL database connection is nil or invalid")
	}
	if deps.MongoDB == nil {
		return EvaluationModuleDeps{}, errors.WithCode(code.ErrModuleInitializationFailed, "MongoDB database connection is nil or invalid")
	}
	if deps.EventPublisher == nil {
		deps.EventPublisher = event.NewNopEventPublisher()
	}
	return deps, nil
}

// Cleanup 清理模块资源
func (m *EvaluationModule) Cleanup() error {
	return nil
}

// CheckHealth 检查模块健康状态
func (m *EvaluationModule) CheckHealth() error {
	// 当前模块依赖仓储装配期校验；无额外运行时健康探针。
	return nil
}

// ModuleInfo 返回模块信息
func (m *EvaluationModule) ModuleInfo() ModuleInfo {
	return ModuleInfo{
		Name:        "evaluation",
		Version:     "1.0.0",
		Description: "评估模块（测评、得分、报告）",
	}
}
