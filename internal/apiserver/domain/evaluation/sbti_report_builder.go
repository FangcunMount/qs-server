package evaluation

import (
	"fmt"
	"strings"

	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

type SBTIReportInput struct {
	AssessmentID domainReport.ID
	ModelCode    string
	TotalScore   float64
	RiskLevel    domainReport.RiskLevel
	Detail       SBTIResultDetail
}

func BuildSBTIReport(input SBTIReportInput) (*domainReport.InterpretReport, error) {
	if input.AssessmentID.IsZero() {
		return nil, fmt.Errorf("assessment is required")
	}
	detail := input.Detail
	return domainReport.NewInterpretReport(
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

func SBTIResultDetailFromPayload(payload any) (SBTIResultDetail, error) {
	switch detail := payload.(type) {
	case SBTIResultDetail:
		return detail, nil
	case *SBTIResultDetail:
		if detail == nil {
			return SBTIResultDetail{}, fmt.Errorf("sbti result detail is nil")
		}
		return *detail, nil
	default:
		return SBTIResultDetail{}, fmt.Errorf("unsupported sbti result detail payload: %T", payload)
	}
}

func sbtiReportModelName(detail SBTIResultDetail) string {
	if detail.TypeName == "" {
		return "SBTI 趣味人格测评"
	}
	return "SBTI 趣味人格测评 - " + detail.TypeName
}

func sbtiReportModelCode(modelCode string, detail SBTIResultDetail) string {
	if modelCode != "" {
		return modelCode
	}
	if detail.TypeCode != "" {
		return detail.TypeCode
	}
	return "SBTI_FUN"
}

func sbtiReportConclusion(detail SBTIResultDetail) string {
	title := strings.TrimSpace(detail.TypeCode + " " + detail.TypeName)
	if detail.OneLiner != "" {
		title += " - " + detail.OneLiner
	}
	if detail.Similarity > 0 {
		title += fmt.Sprintf("（匹配度 %.0f%%）", detail.Similarity*100)
	}
	return strings.TrimSpace(title)
}

func sbtiReportDimensions(detail SBTIResultDetail) []domainReport.DimensionInterpret {
	if len(detail.Dimensions) == 0 {
		return nil
	}
	maxScore := 6.0
	dimensions := make([]domainReport.DimensionInterpret, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		description := strings.TrimSpace(fmt.Sprintf("%s：%s 档，原始分 %.0f/6", dim.Name, dim.Level, dim.RawScore))
		if dim.Model != "" {
			description = dim.Model + " / " + description
		}
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

func sbtiReportSuggestions(detail SBTIResultDetail) []domainReport.Suggestion {
	suggestions := make([]domainReport.Suggestion, 0, 5)
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
	add(detail.Outcome.Commentary)
	if detail.Source.Attribution != "" {
		add(fmt.Sprintf("来源与授权：%s；License: %s；非商业使用: %t。", detail.Source.Attribution, detail.Source.License, detail.Source.NonCommercial))
	}
	return suggestions
}

func sbtiReportModelExtra(detail SBTIResultDetail) *domainReport.ModelExtra {
	extra := &domainReport.ModelExtra{
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
		extra.Rarity = &domainReport.ModelRarity{
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
