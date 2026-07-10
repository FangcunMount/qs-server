package assessmentstore

import (
	"fmt"

	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

func addFactorSnapshot(snapshot *scalesnapshot.ScaleSnapshot, factor scalesnapshot.FactorSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("scale snapshot or factor is nil")
	}
	for _, existing := range snapshot.Factors {
		if existing.Code == factor.Code {
			return fmt.Errorf("factor.code: factor code already exists")
		}
	}
	snapshot.Factors = append(snapshot.Factors, cloneFactorSnapshot(factor))
	return nil
}

func updateFactorSnapshot(snapshot *scalesnapshot.ScaleSnapshot, factor scalesnapshot.FactorSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("scale snapshot or factor is nil")
	}
	for i := range snapshot.Factors {
		if snapshot.Factors[i].Code != factor.Code {
			continue
		}
		snapshot.Factors[i] = cloneFactorSnapshot(factor)
		return nil
	}
	return fmt.Errorf("factor.code: factor not found")
}

func removeFactorSnapshot(snapshot *scalesnapshot.ScaleSnapshot, factorCode string) error {
	if snapshot == nil {
		return fmt.Errorf("scale snapshot is nil")
	}
	for i, factor := range snapshot.Factors {
		if factor.Code != factorCode {
			continue
		}
		snapshot.Factors = append(snapshot.Factors[:i], snapshot.Factors[i+1:]...)
		return nil
	}
	return fmt.Errorf("factor.code: factor not found")
}

func replaceFactorSnapshots(snapshot *scalesnapshot.ScaleSnapshot, factors []scalesnapshot.FactorSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("scale snapshot is nil")
	}
	out := make([]scalesnapshot.FactorSnapshot, 0, len(factors))
	for _, factor := range factors {
		out = append(out, cloneFactorSnapshot(factor))
	}
	snapshot.Factors = out
	return nil
}

func updateFactorInterpretRulesSnapshot(snapshot *scalesnapshot.ScaleSnapshot, factorCode string, rules []scalesnapshot.InterpretRuleSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("scale snapshot is nil")
	}
	for i := range snapshot.Factors {
		if snapshot.Factors[i].Code != factorCode {
			continue
		}
		snapshot.Factors[i].InterpretRules = cloneInterpretRuleSnapshots(rules)
		return nil
	}
	return fmt.Errorf("factor.code: factor not found")
}

func cloneFactorSnapshot(factor scalesnapshot.FactorSnapshot) scalesnapshot.FactorSnapshot {
	return scalesnapshot.FactorSnapshot{
		Code:            factor.Code,
		Title:           factor.Title,
		IsTotalScore:    factor.IsTotalScore,
		QuestionCodes:   append([]string(nil), factor.QuestionCodes...),
		ScoringStrategy: factor.ScoringStrategy,
		ScoringParams: scalesnapshot.ScoringParamsSnapshot{
			CntOptionContents: append([]string(nil), factor.ScoringParams.CntOptionContents...),
		},
		MaxScore:       cloneFloat64(factor.MaxScore),
		InterpretRules: cloneInterpretRuleSnapshots(factor.InterpretRules),
	}
}

func cloneInterpretRuleSnapshots(rules []scalesnapshot.InterpretRuleSnapshot) []scalesnapshot.InterpretRuleSnapshot {
	out := make([]scalesnapshot.InterpretRuleSnapshot, 0, len(rules))
	for _, rule := range rules {
		out = append(out, scalesnapshot.InterpretRuleSnapshot{
			Min:        rule.Min,
			Max:        rule.Max,
			RiskLevel:  rule.RiskLevel,
			Conclusion: rule.Conclusion,
			Suggestion: rule.Suggestion,
		})
	}
	return out
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
