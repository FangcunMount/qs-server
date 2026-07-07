package projection_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/projection"
	brief2norm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/brief2"
)

func TestBrief2NormProjectionAppliesNormAndInterpretation(t *testing.T) {
	t.Parallel()

	outcome := &assessment.AssessmentOutcome{
		Dimensions: []assessment.DimensionResult{{
			Code:  "gec",
			Score: &assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindRawTotal, Value: 5},
		}},
	}
	proj := projection.Brief2NormProjection{
		Tables: &brief2norm.NormTables{
			Factors: []brief2norm.FactorNormTable{{
				FactorCode: "gec",
				Lookup: []brief2norm.NormLookupEntry{
					{RawMin: 0, RawMax: 10, TScore: 65, Percentile: 90},
				},
			}},
			TScoreRules: []brief2norm.TScoreInterpretRule{{
				FactorCode: "gec",
				Ranges: []brief2norm.TScoreRange{
					{MinT: 60, MaxT: 100, Level: "elevated", Conclusion: "升高"},
				},
			}},
		},
	}

	enriched := proj.Apply(outcome)
	if len(enriched.Dimensions) != 1 {
		t.Fatalf("dimensions = %#v", enriched.Dimensions)
	}
	dim := enriched.Dimensions[0]
	if got := derivedScore(dim.DerivedScores, assessment.OutcomeScoreKindTScore); got != 65 {
		t.Fatalf("t_score = %v, want 65", got)
	}
	if got := derivedScore(dim.DerivedScores, assessment.OutcomeScoreKindPercentile); got != 90 {
		t.Fatalf("percentile = %v, want 90", got)
	}
	if dim.Level == nil || dim.Level.Code != "elevated" || dim.Description != "升高" {
		t.Fatalf("level = %#v description = %q", dim.Level, dim.Description)
	}
	if enriched.Level == nil || enriched.Level.Code != "elevated" {
		t.Fatalf("outcome level = %#v", enriched.Level)
	}
}

func TestScoreRangeProjectionIsIdentity(t *testing.T) {
	t.Parallel()

	outcome := &assessment.AssessmentOutcome{Summary: assessment.ResultSummary{PrimaryLabel: "raw"}}
	got := projection.ScoreRangeProjection{}.Apply(outcome)
	if got != outcome || got.Summary.PrimaryLabel != "raw" {
		t.Fatalf("ScoreRangeProjection changed outcome: %#v", got)
	}
}

func derivedScore(scores []assessment.OutcomeScoreValue, kind assessment.OutcomeScoreKind) float64 {
	for _, score := range scores {
		if score.Kind == kind {
			return score.Value
		}
	}
	return 0
}
