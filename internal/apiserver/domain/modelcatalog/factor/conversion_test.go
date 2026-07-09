package factor_test

import (
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestFactorIsDistinctDomainType(t *testing.T) {
	t.Parallel()

	if reflect.TypeOf(factor.Factor{}) == reflect.TypeOf(factor.FactorSnapshot{}) {
		t.Fatal("Factor must be a distinct domain type, not an alias of FactorSnapshot")
	}
}

func TestFactorSnapshotRoundTripPreservesAllFields(t *testing.T) {
	t.Parallel()

	maxScore := 42.0
	original := factor.FactorSnapshot{
		Code:            "total",
		Title:           "总分",
		Role:            factor.FactorRoleIndex,
		ParentCode:      "root",
		SortOrder:       2,
		Level:           1,
		IsTotalScore:    true,
		QuestionCodes:   []string{"q1", "q2"},
		ScoringStrategy: "cnt",
		ScoringParams:   &factor.ScoringParams{CntOptionContents: []string{"yes", "no"}},
		MaxScore:        &maxScore,
		InterpretRules: []factor.ScoreRangeRule{{
			MinScore: 0, MaxScore: 10, Level: "low", Conclusion: "低", Suggestion: "观察",
		}},
		Classification: &factor.ClassificationSpec{
			PositivePole: "E",
			NegativePole: "I",
			DecisionRule: "max",
			TieBreakRule: "positive",
		},
		Norm: &factor.NormRef{
			FactorCode:       "total",
			NormTableVersion: "norm-v1",
		},
		ChildrenPolicy: &factor.ChildrenPolicy{
			Strategy: factor.ChildrenAggregationWeightedSum,
			Children: []string{"f1", "f2"},
			Weights:  map[string]float64{"f1": 0.4, "f2": 0.6},
		},
	}

	domainFactor := factor.FactorFromSnapshot(original)
	got := domainFactor.Snapshot()
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("round trip mismatch\n got: %#v\nwant: %#v", got, original)
	}
	if domainFactor.ResolvedRole() != factor.FactorRoleIndex {
		t.Fatalf("domain role = %s, want %s", domainFactor.ResolvedRole(), factor.FactorRoleIndex)
	}

	original.QuestionCodes[0] = "mutated"
	original.ScoringParams.CntOptionContents[0] = "mutated"
	*original.MaxScore = 99
	original.InterpretRules[0].Level = "mutated"
	original.Classification.PositivePole = "mutated"
	original.Norm.NormTableVersion = "mutated"
	original.ChildrenPolicy.Children[0] = "mutated"
	original.ChildrenPolicy.Weights["f1"] = 9.9

	afterOriginalMutation := domainFactor.Snapshot()
	if afterOriginalMutation.QuestionCodes[0] != "q1" ||
		afterOriginalMutation.ScoringParams.CntOptionContents[0] != "yes" ||
		*afterOriginalMutation.MaxScore != 42 ||
		afterOriginalMutation.InterpretRules[0].Level != "low" ||
		afterOriginalMutation.Classification.PositivePole != "E" ||
		afterOriginalMutation.Norm.NormTableVersion != "norm-v1" ||
		afterOriginalMutation.ChildrenPolicy.Children[0] != "f1" ||
		afterOriginalMutation.ChildrenPolicy.Weights["f1"] != 0.4 {
		t.Fatalf("Factor shares mutable state with source snapshot: %#v", afterOriginalMutation)
	}

	snapshot := domainFactor.Snapshot()
	domainFactor.QuestionCodes[0] = "changed"
	domainFactor.ScoringParams.CntOptionContents[0] = "changed"
	*domainFactor.MaxScore = 100
	domainFactor.InterpretRules[0].Level = "changed"
	domainFactor.Classification.PositivePole = "changed"
	domainFactor.Norm.NormTableVersion = "changed"
	domainFactor.ChildrenPolicy.Children[0] = "changed"
	domainFactor.ChildrenPolicy.Weights["f1"] = 8.8

	if snapshot.QuestionCodes[0] != "q1" ||
		snapshot.ScoringParams.CntOptionContents[0] != "yes" ||
		*snapshot.MaxScore != 42 ||
		snapshot.InterpretRules[0].Level != "low" ||
		snapshot.Classification.PositivePole != "E" ||
		snapshot.Norm.NormTableVersion != "norm-v1" ||
		snapshot.ChildrenPolicy.Children[0] != "f1" ||
		snapshot.ChildrenPolicy.Weights["f1"] != 0.4 {
		t.Fatalf("snapshot shares mutable state with Factor: %#v", snapshot)
	}
}

func TestDefinitionBodyCanMaterializeDomainFactors(t *testing.T) {
	t.Parallel()

	body := factor.DefinitionBody{
		Dimensions: []factor.DimensionRule{{
			Code: "total", Title: "总分", Role: string(factor.FactorRoleTotal),
			QuestionCodes: []string{"q1"}, ScoringStrategy: "sum",
		}},
		InterpretRules: []factor.InterpretRule{{
			DimensionCode: "total",
			Ranges:        []factor.ScoreRangeRule{{MinScore: 0, MaxScore: 10, Level: "low"}},
		}},
	}
	snapshots := factor.ParseFactorsFromDefinitionBody(body.Dimensions, body.InterpretRules)
	factors := factor.ParseFactorsFromDefinitionBodyAsFactors(body.Dimensions, body.InterpretRules)
	if got, want := factor.SnapshotsFromFactors(factors), snapshots; !reflect.DeepEqual(got, want) {
		t.Fatalf("domain materialization mismatch\n got: %#v\nwant: %#v", got, want)
	}
	if len(factors) != 1 || factors[0].ResolvedRole() != factor.FactorRoleTotal {
		t.Fatalf("factors = %#v", factors)
	}
}
