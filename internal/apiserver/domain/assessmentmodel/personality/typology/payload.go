package typology

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"

// Payload is the unified personality typology model payload.
type Payload struct {
	Code                 string                    `json:"code"`
	Version              string                    `json:"version"`
	Title                string                    `json:"title"`
	QuestionnaireCode    string                    `json:"questionnaire_code"`
	QuestionnaireVersion string                    `json:"questionnaire_version"`
	Status               string                    `json:"status"`
	Source               Source                    `json:"source"`
	Algorithm            assessmentmodel.Algorithm `json:"algorithm"`
	DimensionOrder       []string                  `json:"dimension_order"`
	Dimensions           map[string]Dimension      `json:"dimensions"`
	QuestionMappings     []QuestionMapping         `json:"question_mappings"`
	Outcomes             []Outcome                 `json:"outcomes"`
	MatchingSpec         MatchingSpec              `json:"matching_spec"`
	SpecialTriggers      []SpecialTrigger          `json:"special_triggers"`
	Runtime              *RuntimeSpec              `json:"runtime,omitempty"`
}

// HasExplicitRuntime reports whether the payload carries an author-defined runtime spec.
func (p *Payload) HasExplicitRuntime() bool {
	return p != nil && p.Runtime != nil
}

func (p *Payload) IsPublished() bool {
	return p != nil && (p.Status == "" || p.Status == "published")
}

func (p *Payload) MatchesQuestionnaire(code, version string) bool {
	if p == nil || p.QuestionnaireCode != code {
		return false
	}
	return p.QuestionnaireVersion == "" || version == "" || p.QuestionnaireVersion == version
}

func (p *Payload) FindOutcome(code string) (Outcome, bool) {
	if p == nil {
		return Outcome{}, false
	}
	for _, outcome := range p.Outcomes {
		if outcome.Code == code {
			return outcome, true
		}
	}
	return Outcome{}, false
}

type Source struct {
	QuestionsRepo string `json:"questions_repo,omitempty"`
	WikiRepo      string `json:"wiki_repo,omitempty"`
	SourceSite    string `json:"source_site,omitempty"`
	License       string `json:"license,omitempty"`
	Attribution   string `json:"attribution,omitempty"`
	ImageBaseURL  string `json:"image_base_url,omitempty"`
	NonCommercial bool   `json:"non_commercial,omitempty"`
}

type Dimension struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	LeftPole  string  `json:"left_pole,omitempty"`
	RightPole string  `json:"right_pole,omitempty"`
	Constant  float64 `json:"constant,omitempty"`
	Threshold float64 `json:"threshold,omitempty"`
	Model     string  `json:"model,omitempty"`
}

type QuestionMapping struct {
	QuestionCode string             `json:"question_code"`
	Dimension    string             `json:"dimension"`
	Sign         float64            `json:"sign,omitempty"`
	OptionScores map[string]float64 `json:"option_scores,omitempty"`
}

type Outcome struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	OneLiner    string   `json:"one_liner,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Traits      []string `json:"traits,omitempty"`
	Strengths   []string `json:"strengths,omitempty"`
	Weaknesses  []string `json:"weaknesses,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
	ImageURL    string   `json:"image_url,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
	Image       string   `json:"image,omitempty"`
	Rarity      Rarity   `json:"rarity,omitempty"`
	IsSpecial   bool     `json:"is_special,omitempty"`
	Trigger     string   `json:"trigger,omitempty"`
	Commentary  string   `json:"commentary,omitempty"`
}

type Rarity struct {
	Percent float64 `json:"percent,omitempty"`
	Label   string  `json:"label,omitempty"`
	OneInX  int     `json:"one_in_x,omitempty"`
}

type MatchingSpec struct {
	Kind                        assessmentmodel.DecisionKind `json:"kind"`
	FallbackSimilarityThreshold float64                      `json:"fallback_similarity_threshold,omitempty"`
}

type SpecialTrigger struct {
	Code          string   `json:"code"`
	Name          string   `json:"name,omitempty"`
	Trigger       string   `json:"trigger"`
	OutcomeCode   string   `json:"outcome_code,omitempty"`
	QuestionCodes []string `json:"question_codes,omitempty"`
	OptionValues  []string `json:"option_values,omitempty"`
}
