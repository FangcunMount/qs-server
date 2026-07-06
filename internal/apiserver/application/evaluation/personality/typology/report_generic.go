package typology

import (
	"fmt"
	"strings"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	reportpersonality "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/personality"
)

func buildPersonalityTypeReport(outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	if outcome.Assessment == nil {
		return nil, errAssessmentRequired
	}
	if outcome.Execution == nil {
		return nil, errEvaluationOutcomeRequired
	}
	detail, err := evaluationtypology.PersonalityTypeDetailFromPayload(outcome.Execution.Detail.Payload)
	if err != nil {
		return nil, err
	}
	profile := reportpersonality.Profile{
		Kind:             "personality_type",
		DefaultModelName: "人格类型测评",
		DefaultModelCode: "PERSONALITY_TYPE",
		TypeCode:         detail.TypeCode,
		TypeName:         detail.TypeName,
		OneLiner:         detail.OneLiner,
		ImageURL:         detail.ImageURL,
		MatchPercent:     detail.MatchPercent,
		IsSpecial:        detail.IsSpecial,
		SpecialTrigger:   detail.SpecialTrigger,
		Rarity:           genericReportRarity(detail.Rarity),
		Commentary:       firstNonEmpty(detail.Commentary, detail.Summary),
	}
	return reportpersonality.Build(reportpersonality.Input{
		AssessmentID: domainReport.ID(outcome.Assessment.ID()),
		ModelCode:    typologyModelCode(outcome),
		TotalScore:   typologyTotalScore(outcome.Execution),
		RiskLevel:    typologyRiskLevel(outcome.Execution),
		Profile:      profile,
		Conclusion:   profile.Conclusion(""),
		Dimensions:   personalityTypeReportDimensions(detail),
		Suggestions:  personalityTypeReportSuggestions(detail),
	}), nil
}

func buildTraitProfileReport(outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	if outcome.Assessment == nil {
		return nil, errAssessmentRequired
	}
	if outcome.Execution == nil {
		return nil, errEvaluationOutcomeRequired
	}
	detail, err := evaluationtypology.TraitProfileDetailFromPayload(outcome.Execution.Detail.Payload)
	if err != nil {
		return nil, err
	}
	profile := reportpersonality.Profile{
		Kind:             "trait_profile",
		DefaultModelName: "人格特质画像",
		DefaultModelCode: "TRAIT_PROFILE",
		TypeName:         "人格特质画像",
		OneLiner:         "基于各因子原始分展示人格特质分布",
		Commentary:       traitProfileSummary(detail),
	}
	conclusion := "人格特质画像"
	if summary := traitProfileSummary(detail); summary != "" {
		conclusion += " - " + summary
	}
	return reportpersonality.Build(reportpersonality.Input{
		AssessmentID: domainReport.ID(outcome.Assessment.ID()),
		ModelCode:    typologyModelCode(outcome),
		TotalScore:   typologyTotalScore(outcome.Execution),
		RiskLevel:    typologyRiskLevel(outcome.Execution),
		Profile:      profile,
		Conclusion:   conclusion,
		Dimensions:   traitProfileReportDimensions(detail),
		Suggestions:  traitProfileReportSuggestions(detail),
	}), nil
}

func personalityTypeReportDimensions(detail evaluationtypology.PersonalityTypeDetail) []domainReport.DimensionInterpret {
	if len(detail.Dimensions) == 0 {
		return nil
	}
	dimensions := make([]domainReport.DimensionInterpret, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		name := firstNonEmpty(dim.Name, dim.Code)
		kind := domainReport.DimensionKindFactor
		if dim.Preference != "" {
			kind = domainReport.DimensionKindPole
		}
		description := personalityDimensionDescription(dim)
		dimensions = append(dimensions, domainReport.NewNeutralDimensionInterpret(
			domainReport.NewDimensionCode(dim.Code),
			kind,
			name,
			dim.RawScore,
			nil,
			nil,
			description,
			"",
		))
	}
	return dimensions
}

func personalityDimensionDescription(dim evaluationtypology.PersonalityDimensionResult) string {
	name := firstNonEmpty(dim.Name, dim.Code)
	switch {
	case dim.Preference != "":
		return fmt.Sprintf("%s: preference %s, raw %.0f, strength %.0f%%", name, dim.Preference, dim.RawScore, dim.Strength)
	case dim.Level != "":
		return fmt.Sprintf("%s: level %s, raw %.0f", name, dim.Level, dim.RawScore)
	default:
		return fmt.Sprintf("%s: raw %.0f", name, dim.RawScore)
	}
}

func personalityTypeReportSuggestions(detail evaluationtypology.PersonalityTypeDetail) []domainReport.Suggestion {
	suggestions := make([]domainReport.Suggestion, 0, len(detail.Strengths)+len(detail.Weaknesses)+len(detail.Suggestions)+2)
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
	add(detail.Summary)
	add(detail.Commentary)
	for _, s := range detail.Strengths {
		add("优势：" + s)
	}
	for _, s := range detail.Weaknesses {
		add("注意：" + s)
	}
	for _, s := range detail.Suggestions {
		add("建议：" + s)
	}
	return suggestions
}

func traitProfileSummary(detail evaluationtypology.TraitProfileDetail) string {
	if len(detail.Traits) == 0 {
		return ""
	}
	parts := make([]string, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		label := firstNonEmpty(trait.Name, trait.Code)
		parts = append(parts, fmt.Sprintf("%s %.0f", label, trait.RawScore))
	}
	return strings.Join(parts, " / ")
}

func traitProfileReportDimensions(detail evaluationtypology.TraitProfileDetail) []domainReport.DimensionInterpret {
	if len(detail.Traits) == 0 {
		return nil
	}
	dimensions := make([]domainReport.DimensionInterpret, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		label := firstNonEmpty(trait.Name, trait.Code)
		description := fmt.Sprintf("%s: raw %.0f", label, trait.RawScore)
		dimensions = append(dimensions, domainReport.NewNeutralDimensionInterpret(
			domainReport.NewDimensionCode(trait.Code),
			domainReport.DimensionKindTrait,
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

func traitProfileReportSuggestions(detail evaluationtypology.TraitProfileDetail) []domainReport.Suggestion {
	summary := traitProfileSummary(detail)
	if summary == "" {
		return nil
	}
	return []domainReport.Suggestion{{
		Category: domainReport.SuggestionCategoryGeneral,
		Content:  "特质分布：" + summary,
	}}
}

func genericReportRarity(rarity struct {
	Percent float64 `json:"percent,omitempty"`
	Label   string  `json:"label,omitempty"`
	OneInX  int     `json:"one_in_x,omitempty"`
}) *domainReport.ModelRarity {
	if rarity.Percent == 0 && rarity.Label == "" && rarity.OneInX == 0 {
		return nil
	}
	return &domainReport.ModelRarity{
		Percent: rarity.Percent,
		Label:   rarity.Label,
		OneInX:  rarity.OneInX,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
