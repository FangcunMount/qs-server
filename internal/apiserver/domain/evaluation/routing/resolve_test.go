package evaluation

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestDescriptorKeyFromRouteUsesDecisionKind(t *testing.T) {
	t.Parallel()

	key, err := DescriptorKeyFromRoute(ModelRoute{
		DecisionKind:  modelcatalog.DecisionKindNormLookup,
		PayloadFormat: modelcatalog.PayloadFormatBehavioralRatingBrief2V1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if key.AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorNorm {
		t.Fatalf("family=%s want=%s", key.AlgorithmFamily, modelcatalog.AlgorithmFamilyFactorNorm)
	}
	if key.PayloadFormat != modelcatalog.PayloadFormatBehavioralRatingBrief2V1 {
		t.Fatalf("payload format=%s", key.PayloadFormat)
	}
	if key.DecisionKind != modelcatalog.DecisionKindNormLookup {
		t.Fatalf("decision kind=%s want=%s", key.DecisionKind, modelcatalog.DecisionKindNormLookup)
	}
}

func TestDescriptorKeyFromRouteDifferentiatesDecisionKindWithinFamily(t *testing.T) {
	t.Parallel()

	pole, err := DescriptorKeyFromRoute(ModelRoute{
		DecisionKind:  modelcatalog.DecisionKindPoleComposition,
		PayloadFormat: modelcatalog.PayloadFormatPersonalityTypologyV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	trait, err := DescriptorKeyFromRoute(ModelRoute{
		DecisionKind:  modelcatalog.DecisionKindTraitProfile,
		PayloadFormat: modelcatalog.PayloadFormatPersonalityTypologyV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pole.AlgorithmFamily != trait.AlgorithmFamily {
		t.Fatalf("families diverged: pole=%s trait=%s", pole.AlgorithmFamily, trait.AlgorithmFamily)
	}
	if pole.DecisionKind == trait.DecisionKind {
		t.Fatalf("decision kinds should differ within same family: %s", pole.DecisionKind)
	}
	if pole.String() == trait.String() {
		t.Fatalf("key strings should differ: %s", pole.String())
	}
}
