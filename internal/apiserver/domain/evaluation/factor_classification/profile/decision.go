package profile

// DecisionKind selects how a profile vector becomes an outcome.
type DecisionKind string

const (
	DecisionKindPoleComposition DecisionKind = "pole_composition"
	DecisionKindNearestPattern  DecisionKind = "nearest_pattern"
	DecisionKindTraitProfile    DecisionKind = "trait_profile"
)

// PoleSpec resolves a dimension raw score into a pole letter.
type PoleSpec struct {
	FactorID     FactorID
	LeftPole     string
	RightPole    string
	Threshold    float64
	MaxDeviation float64
}

// PatternCandidate is one selectable typology outcome.
type PatternCandidate struct {
	Code    string
	Label   string
	Pattern map[FactorID]string
}

// DecisionSpec describes how to derive an outcome from a profile vector.
type DecisionSpec struct {
	Kind              DecisionKind
	Poles             []PoleSpec
	PatternOrder      []FactorID
	Patterns          []PatternCandidate
	LevelRule         LevelRule
	FallbackThreshold float64
	FallbackCode      string
}

// LevelRule maps raw factor scores to discrete levels for pattern matching.
type LevelRule struct {
	LowMax  float64
	HighMin float64
}

// OutcomeCandidate is the selected personality outcome before mapping to AssessmentOutcome.
type OutcomeCandidate struct {
	Code        string
	Label       string
	Summary     string
	MatchScore  float64
	TraitScores map[FactorID]float64
}
