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
	Kind           = binding.Kind
	SubKind        = binding.SubKind
	Algorithm      = binding.Algorithm
	DecisionKind   = binding.DecisionKind
	ProductChannel = binding.ProductChannel
	Product        = identitypkg.Product
	Identity       = identitypkg.Identity
	Family         = identitypkg.Family

	AlgorithmFamily = identitypkg.AlgorithmFamily
	ExecutionPath   = binding.ExecutionPath

	KindCapability        = binding.KindCapability
	ModelFamilyCapability = binding.ModelFamilyCapability
	CapabilityRole        = binding.CapabilityRole
	CatalogOperation      = binding.CatalogOperation

	QuestionnaireBinding = binding.QuestionnaireBinding

	// Mechanism re-exports (factor).
	Factor                      = factorpkg.Factor
	FactorGraph                 = factorpkg.FactorGraph
	FactorRole                  = factorpkg.FactorRole
	Scoring                     = factorpkg.Scoring
	ScoringStrategy             = factorpkg.ScoringStrategy
	ScoringParams               = factorpkg.ScoringParams
	ChildrenPolicy              = factorpkg.ChildrenPolicy
	ChildrenAggregationStrategy = factorpkg.ChildrenAggregationStrategy

	AssessmentModel         = assessmentmodelpkg.AssessmentModel
	NewAssessmentModelInput = assessmentmodelpkg.NewInput
	DefinitionPayload       = assessmentmodelpkg.DefinitionPayload
	ModelStatus             = assessmentmodelpkg.Status
	ValidationLevel         = assessmentmodelpkg.ValidationLevel
	DomainValidationIssue   = assessmentmodelpkg.DomainValidationIssue
	DomainValidationResult  = assessmentmodelpkg.DomainValidationResult

	Definition        = definitionpkg.Definition
	MeasureSpec       = definitionpkg.MeasureSpec
	Calibration       = definitionpkg.Calibration
	ReportMap         = definitionpkg.ReportMap
	ReportSection     = definitionpkg.ReportSection
	Norm              = normpkg.Norm
	NormRef           = normpkg.Ref
	Conclusion        = conclusionpkg.Conclusion
	Outcome           = conclusionpkg.Outcome
	ScoreRangeOutcome = conclusionpkg.ScoreRangeOutcome
	RiskConclusion    = conclusionpkg.RiskConclusion
	TypeConclusion    = conclusionpkg.TypeConclusion
	NormConclusion    = conclusionpkg.NormConclusion
	AbilityConclusion = conclusionpkg.AbilityConclusion
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

	ProductMedicalScale    = identitypkg.ProductMedicalScale
	ProductTypology        = identitypkg.ProductTypology
	ProductBehaviorAbility = identitypkg.ProductBehaviorAbility

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

	SchemaVersionV1 = payloadformatpkg.SchemaVersionV1
	SchemaVersionV2 = payloadformatpkg.SchemaVersionV2

	APIKindBehaviorAbility = binding.APIKindBehaviorAbility

	ModelStatusDraft     = assessmentmodelpkg.StatusDraft
	ModelStatusPublished = assessmentmodelpkg.StatusPublished
	ModelStatusArchived  = assessmentmodelpkg.StatusArchived

	ValidationLevelError   = assessmentmodelpkg.ValidationLevelError
	ValidationLevelWarning = assessmentmodelpkg.ValidationLevelWarning
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
	ProductFromChannel                     = identitypkg.ProductFromChannel
	NewIdentity                            = identitypkg.New
	FamilyFromDecisionKind                 = identitypkg.FamilyFromDecisionKind
	FamilyFromIdentity                     = identitypkg.FamilyFromIdentity

	AlgorithmFamilyFromDecisionKind = identitypkg.AlgorithmFamilyFromDecisionKind
	DecisionKindForIdentity         = identitypkg.DecisionKindForIdentity
	AlgorithmFamilyFromIdentity     = identitypkg.AlgorithmFamilyFromIdentity
	AllAlgorithmFamilies            = identitypkg.AllAlgorithmFamilies

	IsScalePayloadFormat               = payloadformatpkg.IsScalePayloadFormat
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

	NewAssessmentModel = assessmentmodelpkg.New
	ParseModelStatus   = assessmentmodelpkg.ParseStatus
)
