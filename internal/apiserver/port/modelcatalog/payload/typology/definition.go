package typology

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// DefinitionEnvelope carries wire metadata while a Definition is projected to a runtime payload.
type DefinitionEnvelope struct {
	Code                 string
	Version              string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	Algorithm            binding.Algorithm
}

// ResolveRuntimeSpec materializes runtime only from canonical DefinitionV2.
func ResolveRuntimeSpec(def *definition.Definition) (*RuntimeSpec, error) {
	if def == nil {
		return nil, fmt.Errorf("typology definition_v2 is required")
	}
	return RuntimeSpecFromDefinition(def)
}

// RuntimeSpecFromDefinition reconstructs the typology execution DTO solely from Definition semantics.
func RuntimeSpecFromDefinition(def *definition.Definition) (*RuntimeSpec, error) {
	if def == nil {
		return nil, fmt.Errorf("typology definition is nil")
	}
	typeConclusion, ok := findTypeConclusion(def.Conclusions)
	if !ok {
		return nil, fmt.Errorf("typology definition type conclusion is required")
	}
	graph, err := runtimeGraphFromMeasure(def.Measure, typeConclusion.Decision)
	if err != nil {
		return nil, err
	}
	report := reportSpecFromDefinition(def.ReportMap)
	return &RuntimeSpec{
		FactorGraph:    graph,
		Decision:       PersonalityDecisionSpec{Kind: typeConclusion.Decision.Kind, FallbackSimilarityThreshold: typeConclusion.Decision.FallbackSimilarityThreshold, FallbackCode: typeConclusion.Decision.FallbackCode, LevelRule: levelRuleSpecFromConclusion(typeConclusion.Decision.LevelRule), TopK: typeConclusion.Decision.TopK},
		SpecialRules:   typeSpecialRulesToRuntime(typeConclusion.SpecialRules),
		OutcomeMapping: OutcomeMappingSpec{DetailKind: OutcomeDetailKind(typeConclusion.OutcomeMapping.DetailKind), DetailAdapterKey: DetailAdapterKey(typeConclusion.OutcomeMapping.DetailAdapterKey), Algorithm: typeConclusion.OutcomeMapping.Algorithm},
		Report:         report,
	}, nil
}

// PayloadFromDefinition rebuilds the runtime wire DTO from Definition semantics.
func PayloadFromDefinition(env DefinitionEnvelope, def *definition.Definition) (*Payload, error) {
	runtime, err := RuntimeSpecFromDefinition(def)
	if err != nil {
		return nil, err
	}
	typeConclusion, _ := findTypeConclusion(def.Conclusions)
	profiles := profilesByCode(typeConclusion.Profiles)
	outcomes := make([]Outcome, 0, len(def.Outcomes))
	for _, item := range def.Outcomes {
		profile := profiles[item.Code]
		outcomes = append(outcomes, Outcome{Code: item.Code, Name: item.Title, OneLiner: item.Description, Summary: item.Summary, Pattern: profile.Pattern, Traits: append([]string(nil), profile.Traits...), Strengths: append([]string(nil), profile.Strengths...), Weaknesses: append([]string(nil), profile.Weaknesses...), Suggestions: append([]string(nil), profile.Suggestions...), ImageURL: profile.ImageURL, Image: profile.Image, Rarity: Rarity{Percent: profile.Rarity.Percent, Label: profile.Rarity.Label, OneInX: profile.Rarity.OneInX}, IsSpecial: profile.IsSpecial, Trigger: profile.Trigger, Commentary: profile.Commentary})
	}
	return &Payload{Code: env.Code, Version: env.Version, Title: env.Title, QuestionnaireCode: env.QuestionnaireCode, QuestionnaireVersion: env.QuestionnaireVersion, Status: env.Status, Algorithm: env.Algorithm, Outcomes: outcomes, Runtime: runtime}, nil
}

func runtimeGraphFromMeasure(measure definition.MeasureSpec, decision conclusion.TypeDecision) (FactorGraphSpec, error) {
	factors := make(map[string]FactorSpec, len(measure.Factors))
	dimensions := make(map[string]Dimension, len(measure.Factors))
	scoringByFactor := make(map[string]factor.Scoring, len(measure.Scoring))
	for _, item := range measure.Scoring {
		scoringByFactor[item.FactorCode] = item
	}
	poles := make(map[string]conclusion.TypePole, len(decision.Poles))
	for _, pole := range decision.Poles {
		poles[pole.FactorCode] = pole
	}
	for _, item := range measure.Factors {
		rule := scoringByFactor[item.Code]
		spec := FactorSpec{ID: item.Code, Code: item.Code, Name: item.Title}
		if hasSourceKind(rule.Sources, factor.ScoringSourceFactor) {
			spec.Kind = FactorSpecKindComposite
			spec.Aggregation = aggregationFromScoring(rule.Strategy)
			spec.Weights = cloneFloatMap(rule.Weights)
			for _, source := range rule.Sources {
				if source.Kind == factor.ScoringSourceFactor {
					spec.Children = append(spec.Children, source.Code)
				}
			}
		} else {
			spec.Kind = FactorSpecKindLeaf
			spec.Constant = rule.Constant
			for _, source := range rule.Sources {
				if source.Kind == factor.ScoringSourceQuestion {
					spec.Contributions = append(spec.Contributions, FactorContributionSpec{
						QuestionCode: source.Code, ScoringMode: QuestionScoringMode(source.ScoringMode), Sign: source.Sign, Weight: source.Weight,
						OptionScores: cloneFloatMap(source.OptionScores),
					})
				}
			}
		}
		factors[item.Code] = spec
		pole := poles[item.Code]
		dimensions[item.Code] = Dimension{Code: item.Code, Name: item.Title, LeftPole: pole.LeftPole, RightPole: pole.RightPole, Threshold: pole.Threshold, Model: pole.Model}
	}
	roots := append([]string(nil), measure.FactorGraph.Roots...)
	if len(roots) == 0 {
		roots = rootsFromEdges(measure.Factors, measure.FactorGraph.Edges)
	}
	if len(factors) == 0 || len(roots) == 0 {
		return FactorGraphSpec{}, fmt.Errorf("typology measure requires factors and roots")
	}
	return FactorGraphSpec{Factors: factors, Roots: roots, Dimensions: dimensions}, nil
}

func reportSpecFromDefinition(reportMap definition.ReportMap) ReportSpec {
	if len(reportMap.Sections) == 0 {
		return ReportSpec{}
	}
	section := reportMap.Sections[0]
	return ReportSpec{Kind: ReportKind(section.Kind), AdapterKey: ReportAdapterKey(section.AdapterKey), TemplateID: section.TemplateID, TemplateVersion: section.TemplateVersion, CategoryLabel: section.CategoryLabel}
}

func findTypeConclusion(items []conclusion.Conclusion) (conclusion.TypeConclusion, bool) {
	for _, item := range items {
		if typed, ok := item.(conclusion.TypeConclusion); ok {
			return typed, true
		}
	}
	return conclusion.TypeConclusion{}, false
}

func levelRuleSpecFromConclusion(value *conclusion.TypeLevelRule) *LevelRuleSpec {
	if value == nil {
		return nil
	}
	return &LevelRuleSpec{LowMax: value.LowMax, HighMin: value.HighMin}
}

func typeSpecialRulesToRuntime(items []conclusion.TypeSpecialRule) []SpecialRuleSpec {
	if items == nil {
		return nil
	}
	out := make([]SpecialRuleSpec, 0, len(items))
	for _, item := range items {
		out = append(out, SpecialRuleSpec{Code: item.Code, Kind: SpecialRuleKind(item.Kind), Phase: SpecialRulePhase(item.Phase), Trigger: item.Trigger, OutcomeCode: item.OutcomeCode, Condition: SpecialRuleCondition{QuestionCodes: append([]string(nil), item.QuestionCodes...), OptionValues: append([]string(nil), item.OptionValues...)}})
	}
	return out
}

func rootsFromEdges(factors []factor.Factor, edges []factor.FactorEdge) []string {
	children := make(map[string]bool, len(edges))
	for _, edge := range edges {
		children[edge.ChildCode] = true
	}
	out := make([]string, 0, len(factors))
	for _, item := range factors {
		if !children[item.Code] {
			out = append(out, item.Code)
		}
	}
	return out
}

func aggregationFromScoring(value factor.ScoringStrategy) FactorAggregation {
	switch value {
	case factor.ScoringStrategyAvg:
		return FactorAggregationAvg
	case factor.ScoringStrategyWeightedAvg:
		return FactorAggregationWeightedAvg
	default:
		return FactorAggregationSum
	}
}

func hasSourceKind(items []factor.ScoringSource, kind factor.ScoringSourceKind) bool {
	for _, item := range items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func cloneFloatMap(items map[string]float64) map[string]float64 {
	if items == nil {
		return nil
	}
	out := make(map[string]float64, len(items))
	for key, value := range items {
		out[key] = value
	}
	return out
}

func profilesByCode(items []conclusion.TypeOutcomeProfile) map[string]conclusion.TypeOutcomeProfile {
	out := make(map[string]conclusion.TypeOutcomeProfile, len(items))
	for _, item := range items {
		out[item.OutcomeCode] = item
	}
	return out
}
