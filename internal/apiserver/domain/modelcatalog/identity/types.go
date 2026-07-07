package identity

// Kind 是规范 assessment 模型家族。
type Kind string

const (
	KindScale            Kind = "scale"
	KindPersonality      Kind = "personality"
	KindBehavioralRating Kind = "behavioral_rating"
	KindCognitive        Kind = "cognitive"
	KindCustom           Kind = "custom"
)

// SubKind nar行 类型 when multiple 载荷 结构s share same 家族。
type SubKind string

const (
	SubKindEmpty    SubKind = ""
	SubKindTrait    SubKind = "trait"
	SubKindTypology SubKind = "typology"
)

// Algorithm 选择评估 算法 在 模型家族。
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
	case KindScale, KindPersonality, KindBehavioralRating, KindCognitive, KindCustom:
		return true
	default:
		return false
	}
}

func (s SubKind) String() string { return string(s) }

func (a Algorithm) String() string { return string(a) }

// DecisionKind 描述如何原始分 映射到 结果。
type DecisionKind string

const (
	DecisionKindScoreRange      DecisionKind = "score_range"
	DecisionKindPoleComposition DecisionKind = "pole_composition"
	DecisionKindTraitProfile    DecisionKind = "trait_profile"
	DecisionKindNearestPattern  DecisionKind = "nearest_pattern"
	DecisionKindNormLookup      DecisionKind = "norm_lookup"
	DecisionKindAbilityLevel    DecisionKind = "ability_level"

	// Deprecated: 使用 DecisionKindScoreRange。
	DecisionKindScoreRangeInterpretation DecisionKind = "score_range_interpretation"
)
