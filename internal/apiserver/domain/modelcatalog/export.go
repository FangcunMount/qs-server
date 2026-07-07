package modelcatalog

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/capability"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	legacypkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/routing"
)

type (
	Kind           = identity.Kind
	SubKind        = identity.SubKind
	Algorithm      = identity.Algorithm
	DecisionKind   = identity.DecisionKind
	ProductChannel = identity.ProductChannel

	AlgorithmFamily = routing.AlgorithmFamily
	ExecutionPath   = routing.ExecutionPath

	KindCapability   = capability.KindCapability
	CapabilityRole   = capability.CapabilityRole
	CatalogOperation = capability.CatalogOperation

	QuestionnaireBinding   = catalog.QuestionnaireBinding
	ModelDefinition        = catalog.ModelDefinition
	DecisionSpec           = catalog.DecisionSpec
	SourceRef              = catalog.SourceRef
	PublishedModelSnapshot = catalog.PublishedModelSnapshot

	AssessmentModel         = catalog.AssessmentModel
	NewAssessmentModelInput = catalog.NewAssessmentModelInput
	DefinitionPayload       = catalog.DefinitionPayload
	ModelStatus             = catalog.ModelStatus
	ValidationLevel         = catalog.ValidationLevel
	DomainValidationIssue   = catalog.DomainValidationIssue
	DomainValidationResult  = catalog.DomainValidationResult

	RuleSetKind       = legacypkg.RuleSetKind
	Definition        = legacypkg.Definition
	Snapshot          = legacypkg.Snapshot
	RuleSetDefinition = legacypkg.RuleSetDefinition
	RuleSetSnapshot   = legacypkg.RuleSetSnapshot
)

const (
	KindScale            = identity.KindScale
	KindPersonality      = identity.KindPersonality
	KindBehavioralRating = identity.KindBehavioralRating
	KindCognitive        = identity.KindCognitive
	KindCustom           = identity.KindCustom
	KindMBTIMigration    = identity.KindMBTIMigration
	KindSBTIMigration    = identity.KindSBTIMigration

	SubKindEmpty    = identity.SubKindEmpty
	SubKindTypology = identity.SubKindTypology
	SubKindTrait    = identity.SubKindTrait

	AlgorithmScaleDefault            = identity.AlgorithmScaleDefault
	AlgorithmPersonalityTypology     = identity.AlgorithmPersonalityTypology
	AlgorithmBigFive                 = identity.AlgorithmBigFive
	AlgorithmMBTI                    = identity.AlgorithmMBTI
	AlgorithmSBTI                    = identity.AlgorithmSBTI
	AlgorithmBrief2                  = identity.AlgorithmBrief2
	AlgorithmSPM                     = identity.AlgorithmSPM
	AlgorithmBehavioralRatingDefault = identity.AlgorithmBehavioralRatingDefault

	DecisionKindScoreRange                            = identity.DecisionKindScoreRange
	DecisionKindPoleComposition                       = identity.DecisionKindPoleComposition
	DecisionKindTraitProfile                          = identity.DecisionKindTraitProfile
	DecisionKindNearestPattern                        = identity.DecisionKindNearestPattern
	DecisionKindNormLookup                            = identity.DecisionKindNormLookup
	DecisionKindAbilityLevel                          = identity.DecisionKindAbilityLevel
	DecisionKindScoreRangeInterpretation DecisionKind = "score_range_interpretation"

	ProductChannelMedicalScale    = identity.ProductChannelMedicalScale
	ProductChannelPersonality     = identity.ProductChannelPersonality
	ProductChannelBehaviorAbility = identity.ProductChannelBehaviorAbility
	ProductChannelCognitive       = identity.ProductChannelCognitive
	ProductChannelCustom          = identity.ProductChannelCustom

	AlgorithmFamilyFactorScoring        = routing.AlgorithmFamilyFactorScoring
	AlgorithmFamilyFactorClassification = routing.AlgorithmFamilyFactorClassification
	AlgorithmFamilyFactorNorm           = routing.AlgorithmFamilyFactorNorm
	AlgorithmFamilyTaskPerformance      = routing.AlgorithmFamilyTaskPerformance

	ExecutionPathNone                       = routing.ExecutionPathNone
	ExecutionPathScaleDescriptor            = routing.ExecutionPathScaleDescriptor
	ExecutionPathTypologyDescriptor         = routing.ExecutionPathTypologyDescriptor
	ExecutionPathBehavioralRatingDescriptor = routing.ExecutionPathBehavioralRatingDescriptor
	ExecutionPathCognitiveDescriptor        = routing.ExecutionPathCognitiveDescriptor

	PayloadFormatAssessmentScaleV1         = routing.PayloadFormatAssessmentScaleV1
	PayloadFormatPersonalityTypologyV1     = routing.PayloadFormatPersonalityTypologyV1
	PayloadFormatBehavioralRatingDefaultV1 = routing.PayloadFormatBehavioralRatingDefaultV1
	PayloadFormatBehavioralRatingBrief2V1  = routing.PayloadFormatBehavioralRatingBrief2V1
	PayloadFormatCognitiveDefaultV1        = routing.PayloadFormatCognitiveDefaultV1
	PayloadFormatCognitiveSPMV1            = routing.PayloadFormatCognitiveSPMV1
	PayloadFormatScaleV1                   = routing.PayloadFormatScaleV1
	PayloadFormatMBTIV1                    = routing.PayloadFormatMBTIV1
	PayloadFormatSBTIV1                    = routing.PayloadFormatSBTIV1
	PayloadFormatScaleV1Legacy             = routing.PayloadFormatScaleV1Legacy
	PayloadFormatMBTIV1Legacy              = routing.PayloadFormatMBTIV1Legacy
	PayloadFormatSBTIV1Legacy              = routing.PayloadFormatSBTIV1Legacy

	CapabilityRoleProductChannel = capability.CapabilityRoleProductChannel
	CapabilityRoleModelFamily    = capability.CapabilityRoleModelFamily

	CatalogOpCreate            = capability.CatalogOpCreate
	CatalogOpList              = capability.CatalogOpList
	CatalogOpUpdateBasicInfo   = capability.CatalogOpUpdateBasicInfo
	CatalogOpDelete            = capability.CatalogOpDelete
	CatalogOpPublish           = capability.CatalogOpPublish
	CatalogOpUnpublish         = capability.CatalogOpUnpublish
	CatalogOpArchive           = capability.CatalogOpArchive
	CatalogOpBindQuestionnaire = capability.CatalogOpBindQuestionnaire
	CatalogOpUpdateDefinition  = capability.CatalogOpUpdateDefinition
	CatalogOpPreview           = capability.CatalogOpPreview
	CatalogOpQRCode            = capability.CatalogOpQRCode

	SchemaVersionV1        = catalog.SchemaVersionV1
	SchemaVersionV2        = catalog.SchemaVersionV2
	RuleSetSchemaVersionV1 = legacypkg.RuleSetSchemaVersionV1

	RuleSetKindScale = legacypkg.RuleSetKindScale
	RuleSetKindMBTI  = legacypkg.RuleSetKindMBTI
	RuleSetKindSBTI  = legacypkg.RuleSetKindSBTI

	APIKindBehaviorAbility = legacypkg.APIKindBehaviorAbility

	ModelStatusDraft     = catalog.ModelStatusDraft
	ModelStatusPublished = catalog.ModelStatusPublished
	ModelStatusArchived  = catalog.ModelStatusArchived

	ValidationLevelError   = catalog.ValidationLevelError
	ValidationLevelWarning = catalog.ValidationLevelWarning
)

var (
	DefaultProductChannelFor        = identity.DefaultProductChannelFor
	ResolveProductChannel           = identity.ResolveProductChannel
	CompleteProductChannel          = identity.CompleteProductChannel
	AllProductChannels              = identity.AllProductChannels
	FallbackPersonalityDecisionKind = identity.FallbackPersonalityDecisionKind

	AlgorithmFamilyFromDecisionKind = routing.AlgorithmFamilyFromDecisionKind
	DecisionKindForIdentity         = routing.DecisionKindForIdentity
	AlgorithmFamilyFromIdentity     = routing.AlgorithmFamilyFromIdentity
	AllAlgorithmFamilies            = routing.AllAlgorithmFamilies

	IsScalePayloadFormat               = routing.IsScalePayloadFormat
	IsMBTIPayloadFormat                = routing.IsMBTIPayloadFormat
	IsSBTIPayloadFormat                = routing.IsSBTIPayloadFormat
	IsPersonalityTypologyPayloadFormat = routing.IsPersonalityTypologyPayloadFormat
	AlgorithmFromTypologyPayload       = routing.AlgorithmFromTypologyPayload
	PayloadFormatForBehavioralRating   = routing.PayloadFormatForBehavioralRating
	PayloadFormatForCognitive          = routing.PayloadFormatForCognitive
	IsBehavioralRatingPayloadFormat    = routing.IsBehavioralRatingPayloadFormat
	IsCognitivePayloadFormat           = routing.IsCognitivePayloadFormat
	DraftPayloadFormatForModel         = routing.DraftPayloadFormatForModel

	DefaultCapabilities         = capability.DefaultCapabilities
	ModelFamilyCapabilities     = capability.ModelFamilyCapabilities
	ModelFamilyCapabilityByKind = capability.ModelFamilyCapabilityByKind
	CapabilityByKind            = capability.CapabilityByKind
	CapabilityByAPIKind         = capability.CapabilityByAPIKind
	RuntimeExecutableKinds      = capability.RuntimeExecutableKinds

	LegacyKindMapping                      = legacypkg.LegacyKindMapping
	ModelDefinitionFromLegacy              = legacypkg.ModelDefinitionFromLegacy
	PublishedFromLegacy                    = legacypkg.PublishedFromLegacy
	LegacyFromPublished                    = legacypkg.LegacyFromPublished
	IsBehaviorAbilityProductChannelAPIKind = legacypkg.IsBehaviorAbilityProductChannelAPIKind
	BehaviorAbilityChannelModelFamilies    = legacypkg.BehaviorAbilityChannelModelFamilies
	IsBehaviorAbilityChannelFamily         = legacypkg.IsBehaviorAbilityChannelFamily
	ResolveBehaviorAbilityChannelFamily    = legacypkg.ResolveBehaviorAbilityChannelFamily

	NewAssessmentModel = catalog.NewAssessmentModel
	ParseModelStatus   = catalog.ParseModelStatus
)
