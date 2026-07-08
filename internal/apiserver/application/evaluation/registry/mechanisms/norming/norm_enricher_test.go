package norming_test

import (
	"testing"

	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/norming"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestApplyNormProjectionAppliesNormAndInterpretation(t *testing.T) {
	t.Parallel()

	outcome := &assessment.AssessmentOutcome{
		Dimensions: []assessment.DimensionResult{{
			Code:  "gec",
			Score: &assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindRawTotal, Value: 5},
		}},
	}
	snapshot := &behavioralsnapshot.Snapshot{
		Norming: &behavioralsnapshot.NormingProfile{
			PrimaryDimensionCode: "gec",
			NormTables: &calcnorm.NormTables{
				Factors: []calcnorm.FactorNormTable{{
					FactorCode: "gec",
					Lookup: []calcnorm.NormLookupEntry{
						{RawMin: 0, RawMax: 10, TScore: 65, Percentile: 90},
					},
				}},
				TScoreRules: []calcnorm.TScoreInterpretRule{{
					FactorCode: "gec",
					Ranges: []calcnorm.TScoreRange{
						{MinT: 60, MaxT: 100, Level: "elevated", Conclusion: "升高"},
					},
				}},
			},
		},
	}

	enriched := factornorm.ApplyNormProjection(outcome, snapshot, calcnorm.Subject{})
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

func TestNormSubjectFromInput(t *testing.T) {
	t.Parallel()

	subject := factornorm.NormSubjectFromInput(&evaluationinput.InputSnapshot{
		NormSubject: &evaluationinput.NormSubjectSnapshot{AgeMonths: 72, Gender: "male"},
	})
	if subject.AgeMonths != 72 || subject.Gender != "male" {
		t.Fatalf("subject = %#v", subject)
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
