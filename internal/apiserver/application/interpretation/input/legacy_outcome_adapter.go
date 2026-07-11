// Package input adapts durable Evaluation facts into the Interpretation-owned
// input contract. Compatibility with application/evaluation/outcome is kept
// here so report builders never need a synthetic Assessment.
package input

import (
	"fmt"

	domaininterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scoring"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationcompat"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationtypology"
	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationtypologylegacy"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

// LegacyTemplateVersion is retained only for preview and characterization
// compatibility; production uses DefaultTemplateVersion from Outcome records.
const LegacyTemplateVersion = DefaultTemplateVersion

// FromLegacyOutcome is the temporary compatibility boundary for the old report
// writer. New Generation/Run orchestration will construct the same input from
// EvaluationOutcome directly and remove this adapter.
func FromLegacyOutcome(outcome evaloutcome.Outcome) (interpinput.InterpretationInput, error) {
	if outcome.Assessment == nil || outcome.Execution == nil {
		return interpinput.InterpretationInput{}, fmt.Errorf("persisted evaluation outcome context is incomplete")
	}
	model := modelIdentity(outcome)
	in := interpinput.InterpretationInput{
		Association: report.Association{
			OrgID:        outcome.Assessment.OrgID(),
			AssessmentID: outcome.Assessment.ID(),
			TesteeID:     uint64(outcome.Assessment.TesteeID()),
		},
		Model: model,
		Runtime: interpinput.RuntimeIdentity{
			AlgorithmFamily: outcome.RuntimeDescriptorKey.AlgorithmFamily,
			DecisionKind:    outcome.RuntimeDescriptorKey.DecisionKind,
			PayloadFormat:   outcome.RuntimeDescriptorKey.PayloadFormat,
		},
		Result: interpinput.ResultFacts{Primary: primary(outcome.Execution), Level: level(outcome.Execution)},
		Report: interpinput.ReportSpec{
			ReportType:      policy.ReportTypeStandard,
			TemplateVersion: LegacyTemplateVersion,
			Algorithm:       modelcatalog.Algorithm(model.Algorithm),
			ProductChannel:  modelcatalog.ProductChannel(model.ProductChannel),
			Audience:        policy.AudienceParticipant,
		},
	}
	if in.Runtime.AlgorithmFamily == "" {
		in.Runtime.AlgorithmFamily, _ = modelcatalog.AlgorithmFamilyFromIdentity(
			modelcatalog.Kind(model.Kind), modelcatalog.SubKind(model.SubKind), modelcatalog.Algorithm(model.Algorithm),
		)
	}
	if in.Runtime.DecisionKind == "" {
		in.Runtime.DecisionKind = defaultDecisionKind(in.Runtime.AlgorithmFamily)
	}
	in.Report.ReportProfile = policy.ReportProfileForDecisionKind(in.Runtime.DecisionKind)

	switch in.Runtime.AlgorithmFamily {
	case modelcatalog.AlgorithmFamilyFactorScoring, modelcatalog.AlgorithmFamilyFactorNorm, modelcatalog.AlgorithmFamilyTaskPerformance:
		model := factorModel(outcome.Input, in.Runtime.AlgorithmFamily)
		in.FactorScoring = &interpinput.FactorScoringFacts{Model: model, Factors: factorScores(outcome.Execution, model)}
	case modelcatalog.AlgorithmFamilyFactorClassification:
		if err := populateTypologyFacts(&in, outcome.Execution); err != nil {
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

func modelIdentity(outcome evaloutcome.Outcome) report.ModelIdentity {
	var model report.ModelIdentity
	if outcome.Execution != nil && !outcome.Execution.ModelRef.IsEmpty() {
		ref := outcome.Execution.ModelRef
		identity := ref.ExecutionIdentity()
		model = report.ModelIdentity{
			Kind: string(identity.Kind), SubKind: string(identity.SubKind), Algorithm: string(identity.Algorithm),
			Code: ref.Code().String(), Version: ref.Version(), Title: ref.Title(),
			ProductChannel:  string(binding.ProductChannelForIdentity(identity.Kind, "")),
			AlgorithmFamily: binding.AlgorithmFamilyStringFromIdentity(identity.Kind, identity.SubKind, identity.Algorithm),
		}
	}
	if outcome.Assessment != nil && outcome.Assessment.EvaluationModelRef() != nil {
		ref := outcome.Assessment.EvaluationModelRef()
		identity := ref.ExecutionIdentity()
		if model.Kind == "" {
			model.Kind = string(identity.Kind)
		}
		if model.SubKind == "" {
			model.SubKind = string(identity.SubKind)
		}
		if model.Algorithm == "" {
			model.Algorithm = string(identity.Algorithm)
		}
		if model.Code == "" {
			model.Code = ref.Code().String()
		}
		if model.Version == "" {
			model.Version = ref.Version()
		}
		if model.Title == "" {
			model.Title = ref.Title()
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
	if domaininterpretation.IsRiskLevelCode(value.Code) {
		return report.LevelFromRisk(report.RiskLevel(value.Code))
	}
	return &report.ResultLevel{Code: value.Code, Label: value.Label, Severity: value.Severity}
}

func defaultDecisionKind(family modelcatalog.AlgorithmFamily) modelcatalog.DecisionKind {
	switch family {
	case modelcatalog.AlgorithmFamilyFactorScoring:
		return modelcatalog.DecisionKindScoreRange
	case modelcatalog.AlgorithmFamilyFactorClassification:
		return modelcatalog.DecisionKindPoleComposition
	case modelcatalog.AlgorithmFamilyFactorNorm:
		return modelcatalog.DecisionKindNormLookup
	case modelcatalog.AlgorithmFamilyTaskPerformance:
		return modelcatalog.DecisionKindAbilityLevel
	default:
		return ""
	}
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
		if dimension.Level != nil && domaininterpretation.IsRiskLevelCode(dimension.Level.Code) {
			risk = report.RiskLevel(dimension.Level.Code)
		}
		items = append(items, reportscore.FactorReportScore{FactorCode: dimension.Code, FactorName: dimension.Name, RawScore: dimension.Score.Value, RiskLevel: risk, IsTotalScore: totalCodes[dimension.Code] || dimension.Role == "total", Role: dimension.Role, ParentCode: dimension.ParentCode, HierarchyLevel: dimension.HierarchyLevel, SortOrder: dimension.SortOrder})
	}
	return items
}

func populateTypologyFacts(input *interpinput.InterpretationInput, execution *domainoutcome.Execution) error {
	if execution == nil {
		return fmt.Errorf("evaluation outcome is required")
	}
	switch detail := execution.Detail.Payload.(type) {
	case outcometypology.PersonalityTypeDetail:
		setPersonalityTypeFacts(input, detail)
	case outcometypology.TraitProfileDetail:
		setTraitProfileFacts(input, detail)
	default:
		if legacy, err := typologylegacy.MBTIResultDetailFromPayload(detail); err == nil {
			input.Report.AdapterKey = "mbti"
			setPersonalityTypeFacts(input, typologylegacy.PersonalityTypeDetailFromMBTI(legacy))
			return nil
		}
		if legacy, err := typologylegacy.SBTIResultDetailFromPayload(detail); err == nil {
			input.Report.AdapterKey = "sbti"
			setPersonalityTypeFacts(input, typologylegacy.PersonalityTypeDetailFromSBTI(legacy))
			return nil
		}
		if legacy, err := typologylegacy.BigFiveResultDetailFromPayload(detail); err == nil {
			input.Report.AdapterKey = "bigfive"
			setTraitProfileFacts(input, typologylegacy.TraitProfileDetailFromBigFive(legacy))
			return nil
		}
		return fmt.Errorf("unsupported typology evaluation detail %T", execution.Detail.Payload)
	}
	return nil
}

func setPersonalityTypeFacts(input *interpinput.InterpretationInput, detail outcometypology.PersonalityTypeDetail) {
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

func setTraitProfileFacts(input *interpinput.InterpretationInput, detail outcometypology.TraitProfileDetail) {
	traits := make([]reporttypology.TraitProfileFactorReport, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		traits = append(traits, reporttypology.TraitProfileFactorReport(trait))
	}
	input.TraitProfile = &interpinput.TraitProfileFacts{Detail: reporttypology.TraitProfileReportDetail{Traits: traits, Source: reporttypology.TraitProfileSourceReport{Attribution: detail.Source.Attribution, License: detail.Source.License, NonCommercial: detail.Source.NonCommercial}}}
}

func personalityDimensions(detail outcometypology.PersonalityTypeDetail) []reporttypology.PersonalityTypeDimensionReport {
	dimensions := make([]reporttypology.PersonalityTypeDimensionReport, 0, len(detail.Dimensions))
	for _, dimension := range detail.Dimensions {
		dimensions = append(dimensions, reporttypology.PersonalityTypeDimensionReport{Code: dimension.Code, Name: dimension.Name, LeftPole: dimension.LeftPole, RightPole: dimension.RightPole, RawScore: dimension.RawScore, Preference: dimension.Preference, Strength: dimension.Strength, Model: dimension.Model, Level: dimension.Level})
	}
	return dimensions
}
