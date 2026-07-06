package assessment

import (
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

// toReportResult 将领域模型转换为 ReportResult
func toReportResult(r *report.InterpretReport) *ReportResult {
	if r == nil {
		return nil
	}

	dimensions := make([]DimensionResult, len(r.Dimensions()))
	for i, d := range r.Dimensions() {
		dimensions[i] = DimensionResult{
			FactorCode:  d.Code().String(),
			FactorName:  d.Name(),
			RawScore:    d.RawScore(),
			MaxScore:    d.MaxScore(),
			RiskLevel:   d.Severity(),
			Description: d.Description(),
			Suggestion:  d.Suggestion(),
		}
	}

	return &ReportResult{
		AssessmentID: r.ID().Uint64(),
		ModelName:    r.ModelName(),
		ModelCode:    r.ModelCode(),
		TotalScore:   r.TotalScore(),
		RiskLevel:    string(r.RiskLevel()),
		Conclusion:   r.Conclusion(),
		Dimensions:   dimensions,
		Suggestions:  toSuggestionDTOs(r.Suggestions()),
		CreatedAt:    r.CreatedAt(),
		ModelExtra:   toModelExtraResult(r.ModelExtra()),
	}
}

func toModelExtraResult(extra *report.ModelExtra) *ModelExtraResult {
	if extra == nil || extra.IsEmpty() {
		return nil
	}
	result := &ModelExtraResult{
		Kind:           extra.Kind,
		TypeCode:       extra.TypeCode,
		TypeName:       extra.TypeName,
		OneLiner:       extra.OneLiner,
		ImageURL:       extra.ImageURL,
		MatchPercent:   extra.MatchPercent,
		IsSpecial:      extra.IsSpecial,
		SpecialTrigger: extra.SpecialTrigger,
		Commentary:     extra.Commentary,
	}
	if extra.Rarity != nil {
		result.Rarity = &ModelRarityResult{
			Percent: extra.Rarity.Percent,
			Label:   extra.Rarity.Label,
			OneInX:  extra.Rarity.OneInX,
		}
	}
	return result
}

func reportRowToResult(row evaluationreadmodel.ReportRow) *ReportResult {
	dimensions := make([]DimensionResult, 0, len(row.Dimensions))
	for _, d := range row.Dimensions {
		dimensions = append(dimensions, DimensionResult{
			FactorCode:  d.FactorCode,
			FactorName:  d.FactorName,
			RawScore:    d.RawScore,
			MaxScore:    d.MaxScore,
			RiskLevel:   d.RiskLevel,
			Description: d.Description,
			Suggestion:  d.Suggestion,
		})
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

func toSuggestionDTOs(items []report.Suggestion) []SuggestionDTO {
	if len(items) == 0 {
		return nil
	}
	result := make([]SuggestionDTO, len(items))
	for i, s := range items {
		var fc *string
		if s.FactorCode != nil {
			code := s.FactorCode.String()
			fc = &code
		}
		result[i] = SuggestionDTO{
			Category:   string(s.Category),
			Content:    s.Content,
			FactorCode: fc,
		}
	}
	return result
}
