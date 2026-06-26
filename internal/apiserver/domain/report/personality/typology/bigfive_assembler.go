package typology

import (
	"fmt"
	"strings"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	reportpersonality "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/personality"
)

type BigFiveReportInput struct {
	AssessmentID domainreport.ID
	ModelCode    string
	TotalScore   float64
	RiskLevel    domainreport.RiskLevel
	Detail       BigFiveReportDetail
}

// BuildBigFiveReport 组装 Big Five trait-profile 解读报告。
func BuildBigFiveReport(input BigFiveReportInput) (*domainreport.InterpretReport, error) {
	if input.AssessmentID.IsZero() {
		return nil, fmt.Errorf("assessment is required")
	}
	detail := input.Detail
	profile := bigFivePersonalityProfile(detail)
	return reportpersonality.Build(reportpersonality.Input{
		AssessmentID: input.AssessmentID,
		ModelCode:    input.ModelCode,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Profile:      profile,
		Conclusion:   bigFiveReportConclusion(detail),
		Dimensions:   bigFiveReportDimensions(detail),
		Suggestions:  bigFiveReportSuggestions(detail),
	}), nil
}

func bigFivePersonalityProfile(detail BigFiveReportDetail) reportpersonality.Profile {
	return reportpersonality.Profile{
		Kind:             "bigfive",
		DefaultModelName: "Big Five 五大人格特质测评",
		DefaultModelCode: "BIGFIVE_V1",
		TypeName:         "五大人格特质",
		OneLiner:         "基于各维度原始分展示人格特质分布",
		Commentary:       bigFiveTraitSummary(detail),
	}
}

func bigFiveReportConclusion(detail BigFiveReportDetail) string {
	summary := bigFiveTraitSummary(detail)
	if summary == "" {
		return "五大人格特质画像"
	}
	return "五大人格特质画像 - " + summary
}

func bigFiveTraitSummary(detail BigFiveReportDetail) string {
	if len(detail.Traits) == 0 {
		return ""
	}
	parts := make([]string, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		label := strings.TrimSpace(trait.Name)
		if label == "" {
			label = trait.Code
		}
		parts = append(parts, fmt.Sprintf("%s %.0f", label, trait.RawScore))
	}
	return strings.Join(parts, " / ")
}

func bigFiveReportDimensions(detail BigFiveReportDetail) []domainreport.DimensionInterpret {
	if len(detail.Traits) == 0 {
		return nil
	}
	dimensions := make([]domainreport.DimensionInterpret, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		label := strings.TrimSpace(trait.Name)
		if label == "" {
			label = trait.Code
		}
		description := fmt.Sprintf("%s：原始分 %.0f", label, trait.RawScore)
		dimensions = append(dimensions, domainreport.NewNeutralDimensionInterpret(
			domainreport.NewDimensionCode(trait.Code),
			domainreport.DimensionKindTrait,
			label,
			trait.RawScore,
			nil,
			nil,
			description,
			"",
		))
	}
	return dimensions
}

func bigFiveReportSuggestions(detail BigFiveReportDetail) []domainreport.Suggestion {
	suggestions := make([]domainreport.Suggestion, 0, 2)
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
	if summary := bigFiveTraitSummary(detail); summary != "" {
		add("特质分布：" + summary)
	}
	if detail.Source.Attribution != "" {
		add(fmt.Sprintf("来源与授权：%s；License: %s；非商业使用: %t。",
			detail.Source.Attribution, detail.Source.License, detail.Source.NonCommercial))
	}
	return suggestions
}
