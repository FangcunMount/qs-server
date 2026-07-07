package typology

// MBTILegacyModel 是只读 ruleset.mbti.v1 载荷 结构 用于 迁移。
type MBTILegacyModel struct {
	Code                 string                         `json:"code"`
	Version              string                         `json:"version"`
	Title                string                         `json:"title"`
	QuestionnaireCode    string                         `json:"questionnaire_code"`
	QuestionnaireVersion string                         `json:"questionnaire_version"`
	Status               string                         `json:"status"`
	Source               MBTILegacySource               `json:"source"`
	DimensionOrder       []string                       `json:"dimension_order"`
	Dimensions           map[string]MBTILegacyDimension `json:"dimensions"`
	QuestionMappings     []MBTILegacyQuestionMapping    `json:"question_mappings"`
	TypeProfiles         []MBTILegacyTypeProfile        `json:"type_profiles"`
}

func (m *MBTILegacyModel) IsPublished() bool {
	return m != nil && (m.Status == "" || m.Status == "published")
}

func (m *MBTILegacyModel) MatchesQuestionnaire(code, version string) bool {
	if m == nil || m.QuestionnaireCode != code {
		return false
	}
	return m.QuestionnaireVersion == "" || version == "" || m.QuestionnaireVersion == version
}

func (m *MBTILegacyModel) FindTypeProfile(typeCode string) (MBTILegacyTypeProfile, bool) {
	if m == nil {
		return MBTILegacyTypeProfile{}, false
	}
	for _, profile := range m.TypeProfiles {
		if profile.TypeCode == typeCode {
			return profile, true
		}
	}
	return MBTILegacyTypeProfile{}, false
}

type MBTILegacySource struct {
	QuestionsRepo string `json:"questions_repo"`
	SourceSite    string `json:"source_site"`
	License       string `json:"license"`
	Attribution   string `json:"attribution"`
	NonCommercial bool   `json:"non_commercial"`
}

type MBTILegacyDimension struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	LeftPole  string  `json:"left_pole"`
	RightPole string  `json:"right_pole"`
	Constant  float64 `json:"constant"`
	Threshold float64 `json:"threshold"`
}

type MBTILegacyQuestionMapping struct {
	QuestionCode string  `json:"question_code"`
	Dimension    string  `json:"dimension"`
	Sign         float64 `json:"sign"`
}

type MBTILegacyTypeProfile struct {
	TypeCode    string   `json:"type_code"`
	TypeName    string   `json:"type_name"`
	OneLiner    string   `json:"one_liner"`
	Summary     string   `json:"summary"`
	Traits      []string `json:"traits"`
	Strengths   []string `json:"strengths"`
	Weaknesses  []string `json:"weaknesses"`
	Suggestions []string `json:"suggestions"`
	ImageURL    string   `json:"image_url"`
}
