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
// Ranges use half-open [min, max) with the last rule max-inclusive.
// Level.Code prefers OutcomeCode when present so code and display stay separated.
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
			for index, item := range rule.Rules {
				last := index == len(rule.Rules)-1
				if !scoreInHalfOpenRange(value, item.MinScore, item.MaxScore, last) {
					continue
				}
				code := item.OutcomeCode
				if code == "" {
					code = item.Level
				}
				label := item.Title
				if label == "" {
					label = item.Level
				}
				dimension.Level = &domainoutcome.ResultLevel{Code: code, Label: label}
				break
			}
		}
	}
	return outcome
}

func scoreInHalfOpenRange(score, min, max float64, lastInclusive bool) bool {
	if score < min {
		return false
	}
	if lastInclusive {
		return score <= max
	}
	return score < max
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
