package assessment

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestDefaultAssessmentCreatorCreateKeepsPendingByDefault(t *testing.T) {
	creator := NewDefaultAssessmentCreator()
	req := NewCreateAssessmentRequest(
		1,
		testee.NewID(1001),
		NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v1"),
		NewAnswerSheetRef(meta.FromUint64(2001)),
		NewAdhocOrigin(),
	)

	got, err := creator.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
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
	if a.Events()[0].EventType() != EventTypeSubmitted {
		t.Fatalf("expected submitted event after retry, got %s", a.Events()[0].EventType())
	}
}

func TestApplyEvaluationDoesNotEmitInterpretedEventAndAllowsFailover(t *testing.T) {
	a, err := NewAssessment(
		1,
		testee.NewID(1003),
		NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v3"),
		NewAnswerSheetRef(meta.FromUint64(2003)),
		NewAdhocOrigin(),
		WithID(NewID(5002)),
		WithMedicalScale(NewMedicalScaleRef(meta.FromUint64(3001), meta.NewCode("s-code"), "scale")),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}

	if err := a.Submit(); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	a.ClearEvents()

	result := NewEvaluationResult(
		18.5,
		RiskLevelHigh,
		"high risk",
		"follow up",
		nil,
	)
	if err := a.ApplyEvaluation(result); err != nil {
		t.Fatalf("ApplyEvaluation returned error: %v", err)
	}
	if !a.Status().IsInterpreted() {
		t.Fatalf("expected interpreted status, got %s", a.Status())
	}
	if len(a.Events()) != 0 {
		t.Fatalf("expected apply evaluation to not emit events, got %d", len(a.Events()))
	}

	if err := a.MarkAsFailed("report save failed"); err != nil {
		t.Fatalf("MarkAsFailed after interpretation returned error: %v", err)
	}
	if !a.Status().IsFailed() {
		t.Fatalf("expected failed status after failover, got %s", a.Status())
	}
	if a.InterpretedAt() != nil {
		t.Fatalf("expected interpreted_at to be cleared after failover")
	}
	if a.TotalScore() != nil {
		t.Fatalf("expected total_score to be cleared after failover")
	}
	if a.RiskLevel() != nil {
		t.Fatalf("expected risk_level to be cleared after failover")
	}
	if len(a.Events()) != 1 {
		t.Fatalf("expected one failed event after failover, got %d", len(a.Events()))
	}
	if a.Events()[0].EventType() != EventTypeFailed {
		t.Fatalf("expected failed event after failover, got %s", a.Events()[0].EventType())
	}
}
