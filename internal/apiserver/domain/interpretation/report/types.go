package report

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// ID is the identifier type used by report composition inputs.
type ID = meta.ID

// RiskLevel 风险等级
type RiskLevel string

const (
	RiskLevelNone   RiskLevel = "none"
	RiskLevelLow    RiskLevel = "low"
	RiskLevelMedium RiskLevel = "medium"
	RiskLevelHigh   RiskLevel = "high"
	RiskLevelSevere RiskLevel = "severe"
)

// FactorCode 因子编码
type FactorCode string

// NewFactorCode 创建因子编码
func NewFactorCode(code string) FactorCode {
	return FactorCode(code)
}

func (c FactorCode) String() string {
	return string(c)
}

// DimensionCode 是中性维度 identifier on reports。
type DimensionCode string

func NewDimensionCode(code string) DimensionCode {
	return DimensionCode(code)
}

func (c DimensionCode) String() string {
	return string(c)
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
