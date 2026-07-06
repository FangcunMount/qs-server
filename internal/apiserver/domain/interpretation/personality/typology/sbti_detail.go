package typology

type SBTIDimensionReport struct {
	Code     string
	Name     string
	Model    string
	RawScore float64
	Level    string
}

type SBTIReportDetail struct {
	TypeCode       string
	TypeName       string
	OneLiner       string
	Pattern        string
	Similarity     float64
	ImageURL       string
	Rarity         SBTIRarityReport
	Dimensions     []SBTIDimensionReport
	Outcome        SBTIOutcomeReport
	Source         SBTISourceReport
	SpecialTrigger string
}

type SBTIRarityReport struct {
	Percent float64
	Label   string
	OneInX  int
}

type SBTIOutcomeReport struct {
	Code       string
	Name       string
	OneLiner   string
	Pattern    string
	Image      string
	Rarity     SBTIRarityReport
	IsSpecial  bool
	Trigger    string
	Commentary string
}

type SBTISourceReport struct {
	WikiRepo      string
	SourceSite    string
	License       string
	Attribution   string
	ImageBaseURL  string
	NonCommercial bool
}
