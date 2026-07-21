package factor

import (
	"fmt"
	"math"
)

// ValidateMeasureSpecParts checks measure-layer invariants after Factor is split from graph and scoring.
// AssessmentModel publish/save baseline (MC-R007): Factors are required; empty measure is not a valid model.
func ValidateMeasureSpecParts(factors []Factor, graph FactorGraph, scoring []Scoring) []HierarchyIssue {
	if len(factors) == 0 {
		return []HierarchyIssue{{
			Field:   "factors",
			Code:    "measure.factors.required",
			Message: "assessment model requires at least one factor",
		}}
	}
	byCode := IndexByFactorCode(factors)
	issues := make([]HierarchyIssue, 0)
	seenCodes := make(map[string]struct{}, len(factors))
	for _, item := range factors {
		if item.Code == "" {
			issues = append(issues, HierarchyIssue{
				Field: "code", Code: "factor.code.required", Message: "factor code 不能为空",
			})
			continue
		}
		if _, exists := seenCodes[item.Code]; exists {
			issues = append(issues, HierarchyIssue{
				Field:   fmt.Sprintf("factors[%s]", item.Code),
				Code:    "factor.code.duplicate",
				Message: fmt.Sprintf("factor code %s 重复", item.Code),
			})
		}
		seenCodes[item.Code] = struct{}{}
	}
	issues = append(issues, validateFactorGraph(graph, byCode, scoring)...)
	scoringByFactor := make(map[string]Scoring, len(scoring))
	seenScoringFactor := make(map[string]struct{}, len(scoring))
	for _, rule := range scoring {
		if rule.FactorCode != "" {
			if _, dup := seenScoringFactor[rule.FactorCode]; dup {
				issues = append(issues, HierarchyIssue{
					Field:   fmt.Sprintf("scoring[%s]", rule.FactorCode),
					Code:    "scoring.factor.duplicate",
					Message: fmt.Sprintf("factor %s 只能有一条 scoring", rule.FactorCode),
				})
			}
			seenScoringFactor[rule.FactorCode] = struct{}{}
		}
		scoringByFactor[rule.FactorCode] = rule
		issues = append(issues, validateScoring(rule, byCode)...)
	}
	for _, item := range factors {
		prefix := fmt.Sprintf("factors[%s]", item.Code)
		role := item.ResolvedRole()
		rule, hasScoring := scoringByFactor[item.Code]
		hasQuestionSources := scoringHasSourceKind(rule, ScoringSourceQuestion)
		hasFactorSources := scoringHasSourceKind(rule, ScoringSourceFactor)
		if role == FactorRoleReportGroup {
			if hasScoring {
				issues = append(issues, HierarchyIssue{
					Field:   prefix,
					Code:    "factor.report_group.scoring_forbidden",
					Message: "report_group 不能绑定 scoring 或 question_codes",
				})
			}
			continue
		}
		if hasQuestionSources && !BindsQuestions(role) {
			issues = append(issues, HierarchyIssue{
				Field:   prefix + ".question_codes",
				Code:    "factor.question_codes.role_forbidden",
				Message: fmt.Sprintf("role %s 不允许绑定 question_codes", role),
			})
		}
		if RequiresChildrenPolicy(role) && !hasFactorSources && !hasQuestionSources {
			issues = append(issues, HierarchyIssue{
				Field:   prefix + ".children_policy",
				Code:    "factor.scoring.required",
				Message: "index 必须定义题目或子因子计分来源",
			})
		}
		if hasFactorSources && !RequiresChildrenPolicy(role) && role != FactorRoleTotal {
			issues = append(issues, HierarchyIssue{
				Field:   prefix + ".children_policy",
				Code:    "factor.children_policy.unexpected",
				Message: fmt.Sprintf("role %s 不应定义 children_policy", role),
			})
		}
	}
	return issues
}

func validateFactorGraph(graph FactorGraph, byCode map[string]Factor, scoring []Scoring) []HierarchyIssue {
	issues := make([]HierarchyIssue, 0)
	issues = append(issues, validateFactorGraphSortOrders(graph, byCode)...)
	seenEdge := make(map[string]struct{}, len(graph.Edges))
	parentByChild := make(map[string]string, len(graph.Edges))
	nodesInEdges := make(map[string]struct{}, len(graph.Edges)*2)

	for _, edge := range graph.Edges {
		prefix := fmt.Sprintf("factors[%s]", edge.ChildCode)
		if edge.ParentCode == "" || edge.ChildCode == "" {
			issues = append(issues, HierarchyIssue{
				Field: "factor_graph.edges", Code: "factor_graph.edge.invalid",
				Message: "factor graph edge parent_code 和 child_code 不能为空",
			})
			continue
		}
		if edge.ParentCode == edge.ChildCode {
			issues = append(issues, HierarchyIssue{
				Field: prefix + ".parent_code", Code: "factor.parent_code.self",
				Message: fmt.Sprintf("factor %s 不能以自身为 parent", edge.ChildCode),
			})
		}
		edgeKey := edge.ParentCode + "\x00" + edge.ChildCode
		if _, dup := seenEdge[edgeKey]; dup {
			issues = append(issues, HierarchyIssue{
				Field: "factor_graph.edges", Code: "factor_graph.edge.duplicate",
				Message: fmt.Sprintf("factor graph edge %s -> %s 重复", edge.ParentCode, edge.ChildCode),
			})
		}
		seenEdge[edgeKey] = struct{}{}
		if prev, exists := parentByChild[edge.ChildCode]; exists && prev != edge.ParentCode {
			issues = append(issues, HierarchyIssue{
				Field: prefix + ".parent_code", Code: "factor.parent_code.multiple",
				Message: fmt.Sprintf("factor %s 只能有一个 parent（已有 %s，又出现 %s）", edge.ChildCode, prev, edge.ParentCode),
			})
		}
		parentByChild[edge.ChildCode] = edge.ParentCode
		nodesInEdges[edge.ParentCode] = struct{}{}
		nodesInEdges[edge.ChildCode] = struct{}{}

		if _, ok := byCode[edge.ParentCode]; !ok {
			issues = append(issues, HierarchyIssue{
				Field:   prefix + ".parent_code",
				Code:    "factor.parent_code.not_found",
				Message: fmt.Sprintf("parent_code %s 不存在", edge.ParentCode),
			})
		}
		if _, ok := byCode[edge.ChildCode]; !ok {
			issues = append(issues, HierarchyIssue{
				Field:   prefix + ".children_policy.children",
				Code:    "factor.children_policy.child.not_found",
				Message: fmt.Sprintf("children_policy 引用不存在的子因子 %s", edge.ChildCode),
			})
		}
	}
	if graphHasCycle(graph) {
		issues = append(issues, HierarchyIssue{
			Field: "factor_graph.edges", Code: "factor.parent_code.cycle", Message: "factor graph 存在循环 parent 引用",
		})
	}

	issues = append(issues, validateFactorGraphRoots(graph, byCode, parentByChild)...)
	if len(graph.Edges) > 0 {
		issues = append(issues, validateFactorGraphReachability(graph, nodesInEdges)...)
	}
	issues = append(issues, validateFactorGraphScoringConsistency(graph, scoring)...)
	return issues
}

func validateFactorGraphSortOrders(graph FactorGraph, byCode map[string]Factor) []HierarchyIssue {
	issues := make([]HierarchyIssue, 0)
	byOrder := make(map[int]string, len(graph.SortOrders))
	for code, order := range graph.SortOrders {
		if _, ok := byCode[code]; !ok {
			issues = append(issues, HierarchyIssue{
				Field: "factor_graph.sort_orders", Code: "factor_graph.sort_order.not_found",
				Message: fmt.Sprintf("factor graph sort order 引用了不存在的因子 %s", code),
			})
		}
		if order <= 0 {
			issues = append(issues, HierarchyIssue{
				Field: "factor_graph.sort_orders", Code: "factor_graph.sort_order.invalid",
				Message: fmt.Sprintf("factor %s 的 sort order 必须大于 0", code),
			})
			continue
		}
		if previous, duplicate := byOrder[order]; duplicate {
			issues = append(issues, HierarchyIssue{
				Field: "factor_graph.sort_orders", Code: "factor_graph.sort_order.duplicate",
				Message: fmt.Sprintf("factor %s 与 %s 使用了重复的 sort order %d", previous, code, order),
			})
			continue
		}
		byOrder[order] = code
	}
	return issues
}

func validateFactorGraphRoots(graph FactorGraph, byCode map[string]Factor, parentByChild map[string]string) []HierarchyIssue {
	if len(graph.Roots) == 0 {
		if len(graph.Edges) == 0 {
			return nil
		}
		return []HierarchyIssue{{
			Field: "factor_graph.roots", Code: "factor_graph.roots.required",
			Message: "factor graph 存在 edges 时必须声明 roots",
		}}
	}
	issues := make([]HierarchyIssue, 0)
	seenRoot := make(map[string]struct{}, len(graph.Roots))
	for _, root := range graph.Roots {
		if root == "" {
			issues = append(issues, HierarchyIssue{
				Field: "factor_graph.roots", Code: "factor_graph.root.invalid",
				Message: "factor graph root 不能为空",
			})
			continue
		}
		if _, dup := seenRoot[root]; dup {
			issues = append(issues, HierarchyIssue{
				Field: "factor_graph.roots", Code: "factor_graph.root.duplicate",
				Message: fmt.Sprintf("factor graph root %s 重复", root),
			})
			continue
		}
		seenRoot[root] = struct{}{}
		if _, ok := byCode[root]; !ok {
			issues = append(issues, HierarchyIssue{
				Field: "factor_graph.roots", Code: "factor_graph.root.not_found",
				Message: fmt.Sprintf("factor graph root %s 不存在", root),
			})
		}
		if parent, isChild := parentByChild[root]; isChild {
			issues = append(issues, HierarchyIssue{
				Field: "factor_graph.roots", Code: "factor_graph.root.is_child",
				Message: fmt.Sprintf("factor graph root %s 不能同时是 %s 的 child", root, parent),
			})
		}
	}
	return issues
}

func validateFactorGraphReachability(graph FactorGraph, nodesInEdges map[string]struct{}) []HierarchyIssue {
	reachable := make(map[string]struct{}, len(nodesInEdges))
	childrenByParent := make(map[string][]string, len(graph.Edges))
	for _, edge := range graph.Edges {
		childrenByParent[edge.ParentCode] = append(childrenByParent[edge.ParentCode], edge.ChildCode)
	}
	var walk func(code string)
	walk = func(code string) {
		if _, ok := reachable[code]; ok {
			return
		}
		reachable[code] = struct{}{}
		for _, child := range childrenByParent[code] {
			walk(child)
		}
	}
	for _, root := range graph.Roots {
		walk(root)
	}
	issues := make([]HierarchyIssue, 0)
	for code := range nodesInEdges {
		if _, ok := reachable[code]; ok {
			continue
		}
		issues = append(issues, HierarchyIssue{
			Field: fmt.Sprintf("factors[%s]", code), Code: "factor_graph.node.unreachable",
			Message: fmt.Sprintf("factor %s 无法从 factor graph roots 到达", code),
		})
	}
	return issues
}

func validateFactorGraphScoringConsistency(graph FactorGraph, scoring []Scoring) []HierarchyIssue {
	if len(scoring) == 0 {
		return nil
	}
	issues := make([]HierarchyIssue, 0)
	parentsWithEdges := make(map[string]map[string]struct{})
	for _, edge := range graph.Edges {
		children, ok := parentsWithEdges[edge.ParentCode]
		if !ok {
			children = make(map[string]struct{})
			parentsWithEdges[edge.ParentCode] = children
		}
		children[edge.ChildCode] = struct{}{}
	}
	seenParents := make(map[string]struct{})
	for _, rule := range scoring {
		if !scoringHasSourceKind(rule, ScoringSourceFactor) {
			continue
		}
		seenParents[rule.FactorCode] = struct{}{}
		want := make(map[string]struct{})
		for _, source := range rule.Sources {
			if source.Kind == ScoringSourceFactor && source.Code != "" {
				want[source.Code] = struct{}{}
			}
		}
		got := parentsWithEdges[rule.FactorCode]
		if sameStringSet(want, got) {
			continue
		}
		issues = append(issues, HierarchyIssue{
			Field: fmt.Sprintf("scoring[%s].sources", rule.FactorCode),
			Code:  "factor_graph.children.mismatch",
			Message: fmt.Sprintf(
				"factor %s 的 factor-source children 与 factor graph edges 不一致",
				rule.FactorCode,
			),
		})
	}
	for parent := range parentsWithEdges {
		if _, ok := seenParents[parent]; ok {
			continue
		}
		issues = append(issues, HierarchyIssue{
			Field: fmt.Sprintf("factors[%s].children_policy", parent),
			Code:  "factor_graph.children.mismatch",
			Message: fmt.Sprintf(
				"factor graph 声明了 %s 的 children，但 scoring 未配置对应 factor sources",
				parent,
			),
		})
	}
	return issues
}

func sameStringSet(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if _, ok := right[key]; !ok {
			return false
		}
	}
	return true
}

func validateScoring(rule Scoring, byCode map[string]Factor) []HierarchyIssue {
	issues := make([]HierarchyIssue, 0)
	if _, ok := byCode[rule.FactorCode]; !ok {
		issues = append(issues, HierarchyIssue{
			Field:   fmt.Sprintf("scoring[%s].factor_code", rule.FactorCode),
			Code:    "factor.scoring.factor_code.not_found",
			Message: fmt.Sprintf("scoring 引用了不存在的因子 %s", rule.FactorCode),
		})
	}
	var seenKind ScoringSourceKind
	seenQuestions := make(map[string]struct{})
	for _, source := range rule.Sources {
		if source.Kind == "" || source.Code == "" {
			issues = append(issues, HierarchyIssue{
				Field:   fmt.Sprintf("scoring[%s].sources", rule.FactorCode),
				Code:    "factor.scoring.source.invalid",
				Message: "scoring source kind 和 code 不能为空",
			})
			continue
		}
		if seenKind != "" && seenKind != source.Kind {
			issues = append(issues, HierarchyIssue{
				Field:   fmt.Sprintf("scoring[%s].sources", rule.FactorCode),
				Code:    "factor.scoring.source.mixed",
				Message: "scoring source 不能同时混用 question 和 factor",
			})
		}
		seenKind = source.Kind
		switch source.Kind {
		case ScoringSourceFactor:
			if _, ok := byCode[source.Code]; !ok {
				issues = append(issues, HierarchyIssue{
					Field:   fmt.Sprintf("scoring[%s].sources", rule.FactorCode),
					Code:    "factor.children_policy.child.not_found",
					Message: fmt.Sprintf("children_policy 引用不存在的子因子 %s", source.Code),
				})
			}
			if source.ScoringMode != "" || source.Sign != 0 || source.Weight != 0 || source.OptionScores != nil {
				issues = append(issues, HierarchyIssue{
					Field:   fmt.Sprintf("scoring[%s].sources", rule.FactorCode),
					Code:    "factor.scoring.option_scores.role_forbidden",
					Message: "factor scoring source cannot define question contribution fields",
				})
			}
		case ScoringSourceQuestion:
			if _, duplicate := seenQuestions[source.Code]; duplicate {
				issues = append(issues, HierarchyIssue{Field: fmt.Sprintf("scoring[%s].sources", rule.FactorCode), Code: "question_contribution.duplicate", Message: fmt.Sprintf("question %s 对 factor %s 的贡献重复", source.Code, rule.FactorCode)})
			}
			seenQuestions[source.Code] = struct{}{}
			issues = append(issues, validateQuestionContribution(rule.FactorCode, source)...)
		}
	}
	return issues
}

func validateQuestionContribution(factorCode string, source ScoringSource) []HierarchyIssue {
	field := fmt.Sprintf("scoring[%s].sources", factorCode)
	if source.ScoringMode == "" {
		if source.OptionScores != nil && len(source.OptionScores) == 0 {
			return []HierarchyIssue{{Field: field, Code: "factor.scoring.option_scores.empty", Message: "question option_scores cannot be an empty map"}}
		}
		return nil
	}
	issues := make([]HierarchyIssue, 0)
	if source.ScoringMode != QuestionScoringModeQuestionScore && source.ScoringMode != QuestionScoringModeOptionOverride {
		issues = append(issues, HierarchyIssue{Field: field + ".scoring_mode", Code: "scoring_mode.invalid", Message: fmt.Sprintf("scoring_mode %s 不支持", source.ScoringMode)})
	}
	if source.Sign != 1 && source.Sign != -1 {
		issues = append(issues, HierarchyIssue{Field: field + ".sign", Code: "sign.invalid", Message: "sign 必须是 1 或 -1"})
	}
	if math.IsNaN(source.Weight) || math.IsInf(source.Weight, 0) || source.Weight <= 0 {
		issues = append(issues, HierarchyIssue{Field: field + ".weight", Code: "weight.invalid", Message: "weight 必须是大于 0 的有限数字"})
	}
	switch source.ScoringMode {
	case QuestionScoringModeQuestionScore:
		if source.OptionScores != nil {
			issues = append(issues, HierarchyIssue{Field: field + ".option_scores", Code: "option_scores.forbidden", Message: "question_score 不能配置 option_scores"})
		}
	case QuestionScoringModeOptionOverride:
		if len(source.OptionScores) == 0 {
			issues = append(issues, HierarchyIssue{Field: field + ".option_scores", Code: "option_scores.required", Message: "option_override 必须配置 option_scores"})
		}
	}
	for _, score := range source.OptionScores {
		if math.IsNaN(score) || math.IsInf(score, 0) {
			issues = append(issues, HierarchyIssue{Field: field + ".option_scores", Code: "option_scores.invalid", Message: "option_scores 必须是有限数字"})
			break
		}
	}
	return issues
}

func scoringHasSourceKind(rule Scoring, kind ScoringSourceKind) bool {
	for _, source := range rule.Sources {
		if source.Kind == kind {
			return true
		}
	}
	return false
}

func graphHasCycle(graph FactorGraph) bool {
	childrenByParent := make(map[string][]string)
	for _, edge := range graph.Edges {
		childrenByParent[edge.ParentCode] = append(childrenByParent[edge.ParentCode], edge.ChildCode)
	}
	visiting := make(map[string]bool)
	visited := make(map[string]bool)
	var visit func(code string) bool
	visit = func(code string) bool {
		if visiting[code] {
			return true
		}
		if visited[code] {
			return false
		}
		visiting[code] = true
		for _, child := range childrenByParent[code] {
			if visit(child) {
				return true
			}
		}
		visiting[code] = false
		visited[code] = true
		return false
	}
	for parent := range childrenByParent {
		if visit(parent) {
			return true
		}
	}
	return false
}
