package interpretation

import "context"

// SuggestionStrategy 建议生成策略接口。
type SuggestionStrategy interface {
	Name() string
	CanHandle(report *InterpretReport) bool
	GenerateSuggestions(ctx context.Context, report *InterpretReport) ([]Suggestion, error)
}

// FactorInterpretationSuggestionStrategy 基于因子解读配置的建议策略。
type FactorInterpretationSuggestionStrategy struct {
	input GenerateReportInput
}

// NewFactorInterpretationSuggestionStrategy 创建基于因子解读配置的建议策略。
func NewFactorInterpretationSuggestionStrategy(input GenerateReportInput) *FactorInterpretationSuggestionStrategy {
	return &FactorInterpretationSuggestionStrategy{input: input}
}

func (s *FactorInterpretationSuggestionStrategy) Name() string {
	return "factor_interpretation_strategy"
}

func (s *FactorInterpretationSuggestionStrategy) CanHandle(_ *InterpretReport) bool {
	return len(s.input.FactorScores) > 0
}

func (s *FactorInterpretationSuggestionStrategy) GenerateSuggestions(_ context.Context, _ *InterpretReport) ([]Suggestion, error) {
	if len(s.input.FactorScores) == 0 {
		return []Suggestion{}, nil
	}

	var suggestions []Suggestion

	if s.input.Suggestion != "" {
		suggestions = append(suggestions, Suggestion{
			Category: SuggestionCategoryGeneral,
			Content:  s.input.Suggestion,
		})
	}

	for _, fs := range s.input.FactorScores {
		if fs.Suggestion == "" {
			continue
		}
		if fs.IsTotalScore {
			if fs.Suggestion != s.input.Suggestion {
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
