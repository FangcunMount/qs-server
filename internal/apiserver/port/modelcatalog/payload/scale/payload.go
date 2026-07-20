package scale

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scorerange"
	portmodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// ParsePublishedPayload decodes a published scale payload envelope.
func ParsePublishedPayload(payload []byte) (*ScaleSnapshot, error) {
	var model ScaleSnapshot
	if err := json.Unmarshal(payload, &model); err != nil {
		return nil, fmt.Errorf("decode scale payload: %w", err)
	}
	return &model, nil
}

// ScaleSnapshot 已发布量表规则集 载荷（ruleset.scale.v1）。
type ScaleSnapshot struct {
	ID                   uint64
	Code                 string
	ScaleVersion         string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	Factors              []FactorSnapshot

	// PublishedRuntime is evaluation-only metadata from AssessmentSnapshot; not JSON payload.
	PublishedRuntime *portmodelcatalog.PublishedRuntimeMeta `json:"-"`
}

// ExecutionEnvelope carries non-factor metadata when projecting DefinitionV2
// into the published scale runtime DTO.
type ExecutionEnvelope struct {
	ID                   uint64
	Code                 string
	ScaleVersion         string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
}

func (s *ScaleSnapshot) IsPublished() bool {
	return s != nil && s.Status == "published"
}

func (s *ScaleSnapshot) FindFactor(code string) (*FactorSnapshot, bool) {
	if s == nil {
		return nil, false
	}
	for i := range s.Factors {
		if s.Factors[i].Code == code {
			return &s.Factors[i], true
		}
	}
	return nil, false
}

type FactorSnapshot struct {
	Code            string
	Title           string
	IsTotalScore    bool
	QuestionCodes   []string
	ScoringStrategy string
	ScoringParams   ScoringParamsSnapshot
	MaxScore        *float64
	InterpretRules  []InterpretRuleSnapshot
}

func (f FactorSnapshot) QuestionCount() int {
	return len(f.QuestionCodes)
}

func (f FactorSnapshot) FindInterpretRule(score float64) *InterpretRuleSnapshot {
	if len(f.InterpretRules) == 0 {
		return nil
	}
	bounds := make([]scorerange.Bound, len(f.InterpretRules))
	for i, rule := range f.InterpretRules {
		bounds[i] = scorerange.Bound{
			Min: rule.Min, Max: rule.Max, MaxInclusive: rule.MaxInclusive, UnboundedMax: rule.UnboundedMax,
		}
	}
	index, ok := scorerange.MatchBounds(score, bounds)
	if !ok {
		return nil
	}
	return &f.InterpretRules[index]
}

type ScoringParamsSnapshot struct {
	CntOptionContents []string
}

type InterpretRuleSnapshot struct {
	Min          float64 `json:"Min"`
	Max          float64 `json:"Max"`
	MaxInclusive bool    `json:"MaxInclusive,omitempty"`
	UnboundedMax bool    `json:"UnboundedMax,omitempty"`
	RiskLevel    string  `json:"RiskLevel"`
	Conclusion   string  `json:"Conclusion"`
	Suggestion   string  `json:"Suggestion"`
}

func (r InterpretRuleSnapshot) Matches(score float64) bool {
	return scorerange.Bound{
		Min: r.Min, Max: r.Max, MaxInclusive: r.MaxInclusive, UnboundedMax: r.UnboundedMax,
	}.Contains(score)
}
