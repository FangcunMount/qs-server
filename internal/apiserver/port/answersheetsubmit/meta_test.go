package answersheetsubmit

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestFingerprintIsStableAcrossAnswerOrderAndGeneratedFields(t *testing.T) {
	left := fingerprintTestSheet(t, 1, []string{"Q1", "Q2"}, []string{"a", "b"})
	right := fingerprintTestSheet(t, 2, []string{"Q2", "Q1"}, []string{"b", "a"})
	leftFingerprint, err := Fingerprint(left)
	if err != nil {
		t.Fatal(err)
	}
	rightFingerprint, err := Fingerprint(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftFingerprint != rightFingerprint {
		t.Fatalf("fingerprints differ by answer order/id: %s != %s", leftFingerprint, rightFingerprint)
	}
}

func TestFingerprintChangesWithBusinessContent(t *testing.T) {
	left, _ := Fingerprint(fingerprintTestSheet(t, 1, []string{"Q1"}, []string{"a"}))
	right, _ := Fingerprint(fingerprintTestSheet(t, 2, []string{"Q1"}, []string{"b"}))
	if left == right {
		t.Fatal("different answers must have different fingerprints")
	}
}

func fingerprintTestSheet(t *testing.T, id uint64, codes, values []string) *domainanswersheet.AnswerSheet {
	t.Helper()
	ref, err := domainanswersheet.NewQuestionnaireRef("QNR", "1", "Questionnaire")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := domainanswersheet.NewSubmissionContext(actor.NewFillerRef(11, actor.FillerTypeSelf), actor.NewTesteeRef(meta.FromUint64(22)), meta.FromUint64(33), "task")
	if err != nil {
		t.Fatal(err)
	}
	answers := make([]domainanswersheet.Answer, 0, len(codes))
	for index, code := range codes {
		answer, err := domainanswersheet.NewAnswer(meta.NewCode(code), questionnaire.TypeText, domainanswersheet.NewStringValue(values[index]), 0)
		if err != nil {
			t.Fatal(err)
		}
		answers = append(answers, answer)
	}
	sheet, err := domainanswersheet.Submit(meta.FromUint64(id), ref, ctx, answers, time.Unix(int64(id), 0))
	if err != nil {
		t.Fatal(err)
	}
	return sheet
}
