// Package binding is the mechanism-oriented home for questionnaire/task binding identity (§19).
package binding

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/capability"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

type (
	Kind           = identity.Kind
	SubKind        = identity.SubKind
	Algorithm      = identity.Algorithm
	DecisionKind   = identity.DecisionKind
	ProductChannel = identity.ProductChannel

	QuestionnaireBinding = catalog.QuestionnaireBinding

	KindCapability = capability.ModelFamilyCapability
	CapabilityRole = capability.CapabilityRole
)

const (
	KindScale            = identity.KindScale
	KindPersonality      = identity.KindPersonality
	KindBehavioralRating = identity.KindBehavioralRating
	KindCognitive        = identity.KindCognitive
	KindCustom           = identity.KindCustom

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

	DecisionKindScoreRange      = identity.DecisionKindScoreRange
	DecisionKindPoleComposition = identity.DecisionKindPoleComposition
	DecisionKindTraitProfile    = identity.DecisionKindTraitProfile
	DecisionKindNearestPattern  = identity.DecisionKindNearestPattern
	DecisionKindNormLookup      = identity.DecisionKindNormLookup
	DecisionKindAbilityLevel    = identity.DecisionKindAbilityLevel

	ProductChannelMedicalScale    = identity.ProductChannelMedicalScale
	ProductChannelPersonality     = identity.ProductChannelPersonality
	ProductChannelBehaviorAbility = identity.ProductChannelBehaviorAbility
	ProductChannelCognitive       = identity.ProductChannelCognitive
	ProductChannelCustom          = identity.ProductChannelCustom

	CapabilityRoleProductChannel = capability.CapabilityRoleProductChannel
	CapabilityRoleModelFamily    = capability.CapabilityRoleModelFamily
)

var (
	DefaultProductChannelFor = identity.DefaultProductChannelFor
	ResolveProductChannel    = identity.ResolveProductChannel
	CompleteProductChannel   = identity.CompleteProductChannel
	AllProductChannels       = identity.AllProductChannels

	FamilyCapabilityByKind = capability.FamilyCapabilityByKind
	RuntimeExecutableKinds = capability.RuntimeExecutableKinds
)
