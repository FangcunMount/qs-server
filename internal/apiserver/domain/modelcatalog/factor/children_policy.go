package factor

// ChildrenAggregationStrategy 命名如何 父节点 因子 从中推导分数： 子节点。
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

// ChildrenPolicy 描述如何复合 父节点 从中推导分数： 子节点 因子。
// Only 因子 使用 角色=index (复合 index) 是 required 到 define 这个in publish 校验。
type ChildrenPolicy struct {
	Strategy ChildrenAggregationStrategy
	Children []string
	Weights  map[string]float64
}
