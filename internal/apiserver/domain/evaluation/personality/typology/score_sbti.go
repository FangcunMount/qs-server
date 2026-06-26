package typology

import (
	"fmt"
	"math"
	"strings"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

func ScoreSBTI(model *modeltypology.SBTILegacyModel, answerSheet *evaluationinput.AnswerSheet) (SBTIResultDetail, error) {
	if model == nil {
		return SBTIResultDetail{}, fmt.Errorf("sbti model is required")
	}
	if answerSheet == nil {
		return SBTIResultDetail{}, fmt.Errorf("answer sheet is required")
	}
	if outcome, ok := triggeredDrinkOutcome(model, answerSheet.Answers); ok {
		return sbtiResultDetailFromOutcome(model, outcome, nil, 1, strings.TrimSpace(outcome.Trigger)), nil
	}

	rawScores, err := collectSBTIDimensionScores(model, answerSheet.Answers)
	if err != nil {
		return SBTIResultDetail{}, err
	}
	dimensions := buildSBTIDimensionResults(model, rawScores)
	outcome, similarity, err := bestSBTIOutcome(model, dimensions)
	if err != nil {
		return SBTIResultDetail{}, err
	}
	trigger := ""
	threshold := sbtiFallbackThreshold(model)
	if similarity < threshold {
		fallback, ok := findSBTIOutcome(model.SpecialOutcomes, "HHHH")
		if !ok {
			return SBTIResultDetail{}, fmt.Errorf("sbti fallback outcome HHHH is not configured")
		}
		outcome = fallback
		trigger = fallback.Trigger
	}
	return sbtiResultDetailFromOutcome(model, outcome, dimensions, similarity, trigger), nil
}

func triggeredDrinkOutcome(model *modeltypology.SBTILegacyModel, answers []evaluationinput.Answer) (modeltypology.SBTILegacyOutcome, bool) {
	if model == nil || len(model.DrinkTrigger.QuestionCodes) == 0 || len(model.DrinkTrigger.OptionValues) == 0 {
		return modeltypology.SBTILegacyOutcome{}, false
	}
	questionCodes := evaluationinput.StringSet(model.DrinkTrigger.QuestionCodes)
	values := evaluationinput.StringSet(model.DrinkTrigger.OptionValues)
	for _, answer := range answers {
		if !questionCodes[answer.QuestionCode] {
			continue
		}
		if values[evaluationinput.AnswerValueKey(answer.Value)] {
			outcome, ok := findSBTIOutcome(model.SpecialOutcomes, "DRUNK")
			return outcome, ok
		}
	}
	return modeltypology.SBTILegacyOutcome{}, false
}

func collectSBTIDimensionScores(model *modeltypology.SBTILegacyModel, answers []evaluationinput.Answer) (map[string]float64, error) {
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
		score, err := scoreForSBTIAnswer(mapping, answer)
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

func scoreForSBTIAnswer(mapping modeltypology.SBTILegacyQuestionMapping, answer evaluationinput.Answer) (float64, error) {
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

func buildSBTIDimensionResults(model *modeltypology.SBTILegacyModel, rawScores map[string]float64) []SBTIDimensionResult {
	results := make([]SBTIDimensionResult, 0, len(model.DimensionOrder))
	for _, dimCode := range model.DimensionOrder {
		meta := model.Dimensions[dimCode]
		raw := rawScores[dimCode]
		results = append(results, SBTIDimensionResult{
			Code:     dimCode,
			Name:     meta.Name,
			Model:    meta.Model,
			RawScore: raw,
			Level:    sbtiLevelForScore(raw),
		})
	}
	return results
}

func sbtiLevelForScore(score float64) string {
	switch {
	case score <= 3:
		return "L"
	case score >= 5:
		return "H"
	default:
		return "M"
	}
}

func bestSBTIOutcome(model *modeltypology.SBTILegacyModel, dimensions []SBTIDimensionResult) (modeltypology.SBTILegacyOutcome, float64, error) {
	if len(model.NormalOutcomes) == 0 {
		return modeltypology.SBTILegacyOutcome{}, 0, fmt.Errorf("sbti normal outcomes are not configured")
	}
	actual := make([]string, 0, len(dimensions))
	for _, dim := range dimensions {
		actual = append(actual, dim.Level)
	}

	var (
		best        modeltypology.SBTILegacyOutcome
		bestScore   = math.Inf(-1)
		hasBest     bool
		maxDistance = float64(len(actual) * 2)
	)
	for _, outcome := range model.NormalOutcomes {
		expected := sbtiPatternLevels(outcome.Pattern)
		if len(expected) != len(actual) {
			continue
		}
		distance := 0
		for i := range actual {
			distance += evaluationinput.AbsInt(sbtiLevelValue(actual[i]) - sbtiLevelValue(expected[i]))
		}
		similarity := 1 - float64(distance)/maxDistance
		if !hasBest || similarity > bestScore {
			best = outcome
			bestScore = similarity
			hasBest = true
		}
	}
	if !hasBest {
		return modeltypology.SBTILegacyOutcome{}, 0, fmt.Errorf("no valid sbti outcome patterns configured")
	}
	return best, bestScore, nil
}

func sbtiResultDetailFromOutcome(
	model *modeltypology.SBTILegacyModel,
	outcome modeltypology.SBTILegacyOutcome,
	dimensions []SBTIDimensionResult,
	similarity float64,
	trigger string,
) SBTIResultDetail {
	return SBTIResultDetail{
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

func sbtiFallbackThreshold(model *modeltypology.SBTILegacyModel) float64 {
	if model == nil || model.FallbackSimilarityThreshold <= 0 {
		return 0.6
	}
	return model.FallbackSimilarityThreshold
}

func sbtiPatternLevels(pattern string) []string {
	compact := strings.ReplaceAll(pattern, "-", "")
	levels := make([]string, 0, len(compact))
	for _, r := range compact {
		levels = append(levels, strings.ToUpper(string(r)))
	}
	return levels
}

func sbtiLevelValue(level string) int {
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

func findSBTIOutcome(outcomes []modeltypology.SBTILegacyOutcome, code string) (modeltypology.SBTILegacyOutcome, bool) {
	for _, outcome := range outcomes {
		if outcome.Code == code {
			return outcome, true
		}
	}
	return modeltypology.SBTILegacyOutcome{}, false
}
