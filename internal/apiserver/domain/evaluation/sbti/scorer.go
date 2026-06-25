package sbti

import (
	"fmt"
	"math"
	"strings"

	rulesetsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/sbti"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

func Score(model *rulesetsbti.ModelSnapshot, answerSheet *evaluationinput.AnswerSheet) (ResultDetail, error) {
	if model == nil {
		return ResultDetail{}, fmt.Errorf("sbti model is required")
	}
	if answerSheet == nil {
		return ResultDetail{}, fmt.Errorf("answer sheet is required")
	}
	if outcome, ok := triggeredDrinkOutcome(model, answerSheet.Answers); ok {
		return resultDetailFromOutcome(model, outcome, nil, 1, strings.TrimSpace(outcome.Trigger)), nil
	}

	rawScores, err := collectDimensionScores(model, answerSheet.Answers)
	if err != nil {
		return ResultDetail{}, err
	}
	dimensions := buildDimensionResults(model, rawScores)
	outcome, similarity, err := bestOutcome(model, dimensions)
	if err != nil {
		return ResultDetail{}, err
	}
	trigger := ""
	threshold := fallbackThreshold(model)
	if similarity < threshold {
		fallback, ok := findOutcome(model.SpecialOutcomes, "HHHH")
		if !ok {
			return ResultDetail{}, fmt.Errorf("sbti fallback outcome HHHH is not configured")
		}
		outcome = fallback
		trigger = fallback.Trigger
	}
	return resultDetailFromOutcome(model, outcome, dimensions, similarity, trigger), nil
}

func triggeredDrinkOutcome(model *rulesetsbti.ModelSnapshot, answers []evaluationinput.Answer) (rulesetsbti.OutcomeSnapshot, bool) {
	if model == nil || len(model.DrinkTrigger.QuestionCodes) == 0 || len(model.DrinkTrigger.OptionValues) == 0 {
		return rulesetsbti.OutcomeSnapshot{}, false
	}
	questionCodes := evaluationinput.StringSet(model.DrinkTrigger.QuestionCodes)
	values := evaluationinput.StringSet(model.DrinkTrigger.OptionValues)
	for _, answer := range answers {
		if !questionCodes[answer.QuestionCode] {
			continue
		}
		if values[evaluationinput.AnswerValueKey(answer.Value)] {
			outcome, ok := findOutcome(model.SpecialOutcomes, "DRUNK")
			return outcome, ok
		}
	}
	return rulesetsbti.OutcomeSnapshot{}, false
}

func collectDimensionScores(model *rulesetsbti.ModelSnapshot, answers []evaluationinput.Answer) (map[string]float64, error) {
	answerByQuestion := make(map[string]evaluationinput.Answer, len(answers))
	for _, answer := range answers {
		answerByQuestion[answer.QuestionCode] = answer
	}
	rawScores := make(map[string]float64, len(model.DimensionOrder))
	counts := make(map[string]int, len(model.DimensionOrder))
	for _, mapping := range model.QuestionMappings {
		answer, ok := answerByQuestion[mapping.QuestionCode]
		if !ok {
			return nil, fmt.Errorf("missing sbti answer for question %s", mapping.QuestionCode)
		}
		score, err := scoreForAnswer(mapping, answer)
		if err != nil {
			return nil, err
		}
		rawScores[mapping.Dimension] += score
		counts[mapping.Dimension]++
	}
	for _, dim := range model.DimensionOrder {
		if counts[dim] == 0 {
			return nil, fmt.Errorf("sbti dimension %s has no mapped answers", dim)
		}
	}
	return rawScores, nil
}

func scoreForAnswer(mapping rulesetsbti.QuestionMappingSnapshot, answer evaluationinput.Answer) (float64, error) {
	value := evaluationinput.AnswerValueKey(answer.Value)
	if value != "" {
		if score, ok := mapping.OptionScores[value]; ok {
			return score, nil
		}
		if score, ok := mapping.OptionScores[strings.ToUpper(value)]; ok {
			return score, nil
		}
	}
	if answer.Score > 0 {
		return answer.Score, nil
	}
	return 0, fmt.Errorf("invalid sbti answer for question %s: %v", mapping.QuestionCode, answer.Value)
}

func buildDimensionResults(model *rulesetsbti.ModelSnapshot, rawScores map[string]float64) []DimensionResult {
	results := make([]DimensionResult, 0, len(model.DimensionOrder))
	for _, dimCode := range model.DimensionOrder {
		meta := model.Dimensions[dimCode]
		raw := rawScores[dimCode]
		results = append(results, DimensionResult{
			Code:     dimCode,
			Name:     meta.Name,
			Model:    meta.Model,
			RawScore: raw,
			Level:    levelForScore(raw),
		})
	}
	return results
}

func levelForScore(score float64) string {
	switch {
	case score <= 3:
		return "L"
	case score >= 5:
		return "H"
	default:
		return "M"
	}
}

func bestOutcome(model *rulesetsbti.ModelSnapshot, dimensions []DimensionResult) (rulesetsbti.OutcomeSnapshot, float64, error) {
	if len(model.NormalOutcomes) == 0 {
		return rulesetsbti.OutcomeSnapshot{}, 0, fmt.Errorf("sbti normal outcomes are not configured")
	}
	actual := make([]string, 0, len(dimensions))
	for _, dim := range dimensions {
		actual = append(actual, dim.Level)
	}

	var (
		best        rulesetsbti.OutcomeSnapshot
		bestScore   = math.Inf(-1)
		hasBest     bool
		maxDistance = float64(len(actual) * 2)
	)
	for _, outcome := range model.NormalOutcomes {
		expected := patternLevels(outcome.Pattern)
		if len(expected) != len(actual) {
			continue
		}
		distance := 0
		for i := range actual {
			distance += evaluationinput.AbsInt(levelValue(actual[i]) - levelValue(expected[i]))
		}
		similarity := 1 - float64(distance)/maxDistance
		if !hasBest || similarity > bestScore {
			best = outcome
			bestScore = similarity
			hasBest = true
		}
	}
	if !hasBest {
		return rulesetsbti.OutcomeSnapshot{}, 0, fmt.Errorf("no valid sbti outcome patterns configured")
	}
	return best, bestScore, nil
}

func resultDetailFromOutcome(
	model *rulesetsbti.ModelSnapshot,
	outcome rulesetsbti.OutcomeSnapshot,
	dimensions []DimensionResult,
	similarity float64,
	trigger string,
) ResultDetail {
	return ResultDetail{
		TypeCode:       outcome.Code,
		TypeName:       outcome.Name,
		OneLiner:       outcome.OneLiner,
		Pattern:        outcome.Pattern,
		Similarity:     similarity,
		ImageURL:       outcome.Image,
		Rarity:         outcome.Rarity,
		Dimensions:     dimensions,
		Outcome:        outcome,
		Source:         model.Source,
		SpecialTrigger: trigger,
	}
}

func fallbackThreshold(model *rulesetsbti.ModelSnapshot) float64 {
	if model == nil || model.FallbackSimilarityThreshold <= 0 {
		return 0.6
	}
	return model.FallbackSimilarityThreshold
}

func patternLevels(pattern string) []string {
	compact := strings.ReplaceAll(pattern, "-", "")
	levels := make([]string, 0, len(compact))
	for _, r := range compact {
		levels = append(levels, strings.ToUpper(string(r)))
	}
	return levels
}

func levelValue(level string) int {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "L":
		return 0
	case "M":
		return 1
	case "H":
		return 2
	default:
		return 1
	}
}

func findOutcome(outcomes []rulesetsbti.OutcomeSnapshot, code string) (rulesetsbti.OutcomeSnapshot, bool) {
	for _, outcome := range outcomes {
		if outcome.Code == code {
			return outcome, true
		}
	}
	return rulesetsbti.OutcomeSnapshot{}, false
}
