package identity

import (
	"errors"
	"testing"
)

func TestResolveLegacyRuntime(t *testing.T) {
	t.Run("derives deterministic historical identity", func(t *testing.T) {
		got, err := ResolveLegacyRuntime(KindBehavioralRating, AlgorithmBrief2, "")
		if err != nil || got.DecisionKind != DecisionKindNormLookup || got.AlgorithmFamily != AlgorithmFamilyFactorNorm {
			t.Fatalf("runtime=%#v err=%v", got, err)
		}
	})
	t.Run("rejects ambiguous typology history", func(t *testing.T) {
		if _, err := ResolveLegacyRuntime(KindTypology, AlgorithmPersonalityTypology, ""); !errors.Is(err, ErrLegacyRuntimeIdentity) {
			t.Fatal("expected error")
		}
	})
	t.Run("rejects conflict", func(t *testing.T) {
		if _, err := ResolveLegacyRuntime(KindScale, AlgorithmScaleDefault, DecisionKindNormLookup); !errors.Is(err, ErrLegacyRuntimeIdentity) {
			t.Fatal("expected error")
		}
	})
}
