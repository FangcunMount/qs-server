package evaluation

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestPersonalityTypologyIdentity(t *testing.T) {
	got := PersonalityTypologyIdentity(modelcatalog.AlgorithmMBTI)
	if got.String() != "typology/typology/mbti" {
		t.Fatalf("identity string = %s", got.String())
	}
}

func TestBehavioralRatingIdentityKeepsExactAlgorithm(t *testing.T) {
	got := BehavioralRatingIdentity(modelcatalog.AlgorithmBrief2)
	if got.String() != "behavioral_rating//brief2" {
		t.Fatalf("identity string = %s", got.String())
	}
	if BehavioralRatingIdentity(modelcatalog.AlgorithmBrief2) == ExecutionIdentityBehavioralRatingDefault {
		t.Fatal("brief2 must not equal retired family route key")
	}
}

func TestCognitiveIdentityKeepsExactAlgorithm(t *testing.T) {
	got := CognitiveIdentity(modelcatalog.AlgorithmSPM)
	if got.String() != "cognitive//spm" {
		t.Fatalf("identity string = %s", got.String())
	}
	if got != ExecutionIdentityCognitiveDefault {
		t.Fatal("spm cognitive identity should equal CognitiveDefault constant")
	}
}
