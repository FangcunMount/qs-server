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
		return binding.DecisionKindScoreRange, nil
	case binding.KindBehavioralRating:
		return behavioralDecisionKind(m.DefinitionV2.Conclusions, len(m.DefinitionV2.Calibration.NormRefs) > 0)
	case binding.KindCognitive:
		return binding.DecisionKindAbilityLevel, nil
	case binding.KindTypology:
		return typologyDecisionKind(m.DefinitionV2.Conclusions)
	default:
		return "", fmt.Errorf("model kind %s does not define a publish decision", m.Kind)
	}
}

func behavioralDecisionKind(items []conclusion.Conclusion, hasNormRefs bool) (binding.DecisionKind, error) {
	hasNormConclusion := false
	hasPrimary := false
	for _, item := range items {
		normConclusion, ok := item.(conclusion.NormConclusion)
		if !ok {
			continue
		}
		hasNormConclusion = true
		hasPrimary = hasPrimary || normConclusion.Primary
	}
	if hasNormRefs || hasNormConclusion {
		if !hasPrimary {
			return "", fmt.Errorf("behavioral norm conclusion requires a primary factor")
		}
		return binding.DecisionKindNormLookup, nil
	}
	return binding.DecisionKindScoreRange, nil
}

func typologyDecisionKind(items []conclusion.Conclusion) (binding.DecisionKind, error) {
	for _, item := range items {
		typed, ok := item.(conclusion.TypeConclusion)
		if !ok {
			continue
		}
		switch typed.Decision.Kind {
		case binding.DecisionKindPoleComposition, binding.DecisionKindTraitProfile, binding.DecisionKindNearestPattern:
			return typed.Decision.Kind, nil
		default:
			return "", fmt.Errorf("typology type conclusion decision kind is required")
		}
	}
	return "", fmt.Errorf("typology definition requires a type conclusion")
}
