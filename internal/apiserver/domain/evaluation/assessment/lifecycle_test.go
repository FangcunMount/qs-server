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

func TestDefaultAssessmentCreatorUsesEvaluationModelValidator(t *testing.T) {
	validator := &creatorModelValidatorStub{}
	creator := NewDefaultAssessmentCreator(WithEvaluationModelValidator(validator))
	modelRef := NewEvaluationModelRefByCode(EvaluationModelKindPersonality, meta.NewCode("MBTI-16P"), "1.0.0", "MBTI")
	req := NewCreateAssessmentRequest(
		1,
		testee.NewID(1001),
		NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v1"),
		NewAnswerSheetRef(meta.FromUint64(2001)),
		NewAdhocOrigin(),
	).WithEvaluationModel(modelRef)

	got, err := creator.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if got.EvaluationModelRef() == nil || got.EvaluationModelRef().Kind() != EvaluationModelKindPersonality {
		t.Fatalf("unexpected model ref: %#v", got.EvaluationModelRef())
	}
	if !validator.called || validator.modelRef.Code() != modelRef.Code() || validator.questionnaireRef.Code().String() != "q-code" {
		t.Fatalf("validator call = %v ref=%#v questionnaire=%#v", validator.called, validator.modelRef, validator.questionnaireRef)
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

type creatorModelValidatorStub struct {
	called           bool
	modelRef         EvaluationModelRef
	questionnaireRef QuestionnaireRef
	err              error
}

func (s *creatorModelValidatorStub) ValidateEvaluationModel(_ context.Context, modelRef EvaluationModelRef, questionnaireRef QuestionnaireRef) error {
	s.called = true
	s.modelRef = modelRef
	s.questionnaireRef = questionnaireRef
	return s.err
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

	outcome := NewAssessmentOutcome(
		*a.EvaluationModelRef(),
		ResultSummary{PrimaryLabel: "high risk"},
		EvaluationDetail{Kind: EvaluationModelKindScale},
	)
	outcome.Primary = &OutcomeScoreValue{Kind: OutcomeScoreKindRawTotal, Value: 18.5}
	outcome.Level = &OutcomeResultLevel{Code: string(RiskLevelHigh), Label: "high risk"}
	if err := a.ApplyScoringOutcome(outcome); err != nil {
		t.Fatalf("ApplyScoringOutcome returned error: %v", err)
	}
	if !a.Status().IsEvaluated() || !a.Status().IsTerminal() {
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
	outcome := NewAssessmentOutcome(
		modelRef,
		ResultSummary{PrimaryLabel: "scored"},
		EvaluationDetail{Kind: EvaluationModelKindScale},
	)
	outcome.Primary = &OutcomeScoreValue{Kind: OutcomeScoreKindRawTotal, Value: 12}
	if err := a.ApplyScoringOutcome(outcome); err != nil {
		t.Fatalf("ApplyScoringOutcome returned error: %v", err)
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
	event, ok := a.Events()[0].(AssessmentSubmittedEvent)
	if !ok {
		t.Fatalf("event type = %T, want AssessmentSubmittedEvent", a.Events()[0])
	}
	data := event.Payload()
	if data.ModelKind != "scale" || data.ModelCode != "s-code" || data.ModelVersion != "2.1.0" || data.ScaleCode != "" || data.ScaleVersion != "" {
		t.Fatalf("unexpected submitted event data: %#v", data)
	}
}

func TestApplyScoringOutcomeValidatesEvaluationModelRef(t *testing.T) {
	modelRef := NewEvaluationModelRefByCode(EvaluationModelKindPersonality, meta.NewCode("MBTI-16P"), "1.0.0", "MBTI")
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

	result := NewModelEvaluationResult(modelRef, ResultSummary{PrimaryLabel: "INTJ"}, EvaluationDetail{
		Kind:    EvaluationModelKindPersonality,
		Payload: "INTJ",
	})
	if err := a.ApplyScoringOutcome(AssessmentOutcomeFromEvaluationResult(result)); err != nil {
		t.Fatalf("ApplyScoringOutcome returned error: %v", err)
	}
	if !a.Status().IsEvaluated() {
		t.Fatalf("expected evaluated status, got %s", a.Status())
	}
	if result.Detail.Kind != EvaluationModelKindPersonality {
		t.Fatalf("result detail kind = %s, want mbti", result.Detail.Kind)
	}
}

func TestApplyScoringOutcomeRejectsMismatchedEvaluationModelRef(t *testing.T) {
	a, err := NewAssessment(
		1,
		testee.NewID(1006),
		NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v6"),
		NewAnswerSheetRef(meta.FromUint64(2006)),
		NewAdhocOrigin(),
		WithID(NewID(5005)),
		WithEvaluationModel(NewEvaluationModelRefByCode(EvaluationModelKindPersonality, meta.NewCode("MBTI-16P"), "1.0.0", "MBTI")),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}

	result := NewEvaluationResult(0, RiskLevelNone, "", "", nil).
		WithModelRef(NewEvaluationModelRefByCode(EvaluationModelKindScale, meta.NewCode("SDS"), "1.0.0", "SDS"))
	if err := a.ApplyScoringOutcome(AssessmentOutcomeFromEvaluationResult(result)); err != ErrEvaluationModelMismatch {
		t.Fatalf("ApplyScoringOutcome error = %v, want ErrEvaluationModelMismatch", err)
	}
}
