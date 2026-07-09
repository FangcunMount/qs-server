package factor

// HierarchyIssue 记录一个因子 层级 校验 problem。
type HierarchyIssue struct {
	Field   string
	Code    string
	Message string
}

// ValidateFactors 检查multi-等级 因子 invariants 用于 flat 因子 list。
// Models 不使用 Parent编码 pass 使用 zero issues。
func ValidateFactors(factors []LegacyFactor) []HierarchyIssue {
	if len(factors) == 0 {
		return nil
	}
	return ValidateMeasureSpecParts(SlimFactorsFromLegacy(factors), FactorGraphFromLegacy(factors), ScoringFromLegacy(factors))
}
