package scale

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// FactorManager 因子管理领域服务
// 负责量表中因子的增删改操作
// 通过调用聚合根的私有方法来修改状态，保证领域完整性
type FactorManager struct{}

// AddFactor 添加因子到量表
func (FactorManager) AddFactor(m *MedicalScale, factor *Factor) error {
	if factor == nil {
		return errors.WithCode(code.ErrInvalidArgument, "因子对象不能为空")
	}

	// 调用聚合根的私有方法（会自动检查编码重复）
	return m.addFactor(factor)
}

// RemoveFactor 从量表中移除指定因子
func (FactorManager) RemoveFactor(m *MedicalScale, factorCode FactorCode) error {
	if factorCode.IsEmpty() {
		return errors.WithCode(code.ErrInvalidArgument, "因子编码不能为空")
	}

	// 调用聚合根的私有方法
	return m.removeFactor(factorCode)
}

// RemoveAllFactors 清空量表中的所有因子
func (FactorManager) RemoveAllFactors(m *MedicalScale) {
	m.factors = []*Factor{}
}

// ReplaceFactors 替换量表的所有因子
// 先清空现有因子，再按顺序添加新因子
func (FactorManager) ReplaceFactors(m *MedicalScale, factors []*Factor) error {
	if len(factors) == 0 {
		return errors.WithCode(code.ErrInvalidArgument, "因子列表不能为空")
	}

	// 1. 验证所有因子的有效性和编码唯一性
	codes := make(map[string]bool)
	hasTotalScore := false

	for i, factor := range factors {
		if factor == nil {
			return errors.WithCode(code.ErrInvalidArgument, "第 %d 个因子对象为空", i+1)
		}

		factorCode := factor.GetCode().Value()
		if factorCode == "" {
			return errors.WithCode(code.ErrInvalidArgument, "第 %d 个因子的编码不能为空", i+1)
		}

		if codes[factorCode] {
			return errors.WithCode(code.ErrInvalidArgument, "因子编码 %s 重复", factorCode)
		}
		codes[factorCode] = true

		if factor.IsTotalScore() {
			if hasTotalScore {
				return errors.WithCode(code.ErrInvalidArgument, "量表只能有一个总分因子")
			}
			hasTotalScore = true
		}
	}

	// 2. 替换因子列表
	m.updateFactors(factors)

	return nil
}

// UpdateFactor 更新指定因子
// 通过编码查找并替换因子
func (FactorManager) UpdateFactor(m *MedicalScale, updatedFactor *Factor) error {
	if updatedFactor == nil {
		return errors.WithCode(code.ErrInvalidArgument, "因子对象不能为空")
	}

	factorCode := updatedFactor.GetCode()
	if factorCode.IsEmpty() {
		return errors.WithCode(code.ErrInvalidArgument, "因子编码不能为空")
	}

	// 查找并替换
	for i, f := range m.factors {
		if f.GetCode().Equals(factorCode) {
			m.factors[i] = updatedFactor
			return nil
		}
	}

	return errors.WithCode(code.ErrInvalidArgument, "未找到编码为 %s 的因子", factorCode.Value())
}

// UpdateFactorInterpretRules 更新指定因子的解读规则
func (FactorManager) UpdateFactorInterpretRules(m *MedicalScale, factorCode FactorCode, rules []InterpretationRule) error {
	if factorCode.IsEmpty() {
		return errors.WithCode(code.ErrInvalidArgument, "因子编码不能为空")
	}

	// 查找因子
	factor, found := m.FindFactorByCode(factorCode)
	if !found {
		return errors.WithCode(code.ErrInvalidArgument, "未找到编码为 %s 的因子", factorCode.Value())
	}

	// 验证解读规则
	for i, rule := range rules {
		if !rule.IsValid() {
			return errors.WithCode(code.ErrInvalidArgument, "第 %d 个解读规则无效", i+1)
		}
	}

	// 更新解读规则
	factor.updateInterpretRules(rules)

	return nil
}

// AddFactorInterpretRule 为指定因子添加解读规则
func (FactorManager) AddFactorInterpretRule(m *MedicalScale, factorCode FactorCode, rule InterpretationRule) error {
	if factorCode.IsEmpty() {
		return errors.WithCode(code.ErrInvalidArgument, "因子编码不能为空")
	}

	if !rule.IsValid() {
		return errors.WithCode(code.ErrInvalidArgument, "解读规则无效")
	}

	// 查找因子
	factor, found := m.FindFactorByCode(factorCode)
	if !found {
		return errors.WithCode(code.ErrInvalidArgument, "未找到编码为 %s 的因子", factorCode.Value())
	}

	// 添加解读规则
	factor.addInterpretRule(rule)

	return nil
}
