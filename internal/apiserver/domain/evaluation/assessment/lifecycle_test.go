package assessment

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestNewAssessmentKeepsPendingByDefault(t *testing.T) {
	got, err := NewAssessment(
		1,
		testee.NewID(1001),
		NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v1"),
		NewAnswerSheetRef(meta.FromUint64(2001)),
		NewAdhocOrigin(),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}

	if !got.Status().IsPending() {
		t.Fatalf("expected created assessment to stay pending, got %s", got.Status())
	}
	if got.SubmittedAt() != nil {
		t.Fatalf("expected created assessment to have no submitted_at")
	}
	if len(got.Events()) != 0 {
		t.Fatalf("expected create to not emit events, got %d", len(got.Events()))
	}
}

func TestAssessmentFailedAndRetryLifecycleEvents(t *testing.T) {
	a, err := NewAssessment(
		1,
		testee.NewID(1002),
		NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v2"),
		NewAnswerSheetRef(meta.FromUint64(2002)),
		NewAdhocOrigin(),
		WithID(NewID(5001)),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}

	if err := a.Submit(); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	a.ClearEvents()

	if err := a.MarkAsFailed("pipeline failed"); err != nil {
		t.Fatalf("MarkAsFailed returned error: %v", err)
	}
	if !a.Status().IsFailed() {
		t.Fatalf("expected failed status, got %s", a.Status())
	}
	if len(a.Events()) != 1 {
		t.Fatalf("expected one failed event, got %d", len(a.Events()))
	}
	if a.Events()[0].EventType() != EventTypeFailed {
		t.Fatalf("expected failed event, got %s", a.Events()[0].EventType())
	}

	a.ClearEvents()

	if err := a.RetryFromFailed(); err != nil {
		t.Fatalf("RetryFromFailed returned error: %v", err)
	}
	if !a.Status().IsSubmitted() {
		t.Fatalf("expected submitted status after retry, got %s", a.Status())
	}
	if len(a.Events()) != 1 {
		t.Fatalf("expected one submitted event after retry, got %d", len(a.Events()))
	}
	if a.Events()[0].EventType() != EventTypeRequested {
		t.Fatalf("expected requested event after retry, got %s", a.Events()[0].EventType())
	}
}

func TestResumeForExecutionRetryDoesNotEmitDuplicateRequestedEvent(t *testing.T) {
	a, err := NewAssessment(
		1,
		testee.NewID(1002),
		NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v2"),
		NewAnswerSheetRef(meta.FromUint64(2002)),
		NewAdhocOrigin(),
		WithID(NewID(5001)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Submit(); err != nil {
		t.Fatal(err)
	}
	a.ClearEvents()
	if err := a.MarkAsFailed("retryable failure"); err != nil {
		t.Fatal(err)
	}
	a.ClearEvents()

	if err := a.ResumeForExecutionRetry(); err != nil {
		t.Fatal(err)
	}
	if !a.Status().IsSubmitted() || a.FailedAt() != nil || a.FailureReason() != nil {
		t.Fatalf("resumed assessment = status:%s failed_at:%v reason:%v", a.Status(), a.FailedAt(), a.FailureReason())
	}
	if len(a.Events()) != 0 {
		t.Fatalf("resume emitted duplicate events: %#v", a.Events())
	}
}

func TestEvaluatedAssessmentIsTerminalAndRejectsFailureRewrite(t *testing.T) {
	a, err := NewAssessment(
		1,
		testee.NewID(1003),
		NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v3"),
		NewAnswerSheetRef(meta.FromUint64(2003)),
		NewAdhocOrigin(),
		WithID(NewID(5002)),
		WithEvaluationModel(NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("s-code"), "", "scale")),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}

	if err := a.Submit(); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	a.ClearEvents()

	score := 18.5
	if err := a.ApplyScoringProjectionAt(ScoringProjection{
		ModelRef: *a.EvaluationModelRef(), Summary: ResultSummary{PrimaryLabel: "high risk"},
		Score: &score, Level: string(RiskLevelHigh),
	}, time.Unix(100, 0)); err != nil {
		t.Fatalf("ApplyScoringProjectionAt returned error: %v", err)
	}
	if !a.Status().IsEvaluated() {
		t.Fatalf("expected terminal evaluated status, got %s", a.Status())
	}
	if err := a.MarkAsFailed("report save failed"); err == nil {
		t.Fatal("MarkAsFailed from evaluated must be rejected")
	}
	if !a.Status().IsEvaluated() || a.TotalScore() == nil || *a.TotalScore() != 18.5 {
		t.Fatalf("report failure rewrote evaluation facts: status=%s score=%v", a.Status(), a.TotalScore())
	}
}

func TestMarkAsFailedFromEvaluatedStatus(t *testing.T) {
	a, err := NewAssessment(
		1,
		testee.NewID(1004),
		NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v4"),
		NewAnswerSheetRef(meta.FromUint64(2004)),
		NewAdhocOrigin(),
		WithID(NewID(5003)),
		WithEvaluationModel(NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("s-code"), "", "scale")),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	modelRef := *a.EvaluationModelRef()
	score := 12.0
	if err := a.ApplyScoringProjectionAt(ScoringProjection{ModelRef: modelRef, Summary: ResultSummary{PrimaryLabel: "scored"}, Score: &score}, time.Unix(100, 0)); err != nil {
		t.Fatalf("ApplyScoringProjectionAt returned error: %v", err)
	}
	if !a.Status().IsEvaluated() {
		t.Fatalf("expected evaluated status, got %s", a.Status())
	}
	a.ClearEvents()

	if err := a.MarkAsFailed("report generation failed"); err == nil {
		t.Fatal("MarkAsFailed from evaluated must be rejected")
	}
	if !a.Status().IsEvaluated() || len(a.Events()) != 0 {
		t.Fatalf("evaluated facts changed: status=%s events=%#v", a.Status(), a.Events())
	}
}

func TestWithEvaluationModelBindsScaleIdentity(t *testing.T) {
	model := NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("s-code"), "2.1.0", "scale title")
	a, err := NewAssessment(
		1,
		testee.NewID(1004),
		NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v4"),
		NewAnswerSheetRef(meta.FromUint64(2004)),
		NewAdhocOrigin(),
		WithID(NewID(5003)),
		WithEvaluationModel(model),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}

	modelRef := a.EvaluationModelRef()
	if modelRef == nil {
		t.Fatal("expected evaluation model ref")
	} else if modelRef.Kind() != EvaluationModelKindScale || modelRef.Code() != model.Code() || modelRef.Title() != model.Title() || modelRef.Version() != "2.1.0" {
		t.Fatalf("unexpected model ref: %#v", modelRef)
	}

	if err := a.Submit(); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	event, ok := a.Events()[0].(EvaluationRequestedEvent)
	if !ok {
		t.Fatalf("event type = %T, want EvaluationRequestedEvent", a.Events()[0])
	}
	data := event.Payload()
	if data.ModelKind != "scale" || data.ModelCode != "s-code" || data.ModelVersion != "2.1.0" {
		t.Fatalf("unexpected submitted event data: %#v", data)
	}
}

func TestApplyScoringProjectionValidatesEvaluationModelRef(t *testing.T) {
	modelRef := NewEvaluationModelRefByCode(EvaluationModelKindTypology, meta.NewCode("MBTI-16P"), "1.0.0", "MBTI")
	a, err := NewAssessment(
		1,
		testee.NewID(1005),
		NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v5"),
		NewAnswerSheetRef(meta.FromUint64(2005)),
		NewAdhocOrigin(),
		WithID(NewID(5004)),
		WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}

	if err := a.ApplyScoringProjectionAt(ScoringProjection{ModelRef: modelRef, Summary: ResultSummary{PrimaryLabel: "INTJ"}}, time.Unix(100, 0)); err != nil {
		t.Fatalf("ApplyScoringProjectionAt returned error: %v", err)
	}
	if !a.Status().IsEvaluated() {
		t.Fatalf("expected evaluated status, got %s", a.Status())
	}
}

func TestApplyScoringProjectionRejectsMismatchedEvaluationModelRef(t *testing.T) {
	a, err := NewAssessment(
		1,
		testee.NewID(1006),
		NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v6"),
		NewAnswerSheetRef(meta.FromUint64(2006)),
		NewAdhocOrigin(),
		WithID(NewID(5005)),
		WithEvaluationModel(NewEvaluationModelRefByCode(EvaluationModelKindTypology, meta.NewCode("MBTI-16P"), "1.0.0", "MBTI")),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}

	projection := ScoringProjection{ModelRef: NewEvaluationModelRefByCode(EvaluationModelKindScale, meta.NewCode("SDS"), "1.0.0", "SDS")}
	if err := a.ApplyScoringProjectionAt(projection, time.Unix(100, 0)); err != ErrEvaluationModelMismatch {
		t.Fatalf("ApplyScoringProjectionAt error = %v, want ErrEvaluationModelMismatch", err)
	}
}
