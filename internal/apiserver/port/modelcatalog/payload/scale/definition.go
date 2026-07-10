package scale

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
	conclusions := riskConclusionsFromScaleSnapshot(snapshot)
	return &definition.Definition{
		Measure:     measureSpecFromScaleSnapshot(snapshot),
		Calibration: definition.Calibration{},
		Conclusions: conclusions,
		Outcomes:    outcomesFromRiskConclusions(conclusions),
		ReportMap:   reportMapFromScaleSnapshot(snapshot),
	}
}

func reportMapFromScaleSnapshot(snapshot *ScaleSnapshot) definition.ReportMap {
	if snapshot == nil {
		return definition.ReportMap{}
	}
	return definition.ReportMap{Sections: []definition.ReportSection{{
		Code:       definition.ReportSectionKindFactorScores,
		Kind:       definition.ReportSectionKindFactorScores,
		SourceRefs: scaleFactorCodes(snapshot.Factors),
	}}}
}

func outcomesFromRiskConclusions(items []conclusion.Conclusion) []conclusion.Outcome {
	seen := make(map[string]struct{})
	out := make([]conclusion.Outcome, 0)
	for _, item := range items {
		risk, ok := item.(conclusion.RiskConclusion)
		if !ok {
			continue
		}
		for _, outcome := range risk.Outcomes {
			if outcome.Code == "" {
				continue
			}
			if _, exists := seen[outcome.Code]; exists {
				continue
			}
			seen[outcome.Code] = struct{}{}
			out = append(out, outcome)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ScaleSnapshotFromDefinition projects target definition layers back to the
// scale runtime DTO while keeping the published payload schema unchanged.
func ScaleSnapshotFromDefinition(env ExecutionEnvelope, def *definition.Definition) *ScaleSnapshot {
	if def == nil {
		return nil
	}
	snapshot := buildFromMeasureSpec(env, def.Measure)
	applyRiskConclusions(snapshot.Factors, def.Conclusions)
	return snapshot
}

func measureSpecFromScaleSnapshot(snapshot *ScaleSnapshot) definition.MeasureSpec {
	if snapshot == nil || snapshot.Factors == nil {
		return definition.MeasureSpec{}
	}
	factors := make([]factor.Factor, 0, len(snapshot.Factors))
	scoring := make([]factor.Scoring, 0, len(snapshot.Factors))
	for _, item := range snapshot.Factors {
		role := factor.FactorRoleDimension
		if item.IsTotalScore {
			role = factor.FactorRoleTotal
		}
		factors = append(factors, factor.Factor{
			Code:  item.Code,
			Title: item.Title,
			Role:  role,
		})
		if len(item.QuestionCodes) == 0 && item.ScoringStrategy == "" &&
			len(item.ScoringParams.CntOptionContents) == 0 && item.MaxScore == nil {
			continue
		}
		scoring = append(scoring, factor.Scoring{
			FactorCode: item.Code,
			Sources:    questionSources(item.QuestionCodes),
			Strategy:   factor.ScoringStrategy(item.ScoringStrategy),
			Params:     scoringParamsFromSnapshot(item.ScoringParams),
			MaxScore:   cloneFloat64(item.MaxScore),
		})
	}
	return definition.MeasureSpec{
		Factors: factors,
		FactorGraph: factor.FactorGraph{
			Roots: scaleFactorCodes(snapshot.Factors),
		},
		Scoring: scoring,
	}
}

func buildFromMeasureSpec(env ExecutionEnvelope, measure definition.MeasureSpec) *ScaleSnapshot {
	if env.Code == "" && len(measure.Factors) == 0 {
		return nil
	}
	factors := make([]FactorSnapshot, 0, len(measure.Factors))
	scoringByFactor := make(map[string]factor.Scoring, len(measure.Scoring))
	for _, scoring := range measure.Scoring {
		scoringByFactor[scoring.FactorCode] = scoring
	}
	for _, item := range measure.Factors {
		projected := FactorSnapshot{
			Code:         item.Code,
			Title:        item.Title,
			IsTotalScore: item.ResolvedRole() == factor.FactorRoleTotal,
		}
		if rule, ok := scoringByFactor[item.Code]; ok {
			applyScaleScoring(&projected, rule)
		}
		factors = append(factors, projected)
	}
	return &ScaleSnapshot{
		ID:                   env.ID,
		Code:                 env.Code,
		ScaleVersion:         env.ScaleVersion,
		Title:                env.Title,
		QuestionnaireCode:    env.QuestionnaireCode,
		QuestionnaireVersion: env.QuestionnaireVersion,
		Status:               env.Status,
		Factors:              factors,
	}
}

func questionSources(codes []string) []factor.ScoringSource {
	if len(codes) == 0 {
		return nil
	}
	out := make([]factor.ScoringSource, 0, len(codes))
	for _, code := range codes {
		out = append(out, factor.ScoringSource{Kind: factor.ScoringSourceQuestion, Code: code})
	}
	return out
}

func scaleFactorCodes(factors []FactorSnapshot) []string {
	if factors == nil {
		return nil
	}
	out := make([]string, 0, len(factors))
	for _, item := range factors {
		out = append(out, item.Code)
	}
	return out
}

func scoringParamsFromSnapshot(params ScoringParamsSnapshot) *factor.ScoringParams {
	if len(params.CntOptionContents) == 0 {
		return nil
	}
	return &factor.ScoringParams{
		CntOptionContents: append([]string(nil), params.CntOptionContents...),
	}
}

func applyScaleScoring(projected *FactorSnapshot, rule factor.Scoring) {
	projected.ScoringStrategy = rule.Strategy.String()
	if rule.Params != nil {
		projected.ScoringParams = ScoringParamsSnapshot{
			CntOptionContents: append([]string(nil), rule.Params.CntOptionContents...),
		}
	}
	projected.MaxScore = cloneFloat64(rule.MaxScore)
	if scaleSourceKind(rule.Sources) == factor.ScoringSourceQuestion {
		projected.QuestionCodes = scaleSourceCodes(rule.Sources)
	}
}

func scaleSourceKind(sources []factor.ScoringSource) factor.ScoringSourceKind {
	if len(sources) == 0 {
		return ""
	}
	return sources[0].Kind
}

func scaleSourceCodes(sources []factor.ScoringSource) []string {
	if len(sources) == 0 {
		return nil
	}
	out := make([]string, 0, len(sources))
	for _, source := range sources {
		out = append(out, source.Code)
	}
	return out
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

func applyRiskConclusions(factors []FactorSnapshot, conclusions []conclusion.Conclusion) {
	if len(factors) == 0 || len(conclusions) == 0 {
		return
	}
	rulesByFactor := make(map[string][]InterpretRuleSnapshot)
	for _, item := range conclusions {
		risk, ok := item.(conclusion.RiskConclusion)
		if !ok || len(risk.Rules) == 0 {
			continue
		}
		rulesByFactor[risk.FactorCode] = interpretRulesFromRisk(risk.Rules)
	}
	for i := range factors {
		if rules, ok := rulesByFactor[factors[i].Code]; ok {
			factors[i].InterpretRules = rules
		}
	}
}

func interpretRulesFromRisk(rules []conclusion.ScoreRangeOutcome) []InterpretRuleSnapshot {
	if len(rules) == 0 {
		return nil
	}
	out := make([]InterpretRuleSnapshot, 0, len(rules))
	for _, rule := range rules {
		level := rule.OutcomeCode
		if level == "" {
			level = rule.Title
		}
		out = append(out, InterpretRuleSnapshot{
			Min:        rule.MinScore,
			Max:        rule.MaxScore,
			RiskLevel:  level,
			Conclusion: rule.Summary,
			Suggestion: rule.Description,
		})
	}
	return out
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
