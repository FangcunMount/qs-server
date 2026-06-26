package report

// DimensionInterpret 维度解读值对象
// 记录单个因子/维度的解读信息（含解读与建议）
type DimensionInterpret struct {
	code        DimensionCode
	kind        DimensionKind
	factorCode  FactorCode
	factorName  string
	rawScore    float64
	maxScore    *float64
	riskLevel   RiskLevel
	severity    string
	description string
	suggestion  string
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

// NewNeutralDimensionInterpret creates a dimension interpret with explicit neutral metadata.
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
		if isRiskLevelCode(level.Code) {
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

// Code returns the neutral dimension code.
func (d DimensionInterpret) Code() DimensionCode {
	return d.code
}

// Kind returns the neutral dimension kind.
func (d DimensionInterpret) Kind() DimensionKind {
	return d.kind
}

// FactorCode 获取因子编码
func (d DimensionInterpret) FactorCode() FactorCode {
	return d.factorCode
}

// FactorName 获取因子名称
func (d DimensionInterpret) FactorName() string {
	return d.factorName
}

// RawScore 获取原始得分
func (d DimensionInterpret) RawScore() float64 {
	return d.rawScore
}

// RiskLevel 获取风险等级
func (d DimensionInterpret) RiskLevel() RiskLevel {
	return d.riskLevel
}

// Severity returns the neutral severity label.
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

// IsHighRisk 是否高风险
func (d DimensionInterpret) IsHighRisk() bool {
	return IsHighRisk(d.riskLevel)
}

// IsHighSeverity reports whether the dimension has elevated severity.
func (d DimensionInterpret) IsHighSeverity() bool {
	if IsHighSeverity(d.Severity()) {
		return true
	}
	return d.IsHighRisk()
}
