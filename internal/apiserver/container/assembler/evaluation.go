package assembler

import (
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"

	"github.com/FangcunMount/component-base/pkg/errors"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine"
	reportApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	mongoEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/evaluation"
	mysqlEval "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// EvaluationModule 评估模块（测评、得分、报告）
// 整合 evaluation 子域的所有功能
type EvaluationModule struct {
	// ==================== Interface 层 ====================
	Handler *handler.EvaluationHandler

	// ==================== Repository 层 ====================
	AssessmentRepo assessment.Repository
	ScoreRepo      assessment.ScoreRepository
	ReportRepo     report.ReportRepository

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
	eventPublisher event.EventPublisher
}

// NewEvaluationModule 创建评估模块
func NewEvaluationModule() *EvaluationModule {
	return &EvaluationModule{}
}

// Initialize 初始化模块
// params[0]: *gorm.DB (MySQL)
// params[1]: *mongo.Database (MongoDB)
// params[2]: scale.Repository (可选，用于 EvaluationService)
// params[3]: event.EventPublisher (可选，用于事件发布)
func (m *EvaluationModule) Initialize(params ...interface{}) error {
	if len(params) < 2 {
		return errors.WithCode(code.ErrModuleInitializationFailed, "evaluation module requires both MySQL and MongoDB connections")
	}

	mysqlDB, ok := params[0].(*gorm.DB)
	if !ok || mysqlDB == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "MySQL database connection is nil or invalid")
	}

	mongoDB, ok := params[1].(*mongo.Database)
	if !ok || mongoDB == nil {
		return errors.WithCode(code.ErrModuleInitializationFailed, "MongoDB database connection is nil or invalid")
	}

	// 可选的 ScaleRepo（用于 EvaluationService）
	var scaleRepo scale.Repository
	if len(params) > 2 {
		if sr, ok := params[2].(scale.Repository); ok {
			scaleRepo = sr
		}
	}

	// 获取事件发布器（可选参数）
	if len(params) > 3 {
		if ep, ok := params[3].(event.EventPublisher); ok && ep != nil {
			m.eventPublisher = ep
		}
	}
	if m.eventPublisher == nil {
		m.eventPublisher = event.NewNopEventPublisher()
	}

	// ==================== 初始化 Repository 层 ====================
	m.AssessmentRepo = mysqlEval.NewAssessmentRepository(mysqlDB)
	m.ScoreRepo = mysqlEval.NewScoreRepository(mysqlDB)
	m.ReportRepo = mongoEval.NewReportRepository(mongoDB)

	// ==================== 初始化领域服务 ====================

	// 创建 AssessmentCreator（领域服务）
	assessmentCreator := assessment.NewDefaultAssessmentCreator()

	// 创建 SuggestionGenerator（领域服务）
	suggestionGenerator := report.NewRuleBasedSuggestionGenerator()

	// 创建 ReportExporter（领域服务）- 暂使用 nil，后续在 infra 层实现
	// TODO: 在 infra 层实现真正的 ReportExporter
	var reportExporter report.ReportExporter = nil

	// ==================== 初始化评估引擎 ====================
	// 注意：如果有 scaleRepo，则初始化 EvaluationService
	if scaleRepo != nil {
		reportBuilder := report.NewDefaultReportBuilder()
		m.EvaluationService = engine.NewService(
			m.AssessmentRepo,
			m.ScoreRepo,
			m.ReportRepo,
			scaleRepo,
			reportBuilder,
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
	m.SubmissionService = assessmentApp.NewSubmissionService(
		m.AssessmentRepo,
		assessmentCreator,
		m.eventPublisher,
	)

	// 管理服务 - 服务于管理员 (Staff/Admin)
	m.ManagementService = assessmentApp.NewManagementService(m.AssessmentRepo)

	// 报告查询服务 - 服务于报告查询者
	m.ReportQueryService = assessmentApp.NewReportQueryService(m.ReportRepo)

	// 得分查询服务 - 服务于数据分析
	m.ScoreQueryService = assessmentApp.NewScoreQueryService(
		m.ScoreRepo,
		m.AssessmentRepo,
	)

	// ==================== 初始化 Interface 层 ====================
	m.Handler = handler.NewEvaluationHandler(
		m.ManagementService,
		m.ReportQueryService,
		m.ScoreQueryService,
		m.EvaluationService,
	)

	return nil
}

// SetScaleRepository 设置量表仓储（用于跨模块依赖注入）
func (m *EvaluationModule) SetScaleRepository(scaleRepo scale.Repository) {
	// 重新创建需要 scaleRepo 的服务
	reportBuilder := report.NewDefaultReportBuilder()
	m.EvaluationService = engine.NewService(
		m.AssessmentRepo,
		m.ScoreRepo,
		m.ReportRepo,
		scaleRepo,
		reportBuilder,
	)
}

// Cleanup 清理模块资源
func (m *EvaluationModule) Cleanup() error {
	return nil
}

// CheckHealth 检查模块健康状态
func (m *EvaluationModule) CheckHealth() error {
	// TODO: 实现健康检查
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
