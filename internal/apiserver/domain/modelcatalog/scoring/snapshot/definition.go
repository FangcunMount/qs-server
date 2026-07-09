package snapshot

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// DefinitionFromScaleSnapshot materializes target definition layers from the
// legacy scale execution snapshot without changing the snapshot payload shape.
func DefinitionFromScaleSnapshot(snapshot *ScaleSnapshot) *definition.Definition {
	if snapshot == nil {
		return nil
	}
	legacyFactors := factor.LegacyFactorsFromSnapshots(snapshot.CanonicalFactors())
	measure, calibration := definition.MeasureAndCalibrationFromLegacyFactors(legacyFactors)
	return &definition.Definition{
		Measure:     measure,
		Calibration: calibration,
		Conclusions: riskConclusionsFromScaleSnapshot(snapshot),
	}
}

// ScaleSnapshotFromDefinition projects target definition layers back to the
// scale runtime DTO while keeping the published payload schema unchanged.
func ScaleSnapshotFromDefinition(env ExecutionEnvelope, def *definition.Definition) *ScaleSnapshot {
	if def == nil {
		return nil
	}
	legacyFactors := definition.LegacyFactorsFromMeasureSpec(def.Measure, def.Calibration)
	applyRiskConclusions(legacyFactors, def.Conclusions)
	return BuildFromLegacyFactors(env, legacyFactors)
}

func riskConclusionsFromScaleSnapshot(snapshot *ScaleSnapshot) []conclusion.Conclusion {
	if snapshot == nil {
		return nil
	}
	out := make([]conclusion.Conclusion, 0, len(snapshot.Factors))
	for _, item := range snapshot.Factors {
		if len(item.InterpretRules) == 0 {
			continue
		}
		out = append(out, conclusion.RiskConclusion{
			FactorCode: item.Code,
			Rules:      riskRulesFromInterpretRules(item.InterpretRules),
			Outcomes:   riskOutcomesFromRules(item.InterpretRules),
		})
	}
	return out
}

func riskRulesFromInterpretRules(rules []InterpretRuleSnapshot) []conclusion.ScoreRangeOutcome {
	if len(rules) == 0 {
		return nil
	}
	out := make([]conclusion.ScoreRangeOutcome, 0, len(rules))
	for _, rule := range rules {
		out = append(out, conclusion.ScoreRangeOutcome{
			MinScore:    rule.Min,
			MaxScore:    rule.Max,
			OutcomeCode: rule.RiskLevel,
			Title:       rule.RiskLevel,
			Summary:     rule.Conclusion,
			Description: rule.Suggestion,
		})
	}
	return out
}

func riskOutcomesFromRules(rules []InterpretRuleSnapshot) []conclusion.Outcome {
	if len(rules) == 0 {
		return nil
	}
	out := make([]conclusion.Outcome, 0, len(rules))
	for _, rule := range rules {
		out = append(out, conclusion.Outcome{
			Code:        rule.RiskLevel,
			Title:       rule.RiskLevel,
			Summary:     rule.Conclusion,
			Description: rule.Suggestion,
		})
	}
	return out
}

func applyRiskConclusions(factors []factor.LegacyFactor, conclusions []conclusion.Conclusion) {
	if len(factors) == 0 || len(conclusions) == 0 {
		return
	}
	rulesByFactor := make(map[string][]factor.ScoreRangeRule)
	for _, item := range conclusions {
		risk, ok := item.(conclusion.RiskConclusion)
		if !ok || len(risk.Rules) == 0 {
			continue
		}
		rulesByFactor[risk.FactorCode] = scoreRangeRulesFromRisk(risk.Rules)
	}
	for i := range factors {
		if rules, ok := rulesByFactor[factors[i].Code]; ok {
			factors[i].InterpretRules = rules
		}
	}
}

func scoreRangeRulesFromRisk(rules []conclusion.ScoreRangeOutcome) []factor.ScoreRangeRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]factor.ScoreRangeRule, 0, len(rules))
	for _, rule := range rules {
		level := rule.OutcomeCode
		if level == "" {
			level = rule.Title
		}
		out = append(out, factor.ScoreRangeRule{
			MinScore:   rule.MinScore,
			MaxScore:   rule.MaxScore,
			Level:      level,
			Conclusion: rule.Summary,
			Suggestion: rule.Description,
		})
	}
	return out
}
