package scale

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scorerange"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	portmodelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// ScaleSnapshot is a transient runtime projection of canonical DefinitionV2.
// It is never persisted as a model definition or published compatibility payload.
type ScaleSnapshot struct {
	ID                   uint64
	Code                 string
	ScaleVersion         string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	Factors              []FactorSnapshot
	Measure              *definition.MeasureSpec      `json:"Measure,omitempty"`
	InterpretationAssets *interpretationassets.Assets `json:"InterpretationAssets,omitempty"`

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

// HasCanonicalMeasure reports whether this transient projection carries its
// canonical DefinitionV2 measure.
func (s *ScaleSnapshot) HasCanonicalMeasure() bool {
	return s != nil && s.Measure != nil && (len(s.Measure.Factors) > 0 || len(s.Measure.Scoring) > 0 ||
		len(s.Measure.FactorGraph.Roots) > 0 || len(s.Measure.FactorGraph.Edges) > 0)
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
