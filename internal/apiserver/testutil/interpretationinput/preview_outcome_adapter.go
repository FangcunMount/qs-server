// Package input adapts durable Evaluation facts and explicit transient preview
// values into the Interpretation-owned input contract.
package interpretationinput

import (
	"fmt"

	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrule "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rule"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	evaluationfactcodec "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact/codec"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

const PreviewTemplateVersion = policy.TemplateVersionV1

// PreviewOutcome is an explicit transient composition input. It is not a
// committed Evaluation fact and cannot be stored through evaluationfact.Repository.
type PreviewOutcome struct {
	Association report.Association
	Input       *evaluationinput.InputSnapshot
	Execution   *domainoutcome.Execution
	Runtime     domainoutcome.RuntimeIdentity
}

func FromPreviewOutcome(outcome PreviewOutcome) (interpinput.InterpretationInput, error) {
	if outcome.Execution == nil {
		return interpinput.InterpretationInput{}, fmt.Errorf("preview evaluation execution is required")
	}
	model := modelIdentity(outcome)
	in := interpinput.InterpretationInput{
		Association: outcome.Association,
		Model:       model,
		Runtime: interpinput.RuntimeIdentity{
			AlgorithmFamily: outcome.Runtime.AlgorithmFamily,
			DecisionKind:    outcome.Runtime.DecisionKind,
			PayloadFormat:   outcome.Runtime.PayloadFormat,
		},
		Result: interpinput.ResultFacts{Primary: primary(outcome.Execution), Level: level(outcome.Execution)},
		Report: interpinput.ReportSpec{
			ReportType:      policy.ReportTypeStandard,
			TemplateVersion: PreviewTemplateVersion,
			Algorithm:       modelcatalog.Algorithm(model.Algorithm),
			ProductChannel:  modelcatalog.ProductChannel(model.ProductChannel),
		},
	}
	if in.Runtime.AlgorithmFamily == "" {
		in.Runtime.AlgorithmFamily, _ = modelcatalog.AlgorithmFamilyFromIdentity(
			modelcatalog.Kind(model.Kind), modelcatalog.SubKind(model.SubKind), modelcatalog.Algorithm(model.Algorithm),
		)
	}
	if in.Runtime.DecisionKind == "" {
		in.Runtime.DecisionKind = policy.DefaultDecisionKind(in.Runtime.AlgorithmFamily)
	}
	in.Report.ReportProfile = policy.ReportProfileForDecisionKind(in.Runtime.DecisionKind)

	switch in.Runtime.AlgorithmFamily {
	case modelcatalog.AlgorithmFamilyFactorScoring, modelcatalog.AlgorithmFamilyFactorNorm, modelcatalog.AlgorithmFamilyTaskPerformance:
		model := factorModel(outcome.Input, in.Runtime.AlgorithmFamily)
		in.FactorScoring = &interpinput.FactorScoringFacts{Model: model, Factors: factorScores(outcome.Execution, model)}
	case modelcatalog.AlgorithmFamilyFactorClassification:
		if err := populateTypologyFacts(&in, outcome.Execution, outcome.Input); err != nil {
			return interpinput.InterpretationInput{}, err
		}
		if payload, ok := evaluationinput.TypologyPayload(outcome.Input); ok && payload != nil {
			if runtimeSpec, err := payload.ToRuntimeSpec(); err == nil {
				in.Report.TemplateID = runtimeSpec.Report.TemplateID
				in.Report.AdapterKey = string(runtimeSpec.Report.ResolvedAdapterKey(runtimeSpec.OutcomeMapping, runtimeSpec.Decision.Kind))
			}
		}
	}
	return in, nil
}

func modelIdentity(outcome PreviewOutcome) report.ModelIdentity {
	var model report.ModelIdentity
	if outcome.Execution != nil && !outcome.Execution.ModelRef.IsEmpty() {
		ref := outcome.Execution.ModelRef
		model = report.ModelIdentity{
			Kind: string(ref.ModelKind), SubKind: string(ref.ModelSubKind), Algorithm: string(ref.ModelAlgorithm),
			Code: ref.ModelCode, Version: ref.ModelVersion, Title: ref.ModelTitle,
			ProductChannel:  string(binding.ProductChannelForIdentity(ref.ModelKind, "")),
			AlgorithmFamily: binding.AlgorithmFamilyStringFromIdentity(ref.ModelKind, ref.ModelSubKind, ref.ModelAlgorithm),
		}
	}
	if outcome.Input != nil && outcome.Input.Model != nil {
		payload := outcome.Input.Model
		if model.Kind == "" {
			model.Kind = string(payload.Kind)
		}
		if model.SubKind == "" {
			model.SubKind = payload.SubKind
		}
		if model.Algorithm == "" {
			model.Algorithm = payload.Algorithm
		}
		if model.ProductChannel == "" {
			model.ProductChannel = payload.ProductChannel
		}
	}
	if model.Algorithm == "" {
		switch modelcatalog.Kind(model.Kind) {
		case modelcatalog.KindScale:
			model.Algorithm = string(modelcatalog.AlgorithmScaleDefault)
		case modelcatalog.KindTypology:
			model.Algorithm = string(modelcatalog.AlgorithmPersonalityTypology)
		}
	}
	if model.ProductChannel == "" {
		model.ProductChannel = string(modelcatalog.DefaultProductChannelFor(modelcatalog.Kind(model.Kind)))
	}
	if model.AlgorithmFamily == "" {
		model.AlgorithmFamily = binding.AlgorithmFamilyStringFromIdentity(modelcatalog.Kind(model.Kind), modelcatalog.SubKind(model.SubKind), modelcatalog.Algorithm(model.Algorithm))
	}
	return model
}

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
	if interpretationrule.IsRiskLevelCode(value.Code) {
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
	return &reportscore.ReportModel{Code: scale.Code, Title: scale.Title, Factors: factors}
}

func factorRules(rules []scalesnapshot.InterpretRuleSnapshot) []reportscore.FactorInterpretRule {
	converted := make([]reportscore.FactorInterpretRule, 0, len(rules))
	for _, rule := range rules {
		converted = append(converted, reportscore.FactorInterpretRule{Min: rule.Min, Max: rule.Max, RiskLevel: rule.RiskLevel, Conclusion: rule.Conclusion, Suggestion: rule.Suggestion})
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
		if dimension.Level != nil && interpretationrule.IsRiskLevelCode(dimension.Level.Code) {
			risk = report.RiskLevel(dimension.Level.Code)
		}
		items = append(items, reportscore.FactorReportScore{FactorCode: dimension.Code, FactorName: dimension.Name, RawScore: dimension.Score.Value, RiskLevel: risk, IsTotalScore: totalCodes[dimension.Code] || dimension.Role == "total", Role: dimension.Role, ParentCode: dimension.ParentCode, HierarchyLevel: dimension.HierarchyLevel, SortOrder: dimension.SortOrder})
	}
	return items
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
