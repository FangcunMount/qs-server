package task_performance

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/calculationadapter"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
)

// NormalizeOutcome reuses the shared calculationadapter outcome bridge for cognitive/task_performance runs.
func NormalizeOutcome(outcome *domainoutcome.Execution) *domainoutcome.Execution {
	if outcome == nil {
		return nil
	}
	return calculationadapter.MergeCalcResultIntoOutcome(outcome, calculationadapter.CalcResultFromOutcome(outcome))
}

// ApplyAbilityConclusions projects optional DefinitionV2 ability ranges onto
// calculated cognitive factor results. No configured rule means no change.
// Matching uses the shared ScoreRange endpoint contract (half-open by default;
// explicit max_inclusive / unbounded_max; legacy last-inclusive when unset).
// Level.Code prefers OutcomeCode when present so code and display stay separated.
// Primary ability conclusions (or total-role dimensions) promote to Execution.Level.
func ApplyAbilityConclusions(outcome *domainoutcome.Execution, rules []conclusion.AbilityConclusion) *domainoutcome.Execution {
	if outcome == nil || len(rules) == 0 {
		return outcome
	}
	for i := range outcome.Dimensions {
		dimension := &outcome.Dimensions[i]
		if dimension.Score == nil {
			continue
		}
		for _, rule := range rules {
			value, ok := scoreForBasis(*dimension, rule.ScoreBasis)
			if !ok || rule.FactorCode != dimension.Code {
				continue
			}
			matched, ok := conclusion.MatchScoreRangeOutcomes(value, rule.Rules)
			if !ok {
				continue
			}
			code := matched.OutcomeCode
			if code == "" {
				code = matched.Level
			}
			// Decision path keeps OutcomeCode only; presentation is resolved at Interpretation (MC-R016).
			level := &domainoutcome.ResultLevel{Code: code}
			dimension.Level = level
			if rule.Primary || dimension.Role == "total" {
				outcome.Level = level
				if outcome.Summary.Level == nil && code != "" {
					levelCode := code
					outcome.Summary.Level = &levelCode
				}
			}
			break
		}
	}
	return outcome
}

func scoreForBasis(dimension domainoutcome.DimensionResult, basis conclusion.ScoreBasis) (float64, bool) {
	if basis == conclusion.ScoreBasisRaw && dimension.Score != nil {
		return dimension.Score.Value, true
	}
	want := domainoutcome.ScoreKind(basis)
	for _, value := range dimension.DerivedScores {
		if value.Kind == want {
			return value.Value, true
		}
	}
	return 0, false
}
