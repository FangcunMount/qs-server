package report

import (
	"fmt"
	"strings"
)

type SBTIReportInput struct {
	AssessmentID ID
	ModelCode    string
	TotalScore   float64
	RiskLevel    RiskLevel
	Detail       SBTIReportDetail
}

func BuildSBTIReport(input SBTIReportInput) (*InterpretReport, error) {
	if input.AssessmentID.IsZero() {
		return nil, fmt.Errorf("assessment is required")
	}
	detail := input.Detail
	return NewInterpretReport(
		input.AssessmentID,
		sbtiReportModelName(detail),
		sbtiReportModelCode(input.ModelCode, detail),
		input.TotalScore,
		input.RiskLevel,
		sbtiReportConclusion(detail),
		sbtiReportDimensions(detail),
		sbtiReportSuggestions(detail),
		sbtiReportModelExtra(detail),
	), nil
}

func sbtiReportModelName(detail SBTIReportDetail) string {
	if detail.TypeName == "" {
		return "SBTI 趣味人格测评"
	}
	return "SBTI 趣味人格测评 - " + detail.TypeName
}

func sbtiReportModelCode(modelCode string, detail SBTIReportDetail) string {
	if modelCode != "" {
		return modelCode
	}
	if detail.TypeCode != "" {
		return detail.TypeCode
	}
	return "SBTI_FUN"
}

func sbtiReportConclusion(detail SBTIReportDetail) string {
	title := strings.TrimSpace(detail.TypeCode + " " + detail.TypeName)
	if detail.OneLiner != "" {
		title += " - " + detail.OneLiner
	}
	if detail.Similarity > 0 {
		title += fmt.Sprintf("（匹配度 %.0f%%）", detail.Similarity*100)
	}
	return strings.TrimSpace(title)
}

func sbtiReportDimensions(detail SBTIReportDetail) []DimensionInterpret {
	if len(detail.Dimensions) == 0 {
		return nil
	}
	maxScore := 6.0
	dimensions := make([]DimensionInterpret, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		description := strings.TrimSpace(fmt.Sprintf("%s：%s 档，原始分 %.0f/6", dim.Name, dim.Level, dim.RawScore))
		if dim.Model != "" {
			description = dim.Model + " / " + description
		}
		dimensions = append(dimensions, NewDimensionInterpret(
			FactorCode(dim.Code),
			dim.Name,
			dim.RawScore,
			&maxScore,
			RiskLevelNone,
			description,
			"",
		))
	}
	return dimensions
}

func sbtiReportSuggestions(detail SBTIReportDetail) []Suggestion {
	suggestions := make([]Suggestion, 0, 5)
	add := func(content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		suggestions = append(suggestions, Suggestion{
			Category: SuggestionCategoryGeneral,
			Content:  content,
		})
	}
	add(detail.Outcome.Commentary)
	if detail.Source.Attribution != "" {
		add(fmt.Sprintf("来源与授权：%s；License: %s；非商业使用: %t。",
			detail.Source.Attribution, detail.Source.License, detail.Source.NonCommercial))
	}
	return suggestions
}

func sbtiReportModelExtra(detail SBTIReportDetail) *ModelExtra {
	extra := &ModelExtra{
		Kind:           "sbti",
		TypeCode:       detail.TypeCode,
		TypeName:       detail.TypeName,
		OneLiner:       detail.OneLiner,
		ImageURL:       detail.ImageURL,
		MatchPercent:   detail.Similarity * 100,
		IsSpecial:      detail.Outcome.IsSpecial,
		SpecialTrigger: detail.SpecialTrigger,
		Commentary:     detail.Outcome.Commentary,
	}
	if detail.Rarity.Percent > 0 || detail.Rarity.Label != "" || detail.Rarity.OneInX > 0 {
		extra.Rarity = &ModelRarity{
			Percent: detail.Rarity.Percent,
			Label:   detail.Rarity.Label,
			OneInX:  detail.Rarity.OneInX,
		}
	}
	if extra.IsEmpty() {
		return nil
	}
	return extra
}
