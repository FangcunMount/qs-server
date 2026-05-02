package assessment

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
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
			FactorCode:  string(d.FactorCode()),
			FactorName:  d.FactorName(),
			RawScore:    d.RawScore(),
			MaxScore:    d.MaxScore(),
			RiskLevel:   string(d.RiskLevel()),
			Description: d.Description(),
			Suggestion:  d.Suggestion(),
		}
	}

	return &ReportResult{
		AssessmentID: r.ID().Uint64(),
		ScaleName:    r.ScaleName(),
		ScaleCode:    r.ScaleCode(),
		TotalScore:   r.TotalScore(),
		RiskLevel:    string(r.RiskLevel()),
		Conclusion:   r.Conclusion(),
		Dimensions:   dimensions,
		Suggestions:  toSuggestionDTOs(r.Suggestions()),
		CreatedAt:    r.CreatedAt(),
	}
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
		ScaleName:    row.ScaleName,
		ScaleCode:    row.ScaleCode,
		TotalScore:   row.TotalScore,
		RiskLevel:    row.RiskLevel,
		Conclusion:   row.Conclusion,
		Dimensions:   dimensions,
		Suggestions:  suggestions,
		CreatedAt:    row.CreatedAt,
	}
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
