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
	issues := ValidateMeasureSpecParts(
		FactorsFromDefinitionDimensions(body.Dimensions),
		FactorGraphFromDefinitionDimensions(body.Dimensions),
		ScoringFromDefinitionDimensions(body.Dimensions),
	)
	issues = append(issues, validateInterpretRuleRefs(body.InterpretRules, body.Dimensions)...)
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

func validateInterpretRuleRefs(rules []InterpretRule, dimensions []DimensionRule) []HierarchyIssue {
	if len(rules) == 0 {
		return nil
	}
	byCode := make(map[string]struct{}, len(dimensions))
	for _, dimension := range dimensions {
		byCode[dimension.Code] = struct{}{}
	}
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
