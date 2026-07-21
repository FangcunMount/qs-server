package classification

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

// QuestionScoringMode selects the source of a question contribution's base score.
type QuestionScoringMode string

const (
	QuestionScoringModeQuestionScore  QuestionScoringMode = "question_score"
	QuestionScoringModeOptionOverride QuestionScoringMode = "option_override"
)

// AnswerContribution 映射问卷题目 到 叶子 因子 score。
type AnswerContribution struct {
	QuestionCode string
	ScoringMode  QuestionScoringMode
	Sign         float64
	Weight       float64
	OptionScores map[string]float64
}

// LeafScoringSpec 分数 叶子 因子 从 答案值。
type LeafScoringSpec struct {
	Constant      float64
	Contributions []AnswerContribution
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
