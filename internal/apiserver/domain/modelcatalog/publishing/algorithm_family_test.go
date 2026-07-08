package publishing_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
)

func TestAlgorithmFamilyFromDecisionKind(t *testing.T) {
	tests := []struct {
		decision binding.DecisionKind
		want     publishing.AlgorithmFamily
	}{
		{binding.DecisionKindScoreRange, publishing.AlgorithmFamilyFactorScoring},
		{binding.DecisionKind("score_range_interpretation"), publishing.AlgorithmFamilyFactorScoring},
		{binding.DecisionKindPoleComposition, publishing.AlgorithmFamilyFactorClassification},
		{binding.DecisionKindTraitProfile, publishing.AlgorithmFamilyFactorClassification},
		{binding.DecisionKindNearestPattern, publishing.AlgorithmFamilyFactorClassification},
		{binding.DecisionKindNormLookup, publishing.AlgorithmFamilyFactorNorm},
		{binding.DecisionKindAbilityLevel, publishing.AlgorithmFamilyTaskPerformance},
	}
	for _, tc := range tests {
		got, ok := publishing.AlgorithmFamilyFromDecisionKind(tc.decision)
		if !ok {
			t.Fatalf("AlgorithmFamilyFromDecisionKind(%s) ok = false", tc.decision)
		}
		if got != tc.want {
			t.Fatalf("AlgorithmFamilyFromDecisionKind(%s) = %q, want %q", tc.decision, got, tc.want)
		}
	}
}

func TestAlgorithmFamilyFromIdentityMatrix(t *testing.T) {
	tests := []struct {
		name      string
		kind      binding.Kind
		subKind   binding.SubKind
		algorithm binding.Algorithm
		want      publishing.AlgorithmFamily
		wantOK    bool
	}{
		{name: "scale", kind: binding.KindScale, algorithm: binding.AlgorithmScaleDefault, want: publishing.AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "personality_mbti", kind: binding.KindTypology, subKind: binding.SubKindTypology, algorithm: binding.AlgorithmMBTI, want: publishing.AlgorithmFamilyFactorClassification, wantOK: true},
		{name: "personality_sbti", kind: binding.KindTypology, subKind: binding.SubKindTypology, algorithm: binding.AlgorithmSBTI, want: publishing.AlgorithmFamilyFactorClassification, wantOK: true},
		{name: "personality_bigfive", kind: binding.KindTypology, subKind: binding.SubKindTypology, algorithm: binding.AlgorithmBigFive, want: publishing.AlgorithmFamilyFactorClassification, wantOK: true},
		{name: "behavioral_rating_brief2", kind: binding.KindBehavioralRating, algorithm: binding.AlgorithmBrief2, want: publishing.AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "behavioral_rating_default", kind: binding.KindBehavioralRating, algorithm: binding.AlgorithmBehavioralRatingDefault, want: publishing.AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "behavioral_rating_empty_algo", kind: binding.KindBehavioralRating, algorithm: "", want: publishing.AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "cognitive_spm", kind: binding.KindCognitive, algorithm: binding.AlgorithmSPM, want: publishing.AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "cognitive_empty_algo", kind: binding.KindCognitive, algorithm: "", want: publishing.AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "custom", kind: binding.KindCustom, wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := publishing.AlgorithmFamilyFromIdentity(tc.kind, tc.subKind, tc.algorithm)
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
	cases := []struct {
		kind      binding.Kind
		subKind   binding.SubKind
		algorithm binding.Algorithm
		decision  binding.DecisionKind
	}{
		{binding.KindScale, binding.SubKindEmpty, binding.AlgorithmScaleDefault, binding.DecisionKindScoreRange},
		{binding.KindBehavioralRating, binding.SubKindEmpty, binding.AlgorithmBrief2, binding.DecisionKindScoreRange},
		{binding.KindBehavioralRating, binding.SubKindEmpty, binding.AlgorithmBehavioralRatingDefault, binding.DecisionKindScoreRange},
		{binding.KindBehavioralRating, binding.SubKindEmpty, "", binding.DecisionKindScoreRange},
		{binding.KindCognitive, binding.SubKindEmpty, binding.AlgorithmSPM, binding.DecisionKindScoreRange},
	}
	for _, tc := range cases {
		decision, ok := publishing.DecisionKindForIdentity(tc.kind, tc.subKind, tc.algorithm)
		if !ok {
			t.Fatalf("DecisionKindForIdentity(%s,%s,%s) ok = false", tc.kind, tc.subKind, tc.algorithm)
		}
		if decision != tc.decision {
			t.Fatalf("DecisionKindForIdentity(%s,%s,%s) = %s, want %s", tc.kind, tc.subKind, tc.algorithm, decision, tc.decision)
		}
		fromIdentity, ok := publishing.AlgorithmFamilyFromIdentity(tc.kind, tc.subKind, tc.algorithm)
		if !ok {
			t.Fatalf("AlgorithmFamilyFromIdentity(%s,%s,%s) ok = false", tc.kind, tc.subKind, tc.algorithm)
		}
		fromDecision, ok := publishing.AlgorithmFamilyFromDecisionKind(decision)
		if !ok {
			t.Fatalf("AlgorithmFamilyFromDecisionKind(%s) ok = false", decision)
		}
		if fromIdentity != fromDecision {
			t.Fatalf("identity family %q != decision family %q for %+v", fromIdentity, fromDecision, tc)
		}
	}
}

func TestDecisionKindForIdentityRequiresExplicitTypologyDecision(t *testing.T) {
	if _, ok := publishing.DecisionKindForIdentity(binding.KindTypology, binding.SubKindTypology, binding.AlgorithmMBTI); ok {
		t.Fatal("personality typology must not infer decision.kind from algorithm")
	}
	family, ok := publishing.AlgorithmFamilyFromIdentity(binding.KindTypology, binding.SubKindTypology, binding.AlgorithmMBTI)
	if !ok || family != publishing.AlgorithmFamilyFactorClassification {
		t.Fatalf("family = %s ok=%v, want factor_classification", family, ok)
	}
}
