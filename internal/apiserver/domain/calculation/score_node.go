package calculation

// AggregationStrategy names how a composite node derives its score from children.
// It is a calculation-native enum; callers translate their domain policies into it.
type AggregationStrategy string

const (
	AggregationNone        AggregationStrategy = "none"
	AggregationSum         AggregationStrategy = "sum"
	AggregationAverage     AggregationStrategy = "average"
	AggregationWeightedSum AggregationStrategy = "weighted_sum"
	AggregationLookup      AggregationStrategy = "lookup"
	AggregationCustom      AggregationStrategy = "custom"
)

// ScoreNode is an abstract scoring-tree node consumed by projections.
// It carries no model-catalog/factor coupling: callers translate their own domain
// assets (e.g. factor snapshots) into ScoreNode before invoking calculation.
type ScoreNode struct {
	Code        string
	Name        string
	Role        string
	Kind        DimensionKind
	ParentCode  string
	Level       int
	SortOrder   int
	Aggregation AggregationStrategy
	Children    []string
	Weights     map[string]float64
}
