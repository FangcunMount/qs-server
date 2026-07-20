package factor_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestValidateMeasureSpecPartsRequiresFactors(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateMeasureSpecParts(nil, factor.FactorGraph{}, nil)
	if len(issues) != 1 || issues[0].Code != "measure.factors.required" {
		t.Fatalf("issues = %#v, want measure.factors.required", issues)
	}
}

func TestValidateMeasureSpecPartsAcceptsSingleFactor(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateMeasureSpecParts([]factor.Factor{{
		Code: "total", Role: factor.FactorRoleTotal,
	}}, factor.FactorGraph{}, nil)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func TestValidateMeasureSpecPartsRejectsDuplicateScoring(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateMeasureSpecParts(
		[]factor.Factor{{Code: "total", Role: factor.FactorRoleTotal}},
		factor.FactorGraph{},
		[]factor.Scoring{
			{FactorCode: "total", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "q1"}}},
			{FactorCode: "total", Strategy: factor.ScoringStrategyAvg, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "q2"}}},
		},
	)
	if !hasHierarchyIssueCode(issues, "scoring.factor.duplicate") {
		t.Fatalf("issues = %#v, want scoring.factor.duplicate", issues)
	}
}

func TestValidateMeasureSpecPartsAcceptsFlatRootsWithoutEdges(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateMeasureSpecParts(
		[]factor.Factor{{Code: "a"}, {Code: "b"}},
		factor.FactorGraph{Roots: []string{"a", "b"}},
		nil,
	)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none for flat roots", issues)
	}
}

func TestValidateMeasureSpecPartsRejectsEdgesWithoutRoots(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateMeasureSpecParts(
		[]factor.Factor{{Code: "parent", Role: factor.FactorRoleIndex}, {Code: "child"}},
		factor.FactorGraph{Edges: []factor.FactorEdge{{ParentCode: "parent", ChildCode: "child"}}},
		[]factor.Scoring{{
			FactorCode: "parent", Strategy: factor.ScoringStrategySum,
			Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceFactor, Code: "child"}},
		}},
	)
	if !hasHierarchyIssueCode(issues, "factor_graph.roots.required") {
		t.Fatalf("issues = %#v, want factor_graph.roots.required", issues)
	}
}

func TestValidateMeasureSpecPartsRejectsMultipleParents(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateMeasureSpecParts(
		[]factor.Factor{{Code: "p1", Role: factor.FactorRoleIndex}, {Code: "p2", Role: factor.FactorRoleIndex}, {Code: "child"}},
		factor.FactorGraph{
			Roots: []string{"p1", "p2"},
			Edges: []factor.FactorEdge{
				{ParentCode: "p1", ChildCode: "child"},
				{ParentCode: "p2", ChildCode: "child"},
			},
		},
		[]factor.Scoring{
			{FactorCode: "p1", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceFactor, Code: "child"}}},
			{FactorCode: "p2", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceFactor, Code: "child"}}},
		},
	)
	if !hasHierarchyIssueCode(issues, "factor.parent_code.multiple") {
		t.Fatalf("issues = %#v, want factor.parent_code.multiple", issues)
	}
}

func TestValidateMeasureSpecPartsRejectsDuplicateEdges(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateMeasureSpecParts(
		[]factor.Factor{{Code: "parent", Role: factor.FactorRoleIndex}, {Code: "child"}},
		factor.FactorGraph{
			Roots: []string{"parent"},
			Edges: []factor.FactorEdge{
				{ParentCode: "parent", ChildCode: "child"},
				{ParentCode: "parent", ChildCode: "child"},
			},
		},
		[]factor.Scoring{{
			FactorCode: "parent", Strategy: factor.ScoringStrategySum,
			Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceFactor, Code: "child"}},
		}},
	)
	if !hasHierarchyIssueCode(issues, "factor_graph.edge.duplicate") {
		t.Fatalf("issues = %#v, want factor_graph.edge.duplicate", issues)
	}
}

func TestValidateMeasureSpecPartsRejectsUnreachableNode(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateMeasureSpecParts(
		[]factor.Factor{{Code: "root", Role: factor.FactorRoleIndex}, {Code: "a"}, {Code: "orphan", Role: factor.FactorRoleIndex}, {Code: "b"}},
		factor.FactorGraph{
			Roots: []string{"root"},
			Edges: []factor.FactorEdge{
				{ParentCode: "root", ChildCode: "a"},
				{ParentCode: "orphan", ChildCode: "b"},
			},
		},
		[]factor.Scoring{
			{FactorCode: "root", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceFactor, Code: "a"}}},
			{FactorCode: "orphan", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceFactor, Code: "b"}}},
		},
	)
	if !hasHierarchyIssueCode(issues, "factor_graph.node.unreachable") {
		t.Fatalf("issues = %#v, want factor_graph.node.unreachable", issues)
	}
}

func TestValidateMeasureSpecPartsRejectsGraphScoringChildrenMismatch(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateMeasureSpecParts(
		[]factor.Factor{{Code: "parent", Role: factor.FactorRoleIndex}, {Code: "child"}, {Code: "other"}},
		factor.FactorGraph{
			Roots: []string{"parent"},
			Edges: []factor.FactorEdge{{ParentCode: "parent", ChildCode: "child"}},
		},
		[]factor.Scoring{{
			FactorCode: "parent", Strategy: factor.ScoringStrategySum,
			Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceFactor, Code: "other"}},
		}},
	)
	if !hasHierarchyIssueCode(issues, "factor_graph.children.mismatch") {
		t.Fatalf("issues = %#v, want factor_graph.children.mismatch", issues)
	}
}

func TestValidateMeasureSpecPartsAcceptsConsistentHierarchy(t *testing.T) {
	t.Parallel()
	issues := factor.ValidateMeasureSpecParts(
		[]factor.Factor{{Code: "parent", Role: factor.FactorRoleIndex}, {Code: "child"}},
		factor.FactorGraph{
			Roots: []string{"parent"},
			Edges: []factor.FactorEdge{{ParentCode: "parent", ChildCode: "child"}},
		},
		[]factor.Scoring{{
			FactorCode: "parent", Strategy: factor.ScoringStrategySum,
			Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceFactor, Code: "child"}},
		}},
	)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func hasHierarchyIssueCode(issues []factor.HierarchyIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
