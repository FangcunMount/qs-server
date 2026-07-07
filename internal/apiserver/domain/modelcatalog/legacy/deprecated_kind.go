package legacy

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"

// KindBehaviorAbilityLegacy is the deprecated flat kind used as a product-channel API filter.
// New models must use behavioral_rating or cognitive instead.
const KindBehaviorAbilityLegacy = "behavior_ability"

// DecisionKindScoreRangeInterpretationLegacy is the deprecated decision-kind alias.
const DecisionKindScoreRangeInterpretationLegacy = "score_range_interpretation"

// BehaviorAbilityKind returns the identity.Kind for the behavior_ability product-channel slot.
func BehaviorAbilityKind() identity.Kind {
	return identity.Kind(KindBehaviorAbilityLegacy)
}

// ScoreRangeInterpretationDecisionKind returns the legacy score_range_interpretation decision kind.
func ScoreRangeInterpretationDecisionKind() identity.DecisionKind {
	return identity.DecisionKind(DecisionKindScoreRangeInterpretationLegacy)
}

// IsDeprecatedProductChannelKind reports legacy kinds that are product-channel slots only.
func IsDeprecatedProductChannelKind(kind string) bool {
	return kind == KindBehaviorAbilityLegacy
}
