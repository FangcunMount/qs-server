package routing

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestAlgorithmFamilyFromDecisionKind(t *testing.T) {
	tests := []struct {
		decision identity.DecisionKind
		want     AlgorithmFamily
	}{
		{identity.DecisionKindScoreRange, AlgorithmFamilyFactorScoring},
		{identity.DecisionKind("score_range_interpretation"), AlgorithmFamilyFactorScoring},
		{identity.DecisionKindPoleComposition, AlgorithmFamilyFactorClassification},
		{identity.DecisionKindTraitProfile, AlgorithmFamilyFactorClassification},
		{identity.DecisionKindNearestPattern, AlgorithmFamilyFactorClassification},
		{identity.DecisionKindNormLookup, AlgorithmFamilyFactorNorm},
		{identity.DecisionKindAbilityLevel, AlgorithmFamilyTaskPerformance},
	}
	for _, tc := range tests {
		got, ok := AlgorithmFamilyFromDecisionKind(tc.decision)
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
		kind      identity.Kind
		subKind   identity.SubKind
		algorithm identity.Algorithm
		want      AlgorithmFamily
		wantOK    bool
	}{
		{name: "scale", kind: identity.KindScale, algorithm: identity.AlgorithmScaleDefault, want: AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "personality_mbti", kind: identity.KindPersonality, subKind: identity.SubKindTypology, algorithm: identity.AlgorithmMBTI, want: AlgorithmFamilyFactorClassification, wantOK: true},
		{name: "personality_sbti", kind: identity.KindPersonality, subKind: identity.SubKindTypology, algorithm: identity.AlgorithmSBTI, want: AlgorithmFamilyFactorClassification, wantOK: true},
		{name: "personality_bigfive", kind: identity.KindPersonality, subKind: identity.SubKindTypology, algorithm: identity.AlgorithmBigFive, want: AlgorithmFamilyFactorClassification, wantOK: true},
		{name: "behavioral_rating_brief2", kind: identity.KindBehavioralRating, algorithm: identity.AlgorithmBrief2, want: AlgorithmFamilyFactorNorm, wantOK: true},
		{name: "behavioral_rating_default", kind: identity.KindBehavioralRating, algorithm: identity.AlgorithmBehavioralRatingDefault, want: AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "behavioral_rating_empty_algo", kind: identity.KindBehavioralRating, algorithm: "", want: AlgorithmFamilyFactorNorm, wantOK: true},
		{name: "cognitive_spm", kind: identity.KindCognitive, algorithm: identity.AlgorithmSPM, want: AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "cognitive_empty_algo", kind: identity.KindCognitive, algorithm: "", want: AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "custom", kind: identity.KindCustom, wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := AlgorithmFamilyFromIdentity(tc.kind, tc.subKind, tc.algorithm)
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
		kind      identity.Kind
		subKind   identity.SubKind
		algorithm identity.Algorithm
		decision  identity.DecisionKind
	}{
		{identity.KindScale, identity.SubKindEmpty, identity.AlgorithmScaleDefault, identity.DecisionKindScoreRange},
		{identity.KindPersonality, identity.SubKindTypology, identity.AlgorithmMBTI, identity.DecisionKindPoleComposition},
		{identity.KindPersonality, identity.SubKindTypology, identity.AlgorithmSBTI, identity.DecisionKindNearestPattern},
		{identity.KindPersonality, identity.SubKindTypology, identity.AlgorithmBigFive, identity.DecisionKindTraitProfile},
		{identity.KindBehavioralRating, identity.SubKindEmpty, identity.AlgorithmBrief2, identity.DecisionKindNormLookup},
		{identity.KindBehavioralRating, identity.SubKindEmpty, identity.AlgorithmBehavioralRatingDefault, identity.DecisionKindScoreRange},
		{identity.KindBehavioralRating, identity.SubKindEmpty, "", identity.DecisionKindNormLookup},
		{identity.KindCognitive, identity.SubKindEmpty, identity.AlgorithmSPM, identity.DecisionKindScoreRange},
	}
	for _, tc := range cases {
		decision, ok := DecisionKindForIdentity(tc.kind, tc.subKind, tc.algorithm)
		if !ok {
			t.Fatalf("DecisionKindForIdentity(%s,%s,%s) ok = false", tc.kind, tc.subKind, tc.algorithm)
		}
		if decision != tc.decision {
			t.Fatalf("DecisionKindForIdentity(%s,%s,%s) = %s, want %s", tc.kind, tc.subKind, tc.algorithm, decision, tc.decision)
		}
		fromIdentity, ok := AlgorithmFamilyFromIdentity(tc.kind, tc.subKind, tc.algorithm)
		if !ok {
			t.Fatalf("AlgorithmFamilyFromIdentity(%s,%s,%s) ok = false", tc.kind, tc.subKind, tc.algorithm)
		}
		fromDecision, ok := AlgorithmFamilyFromDecisionKind(decision)
		if !ok {
			t.Fatalf("AlgorithmFamilyFromDecisionKind(%s) ok = false", decision)
		}
		if fromIdentity != fromDecision {
			t.Fatalf("identity family %q != decision family %q for %+v", fromIdentity, fromDecision, tc)
		}
	}
}
