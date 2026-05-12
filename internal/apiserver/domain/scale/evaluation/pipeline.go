package evaluation

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

func (e *Evaluator) runEvaluation(ctx context.Context, input ScaleEvaluationInput) (*ScaleEvaluationResult, error) {
	factorScores, totalScore := e.calculateScores(ctx, input)
	factorScores, riskLevel := e.classifyRisk(input.Scale, factorScores)
	factorScores, conclusion, suggestion := e.interpret(input.Scale, factorScores, totalScore, riskLevel)

	return &ScaleEvaluationResult{
		TotalScore:   totalScore,
		RiskLevel:    riskLevel,
		Conclusion:   conclusion,
		Suggestion:   suggestion,
		FactorScores: factorScores,
	}, nil
}

func (e *Evaluator) calculateScores(ctx context.Context, input ScaleEvaluationInput) ([]ScaleFactorScore, float64) {
	factorScores := make([]ScaleFactorScore, 0, len(input.Scale.Factors))
	for _, factor := range input.Scale.Factors {
		rawScore := e.calculateFactorRawScore(ctx, factor, input.AnswerSheet, input.Questionnaire)
		factorScores = append(factorScores, ScaleFactorScore{
			FactorCode:   factor.Code,
			FactorName:   factor.Title,
			RawScore:     rawScore,
			MaxScore:     cloneEvaluationFloat64Ptr(factor.MaxScore),
			RiskLevel:    scale.RiskLevelNone,
			IsTotalScore: factor.IsTotalScore,
		})
	}
	return factorScores, calculateTotalScore(factorScores)
}

func (e *Evaluator) calculateFactorRawScore(ctx context.Context, factor scale.FactorSnapshot, sheet *ScaleAnswerSheetSnapshot, qnr *ScaleQuestionnaireSnapshot) float64 {
	if sheet == nil {
		return simulateFactorScore(factor)
	}
	if e == nil || e.scoringRegistry == nil {
		return 0
	}
	values, err := collectFactorValues(factor, sheet, qnr)
	if err != nil {
		return 0
	}
	score, err := e.calculator.ScoreDimension(ctx, calculation.Dimension{
		Code:            factor.Code.String(),
		ScoringStrategy: string(factor.ScoringStrategy),
	}, values)
	if e.calculator == nil {
		score, err = e.scoringRegistry.ScoreFactor(ctx, factor, values)
	}
	if err != nil {
		return 0
	}
	return score
}

func calculateTotalScore(factorScores []ScaleFactorScore) float64 {
	var totalScore float64
	for _, fs := range factorScores {
		if fs.IsTotalScore {
			return fs.RawScore
		}
		totalScore += fs.RawScore
	}
	return totalScore
}

func cloneEvaluationFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
