package scoring

import (
	"fmt"
	"slices"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
	"github.com/FangcunMount/qs-server/internal/pkg/answervalue"
)

func collectFactorValues(factor Factor, sheet *AnswerSheet, qnr *Questionnaire) ([]float64, error) {
	if len(factor.ChildCodes) > 0 {
		return nil, fmt.Errorf("composite factor %s must be scored from child factor scores", factor.Code)
	}
	code, ok := capability.Canonical(capability.PathScaleDescriptor, capability.UsageQuestionAggregation, factor.ScoringStrategy)
	if !ok {
		return nil, fmt.Errorf("unsupported factor scoring strategy for %s: %s", factor.Code, factor.ScoringStrategy)
	}
	switch Strategy(code) {
	case StrategySum, StrategyAvg:
		return collectQuestionScores(factor, sheet), nil
	case StrategyCnt:
		if qnr == nil {
			return nil, fmt.Errorf("questionnaire is required")
		}
		return collectCntMatches(factor, sheet, qnr), nil
	default:
		return nil, fmt.Errorf("unsupported factor scoring strategy for %s: %s", factor.Code, factor.ScoringStrategy)
	}
}

func collectQuestionScores(factor Factor, sheet *AnswerSheet) []float64 {
	// MissingAnswerPolicyFor(scale, question_aggregation) == skip.
	answerMap := factorScoreAnswerMap(sheet)
	if len(factor.Contributions) > 0 {
		scores := make([]float64, 0, len(factor.Contributions))
		for _, contrib := range factor.Contributions {
			answer, found := answerMap[contrib.Code]
			if !found {
				continue
			}
			scores = append(scores, applyQuestionContribution(contrib, answer))
		}
		return scores
	}
	scores := make([]float64, 0, len(factor.QuestionCodes))
	for _, qCode := range factor.QuestionCodes {
		if answer, found := answerMap[qCode]; found {
			scores = append(scores, answer.Score)
		}
	}
	return scores
}

func applyQuestionContribution(contrib QuestionContribution, answer Answer) float64 {
	base := answer.Score
	if contrib.ScoringMode == "option_override" {
		if optionID := factorScoreOptionID(answer); optionID != "" {
			if score, ok := contrib.OptionScores[optionID]; ok {
				base = score
			}
		}
	}
	sign := contrib.Sign
	if sign == 0 {
		sign = 1
	}
	weight := contrib.Weight
	if weight == 0 {
		weight = 1
	}
	return base * sign * weight
}

func collectChildValues(factor Factor, rawByCode map[string]float64) []float64 {
	code, _ := capability.Canonical(capability.PathScaleDescriptor, capability.UsageCompositeProjection, factor.ScoringStrategy)
	values := make([]float64, 0, len(factor.ChildCodes))
	for _, child := range factor.ChildCodes {
		score, ok := rawByCode[child]
		if !ok {
			continue
		}
		if code == "weighted_sum" {
			weight := 1.0
			if factor.ChildWeights != nil {
				if w, ok := factor.ChildWeights[child]; ok {
					weight = w
				}
			}
			values = append(values, score*weight)
			continue
		}
		values = append(values, score)
	}
	return values
}

func collectCntMatches(factor Factor, sheet *AnswerSheet, qnr *Questionnaire) []float64 {
	targetContents := factor.ScoringParams.CntOptionContents
	if len(targetContents) == 0 {
		return nil
	}
	optionContentMap := factorScoreOptionContentMap(qnr)
	answerMap := factorScoreAnswerMap(sheet)
	codes := factor.QuestionCodes
	if len(factor.Contributions) > 0 {
		codes = make([]string, 0, len(factor.Contributions))
		for _, contrib := range factor.Contributions {
			codes = append(codes, contrib.Code)
		}
	}
	matchValues := make([]float64, 0, len(codes))
	for _, qCode := range codes {
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
		if slices.Contains(targetContents, optionContent) {
			matchValues = append(matchValues, 1.0)
		}
	}
	return matchValues
}

func factorScoreOptionContentMap(qnr *Questionnaire) map[string]string {
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

func factorScoreAnswerMap(sheet *AnswerSheet) map[string]Answer {
	answerMap := make(map[string]Answer)
	if sheet == nil {
		return answerMap
	}
	for _, ans := range sheet.Answers {
		answerMap[ans.QuestionCode.String()] = ans
	}
	return answerMap
}

func factorScoreOptionID(answer Answer) string {
	raw := answer.Value
	if raw == nil {
		return ""
	}
	if option, ok := answervalue.NormalizeSingleOption(raw); ok {
		return option
	}
	if str, ok := raw.(string); ok {
		return str
	}
	if arr, ok := raw.([]string); ok && len(arr) > 0 {
		return arr[0]
	}
	return ""
}
