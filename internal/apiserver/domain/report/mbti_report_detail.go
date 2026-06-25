package report

import rulesetmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/mbti"

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
	Profile      rulesetmbti.TypeProfileSnapshot
	Source       rulesetmbti.SourceSnapshot
}
