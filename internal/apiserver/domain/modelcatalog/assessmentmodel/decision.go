package assessmentmodel

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
)

// DecisionKindForDefinition derives publication routing from canonical model
// identity and DefinitionV2, never from the legacy payload bytes.
func (m *AssessmentModel) DecisionKindForDefinition() (binding.DecisionKind, error) {
	if m == nil {
		return "", fmt.Errorf("assessment model is nil")
	}
	if m.DefinitionV2 == nil {
		return "", fmt.Errorf("definition_v2 is required")
	}
	switch m.Kind {
	case binding.KindScale:
		return scaleDecisionKind(m.DefinitionV2.Conclusions)
	case binding.KindBehavioralRating:
		return behavioralDecisionKind(m.DefinitionV2.Conclusions, len(m.DefinitionV2.Calibration.NormRefs) > 0)
	case binding.KindCognitive:
		return cognitiveDecisionKind(m.DefinitionV2.Conclusions)
	case binding.KindTypology:
		return typologyDecisionKind(m.DefinitionV2.Conclusions)
	default:
		return "", fmt.Errorf("model kind %s does not define a publish decision", m.Kind)
	}
}

// scaleDecisionKind requires an explicit RiskConclusion; Kind alone must not invent Decision.
func scaleDecisionKind(items []conclusion.Conclusion) (binding.DecisionKind, error) {
	for _, item := range items {
		if _, ok := item.(conclusion.RiskConclusion); ok {
			return binding.DecisionKindScoreRange, nil
		}
	}
	return "", fmt.Errorf("scale definition requires a risk conclusion")
}

// cognitiveDecisionKind requires an explicit AbilityConclusion; Kind alone must not invent Decision.
func cognitiveDecisionKind(items []conclusion.Conclusion) (binding.DecisionKind, error) {
	for _, item := range items {
		if _, ok := item.(conclusion.AbilityConclusion); ok {
			return binding.DecisionKindAbilityLevel, nil
		}
	}
	return "", fmt.Errorf("cognitive definition requires an ability conclusion")
}

// behavioralDecisionKind enforces the domain rule that behavioral_rating always
// publishes as norm_lookup. Raw score-range models must use KindScale instead.
func behavioralDecisionKind(items []conclusion.Conclusion, hasNormRefs bool) (binding.DecisionKind, error) {
	if !hasNormRefs {
		return "", fmt.Errorf("behavioral_rating requires at least one calibration.norm_refs entry")
	}
	primaryCount := 0
	hasNormConclusion := false
	for _, item := range items {
		normConclusion, ok := item.(conclusion.NormConclusion)
		if !ok {
			continue
		}
		hasNormConclusion = true
		if normConclusion.Primary {
			primaryCount++
		}
	}
	if !hasNormConclusion {
		return "", fmt.Errorf("behavioral_rating requires at least one norm conclusion")
	}
	if primaryCount == 0 {
		return "", fmt.Errorf("behavioral_rating requires exactly one primary norm conclusion")
	}
	if primaryCount > 1 {
		return "", fmt.Errorf("behavioral_rating allows only one primary norm conclusion")
	}
	return binding.DecisionKindNormLookup, nil
}

func typologyDecisionKind(items []conclusion.Conclusion) (binding.DecisionKind, error) {
	for _, item := range items {
		typed, ok := item.(conclusion.TypeConclusion)
		if !ok {
			continue
		}
		switch typed.Decision.Kind {
		case binding.DecisionKindPoleComposition, binding.DecisionKindTraitProfile, binding.DecisionKindNearestPattern, binding.DecisionKindDominantFactor:
			return typed.Decision.Kind, nil
		default:
			return "", fmt.Errorf("typology type conclusion decision kind is required")
		}
	}
	return "", fmt.Errorf("typology definition requires a type conclusion")
}
