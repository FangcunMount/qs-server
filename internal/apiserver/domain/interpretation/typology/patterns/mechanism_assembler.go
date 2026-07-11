package patterns

import (
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology"
)

// PersonalityTypeReportTemplate 携带呈现 labels 用于 面向机制 reports。
type PersonalityTypeReportTemplate struct {
	Kind                 string
	DefaultModelName     string
	DefaultModelCode     string
	DimensionMaxScore    *float64
	DimensionDescription func(name, preference string, rawScore, strength float64, level, model string) string
	ConclusionSuffix     func(detail PersonalityTypeReportDetail) string
}

// TraitProfileReportTemplate 携带呈现 labels 用于 trait-画像 reports。
type TraitProfileReportTemplate struct {
	Kind             string
	DefaultModelName string
	DefaultModelCode string
	TypeName         string
	OneLiner         string
	ConclusionTitle  string
}

func BuildPersonalityTypeContent(input PersonalityTypeReportInput, tmpl PersonalityTypeReportTemplate) (report.Content, error) {
	if input.AssessmentID.IsZero() {
		return report.Content{}, fmt.Errorf("assessment is required")
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
	profile := reporttypology.Profile{
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
	return report.Content{
		Model:       report.ModelIdentity{Title: profile.ReportModelName(), Code: profile.ReportModelCode(input.ModelCode)},
		Conclusion:  profile.Conclusion(mechanismConclusionSuffix(tmpl, detail)),
		Dimensions:  mechanismPersonalityDimensions(detail, tmpl),
		Suggestions: mechanismPersonalitySuggestions(detail),
		ModelExtra:  profile.ModelExtra(),
	}, nil
}

func mechanismConclusionSuffix(tmpl PersonalityTypeReportTemplate, detail PersonalityTypeReportDetail) string {
	if tmpl.ConclusionSuffix != nil {
		return tmpl.ConclusionSuffix(detail)
	}
	return ""
}

// BuildTraitProfileReport 组装trait-画像 report 从 机制无关 detail。
func BuildTraitProfileContent(input TraitProfileReportInput, tmpl TraitProfileReportTemplate) (report.Content, error) {
	if input.AssessmentID.IsZero() {
		return report.Content{}, fmt.Errorf("assessment is required")
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
	profile := reporttypology.Profile{
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
	return report.Content{
		Model:       report.ModelIdentity{Title: profile.ReportModelName(), Code: profile.ReportModelCode(input.ModelCode)},
		Conclusion:  conclusion,
		Dimensions:  mechanismTraitDimensions(detail),
		Suggestions: mechanismTraitSuggestions(detail),
		ModelExtra:  profile.ModelExtra(),
	}, nil
}

type PersonalityTypeReportInput struct {
	AssessmentID report.ID
	ModelCode    string
	TotalScore   float64
	RiskLevel    report.RiskLevel
	Detail       PersonalityTypeReportDetail
}

type TraitProfileReportInput struct {
	AssessmentID report.ID
	ModelCode    string
	TotalScore   float64
	RiskLevel    report.RiskLevel
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

func mechanismPersonalityDimensions(detail PersonalityTypeReportDetail, tmpl PersonalityTypeReportTemplate) []report.DimensionInterpret {
	if len(detail.Dimensions) == 0 {
		return nil
	}
	dimensions := make([]report.DimensionInterpret, 0, len(detail.Dimensions))
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
		kind := report.DimensionKindFactor
		if dim.Preference != "" {
			kind = report.DimensionKindPole
		}
		if maxScore != nil {
			dimensions = append(dimensions, report.NewDimensionInterpret(
				report.FactorCode(dim.Code), name, dim.RawScore, maxScore, report.RiskLevelNone, description, "",
			))
			continue
		}
		dimensions = append(dimensions, report.NewNeutralDimensionInterpret(
			report.NewDimensionCode(dim.Code), kind, name, dim.RawScore, nil, nil, description, "",
		))
	}
	return dimensions
}

func mechanismPersonalitySuggestions(detail PersonalityTypeReportDetail) []report.Suggestion {
	suggestions := make([]report.Suggestion, 0, 8)
	add := func(content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		suggestions = append(suggestions, report.Suggestion{Category: report.SuggestionCategoryGeneral, Content: content})
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

func mechanismTraitDimensions(detail TraitProfileReportDetail) []report.DimensionInterpret {
	if len(detail.Traits) == 0 {
		return nil
	}
	dimensions := make([]report.DimensionInterpret, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		label := firstNonEmptyMechanism(trait.Name, trait.Code)
		description := fmt.Sprintf("%s：原始分 %.0f", label, trait.RawScore)
		dimensions = append(dimensions, report.NewNeutralDimensionInterpret(
			report.NewDimensionCode(trait.Code), report.DimensionKindTrait, label, trait.RawScore, nil, nil, description, "",
		))
	}
	return dimensions
}

func mechanismTraitSuggestions(detail TraitProfileReportDetail) []report.Suggestion {
	summary := mechanismTraitSummary(detail)
	suggestions := make([]report.Suggestion, 0, 2)
	if summary != "" {
		suggestions = append(suggestions, report.Suggestion{Category: report.SuggestionCategoryGeneral, Content: "特质分布：" + summary})
	}
	if detail.Source.Attribution != "" {
		suggestions = append(suggestions, report.Suggestion{
			Category: report.SuggestionCategoryGeneral,
			Content: fmt.Sprintf("来源与授权：%s；License: %s；非商业使用: %t。",
				detail.Source.Attribution, detail.Source.License, detail.Source.NonCommercial),
		})
	}
	if len(suggestions) == 0 {
		return nil
	}
	return suggestions
}

func mechanismReportRarity(rarity PersonalityTypeRarityReport) *report.ModelRarity {
	if rarity.Percent == 0 && rarity.Label == "" && rarity.OneInX == 0 {
		return nil
	}
	return &report.ModelRarity{Percent: rarity.Percent, Label: rarity.Label, OneInX: rarity.OneInX}
}

func firstNonEmptyMechanism(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
