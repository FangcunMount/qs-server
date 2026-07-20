package task_performance

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scorerange"
)

// ScoreBasis selects which score value an ability rule matches against.
type ScoreBasis string

const (
	ScoreBasisRaw           ScoreBasis = "raw_score"
	ScoreBasisTScore        ScoreBasis = "t_score"
	ScoreBasisPercentile    ScoreBasis = "percentile"
	ScoreBasisStandardScore ScoreBasis = "standard_score"
)

// AbilityRange is one score-range outcome for ability projection.
type AbilityRange struct {
	Bound       scorerange.Bound
	Level       string
	OutcomeCode string
}

// AbilityRule projects an ability level onto a calculated factor dimension.
type AbilityRule struct {
	FactorCode string
	ScoreBasis ScoreBasis
	Primary    bool
	Ranges     []AbilityRange
}

// ApplyAbilityConclusions projects optional ability ranges onto calculated
// factor results. No configured rule means no change. Matching uses the shared
// ScoreRange endpoint contract. Level.Code prefers OutcomeCode when present.
// Primary rules (or total-role dimensions) promote to Result.Level.
func ApplyAbilityConclusions(result calculation.Result, rules []AbilityRule) calculation.Result {
	if len(rules) == 0 || len(result.Dimensions) == 0 {
		return result
	}
	for i := range result.Dimensions {
		dimension := &result.Dimensions[i]
		if dimension.Score == nil {
			continue
		}
		for _, rule := range rules {
			value, ok := scoreForBasis(*dimension, rule.ScoreBasis)
			if !ok || rule.FactorCode != dimension.Code {
				continue
			}
			matched, ok := matchAbilityRange(value, rule.Ranges)
			if !ok {
				continue
			}
			code := matched.OutcomeCode
			if code == "" {
				code = matched.Level
			}
			level := &calculation.ResultLevel{Code: code}
			dimension.Level = level
			if rule.Primary || dimension.Role == "total" {
				result.Level = level
			}
			break
		}
	}
	return result
}

func scoreForBasis(dimension calculation.DimensionResult, basis ScoreBasis) (float64, bool) {
	if basis == ScoreBasisRaw && dimension.Score != nil {
		return dimension.Score.Value, true
	}
	want := calculation.ScoreKind(basis)
	for _, value := range dimension.DerivedScores {
		if value.Kind == want {
			return value.Value, true
		}
	}
	return 0, false
}

func matchAbilityRange(score float64, ranges []AbilityRange) (AbilityRange, bool) {
	if len(ranges) == 0 {
		return AbilityRange{}, false
	}
	bounds := make([]scorerange.Bound, len(ranges))
	for i := range ranges {
		bounds[i] = ranges[i].Bound
	}
	index, ok := scorerange.MatchBounds(score, bounds)
	if !ok {
		return AbilityRange{}, false
	}
	return ranges[index], true
}
