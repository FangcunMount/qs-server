package input

import (
	"fmt"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	evaluationfactcodec "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact/codec"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
)

func primary(execution *domainoutcome.Execution) *report.ScoreValue {
	if execution == nil || execution.Primary == nil {
		return nil
	}
	value := execution.Primary
	if value.Kind == domainoutcome.ScoreKindMatchPercent {
		return report.NewMatchPercentScore(value.Value, value.Label)
	}
	return report.NewRawTotalScore(value.Value, value.Max)
}

func level(execution *domainoutcome.Execution) *report.ResultLevel {
	if execution == nil || execution.Level == nil {
		return nil
	}
	value := execution.Level
	if eventoutcome.IsRiskLevelCode(value.Code) {
		return report.LevelFromRisk(report.RiskLevel(value.Code))
	}
	return &report.ResultLevel{Code: value.Code, Label: value.Label, Severity: value.Severity}
}

func factorModel(snapshot *evaluationinput.InputSnapshot, family modelcatalog.AlgorithmFamily) *reportscore.ReportModel {
	var scale *scalesnapshot.ScaleSnapshot
	switch family {
	case modelcatalog.AlgorithmFamilyFactorNorm:
		scale, _ = evaluationinput.BehavioralRatingScaleSnapshot(snapshot)
	case modelcatalog.AlgorithmFamilyTaskPerformance:
		scale, _ = evaluationinput.CognitiveScaleSnapshot(snapshot)
	default:
		scale, _ = evaluationinput.ScalePayload(snapshot)
	}
	if scale == nil {
		return nil
	}
	factors := make([]reportscore.FactorReportModel, 0, len(scale.Factors))
	for _, factor := range scale.Factors {
		factors = append(factors, reportscore.FactorReportModel{
			Code: factor.Code, Title: factor.Title, MaxScore: factor.MaxScore, IsTotalScore: factor.IsTotalScore,
			InterpretRules: factorRules(factor.InterpretRules),
		})
	}
	assets := factorModelAssets(snapshot)
	return &reportscore.ReportModel{Code: scale.Code, Title: scale.Title, Factors: factors, Assets: assets}
}

func factorModelAssets(snapshot *evaluationinput.InputSnapshot) *interpretationassets.Assets {
	frozen, ok := evaluationinput.InterpretationAssetsFromSnapshot(snapshot)
	if !ok {
		return nil
	}
	assets := frozen
	return &assets
}

func factorRules(rules []scalesnapshot.InterpretRuleSnapshot) []reportscore.FactorInterpretRule {
	converted := make([]reportscore.FactorInterpretRule, 0, len(rules))
	for _, rule := range rules {
		converted = append(converted, reportscore.FactorInterpretRule{
			Min: rule.Min, Max: rule.Max, MaxInclusive: rule.MaxInclusive, UnboundedMax: rule.UnboundedMax,
			RiskLevel: rule.RiskLevel, Conclusion: rule.Conclusion, Suggestion: rule.Suggestion,
		})
	}
	return converted
}

func factorScores(execution *domainoutcome.Execution, model *reportscore.ReportModel) []reportscore.FactorReportScore {
	if execution == nil {
		return nil
	}
	totalCodes := make(map[string]bool)
	if model != nil {
		for _, factor := range model.Factors {
			totalCodes[factor.Code] = factor.IsTotalScore
		}
	}
	items := make([]reportscore.FactorReportScore, 0, len(execution.Dimensions))
	for _, dimension := range execution.Dimensions {
		if dimension.Score == nil {
			continue
		}
		risk := report.RiskLevelNone
		if dimension.Level != nil && eventoutcome.IsRiskLevelCode(dimension.Level.Code) {
			risk = report.RiskLevel(dimension.Level.Code)
		}
		item := reportscore.FactorReportScore{FactorCode: dimension.Code, FactorName: dimension.Name, RawScore: dimension.Score.Value, RiskLevel: risk, IsTotalScore: totalCodes[dimension.Code] || dimension.Role == "total", Role: dimension.Role, ParentCode: dimension.ParentCode, HierarchyLevel: dimension.HierarchyLevel, SortOrder: dimension.SortOrder}
		for _, score := range dimension.DerivedScores {
			item.DerivedScores = append(item.DerivedScores, report.ScoreValue{Kind: string(score.Kind), Value: score.Value, Label: score.Label, Max: score.Max})
		}
		if dimension.Level != nil {
			item.Level = &report.ResultLevel{Code: dimension.Level.Code, Label: dimension.Level.Label, Severity: dimension.Level.Severity}
		}
		if dimension.NormReference != nil {
			item.NormReference = &report.NormReference{
				ScoreKind: string(dimension.NormReference.ScoreKind), Benchmark: dimension.NormReference.Benchmark,
				TableVersion: dimension.NormReference.TableVersion, FormVariant: dimension.NormReference.FormVariant,
				MinAgeMonths: dimension.NormReference.MinAgeMonths, MaxAgeMonths: dimension.NormReference.MaxAgeMonths,
				Gender: dimension.NormReference.Gender,
			}
		}
		items = append(items, item)
	}
	return items
}

// applyFrozenNormInterpretation restores display prose from the immutable
// report-input snapshot. It never derives the Outcome-owned Level code.
func applyFrozenNormInterpretation(items []reportscore.FactorReportScore, assets *evaluationinput.InputSnapshot, profile *report.PresentationProfile) error {
	visible, configured := visibleFactorCodes(profile)
	for i := range items {
		if configured && !visible[items[i].FactorCode] {
			continue
		}
		if _, ok := reportScoreValue(items[i].DerivedScores, report.ScoreKindTScore); !ok {
			continue
		}
		if items[i].Level == nil || items[i].Level.Code == "" {
			return fmt.Errorf("norm factor %q has T-score but no outcome level code", items[i].FactorCode)
		}
	}
	payload, ok := evaluationinput.BehavioralRatingPayload(assets)
	if !ok || payload.Snapshot == nil || payload.Snapshot.Norming == nil {
		return nil
	}
	tables := payload.Snapshot.Norming.NormTablesOrNil()
	if tables == nil {
		return nil
	}
	for i := range items {
		if configured && !visible[items[i].FactorCode] {
			continue
		}
		tScore, ok := reportScoreValue(items[i].DerivedScores, report.ScoreKindTScore)
		if !ok {
			continue
		}
		level, conclusion, suggestion, interpreted := calcnorm.InterpretTScore(tables, items[i].FactorCode, tScore)
		if !interpreted {
			continue
		}
		if items[i].Level.Code != level {
			return fmt.Errorf("norm factor %q outcome level %q does not match frozen norm level %q", items[i].FactorCode, items[i].Level.Code, level)
		}
		items[i].Conclusion = conclusion
		items[i].Suggestion = suggestion
		if items[i].Level.Label == "" {
			items[i].Level.Label = conclusion
		}
	}
	return nil
}

func visibleFactorCodes(profile *report.PresentationProfile) (map[string]bool, bool) {
	if profile == nil || !profile.Configured() {
		return nil, false
	}
	return profile.VisibleSet(), true
}

func reportScoreValue(scores []report.ScoreValue, kind string) (float64, bool) {
	for _, score := range scores {
		if score.Kind == kind {
			return score.Value, true
		}
	}
	return 0, false
}

func populateTypologyFacts(input *interpinput.InterpretationInput, execution *domainoutcome.Execution, assets *evaluationinput.InputSnapshot) error {
	if execution == nil {
		return fmt.Errorf("evaluation outcome is required")
	}
	if detail, ok := evaluationfactcodec.PersonalityTypeDetailFromPayload(execution.Detail.Payload); ok {
		setPersonalityTypeFacts(input, detail)
		return nil
	}
	if detail, ok := evaluationfactcodec.TraitProfileDetailFromPayload(execution.Detail.Payload); ok {
		setTraitProfileFacts(input, detail)
		return nil
	}
	if fact, ok := evaluationfactcodec.ClassificationFactFromPayload(execution.Detail.Payload); ok {
		return setPersonalityTypeFactsFromV2(input, execution, assets, fact)
	}
	if execution.Profile != nil && execution.Profile.Kind == domainoutcome.ProfileKindPersonalityTrait {
		setTraitProfileFactsFromV2(input, execution, assets)
		return nil
	}
	return fmt.Errorf("unsupported typology evaluation detail %T", execution.Detail.Payload)
}

func setPersonalityTypeFactsFromV2(input *interpinput.InterpretationInput, execution *domainoutcome.Execution, assets *evaluationinput.InputSnapshot, fact evaluationfactcodec.ClassificationFact) error {
	payload, ok := evaluationinput.TypologyPayload(assets)
	if !ok || payload == nil {
		return fmt.Errorf("schema v2 typology report input is required")
	}
	configured, ok := payload.FindOutcome(fact.TypeCode)
	if !ok {
		return fmt.Errorf("schema v2 typology code %s is not present in frozen report input", fact.TypeCode)
	}
	dimensions := make([]reporttypology.PersonalityTypeDimensionReport, 0, len(execution.Dimensions))
	for _, dimension := range execution.Dimensions {
		if dimension.Score == nil {
			continue
		}
		strength := 0.0
		if dimension.Strength != nil {
			strength = *dimension.Strength
		}
		levelCode := ""
		if dimension.Level != nil {
			levelCode = dimension.Level.Code
		}
		dimensions = append(dimensions, reporttypology.PersonalityTypeDimensionReport{
			Code: dimension.Code, Name: dimension.Name, Model: dimension.Model,
			LeftPole: dimension.LeftPole, RightPole: dimension.RightPole,
			RawScore: dimension.Score.Value, Preference: dimension.Preference,
			Strength: strength, Level: levelCode,
		})
	}
	input.PersonalityType = &interpinput.PersonalityTypeFacts{Detail: reporttypology.PersonalityTypeReportDetail{
		TypeCode: fact.TypeCode, TypeName: configured.Name, OneLiner: configured.OneLiner,
		MatchPercent: fact.MatchPercent, ImageURL: firstNonEmpty(configured.ImageURL, configured.Image),
		IsSpecial: fact.IsSpecial, SpecialTrigger: fact.SpecialTrigger, Commentary: configured.Commentary,
		Profile: reporttypology.PersonalityTypeProfileReport{
			Summary: configured.Summary, Strengths: append([]string(nil), configured.Strengths...),
			Weaknesses: append([]string(nil), configured.Weaknesses...), Suggestions: append([]string(nil), configured.Suggestions...),
		},
		Rarity:     reporttypology.PersonalityTypeRarityReport{Percent: configured.Rarity.Percent, Label: configured.Rarity.Label, OneInX: configured.Rarity.OneInX},
		Dimensions: dimensions, SourceAttribution: payload.Source.Attribution,
		SourceLicense: payload.Source.License, SourceNonCommercial: payload.Source.NonCommercial,
	}}
	return nil
}

func setTraitProfileFactsFromV2(input *interpinput.InterpretationInput, execution *domainoutcome.Execution, assets *evaluationinput.InputSnapshot) {
	traits := make([]reporttypology.TraitProfileFactorReport, 0, len(execution.Dimensions))
	for _, dimension := range execution.Dimensions {
		if dimension.Score != nil {
			traits = append(traits, reporttypology.TraitProfileFactorReport{Code: dimension.Code, Name: dimension.Name, RawScore: dimension.Score.Value})
		}
	}
	detail := reporttypology.TraitProfileReportDetail{Traits: traits}
	if payload, ok := evaluationinput.TypologyPayload(assets); ok && payload != nil {
		detail.Source = reporttypology.TraitProfileSourceReport{Attribution: payload.Source.Attribution, License: payload.Source.License, NonCommercial: payload.Source.NonCommercial}
	}
	input.TraitProfile = &interpinput.TraitProfileFacts{Detail: detail}
}

func setPersonalityTypeFacts(input *interpinput.InterpretationInput, detail evaluationfactcodec.PersonalityTypeDetail) {
	input.PersonalityType = &interpinput.PersonalityTypeFacts{Detail: reporttypology.PersonalityTypeReportDetail{
		TypeCode: detail.TypeCode, TypeName: detail.TypeName, OneLiner: detail.OneLiner, MatchPercent: detail.MatchPercent, ImageURL: detail.ImageURL,
		IsSpecial: detail.IsSpecial, SpecialTrigger: detail.SpecialTrigger, Commentary: detail.Commentary,
		Profile:    reporttypology.PersonalityTypeProfileReport{Summary: firstNonEmpty(detail.Summary, detail.Commentary), Strengths: append([]string(nil), detail.Strengths...), Weaknesses: append([]string(nil), detail.Weaknesses...), Suggestions: append([]string(nil), detail.Suggestions...)},
		Rarity:     reporttypology.PersonalityTypeRarityReport{Percent: detail.Rarity.Percent, Label: detail.Rarity.Label, OneInX: detail.Rarity.OneInX},
		Dimensions: personalityDimensions(detail), SourceAttribution: detail.Source.Attribution, SourceLicense: detail.Source.License, SourceNonCommercial: detail.Source.NonCommercial,
	}}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func setTraitProfileFacts(input *interpinput.InterpretationInput, detail evaluationfactcodec.TraitProfileDetail) {
	traits := make([]reporttypology.TraitProfileFactorReport, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		traits = append(traits, reporttypology.TraitProfileFactorReport(trait))
	}
	input.TraitProfile = &interpinput.TraitProfileFacts{Detail: reporttypology.TraitProfileReportDetail{Traits: traits, Source: reporttypology.TraitProfileSourceReport{Attribution: detail.Source.Attribution, License: detail.Source.License, NonCommercial: detail.Source.NonCommercial}}}
}

func personalityDimensions(detail evaluationfactcodec.PersonalityTypeDetail) []reporttypology.PersonalityTypeDimensionReport {
	dimensions := make([]reporttypology.PersonalityTypeDimensionReport, 0, len(detail.Dimensions))
	for _, dimension := range detail.Dimensions {
		dimensions = append(dimensions, reporttypology.PersonalityTypeDimensionReport{Code: dimension.Code, Name: dimension.Name, LeftPole: dimension.LeftPole, RightPole: dimension.RightPole, RawScore: dimension.RawScore, Preference: dimension.Preference, Strength: dimension.Strength, Model: dimension.Model, Level: dimension.Level})
	}
	return dimensions
}
