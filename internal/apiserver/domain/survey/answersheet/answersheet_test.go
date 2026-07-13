package answersheet

import (
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
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

func TestAnswerSheetEventsReturnsCopy(t *testing.T) {
	t.Parallel()

	sheet, err := Submit(
		meta.FromUint64(1001),
		mustQuestionnaireRef(t),
		mustSubmissionContext(t),
		[]Answer{mustAnswer(t)},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}

	events := sheet.Events()
	events[0] = event.New("mutated", "AnswerSheet", "1001", map[string]string{})

	if got := sheet.Events()[0].EventType(); got != EventTypeSubmitted {
		t.Fatalf("stored event type = %q, want %s", got, EventTypeSubmitted)
	}
}

func TestSubmissionContextCopiesActorRefsAtBoundaries(t *testing.T) {
	t.Parallel()

	filler := actor.NewFillerRef(301, actor.FillerTypeSelf)
	testee := actor.NewTesteeRefWithProfile(meta.FromUint64(401), 901)
	ctx, err := NewSubmissionContext(filler, testee, meta.FromUint64(501), "task-1")
	if err != nil {
		t.Fatalf("NewSubmissionContext() error = %v", err)
	}
	if ctx.filler == filler || ctx.testee == testee {
		t.Fatalf("SubmissionContext reused input refs")
	}
	if ctx.Filler() == ctx.filler || ctx.Testee() == ctx.testee {
		t.Fatalf("SubmissionContext getter exposed internal refs")
	}
	fillerA, fillerB := ctx.Filler(), ctx.Filler()
	testeeA, testeeB := ctx.Testee(), ctx.Testee()
	if fillerA == fillerB || testeeA == testeeB {
		t.Fatalf("SubmissionContext getter returned reusable refs")
	}

	reconstructed := ReconstructSubmissionContext(filler, testee, meta.FromUint64(501), "task-1")
	if reconstructed.filler == filler || reconstructed.testee == testee {
		t.Fatalf("ReconstructSubmissionContext reused input refs")
	}

	sheet, err := Submit(
		meta.FromUint64(1001),
		mustQuestionnaireRef(t),
		ctx,
		[]Answer{mustAnswer(t)},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if sheet.submissionContext.filler == ctx.filler || sheet.submissionContext.testee == ctx.testee {
		t.Fatalf("Submit reused submission context refs")
	}
	gotCtx := sheet.SubmissionContext()
	if gotCtx.filler == sheet.submissionContext.filler || gotCtx.testee == sheet.submissionContext.testee {
		t.Fatalf("AnswerSheet.SubmissionContext exposed internal refs")
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

func TestAnswerValueAdapterAsArrayReturnsCopy(t *testing.T) {
	t.Parallel()

	source := []string{"A"}
	adapter := NewAnswerValueAdapter(mutableArrayAnswerValue{values: source})

	got := adapter.AsArray()
	got[0] = "B"

	if again := adapter.AsArray(); again[0] != "A" {
		t.Fatalf("AsArray() after caller mutation = %q, want A", again[0])
	}
}

type mutableArrayAnswerValue struct {
	values []string
}

func (v mutableArrayAnswerValue) Raw() any {
	return v.values
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
