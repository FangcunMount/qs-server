package assessment

import "time"

// ============= 输入 DTO 定义 =============
// DTOs 用于应用服务层的输入参数

// CreateAssessmentDTO 创建测评 DTO
type CreateAssessmentDTO struct {
	OrgID                uint64  // 组织ID
	TesteeID             uint64  // 受试者ID
	QuestionnaireID      uint64  // 问卷ID
	QuestionnaireCode    string  // 问卷编码
	QuestionnaireVersion string  // 问卷版本
	AnswerSheetID        uint64  // 答卷ID
	MedicalScaleID       *uint64 // 量表ID（可选，纯问卷模式为空）
	MedicalScaleCode     *string // 量表编码（可选）
	MedicalScaleName     *string // 量表名称（可选）
	OriginType           string  // 来源类型：adhoc/plan/screening
	OriginID             *string // 来源ID（计划ID或筛查项目ID）
}

// ListMyAssessmentsDTO 查询我的测评列表 DTO
type ListMyAssessmentsDTO struct {
	TesteeID uint64 // 受试者ID
	Page     int    // 页码
	PageSize int    // 每页数量
	Status   string // 状态筛选（可选）
}

// ListAssessmentsDTO 查询测评列表 DTO
type ListAssessmentsDTO struct {
	OrgID      uint64            // 组织ID
	Page       int               // 页码
	PageSize   int               // 每页数量
	Conditions map[string]string // 查询条件
}

// GetStatisticsDTO 获取统计数据 DTO
type GetStatisticsDTO struct {
	OrgID     uint64     // 组织ID
	StartTime *time.Time // 开始时间（可选）
	EndTime   *time.Time // 结束时间（可选）
	ScaleCode *string    // 量表编码筛选（可选）
}

// ListReportsDTO 查询报告列表 DTO
type ListReportsDTO struct {
	TesteeID uint64 // 受试者ID
	Page     int    // 页码
	PageSize int    // 每页数量
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
	QuestionnaireID      uint64     // 问卷ID
	QuestionnaireCode    string     // 问卷编码
	QuestionnaireVersion string     // 问卷版本
	AnswerSheetID        uint64     // 答卷ID
	MedicalScaleID       *uint64    // 量表ID
	MedicalScaleCode     *string    // 量表编码
	MedicalScaleName     *string    // 量表名称
	OriginType           string     // 来源类型
	OriginID             *string    // 来源ID
	Status               string     // 状态
	TotalScore           *float64   // 总分
	RiskLevel            *string    // 风险等级
	SubmittedAt          *time.Time // 提交时间
	InterpretedAt        *time.Time // 解读时间
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

// AssessmentStatistics 测评统计
type AssessmentStatistics struct {
	TotalCount       int               // 总测评数
	PendingCount     int               // 待提交数
	SubmittedCount   int               // 已提交数
	InterpretedCount int               // 已解读数
	FailedCount      int               // 失败数
	AverageScore     *float64          // 平均分（仅已解读）
	RiskDistribution map[string]int    // 风险等级分布
	ScaleStats       []ScaleStatistics // 按量表统计
}

// ScaleStatistics 量表统计
type ScaleStatistics struct {
	ScaleCode    string   // 量表编码
	ScaleName    string   // 量表名称
	Count        int      // 测评数
	AverageScore *float64 // 平均分
}

// ReportResult 报告结果
type ReportResult struct {
	AssessmentID uint64            // 测评ID
	ScaleName    string            // 量表名称
	ScaleCode    string            // 量表编码
	TotalScore   float64           // 总分
	RiskLevel    string            // 风险等级
	Conclusion   string            // 总结论
	Dimensions   []DimensionResult // 维度解读列表
	Suggestions  []string          // 建议列表
	CreatedAt    time.Time         // 创建时间
}

// DimensionResult 维度解读结果
type DimensionResult struct {
	FactorCode  string  // 因子编码
	FactorName  string  // 因子名称
	RawScore    float64 // 原始分
	RiskLevel   string  // 风险等级
	Description string  // 解读描述
}

// ReportListResult 报告列表结果
type ReportListResult struct {
	Items      []*ReportResult // 报告列表
	Total      int             // 总数
	Page       int             // 当前页
	PageSize   int             // 每页数量
	TotalPages int             // 总页数
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
	FactorCode   string  // 因子编码
	FactorName   string  // 因子名称
	RawScore     float64 // 原始分
	RiskLevel    string  // 风险等级
	Conclusion   string  // 结论
	Suggestion   string  // 建议
	IsTotalScore bool    // 是否为总分因子
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
