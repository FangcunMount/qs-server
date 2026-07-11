package reporting

import (
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

func TestScaleReportInputPrefersOutcomeDimensionsWithHierarchy(t *testing.T) {
	t.Parallel()

	input := factorScoringReportInputFromOutcome(evaloutcome.Outcome{
		Execution: &domainoutcome.Execution{
			Primary: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 10},
			Level:   &domainoutcome.ResultLevel{Code: string(assessment.RiskLevelMedium)},
			Dimensions: []domainoutcome.DimensionResult{
				{
					Code: "gec", Name: "GEC", Role: "index", HierarchyLevel: 1,
					Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 10},
					Level: &domainoutcome.ResultLevel{Code: string(assessment.RiskLevelMedium)},
				},
				{
					Code: "bri", Name: "BRI", Role: "index", ParentCode: "gec", HierarchyLevel: 2,
					Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 10},
				},
			},
			Detail: domainoutcome.Detail{
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

func TestScaleReportInputUsesFlatFactorScoresWithoutHierarchy(t *testing.T) {
	t.Parallel()

	input := factorScoringReportInputFromOutcome(evaloutcome.Outcome{
		Execution: &domainoutcome.Execution{
			Dimensions: []domainoutcome.DimensionResult{
				{Code: "total", Name: "总分", Score: &domainoutcome.ScoreValue{Value: 5}},
			},
			Detail: domainoutcome.Detail{
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
