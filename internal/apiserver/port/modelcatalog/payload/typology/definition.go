package typology

import (
	"fmt"
	"sort"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	sharedpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/shared"
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

// ImportLegacyDefinition imports a complete typology Definition from legacy or explicit wire payload.
func ImportLegacyDefinition(payload []byte, algorithm binding.Algorithm) (sharedpayload.DefinitionMaterialization, error) {
	decoded, runtime, err := PayloadAndRuntimeSpecFromDefinition(payload, algorithm)
	if err != nil {
		return sharedpayload.DefinitionMaterialization{}, err
	}
	def, err := definitionFromRuntime(decoded, runtime)
	if err != nil {
		return sharedpayload.DefinitionMaterialization{}, err
	}
	return sharedpayload.DefinitionMaterialization{Definition: def}, nil
}

// DefinitionFromLegacyPayload imports the target typology definition model
// without changing the payload contract.
func DefinitionFromLegacyPayload(payload []byte, algorithm binding.Algorithm) (*definition.Definition, error) {
	materialized, err := ImportLegacyDefinition(payload, algorithm)
	if err != nil {
		return nil, err
	}
	return materialized.Definition, nil
}

// DefinitionFromRuntime remains a compatibility helper for callers that already decoded runtime payload.
func DefinitionFromRuntime(payload *Payload, runtime *RuntimeSpec) *definition.Definition {
	def, err := definitionFromRuntime(payload, runtime)
	if err != nil {
		return &definition.Definition{}
	}
	return def
}

func definitionFromRuntime(payload *Payload, runtime *RuntimeSpec) (*definition.Definition, error) {
	if runtime == nil {
		return &definition.Definition{}, nil
	}
	measure, codes, err := measureSpecFromRuntime(runtime.FactorGraph, runtime.Decision.Kind)
	if err != nil {
		return nil, err
	}
	outcomes := conclusionOutcomes(payload)
	typeConclusion := conclusion.TypeConclusion{
		FactorCodes:    orderedFactorCodes(measure),
		Decision:       typeDecisionFromRuntime(runtime, codes),
		SpecialRules:   typeSpecialRulesFromRuntime(runtime.SpecialRules),
		OutcomeMapping: typeOutcomeMappingFromRuntime(runtime.OutcomeMapping),
		Profiles:       typeOutcomeProfilesFromPayload(payload),
		Outcomes:       outcomes,
	}
	return &definition.Definition{
		Measure:     measure,
		Conclusions: []conclusion.Conclusion{typeConclusion},
		Outcomes:    outcomes,
		ReportMap:   reportMapFromRuntime(runtime),
	}, nil
}

func measureSpecFromRuntime(graph FactorGraphSpec, decisionKind binding.DecisionKind) (definition.MeasureSpec, map[string]string, error) {
	if graph.HasExplicitFactorGraph() {
		return measureSpecFromExplicitGraph(graph)
	}
	return measureSpecFromLegacyGraph(graph, decisionKind)
}

func measureSpecFromExplicitGraph(graph FactorGraphSpec) (definition.MeasureSpec, map[string]string, error) {
	keys := make([]string, 0, len(graph.Factors))
	for key := range graph.Factors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	codes := make(map[string]string, len(graph.Factors))
	usedCodes := make(map[string]struct{}, len(graph.Factors))
	for _, key := range keys {
		spec := graph.Factors[key]
		id := firstNonEmpty(spec.ID, key)
		code := firstNonEmpty(spec.Code, id)
		if code == "" {
			return definition.MeasureSpec{}, nil, fmt.Errorf("typology factor %s code is required", key)
		}
		if _, exists := usedCodes[code]; exists {
			return definition.MeasureSpec{}, nil, fmt.Errorf("typology factor code %s is duplicated", code)
		}
		codes[id] = code
		codes[key] = code
		usedCodes[code] = struct{}{}
	}
	measure := definition.MeasureSpec{
		Factors:     make([]factor.Factor, 0, len(keys)),
		Scoring:     make([]factor.Scoring, 0, len(keys)),
		FactorGraph: factor.FactorGraph{SortOrders: make(map[string]int, len(keys))},
	}
	for order, key := range keys {
		spec := graph.Factors[key]
		id := firstNonEmpty(spec.ID, key)
		code := codes[id]
		name := spec.Name
		if meta, ok := graph.Dimensions[id]; ok && name == "" {
			name = meta.Name
		}
		role := factor.FactorRoleDimension
		if spec.Kind == FactorSpecKindComposite {
			role = factor.FactorRoleIndex
		}
		measure.Factors = append(measure.Factors, factor.Factor{Code: code, Title: name, Role: role})
		measure.FactorGraph.SortOrders[code] = order + 1
		scoring, edges, err := scoringFromExplicitFactor(spec, code, codes)
		if err != nil {
			return definition.MeasureSpec{}, nil, err
		}
		if scoring != nil {
			measure.Scoring = append(measure.Scoring, *scoring)
		}
		measure.FactorGraph.Edges = append(measure.FactorGraph.Edges, edges...)
	}
	for _, rootID := range graph.Roots {
		code, ok := codes[rootID]
		if !ok {
			return definition.MeasureSpec{}, nil, fmt.Errorf("typology root %s is not defined", rootID)
		}
		measure.FactorGraph.Roots = append(measure.FactorGraph.Roots, code)
	}
	if len(measure.FactorGraph.Roots) == 0 {
		measure.FactorGraph.Roots = rootsFromEdges(measure.Factors, measure.FactorGraph.Edges)
	}
	return measure, codes, nil
}

func scoringFromExplicitFactor(spec FactorSpec, code string, codes map[string]string) (*factor.Scoring, []factor.FactorEdge, error) {
	if spec.Kind == FactorSpecKindComposite {
		sources := make([]factor.ScoringSource, 0, len(spec.Children))
		edges := make([]factor.FactorEdge, 0, len(spec.Children))
		weights := make(map[string]float64, len(spec.Weights))
		for _, childID := range spec.Children {
			childCode, ok := codes[childID]
			if !ok {
				return nil, nil, fmt.Errorf("typology composite %s child %s is not defined", code, childID)
			}
			sources = append(sources, factor.ScoringSource{Kind: factor.ScoringSourceFactor, Code: childCode})
			edges = append(edges, factor.FactorEdge{ParentCode: code, ChildCode: childCode})
			if weight, ok := spec.Weights[childID]; ok {
				weights[childCode] = weight
			}
		}
		if len(weights) == 0 {
			weights = nil
		}
		return &factor.Scoring{FactorCode: code, Sources: sources, Strategy: scoringStrategyFromAggregation(spec.Aggregation), Weights: weights}, edges, nil
	}
	sources := make([]factor.ScoringSource, 0, len(spec.Contributions))
	for _, contribution := range spec.Contributions {
		sources = append(sources, factor.ScoringSource{
			Kind: factor.ScoringSourceQuestion, Code: contribution.QuestionCode,
			ScoringMode: factor.QuestionScoringMode(contribution.ScoringMode), Sign: contribution.Sign, Weight: contribution.Weight,
			OptionScores: cloneFloatMap(contribution.OptionScores),
		})
	}
	return &factor.Scoring{FactorCode: code, Sources: sources, Strategy: factor.ScoringStrategySum, Constant: spec.Constant, OptionScoring: optionScoringFromRuntime(spec.OptionScoring)}, nil, nil
}

func measureSpecFromLegacyGraph(graph FactorGraphSpec, decisionKind binding.DecisionKind) (definition.MeasureSpec, map[string]string, error) {
	codes := make(map[string]string, len(graph.DimensionOrder))
	measure := definition.MeasureSpec{
		Factors:     make([]factor.Factor, 0, len(graph.DimensionOrder)),
		Scoring:     make([]factor.Scoring, 0, len(graph.DimensionOrder)),
		FactorGraph: factor.FactorGraph{Roots: make([]string, 0, len(graph.DimensionOrder)), SortOrders: make(map[string]int, len(graph.DimensionOrder))},
	}
	mappings := mappingsByDimension(graph.QuestionMappings)
	for order, id := range graph.DimensionOrder {
		dimension, ok := graph.Dimensions[id]
		if !ok {
			return definition.MeasureSpec{}, nil, fmt.Errorf("typology dimension %s is not defined", id)
		}
		code := firstNonEmpty(dimension.Code, id)
		if _, exists := codes[id]; exists {
			return definition.MeasureSpec{}, nil, fmt.Errorf("typology dimension %s is duplicated", code)
		}
		codes[id] = code
		measure.Factors = append(measure.Factors, factor.Factor{Code: code, Title: dimension.Name, Role: factor.FactorRoleDimension})
		measure.FactorGraph.Roots = append(measure.FactorGraph.Roots, code)
		measure.FactorGraph.SortOrders[code] = order + 1
		sources := make([]factor.ScoringSource, 0, len(mappings[id]))
		for _, mapping := range mappings[id] {
			sources = append(sources, factor.ScoringSource{Kind: factor.ScoringSourceQuestion, Code: mapping.QuestionCode, Sign: mapping.Sign, OptionScores: cloneFloatMap(mapping.OptionScores)})
		}
		optionScoring := factor.OptionScoringStrict
		if decisionKind == binding.DecisionKindNearestPattern {
			optionScoring = factor.OptionScoringCompat
		}
		measure.Scoring = append(measure.Scoring, factor.Scoring{FactorCode: code, Sources: sources, Strategy: factor.ScoringStrategySum, Constant: dimension.Constant, OptionScoring: optionScoring})
	}
	return measure, codes, nil
}

func typeDecisionFromRuntime(runtime *RuntimeSpec, codes map[string]string) conclusion.TypeDecision {
	if runtime == nil {
		return conclusion.TypeDecision{}
	}
	out := conclusion.TypeDecision{Kind: runtime.Decision.Kind, FallbackSimilarityThreshold: runtime.Decision.FallbackSimilarityThreshold, FallbackCode: runtime.Decision.FallbackCode, TopK: runtime.Decision.TopK}
	if runtime.Decision.LevelRule != nil {
		out.LevelRule = &conclusion.TypeLevelRule{LowMax: runtime.Decision.LevelRule.LowMax, HighMin: runtime.Decision.LevelRule.HighMin}
	}
	for _, id := range runtime.FactorGraph.DecisionFactorOrder() {
		code := codes[id]
		if code == "" {
			continue
		}
		meta, ok := runtime.FactorGraph.Dimensions[id]
		if !ok {
			meta, ok = runtime.FactorGraph.Dimensions[code]
		}
		if !ok {
			continue
		}
		out.Poles = append(out.Poles, conclusion.TypePole{FactorCode: code, LeftPole: meta.LeftPole, RightPole: meta.RightPole, Threshold: meta.Threshold, Model: meta.Model})
	}
	return out
}

func typeSpecialRulesFromRuntime(items []SpecialRuleSpec) []conclusion.TypeSpecialRule {
	if items == nil {
		return nil
	}
	out := make([]conclusion.TypeSpecialRule, 0, len(items))
	for _, item := range items {
		out = append(out, conclusion.TypeSpecialRule{
			Code: item.Code, Kind: conclusion.TypeSpecialRuleKind(item.ResolvedKind()), Phase: conclusion.TypeSpecialRulePhase(item.Phase), Trigger: item.Trigger,
			OutcomeCode: item.OutcomeCode, QuestionCodes: item.ResolvedQuestionCodes(), OptionValues: item.ResolvedOptionValues(),
		})
	}
	return out
}

func typeOutcomeMappingFromRuntime(value OutcomeMappingSpec) conclusion.TypeOutcomeMapping {
	return conclusion.TypeOutcomeMapping{DetailKind: string(value.DetailKind), DetailAdapterKey: string(value.DetailAdapterKey), Algorithm: value.Algorithm}
}

func typeOutcomeProfilesFromPayload(payload *Payload) []conclusion.TypeOutcomeProfile {
	if payload == nil || payload.Outcomes == nil {
		return nil
	}
	out := make([]conclusion.TypeOutcomeProfile, 0, len(payload.Outcomes))
	for _, item := range payload.Outcomes {
		out = append(out, conclusion.TypeOutcomeProfile{OutcomeCode: item.Code, Pattern: item.Pattern, Traits: append([]string(nil), item.Traits...), Strengths: append([]string(nil), item.Strengths...), Weaknesses: append([]string(nil), item.Weaknesses...), Suggestions: append([]string(nil), item.Suggestions...), ImageURL: item.ImageURL, Image: item.Image, Rarity: conclusion.Rarity{Percent: item.Rarity.Percent, Label: item.Rarity.Label, OneInX: item.Rarity.OneInX}, IsSpecial: item.IsSpecial, Trigger: item.Trigger, Commentary: item.Commentary})
	}
	return out
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
	return &Payload{Code: env.Code, Version: env.Version, Title: env.Title, QuestionnaireCode: env.QuestionnaireCode, QuestionnaireVersion: env.QuestionnaireVersion, Status: env.Status, Algorithm: env.Algorithm, Outcomes: outcomes, MatchingSpec: MatchingSpec{Kind: runtime.Decision.Kind, FallbackSimilarityThreshold: runtime.Decision.FallbackSimilarityThreshold}, Runtime: runtime}, nil
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
			spec.OptionScoring = FactorOptionScoring(rule.OptionScoring)
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
	return ReportSpec{Kind: ReportKind(section.Kind), AdapterKey: ReportAdapterKey(section.AdapterKey), TemplateID: section.TemplateID, CategoryLabel: section.CategoryLabel}
}

func reportMapFromRuntime(runtime *RuntimeSpec) definition.ReportMap {
	if runtime == nil || runtime.Report.Kind == "" {
		return definition.ReportMap{}
	}
	return definition.ReportMap{Sections: []definition.ReportSection{{Code: string(runtime.Report.Kind), Title: firstNonEmpty(runtime.Report.CategoryLabel, string(runtime.Report.Kind)), Kind: string(runtime.Report.Kind), AdapterKey: string(runtime.Report.ResolvedAdapterKey(runtime.OutcomeMapping, runtime.Decision.Kind)), TemplateID: runtime.Report.TemplateID, CategoryLabel: runtime.Report.CategoryLabel}}}
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

func conclusionOutcomes(payload *Payload) []conclusion.Outcome {
	if payload == nil || payload.Outcomes == nil {
		return nil
	}
	out := make([]conclusion.Outcome, 0, len(payload.Outcomes))
	for _, item := range payload.Outcomes {
		out = append(out, conclusion.Outcome{Code: item.Code, Title: item.Name, Summary: item.Summary, Description: item.OneLiner})
	}
	return out
}

func orderedFactorCodes(measure definition.MeasureSpec) []string {
	if measure.Factors == nil {
		return nil
	}
	out := make([]string, 0, len(measure.Factors))
	for _, item := range measure.Factors {
		if item.Code != "" {
			out = append(out, item.Code)
		}
	}
	return out
}

func mappingsByDimension(items []QuestionMapping) map[string][]QuestionMapping {
	out := make(map[string][]QuestionMapping)
	for _, item := range items {
		out[item.Dimension] = append(out[item.Dimension], item)
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

func scoringStrategyFromAggregation(value FactorAggregation) factor.ScoringStrategy {
	switch value {
	case FactorAggregationAvg:
		return factor.ScoringStrategyAvg
	case FactorAggregationWeightedAvg:
		return factor.ScoringStrategyWeightedAvg
	default:
		return factor.ScoringStrategySum
	}
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

func optionScoringFromRuntime(value FactorOptionScoring) factor.OptionScoring {
	if value == FactorOptionScoringCompat {
		return factor.OptionScoringCompat
	}
	return factor.OptionScoringStrict
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
