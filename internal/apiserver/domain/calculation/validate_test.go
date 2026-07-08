package calculation_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
)

func TestValidateScoreNodesAcceptsValidTree(t *testing.T) {
	t.Parallel()

	nodes := []calculation.ScoreNode{
		{Code: "gec", Children: []string{"bri", "eri"}, Aggregation: calculation.AggregationSum},
		{Code: "bri", Children: []string{"inhibit"}, Aggregation: calculation.AggregationSum},
		{Code: "eri"},
		{Code: "inhibit"},
	}
	issues := calculation.ValidateScoreNodes(nodes)
	for _, issue := range issues {
		if issue.Code == calculation.IssueScoreNodeCycle ||
			issue.Code == calculation.IssueScoreNodeDanglingChild ||
			issue.Code == calculation.IssueScoreNodeDuplicateCode ||
			issue.Code == calculation.IssueScoreNodeEmptyCode {
			t.Fatalf("unexpected issue: %#v", issue)
		}
	}
}

func TestValidateScoreNodesRejectsDuplicateCode(t *testing.T) {
	t.Parallel()

	issues := calculation.ValidateScoreNodes([]calculation.ScoreNode{
		{Code: "a"},
		{Code: "a"},
	})
	if len(issues) != 1 || issues[0].Code != calculation.IssueScoreNodeDuplicateCode {
		t.Fatalf("issues = %#v, want duplicate code", issues)
	}
}

func TestValidateScoreNodesRejectsDanglingChild(t *testing.T) {
	t.Parallel()

	issues := calculation.ValidateScoreNodes([]calculation.ScoreNode{
		{Code: "parent", Children: []string{"missing"}, Aggregation: calculation.AggregationSum},
	})
	if len(issues) != 1 || issues[0].Code != calculation.IssueScoreNodeDanglingChild {
		t.Fatalf("issues = %#v, want dangling child", issues)
	}
}

func TestValidateScoreNodesRejectsCycle(t *testing.T) {
	t.Parallel()

	issues := calculation.ValidateScoreNodes([]calculation.ScoreNode{
		{Code: "a", Children: []string{"b"}, Aggregation: calculation.AggregationSum},
		{Code: "b", Children: []string{"a"}, Aggregation: calculation.AggregationSum},
	})
	if len(issues) == 0 || issues[0].Code != calculation.IssueScoreNodeCycle {
		t.Fatalf("issues = %#v, want cycle", issues)
	}
}

func TestValidateScoreNodesRejectsMissingWeight(t *testing.T) {
	t.Parallel()

	issues := calculation.ValidateScoreNodes([]calculation.ScoreNode{
		{
			Code:        "index",
			Aggregation: calculation.AggregationWeightedSum,
			Children:    []string{"a", "b"},
			Weights:     map[string]float64{"a": 2},
		},
		{Code: "a"},
		{Code: "b"},
	})
	if len(issues) != 1 || issues[0].Code != calculation.IssueScoreNodeMissingWeight {
		t.Fatalf("issues = %#v, want missing weight error", issues)
	}
}

func TestValidateScoreNodesRejectsInvalidCompositeAggregation(t *testing.T) {
	t.Parallel()

	issues := calculation.ValidateScoreNodes([]calculation.ScoreNode{
		{Code: "parent", Children: []string{"child"}},
		{Code: "child"},
	})
	if len(issues) != 1 || issues[0].Code != calculation.IssueScoreNodeInvalidAggregation {
		t.Fatalf("issues = %#v, want invalid aggregation", issues)
	}
}

func TestValidateResultRejectsDuplicateDimensionCode(t *testing.T) {
	t.Parallel()

	issues := calculation.ValidateResult(&calculation.Result{
		Dimensions: []calculation.DimensionResult{
			{Code: "a"},
			{Code: "a"},
		},
	})
	if len(issues) != 1 || issues[0].Code != calculation.IssueResultDimensionDuplicate {
		t.Fatalf("issues = %#v, want duplicate dimension", issues)
	}
}
