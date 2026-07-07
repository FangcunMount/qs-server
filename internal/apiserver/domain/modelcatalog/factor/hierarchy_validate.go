package factor

import "fmt"

// HierarchyIssue records one factor hierarchy validation problem.
type HierarchyIssue struct {
	Field   string
	Code    string
	Message string
}

// ValidateFactorHierarchy checks multi-level factor invariants for a flat factor list.
// Models without ParentCode pass with zero issues.
func ValidateFactorHierarchy(factors []FactorSnapshot) []HierarchyIssue {
	if len(factors) == 0 {
		return nil
	}
	byCode := IndexByCode(factors)
	issues := make([]HierarchyIssue, 0)

	seenCodes := make(map[string]struct{}, len(factors))
	for _, factor := range factors {
		if factor.Code == "" {
			issues = append(issues, HierarchyIssue{
				Field: "code", Code: "factor.code.required", Message: "factor code 不能为空",
			})
			continue
		}
		if _, exists := seenCodes[factor.Code]; exists {
			issues = append(issues, HierarchyIssue{
				Field:   fmt.Sprintf("factors[%s]", factor.Code),
				Code:    "factor.code.duplicate",
				Message: fmt.Sprintf("factor code %s 重复", factor.Code),
			})
		}
		seenCodes[factor.Code] = struct{}{}
	}

	for _, factor := range factors {
		prefix := fmt.Sprintf("factors[%s]", factor.Code)
		role := factor.ResolvedRole()

		if factor.ParentCode != "" {
			parent, ok := byCode[factor.ParentCode]
			if !ok {
				issues = append(issues, HierarchyIssue{
					Field:   prefix + ".parent_code",
					Code:    "factor.parent_code.not_found",
					Message: fmt.Sprintf("parent_code %s 不存在", factor.ParentCode),
				})
			} else if hasCycle(byCode, factor.Code) {
				issues = append(issues, HierarchyIssue{
					Field:   prefix + ".parent_code",
					Code:    "factor.parent_code.cycle",
					Message: fmt.Sprintf("factor %s 存在循环 parent 引用", factor.Code),
				})
			} else if parent.ParentCode == factor.Code {
				issues = append(issues, HierarchyIssue{
					Field:   prefix + ".parent_code",
					Code:    "factor.parent_code.cycle",
					Message: fmt.Sprintf("factor %s 与 parent %s 互相引用", factor.Code, factor.ParentCode),
				})
			}
		}

		if role == FactorRoleReportGroup {
			if factor.ScoringStrategy != "" || len(factor.QuestionCodes) > 0 {
				issues = append(issues, HierarchyIssue{
					Field:   prefix,
					Code:    "factor.report_group.scoring_forbidden",
					Message: "report_group 不能绑定 scoring 或 question_codes",
				})
			}
			continue
		}

		if len(factor.QuestionCodes) > 0 && !BindsQuestions(role) {
			issues = append(issues, HierarchyIssue{
				Field:   prefix + ".question_codes",
				Code:    "factor.question_codes.role_forbidden",
				Message: fmt.Sprintf("role %s 不允许绑定 question_codes", role),
			})
		}

		if RequiresChildrenPolicy(role) {
			if factor.ChildrenPolicy == nil {
				issues = append(issues, HierarchyIssue{
					Field:   prefix + ".children_policy",
					Code:    "factor.children_policy.required",
					Message: "composite index 必须定义 children_policy",
				})
			} else {
				issues = append(issues, validateChildrenPolicy(prefix, factor.ChildrenPolicy, byCode)...)
			}
		}

		if factor.ChildrenPolicy != nil && !RequiresChildrenPolicy(role) && role != FactorRoleTotal {
			if factor.ChildrenPolicy.Strategy != "" && factor.ChildrenPolicy.Strategy != ChildrenAggregationNone {
				issues = append(issues, HierarchyIssue{
					Field:   prefix + ".children_policy",
					Code:    "factor.children_policy.unexpected",
					Message: fmt.Sprintf("role %s 不应定义 children_policy", role),
				})
			}
		}
	}

	return issues
}

func validateChildrenPolicy(prefix string, policy *ChildrenPolicy, byCode map[string]FactorSnapshot) []HierarchyIssue {
	if policy == nil {
		return nil
	}
	issues := make([]HierarchyIssue, 0)
	if policy.Strategy == "" {
		issues = append(issues, HierarchyIssue{
			Field: prefix + ".children_policy.strategy", Code: "factor.children_policy.strategy.required",
			Message: "children_policy.strategy 不能为空",
		})
	} else if !policy.Strategy.IsValid() {
		issues = append(issues, HierarchyIssue{
			Field: prefix + ".children_policy.strategy", Code: "factor.children_policy.strategy.invalid",
			Message: fmt.Sprintf("children_policy.strategy %s 不支持", policy.Strategy),
		})
	}
	if len(policy.Children) == 0 && policy.Strategy != ChildrenAggregationCustom {
		issues = append(issues, HierarchyIssue{
			Field: prefix + ".children_policy.children", Code: "factor.children_policy.children.required",
			Message: "children_policy.children 不能为空",
		})
	}
	for _, childCode := range policy.Children {
		if _, ok := byCode[childCode]; !ok {
			issues = append(issues, HierarchyIssue{
				Field:   prefix + ".children_policy.children",
				Code:    "factor.children_policy.child.not_found",
				Message: fmt.Sprintf("children_policy 引用不存在的子因子 %s", childCode),
			})
		}
	}
	return issues
}

func hasCycle(byCode map[string]FactorSnapshot, start string) bool {
	visited := make(map[string]struct{})
	current := start
	for {
		factor, ok := byCode[current]
		if !ok || factor.ParentCode == "" {
			return false
		}
		if _, seen := visited[current]; seen {
			return true
		}
		visited[current] = struct{}{}
		if factor.ParentCode == start {
			return true
		}
		current = factor.ParentCode
	}
}
