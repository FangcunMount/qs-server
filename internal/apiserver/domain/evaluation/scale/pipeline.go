package scale

import (
	"context"
	"fmt"

	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
)

func (e *Evaluator) runScoring(ctx context.Context, input ScaleInterpretationInput) ([]ScaleFactorScore, float64, RiskLevel, error) {
	factorScores, totalScore, err := e.calculateScores(ctx, input)
	if err != nil {
		return nil, 0, "", err
	}
	factorScores, riskLevel := e.classifyRisk(input.Scale, factorScores)
	return factorScores, totalScore, riskLevel, nil
}

func (e *Evaluator) runEvaluation(ctx context.Context, input ScaleInterpretationInput) (*ScaleInterpretationResult, error) {
	factorScores, totalScore, riskLevel, err := e.runScoring(ctx, input)
	if err != nil {
		return nil, err
	}
	factorScores, conclusion, suggestion := e.interpret(input.Scale, factorScores, totalScore, riskLevel)

	return &ScaleInterpretationResult{
		TotalScore:   totalScore,
		RiskLevel:    riskLevel,
		Conclusion:   conclusion,
		Suggestion:   suggestion,
		FactorScores: factorScores,
	}, nil
}

func (e *Evaluator) calculateScores(ctx context.Context, input ScaleInterpretationInput) ([]ScaleFactorScore, float64, error) {
	factorScores := make([]ScaleFactorScore, 0, len(input.Scale.Factors))
	for _, factor := range input.Scale.Factors {
		rawScore, err := e.calculateFactorRawScore(ctx, factor, input.AnswerSheet, input.Questionnaire)
		if err != nil {
			return nil, 0, err
		}
		factorScores = append(factorScores, ScaleFactorScore{
			FactorCode:   factor.Code,
			FactorName:   factor.Title,
			RawScore:     rawScore,
			MaxScore:     cloneEvaluationFloat64Ptr(factor.MaxScore),
			RiskLevel:    RiskLevelNone,
			IsTotalScore: factor.IsTotalScore,
		})
	}
	return factorScores, calculateTotalScore(factorScores), nil
}

func (e *Evaluator) calculateFactorRawScore(ctx context.Context, factor scalesnapshot.FactorSnapshot, sheet *ScaleAnswerSheetSnapshot, qnr *ScaleQuestionnaireSnapshot) (float64, error) {
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
