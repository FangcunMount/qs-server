package factor_test

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestValidateFactorHierarchyAcceptsFlatModel(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateFactorHierarchy([]factor.FactorSnapshot{{
		Code: "total", Title: "总分", Role: factor.FactorRoleTotal,
		QuestionCodes: []string{"q1"}, ScoringStrategy: "sum",
	}})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}

func TestValidateFactorHierarchyRejectsUnknownParent(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateFactorHierarchy([]factor.FactorSnapshot{{
		Code: "inhibit", ParentCode: "missing", Role: factor.FactorRoleDimension,
	}})
	if len(issues) == 0 {
		t.Fatal("expected parent_code.not_found issue")
	}
}

func TestValidateFactorHierarchyRequiresChildrenPolicyForIndex(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateFactorHierarchy([]factor.FactorSnapshot{
		{Code: "bri", Role: factor.FactorRoleIndex},
	})
	if len(issues) == 0 {
		t.Fatal("expected children_policy.required issue")
	}
}

func TestValidateFactorHierarchyRejectsReportGroupScoring(t *testing.T) {
	t.Parallel()

	issues := factor.ValidateFactorHierarchy([]factor.FactorSnapshot{{
		Code: "section_a", Role: factor.FactorRoleReportGroup, ScoringStrategy: "sum",
	}})
	if len(issues) == 0 {
		t.Fatal("expected report_group.scoring_forbidden issue")
	}
}

func TestDeriveLevels(t *testing.T) {
	t.Parallel()

	factors := factor.DeriveLevels([]factor.FactorSnapshot{
		{Code: "gec", Role: factor.FactorRoleIndex},
		{Code: "bri", ParentCode: "gec", Role: factor.FactorRoleIndex},
		{Code: "inhibit", ParentCode: "bri", Role: factor.FactorRoleDimension},
	})
	if factors[0].Level != 1 || factors[1].Level != 2 || factors[2].Level != 3 {
		t.Fatalf("levels = %d,%d,%d, want 1,2,3", factors[0].Level, factors[1].Level, factors[2].Level)
	}
}

func TestFactorCorePathMatchesSnapshotWrappers(t *testing.T) {
	t.Parallel()

	snapshots := []factor.FactorSnapshot{
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
	factors := factor.FactorsFromSnapshots(snapshots)

	if got, want := factor.SnapshotsFromFactors(factor.DeriveFactorLevels(factors)), factor.DeriveLevels(snapshots); !reflect.DeepEqual(got, want) {
		t.Fatalf("DeriveFactorLevels mismatch\n got: %#v\nwant: %#v", got, want)
	}
	if got, want := factor.ValidateFactors(factors), factor.ValidateFactorHierarchy(snapshots); !reflect.DeepEqual(got, want) {
		t.Fatalf("ValidateFactors mismatch\n got: %#v\nwant: %#v", got, want)
	}
	if got, want := factor.CalculationScoreNodesFromFactors(factors), factor.CalculationScoreNodesFromSnapshots(snapshots); !reflect.DeepEqual(got, want) {
		t.Fatalf("CalculationScoreNodesFromFactors mismatch\n got: %#v\nwant: %#v", got, want)
	}
}
