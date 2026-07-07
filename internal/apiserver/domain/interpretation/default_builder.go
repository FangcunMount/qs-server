package interpretation

import "context"

func (b *DefaultReportBuilder) Build(input GenerateReportInput) (*InterpretReport, error) {
	if input.AssessmentID.IsZero() {
		return nil, ErrInvalidArgument
	}

	conclusion := b.buildConclusion(input)
	dimensions := b.buildDimensions(input)
	suggestions := b.buildSuggestions(context.Background(), input, dimensions)

	return interpretReportDraft{
		assessmentID: input.AssessmentID,
		modelName:    input.ModelName,
		modelCode:    input.ModelCode,
		totalScore:   input.TotalScore,
		riskLevel:    input.RiskLevel,
		conclusion:   conclusion,
		dimensions:   dimensions,
		suggestions:  suggestions,
	}.build(), nil
}

func (b *DefaultReportBuilder) buildConclusion(input GenerateReportInput) string {
	for _, fs := range input.FactorScores {
		if fs.IsTotalScore && fs.Description != "" {
			return fs.Description
		}
	}
	if input.Conclusion != "" {
		return input.Conclusion
	}
	return ""
}

func (b *DefaultReportBuilder) buildDimensions(input GenerateReportInput) []DimensionInterpret {
	if len(input.FactorScores) == 0 {
		return nil
	}

	dimensions := make([]DimensionInterpret, 0, len(input.FactorScores))
	for _, fs := range input.FactorScores {
		dim := NewDimensionInterpret(
			fs.FactorCode,
			fs.FactorName,
			fs.RawScore,
			fs.MaxScore,
			fs.RiskLevel,
			fs.Description,
			fs.Suggestion,
		)
		if fs.Role != "" || fs.ParentCode != "" || fs.HierarchyLevel > 0 || fs.SortOrder > 0 {
			dim = dim.WithHierarchy(fs.Role, fs.ParentCode, fs.HierarchyLevel, fs.SortOrder)
		}
		dimensions = append(dimensions, dim)
	}
	return dimensions
}

func (b *DefaultReportBuilder) buildSuggestions(
	ctx context.Context,
	input GenerateReportInput,
	dimensions []DimensionInterpret,
) []Suggestion {
	var allSuggestions []Suggestion

	factorStrategy := NewFactorInterpretationSuggestionStrategy(input)
	if factorStrategy.CanHandle(nil) {
		factorSuggestions, err := factorStrategy.GenerateSuggestions(ctx, nil)
		if err == nil {
			allSuggestions = append(allSuggestions, factorSuggestions...)
		}
	}

	if b.suggestionGenerator != nil {
		tempReport := interpretReportDraft{
			assessmentID: input.AssessmentID,
			modelName:    input.ModelName,
			modelCode:    input.ModelCode,
			totalScore:   input.TotalScore,
			riskLevel:    input.RiskLevel,
			conclusion:   b.buildConclusion(input),
			dimensions:   dimensions,
		}.build()

		generatedSuggestions, err := b.suggestionGenerator.Generate(ctx, tempReport)
		if err == nil {
			allSuggestions = append(allSuggestions, generatedSuggestions...)
		}
	}

	return uniqueSuggestions(allSuggestions)
}
