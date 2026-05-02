package assembler

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	redis "github.com/redis/go-redis/v9"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine/pipeline"
	reportApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/report"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
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
	mysqlDB *gorm.DB

	// ==================== Repository / Read Model 层 ====================
	assessmentRepo               assessment.Repository
	scoreRepo                    assessment.ScoreRepository
	reportRepo                   report.ReportRepository
	assessmentReader             evaluationreadmodel.AssessmentReader
	scoreReader                  evaluationreadmodel.ScoreReader
	reportReader                 evaluationreadmodel.ReportReader
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

	// ==================== 评估引擎 ====================

	// 评估引擎服务 - 服务于评估引擎 (qs-worker)
	EvaluationService engine.Service

	// ==================== Report 应用服务 ====================

	// 报告生成服务 - 服务于评估引擎
	ReportGenerationService reportApp.ReportGenerationService

	// 报告导出服务 - 服务于用户
	ReportExportService reportApp.ReportExportService

	// 建议服务 - 服务于评估引擎
	SuggestionService reportApp.SuggestionService

	// 事件发布器（由容器统一注入）
	eventPublisher        event.EventPublisher
	testeeAccessService   actorAccessApp.TesteeAccessService
	assessmentOutboxStore *mysqlEventOutbox.Store
	reportDurableSaver    pipeline.ReportDurableSaver
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
	TesteeAccessService  actorAccessApp.TesteeAccessService
}

// NewEvaluationModule 创建评估模块。
func NewEvaluationModule(deps EvaluationModuleDeps) (*EvaluationModule, error) {
	normalized, err := normalizeEvaluationModuleDeps(deps)
	if err != nil {
		return nil, err
	}
	module := &EvaluationModule{}
	module.mysqlDB = normalized.MySQLDB
	module.eventPublisher = normalized.EventPublisher
	module.testeeAccessService = normalized.TesteeAccessService

	// ==================== 初始化 Repository 层 ====================
	// 初始化基础 Repository
	mysqlOptions := mysql.BaseRepositoryOptions{Limiter: normalized.MySQLLimiter}
	mongoOptions := mongoBase.BaseRepositoryOptions{Limiter: normalized.MongoLimiter}
	baseAssessmentRepo := mysqlEval.NewAssessmentRepositoryWithTopicResolver(normalized.MySQLDB, normalized.TopicResolver, mysqlOptions)
	// 如果提供了 Redis 客户端，使用缓存装饰器
	if normalized.RedisClient != nil {
		module.assessmentRepo = assessmentCache.NewCachedAssessmentRepositoryWithBuilderPolicyAndObserver(baseAssessmentRepo, normalized.RedisClient, normalized.CacheBuilder, normalized.AssessmentPolicy, normalized.Observer)
	} else {
		module.assessmentRepo = baseAssessmentRepo
	}

	module.assessmentReader = mysqlEval.NewAssessmentReadModel(normalized.MySQLDB, mysqlOptions)
	module.scoreRepo = mysqlEval.NewScoreRepository(normalized.MySQLDB, mysqlOptions)
	module.scoreReader = mysqlEval.NewScoreReadModel(normalized.MySQLDB, mysqlOptions)
	reportRepo, err := mongoEval.NewReportRepositoryWithTopicResolver(normalized.MongoDB, normalized.TopicResolver, mongoOptions)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report repository: %v", err)
	}
	module.reportRepo = reportRepo
	module.reportReader = mongoEval.NewReportReadModel(normalized.MongoDB, mongoOptions)
	txRunner := newMySQLTransactionRunner(normalized.MySQLDB)
	mongoTxRunner := newMongoTransactionRunner(normalized.MongoDB)
	assessmentOutboxStore := mysqlEventOutbox.NewStoreWithTopicResolver(normalized.MySQLDB, normalized.TopicResolver)
	reportOutboxStore, err := mongoEventOutbox.NewStoreWithTopicResolver(normalized.MongoDB, normalized.TopicResolver)
	if err != nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report outbox store: %v", err)
	}
	module.reportDurableSaver = pipeline.NewTransactionalReportDurableSaver(mongoTxRunner, reportRepo, reportOutboxStore)
	module.assessmentOutboxStore = assessmentOutboxStore
	module.AssessmentOutboxRelay = appEventing.NewDurableOutboxRelay("assessment-mysql-outbox", assessmentOutboxStore, module.eventPublisher)
	module.AssessmentOutboxStatusReader = appEventing.NamedOutboxStatusReader{
		Name:   "assessment-mysql-outbox",
		Reader: assessmentOutboxStore,
	}

	// ==================== 初始化领域服务 ====================

	// 创建 AssessmentCreator（领域服务）
	assessmentCreator := assessment.NewDefaultAssessmentCreator()

	// 创建 SuggestionGenerator（领域服务）
	// 注意：因子解读配置中的建议已通过 FactorInterpretationSuggestionStrategy 收集
	// 如果需要额外的建议生成策略，可以在这里注册
	// 当前不注册任何策略，完全依赖因子解读配置中的建议
	var suggestionGenerator report.SuggestionGenerator

	// 当前导出能力保留入口，但显式收口为 unsupported，避免主路径继续装配空实现。
	reportExporter := reportApp.NewUnsupportedReportExporter()

	// ====================  初始化评估引擎 ====================
	// 创建等待队列注册表（用于长轮询，在创建 EvaluationService 和 Handler 时使用）
	var waiterRegistry *waiter.WaiterRegistry
	if normalized.InputResolver != nil {
		waiterRegistry = waiter.NewWaiterRegistry(logger.L(context.Background()))
	}

	// 注意：如果有输入解析器，则初始化 EvaluationService
	if normalized.InputResolver != nil {
		// 创建 ReportBuilder，注入 SuggestionGenerator
		reportBuilder := report.NewDefaultReportBuilder(suggestionGenerator)

		serviceOpts := []engine.ServiceOption{}
		if waiterRegistry != nil {
			serviceOpts = append(serviceOpts, engine.WithWaiterRegistry(waiterRegistry))
		}
		serviceOpts = append(serviceOpts, engine.WithTransactionalOutbox(txRunner, assessmentOutboxStore))
		serviceOpts = append(serviceOpts, engine.WithReportDurableSaver(module.reportDurableSaver))
		serviceOpts = append(serviceOpts, engine.WithScaleFactorScorer(ruleengineInfra.NewScaleFactorScorer()))
		serviceOpts = append(serviceOpts, engine.WithInterpretEngine(
			interpretengineInfra.NewInterpreter(),
			interpretengineInfra.NewDefaultProvider(),
		))

		module.EvaluationService = engine.NewService(
			module.assessmentRepo,
			module.scoreRepo,
			module.reportRepo,
			normalized.InputResolver,
			reportBuilder,
			serviceOpts...,
		)
	}

	// ==================== 初始化 Report 应用服务 ====================

	// 建议服务
	module.SuggestionService = reportApp.NewSuggestionService(
		module.reportRepo,
		suggestionGenerator,
	)

	// 报告生成服务
	module.ReportGenerationService = reportApp.NewReportGenerationService(module.reportRepo)

	// 报告导出服务
	module.ReportExportService = reportApp.NewReportExportService(
		module.reportRepo,
		reportExporter,
	)

	// ==================== 初始化 Assessment 应用服务 ====================

	// 提交服务 - 服务于答题者 (Testee)
	if normalized.QueryRedisClient != nil && normalized.VersionStore != nil {
		listCache := cachequery.NewMyAssessmentListCacheWithBuilderPolicyAndObserver(
			cacheentry.NewRedisCache(normalized.QueryRedisClient),
			normalized.VersionStore,
			normalized.QueryCacheBuilder,
			normalized.AssessmentListPolicy,
			normalized.Observer,
		)
		module.SubmissionService = assessmentApp.NewSubmissionServiceWithTransactionalOutboxAndReadModel(
			module.assessmentRepo,
			module.assessmentReader,
			assessmentCreator,
			txRunner,
			assessmentOutboxStore,
			listCache,
		)
	} else {
		module.SubmissionService = assessmentApp.NewSubmissionServiceWithTransactionalOutboxAndReadModel(
			module.assessmentRepo,
			module.assessmentReader,
			assessmentCreator,
			txRunner,
			assessmentOutboxStore,
			nil,
		)
	}

	// 管理服务 - 服务于管理员 (Staff/Admin)
	module.ManagementService = assessmentApp.NewManagementServiceWithTransactionalOutboxAndReadModel(module.assessmentRepo, module.assessmentReader, module.eventPublisher, txRunner, assessmentOutboxStore)

	// 报告查询服务 - 服务于报告查询者
	module.ReportQueryService = assessmentApp.NewReportQueryServiceWithReadModel(module.reportRepo, module.reportReader)

	// 得分查询服务 - 服务于数据分析
	module.ScoreQueryService = assessmentApp.NewScoreQueryServiceWithReadModel(
		module.scoreRepo,
		module.assessmentRepo,
		module.scoreReader,
		module.assessmentReader,
		normalized.ScaleCatalog,
	)

	module.WaitService = assessmentApp.NewWaitService(module.ManagementService, waiterRegistry)

	return module, nil
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
