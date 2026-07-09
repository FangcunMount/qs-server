// Package typology owns the runtime/published JSON DTO for typology execution.
package typology

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	oldtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

type (
	Payload         = oldtypology.Payload
	Source          = oldtypology.Source
	Dimension       = oldtypology.Dimension
	QuestionMapping = oldtypology.QuestionMapping
	Outcome         = oldtypology.Outcome
	Rarity          = oldtypology.Rarity
	MatchingSpec    = oldtypology.MatchingSpec
	SpecialTrigger  = oldtypology.SpecialTrigger

	RuntimeSpec             = oldtypology.RuntimeSpec
	FactorGraphSpec         = oldtypology.FactorGraphSpec
	FactorSpec              = oldtypology.FactorSpec
	FactorSpecKind          = oldtypology.FactorSpecKind
	FactorAggregation       = oldtypology.FactorAggregation
	FactorOptionScoring     = oldtypology.FactorOptionScoring
	FactorContributionSpec  = oldtypology.FactorContributionSpec
	PersonalityDecisionSpec = oldtypology.PersonalityDecisionSpec
	LevelRuleSpec           = oldtypology.LevelRuleSpec
	SpecialRulePhase        = oldtypology.SpecialRulePhase
	SpecialRuleSpec         = oldtypology.SpecialRuleSpec
	SpecialRuleKind         = oldtypology.SpecialRuleKind
	SpecialRuleCondition    = oldtypology.SpecialRuleCondition
	OutcomeDetailKind       = oldtypology.OutcomeDetailKind
	OutcomeMappingSpec      = oldtypology.OutcomeMappingSpec
	DetailAdapterKey        = oldtypology.DetailAdapterKey
	ReportKind              = oldtypology.ReportKind
	ReportSpec              = oldtypology.ReportSpec
	ReportAdapterKey        = oldtypology.ReportAdapterKey

	QuestionnaireSnapshot        = oldtypology.QuestionnaireSnapshot
	QuestionSnapshot             = oldtypology.QuestionSnapshot
	RuntimeSpecValidationContext = oldtypology.RuntimeSpecValidationContext

	MBTILegacyModel           = oldtypology.MBTILegacyModel
	MBTILegacySource          = oldtypology.MBTILegacySource
	MBTILegacyDimension       = oldtypology.MBTILegacyDimension
	MBTILegacyQuestionMapping = oldtypology.MBTILegacyQuestionMapping
	MBTILegacyTypeProfile     = oldtypology.MBTILegacyTypeProfile

	SBTILegacyModel           = oldtypology.SBTILegacyModel
	SBTILegacySource          = oldtypology.SBTILegacySource
	SBTILegacyDimension       = oldtypology.SBTILegacyDimension
	SBTILegacyQuestionMapping = oldtypology.SBTILegacyQuestionMapping
	SBTILegacyOutcome         = oldtypology.SBTILegacyOutcome
	SBTILegacyRarity          = oldtypology.SBTILegacyRarity
	SBTILegacyDrinkTrigger    = oldtypology.SBTILegacyDrinkTrigger
)

const (
	FactorSpecKindLeaf      = oldtypology.FactorSpecKindLeaf
	FactorSpecKindComposite = oldtypology.FactorSpecKindComposite

	FactorAggregationSum         = oldtypology.FactorAggregationSum
	FactorAggregationAvg         = oldtypology.FactorAggregationAvg
	FactorAggregationWeightedAvg = oldtypology.FactorAggregationWeightedAvg

	FactorOptionScoringStrict = oldtypology.FactorOptionScoringStrict
	FactorOptionScoringCompat = oldtypology.FactorOptionScoringCompat

	SpecialRuleBeforeScore    = oldtypology.SpecialRuleBeforeScore
	SpecialRuleBeforeDecision = oldtypology.SpecialRuleBeforeDecision
	SpecialRuleAfterDecision  = oldtypology.SpecialRuleAfterDecision

	SpecialRuleKindAnswerMatch       = oldtypology.SpecialRuleKindAnswerMatch
	SpecialRuleKindFallbackThreshold = oldtypology.SpecialRuleKindFallbackThreshold

	OutcomeDetailPersonalityType = oldtypology.OutcomeDetailPersonalityType
	OutcomeDetailTraitProfile    = oldtypology.OutcomeDetailTraitProfile

	DetailAdapterPersonalityType = oldtypology.DetailAdapterPersonalityType
	DetailAdapterTraitProfile    = oldtypology.DetailAdapterTraitProfile
	DetailAdapterMBTI            = oldtypology.DetailAdapterMBTI
	DetailAdapterSBTI            = oldtypology.DetailAdapterSBTI
	DetailAdapterBigFive         = oldtypology.DetailAdapterBigFive

	ReportKindPersonalityType = oldtypology.ReportKindPersonalityType
	ReportKindTraitProfile    = oldtypology.ReportKindTraitProfile
	ReportKindTemplate        = oldtypology.ReportKindTemplate

	ReportAdapterPersonalityType = oldtypology.ReportAdapterPersonalityType
	ReportAdapterTraitProfile    = oldtypology.ReportAdapterTraitProfile
	ReportAdapterMBTI            = oldtypology.ReportAdapterMBTI
	ReportAdapterSBTI            = oldtypology.ReportAdapterSBTI
	ReportAdapterBigFive         = oldtypology.ReportAdapterBigFive
)

func FromMBTI(model *MBTILegacyModel) *Payload {
	return oldtypology.FromMBTI(model)
}

func ToMBTI(payload *Payload) (*MBTILegacyModel, error) {
	return oldtypology.ToMBTI(payload)
}

func FromSBTI(model *SBTILegacyModel) *Payload {
	return oldtypology.FromSBTI(model)
}

func ToSBTI(payload *Payload) (*SBTILegacyModel, error) {
	return oldtypology.ToSBTI(payload)
}

func PayloadAndRuntimeSpecFromDefinition(data []byte, defaultAlgorithm binding.Algorithm) (*Payload, *RuntimeSpec, error) {
	return oldtypology.PayloadAndRuntimeSpecFromDefinition(data, defaultAlgorithm)
}

func DefinitionFromPayload(payload []byte, algorithm binding.Algorithm) (*definition.Definition, error) {
	return oldtypology.DefinitionFromPayload(payload, algorithm)
}

func DefinitionFromRuntime(payload *Payload, runtime *RuntimeSpec) *definition.Definition {
	return oldtypology.DefinitionFromRuntime(payload, runtime)
}

func LegacyOutcomeMappingFromAlgorithm(algorithm binding.Algorithm) OutcomeMappingSpec {
	return oldtypology.LegacyOutcomeMappingFromAlgorithm(algorithm)
}

func LegacyReportSpecFromPayload(payload *Payload) ReportSpec {
	return oldtypology.LegacyReportSpecFromPayload(payload)
}

func LegacyReportSpecFromAlgorithm(algorithm binding.Algorithm) ReportSpec {
	return oldtypology.LegacyReportSpecFromAlgorithm(algorithm)
}

func CanonicalFactorsFromGraph(graph FactorGraphSpec) []factor.Factor {
	return oldtypology.CanonicalFactorsFromGraph(graph)
}

func CanonicalMeasureSpecFromGraph(graph FactorGraphSpec) definition.MeasureSpec {
	return oldtypology.CanonicalMeasureSpecFromGraph(graph)
}

func ValidateRuntimeSpecForPublish(spec *RuntimeSpec, questionnaire QuestionnaireSnapshot) []binding.DomainValidationIssue {
	return oldtypology.ValidateRuntimeSpecForPublish(spec, questionnaire)
}

func ValidateRuntimeSpecForPublishWithContext(spec *RuntimeSpec, questionnaire QuestionnaireSnapshot, validationContext RuntimeSpecValidationContext) []binding.DomainValidationIssue {
	return oldtypology.ValidateRuntimeSpecForPublishWithContext(spec, questionnaire, validationContext)
}
