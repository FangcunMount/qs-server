package scoring

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
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
	factorScores := make([]FactorScore, 0, len(input.Model.Factors))
	for _, factor := range input.Model.Factors {
		rawScore, err := e.calculateFactorRawScore(ctx, factor, input.AnswerSheet, input.Questionnaire)
		if err != nil {
			return nil, 0, err
		}
		factorScores = append(factorScores, FactorScore{
			FactorCode:   factor.Code,
			FactorName:   factor.Title,
			RawScore:     rawScore,
			MaxScore:     cloneFloat64Ptr(factor.MaxScore),
			RiskLevel:    RiskLevelNone,
			IsTotalScore: factor.IsTotalScore,
		})
	}
	return factorScores, calculateTotalScore(factorScores), nil
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
	score, err := e.calculator.ScoreDimension(ctx, calculation.Dimension{
		Code:         factor.Code,
		StrategyCode: factor.ScoringStrategy,
	}, values)
	if e.calculator == nil {
		score, err = e.scoringRegistry.ScoreFactor(ctx, factor, values)
	}
	if err != nil {
		return 0, err
	}
	return score, nil
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
