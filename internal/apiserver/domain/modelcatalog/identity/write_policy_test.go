package identity_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestClassifyAlgorithmWritePolicy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                string
		kind                binding.Kind
		algorithm           binding.Algorithm
		want                identity.AlgorithmWritePolicy
	}{
		{name: "scale_default", kind: binding.KindScale, algorithm: binding.AlgorithmScaleDefault, want: identity.AlgorithmWriteCanonical},
		{name: "scale_empty", kind: binding.KindScale, want: identity.AlgorithmWriteDraftOK},
		{name: "typology_canonical", kind: binding.KindTypology, algorithm: binding.AlgorithmPersonalityTypology, want: identity.AlgorithmWriteCanonical},
		{name: "typology_mbti_retired", kind: binding.KindTypology, algorithm: binding.AlgorithmMBTI, want: identity.AlgorithmWriteUnknown},
		{name: "typology_empty_draft", kind: binding.KindTypology, want: identity.AlgorithmWriteDraftOK},
		{name: "behavioral_default_retired", kind: binding.KindBehavioralRating, algorithm: binding.AlgorithmBehavioralRatingDefault, want: identity.AlgorithmWriteUnknown},
		{name: "behavioral_brief2", kind: binding.KindBehavioralRating, algorithm: binding.AlgorithmBrief2, want: identity.AlgorithmWriteCanonical},
		{name: "cognitive_spm", kind: binding.KindCognitive, algorithm: binding.AlgorithmSPM, want: identity.AlgorithmWriteCanonical},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := identity.ClassifyAlgorithmWritePolicy(tc.kind, tc.algorithm)
			if got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestAuditIdentityWritePolicyRetained(t *testing.T) {
	t.Parallel()
	issues := identity.AuditIdentityWritePolicy(binding.KindTypology, binding.AlgorithmSBTI)
	if len(issues) != 1 || issues[0].Policy != identity.AlgorithmWriteUnknown {
		t.Fatalf("issues = %#v", issues)
	}
	if issues := identity.AuditIdentityWritePolicy(binding.KindTypology, binding.AlgorithmPersonalityTypology); len(issues) != 0 {
		t.Fatalf("canonical issues = %#v", issues)
	}
}
