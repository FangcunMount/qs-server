package assessment

import "time"

// ============= 输入 DTO 定义 =============
// DTOs 用于应用服务层的输入参数

// CreateAssessmentDTO 创建测评 DTO
type CreateAssessmentDTO struct {
	OrgID                uint64  // 组织ID
	TesteeID             uint64  // 受试者ID
	QuestionnaireCode    string  // 问卷编码（唯一标识）
	QuestionnaireVersion string  // 问卷版本
	AnswerSheetID        uint64  // 答卷ID
	ModelKind            *string // 通用解释模型类型（可选）
	ModelSubKind         *string // v2 子形态（typology 等）
	ModelAlgorithm       *string // v2 算法（mbti/sbti/scale_默认 等）
	ModelCode            *string // 通用解释模型编码（可选）
	ModelVersion         *string // 通用解释模型版本（可选）
	ModelTitle           *string // 通用解释模型标题（可选）
	OriginType           string  // 来源类型：adhoc/plan
	OriginID             *string // 来源ID（计划ID）
}

// ListMyAssessmentsDTO 查询我的测评列表 DTO
type ListMyAssessmentsDTO struct {
	TesteeID  uint64     // 受试者ID
	Page      int        // 页码
	PageSize  int        // 每页数量
	Status    string     // 状态筛选（可选）
	ScaleCode string     // 量表编码筛选（可选）
	RiskLevel string     // 风险等级筛选（可选）
	ModelKind string     // 解释模型类型筛选（可选）：scale/personality
	DateFrom  *time.Time // 开始日期筛选（可选）
	DateTo    *time.Time // 结束日期筛选（可选）
}

// ListAssessmentsDTO 查询测评列表 DTO
type ListAssessmentsDTO struct {
	OrgID                 uint64   // 组织ID
	Page                  int      // 页码
	PageSize              int      // 每页数量
	TesteeID              *uint64  // 受试者ID筛选（可选）
	Status                string   // 状态筛选（可选）
	AccessibleTesteeIDs   []uint64 // 可访问的受试者范围（可选）
	RestrictToAccessScope bool     // 是否按可访问范围过滤
}

// GetFactorTrendDTO 获取因子趋势 DTO
type GetFactorTrendDTO struct {
	TesteeID   uint64 // 受试者ID
	FactorCode string // 因子编码
	Limit      int    // 返回记录数限制
}

// ============= 输出 Result 定义 =============
// Results 用于应用服务层的返回值

// AssessmentResult 测评结果
type AssessmentResult struct {
	ID                   uint64     // 测评ID
	OrgID                uint64     // 组织ID
	TesteeID             uint64     // 受试者ID
	QuestionnaireCode    string     // 问卷编码（唯一标识）
	QuestionnaireVersion string     // 问卷版本
	AnswerSheetID        uint64     // 答卷ID
	ModelKind            *string    // 解释模型类型
	ModelSubKind         *string    // 解释模型子类型
	ModelAlgorithm       *string    // 解释模型算法
	ModelCode            *string    // 解释模型编码
	ModelVersion         *string    // 解释模型版本
	ModelTitle           *string    // 解释模型标题
	OriginType           string     // 来源类型
	OriginID             *string    // 来源ID
	Status               string     // 状态
	TotalScore           *float64   // 总分
	RiskLevel            *string    // 风险等级
	SubmittedAt          *time.Time // 提交时间
	EvaluatedAt          *time.Time // 评分完成时间
	FailedAt             *time.Time // 失败时间
	FailureReason        *string    // 失败原因
}

// AssessmentListResult 测评列表结果
type AssessmentListResult struct {
	Items      []*AssessmentResult // 测评列表
	Total      int                 // 总数
	Page       int                 // 当前页
	PageSize   int                 // 每页数量
	TotalPages int                 // 总页数
}

// ScoreResult 得分结果
type ScoreResult struct {
	AssessmentID uint64              // 测评ID
	TotalScore   float64             // 总分
	RiskLevel    string              // 整体风险等级
	FactorScores []FactorScoreResult // 因子得分列表
}

// FactorScoreResult 因子得分结果
type FactorScoreResult struct {
	FactorCode   string   // 因子编码
	FactorName   string   // 因子名称
	RawScore     float64  // 原始分
	MaxScore     *float64 // 最大分
	RiskLevel    string   // 风险等级
	IsTotalScore bool     // 是否为总分因子
}

// FactorTrendResult 因子趋势结果
type FactorTrendResult struct {
	TesteeID   uint64           // 受试者ID
	FactorCode string           // 因子编码
	FactorName string           // 因子名称
	DataPoints []TrendDataPoint // 数据点列表
}

// TrendDataPoint 趋势数据点
type TrendDataPoint struct {
	AssessmentID uint64  // 测评ID
	RawScore     float64 // 得分
	RiskLevel    string  // 风险等级
}

// HighRiskFactorsResult 高风险因子结果
type HighRiskFactorsResult struct {
	AssessmentID    uint64              // 测评ID
	HasHighRisk     bool                // 是否存在高风险
	HighRiskFactors []FactorScoreResult // 高风险因子列表
	NeedsUrgentCare bool                // 是否需要紧急关注
}
