package task_performance

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/calculationadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scorerange"
	calctask "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/task_performance"
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
// calculated cognitive factor results via the calculation kernel.
func ApplyAbilityConclusions(outcome *domainoutcome.Execution, rules []conclusion.AbilityConclusion) *domainoutcome.Execution {
	if outcome == nil || len(rules) == 0 {
		return outcome
	}
	calcResult := calculationadapter.CalcResultFromOutcome(outcome)
	if calcResult == nil {
		return outcome
	}
	applied := calctask.ApplyAbilityConclusions(*calcResult, abilityRulesFromConclusion(rules))
	merged := calculationadapter.MergeCalcResultIntoOutcome(outcome, &applied)
	if applied.Level != nil && applied.Level.Code != "" {
		if merged.Summary.Level == nil || *merged.Summary.Level == "" {
			code := applied.Level.Code
			merged.Summary.Level = &code
		}
	}
	return merged
}

func abilityRulesFromConclusion(rules []conclusion.AbilityConclusion) []calctask.AbilityRule {
	out := make([]calctask.AbilityRule, 0, len(rules))
	for _, rule := range rules {
		ranges := make([]calctask.AbilityRange, 0, len(rule.Rules))
		for _, r := range rule.Rules {
			ranges = append(ranges, calctask.AbilityRange{
				Bound: scorerange.Bound{
					Min: r.MinScore, Max: r.MaxScore,
					MaxInclusive: r.MaxInclusive, UnboundedMax: r.UnboundedMax,
				},
				Level:       r.Level,
				OutcomeCode: r.OutcomeCode,
			})
		}
		out = append(out, calctask.AbilityRule{
			FactorCode: rule.FactorCode,
			ScoreBasis: calctask.ScoreBasis(rule.ScoreBasis),
			Primary:    rule.Primary,
			Ranges:     ranges,
		})
	}
	return out
}
