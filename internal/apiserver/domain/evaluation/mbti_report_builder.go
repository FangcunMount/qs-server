package evaluation

import (
	"fmt"
	"strings"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

type MBTIReportInput struct {
	AssessmentID domainReport.ID
	ModelCode    string
	TotalScore   float64
	RiskLevel    domainReport.RiskLevel
	Detail       MBTIResultDetail
}

func BuildMBTIReport(input MBTIReportInput) (*domainReport.InterpretReport, error) {
	if input.AssessmentID.IsZero() {
		return nil, fmt.Errorf("assessment is required")
	}
	detail := input.Detail
	return domainReport.NewInterpretReport(
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

func MBTIResultDetailFromPayload(payload any) (MBTIResultDetail, error) {
	switch detail := payload.(type) {
	case MBTIResultDetail:
		return detail, nil
	case *MBTIResultDetail:
		if detail == nil {
			return MBTIResultDetail{}, fmt.Errorf("mbti result detail is nil")
		}
		return *detail, nil
	default:
		return MBTIResultDetail{}, fmt.Errorf("unsupported mbti result detail payload: %T", payload)
	}
}

func mbtiReportModelName(detail MBTIResultDetail) string {
	if detail.TypeName == "" {
		return "MBTI 人格类型测评"
	}
	return "MBTI 人格类型测评 - " + detail.TypeName
}

func mbtiReportModelCode(modelCode string, detail MBTIResultDetail) string {
	if modelCode != "" {
		return modelCode
	}
	if detail.TypeCode != "" {
		return detail.TypeCode
	}
	return "MBTI_OEJTS"
}

func mbtiReportConclusion(detail MBTIResultDetail) string {
	title := strings.TrimSpace(detail.TypeCode + " " + detail.TypeName)
	if detail.OneLiner != "" {
		title += " - " + detail.OneLiner
	}
	return strings.TrimSpace(title)
}

func mbtiReportDimensions(detail MBTIResultDetail) []domainReport.DimensionInterpret {
	if len(detail.Dimensions) == 0 {
		return nil
	}
	maxScore := 40.0
	dimensions := make([]domainReport.DimensionInterpret, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		description := fmt.Sprintf("%s：倾向 %s（原始分 %.0f，偏好强度 %.0f%%）",
			dim.Name, dim.Preference, dim.RawScore, dim.Strength)
		dimensions = append(dimensions, domainReport.NewDimensionInterpret(
			domainReport.FactorCode(dim.Code),
			dim.Name,
			dim.RawScore,
			&maxScore,
			domainReport.RiskLevelNone,
			description,
			"",
		))
	}
	return dimensions
}

func mbtiReportSuggestions(detail MBTIResultDetail) []domainReport.Suggestion {
	suggestions := make([]domainReport.Suggestion, 0, 8)
	add := func(content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		suggestions = append(suggestions, domainReport.Suggestion{
			Category: domainReport.SuggestionCategoryGeneral,
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

func mbtiReportModelExtra(detail MBTIResultDetail) *domainReport.ModelExtra {
	extra := &domainReport.ModelExtra{
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
