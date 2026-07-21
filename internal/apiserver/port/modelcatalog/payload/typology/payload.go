package typology

import (
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	portmodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// Payload 是unified personality 类型学 model 载荷。
type Payload struct {
	Code                 string            `json:"code"`
	Version              string            `json:"version"`
	Title                string            `json:"title"`
	QuestionnaireCode    string            `json:"questionnaire_code"`
	QuestionnaireVersion string            `json:"questionnaire_version"`
	Status               string            `json:"status"`
	Source               Source            `json:"source"`
	Algorithm            binding.Algorithm `json:"algorithm"`
	Outcomes             []Outcome         `json:"outcomes"`
	Runtime              *RuntimeSpec      `json:"runtime,omitempty"`

	// PublishedRuntime is evaluation-only metadata from AssessmentSnapshot; not JSON payload.
	PublishedRuntime *portmodelcatalog.PublishedRuntimeMeta `json:"-"`
}

// HasExplicitRuntime 报告是否 载荷 携带 作者定义 运行时规格。
func (p *Payload) HasExplicitRuntime() bool {
	return p != nil && p.Runtime != nil
}

func (p *Payload) IsPublished() bool {
	if p == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(p.Status)) {
	case "", "published":
		return true
	default:
		return false
	}
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
