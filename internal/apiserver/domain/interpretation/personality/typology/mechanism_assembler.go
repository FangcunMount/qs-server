package typology

import (
	"fmt"
	"strings"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	reportpersonality "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/personality"
)

// PersonalityTypeReportTemplate carries presentation labels for mechanism-oriented reports.
type PersonalityTypeReportTemplate struct {
	Kind                 string
	DefaultModelName     string
	DefaultModelCode     string
	DimensionMaxScore    *float64
	DimensionDescription func(name, preference string, rawScore, strength float64, level, model string) string
	ConclusionSuffix     func(detail PersonalityTypeReportDetail) string
}

// TraitProfileReportTemplate carries presentation labels for trait-profile reports.
type TraitProfileReportTemplate struct {
	Kind             string
	DefaultModelName string
	DefaultModelCode string
	TypeName         string
	OneLiner         string
	ConclusionTitle  string
}

// BuildPersonalityTypeReport assembles a personality-type report from mechanism-neutral detail.
func BuildPersonalityTypeReport(input PersonalityTypeReportInput, tmpl PersonalityTypeReportTemplate) (*domainreport.InterpretReport, error) {
	if input.AssessmentID.IsZero() {
		return nil, fmt.Errorf("assessment is required")
	}
	if tmpl.Kind == "" {
		tmpl.Kind = "personality_type"
	}
	if tmpl.DefaultModelName == "" {
		tmpl.DefaultModelName = "人格类型测评"
	}
	if tmpl.DefaultModelCode == "" {
		tmpl.DefaultModelCode = "PERSONALITY_TYPE"
	}
	detail := input.Detail
	profile := reportpersonality.Profile{
		Kind:             tmpl.Kind,
		DefaultModelName: tmpl.DefaultModelName,
		DefaultModelCode: tmpl.DefaultModelCode,
		TypeCode:         detail.TypeCode,
		TypeName:         detail.TypeName,
		OneLiner:         detail.OneLiner,
		ImageURL:         detail.ImageURL,
		MatchPercent:     detail.MatchPercent,
		IsSpecial:        detail.IsSpecial,
		SpecialTrigger:   detail.SpecialTrigger,
		Rarity:           mechanismReportRarity(detail.Rarity),
		Commentary:       firstNonEmptyMechanism(detail.Profile.Summary, detail.Commentary),
	}
	return reportpersonality.Build(reportpersonality.Input{
		AssessmentID: input.AssessmentID,
		ModelCode:    input.ModelCode,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Profile:      profile,
		Conclusion:   profile.Conclusion(mechanismConclusionSuffix(tmpl, detail)),
		Dimensions:   mechanismPersonalityDimensions(detail, tmpl),
		Suggestions:  mechanismPersonalitySuggestions(detail),
	}), nil
}

func mechanismConclusionSuffix(tmpl PersonalityTypeReportTemplate, detail PersonalityTypeReportDetail) string {
	if tmpl.ConclusionSuffix != nil {
		return tmpl.ConclusionSuffix(detail)
	}
	return ""
}

// BuildTraitProfileReport assembles a trait-profile report from mechanism-neutral detail.
func BuildTraitProfileReport(input TraitProfileReportInput, tmpl TraitProfileReportTemplate) (*domainreport.InterpretReport, error) {
	if input.AssessmentID.IsZero() {
		return nil, fmt.Errorf("assessment is required")
	}
	if tmpl.Kind == "" {
		tmpl.Kind = "trait_profile"
	}
	if tmpl.DefaultModelName == "" {
		tmpl.DefaultModelName = "人格特质画像"
	}
	if tmpl.DefaultModelCode == "" {
		tmpl.DefaultModelCode = "TRAIT_PROFILE"
	}
	if tmpl.TypeName == "" {
		tmpl.TypeName = tmpl.DefaultModelName
	}
	if tmpl.OneLiner == "" {
		tmpl.OneLiner = "基于各因子原始分展示人格特质分布"
	}
	detail := input.Detail
	profile := reportpersonality.Profile{
		Kind:             tmpl.Kind,
		DefaultModelName: tmpl.DefaultModelName,
		DefaultModelCode: tmpl.DefaultModelCode,
		TypeName:         tmpl.TypeName,
		OneLiner:         tmpl.OneLiner,
		Commentary:       mechanismTraitSummary(detail),
	}
	conclusion := tmpl.ConclusionTitle
	if conclusion == "" {
		conclusion = tmpl.TypeName
	}
	if summary := mechanismTraitSummary(detail); summary != "" {
		conclusion += " - " + summary
	}
	return reportpersonality.Build(reportpersonality.Input{
		AssessmentID: input.AssessmentID,
		ModelCode:    input.ModelCode,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Profile:      profile,
		Conclusion:   conclusion,
		Dimensions:   mechanismTraitDimensions(detail),
		Suggestions:  mechanismTraitSuggestions(detail),
	}), nil
}

type PersonalityTypeReportInput struct {
	AssessmentID domainreport.ID
	ModelCode    string
	TotalScore   float64
	RiskLevel    domainreport.RiskLevel
	Detail       PersonalityTypeReportDetail
}

type TraitProfileReportInput struct {
	AssessmentID domainreport.ID
	ModelCode    string
	TotalScore   float64
	RiskLevel    domainreport.RiskLevel
	Detail       TraitProfileReportDetail
}

type PersonalityTypeReportDetail struct {
	TypeCode            string
	TypeName            string
	OneLiner            string
	MatchPercent        float64
	ImageURL            string
	IsSpecial           bool
	SpecialTrigger      string
	Commentary          string
	Profile             PersonalityTypeProfileReport
	Rarity              PersonalityTypeRarityReport
	Dimensions          []PersonalityTypeDimensionReport
	SourceAttribution   string
	SourceLicense       string
	SourceNonCommercial bool
}

type PersonalityTypeProfileReport struct {
	Summary     string
	Strengths   []string
	Weaknesses  []string
	Suggestions []string
}

type PersonalityTypeRarityReport struct {
	Percent float64
	Label   string
	OneInX  int
}

type PersonalityTypeDimensionReport struct {
	Code       string
	Name       string
	LeftPole   string
	RightPole  string
	RawScore   float64
	Preference string
	Strength   float64
	Model      string
	Level      string
}

type TraitProfileReportDetail struct {
	Traits []TraitProfileFactorReport
	Source TraitProfileSourceReport
}

type TraitProfileFactorReport struct {
	Code     string
	Name     string
	RawScore float64
}

type TraitProfileSourceReport struct {
	Attribution   string
	License       string
	NonCommercial bool
}

func mechanismPersonalityDimensions(detail PersonalityTypeReportDetail, tmpl PersonalityTypeReportTemplate) []domainreport.DimensionInterpret {
	if len(detail.Dimensions) == 0 {
		return nil
	}
	dimensions := make([]domainreport.DimensionInterpret, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		name := firstNonEmptyMechanism(dim.Name, dim.Code)
		description := fmt.Sprintf("%s: raw %.0f", name, dim.RawScore)
		if tmpl.DimensionDescription != nil {
			description = tmpl.DimensionDescription(name, dim.Preference, dim.RawScore, dim.Strength, dim.Level, dim.Model)
		} else if dim.Preference != "" {
			description = fmt.Sprintf("%s: preference %s, raw %.0f, strength %.0f%%", name, dim.Preference, dim.RawScore, dim.Strength)
		} else if dim.Level != "" {
			description = fmt.Sprintf("%s: level %s, raw %.0f", name, dim.Level, dim.RawScore)
		}
		var maxScore *float64
		if tmpl.DimensionMaxScore != nil {
			maxScore = tmpl.DimensionMaxScore
		}
		kind := domainreport.DimensionKindFactor
		if dim.Preference != "" {
			kind = domainreport.DimensionKindPole
		}
		if maxScore != nil {
			dimensions = append(dimensions, domainreport.NewDimensionInterpret(
				domainreport.FactorCode(dim.Code), name, dim.RawScore, maxScore, domainreport.RiskLevelNone, description, "",
			))
			continue
		}
		dimensions = append(dimensions, domainreport.NewNeutralDimensionInterpret(
			domainreport.NewDimensionCode(dim.Code), kind, name, dim.RawScore, nil, nil, description, "",
		))
	}
	return dimensions
}

func mechanismPersonalitySuggestions(detail PersonalityTypeReportDetail) []domainreport.Suggestion {
	suggestions := make([]domainreport.Suggestion, 0, 8)
	add := func(content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		suggestions = append(suggestions, domainreport.Suggestion{Category: domainreport.SuggestionCategoryGeneral, Content: content})
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
	if detail.SourceAttribution != "" {
		add(fmt.Sprintf("来源与授权：%s；License: %s；非商业使用: %t。",
			detail.SourceAttribution, detail.SourceLicense, detail.SourceNonCommercial))
	}
	return suggestions
}

func mechanismTraitSummary(detail TraitProfileReportDetail) string {
	if len(detail.Traits) == 0 {
		return ""
	}
	parts := make([]string, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		label := firstNonEmptyMechanism(trait.Name, trait.Code)
		parts = append(parts, fmt.Sprintf("%s %.0f", label, trait.RawScore))
	}
	return strings.Join(parts, " / ")
}

func mechanismTraitDimensions(detail TraitProfileReportDetail) []domainreport.DimensionInterpret {
	if len(detail.Traits) == 0 {
		return nil
	}
	dimensions := make([]domainreport.DimensionInterpret, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		label := firstNonEmptyMechanism(trait.Name, trait.Code)
		description := fmt.Sprintf("%s：原始分 %.0f", label, trait.RawScore)
		dimensions = append(dimensions, domainreport.NewNeutralDimensionInterpret(
			domainreport.NewDimensionCode(trait.Code), domainreport.DimensionKindTrait, label, trait.RawScore, nil, nil, description, "",
		))
	}
	return dimensions
}

func mechanismTraitSuggestions(detail TraitProfileReportDetail) []domainreport.Suggestion {
	summary := mechanismTraitSummary(detail)
	suggestions := make([]domainreport.Suggestion, 0, 2)
	if summary != "" {
		suggestions = append(suggestions, domainreport.Suggestion{Category: domainreport.SuggestionCategoryGeneral, Content: "特质分布：" + summary})
	}
	if detail.Source.Attribution != "" {
		suggestions = append(suggestions, domainreport.Suggestion{
			Category: domainreport.SuggestionCategoryGeneral,
			Content: fmt.Sprintf("来源与授权：%s；License: %s；非商业使用: %t。",
				detail.Source.Attribution, detail.Source.License, detail.Source.NonCommercial),
		})
	}
	if len(suggestions) == 0 {
		return nil
	}
	return suggestions
}

func mechanismReportRarity(rarity PersonalityTypeRarityReport) *domainreport.ModelRarity {
	if rarity.Percent == 0 && rarity.Label == "" && rarity.OneInX == 0 {
		return nil
	}
	return &domainreport.ModelRarity{Percent: rarity.Percent, Label: rarity.Label, OneInX: rarity.OneInX}
}

func firstNonEmptyMechanism(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
