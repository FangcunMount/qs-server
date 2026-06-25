package evaluation

import rulesetmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/mbti"

type MBTIDimensionResult struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	LeftPole   string  `json:"left_pole"`
	RightPole  string  `json:"right_pole"`
	RawScore   float64 `json:"raw_score"`
	Preference string  `json:"preference"`
	Strength   float64 `json:"strength"`
}

type MBTIResultDetail struct {
	TypeCode     string                          `json:"type_code"`
	TypeName     string                          `json:"type_name"`
	OneLiner     string                          `json:"one_liner"`
	MatchPercent float64                         `json:"match_percent"`
	ImageURL     string                          `json:"image_url"`
	Dimensions   []MBTIDimensionResult           `json:"dimensions"`
	Profile      rulesetmbti.TypeProfileSnapshot `json:"profile"`
	Source       rulesetmbti.SourceSnapshot      `json:"source"`
}
