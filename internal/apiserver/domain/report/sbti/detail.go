package sbti

type DimensionReport struct {
	Code     string
	Name     string
	Model    string
	RawScore float64
	Level    string
}

type ReportDetail struct {
	TypeCode       string
	TypeName       string
	OneLiner       string
	Pattern        string
	Similarity     float64
	ImageURL       string
	Rarity         RarityReport
	Dimensions     []DimensionReport
	Outcome        OutcomeReport
	Source         SourceReport
	SpecialTrigger string
}

type RarityReport struct {
	Percent float64
	Label   string
	OneInX  int
}

type OutcomeReport struct {
	Code       string
	Name       string
	OneLiner   string
	Pattern    string
	Image      string
	Rarity     RarityReport
	IsSpecial  bool
	Trigger    string
	Commentary string
}

type SourceReport struct {
	WikiRepo      string
	SourceSite    string
	License       string
	Attribution   string
	ImageBaseURL  string
	NonCommercial bool
}
