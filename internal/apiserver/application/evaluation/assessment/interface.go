package assessment

import (
	"context"

	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
)

// ============= 按行为者组织的应用服务接口（Driving Ports）=============
//
// 设计原则：单一职责原则 (SRP)
// 每个服务只对一个行为者负责，避免不同行为者的需求变更影响同一个类
//
// 行为者识别：
// 1. 答题者 (Testee) - C端用户，创建和提交测评
// 2. 管理员 (Staff/Admin) - B端用户，查看、管理测评记录
// 3. 评估引擎 (Evaluation Engine) - 异步消费事件，执行计算和解读
// 4. 报告查询者 (Report Viewer) - 查看测评报告（答题者或管理员）

// ==================== 答卷编排服务 ====================

// AnswerSheetAssessmentIntakeService 服务于答卷编排系统。
//
// 它只负责将已提交的答卷转化为 Assessment，并推进 Assessment 到 submitted；
// 不承担受试者查询或报告读取职责。
type AnswerSheetAssessmentIntakeService interface {
	CreateForAnswerSheet(ctx context.Context, dto CreateAssessmentDTO) (*AssessmentResult, error)
	SubmitForEvaluation(ctx context.Context, assessmentID uint64) (*AssessmentResult, error)
	FindByAnswerSheetID(ctx context.Context, answerSheetID uint64) (*AssessmentResult, error)
}

// ==================== 受试者查询服务 ====================

// TesteeAssessmentQueryService 服务于受试者查询。
//
// 它负责所有权校验和“我的测评”列表读取，不创建或提交 Assessment。
type TesteeAssessmentQueryService interface {
	GetMine(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentResult, error)
	ListMine(ctx context.Context, dto ListMyAssessmentsDTO) (*AssessmentListResult, error)
}

// ==================== 兼容服务 ====================

// AssessmentSubmissionService 是拆分前的兼容门面。
//
// 新代码应依行为者使用 AnswerSheetAssessmentIntakeService 或
// TesteeAssessmentQueryService。保留该接口仅为尚未迁移的调用方和测试提供
// 无行为变化的过渡路径。
type AssessmentSubmissionService interface {
	// Create 创建测评
	// 场景：答题者开始填写问卷时，创建测评记录
	// 说明：创建后状态为 pending，等待提交
	Create(ctx context.Context, dto CreateAssessmentDTO) (*AssessmentResult, error)

	// Submit 提交测评
	// 场景：答题者完成答卷后，提交测评
	// 说明：提交后状态变为 submitted，触发 AssessmentSubmittedEvent
	Submit(ctx context.Context, assessmentID uint64) (*AssessmentResult, error)

	// GetMyAssessment 获取我的测评详情
	// 场景：答题者查看自己的测评结果
	GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*AssessmentResult, error)

	// GetMyAssessmentByAnswerSheetID 通过答卷ID获取测评详情
	// 场景：通过答卷ID直接查看测评结果
	GetMyAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (*AssessmentResult, error)

	// ListMyAssessments 查询我的测评列表
	// 场景：答题者查看自己的所有测评记录
	ListMyAssessments(ctx context.Context, dto ListMyAssessmentsDTO) (*AssessmentListResult, error)
}

// ==================== 管理员服务 ====================

// AssessmentOperatorQueryService 服务于后台操作者的 Assessment 查询。
//
// 调用方必须先完成组织与受试者访问范围的校验；该端口只执行已授权的读取。
type AssessmentOperatorQueryService interface {
	GetByID(ctx context.Context, id uint64) (*AssessmentResult, error)
	List(ctx context.Context, dto ListAssessmentsDTO) (*AssessmentListResult, error)
}

// AssessmentOperatorRecoveryService 服务于后台操作者的失败恢复动作。
type AssessmentOperatorRecoveryService interface {
	Retry(ctx context.Context, orgID int64, assessmentID uint64) (*AssessmentResult, error)
}

// AssessmentManagementService 是拆分前后台管理端口的兼容门面。
//
// 新的后台查询应使用 AssessmentOperatorQueryService，失败恢复应使用
// AssessmentOperatorRecoveryService。保留该接口以支撑尚未迁移的 worker 和
// actor 组合调用，直到后续批次将它们迁走。
type AssessmentManagementService interface {
	// GetByID 根据ID获取测评详情
	// 场景：管理员查看测评的完整信息
	GetByID(ctx context.Context, id uint64) (*AssessmentResult, error)

	// List 查询测评列表
	// 场景：管理员查询测评列表，支持按状态、受试者、时间等条件筛选
	List(ctx context.Context, dto ListAssessmentsDTO) (*AssessmentListResult, error)

	// Retry 重试失败的测评
	// 场景：管理员对评估失败的测评进行重试
	Retry(ctx context.Context, orgID int64, assessmentID uint64) (*AssessmentResult, error)
}

type AssessmentWaitService interface {
	WaitReport(ctx context.Context, assessmentID uint64) evaluationwaiter.StatusSummary
}

// TesteeAccessScope 描述 evaluation 查询用例看到的 testee 可见范围。
type TesteeAccessScope struct {
	IsAdmin     bool
	ClinicianID *uint64
}

// TesteeAccessChecker 是 evaluation application 消费的窄权限端口。
type TesteeAccessChecker interface {
	ResolveAccessScope(ctx context.Context, orgID int64, operatorUserID int64) (*TesteeAccessScope, error)
	ValidateTesteeAccess(ctx context.Context, orgID int64, operatorUserID int64, testeeID uint64) error
	ListAccessibleTesteeIDs(ctx context.Context, orgID int64, operatorUserID int64) ([]uint64, error)
}

// AccessibleAssessmentContext 是已完成 testee 访问校验的测评上下文。
type AccessibleAssessmentContext struct {
	AssessmentID uint64
	Assessment   *AssessmentResult
}

// AssessmentAccessQueryService 收口 evaluation REST 查询所需的访问控制编排。
type AssessmentAccessQueryService interface {
	LoadAccessibleAssessment(ctx context.Context, orgID int64, operatorUserID int64, assessmentID uint64) (*AccessibleAssessmentContext, error)
	ValidateTesteeAccess(ctx context.Context, orgID int64, operatorUserID int64, testeeID uint64) error
	ScopeListAssessments(ctx context.Context, orgID int64, operatorUserID int64, dto ListAssessmentsDTO) (ListAssessmentsDTO, error)
	ScopeListReports(ctx context.Context, orgID int64, operatorUserID int64, dto ListReportsDTO) (ListReportsDTO, error)
	ScopeFactorTrend(ctx context.Context, orgID int64, operatorUserID int64, dto GetFactorTrendDTO) (GetFactorTrendDTO, error)
}

// ProtectedQueryScope 是 REST/gRPC 管理端查询传入 evaluation application 的保护域上下文。
type ProtectedQueryScope struct {
	OrgID          int64
	OperatorUserID int64
}

// AssessmentProtectedQueryService 收口 evaluation 查询用例的访问控制与查询编排。
type AssessmentProtectedQueryService interface {
	GetAssessment(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*AssessmentResult, error)
	ListAssessments(ctx context.Context, scope ProtectedQueryScope, dto ListAssessmentsDTO) (*AssessmentListResult, error)
	GetAssessmentOutcome(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*AssessmentOutcomeResult, error)
	ListAssessmentsOutcome(ctx context.Context, scope ProtectedQueryScope, dto ListAssessmentsDTO) (*AssessmentOutcomeListResult, error)
	GetScores(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*ScoreResult, error)
	GetHighRiskFactors(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*HighRiskFactorsResult, error)
	GetFactorTrend(ctx context.Context, scope ProtectedQueryScope, dto GetFactorTrendDTO) (*FactorTrendResult, error)
	GetReport(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*ReportResult, error)
	ListReports(ctx context.Context, scope ProtectedQueryScope, dto ListReportsDTO) (*ReportListResult, error)
	GetReportOutcome(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*ReportOutcomeResult, error)
	ListReportsOutcome(ctx context.Context, scope ProtectedQueryScope, dto ListReportsDTO) (*ReportOutcomeListResult, error)
	WaitReport(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (evaluationwaiter.StatusSummary, error)
	ListAssessmentRuns(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64, limit int) (*AssessmentRunListResult, error)
	GetLatestAssessmentRun(ctx context.Context, scope ProtectedQueryScope, assessmentID uint64) (*AssessmentRunResult, error)
}

// ==================== 评估引擎服务 ====================
//
// 注意：EvaluationService 已独立到 execute 包中
// 请使用: github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute
//
// 行为者：评估引擎 (Evaluation Engine / qs-worker)
// 职责：执行计分、解读、生成报告
// 变更来源：评估算法和流程变化

// ==================== 报告服务 ====================

// ReportQueryService 报告查询服务
// 行为者：报告查询者（答题者或管理员）
// 职责：查询和获取测评报告
// 变更来源：报告展示需求变化
type ReportQueryService interface {
	// GetByAssessmentID 根据测评ID获取报告
	// 场景：用户查看测评报告详情
	GetByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportResult, error)

	// GetByTesteeID 获取受试者的报告列表
	// 场景：用户查看自己的所有报告
	ListByTesteeID(ctx context.Context, dto ListReportsDTO) (*ReportListResult, error)

	GetOutcomeByAssessmentID(ctx context.Context, assessmentID uint64) (*ReportOutcomeResult, error)
	ListOutcomeByTesteeID(ctx context.Context, dto ListReportsDTO) (*ReportOutcomeListResult, error)
}

// ==================== 得分查询服务 ====================

// ScoreQueryService 得分查询服务
// 行为者：报告查询者、数据分析系统
// 职责：查询因子得分、趋势分析
// 变更来源：数据分析需求变化
type ScoreQueryService interface {
	// GetByAssessmentID 获取测评的所有因子得分
	// 场景：查看测评的详细因子分数
	GetByAssessmentID(ctx context.Context, assessmentID uint64) (*ScoreResult, error)

	// GetFactorTrend 获取因子得分趋势
	// 场景：分析受试者某个因子的历史变化趋势
	GetFactorTrend(ctx context.Context, dto GetFactorTrendDTO) (*FactorTrendResult, error)

	// GetHighRiskFactors 获取高风险因子
	// 场景：快速识别需要关注的高风险因子
	GetHighRiskFactors(ctx context.Context, assessmentID uint64) (*HighRiskFactorsResult, error)
}
