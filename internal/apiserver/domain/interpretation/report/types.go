package report

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// ID 报告ID类型（与 AssessmentID 一致，使用 meta.ID）
type ID = meta.ID

// AssessmentID 测评ID类型（用于关联 assessment 聚合）
type AssessmentID = ID

// RiskLevel 风险等级
type RiskLevel string

const (
	RiskLevelNone   RiskLevel = "none"
	RiskLevelLow    RiskLevel = "low"
	RiskLevelMedium RiskLevel = "medium"
	RiskLevelHigh   RiskLevel = "high"
	RiskLevelSevere RiskLevel = "severe"
)

func (r RiskLevel) String() string {
	return string(r)
}

// IsHighRisk 是否高风险（包含 high 和 severe）
func IsHighRisk(r RiskLevel) bool {
	return r == RiskLevelHigh || r == RiskLevelSevere
}

// FactorCode 因子编码
type FactorCode string

// NewFactorCode 创建因子编码
func NewFactorCode(code string) FactorCode {
	return FactorCode(code)
}

func (c FactorCode) Value() string {
	return string(c)
}

func (c FactorCode) String() string {
	return string(c)
}

func (c FactorCode) IsEmpty() bool {
	return c == ""
}

func (c FactorCode) Equals(other FactorCode) bool {
	return c == other
}

// DimensionCode 是中性维度 identifier on reports。
type DimensionCode string

func NewDimensionCode(code string) DimensionCode {
	return DimensionCode(code)
}

func (c DimensionCode) String() string {
	return string(c)
}

func (c DimensionCode) IsEmpty() bool {
	return c == ""
}

func (c DimensionCode) Equals(other DimensionCode) bool {
	return c == other
}

// DimensionKind 划分 report 维度，独立于 scale 因子 semantics。
type DimensionKind string

const (
	DimensionKindFactor  DimensionKind = "factor"
	DimensionKindPole    DimensionKind = "pole"
	DimensionKindTrait   DimensionKind = "trait"
	DimensionKindIndex   DimensionKind = "index"
	DimensionKindAbility DimensionKind = "ability"
)

// FactorScoreInput 因子得分输入（报告构建共用）。
type FactorScoreInput struct {
	FactorCode     FactorCode
	FactorName     string
	RawScore       float64
	MaxScore       *float64
	RiskLevel      RiskLevel
	Description    string
	Suggestion     string
	IsTotalScore   bool
	Role           string
	ParentCode     string
	HierarchyLevel int
	SortOrder      int
}
