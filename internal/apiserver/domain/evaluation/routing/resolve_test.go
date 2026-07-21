package evaluation

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestDescriptorKeyFromRouteRequiresExactFrozenIdentity(t *testing.T) {
	key, err := DescriptorKeyFromRoute(ModelRoute{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm, DecisionKind: modelcatalog.DecisionKindNormLookup})
	if err != nil {
		t.Fatal(err)
	}
	if key.AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorNorm || key.DecisionKind != modelcatalog.DecisionKindNormLookup {
		t.Fatalf("key = %#v", key)
	}
	if _, err := DescriptorKeyFromRoute(ModelRoute{DecisionKind: modelcatalog.DecisionKindNormLookup}); err == nil {
		t.Fatal("missing family was accepted")
	}
}

func TestDescriptorKeyDifferentiatesDecisionWithinFamily(t *testing.T) {
	pole, _ := DescriptorKeyFromRoute(ModelRoute{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindPoleComposition})
	trait, _ := DescriptorKeyFromRoute(ModelRoute{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindTraitProfile})
	if pole == trait || pole.String() == trait.String() {
		t.Fatal("distinct decisions collapsed to one descriptor key")
	}
}
