package calculation

import "strconv"

// Issue 编码 用于 ScoreNode 校验。
const (
	IssueScoreNodeEmptyCode          = "score_node_empty_code"
	IssueScoreNodeDuplicateCode      = "score_node_duplicate_code"
	IssueScoreNodeDanglingChild      = "score_node_dangling_child"
	IssueScoreNodeCycle              = "score_node_cycle"
	IssueScoreNodeMissingWeight      = "score_node_missing_weight"
	IssueScoreNodeInvalidAggregation = "score_node_invalid_aggregation"
	IssueResultDimensionEmpty        = "result_dimension_empty_code"
	IssueResultDimensionDuplicate    = "result_dimension_duplicate_code"
)

// ValidateScoreNodes checks calculation ScoreNode input for structural integrity.
// Missing weights on weighted_sum nodes are validation errors.
func ValidateScoreNodes(nodes []ScoreNode) []Issue {
	if len(nodes) == 0 {
		return nil
	}
	var issues []Issue
	byCode := make(map[string]int, len(nodes))
	for i, node := range nodes {
		if node.Code == "" {
			issues = append(issues, NewIssue(IssueScoreNodeEmptyCode, "score node code is required"))
			continue
		}
		if prev, ok := byCode[node.Code]; ok {
			issues = append(issues, NewIssue(
				IssueScoreNodeDuplicateCode,
				"duplicate score node code "+node.Code+" at index "+strconv.Itoa(prev)+" and "+strconv.Itoa(i),
			))
			continue
		}
		byCode[node.Code] = i
	}
	for _, node := range nodes {
		if node.Code == "" {
			continue
		}
		for _, child := range node.Children {
			if child == "" {
				continue
			}
			if _, ok := byCode[child]; !ok {
				issues = append(issues, NewIssue(
					IssueScoreNodeDanglingChild,
					"score node "+node.Code+" references missing child "+child,
				))
			}
		}
		if node.Aggregation == AggregationWeightedSum {
			for _, child := range node.Children {
				if child == "" {
					continue
				}
				if node.Weights == nil {
					issues = append(issues, missingWeightIssue(node.Code, child))
					continue
				}
				if _, ok := node.Weights[child]; !ok {
					issues = append(issues, missingWeightIssue(node.Code, child))
				}
			}
		}
		if len(node.Children) > 0 {
			issues = append(issues, validateCompositeAggregation(node)...)
		}
	}
	issues = append(issues, detectScoreNodeCycles(nodes)...)
	return issues
}

// ValidateResult 检查计算结果 用于 duplicate 或 空 维度 编码。
func ValidateResult(result *Result) []Issue {
	if result == nil {
		return nil
	}
	var issues []Issue
	seen := make(map[string]int, len(result.Dimensions))
	for i, dim := range result.Dimensions {
		if dim.Code == "" {
			issues = append(issues, NewIssue(IssueResultDimensionEmpty, "dimension code is required"))
			continue
		}
		if prev, ok := seen[dim.Code]; ok {
			issues = append(issues, NewIssue(
				IssueResultDimensionDuplicate,
				"duplicate dimension code "+dim.Code+" at index "+strconv.Itoa(prev)+" and "+strconv.Itoa(i),
			))
			continue
		}
		seen[dim.Code] = i
	}
	return issues
}

func missingWeightIssue(parentCode, childCode string) Issue {
	return NewIssue(
		IssueScoreNodeMissingWeight,
		"weighted_sum node "+parentCode+" child "+childCode+" requires an explicit weight",
	)
}

func validateCompositeAggregation(node ScoreNode) []Issue {
	switch node.Aggregation {
	case AggregationSum, AggregationAverage, AggregationWeightedSum:
		return nil
	case AggregationNone, AggregationLookup, AggregationCustom:
		return []Issue{NewIssue(
			IssueScoreNodeInvalidAggregation,
			"composite score node "+node.Code+" uses non-aggregating strategy "+string(node.Aggregation),
		)}
	case "":
		return []Issue{NewIssue(
			IssueScoreNodeInvalidAggregation,
			"composite score node "+node.Code+" requires an explicit aggregation strategy",
		)}
	default:
		return []Issue{NewIssue(
			IssueScoreNodeInvalidAggregation,
			"composite score node "+node.Code+" has unsupported aggregation "+string(node.Aggregation),
		)}
	}
}

func detectScoreNodeCycles(nodes []ScoreNode) []Issue {
	adjacency := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		if node.Code == "" {
			continue
		}
		adjacency[node.Code] = append([]string(nil), node.Children...)
	}
	visited := make(map[string]bool, len(nodes))
	inStack := make(map[string]bool, len(nodes))
	var issues []Issue
	var visit func(code string)
	visit = func(code string) {
		if inStack[code] {
			issues = append(issues, NewIssue(IssueScoreNodeCycle, "cycle detected at score node "+code))
			return
		}
		if visited[code] {
			return
		}
		visited[code] = true
		inStack[code] = true
		for _, child := range adjacency[code] {
			visit(child)
		}
		delete(inStack, code)
	}
	for code := range adjacency {
		visit(code)
	}
	return issues
}
