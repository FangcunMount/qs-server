package main

import modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"

func traitItemsFromSeed(seed questionnaireSeedFile) []traitItem {
	items := make([]traitItem, 0, len(seed.Questions))
	for _, q := range seed.Questions {
		items = append(items, traitItem{
			Code:    q.Code,
			Factor:  q.Factor,
			Reverse: q.Reverse,
			Title:   q.Stem,
		})
	}
	return items
}

func traitFactorsFromSeed(seed questionnaireSeedFile) []traitFactor {
	factors := make([]traitFactor, 0, len(seed.Factors))
	for _, factor := range seed.Factors {
		factors = append(factors, traitFactor(factor))
	}
	return factors
}

func mbtiDimensionsFromSeed(seed questionnaireSeedFile) map[string]modeltypology.Dimension {
	dimensions := make(map[string]modeltypology.Dimension, len(seed.Dimensions))
	for code, dim := range seed.Dimensions {
		dimensions[code] = modeltypology.Dimension{
			Code:      dim.Code,
			Name:      dim.Name,
			LeftPole:  dim.LeftPole,
			RightPole: dim.RightPole,
			Threshold: dim.Threshold,
		}
	}
	return dimensions
}

func questionMappingsFromSeed(seed questionnaireSeedFile) []modeltypology.QuestionMapping {
	mappings := make([]modeltypology.QuestionMapping, 0, len(seed.Questions))
	for _, q := range seed.Questions {
		var optionScores map[string]float64
		if isForcedChoiceQuestion(q) {
			optionScores = optionScoresFromQuestion(q)
		} else {
			optionScores = optionScoresForItem(q.Reverse)
		}
		mappings = append(mappings, modeltypology.QuestionMapping{
			QuestionCode: q.Code,
			Dimension:    q.Factor,
			Sign:         1,
			OptionScores: optionScores,
		})
	}
	return mappings
}

func isForcedChoiceQuestion(q questionSeed) bool {
	return len(q.Options) == 2 && q.Options[0].Code == "A" && q.Options[1].Code == "B"
}

func optionScoresFromQuestion(q questionSeed) map[string]float64 {
	scores := make(map[string]float64, len(q.Options))
	for _, opt := range q.Options {
		scores[opt.Code] = opt.Score
	}
	return scores
}

func factorOrderFromSeed(seed questionnaireSeedFile) []string {
	if len(seed.Factors) > 0 {
		order := make([]string, 0, len(seed.Factors))
		for _, factor := range seed.Factors {
			order = append(order, factor.Code)
		}
		return order
	}
	if len(seed.Dimensions) > 0 {
		order := []string{"EI", "SN", "TF", "JP"}
		result := make([]string, 0, len(order))
		for _, code := range order {
			if _, ok := seed.Dimensions[code]; ok {
				result = append(result, code)
			}
		}
		return result
	}
	return nil
}
