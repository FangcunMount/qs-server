package factor_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestDefinitionBodyJSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := factor.DefinitionBody{
		Dimensions: []factor.DimensionRule{{
			Code: "total", Title: "总分", ScoringStrategy: "sum", IsTotalScore: true,
			ScoringParams: &factor.ScoringParamsPayload{CntOptionContents: []string{"yes"}},
		}},
		InterpretRules: []factor.InterpretRule{{
			DimensionCode: "total",
			Ranges:        []factor.ScoreRangeRule{{MinScore: 0, MaxScore: 10, Level: "low"}},
		}},
	}
	raw, err := factor.MarshalDefinitionBodyJSON(original)
	if err != nil {
		t.Fatalf("MarshalDefinitionBodyJSON: %v", err)
	}
	got, err := factor.ParseDefinitionBodyJSON(raw)
	if err != nil {
		t.Fatalf("ParseDefinitionBodyJSON: %v", err)
	}
	if len(got.Dimensions) != 1 || got.Dimensions[0].Code != "total" {
		t.Fatalf("dimensions = %#v", got.Dimensions)
	}
	if got.Dimensions[0].ScoringParams == nil || len(got.Dimensions[0].ScoringParams.CntOptionContents) != 1 {
		t.Fatalf("scoring params = %#v", got.Dimensions[0].ScoringParams)
	}
}

func TestApplyBrief2NormMetadata(t *testing.T) {
	t.Parallel()

	factors := factor.ApplyBrief2NormMetadata([]factor.FactorSnapshot{
		{Code: "bri"},
		{Code: "inconsistency"},
		{Code: "gec"},
	}, factor.Brief2NormContext{
		NormTableVersion: "2024",
		IndexCodes:       []string{"bri", "gec"},
		ValidityCodes:    []string{"inconsistency"},
		NormFactorCodes:  []string{"gec"},
	})
	if factors[0].ResolvedRole() != factor.FactorRoleIndex {
		t.Fatalf("bri role = %s", factors[0].ResolvedRole())
	}
	if factors[1].ResolvedRole() != factor.FactorRoleValidity {
		t.Fatalf("validity role = %s", factors[1].ResolvedRole())
	}
	if factors[2].Norm == nil || factors[2].Norm.NormTableVersion != "2024" {
		t.Fatalf("gec norm = %#v", factors[2].Norm)
	}
}

func TestApplySPMNormMetadata(t *testing.T) {
	t.Parallel()

	factors := factor.ApplySPMNormMetadata([]factor.FactorSnapshot{
		{Code: "A"},
		{Code: "total", IsTotalScore: true},
	}, factor.SPMNormContext{
		NormTableVersion: "2024",
		ItemSetCodes:     []string{"A"},
	})
	if factors[0].ResolvedRole() != factor.FactorRoleTaskSet {
		t.Fatalf("task set role = %s", factors[0].ResolvedRole())
	}
	if factors[1].Norm == nil || factors[1].Norm.NormTableVersion != "2024" {
		t.Fatalf("total norm = %#v", factors[1].Norm)
	}
}
