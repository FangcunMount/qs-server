package assessmentstore

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
)

func addFactorSnapshot(snapshot *scalesnapshot.ScaleSnapshot, factor *scaledefinition.Factor) error {
	if snapshot == nil || factor == nil {
		return fmt.Errorf("scale snapshot or factor is nil")
	}
	code := factor.GetCode().String()
	for _, existing := range snapshot.Factors {
		if existing.Code == code {
			return scaledefinition.ToError([]scaledefinition.ValidationError{{
				Field:   "factor.code",
				Message: "factor code already exists",
			}})
		}
	}
	snapshot.Factors = append(snapshot.Factors, legacyadapter.ScaleFactorSnapshotFromMedicalScale(factor.Snapshot()))
	return nil
}

func updateFactorSnapshot(snapshot *scalesnapshot.ScaleSnapshot, factor *scaledefinition.Factor) error {
	if snapshot == nil || factor == nil {
		return fmt.Errorf("scale snapshot or factor is nil")
	}
	code := factor.GetCode().String()
	for i := range snapshot.Factors {
		if snapshot.Factors[i].Code != code {
			continue
		}
		snapshot.Factors[i] = legacyadapter.ScaleFactorSnapshotFromMedicalScale(factor.Snapshot())
		return nil
	}
	return scaledefinition.ToError([]scaledefinition.ValidationError{{
		Field:   "factor.code",
		Message: "factor not found",
	}})
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
	return scaledefinition.ToError([]scaledefinition.ValidationError{{
		Field:   "factor.code",
		Message: "factor not found",
	}})
}

func replaceFactorSnapshots(snapshot *scalesnapshot.ScaleSnapshot, factors []*scaledefinition.Factor) error {
	if snapshot == nil {
		return fmt.Errorf("scale snapshot is nil")
	}
	out := make([]scalesnapshot.FactorSnapshot, 0, len(factors))
	for _, factor := range factors {
		out = append(out, legacyadapter.ScaleFactorSnapshotFromMedicalScale(factor.Snapshot()))
	}
	snapshot.Factors = out
	return nil
}

func updateFactorInterpretRulesSnapshot(snapshot *scalesnapshot.ScaleSnapshot, factorCode string, rules []scaledefinition.InterpretationRule) error {
	if snapshot == nil {
		return fmt.Errorf("scale snapshot is nil")
	}
	for i := range snapshot.Factors {
		if snapshot.Factors[i].Code != factorCode {
			continue
		}
		snapshotRules := make([]scalesnapshot.InterpretRuleSnapshot, 0, len(rules))
		for _, rule := range rules {
			scoreRange := rule.GetScoreRange()
			snapshotRules = append(snapshotRules, scalesnapshot.InterpretRuleSnapshot{
				Min:        scoreRange.Min(),
				Max:        scoreRange.Max(),
				RiskLevel:  string(rule.GetRiskLevel()),
				Conclusion: rule.GetConclusion(),
				Suggestion: rule.GetSuggestion(),
			})
		}
		snapshot.Factors[i].InterpretRules = snapshotRules
		return nil
	}
	return scaledefinition.ToError([]scaledefinition.ValidationError{{
		Field:   "factor.code",
		Message: "factor not found",
	}})
}
