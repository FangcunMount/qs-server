package sbti

// ModelSnapshot 已发布 SBTI 规则集 payload（ruleset.sbti.v1）。
type ModelSnapshot struct {
	Code                        string                           `json:"code"`
	Version                     string                           `json:"version"`
	Title                       string                           `json:"title"`
	QuestionnaireCode           string                           `json:"questionnaire_code"`
	QuestionnaireVersion        string                           `json:"questionnaire_version"`
	Status                      string                           `json:"status"`
	Source                      SourceSnapshot                   `json:"source"`
	DimensionOrder              []string                         `json:"dimension_order"`
	Dimensions                  map[string]DimensionSnapshot     `json:"dimensions"`
	QuestionMappings            []QuestionMappingSnapshot        `json:"question_mappings"`
	NormalOutcomes              []OutcomeSnapshot                `json:"normal_outcomes"`
	SpecialOutcomes             []OutcomeSnapshot                `json:"special_outcomes"`
	FallbackSimilarityThreshold float64                          `json:"fallback_similarity_threshold"`
	DrinkTrigger                DrinkTriggerSnapshot             `json:"drink_trigger"`
}

func (m *ModelSnapshot) IsPublished() bool {
	return m != nil && (m.Status == "" || m.Status == "published")
}

func (m *ModelSnapshot) MatchesQuestionnaire(code, version string) bool {
	if m == nil || m.QuestionnaireCode != code {
		return false
	}
	return m.QuestionnaireVersion == "" || version == "" || m.QuestionnaireVersion == version
}

type SourceSnapshot struct {
	WikiRepo      string `json:"wiki_repo"`
	SourceSite    string `json:"source_site"`
	License       string `json:"license"`
	Attribution   string `json:"attribution"`
	ImageBaseURL  string `json:"image_base_url"`
	NonCommercial bool   `json:"non_commercial"`
}

type DimensionSnapshot struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Model string `json:"model"`
}

type QuestionMappingSnapshot struct {
	QuestionCode string             `json:"question_code"`
	Dimension    string             `json:"dimension"`
	OptionScores map[string]float64 `json:"option_scores"`
}

type OutcomeSnapshot struct {
	Code       string         `json:"code"`
	Name       string         `json:"name"`
	OneLiner   string         `json:"one_liner"`
	Pattern    string         `json:"pattern,omitempty"`
	Image      string         `json:"image"`
	Rarity     RaritySnapshot `json:"rarity"`
	IsSpecial  bool           `json:"is_special"`
	Trigger    string         `json:"trigger,omitempty"`
	Commentary string         `json:"commentary,omitempty"`
}

type RaritySnapshot struct {
	Percent float64 `json:"percent"`
	Label   string  `json:"label"`
	OneInX  int     `json:"one_in_x"`
}

type DrinkTriggerSnapshot struct {
	QuestionCodes []string `json:"question_codes"`
	OptionValues  []string `json:"option_values"`
}
