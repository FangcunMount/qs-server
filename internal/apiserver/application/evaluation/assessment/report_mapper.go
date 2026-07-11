package assessment

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

func reportRowToResult(row evaluationreadmodel.ReportRow) *ReportResult {
	dimensions := make([]DimensionResult, 0, len(row.Dimensions))
	for _, d := range row.Dimensions {
		dimensions = append(dimensions, dimensionResultFromReadRow(d))
	}
	suggestions := make([]SuggestionDTO, 0, len(row.Suggestions))
	for _, s := range row.Suggestions {
		suggestions = append(suggestions, SuggestionDTO{
			Category:   s.Category,
			Content:    s.Content,
			FactorCode: s.FactorCode,
		})
	}
	return &ReportResult{
		AssessmentID: row.AssessmentID,
		ModelName:    row.ModelName,
		ModelCode:    row.ModelCode,
		TotalScore:   row.TotalScore,
		RiskLevel:    row.RiskLevel,
		Conclusion:   row.Conclusion,
		Dimensions:   dimensions,
		Suggestions:  suggestions,
		ModelExtra:   reportModelExtraRowToResult(row.ModelExtra),
		CreatedAt:    row.CreatedAt,
	}
}

func reportModelExtraRowToResult(row *evaluationreadmodel.ReportModelExtraRow) *ModelExtraResult {
	if row == nil {
		return nil
	}
	result := &ModelExtraResult{
		Kind:           row.Kind,
		TypeCode:       row.TypeCode,
		TypeName:       row.TypeName,
		OneLiner:       row.OneLiner,
		ImageURL:       row.ImageURL,
		MatchPercent:   row.MatchPercent,
		IsSpecial:      row.IsSpecial,
		SpecialTrigger: row.SpecialTrigger,
		Commentary:     row.Commentary,
	}
	if row.Rarity != nil {
		result.Rarity = &ModelRarityResult{
			Percent: row.Rarity.Percent,
			Label:   row.Rarity.Label,
			OneInX:  row.Rarity.OneInX,
		}
	}
	return result
}

func dimensionResultFromReadRow(d evaluationreadmodel.ReportDimensionRow) DimensionResult {
	return DimensionResult{
		FactorCode:     d.FactorCode,
		FactorName:     d.FactorName,
		RawScore:       d.RawScore,
		MaxScore:       d.MaxScore,
		RiskLevel:      d.RiskLevel,
		Role:           d.Role,
		ParentCode:     d.ParentCode,
		HierarchyLevel: d.HierarchyLevel,
		SortOrder:      d.SortOrder,
		Description:    d.Description,
		Suggestion:     d.Suggestion,
	}
}
