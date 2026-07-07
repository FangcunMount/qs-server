package factor

import "fmt"

// ValidateDefinitionBodyForPublish 检查共享 因子 definition invariants 在之前 publish。
func ValidateDefinitionBodyForPublish(body DefinitionBody) []HierarchyIssue {
	if len(body.Dimensions) == 0 {
		return []HierarchyIssue{{
			Field:   "dimensions",
			Code:    "dimensions.required",
			Message: "dimensions 不能为空",
		}}
	}
	factors := DeriveLevels(ParseFactorsFromDefinitionBody(body.Dimensions, body.InterpretRules))
	issues := ValidateFactorHierarchy(factors)
	issues = append(issues, validateInterpretRuleRefs(body.InterpretRules, factors)...)
	issues = append(issues, validateNormRefs(factors)...)
	return issues
}

// ValidateDefinitionBodyJSONForPublish de编码 和 有效ates 共享 因子 definition 载荷。
func ValidateDefinitionBodyJSONForPublish(payload []byte) ([]HierarchyIssue, error) {
	body, err := ParseDefinitionBodyJSON(payload)
	if err != nil {
		return nil, fmt.Errorf("decode factor definition body: %w", err)
	}
	return ValidateDefinitionBodyForPublish(body), nil
}

func validateInterpretRuleRefs(rules []InterpretRule, factors []FactorSnapshot) []HierarchyIssue {
	if len(rules) == 0 {
		return nil
	}
	byCode := IndexByCode(factors)
	issues := make([]HierarchyIssue, 0, len(rules))
	for _, rule := range rules {
		field := "interpret_rules"
		if rule.DimensionCode != "" {
			field = fmt.Sprintf("interpret_rules[%s]", rule.DimensionCode)
		}
		if rule.DimensionCode == "" {
			issues = append(issues, HierarchyIssue{
				Field:   field + ".dimension_code",
				Code:    "interpret_rules.dimension_code.required",
				Message: "interpret_rules.dimension_code 不能为空",
			})
			continue
		}
		if _, ok := byCode[rule.DimensionCode]; !ok {
			issues = append(issues, HierarchyIssue{
				Field:   field + ".dimension_code",
				Code:    "interpret_rules.dimension_code.not_found",
				Message: fmt.Sprintf("interpret_rules 引用了不存在的维度 %s", rule.DimensionCode),
			})
		}
	}
	return issues
}

func validateNormRefs(factors []FactorSnapshot) []HierarchyIssue {
	byCode := IndexByCode(factors)
	issues := make([]HierarchyIssue, 0)
	for _, factor := range factors {
		if factor.Norm == nil {
			continue
		}
		prefix := fmt.Sprintf("factors[%s].norm", factor.Code)
		refCode := factor.Norm.FactorCode
		if refCode == "" {
			refCode = factor.Code
		}
		if _, ok := byCode[refCode]; !ok {
			issues = append(issues, HierarchyIssue{
				Field:   prefix + ".factor_code",
				Code:    "factor.norm.factor_code.not_found",
				Message: fmt.Sprintf("norm 引用了不存在的因子 %s", refCode),
			})
		}
	}
	return issues
}
