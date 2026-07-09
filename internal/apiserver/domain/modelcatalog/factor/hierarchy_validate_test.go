package factor_test

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestValidateFactorsAcceptsFlatModel(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateFactors([]factor.LegacyFactor{{
		Code: "total", Title: "总分", Role: factor.FactorRoleTotal,
		QuestionCodes: []string{"q1"}, ScoringStrategy: "sum",
	}})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func TestValidateFactorsRejectsUnknownParent(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateFactors([]factor.LegacyFactor{{
		Code: "inhibit", ParentCode: "missing", Role: factor.FactorRoleDimension,
	}})
	if len(issues) == 0 {
		t.Fatal("expected parent_code.not_found issue")
	}
}

func TestValidateFactorsRequiresChildrenPolicyForIndex(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateFactors([]factor.LegacyFactor{
		{Code: "bri", Role: factor.FactorRoleIndex},
	})
	if len(issues) == 0 {
		t.Fatal("expected children_policy.required issue")
	}
}

func TestValidateFactorsRejectsReportGroupScoring(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateFactors([]factor.LegacyFactor{{
		Code: "section_a", Role: factor.FactorRoleReportGroup, ScoringStrategy: "sum",
	}})
	if len(issues) == 0 {
		t.Fatal("expected report_group.scoring_forbidden issue")
	}
}

func TestDeriveFactorLevels(t *testing.T) {
	t.Parallel()

	factors := factor.DeriveFactorLevels([]factor.LegacyFactor{
		{Code: "gec", Role: factor.FactorRoleIndex},
		{Code: "bri", ParentCode: "gec", Role: factor.FactorRoleIndex},
		{Code: "inhibit", ParentCode: "bri", Role: factor.FactorRoleDimension},
	})
	if factors[0].Level != 1 || factors[1].Level != 2 || factors[2].Level != 3 {
		t.Fatalf("levels = %d,%d,%d, want 1,2,3", factors[0].Level, factors[1].Level, factors[2].Level)
	}
}

func TestFactorCorePathDerivesValidGraphAndScoreNodes(t *testing.T) {
	t.Parallel()

	factors := []factor.LegacyFactor{
		{
			Code: "gec", Title: "全局执行指数", Role: factor.FactorRoleIndex,
			ChildrenPolicy: &factor.ChildrenPolicy{
				Strategy: factor.ChildrenAggregationWeightedSum,
				Children: []string{"bri", "mi"},
				Weights:  map[string]float64{"bri": 0.4, "mi": 0.6},
			},
		},
		{
			Code: "bri", Title: "行为调节指数", Role: factor.FactorRoleIndex, ParentCode: "gec",
			ChildrenPolicy: &factor.ChildrenPolicy{
				Strategy: factor.ChildrenAggregationSum,
				Children: []string{"inhibit"},
			},
		},
		{
			Code: "mi", Title: "元认知指数", Role: factor.FactorRoleIndex, ParentCode: "gec",
			ChildrenPolicy: &factor.ChildrenPolicy{
				Strategy: factor.ChildrenAggregationAverage,
				Children: []string{"working_memory"},
			},
		},
		{
			Code: "inhibit", Title: "抑制", Role: factor.FactorRoleDimension, ParentCode: "bri",
			QuestionCodes: []string{"q1", "q2"}, ScoringStrategy: "sum",
		},
		{
			Code: "working_memory", Title: "工作记忆", Role: factor.FactorRoleDimension, ParentCode: "mi",
			QuestionCodes: []string{"q3"}, ScoringStrategy: "avg",
		},
		{
			Code: "section_a", Title: "报告分组", Role: factor.FactorRoleReportGroup,
		},
	}

	derived := factor.DeriveFactorLevels(factors)
	byCode := factor.IndexByLegacyFactorCode(derived)
	if byCode["gec"].Level != 1 || byCode["bri"].Level != 2 || byCode["inhibit"].Level != 3 {
		t.Fatalf("levels = gec:%d bri:%d inhibit:%d", byCode["gec"].Level, byCode["bri"].Level, byCode["inhibit"].Level)
	}
	if issues := factor.ValidateFactors(derived); len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}

	nodes := factor.CalculationScoreNodesFromLegacyFactors(factors)
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
		t.Fatalf("CalculationScoreNodesFromLegacyFactors mismatch\n got: %#v\nwant: %#v", nodes, want)
	}
}
