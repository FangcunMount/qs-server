package evaluation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// CompatibilitySource identifies how a missing frozen RuntimeIdentity field was filled.
type CompatibilitySource string

const (
	CompatibilitySourceNone               CompatibilitySource = ""
	CompatibilitySourceFrozen             CompatibilitySource = "frozen"
	CompatibilitySourceIdentity           CompatibilitySource = "identity"
	CompatibilitySourceDecisionKind       CompatibilitySource = "decision_kind"
	CompatibilitySourceLegacyTypology     CompatibilitySource = "legacy_typology"
	CompatibilitySourceFamilyDefault      CompatibilitySource = "family_default_decision"
	CompatibilitySourceDraftFormat        CompatibilitySource = "draft_payload_format"
	CompatibilitySourceAssessmentModelRef CompatibilitySource = "assessment_model_ref"
)

// CompatibilityHit records whether evaluation used a migration fallback.
type CompatibilityHit struct {
	Used   bool
	Source CompatibilitySource
}

// DescriptorKeyFromRoute derives the single runtime routing key from a model route.
// Frozen RuntimeIdentity is validated exactly; legacy routes go through CompatibilityResolver (EV-R008).
func DescriptorKeyFromRoute(route ModelRoute) (DescriptorKey, error) {
	modelcatalog.ObserveWritePolicy(route.Kind, route.Algorithm)
	if route.HasFrozenRuntime() {
		family, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(route.DecisionKind)
		if !ok || family != route.AlgorithmFamily {
			return DescriptorKey{}, fmt.Errorf(
				"frozen runtime identity conflict: family=%s decision=%s",
				route.AlgorithmFamily, route.DecisionKind,
			)
		}
		observeRuntimeCompat(CompatibilityHit{Source: CompatibilitySourceFrozen}, "runtime")
		return DescriptorKey{
			AlgorithmFamily: route.AlgorithmFamily,
			DecisionKind:    route.DecisionKind,
			PayloadFormat:   route.PayloadFormat,
		}, nil
	}
	return NewCompatibilityResolver().ResolveDescriptorKey(route)
}

// ExecutionFamilyFromRoute 解析执行家族 using frozen RuntimeIdentity first.
func ExecutionFamilyFromRoute(route ModelRoute) (modelcatalog.AlgorithmFamily, bool) {
	family, _, ok := ExecutionFamilyFromRouteWithCompat(route)
	return family, ok
}

// ExecutionFamilyFromRouteWithCompat prefers frozen AlgorithmFamily; otherwise uses
// explicit compatibility derivation for legacy snapshots.
func ExecutionFamilyFromRouteWithCompat(route ModelRoute) (modelcatalog.AlgorithmFamily, CompatibilityHit, bool) {
	if route.AlgorithmFamily != "" {
		return route.AlgorithmFamily, CompatibilityHit{Source: CompatibilitySourceFrozen}, true
	}
	if family, ok := modelcatalog.AlgorithmFamilyFromIdentity(route.Kind, route.SubKind, route.Algorithm); ok {
		return family, CompatibilityHit{Used: true, Source: CompatibilitySourceIdentity}, true
	}
	if family, ok := legacyTypologyFamilyFromRoute(route); ok {
		return family, CompatibilityHit{Used: true, Source: CompatibilitySourceLegacyTypology}, true
	}
	if route.DecisionKind != "" {
		if family, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(route.DecisionKind); ok {
			return family, CompatibilityHit{Used: true, Source: CompatibilitySourceDecisionKind}, true
		}
	}
	return "", CompatibilityHit{}, false
}

// ExecutionDecisionFromRoute 解析判定类型 aligned 使用 执行家族。
func ExecutionDecisionFromRoute(route ModelRoute, family modelcatalog.AlgorithmFamily) modelcatalog.DecisionKind {
	decision, _ := ExecutionDecisionFromRouteWithCompat(route, family)
	return decision
}

// ExecutionDecisionFromRouteWithCompat prefers frozen DecisionKind when compatible with family.
// Incomplete legacy routes may fall back to the family default decision.
func ExecutionDecisionFromRouteWithCompat(route ModelRoute, family modelcatalog.AlgorithmFamily) (modelcatalog.DecisionKind, CompatibilityHit) {
	if route.DecisionKind != "" {
		if decisionFamily, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(route.DecisionKind); ok && decisionFamily == family {
			source := CompatibilitySourceFrozen
			if route.AlgorithmFamily == "" {
				source = CompatibilitySourceDecisionKind
			}
			return route.DecisionKind, CompatibilityHit{Used: source != CompatibilitySourceFrozen, Source: source}
		}
		// Frozen complete routes must not silently replace an incompatible decision.
		if route.HasFrozenRuntime() {
			return "", CompatibilityHit{}
		}
	}
	return DecisionKindForFamily(family), CompatibilityHit{Used: true, Source: CompatibilitySourceFamilyDefault}
}

func legacyTypologyFamilyFromRoute(route ModelRoute) (modelcatalog.AlgorithmFamily, bool) {
	switch route.Kind {
	case modelcatalog.KindTypology:
		if route.SubKind == "" {
			return modelcatalog.AlgorithmFamilyFactorClassification, true
		}
	}
	return "", false
}

// DecisionKindForFamily is the canonical pure mapping from an algorithm family
// to its default decision kind.
func DecisionKindForFamily(family modelcatalog.AlgorithmFamily) modelcatalog.DecisionKind {
	switch family {
	case modelcatalog.AlgorithmFamilyFactorScoring:
		return modelcatalog.DecisionKindScoreRange
	case modelcatalog.AlgorithmFamilyFactorClassification:
		return modelcatalog.DecisionKindPoleComposition
	case modelcatalog.AlgorithmFamilyFactorNorm:
		return modelcatalog.DecisionKindNormLookup
	case modelcatalog.AlgorithmFamilyTaskPerformance:
		return modelcatalog.DecisionKindAbilityLevel
	default:
		return ""
	}
}
