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
	MedicalScaleID       *uint64 // 量表ID（可选，纯问卷模式为空）
	MedicalScaleCode     *string // 量表编码（可选）
	MedicalScaleName     *string // 量表名称（可选）
	ScaleVersion         *string // 量表解释模型版本（可选）
	ModelKind            *string // 通用解释模型类型（可选）
	ModelSubKind         *string // v2 子形态（typology 等）
	ModelAlgorithm       *string // v2 算法（mbti/sbti/scale_default 等）
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

// ListReportsDTO 查询报告列表 DTO
type ListReportsDTO struct {
	TesteeID              uint64   // 受试者ID
	Page                  int      // 页码
	PageSize              int      // 每页数量
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

// ReportResult 报告结果
type ReportResult struct {
	AssessmentID uint64            // 测评ID
	ModelName    string            // 解释模型名称
	ModelCode    string            // 解释模型编码
	TotalScore   float64           // 总分
	RiskLevel    string            // 风险等级
	Conclusion   string            // 总结论
	Dimensions   []DimensionResult // 维度解读列表
	Suggestions  []SuggestionDTO   // 建议列表
	CreatedAt    time.Time         // 创建时间
	ModelExtra   *ModelExtraResult // 解释模型扩展（SBTI 等）
}

// ModelExtraResult 解释模型扩展结果
type ModelExtraResult struct {
	Kind           string             `json:"kind,omitempty"`
	TypeCode       string             `json:"type_code,omitempty"`
	TypeName       string             `json:"type_name,omitempty"`
	OneLiner       string             `json:"one_liner,omitempty"`
	ImageURL       string             `json:"image_url,omitempty"`
	MatchPercent   float64            `json:"match_percent,omitempty"`
	IsSpecial      bool               `json:"is_special,omitempty"`
	SpecialTrigger string             `json:"special_trigger,omitempty"`
	Commentary     string             `json:"commentary,omitempty"`
	Rarity         *ModelRarityResult `json:"rarity,omitempty"`
}

// ModelRarityResult 理论稀有度
type ModelRarityResult struct {
	Percent float64 `json:"percent,omitempty"`
	Label   string  `json:"label,omitempty"`
	OneInX  int     `json:"one_in_x,omitempty"`
}

// DimensionResult 维度解读结果
type DimensionResult struct {
	FactorCode     string   // 因子编码
	FactorName     string   // 因子名称
	RawScore       float64  // 原始分
	MaxScore       *float64 // 最大分
	RiskLevel      string   // 风险等级
	Role           string   `json:"role,omitempty"`
	ParentCode     string   `json:"parent_code,omitempty"`
	HierarchyLevel int      `json:"hierarchy_level,omitempty"`
	SortOrder      int      `json:"sort_order,omitempty"`
	Description    string   // 解读描述
	Suggestion     string   // 维度建议
}

// SuggestionDTO 结构化建议
type SuggestionDTO struct {
	Category   string  // 建议分类
	Content    string  // 文本
	FactorCode *string // 关联因子编码（可选）
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
	FactorCode   string   // 因子编码
	FactorName   string   // 因子名称
	RawScore     float64  // 原始分
	MaxScore     *float64 // 最大分
	RiskLevel    string   // 风险等级
	Conclusion   string   // 结论
	Suggestion   string   // 建议
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
