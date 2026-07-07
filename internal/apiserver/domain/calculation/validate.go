package calculation

import "strconv"

// Issue codes for ScoreNode validation.
const (
	IssueScoreNodeEmptyCode       = "score_node_empty_code"
	IssueScoreNodeDuplicateCode   = "score_node_duplicate_code"
	IssueScoreNodeDanglingChild   = "score_node_dangling_child"
	IssueScoreNodeCycle           = "score_node_cycle"
	IssueScoreNodeMissingWeight   = "score_node_missing_weight"
	IssueResultDimensionEmpty     = "result_dimension_empty_code"
	IssueResultDimensionDuplicate = "result_dimension_duplicate_code"
)

// ValidateScoreNodes checks calculation ScoreNode inputs for structural integrity.
// Missing weights on weighted_sum nodes emit IssueScoreNodeMissingWeight warnings only.
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
	}
	issues = append(issues, detectScoreNodeCycles(nodes)...)
	return issues
}

// ValidateResult checks a calculation result for duplicate or empty dimension codes.
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
		"weighted_sum node "+parentCode+" child "+childCode+" has no explicit weight; defaulting to 1",
	)
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
