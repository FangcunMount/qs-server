package evaluation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// DescriptorKeyFromRoute derives the single runtime routing key from a model route.
func DescriptorKeyFromRoute(route ModelRoute) (DescriptorKey, error) {
	family, ok := ExecutionFamilyFromRoute(route)
	if !ok {
		return DescriptorKey{}, fmt.Errorf("unsupported model route for runtime descriptor: %s/%s", route.Kind, route.Algorithm)
	}
	return DescriptorKey{
		AlgorithmFamily: family,
		DecisionKind:    ExecutionDecisionFromRoute(route, family),
		PayloadFormat:   route.PayloadFormat,
	}, nil
}

// ExecutionFamilyFromRoute 解析执行家族 using modelcatalog identity as the primary route.
func ExecutionFamilyFromRoute(route ModelRoute) (modelcatalog.AlgorithmFamily, bool) {
	if family, ok := modelcatalog.AlgorithmFamilyFromIdentity(route.Kind, route.SubKind, route.Algorithm); ok {
		return family, true
	}
	if family, ok := legacyTypologyFamilyFromRoute(route); ok {
		return family, true
	}
	if route.DecisionKind != "" {
		return modelcatalog.AlgorithmFamilyFromDecisionKind(route.DecisionKind)
	}
	return modelcatalog.AlgorithmFamilyFromIdentity(route.Kind, route.SubKind, route.Algorithm)
}

// ExecutionDecisionFromRoute 解析判定类型 aligned 使用 执行家族。
func ExecutionDecisionFromRoute(route ModelRoute, family modelcatalog.AlgorithmFamily) modelcatalog.DecisionKind {
	if route.DecisionKind != "" {
		if decisionFamily, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(route.DecisionKind); ok && decisionFamily == family {
			return route.DecisionKind
		}
	}
	return DecisionKindForFamily(family)
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
