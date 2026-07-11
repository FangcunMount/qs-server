package classification

// DecisionKind 选择如何画像 vector 转成 结果。
type DecisionKind string

const (
	DecisionKindPoleComposition DecisionKind = "pole_composition"
	DecisionKindNearestPattern  DecisionKind = "nearest_pattern"
	DecisionKindTraitProfile    DecisionKind = "trait_profile"
)

// PoleSpec 解析维度 原始分 为 pole letter。
type PoleSpec struct {
	FactorID     FactorID
	LeftPole     string
	RightPole    string
	Threshold    float64
	MaxDeviation float64
}

// PatternCandidate 是一个selectable 类型学 结果。
type PatternCandidate struct {
	Code    string
	Label   string
	Pattern map[FactorID]string
}

// DecisionSpec 描述如何 derive 结果 从 画像 vector。
type DecisionSpec struct {
	Kind              DecisionKind
	Poles             []PoleSpec
	PatternOrder      []FactorID
	Patterns          []PatternCandidate
	LevelRule         LevelRule
	FallbackThreshold float64
	FallbackCode      string
}

// LevelRule 映射原始 因子 分数 到 离散等级 用于 模式匹配。
type LevelRule struct {
	LowMax  float64
	HighMin float64
}

// OutcomeCandidate is the selected personality result before mapping to Execution.
type OutcomeCandidate struct {
	Code        string
	Label       string
	Summary     string
	MatchScore  float64
	TraitScores map[FactorID]float64
}
