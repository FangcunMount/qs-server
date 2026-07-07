package factor

// HierarchyMeta carries optional parent/ordering metadata for multi-level factor trees.
// Level is usually derived from ParentCode during validation; payload may omit it.
type HierarchyMeta struct {
	ParentCode string
	SortOrder  int
	Level      int
}

// ScorableRole reports whether a factor role participates in scoring pipelines.
func ScorableRole(role FactorRole) bool {
	switch role.Resolved() {
	case FactorRoleDimension, FactorRoleTotal, FactorRoleIndex,
		FactorRoleValidity, FactorRoleSubtest, FactorRoleTaskSet, FactorRoleAbilityDomain:
		return true
	default:
		return false
	}
}

// RequiresChildrenPolicy reports whether publish validation must require ChildrenPolicy.
func RequiresChildrenPolicy(role FactorRole) bool {
	return role.Resolved() == FactorRoleIndex
}

// BindsQuestions reports whether a role may carry question_codes.
func BindsQuestions(role FactorRole) bool {
	switch role.Resolved() {
	case FactorRoleDimension, FactorRoleTotal, FactorRoleValidity,
		FactorRoleSubtest, FactorRoleTaskSet, FactorRoleAbilityDomain:
		return true
	default:
		return false
	}
}

// Resolved normalizes empty role to dimension.
func (r FactorRole) Resolved() FactorRole {
	if r == "" {
		return FactorRoleDimension
	}
	return r
}

// IndexByCode builds a lookup map for hierarchy validation and tree materialization.
func IndexByCode(factors []FactorSnapshot) map[string]FactorSnapshot {
	index := make(map[string]FactorSnapshot, len(factors))
	for _, factor := range factors {
		index[factor.Code] = factor
	}
	return index
}

// InferParentCodesFromChildrenPolicy fills empty ParentCode from index ChildrenPolicy edges,
// then derives hierarchy levels.
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

// DeriveLevels fills Level from ParentCode when missing. Returns a copy with derived levels.
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
