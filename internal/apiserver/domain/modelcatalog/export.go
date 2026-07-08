package modelcatalog

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	legacypkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
)

type (
	Kind           = binding.Kind
	SubKind        = binding.SubKind
	Algorithm      = binding.Algorithm
	DecisionKind   = binding.DecisionKind
	ProductChannel = binding.ProductChannel

	AlgorithmFamily = publishing.AlgorithmFamily
	ExecutionPath   = publishing.ExecutionPath

	KindCapability   = binding.KindCapability
	CapabilityRole   = binding.CapabilityRole
	CatalogOperation = binding.CatalogOperation

	QuestionnaireBinding   = binding.QuestionnaireBinding
	ModelDefinition        = publishing.ModelDefinition
	DecisionSpec           = publishing.DecisionSpec
	SourceRef              = publishing.SourceRef
	PublishedModelSnapshot = publishing.PublishedModelSnapshot

	AssessmentModel         = publishing.AssessmentModel
	NewAssessmentModelInput = publishing.NewAssessmentModelInput
	DefinitionPayload       = publishing.DefinitionPayload
	ModelStatus             = publishing.ModelStatus
	ValidationLevel         = publishing.ValidationLevel
	DomainValidationIssue   = publishing.DomainValidationIssue
	DomainValidationResult  = publishing.DomainValidationResult

	RuleSetKind       = legacypkg.RuleSetKind
	Definition        = legacypkg.Definition
	Snapshot          = legacypkg.Snapshot
	RuleSetDefinition = legacypkg.RuleSetDefinition
	RuleSetSnapshot   = legacypkg.RuleSetSnapshot
)

const (
	KindScale            = binding.KindScale
	KindPersonality      = binding.KindPersonality
	KindBehavioralRating = binding.KindBehavioralRating
	KindCognitive        = binding.KindCognitive
	KindCustom           = binding.KindCustom

	SubKindEmpty    = binding.SubKindEmpty
	SubKindTypology = binding.SubKindTypology
	SubKindTrait    = binding.SubKindTrait

	AlgorithmScaleDefault            = binding.AlgorithmScaleDefault
	AlgorithmPersonalityTypology     = binding.AlgorithmPersonalityTypology
	AlgorithmBigFive                 = binding.AlgorithmBigFive
	AlgorithmMBTI                    = binding.AlgorithmMBTI
	AlgorithmSBTI                    = binding.AlgorithmSBTI
	AlgorithmBrief2                  = binding.AlgorithmBrief2
	AlgorithmSPM                     = binding.AlgorithmSPM
	AlgorithmBehavioralRatingDefault = binding.AlgorithmBehavioralRatingDefault

	DecisionKindScoreRange                            = binding.DecisionKindScoreRange
	DecisionKindPoleComposition                       = binding.DecisionKindPoleComposition
	DecisionKindTraitProfile                          = binding.DecisionKindTraitProfile
	DecisionKindNearestPattern                        = binding.DecisionKindNearestPattern
	DecisionKindNormLookup                            = binding.DecisionKindNormLookup
	DecisionKindAbilityLevel                          = binding.DecisionKindAbilityLevel
	DecisionKindScoreRangeInterpretation DecisionKind = binding.DecisionKindScoreRangeInterpretation //nolint:staticcheck // legacy decode alias

	ProductChannelMedicalScale    = binding.ProductChannelMedicalScale
	ProductChannelPersonality     = binding.ProductChannelPersonality
	ProductChannelBehaviorAbility = binding.ProductChannelBehaviorAbility
	ProductChannelCognitive       = binding.ProductChannelCognitive
	ProductChannelCustom          = binding.ProductChannelCustom

	AlgorithmFamilyFactorScoring        = publishing.AlgorithmFamilyFactorScoring
	AlgorithmFamilyFactorClassification = publishing.AlgorithmFamilyFactorClassification
	AlgorithmFamilyFactorNorm           = publishing.AlgorithmFamilyFactorNorm
	AlgorithmFamilyTaskPerformance      = publishing.AlgorithmFamilyTaskPerformance

	ExecutionPathNone                       = publishing.ExecutionPathNone
	ExecutionPathScaleDescriptor            = publishing.ExecutionPathScaleDescriptor
	ExecutionPathTypologyDescriptor         = publishing.ExecutionPathTypologyDescriptor
	ExecutionPathBehavioralRatingDescriptor = publishing.ExecutionPathBehavioralRatingDescriptor
	ExecutionPathCognitiveDescriptor        = publishing.ExecutionPathCognitiveDescriptor

	PayloadFormatAssessmentScaleV1         = publishing.PayloadFormatAssessmentScaleV1
	PayloadFormatPersonalityTypologyV1     = publishing.PayloadFormatPersonalityTypologyV1
	PayloadFormatBehavioralRatingDefaultV1 = publishing.PayloadFormatBehavioralRatingDefaultV1
	PayloadFormatBehavioralRatingBrief2V1  = publishing.PayloadFormatBehavioralRatingBrief2V1
	PayloadFormatCognitiveDefaultV1        = publishing.PayloadFormatCognitiveDefaultV1
	PayloadFormatCognitiveSPMV1            = publishing.PayloadFormatCognitiveSPMV1
	PayloadFormatScaleV1                   = publishing.PayloadFormatScaleV1
	PayloadFormatMBTIV1                    = publishing.PayloadFormatMBTIV1 //nolint:staticcheck // legacy decode data value
	PayloadFormatSBTIV1                    = publishing.PayloadFormatSBTIV1 //nolint:staticcheck // legacy decode data value
	PayloadFormatScaleV1Legacy             = publishing.PayloadFormatScaleV1Legacy
	PayloadFormatMBTIV1Legacy              = publishing.PayloadFormatMBTIV1Legacy
	PayloadFormatSBTIV1Legacy              = publishing.PayloadFormatSBTIV1Legacy

	CapabilityRoleProductChannel = binding.CapabilityRoleProductChannel
	CapabilityRoleModelFamily    = binding.CapabilityRoleModelFamily

	CatalogOpCreate            = binding.CatalogOpCreate
	CatalogOpList              = binding.CatalogOpList
	CatalogOpUpdateBasicInfo   = binding.CatalogOpUpdateBasicInfo
	CatalogOpDelete            = binding.CatalogOpDelete
	CatalogOpPublish           = binding.CatalogOpPublish
	CatalogOpUnpublish         = binding.CatalogOpUnpublish
	CatalogOpArchive           = binding.CatalogOpArchive
	CatalogOpBindQuestionnaire = binding.CatalogOpBindQuestionnaire
	CatalogOpUpdateDefinition  = binding.CatalogOpUpdateDefinition
	CatalogOpPreview           = binding.CatalogOpPreview
	CatalogOpQRCode            = binding.CatalogOpQRCode

	SchemaVersionV1        = publishing.SchemaVersionV1
	SchemaVersionV2        = publishing.SchemaVersionV2
	RuleSetSchemaVersionV1 = legacypkg.RuleSetSchemaVersionV1

	RuleSetKindScale = legacypkg.RuleSetKindScale
	RuleSetKindMBTI  = legacypkg.RuleSetKindMBTI
	RuleSetKindSBTI  = legacypkg.RuleSetKindSBTI

	APIKindBehaviorAbility = legacypkg.APIKindBehaviorAbility

	ModelStatusDraft     = publishing.ModelStatusDraft
	ModelStatusPublished = publishing.ModelStatusPublished
	ModelStatusArchived  = publishing.ModelStatusArchived

	ValidationLevelError   = publishing.ValidationLevelError
	ValidationLevelWarning = publishing.ValidationLevelWarning
)

var (
	DefaultProductChannelFor        = binding.DefaultProductChannelFor
	ResolveProductChannel           = binding.ResolveProductChannel
	CompleteProductChannel          = binding.CompleteProductChannel
	AllProductChannels              = binding.AllProductChannels
	FallbackPersonalityDecisionKind = legacypkg.FallbackPersonalityDecisionKind

	AlgorithmFamilyFromDecisionKind = publishing.AlgorithmFamilyFromDecisionKind
	DecisionKindForIdentity         = publishing.DecisionKindForIdentity
	AlgorithmFamilyFromIdentity     = publishing.AlgorithmFamilyFromIdentity
	AllAlgorithmFamilies            = publishing.AllAlgorithmFamilies

	IsScalePayloadFormat               = publishing.IsScalePayloadFormat
	IsMBTIPayloadFormat                = publishing.IsMBTIPayloadFormat
	IsSBTIPayloadFormat                = publishing.IsSBTIPayloadFormat
	IsPersonalityTypologyPayloadFormat = publishing.IsPersonalityTypologyPayloadFormat
	AlgorithmFromTypologyPayload       = publishing.AlgorithmFromTypologyPayload
	PayloadFormatForBehavioralRating   = publishing.PayloadFormatForBehavioralRating
	PayloadFormatForCognitive          = publishing.PayloadFormatForCognitive
	IsBehavioralRatingPayloadFormat    = publishing.IsBehavioralRatingPayloadFormat
	IsCognitivePayloadFormat           = publishing.IsCognitivePayloadFormat
	DraftPayloadFormatForModel         = publishing.DraftPayloadFormatForModel

	FamilyCapabilityByKind = binding.FamilyCapabilityByKind
	RuntimeExecutableKinds = binding.RuntimeExecutableKinds

	LegacyKindMapping                      = legacypkg.LegacyKindMapping
	ModelDefinitionFromLegacy              = legacypkg.ModelDefinitionFromLegacy
	PublishedFromLegacy                    = legacypkg.PublishedFromLegacy
	LegacyFromPublished                    = legacypkg.LegacyFromPublished
	IsBehaviorAbilityProductChannelAPIKind = legacypkg.IsBehaviorAbilityProductChannelAPIKind
	BehaviorAbilityChannelModelFamilies    = legacypkg.BehaviorAbilityChannelModelFamilies
	IsBehaviorAbilityChannelFamily         = legacypkg.IsBehaviorAbilityChannelFamily
	ResolveBehaviorAbilityChannelFamily    = legacypkg.ResolveBehaviorAbilityChannelFamily

	NewAssessmentModel = publishing.NewAssessmentModel
	ParseModelStatus   = publishing.ParseModelStatus
)
