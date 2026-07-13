package report

import "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"

// DimensionInterpret 维度解读值对象
// 记录单个因子/维度的解读信息（含解读与建议）
type DimensionInterpret struct {
	code           DimensionCode
	kind           DimensionKind
	factorCode     FactorCode
	factorName     string
	rawScore       float64
	maxScore       *float64
	riskLevel      RiskLevel
	severity       string
	description    string
	suggestion     string
	role           string
	parentCode     string
	hierarchyLevel int
	sortOrder      int
}

// NewDimensionInterpret 创建维度解读
func NewDimensionInterpret(
	factorCode FactorCode,
	factorName string,
	rawScore float64,
	maxScore *float64,
	riskLevel RiskLevel,
	description string,
	suggestion string,
) DimensionInterpret {
	return DimensionInterpret{
		code:        NewDimensionCode(factorCode.String()),
		kind:        DimensionKindFactor,
		factorCode:  factorCode,
		factorName:  factorName,
		rawScore:    rawScore,
		maxScore:    maxScore,
		riskLevel:   riskLevel,
		severity:    severityFromRiskLevel(riskLevel),
		description: description,
		suggestion:  suggestion,
	}
}

// NewNeutralDimensionInterpret 创建维度 interpret 使用显式中性元数据。
func NewNeutralDimensionInterpret(
	code DimensionCode,
	kind DimensionKind,
	name string,
	rawScore float64,
	maxScore *float64,
	level *ResultLevel,
	description string,
	suggestion string,
) DimensionInterpret {
	risk := RiskLevelNone
	severity := ""
	if level != nil {
		severity = level.Severity
		if eventoutcome.IsRiskLevelCode(level.Code) {
			risk = RiskLevel(level.Code)
		}
	}
	return DimensionInterpret{
		code:        code,
		kind:        kind,
		factorCode:  FactorCode(code),
		factorName:  name,
		rawScore:    rawScore,
		maxScore:    maxScore,
		riskLevel:   risk,
		severity:    severity,
		description: description,
		suggestion:  suggestion,
	}
}

func severityFromRiskLevel(risk RiskLevel) string {
	if risk == "" || risk == RiskLevelNone {
		return "none"
	}
	return string(risk)
}

// Code 返回中性维度编码。
func (d DimensionInterpret) Code() DimensionCode {
	return d.code
}

// Kind 返回中性维度类型。
func (d DimensionInterpret) Kind() DimensionKind {
	return d.kind
}

// Name 返回中性维度 display name。
func (d DimensionInterpret) Name() string {
	return d.factorName
}

// RawScore 获取原始得分
func (d DimensionInterpret) RawScore() float64 {
	return d.rawScore
}

// Severity 返回中性 severity label。
func (d DimensionInterpret) Severity() string {
	if d.severity != "" {
		return d.severity
	}
	return severityFromRiskLevel(d.riskLevel)
}

// Description 获取解读描述
func (d DimensionInterpret) Description() string {
	return d.description
}

// Suggestion 获取维度建议
func (d DimensionInterpret) Suggestion() string {
	return d.suggestion
}

// MaxScore 获取最大分
func (d DimensionInterpret) MaxScore() *float64 {
	return d.maxScore
}

// Role 返回目录因子角色 when 存在。
func (d DimensionInterpret) Role() string {
	return d.role
}

// ParentCode 返回父节点因子编码 in 层级 tree。
func (d DimensionInterpret) ParentCode() string {
	return d.parentCode
}

// HierarchyLevel 返回 tree depth; 1 是根。
func (d DimensionInterpret) HierarchyLevel() int {
	return d.hierarchyLevel
}

// SortOrder 返回 sibling ordering 在 same 父节点。
func (d DimensionInterpret) SortOrder() int {
	return d.sortOrder
}

// WithHierarchy 返回 copy annotated 使用因子 tree 元数据。
func (d DimensionInterpret) WithHierarchy(role, parentCode string, hierarchyLevel, sortOrder int) DimensionInterpret {
	d.role = role
	d.parentCode = parentCode
	d.hierarchyLevel = hierarchyLevel
	d.sortOrder = sortOrder
	return d
}
