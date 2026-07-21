package evaluationinput

import (
	"strings"
	"testing"
)

func ageMonthsPtr(value int) *int { return &value }

func identityFixtureSnapshot() *InputSnapshot {
	return &InputSnapshot{
		Model: &ModelSnapshot{
			Kind: EvaluationModelKindScale, Algorithm: "scale_default",
			AlgorithmFamily: "factor_scoring", DecisionKind: "score_banding", PayloadFormat: "scale/v1",
			Code: "PHQ9", Version: "1.0.0",
		},
		Questionnaire: &QuestionnaireSnapshot{
			Code: "PHQ9-Q", Version: "1.0.0",
			Questions: []QuestionSnapshot{{
				Code: "q1", Type: "radio",
				Options: []OptionSnapshot{{Code: "a", Score: 0}, {Code: "b", Score: 1}},
			}},
		},
		AnswerSheet: &AnswerSheetSnapshot{
			ID: 2001, QuestionnaireCode: "PHQ9-Q", QuestionnaireVersion: "1.0.0",
			Answers: []AnswerSnapshot{{QuestionCode: "q1", Score: 1, Value: "b"}},
		},
		NormSubject: &NormSubjectSnapshot{AgeMonths: ageMonthsPtr(120), Gender: "male"},
	}
}

func TestInputSnapshotIdentityIsDeterministic(t *testing.T) {
	t.Parallel()
	first, ok := NewInputSnapshotIdentity(identityFixtureSnapshot())
	if !ok {
		t.Fatal("identity not derivable")
	}
	second, _ := NewInputSnapshotIdentity(identityFixtureSnapshot())
	if first != second {
		t.Fatalf("identity drifted between identical snapshots:\n%+v\n%+v", first, second)
	}
	if !strings.HasPrefix(first.Ref(), IdentityRefPrefix) {
		t.Fatalf("ref = %q", first.Ref())
	}
	if len(first.Ref()) > 200 {
		t.Fatalf("ref exceeds VARCHAR(200): %d", len(first.Ref()))
	}
}

func TestInputSnapshotIdentityDistinguishesKnownZeroAgeFromMissing(t *testing.T) {
	t.Parallel()

	missing := identityFixtureSnapshot()
	missing.NormSubject.AgeMonths = nil
	missingIdentity, _ := NewInputSnapshotIdentity(missing)

	knownZero := identityFixtureSnapshot()
	knownZero.NormSubject.AgeMonths = ageMonthsPtr(0)
	knownIdentity, _ := NewInputSnapshotIdentity(knownZero)
	if missingIdentity.Ref() == knownIdentity.Ref() {
		t.Fatal("v2 identity conflates missing age with known zero months")
	}
	legacy, _ := NewLegacyV1InputSnapshotIdentity(knownZero)
	if !strings.HasPrefix(legacy.Ref(), IdentityRefV1Prefix) {
		t.Fatalf("legacy ref = %q", legacy.Ref())
	}
}

func TestInputSnapshotIdentityChangesWithAnyComponent(t *testing.T) {
	t.Parallel()
	base, _ := NewInputSnapshotIdentity(identityFixtureSnapshot())

	mutations := map[string]func(*InputSnapshot){
		"model version":        func(s *InputSnapshot) { s.Model.Version = "1.0.1" },
		"payload format":       func(s *InputSnapshot) { s.Model.PayloadFormat = "scale/v2" },
		"questionnaire option": func(s *InputSnapshot) { s.Questionnaire.Questions[0].Options[1].Score = 2 },
		"answer score":         func(s *InputSnapshot) { s.AnswerSheet.Answers[0].Score = 3 },
		"answer value":         func(s *InputSnapshot) { s.AnswerSheet.Answers[0].Value = "a" },
		"answersheet id":       func(s *InputSnapshot) { s.AnswerSheet.ID = 2002 },
		"subject age":          func(s *InputSnapshot) { s.NormSubject.AgeMonths = ageMonthsPtr(121) },
	}
	for name, mutate := range mutations {
		snapshot := identityFixtureSnapshot()
		mutate(snapshot)
		mutated, ok := NewInputSnapshotIdentity(snapshot)
		if !ok {
			t.Fatalf("%s: identity not derivable", name)
		}
		if mutated.CompositeDigest == base.CompositeDigest {
			t.Fatalf("%s: composite digest did not change", name)
		}
	}
}

func TestInputSnapshotIdentityIgnoresAnswerOrder(t *testing.T) {
	t.Parallel()
	snapshot := identityFixtureSnapshot()
	snapshot.AnswerSheet.Answers = []AnswerSnapshot{
		{QuestionCode: "q2", Score: 2, Value: "c"},
		{QuestionCode: "q1", Score: 1, Value: "b"},
	}
	first, _ := NewInputSnapshotIdentity(snapshot)

	reordered := identityFixtureSnapshot()
	reordered.AnswerSheet.Answers = []AnswerSnapshot{
		{QuestionCode: "q1", Score: 1, Value: "b"},
		{QuestionCode: "q2", Score: 2, Value: "c"},
	}
	second, _ := NewInputSnapshotIdentity(reordered)
	if first.CompositeDigest != second.CompositeDigest {
		t.Fatal("answer order must not change the digest")
	}
}

func TestInputSnapshotIdentityRequiresComponents(t *testing.T) {
	t.Parallel()
	if _, ok := NewInputSnapshotIdentity(nil); ok {
		t.Fatal("nil snapshot must not derive identity")
	}
	if _, ok := NewInputSnapshotIdentity(&InputSnapshot{}); ok {
		t.Fatal("empty snapshot must not derive identity")
	}
}

func TestIsIdentityRefDistinguishesLegacyRefs(t *testing.T) {
	t.Parallel()
	if IsIdentityRef("model:PHQ9@1.0.0") || IsIdentityRef("answersheet:2001") || IsIdentityRef("") {
		t.Fatal("legacy refs misclassified")
	}
	if !IsIdentityRef("isn:v1:abc") {
		t.Fatal("v1 identity ref not recognized")
	}
	if !IsIdentityRef("isn:v2:abc") || !IsV1IdentityRef("isn:v1:abc") || !IsV2IdentityRef("isn:v2:abc") {
		t.Fatal("versioned identity ref not recognized")
	}
}
