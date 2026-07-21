package reportprojection

import "time"

type ModelIdentity struct {
	Kind, SubKind, Algorithm, Code, Version, Title string
	ProductChannel, AlgorithmFamily                string
}

type ScoreValue struct {
	Kind, Label string
	Value       float64
	Max         *float64
}

type ResultLevel struct{ Code, Label, Severity string }

type NormReference struct {
	ScoreKind, TableVersion, FormVariant, Gender string
	Benchmark                                    float64
	MinAgeMonths, MaxAgeMonths                   int
}

type ModelRarity struct {
	Percent float64
	Label   string
	OneInX  int
}

type ModelExtra struct {
	Kind, TypeCode, TypeName, OneLiner, ImageURL string
	MatchPercent                                 float64
	IsSpecial                                    bool
	SpecialTrigger, Commentary                   string
	Rarity                                       *ModelRarity
}

type Dimension struct {
	FactorCode, FactorName string
	RawScore               float64
	MaxScore               *float64
	RiskLevel, Role        string
	DerivedScores          []ScoreValue
	Level                  *ResultLevel
	NormReference          *NormReference
	ParentCode             string
	HierarchyLevel         int
	SortOrder              int
	Description            string
	Suggestion             string
}

type Suggestion struct {
	Category, Content string
	FactorCode        *string
}

type Report struct {
	AssessmentID       uint64
	Model              ModelIdentity
	PrimaryScore       *ScoreValue
	Level              *ResultLevel
	Conclusion         string
	Dimensions         []Dimension
	Suggestions        []Suggestion
	ModelExtra         *ModelExtra
	CreatedAt          time.Time
	PresentationSource string
}

type ListResult struct {
	Items      []*Report
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}
