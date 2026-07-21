package identity_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestIdentityDerivesFamilyAndDecision(t *testing.T) {
	t.Parallel()

	rating := identity.New(binding.KindBehavioralRating, binding.SubKindEmpty, binding.AlgorithmBrief2)
	family, ok := rating.Family()
	if !ok || family != identity.FamilyFactorNorm {
		t.Fatalf("rating family = %q ok=%v, want factor_norm", family, ok)
	}
	decision, ok := rating.DecisionKind()
	if !ok || decision != binding.DecisionKindNormLookup {
		t.Fatalf("rating decision = %q ok=%v, want norm_lookup", decision, ok)
	}

	typology := identity.New(binding.KindTypology, binding.SubKindTypology, binding.AlgorithmPersonalityTypology)
	family, ok = typology.Family()
	if !ok || family != identity.FamilyFactorClassification {
		t.Fatalf("typology family = %q ok=%v, want factor_classification", family, ok)
	}
	if _, ok := typology.DecisionKind(); ok {
		t.Fatal("typology decision must be payload-derived, not identity-derived")
	}
}

func TestAlgorithmFamilyFromDecisionKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		decision binding.DecisionKind
		want     identity.AlgorithmFamily
	}{
		{binding.DecisionKindScoreRange, identity.AlgorithmFamilyFactorScoring},
		{binding.DecisionKindPoleComposition, identity.AlgorithmFamilyFactorClassification},
		{binding.DecisionKindTraitProfile, identity.AlgorithmFamilyFactorClassification},
		{binding.DecisionKindNearestPattern, identity.AlgorithmFamilyFactorClassification},
		{binding.DecisionKindNormLookup, identity.AlgorithmFamilyFactorNorm},
		{binding.DecisionKindAbilityLevel, identity.AlgorithmFamilyTaskPerformance},
	}
	for _, tc := range tests {
		got, ok := identity.AlgorithmFamilyFromDecisionKind(tc.decision)
		if !ok {
			t.Fatalf("AlgorithmFamilyFromDecisionKind(%s) ok = false", tc.decision)
		}
		if got != tc.want {
			t.Fatalf("AlgorithmFamilyFromDecisionKind(%s) = %q, want %q", tc.decision, got, tc.want)
		}
	}
}

func TestAlgorithmFamilyFromIdentityMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		kind      binding.Kind
		subKind   binding.SubKind
		algorithm binding.Algorithm
		want      identity.AlgorithmFamily
		wantOK    bool
	}{
		{name: "scale", kind: binding.KindScale, algorithm: binding.AlgorithmScaleDefault, want: identity.AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "personality_mbti", kind: binding.KindTypology, subKind: binding.SubKindTypology, algorithm: binding.AlgorithmPersonalityTypology, want: identity.AlgorithmFamilyFactorClassification, wantOK: true},
		{name: "behavioral_rating_brief2", kind: binding.KindBehavioralRating, algorithm: binding.AlgorithmBrief2, want: identity.AlgorithmFamilyFactorNorm, wantOK: true},
		{name: "behavioral_rating_spm_sensory", kind: binding.KindBehavioralRating, algorithm: binding.AlgorithmSPMSensory, want: identity.AlgorithmFamilyFactorNorm, wantOK: true},
		{name: "behavioral_rating_empty_algo", kind: binding.KindBehavioralRating, algorithm: "", want: identity.AlgorithmFamilyFactorNorm, wantOK: true},
		{name: "cognitive_spm", kind: binding.KindCognitive, algorithm: binding.AlgorithmSPM, want: identity.AlgorithmFamilyTaskPerformance, wantOK: true},
		{name: "cognitive_empty_algo", kind: binding.KindCognitive, algorithm: "", want: identity.AlgorithmFamilyTaskPerformance, wantOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := identity.AlgorithmFamilyFromIdentity(tc.kind, tc.subKind, tc.algorithm)
			if ok != tc.wantOK {
				t.Fatalf("AlgorithmFamilyFromIdentity ok = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if got != tc.want {
				t.Fatalf("AlgorithmFamilyFromIdentity = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAlgorithmFamilyIdentityMatchesPublishDecision(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind      binding.Kind
		subKind   binding.SubKind
		algorithm binding.Algorithm
		decision  binding.DecisionKind
	}{
		{binding.KindScale, binding.SubKindEmpty, binding.AlgorithmScaleDefault, binding.DecisionKindScoreRange},
		{binding.KindBehavioralRating, binding.SubKindEmpty, binding.AlgorithmSPMSensory, binding.DecisionKindNormLookup},
		{binding.KindBehavioralRating, binding.SubKindEmpty, binding.AlgorithmBrief2, binding.DecisionKindNormLookup},
		{binding.KindBehavioralRating, binding.SubKindEmpty, "", binding.DecisionKindNormLookup},
		{binding.KindCognitive, binding.SubKindEmpty, binding.AlgorithmSPM, binding.DecisionKindAbilityLevel},
	}
	for _, tc := range cases {
		decision, ok := identity.DecisionKindForIdentity(tc.kind, tc.subKind, tc.algorithm)
		if !ok {
			t.Fatalf("DecisionKindForIdentity(%s,%s,%s) ok = false", tc.kind, tc.subKind, tc.algorithm)
		}
		if decision != tc.decision {
			t.Fatalf("DecisionKindForIdentity(%s,%s,%s) = %s, want %s", tc.kind, tc.subKind, tc.algorithm, decision, tc.decision)
		}
		fromIdentity, ok := identity.AlgorithmFamilyFromIdentity(tc.kind, tc.subKind, tc.algorithm)
		if !ok {
			t.Fatalf("AlgorithmFamilyFromIdentity(%s,%s,%s) ok = false", tc.kind, tc.subKind, tc.algorithm)
		}
		fromDecision, ok := identity.AlgorithmFamilyFromDecisionKind(decision)
		if !ok {
			t.Fatalf("AlgorithmFamilyFromDecisionKind(%s) ok = false", decision)
		}
		if fromIdentity != fromDecision {
			t.Fatalf("identity family %q != decision family %q for %+v", fromIdentity, fromDecision, tc)
		}
	}
}

func TestDecisionKindForIdentityRequiresExplicitTypologyDecision(t *testing.T) {
	t.Parallel()

	if _, ok := identity.DecisionKindForIdentity(binding.KindTypology, binding.SubKindTypology, binding.AlgorithmPersonalityTypology); ok {
		t.Fatal("personality typology must not infer decision.kind from algorithm")
	}
	family, ok := identity.AlgorithmFamilyFromIdentity(binding.KindTypology, binding.SubKindTypology, binding.AlgorithmPersonalityTypology)
	if !ok || family != identity.AlgorithmFamilyFactorClassification {
		t.Fatalf("family = %s ok=%v, want factor_classification", family, ok)
	}
}

func TestIdentityRoutingStringHelpers(t *testing.T) {
	t.Parallel()

	if got := binding.ProductChannelForIdentity(binding.KindCognitive, string(binding.ProductChannelBehaviorAbility)); got != string(binding.ProductChannelBehaviorAbility) {
		t.Fatalf("ProductChannelForIdentity() = %q, want behavior_ability", got)
	}
	if got := identity.AlgorithmFamilyStringFromIdentity(binding.KindCognitive, binding.SubKindEmpty, binding.AlgorithmSPM); got != string(identity.AlgorithmFamilyTaskPerformance) {
		t.Fatalf("AlgorithmFamilyStringFromIdentity() = %q, want task_performance", got)
	}
}
