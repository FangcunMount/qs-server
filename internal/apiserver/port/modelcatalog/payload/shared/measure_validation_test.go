package shared_test

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	sharedpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/shared"
)

func TestValidateFactorsAcceptsFlatModel(t *testing.T) {
	t.Parallel()

	issues := validateDimensions([]sharedpayload.DimensionRule{{
		Code: "total", Title: "总分", Role: string(factor.FactorRoleTotal),
		QuestionCodes: []string{"q1"}, ScoringStrategy: "sum",
	}})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func TestValidateFactorsRejectsUnknownParent(t *testing.T) {
	t.Parallel()

	issues := validateDimensions([]sharedpayload.DimensionRule{{
		Code: "inhibit", ParentCode: "missing", Role: string(factor.FactorRoleDimension),
	}})
	if len(issues) == 0 {
		t.Fatal("expected parent_code.not_found issue")
	}
}

func TestValidateFactorsRequiresChildrenPolicyForIndex(t *testing.T) {
	t.Parallel()

	issues := validateDimensions([]sharedpayload.DimensionRule{
		{Code: "bri", Role: string(factor.FactorRoleIndex)},
	})
	if len(issues) == 0 {
		t.Fatal("expected children_policy.required issue")
	}
}

func TestValidateFactorsRejectsReportGroupScoring(t *testing.T) {
	t.Parallel()

	issues := validateDimensions([]sharedpayload.DimensionRule{{
		Code: "section_a", Role: string(factor.FactorRoleReportGroup), ScoringStrategy: "sum",
	}})
	if len(issues) == 0 {
		t.Fatal("expected report_group.scoring_forbidden issue")
	}
}

func TestDeriveFactorLevels(t *testing.T) {
	t.Parallel()

	graph := sharedpayload.FactorGraphFromDefinitionDimensions([]sharedpayload.DimensionRule{
		{Code: "gec", Role: string(factor.FactorRoleIndex)},
		{Code: "bri", ParentCode: "gec", Role: string(factor.FactorRoleIndex)},
		{Code: "inhibit", ParentCode: "bri", Role: string(factor.FactorRoleDimension)},
	})
	levels := graph.Levels()
	if levels["gec"] != 1 || levels["bri"] != 2 || levels["inhibit"] != 3 {
		t.Fatalf("levels = %#v, want gec:1 bri:2 inhibit:3", levels)
	}
}

func TestFactorCorePathDerivesValidGraphAndScoreNodes(t *testing.T) {
	t.Parallel()

	dimensions := []sharedpayload.DimensionRule{
		{
			Code: "gec", Title: "全局执行指数", Role: string(factor.FactorRoleIndex),
			ChildrenPolicy: &sharedpayload.ChildrenPolicyPayload{
				Strategy: string(factor.ChildrenAggregationWeightedSum),
				Children: []string{"bri", "mi"},
				Weights:  map[string]float64{"bri": 0.4, "mi": 0.6},
			},
		},
		{
			Code: "bri", Title: "行为调节指数", Role: string(factor.FactorRoleIndex), ParentCode: "gec",
			ChildrenPolicy: &sharedpayload.ChildrenPolicyPayload{
				Strategy: string(factor.ChildrenAggregationSum),
				Children: []string{"inhibit"},
			},
		},
		{
			Code: "mi", Title: "元认知指数", Role: string(factor.FactorRoleIndex), ParentCode: "gec",
			ChildrenPolicy: &sharedpayload.ChildrenPolicyPayload{
				Strategy: string(factor.ChildrenAggregationAverage),
				Children: []string{"working_memory"},
			},
		},
		{
			Code: "inhibit", Title: "抑制", Role: string(factor.FactorRoleDimension), ParentCode: "bri",
			QuestionCodes: []string{"q1", "q2"}, ScoringStrategy: "sum",
		},
		{
			Code: "working_memory", Title: "工作记忆", Role: string(factor.FactorRoleDimension), ParentCode: "mi",
			QuestionCodes: []string{"q3"}, ScoringStrategy: "avg",
		},
		{
			Code: "section_a", Title: "报告分组", Role: string(factor.FactorRoleReportGroup),
		},
	}

	measure := sharedpayload.MeasureSpecFromDefinitionBody(sharedpayload.DefinitionBody{Dimensions: dimensions})
	factors := measure.Factors
	graph := measure.FactorGraph
	scoring := measure.Scoring
	levels := graph.Levels()
	if levels["gec"] != 1 || levels["bri"] != 2 || levels["inhibit"] != 3 {
		t.Fatalf("levels = %#v", levels)
	}
	if issues := factor.ValidateMeasureSpecParts(factors, graph, scoring); len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}

	nodes := factor.CalculationScoreNodesFromMeasureParts(factors, graph, scoring)
	want := []calculation.ScoreNode{
		{
			Code: "gec", Name: "全局执行指数", Role: "index", Kind: calculation.DimensionKindIndex, Level: 1,
			Aggregation: calculation.AggregationWeightedSum, Children: []string{"bri", "mi"},
			Weights: map[string]float64{"bri": 0.4, "mi": 0.6},
		},
		{
			Code: "bri", Name: "行为调节指数", Role: "index", Kind: calculation.DimensionKindIndex, ParentCode: "gec", Level: 2,
			Aggregation: calculation.AggregationSum, Children: []string{"inhibit"},
		},
		{
			Code: "mi", Name: "元认知指数", Role: "index", Kind: calculation.DimensionKindIndex, ParentCode: "gec", Level: 2,
			Aggregation: calculation.AggregationAverage, Children: []string{"working_memory"},
		},
		{
			Code: "inhibit", Name: "抑制", Role: "dimension", Kind: calculation.DimensionKindFactor, ParentCode: "bri", Level: 3,
		},
		{
			Code: "working_memory", Name: "工作记忆", Role: "dimension", Kind: calculation.DimensionKindFactor, ParentCode: "mi", Level: 3,
		},
		{
			Code: "section_a", Name: "报告分组", Role: "report_group", Kind: calculation.DimensionKindFactor, Level: 1,
		},
	}
	if !reflect.DeepEqual(nodes, want) {
		t.Fatalf("CalculationScoreNodesFromMeasureParts mismatch\n got: %#v\nwant: %#v", nodes, want)
	}
}

func validateDimensions(dimensions []sharedpayload.DimensionRule) []factor.HierarchyIssue {
	measure := sharedpayload.MeasureSpecFromDefinitionBody(sharedpayload.DefinitionBody{Dimensions: dimensions})
	return factor.ValidateMeasureSpecParts(measure.Factors, measure.FactorGraph, measure.Scoring)
}
