package identity_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestClassifyAlgorithmWritePolicy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		kind      binding.Kind
		algorithm binding.Algorithm
		want      identity.AlgorithmWritePolicy
	}{
		{name: "typology_canonical", kind: binding.KindTypology, algorithm: binding.AlgorithmPersonalityTypology, want: identity.AlgorithmWriteCanonical},
		{name: "typology_mbti_retained", kind: binding.KindTypology, algorithm: binding.AlgorithmMBTI, want: identity.AlgorithmWriteRetainedRead},
		{name: "typology_empty_draft", kind: binding.KindTypology, want: identity.AlgorithmWriteDraftOK},
		{name: "behavioral_default_retained", kind: binding.KindBehavioralRating, algorithm: binding.AlgorithmBehavioralRatingDefault, want: identity.AlgorithmWriteRetainedRead},
		{name: "behavioral_brief2", kind: binding.KindBehavioralRating, algorithm: binding.AlgorithmBrief2, want: identity.AlgorithmWriteCanonical},
		{name: "cognitive_empty_draft", kind: binding.KindCognitive, want: identity.AlgorithmWriteDraftOK},
		{name: "cognitive_spm", kind: binding.KindCognitive, algorithm: binding.AlgorithmSPM, want: identity.AlgorithmWriteCanonical},
		{name: "scale_default", kind: binding.KindScale, algorithm: binding.AlgorithmScaleDefault, want: identity.AlgorithmWriteCanonical},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := identity.ClassifyAlgorithmWritePolicy(tc.kind, tc.algorithm)
			if got != tc.want {
				t.Fatalf("policy = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAuditIdentityWritePolicyFlagsRetained(t *testing.T) {
	t.Parallel()
	issues := identity.AuditIdentityWritePolicy(binding.KindTypology, binding.AlgorithmSBTI)
	if len(issues) != 1 || issues[0].Code != "identity.algorithm.retained_read" {
		t.Fatalf("issues = %#v", issues)
	}
	if issues := identity.AuditIdentityWritePolicy(binding.KindTypology, binding.AlgorithmPersonalityTypology); len(issues) != 0 {
		t.Fatalf("canonical should be clean: %#v", issues)
	}
}
