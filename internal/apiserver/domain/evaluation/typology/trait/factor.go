package trait

// FactorID identifies a node in a personality factor graph.
type FactorID string

// FactorKind distinguishes leaf factors (from answers) and composite factors.
type FactorKind string

const (
	FactorKindLeaf      FactorKind = "leaf"
	FactorKindComposite FactorKind = "composite"
)

// AggregationMethod defines how composite factors combine child scores.
type AggregationMethod string

const (
	AggregationSum         AggregationMethod = "sum"
	AggregationAvg         AggregationMethod = "avg"
	AggregationWeightedAvg AggregationMethod = "weighted_avg"
)

// OptionScoringPolicy controls how option-mapped answers are scored.
type OptionScoringPolicy string

const (
	// OptionScoringStrict requires a known option key in OptionScores.
	OptionScoringStrict OptionScoringPolicy = "strict"
	// OptionScoringCompat falls back to answer.Score when option key is unknown.
	OptionScoringCompat OptionScoringPolicy = "compat"
)

// AnswerContribution maps a questionnaire item to a leaf factor score.
type AnswerContribution struct {
	QuestionCode string
	Sign         float64
	OptionScores map[string]float64
}

// LeafScoringSpec scores a leaf factor from answer values.
type LeafScoringSpec struct {
	Constant      float64
	Contributions []AnswerContribution
	OptionScoring OptionScoringPolicy
}

// PersonalityFactor is a node in the factor hierarchy.
type PersonalityFactor struct {
	ID          FactorID
	Code        string
	Name        string
	Kind        FactorKind
	Children    []FactorID
	Aggregation AggregationMethod
	Weights     map[FactorID]float64
}
