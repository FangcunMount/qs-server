package definition

import (
	"slices"
	"sort"
)

// InterpretationRules 是一组解读规则，负责维护规则集级别的不变量。
type InterpretationRules struct {
	items []InterpretationRule
}

func NewInterpretationRules(items []InterpretationRule) (InterpretationRules, error) {
	rules := slices.Clone(items)
	sort.SliceStable(rules, func(i, j int) bool {
		return rules[i].GetScoreRange().Min() < rules[j].GetScoreRange().Min()
	})
	for i, rule := range rules {
		if !rule.IsValid() {
			return InterpretationRules{}, newError(ErrorKindInvalidArgument, "interpretation rule %d is invalid", i+1)
		}
		if i == 0 {
			continue
		}
		previous := rules[i-1].GetScoreRange()
		current := rule.GetScoreRange()
		if previous.Max() > current.Min() {
			return InterpretationRules{}, newError(ErrorKindInvalidArgument, "interpretation rules overlap: [%.2f, %.2f) and [%.2f, %.2f)", previous.Min(), previous.Max(), current.Min(), current.Max())
		}
	}
	return InterpretationRules{items: rules}, nil
}

func MustInterpretationRules(items []InterpretationRule) InterpretationRules {
	rules, err := NewInterpretationRules(items)
	if err != nil {
		panic(err)
	}
	return rules
}

func (r InterpretationRules) Items() []InterpretationRule {
	return slices.Clone(r.items)
}

func (r InterpretationRules) Len() int {
	return len(r.items)
}

func (r InterpretationRules) Match(score float64) (InterpretationRule, bool) {
	for _, rule := range r.items {
		if rule.Matches(score) {
			return rule, true
		}
	}
	return InterpretationRule{}, false
}

func (r InterpretationRules) WithAppended(rule InterpretationRule) (InterpretationRules, error) {
	items := r.Items()
	items = append(items, rule)
	return NewInterpretationRules(items)
}
