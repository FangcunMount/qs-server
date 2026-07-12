package rule

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"

// CollectConfiguredSuggestions turns frozen outcome/template suggestions into
// the report's structured suggestion list. It performs no runtime lookup.
func CollectConfiguredSuggestions(overall string, factorScores []report.FactorScoreInput) []report.Suggestion {
	if len(factorScores) == 0 {
		return nil
	}
	var suggestions []report.Suggestion
	if overall != "" {
		suggestions = append(suggestions, report.Suggestion{Category: report.SuggestionCategoryGeneral, Content: overall})
	}
	for _, score := range factorScores {
		if score.Suggestion == "" {
			continue
		}
		if score.IsTotalScore {
			if score.Suggestion != overall {
				suggestions = append(suggestions, report.Suggestion{Category: report.SuggestionCategoryGeneral, Content: score.Suggestion})
			}
			continue
		}
		factorCode := score.FactorCode
		suggestions = append(suggestions, report.Suggestion{
			Category:   report.SuggestionCategoryDimension,
			Content:    score.Suggestion,
			FactorCode: &factorCode,
		})
	}
	type key struct {
		category report.SuggestionCategory
		content  string
		factor   string
	}
	seen := make(map[key]struct{}, len(suggestions))
	var result []report.Suggestion
	for _, suggestion := range suggestions {
		itemKey := key{category: suggestion.Category, content: suggestion.Content}
		if suggestion.FactorCode != nil {
			itemKey.factor = suggestion.FactorCode.String()
		}
		if suggestion.Content == "" {
			continue
		}
		if _, exists := seen[itemKey]; exists {
			continue
		}
		seen[itemKey] = struct{}{}
		result = append(result, suggestion)
	}
	return result
}
