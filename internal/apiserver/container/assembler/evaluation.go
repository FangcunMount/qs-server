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
	reportApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/report"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	assessmentCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	mongoEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/evaluation"
	mysqlEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/waiter"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// EvaluationModule 评估模块（测评、得分、报告）
// 整合 evaluation 子域的所有功能
type EvaluationModule struct {
	// ==================== Interface 层 ====================
	Handler *handler.EvaluationHandler
	mysqlDB *gorm.DB

	// ==================== Repository 层 ====================
	AssessmentRepo        assessment.Repository
	ScoreRepo             assessment.ScoreRepository
	ReportRepo            report.ReportRepository
	AssessmentOutboxRelay appEventing.OutboxRelay

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
	eventPublisher      event.EventPublisher
	testeeAccessService actorAccessApp.TesteeAccessService
}

type evaluationModuleDeps struct {
	mysqlDB              *gorm.DB
	mongoDB              *mongo.Database
	scaleRepo            scale.Repository
	answerSheetRepo      answersheet.Repository
	questionnaireRepo    questionnaire.Repository
	eventPublisher       event.EventPublisher
	redisClient          redis.UniversalClient
	cacheBuilder         *rediskey.Builder
	assessmentPolicy     cachepolicy.CachePolicy
	queryRedisClient     redis.UniversalClient
	queryCacheBuilder    *rediskey.Builder
	assessmentListPolicy cachepolicy.CachePolicy
	versionStore         assessmentCache.VersionTokenStore
	observer             *assessmentCache.Observer
}

// NewEvaluationModule 创建评估模块
func NewEvaluationModule() *EvaluationModule {
	return &EvaluationModule{}
}

// Initialize 初始化模块
// params[0]: *gorm.DB (MySQL)
// params[1]: *mongo.Database (MongoDB)
// params[2]: scale.Repository (可选，用于 EvaluationService)
// params[3]: answersheet.Repository (可选，用于 EvaluationService)
// params[4]: questionnaire.Repository (可选，用于 EvaluationService 的 cnt 计分规则)
// params[5]: event.EventPublisher (可选，用于事件发布)
// params[6]: redis.UniversalClient (可选，用于对象缓存装饰器)
// params[7]: *rediskey.Builder (可选，用于对象缓存 key builder)
// params[8]: cachepolicy.CachePolicy (可选，用于测评详情缓存策略)
// params[9]: redis.UniversalClient (可选，用于 query cache，如我的测评列表)
// params[10]: *rediskey.Builder (可选，用于 query cache key builder)
// params[11]: cachepolicy.CachePolicy (可选，用于我的测评列表缓存策略)
// params[12]: assessmentCache.VersionTokenStore (可选，用于 versioned query invalidation)
func (m *EvaluationModule) Initialize(params ...interface{}) error {
	deps, err := parseEvaluationModuleDeps(params)
	if err != nil {
		return err
	}
	m.mysqlDB = deps.mysqlDB
	m.eventPublisher = deps.eventPublisher

	// ==================== 初始化 Repository 层 ====================
	// 初始化基础 Repository
	baseAssessmentRepo := mysqlEval.NewAssessmentRepository(deps.mysqlDB)
	// 如果提供了 Redis 客户端，使用缓存装饰器
	if deps.redisClient != nil {
		m.AssessmentRepo = assessmentCache.NewCachedAssessmentRepositoryWithBuilderPolicyAndObserver(baseAssessmentRepo, deps.redisClient, deps.cacheBuilder, deps.assessmentPolicy, deps.observer)
	} else {
		m.AssessmentRepo = baseAssessmentRepo
	}

	m.ScoreRepo = mysqlEval.NewScoreRepository(deps.mysqlDB)
	reportRepo, err := mongoEval.NewReportRepository(deps.mongoDB)
	if err != nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "failed to initialize report repository: %v", err)
	}
	m.ReportRepo = reportRepo
	m.AssessmentOutboxRelay = appEventing.NewOutboxRelay("assessment-mysql-outbox", mysqlEventOutbox.NewStore(deps.mysqlDB), m.eventPublisher)

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
	if deps.scaleRepo != nil && deps.answerSheetRepo != nil && deps.questionnaireRepo != nil {
		waiterRegistry = waiter.NewWaiterRegistry(logger.L(context.Background()))
	}

	// 注意：如果有 scaleRepo、answerSheetRepo 和 questionnaireRepo，则初始化 EvaluationService
	if deps.scaleRepo != nil && deps.answerSheetRepo != nil && deps.questionnaireRepo != nil {
		// 创建 ReportBuilder，注入 SuggestionGenerator
		reportBuilder := report.NewDefaultReportBuilder(suggestionGenerator)

		serviceOpts := []engine.ServiceOption{}
		if waiterRegistry != nil {
			serviceOpts = append(serviceOpts, engine.WithWaiterRegistry(waiterRegistry))
		}

		m.EvaluationService = engine.NewService(
			m.AssessmentRepo,
			m.ScoreRepo,
			m.ReportRepo,
			deps.scaleRepo,
			deps.answerSheetRepo,
			deps.questionnaireRepo,
			reportBuilder,
			serviceOpts...,
		)
	}

	// ==================== 初始化 Report 应用服务 ====================

	// 建议服务
	m.SuggestionService = reportApp.NewSuggestionService(
		m.ReportRepo,
		suggestionGenerator,
	)

	// 报告生成服务
	m.ReportGenerationService = reportApp.NewReportGenerationService(m.ReportRepo)

	// 报告导出服务
	m.ReportExportService = reportApp.NewReportExportService(
		m.ReportRepo,
		reportExporter,
	)

	// ==================== 初始化 Assessment 应用服务 ====================

	// 提交服务 - 服务于答题者 (Testee)
	if deps.queryRedisClient != nil && deps.versionStore != nil {
		listCache := assessmentCache.NewMyAssessmentListCacheWithBuilderPolicyAndObserver(
			assessmentCache.NewRedisCache(deps.queryRedisClient),
			deps.versionStore,
			deps.queryCacheBuilder,
			deps.assessmentListPolicy,
			deps.observer,
		)
		m.SubmissionService = assessmentApp.NewSubmissionServiceWithListCache(
			m.AssessmentRepo,
			assessmentCreator,
			m.eventPublisher,
			listCache,
		)
	} else {
		m.SubmissionService = assessmentApp.NewSubmissionService(
			m.AssessmentRepo,
			assessmentCreator,
			m.eventPublisher,
		)
	}

	// 管理服务 - 服务于管理员 (Staff/Admin)
	m.ManagementService = assessmentApp.NewManagementService(m.AssessmentRepo, m.eventPublisher)

	// 报告查询服务 - 服务于报告查询者
	m.ReportQueryService = assessmentApp.NewReportQueryService(m.ReportRepo)

	// 得分查询服务 - 服务于数据分析
	m.ScoreQueryService = assessmentApp.NewScoreQueryService(
		m.ScoreRepo,
		m.AssessmentRepo,
		deps.scaleRepo, // 传入 scaleRepo（可能为 nil，但会在 SetScaleRepository 中更新）
	)

	// ==================== 初始化 Interface 层 ====================
	m.Handler = handler.NewEvaluationHandler(
		m.ManagementService,
		m.ReportQueryService,
		m.ScoreQueryService,
		m.EvaluationService,
	)

	if m.testeeAccessService != nil {
		m.Handler.SetTesteeAccessService(m.testeeAccessService)
	}

	// 注入等待队列注册表（如果可用，用于长轮询接口）
	if waiterRegistry != nil {
		m.Handler.SetWaiterRegistry(waiterRegistry)
	}

	return nil
}

func parseEvaluationModuleDeps(params []interface{}) (*evaluationModuleDeps, error) {
	if len(params) < 2 {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "evaluation module requires both MySQL and MongoDB connections")
	}

	mysqlDB, ok := params[0].(*gorm.DB)
	if !ok || mysqlDB == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "MySQL database connection is nil or invalid")
	}
	mongoDB, ok := params[1].(*mongo.Database)
	if !ok || mongoDB == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "MongoDB database connection is nil or invalid")
	}

	deps := &evaluationModuleDeps{
		mysqlDB:        mysqlDB,
		mongoDB:        mongoDB,
		eventPublisher: event.NewNopEventPublisher(),
	}
	applyOptionalParam(params, 2, func(repo scale.Repository) {
		deps.scaleRepo = repo
	})
	applyOptionalParam(params, 3, func(repo answersheet.Repository) {
		deps.answerSheetRepo = repo
	})
	applyOptionalParam(params, 4, func(repo questionnaire.Repository) {
		deps.questionnaireRepo = repo
	})
	applyOptionalParam(params, 5, func(publisher event.EventPublisher) {
		if publisher != nil {
			deps.eventPublisher = publisher
		}
	})
	applyOptionalParam(params, 6, func(client redis.UniversalClient) {
		if client != nil {
			deps.redisClient = client
		}
	})
	applyOptionalParam(params, 7, func(builder *rediskey.Builder) {
		deps.cacheBuilder = builder
	})
	applyOptionalParam(params, 8, func(policy cachepolicy.CachePolicy) {
		deps.assessmentPolicy = policy
	})
	applyOptionalParam(params, 9, func(client redis.UniversalClient) {
		if client != nil {
			deps.queryRedisClient = client
		}
	})
	applyOptionalParam(params, 10, func(builder *rediskey.Builder) {
		deps.queryCacheBuilder = builder
	})
	applyOptionalParam(params, 11, func(policy cachepolicy.CachePolicy) {
		deps.assessmentListPolicy = policy
	})
	applyOptionalParam(params, 12, func(store assessmentCache.VersionTokenStore) {
		deps.versionStore = store
	})
	applyOptionalParam(params, 13, func(observer *assessmentCache.Observer) {
		deps.observer = observer
	})
	return deps, nil
}

// SetScaleRepository 设置量表仓储（用于跨模块依赖注入）
// 注意：需要同时有 answerSheetRepo 和 questionnaireRepo 才能创建 EvaluationService
func (m *EvaluationModule) SetScaleRepository(
	scaleRepo scale.Repository,
	answerSheetRepo answersheet.Repository,
	questionnaireRepo questionnaire.Repository,
) {
	// 重新创建需要 scaleRepo、answerSheetRepo 和 questionnaireRepo 的服务
	if answerSheetRepo == nil || questionnaireRepo == nil {
		return
	}
	// 使用默认策略创建 SuggestionGenerator
	// 注意：因子解读配置中的建议已通过 FactorInterpretationSuggestionStrategy 收集
	// 当前不注册任何策略，完全依赖因子解读配置中的建议
	var suggestionGenerator report.SuggestionGenerator
	reportBuilder := report.NewDefaultReportBuilder(suggestionGenerator)
	if m.mysqlDB == nil {
		return
	}
	m.EvaluationService = engine.NewService(
		m.AssessmentRepo,
		m.ScoreRepo,
		m.ReportRepo,
		scaleRepo,
		answerSheetRepo,
		questionnaireRepo,
		reportBuilder,
	)

	// 重新创建 ScoreQueryService，传入 scaleRepo
	if scaleRepo != nil {
		m.ScoreQueryService = assessmentApp.NewScoreQueryService(
			m.ScoreRepo,
			m.AssessmentRepo,
			scaleRepo,
		)
		// 重新创建 Handler，因为 ScoreQueryService 已更新
		// 注意：这里不传入 QRCodeService，因为它在容器初始化后才创建
		// QRCodeService 需要通过 SetQRCodeService 方法单独设置
		m.Handler = handler.NewEvaluationHandler(
			m.ManagementService,
			m.ReportQueryService,
			m.ScoreQueryService,
			m.EvaluationService,
		)
		if m.testeeAccessService != nil {
			m.Handler.SetTesteeAccessService(m.testeeAccessService)
		}
	}
}

// SetTesteeAccessService 设置 testee 访问控制服务。
func (m *EvaluationModule) SetTesteeAccessService(testeeAccessService actorAccessApp.TesteeAccessService) {
	m.testeeAccessService = testeeAccessService
	if m.Handler != nil {
		m.Handler.SetTesteeAccessService(testeeAccessService)
	}
}

// SetQRCodeService 设置二维码服务（用于跨模块依赖注入）
// 注意：EvaluationHandler 不再需要 QRCodeService，此方法保留以保持接口一致性
func (m *EvaluationModule) SetQRCodeService(_ qrcodeApp.QRCodeService) {
	// EvaluationHandler 不再需要 QRCodeService，因为二维码查询已移至问卷和量表 Handler
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
