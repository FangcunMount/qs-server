package modelcatalog

// Kind is the canonical assessment model family.
type Kind string

const (
	KindScale       Kind = "scale"
	KindPersonality Kind = "personality"
	// Deprecated: behavior_ability is a product channel / legacy API filter only.
	// New models must use behavioral_rating or cognitive.
	KindBehaviorAbility  Kind = "behavior_ability"
	KindBehavioralRating Kind = "behavioral_rating"
	KindCognitive        Kind = "cognitive"
	KindCustom           Kind = "custom"

	// Migration-only flat kinds read from legacy envelopes; do not use in new writes.
	KindMBTIMigration Kind = "mbti"
	KindSBTIMigration Kind = "sbti"
)

// SubKind narrows a Kind when multiple payload shapes share the same family.
type SubKind string

const (
	SubKindEmpty    SubKind = ""
	SubKindTrait    SubKind = "trait"
	SubKindTypology SubKind = "typology"
)

// Algorithm selects the evaluation algorithm within a model family.
type Algorithm string

const (
	AlgorithmScaleDefault            Algorithm = "scale_default"
	AlgorithmPersonalityTypology     Algorithm = "personality_typology"
	AlgorithmBigFive                 Algorithm = "bigfive"
	AlgorithmMBTI                    Algorithm = "mbti"
	AlgorithmSBTI                    Algorithm = "sbti"
	AlgorithmBrief2                  Algorithm = "brief2"
	AlgorithmSPM                     Algorithm = "spm"
	AlgorithmBehavioralRatingDefault Algorithm = "behavioral_rating_default"
)

func (k Kind) String() string { return string(k) }

func (k Kind) IsValid() bool {
	switch k {
	case KindScale, KindPersonality, KindBehaviorAbility, KindBehavioralRating, KindCognitive, KindCustom,
		KindMBTIMigration, KindSBTIMigration:
		return true
	default:
		return false
	}
}

func (s SubKind) String() string { return string(s) }

func (a Algorithm) String() string { return string(a) }

// DecisionKind describes how raw scores map to outcomes.
type DecisionKind string

const (
	DecisionKindScoreRange      DecisionKind = "score_range"
	DecisionKindPoleComposition DecisionKind = "pole_composition"
	DecisionKindTraitProfile    DecisionKind = "trait_profile"
	DecisionKindNearestPattern  DecisionKind = "nearest_pattern"
	DecisionKindNormLookup      DecisionKind = "norm_lookup"
	DecisionKindAbilityLevel    DecisionKind = "ability_level"

	// Deprecated: use DecisionKindScoreRange.
	DecisionKindScoreRangeInterpretation DecisionKind = "score_range_interpretation"
)
