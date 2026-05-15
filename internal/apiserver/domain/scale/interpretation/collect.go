package interpretation

import (
	"fmt"
	"slices"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

func collectFactorValues(factor scale.FactorSnapshot, sheet *ScaleAnswerSheetSnapshot, qnr *ScaleQuestionnaireSnapshot) ([]float64, error) {
	switch factor.ScoringStrategy {
	case scale.ScoringStrategySum, scale.ScoringStrategyAvg:
		return collectQuestionScores(factor, sheet), nil
	case scale.ScoringStrategyCnt:
		if qnr == nil {
			return nil, fmt.Errorf("questionnaire is required")
		}
		return collectCntMatches(factor, sheet, qnr), nil
	default:
		return nil, fmt.Errorf("unsupported factor scoring strategy for %s: %s", factor.Code, factor.ScoringStrategy)
	}
}

func collectQuestionScores(factor scale.FactorSnapshot, sheet *ScaleAnswerSheetSnapshot) []float64 {
	answerMap := factorScoreAnswerMap(sheet)
	scores := make([]float64, 0, len(factor.QuestionCodes))
	for _, qCode := range factor.QuestionCodes {
		if answer, found := answerMap[qCode.String()]; found {
			scores = append(scores, answer.Score)
		}
	}
	return scores
}

func collectCntMatches(factor scale.FactorSnapshot, sheet *ScaleAnswerSheetSnapshot, qnr *ScaleQuestionnaireSnapshot) []float64 {
	targetContents := factor.ScoringParams.GetCntOptionContents()
	if len(targetContents) == 0 {
		return nil
	}
	optionContentMap := factorScoreOptionContentMap(qnr)
	answerMap := factorScoreAnswerMap(sheet)
	matchValues := make([]float64, 0, len(factor.QuestionCodes))
	for _, qCode := range factor.QuestionCodes {
		answer, found := answerMap[qCode.String()]
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
		if slices.Contains(targetContents, optionContent) {
			matchValues = append(matchValues, 1.0)
		}
	}
	return matchValues
}

func factorScoreOptionContentMap(qnr *ScaleQuestionnaireSnapshot) map[string]string {
	contentMap := make(map[string]string)
	if qnr == nil {
		return contentMap
	}
	for _, q := range qnr.Questions {
		for _, opt := range q.Options {
			contentMap[opt.Code] = opt.Content
		}
	}
	return contentMap
}

func factorScoreAnswerMap(sheet *ScaleAnswerSheetSnapshot) map[string]ScaleAnswerSnapshot {
	answerMap := make(map[string]ScaleAnswerSnapshot)
	if sheet == nil {
		return answerMap
	}
	for _, ans := range sheet.Answers {
		answerMap[ans.QuestionCode.String()] = ans
	}
	return answerMap
}

func factorScoreOptionID(answer ScaleAnswerSnapshot) string {
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
