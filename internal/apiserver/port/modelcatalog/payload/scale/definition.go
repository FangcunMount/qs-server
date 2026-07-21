package scale

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// ScaleSnapshotFromDefinition projects DefinitionV2 into the scale runtime DTO.
// Measure remains canonical; flat Factors are a temporary Calculation/report
// view materialized from the same DefinitionV2.
func ScaleSnapshotFromDefinition(env ExecutionEnvelope, def *definition.Definition) *ScaleSnapshot {
	if def == nil {
		return nil
	}
	snapshot := buildFromMeasureSpec(env, def.Measure)
	applyRiskConclusions(snapshot.Factors, def.Conclusions)
	if assets := def.ResolvedInterpretationAssets(); assets.IsMaterialized() {
		cloned := assets
		snapshot.InterpretationAssets = &cloned
	}
	return snapshot
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
	canonical := cloneMeasureSpec(measure)
	return &ScaleSnapshot{
		ID:                   env.ID,
		Code:                 env.Code,
		ScaleVersion:         env.ScaleVersion,
		Title:                env.Title,
		QuestionnaireCode:    env.QuestionnaireCode,
		QuestionnaireVersion: env.QuestionnaireVersion,
		Status:               env.Status,
		Factors:              factors,
		Measure:              &canonical,
	}
}

// cloneMeasureSpec deep-copies MeasureSpec so snapshot storage cannot mutate Definition.
func cloneMeasureSpec(measure definition.MeasureSpec) definition.MeasureSpec {
	out := definition.MeasureSpec{
		Factors: append([]factor.Factor(nil), measure.Factors...),
		FactorGraph: factor.FactorGraph{
			Roots: append([]string(nil), measure.FactorGraph.Roots...),
			Edges: append([]factor.FactorEdge(nil), measure.FactorGraph.Edges...),
		},
	}
	if len(measure.FactorGraph.SortOrders) > 0 {
		out.FactorGraph.SortOrders = make(map[string]int, len(measure.FactorGraph.SortOrders))
		for k, v := range measure.FactorGraph.SortOrders {
			out.FactorGraph.SortOrders[k] = v
		}
	}
	if len(measure.Scoring) == 0 {
		return out
	}
	out.Scoring = make([]factor.Scoring, 0, len(measure.Scoring))
	for _, rule := range measure.Scoring {
		cloned := factor.Scoring{
			FactorCode: rule.FactorCode,
			Strategy:   rule.Strategy,
			MaxScore:   cloneFloat64(rule.MaxScore),
			Constant:   rule.Constant,
			Params:     cloneScoringParams(rule.Params),
			Sources:    cloneScoringSources(rule.Sources),
		}
		if len(rule.Weights) > 0 {
			cloned.Weights = make(map[string]float64, len(rule.Weights))
			for k, v := range rule.Weights {
				cloned.Weights[k] = v
			}
		}
		out.Scoring = append(out.Scoring, cloned)
	}
	return out
}

func cloneScoringParams(params *factor.ScoringParams) *factor.ScoringParams {
	if params == nil {
		return nil
	}
	return &factor.ScoringParams{
		CntOptionContents: append([]string(nil), params.CntOptionContents...),
	}
}

func cloneScoringSources(sources []factor.ScoringSource) []factor.ScoringSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]factor.ScoringSource, 0, len(sources))
	for _, source := range sources {
		cloned := source
		if len(source.OptionScores) > 0 {
			cloned.OptionScores = make(map[string]float64, len(source.OptionScores))
			for k, v := range source.OptionScores {
				cloned.OptionScores[k] = v
			}
		}
		out = append(out, cloned)
	}
	return out
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
			Min:          rule.MinScore,
			Max:          rule.MaxScore,
			MaxInclusive: rule.MaxInclusive,
			UnboundedMax: rule.UnboundedMax,
			RiskLevel:    level,
			Conclusion:   rule.Summary,
			Suggestion:   rule.Description,
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
