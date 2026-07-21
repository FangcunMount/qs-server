package identity

// Kind is the canonical assessment-model family identity.
type Kind string

const (
	KindScale            Kind = "scale"             // 量表
	KindTypology         Kind = "typology"          // 类型学
	KindBehavioralRating Kind = "behavioral_rating" // 行为评分
	KindCognitive        Kind = "cognitive"         // 认知测验
)

func (k Kind) String() string { return string(k) }

func (k Kind) IsValid() bool {
	switch k {
	case KindScale, KindTypology, KindBehavioralRating, KindCognitive:
		return true
	default:
		return false
	}
}

// SubKind narrows a family when multiple payload structures share it.
type SubKind string

const (
	SubKindEmpty    SubKind = ""
	SubKindTrait    SubKind = "trait"
	SubKindTypology SubKind = "typology"
)

func (s SubKind) String() string { return string(s) }

// Algorithm selects an evaluator algorithm within a model family.
type Algorithm string

const (
	AlgorithmScaleDefault        Algorithm = "scale_default"
	AlgorithmPersonalityTypology Algorithm = "personality_typology"
	AlgorithmBrief2              Algorithm = "brief2"
	// AlgorithmSPMSensory is Sensory Processing Measure. It deliberately does
	// not reuse AlgorithmSPM, which names Raven Standard Progressive Matrices.
	AlgorithmSPMSensory Algorithm = "spm_sensory"
	AlgorithmSPM        Algorithm = "spm"
)

func (a Algorithm) String() string { return string(a) }

// DecisionKind describes how calculated values map to outcomes.
type DecisionKind string

const (
	DecisionKindScoreRange      DecisionKind = "score_range"
	DecisionKindPoleComposition DecisionKind = "pole_composition"
	DecisionKindTraitProfile    DecisionKind = "trait_profile"
	DecisionKindNearestPattern  DecisionKind = "nearest_pattern"
	DecisionKindDominantFactor  DecisionKind = "dominant_factor"
	DecisionKindNormLookup      DecisionKind = "norm_lookup"
	DecisionKindAbilityLevel    DecisionKind = "ability_level"
)
