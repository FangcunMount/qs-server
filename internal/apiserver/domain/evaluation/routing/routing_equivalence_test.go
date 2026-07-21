package evaluation_test

import (
	"testing"

	evalrouting "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestFrozenRoutesAlignFamilyAndDecision(t *testing.T) {
	cases := []evalrouting.ModelRoute{
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring, DecisionKind: modelcatalog.DecisionKindScoreRange},
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindPoleComposition},
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindTraitProfile},
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindNearestPattern},
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindDominantFactor},
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm, DecisionKind: modelcatalog.DecisionKindNormLookup},
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyTaskPerformance, DecisionKind: modelcatalog.DecisionKindAbilityLevel},
	}
	for _, route := range cases {
		key, err := evalrouting.DescriptorKeyFromRoute(route)
		if err != nil {
			t.Fatalf("route %#v: %v", route, err)
		}
		if key.AlgorithmFamily != route.AlgorithmFamily || key.DecisionKind != route.DecisionKind {
			t.Fatalf("route %#v => key %#v", route, key)
		}
	}
}

func TestIncompleteOrConflictingRoutesFailClosed(t *testing.T) {
	cases := []evalrouting.ModelRoute{
		{Kind: modelcatalog.KindScale, Algorithm: modelcatalog.AlgorithmScaleDefault},
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring},
		{DecisionKind: modelcatalog.DecisionKindScoreRange},
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm, DecisionKind: modelcatalog.DecisionKindScoreRange},
	}
	for _, route := range cases {
		if _, err := evalrouting.DescriptorKeyFromRoute(route); err == nil {
			t.Fatalf("route %#v unexpectedly resolved", route)
		}
	}
}
