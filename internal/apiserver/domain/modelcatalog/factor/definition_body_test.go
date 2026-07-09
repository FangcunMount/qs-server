package factor_test

import (
	"encoding/json"
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
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
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
