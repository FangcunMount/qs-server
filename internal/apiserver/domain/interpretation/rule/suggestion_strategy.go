package rule

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

// SuggestionStrategy 建议生成策略接口。
type SuggestionStrategy interface {
	Name() string
	CanHandle(content report.Content) bool
	GenerateSuggestions(ctx context.Context, content report.Content) ([]Suggestion, error)
}

// FactorInterpretationSuggestionStrategy 基于因子解读配置的建议策略。
type FactorInterpretationSuggestionStrategy struct {
	suggestion   string
	factorScores []report.FactorScoreInput
}

// NewFactorInterpretationSuggestionStrategy 创建基于因子解读配置的建议策略。
func NewFactorInterpretationSuggestionStrategy(suggestion string, factorScores []report.FactorScoreInput) *FactorInterpretationSuggestionStrategy {
	return &FactorInterpretationSuggestionStrategy{suggestion: suggestion, factorScores: factorScores}
}

func (s *FactorInterpretationSuggestionStrategy) Name() string {
	return "factor_interpretation_strategy"
}

func (s *FactorInterpretationSuggestionStrategy) CanHandle(_ report.Content) bool {
	return len(s.factorScores) > 0
}

func (s *FactorInterpretationSuggestionStrategy) GenerateSuggestions(_ context.Context, _ report.Content) ([]Suggestion, error) {
	if len(s.factorScores) == 0 {
		return []Suggestion{}, nil
	}

	var suggestions []Suggestion

	if s.suggestion != "" {
		suggestions = append(suggestions, Suggestion{
			Category: SuggestionCategoryGeneral,
			Content:  s.suggestion,
		})
	}

	for _, fs := range s.factorScores {
		if fs.Suggestion == "" {
			continue
		}
		if fs.IsTotalScore {
			if fs.Suggestion != s.suggestion {
				suggestions = append(suggestions, Suggestion{
					Category: SuggestionCategoryGeneral,
					Content:  fs.Suggestion,
				})
			}
			continue
		}
		factorCode := fs.FactorCode
		suggestions = append(suggestions, Suggestion{
			Category:   SuggestionCategoryDimension,
			Content:    fs.Suggestion,
			FactorCode: &factorCode,
		})
	}

	return suggestions, nil
}
