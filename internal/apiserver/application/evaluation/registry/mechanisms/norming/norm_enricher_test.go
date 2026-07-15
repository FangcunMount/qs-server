package norming_test

import (
	"testing"

	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/norming"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

func TestApplyNormProjectionAppliesNormAndInterpretation(t *testing.T) {
	t.Parallel()

	outcome := &domainoutcome.Execution{
		Dimensions: []domainoutcome.DimensionResult{{
			Code:  "gec",
			Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 5},
		}},
	}
	snapshot := &behavioralsnapshot.Snapshot{
		Norming: &behavioralsnapshot.NormingProfile{
			PrimaryDimensionCode: "gec",
			NormTables: &calcnorm.NormTables{
				NormTableVersion: "2026",
				FormVariant:      "teacher",
				Factors: []calcnorm.FactorNormTable{{
					FactorCode: "gec",
					Lookup: []calcnorm.NormLookupEntry{
						{RawMin: 0, RawMax: 10, MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female", TScore: 65, Percentile: 90},
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

	enriched := factornorm.ApplyNormProjection(outcome, snapshot, calcnorm.Subject{AgeMonths: 72, Gender: "female"})
	if len(enriched.Dimensions) != 1 {
		t.Fatalf("dimensions = %#v", enriched.Dimensions)
	}
	dim := enriched.Dimensions[0]
	if got := derivedScore(dim.DerivedScores, domainoutcome.ScoreKindTScore); got != 65 {
		t.Fatalf("t_score = %v, want 65", got)
	}
	if got := derivedScore(dim.DerivedScores, domainoutcome.ScoreKindPercentile); got != 90 {
		t.Fatalf("percentile = %v, want 90", got)
	}
	if dim.Level == nil || dim.Level.Code != "elevated" {
		t.Fatalf("level = %#v", dim.Level)
	}
	if dim.NormReference == nil || dim.NormReference.ScoreKind != domainoutcome.ScoreKindTScore || dim.NormReference.Benchmark != 50 || dim.NormReference.TableVersion != "2026" || dim.NormReference.FormVariant != "teacher" || dim.NormReference.MinAgeMonths != 60 || dim.NormReference.MaxAgeMonths != 95 || dim.NormReference.Gender != "female" {
		t.Fatalf("norm reference = %#v", dim.NormReference)
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

func derivedScore(scores []domainoutcome.ScoreValue, kind domainoutcome.ScoreKind) float64 {
	for _, score := range scores {
		if score.Kind == kind {
			return score.Value
		}
	}
	return 0
}
