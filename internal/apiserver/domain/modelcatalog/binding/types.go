package binding

import identitypkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"

// Canonical model identity types live in identity. These aliases preserve the
// binding package as the compatibility boundary for existing callers.
type (
	Kind         = identitypkg.Kind
	SubKind      = identitypkg.SubKind
	Algorithm    = identitypkg.Algorithm
	DecisionKind = identitypkg.DecisionKind
)

const (
	KindScale            = identitypkg.KindScale
	KindTypology         = identitypkg.KindTypology
	KindBehavioralRating = identitypkg.KindBehavioralRating
	KindCognitive        = identitypkg.KindCognitive

	SubKindEmpty    = identitypkg.SubKindEmpty
	SubKindTrait    = identitypkg.SubKindTrait
	SubKindTypology = identitypkg.SubKindTypology

	AlgorithmScaleDefault            = identitypkg.AlgorithmScaleDefault
	AlgorithmPersonalityTypology     = identitypkg.AlgorithmPersonalityTypology
	AlgorithmBigFive                 = identitypkg.AlgorithmBigFive
	AlgorithmMBTI                    = identitypkg.AlgorithmMBTI
	AlgorithmSBTI                    = identitypkg.AlgorithmSBTI
	AlgorithmBrief2                  = identitypkg.AlgorithmBrief2
	AlgorithmSPMSensory              = identitypkg.AlgorithmSPMSensory
	AlgorithmSPM                     = identitypkg.AlgorithmSPM
	AlgorithmBehavioralRatingDefault = identitypkg.AlgorithmBehavioralRatingDefault

	DecisionKindScoreRange      = identitypkg.DecisionKindScoreRange
	DecisionKindPoleComposition = identitypkg.DecisionKindPoleComposition
	DecisionKindTraitProfile    = identitypkg.DecisionKindTraitProfile
	DecisionKindNearestPattern  = identitypkg.DecisionKindNearestPattern
	DecisionKindNormLookup      = identitypkg.DecisionKindNormLookup
	DecisionKindAbilityLevel    = identitypkg.DecisionKindAbilityLevel
)
