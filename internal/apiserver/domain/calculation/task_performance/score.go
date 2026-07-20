// Package task_performance implements Raven SPM and other task-performance
// pure scoring for cognitive models. Callers adapt Survey/ModelCatalog assets
// into the neutral ItemSet inputs and map calculation.Result to Outcome.
package task_performance

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"

// Item is one scored cognitive task item with a frozen correct option.
type Item struct {
	QuestionCode      string
	CorrectOptionCode string
}

// ItemSet is a scored group of items (for example SPM set A/B/C).
type ItemSet struct {
	Code  string
	Items []Item
}

// ScoreSPM scores Raven SPM item sets from frozen answer keys.
// Unanswered items contribute zero; elapsed time is not enforced here.
func ScoreSPM(answers map[string]string, sets []ItemSet, totalFactorCode string) calculation.Result {
	if answers == nil {
		answers = map[string]string{}
	}
	dimensions := make([]calculation.DimensionResult, 0, len(sets)+1)
	total := 0.0
	max := 0.0
	for _, set := range sets {
		setScore := 0.0
		setMax := float64(len(set.Items))
		for _, item := range set.Items {
			if answer, ok := answers[item.QuestionCode]; ok && answer == item.CorrectOptionCode {
				setScore++
			}
		}
		total += setScore
		max += setMax
		setMaxCopy := setMax
		dimensions = append(dimensions, calculation.DimensionResult{
			Code: set.Code,
			Name: set.Code,
			Kind: calculation.DimensionKindAbility,
			Role: "task_set",
			Score: &calculation.ScoreValue{
				Kind:  calculation.ScoreKindRawTotal,
				Value: setScore,
				Max:   &setMaxCopy,
			},
		})
	}
	maxCopy := max
	primary := &calculation.ScoreValue{
		Kind:  calculation.ScoreKindRawTotal,
		Value: total,
		Max:   &maxCopy,
	}
	if totalFactorCode != "" {
		dimensions = append(dimensions, calculation.DimensionResult{
			Code: totalFactorCode,
			Name: totalFactorCode,
			Kind: calculation.DimensionKindAbility,
			Role: "total",
			Score: &calculation.ScoreValue{
				Kind:  calculation.ScoreKindRawTotal,
				Value: total,
				Max:   &maxCopy,
			},
		})
	}
	return calculation.Result{Primary: primary, Dimensions: dimensions}
}
