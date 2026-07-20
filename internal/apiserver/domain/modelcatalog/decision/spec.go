// Package decision holds DecisionSpec facts used for outcome determination.
// Presentation copy (title/summary/description) belongs in interpretationassets.
package decision

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scorerange"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

// ScoreRangeRule is the decision-only view of a score-range conclusion rule.
// It intentionally omits Title/Summary/Description (MC-R016).
type ScoreRangeRule struct {
	MinScore     float64
	MaxScore     float64
	MaxInclusive bool
	UnboundedMax bool
	Level        string
	OutcomeCode  string
}

// Bound returns the shared endpoint contract.
func (r ScoreRangeRule) Bound() scorerange.Bound {
	return scorerange.Bound{
		Min:          r.MinScore,
		Max:          r.MaxScore,
		MaxInclusive: r.MaxInclusive,
		UnboundedMax: r.UnboundedMax,
	}
}

// MatchOutcomeCode returns the OutcomeCode of the first matching rule.
func MatchOutcomeCode(score float64, rules []ScoreRangeRule) (string, bool) {
	if len(rules) == 0 {
		return "", false
	}
	bounds := make([]scorerange.Bound, len(rules))
	for i := range rules {
		bounds[i] = rules[i].Bound()
	}
	index, ok := scorerange.MatchBounds(score, bounds)
	if !ok {
		return "", false
	}
	code := rules[index].OutcomeCode
	if code == "" {
		code = rules[index].Level
	}
	return code, code != ""
}

// TypeDecisionFact is the decision-only view of typology decision config.
type TypeDecisionFact struct {
	Kind                        binding.DecisionKind
	FallbackSimilarityThreshold float64
	FallbackCode                string
	TopK                        int
}

// OutcomeRef is a stable outcome identity without presentation copy.
type OutcomeRef struct {
	Code string
}

// Spec is the logical DecisionSpec projected from DefinitionV2 Conclusions/Outcomes.
// Storage shape still lives on Definition; this type is the consumption boundary.
type Spec struct {
	ScoreRanges     []FactorScoreRanges
	TypeDecision    *TypeDecisionFact
	OutcomeRefs     []OutcomeRef
	SpecialOutcomes []string // special-rule / profile outcome codes referenced by decision
}

// IsMaterialized reports whether stored decision facts are present (MC-R016).
func (s Spec) IsMaterialized() bool {
	return len(s.ScoreRanges) > 0 || len(s.OutcomeRefs) > 0 || s.TypeDecision != nil || len(s.SpecialOutcomes) > 0
}

// FactorScoreRanges groups decision rules for one factor.
type FactorScoreRanges struct {
	FactorCode string
	Kind       string // risk | norm | ability
	Primary    bool
	Rules      []ScoreRangeRule
}
