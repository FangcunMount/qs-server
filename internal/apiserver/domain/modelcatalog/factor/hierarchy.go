package factor

// HierarchyMeta 携带可选 父节点/ordering 元数据 用于 multi-等级 因子 trees。
// Level 是usually 派生 从 Parent编码 在 校验; 载荷 may omit it。
type HierarchyMeta struct {
	ParentCode string
	SortOrder  int
	Level      int
}

// ScorableRole 报告是否 因子 角色 participates in 计分 pipelines。
func ScorableRole(role FactorRole) bool {
	switch role.Resolved() {
	case FactorRoleDimension, FactorRoleTotal, FactorRoleIndex,
		FactorRoleValidity, FactorRoleSubtest, FactorRoleTaskSet, FactorRoleAbilityDomain:
		return true
	default:
		return false
	}
}

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

// IndexByCode 构建lookup 映射 用于 层级 校验 和 tree 物化。
func IndexByCode(factors []FactorSnapshot) map[string]FactorSnapshot {
	index := make(map[string]FactorSnapshot, len(factors))
	for _, factor := range factors {
		index[factor.Code] = factor
	}
	return index
}

// InferParentCodesFromChildrenPolicy fills 空 Parent编码 从 index 子节点策略 edges,。
// then derives 层级 等级。
func InferParentCodesFromChildrenPolicy(factors []FactorSnapshot) []FactorSnapshot {
	if len(factors) == 0 {
		return nil
	}
	out := make([]FactorSnapshot, len(factors))
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
	return DeriveLevels(out)
}

// DeriveLevels fills 等级 从 Parent编码 when 缺失. Returns copy 使用 派生 等级。
func DeriveLevels(factors []FactorSnapshot) []FactorSnapshot {
	if len(factors) == 0 {
		return nil
	}
	byCode := IndexByCode(factors)
	derived := make([]FactorSnapshot, len(factors))
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
