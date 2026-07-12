package calculationadapter

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scoring"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

// ExecutionFromScoringInterpretation maps factor-scoring output to the
// canonical in-memory Evaluation result.
func ExecutionFromScoringInterpretation(
	result *scoring.Result,
	modelRef domainoutcome.ModelRef,
) *domainoutcome.Execution {
	if result == nil {
		return nil
	}
	level := string(result.RiskLevel)
	summaryScore := result.TotalScore
	execution := domainoutcome.NewExecution(
		modelRef,
		domainoutcome.Summary{
			PrimaryLabel: level,
			Score:        &summaryScore,
			Level:        &level,
		},
		domainoutcome.Detail{Kind: modelRef.Kind()},
	)
	execution.Primary = &domainoutcome.ScoreValue{
		Kind:  domainoutcome.ScoreKindRawTotal,
		Value: result.TotalScore,
	}
	if result.RiskLevel != "" {
		execution.Level = &domainoutcome.ResultLevel{
			Code:  string(result.RiskLevel),
			Label: string(result.RiskLevel),
		}
	}
	execution.Dimensions = dimensionResultsFromScoring(result)
	return execution
}

func dimensionResultsFromScoring(result *scoring.Result) []domainoutcome.DimensionResult {
	dimensions := make([]domainoutcome.DimensionResult, 0, len(result.FactorScores))
	for _, score := range result.FactorScores {
		dim := domainoutcome.DimensionResult{
			Code: score.FactorCode,
			Name: score.FactorName,
			Kind: domainoutcome.DimensionKindFactor,
			Score: &domainoutcome.ScoreValue{
				Kind:  domainoutcome.ScoreKindRawTotal,
				Value: score.RawScore,
				Max:   cloneFloat64(score.MaxScore),
			},
		}
		if score.IsTotalScore {
			dim.Role = "total"
		}
		if score.RiskLevel != "" {
			dim.Level = &domainoutcome.ResultLevel{Code: string(score.RiskLevel)}
		}
		dimensions = append(dimensions, dim)
	}
	return dimensions
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
