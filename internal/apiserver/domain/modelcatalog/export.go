package modelcatalog

// Root facade: thin aliases over mechanism subpackages. Deep-import factor/scoring/typology
// when mechanism-specific types are required; use this package for cross-mechanism identity
// and publishing surfaces.

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	factorpkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
	scoringsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	typologypkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

type (
	Kind           = binding.Kind
	SubKind        = binding.SubKind
	Algorithm      = binding.Algorithm
	DecisionKind   = binding.DecisionKind
	ProductChannel = binding.ProductChannel

	AlgorithmFamily = publishing.AlgorithmFamily
	ExecutionPath   = publishing.ExecutionPath

	KindCapability        = binding.KindCapability
	ModelFamilyCapability = binding.ModelFamilyCapability
	CapabilityRole        = binding.CapabilityRole
	CatalogOperation      = binding.CatalogOperation

	QuestionnaireBinding   = binding.QuestionnaireBinding
	ModelDefinition        = publishing.ModelDefinition
	DecisionSpec           = publishing.DecisionSpec
	SourceRef              = publishing.SourceRef
	PublishedModelSnapshot = publishing.PublishedModelSnapshot

	// Mechanism re-exports (factor).
	FactorSnapshot              = factorpkg.FactorSnapshot
	DefinitionBody              = factorpkg.DefinitionBody
	FactorRole                  = factorpkg.FactorRole
	ScoringStrategy             = factorpkg.ScoringStrategy
	ScoringParams               = factorpkg.ScoringParams
	ChildrenPolicy              = factorpkg.ChildrenPolicy
	ChildrenAggregationStrategy = factorpkg.ChildrenAggregationStrategy
	InterpretRule               = factorpkg.InterpretRule
	HierarchyIssue              = factorpkg.HierarchyIssue

	// Mechanism re-exports (typology).
	TypologyPayload = typologypkg.Payload
	RuntimeSpec     = typologypkg.RuntimeSpec
	ReportSpec      = typologypkg.ReportSpec
	FactorGraphSpec = typologypkg.FactorGraphSpec
	FactorSpec      = typologypkg.FactorSpec

	// Mechanism re-exports (scoring snapshot).
	ScaleSnapshot         = scoringsnapshot.ScaleSnapshot
	ScaleFactorSnapshot   = scoringsnapshot.FactorSnapshot
	InterpretRuleSnapshot = scoringsnapshot.InterpretRuleSnapshot

	AssessmentModel         = publishing.AssessmentModel
	NewAssessmentModelInput = publishing.NewAssessmentModelInput
	DefinitionPayload       = publishing.DefinitionPayload
	ModelStatus             = publishing.ModelStatus
	ValidationLevel         = publishing.ValidationLevel
	DomainValidationIssue   = publishing.DomainValidationIssue
	DomainValidationResult  = publishing.DomainValidationResult
)

const (
	KindScale            = binding.KindScale
	KindTypology         = binding.KindTypology
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
	ProductChannelTypology        = binding.ProductChannelTypology
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
	PayloadFormatTypologyV1                = publishing.PayloadFormatTypologyV1
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

	SchemaVersionV1 = publishing.SchemaVersionV1
	SchemaVersionV2 = publishing.SchemaVersionV2

	APIKindBehaviorAbility = binding.APIKindBehaviorAbility

	ModelStatusDraft     = publishing.ModelStatusDraft
	ModelStatusPublished = publishing.ModelStatusPublished
	ModelStatusArchived  = publishing.ModelStatusArchived

	ValidationLevelError   = publishing.ValidationLevelError
	ValidationLevelWarning = publishing.ValidationLevelWarning
)

var (
	NormalizeKind                          = binding.NormalizeKind
	KindsEqual                             = binding.KindsEqual
	IsTypologyKind                         = binding.IsTypologyKind
	KindQueryValues                        = binding.KindQueryValues
	NormalizeProductChannel                = binding.NormalizeProductChannel
	ProductChannelsEqual                   = binding.ProductChannelsEqual
	IsTypologyProductChannel               = binding.IsTypologyProductChannel
	ProductChannelQueryValues              = binding.ProductChannelQueryValues
	DefaultProductChannelFor               = binding.DefaultProductChannelFor
	ResolveProductChannel                  = binding.ResolveProductChannel
	CompleteProductChannel                 = binding.CompleteProductChannel
	AllProductChannels                     = binding.AllProductChannels
	LegacyKindMapping                      = binding.LegacyKindMapping
	IsBehaviorAbilityProductChannelAPIKind = binding.IsBehaviorAbilityProductChannelAPIKind
	BehaviorAbilityChannelModelFamilies    = binding.BehaviorAbilityChannelModelFamilies
	IsBehaviorAbilityChannelFamily         = binding.IsBehaviorAbilityChannelFamily
	ResolveBehaviorAbilityChannelFamily    = binding.ResolveBehaviorAbilityChannelFamily

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

	NewAssessmentModel = publishing.NewAssessmentModel
	ParseModelStatus   = publishing.ParseModelStatus

	BuildPublishedSnapshot                 = publishing.BuildPublishedSnapshot
	BuildScoringPublishedSnapshotFromScale = publishing.BuildScoringPublishedSnapshotFromScale
	ParseDefinitionBodyJSON                = factorpkg.ParseDefinitionBodyJSON
	FactorsFromDefinitionBodyJSON          = factorpkg.FactorsFromDefinitionBodyJSON
	ValidateDefinitionBodyForPublish       = factorpkg.ValidateDefinitionBodyForPublish
	ValidateDefinitionBodyJSONForPublish   = factorpkg.ValidateDefinitionBodyJSONForPublish
	ParsePublishedScalePayload             = scoringsnapshot.ParsePublishedPayload
)
