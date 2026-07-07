package factor

// ChildrenAggregationStrategy names how a parent factor derives its score from children.
type ChildrenAggregationStrategy string

const (
	ChildrenAggregationNone        ChildrenAggregationStrategy = "none"
	ChildrenAggregationSum         ChildrenAggregationStrategy = "sum"
	ChildrenAggregationAverage     ChildrenAggregationStrategy = "avg"
	ChildrenAggregationWeightedSum ChildrenAggregationStrategy = "weighted_sum"
	ChildrenAggregationLookup      ChildrenAggregationStrategy = "lookup"
	ChildrenAggregationCustom      ChildrenAggregationStrategy = "custom"
)

func (s ChildrenAggregationStrategy) String() string { return string(s) }

func (s ChildrenAggregationStrategy) IsValid() bool {
	switch s {
	case ChildrenAggregationNone, ChildrenAggregationSum, ChildrenAggregationAverage,
		ChildrenAggregationWeightedSum, ChildrenAggregationLookup, ChildrenAggregationCustom:
		return true
	default:
		return false
	}
}

// ChildrenPolicy describes how a composite parent derives scores from child factors.
// Only factors with role=index (composite index) are required to define this in publish validation.
type ChildrenPolicy struct {
	Strategy ChildrenAggregationStrategy
	Children []string
	Weights  map[string]float64
}
