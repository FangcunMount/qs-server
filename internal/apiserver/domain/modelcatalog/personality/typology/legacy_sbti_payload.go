package typology

// SBTILegacyModel 是只读 ruleset.sbti.v1 载荷 结构 用于 迁移。
type SBTILegacyModel struct {
	Code                        string                         `json:"code"`
	Version                     string                         `json:"version"`
	Title                       string                         `json:"title"`
	QuestionnaireCode           string                         `json:"questionnaire_code"`
	QuestionnaireVersion        string                         `json:"questionnaire_version"`
	Status                      string                         `json:"status"`
	Source                      SBTILegacySource               `json:"source"`
	DimensionOrder              []string                       `json:"dimension_order"`
	Dimensions                  map[string]SBTILegacyDimension `json:"dimensions"`
	QuestionMappings            []SBTILegacyQuestionMapping    `json:"question_mappings"`
	NormalOutcomes              []SBTILegacyOutcome            `json:"normal_outcomes"`
	SpecialOutcomes             []SBTILegacyOutcome            `json:"special_outcomes"`
	FallbackSimilarityThreshold float64                        `json:"fallback_similarity_threshold"`
	DrinkTrigger                SBTILegacyDrinkTrigger         `json:"drink_trigger"`
}

func (m *SBTILegacyModel) IsPublished() bool {
	return m != nil && (m.Status == "" || m.Status == "published")
}

func (m *SBTILegacyModel) MatchesQuestionnaire(code, version string) bool {
	if m == nil || m.QuestionnaireCode != code {
		return false
	}
	return m.QuestionnaireVersion == "" || version == "" || m.QuestionnaireVersion == version
}

type SBTILegacySource struct {
	WikiRepo      string `json:"wiki_repo"`
	SourceSite    string `json:"source_site"`
	License       string `json:"license"`
	Attribution   string `json:"attribution"`
	ImageBaseURL  string `json:"image_base_url"`
	NonCommercial bool   `json:"non_commercial"`
}

type SBTILegacyDimension struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Model string `json:"model"`
}

type SBTILegacyQuestionMapping struct {
	QuestionCode string             `json:"question_code"`
	Dimension    string             `json:"dimension"`
	OptionScores map[string]float64 `json:"option_scores"`
}

type SBTILegacyOutcome struct {
	Code       string           `json:"code"`
	Name       string           `json:"name"`
	OneLiner   string           `json:"one_liner"`
	Pattern    string           `json:"pattern,omitempty"`
	Image      string           `json:"image"`
	Rarity     SBTILegacyRarity `json:"rarity"`
	IsSpecial  bool             `json:"is_special"`
	Trigger    string           `json:"trigger,omitempty"`
	Commentary string           `json:"commentary,omitempty"`
}

type SBTILegacyRarity struct {
	Percent float64 `json:"percent"`
	Label   string  `json:"label"`
	OneInX  int     `json:"one_in_x"`
}

type SBTILegacyDrinkTrigger struct {
	QuestionCodes []string `json:"question_codes"`
	OptionValues  []string `json:"option_values"`
}
