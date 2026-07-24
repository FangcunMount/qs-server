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
	const historicalGolden = "bee73c4e4ee1faf7e7dea607c9863b2154b8efc2f14f8035707fe8042c888326"
	if leftFingerprint != historicalGolden {
		t.Fatalf("fingerprint = %s, want historical golden %s", leftFingerprint, historicalGolden)
	}
}

func TestFingerprintChangesWithBusinessContent(t *testing.T) {
	left, _ := Fingerprint(fingerprintTestSheet(t, 1, []string{"Q1"}, []string{"a"}))
	right, _ := Fingerprint(fingerprintTestSheet(t, 2, []string{"Q1"}, []string{"b"}))
	if left == right {
		t.Fatal("different answers must have different fingerprints")
	}
}

func TestFingerprintIntentMatchesAnswerSheetFingerprint(t *testing.T) {
	sheet := fingerprintTestSheet(t, 1, []string{"Q2", "Q1"}, []string{"b", "a"})
	fromSheet, err := Fingerprint(sheet)
	if err != nil {
		t.Fatal(err)
	}
	fromIntent, err := FingerprintIntent(SubmissionIntent{
		WriterID:             11,
		TesteeID:             22,
		OrgID:                33,
		TaskID:               "task",
		QuestionnaireCode:    "QNR",
		QuestionnaireVersion: "1",
		Answers: []SubmissionAnswer{
			{QuestionCode: "Q1", QuestionType: "Text", Value: "a"},
			{QuestionCode: "Q2", QuestionType: "Text", Value: "b"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fromIntent != fromSheet {
		t.Fatalf("intent fingerprint = %s, sheet fingerprint = %s", fromIntent, fromSheet)
	}
}

func TestFingerprintIntentCoversOriginTaskNumericAndMultiSelect(t *testing.T) {
	base := SubmissionIntent{
		WriterID:             11,
		TesteeID:             22,
		OrgID:                33,
		TaskID:               "task-1",
		OriginType:           "plan_task",
		OriginID:             "task-1",
		QuestionnaireCode:    "QNR",
		QuestionnaireVersion: "1",
		Answers: []SubmissionAnswer{
			{QuestionCode: "Q2", QuestionType: "Checkbox", Value: []string{"A", "B"}},
			{QuestionCode: "Q1", QuestionType: "Number", Value: 3.5},
		},
	}
	reordered := base
	reordered.Answers = []SubmissionAnswer{base.Answers[1], base.Answers[0]}

	baseFingerprint, err := FingerprintIntent(base)
	if err != nil {
		t.Fatal(err)
	}
	reorderedFingerprint, err := FingerprintIntent(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if baseFingerprint != reorderedFingerprint {
		t.Fatalf("fingerprints differ by answer order: %s != %s", baseFingerprint, reorderedFingerprint)
	}

	changedTask := base
	changedTask.TaskID = "task-2"
	changedTaskFingerprint, _ := FingerprintIntent(changedTask)
	if changedTaskFingerprint == baseFingerprint {
		t.Fatal("task_id must participate in the fingerprint")
	}

	changedOrigin := base
	changedOrigin.OriginID = "task-2"
	changedOriginFingerprint, _ := FingerprintIntent(changedOrigin)
	if changedOriginFingerprint == baseFingerprint {
		t.Fatal("origin_ref must participate in the fingerprint")
	}

	changedMultiSelect := base
	changedMultiSelect.Answers = append([]SubmissionAnswer(nil), base.Answers...)
	changedMultiSelect.Answers[0].Value = []string{"B", "A"}
	changedMultiSelectFingerprint, _ := FingerprintIntent(changedMultiSelect)
	if changedMultiSelectFingerprint == baseFingerprint {
		t.Fatal("multi-select value order must preserve the historical fingerprint semantics")
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
