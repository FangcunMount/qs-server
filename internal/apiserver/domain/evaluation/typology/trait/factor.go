package trait

// FactorID 标识node in personality 因子图。
type FactorID string

// FactorKind 区分叶子 因子 (从 answers) 和 复合 因子。
type FactorKind string

const (
	FactorKindLeaf      FactorKind = "leaf"
	FactorKindComposite FactorKind = "composite"
)

// AggregationMethod 定义如何复合 因子 组合子节点 分数。
type AggregationMethod string

const (
	AggregationSum         AggregationMethod = "sum"
	AggregationAvg         AggregationMethod = "avg"
	AggregationWeightedAvg AggregationMethod = "weighted_avg"
)

// OptionScoringPolicy 控制如何选项-mapped answers 是 scored。
type OptionScoringPolicy string

const (
	// OptionScoringStrict requires known 选项键 in 选项cores。
	OptionScoringStrict OptionScoringPolicy = "strict"
	// OptionScoringCompat falls back 到 answer.Score when 选项键 是 unknown。
	OptionScoringCompat OptionScoringPolicy = "compat"
)

// AnswerContribution 映射问卷题目 到 叶子 因子 score。
type AnswerContribution struct {
	QuestionCode string
	Sign         float64
	OptionScores map[string]float64
}

// LeafScoringSpec 分数 叶子 因子 从 答案值。
type LeafScoringSpec struct {
	Constant      float64
	Contributions []AnswerContribution
	OptionScoring OptionScoringPolicy
}

// PersonalityFactor 是node in 因子 层级。
type PersonalityFactor struct {
	ID          FactorID
	Code        string
	Name        string
	Kind        FactorKind
	Children    []FactorID
	Aggregation AggregationMethod
	Weights     map[FactorID]float64
}
