package typology

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

// ToRuntimeSpec 返回执行规格 用于 类型学载荷。
// Explicit 载荷.运行时 takes precedence; 旧版 字段 fill 仅 缺失 sections。
func (p *Payload) ToRuntimeSpec() (*RuntimeSpec, error) {
	if p == nil {
		return nil, fmt.Errorf("typology payload is required")
	}
	if p.Runtime != nil {
		spec := cloneRuntimeSpec(p.Runtime)
		legacy, err := p.deriveLegacyRuntimeSpec(false)
		if err != nil {
			return nil, err
		}
		mergeRuntimeSpec(spec, legacy)
		if err := validateRuntimeSpec(spec); err != nil {
			return nil, err
		}
		return spec, nil
	}
	spec, err := p.deriveLegacyRuntimeSpec(true)
	if err != nil {
		return nil, err
	}
	if err := validateRuntimeSpec(spec); err != nil {
		return nil, err
	}
	return spec, nil
}

func (p *Payload) deriveLegacyRuntimeSpec(requireAlgorithm bool) (*RuntimeSpec, error) {
	if requireAlgorithm && p.Algorithm == "" {
		return nil, fmt.Errorf("typology payload algorithm is required")
	}
	spec := &RuntimeSpec{
		FactorGraph: FactorGraphSpec{
			DimensionOrder:   append([]string(nil), p.DimensionOrder...),
			Dimensions:       cloneDimensions(p.Dimensions),
			QuestionMappings: append([]QuestionMapping(nil), p.QuestionMappings...),
		},
		Decision:     decisionSpecFromPayload(p),
		SpecialRules: specialRulesFromPayload(p),
	}
	if p.Algorithm != "" {
		spec.OutcomeMapping = LegacyOutcomeMappingFromAlgorithm(p.Algorithm)
		spec.Report = LegacyReportSpecFromAlgorithm(p.Algorithm)
	}
	return spec, nil
}

func mergeRuntimeSpec(explicit, legacy *RuntimeSpec) {
	if explicit == nil || legacy == nil {
		return
	}
	if len(explicit.FactorGraph.DimensionOrder) == 0 {
		explicit.FactorGraph.DimensionOrder = append([]string(nil), legacy.FactorGraph.DimensionOrder...)
	}
	if len(explicit.FactorGraph.Dimensions) == 0 {
		explicit.FactorGraph.Dimensions = cloneDimensions(legacy.FactorGraph.Dimensions)
	}
	if len(explicit.FactorGraph.QuestionMappings) == 0 {
		explicit.FactorGraph.QuestionMappings = append([]QuestionMapping(nil), legacy.FactorGraph.QuestionMappings...)
	}
	if len(explicit.FactorGraph.Factors) == 0 {
		explicit.FactorGraph.Factors = cloneFactorSpecs(legacy.FactorGraph.Factors)
	}
	if len(explicit.FactorGraph.Roots) == 0 {
		explicit.FactorGraph.Roots = append([]string(nil), legacy.FactorGraph.Roots...)
	}
	if explicit.Decision.Kind == "" {
		explicit.Decision.Kind = legacy.Decision.Kind
	}
	if explicit.Decision.FallbackSimilarityThreshold == 0 {
		explicit.Decision.FallbackSimilarityThreshold = legacy.Decision.FallbackSimilarityThreshold
	}
	if explicit.Decision.FallbackCode == "" {
		explicit.Decision.FallbackCode = legacy.Decision.FallbackCode
	}
	if explicit.Decision.LevelRule == nil {
		explicit.Decision.LevelRule = cloneLevelRule(legacy.Decision.LevelRule)
	}
	if len(explicit.SpecialRules) == 0 {
		explicit.SpecialRules = cloneSpecialRules(legacy.SpecialRules)
	}
	if explicit.OutcomeMapping.DetailKind == "" {
		explicit.OutcomeMapping = legacy.OutcomeMapping
	} else {
		if explicit.OutcomeMapping.DetailAdapterKey == "" {
			explicit.OutcomeMapping.DetailAdapterKey = legacy.OutcomeMapping.DetailAdapterKey
		}
		if explicit.OutcomeMapping.Algorithm == "" {
			explicit.OutcomeMapping.Algorithm = legacy.OutcomeMapping.Algorithm
		}
	}
	if explicit.Report.Kind == "" {
		explicit.Report = legacy.Report
	} else {
		if explicit.Report.CategoryLabel == "" {
			explicit.Report.CategoryLabel = legacy.Report.CategoryLabel
		}
		if explicit.Report.TemplateID == "" {
			explicit.Report.TemplateID = legacy.Report.TemplateID
		}
		if explicit.Report.AdapterKey == "" {
			explicit.Report.AdapterKey = legacy.Report.AdapterKey
		}
	}
}

func validateRuntimeSpec(spec *RuntimeSpec) error {
	if spec == nil {
		return fmt.Errorf("runtime spec is required")
	}
	if spec.Decision.Kind == "" {
		return fmt.Errorf("runtime decision kind is required")
	}
	if spec.FactorGraph.HasExplicitFactorGraph() {
		for _, rootID := range spec.FactorGraph.Roots {
			if _, ok := spec.FactorGraph.Factors[rootID]; !ok {
				return fmt.Errorf("runtime factor graph root %s is not defined", rootID)
			}
		}
	} else {
		if len(spec.FactorGraph.DimensionOrder) == 0 {
			return fmt.Errorf("runtime factor graph dimension order is required")
		}
		if len(spec.FactorGraph.Dimensions) == 0 {
			return fmt.Errorf("runtime factor graph dimensions are required")
		}
	}
	if spec.OutcomeMapping.DetailKind == "" {
		return fmt.Errorf("runtime outcome mapping detail kind is required")
	}
	if spec.Report.Kind == "" {
		return fmt.Errorf("runtime report kind is required")
	}
	if spec.Report.Kind == ReportKindTemplate && spec.Report.AdapterKey == "" {
		return fmt.Errorf("runtime template report adapter key is required")
	}
	for _, rule := range spec.SpecialRules {
		if err := validateSpecialRuleSpec(rule); err != nil {
			return err
		}
	}
	return nil
}

func validateSpecialRuleSpec(rule SpecialRuleSpec) error {
	switch rule.Phase {
	case "", SpecialRuleBeforeScore, SpecialRuleAfterDecision:
	case SpecialRuleBeforeDecision:
		return fmt.Errorf("runtime special rule phase %s is not implemented", rule.Phase)
	default:
		return fmt.Errorf("runtime special rule phase %s is unsupported", rule.Phase)
	}
	switch rule.ResolvedKind() {
	case "", SpecialRuleKindAnswerMatch, SpecialRuleKindFallbackThreshold:
		return nil
	default:
		return fmt.Errorf("runtime special rule kind %s is unsupported", rule.ResolvedKind())
	}
}

func cloneRuntimeSpec(source *RuntimeSpec) *RuntimeSpec {
	if source == nil {
		return nil
	}
	return &RuntimeSpec{
		FactorGraph: FactorGraphSpec{
			DimensionOrder:   append([]string(nil), source.FactorGraph.DimensionOrder...),
			Dimensions:       cloneDimensions(source.FactorGraph.Dimensions),
			QuestionMappings: cloneQuestionMappings(source.FactorGraph.QuestionMappings),
			Factors:          cloneFactorSpecs(source.FactorGraph.Factors),
			Roots:            append([]string(nil), source.FactorGraph.Roots...),
		},
		Decision: PersonalityDecisionSpec{
			Kind:                        source.Decision.Kind,
			FallbackSimilarityThreshold: source.Decision.FallbackSimilarityThreshold,
			FallbackCode:                source.Decision.FallbackCode,
			LevelRule:                   cloneLevelRule(source.Decision.LevelRule),
		},
		SpecialRules:   cloneSpecialRules(source.SpecialRules),
		OutcomeMapping: source.OutcomeMapping,
		Report:         source.Report,
	}
}

func cloneFactorSpecs(source map[string]FactorSpec) map[string]FactorSpec {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]FactorSpec, len(source))
	for key, factor := range source {
		cloned[key] = cloneFactorSpec(factor)
	}
	return cloned
}

func cloneFactorSpec(source FactorSpec) FactorSpec {
	cloned := FactorSpec{
		ID:            source.ID,
		Code:          source.Code,
		Name:          source.Name,
		Kind:          source.Kind,
		Children:      append([]string(nil), source.Children...),
		Aggregation:   source.Aggregation,
		Constant:      source.Constant,
		OptionScoring: source.OptionScoring,
		Contributions: make([]FactorContributionSpec, len(source.Contributions)),
	}
	copy(cloned.Contributions, source.Contributions)
	if source.Weights != nil {
		cloned.Weights = make(map[string]float64, len(source.Weights))
		for key, value := range source.Weights {
			cloned.Weights[key] = value
		}
	}
	for i := range cloned.Contributions {
		if source.Contributions[i].OptionScores != nil {
			cloned.Contributions[i].OptionScores = make(map[string]float64, len(source.Contributions[i].OptionScores))
			for key, value := range source.Contributions[i].OptionScores {
				cloned.Contributions[i].OptionScores[key] = value
			}
		}
	}
	return cloned
}

func cloneQuestionMappings(source []QuestionMapping) []QuestionMapping {
	if len(source) == 0 {
		return nil
	}
	cloned := make([]QuestionMapping, len(source))
	for i, mapping := range source {
		cloned[i] = QuestionMapping{
			QuestionCode: mapping.QuestionCode,
			Dimension:    mapping.Dimension,
			Sign:         mapping.Sign,
		}
		if mapping.OptionScores != nil {
			cloned[i].OptionScores = make(map[string]float64, len(mapping.OptionScores))
			for key, value := range mapping.OptionScores {
				cloned[i].OptionScores[key] = value
			}
		}
	}
	return cloned
}

func cloneLevelRule(source *LevelRuleSpec) *LevelRuleSpec {
	if source == nil {
		return nil
	}
	cloned := *source
	return &cloned
}

func cloneSpecialRules(source []SpecialRuleSpec) []SpecialRuleSpec {
	if len(source) == 0 {
		return nil
	}
	cloned := make([]SpecialRuleSpec, len(source))
	for i, rule := range source {
		cloned[i] = SpecialRuleSpec{
			Code:          rule.Code,
			Kind:          rule.Kind,
			Phase:         rule.Phase,
			Trigger:       rule.Trigger,
			OutcomeCode:   rule.OutcomeCode,
			Condition:     rule.Condition,
			QuestionCodes: append([]string(nil), rule.QuestionCodes...),
			OptionValues:  append([]string(nil), rule.OptionValues...),
		}
	}
	return cloned
}

func decisionSpecFromPayload(p *Payload) PersonalityDecisionSpec {
	spec := PersonalityDecisionSpec{
		Kind:                        p.MatchingSpec.Kind,
		FallbackSimilarityThreshold: p.MatchingSpec.FallbackSimilarityThreshold,
	}
	switch p.MatchingSpec.Kind {
	case binding.DecisionKindNearestPattern:
		spec.FallbackCode = fallbackCodeFromOutcomes(p.Outcomes)
		spec.LevelRule = &LevelRuleSpec{LowMax: 3, HighMin: 5}
	case "", binding.DecisionKindPoleComposition:
		spec.Kind = binding.DecisionKindPoleComposition
	case binding.DecisionKindTraitProfile:
		spec.Kind = binding.DecisionKindTraitProfile
	}
	return spec
}

func fallbackCodeFromOutcomes(outcomes []Outcome) string {
	for _, outcome := range outcomes {
		if outcome.IsSpecial && isFallbackTrigger(outcome.Trigger) {
			return outcome.Code
		}
	}
	return ""
}

func isFallbackTrigger(trigger string) bool {
	return len(trigger) >= 9 && trigger[:9] == "fallback:"
}

func specialRulesFromPayload(p *Payload) []SpecialRuleSpec {
	rules := make([]SpecialRuleSpec, 0, len(p.SpecialTriggers)+1)
	for _, trigger := range p.SpecialTriggers {
		questionCodes := append([]string(nil), trigger.QuestionCodes...)
		optionValues := append([]string(nil), trigger.OptionValues...)
		kind := SpecialRuleKindAnswerMatch
		phase := SpecialRuleBeforeScore
		if len(questionCodes) == 0 {
			kind = SpecialRuleKindFallbackThreshold
			phase = SpecialRuleAfterDecision
		}
		rules = append(rules, SpecialRuleSpec{
			Code:        trigger.Code,
			Kind:        kind,
			Phase:       phase,
			Trigger:     trigger.Trigger,
			OutcomeCode: firstNonEmpty(trigger.OutcomeCode, trigger.Code),
			Condition: SpecialRuleCondition{
				QuestionCodes: questionCodes,
				OptionValues:  optionValues,
			},
			QuestionCodes: questionCodes,
			OptionValues:  optionValues,
		})
	}
	if p.MatchingSpec.Kind == binding.DecisionKindNearestPattern &&
		p.MatchingSpec.FallbackSimilarityThreshold > 0 {
		fallbackCode := fallbackCodeFromOutcomes(p.Outcomes)
		if fallbackCode != "" {
			rules = append(rules, SpecialRuleSpec{
				Code:        "fallback:" + fallbackCode,
				Kind:        SpecialRuleKindFallbackThreshold,
				Phase:       SpecialRuleAfterDecision,
				OutcomeCode: fallbackCode,
			})
		}
	}
	return rules
}

func cloneDimensions(source map[string]Dimension) map[string]Dimension {
	if source == nil {
		return nil
	}
	cloned := make(map[string]Dimension, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
