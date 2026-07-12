package calculation

// DimensionKind 划分维度结果 独立于 scale 因子 semantics。
type DimensionKind string

const (
	DimensionKindFactor  DimensionKind = "factor"
	DimensionKindPole    DimensionKind = "pole"
	DimensionKindTrait   DimensionKind = "trait"
	DimensionKindIndex   DimensionKind = "index"
	DimensionKindAbility DimensionKind = "ability"
)

// ScoreKind 划分主 或 维度分 value。
type ScoreKind string

const (
	ScoreKindRawTotal      ScoreKind = "raw_total"
	ScoreKindMatchPercent  ScoreKind = "match_percent"
	ScoreKindTScore        ScoreKind = "t_score"
	ScoreKindPercentile    ScoreKind = "percentile"
	ScoreKindStandardScore ScoreKind = "standard_score"
)

// ScoreValue 是规范 score re呈现 on 计算结果。
type ScoreValue struct {
	Kind  ScoreKind
	Value float64
	Label string
	Max   *float64
}

// ResultLevel 是规范 等级 re呈现 on 计算结果。
type ResultLevel struct {
	Code     string
	Label    string
	Severity string
}

// DimensionResult 记录一个scored 维度 on 计算结果。
type DimensionResult struct {
	Code           string
	Name           string
	Kind           DimensionKind
	Role           string
	ParentCode     string
	HierarchyLevel int
	SortOrder      int
	Score          *ScoreValue
	DerivedScores  []ScoreValue
	Level          *ResultLevel
	Description    string
	Suggestion     string
}

// Result 是规范 output of 计算投影。
type Result struct {
	Primary      *ScoreValue
	Level        *ResultLevel
	PrimaryLabel string
	Dimensions   []DimensionResult
}
