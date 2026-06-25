package report

import (
	"fmt"
	"strings"
)

type MBTIReportInput struct {
	AssessmentID ID
	ModelCode    string
	TotalScore   float64
	RiskLevel    RiskLevel
	Detail       MBTIReportDetail
}

func BuildMBTIReport(input MBTIReportInput) (*InterpretReport, error) {
	if input.AssessmentID.IsZero() {
		return nil, fmt.Errorf("assessment is required")
	}
	detail := input.Detail
	return NewInterpretReport(
		input.AssessmentID,
		mbtiReportModelName(detail),
		mbtiReportModelCode(input.ModelCode, detail),
		input.TotalScore,
		input.RiskLevel,
		mbtiReportConclusion(detail),
		mbtiReportDimensions(detail),
		mbtiReportSuggestions(detail),
		mbtiReportModelExtra(detail),
	), nil
}

func mbtiReportModelName(detail MBTIReportDetail) string {
	if detail.TypeName == "" {
		return "MBTI 人格类型测评"
	}
	return "MBTI 人格类型测评 - " + detail.TypeName
}

func mbtiReportModelCode(modelCode string, detail MBTIReportDetail) string {
	if modelCode != "" {
		return modelCode
	}
	if detail.TypeCode != "" {
		return detail.TypeCode
	}
	return "MBTI_OEJTS"
}

func mbtiReportConclusion(detail MBTIReportDetail) string {
	title := strings.TrimSpace(detail.TypeCode + " " + detail.TypeName)
	if detail.OneLiner != "" {
		title += " - " + detail.OneLiner
	}
	return strings.TrimSpace(title)
}

func mbtiReportDimensions(detail MBTIReportDetail) []DimensionInterpret {
	if len(detail.Dimensions) == 0 {
		return nil
	}
	maxScore := 40.0
	dimensions := make([]DimensionInterpret, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		description := fmt.Sprintf("%s：倾向 %s（原始分 %.0f，偏好强度 %.0f%%）",
			dim.Name, dim.Preference, dim.RawScore, dim.Strength)
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

func mbtiReportSuggestions(detail MBTIReportDetail) []Suggestion {
	suggestions := make([]Suggestion, 0, 8)
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
	add(detail.Profile.Summary)
	for _, s := range detail.Profile.Strengths {
		add("优势：" + s)
	}
	for _, s := range detail.Profile.Weaknesses {
		add("注意：" + s)
	}
	for _, s := range detail.Profile.Suggestions {
		add("建议：" + s)
	}
	if detail.Source.Attribution != "" {
		add(fmt.Sprintf("来源与授权：%s；License: %s；非商业使用: %t。",
			detail.Source.Attribution, detail.Source.License, detail.Source.NonCommercial))
	}
	return suggestions
}

func mbtiReportModelExtra(detail MBTIReportDetail) *ModelExtra {
	extra := &ModelExtra{
		Kind:         "mbti",
		TypeCode:     detail.TypeCode,
		TypeName:     detail.TypeName,
		OneLiner:     detail.OneLiner,
		ImageURL:     detail.ImageURL,
		MatchPercent: detail.MatchPercent,
		Commentary:   detail.Profile.Summary,
	}
	if extra.IsEmpty() {
		return nil
	}
	return extra
}
