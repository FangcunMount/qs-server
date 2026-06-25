package mbti

type DimensionReport struct {
	Code       string
	Name       string
	LeftPole   string
	RightPole  string
	RawScore   float64
	Preference string
	Strength   float64
}

type ReportDetail struct {
	TypeCode     string
	TypeName     string
	OneLiner     string
	MatchPercent float64
	ImageURL     string
	Dimensions   []DimensionReport
	Profile      ProfileReport
	Source       SourceReport
}

type ProfileReport struct {
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

type SourceReport struct {
	QuestionsRepo string
	SourceSite    string
	License       string
	Attribution   string
	NonCommercial bool
}
