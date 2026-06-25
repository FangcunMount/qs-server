package assembler

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaluationResult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	sbtiEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/sbti"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	scaleInterpretation "github.com/FangcunMount/qs-server/internal/apiserver/application/scale/interpretation"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	assessmentCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cacheentry"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachequery"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/evaluation"
	mongoEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventoutbox"
	mysqlEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/waiter"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
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

	// 最新风险读模型 - 服务于医生工作台等跨模块读侧编排
	LatestRiskReader evaluationreadmodel.LatestRiskReader

	// ==================== 评估引擎 ====================

	// 评估引擎服务 - 服务于评估引擎 (qs-worker)
	EvaluationService execute.Service

	// ReportStatusReporter best-effort 报告等待状态写入与 signaling。
	ReportStatusReporter *reportstatus.Reporter
}

// EvaluationModuleDeps 定义 Evaluation 模块的显式构造依赖。
type EvaluationModuleDeps struct {
	MySQLDB                        *gorm.DB
	MongoDB                        *mongo.Database
	InputResolver                  evaluationinput.Resolver
	ScaleCatalog                   evaluationinput.ScaleCatalog
	EventPublisher                 event.EventPublisher
	RedisClient                    redis.UniversalClient
	CacheBuilder                   *keyspace.Builder
	AssessmentPolicy               cachepolicy.CachePolicy
	QueryRedisClient               redis.UniversalClient
	QueryCacheBuilder              *keyspace.Builder
	AssessmentListPolicy           cachepolicy.CachePolicy
	VersionStore                   cachequery.VersionTokenStore
	Observer                       *observability.ComponentObserver
	TopicResolver                  eventcatalog.TopicResolver
	MySQLLimiter                   backpressure.Acquirer
	MongoLimiter                   backpressure.Acquirer
	AssessmentOutboxRelayBatchSize int
	TesteeAccessChecker            assessmentApp.TesteeAccessChecker
	OpsHandle                      *cacheplane.Handle
	ReportStatusConfig             reportstatus.Config
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
	reportReader                 evaluationreadmodel.ReportReader
	assessmentOutboxStore        *mysqlEventOutbox.Store
	reportDurableSaver           evaluationResult.ReportDurableSaver
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

	assessmentReadModel := mysqlEval.NewAssessmentReadModel(normalized.MySQLDB, mysqlOptions)
	infra.assessmentReader = assessmentReadModel
	infra.latestRiskReader = assessmentReadModel
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
	infra.reportDurableSaver = evaluationResult.NewTransactionalReportDurableSaver(mongoTxRunner, reportRepo, reportOutboxStore)
	infra.assessmentOutboxStore = assessmentOutboxStore
	infra.assessmentOutboxRelay = appEventing.NewOutboxRelayWithOptions(appEventing.OutboxRelayOptions{
		Name:                    "assessment-mysql-outbox",
		Store:                   assessmentOutboxStore,
		Publisher:               normalized.EventPublisher,
		BatchSize:               normalized.AssessmentOutboxRelayBatchSize,
		RequireDurablePublisher: true,
	})
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
) error {
	var suggestionGenerator report.SuggestionGenerator

	if normalized.InputResolver != nil {
		reportStatusReporter, err := reportstatus.NewReporter(normalized.OpsHandle, normalized.ReportStatusConfig)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report status reporter: %v", err)
		}
		m.ReportStatusReporter = reportStatusReporter

		reportBuilder := report.NewScaleReportBuilder(suggestionGenerator)
		scaleEvaluator := scaleInterpretation.NewExecutorWithService(
			scaleInterpretation.NewService(
				scaleInterpretation.DefaultInputValidator{},
				scaleInterpretation.DefaultInputAssembler{},
				nil,
				scaleInterpretation.DefaultResultMapper{},
			),
		)
		sbtiEvaluator := sbtiEvaluation.NewExecutor()
		evaluatorRegistry, err := execute.NewEvaluatorRegistry(scaleEvaluator, sbtiEvaluator)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize evaluation evaluator registry: %v", err)
		}
		scoreProjectors, err := evaluationResult.NewScoreProjectorRegistry(
			evaluationResult.NewScaleScoreProjector(infra.scoreRepo),
		)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize evaluation score projector registry: %v", err)
		}
		reportBuilders, err := evaluationResult.NewReportBuilderRegistry(
			evaluationResult.NewScaleReportBuilder(reportBuilder),
			sbtiEvaluation.NewReportBuilder(),
		)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize evaluation report builder registry: %v", err)
		}
		resultWriter, err := evaluationResult.NewWriter(
			infra.assessmentRepo,
			scoreProjectors,
			reportBuilders,
			infra.reportDurableSaver,
			evaluationResult.NewWaiterCompletionNotifier(infra.waiterRegistry),
			reportStatusReporter,
		)
		if err != nil {
			return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize evaluation result writer: %v", err)
		}

		m.EvaluationService = execute.NewService(
			infra.assessmentRepo,
			normalized.InputResolver,
			resultWriter,
			execute.WithTransactionalOutbox(infra.txRunner, infra.assessmentOutboxStore),
			execute.WithEvaluatorRegistry(evaluatorRegistry),
			execute.WithReportStatusReporter(reportStatusReporter),
		)
	}
	return nil
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
	m.LatestRiskReader = infra.latestRiskReader
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
