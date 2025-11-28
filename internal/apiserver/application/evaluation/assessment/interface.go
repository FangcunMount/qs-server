package assessment

import "context"

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

// ==================== 答题者服务 ====================

// AssessmentSubmissionService 测评提交服务
// 行为者：答题者 (Testee)
// 职责：创建测评、提交答卷、查看自己的测评
// 变更来源：答题者的使用需求变化
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

	// ListMyAssessments 查询我的测评列表
	// 场景：答题者查看自己的所有测评记录
	ListMyAssessments(ctx context.Context, dto ListMyAssessmentsDTO) (*AssessmentListResult, error)
}

// ==================== 管理员服务 ====================

// AssessmentManagementService 测评管理服务
// 行为者：管理员 (Staff/Admin)
// 职责：查看、管理、统计测评记录
// 变更来源：管理后台的管理需求变化
type AssessmentManagementService interface {
	// GetByID 根据ID获取测评详情
	// 场景：管理员查看测评的完整信息
	GetByID(ctx context.Context, id uint64) (*AssessmentResult, error)

	// List 查询测评列表
	// 场景：管理员查询测评列表，支持按状态、受试者、时间等条件筛选
	List(ctx context.Context, dto ListAssessmentsDTO) (*AssessmentListResult, error)

	// GetStatistics 获取测评统计
	// 场景：管理员查看测评统计数据（完成数、平均分、风险分布等）
	GetStatistics(ctx context.Context, dto GetStatisticsDTO) (*AssessmentStatistics, error)

	// Retry 重试失败的测评
	// 场景：管理员对评估失败的测评进行重试
	Retry(ctx context.Context, assessmentID uint64) (*AssessmentResult, error)
}

// ==================== 评估引擎服务 ====================

// EvaluationService 评估服务
// 行为者：评估引擎 (Evaluation Engine / qs-worker)
// 职责：执行计分、解读、生成报告
// 变更来源：评估算法和流程变化
// 说明：此服务由 qs-worker 调用，消费 AssessmentSubmittedEvent
type EvaluationService interface {
	// Evaluate 执行评估
	// 场景：qs-worker 消费 AssessmentSubmittedEvent 后调用
	// 流程：
	//   1. 加载 Assessment、MedicalScale、Questionnaire、AnswerSheet
	//   2. 调用 calculation 功能域计算各因子得分
	//   3. 调用 interpretation 功能域生成解读
	//   4. 组装 EvaluationResult
	//   5. 应用评估结果到 Assessment
	//   6. 保存 AssessmentScore
	//   7. 生成并保存 InterpretReport
	Evaluate(ctx context.Context, assessmentID uint64) error

	// EvaluateBatch 批量评估
	// 场景：批量处理积压的测评任务
	EvaluateBatch(ctx context.Context, assessmentIDs []uint64) (*BatchEvaluationResult, error)
}

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

	// ExportPDF 导出PDF报告
	// 场景：用户下载PDF格式的测评报告
	ExportPDF(ctx context.Context, assessmentID uint64) ([]byte, error)
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
