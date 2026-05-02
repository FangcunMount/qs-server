package pipeline

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

type FactorScoreCalculator interface {
	Calculate(
		ctx context.Context,
		medicalScale *evaluationinput.ScaleSnapshot,
		sheet *evaluationinput.AnswerSheetSnapshot,
		qnr *evaluationinput.QuestionnaireSnapshot,
	) ([]assessment.FactorScoreResult, float64)
}

type ruleEngineFactorScoreCalculator struct {
	scorer ruleengine.ScaleFactorScorer
}

func NewFactorScoreCalculator(scorer ruleengine.ScaleFactorScorer) FactorScoreCalculator {
	return ruleEngineFactorScoreCalculator{scorer: scorer}
}

func (c ruleEngineFactorScoreCalculator) Calculate(
	ctx context.Context,
	medicalScale *evaluationinput.ScaleSnapshot,
	sheet *evaluationinput.AnswerSheetSnapshot,
	qnr *evaluationinput.QuestionnaireSnapshot,
) ([]assessment.FactorScoreResult, float64) {
	if medicalScale == nil {
		return nil, 0
	}
	factorScores := make([]assessment.FactorScoreResult, 0, len(medicalScale.Factors))
	for _, factor := range medicalScale.Factors {
		rawScore := c.calculateFactorRawScore(ctx, factor, sheet, qnr)
		factorScores = append(factorScores, assessment.NewFactorScoreResult(
			assessment.NewFactorCode(factor.Code),
			factor.Title,
			rawScore,
			assessment.RiskLevelNone,
			"",
			"",
			factor.IsTotalScore,
		))
	}
	return factorScores, calculateTotalScore(factorScores)
}

func (c ruleEngineFactorScoreCalculator) calculateFactorRawScore(
	ctx context.Context,
	factor evaluationinput.FactorSnapshot,
	sheet *evaluationinput.AnswerSheetSnapshot,
	qnr *evaluationinput.QuestionnaireSnapshot,
) float64 {
	if sheet == nil {
		return simulateFactorScore(factor)
	}
	if c.scorer == nil {
		return 0
	}
	values, err := collectFactorValues(factor, sheet, qnr)
	if err != nil {
		return 0
	}
	score, err := c.scorer.ScoreFactor(ctx, factor.Code, values, factor.ScoringStrategy, nil)
	if err != nil {
		return 0
	}
	return score
}

func calculateTotalScore(factorScores []assessment.FactorScoreResult) float64 {
	var totalScore float64
	for _, fs := range factorScores {
		if fs.IsTotalScore {
			return fs.RawScore
		}
		totalScore += fs.RawScore
	}
	return totalScore
}

func collectFactorValues(factor evaluationinput.FactorSnapshot, sheet *evaluationinput.AnswerSheetSnapshot, qnr *evaluationinput.QuestionnaireSnapshot) ([]float64, error) {
	switch factor.ScoringStrategy {
	case "sum", "avg":
		return collectQuestionScores(factor, sheet), nil
	case "cnt":
		if qnr == nil {
			return nil, NewHandlerError("questionnaire is required")
		}
		return collectCntMatches(factor, sheet, qnr), nil
	default:
		return nil, nil
	}
}

func collectQuestionScores(factor evaluationinput.FactorSnapshot, sheet *evaluationinput.AnswerSheetSnapshot) []float64 {
	answerMap := factorScoreAnswerMap(sheet)
	scores := make([]float64, 0, len(factor.QuestionCodes))
	for _, qCode := range factor.QuestionCodes {
		if answer, found := answerMap[qCode]; found {
			scores = append(scores, answer.Score)
		}
	}
	return scores
}

func collectCntMatches(factor evaluationinput.FactorSnapshot, sheet *evaluationinput.AnswerSheetSnapshot, qnr *evaluationinput.QuestionnaireSnapshot) []float64 {
	targetContents := factor.ScoringParams.CntOptionContents
	if len(targetContents) == 0 {
		return nil
	}
	optionContentMap := factorScoreOptionContentMap(qnr)
	answerMap := factorScoreAnswerMap(sheet)
	matchValues := make([]float64, 0, len(factor.QuestionCodes))
	for _, qCode := range factor.QuestionCodes {
		answer, found := answerMap[qCode]
		if !found {
			continue
		}
		optionID := factorScoreOptionID(answer)
		if optionID == "" {
			continue
		}
		optionContent, found := optionContentMap[optionID]
		if !found {
			continue
		}
		if factorScoreContainsString(targetContents, optionContent) {
			matchValues = append(matchValues, 1.0)
		}
	}
	return matchValues
}

func simulateFactorScore(factor evaluationinput.FactorSnapshot) float64 {
	questionCount := factor.QuestionCount()
	if questionCount == 0 {
		return 50.0
	}
	return float64(questionCount) * 2.5
}

func factorScoreOptionContentMap(qnr *evaluationinput.QuestionnaireSnapshot) map[string]string {
	contentMap := make(map[string]string)
	for _, q := range qnr.Questions {
		for _, opt := range q.Options {
			contentMap[opt.Code] = opt.Content
		}
	}
	return contentMap
}

func factorScoreAnswerMap(sheet *evaluationinput.AnswerSheetSnapshot) map[string]evaluationinput.AnswerSnapshot {
	answerMap := make(map[string]evaluationinput.AnswerSnapshot)
	for _, ans := range sheet.Answers {
		answerMap[ans.QuestionCode] = ans
	}
	return answerMap
}

func factorScoreOptionID(answer evaluationinput.AnswerSnapshot) string {
	raw := answer.Value
	if raw == nil {
		return ""
	}
	if str, ok := raw.(string); ok {
		return str
	}
	if arr, ok := raw.([]string); ok && len(arr) > 0 {
		return arr[0]
	}
	return ""
}

func factorScoreContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
