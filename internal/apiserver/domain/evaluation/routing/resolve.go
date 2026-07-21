package evaluation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// DescriptorKeyFromRoute accepts only a complete, internally consistent
// publish-time route. There is no format, family, or model-reference fallback.
func DescriptorKeyFromRoute(route ModelRoute) (DescriptorKey, error) {
	if !route.HasFrozenRuntime() {
		return DescriptorKey{}, fmt.Errorf("complete frozen runtime identity is required")
	}
	family, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(route.DecisionKind)
	if !ok || family != route.AlgorithmFamily {
		return DescriptorKey{}, fmt.Errorf("frozen runtime identity conflict: family=%s decision=%s", route.AlgorithmFamily, route.DecisionKind)
	}
	return DescriptorKey{AlgorithmFamily: route.AlgorithmFamily, DecisionKind: route.DecisionKind}, nil
}

func ExecutionFamilyFromRoute(route ModelRoute) (modelcatalog.AlgorithmFamily, bool) {
	if _, err := DescriptorKeyFromRoute(route); err != nil {
		return "", false
	}
	return route.AlgorithmFamily, true
}

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
