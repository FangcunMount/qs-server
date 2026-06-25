package mbti

import (
	"fmt"
	"strings"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	reportpersonality "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/personality"
)

type ReportInput struct {
	AssessmentID domainreport.ID
	ModelCode    string
	TotalScore   float64
	RiskLevel    domainreport.RiskLevel
	Detail       ReportDetail
}

func BuildReport(input ReportInput) (*domainreport.InterpretReport, error) {
	if input.AssessmentID.IsZero() {
		return nil, fmt.Errorf("assessment is required")
	}
	detail := input.Detail
	profile := personalityProfile(detail)
	return reportpersonality.Build(reportpersonality.Input{
		AssessmentID: input.AssessmentID,
		ModelCode:    input.ModelCode,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Profile:      profile,
		Conclusion:   profile.Conclusion(""),
		Dimensions:   reportDimensions(detail),
		Suggestions:  reportSuggestions(detail),
	}), nil
}

func personalityProfile(detail ReportDetail) reportpersonality.Profile {
	return reportpersonality.Profile{
		Kind:             "mbti",
		DefaultModelName: "MBTI 人格类型测评",
		DefaultModelCode: "MBTI_OEJTS",
		TypeCode:         detail.TypeCode,
		TypeName:         detail.TypeName,
		OneLiner:         detail.OneLiner,
		ImageURL:         detail.ImageURL,
		MatchPercent:     detail.MatchPercent,
		Commentary:       detail.Profile.Summary,
	}
}

func reportDimensions(detail ReportDetail) []domainreport.DimensionInterpret {
	if len(detail.Dimensions) == 0 {
		return nil
	}
	maxScore := 40.0
	dimensions := make([]domainreport.DimensionInterpret, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		description := fmt.Sprintf("%s：倾向 %s（原始分 %.0f，偏好强度 %.0f%%）",
			dim.Name, dim.Preference, dim.RawScore, dim.Strength)
		dimensions = append(dimensions, domainreport.NewDimensionInterpret(
			domainreport.FactorCode(dim.Code),
			dim.Name,
			dim.RawScore,
			&maxScore,
			domainreport.RiskLevelNone,
			description,
			"",
		))
	}
	return dimensions
}

func reportSuggestions(detail ReportDetail) []domainreport.Suggestion {
	suggestions := make([]domainreport.Suggestion, 0, 8)
	add := func(content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		suggestions = append(suggestions, domainreport.Suggestion{
			Category: domainreport.SuggestionCategoryGeneral,
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
