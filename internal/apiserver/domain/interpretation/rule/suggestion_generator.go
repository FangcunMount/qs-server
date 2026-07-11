package rule

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

// SuggestionGenerator 建议生成器接口。
type SuggestionGenerator interface {
	Generate(ctx context.Context, content report.Content) ([]Suggestion, error)
}

// RuleBasedSuggestionGenerator 基于规则的建议生成器。
type RuleBasedSuggestionGenerator struct {
	strategies []SuggestionStrategy
}

// NewRuleBasedSuggestionGenerator 创建规则建议生成器。
func NewRuleBasedSuggestionGenerator(strategies ...SuggestionStrategy) *RuleBasedSuggestionGenerator {
	return &RuleBasedSuggestionGenerator{
		strategies: strategies,
	}
}

func (g *RuleBasedSuggestionGenerator) Generate(ctx context.Context, content report.Content) ([]Suggestion, error) {
	var allSuggestions []Suggestion

	for _, strategy := range g.strategies {
		if strategy.CanHandle(content) {
			suggestions, err := strategy.GenerateSuggestions(ctx, content)
			if err != nil {
				continue
			}
			allSuggestions = append(allSuggestions, suggestions...)
		}
	}

	return uniqueSuggestions(allSuggestions), nil
}

func uniqueSuggestions(suggestions []Suggestion) []Suggestion {
	type key struct {
		category SuggestionCategory
		content  string
		factor   string
	}
	seen := make(map[key]bool)
	var result []Suggestion
	for _, s := range suggestions {
		k := key{
			category: s.Category,
			content:  s.Content,
		}
		if s.FactorCode != nil {
			k.factor = s.FactorCode.String()
		}
		if s.Content == "" {
			continue
		}
		if !seen[k] {
			seen[k] = true
			result = append(result, s)
		}
	}
	return result
}
