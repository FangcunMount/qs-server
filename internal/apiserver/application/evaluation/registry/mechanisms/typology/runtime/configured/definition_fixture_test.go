package configured_test

import (
	"sort"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// canonicalDefinitionFixture turns an explicit test graph into the canonical
// DefinitionV2 supplied to production scoring. It intentionally rejects the
// removed flat typology layout.
func canonicalDefinitionFixture(t *testing.T, payload *modeltypology.Payload) *modeldefinition.Definition {
	t.Helper()
	if payload == nil || payload.Runtime == nil || !payload.Runtime.FactorGraph.HasExplicitFactorGraph() {
		t.Fatal("test fixture requires an explicit canonical factor graph")
	}
	runtime := payload.Runtime
	keys := make([]string, 0, len(runtime.FactorGraph.Factors))
	for key := range runtime.FactorGraph.Factors {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	measure := modeldefinition.MeasureSpec{
		Factors:     make([]factor.Factor, 0, len(keys)),
		Scoring:     make([]factor.Scoring, 0, len(keys)),
		FactorGraph: factor.FactorGraph{Roots: append([]string(nil), runtime.FactorGraph.Roots...), SortOrders: map[string]int{}},
	}
	for order, key := range keys {
		spec := runtime.FactorGraph.Factors[key]
		code := spec.Code
		if code == "" {
			code = spec.ID
		}
		if code == "" {
			code = key
		}
		role := factor.FactorRoleDimension
		if spec.Kind == modeltypology.FactorSpecKindComposite {
			role = factor.FactorRoleIndex
		}
		measure.Factors = append(measure.Factors, factor.Factor{Code: code, Title: spec.Name, Role: role})
		measure.FactorGraph.SortOrders[code] = order + 1

		rule := factor.Scoring{FactorCode: code, Constant: spec.Constant}
		if spec.Kind == modeltypology.FactorSpecKindComposite {
			rule.Strategy = factor.ScoringStrategySum
			switch spec.Aggregation {
			case modeltypology.FactorAggregationAvg:
				rule.Strategy = factor.ScoringStrategyAvg
			case modeltypology.FactorAggregationWeightedAvg:
				rule.Strategy = factor.ScoringStrategyWeightedAvg
			}
			for _, child := range spec.Children {
				childSpec := runtime.FactorGraph.Factors[child]
				childCode := childSpec.Code
				if childCode == "" {
					childCode = child
				}
				rule.Sources = append(rule.Sources, factor.ScoringSource{Kind: factor.ScoringSourceFactor, Code: childCode})
				measure.FactorGraph.Edges = append(measure.FactorGraph.Edges, factor.FactorEdge{ParentCode: code, ChildCode: childCode})
			}
		} else {
			rule.Strategy = factor.ScoringStrategySum
			for _, source := range spec.Contributions {
				mode := factor.QuestionScoringMode(source.ScoringMode)
				if mode == "" {
					if len(source.OptionScores) > 0 {
						mode = factor.QuestionScoringModeOptionOverride
					} else {
						mode = factor.QuestionScoringModeQuestionScore
					}
				}
				sign, weight := source.Sign, source.Weight
				if sign == 0 {
					sign = 1
				}
				if weight == 0 {
					weight = 1
				}
				rule.Sources = append(rule.Sources, factor.ScoringSource{
					Kind: factor.ScoringSourceQuestion, Code: source.QuestionCode,
					ScoringMode: mode, Sign: sign, Weight: weight, OptionScores: cloneFixtureScores(source.OptionScores),
				})
			}
		}
		measure.Scoring = append(measure.Scoring, rule)
	}

	decision := conclusion.TypeDecision{
		Kind: runtime.Decision.Kind, FallbackSimilarityThreshold: runtime.Decision.FallbackSimilarityThreshold,
		FallbackCode: runtime.Decision.FallbackCode, TopK: runtime.Decision.TopK,
	}
	if runtime.Decision.LevelRule != nil {
		decision.LevelRule = &conclusion.TypeLevelRule{LowMax: runtime.Decision.LevelRule.LowMax, HighMin: runtime.Decision.LevelRule.HighMin}
	}
	for _, root := range runtime.FactorGraph.Roots {
		meta := runtime.FactorGraph.Dimensions[root]
		decision.Poles = append(decision.Poles, conclusion.TypePole{
			FactorCode: root, LeftPole: meta.LeftPole, RightPole: meta.RightPole, Threshold: meta.Threshold, Model: meta.Model,
		})
	}
	detailKey := runtime.OutcomeMapping.DetailAdapterKey
	if detailKey == "" {
		switch runtime.OutcomeMapping.DetailKind {
		case modeltypology.OutcomeDetailTraitProfile:
			detailKey = modeltypology.DetailAdapterTraitProfile
		default:
			detailKey = modeltypology.DetailAdapterPersonalityType
		}
	}
	typeConclusion := conclusion.TypeConclusion{
		FactorCodes: append([]string(nil), runtime.FactorGraph.Roots...),
		Decision:    decision,
		OutcomeMapping: conclusion.TypeOutcomeMapping{
			DetailKind: string(runtime.OutcomeMapping.DetailKind), DetailAdapterKey: string(detailKey), Algorithm: runtime.OutcomeMapping.Algorithm,
		},
	}
	for _, rule := range runtime.SpecialRules {
		typeConclusion.SpecialRules = append(typeConclusion.SpecialRules, conclusion.TypeSpecialRule{
			Code: rule.Code, Kind: conclusion.TypeSpecialRuleKind(rule.Kind), Phase: conclusion.TypeSpecialRulePhase(rule.Phase),
			Trigger: rule.Trigger, OutcomeCode: rule.OutcomeCode,
			QuestionCodes: append([]string(nil), rule.Condition.QuestionCodes...), OptionValues: append([]string(nil), rule.Condition.OptionValues...),
		})
	}
	def := &modeldefinition.Definition{Measure: measure, Conclusions: []conclusion.Conclusion{typeConclusion}}
	if runtime.Report.Kind != "" {
		adapterKey := runtime.Report.AdapterKey
		if adapterKey == "" {
			adapterKey = modeltypology.ReportAdapterKey(detailKey)
		}
		def.ReportMap.Sections = []modeldefinition.ReportSection{{
			Code: string(runtime.Report.Kind), Kind: string(runtime.Report.Kind), AdapterKey: string(adapterKey),
			TemplateID: runtime.Report.TemplateID, TemplateVersion: runtime.Report.TemplateVersion, CategoryLabel: runtime.Report.CategoryLabel,
		}}
	}
	return def
}

func cloneFixtureScores(source map[string]float64) map[string]float64 {
	if source == nil {
		return nil
	}
	out := make(map[string]float64, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
