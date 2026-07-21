package typology

import (
	"fmt"
)

// ToRuntimeSpec 返回执行规格 用于 类型学载荷。
// DefinitionV2 materialization always supplies an explicit runtime section.
func (p *Payload) ToRuntimeSpec() (*RuntimeSpec, error) {
	if p == nil || p.Runtime == nil {
		return nil, fmt.Errorf("typology definition_v2 runtime is required")
	}
	spec := cloneRuntimeSpec(p.Runtime)
	if err := validateRuntimeSpec(spec); err != nil {
		return nil, err
	}
	return spec, nil
}

func validateRuntimeSpec(spec *RuntimeSpec) error {
	if spec == nil {
		return fmt.Errorf("runtime spec is required")
	}
	if spec.Decision.Kind == "" {
		return fmt.Errorf("runtime decision kind is required")
	}
	if !spec.FactorGraph.HasExplicitFactorGraph() {
		return fmt.Errorf("runtime explicit factor graph is required")
	}
	for _, rootID := range spec.FactorGraph.Roots {
		if _, ok := spec.FactorGraph.Factors[rootID]; !ok {
			return fmt.Errorf("runtime factor graph root %s is not defined", rootID)
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
	case SpecialRuleKindAnswerMatch, SpecialRuleKindFallbackThreshold:
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
			Dimensions: cloneDimensions(source.FactorGraph.Dimensions),
			Factors:    cloneFactorSpecs(source.FactorGraph.Factors),
			Roots:      append([]string(nil), source.FactorGraph.Roots...),
		},
		Decision: PersonalityDecisionSpec{
			Kind:                        source.Decision.Kind,
			FallbackSimilarityThreshold: source.Decision.FallbackSimilarityThreshold,
			FallbackCode:                source.Decision.FallbackCode,
			LevelRule:                   cloneLevelRule(source.Decision.LevelRule),
			TopK:                        source.Decision.TopK,
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
		cloned.Contributions[i].ScoringMode = source.Contributions[i].ScoringMode
		cloned.Contributions[i].Weight = source.Contributions[i].Weight
		if source.Contributions[i].OptionScores != nil {
			cloned.Contributions[i].OptionScores = make(map[string]float64, len(source.Contributions[i].OptionScores))
			for key, value := range source.Contributions[i].OptionScores {
				cloned.Contributions[i].OptionScores[key] = value
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
			Code:        rule.Code,
			Kind:        rule.Kind,
			Phase:       rule.Phase,
			Trigger:     rule.Trigger,
			OutcomeCode: rule.OutcomeCode,
			Condition: SpecialRuleCondition{
				QuestionCodes: append([]string(nil), rule.Condition.QuestionCodes...),
				OptionValues:  append([]string(nil), rule.Condition.OptionValues...),
			},
		}
	}
	return cloned
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
