package factor

// RequiresChildrenPolicy 报告是否 publish 校验 必须 require 子节点策略。
func RequiresChildrenPolicy(role FactorRole) bool {
	return role.Resolved() == FactorRoleIndex
}

// BindsQuestions 报告是否 角色 may 携带 question_编码。
func BindsQuestions(role FactorRole) bool {
	switch role.Resolved() {
	case FactorRoleDimension, FactorRoleTotal, FactorRoleValidity,
		FactorRoleSubtest, FactorRoleTaskSet, FactorRoleAbilityDomain, FactorRoleIndex:
		return true
	default:
		return false
	}
}

// Resolved 归一化空 角色 到 维度。
func (r FactorRole) Resolved() FactorRole {
	if r == "" {
		return FactorRoleDimension
	}
	return r
}

// IndexByFactorCode 构建领域 Factor lookup 映射，用于层级校验和 tree 物化。
func IndexByFactorCode(factors []Factor) map[string]Factor {
	index := make(map[string]Factor, len(factors))
	for _, factor := range factors {
		index[factor.Code] = factor
	}
	return index
}
