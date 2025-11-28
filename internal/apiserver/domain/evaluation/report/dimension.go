package report

// ==================== DimensionInterpret 值对象 ====================

// DimensionInterpret 维度解读值对象
// 记录单个因子/维度的解读信息
type DimensionInterpret struct {
	factorCode  FactorCode
	factorName  string
	rawScore    float64
	riskLevel   RiskLevel
	description string
}

// NewDimensionInterpret 创建维度解读
func NewDimensionInterpret(
	factorCode FactorCode,
	factorName string,
	rawScore float64,
	riskLevel RiskLevel,
	description string,
) DimensionInterpret {
	return DimensionInterpret{
		factorCode:  factorCode,
		factorName:  factorName,
		rawScore:    rawScore,
		riskLevel:   riskLevel,
		description: description,
	}
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

// Description 获取解读描述
func (d DimensionInterpret) Description() string {
	return d.description
}

// IsHighRisk 是否高风险
func (d DimensionInterpret) IsHighRisk() bool {
	return IsHighRisk(d.riskLevel)
}
