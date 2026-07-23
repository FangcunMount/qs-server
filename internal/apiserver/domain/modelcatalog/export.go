package modelcatalog

// Root facade: thin aliases over mechanism subpackages. Deep-import factor/scoring/typology
// when mechanism-specific types are required; use this package for cross-mechanism identity,
// and read-model surfaces.

import (
	assessmentmodelpkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	conclusionpkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	definitionpkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	factorpkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	identitypkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	normpkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
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

	AlgorithmWritePolicy = identitypkg.AlgorithmWritePolicy

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
	ChildrenPolicy              = factorpkg.ChildrenPolicy
	ChildrenAggregationStrategy = factorpkg.ChildrenAggregationStrategy

	AssessmentModel         = assessmentmodelpkg.AssessmentModel
	NewAssessmentModelInput = assessmentmodelpkg.NewInput
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

	AlgorithmScaleDefault        = identitypkg.AlgorithmScaleDefault
	AlgorithmPersonalityTypology = identitypkg.AlgorithmPersonalityTypology
	AlgorithmBrief2              = identitypkg.AlgorithmBrief2
	AlgorithmSPMSensory          = identitypkg.AlgorithmSPMSensory
	AlgorithmSPM                 = identitypkg.AlgorithmSPM

	DecisionKindScoreRange      = identitypkg.DecisionKindScoreRange
	DecisionKindPoleComposition = identitypkg.DecisionKindPoleComposition
	DecisionKindTraitProfile    = identitypkg.DecisionKindTraitProfile
	DecisionKindNearestPattern  = identitypkg.DecisionKindNearestPattern
	DecisionKindDominantFactor  = identitypkg.DecisionKindDominantFactor
	DecisionKindNormLookup      = identitypkg.DecisionKindNormLookup
	DecisionKindAbilityLevel    = identitypkg.DecisionKindAbilityLevel

	ScoringSourceQuestion      = factorpkg.ScoringSourceQuestion
	ScoringSourceFactor        = factorpkg.ScoringSourceFactor
	ScoringStrategyWeightedAvg = factorpkg.ScoringStrategyWeightedAvg

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

	CapabilityRoleProductChannel = binding.CapabilityRoleProductChannel
	CapabilityRoleModelFamily    = binding.CapabilityRoleModelFamily

	// SchemaVersionV2 is the only persisted Definition schema.
	SchemaVersionV2 = "2"

	ModelStatusDraft      = assessmentmodelpkg.StatusDraft
	ModelStatusPublished  = assessmentmodelpkg.StatusPublished
	ModelStatusArchived   = assessmentmodelpkg.StatusArchived
	ReleaseStatusActive   = assessmentmodelpkg.ReleaseStatusActive
	ReleaseStatusArchived = assessmentmodelpkg.ReleaseStatusArchived

	ValidationLevelError   = assessmentmodelpkg.ValidationLevelError
	ValidationLevelWarning = assessmentmodelpkg.ValidationLevelWarning
)

var (
	ErrLegacyRuntimeIdentity  = identitypkg.ErrLegacyRuntimeIdentity
	DefaultProductChannelFor  = binding.DefaultProductChannelFor
	CanonicalSubKindFor       = binding.CanonicalSubKindFor
	IsCanonicalProductChannel = binding.IsCanonicalProductChannel
	IsCanonicalSubKind        = binding.IsCanonicalSubKind
	ResolveProductChannel     = binding.ResolveProductChannel
	CompleteProductChannel    = binding.CompleteProductChannel
	ValidateNewProductChannel = binding.ValidateNewProductChannel
	HasValidationErrors       = binding.HasValidationErrors
	AllProductChannels        = binding.AllProductChannels
	ProductFromChannel        = binding.ProductFromChannel
	NewIdentity               = identitypkg.New
	FamilyFromDecisionKind    = identitypkg.FamilyFromDecisionKind
	FamilyFromIdentity        = identitypkg.FamilyFromIdentity

	AlgorithmFamilyFromDecisionKind   = identitypkg.AlgorithmFamilyFromDecisionKind
	ResolveLegacyRuntime              = identitypkg.ResolveLegacyRuntime
	DecisionKindForIdentity           = binding.DecisionKindForIdentity
	AlgorithmFamilyFromIdentity       = binding.AlgorithmFamilyFromIdentity
	CompatibleAlgorithmBinding        = identitypkg.CompatibleAlgorithmBinding
	CompatibleIdentity                = identitypkg.CompatibleIdentity
	AllAlgorithmFamilies              = identitypkg.AllAlgorithmFamilies
	ResolveRuntimeIdentity            = identitypkg.ResolveRuntimeIdentity
	ClassifyAlgorithmWritePolicy      = identitypkg.ClassifyAlgorithmWritePolicy
	IsCanonicalPublishAlgorithm       = identitypkg.IsCanonicalPublishAlgorithm
	AuditIdentityWritePolicy          = identitypkg.AuditIdentityWritePolicy
	CanonicalTypologyPublishAlgorithm = identitypkg.CanonicalTypologyPublishAlgorithm
	AlgorithmWriteCanonical           = identitypkg.AlgorithmWriteCanonical
	AlgorithmWriteDraftOK             = identitypkg.AlgorithmWriteDraftOK
	AlgorithmWriteUnknown             = identitypkg.AlgorithmWriteUnknown

	FamilyCapabilityByKind = binding.FamilyCapabilityByKind
	RuntimeExecutableKinds = binding.RuntimeExecutableKinds

	NewAssessmentModel     = assessmentmodelpkg.New
	ParseModelStatus       = assessmentmodelpkg.ParseStatus
	NormalizeReleaseStatus = assessmentmodelpkg.NormalizeReleaseStatus
	ValidateDefinition     = definitionpkg.Validate
)
