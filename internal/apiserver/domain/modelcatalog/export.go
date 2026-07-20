package modelcatalog

// Root facade: thin aliases over mechanism subpackages. Deep-import factor/scoring/typology
// when mechanism-specific types are required; use this package for cross-mechanism identity,
// payload format, and read-model surfaces.

import (
	assessmentmodelpkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	conclusionpkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	definitionpkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	factorpkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	identitypkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	normpkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	payloadformatpkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/payloadformat"
)

type (
	Kind           = identitypkg.Kind
	SubKind        = identitypkg.SubKind
	Algorithm      = identitypkg.Algorithm
	DecisionKind   = identitypkg.DecisionKind
	ProductChannel = binding.ProductChannel
	Product        = binding.Product
	Identity       = identitypkg.Identity
	Family         = identitypkg.Family

	AlgorithmFamily = identitypkg.AlgorithmFamily
	RuntimeIdentity = identitypkg.RuntimeIdentity
	ExecutionPath   = binding.ExecutionPath

	KindCapability        = binding.KindCapability
	ModelFamilyCapability = binding.ModelFamilyCapability
	CapabilityRole        = binding.CapabilityRole

	QuestionnaireBinding = binding.QuestionnaireBinding

	// Mechanism re-exports (factor).
	Factor                      = factorpkg.Factor
	FactorGraph                 = factorpkg.FactorGraph
	FactorRole                  = factorpkg.FactorRole
	Scoring                     = factorpkg.Scoring
	ScoringSource               = factorpkg.ScoringSource
	ScoringSourceKind           = factorpkg.ScoringSourceKind
	ScoringStrategy             = factorpkg.ScoringStrategy
	ScoringParams               = factorpkg.ScoringParams
	OptionScoring               = factorpkg.OptionScoring
	ChildrenPolicy              = factorpkg.ChildrenPolicy
	ChildrenAggregationStrategy = factorpkg.ChildrenAggregationStrategy

	AssessmentModel         = assessmentmodelpkg.AssessmentModel
	NewAssessmentModelInput = assessmentmodelpkg.NewInput
	DefinitionPayload       = assessmentmodelpkg.DefinitionPayload
	ModelStatus             = assessmentmodelpkg.Status
	ReleaseStatus           = assessmentmodelpkg.ReleaseStatus
	ValidationLevel         = assessmentmodelpkg.ValidationLevel
	DomainValidationIssue   = assessmentmodelpkg.DomainValidationIssue
	DomainValidationResult  = assessmentmodelpkg.DomainValidationResult

	Definition                = definitionpkg.Definition
	MeasureSpec               = definitionpkg.MeasureSpec
	Calibration               = definitionpkg.Calibration
	ExecutionSpec             = definitionpkg.ExecutionSpec
	Brief2Spec                = definitionpkg.Brief2Spec
	SPMSpec                   = definitionpkg.SPMSpec
	SPMItemSet                = definitionpkg.SPMItemSet
	SPMItem                   = definitionpkg.SPMItem
	ReportMap                 = definitionpkg.ReportMap
	ReportSection             = definitionpkg.ReportSection
	Norm                      = normpkg.Norm
	NormRef                   = normpkg.Ref
	NormFactorTable           = normpkg.FactorTable
	NormBand                  = normpkg.Band
	NormLookupEntry           = normpkg.LookupEntry
	Conclusion                = conclusionpkg.Conclusion
	Outcome                   = conclusionpkg.Outcome
	ScoreRangeOutcome         = conclusionpkg.ScoreRangeOutcome
	ScoreBasis                = conclusionpkg.ScoreBasis
	TypeDecision              = conclusionpkg.TypeDecision
	TypeLevelRule             = conclusionpkg.TypeLevelRule
	TypePole                  = conclusionpkg.TypePole
	TypeSpecialRule           = conclusionpkg.TypeSpecialRule
	TypeOutcomeMapping        = conclusionpkg.TypeOutcomeMapping
	TypeOutcomeProfile        = conclusionpkg.TypeOutcomeProfile
	RiskConclusion            = conclusionpkg.RiskConclusion
	TypeConclusion            = conclusionpkg.TypeConclusion
	NormConclusion            = conclusionpkg.NormConclusion
	AbilityConclusion         = conclusionpkg.AbilityConclusion
	DefinitionValidationIssue = definitionpkg.ValidationIssue
)

const (
	KindScale            = identitypkg.KindScale
	KindTypology         = identitypkg.KindTypology
	KindBehavioralRating = identitypkg.KindBehavioralRating
	KindCognitive        = identitypkg.KindCognitive

	SubKindEmpty    = identitypkg.SubKindEmpty
	SubKindTypology = identitypkg.SubKindTypology
	SubKindTrait    = identitypkg.SubKindTrait

	AlgorithmScaleDefault            = identitypkg.AlgorithmScaleDefault
	AlgorithmPersonalityTypology     = identitypkg.AlgorithmPersonalityTypology
	AlgorithmBigFive                 = identitypkg.AlgorithmBigFive
	AlgorithmMBTI                    = identitypkg.AlgorithmMBTI
	AlgorithmSBTI                    = identitypkg.AlgorithmSBTI
	AlgorithmBrief2                  = identitypkg.AlgorithmBrief2
	AlgorithmSPMSensory              = identitypkg.AlgorithmSPMSensory
	AlgorithmSPM                     = identitypkg.AlgorithmSPM
	AlgorithmBehavioralRatingDefault = identitypkg.AlgorithmBehavioralRatingDefault

	DecisionKindScoreRange                            = identitypkg.DecisionKindScoreRange
	DecisionKindPoleComposition                       = identitypkg.DecisionKindPoleComposition
	DecisionKindTraitProfile                          = identitypkg.DecisionKindTraitProfile
	DecisionKindNearestPattern                        = identitypkg.DecisionKindNearestPattern
	DecisionKindDominantFactor                        = identitypkg.DecisionKindDominantFactor
	DecisionKindNormLookup                            = identitypkg.DecisionKindNormLookup
	DecisionKindAbilityLevel                          = identitypkg.DecisionKindAbilityLevel
	DecisionKindScoreRangeInterpretation DecisionKind = identitypkg.DecisionKindScoreRangeInterpretation //nolint:staticcheck // legacy decode alias

	ScoringSourceQuestion      = factorpkg.ScoringSourceQuestion
	ScoringSourceFactor        = factorpkg.ScoringSourceFactor
	ScoringStrategyWeightedAvg = factorpkg.ScoringStrategyWeightedAvg
	OptionScoringStrict        = factorpkg.OptionScoringStrict
	OptionScoringCompat        = factorpkg.OptionScoringCompat

	ScoreBasisRaw           = conclusionpkg.ScoreBasisRaw
	ScoreBasisTScore        = conclusionpkg.ScoreBasisTScore
	ScoreBasisPercentile    = conclusionpkg.ScoreBasisPercentile
	ScoreBasisStandardScore = conclusionpkg.ScoreBasisStandardScore

	ProductChannelMedicalScale    = binding.ProductChannelMedicalScale
	ProductChannelTypology        = binding.ProductChannelTypology
	ProductChannelBehaviorAbility = binding.ProductChannelBehaviorAbility

	ProductMedicalScale    = binding.ProductMedicalScale
	ProductTypology        = binding.ProductTypology
	ProductBehaviorAbility = binding.ProductBehaviorAbility

	FamilyFactorScoring        = identitypkg.FamilyFactorScoring
	FamilyFactorClassification = identitypkg.FamilyFactorClassification
	FamilyFactorNorm           = identitypkg.FamilyFactorNorm
	FamilyTaskPerformance      = identitypkg.FamilyTaskPerformance

	AlgorithmFamilyFactorScoring        = identitypkg.AlgorithmFamilyFactorScoring
	AlgorithmFamilyFactorClassification = identitypkg.AlgorithmFamilyFactorClassification
	AlgorithmFamilyFactorNorm           = identitypkg.AlgorithmFamilyFactorNorm
	AlgorithmFamilyTaskPerformance      = identitypkg.AlgorithmFamilyTaskPerformance

	ExecutionPathNone                       = binding.ExecutionPathNone
	ExecutionPathScaleDescriptor            = binding.ExecutionPathScaleDescriptor
	ExecutionPathTypologyDescriptor         = binding.ExecutionPathTypologyDescriptor
	ExecutionPathBehavioralRatingDescriptor = binding.ExecutionPathBehavioralRatingDescriptor
	ExecutionPathCognitiveDescriptor        = binding.ExecutionPathCognitiveDescriptor

	PayloadFormatAssessmentScaleV1         = payloadformatpkg.PayloadFormatAssessmentScaleV1
	PayloadFormatPersonalityTypologyV1     = payloadformatpkg.PayloadFormatPersonalityTypologyV1
	PayloadFormatTypologyV1                = payloadformatpkg.PayloadFormatTypologyV1
	PayloadFormatBehavioralRatingDefaultV1 = payloadformatpkg.PayloadFormatBehavioralRatingDefaultV1
	PayloadFormatBehavioralRatingBrief2V1  = payloadformatpkg.PayloadFormatBehavioralRatingBrief2V1
	PayloadFormatCognitiveDefaultV1        = payloadformatpkg.PayloadFormatCognitiveDefaultV1
	PayloadFormatCognitiveSPMV1            = payloadformatpkg.PayloadFormatCognitiveSPMV1
	PayloadFormatScaleV1                   = payloadformatpkg.PayloadFormatScaleV1
	PayloadFormatMBTIV1                    = payloadformatpkg.PayloadFormatMBTIV1 //nolint:staticcheck // legacy decode data value
	PayloadFormatSBTIV1                    = payloadformatpkg.PayloadFormatSBTIV1 //nolint:staticcheck // legacy decode data value
	PayloadFormatScaleV1Legacy             = payloadformatpkg.PayloadFormatScaleV1Legacy
	PayloadFormatMBTIV1Legacy              = payloadformatpkg.PayloadFormatMBTIV1Legacy
	PayloadFormatSBTIV1Legacy              = payloadformatpkg.PayloadFormatSBTIV1Legacy

	CapabilityRoleProductChannel = binding.CapabilityRoleProductChannel
	CapabilityRoleModelFamily    = binding.CapabilityRoleModelFamily

	SchemaVersionV1 = payloadformatpkg.SchemaVersionV1
	SchemaVersionV2 = payloadformatpkg.SchemaVersionV2

	ModelStatusDraft      = assessmentmodelpkg.StatusDraft
	ModelStatusPublished  = assessmentmodelpkg.StatusPublished
	ModelStatusArchived   = assessmentmodelpkg.StatusArchived
	ReleaseStatusActive   = assessmentmodelpkg.ReleaseStatusActive
	ReleaseStatusArchived = assessmentmodelpkg.ReleaseStatusArchived

	ValidationLevelError   = assessmentmodelpkg.ValidationLevelError
	ValidationLevelWarning = assessmentmodelpkg.ValidationLevelWarning
)

var (
	DefaultProductChannelFor  = binding.DefaultProductChannelFor
	ResolveProductChannel     = binding.ResolveProductChannel
	CompleteProductChannel    = binding.CompleteProductChannel
	ValidateNewProductChannel = binding.ValidateNewProductChannel
	HasValidationErrors       = binding.HasValidationErrors
	AllProductChannels        = binding.AllProductChannels
	LegacyKindMapping         = binding.LegacyKindMapping
	ProductFromChannel        = binding.ProductFromChannel
	NewIdentity               = identitypkg.New
	FamilyFromDecisionKind    = identitypkg.FamilyFromDecisionKind
	FamilyFromIdentity        = identitypkg.FamilyFromIdentity

	AlgorithmFamilyFromDecisionKind = identitypkg.AlgorithmFamilyFromDecisionKind
	DecisionKindForIdentity         = binding.DecisionKindForIdentity
	AlgorithmFamilyFromIdentity     = binding.AlgorithmFamilyFromIdentity
	AllAlgorithmFamilies            = identitypkg.AllAlgorithmFamilies
	ResolveRuntimeIdentity          = identitypkg.ResolveRuntimeIdentity

	IsScalePayloadFormat               = payloadformatpkg.IsScalePayloadFormat
	IsLegacyDecodeOnlyPayloadFormat    = payloadformatpkg.IsLegacyDecodeOnlyPayloadFormat
	IsMBTIPayloadFormat                = payloadformatpkg.IsMBTIPayloadFormat
	IsSBTIPayloadFormat                = payloadformatpkg.IsSBTIPayloadFormat
	IsPersonalityTypologyPayloadFormat = payloadformatpkg.IsPersonalityTypologyPayloadFormat
	AlgorithmFromTypologyPayload       = payloadformatpkg.AlgorithmFromTypologyPayload
	PayloadFormatForBehavioralRating   = payloadformatpkg.PayloadFormatForBehavioralRating
	PayloadFormatForCognitive          = payloadformatpkg.PayloadFormatForCognitive
	IsBehavioralRatingPayloadFormat    = payloadformatpkg.IsBehavioralRatingPayloadFormat
	IsCognitivePayloadFormat           = payloadformatpkg.IsCognitivePayloadFormat
	DraftPayloadFormatForModel         = payloadformatpkg.DraftPayloadFormatForModel

	FamilyCapabilityByKind = binding.FamilyCapabilityByKind
	RuntimeExecutableKinds = binding.RuntimeExecutableKinds

	NewAssessmentModel     = assessmentmodelpkg.New
	ParseModelStatus       = assessmentmodelpkg.ParseStatus
	NormalizeReleaseStatus = assessmentmodelpkg.NormalizeReleaseStatus
	ValidateDefinition     = definitionpkg.Validate
)
