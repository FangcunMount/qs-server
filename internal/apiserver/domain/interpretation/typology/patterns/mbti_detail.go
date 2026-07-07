package patterns

type MBTIDimensionReport struct {
	Code       string
	Name       string
	LeftPole   string
	RightPole  string
	RawScore   float64
	Preference string
	Strength   float64
}

type MBTIReportDetail struct {
	TypeCode     string
	TypeName     string
	OneLiner     string
	MatchPercent float64
	ImageURL     string
	Dimensions   []MBTIDimensionReport
	Profile      MBTIProfileReport
	Source       MBTISourceReport
}

type MBTIProfileReport struct {
	TypeCode    string
	TypeName    string
	OneLiner    string
	Summary     string
	Traits      []string
	Strengths   []string
	Weaknesses  []string
	Suggestions []string
	ImageURL    string
}

type MBTISourceReport struct {
	QuestionsRepo string
	SourceSite    string
	License       string
	Attribution   string
	NonCommercial bool
}
