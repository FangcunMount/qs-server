package calculation

// DimensionKind classifies a dimension result independent of scale factor semantics.
type DimensionKind string

const (
	DimensionKindFactor  DimensionKind = "factor"
	DimensionKindPole    DimensionKind = "pole"
	DimensionKindTrait   DimensionKind = "trait"
	DimensionKindIndex   DimensionKind = "index"
	DimensionKindAbility DimensionKind = "ability"
)

// ScoreKind classifies a primary or dimension score value.
type ScoreKind string

const (
	ScoreKindRawTotal     ScoreKind = "raw_total"
	ScoreKindMatchPercent ScoreKind = "match_percent"
	ScoreKindTScore       ScoreKind = "t_score"
	ScoreKindPercentile   ScoreKind = "percentile"
)

// ScoreValue is the canonical score representation on a calculation result.
type ScoreValue struct {
	Kind  ScoreKind
	Value float64
	Label string
	Max   *float64
}

// ResultLevel is the canonical level representation on a calculation result.
type ResultLevel struct {
	Code     string
	Label    string
	Severity string
}

// DimensionResult records one scored dimension on a calculation result.
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

// Result is the canonical output of calculation projections.
type Result struct {
	Primary      *ScoreValue
	Level        *ResultLevel
	PrimaryLabel string
	Dimensions   []DimensionResult
}
