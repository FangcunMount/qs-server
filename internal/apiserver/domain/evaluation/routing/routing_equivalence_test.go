package evaluation_test

import (
	"testing"

	evalrouting "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestFrozenRoutesAlignFamilyAndDecision(t *testing.T) {
	cases := []evalrouting.ModelRoute{
		{DecisionKind: modelcatalog.DecisionKindScoreRange}, {DecisionKind: modelcatalog.DecisionKindPoleComposition},
		{DecisionKind: modelcatalog.DecisionKindTraitProfile}, {DecisionKind: modelcatalog.DecisionKindNearestPattern},
		{DecisionKind: modelcatalog.DecisionKindDominantFactor}, {DecisionKind: modelcatalog.DecisionKindNormLookup},
		{DecisionKind: modelcatalog.DecisionKindAbilityLevel},
	}
	for _, route := range cases {
		key, err := evalrouting.DescriptorKeyFromRoute(route)
		if err != nil {
			t.Fatalf("route %#v: %v", route, err)
		}
		if key.DecisionKind != route.DecisionKind {
			t.Fatalf("route %#v => key %#v", route, key)
		}
	}
}

func TestIncompleteOrConflictingRoutesFailClosed(t *testing.T) {
	cases := []evalrouting.ModelRoute{
		{}, {DecisionKind: "unknown"},
	}
	for _, route := range cases {
		if _, err := evalrouting.DescriptorKeyFromRoute(route); err == nil {
			t.Fatalf("route %#v unexpectedly resolved", route)
		}
	}
}
