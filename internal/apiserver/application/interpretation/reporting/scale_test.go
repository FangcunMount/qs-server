package reporting

import (
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

func TestScaleReportInputPrefersOutcomeDimensionsWithHierarchy(t *testing.T) {
	t.Parallel()

	input := factorScoringReportInputFromOutcome(evaloutcome.Outcome{
		Execution: &assessment.AssessmentOutcome{
			Primary: &assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindRawTotal, Value: 10},
			Level:   &assessment.OutcomeResultLevel{Code: string(assessment.RiskLevelMedium)},
			Dimensions: []assessment.DimensionResult{
				{
					Code: "gec", Name: "GEC", Role: "index", HierarchyLevel: 1,
					Score:       &assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindRawTotal, Value: 10},
					Level:       &assessment.OutcomeResultLevel{Code: string(assessment.RiskLevelMedium)},
					Description: "overall",
				},
				{
					Code: "bri", Name: "BRI", Role: "index", ParentCode: "gec", HierarchyLevel: 2,
					Score: &assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindRawTotal, Value: 10},
				},
			},
			Detail: assessment.EvaluationDetail{
				Payload: []assessment.FactorScoreResult{
					{FactorCode: assessment.NewFactorCode("legacy"), RawScore: 1},
				},
			},
		},
	})

	if len(input.FactorScores) != 2 {
		t.Fatalf("factor scores = %d, want 2 from dimensions", len(input.FactorScores))
	}
	if input.FactorScores[1].ParentCode != "gec" || input.FactorScores[1].HierarchyLevel != 2 {
		t.Fatalf("bri score = %#v, want hierarchy metadata", input.FactorScores[1])
	}
}

func TestScaleReportInputUsesLegacyFactorScoresWithoutHierarchy(t *testing.T) {
	t.Parallel()

	input := factorScoringReportInputFromOutcome(evaloutcome.Outcome{
		Execution: &assessment.AssessmentOutcome{
			Dimensions: []assessment.DimensionResult{
				{Code: "total", Name: "总分", Score: &assessment.OutcomeScoreValue{Value: 5}},
			},
			Detail: assessment.EvaluationDetail{
				Payload: []assessment.FactorScoreResult{
					{
						FactorCode: assessment.NewFactorCode("total"), FactorName: "总分",
						RawScore: 5, IsTotalScore: true,
					},
				},
			},
		},
	})

	if len(input.FactorScores) != 1 || !input.FactorScores[0].IsTotalScore {
		t.Fatalf("factor scores = %#v, want legacy payload with IsTotalScore", input.FactorScores)
	}
}
