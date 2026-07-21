package norm_test

import (
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
)

func TestProjectionKeepsOutcomeCodeWithoutPresentationCopy(t *testing.T) {
	t.Parallel()
	const outcomeCode = "elevated"
	const longCopy = "问卷提示日常执行功能表现的整体水平方面的困难较为明显，可能已影响学习、生活自理、情绪行为和人际适应的整体功能。"
	tables := &calcnorm.NormTables{
		Factors: []calcnorm.FactorNormTable{{
			FactorCode: "gec",
			Lookup: []calcnorm.NormLookupEntry{{
				RawMin: 0, RawMax: 100, TScore: 65, Percentile: 90,
			}},
		}},
		TScoreRules: []calcnorm.TScoreInterpretRule{{
			FactorCode: "gec",
			Ranges: []calcnorm.TScoreRange{{
				MinT: 60, MaxT: 100, MaxInclusive: true,
				Level: outcomeCode, Conclusion: longCopy, Suggestion: "建议关注",
			}},
		}},
	}
	result, err := calcnorm.Projection{
		Tables:               tables,
		PrimaryDimensionCode: "gec",
	}.Apply(&calculation.Result{
		Dimensions: []calculation.DimensionResult{{
			Code:  "gec",
			Score: &calculation.ScoreValue{Value: 10},
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	dim := result.Dimensions[0]
	if dim.Level == nil || dim.Level.Code != outcomeCode {
		t.Fatalf("level code = %#v, want %q", dim.Level, outcomeCode)
	}
	if dim.Level.Label != "" || dim.Description != "" || dim.Suggestion != "" {
		t.Fatalf("dimension carries presentation copy: level=%#v desc=%q sugg=%q", dim.Level, dim.Description, dim.Suggestion)
	}
	if result.PrimaryLabel != "" {
		t.Fatalf("PrimaryLabel = %q, want unset", result.PrimaryLabel)
	}
}

func TestProjectionRequiredFactorFailureIsAtomic(t *testing.T) {
	t.Parallel()

	tables := &calcnorm.NormTables{Factors: []calcnorm.FactorNormTable{
		{FactorCode: "optional", Lookup: []calcnorm.NormLookupEntry{{RawMin: 0, RawMax: 10, TScore: 55, Percentile: 70}}},
		{FactorCode: "required", Lookup: []calcnorm.NormLookupEntry{{RawMin: 0, RawMax: 10, MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female", TScore: 65, Percentile: 92}}},
	}}
	result := &calculation.Result{Dimensions: []calculation.DimensionResult{
		{Code: "optional", Score: &calculation.ScoreValue{Value: 5}},
		{Code: "required", Score: &calculation.ScoreValue{Value: 5}},
	}}

	got, err := (calcnorm.Projection{Tables: tables, RequiredFactorCodes: []string{"required"}}).Apply(result)
	if got != nil {
		t.Fatalf("result = %#v, want nil on required failure", got)
	}
	var resolutionErr *calcnorm.ResolutionError
	if !errors.As(err, &resolutionErr) || resolutionErr.Kind != calcnorm.ErrorKindSubjectMissing {
		t.Fatalf("error = %T %v, want norm_subject_missing", err, err)
	}
	for _, dim := range result.Dimensions {
		if len(dim.DerivedScores) != 0 || dim.NormReference != nil {
			t.Fatalf("original result was partially mutated: %#v", result)
		}
	}
}

func TestProjectionOptionalFactorMissKeepsRawScore(t *testing.T) {
	t.Parallel()

	result := &calculation.Result{Dimensions: []calculation.DimensionResult{{Code: "optional", Score: &calculation.ScoreValue{Value: 20}}}}
	got, err := (calcnorm.Projection{Tables: &calcnorm.NormTables{Factors: []calcnorm.FactorNormTable{{
		FactorCode: "optional", Lookup: []calcnorm.NormLookupEntry{{RawMin: 0, RawMax: 10, TScore: 55, Percentile: 70}},
	}}}}).Apply(result)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got.Dimensions[0].Score == nil || got.Dimensions[0].Score.Value != 20 || len(got.Dimensions[0].DerivedScores) != 0 || got.Dimensions[0].NormReference != nil {
		t.Fatalf("optional miss = %#v", got.Dimensions[0])
	}
}
