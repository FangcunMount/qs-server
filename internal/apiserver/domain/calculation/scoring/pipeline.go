package scoring

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
)

func (e *Evaluator) runScoring(ctx context.Context, input Input) ([]FactorScore, float64, RiskLevel, error) {
	factorScores, totalScore, err := e.calculateScores(ctx, input)
	if err != nil {
		return nil, 0, "", err
	}
	factorScores, riskLevel := classifyRisk(input.Model, factorScores)
	return factorScores, totalScore, riskLevel, nil
}

func (e *Evaluator) calculateScores(ctx context.Context, input Input) ([]FactorScore, float64, error) {
	factorsByCode := make(map[string]Factor, len(input.Model.Factors))
	for _, factor := range input.Model.Factors {
		factorsByCode[factor.Code] = factor
	}
	rawByCode := make(map[string]float64, len(input.Model.Factors))

	for _, factor := range input.Model.Factors {
		if len(factor.ChildCodes) > 0 {
			continue
		}
		rawScore, err := e.calculateFactorRawScore(ctx, factor, input.AnswerSheet, input.Questionnaire)
		if err != nil {
			return nil, 0, err
		}
		rawByCode[factor.Code] = rawScore
	}

	for progress := true; progress; {
		progress = false
		for _, factor := range input.Model.Factors {
			if len(factor.ChildCodes) == 0 {
				continue
			}
			if _, done := rawByCode[factor.Code]; done {
				continue
			}
			if !compositeChildrenReady(factor, factorsByCode, rawByCode) {
				continue
			}
			values := collectChildValues(factor, rawByCode)
			rawScore, err := e.aggregateFactorValues(ctx, factor, values)
			if err != nil {
				return nil, 0, err
			}
			rawByCode[factor.Code] = rawScore
			progress = true
		}
	}

	factorScores := make([]FactorScore, 0, len(input.Model.Factors))
	for _, factor := range input.Model.Factors {
		rawScore, ok := rawByCode[factor.Code]
		if !ok {
			return nil, 0, fmt.Errorf("unable to score factor %s (unresolved composite dependencies)", factor.Code)
		}
		factorScores = append(factorScores, FactorScore{
			FactorCode:   factor.Code,
			FactorName:   factor.Title,
			SortOrder:    factor.SortOrder,
			RawScore:     rawScore,
			MaxScore:     cloneFloat64Ptr(factor.MaxScore),
			RiskLevel:    RiskLevelNone,
			IsTotalScore: factor.IsTotalScore,
		})
	}
	return factorScores, calculateTotalScore(factorScores), nil
}

func compositeChildrenReady(factor Factor, factorsByCode map[string]Factor, rawByCode map[string]float64) bool {
	for _, child := range factor.ChildCodes {
		if _, known := factorsByCode[child]; !known {
			continue
		}
		if _, ok := rawByCode[child]; !ok {
			return false
		}
	}
	return true
}

func (e *Evaluator) calculateFactorRawScore(ctx context.Context, factor Factor, sheet *AnswerSheet, qnr *Questionnaire) (float64, error) {
	if sheet == nil {
		return 0, fmt.Errorf("answer sheet is required for scale factor scoring")
	}
	if e == nil || e.scoringRegistry == nil {
		return 0, nil
	}
	values, err := collectFactorValues(factor, sheet, qnr)
	if err != nil {
		return 0, err
	}
	return e.aggregateFactorValues(ctx, factor, values)
}

func (e *Evaluator) aggregateFactorValues(ctx context.Context, factor Factor, values []float64) (float64, error) {
	if e == nil || e.scoringRegistry == nil {
		return 0, nil
	}
	return e.scoringRegistry.ScoreFactor(ctx, factor, values)
}

func calculateTotalScore(factorScores []FactorScore) float64 {
	var totalScore float64
	for _, fs := range factorScores {
		if fs.IsTotalScore {
			return fs.RawScore
		}
		totalScore += fs.RawScore
	}
	return totalScore
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// usageForFactor selects the capability usage for strategy validation/scoring.
func usageForFactor(factor Factor) capability.Usage {
	if len(factor.ChildCodes) > 0 {
		return capability.UsageCompositeProjection
	}
	return capability.UsageQuestionAggregation
}
