package identity_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestResolveRuntimeIdentityFreezesCompatibleRoute(t *testing.T) {
	t.Parallel()

	got, err := identity.ResolveRuntimeIdentity(
		identity.KindScale, "", identity.AlgorithmScaleDefault,
		identity.DecisionKindScoreRange,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.AlgorithmFamily != identity.AlgorithmFamilyFactorScoring || !got.Complete() {
		t.Fatalf("got %#v", got)
	}
}

func TestResolveRuntimeIdentityRejectsFamilyConflict(t *testing.T) {
	t.Parallel()

	_, err := identity.ResolveRuntimeIdentity(
		identity.KindBehavioralRating, "", identity.AlgorithmBrief2,
		identity.DecisionKindScoreRange,
	)
	if err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestResolveRuntimeIdentityRejectsMissingDecision(t *testing.T) {
	t.Parallel()

	_, err := identity.ResolveRuntimeIdentity(
		identity.KindScale, "", identity.AlgorithmScaleDefault,
		"",
	)
	if err == nil {
		t.Fatal("expected missing decision error")
	}
}
