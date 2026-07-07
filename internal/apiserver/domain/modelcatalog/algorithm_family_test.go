package modelcatalog

import "testing"

func TestDefaultProductChannelFor(t *testing.T) {
	tests := []struct {
		kind Kind
		want ProductChannel
	}{
		{KindScale, ProductChannelMedicalScale},
		{KindPersonality, ProductChannelPersonality},
		{KindBehaviorAbility, ProductChannelBehaviorAbility},
		{KindBehavioralRating, ProductChannelBehaviorAbility},
		{KindCognitive, ProductChannelCognitive},
		{KindCustom, ProductChannelCustom},
	}
	for _, tc := range tests {
		if got := DefaultProductChannelFor(tc.kind); got != tc.want {
			t.Fatalf("DefaultProductChannelFor(%s) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestAlgorithmFamilyFromDecisionKind(t *testing.T) {
	tests := []struct {
		decision DecisionKind
		want     AlgorithmFamily
	}{
		{DecisionKindScoreRange, AlgorithmFamilyFactorScoring},
		{DecisionKindScoreRangeInterpretation, AlgorithmFamilyFactorScoring},
		{DecisionKindPoleComposition, AlgorithmFamilyFactorClassification},
		{DecisionKindTraitProfile, AlgorithmFamilyFactorClassification},
		{DecisionKindNearestPattern, AlgorithmFamilyFactorClassification},
		{DecisionKindNormLookup, AlgorithmFamilyFactorNorm},
		{DecisionKindAbilityLevel, AlgorithmFamilyTaskPerformance},
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
		kind      Kind
		subKind   SubKind
		algorithm Algorithm
		want      AlgorithmFamily
		wantOK    bool
	}{
		{name: "scale", kind: KindScale, algorithm: AlgorithmScaleDefault, want: AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "personality_mbti", kind: KindPersonality, subKind: SubKindTypology, algorithm: AlgorithmMBTI, want: AlgorithmFamilyFactorClassification, wantOK: true},
		{name: "personality_sbti", kind: KindPersonality, subKind: SubKindTypology, algorithm: AlgorithmSBTI, want: AlgorithmFamilyFactorClassification, wantOK: true},
		{name: "personality_bigfive", kind: KindPersonality, subKind: SubKindTypology, algorithm: AlgorithmBigFive, want: AlgorithmFamilyFactorClassification, wantOK: true},
		{name: "behavioral_rating_brief2", kind: KindBehavioralRating, algorithm: AlgorithmBrief2, want: AlgorithmFamilyFactorNorm, wantOK: true},
		{name: "behavioral_rating_default", kind: KindBehavioralRating, algorithm: AlgorithmBehavioralRatingDefault, want: AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "behavioral_rating_empty_algo", kind: KindBehavioralRating, algorithm: "", want: AlgorithmFamilyFactorNorm, wantOK: true},
		{name: "cognitive_spm", kind: KindCognitive, algorithm: AlgorithmSPM, want: AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "cognitive_empty_algo", kind: KindCognitive, algorithm: "", want: AlgorithmFamilyFactorScoring, wantOK: true},
		{name: "behavior_ability_channel", kind: KindBehaviorAbility, wantOK: false},
		{name: "custom", kind: KindCustom, wantOK: false},
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
		kind      Kind
		subKind   SubKind
		algorithm Algorithm
		decision  DecisionKind
	}{
		{KindScale, SubKindEmpty, AlgorithmScaleDefault, DecisionKindScoreRange},
		{KindPersonality, SubKindTypology, AlgorithmMBTI, DecisionKindPoleComposition},
		{KindPersonality, SubKindTypology, AlgorithmSBTI, DecisionKindNearestPattern},
		{KindPersonality, SubKindTypology, AlgorithmBigFive, DecisionKindTraitProfile},
		{KindBehavioralRating, SubKindEmpty, AlgorithmBrief2, DecisionKindNormLookup},
		{KindBehavioralRating, SubKindEmpty, AlgorithmBehavioralRatingDefault, DecisionKindScoreRange},
		{KindBehavioralRating, SubKindEmpty, "", DecisionKindNormLookup},
		{KindCognitive, SubKindEmpty, AlgorithmSPM, DecisionKindScoreRange},
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

func TestCompleteProductChannel(t *testing.T) {
	got, err := CompleteProductChannel(KindBehavioralRating, ProductChannelMedicalScale)
	if err != nil {
		t.Fatalf("CompleteProductChannel: %v", err)
	}
	if got != ProductChannelMedicalScale {
		t.Fatalf("got %q, want medical_scale", got)
	}

	if _, err := CompleteProductChannel(KindBehavioralRating, ProductChannel("invalid")); err == nil {
		t.Fatal("expected invalid product channel error")
	}

	got, err = CompleteProductChannel(KindBehavioralRating, "")
	if err != nil {
		t.Fatalf("CompleteProductChannel default: %v", err)
	}
	if got != ProductChannelBehaviorAbility {
		t.Fatalf("default channel = %q, want behavior_ability", got)
	}
}
