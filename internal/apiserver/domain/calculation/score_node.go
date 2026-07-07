package calculation

// AggregationStrategy 命名如何 复合 node 从中推导分数： 子节点。
// 它是计算层原生枚举; callers 转换其 领域策略 为 it。
type AggregationStrategy string

const (
	AggregationNone        AggregationStrategy = "none"
	AggregationSum         AggregationStrategy = "sum"
	AggregationAverage     AggregationStrategy = "average"
	AggregationWeightedSum AggregationStrategy = "weighted_sum"
	AggregationLookup      AggregationStrategy = "lookup"
	AggregationCustom      AggregationStrategy = "custom"
)

// ScoreNode 是abstract 计分树节点 消费d 按 投影。
// It 携带no model-目录/因子 coupling: callers 转换其 own 领域。
// assets (例如 因子快照) 为 ScoreNode 在之前 调用 计算。
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
