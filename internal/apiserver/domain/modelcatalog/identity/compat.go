// Package identity is a transitional compat facade; canonical definitions live in binding.
package identity

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

type (
	Kind           = binding.Kind
	SubKind        = binding.SubKind
	Algorithm      = binding.Algorithm
	DecisionKind   = binding.DecisionKind
	ProductChannel = binding.ProductChannel
)

const (
	KindScale            = binding.KindScale
	KindPersonality      = binding.KindPersonality
	KindBehavioralRating = binding.KindBehavioralRating
	KindCognitive        = binding.KindCognitive
	KindCustom           = binding.KindCustom

	SubKindEmpty    = binding.SubKindEmpty
	SubKindTrait    = binding.SubKindTrait
	SubKindTypology = binding.SubKindTypology

	AlgorithmScaleDefault            = binding.AlgorithmScaleDefault
	AlgorithmPersonalityTypology     = binding.AlgorithmPersonalityTypology
	AlgorithmBigFive                 = binding.AlgorithmBigFive
	AlgorithmMBTI                    = binding.AlgorithmMBTI
	AlgorithmSBTI                    = binding.AlgorithmSBTI
	AlgorithmBrief2                  = binding.AlgorithmBrief2
	AlgorithmSPM                     = binding.AlgorithmSPM
	AlgorithmBehavioralRatingDefault = binding.AlgorithmBehavioralRatingDefault

	DecisionKindScoreRange      = binding.DecisionKindScoreRange
	DecisionKindPoleComposition = binding.DecisionKindPoleComposition
	DecisionKindTraitProfile    = binding.DecisionKindTraitProfile
	DecisionKindNearestPattern  = binding.DecisionKindNearestPattern
	DecisionKindNormLookup      = binding.DecisionKindNormLookup
	DecisionKindAbilityLevel    = binding.DecisionKindAbilityLevel

	ProductChannelMedicalScale    = binding.ProductChannelMedicalScale
	ProductChannelPersonality     = binding.ProductChannelPersonality
	ProductChannelBehaviorAbility = binding.ProductChannelBehaviorAbility
	ProductChannelCognitive       = binding.ProductChannelCognitive
	ProductChannelScreening       = binding.ProductChannelScreening
	ProductChannelFollowup        = binding.ProductChannelFollowup
	ProductChannelCustom          = binding.ProductChannelCustom
)

var (
	ErrInvalidArgument       = binding.ErrInvalidArgument
	DefaultProductChannelFor = binding.DefaultProductChannelFor
	ResolveProductChannel    = binding.ResolveProductChannel
	CompleteProductChannel   = binding.CompleteProductChannel
	AllProductChannels       = binding.AllProductChannels
)
