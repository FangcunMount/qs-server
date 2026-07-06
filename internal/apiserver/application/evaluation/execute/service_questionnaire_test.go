package execute

import (
	"context"
	"errors"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type fakeAssessmentRepo struct {
	assessment         *domainAssessment.Assessment
	saveCalls          int
	saveCtxHadTxMarker bool
}

type engineTxCtxMarker struct{}

type engineRecordingTxRunner struct {
	called bool
}

func (r *engineRecordingTxRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	r.called = true
	return fn(context.WithValue(ctx, engineTxCtxMarker{}, true))
}

type engineRecordingEventStager struct {
	ctxHadTxMarker bool
	eventTypes     []string
}

func (s *engineRecordingEventStager) Stage(ctx context.Context, events ...event.DomainEvent) error {
	s.ctxHadTxMarker, _ = ctx.Value(engineTxCtxMarker{}).(bool)
	for _, evt := range events {
		s.eventTypes = append(s.eventTypes, evt.EventType())
	}
	return nil
}

func (r *fakeAssessmentRepo) Save(ctx context.Context, assessment *domainAssessment.Assessment) error {
	r.saveCtxHadTxMarker, _ = ctx.Value(engineTxCtxMarker{}).(bool)
	r.assessment = assessment
	r.saveCalls++
	return nil
}

func (r *fakeAssessmentRepo) FindByID(_ context.Context, _ domainAssessment.ID) (*domainAssessment.Assessment, error) {
	return r.assessment, nil
}

func (r *fakeAssessmentRepo) Delete(_ context.Context, _ domainAssessment.ID) error { return nil }
func (r *fakeAssessmentRepo) FindByAnswerSheetID(_ context.Context, _ domainAssessment.AnswerSheetRef) (*domainAssessment.Assessment, error) {
	return nil, nil
}

func TestEvaluateFailsWhenQuestionnaireVersionDoesNotResolveCurrentQuestionnaire(t *testing.T) {
	aRepo := &fakeAssessmentRepo{
		assessment: domainAssessment.Reconstruct(
			meta.FromUint64(101),
			1,
			testee.NewID(202),
			domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "0.9.0"),
			domainAssessment.NewAnswerSheetRef(meta.FromUint64(303)),
			ptr(domainAssessment.NewMedicalScaleRef(meta.FromUint64(404), meta.NewCode("S-001"), "Scale")),
			domainAssessment.NewAdhocOrigin(),
			domainAssessment.StatusSubmitted,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
	}

	svc := &service{
		assessmentRepo: aRepo,
		inputResolver:  failingInputResolver{err: inputFailure{reason: "加载问卷失败: 问卷不存在或版本不匹配"}},
		txRunner:       &engineRecordingTxRunner{},
		eventStager:    &engineRecordingEventStager{},
	}

	err := svc.Evaluate(context.Background(), 101)
	if err == nil {
		t.Fatal("Evaluate() error = nil, want questionnaire version failure")
	}
	if !aRepo.assessment.Status().IsFailed() {
		t.Fatalf("assessment status = %s, want failed", aRepo.assessment.Status())
	}
	if aRepo.saveCalls == 0 {
		t.Fatal("assessment should be persisted after markAsFailed")
	}
	if !aRepo.saveCtxHadTxMarker {
		t.Fatal("assessment Save should receive transaction context")
	}
}

func TestSaveAssessmentWithEventsRequiresTransactionalOutbox(t *testing.T) {
	a := engineAssessmentForOutboxTest(t)
	if err := a.MarkAsFailed("pipeline failed"); err != nil {
		t.Fatalf("MarkAsFailed returned error: %v", err)
	}
	repo := &fakeAssessmentRepo{}
	finalizer := evaluationFailureFinalizer{repo: repo}

	err := finalizer.SaveAssessmentWithEvents(context.Background(), a)
	if err == nil {
		t.Fatal("expected missing transactional outbox dependencies to fail")
	}
	if repo.saveCalls != 0 {
		t.Fatalf("repository save calls = %d, want 0", repo.saveCalls)
	}
}

func TestMapInputResolveErrorPreservesExternalAPICodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		kind evaluationinput.FailureKind
		code int
	}{
		{name: "scale", kind: evaluationinput.FailureKindScaleNotFound, code: errorCode.ErrMedicalScaleNotFound},
		{name: "answer sheet", kind: evaluationinput.FailureKindAnswerSheetNotFound, code: errorCode.ErrAnswerSheetNotFound},
		{name: "questionnaire", kind: evaluationinput.FailureKindQuestionnaireNotFound, code: errorCode.ErrQuestionnaireNotFound},
		{name: "questionnaire version", kind: evaluationinput.FailureKindQuestionnaireVersionMismatch, code: errorCode.ErrQuestionnaireNotFound},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := evaluationinput.NewResolveError(tc.kind, errors.New("missing"), "missing", "load failed")
			mapped := mapInputResolveError(err)
			if !cberrors.IsCode(mapped, tc.code) {
				t.Fatalf("mapped code = %d, want %d", cberrors.ParseCoder(mapped).Code(), tc.code)
			}
			var reason evaluationinput.FailureReasonCarrier
			if !errors.As(mapped, &reason) || reason.FailureReason() != "load failed: missing" {
				t.Fatalf("mapped error should preserve failure reason, got %v", mapped)
			}
		})
	}
}

func TestSaveAssessmentWithEventsStagesThroughApplicationTransaction(t *testing.T) {
	a := engineAssessmentForOutboxTest(t)
	if err := a.MarkAsFailed("pipeline failed"); err != nil {
		t.Fatalf("MarkAsFailed returned error: %v", err)
	}
	repo := &fakeAssessmentRepo{}
	txRunner := &engineRecordingTxRunner{}
	stager := &engineRecordingEventStager{}
	finalizer := evaluationFailureFinalizer{repo: repo, txRunner: txRunner, eventStager: stager}

	if err := finalizer.SaveAssessmentWithEvents(context.Background(), a); err != nil {
		t.Fatalf("SaveAssessmentWithEvents returned error: %v", err)
	}
	if !txRunner.called {
		t.Fatal("expected transaction runner to be used")
	}
	if repo.saveCalls != 1 {
		t.Fatalf("repository save calls = %d, want 1", repo.saveCalls)
	}
	if !repo.saveCtxHadTxMarker {
		t.Fatal("assessment Save should receive transaction context")
	}
	if !stager.ctxHadTxMarker {
		t.Fatal("event stager should receive transaction context")
	}
	if len(stager.eventTypes) != 1 || stager.eventTypes[0] != domainAssessment.EventTypeFailed {
		t.Fatalf("staged event types = %#v, want assessment failed", stager.eventTypes)
	}
	if len(a.Events()) != 0 {
		t.Fatalf("expected events to be cleared after successful transaction, got %d", len(a.Events()))
	}
}

func engineAssessmentForOutboxTest(t *testing.T) *domainAssessment.Assessment {
	t.Helper()
	return domainAssessment.Reconstruct(
		meta.FromUint64(9901),
		1,
		testee.NewID(202),
		domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "0.9.0"),
		domainAssessment.NewAnswerSheetRef(meta.FromUint64(303)),
		ptr(domainAssessment.NewMedicalScaleRef(meta.FromUint64(404), meta.NewCode("S-001"), "Scale")),
		domainAssessment.NewAdhocOrigin(),
		domainAssessment.StatusSubmitted,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}

type recordingResultWriter struct {
	calls   int
	outcome evaloutcome.Outcome
}

func (w *recordingResultWriter) Write(_ context.Context, outcome evaloutcome.Outcome) error {
	w.calls++
	w.outcome = outcome
	return nil
}

type failingInputResolver struct {
	err error
}

func (r failingInputResolver) Resolve(context.Context, evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	return nil, r.err
}

type successfulInputResolver struct {
	snapshot *evaluationinput.InputSnapshot
	lastRef  evaluationinput.InputRef
	calls    int
}

func (r *successfulInputResolver) Resolve(_ context.Context, ref evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	r.calls++
	r.lastRef = ref
	return r.snapshot, nil
}

type inputFailure struct {
	reason string
}

func (e inputFailure) Error() string {
	return e.reason
}

func (e inputFailure) FailureReason() string {
	return e.reason
}

func ptr[T any](v T) *T {
	return &v
}


func TestEvaluateDispatchesScaleModelToScaleEvaluator(t *testing.T) {
	scaleRef := domainAssessment.NewMedicalScaleRef(meta.FromUint64(404), meta.NewCode("S-001"), "Scale")
	aRepo := &fakeAssessmentRepo{
		assessment: domainAssessment.Reconstruct(
			meta.FromUint64(101),
			1,
			testee.NewID(202),
			domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
			domainAssessment.NewAnswerSheetRef(meta.FromUint64(303)),
			&scaleRef,
			domainAssessment.NewAdhocOrigin(),
			domainAssessment.StatusSubmitted,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
	}
	input := &successfulInputResolver{snapshot: &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:  evaluationinput.EvaluationModelKindScale,
			Code:  "S-001",
			Title: "Scale",
		},
		MedicalScale:  &scalesnapshot.ScaleSnapshot{Code: "S-001", Title: "Scale"},
		AnswerSheet:   &evaluationinput.AnswerSheetSnapshot{ID: 303, QuestionnaireCode: "Q-001", QuestionnaireVersion: "1.0.0"},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-001", Version: "1.0.0"},
	}}
	capture := &splitPhaseCapture{}
	var executionInput ExecutionInput
	registry, err := NewEvaluatorRegistry(evaluatorStub{
		key: evaluation.EvaluatorKeyScaleDefault,
		execute: func(ctx context.Context, input ExecutionInput) (*domainAssessment.AssessmentOutcome, error) {
			executionInput = input
			modelRef := *input.Assessment.EvaluationModelRef()
			score := 7.0
			level := string(domainAssessment.RiskLevelLow)
			outcome := domainAssessment.NewAssessmentOutcome(
				modelRef,
				domainAssessment.ResultSummary{
					PrimaryLabel: "ok",
					Score:        &score,
					Level:        &level,
				},
				domainAssessment.EvaluationDetail{Kind: domainAssessment.EvaluationModelKindScale},
			)
			outcome.Primary = &domainAssessment.OutcomeScoreValue{
				Kind:  domainAssessment.OutcomeScoreKindRawTotal,
				Value: score,
			}
			outcome.Level = &domainAssessment.OutcomeResultLevel{Code: level, Label: "ok"}
			return outcome, nil
		},
	})
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry returned error: %v", err)
	}
	svc := newSplitPhaseTestService(aRepo, input, capture, WithEvaluatorRegistry(registry))

	if err := svc.Evaluate(context.Background(), 101); err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if executionInput.Assessment != aRepo.assessment || executionInput.Input != input.snapshot {
		t.Fatalf("unexpected executor input: %#v", executionInput)
	}
	if capture.InterpretationCalls != 1 || evaloutcome.LegacyResult(capture.Outcome) == nil || evaloutcome.LegacyResult(capture.Outcome).TotalScore != 7 {
		t.Fatalf("unexpected interpretation outcome: %#v", capture.Outcome)
	}
	if input.calls != 1 || input.lastRef.ModelRef.Kind != evaluationinput.EvaluationModelKindScale || input.lastRef.ModelRef.Code != "S-001" {
		t.Fatalf("unexpected input ref: %#v", input.lastRef)
	}
}

func TestEvaluateDispatchesNonScaleModelThroughRegistry(t *testing.T) {
	modelRef := domainAssessment.NewEvaluationModelRefWithIdentity(
		domainAssessment.EvaluationModelKindPersonality,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmMBTI,
		meta.ID(0),
		meta.NewCode("FAKE-MODEL"),
		"1.0.0",
		"Fake Model",
	)
	aRepo := &fakeAssessmentRepo{
		assessment: domainAssessment.Reconstruct(
			meta.FromUint64(103),
			1,
			testee.NewID(202),
			domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-FAKE"), "1.0.0"),
			domainAssessment.NewAnswerSheetRef(meta.FromUint64(305)),
			nil,
			domainAssessment.NewAdhocOrigin(),
			domainAssessment.StatusSubmitted,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			&modelRef,
		),
	}
	input := &successfulInputResolver{snapshot: &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:      evaluationinput.EvaluationModelKindPersonality,
			SubKind:   string(modelcatalog.SubKindTypology),
			Algorithm: string(modelcatalog.AlgorithmMBTI),
			Code:      "FAKE-MODEL",
			Version:   "1.0.0",
			Title:     "Fake Model",
		},
		AnswerSheet:   &evaluationinput.AnswerSheetSnapshot{ID: 305, QuestionnaireCode: "Q-FAKE", QuestionnaireVersion: "1.0.0"},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-FAKE", Version: "1.0.0"},
	}}
	capture := &splitPhaseCapture{}
	registry, err := NewEvaluatorRegistry(evaluatorStub{
		key: evaluation.EvaluatorKeyMBTI,
		execute: func(ctx context.Context, input ExecutionInput) (*domainAssessment.AssessmentOutcome, error) {
			modelRef := *input.Assessment.EvaluationModelRef()
			outcome := domainAssessment.NewAssessmentOutcome(
				modelRef,
				domainAssessment.ResultSummary{PrimaryLabel: "INTJ"},
				domainAssessment.EvaluationDetail{
					Kind:    domainAssessment.EvaluationModelKindPersonality,
					Payload: "INTJ",
				},
			)
			outcome.Level = &domainAssessment.OutcomeResultLevel{
				Code:     "INTJ",
				Label:    "INTJ",
				Severity: "none",
			}
			return outcome, nil
		},
	})
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry returned error: %v", err)
	}
	svc := newSplitPhaseTestService(aRepo, input, capture, WithEvaluatorRegistry(registry))

	if err := svc.Evaluate(context.Background(), 103); err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if capture.InterpretationCalls != 1 || evaloutcome.LegacyResult(capture.Outcome) == nil || evaloutcome.LegacyResult(capture.Outcome).ModelRef.Kind() != domainAssessment.EvaluationModelKindPersonality {
		t.Fatalf("unexpected interpretation outcome: %#v", capture.Outcome)
	}
	if input.lastRef.ModelRef.Kind != evaluationinput.EvaluationModelKindPersonality || input.lastRef.ModelRef.Code != "FAKE-MODEL" {
		t.Fatalf("unexpected input ref: %#v", input.lastRef)
	}
}

func TestEvaluateUnknownRuleSetKindMarksAssessmentFailed(t *testing.T) {
	modelRef := domainAssessment.NewEvaluationModelRefByCode(domainAssessment.EvaluationModelKindPersonality, meta.NewCode("MBTI-16P"), "1.0.0", "MBTI")
	aRepo := &fakeAssessmentRepo{
		assessment: domainAssessment.Reconstruct(
			meta.FromUint64(102),
			1,
			testee.NewID(202),
			domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-MBTI"), "1.0.0"),
			domainAssessment.NewAnswerSheetRef(meta.FromUint64(304)),
			nil,
			domainAssessment.NewAdhocOrigin(),
			domainAssessment.StatusSubmitted,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			&modelRef,
		),
	}
	input := &successfulInputResolver{snapshot: &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:    evaluationinput.EvaluationModelKindPersonality,
			Code:    "MBTI-16P",
			Version: "1.0.0",
			Title:   "MBTI",
		},
		AnswerSheet:   &evaluationinput.AnswerSheetSnapshot{ID: 304, QuestionnaireCode: "Q-MBTI", QuestionnaireVersion: "1.0.0"},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-MBTI", Version: "1.0.0"},
	}}
	capture := &splitPhaseCapture{}
	txRunner := &engineRecordingTxRunner{}
	stager := &engineRecordingEventStager{}
	registry, registryErr := NewEvaluatorRegistry(evaluatorStub{key: evaluation.EvaluatorKeyScaleDefault})
	if registryErr != nil {
		t.Fatalf("NewEvaluatorRegistry returned error: %v", registryErr)
	}
	svc := newSplitPhaseTestService(
		aRepo,
		input,
		capture,
		WithTransactionalOutbox(txRunner, stager),
		WithEvaluatorRegistry(registry),
	)

	err := svc.Evaluate(context.Background(), 102)
	if err == nil {
		t.Fatal("Evaluate error = nil, want unsupported model kind")
	}
	if !aRepo.assessment.Status().IsFailed() {
		t.Fatalf("assessment status = %s, want failed", aRepo.assessment.Status())
	}
	if aRepo.saveCalls != 1 || !txRunner.called {
		t.Fatalf("failure finalizer did not persist through transaction: saveCalls=%d tx=%v", aRepo.saveCalls, txRunner.called)
	}
	if len(stager.eventTypes) != 1 || stager.eventTypes[0] != domainAssessment.EventTypeFailed {
		t.Fatalf("staged event types = %#v, want assessment failed", stager.eventTypes)
	}
}
