package answersheet

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestNewQuestionnaireRefRejectsMissingVersion(t *testing.T) {
	t.Parallel()

	if _, err := NewQuestionnaireRef("QNR-1", "", "Questionnaire"); err == nil {
		t.Fatal("NewQuestionnaireRef() error = nil, want missing version error")
	}
}

func TestSubmitRequiresIDAndCompleteSubmissionContext(t *testing.T) {
	t.Parallel()

	ref := mustQuestionnaireRef(t)
	answers := []Answer{mustAnswer(t)}
	if _, err := Submit(meta.ZeroID, ref, mustSubmissionContext(t), answers, time.Now()); err == nil {
		t.Fatal("Submit() error = nil, want missing id error")
	}
	if _, err := Submit(meta.FromUint64(1), ref, SubmissionContext{}, answers, time.Now()); err == nil {
		t.Fatal("Submit() error = nil, want missing submission context error")
	}
}

func TestSubmitRaisesSubmittedEventWithSubmissionContext(t *testing.T) {
	t.Parallel()

	submittedAt := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	sheet, err := Submit(
		meta.FromUint64(1001),
		mustQuestionnaireRef(t),
		mustSubmissionContext(t),
		[]Answer{mustAnswer(t)},
		submittedAt,
	)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	events := sheet.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	evt, ok := events[0].(AnswerSheetSubmittedEvent)
	if !ok {
		t.Fatalf("event type = %T, want AnswerSheetSubmittedEvent", events[0])
	}
	payload := evt.Payload()
	if payload.AnswerSheetID != "1001" ||
		payload.QuestionnaireCode != "QNR-1" ||
		payload.QuestionnaireVersion != "1.0.0" ||
		payload.FillerID != 301 ||
		payload.FillerType != actor.FillerTypeSelf.String() ||
		payload.TesteeID != 401 ||
		payload.OrgID != 501 ||
		payload.TaskID != "task-1" ||
		!payload.SubmittedAt.Equal(submittedAt) {
		t.Fatalf("submitted payload = %+v", payload)
	}
}

func TestOptionsValueClonesInputAndRawOutput(t *testing.T) {
	t.Parallel()

	source := []string{"A"}
	value := NewOptionsValue(source)
	source[0] = "B"
	raw := value.Raw().([]string)
	if raw[0] != "A" {
		t.Fatalf("raw value after source mutation = %q, want A", raw[0])
	}
	raw[0] = "C"
	rawAgain := value.Raw().([]string)
	if rawAgain[0] != "A" {
		t.Fatalf("raw value after output mutation = %q, want A", rawAgain[0])
	}
}

func mustQuestionnaireRef(t *testing.T) QuestionnaireRef {
	t.Helper()
	ref, err := NewQuestionnaireRef("QNR-1", "1.0.0", "Questionnaire")
	if err != nil {
		t.Fatalf("NewQuestionnaireRef() error = %v", err)
	}
	return ref
}

func mustSubmissionContext(t *testing.T) SubmissionContext {
	t.Helper()
	ctx, err := NewSubmissionContext(
		actor.NewFillerRef(301, actor.FillerTypeSelf),
		actor.NewTesteeRef(meta.FromUint64(401)),
		meta.FromUint64(501),
		"task-1",
	)
	if err != nil {
		t.Fatalf("NewSubmissionContext() error = %v", err)
	}
	return ctx
}

func mustAnswer(t *testing.T) Answer {
	t.Helper()
	answer, err := NewAnswer(meta.NewCode("Q1"), questionnaire.TypeRadio, NewOptionValue("A"), 0)
	if err != nil {
		t.Fatalf("NewAnswer() error = %v", err)
	}
	return answer
}
