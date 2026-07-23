package evaluation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// DescriptorKeyFromRoute accepts only a frozen DecisionKind. AlgorithmFamily is
// an in-process implementation detail derived from that canonical input.
func DescriptorKeyFromRoute(route ModelRoute) (DescriptorKey, error) {
	if !route.HasFrozenRuntime() {
		return DescriptorKey{}, fmt.Errorf("frozen decision_kind is required")
	}
	if _, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(route.DecisionKind); !ok {
		return DescriptorKey{}, fmt.Errorf("unknown frozen decision_kind: %s", route.DecisionKind)
	}
	return DescriptorKey{DecisionKind: route.DecisionKind}, nil
}

func ExecutionFamilyFromRoute(route ModelRoute) (modelcatalog.AlgorithmFamily, bool) {
	if _, err := DescriptorKeyFromRoute(route); err != nil {
		return "", false
	}
	family, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(route.DecisionKind)
	return family, ok
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
