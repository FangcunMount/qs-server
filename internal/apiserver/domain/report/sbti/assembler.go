package sbti

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
		Conclusion:   profile.Conclusion(reportConclusionSuffix(detail)),
		Dimensions:   reportDimensions(detail),
		Suggestions:  reportSuggestions(detail),
	}), nil
}

func personalityProfile(detail ReportDetail) reportpersonality.Profile {
	return reportpersonality.Profile{
		Kind:             "sbti",
		DefaultModelName: "SBTI 趣味人格测评",
		DefaultModelCode: "SBTI_FUN",
		TypeCode:         detail.TypeCode,
		TypeName:         detail.TypeName,
		OneLiner:         detail.OneLiner,
		ImageURL:         detail.ImageURL,
		MatchPercent:     detail.Similarity * 100,
		IsSpecial:        detail.Outcome.IsSpecial,
		SpecialTrigger:   detail.SpecialTrigger,
		Rarity:           reportRarity(detail.Rarity),
		Commentary:       detail.Outcome.Commentary,
	}
}

func reportConclusionSuffix(detail ReportDetail) string {
	if detail.Similarity > 0 {
		return fmt.Sprintf("（匹配度 %.0f%%）", detail.Similarity*100)
	}
	return ""
}

func reportDimensions(detail ReportDetail) []domainreport.DimensionInterpret {
	if len(detail.Dimensions) == 0 {
		return nil
	}
	maxScore := 6.0
	dimensions := make([]domainreport.DimensionInterpret, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		description := strings.TrimSpace(fmt.Sprintf("%s：%s 档，原始分 %.0f/6", dim.Name, dim.Level, dim.RawScore))
		if dim.Model != "" {
			description = dim.Model + " / " + description
		}
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
	suggestions := make([]domainreport.Suggestion, 0, 5)
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
	add(detail.Outcome.Commentary)
	if detail.Source.Attribution != "" {
		add(fmt.Sprintf("来源与授权：%s；License: %s；非商业使用: %t。",
			detail.Source.Attribution, detail.Source.License, detail.Source.NonCommercial))
	}
	return suggestions
}

func reportRarity(rarity RarityReport) *domainreport.ModelRarity {
	if rarity.Percent == 0 && rarity.Label == "" && rarity.OneInX == 0 {
		return nil
	}
	return &domainreport.ModelRarity{
		Percent: rarity.Percent,
		Label:   rarity.Label,
		OneInX:  rarity.OneInX,
	}
}
