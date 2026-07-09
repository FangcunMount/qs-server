package factor

// RequiresChildrenPolicy 报告是否 publish 校验 必须 require 子节点策略。
func RequiresChildrenPolicy(role FactorRole) bool {
	return role.Resolved() == FactorRoleIndex
}

// BindsQuestions 报告是否 角色 may 携带 question_编码。
func BindsQuestions(role FactorRole) bool {
	switch role.Resolved() {
	case FactorRoleDimension, FactorRoleTotal, FactorRoleValidity,
		FactorRoleSubtest, FactorRoleTaskSet, FactorRoleAbilityDomain:
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

// IndexByLegacyFactorCode 构建 legacy flat factor lookup 映射，用于兼容 DTO 测试和投影。
func IndexByLegacyFactorCode(factors []LegacyFactor) map[string]LegacyFactor {
	index := make(map[string]LegacyFactor, len(factors))
	for _, factor := range factors {
		index[factor.Code] = factor
	}
	return index
}

// InferParentCodesFromFactorChildrenPolicy fills 空 Parent编码 从 index 子节点策略 edges,
// then derives 层级 等级。
func InferParentCodesFromFactorChildrenPolicy(factors []LegacyFactor) []LegacyFactor {
	if len(factors) == 0 {
		return nil
	}
	out := make([]LegacyFactor, len(factors))
	copy(out, factors)
	for _, parent := range out {
		if parent.ChildrenPolicy == nil {
			continue
		}
		for _, childCode := range parent.ChildrenPolicy.Children {
			for i := range out {
				if out[i].Code != childCode || out[i].ParentCode != "" {
					continue
				}
				out[i].ParentCode = parent.Code
			}
		}
	}
	return DeriveFactorLevels(out)
}

// DeriveFactorLevels fills 等级 从 Parent编码 when 缺失. Returns copy 使用 派生 等级。
func DeriveFactorLevels(factors []LegacyFactor) []LegacyFactor {
	if len(factors) == 0 {
		return nil
	}
	byCode := IndexByLegacyFactorCode(factors)
	derived := make([]LegacyFactor, len(factors))
	copy(derived, factors)

	memo := make(map[string]int, len(factors))
	var walk func(code string) int
	walk = func(code string) int {
		if level, ok := memo[code]; ok {
			return level
		}
		factor, ok := byCode[code]
		if !ok {
			return 0
		}
		if factor.Level > 0 {
			memo[code] = factor.Level
			return factor.Level
		}
		if factor.ParentCode == "" {
			memo[code] = 1
			return 1
		}
		level := walk(factor.ParentCode) + 1
		memo[code] = level
		return level
	}
	for i := range derived {
		derived[i].Level = walk(derived[i].Code)
	}
	return derived
}
