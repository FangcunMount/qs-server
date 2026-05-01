package scale

// FactorManager 因子管理领域服务
// 负责量表中因子的增删改操作
// 通过调用聚合根的私有方法来修改状态，保证领域完整性
type FactorManager struct{}

// AddFactor 添加因子到量表
func (FactorManager) AddFactor(m *MedicalScale, factor *Factor) error {
	return m.AddFactor(factor)
}

// RemoveFactor 从量表中移除指定因子
func (FactorManager) RemoveFactor(m *MedicalScale, factorCode FactorCode) error {
	return m.RemoveFactor(factorCode)
}

// RemoveAllFactors 清空量表中的所有因子
func (FactorManager) RemoveAllFactors(m *MedicalScale) {
	m.RemoveAllFactors()
}

// ReplaceFactors 替换量表的所有因子
// 先清空现有因子，再按顺序添加新因子
func (FactorManager) ReplaceFactors(m *MedicalScale, factors []*Factor) error {
	return m.ReplaceFactors(factors)
}

// UpdateFactor 更新指定因子
// 通过编码查找并替换因子
func (FactorManager) UpdateFactor(m *MedicalScale, updatedFactor *Factor) error {
	return m.UpdateFactor(updatedFactor)
}

// UpdateFactorInterpretRules 更新指定因子的解读规则
func (FactorManager) UpdateFactorInterpretRules(m *MedicalScale, factorCode FactorCode, rules []InterpretationRule) error {
	return m.UpdateFactorInterpretRules(factorCode, rules)
}

// AddFactorInterpretRule 为指定因子添加解读规则
func (FactorManager) AddFactorInterpretRule(m *MedicalScale, factorCode FactorCode, rule InterpretationRule) error {
	return m.AddFactorInterpretRule(factorCode, rule)
}
